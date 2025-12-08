package utils

import (
	"encoding/json"
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

// LLMRequest logs the complete LLM API request
func (l *Logger) LLMRequest(provider, url string, headers map[string]string, requestBody interface{}) {
	timestamp := formatTimestamp()
	areaTag := formatArea("llm-request")
	
	fmt.Printf("%s%s%s %s%s%s %s🤖 [%s] Request to %s%s\n", 
		colorGray, timestamp, colorReset, 
		colorMagenta, areaTag, colorReset,
		colorCyan, strings.ToUpper(provider), url, colorReset)
	
	// Log headers (mask API keys for console)
	logHeaders := make(map[string]string)
	for k, v := range headers {
		if strings.Contains(strings.ToLower(k), "authorization") || strings.Contains(strings.ToLower(k), "api-key") {
			if len(v) > 10 {
				logHeaders[k] = v[:7] + "..." + v[len(v)-4:]
			} else {
				logHeaders[k] = "***"
			}
		} else {
			logHeaders[k] = v
		}
	}
	
	// Format and print request body as JSON
	if requestBody != nil {
		jsonData, err := json.MarshalIndent(requestBody, "", "  ")
		if err == nil {
			fmt.Printf("%s%s%s %s%s%s\n", 
				colorGray, strings.Repeat(" ", 24), colorReset, 
				colorDim, "Headers:", colorReset)
			headersJSON, _ := json.MarshalIndent(logHeaders, strings.Repeat(" ", 26), "  ")
			fmt.Printf("%s%s%s %s%s%s\n", 
				colorGray, strings.Repeat(" ", 24), colorReset, 
				colorDim, string(headersJSON), colorReset)
			fmt.Printf("%s%s%s %s%s%s\n", 
				colorGray, strings.Repeat(" ", 24), colorReset, 
				colorDim, "Request Body:", colorReset)
			fmt.Printf("%s%s%s %s%s%s\n", 
				colorGray, strings.Repeat(" ", 24), colorReset, 
				colorDim, string(jsonData), colorReset)
		}
	}
	
	// Save to file (with full headers including API keys)
	l.saveLLMRequestToFile(provider, url, headers, requestBody)
}

// LLMResponse logs the complete LLM API response
func (l *Logger) LLMResponse(provider string, statusCode int, responseBody interface{}, rawBody []byte) {
	timestamp := formatTimestamp()
	areaTag := formatArea("llm-response")
	
	statusColor := colorGreen
	if statusCode != 200 {
		statusColor = colorRed
	}
	
	fmt.Printf("%s%s%s %s%s%s %s📥 [%s] Response Status: %s%d%s\n", 
		colorGray, timestamp, colorReset, 
		colorMagenta, areaTag, colorReset,
		statusColor, strings.ToUpper(provider), statusColor, statusCode, colorReset)
	
	// Log response body
	if responseBody != nil {
		jsonData, err := json.MarshalIndent(responseBody, "", "  ")
		if err == nil {
			fmt.Printf("%s%s%s %s%s%s\n", 
				colorGray, strings.Repeat(" ", 24), colorReset, 
				colorDim, "Response Body:", colorReset)
			fmt.Printf("%s%s%s %s%s%s\n", 
				colorGray, strings.Repeat(" ", 24), colorReset, 
				colorDim, string(jsonData), colorReset)
		}
	} else if len(rawBody) > 0 {
		// If responseBody is nil but we have raw body, try to format it
		var jsonObj interface{}
		if err := json.Unmarshal(rawBody, &jsonObj); err == nil {
			jsonData, _ := json.MarshalIndent(jsonObj, "", "  ")
			fmt.Printf("%s%s%s %s%s%s\n", 
				colorGray, strings.Repeat(" ", 24), colorReset, 
				colorDim, "Response Body:", colorReset)
			fmt.Printf("%s%s%s %s%s%s\n", 
				colorGray, strings.Repeat(" ", 24), colorReset, 
				colorDim, string(jsonData), colorReset)
		} else {
			// If not JSON, print as string (truncated if too long)
			bodyStr := string(rawBody)
			if len(bodyStr) > 1000 {
				bodyStr = bodyStr[:1000] + "... (truncated)"
			}
			fmt.Printf("%s%s%s %s%s%s\n", 
				colorGray, strings.Repeat(" ", 24), colorReset, 
				colorDim, "Response Body (raw):", colorReset)
			fmt.Printf("%s%s%s %s%s%s\n", 
				colorGray, strings.Repeat(" ", 24), colorReset, 
				colorDim, bodyStr, colorReset)
		}
	}
	
	// Save to file
	l.saveLLMResponseToFile(provider, statusCode, responseBody, rawBody)
}

// saveLLMRequestToFile saves the complete LLM request to a JSON file
func (l *Logger) saveLLMRequestToFile(provider, url string, headers map[string]string, requestBody interface{}) {
	logDir := "logs/llm"
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return // Silently fail if can't create directory
	}

	timestamp := time.Now()
	filename := fmt.Sprintf("%s/%s_request_%s.json", 
		logDir, 
		strings.ToLower(provider),
		timestamp.Format("20060102_150405.000"))
	
	logEntry := map[string]interface{}{
		"timestamp": timestamp.Format(time.RFC3339Nano),
		"provider":  provider,
		"url":       url,
		"headers":   headers, // Full headers including API keys
		"request":   requestBody,
	}
	
	jsonData, err := json.MarshalIndent(logEntry, "", "  ")
	if err != nil {
		return
	}
	
	os.WriteFile(filename, jsonData, 0644)
}

// saveLLMResponseToFile saves the complete LLM response to a JSON file
func (l *Logger) saveLLMResponseToFile(provider string, statusCode int, responseBody interface{}, rawBody []byte) {
	logDir := "logs/llm"
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return // Silently fail if can't create directory
	}

	timestamp := time.Now()
	filename := fmt.Sprintf("%s/%s_response_%s.json", 
		logDir, 
		strings.ToLower(provider),
		timestamp.Format("20060102_150405.000"))
	
	logEntry := map[string]interface{}{
		"timestamp":    timestamp.Format(time.RFC3339Nano),
		"provider":     provider,
		"status_code":  statusCode,
		"response":     responseBody,
		"raw_response": string(rawBody),
	}
	
	jsonData, err := json.MarshalIndent(logEntry, "", "  ")
	if err != nil {
		return
	}
	
	os.WriteFile(filename, jsonData, 0644)
}

