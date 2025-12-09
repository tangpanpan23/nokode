package handler

import (
	"bufio"
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/nokode/nokode/internal/config"
	"github.com/nokode/nokode/internal/tools"
	"github.com/nokode/nokode/internal/utils"
	openai "github.com/sashabaranov/go-openai"
)

// Global rate limiting
var (
	lastAPICallTime time.Time
	apiCallMutex    sync.Mutex
	minInterval     = 3 * time.Second // Minimum 3 seconds between API calls (configurable)
)

// init initializes rate limiting settings
func init() {
	// Allow configuration via environment variable
	if interval := os.Getenv("API_RATE_LIMIT_INTERVAL"); interval != "" {
		if parsed, err := time.ParseDuration(interval); err == nil {
			minInterval = parsed
		}
	}
}

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
		ResponseHeaderTimeout: 300 * time.Second, // 5 minutes for AI model responses
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

// doAPIRequestWithRetry performs HTTP request with retry logic for both network errors and HTTP status codes
func doAPIRequestWithRetry(client *http.Client, req *http.Request, maxRetries int) (*http.Response, error) {
	var lastResp *http.Response
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff with longer delay for rate limits
			var backoff time.Duration
			if lastResp != nil && lastResp.StatusCode == 429 {
				// For rate limits, use exponential backoff: 30s, 90s, 270s (4.5min), 810s (13.5min)
				backoff = time.Duration(30*attempt) * time.Second
				utils.Log.Warn("llm", fmt.Sprintf("Rate limit hit, retrying (attempt %d/%d) after %v", attempt, maxRetries, backoff), nil)
			} else {
				// For other errors, use shorter backoff: 1s, 2s, 4s
				backoff = time.Duration(1<<uint(attempt-1)) * time.Second
				utils.Log.Warn("llm", fmt.Sprintf("Retrying request (attempt %d/%d) after %v", attempt, maxRetries, backoff), nil)
			}
			time.Sleep(backoff)
		}

		// Clone the request for retry
		reqClone := req.Clone(req.Context())

		// Fix: Re-set the request body for retry, as Clone() doesn't handle Body correctly
		if req.Body != nil && req.GetBody != nil {
			// For requests with GetBody (usually from strings.NewReader), restore the body
			body, err := req.GetBody()
			if err != nil {
				return nil, fmt.Errorf("failed to restore request body: %w", err)
			}
			reqClone.Body = body
		} else if req.Body != nil {
			// For other cases, try to clone the body if it's seekable
			if seeker, ok := req.Body.(io.Seeker); ok {
				seeker.Seek(0, io.SeekStart) // Reset to beginning
				reqClone.Body = req.Body
			}
		}

		resp, err := client.Do(reqClone)
		if err != nil {
			lastErr = err
			// Check if it's a network error that might be retryable
			if isRetryableError(err) {
				utils.Log.Warn("llm", fmt.Sprintf("Network error (attempt %d/%d): %v", attempt+1, maxRetries+1, err), nil)
				continue
			}
			// Non-retryable network error
			return nil, err
		}

		// Check if HTTP status code indicates retryable error
		if resp.StatusCode == 429 {
			lastResp = resp
			lastErr = fmt.Errorf("rate limit exceeded: status %d", resp.StatusCode)
			utils.Log.Warn("llm", fmt.Sprintf("Rate limit exceeded (attempt %d/%d): status %d", attempt+1, maxRetries+1, resp.StatusCode), nil)
			continue
		}

		// Check for other retryable status codes
		if resp.StatusCode >= 500 {
			lastResp = resp
			lastErr = fmt.Errorf("server error: status %d", resp.StatusCode)
			utils.Log.Warn("llm", fmt.Sprintf("Server error (attempt %d/%d): status %d", attempt+1, maxRetries+1, resp.StatusCode), nil)
			continue
		}

		// Success or non-retryable client error
		return resp, nil
	}

	if lastResp != nil {
		return lastResp, fmt.Errorf("request failed after %d attempts: %w", maxRetries+1, lastErr)
	}
	return nil, fmt.Errorf("request failed after %d attempts: %w", maxRetries+1, lastErr)
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
			// Fallback: try to get a random poem from database
			fallbackHTML := generateFallbackPoemPage(cfg)
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(fallbackHTML))
			utils.Log.Warn("response", "No webResponse found, showing fallback poem page", nil)
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
				Description: "Execute SQL queries on the MySQL database. You can create tables, insert data, query, update, delete - any SQL operation.",
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
				// First try to parse as JSON to find webResponse (for OpenAI/Qwen style)
				var result map[string]interface{}
				if err := json.Unmarshal([]byte(contentStr), &result); err == nil {
					if statusCode, ok := result["statusCode"].(float64); ok {
						body, _ := result["body"].(string)
						contentType, _ := result["contentType"].(string)
						wr := tools.CreateWebResponse(int(statusCode), contentType, body)
						return &wr
					}
				}

				// If not JSON, check if it's HTML content (for Spark direct response)
				if strings.Contains(contentStr, "<html") || strings.Contains(contentStr, "<!DOCTYPE html") {
					// This is direct HTML content from Spark
					wr := tools.CreateWebResponse(200, "text/html", contentStr)
					return &wr
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
	} else if cfg.Provider == "baidu" {
		return processToolCallsBaidu(cfg, initialPrompt, toolsList, initialResp)
	} else if cfg.Provider == "spark" {
		return processToolCallsSpark(cfg, initialPrompt, toolsList, initialResp)
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
	} else if cfg.Provider == "baidu" {
		return callBaidu(cfg, prompt, toolsList)
	} else if cfg.Provider == "spark" {
		return callSpark(cfg, prompt, toolsList)
	}
	return nil, fmt.Errorf("unsupported provider: %s", cfg.Provider)
}

