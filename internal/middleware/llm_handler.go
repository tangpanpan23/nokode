package middleware

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/nokode/nokode/internal/config"
	"github.com/nokode/nokode/internal/tools"
	"github.com/nokode/nokode/internal/utils"
)

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
	Type        string                 `json:"type"`
	Function    ToolFunction           `json:"function"`
}

type ToolFunction struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

type LLMResponse struct {
	ID      string    `json:"id"`
	Choices []Choice  `json:"choices"`
	Usage   Usage     `json:"usage"`
}

type Choice struct {
	Index        int         `json:"index"`
	Message      Message     `json:"message"`
	FinishReason string      `json:"finish_reason"`
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

func HandleLLMRequest(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		requestStartTime := time.Now()
		requestID := uuid.New().String()[:9]

		utils.Log.Request(c.Request.Method, c.Request.URL.Path, map[string]interface{}{
			"requestId": requestID,
			"query":     c.Request.URL.Query(),
			"ip":        c.ClientIP(),
		})

		// Prepare request context
		var bodyBytes []byte
		if c.Request.Body != nil {
			bodyBytes, _ = io.ReadAll(c.Request.Body)
			c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
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
		queryJSON, _ := json.Marshal(c.Request.URL.Query())
		headersJSON, _ := json.Marshal(c.Request.Header)
		bodyJSON, _ := json.Marshal(body)

		vars := map[string]string{
			"METHOD":    c.Request.Method,
			"PATH":      c.Request.URL.Path,
			"URL":       c.Request.URL.String(),
			"QUERY":     string(queryJSON),
			"HEADERS":   string(headersJSON),
			"BODY":      string(bodyJSON),
			"IP":        c.ClientIP(),
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
			c.HTML(http.StatusInternalServerError, "", fmt.Sprintf(`
				<html>
					<body>
						<h1>Server Error</h1>
						<p>An error occurred while processing your request.</p>
						<p><strong>Request ID:</strong> %s</p>
						<pre>%s</pre>
					</body>
				</html>
			`, requestID, err.Error()))
			return
		}

		utils.Log.Info("llm", fmt.Sprintf("LLM call completed in %dms", llmDuration), map[string]interface{}{
			"requestId": requestID,
			"duration":  llmDuration,
		})

		// Extract webResponse from the final response
		// The processToolCalls functions handle tool execution and return final response
		var webResponse *tools.WebResponse
		
		// Check if we need to process tool calls first (OpenAI only)
		// Anthropic tool calls are handled in callAnthropic
		if cfg.Provider == "openai" {
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
		webResponse = extractWebResponse(response)
						for _, tc := range toolCalls {
							if tcMap, ok := tc.(map[string]interface{}); ok {
								result := executeToolCall(tcMap)
								if result != nil {
									if wr, ok := result.(*tools.WebResponse); ok {
										webResponse = wr
									}
								}
							}
						}
					}
				}
			}
		}

		// Send response
		totalDuration := time.Since(requestStartTime).Milliseconds()
		if webResponse != nil {
			// Set status code
			c.Status(webResponse.StatusCode)

			// Set headers
			for key, value := range webResponse.Headers {
				c.Header(key, value)
			}

			// Send body
			c.String(webResponse.StatusCode, webResponse.Body)
			utils.Log.Success("response", fmt.Sprintf("Sent webResponse (%d) in %dms", webResponse.StatusCode, totalDuration), nil)
		} else {
			// Fallback: return text response
			c.String(http.StatusOK, "No response generated")
			utils.Log.Warn("response", "No webResponse found, returning default", nil)
		}
	}
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

