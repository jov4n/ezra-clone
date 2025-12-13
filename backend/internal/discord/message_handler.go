package discord

import (
	"context"
	"fmt"
	"strings"

	"github.com/bwmarrin/discordgo"
	"ezra-clone/backend/internal/agent"
	"ezra-clone/backend/internal/constants"
	"ezra-clone/backend/internal/graph"
	"ezra-clone/backend/internal/utils"
	apperrors "ezra-clone/backend/pkg/errors"
	"go.uber.org/zap"
)

// Handler handles Discord message processing
type Handler struct {
	agentOrch *agent.Orchestrator
	graphRepo *graph.Repository
	logger    *zap.Logger
}

// NewHandler creates a new Discord message handler
func NewHandler(agentOrch *agent.Orchestrator, graphRepo *graph.Repository, logger *zap.Logger) *Handler {
	return &Handler{
		agentOrch: agentOrch,
		graphRepo: graphRepo,
		logger:    logger,
	}
}

// HandleMessage processes a Discord message
func (h *Handler) HandleMessage(s *discordgo.Session, m *discordgo.MessageCreate) {
	// Ignore messages from the bot itself
	if m.Author.ID == s.State.User.ID {
		return
	}

	// Check if message is a DM or mentions the bot
	isDM := m.GuildID == ""
	isMentioned := false

	// Check for mentions
	for _, mention := range m.Mentions {
		if mention.ID == s.State.User.ID {
			isMentioned = true
			break
		}
	}

	// Also check if message starts with bot mention
	content := strings.TrimSpace(m.Content)
	if strings.HasPrefix(content, "<@"+s.State.User.ID+">") || strings.HasPrefix(content, "<!@"+s.State.User.ID+">") {
		isMentioned = true
		// Remove mention from content
		content = strings.TrimPrefix(content, "<@"+s.State.User.ID+">")
		content = strings.TrimPrefix(content, "<!@"+s.State.User.ID+">")
		content = strings.TrimSpace(content)
	}

	// Only respond to DMs or mentions
	if !isDM && !isMentioned {
		return
	}

	// Skip empty messages
	if content == "" {
		return
	}

	h.logger.Info("Processing Discord message",
		zap.String("user_id", m.Author.ID),
		zap.String("channel_id", m.ChannelID),
		zap.Bool("is_dm", isDM),
	)

	ctx := context.Background()

	// Ensure message author exists in database before processing
	_, err := h.graphRepo.GetOrCreateUser(ctx, m.Author.ID, m.Author.ID, m.Author.Username, "discord")
	if err != nil {
		h.logger.Error("Failed to get/create user",
			zap.String("user_id", m.Author.ID),
			zap.Error(err),
		)
		// Continue anyway - user creation failure shouldn't block message processing
	}

	// Create users for any mentioned users (even if they haven't talked yet)
	h.createMentionedUsers(ctx, s, m)

	// Check for language preference instructions before processing
	languagePreferenceSet, targetUserForLang := HandleLanguagePreferenceInstruction(ctx, s, m, content, h.graphRepo, h.logger)

	// If language preference was set, send confirmation and skip LLM processing
	if languagePreferenceSet && targetUserForLang != "" {
		langCode := utils.ExtractLanguageFromMessage(content)
		langName := utils.GetLanguageName(langCode)
		confirmationMsg := fmt.Sprintf("✅ I've noted that %s prefers %s!", targetUserForLang, langName)
		_, _ = s.ChannelMessageSend(m.ChannelID, confirmationMsg)
		return
	} else if languagePreferenceSet {
		// Language preference set for requester
		langCode := utils.ExtractLanguageFromMessage(content)
		langName := utils.GetLanguageName(langCode)
		confirmationMsg := fmt.Sprintf("✅ I've noted that you prefer %s! I'll respond in %s from now on.", langName, langName)
		_, _ = s.ChannelMessageSend(m.ChannelID, confirmationMsg)
		return
	}

	// Run agent turn with full context
	agentID := constants.DefaultAgentID // Default agent ID
	channelID := m.ChannelID
	platform := "discord"
	result, err := h.agentOrch.RunTurnWithContext(ctx, agentID, m.Author.ID, channelID, platform, content)

	if err != nil {
		if apperrors.IsErrorType(err, apperrors.ErrorTypeAgent) && err == agent.ErrIgnored {
			// Agent chose to ignore - do nothing (lurker mode)
			h.logger.Debug("Agent ignored message",
				zap.String("user_id", m.Author.ID),
			)
			return
		}

		// Log error with type information
		errType := "unknown"
		if baseErr, ok := err.(*apperrors.BaseError); ok {
			errType = string(baseErr.Type)
		}
		h.logger.Error("Failed to process message",
			zap.Error(err),
			zap.String("error_type", errType),
			zap.String("user_id", m.Author.ID),
		)

		// If language preference was set but LLM failed, at least acknowledge that
		if languagePreferenceSet {
			_, _ = s.ChannelMessageSend(m.ChannelID, "✅ Language preference has been set! However, I encountered an error generating a response. Please check your API configuration.")
			return
		}

		// Optionally notify user of error
		_, _ = s.ChannelMessageSend(m.ChannelID, "Sorry, I encountered an error processing your message.")
		return
	}

	// Send the response
	h.sendResponse(s, m.ChannelID, result)
}

// Note: sendResponse, sendLongMessage, and splitMessage are now in response_sender.go
// Note: createMentionedUsers is now in user_management.go
