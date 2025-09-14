// internal/middleware/jwt.go

package middleware

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"synapse-backend/internal/config"
	"synapse-backend/internal/utils"
	"synapse-backend/pkg/logger"
)

// JWTAuthMiddleware 使用 JWKS 公钥验证 JWT Token
func JWTAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "请求头中缺少 Authorization",
			})
			c.Abort()
			return
		}

		// 提取 Token
		tokenString := strings.TrimPrefix(authHeader, "Bearer ")
		if len(tokenString) == len(authHeader) {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Authorization 格式错误，应为 Bearer <token>",
			})
			c.Abort()
			return
		}

		// 先解析 Token Header 获取 kid（不验证签名）
		token, err := jwt.Parse(tokenString, nil)
		if err != nil {
			logger.Warnf("解析 Token Header 失败: %v", err)
			c.JSON(http.StatusUnauthorized, gin.H{"error": "无效 Token"})
			c.Abort()
			return
		}

		// 获取 kid
		kid, ok := token.Header["kid"].(string)
		if !ok {
			logger.Warnf("Token Header 中缺少 kid")
			c.JSON(http.StatusUnauthorized, gin.H{"error": "无效 Token: 缺少 kid"})
			c.Abort()
			return
		}

		// 从注册服务获取公钥
		publicKey, err := utils.GetPublicKey()
		if err != nil {
			logger.Warnf("未找到 kid=%s 的公钥: %v", kid, err)
			c.JSON(http.StatusUnauthorized, gin.H{"error": "无效 Token: 未知密钥"})
			c.Abort()
			return
		}

		// 重新解析并验证签名
		token, err = jwt.ParseWithClaims(tokenString, jwt.MapClaims{}, func(token *jwt.Token) (interface{}, error) {
			// 确保是 RSA256
			if token.Method != jwt.SigningMethodRS256 {
				return nil, errors.New("签名算法不匹配，期望 RS256")
			}
			return publicKey, nil
		})

		if err != nil {
			logger.Warnf("验证 Token 签名失败: %v", err)
			c.JSON(http.StatusUnauthorized, gin.H{"error": "无效或过期的 Token"})
			c.Abort()
			return
		}

		if !token.Valid {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Token 无效"})
			c.Abort()
			return
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "无法解析用户信息"})
			c.Abort()
			return
		}

		// 校验 Issuer
		if issuer, ok := claims["iss"].(string); !ok || issuer != config.AppConfig.Jwt.Issuer {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "无效签发者"})
			c.Abort()
			return
		}

		// 提取用户信息
		userIDFloat, _ := claims["userId"].(float64) // JWT 数字默认是 float64
		userID := int64(userIDFloat)

		username, _ := claims["sub"].(string)
		email, _ := claims["email"].(string)
		role, _ := claims["role"].(string)

		// 注入 Context
		c.Set("user_id", userID)
		c.Set("username", username)
		c.Set("email", email)
		c.Set("role", role)
		c.Set("claims", claims)

		// 继续执行
		c.Next()
	}
}
