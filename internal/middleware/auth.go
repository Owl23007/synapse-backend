package middleware

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"synapse-backend/internal/utils"
	"synapse-backend/pkg/logger"
)

// UserInfo 用户信息结构体
type UserInfo struct {
	UserID     string `json:"user_id"`      // Base62 短 ID
	UserLongID int64  `json:"user_long_id"` // Long ID
	Role       string `json:"role"`
	Scopes     string `json:"scopes"`
}

// GatewayAuthMiddleware 从网关请求头获取用户信息
func GatewayAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		shortUserID := c.GetHeader("X-User-ID")
		longUserIDStr := c.GetHeader("X-User-Long-ID")

		// 必须同时存在
		if shortUserID == "" || longUserIDStr == "" {
			logger.Warnf("请求缺少认证头: X-User-ID=%s, X-User-Long-ID=%s, Path=%s", shortUserID, longUserIDStr, c.Request.URL.Path)
			utils.ResponseError(c, http.StatusUnauthorized, "Invalid user ID")
			c.Abort()
			return
		}

		longUserID, err := strconv.ParseInt(longUserIDStr, 10, 64)
		if err != nil {
			logger.Warnf("X-User-Long-ID格式错误: %s", longUserIDStr)
			utils.ResponseError(c, http.StatusUnauthorized, "Invalid user ID format")
			c.Abort()
			return
		}

		// 注入用户信息到Context
		c.Set("user_id", shortUserID)
		c.Set("user_long_id", longUserID)
		c.Set("role", c.GetHeader("X-User-Role"))
		c.Set("scopes", c.GetHeader("X-User-Scopes"))
		c.Set("token_jti", c.GetHeader("X-Token-JTI"))
		c.Set("token_type", c.GetHeader("X-Token-Type"))

		logger.Infof("Gateway认证成功: user_id=%s, long_id=%d", shortUserID, longUserID)

		c.Next()
	}
}

// GatewayOptionalAuthMiddleware 可选的网关认证（存在则解析，不存在则跳过）
func GatewayOptionalAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		shortUserID := c.GetHeader("X-User-ID")
		longUserIDStr := c.GetHeader("X-User-Long-ID")

		if shortUserID != "" && longUserIDStr != "" {
			longUserID, err := strconv.ParseInt(longUserIDStr, 10, 64)
			if err == nil {
				c.Set("user_id", shortUserID)
				c.Set("user_long_id", longUserID)
				c.Set("role", c.GetHeader("X-User-Role"))
				c.Set("scopes", c.GetHeader("X-User-Scopes"))
				c.Set("token_jti", c.GetHeader("X-Token-JTI"))
				c.Set("token_type", c.GetHeader("X-Token-Type"))
				
				logger.Infof("Gateway可选认证成功: user_id=%s", shortUserID)
			}
		}

		c.Next()
	}
}

// GetUserFromContext 从Context中获取用户信息
func GetUserFromContext(c *gin.Context) (*UserInfo, bool) {
	userID, exists := c.Get("user_id")
	if !exists {
		return nil, false
	}

	userLongID, _ := c.Get("user_long_id")
	role, _ := c.Get("role")
	scopes, _ := c.Get("scopes")

	// 安全类型断言
	var uid string
	if v, ok := userID.(string); ok {
		uid = v
	}

	var ulid int64
	if v, ok := userLongID.(int64); ok {
		ulid = v
	}

	var r string
	if v, ok := role.(string); ok {
		r = v
	}

	var s string
	if v, ok := scopes.(string); ok {
		s = v
	}

	return &UserInfo{
		UserID:     uid,
		UserLongID: ulid,
		Role:       r,
		Scopes:     s,
	}, true
}
