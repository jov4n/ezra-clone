package adapter

import (
	"context"
	"testing"
)

// TestLLMAdapter_Generate requires a running LiteLLM instance
// This is a basic integration test
func TestLLMAdapter_Generate(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	adapter := NewLLMAdapter("http://localhost:4000", "", "openrouter/anthropic/claude-3.5-sonnet")
	
	ctx := context.Background()
	systemPrompt := "You are a helpful assistant."
	userMsg := "Say hello in one sentence."

	response, err := adapter.Generate(ctx, systemPrompt, userMsg, []Tool{})
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	if response.Content == "" {
		t.Error("Expected non-empty content in response")
	}
}

func TestLLMAdapter_Generate_WithTools(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	adapter := NewLLMAdapter("http://localhost:4000", "", "openrouter/anthropic/claude-3.5-sonnet")
	
	ctx := context.Background()
	systemPrompt := "You are a helpful assistant with access to tools."
	userMsg := "Update your name to 'TestBot' using the update_core_memory tool."

	tools := []Tool{
		{
			Type: "function",
			Function: FunctionDefinition{
				Name:        "update_core_memory",
				Description: "Update a memory block",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"name": map[string]interface{}{
							"type": "string",
						},
						"content": map[string]interface{}{
							"type": "string",
						},
					},
					"required": []string{"name", "content"},
				},
			},
		},
	}

	response, err := adapter.Generate(ctx, systemPrompt, userMsg, tools)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	if len(response.ToolCalls) == 0 {
		t.Log("No tool calls in response (this is acceptable if model chose not to use tools)")
	} else {
		t.Logf("Received %d tool calls", len(response.ToolCalls))
		for _, tc := range response.ToolCalls {
			t.Logf("Tool: %s, Args: %v", tc.Name, tc.Arguments)
		}
	}
}

