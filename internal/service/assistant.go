package service

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"synapse-backend/internal/config"
	"synapse-backend/internal/model"
	"synapse-backend/pkg/logger"
	"sync"
	"time"
)

// AssistantService 助手服务
type AssistantService struct {
	tasks    map[string]*ChatTask // 任务映射
	tasksMux sync.RWMutex         // 任务读写锁
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

// OpenAI兼容API请求结构
type ChatCompletionRequest struct {
	Model     string              `json:"model"`
	Messages  []model.ChatMessage `json:"messages"`
	Stream    bool                `json:"stream"`
	MaxTokens int                 `json:"max_tokens,omitempty"`
}

// OpenAI兼容API响应结构
type ChatCompletionResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index int `json:"index"`
		Delta struct {
			Content string `json:"content"`
		} `json:"delta,omitempty"`
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message,omitempty"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
}

// NewAssistantService 创建助手服务
func NewAssistantService() *AssistantService {
	return &AssistantService{
		tasks: make(map[string]*ChatTask),
	}
}

// generateTaskID 生成任务ID
func generateTaskID() string {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	return fmt.Sprintf("%x-%x-%x-%x-%x",
		bytes[0:4], bytes[4:6], bytes[6:8], bytes[8:10], bytes[10:16])
}

// IsSupportedModel 检查是否支持该模型
func (as *AssistantService) IsSupportedModel(model string) bool {
	// 如果是"default"，则查找配置中的默认模型
	if model == "default" {
		for _, provider := range config.AppConfig.LLM.Providers {
			if !provider.Enabled {
				continue
			}
			// 检查该提供商是否有默认模型且该模型已启用
			if provider.Default != "" {
				for _, llmModel := range provider.Models {
					if llmModel.Enabled && llmModel.Code == provider.Default {
						return true
					}
				}
			}
		}
		return false
	}

	// 遍历配置中的所有LLM提供商
	for _, provider := range config.AppConfig.LLM.Providers {
		if !provider.Enabled {
			continue
		}
		// 检查提供商的所有模型
		for _, llmModel := range provider.Models {
			if llmModel.Enabled && llmModel.Code == model {
				return true
			}
		}
	}
	return false
}

// GetSupportedModels 获取所有支持的模型列表
func (as *AssistantService) GetSupportedModels() []config.LLMModel {
	var supportedModels []config.LLMModel

	// 遍历配置中的所有LLM提供商
	for _, provider := range config.AppConfig.LLM.Providers {
		if !provider.Enabled {
			continue
		}
		// 添加启用的模型
		for _, llmModel := range provider.Models {
			if llmModel.Enabled {
				supportedModels = append(supportedModels, llmModel)
			}
		}
	}

	return supportedModels
}

// getProviderForModel 根据模型获取对应的提供商配置
func (as *AssistantService) getProviderForModel(model string) (*config.LLMProvider, error) {
	// 如果是"default"，则查找有默认模型的提供商
	if model == "default" {
		for _, provider := range config.AppConfig.LLM.Providers {
			if !provider.Enabled {
				continue
			}
			// 检查该提供商是否有默认模型且该模型已启用
			if provider.Default != "" {
				for _, llmModel := range provider.Models {
					if llmModel.Enabled && llmModel.Code == provider.Default {
						return &provider, nil
					}
				}
			}
		}
		return nil, errors.New("未找到启用的默认模型")
	}

	for _, provider := range config.AppConfig.LLM.Providers {
		if !provider.Enabled {
			continue
		}
		for _, llmModel := range provider.Models {
			if llmModel.Enabled && llmModel.Code == model {
				return &provider, nil
			}
		}
	}
	return nil, errors.New("未找到对应的LLM提供商")
}

// CreateChat 创建聊天任务
func (as *AssistantService) CreateChat(messages []model.ChatMessage, model string) (string, error) {
	// 生成任务ID
	taskID := generateTaskID()

	// 创建任务
	task := &ChatTask{
		ID:        taskID,
		Messages:  messages,
		Model:     model,
		Stream:    make(chan string, 100), // 带缓冲的通道
		IsActive:  true,
		CreatedAt: time.Now(),
	}

	// 存储任务
	as.tasksMux.Lock()
	as.tasks[taskID] = task
	as.tasksMux.Unlock()

	// 启动异步处理
	go as.processChat(task)

	return taskID, nil
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
		return errors.New("任务不存在")
	}

	// 标记为非活跃状态
	task.IsActive = false

	// 关闭通道
	close(task.Stream)

	// 清理任务
	delete(as.tasks, taskID)

	return nil
}

