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

// AssistantService 助手服务
type AssistantService struct {
	tasks    map[string]*ChatTask
	tasksMux sync.RWMutex
}

// ChatTask 聊天任务
type ChatTask struct {
	ID        string              `json:"id"`
	Messages  []model.ChatMessage `json:"messages"`
	Model     string              `json:"model"`
	Stream    chan string         `json:"-"`
	IsActive  bool                `json:"is_active"`
	CreatedAt time.Time           `json:"created_at"`
}

// NewAssistantService 创建助手服务
func NewAssistantService() *AssistantService {
	return &AssistantService{
		tasks: make(map[string]*ChatTask),
	}
}

// CreateChat 创建聊天任务
func (as *AssistantService) CreateChat(messages []model.ChatMessage, model string) (string, error) {
	taskID := generateTaskID()
	task := &ChatTask{
		ID:        taskID,
		Messages:  messages,
		Model:     model,
		Stream:    make(chan string, 1000),
		IsActive:  true,
		CreatedAt: time.Now(),
	}

	as.tasksMux.Lock()
	as.tasks[taskID] = task
	as.tasksMux.Unlock()

	go as.processChat(task)
	return taskID, nil
}

// ContinueChat 继续聊天
func (as *AssistantService) ContinueChat(taskID string, newMessages []model.ChatMessage) error {
	as.tasksMux.Lock()
	defer as.tasksMux.Unlock()

	task, exists := as.tasks[taskID]
	if !exists || !task.IsActive {
		return errors.New("任务不存在或已结束")
	}
	task.Messages = newMessages
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
		return nil, errors.New("任务已结束")
	}
	return task.Stream, nil
}

// StopChat 停止聊天
func (as *AssistantService) StopChat(taskID string) error {
	as.tasksMux.Lock()
	defer as.tasksMux.Unlock()

	task, exists := as.tasks[taskID]
	if !exists {
		return nil
	}
	task.IsActive = false
	close(task.Stream)
	delete(as.tasks, taskID)
	logger.Infof("任务已停止: %s", taskID)
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
		task.IsActive = false
		close(task.Stream)
		as.tasksMux.Lock()
		delete(as.tasks, task.ID)
		as.tasksMux.Unlock()
		logger.Infof("任务结束: %s", task.ID)
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

	// 最多尝试5轮
	for round := 0; round < 5; round++ {
		if !task.IsActive {
			return
		}

		// 第0轮启用工具，后续禁用
		var tools []model.Tool
		if round == 0 {
			tools = model.ScheduleTools
		}

		// 构造请求
		reqBody := model.ChatCompletionRequest{
			Model:     actualModel,
			Messages:  task.Messages,
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
		defer resp.Body.Close()

		// 解析流
		scanner := bufio.NewScanner(resp.Body)
		var fullToolCalls []model.ToolCall

		for scanner.Scan() {
			if !task.IsActive {
				return
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

			// 实时推送内容
			if delta.Content != "" {
				select {
				case task.Stream <- delta.Content:
				default:
					logger.Warnf("流通道满，丢弃内容: %s", task.ID)
				}
			}

			// 累积工具调用
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
			}
		}

		// 检查是否需要工具调用
		if len(fullToolCalls) > 0 {
			// 添加助手消息（包含 tool_calls）
			assistantMsg := model.ChatMessage{
				Role:      "assistant",
				ToolCalls: fullToolCalls,
			}
			task.Messages = append(task.Messages, assistantMsg)

			// 执行每个工具
			for _, tc := range fullToolCalls {
				resultMsg := as.executeTool(tc)
				task.Messages = append(task.Messages, resultMsg)
			}

			// 继续下一轮（让 LLM 看到工具结果）
			continue
		} else {
			// 无工具调用，说明是最终回复，结束
			break
		}
	}
}

// executeTool 执行工具（保持 Mock，后续可替换为真实日历API）
func (as *AssistantService) executeTool(toolCall model.ToolCall) model.ChatMessage {
	var result string
	switch toolCall.Function.Name {
	case "create_schedule":
		result = `{"status": "success", "message": "日程创建成功"}`
	case "get_schedule":
		result = `{"status": "success", "events": []}`
	case "update_schedule":
		result = `{"status": "success", "message": "日程更新成功"}`
	case "delete_schedule":
		result = `{"status": "success", "message": "日程删除成功"}`
	default:
		result = `{"status": "error", "message": "未知工具"}`
	}
	return model.ChatMessage{
		Role:       "tool",
		Content:    result,
		ToolCallID: toolCall.ID,
	}
}

func (as *AssistantService) sendError(task *ChatTask, msg string) {
	select {
	case task.Stream <- "[ERROR] " + msg:
	default:
	}
}

func generateTaskID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}
