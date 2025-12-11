package tools

import (
	"context"
	"fmt"

	"go.uber.org/zap"
)

// ============================================================================
// Personality/Mimic Tool Implementations
// ============================================================================

func (e *Executor) executeMimicPersonality(ctx context.Context, execCtx *ExecutionContext, args map[string]interface{}) *ToolResult {
	if e.discordExecutor == nil {
		return &ToolResult{Success: false, Error: "Discord not available - mimicking only works in Discord"}
	}

	userID, _ := args["user_id"].(string)
	if userID == "" {
		return &ToolResult{Success: false, Error: "user_id is required"}
	}

	channelID, _ := args["channel_id"].(string)
	if channelID == "" {
		channelID = execCtx.ChannelID
	}
	if channelID == "" {
		return &ToolResult{Success: false, Error: "channel_id is required for personality analysis"}
	}

	messageCount := 50
	if mc, ok := args["message_count"].(float64); ok {
		messageCount = int(mc)
	}

	// Save the original personality before mimicking
	originalPersonality := ""
	state, err := e.repo.FetchState(ctx, execCtx.AgentID)
	if err == nil && state != nil {
		originalPersonality = state.Identity.Personality
	}

	// Analyze the user's personality
	profile, err := e.discordExecutor.AnalyzeUserPersonality(ctx, channelID, userID, messageCount)
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("Failed to analyze personality: %v", err)}
	}

	// Store the mimic state
	e.mimicStates[execCtx.AgentID] = &MimicState{
		Active:              true,
		OriginalPersonality: originalPersonality,
		MimicProfile:        profile,
	}

	e.logger.Info("Mimic mode activated",
		zap.String("agent_id", execCtx.AgentID),
		zap.String("mimicking_user", profile.Username),
		zap.Int("messages_analyzed", profile.MessageCount),
	)

	return &ToolResult{
		Success: true,
		Data: map[string]interface{}{
			"mimicking":          profile.Username,
			"messages_analyzed":  profile.MessageCount,
			"style":              profile.ToneIndicators,
			"capitalization":     profile.Capitalization,
			"avg_message_length": profile.AvgMessageLength,
		},
		Message: fmt.Sprintf("Now mimicking %s's personality based on %d messages. Use revert_personality to stop.", profile.Username, profile.MessageCount),
	}
}

func (e *Executor) executeRevertPersonality(ctx context.Context, execCtx *ExecutionContext) *ToolResult {
	state := e.mimicStates[execCtx.AgentID]
	if state == nil || !state.Active {
		return &ToolResult{
			Success: true,
			Message: "Not currently mimicking anyone.",
		}
	}

	mimickedUser := ""
	if state.MimicProfile != nil {
		mimickedUser = state.MimicProfile.Username
	}

	// Clear the mimic state
	delete(e.mimicStates, execCtx.AgentID)

	e.logger.Info("Mimic mode deactivated",
		zap.String("agent_id", execCtx.AgentID),
		zap.String("was_mimicking", mimickedUser),
	)

	return &ToolResult{
		Success: true,
		Message: fmt.Sprintf("Stopped mimicking %s. Reverted to original personality.", mimickedUser),
	}
}

func (e *Executor) executeAnalyzeUserStyle(ctx context.Context, execCtx *ExecutionContext, args map[string]interface{}) *ToolResult {
	if e.discordExecutor == nil {
		return &ToolResult{Success: false, Error: "Discord not available"}
	}

	userID, _ := args["user_id"].(string)
	if userID == "" {
		return &ToolResult{Success: false, Error: "user_id is required"}
	}

	channelID, _ := args["channel_id"].(string)
	if channelID == "" {
		channelID = execCtx.ChannelID
	}
	if channelID == "" {
		return &ToolResult{Success: false, Error: "channel_id is required"}
	}

	profile, err := e.discordExecutor.AnalyzeUserPersonality(ctx, channelID, userID, 100)
	if err != nil {
		return &ToolResult{Success: false, Error: err.Error()}
	}

	return &ToolResult{
		Success: true,
		Data: map[string]interface{}{
			"username":           profile.Username,
			"messages_analyzed":  profile.MessageCount,
			"avg_message_length": profile.AvgMessageLength,
			"capitalization":     profile.Capitalization,
			"punctuation":        profile.PunctuationStyle,
			"tone":               profile.ToneIndicators,
			"common_words":       profile.CommonWords,
			"emoji_usage":        profile.EmojiUsage,
			"sample_messages":    profile.SampleMessages,
		},
		Message: fmt.Sprintf("Analyzed %d messages from %s", profile.MessageCount, profile.Username),
	}
}