// Spark API structures for WebSocket API (X1.5)
type SparkWSRequest struct {
	Header struct {
		AppID string `json:"app_id"`
		UID   string `json:"uid,omitempty"`
	} `json:"header"`
	Parameter struct {
		Chat struct {
			Domain           string      `json:"domain"`
			Temperature      float64     `json:"temperature,omitempty"`
			MaxTokens        int         `json:"max_tokens,omitempty"`
			TopK             int         `json:"top_k,omitempty"`
			TopP             float64     `json:"top_p,omitempty"`
			PresencePenalty  float64     `json:"presence_penalty,omitempty"`
			FrequencyPenalty float64     `json:"frequency_penalty,omitempty"`
			Thinking         interface{} `json:"thinking,omitempty"` // 动态调整思考模式对象
			Tools            []SparkTool `json:"tools,omitempty"`
		} `json:"chat"`
	} `json:"parameter"`
	Payload struct {
		Message struct {
			Text []SparkWSText `json:"text"`
		} `json:"message"`
	} `json:"payload"`
}

type SparkWSText struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type SparkTool struct {
	Type       string                `json:"type"`
	Function   *SparkFunction        `json:"function,omitempty"`
	WebSearch  *SparkWebSearch       `json:"web_search,omitempty"`
}

type SparkFunction struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Parameters  interface{} `json:"parameters"`
}

type SparkWebSearch struct {
	Enable      bool   `json:"enable"`
	SearchMode  string `json:"search_mode,omitempty"`
}

type SparkWSResponse struct {
	Header struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		SID     string `json:"sid"`
		Status  int    `json:"status"`
	} `json:"header"`
	Payload struct {
		Choices struct {
			Status int `json:"status"`
			Seq    int `json:"seq"`
			Text   []struct {
				Content          string `json:"content"`
				ReasoningContent string `json:"reasoning_content,omitempty"` // 推理内容
				Role             string `json:"role"`
				Index            int    `json:"index"`
			} `json:"text"`
		} `json:"choices"`
		Usage struct {
			Text struct {
				QuestionTokens   int `json:"question_tokens"`
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
				TotalTokens      int `json:"total_tokens"`
			} `json:"text"`
		} `json:"usage"`
	} `json:"payload"`
}

