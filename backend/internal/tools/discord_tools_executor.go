package tools

import (
	"context"
	"fmt"
)

// ============================================================================
// Discord Tool Implementations
// ============================================================================

func (e *Executor) executeDiscordReadHistory(ctx context.Context, execCtx *ExecutionContext, args map[string]interface{}) *ToolResult {
	if e.discordExecutor == nil {
		return &ToolResult{Success: false, Error: "Discord not available (only works in Discord bot context)"}
	}

	channelID, _ := args["channel_id"].(string)
	if channelID == "" {
		channelID = execCtx.ChannelID
	}
	if channelID == "" {
		return &ToolResult{Success: false, Error: "channel_id is required"}
	}

	limit := 50
	if l, ok := args["limit"].(float64); ok {
		limit = int(l)
	}

	fromUserID, _ := args["from_user_id"].(string)

	messages, err := e.discordExecutor.ReadChannelHistory(ctx, channelID, limit, fromUserID)
	if err != nil {
		return &ToolResult{Success: false, Error: err.Error()}
	}

	return &ToolResult{
		Success: true,
		Data:    messages,
		Message: fmt.Sprintf("Retrieved %d messages from channel", len(messages)),
	}
}

func (e *Executor) executeDiscordGetUserInfo(ctx context.Context, args map[string]interface{}) *ToolResult {
	if e.discordExecutor == nil {
		return &ToolResult{Success: false, Error: "Discord not available"}
	}

	userID, _ := args["user_id"].(string)
	if userID == "" {
		return &ToolResult{Success: false, Error: "user_id is required"}
	}

	userInfo, err := e.discordExecutor.GetUserInfo(ctx, userID)
	if err != nil {
		return &ToolResult{Success: false, Error: err.Error()}
	}

	return &ToolResult{
		Success: true,
		Data:    userInfo,
	}
}

func (e *Executor) executeDiscordGetChannelInfo(ctx context.Context, execCtx *ExecutionContext, args map[string]interface{}) *ToolResult {
	if e.discordExecutor == nil {
		return &ToolResult{Success: false, Error: "Discord not available"}
	}

	channelID, _ := args["channel_id"].(string)
	if channelID == "" {
		channelID = execCtx.ChannelID
	}
	if channelID == "" {
		return &ToolResult{Success: false, Error: "channel_id is required"}
	}

	channelInfo, err := e.discordExecutor.GetChannelInfo(ctx, channelID)
	if err != nil {
		return &ToolResult{Success: false, Error: err.Error()}
	}

	return &ToolResult{
		Success: true,
		Data:    channelInfo,
	}
}

// Note: executeReadCodebase and codebase reading functions are now in codebase_reader.go

