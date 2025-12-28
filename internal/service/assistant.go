package service

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"synapse-backend/internal/config"
	"synapse-backend/internal/model"
	"synapse-backend/pkg/logger"
)

// ToolFunc 工具函数类型
type ToolFunc func(args map[string]interface{}) (string, error)

// AssistantService 助手服务
type AssistantService struct {
	tasks        map[string]*ChatTask
	taskOrder    []string // 用于 FIFO 淘汰（仅记录活跃任务插入顺序）
	totalSize    int64    // 所有任务消息的估算总字节数
	sizeLimit    int64    // 最大缓冲 100MB
	tasksMux     sync.RWMutex
	toolRegistry map[string]ToolFunc // 工具注册表
}

// ChatTask 聊天任务
type ChatTask struct {
	sync.Mutex
	ID        string              `json:"id"`
	Messages  []model.ChatMessage `json:"messages"`
	Model     string              `json:"model"`
	Stream    chan string         `json:"-"`
	IsActive  bool                `json:"is_active"`
	CreatedAt time.Time           `json:"created_at"`
	size      int64               // 本任务消息占用的字节数（估算）
}

// NewAssistantService 创建助手服务
func NewAssistantService() *AssistantService {
	s := &AssistantService{
		tasks:        make(map[string]*ChatTask),
		taskOrder:    make([]string, 0),
		totalSize:    0,
		sizeLimit:    100 * 1024 * 1024, // 100 MB
		toolRegistry: make(map[string]ToolFunc),
	}

	// 注册内置工具
	// s.RegisterTool("create_schedule", s.toolCreateSchedule)
	// s.RegisterTool("get_schedule", s.toolGetSchedule)

	return s
}

// RegisterTool 注册工具
func (as *AssistantService) RegisterTool(name string, fn ToolFunc) {
	as.toolRegistry[name] = fn
}

// estimateMessageSize 估算消息列表的 JSON 序列化大小（字节）
func (as *AssistantService) estimateMessageSize(msgs []model.ChatMessage) int64 {
	if len(msgs) == 0 {
		return 0
	}
	b, err := json.Marshal(msgs)
	if err != nil {
		// fallback: 每条消息按 1KB 估算
		return int64(len(msgs)) * 1024
	}
	return int64(len(b))
}

// evictTasksIfNeeded 淘汰最早的任务，直到 totalSize <= sizeLimit
func (as *AssistantService) evictTasksIfNeeded() {
	for as.totalSize > as.sizeLimit && len(as.taskOrder) > 0 {
		oldestID := as.taskOrder[0]

		as.tasksMux.Lock()
		task, exists := as.tasks[oldestID]
		if !exists {
			as.taskOrder = as.taskOrder[1:]
			as.tasksMux.Unlock()
			continue
		}

		// 强制中断
		task.Lock()
		task.IsActive = false
		task.Unlock()

		// 发送 [DONE] 并关闭 stream（服务端主动回收）
		select {
		case task.Stream <- "data: [DONE]\n\n":
		default:
		}
		close(task.Stream)

		// 真正清理
		delete(as.tasks, oldestID)
		as.totalSize -= task.size
		as.taskOrder = as.taskOrder[1:]
		as.tasksMux.Unlock()

		logger.Infof("任务因内存限制被清理: %s", oldestID)
	}
}

// CreateChat 创建聊天任务
func (as *AssistantService) CreateChat(messages []model.ChatMessage, modelName string) (string, error) {
	taskID := generateTaskID()

	systemMessage := model.ChatMessage{
		Role:    "system",
		Content: "你是一个日程管理助手，可以创建、查询、更新和删除日程事件。请根据用户请求调用相应的工具函数。",
	}
	fullMessages := append([]model.ChatMessage{systemMessage}, messages...)
	taskSize := as.estimateMessageSize(fullMessages)

	as.tasksMux.Lock()
	defer as.tasksMux.Unlock()

	// 加入 FIFO 队列并累加大小
	as.taskOrder = append(as.taskOrder, taskID)
	as.totalSize += taskSize

	// 淘汰旧任务（可能包括刚加入的，但概率极低）
	as.evictTasksIfNeeded()

	// 创建任务（此时 totalSize 已合规）
	task := &ChatTask{
		ID:        taskID,
		Messages:  fullMessages,
		Model:     modelName,
		Stream:    make(chan string, 1000),
		IsActive:  true,
		CreatedAt: time.Now(),
		size:      taskSize,
	}
	as.tasks[taskID] = task

	go as.processChat(task)
	return taskID, nil
}

// ContinueChat 继续聊天
func (as *AssistantService) ContinueChat(taskID string, newMessages []model.ChatMessage) error {
	as.tasksMux.RLock()
	task, exists := as.tasks[taskID]
	as.tasksMux.RUnlock()

	if !exists {
		return errors.New("任务不存在")
	}

	task.Lock()
	if !task.IsActive {
		task.Unlock()
		return errors.New("任务已结束")
	}

	oldSize := task.size
	task.Messages = append(task.Messages, newMessages...)
	newSize := as.estimateMessageSize(task.Messages)
	delta := newSize - oldSize
	task.size = newSize
	task.Unlock()

	// 更新总大小（需加锁）
	as.tasksMux.Lock()
	as.totalSize += delta
	as.evictTasksIfNeeded()
	as.tasksMux.Unlock()

	go as.processChat(task)
	return nil
}

