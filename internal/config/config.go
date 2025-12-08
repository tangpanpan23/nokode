package config

import (
	"os"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/rest"
)

type Config struct {
	RestConf rest.RestConf `yaml:",inline"`
	Port     string        `json:",optional"`
	Provider string        `json:",optional"`
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
	if c.Port == "" {
		c.Port = getEnv("PORT", "3001")
	}
	if c.Provider == "" {
		c.Provider = getEnv("LLM_PROVIDER", "anthropic")
	}
	if c.RestConf.Port == 0 {
		c.RestConf.Port = 3001
	}
	if c.RestConf.Host == "" {
		c.RestConf.Host = "0.0.0.0"
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