// generateSparkToken generates the Bearer token for Spark API
// According to official documentation, use APIpassword directly
func generateSparkToken(apiKey, apiSecret string) string {
	// The APIpassword format is typically "AK:SK" where AK is apiKey and SK is apiSecret
	return apiKey + ":" + apiSecret
}

// generateSparkAuthURL generates the authenticated WebSocket URL for Spark X1.5 API
// Based on Python demo implementation
func generateSparkAuthURL(appID, apiKey, apiSecret string) (string, error) {
	host := "spark-api.xf-yun.com"
	path := "/v1/x1"
	gptURL := "wss://" + host + path

	// Current timestamp in RFC1123 format (same as Python demo)
	now := time.Now()
	date := now.UTC().Format(time.RFC1123)

	// Create the string to sign (exactly like Python demo)
	signatureOrigin := fmt.Sprintf("host: %s\ndate: %s\nGET %s HTTP/1.1", host, date, path)

	// Create HMAC-SHA256 signature (apiSecret as UTF-8 bytes)
	h := hmac.New(sha256.New, []byte(apiSecret))
	h.Write([]byte(signatureOrigin))
	signatureSha := h.Sum(nil)
	signatureShaBase64 := base64.StdEncoding.EncodeToString(signatureSha)

	// Create authorization origin string
	authorizationOrigin := fmt.Sprintf(`api_key="%s", algorithm="hmac-sha256", headers="host date request-line", signature="%s"`,
		apiKey, signatureShaBase64)

	// Base64 encode the entire authorization string (key difference from current implementation)
	authorization := base64.StdEncoding.EncodeToString([]byte(authorizationOrigin))

	// Create parameter map
	v := map[string]string{
		"authorization": authorization,
		"date":          date,
		"host":          host,
	}

	// URL encode parameters and construct final URL
	params := url.Values{}
	for key, value := range v {
		params.Add(key, value)
	}

	return gptURL + "?" + params.Encode(), nil
}


