package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

var AppConfig Config

// InitConfig 初始化配置
func InitConfig() error {
	// 获取可执行文件所在目录
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("[Config] 无法获取可执行文件路径: %v", err)
	}
	exeDir := filepath.Dir(exePath)

	// 读取环境变量 APP_ENV，默认 dev
	env := getEnv("APP_ENV", "dev")
	configName := "dev"
	if env != "dev" {
		configName = env
	}

	viper.SetConfigName(configName)
	viper.SetConfigType("yml")
	viper.AddConfigPath(exeDir)

	// 读取配置文件
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			return fmt.Errorf("[Config] 未在目录 %q 找到配置文件 %s.yml", exeDir, configName)
		}
		return fmt.Errorf("[Config] 读取配置文件失败: %v", err)
	}

	// 绑定到结构体
	if err := viper.Unmarshal(&AppConfig); err != nil {
		return fmt.Errorf("[Config] 无法解析配置文件: %v", err)
	}

	// 后处理：Trim BaseURL 空格
	for i, provider := range AppConfig.LLM.Providers {
		AppConfig.LLM.Providers[i].BaseURL = strings.TrimSpace(provider.BaseURL)
	}

	// 校验配置
	if err := validateConfig(); err != nil {
		return fmt.Errorf("[Config] 配置校验失败: %v", err)
	}

	return nil
}

// getEnv 获取环境变量，支持默认值
func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}

// validateConfig 校验配置合法性
func validateConfig() error {
	for i, provider := range AppConfig.LLM.Providers {
		if !provider.Enabled {
			continue
		}
		// 如果 default 为空，且有模型，设为第一个
		if provider.Default == "" && len(provider.Models) > 0 {
			AppConfig.LLM.Providers[i].Default = provider.Models[0].Code
		}

		// 校验 default 模型是否存在且启用
		found := false
		for _, model := range provider.Models {
			if model.Code == AppConfig.LLM.Providers[i].Default && model.Enabled {
				found = true
				break
			}
		}
		if !found && len(provider.Models) > 0 {
			AppConfig.LLM.Providers[i].Default = provider.Models[0].Code
		}
	}
	return nil
}
