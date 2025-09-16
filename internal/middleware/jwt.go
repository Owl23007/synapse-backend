// internal/middleware/jwt.go

package middleware

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"synapse-backend/internal/config"
	"synapse-backend/internal/utils"
	"synapse-backend/pkg/logger"
)

// JWTAuthMiddleware 使用 RSA 公钥验证 JWT Token（必须验证）
func JWTAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			logger.Warnf("请求缺少Authorization头: %s", c.Request.URL.Path)
			utils.ResponseError(c, http.StatusUnauthorized, "请求头中缺少 Authorization")
			c.Abort()
			return
		}

		// 提取 Token
		tokenString := strings.TrimPrefix(authHeader, "Bearer ")
		if len(tokenString) == len(authHeader) {
			logger.Warnf("Authorization格式错误: %s", authHeader)
			utils.ResponseError(c, http.StatusUnauthorized, "Authorization 格式错误，应为 Bearer <token>")
			c.Abort()
			return
		}

		// 验证Token并提取用户信息
		userInfo, err := validateJWTToken(tokenString)
		if err != nil {
			logger.Warnf("JWT验证失败: %s, Error: %v", c.Request.URL.Path, err)
			utils.ResponseError(c, http.StatusUnauthorized, "无效或过期的 Token")
			c.Abort()
			return
		}

		// 注入用户信息到Context
		c.Set("user_id", userInfo.UserID)
		c.Set("username", userInfo.Username)
		c.Set("email", userInfo.Email)
		c.Set("role", userInfo.Role)
		c.Set("claims", userInfo.Claims)

		logger.Infof("JWT验证成功: user_id=%d, username=%s", userInfo.UserID, userInfo.Username)

		// 继续执行
		c.Next()
	}
}

// JWTOptionalMiddleware 可选的JWT验证中间件（Token存在时验证，不存在时跳过）
func JWTOptionalMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			// 没有Token，继续执行但不设置用户信息
			c.Next()
			return
		}

		// 提取 Token
		tokenString := strings.TrimPrefix(authHeader, "Bearer ")
		if len(tokenString) == len(authHeader) {
			// Token格式错误，但不强制要求，继续执行
			c.Next()
			return
		}

		// 验证Token
		userInfo, err := validateJWTToken(tokenString)
		if err != nil {
			logger.Warnf("可选JWT验证失败: %s, Error: %v", c.Request.URL.Path, err)
			// 验证失败但不阻止请求，继续执行
			c.Next()
			return
		}

		// 注入用户信息到Context
		c.Set("user_id", userInfo.UserID)
		c.Set("username", userInfo.Username)
		c.Set("email", userInfo.Email)
		c.Set("role", userInfo.Role)
		c.Set("claims", userInfo.Claims)

		logger.Infof("可选JWT验证成功: user_id=%d, username=%s", userInfo.UserID, userInfo.Username)

		// 继续执行
		c.Next()
	}
}

// UserInfo 用户信息结构体
type UserInfo struct {
	UserID   int64         `json:"user_id"`
	Username string        `json:"username"`
	Email    string        `json:"email"`
	Role     string        `json:"role"`
	Claims   jwt.MapClaims `json:"claims"`
}

// validateJWTToken 验证JWT Token并返回用户信息
func validateJWTToken(tokenString string) (*UserInfo, error) {
	// 从注册服务获取公钥
	publicKey, err := utils.GetPublicKey()
	if err != nil {
		return nil, fmt.Errorf("获取公钥失败: %w", err)
	}

	// 解析并验证Token
	token, err := jwt.ParseWithClaims(tokenString, jwt.MapClaims{}, func(token *jwt.Token) (interface{}, error) {
		// 确保是 RSA256
		if token.Method != jwt.SigningMethodRS256 {
			return nil, fmt.Errorf("签名算法不匹配，期望 RS256，实际 %s", token.Method.Alg())
		}
		return publicKey, nil
	})

	if err != nil {
		return nil, fmt.Errorf("验证Token签名失败: %w", err)
	}

	if !token.Valid {
		return nil, fmt.Errorf("Token无效")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, fmt.Errorf("无法解析用户信息")
	}

	// 校验 Issuer
	if issuer, ok := claims["iss"].(string); !ok || issuer != config.AppConfig.Jwt.Issuer {
		return nil, fmt.Errorf("无效签发者: %s", issuer)
	}

	// 校验过期时间
	if exp, ok := claims["exp"].(float64); ok {
		if time.Now().Unix() > int64(exp) {
			return nil, fmt.Errorf("Token已过期")
		}
	}

	// 提取用户信息
	userIDFloat, _ := claims["userId"].(float64) // JWT 数字默认是 float64
	userID := int64(userIDFloat)

	username, _ := claims["sub"].(string)
	email, _ := claims["email"].(string)
	role, _ := claims["role"].(string)

	return &UserInfo{
		UserID:   userID,
		Username: username,
		Email:    email,
		Role:     role,
		Claims:   claims,
	}, nil
}

// GetUserFromContext 从Context中获取用户信息
func GetUserFromContext(c *gin.Context) (*UserInfo, bool) {
	userID, exists := c.Get("user_id")
	if !exists {
		return nil, false
	}

	username, _ := c.Get("username")
	email, _ := c.Get("email")
	role, _ := c.Get("role")
	claims, _ := c.Get("claims")

	return &UserInfo{
		UserID:   userID.(int64),
		Username: username.(string),
		Email:    email.(string),
		Role:     role.(string),
		Claims:   claims.(jwt.MapClaims),
	}, true
}
