package agent

import (
	"context"
	"ezra-clone/backend/internal/tools"
)

// OrchestratorAdapter wraps Orchestrator to implement tools.AgentOrchestrator interface
type OrchestratorAdapter struct {
	*Orchestrator
}

// RunTurn implements tools.AgentOrchestrator interface
func (a *OrchestratorAdapter) RunTurn(ctx context.Context, agentID, userID, message string) (tools.TurnResult, error) {
	result, err := a.Orchestrator.RunTurn(ctx, agentID, userID, message)
	if err != nil {
		return nil, err
	}
	return &TurnResultWrapper{TurnResult: result}, nil
}

// TurnResultWrapper wraps agent.TurnResult to implement tools.TurnResult interface
type TurnResultWrapper struct {
	*TurnResult
}

// GetContent returns the content of the turn result
func (w *TurnResultWrapper) GetContent() string {
	return w.TurnResult.Content
}

// IsIgnored returns whether the turn was ignored
func (w *TurnResultWrapper) IsIgnored() bool {
	return w.TurnResult.Ignored
}