func callLLM(cfg *config.Config, prompt string, toolsList []Tool) (*LLMResponse, error) {
	if cfg.Provider == "openai" {
		return callOpenAI(cfg, prompt, toolsList)
	} else if cfg.Provider == "anthropic" {
		return callAnthropic(cfg, prompt, toolsList)
	}
	return nil, fmt.Errorf("unsupported provider: %s", cfg.Provider)
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

	client := &http.Client{Timeout: 300 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OpenAI API error: %s", string(body))
	}

	var llmResp LLMResponse
	if err := json.Unmarshal(body, &llmResp); err != nil {
		return nil, err
	}

	// Handle tool calls in response
	for i := range llmResp.Choices {
		choice := &llmResp.Choices[i]
		if choice.FinishReason == "tool_calls" {
			// Process tool calls and make another request
			return processToolCallsOpenAI(cfg, messages, toolsList, &llmResp)
		}
	}

	return &llmResp, nil
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
	if cfg.Provider == "openai" {
		return processToolCallsOpenAI(cfg, initialPrompt, toolsList, initialResp)
	} else {
		// Anthropic tool calls are handled in callAnthropic function
		// This should not be called for Anthropic as tool calls are handled there
		return initialResp, nil
	}
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

	client := &http.Client{Timeout: 300 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OpenAI API error: %s", string(body))
	}

	var llmResp LLMResponse
	if err := json.Unmarshal(body, &llmResp); err != nil {
		return nil, err
	}

	// Check if more tool calls are needed
	for _, choice := range llmResp.Choices {
		if choice.FinishReason == "tool_calls" {
			return processToolCallsOpenAI(cfg, initialPrompt, toolsList, &llmResp)
		}
	}

	return &llmResp, nil
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
						"type":       "tool_result",
						"tool_use_id": toolID,
						"content":    string(resultJSON),
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
			"name":        tool.Function.Name,
			"description": tool.Function.Description,
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

	client := &http.Client{Timeout: 300 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Anthropic API error: %s", string(body))
	}

	var anthropicResp map[string]interface{}
	if err := json.Unmarshal(body, &anthropicResp); err != nil {
		return nil, err
	}

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

func callAnthropic(cfg *config.Config, prompt string, toolsList []Tool) (*LLMResponse, error) {
	url := "https://api.anthropic.com/v1/messages"

	// Convert tools to Anthropic format
	anthropicTools := make([]map[string]interface{}, len(toolsList))
	for i, tool := range toolsList {
		anthropicTools[i] = map[string]interface{}{
			"name":        tool.Function.Name,
			"description": tool.Function.Description,
			"input_schema": tool.Function.Parameters,
		}
	}

	reqBody := map[string]interface{}{
		"model":       cfg.Anthropic.Model,
		"max_tokens":  50000,
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

	client := &http.Client{Timeout: 300 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Anthropic API error: %s", string(body))
	}

	// Parse Anthropic response format
	var anthropicResp map[string]interface{}
	if err := json.Unmarshal(body, &anthropicResp); err != nil {
		return nil, err
	}

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

func processToolCallsAnthropic(cfg *config.Config, prompt string, toolsList []Tool, initialResp map[string]interface{}) (*LLMResponse, error) {
	url := "https://api.anthropic.com/v1/messages"

	messages := []map[string]interface{}{
		{
			"role":    "user",
			"content": prompt,
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
	if content, ok := initialResp["content"].([]interface{}); ok {
		for _, item := range content {
			if itemMap, ok := item.(map[string]interface{}); ok {
				if itemType, ok := itemMap["type"].(string); ok && itemType == "tool_use" {
					toolID, _ := itemMap["id"].(string)
					result := executeToolCall(itemMap)

					resultJSON, _ := json.Marshal(result)
					toolResults = append(toolResults, map[string]interface{}{
						"type":       "tool_result",
						"tool_use_id": toolID,
						"content":    string(resultJSON),
					})
				}
			}
		}
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
			"name":        tool.Function.Name,
			"description": tool.Function.Description,
			"input_schema": tool.Function.Parameters,
		}
	}

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

	client := &http.Client{Timeout: 300 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Anthropic API error: %s", string(body))
	}

	var anthropicResp map[string]interface{}
	if err := json.Unmarshal(body, &anthropicResp); err != nil {
		return nil, err
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
				} else if itemType == "tool_use" {
					// Another tool use - would need recursive handling
					// For now, execute it
					result := executeToolCall(itemMap)
					if wr, ok := result.(*tools.WebResponse); ok {
						// Return immediately with web response
						return &LLMResponse{
							Choices: []Choice{
								{
									Message: Message{
										Role:    "assistant",
										Content: wr,
									},
								},
							},
						}, nil
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

