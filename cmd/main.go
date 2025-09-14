// cmd/main.go

package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"synapse-backend/internal/config"
	"synapse-backend/pkg/logger"

	"github.com/gin-gonic/gin"
)

func main() {
	// 设置 Gin 为生产模式
	gin.SetMode(gin.ReleaseMode)
	// 初始化配置
	if err := config.InitConfig(); err != nil {
		logger.Errorf("初始化配置失败: %v", err)
		os.Exit(1)
	}

	// Logger 会在首次调用时自动初始化，但显式调用更清晰
	logger.Init()

	r := gin.New()

	r.Use(ginLoggerMiddleware(), gin.Recovery())

	r.GET("/health", func(c *gin.Context) {
		logger.Info("连通性检查成功")
		c.JSON(200, gin.H{"status": "OK"})
	})

	// 启动前打印美观横幅
	printBanner()

	logger.Infof("服务器启动成功，监听端口 :%d", config.AppConfig.Server.Port)

	// 优雅关闭
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		<-sig
		logger.Info("服务器退出进程")
		os.Exit(0)
	}()

	if err := r.Run(fmt.Sprintf(":%d", config.AppConfig.Server.Port)); err != nil {
		logger.Errorf("服务器启动失败: %v", err)
	}

}

func printBanner() {
	banner := `
███████╗██╗   ██╗███╗   ██╗ █████╗ ██████╗ ███████╗███████╗
██╔════╝╚██╗ ██╔╝████╗  ██║██╔══██╗██╔══██╗██╔════╝██╔════╝
███████╗ ╚████╔╝ ██╔██╗ ██║███████║██████╔╝███████╗█████╗  
╚════██║  ╚██╔╝  ██║╚██╗██║██╔══██║██╔═══╝ ╚════██║██╔══╝  
███████║   ██║   ██║ ╚████║██║  ██║██║     ███████║███████╗
╚══════╝   ╚═╝   ╚═╝  ╚═══╝╚═╝  ╚═╝╚═╝     ╚══════╝╚══════╝
                                                           
============== 🌐 Synapse Backend Server 🚀 ==============
`
	fmt.Print(banner)
	fmt.Printf("   ➤ 环境: %s\n", getEnv("APP_ENV", "dev"))
	fmt.Printf("   ➤ 版本: v1.0.0\n")
	fmt.Printf("   ➤ 端口: %d\n", config.AppConfig.Server.Port)
	fmt.Printf("   ➤ 数据库: %s@%s:%d/%s\n",
		config.AppConfig.Database.User,
		config.AppConfig.Database.Host,
		config.AppConfig.Database.Port,
		config.AppConfig.Database.Name)
	fmt.Println()
}

// getEnv 辅助函数（避免重复导入）
func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}

// ginLoggerMiddleware 自定义 Gin 日志中间件
func ginLoggerMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		// 处理请求
		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()
		clientIP := c.ClientIP()
		method := c.Request.Method
		path := c.Request.URL.Path

		// 根据状态码选择日志级别
		if status >= 500 {
			logger.Errorf(" %s | %s | %s | %d | %v | %s",
				clientIP, method, path, status, latency, c.Errors.ByType(gin.ErrorTypePrivate).String())
		} else if status >= 400 {
			logger.Warnf("  %s | %s | %s | %d | %v",
				clientIP, method, path, status, latency)
		} else {
			logger.Infof(" %s | %s | %s | %d | %v",
				clientIP, method, path, status, latency)
		}
	}
}