func callSpark(cfg *config.Config, prompt string, toolsList []Tool) (*LLMResponse, error) {
	// Rate limiting: ensure minimum interval between API calls
	apiCallMutex.Lock()
	timeSinceLastCall := time.Since(lastAPICallTime)
	if timeSinceLastCall < minInterval {
		sleepTime := minInterval - timeSinceLastCall
		utils.Log.Info("llm", fmt.Sprintf("Rate limiting: waiting %v before API call", sleepTime), nil)
		time.Sleep(sleepTime)
	}
	lastAPICallTime = time.Now()
	apiCallMutex.Unlock()

	// Use official HTTP API endpoint
	url := "https://spark-api-open.xf-yun.com/v2/chat/completions"

	// Generate Bearer token (APIpassword format: AK:SK)
	token := generateSparkToken(cfg.Spark.APIKey, cfg.Spark.APISecret)

	// Prepare tools for FunctionCall
	var sparkTools []map[string]interface{}
	if len(toolsList) > 0 {
		for _, tool := range toolsList {
			sparkTools = append(sparkTools, map[string]interface{}{
				"type": "function",
				"function": map[string]interface{}{
					"name":        tool.Function.Name,
					"description": tool.Function.Description,
					"parameters":  tool.Function.Parameters,
				},
			})
		}
	}

	// Prepare request body according to official documentation
	requestBody := map[string]interface{}{
		"model": cfg.Spark.Model, // spark-x for X1.5
		"user":  "nokode-user",
		"messages": []map[string]interface{}{
			{
				"role":    "system",
				"content": "You are a contact manager. You can use database and webResponse tools to handle requests. Always use tools appropriately.",
			},
			{
				"role":    "user",
				"content": prompt,
			},
		},
		"temperature": 0.7,
		"max_tokens":  65535,
		"thinking": map[string]interface{}{
			"type": "disabled", // disabled/auto/enabled
		},
		"stream": true,
	}

	// Add tools if available
	if len(sparkTools) > 0 {
		requestBody["tools"] = sparkTools
		requestBody["tool_choice"] = "auto"
	}

	// Convert to JSON
	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers according to documentation
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	// Log request
	utils.Log.LLMRequest("spark", url, map[string]string{
		"authorization": "Bearer [REDACTED]",
		"content-type":  "application/json",
	}, jsonData)

	// Send request with retry logic
	client := createHTTPClient()
	resp, err := doAPIRequestWithRetry(client, req, 5)
	if err != nil {
		utils.Log.Error("llm", "Network or rate limit error calling Spark API after retries", err)
		return nil, fmt.Errorf("network or rate limit error: %w", err)
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Spark API error: status %d, body: %s", resp.StatusCode, string(body))
	}

	// Handle streaming response for faster response times
	var fullContent strings.Builder
	var toolCallsFound bool
	reader := bufio.NewReader(resp.Body)

	// Set up timeout for streaming
	timeout := time.AfterFunc(60*time.Second, func() {
		// This will be cancelled when we return successfully
	})

	defer timeout.Stop()

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("failed to read stream: %w", err)
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Handle SSE format
		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				break
			}

			var chunk map[string]interface{}
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				continue // Skip invalid chunks
			}

			// Log first chunk for debugging
			if fullContent.Len() == 0 {
				utils.Log.LLMResponse("spark", resp.StatusCode, nil, []byte(data))
			}

			// Process chunk
			if choices, ok := chunk["choices"].([]interface{}); ok && len(choices) > 0 {
				if choice, ok := choices[0].(map[string]interface{}); ok {
					if delta, ok := choice["delta"].(map[string]interface{}); ok {
						// Check for tool calls first (higher priority)
						if toolCalls, ok := delta["tool_calls"].([]interface{}); ok && len(toolCalls) > 0 {
							toolCallsFound = true
							for _, tc := range toolCalls {
								if toolCall, ok := tc.(map[string]interface{}); ok {
									if function, ok := toolCall["function"].(map[string]interface{}); ok {
										if funcName, ok := function["name"].(string); ok && funcName == "webResponse" {
											if argsStr, ok := function["arguments"].(string); ok {
												var args map[string]interface{}
												if json.Unmarshal([]byte(argsStr), &args) == nil {
													// Extract web response parameters
													statusCode := 200
													if sc, ok := args["statusCode"].(float64); ok {
														statusCode = int(sc)
													}

													contentType := "text/html"
													if ct, ok := args["contentType"].(string); ok {
														contentType = ct
													}

													body := ""
													if b, ok := args["body"].(string); ok {
														body = b
													}

													// Return immediately when webResponse tool is called
													webResp := tools.CreateWebResponse(statusCode, contentType, body)

													llmResp := &LLMResponse{
														ID: uuid.New().String(),
														Choices: []Choice{
															{
																Index: 0,
																Message: Message{
																	Role:    "assistant",
																	Content: webResp.Body,
																},
																FinishReason: "tool_calls",
															},
														},
														Usage: Usage{PromptTokens: 0, CompletionTokens: 0, TotalTokens: 0},
													}

													utils.Log.Success("llm", "Spark streaming webResponse executed - immediate response", nil)
													return llmResp, nil
												}
											}
										}
									}
								}
							}
						}

						// Accumulate content if no tool calls
						if reasoning, ok := delta["reasoning_content"].(string); ok && reasoning != "" {
							fullContent.WriteString(reasoning)
							fullContent.WriteString(" ")
						}
						if content, ok := delta["content"].(string); ok && content != "" {
							fullContent.WriteString(content)
						}
					}
				}
			}
		}
	}

	// If no tool calls found but we have content, return it
	if !toolCallsFound && fullContent.Len() > 0 {
		content := fullContent.String()
		llmResp := &LLMResponse{
			ID: uuid.New().String(),
			Choices: []Choice{
				{
					Index: 0,
					Message: Message{
						Role:    "assistant",
						Content: content,
					},
					FinishReason: "stop",
				},
			},
			Usage: Usage{PromptTokens: 0, CompletionTokens: 0, TotalTokens: 0},
		}
		utils.Log.Success("llm", "Spark streaming completed with content", nil)
		return llmResp, nil
	}

	// Fallback response
	fallbackContent := `<html><body><h1>Contact Manager</h1><p>AI response processing completed but no valid content generated.</p><a href="/">Home</a></body></html>`
	llmResp := &LLMResponse{
		ID: uuid.New().String(),
		Choices: []Choice{
			{
				Index: 0,
				Message: Message{
					Role:    "assistant",
					Content: fallbackContent,
				},
				FinishReason: "stop",
			},
		},
		Usage: Usage{PromptTokens: 0, CompletionTokens: 0, TotalTokens: 0},
	}

	utils.Log.Success("llm", "Spark streaming completed with fallback", nil)
	return llmResp, nil
}

