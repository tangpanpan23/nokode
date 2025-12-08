package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func LoadPrompt() string {
	// Try to find prompt.md in the project root
	// First try current directory, then go up to find it
	promptPath := "prompt.md"
	
	// Try to find it by going up directories
	for i := 0; i < 5; i++ {
		if _, err := os.Stat(promptPath); err == nil {
			break
		}
		promptPath = filepath.Join("..", promptPath)
	}
	
	// If still not found, try absolute path from executable location
	if _, err := os.Stat(promptPath); err != nil {
		// Try to get executable directory
		exe, err := os.Executable()
		if err == nil {
			exeDir := filepath.Dir(exe)
			promptPath = filepath.Join(exeDir, "prompt.md")
		}
	}

	content, err := os.ReadFile(promptPath)
	if err != nil {
		// Fallback prompt
		return `You are a web server. Generate an appropriate response for this HTTP request using the webResponse tool.

Request Information:
Method: {{METHOD}}
Path: {{PATH}}
URL: {{URL}}
Query Parameters: {{QUERY}}
Headers: {{HEADERS}}
Body: {{BODY}}
Client IP: {{IP}}
Timestamp: {{TIMESTAMP}}

Use the webResponse tool to generate an appropriate response.`
	}

	return string(content)
}

func ReplaceTemplateVars(template string, vars map[string]string) string {
	result := template
	for key, value := range vars {
		placeholder := fmt.Sprintf("{{%s}}", key)
		result = strings.ReplaceAll(result, placeholder, value)
	}
	return result
}

