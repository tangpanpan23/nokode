package main

import (
	"log"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/nokode/nokode/internal/config"
	"github.com/nokode/nokode/internal/middleware"
)

func main() {
	// Load environment variables
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	cfg := config.Load()

	// Set Gin mode
	if os.Getenv("GIN_MODE") == "" {
		gin.SetMode(gin.ReleaseMode)
	}

	// Create router
	r := gin.Default()

	// Parse JSON and form data
	r.Use(gin.Recovery())

	// All requests are handled by the LLM
	r.NoRoute(middleware.HandleLLMRequest(cfg))

	// Start the server
	port := cfg.Port
	if port == "" {
		port = "3001"
	}

	log.Printf("🤖 nokode server running on http://localhost:%s", port)
	log.Printf("🧠 Using %s provider", cfg.Provider)

	var model string
	if cfg.Provider == "anthropic" {
		model = cfg.Anthropic.Model
	} else {
		model = cfg.OpenAI.Model
	}
	log.Printf("⚡ Model: %s", model)
	log.Printf("🚀 Every request will be handled by AI. Make any HTTP request and see what happens.")
	log.Printf("💰 Warning: Each request costs API tokens!")

	if err := r.Run(":" + port); err != nil {
		log.Fatal("Failed to start server:", err)
	}
}

