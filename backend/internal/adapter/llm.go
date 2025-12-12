package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/sashabaranov/go-openai"
	"ezra-clone/backend/pkg/logger"
	"go.uber.org/zap"
)

// LLMAdapter handles communication with the LLM via LiteLLM
type LLMAdapter struct {
	client *openai.Client
	model  string
	mu     sync.RWMutex // Protects model field for concurrent access
	logger *zap.Logger
}

// SetModel updates the model used by this adapter
func (a *LLMAdapter) SetModel(model string) {
	if model != "" {
		a.mu.Lock()
		a.model = model
		a.mu.Unlock()
		a.logger.Debug("LLM adapter model updated", zap.String("model", model))
	}
}

// GetModel returns the current model
func (a *LLMAdapter) GetModel() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.model
}

// NewLLMAdapter creates a new LLM adapter
func NewLLMAdapter(baseURL, apiKey, modelID string) *LLMAdapter {
	// For LiteLLM, we can use a dummy API key if not provided
	if apiKey == "" {
		apiKey = "dummy-key"
	}
	
	config := openai.DefaultConfig(apiKey)
	config.BaseURL = baseURL + "/v1"

	return &LLMAdapter{
		client: openai.NewClientWithConfig(config),
		model:  modelID,
		logger: logger.Get(),
	}
}

// Tool represents a function that can be called by the LLM
type Tool struct {
	Type        string                 `json:"type"`
	Function    FunctionDefinition     `json:"function"`
}

// FunctionDefinition defines a function that can be called
type FunctionDefinition struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

// Response represents the LLM's response
type Response struct {
	Content   string
	ToolCalls []ToolCall
}

// ToolCall represents a function call from the LLM
type ToolCall struct {
	ID       string
	Name     string
	Arguments map[string]interface{}
}

// Generate sends a request to the LLM and returns the response
func (a *LLMAdapter) Generate(ctx context.Context, systemPrompt, userMsg string, tools []Tool) (*Response, error) {
	messages := []openai.ChatCompletionMessage{
		{
			Role:    openai.ChatMessageRoleSystem,
			Content: systemPrompt,
		},
		{
			Role:    openai.ChatMessageRoleUser,
			Content: userMsg,
		},
	}

	// Convert tools to OpenAI format
	openaiTools := make([]openai.Tool, 0, len(tools))
	for _, tool := range tools {
		openaiTools = append(openaiTools, openai.Tool{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        tool.Function.Name,
				Description: tool.Function.Description,
				Parameters:  tool.Function.Parameters,
			},
		})
	}

	a.mu.RLock()
	currentModel := a.model
	a.mu.RUnlock()

	req := openai.ChatCompletionRequest{
		Model:       currentModel,
		Messages:    messages,
		Tools:       openaiTools,
		// ToolChoice defaults to "auto" when tools are provided
		Temperature: 0.7,
	}

	// Retry logic with exponential backoff
	var resp openai.ChatCompletionResponse
	var err error
	maxRetries := 3
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(attempt) * time.Second
			a.logger.Warn("Retrying LLM request",
				zap.Int("attempt", attempt+1),
				zap.Duration("backoff", backoff),
			)
			time.Sleep(backoff)
		}

		resp, err = a.client.CreateChatCompletion(ctx, req)
		if err == nil {
			break
		}

		// Log detailed error information
		errMsg := err.Error()
		a.logger.Error("LLM request failed",
			zap.Error(err),
			zap.Int("attempt", attempt+1),
			zap.String("model", currentModel),
			zap.String("error_message", errMsg),
		)

		// Check if it's a JSON parsing error (likely server returned non-JSON error)
		if strings.Contains(errMsg, "invalid character") || strings.Contains(errMsg, "json") {
			a.logger.Warn("LLM service returned non-JSON error response - this may be a transient server issue",
				zap.String("error", errMsg),
			)
		}
	}

	if err != nil {
		return nil, fmt.Errorf("failed to generate response after %d attempts: %w", maxRetries, err)
	}

	// Parse response
	response := &Response{
		Content:   "",
		ToolCalls: []ToolCall{},
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("no choices in LLM response")
	}

	choice := resp.Choices[0]
	
	// Extract content
	if choice.Message.Content != "" {
		response.Content = choice.Message.Content
	}

	// Extract tool calls
	if len(choice.Message.ToolCalls) > 0 {
		for _, tc := range choice.Message.ToolCalls {
			toolCall := ToolCall{
				ID:   tc.ID,
				Name: tc.Function.Name,
			}

			// Parse arguments JSON
			args, err := parseJSONArguments(tc.Function.Arguments)
			if err != nil {
				a.logger.Warn("Failed to parse tool call arguments",
					zap.String("tool_id", tc.ID),
					zap.Error(err),
				)
				args = make(map[string]interface{})
			}
			toolCall.Arguments = args

			response.ToolCalls = append(response.ToolCalls, toolCall)
		}
	}

	a.mu.RLock()
	modelUsed := a.model
	a.mu.RUnlock()

	a.logger.Debug("LLM response generated",
		zap.String("model", modelUsed),
		zap.Int("tool_calls", len(response.ToolCalls)),
		zap.Bool("has_content", response.Content != ""),
	)

	return response, nil
}

// parseJSONArguments parses the JSON string arguments into a map
func parseJSONArguments(jsonStr string) (map[string]interface{}, error) {
	var args map[string]interface{}
	if jsonStr == "" {
		return make(map[string]interface{}), nil
	}
	
	err := json.Unmarshal([]byte(jsonStr), &args)
	if err != nil {
		return nil, fmt.Errorf("failed to parse arguments: %w", err)
	}
	
	return args, nil
}

