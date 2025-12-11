package tools

import (
	"context"
)

// ============================================================================
// Conversation Tool Implementations
// ============================================================================

func (e *Executor) executeGetHistory(ctx context.Context, execCtx *ExecutionContext, args map[string]interface{}) *ToolResult {
	channelID, _ := args["channel_id"].(string)
	if channelID == "" {
		channelID = execCtx.ChannelID
	}

	limit := 20
	if l, ok := args["limit"].(float64); ok {
		limit = int(l)
	}

	messages, err := e.repo.GetConversationHistory(ctx, channelID, limit)
	if err != nil {
		return &ToolResult{Success: false, Error: err.Error()}
	}

	return &ToolResult{
		Success: true,
		Data:    messages,
	}
}

func (e *Executor) executeSendMessage(ctx context.Context, execCtx *ExecutionContext, args map[string]interface{}) *ToolResult {
	message, _ := args["message"].(string)
	
	// This is handled specially - the message becomes the response content
	return &ToolResult{
		Success: true,
		Message: message,
	}
}

