package config

import (
	"os"
)

type Config struct {
	Port     string
	Provider string
	Anthropic struct {
		Model  string
		APIKey string
	}
	OpenAI struct {
		Model  string
		APIKey string
	}
}

func Load() *Config {
	cfg := &Config{
		Port:     getEnv("PORT", "3001"),
		Provider: getEnv("LLM_PROVIDER", "anthropic"),
	}

	cfg.Anthropic.Model = getEnv("ANTHROPIC_MODEL", "claude-3-haiku-20240307")
	cfg.Anthropic.APIKey = getEnv("ANTHROPIC_API_KEY", "")

	cfg.OpenAI.Model = getEnv("OPENAI_MODEL", "gpt-4-turbo-preview")
	cfg.OpenAI.APIKey = getEnv("OPENAI_API_KEY", "")

	return cfg
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

