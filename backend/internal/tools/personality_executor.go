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

	// Check if already mimicking this user
	currentState := e.mimicStates[execCtx.AgentID]
	if currentState != nil && currentState.Active && currentState.MimicProfile != nil {
		if currentState.MimicProfile.UserID == userID {
			// Already mimicking this user, return a result that tells the LLM to just respond
			e.logger.Info("Already mimicking this user - skipping tool call",
				zap.String("agent_id", execCtx.AgentID),
				zap.String("user_id", userID),
				zap.String("username", currentState.MimicProfile.Username),
			)
			// Return a result that indicates success but tells the LLM to respond naturally
			// The LLM should interpret this as "you're already set up, just respond"
			return &ToolResult{
				Success: true,
				Data: map[string]interface{}{
					"mimicking":         currentState.MimicProfile.Username,
					"messages_analyzed": currentState.MimicProfile.MessageCount,
					"already_active":    true,
					"instruction":       "You are already mimicking this user. Do NOT call this tool again. Just respond to the user's message naturally in the mimicked style.",
				},
				Message: fmt.Sprintf("You are already mimicking %s's personality. Respond to the user's message naturally in their style - do not call this tool again.", currentState.MimicProfile.Username),
			}
		}
	}

	channelID, _ := args["channel_id"].(string)
	if channelID == "" {
		channelID = execCtx.ChannelID
	}
	if channelID == "" {
		return &ToolResult{Success: false, Error: "channel_id is required for personality analysis"}
	}

	// Check if user wants to force update
	forceUpdate := false
	if update, ok := args["update"].(bool); ok {
		forceUpdate = update
	} else if update, ok := args["update"].(string); ok {
		forceUpdate = (update == "true" || update == "1")
	}

	messageCount := 300 // Default to 300 for better analysis
	if mc, ok := args["message_count"].(float64); ok {
		specifiedCount := int(mc)
		// Enforce minimum of 300 for better analysis
		if specifiedCount < 300 {
			e.logger.Info("Message count specified is less than 300, enforcing minimum",
				zap.Int("specified_count", specifiedCount),
				zap.Int("enforced_count", messageCount),
			)
		} else {
			messageCount = specifiedCount
			e.logger.Info("Message count specified in tool call",
				zap.Int("specified_count", messageCount),
			)
		}
	} else {
		e.logger.Info("Using default message count (no message_count in args)",
			zap.Int("default_count", messageCount),
		)
	}

	// Save the original personality before mimicking
	originalPersonality := ""
	state, err := e.repo.FetchState(ctx, execCtx.AgentID)
	if err == nil && state != nil {
		originalPersonality = state.Identity.Personality
	}

	// Analyze the user's personality (will use cache unless forceUpdate is true)
	profile, err := e.discordExecutor.AnalyzeUserPersonality(ctx, channelID, userID, messageCount, forceUpdate)
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("Failed to analyze personality: %v", err)}
	}

	// Store the mimic state
	e.mimicStates[execCtx.AgentID] = &MimicState{
		Active:              true,
		OriginalPersonality: originalPersonality,
		MimicProfile:        profile,
	}

	// Start background task if available
	if e.mimicBackgroundTask != nil {
		e.mimicBackgroundTask.Start(execCtx.AgentID)
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

	// Stop background task if running
	if e.mimicBackgroundTask != nil {
		e.mimicBackgroundTask.Stop()
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

	profile, err := e.discordExecutor.AnalyzeUserPersonality(ctx, channelID, userID, 100, false)
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

