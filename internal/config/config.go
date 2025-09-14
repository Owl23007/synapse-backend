// Config 全局配置结构体
package config

type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	LLM      LLMConfig      `mapstructure:"llm"`
	Logger   LoggerConfig   `mapstructure:"logger"`
	Database DatabaseConfig `mapstructure:"database"`
}

// ServerConfig 服务配置
type ServerConfig struct {
	Port int `mapstructure:"port"`
}

// LLMConfig LLM 配置
type LLMConfig struct {
	Providers []LLMProvider `mapstructure:"providers"`
}

// LLMProvider LLM 提供商
type LLMProvider struct {
	Name    string     `mapstructure:"name"`
	APIKey  string     `mapstructure:"api_key"`
	BaseURL string     `mapstructure:"base_url"`
	Models  []LLMModel `mapstructure:"models"`
	Default string     `mapstructure:"default"`
	Enabled bool       `mapstructure:"enabled"`
}

// LLMModel 模型定义
type LLMModel struct {
	Code        string `mapstructure:"code"`
	Name        string `mapstructure:"name"`
	Description string `mapstructure:"description"`
	Stream      bool   `mapstructure:"stream"`
	Enabled     bool   `mapstructure:"enabled"`
}

// LoggerConfig 日志配置
type LoggerConfig struct {
	Level     string `mapstructure:"level"`
	OutputDir string `mapstructure:"output_dir"`
	Output    bool   `mapstructure:"output"`
}

// DatabaseConfig 数据库配置
type DatabaseConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	Name     string `mapstructure:"name"`
	Charset  string `mapstructure:"charset"`
	Timezone string `mapstructure:"timezone"`
}
