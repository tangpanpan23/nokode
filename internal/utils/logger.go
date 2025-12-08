package utils

import (
	"fmt"
	"os"
	"strings"
	"time"
)

const (
	colorReset  = "\033[0m"
	colorBright = "\033[1m"
	colorDim    = "\033[2m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorMagenta = "\033[35m"
	colorCyan   = "\033[36m"
	colorWhite  = "\033[37m"
	colorGray   = "\033[90m"
)

type Logger struct{}

var Log = &Logger{}

func formatTimestamp() string {
	return time.Now().Format("2006-01-02 15:04:05.000")
}

func formatArea(area string) string {
	return fmt.Sprintf("[%s]", strings.ToUpper(area))
}

func (l *Logger) Info(area, message string, data interface{}) {
	timestamp := formatTimestamp()
	areaTag := formatArea(area)
	fmt.Printf("%s%s%s %s%s%s %s%s%s\n", colorGray, timestamp, colorReset, colorCyan, areaTag, colorReset, colorWhite, message, colorReset)
	if data != nil {
		printData(data)
	}
}

func (l *Logger) Success(area, message string, data interface{}) {
	timestamp := formatTimestamp()
	areaTag := formatArea(area)
	fmt.Printf("%s%s%s %s%s%s %s✅ %s%s\n", colorGray, timestamp, colorReset, colorGreen, areaTag, colorReset, colorGreen, message, colorReset)
	if data != nil {
		printData(data)
	}
}

func (l *Logger) Error(area, message string, err error) {
	timestamp := formatTimestamp()
	areaTag := formatArea(area)
	fmt.Printf("%s%s%s %s%s%s %s❌ %s%s\n", colorGray, timestamp, colorReset, colorRed, areaTag, colorReset, colorRed, message, colorReset)
	if err != nil {
		fmt.Printf("%s%s%s %s%s%s\n", colorGray, strings.Repeat(" ", 24), colorReset, colorRed, err.Error(), colorReset)
		if os.Getenv("DEBUG") == "true" {
			fmt.Printf("%s%s\n", colorRed, err)
		}
	}
}

func (l *Logger) Warn(area, message string, data interface{}) {
	timestamp := formatTimestamp()
	areaTag := formatArea(area)
	fmt.Printf("%s%s%s %s%s%s %s⚠️  %s%s\n", colorGray, timestamp, colorReset, colorYellow, areaTag, colorReset, colorYellow, message, colorReset)
	if data != nil {
		printData(data)
	}
}

func (l *Logger) Debug(area, message string, data interface{}) {
	if os.Getenv("DEBUG") != "true" {
		return
	}
	timestamp := formatTimestamp()
	areaTag := formatArea(area)
	fmt.Printf("%s%s%s %s%s%s %s🔧 %s%s\n", colorGray, timestamp, colorReset, colorMagenta, areaTag, colorReset, colorDim, message, colorReset)
	if data != nil {
		printData(data)
	}
}

func (l *Logger) Database(message string, data interface{}) {
	timestamp := formatTimestamp()
	areaTag := formatArea("database")
	fmt.Printf("%s%s%s %s%s%s %s📊 %s%s\n", colorGray, timestamp, colorReset, colorBlue, areaTag, colorReset, colorBlue, message, colorReset)
	if data != nil {
		printData(data)
	}
}

func (l *Logger) Request(method, path string, data interface{}) {
	timestamp := formatTimestamp()
	areaTag := formatArea("request")
	
	var methodColor string
	switch method {
	case "GET":
		methodColor = colorGreen
	case "POST":
		methodColor = colorYellow
	case "PUT":
		methodColor = colorBlue
	case "DELETE":
		methodColor = colorRed
	default:
		methodColor = colorWhite
	}
	
	fmt.Printf("%s%s%s %s%s%s %s%s%s %s%s%s\n", 
		colorGray, timestamp, colorReset, 
		colorCyan, areaTag, colorReset, 
		methodColor, method, colorReset, 
		colorWhite, path, colorReset)
	if data != nil {
		printData(data)
	}
}

func (l *Logger) Tool(toolName, message string, data interface{}) {
	timestamp := formatTimestamp()
	areaTag := formatArea("tool")
	fmt.Printf("%s%s%s %s%s%s %s%s🔧 %s%s %s%s%s\n", 
		colorGray, timestamp, colorReset, 
		colorMagenta, areaTag, colorReset, 
		colorBright, toolName, colorReset, 
		colorWhite, message, colorReset)
	if data != nil {
		printData(data)
	}
}

func (l *Logger) Separator(title string) {
	timestamp := formatTimestamp()
	line := strings.Repeat("═", 60)
	if title != "" {
		paddedTitle := fmt.Sprintf(" %s ", title)
		padding := (60 - len(paddedTitle)) / 2
		leftPad := strings.Repeat("═", padding)
		rightPad := strings.Repeat("═", 60-padding-len(paddedTitle))
		fmt.Printf("%s%s%s %s%s%s%s%s\n", colorGray, timestamp, colorReset, colorCyan, leftPad, paddedTitle, rightPad, colorReset)
	} else {
		fmt.Printf("%s%s%s %s%s%s\n", colorGray, timestamp, colorReset, colorCyan, line, colorReset)
	}
}

func printData(data interface{}) {
	// Simple data printing - can be enhanced with JSON formatting
	if data != nil {
		fmt.Printf("%s%s%s %s%v%s\n", colorGray, strings.Repeat(" ", 24), colorReset, colorDim, data, colorReset)
	}
}

