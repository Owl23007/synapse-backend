package model

// ChatMessage 聊天消息结构
type ChatMessage struct {
	Role    string `json:"role" binding:"required"`    // 角色：user, assistant, system
	Content string `json:"content" binding:"required"` // 消息内容
}

// ChatRequest 聊天请求结构
type ChatRequest struct {
	Messages []ChatMessage `json:"messages" binding:"required,dive"` // 消息列表
	Model    string        `json:"model" binding:"required"`         // 模型类型
}

// TaskResponse 任务响应结构
type TaskResponse struct {
	TaskID string `json:"taskId"`
}