// parseToolCalls parses tool calls from Spark API response
func parseToolCalls(toolCalls []interface{}) []ToolCall {
	var result []ToolCall
	for _, tc := range toolCalls {
		if toolCall, ok := tc.(map[string]interface{}); ok {
			var call ToolCall
			call.ID, _ = toolCall["id"].(string)
			call.Type, _ = toolCall["type"].(string)

			if function, ok := toolCall["function"].(map[string]interface{}); ok {
				call.Name, _ = function["name"].(string)
				if args, ok := function["arguments"].(string); ok {
					// Parse arguments as JSON
					var argsMap map[string]interface{}
					if json.Unmarshal([]byte(args), &argsMap) == nil {
						call.Arguments = argsMap
					}
				}
			}

			result = append(result, call)
		}
	}
	return result
}

// parseUsage parses token usage from Spark API response
func parseUsage(response map[string]interface{}) Usage {
	var usage Usage
	if usageData, ok := response["usage"].(map[string]interface{}); ok {
		if prompt, ok := usageData["prompt_tokens"].(float64); ok {
			usage.PromptTokens = int(prompt)
		}
		if completion, ok := usageData["completion_tokens"].(float64); ok {
			usage.CompletionTokens = int(completion)
		}
		if total, ok := usageData["total_tokens"].(float64); ok {
			usage.TotalTokens = int(total)
		}
	}
	return usage
}

