package tools

import (
	"context"
	"fmt"
)

// ============================================================================
// Knowledge Tool Implementations
// ============================================================================

func (e *Executor) executeCreateFact(ctx context.Context, execCtx *ExecutionContext, args map[string]interface{}) *ToolResult {
	content, _ := args["content"].(string)
	if content == "" {
		return &ToolResult{Success: false, Error: "content is required"}
	}

	source, _ := args["source"].(string)
	
	var topics []string
	if topicsArg, ok := args["topics"].([]interface{}); ok {
		for _, t := range topicsArg {
			if ts, ok := t.(string); ok {
				topics = append(topics, ts)
			}
		}
	}

	fact, err := e.repo.CreateFact(ctx, execCtx.AgentID, content, source, execCtx.UserID, topics)
	if err != nil {
		return &ToolResult{Success: false, Error: err.Error()}
	}

	return &ToolResult{
		Success: true,
		Data:    fact,
		Message: fmt.Sprintf("Fact stored: %s", content),
	}
}

func (e *Executor) executeSearchFacts(ctx context.Context, execCtx *ExecutionContext, args map[string]interface{}) *ToolResult {
	topic, _ := args["topic"].(string)
	if topic == "" {
		return &ToolResult{Success: false, Error: "topic is required"}
	}

	facts, err := e.repo.GetFactsAboutTopic(ctx, topic)
	if err != nil {
		return &ToolResult{Success: false, Error: err.Error()}
	}

	return &ToolResult{
		Success: true,
		Data:    facts,
		Message: fmt.Sprintf("Found %d facts about '%s'", len(facts), topic),
	}
}

func (e *Executor) executeGetUserContext(ctx context.Context, execCtx *ExecutionContext, args map[string]interface{}) *ToolResult {
	userID, _ := args["user_id"].(string)
	if userID == "" {
		userID = execCtx.UserID
	}

	userCtx, err := e.repo.GetUserContext(ctx, userID)
	if err != nil {
		return &ToolResult{Success: false, Error: err.Error()}
	}

	// Build a helpful message summarizing what was found
	message := fmt.Sprintf("Retrieved user context for user %s", userID)
	if userCtx != nil {
		if len(userCtx.Facts) > 0 {
			message += fmt.Sprintf(" - Found %d fact(s)", len(userCtx.Facts))
		}
		if len(userCtx.Topics) > 0 {
			message += fmt.Sprintf(" - %d interest(s)", len(userCtx.Topics))
		}
		if len(userCtx.Facts) == 0 && len(userCtx.Topics) == 0 {
			message += " - No information found yet"
		}
	}

	return &ToolResult{
		Success: true,
		Data:    userCtx,
		Message: message,
	}
}

