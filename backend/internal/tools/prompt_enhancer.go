package tools

import (
	"context"
	"fmt"

	"ezra-clone/backend/internal/adapter"
	"ezra-clone/backend/pkg/logger"
	"go.uber.org/zap"
)

// PromptEnhancer enhances prompts using Z-Image Turbo methodology
type PromptEnhancer struct {
	llmAdapter *adapter.LLMAdapter
	logger     *zap.Logger
}

// NewPromptEnhancer creates a new prompt enhancer
func NewPromptEnhancer(llmAdapter *adapter.LLMAdapter) *PromptEnhancer {
	return &PromptEnhancer{
		llmAdapter: llmAdapter,
		logger:     logger.Get(),
	}
}

// Enhance enhances a user prompt using Z-Image Turbo template
func (p *PromptEnhancer) Enhance(ctx context.Context, userRequest string) (string, error) {
	if userRequest == "" {
		return "", fmt.Errorf("user request cannot be empty")
	}

	p.logger.Debug("Enhancing prompt",
		zap.String("original", truncateString(userRequest, 50)),
	)

	// Create system prompt with Z-Image Turbo template
	systemPrompt := ZIT_PROMPT_TEMPLATE

	// Call LLM to enhance the prompt
	// We use the LLM adapter's Generate method, but we need to make a direct API call
	// since we want a simple text completion, not tool calling
	response, err := p.llmAdapter.Generate(ctx, systemPrompt, userRequest, []adapter.Tool{})
	if err != nil {
		p.logger.Warn("Failed to enhance prompt, using original",
			zap.Error(err),
		)
		// Fallback to original prompt
		return userRequest, nil
	}

	enhanced := response.Content
	if enhanced == "" {
		p.logger.Warn("Empty enhancement response, using original")
		return userRequest, nil
	}

	p.logger.Debug("Prompt enhanced successfully",
		zap.String("enhanced", truncateString(enhanced, 50)),
	)

	return enhanced, nil
}

// truncateString truncates a string for logging
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

