package controller

import (
	"net/http"
	"synapse-backend/internal/model"
	"synapse-backend/internal/service"
	"synapse-backend/internal/utils"
	"github.com/gin-gonic/gin"
)

type AssistantController struct {
	assistantService *service.AssistantService
}

func NewAssistantController(assistantService *service.AssistantService) *AssistantController {
	return &AssistantController{
		assistantService: assistantService,
	}
}

func (ac *AssistantController) StartChat(c *gin.Context) {
	var request model.ChatRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		utils.ResponseError(c, http.StatusBadRequest, "请求参数格式错误: "+err.Error())
		return
	}
	if len(request.Messages) == 0 {
		utils.ResponseError(c, http.StatusBadRequest, "消息内容缺失")
		return
	}
	if !ac.assistantService.IsSupportedModel(request.Model) {
		utils.ResponseError(c, http.StatusBadRequest, "不支持的模型类型: "+request.Model)
		return
	}

	var taskID string
	var err error

	if request.TaskID != "" {
		taskID = request.TaskID
		err = ac.assistantService.ContinueChat(taskID, request.Messages)
		if err != nil {
			utils.ResponseError(c, http.StatusBadRequest, "继续聊天失败: "+err.Error())
			return
		}
	} else {
		taskID, err = ac.assistantService.CreateChat(request.Messages, request.Model)
		if err != nil {
			utils.ResponseError(c, http.StatusInternalServerError, "创建聊天任务失败: "+err.Error())
			return
		}
	}

	utils.ResponseSuccess(c, model.TaskResponse{TaskID: taskID})
}

// StreamChat：获取聊天流
func (ac *AssistantController) StreamChat(c *gin.Context) {
	taskID := c.Param("taskId")
	if taskID == "" {
		utils.ResponseError(c, http.StatusBadRequest, "任务ID不能为空")
		return
	}

	// 直接获取流通道（无需等待）
	streamChan, err := ac.assistantService.GetStream(taskID)
	if err != nil {
		utils.ResponseError(c, http.StatusNotFound, "任务不存在或已结束: "+err.Error())
		return
	}

	// 设置 SSE 头
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")

	// 监听流
	for data := range streamChan {
		_, writeErr := c.Writer.Write([]byte(data))
		if writeErr != nil {
			return // 客户端断开
		}
		c.Writer.Flush()
	}
}

// StopChat：停止当前对话
func (ac *AssistantController) StopChat(c *gin.Context) {
	taskID := c.Param("taskId")
	if taskID == "" {
		utils.ResponseError(c, http.StatusBadRequest, "任务ID不能为空")
		return
	}

	ac.assistantService.StopChat(taskID)

	// 无论任务是否存在/是否已结束，都返回成功
	utils.ResponseSuccess(c, nil)
}

// GetSupportedModels：获取支持的模型列表
func (ac *AssistantController) GetSupportedModels(c *gin.Context) {
	models := ac.assistantService.GetSupportedModels()
	utils.ResponseSuccess(c, models)
}