package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type MemoryResult struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

func UpdateMemory(content string, mode string) MemoryResult {
	memoryPath := "memory.md"
	
	// Try to find it in project root
	for i := 0; i < 5; i++ {
		if _, err := os.Stat(memoryPath); err == nil {
			break
		}
		memoryPath = filepath.Join("..", memoryPath)
	}
	
	// If still not found, try absolute path from executable location
	if _, err := os.Stat(memoryPath); err != nil {
		exe, err := os.Executable()
		if err == nil {
			exeDir := filepath.Dir(exe)
			memoryPath = filepath.Join(exeDir, "memory.md")
		}
	}

	var err error
	if mode == "append" {
		// Append mode
		var existingContent string
		if _, statErr := os.Stat(memoryPath); statErr == nil {
			existingBytes, readErr := os.ReadFile(memoryPath)
			if readErr == nil {
				existingContent = string(existingBytes)
			}
		}

		// Add newline if needed
		if existingContent != "" && !strings.HasSuffix(existingContent, "\n") {
			existingContent += "\n"
		}

		err = os.WriteFile(memoryPath, []byte(existingContent+content), 0644)
		if err == nil {
			return MemoryResult{
				Success: true,
				Message: "Memory appended successfully",
			}
		}
	} else {
		// Rewrite mode
		err = os.WriteFile(memoryPath, []byte(content), 0644)
		if err == nil {
			return MemoryResult{
				Success: true,
				Message: "Memory rewritten successfully",
			}
		}
	}

	return MemoryResult{
		Success: false,
		Message: fmt.Sprintf("Failed to update memory: %v", err),
	}
}