// generateFallbackPoemPage generates a fallback HTML page with a random poem from database
func generateFallbackPoemPage(cfg *config.Config) string {
	// Try to query a random poem from database
	query := "SELECT title, author, dynasty, content FROM poems ORDER BY RAND() LIMIT 1"

	result := tools.ExecuteDatabaseQuery(query, []interface{}{}, "select")

	// If we got results, format them
	if result.Success && len(result.Rows) > 0 {
		row := result.Rows[0]
		title := row["title"]
		author := row["author"]
		dynasty := row["dynasty"]
		content := row["content"]

		// Convert dynasty enum to display text
		dynastyText := "唐代"
		if d, ok := dynasty.(string); ok && d == "song" {
			dynastyText = "宋代"
		}

		return fmt.Sprintf(`<html>
<head>
    <title>Chinese Poetry Generator</title>
    <meta charset="utf-8">
    <style>
        body { font-family: 'Microsoft YaHei', sans-serif; margin: 40px; background: #f5f5f5; }
        .poem { background: white; padding: 30px; border-radius: 10px; box-shadow: 0 2px 10px rgba(0,0,0,0.1); max-width: 600px; margin: 0 auto; }
        .title { font-size: 24px; color: #2c3e50; margin-bottom: 10px; }
        .author { color: #7f8c8d; font-style: italic; margin-bottom: 20px; }
        .content { font-size: 18px; line-height: 2; color: #34495e; white-space: pre-line; }
        .nav { text-align: center; margin-top: 30px; }
        .nav a { color: #3498db; text-decoration: none; margin: 0 15px; padding: 10px 20px; border: 1px solid #3498db; border-radius: 5px; }
        .nav a:hover { background: #3498db; color: white; }
        .fallback { background: #fff3cd; color: #856404; padding: 10px; border-radius: 5px; margin-bottom: 20px; }
    </style>
</head>
<body>
    <div class="poem">
        <div class="fallback">⚠️ AI 服务暂时不可用，显示历史诗歌</div>
        <div class="title">%s</div>
        <div class="author">%s · %s</div>
        <div class="content">%s</div>
    </div>
    <div class="nav">
        <a href="/">生成新诗</a>
        <a href="/poems">查看所有诗歌</a>
    </div>
</body>
</html>`, title, author, dynastyText, content)
	}

	// Fallback if no poems in database or query failed
	return `<html>
<head>
    <title>Chinese Poetry Generator</title>
    <meta charset="utf-8">
    <style>
        body { font-family: 'Microsoft YaHei', sans-serif; margin: 40px; background: #f5f5f5; }
        .poem { background: white; padding: 30px; border-radius: 10px; box-shadow: 0 2px 10px rgba(0,0,0,0.1); max-width: 600px; margin: 0 auto; }
        .title { font-size: 24px; color: #2c3e50; margin-bottom: 10px; }
        .author { color: #7f8c8d; font-style: italic; margin-bottom: 20px; }
        .content { font-size: 18px; line-height: 2; color: #34495e; }
        .nav { text-align: center; margin-top: 30px; }
        .nav a { color: #3498db; text-decoration: none; margin: 0 15px; padding: 10px 20px; border: 1px solid #3498db; border-radius: 5px; }
        .nav a:hover { background: #3498db; color: white; }
    </style>
</head>
<body>
    <div class="poem">
        <div class="title">静夜思</div>
        <div class="author">李白 · 唐代</div>
        <div class="content">
            床前明月光，<br>
            疑是地上霜。<br>
            举头望明月，<br>
            低头思故乡。
        </div>
    </div>
    <div class="nav">
        <a href="/">生成新诗</a>
        <a href="/poems">查看所有诗歌</a>
    </div>
</body>
</html>`
}

