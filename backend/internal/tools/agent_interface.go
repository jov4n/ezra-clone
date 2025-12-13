package tools

import (
	"context"
)

// AgentOrchestrator defines the interface for agent orchestration
// This interface breaks the import cycle between tools and agent packages
type AgentOrchestrator interface {
	// RunTurn processes a user message and returns the agent's response
	// Returns a TurnResult interface that must have a Content() method
	RunTurn(ctx context.Context, agentID, userID, message string) (TurnResult, error)
}

// TurnResult defines the interface for agent turn results
// This allows the tools package to work with agent results without importing agent
type TurnResult interface {
	GetContent() string
	IsIgnored() bool
}

