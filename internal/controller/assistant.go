package controller

import (
	"net/http"
	"synapse-backend/internal/model"
	"synapse-backend/internal/service"
	"synapse-backend/internal/utils"
	"sync"

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

// StartChat 创建新聊天或继续已有聊天
func (ac *AssistantController) StartChat(c *gin.Context) {
	var request model.ChatRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		utils.ResponseError(c, http.StatusBadRequest, "请求参数格式错误: "+err.Error())
		return
	}

	if len(request.Messages) == 0 {
		utils.ResponseError(c, http.StatusBadRequest, "消息内容不能为空")
		return
	}

	// 校验模型
	modelName := request.Model
	if modelName == "" {
		modelName = "default"
	}
	if !ac.assistantService.IsSupportedModel(modelName) {
		utils.ResponseError(c, http.StatusBadRequest, "不支持的模型: "+modelName)
		return
	}

	var (
		taskID string
		err    error
	)

	if request.TaskID != "" {
		// 继续对话
		taskID = request.TaskID
		err = ac.assistantService.ContinueChat(taskID, request.Messages)
		if err != nil {
			utils.ResponseError(c, http.StatusBadRequest, "无法继续聊天: "+err.Error())
			return
		}
	} else {
		// 创建新对话
		taskID, err = ac.assistantService.CreateChat(request.Messages, modelName)
		if err != nil {
			utils.ResponseError(c, http.StatusInternalServerError, "创建聊天任务失败: "+err.Error())
			return
		}
	}

	utils.ResponseSuccess(c, model.TaskResponse{TaskID: taskID})
}

// StreamChat SSE 流式响应
func (ac *AssistantController) StreamChat(c *gin.Context) {
	taskID := c.Param("taskId")
	if taskID == "" {
		utils.ResponseError(c, http.StatusBadRequest, "任务ID不能为空")
		return
	}

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

	// 使用 context 监听客户端断开（Gin 内置）
	ctx := c.Request.Context()
	var wg sync.WaitGroup
	wg.Add(1)

	// 启动 goroutine 发送数据
	go func() {
		defer wg.Done()
		for {
			select {
			case data, ok := <-streamChan:
				if !ok {
					// channel 已关闭（被内存淘汰）
					return
				}
				_, writeErr := c.Writer.Write([]byte(data))
				if writeErr != nil {
					// 客户端断开
					return
				}
				c.Writer.Flush()
			case <-ctx.Done():
				// 客户端主动关闭连接
				return
			}
		}
	}()

	// 等待发送完成或中断
	wg.Wait()
}

// StopChat 中断任务
func (ac *AssistantController) StopChat(c *gin.Context) {
	taskID := c.Param("taskId")
	if taskID == "" {
		utils.ResponseError(c, http.StatusBadRequest, "任务ID不能为空")
		return
	}

	// 即使任务不存在或已结束，也静默成功
	err := ac.assistantService.StopChat(taskID)
	if err != nil {
		// 通常只有 "任务不存在"，记录日志但不暴露给前端
		utils.ResponseError(c, http.StatusNotFound, err.Error())
	}

	utils.ResponseSuccess(c, gin.H{"message": "任务已中断", "task_id": taskID})
}

// GetSupportedModels 获取支持的模型列表
func (ac *AssistantController) GetSupportedModels(c *gin.Context) {
	models := ac.assistantService.GetSupportedModels()
	utils.ResponseSuccess(c, models)
}