// processChat 处理聊天任务（调用真实的LLM API）
func (as *AssistantService) processChat(task *ChatTask) {
	logger.Infof("开始处理聊天任务: TaskID=%s, Model=%s", task.ID, task.Model)

	defer func() {
		// 确保任务结束时关闭通道
		if task.IsActive {
			task.IsActive = false
			close(task.Stream)
			// 清理任务
			as.tasksMux.Lock()
			delete(as.tasks, task.ID)
			as.tasksMux.Unlock()
			logger.Infof("聊天任务处理完成: TaskID=%s", task.ID)
		}
	}()

	// 获取对应的LLM提供商配置
	provider, err := as.getProviderForModel(task.Model)
	if err != nil {
		logger.Errorf("获取LLM提供商失败: TaskID=%s, Model=%s, Error=%s", task.ID, task.Model, err.Error())
		as.sendErrorToStream(task, fmt.Sprintf("配置错误: %s", err.Error()))
		return
	}

	logger.Infof("使用LLM提供商: %s, BaseURL=%s", provider.Name, provider.BaseURL)

	// 获取实际的模型名称（如果请求的是"default"，则使用配置中的默认模型）
	actualModel := task.Model
	if task.Model == "default" {
		actualModel = provider.Default
	}

	// 准备API请求
	requestBody := ChatCompletionRequest{
		Model:     actualModel,
		Messages:  task.Messages,
		Stream:    true,
		MaxTokens: 2000,
	}

	requestData, err := json.Marshal(requestBody)
	if err != nil {
		logger.Errorf("请求数据编码失败: TaskID=%s, Error=%s", task.ID, err.Error())
		as.sendErrorToStream(task, fmt.Sprintf("请求数据编码失败: %s", err.Error()))
		return
	}

	// 创建HTTP请求
	req, err := http.NewRequest("POST", provider.BaseURL, bytes.NewBuffer(requestData))
	if err != nil {
		logger.Errorf("创建HTTP请求失败: TaskID=%s, Error=%s", task.ID, err.Error())
		as.sendErrorToStream(task, fmt.Sprintf("创建请求失败: %s", err.Error()))
		return
	}

	// 设置请求头
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+provider.APIKey)
	req.Header.Set("Accept", "text/event-stream")

	// 发送请求
	client := &http.Client{
		Timeout: 60 * time.Second,
	}

	logger.Infof("发送API请求: TaskID=%s, URL=%s", task.ID, provider.BaseURL)
	resp, err := client.Do(req)
	if err != nil {
		logger.Errorf("API请求失败: TaskID=%s, Error=%s", task.ID, err.Error())
		as.sendErrorToStream(task, fmt.Sprintf("API请求失败: %s", err.Error()))
		return
	}
	defer resp.Body.Close()

	// 检查响应状态码
	if resp.StatusCode != http.StatusOK {
		logger.Errorf("API返回错误状态码: TaskID=%s, StatusCode=%d", task.ID, resp.StatusCode)
		as.sendErrorToStream(task, fmt.Sprintf("API返回状态码: %d", resp.StatusCode))
		return
	}

	logger.Infof("开始接收流式响应: TaskID=%s", task.ID)

	// 处理流式响应
	scanner := bufio.NewScanner(resp.Body)
	messageCount := 0

	for scanner.Scan() {
		// 检查任务是否仍然活跃
		if !task.IsActive {
			logger.Infof("任务被标记为非活跃状态，停止处理: TaskID=%s", task.ID)
			return
		}

		line := scanner.Text()

		// 跳过空行和注释行
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}

		// 处理SSE数据
		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")

			// 处理结束标记
			if data == "[DONE]" {
				logger.Infof("接收到流式响应结束标记: TaskID=%s, 消息片段数=%d", task.ID, messageCount)
				return
			}

			// 解析JSON响应
			var chatResp ChatCompletionResponse
			if err := json.Unmarshal([]byte(data), &chatResp); err != nil {
				logger.Warnf("无法解析响应数据: TaskID=%s, Data=%s, Error=%s", task.ID, data, err.Error())
				continue // 跳过无法解析的数据
			}

			// 提取内容并发送
			if len(chatResp.Choices) > 0 && chatResp.Choices[0].Delta.Content != "" {
				content := chatResp.Choices[0].Delta.Content
				select {
				case task.Stream <- content:
					messageCount++
				default:
					// 通道已满或已关闭，停止处理
					logger.Warnf("通道已满或已关闭，停止处理: TaskID=%s", task.ID)
					return
				}
			}
		}
	}

	// 检查扫描器是否遇到错误
	if err := scanner.Err(); err != nil {
		logger.Errorf("读取响应流失败: TaskID=%s, Error=%s", task.ID, err.Error())
		as.sendErrorToStream(task, fmt.Sprintf("读取响应流失败: %s", err.Error()))
	} else {
		logger.Infof("流式响应处理完成: TaskID=%s, 消息片段数=%d", task.ID, messageCount)
	}
}

// sendErrorToStream 向流中发送错误信息
func (as *AssistantService) sendErrorToStream(task *ChatTask, message string) {
	select {
	case task.Stream <- fmt.Sprintf("错误: %s", message):
	default:
		// 通道已满或已关闭，记录日志但不阻塞
		logger.Warnf("无法发送错误信息到流: TaskID=%s, Message=%s", task.ID, message)
	}
}
