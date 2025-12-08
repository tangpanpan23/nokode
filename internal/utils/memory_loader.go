package utils

import (
	"os"
	"path/filepath"
)

func LoadMemory() string {
	// Try to find memory.md in the project root
	memoryPath := "memory.md"
	
	// Try to find it by going up directories
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

	if _, err := os.Stat(memoryPath); err != nil {
		return ""
	}

	content, err := os.ReadFile(memoryPath)
	if err != nil {
		return ""
	}

	return string(content)
}

