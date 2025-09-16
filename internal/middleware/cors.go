// internal/middleware/cors.go

package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// CORSMiddleware 跨域资源共享中间件
func CORSMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")

		// 设置允许的源站
		if origin != "" {
			c.Header("Access-Control-Allow-Origin", origin)
		} else {
			c.Header("Access-Control-Allow-Origin", "*")
		}

		// 设置允许的请求方法
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS, PATCH")

		// 设置允许的请求头
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Accept, Authorization, X-Requested-With, Content-Length, Accept-Encoding, X-CSRF-Token")

		// 设置允许凭证
		c.Header("Access-Control-Allow-Credentials", "true")

		// 设置预检请求的缓存时间
		c.Header("Access-Control-Max-Age", "86400")

		// 如果是 OPTIONS 预检请求，直接返回 200
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusOK)
			return
		}

		c.Next()
	}
}
