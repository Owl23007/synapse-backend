package utils

import (
	"net/http"
	"synapse-backend/internal/model"

	"github.com/gin-gonic/gin"
)

// ResponseSuccess 成功响应
func ResponseSuccess(c *gin.Context, data interface{}) {
	result := model.Success(data)
	c.JSON(http.StatusOK, result)
}

// ResponseError 错误响应
func ResponseError(c *gin.Context, code int, message string) {
	result := model.Error[interface{}](message)
	result.Code = code
	c.JSON(code, result)
}
