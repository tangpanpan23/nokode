package config

import (
	"fmt"
	"os"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/rest"
)

type Config struct {
	RestConf rest.RestConf `yaml:",inline"`
	Provider string        `json:",optional"`
	Database struct {
		Host     string `json:",optional"`
		Port     int    `json:",optional"`
		User     string `json:",optional"`
		Password string `json:",optional"`
		Database string `json:",optional"`
	}
	Qwen struct {
		Model  string `json:",optional"`
		APIKey string `json:",optional"`
	}
	Anthropic struct {
		Model  string `json:",optional"`
		APIKey string `json:",optional"`
	}
	OpenAI struct {
		Model  string `json:",optional"`
		APIKey string `json:",optional"`
	}
}

func Load(configFile string) (*Config, error) {
	var c Config
	
	// Load from YAML file if provided
	if configFile != "" {
		conf.MustLoad(configFile, &c)
	}
	
	// Override with environment variables
	if c.Provider == "" {
		c.Provider = getEnv("LLM_PROVIDER", "qwen")
	}
	if c.RestConf.Port == 0 {
		portStr := getEnv("PORT", "3001")
		var port int
		if _, err := fmt.Sscanf(portStr, "%d", &port); err == nil {
			c.RestConf.Port = port
		} else {
			c.RestConf.Port = 3001
		}
	}
	if c.RestConf.Host == "" {
		c.RestConf.Host = "0.0.0.0"
	}
	if c.RestConf.Timeout == 0 {
		c.RestConf.Timeout = 300000
	}

	// Database configuration
	if c.Database.Host == "" {
		c.Database.Host = getEnv("DB_HOST", "localhost")
	}
	if c.Database.Port == 0 {
		portStr := getEnv("DB_PORT", "3306")
		fmt.Sscanf(portStr, "%d", &c.Database.Port)
		if c.Database.Port == 0 {
			c.Database.Port = 3306
		}
	}
	if c.Database.User == "" {
		c.Database.User = getEnv("DB_USER", "root")
	}
	if c.Database.Password == "" {
		c.Database.Password = getEnv("DB_PASSWORD", "")
	}
	if c.Database.Database == "" {
		c.Database.Database = getEnv("DB_NAME", "nokode")
	}

	c.Qwen.Model = getEnv("QWEN_MODEL", "qwen-turbo")
	if c.Qwen.APIKey == "" {
		c.Qwen.APIKey = getEnv("QWEN_API_KEY", getEnv("DASHSCOPE_API_KEY", ""))
	}

	c.Anthropic.Model = getEnv("ANTHROPIC_MODEL", "claude-3-haiku-20240307")
	if c.Anthropic.APIKey == "" {
		c.Anthropic.APIKey = getEnv("ANTHROPIC_API_KEY", "")
	}

	c.OpenAI.Model = getEnv("OPENAI_MODEL", "gpt-4-turbo-preview")
	if c.OpenAI.APIKey == "" {
		c.OpenAI.APIKey = getEnv("OPENAI_API_KEY", "")
	}

	return &c, nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

