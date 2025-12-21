package model

// ChatMessage 聊天消息结构
type ChatMessage struct {
	Role       string     `json:"role" binding:"required"`    // 角色：user, assistant, system
	Content    string     `json:"content" binding:"required"` // 消息内容
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`       // 工具调用
	ToolCallID string     `json:"tool_call_id,omitempty"`     // 工具调用ID
}

// ChatRequest 聊天请求结构
type ChatRequest struct {
	Messages []ChatMessage `json:"messages" binding:"required,dive"` // 消息列表
	Model    string        `json:"model" binding:"required"`         // 模型类型
	TaskID   string        `json:"taskId,omitempty"`                 // 可选的任务ID，用于多轮对话
}

// TaskResponse 任务响应结构
type TaskResponse struct {
	TaskID string `json:"taskId"`
}

// OpenAI兼容API请求结构
type ChatCompletionRequest struct {
	Model     string        `json:"model"`
	Messages  []ChatMessage `json:"messages"`
	Stream    bool          `json:"stream"`
	MaxTokens int           `json:"max_tokens,omitempty"`
	Tools     []Tool        `json:"tools,omitempty"`
}