// GetStream 获取流式响应通道
func (as *AssistantService) GetStream(taskID string) (<-chan string, error) {
	as.tasksMux.RLock()
	task, exists := as.tasks[taskID]
	as.tasksMux.RUnlock()

	if !exists {
		return nil, errors.New("任务不存在")
	}
	if !task.IsActive {
		// 仍允许读取剩余内容，直到 channel 关闭
	}
	return task.Stream, nil
}

// StopChat 仅中断任务（不停止流读取，不删除任务）
func (as *AssistantService) StopChat(taskID string) error {
	as.tasksMux.RLock()
	task, exists := as.tasks[taskID]
	as.tasksMux.RUnlock()

	if !exists {
		return errors.New("任务不存在")
	}

	task.Lock()
	task.IsActive = false
	task.Unlock()

	logger.Infof("任务已中断: %s", taskID)
	return nil
}

// IsSupportedModel 检查模型是否受支持
func (as *AssistantService) IsSupportedModel(model string) bool {
	if model == "default" {
		for _, p := range config.AppConfig.LLM.Providers {
			if !p.Enabled || p.Default == "" {
				continue
			}
			for _, m := range p.Models {
				if m.Enabled && m.Code == p.Default {
					return true
				}
			}
		}
		return false
	}
	for _, p := range config.AppConfig.LLM.Providers {
		if !p.Enabled {
			continue
		}
		for _, m := range p.Models {
			if m.Enabled && m.Code == model {
				return true
			}
		}
	}
	return false
}

// GetSupportedModels 获取支持的模型列表
func (as *AssistantService) GetSupportedModels() []config.LLMModel {
	var models []config.LLMModel
	for _, p := range config.AppConfig.LLM.Providers {
		if !p.Enabled {
			continue
		}
		for _, m := range p.Models {
			if m.Enabled {
				models = append(models, m)
			}
		}
	}
	return models
}

// 获取LLM提供商
func (as *AssistantService) getProviderForModel(model string) (*config.LLMProvider, error) {
	if model == "default" {
		for _, p := range config.AppConfig.LLM.Providers {
			if !p.Enabled || p.Default == "" {
				continue
			}
			for _, m := range p.Models {
				if m.Enabled && m.Code == p.Default {
					return &p, nil
				}
			}
		}
		return nil, errors.New("未找到启用的默认模型")
	}
	for _, p := range config.AppConfig.LLM.Providers {
		if !p.Enabled {
			continue
		}
		for _, m := range p.Models {
			if m.Enabled && m.Code == model {
				return &p, nil
			}
		}
	}
	return nil, errors.New("未找到对应的LLM提供商")
}

// processChat 处理聊天任务
func (as *AssistantService) processChat(task *ChatTask) {
	defer func() {
		// 确保发送 [DONE]
		select {
		case task.Stream <- "data: [DONE]\n\n":
		default:
		}
		// 注意：不在此关闭 channel！由淘汰机制或长期保留
	}()

	provider, err := as.getProviderForModel(task.Model)
	if err != nil {
		as.sendError(task, fmt.Sprintf("模型配置错误: %v", err))
		return
	}

	actualModel := task.Model
	if task.Model == "default" {
		actualModel = provider.Default
	}

	for round := 0; round < 5; round++ {
		task.Lock()
		if !task.IsActive {
			task.Unlock()
			return
		}
		msgs := make([]model.ChatMessage, len(task.Messages))
		copy(msgs, task.Messages)
		task.Unlock()

		tools := model.ScheduleTools

		reqBody := model.ChatCompletionRequest{
			Model:     actualModel,
			Messages:  msgs,
			Stream:    true,
			MaxTokens: 2000,
			Tools:     tools,
		}

		jsonData, err := json.Marshal(reqBody)
		if err != nil {
			as.sendError(task, "请求序列化失败")
			return
		}

		httpReq, err := http.NewRequest("POST", provider.BaseURL, bytes.NewBuffer(jsonData))
		if err != nil {
			as.sendError(task, "HTTP请求创建失败")
			return
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Authorization", "Bearer "+provider.APIKey)
		httpReq.Header.Set("Accept", "text/event-stream")

		client := &http.Client{Timeout: 60 * time.Second}
		resp, err := client.Do(httpReq)
		if err != nil {
			as.sendError(task, fmt.Sprintf("LLM调用失败: %v", err))
			return
		}

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			as.sendError(task, fmt.Sprintf("LLM返回错误: %d %s", resp.StatusCode, string(body)))
			return
		}

		fullToolCalls := as.handleStreamResponse(task, resp.Body)
		resp.Body.Close()

		if len(fullToolCalls) > 0 {
			assistantMsg := model.ChatMessage{
				Role:      "assistant",
				ToolCalls: fullToolCalls,
			}
			task.Lock()
			task.Messages = append(task.Messages, assistantMsg)
			task.Unlock()

			var clientToolCalls []model.ToolCall
			for _, toolCall := range fullToolCalls {
				toolResultMsg, handled := as.executeTool(toolCall)
				if handled {
					task.Lock()
					task.Messages = append(task.Messages, toolResultMsg)
					task.Unlock()
				} else {
					clientToolCalls = append(clientToolCalls, toolCall)
				}
			}

			if len(clientToolCalls) > 0 {
				as.sendToolRequestNotice(task, clientToolCalls)
				return
			}
			continue
		} else {
			break
		}
	}
}