func callQwen(cfg *config.Config, prompt string, toolsList []Tool) (*LLMResponse, error) {
	// Rate limiting: ensure minimum interval between API calls
	apiCallMutex.Lock()
	timeSinceLastCall := time.Since(lastAPICallTime)
	if timeSinceLastCall < minInterval {
		sleepTime := minInterval - timeSinceLastCall
		utils.Log.Info("llm", fmt.Sprintf("Rate limiting: waiting %v before API call", sleepTime), nil)
		time.Sleep(sleepTime)
	}
	lastAPICallTime = time.Now()
	apiCallMutex.Unlock()

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
	// 直接使用 HTTP 请求调用千问（兼容 OpenAI 格式），避免 SDK 序列化问题
	url := "https://dashscope.aliyuncs.com/compatible-mode/v1/chat/completions"

	// 构建消息列表，确保 content 是字符串类型
	messages := []map[string]interface{}{
		{
			"role":    "user",
			"content": initialPrompt,
		},
	}

	// 添加 assistant 消息
	if len(initialResp.Choices) > 0 {
		choice := initialResp.Choices[0]
		assistantMsg := map[string]interface{}{
			"role": "assistant",
		}

		if content, ok := choice.Message.Content.(string); ok && content != "" {
			assistantMsg["content"] = content
		}

		// TODO: 处理 tool calls 和 tool results
		// 这里需要根据实际的 tool calls 结果来构建消息

		messages = append(messages, assistantMsg)
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
		"max_tokens": 16384,
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

	// Check if more tool calls are needed
	for _, choice := range llmResp.Choices {
		if choice.FinishReason == "tool_calls" {
			return processToolCallsQwen(cfg, initialPrompt, toolsList, llmResp)
		}
	}

	return llmResp, nil
}

func callOpenAI(cfg *config.Config, prompt string, toolsList []Tool) (*LLMResponse, error) {
	// Rate limiting: ensure minimum interval between API calls
	apiCallMutex.Lock()
	timeSinceLastCall := time.Since(lastAPICallTime)
	if timeSinceLastCall < minInterval {
		sleepTime := minInterval - timeSinceLastCall
		utils.Log.Info("llm", fmt.Sprintf("Rate limiting: waiting %v before API call", sleepTime), nil)
		time.Sleep(sleepTime)
	}
	lastAPICallTime = time.Now()
	apiCallMutex.Unlock()

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
	// Rate limiting: ensure minimum interval between API calls
	apiCallMutex.Lock()
	timeSinceLastCall := time.Since(lastAPICallTime)
	if timeSinceLastCall < minInterval {
		sleepTime := minInterval - timeSinceLastCall
		utils.Log.Info("llm", fmt.Sprintf("Rate limiting: waiting %v before API call", sleepTime), nil)
		time.Sleep(sleepTime)
	}
	lastAPICallTime = time.Now()
	apiCallMutex.Unlock()

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

// getBaiduAccessToken 获取百度API的access token
func getBaiduAccessToken(apiKey, secretKey string) (string, error) {
	url := fmt.Sprintf("https://aip.baidubce.com/oauth/2.0/token?grant_type=client_credentials&client_id=%s&client_secret=%s",
		apiKey, secretKey)

	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create token request: %w", err)
	}

	client := createHTTPClient()
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to get token: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token request failed: status %d, body: %s", resp.StatusCode, string(body))
	}

	var tokenResp map[string]interface{}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", fmt.Errorf("failed to parse token response: %w", err)
	}

	if accessToken, ok := tokenResp["access_token"].(string); ok {
		return accessToken, nil
	}

	return "", fmt.Errorf("access_token not found in response")
}

