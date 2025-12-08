package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/nokode/nokode/internal/config"
	"github.com/nokode/nokode/internal/tools"
	"github.com/nokode/nokode/internal/utils"
	openai "github.com/sashabaranov/go-openai"
)

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// createHTTPClient creates an HTTP client with proper timeout and DNS configuration
func createHTTPClient() *http.Client {
	// Create a custom dialer with timeout
	dialer := &net.Dialer{
		Timeout:   30 * time.Second, // DNS lookup and connection timeout
		KeepAlive: 30 * time.Second,
	}

	// Create transport with custom dialer
	transport := &http.Transport{
		DialContext:           dialer.DialContext,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
	}

	return &http.Client{
		Timeout:   300 * time.Second, // Total request timeout
		Transport: transport,
	}
}

// doHTTPRequestWithRetry performs HTTP request with retry logic for network errors
func doHTTPRequestWithRetry(client *http.Client, req *http.Request, maxRetries int) (*http.Response, error) {
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff: 1s, 2s, 4s
			backoff := time.Duration(1<<uint(attempt-1)) * time.Second
			utils.Log.Warn("llm", fmt.Sprintf("Retrying request (attempt %d/%d) after %v", attempt, maxRetries, backoff), nil)
			time.Sleep(backoff)
		}

		resp, err := client.Do(req)
		if err == nil {
			return resp, nil
		}

		lastErr = err

		// Check if it's a network error that might be retryable
		if isRetryableError(err) {
			utils.Log.Warn("llm", fmt.Sprintf("Network error (attempt %d/%d): %v", attempt+1, maxRetries+1, err), nil)
			continue
		}

		// Non-retryable error, return immediately
		return nil, err
	}

	return nil, fmt.Errorf("request failed after %d attempts: %w", maxRetries+1, lastErr)
}

// isRetryableError checks if an error is retryable (network/DNS issues)
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()
	retryablePatterns := []string{
		"timeout",
		"connection refused",
		"connection reset",
		"no such host",
		"i/o timeout",
		"lookup",
		"temporary failure",
		"network is unreachable",
	}

	for _, pattern := range retryablePatterns {
		if strings.Contains(strings.ToLower(errStr), pattern) {
			return true
		}
	}

	return false
}

