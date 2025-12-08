package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
	"github.com/nokode/nokode/internal/config"
	"github.com/nokode/nokode/internal/handler"
	"github.com/nokode/nokode/internal/tools"
	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/rest"
)

var configFile = flag.String("f", "etc/nokode-api.yaml", "the config file")

func main() {
	flag.Parse()

	// Load environment variables
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	// Load config
	var c config.Config
	conf.MustLoad(*configFile, &c)

	// Override with environment variables
	portStr := getEnv("PORT", "3001")
	if c.RestConf.Port == 0 {
		// Try to parse port from string
		var port int
		if _, err := fmt.Sscanf(portStr, "%d", &port); err == nil {
			c.RestConf.Port = port
		} else {
			c.RestConf.Port = 3001
		}
	}
	if c.Provider == "" {
		c.Provider = getEnv("LLM_PROVIDER", "anthropic")
	}
	if c.RestConf.Host == "" {
		c.RestConf.Host = "0.0.0.0"
	}
	if c.RestConf.Timeout == 0 {
		c.RestConf.Timeout = 300000 // 5 minutes in milliseconds
	}

	c.Anthropic.Model = getEnv("ANTHROPIC_MODEL", "claude-3-haiku-20240307")
	if c.Anthropic.APIKey == "" {
		c.Anthropic.APIKey = getEnv("ANTHROPIC_API_KEY", "")
	}

	c.OpenAI.Model = getEnv("OPENAI_MODEL", "gpt-4-turbo-preview")
	if c.OpenAI.APIKey == "" {
		c.OpenAI.APIKey = getEnv("OPENAI_API_KEY", "")
	}

	// Initialize database
	if err := tools.InitDatabase(&c); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	// Create server
	server := rest.MustNewServer(c.RestConf)
	defer server.Stop()

	// Register catch-all route for all methods and paths
	llmHandler := handler.HandleLLMRequest(&c)
	server.AddRoute(rest.Route{
		Method:  "GET",
		Path:    "/",
		Handler: llmHandler,
	})
	server.AddRoute(rest.Route{
		Method:  "POST",
		Path:    "/",
		Handler: llmHandler,
	})
	server.AddRoute(rest.Route{
		Method:  "PUT",
		Path:    "/",
		Handler: llmHandler,
	})
	server.AddRoute(rest.Route{
		Method:  "DELETE",
		Path:    "/",
		Handler: llmHandler,
	})
	server.AddRoute(rest.Route{
		Method:  "GET",
		Path:    "/:path",
		Handler: llmHandler,
	})
	server.AddRoute(rest.Route{
		Method:  "POST",
		Path:    "/:path",
		Handler: llmHandler,
	})
	server.AddRoute(rest.Route{
		Method:  "PUT",
		Path:    "/:path",
		Handler: llmHandler,
	})
	server.AddRoute(rest.Route{
		Method:  "DELETE",
		Path:    "/:path",
		Handler: llmHandler,
	})
	server.AddRoute(rest.Route{
		Method:  "GET",
		Path:    "/:path/:rest",
		Handler: llmHandler,
	})
	server.AddRoute(rest.Route{
		Method:  "POST",
		Path:    "/:path/:rest",
		Handler: llmHandler,
	})
	server.AddRoute(rest.Route{
		Method:  "PUT",
		Path:    "/:path/:rest",
		Handler: llmHandler,
	})
	server.AddRoute(rest.Route{
		Method:  "DELETE",
		Path:    "/:path/:rest",
		Handler: llmHandler,
	})

	log.Printf("🤖 nokode server running on http://localhost:%d", c.RestConf.Port)
	log.Printf("🧠 Using %s provider", c.Provider)

	var model string
	if c.Provider == "anthropic" {
		model = c.Anthropic.Model
	} else {
		model = c.OpenAI.Model
	}
	log.Printf("⚡ Model: %s", model)
	log.Printf("🚀 Every request will be handled by AI. Make any HTTP request and see what happens.")
	log.Printf("💰 Warning: Each request costs API tokens!")

	server.Start()
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
