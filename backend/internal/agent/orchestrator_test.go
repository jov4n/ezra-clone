package agent

import (
	"context"
	"testing"
	"time"

	"ezra-clone/backend/internal/adapter"
	"ezra-clone/backend/internal/graph"
	"ezra-clone/backend/internal/state"
)

// Mock implementations for testing

type mockGraphRepo struct {
	state      *state.ContextWindow
	updateErr  error
	logErr     error
}

func (m *mockGraphRepo) FetchState(ctx context.Context, agentID string) (*state.ContextWindow, error) {
	if m.state == nil {
		return &state.ContextWindow{
			Identity: state.AgentIdentity{
				Name:        "TestAgent",
				Personality: "Helpful",
				Capabilities: []string{"chat"},
			},
			CoreMemory: []state.MemoryBlock{
				{Name: "identity", Content: "I am TestAgent", UpdatedAt: time.Now()},
			},
			ArchivalRefs: []state.ArchivalPointer{},
			UserContext:  make(map[string]interface{}),
		}, nil
	}
	return m.state, nil
}

func (m *mockGraphRepo) UpdateMemory(ctx context.Context, agentID, blockName, newContent string) error {
	if m.updateErr != nil {
		return m.updateErr
	}
	// Update mock state
	if m.state == nil {
		m.state = &state.ContextWindow{
			Identity: state.AgentIdentity{Name: "TestAgent"},
			CoreMemory: []state.MemoryBlock{},
		}
	}
	// Simple update logic
	for i, mem := range m.state.CoreMemory {
		if mem.Name == blockName {
			m.state.CoreMemory[i].Content = newContent
			return nil
		}
	}
	m.state.CoreMemory = append(m.state.CoreMemory, state.MemoryBlock{
		Name:      blockName,
		Content:   newContent,
		UpdatedAt: time.Now(),
	})
	return nil
}

func (m *mockGraphRepo) LogInteraction(ctx context.Context, agentID, userID, message string, timestamp time.Time) error {
	return m.logErr
}

type mockLLMAdapter struct {
	response *adapter.Response
	err     error
	generateFunc func(ctx context.Context, systemPrompt, userMsg string, tools []adapter.Tool) (*adapter.Response, error)
}

func (m *mockLLMAdapter) Generate(ctx context.Context, systemPrompt, userMsg string, tools []adapter.Tool) (*adapter.Response, error) {
	if m.generateFunc != nil {
		return m.generateFunc(ctx, systemPrompt, userMsg, tools)
	}
	if m.err != nil {
		return nil, m.err
	}
	if m.response != nil {
		return m.response, nil
	}
	return &adapter.Response{
		Content:   "Hello!",
		ToolCalls: []adapter.ToolCall{},
	}, nil
}

func TestOrchestrator_RunTurn_ContentResponse(t *testing.T) {
	ctx := context.Background()
	mockGraph := &mockGraphRepo{}
	mockLLM := &mockLLMAdapter{
		response: &adapter.Response{
			Content:   "Hello, how can I help you?",
			ToolCalls: []adapter.ToolCall{},
		},
	}

	orch := NewOrchestrator(mockGraph, mockLLM)
	result, err := orch.RunTurn(ctx, "test-agent", "test-user", "Hello")
	
	if err != nil {
		t.Fatalf("RunTurn failed: %v", err)
	}
	if result.Content != "Hello, how can I help you?" {
		t.Errorf("Expected content 'Hello, how can I help you?', got '%s'", result.Content)
	}
	if result.Ignored {
		t.Error("Expected ignored to be false")
	}
}

func TestOrchestrator_RunTurn_UpdateMemory(t *testing.T) {
	ctx := context.Background()
	mockGraph := &mockGraphRepo{}
	
	// Use a counter to simulate recursion
	callCount := 0
	mockLLM := &mockLLMAdapter{
		generateFunc: func(ctx context.Context, systemPrompt, userMsg string, tools []adapter.Tool) (*adapter.Response, error) {
			callCount++
			if callCount == 1 {
				// First call: tool call to update memory
				return &adapter.Response{
					Content: "",
					ToolCalls: []adapter.ToolCall{
						{
							ID:   "call-1",
							Name: "update_core_memory",
							Arguments: map[string]interface{}{
								"name":    "identity",
								"content": "I am UpdatedAgent",
							},
						},
					},
				}, nil
			}
			// Second call: content response
			return &adapter.Response{
				Content:   "I've updated my identity.",
				ToolCalls: []adapter.ToolCall{},
			}, nil
		},
	}

	orch := NewOrchestrator(mockGraph, mockLLM)
	result, err := orch.RunTurn(ctx, "test-agent", "test-user", "Change your name to UpdatedAgent")
	
	if err != nil {
		t.Fatalf("RunTurn failed: %v", err)
	}
	if result.Content == "" {
		t.Error("Expected content after recursion")
	}
}

func TestOrchestrator_RunTurn_Ignore(t *testing.T) {
	ctx := context.Background()
	mockGraph := &mockGraphRepo{}
	mockLLM := &mockLLMAdapter{
		response: &adapter.Response{
			Content: "",
			ToolCalls: []adapter.ToolCall{
				{
					ID:       "call-1",
					Name:     "ignore",
					Arguments: map[string]interface{}{},
				},
			},
		},
	}

	orch := NewOrchestrator(mockGraph, mockLLM)
	result, err := orch.RunTurn(ctx, "test-agent", "test-user", "Some irrelevant message")
	
	if err != nil {
		t.Fatalf("RunTurn failed: %v", err)
	}
	if !result.Ignored {
		t.Error("Expected ignored to be true")
	}
}

func TestOrchestrator_RunTurn_MaxRecursion(t *testing.T) {
	ctx := context.Background()
	mockGraph := &mockGraphRepo{}
	
	// Create a response that always triggers memory update
	updateResponse := &adapter.Response{
		Content: "",
		ToolCalls: []adapter.ToolCall{
			{
				ID:   "call-1",
				Name: "update_core_memory",
				Arguments: map[string]interface{}{
					"name":    "test",
					"content": "test",
				},
			},
		},
	}
	
	mockLLM := &mockLLMAdapter{
		response: updateResponse,
	}

	orch := NewOrchestrator(mockGraph, mockLLM)
	_, err := orch.RunTurn(ctx, "test-agent", "test-user", "Test")
	
	if err != ErrMaxRecursion {
		t.Errorf("Expected ErrMaxRecursion, got %v", err)
	}
}