func HandleLLMRequest(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		requestStartTime := time.Now()
		requestID := uuid.New().String()[:9]

		// Get the actual path (go-zero might have path parameters)
		path := r.URL.Path
		// Replace path parameters with actual values for logging
		utils.Log.Request(r.Method, path, map[string]interface{}{
			"requestId": requestID,
			"query":     r.URL.Query(),
			"ip":        getClientIP(r),
		})

		// Prepare request context
		var bodyBytes []byte
		if r.Body != nil {
			bodyBytes, _ = io.ReadAll(r.Body)
			r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		}

		var body interface{}
		if len(bodyBytes) > 0 {
			json.Unmarshal(bodyBytes, &body)
		}

		// Load memory and prompt
		memory := utils.LoadMemory()
		promptTemplate := utils.LoadPrompt()
		schema := tools.GetCachedSchema()
		dbContext := tools.GetDatabaseContext()

		// Replace template variables
		queryJSON, _ := json.Marshal(r.URL.Query())
		headersJSON, _ := json.Marshal(r.Header)
		bodyJSON, _ := json.Marshal(body)

		vars := map[string]string{
			"METHOD":    r.Method,
			"PATH":      path,
			"URL":       r.URL.String(),
			"QUERY":     string(queryJSON),
			"HEADERS":   string(headersJSON),
			"BODY":      string(bodyJSON),
			"IP":        getClientIP(r),
			"TIMESTAMP": time.Now().Format(time.RFC3339),
			"MEMORY":    memory + schema + dbContext,
		}

		prompt := utils.ReplaceTemplateVars(promptTemplate, vars)

		// Define tools
		toolsList := getTools()

		// Call LLM
		llmStartTime := time.Now()
		response, err := callLLM(cfg, prompt, toolsList)
		llmDuration := time.Since(llmStartTime).Milliseconds()

		if err != nil {
			utils.Log.Error("llm", "LLM call failed", err)
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, `
				<html>
					<body>
						<h1>Server Error</h1>
						<p>An error occurred while processing your request.</p>
						<p><strong>Request ID:</strong> %s</p>
						<pre>%s</pre>
					</body>
				</html>
			`, requestID, err.Error())
			return
		}

		utils.Log.Info("llm", fmt.Sprintf("LLM call completed in %dms", llmDuration), map[string]interface{}{
			"requestId": requestID,
			"duration":  llmDuration,
		})

		// Check if we need to process tool calls first (OpenAI/Qwen compatible)
		// Anthropic tool calls are handled in callAnthropic
		if cfg.Provider == "openai" || cfg.Provider == "qwen" {
			needsToolProcessing := false
			for _, choice := range response.Choices {
				if choice.FinishReason == "tool_calls" {
					needsToolProcessing = true
					break
				}
			}

			// If tool calls are needed, process them recursively
			if needsToolProcessing {
				finalResponse, err := processToolCallsRecursive(cfg, prompt, toolsList, response)
				if err != nil {
					utils.Log.Error("llm", "Failed to process tool calls", err)
				} else {
					response = finalResponse
				}
			}
		}

		// Extract webResponse from final response
		webResponse := extractWebResponse(response)

		// Send response
		totalDuration := time.Since(requestStartTime).Milliseconds()
		if webResponse != nil {
			// Set status code
			w.WriteHeader(webResponse.StatusCode)

			// Set headers
			for key, value := range webResponse.Headers {
				w.Header().Set(key, value)
			}

			// Send body
			w.Write([]byte(webResponse.Body))
			utils.Log.Success("response", fmt.Sprintf("Sent webResponse (%d) in %dms", webResponse.StatusCode, totalDuration), nil)
		} else {
			// Fallback: return text response
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("No response generated"))
			utils.Log.Warn("response", "No webResponse found, returning default", nil)
		}
	}
}

func getClientIP(r *http.Request) string {
	// Try X-Forwarded-For header first
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		ips := strings.Split(xff, ",")
		if len(ips) > 0 {
			return strings.TrimSpace(ips[0])
		}
	}

	// Try X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Fall back to RemoteAddr
	ip := r.RemoteAddr
	if idx := strings.LastIndex(ip, ":"); idx != -1 {
		ip = ip[:idx]
	}
	return ip
}

// Copy all the LLM-related types and functions from middleware
// (ToolCall, ToolResult, Message, LLMRequest, Tool, ToolFunction, LLMResponse, Choice, Usage)
// and all the helper functions (getTools, executeToolCall, extractWebResponse, etc.)

type ToolCall struct {
	Type      string                 `json:"type"`
	ID        string                 `json:"id"`
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

type ToolResult struct {
	ToolCallID string      `json:"tool_call_id"`
	Content    interface{} `json:"content"`
}

type Message struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"`
}

type LLMRequest struct {
	Model       string      `json:"model"`
	Messages    []Message   `json:"messages"`
	Tools       []Tool      `json:"tools,omitempty"`
	ToolChoice  interface{} `json:"tool_choice,omitempty"`
	MaxTokens   int         `json:"max_tokens,omitempty"`
	Temperature float64     `json:"temperature,omitempty"`
}

type Tool struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

type ToolFunction struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

type LLMResponse struct {
	ID      string   `json:"id"`
	Choices []Choice `json:"choices"`
	Usage   Usage    `json:"usage"`
}