func callBaidu(cfg *config.Config, prompt string, toolsList []Tool) (*LLMResponse, error) {
	// Rate limiting: ensure minimum interval between API calls
	apiCallMutex.Lock()
	timeSinceLastCall := time.Since(lastAPICallTime)
	if timeSinceLastCall < minInterval {
		sleepTime := minInterval - timeSinceLastCall
		utils.Log.Info("llm", fmt.Sprintf("Rate limiting: waiting %v before API call", sleepTime), nil)
		time.Sleep(sleepTime)
	}
	lastAPICallTime = time.Now()
	apiCallMutex.Unlock()

	// 使用千帆平台接口
	url := "https://qianfan.baidubce.com/v2/chat/completions"

	// 检查是否有bce-v3格式的token（千帆平台）
	var authHeader string
	if cfg.Baidu.APIToken != "" {
		// 使用千帆平台的bce-v3 token认证
		authHeader = fmt.Sprintf("Bearer %s", cfg.Baidu.APIToken)
	} else if cfg.Baidu.APIKey != "" && cfg.Baidu.Secret != "" {
		// 回退到旧版oauth认证
		accessToken, err := getBaiduAccessToken(cfg.Baidu.APIKey, cfg.Baidu.Secret)
		if err != nil {
			return nil, fmt.Errorf("failed to get baidu access token: %w", err)
		}
		authHeader = fmt.Sprintf("Bearer %s", accessToken)
	} else {
		return nil, fmt.Errorf("baidu authentication not configured: need either APIToken or APIKey+Secret")
	}

	// 构建千帆平台的请求格式（类似OpenAI格式）
	reqBody := map[string]interface{}{
		"model": cfg.Baidu.Model,
		"messages": []map[string]interface{}{
			{
				"role":    "user",
				"content": prompt,
			},
		},
		"temperature": 0.7,
		"top_p":       0.8,
		"stream":      false,
	}

	// 如果有工具，添加到请求中
	if len(toolsList) > 0 {
		// 千帆平台支持tools格式（OpenAI兼容）
		tools := make([]map[string]interface{}, len(toolsList))
		for i, tool := range toolsList {
			tools[i] = map[string]interface{}{
				"type": "function",
				"function": map[string]interface{}{
					"name":        tool.Function.Name,
					"description": tool.Function.Description,
					"parameters":  tool.Function.Parameters,
				},
			}
		}
		reqBody["tools"] = tools
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Use strings.NewReader for retry-able request body
	req, err := http.NewRequest("POST", url, strings.NewReader(string(jsonData)))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	// Set GetBody for retry support
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader(string(jsonData))), nil
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)

	// 如果配置了appid，添加到header
	if cfg.Baidu.AppID != "" {
		req.Header.Set("appid", cfg.Baidu.AppID)
	}

	// Log request
	headers := make(map[string]string)
	for k, v := range req.Header {
		if len(v) > 0 {
			headers[k] = v[0]
		}
	}
	utils.Log.LLMRequest("baidu", url, headers, reqBody)

	client := createHTTPClient()
	resp, err := doAPIRequestWithRetry(client, req, 5) // Retry up to 5 times with rate limit handling
	if err != nil {
		utils.Log.Error("llm", "Network or rate limit error calling Baidu API after retries", err)
		return nil, fmt.Errorf("network or rate limit error: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		utils.Log.LLMResponse("baidu", resp.StatusCode, nil, body)
		return nil, fmt.Errorf("Baidu API error: status %d, body: %s", resp.StatusCode, string(body))
	}

	// 解析百度文心一言的响应格式
	var baiduResp map[string]interface{}
	if err := json.Unmarshal(body, &baiduResp); err != nil {
		utils.Log.LLMResponse("baidu", resp.StatusCode, nil, body)
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	// Log response
	utils.Log.LLMResponse("baidu", resp.StatusCode, baiduResp, body)

	// 转换为我们的格式
	llmResp := &LLMResponse{
		Choices: []Choice{},
	}

	if result, ok := baiduResp["result"].(string); ok {
		llmChoice := Choice{
			Message: Message{
				Role:    "assistant",
				Content: result,
			},
			FinishReason: "stop",
		}

		// 检查是否有函数调用
		if _, exists := baiduResp["function_call"].(map[string]interface{}); exists {
			llmChoice.FinishReason = "function_call"
			// 文心一言的函数调用格式可能不同，需要根据实际API文档调整
		}

		llmResp.Choices = append(llmResp.Choices, llmChoice)
	}

	// Handle tool calls in response
	for i := range llmResp.Choices {
		choice := &llmResp.Choices[i]
		if choice.FinishReason == "function_call" {
			// Process function calls and make another request
			return processToolCallsBaidu(cfg, prompt, toolsList, llmResp)
		}
	}

	return llmResp, nil
}

func processToolCallsBaidu(cfg *config.Config, initialPrompt string, toolsList []Tool, initialResp *LLMResponse) (*LLMResponse, error) {
	// 简化实现：目前百度文心一言的函数调用支持可能有限
	// 这里返回原始响应，实际实现需要根据百度API文档调整
	return initialResp, nil
}

func processToolCallsSpark(cfg *config.Config, initialPrompt string, toolsList []Tool, initialResp *LLMResponse) (*LLMResponse, error) {
	// For Spark, tool calls are handled directly in callSpark function
	// This function is kept for compatibility with the recursive processing framework
	return initialResp, nil
}
