package controller

import (
	"net/http"
	"synapse-backend/internal/model"
	"synapse-backend/internal/service"
	"synapse-backend/internal/utils"

	"github.com/gin-gonic/gin"
)

// AssistantController 助手控制器
type AssistantController struct {
	assistantService *service.AssistantService
}

// NewAssistantController 创建助手控制器
func NewAssistantController(assistantService *service.AssistantService) *AssistantController {
	return &AssistantController{
		assistantService: assistantService,
	}
}

// StartChat 开始聊天
func (ac *AssistantController) StartChat(c *gin.Context) {
	var request model.ChatRequest

	// 绑定并验证请求体
	if err := c.ShouldBindJSON(&request); err != nil {
		utils.ResponseError(c, http.StatusBadRequest, "请求参数格式错误: "+err.Error())
		return
	}

	// 验证消息列表不为空
	if len(request.Messages) == 0 {
		utils.ResponseError(c, http.StatusBadRequest, "消息内容缺失")
		return
	}

	// 验证模型参数
	if !ac.assistantService.IsSupportedModel(request.Model) {
		utils.ResponseError(c, http.StatusBadRequest, "不支持的模型类型: "+request.Model)
		return
	}

	// 调用服务层创建任务
	taskID, err := ac.assistantService.CreateChat(request.Messages, request.Model)
	if err != nil {
		utils.ResponseError(c, http.StatusInternalServerError, "创建聊天任务失败: "+err.Error())
		return
	}

	// 返回任务ID
	utils.ResponseSuccess(c, model.TaskResponse{TaskID: taskID})
}

// StreamChat 流式聊天响应
func (ac *AssistantController) StreamChat(c *gin.Context) {
	taskID := c.Param("taskId")

	if taskID == "" {
		utils.ResponseError(c, http.StatusBadRequest, "任务ID不能为空")
		return
	}

	// 设置SSE响应头
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Allow-Headers", "Cache-Control")

	// 获取流式响应通道
	streamChan, err := ac.assistantService.GetStream(taskID)
	if err != nil {
		utils.ResponseError(c, http.StatusNotFound, "任务不存在或已结束")
		return
	}

	// 流式发送数据
	for {
		select {
		case data, ok := <-streamChan:
			if !ok {
				// 通道关闭，结束响应
				return
			}
			// 发送SSE数据
			c.Writer.WriteString("data: " + data + "\n\n")
			c.Writer.Flush()
		case <-c.Request.Context().Done():
			// 客户端断开连接
			return
		}
	}
}

// StopChat 停止聊天
func (ac *AssistantController) StopChat(c *gin.Context) {
	taskID := c.Param("taskId")

	if taskID == "" {
		utils.ResponseError(c, http.StatusBadRequest, "任务ID不能为空")
		return
	}

	// 调用服务层停止聊天
	err := ac.assistantService.StopChat(taskID)
	if err != nil {
		utils.ResponseError(c, http.StatusInternalServerError, "停止聊天失败: "+err.Error())
		return
	}

	utils.ResponseSuccess(c, nil)
}

// GetSupportedModels 获取支持的模型列表
func (ac *AssistantController) GetSupportedModels(c *gin.Context) {
	models := ac.assistantService.GetSupportedModels()
	utils.ResponseSuccess(c, models)
}