type Choice struct {
	Index        int     `json:"index"`
	Message      Message `json:"message"`
	FinishReason string  `json:"finish_reason"`
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

func getTools() []Tool {
	return []Tool{
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "database",
				Description: "Execute SQL queries on the SQLite database. You can create tables, insert data, query, update, delete - any SQL operation.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"query": map[string]interface{}{
							"type":        "string",
							"description": "The SQL query to execute",
						},
						"params": map[string]interface{}{
							"type":        "array",
							"description": "Optional parameters for prepared statements (prevents SQL injection)",
						},
						"mode": map[string]interface{}{
							"type":        "string",
							"enum":        []string{"query", "exec"},
							"default":     "query",
							"description": "Mode: 'query' for SELECT/returning data, 'exec' for DDL/multiple statements",
						},
					},
					"required": []string{"query"},
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "webResponse",
				Description: "Generate a web response with full control over status, headers, and body",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"statusCode": map[string]interface{}{
							"type":        "number",
							"description": "HTTP status code (default 200)",
						},
						"contentType": map[string]interface{}{
							"type":        "string",
							"description": "Content-Type header value",
						},
						"body": map[string]interface{}{
							"type":        "string",
							"description": "Response body as a string (can be HTML, JSON string, plain text, etc.)",
						},
					},
					"required": []string{"body"},
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "updateMemory",
				Description: "Update persistent memory to store user feedback, preferences, and instructions that shape the application.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"content": map[string]interface{}{
							"type":        "string",
							"description": "User preferences, feedback, or instructions to save (markdown format)",
						},
						"mode": map[string]interface{}{
							"type":        "string",
							"enum":        []string{"append", "rewrite"},
							"description": "Whether to append to existing memory or rewrite the entire file",
						},
					},
					"required": []string{"content", "mode"},
				},
			},
		},
	}
}

func executeToolCall(tcMap map[string]interface{}) interface{} {
	toolName, _ := tcMap["name"].(string)
	argsRaw, _ := tcMap["arguments"]

	var args map[string]interface{}
	if argsStr, ok := argsRaw.(string); ok {
		json.Unmarshal([]byte(argsStr), &args)
	} else if argsMap, ok := argsRaw.(map[string]interface{}); ok {
		args = argsMap
	}

	utils.Log.Tool(toolName, "called", args)

	switch toolName {
	case "database":
		query, _ := args["query"].(string)
		mode := "query"
		if m, ok := args["mode"].(string); ok {
			mode = m
		}

		var params []interface{}
		if p, ok := args["params"].([]interface{}); ok {
			params = p
		}

		result := tools.ExecuteDatabaseQuery(query, params, mode)
		return result

	case "webResponse":
		statusCode := 200
		if sc, ok := args["statusCode"].(float64); ok {
			statusCode = int(sc)
		}
		contentType, _ := args["contentType"].(string)
		body, _ := args["body"].(string)

		response := tools.CreateWebResponse(statusCode, contentType, body)
		return &response

	case "updateMemory":
		content, _ := args["content"].(string)
		mode, _ := args["mode"].(string)
		result := tools.UpdateMemory(content, mode)
		return result

	default:
		return nil
	}
}

func extractWebResponse(response *LLMResponse) *tools.WebResponse {
	// Look through all choices for webResponse tool result
	for _, choice := range response.Choices {
		if choice.Message.Role == "assistant" {
			// Check if content is a string that might contain webResponse info
			if contentStr, ok := choice.Message.Content.(string); ok {
				// Try to parse as JSON to find webResponse
				var result map[string]interface{}
				if err := json.Unmarshal([]byte(contentStr), &result); err == nil {
					if statusCode, ok := result["statusCode"].(float64); ok {
						body, _ := result["body"].(string)
						contentType, _ := result["contentType"].(string)
						wr := tools.CreateWebResponse(int(statusCode), contentType, body)
						return &wr
					}
				}
			}
		}
	}
	return nil
}