// handleStreamResponse 处理流式响应
func (as *AssistantService) handleStreamResponse(task *ChatTask, body io.ReadCloser) []model.ToolCall {
	scanner := bufio.NewScanner(body)
	var fullToolCalls []model.ToolCall

	for scanner.Scan() {
		if !task.IsActive {
			break
		}
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}

		var chunk model.ChatCompletionResponse
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}

		if len(chunk.Choices) == 0 {
			continue
		}
		delta := chunk.Choices[0].Delta

		if delta.Content != "" {
			respData := map[string]interface{}{
				"choices": []interface{}{
					map[string]interface{}{
						"delta": map[string]interface{}{
							"content": delta.Content,
						},
					},
				},
			}
			if b, _ := json.Marshal(respData); b != nil {
				select {
				case task.Stream <- "data: " + string(b) + "\n\n":
				default:
				}
			}
		}

		for _, tc := range delta.ToolCalls {
			for len(fullToolCalls) <= tc.Index {
				fullToolCalls = append(fullToolCalls, model.ToolCall{
					Type:     "function",
					Function: model.ToolCallFunction{},
				})
			}
			call := &fullToolCalls[tc.Index]
			if tc.ID != "" {
				call.ID = tc.ID
			}
			if tc.Type != "" {
				call.Type = tc.Type
			}
			if tc.Function.Name != "" {
				call.Function.Name = tc.Function.Name
			}
			call.Function.Arguments += tc.Function.Arguments

			respData := map[string]interface{}{
				"choices": []interface{}{
					map[string]interface{}{
						"delta": map[string]interface{}{
							"tool_calls": []interface{}{
								map[string]interface{}{
									"index": tc.Index,
									"function": map[string]interface{}{
										"name":      tc.Function.Name,
										"arguments": tc.Function.Arguments,
									},
								},
							},
						},
					},
				},
			}
			if b, _ := json.Marshal(respData); b != nil {
				select {
				case task.Stream <- "data: " + string(b) + "\n\n":
				default:
				}
			}
		}

		if delta.FunctionCall != nil {
			fullToolCalls = append(fullToolCalls, model.ToolCall{
				Type: "function",
				Function: model.ToolCallFunction{
					Name:      delta.FunctionCall.Name,
					Arguments: delta.FunctionCall.Arguments,
				},
			})
		}
	}
	return fullToolCalls
}

// executeTool 执行工具
func (as *AssistantService) executeTool(toolCall model.ToolCall) (model.ChatMessage, bool) {
	name := toolCall.Function.Name
	argsStr := toolCall.Function.Arguments

	logger.Infof("执行工具: %s, 参数: %s", name, argsStr)

	var args map[string]interface{}
	if jsonErr := json.Unmarshal([]byte(argsStr), &args); jsonErr != nil {
		result := fmt.Sprintf("参数解析失败: %v", jsonErr)
		return model.ChatMessage{
			Role:       "tool",
			Content:    result,
			ToolCallID: toolCall.ID,
		}, true
	}

	if fn, ok := as.toolRegistry[name]; ok {
		result, err := fn(args)
		if err != nil {
			result = fmt.Sprintf("工具执行出错: %v", err)
		}
		return model.ChatMessage{
			Role:       "tool",
			Content:    result,
			ToolCallID: toolCall.ID,
		}, true
	}

	// 未注册 -> 客户端处理
	return model.ChatMessage{}, false
}

// sendError 发送错误
func (as *AssistantService) sendError(task *ChatTask, msg string) {
	errData := map[string]interface{}{
		"error": map[string]string{
			"message": msg,
		},
	}
	if b, _ := json.Marshal(errData); b != nil {
		select {
		case task.Stream <- "data: " + string(b) + "\n\n":
		default:
		}
	}
}

// sendToolRequestNotice 通知客户端需要处理工具
func (as *AssistantService) sendToolRequestNotice(task *ChatTask, toolCalls []model.ToolCall) {
	// 示例：发送一个特殊事件
	notice := map[string]interface{}{
		"type":       "tool_request",
		"tool_calls": toolCalls,
	}
	if b, _ := json.Marshal(notice); b != nil {
		select {
		case task.Stream <- "data: " + string(b) + "\n\n":
		default:
		}
	}
}

// generateTaskID 生成任务ID
func generateTaskID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}