func processToolCallsRecursive(cfg *config.Config, initialPrompt string, toolsList []Tool, initialResp *LLMResponse) (*LLMResponse, error) {
	if cfg.Provider == "qwen" {
		return processToolCallsQwen(cfg, initialPrompt, toolsList, initialResp)
	} else if cfg.Provider == "openai" {
		return processToolCallsOpenAI(cfg, initialPrompt, toolsList, initialResp)
	} else {
		// Anthropic tool calls are handled in callAnthropic function
		// This should not be called for Anthropic as tool calls are handled there
		return initialResp, nil
	}
}

func callLLM(cfg *config.Config, prompt string, toolsList []Tool) (*LLMResponse, error) {
	if cfg.Provider == "qwen" {
		return callQwen(cfg, prompt, toolsList)
	} else if cfg.Provider == "openai" {
		return callOpenAI(cfg, prompt, toolsList)
	} else if cfg.Provider == "anthropic" {
		return callAnthropic(cfg, prompt, toolsList)
	}
	return nil, fmt.Errorf("unsupported provider: %s", cfg.Provider)
}

func callQwen(cfg *config.Config, prompt string, toolsList []Tool) (*LLMResponse, error) {
	// 直接使用 HTTP 请求调用千问（兼容 OpenAI 格式），避免 SDK 序列化问题
	url := "https://dashscope.aliyuncs.com/compatible-mode/v1/chat/completions"

	// 构建消息列表，确保 content 是字符串类型
	messages := []map[string]interface{}{
		{
			"role":    "user",
			"content": prompt, // 确保是字符串类型
		},
	}

	// 转换 tools
	var openaiTools []map[string]interface{}
	for _, tool := range toolsList {
		openaiTools = append(openaiTools, map[string]interface{}{
			"type": "function",
			"function": map[string]interface{}{
				"name":        tool.Function.Name,
				"description": tool.Function.Description,
				"parameters":  tool.Function.Parameters,
			},
		})
	}

	// 构建请求体
	reqBody := map[string]interface{}{
		"model":      cfg.Qwen.Model,
		"messages":   messages,
		"max_tokens": 16384, // 千问 API 最大值为 16384
	}
	if len(openaiTools) > 0 {
		reqBody["tools"] = openaiTools
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.Qwen.APIKey)

	// Log request
	headers := make(map[string]string)
	for k, v := range req.Header {
		if len(v) > 0 {
			headers[k] = v[0]
		}
	}
	utils.Log.LLMRequest("qwen", url, headers, reqBody)

	client := createHTTPClient()
	resp, err := doHTTPRequestWithRetry(client, req, 3) // Retry up to 3 times
	if err != nil {
		utils.Log.Error("llm", "Network error calling Qwen API after retries", err)
		return nil, fmt.Errorf("network error: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		utils.Log.LLMResponse("qwen", resp.StatusCode, nil, body)
		return nil, fmt.Errorf("Qwen API error: status %d, body: %s", resp.StatusCode, string(body))
	}

	// 解析响应
	var openaiResp openai.ChatCompletionResponse
	if err := json.Unmarshal(body, &openaiResp); err != nil {
		utils.Log.LLMResponse("qwen", resp.StatusCode, nil, body)
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	// Log response
	respJSON, _ := json.MarshalIndent(openaiResp, "", "  ")
	utils.Log.LLMResponse("qwen", resp.StatusCode, openaiResp, respJSON)

	// 转换为我们的格式
	llmResp := &LLMResponse{
		Choices: []Choice{},
	}

	for _, choice := range openaiResp.Choices {
		llmChoice := Choice{
			Message: Message{
				Role:    choice.Message.Role,
				Content: choice.Message.Content,
			},
			FinishReason: string(choice.FinishReason),
		}

		// 处理 tool calls
		if len(choice.Message.ToolCalls) > 0 {
			llmChoice.FinishReason = "tool_calls"
		}

		llmResp.Choices = append(llmResp.Choices, llmChoice)
	}

	// Handle tool calls in response
	for i := range llmResp.Choices {
		choice := &llmResp.Choices[i]
		if choice.FinishReason == "tool_calls" {
			// Process tool calls and make another request
			return processToolCallsQwen(cfg, prompt, toolsList, llmResp)
		}
	}

	return llmResp, nil
}

func processToolCallsQwen(cfg *config.Config, initialPrompt string, toolsList []Tool, initialResp *LLMResponse) (*LLMResponse, error) {
	// 使用 OpenAI SDK 调用千问（兼容 OpenAI 格式）
	baseURL := "https://dashscope.aliyuncs.com/compatible-mode/v1"

	// 创建 OpenAI 客户端配置，设置千问的 base URL
	config := openai.DefaultConfig(cfg.Qwen.APIKey)
	config.BaseURL = baseURL
	client := openai.NewClientWithConfig(config)

	// 构建消息列表
	// 使用零值初始化，确保 MultiContent 为 nil
	userMsg := openai.ChatCompletionMessage{}
	userMsg.Role = openai.ChatMessageRoleUser
	userMsg.Content = initialPrompt
	messages := []openai.ChatCompletionMessage{userMsg}

	// 添加 assistant 消息
	if len(initialResp.Choices) > 0 {
		choice := initialResp.Choices[0]
		// 使用零值初始化，确保 MultiContent 为 nil
		assistantMsg := openai.ChatCompletionMessage{}
		assistantMsg.Role = openai.ChatMessageRoleAssistant

		if content, ok := choice.Message.Content.(string); ok {
			assistantMsg.Content = content
		}

		// TODO: 处理 tool calls 和 tool results
		// 这里需要根据实际的 tool calls 结果来构建消息

		messages = append(messages, assistantMsg)
	}

	// 转换 tools
	var openaiTools []openai.Tool
	for _, tool := range toolsList {
		openaiTools = append(openaiTools, openai.Tool{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        tool.Function.Name,
				Description: tool.Function.Description,
				Parameters:  tool.Function.Parameters,
			},
		})
	}

	// 构建请求
	req := openai.ChatCompletionRequest{
		Model:     cfg.Qwen.Model,
		Messages:  messages,
		MaxTokens: 16384,
	}
	if len(openaiTools) > 0 {
		req.Tools = openaiTools
	}

	// Log request
	utils.Log.LLMRequest("qwen", baseURL+"/chat/completions", map[string]string{
		"Authorization": "Bearer " + cfg.Qwen.APIKey[:min(7, len(cfg.Qwen.APIKey))] + "...",
	}, req)

	// 调用 API
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	resp, err := client.CreateChatCompletion(ctx, req)
	if err != nil {
		utils.Log.Error("llm", "Qwen API call failed", err)
		return nil, fmt.Errorf("Qwen API error: %w", err)
	}

	// Log response
	respJSON, _ := json.MarshalIndent(resp, "", "  ")
	utils.Log.LLMResponse("qwen", 200, resp, respJSON)

	// 转换为我们的格式
	llmResp := &LLMResponse{
		Choices: []Choice{},
	}

	for _, choice := range resp.Choices {
		llmChoice := Choice{
			Message: Message{
				Role:    choice.Message.Role,
				Content: choice.Message.Content,
			},
			FinishReason: string(choice.FinishReason),
		}

		// 处理 tool calls
		if len(choice.Message.ToolCalls) > 0 {
			llmChoice.FinishReason = "tool_calls"
		}

		llmResp.Choices = append(llmResp.Choices, llmChoice)
	}

	// Check if more tool calls are needed
	for _, choice := range llmResp.Choices {
		if choice.FinishReason == "tool_calls" {
			return processToolCallsQwen(cfg, initialPrompt, toolsList, llmResp)
		}
	}

	return llmResp, nil
}

func callOpenAI(cfg *config.Config, prompt string, toolsList []Tool) (*LLMResponse, error) {
	url := "https://api.openai.com/v1/chat/completions"

	messages := []Message{
		{
			Role:    "user",
			Content: prompt,
		},
	}

	reqBody := LLMRequest{
		Model:     cfg.OpenAI.Model,
		Messages:  messages,
		Tools:     toolsList,
		MaxTokens: 50000,
	}

	jsonData, _ := json.Marshal(reqBody)

	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.OpenAI.APIKey)

	// Log request
	headers := make(map[string]string)
	for k, v := range req.Header {
		if len(v) > 0 {
			headers[k] = v[0]
		}
	}
	utils.Log.LLMRequest("openai", url, headers, reqBody)

	client := createHTTPClient()
	resp, err := doHTTPRequestWithRetry(client, req, 3) // Retry up to 3 times
	if err != nil {
		// Log network error with more details
		utils.Log.Error("llm", fmt.Sprintf("Network error calling Qwen API after retries: %v", err), err)
		return nil, fmt.Errorf("network error: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		// Log error response
		utils.Log.LLMResponse("openai", resp.StatusCode, nil, body)
		return nil, fmt.Errorf("OpenAI API error: %s", string(body))
	}

	var llmResp LLMResponse
	if err := json.Unmarshal(body, &llmResp); err != nil {
		// Log error response
		utils.Log.LLMResponse("openai", resp.StatusCode, nil, body)
		return nil, err
	}

	// Log successful response
	utils.Log.LLMResponse("openai", resp.StatusCode, &llmResp, body)

	// Handle tool calls in response
	for i := range llmResp.Choices {
		choice := &llmResp.Choices[i]
		if choice.FinishReason == "tool_calls" {
			// Process tool calls and make another request
			return processToolCallsOpenAI(cfg, prompt, toolsList, &llmResp)
		}
	}

	return &llmResp, nil
}

func processToolCallsOpenAI(cfg *config.Config, initialPrompt string, toolsList []Tool, initialResp *LLMResponse) (*LLMResponse, error) {
	messages := []Message{
		{
			Role:    "user",
			Content: initialPrompt,
		},
	}

	// Add assistant message with tool calls
	if len(initialResp.Choices) > 0 {
		messages = append(messages, Message{
			Role:    "assistant",
			Content: initialResp.Choices[0].Message.Content,
		})
	}

	// Execute tool calls and add results
	// Note: OpenAI response structure needs proper parsing
	// This is a simplified version - full implementation would parse tool_calls properly
	for _, choice := range initialResp.Choices {
		// Try to get tool_calls from message
		// In real implementation, we'd need to properly unmarshal the response
		// For now, we'll make a simplified version
		_ = choice // Placeholder
	}

	// Make another request with tool results
	url := "https://api.openai.com/v1/chat/completions"
	reqBody := LLMRequest{
		Model:     cfg.OpenAI.Model,
		Messages:  messages,
		Tools:     toolsList,
		MaxTokens: 50000,
	}

	jsonData, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.OpenAI.APIKey)

	// Log request
	headers := make(map[string]string)
	for k, v := range req.Header {
		if len(v) > 0 {
			headers[k] = v[0]
		}
	}
	utils.Log.LLMRequest("openai", url, headers, reqBody)

	client := createHTTPClient()
	resp, err := doHTTPRequestWithRetry(client, req, 3) // Retry up to 3 times
	if err != nil {
		// Log network error with more details
		utils.Log.Error("llm", fmt.Sprintf("Network error calling Qwen API after retries: %v", err), err)
		return nil, fmt.Errorf("network error: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		// Log error response
		utils.Log.LLMResponse("openai", resp.StatusCode, nil, body)
		return nil, fmt.Errorf("OpenAI API error: %s", string(body))
	}

	var llmResp LLMResponse
	if err := json.Unmarshal(body, &llmResp); err != nil {
		// Log error response
		utils.Log.LLMResponse("openai", resp.StatusCode, nil, body)
		return nil, err
	}

	// Log successful response
	utils.Log.LLMResponse("openai", resp.StatusCode, &llmResp, body)

	// Check if more tool calls are needed
	for _, choice := range llmResp.Choices {
		if choice.FinishReason == "tool_calls" {
			return processToolCallsOpenAI(cfg, initialPrompt, toolsList, &llmResp)
		}
	}

	return &llmResp, nil
}

func callAnthropic(cfg *config.Config, prompt string, toolsList []Tool) (*LLMResponse, error) {
	url := "https://api.anthropic.com/v1/messages"

	// Convert tools to Anthropic format
	anthropicTools := make([]map[string]interface{}, len(toolsList))
	for i, tool := range toolsList {
		anthropicTools[i] = map[string]interface{}{
			"name":         tool.Function.Name,
			"description":  tool.Function.Description,
			"input_schema": tool.Function.Parameters,
		}
	}

	reqBody := map[string]interface{}{
		"model":      cfg.Anthropic.Model,
		"max_tokens": 50000,
		"messages": []map[string]interface{}{
			{
				"role":    "user",
				"content": prompt,
			},
		},
		"tools": anthropicTools,
	}

	jsonData, _ := json.Marshal(reqBody)

	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", cfg.Anthropic.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	// Log request
	headers := make(map[string]string)
	for k, v := range req.Header {
		if len(v) > 0 {
			headers[k] = v[0]
		}
	}
	utils.Log.LLMRequest("anthropic", url, headers, reqBody)

	client := createHTTPClient()
	resp, err := doHTTPRequestWithRetry(client, req, 3) // Retry up to 3 times
	if err != nil {
		// Log network error with more details
		utils.Log.Error("llm", fmt.Sprintf("Network error calling Qwen API after retries: %v", err), err)
		return nil, fmt.Errorf("network error: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		// Log error response
		utils.Log.LLMResponse("anthropic", resp.StatusCode, nil, body)
		return nil, fmt.Errorf("Anthropic API error: %s", string(body))
	}

	// Parse Anthropic response format
	var anthropicResp map[string]interface{}
	if err := json.Unmarshal(body, &anthropicResp); err != nil {
		// Log error response
		utils.Log.LLMResponse("anthropic", resp.StatusCode, nil, body)
		return nil, err
	}

	// Log successful response
	utils.Log.LLMResponse("anthropic", resp.StatusCode, anthropicResp, body)

	// Convert to our format
	llmResp := &LLMResponse{
		Choices: []Choice{},
	}

	if content, ok := anthropicResp["content"].([]interface{}); ok {
		// Check for tool use
		hasToolUse := false
		for _, item := range content {
			if itemMap, ok := item.(map[string]interface{}); ok {
				if itemType, ok := itemMap["type"].(string); ok && itemType == "tool_use" {
					hasToolUse = true
					break
				}
			}
		}

		if hasToolUse {
			// Process tool calls
			return processToolCallsAnthropic(cfg, prompt, toolsList, anthropicResp)
		}

		// Regular text response
		var textContent strings.Builder
		for _, item := range content {
			if itemMap, ok := item.(map[string]interface{}); ok {
				if itemType, ok := itemMap["type"].(string); ok && itemType == "text" {
					if text, ok := itemMap["text"].(string); ok {
						textContent.WriteString(text)
					}
				}
			}
		}

		llmResp.Choices = append(llmResp.Choices, Choice{
			Message: Message{
				Role:    "assistant",
				Content: textContent.String(),
			},
		})
	}

	return llmResp, nil
}

func processToolCallsAnthropic(cfg *config.Config, initialPrompt string, toolsList []Tool, initialResp map[string]interface{}) (*LLMResponse, error) {
	messages := []map[string]interface{}{
		{
			"role":    "user",
			"content": initialPrompt,
		},
	}

	// Add assistant message with tool use
	if content, ok := initialResp["content"].([]interface{}); ok {
		messages = append(messages, map[string]interface{}{
			"role":    "assistant",
			"content": content,
		})
	}

	// Execute tool calls and build tool results
	toolResults := []map[string]interface{}{}
	var webResponse *tools.WebResponse

	if content, ok := initialResp["content"].([]interface{}); ok {
		for _, item := range content {
			if itemMap, ok := item.(map[string]interface{}); ok {
				if itemType, ok := itemMap["type"].(string); ok && itemType == "tool_use" {
					toolID, _ := itemMap["id"].(string)
					result := executeToolCall(itemMap)

					// Check if this is a webResponse
					if wr, ok := result.(*tools.WebResponse); ok {
						webResponse = wr
					}

					resultJSON, _ := json.Marshal(result)
					toolResults = append(toolResults, map[string]interface{}{
						"type":        "tool_result",
						"tool_use_id": toolID,
						"content":     string(resultJSON),
					})
				}
			}
		}
	}

	// If we got a webResponse, return it immediately
	if webResponse != nil {
		return &LLMResponse{
			Choices: []Choice{
				{
					Message: Message{
						Role:    "assistant",
						Content: webResponse,
					},
				},
			},
		}, nil
	}

	// Add tool results as user message
	if len(toolResults) > 0 {
		messages = append(messages, map[string]interface{}{
			"role":    "user",
			"content": toolResults,
		})
	}

	// Convert tools to Anthropic format
	anthropicTools := make([]map[string]interface{}, len(toolsList))
	for i, tool := range toolsList {
		anthropicTools[i] = map[string]interface{}{
			"name":         tool.Function.Name,
			"description":  tool.Function.Description,
			"input_schema": tool.Function.Parameters,
		}
	}

	url := "https://api.anthropic.com/v1/messages"
	reqBody := map[string]interface{}{
		"model":      cfg.Anthropic.Model,
		"max_tokens": 50000,
		"messages":   messages,
		"tools":      anthropicTools,
	}

	jsonData, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", cfg.Anthropic.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	// Log request
	headers := make(map[string]string)
	for k, v := range req.Header {
		if len(v) > 0 {
			headers[k] = v[0]
		}
	}
	utils.Log.LLMRequest("anthropic", url, headers, reqBody)

	client := createHTTPClient()
	resp, err := doHTTPRequestWithRetry(client, req, 3) // Retry up to 3 times
	if err != nil {
		// Log network error with more details
		utils.Log.Error("llm", fmt.Sprintf("Network error calling Qwen API after retries: %v", err), err)
		return nil, fmt.Errorf("network error: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		// Log error response
		utils.Log.LLMResponse("anthropic", resp.StatusCode, nil, body)
		return nil, fmt.Errorf("Anthropic API error: %s", string(body))
	}

	var anthropicResp map[string]interface{}
	if err = json.Unmarshal(body, &anthropicResp); err != nil {
		// Log error response
		utils.Log.LLMResponse("anthropic", resp.StatusCode, nil, body)
		return nil, err
	}

	// Log successful response
	utils.Log.LLMResponse("anthropic", resp.StatusCode, anthropicResp, body)

	// Check for more tool calls
	if content, ok := anthropicResp["content"].([]interface{}); ok {
		for _, item := range content {
			if itemMap, ok := item.(map[string]interface{}); ok {
				if itemType, ok := itemMap["type"].(string); ok && itemType == "tool_use" {
					// Recursive call for more tool uses
					return processToolCallsAnthropic(cfg, initialPrompt, toolsList, anthropicResp)
				}
			}
		}
	}

	// Convert to our format
	llmResp := &LLMResponse{
		Choices: []Choice{},
	}

	if content, ok := anthropicResp["content"].([]interface{}); ok {
		var textContent strings.Builder
		for _, item := range content {
			if itemMap, ok := item.(map[string]interface{}); ok {
				if itemType, ok := itemMap["type"].(string); ok && itemType == "text" {
					if text, ok := itemMap["text"].(string); ok {
						textContent.WriteString(text)
					}
				}
			}
		}

		llmResp.Choices = append(llmResp.Choices, Choice{
			Message: Message{
				Role:    "assistant",
				Content: textContent.String(),
			},
		})
	}

	return llmResp, nil
}
