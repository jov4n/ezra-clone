package tools

import (
	"context"
	"fmt"
	"strings"

	"ezra-clone/backend/internal/graph"
	apperrors "ezra-clone/backend/pkg/errors"

	"github.com/bwmarrin/discordgo"
	"go.uber.org/zap"
)

// DiscordMessage represents a simplified Discord message
type DiscordMessage struct {
	ID        string `json:"id"`
	Content   string `json:"content"`
	AuthorID  string `json:"author_id"`
	Author    string `json:"author"`
	Timestamp string `json:"timestamp"`
}

// DiscordUserInfo represents Discord user information
type DiscordUserInfo struct {
	ID            string `json:"id"`
	Username      string `json:"username"`
	Discriminator string `json:"discriminator"`
	GlobalName    string `json:"global_name,omitempty"`
	Bot           bool   `json:"bot"`
	AvatarURL     string `json:"avatar_url,omitempty"`
}

// DiscordChannelInfo represents Discord channel information
type DiscordChannelInfo struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Topic   string `json:"topic,omitempty"`
	Type    string `json:"type"`
	GuildID string `json:"guild_id,omitempty"`
}

// FormatHabits represents Discord-native formatting patterns
type FormatHabits struct {
	CodeTicksRate float64 `json:"code_ticks_rate"` // Rate of `code` usage
	CodeBlockRate float64 `json:"code_block_rate"` // Rate of ``` code blocks
	EmphasisRate  float64 `json:"emphasis_rate"`   // Rate of * or ** usage
	QuoteRate     float64 `json:"quote_rate"`      // Rate of > quote blocks
	MultiLineRate float64 `json:"multi_line_rate"` // Rate of multi-line messages
	EllipsisRate  float64 `json:"ellipsis_rate"`   // Rate of ... usage
}

// MessageLengthDistribution represents message length statistics
type MessageLengthDistribution struct {
	P25        float64 `json:"p25"`        // 25th percentile
	P50        float64 `json:"p50"`        // Median
	P75        float64 `json:"p75"`        // 75th percentile
	Burstiness float64 `json:"burstiness"` // Messages within 60s windows
}

// PersonalityProfile represents an analyzed user's communication style
type PersonalityProfile struct {
	UserID             string                    `json:"user_id"`
	Username           string                    `json:"username"`
	MessageCount       int                       `json:"message_count"`
	AvgMessageLength   float64                   `json:"avg_message_length"`
	LengthDistribution MessageLengthDistribution `json:"length_distribution"`
	CommonWords        []string                  `json:"common_words"`
	CommonPhrases      []string                  `json:"common_phrases"`
	EmojiUsage         []string                  `json:"emoji_usage"`
	Capitalization     string                    `json:"capitalization"`    // "normal", "lowercase", "uppercase", "mixed"
	PunctuationStyle   string                    `json:"punctuation_style"` // "minimal", "normal", "heavy"
	ToneIndicators     []string                  `json:"tone_indicators"`   // e.g., "casual", "formal", "enthusiastic"
	FormatHabits       FormatHabits              `json:"format_habits"`
	SampleMessages     []string                  `json:"sample_messages"`
	StylePrompt        string                    `json:"style_prompt"` // Generated prompt for LLM to mimic
}

// DiscordExecutor handles Discord-specific tool execution
type DiscordExecutor struct {
	session *discordgo.Session
	logger  *zap.Logger
	repo    *graph.Repository // For RAG memory access
}

// NewDiscordExecutor creates a new Discord executor
func NewDiscordExecutor(session *discordgo.Session, logger *zap.Logger) *DiscordExecutor {
	return &DiscordExecutor{
		session: session,
		logger:  logger,
	}
}

// SetRepository sets the graph repository for RAG memory access
func (d *DiscordExecutor) SetRepository(repo *graph.Repository) {
	d.repo = repo
}

// SetSession updates the Discord session (useful for late binding)
func (d *DiscordExecutor) SetSession(session *discordgo.Session) {
	d.session = session
}

// ReadChannelHistory reads messages from a Discord channel
func (d *DiscordExecutor) ReadChannelHistory(ctx context.Context, channelID string, limit int, fromUserID string) ([]DiscordMessage, error) {
	if ctx != nil {
		select {
		case <-ctx.Done():
			return nil, apperrors.NewContextCancelled("AnalyzeUserPersonality", ctx.Err())
		default:
		}
	}

	if d.session == nil {
		return nil, apperrors.ErrDiscordSessionUnavailable
	}

	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}

	messages, err := d.session.ChannelMessages(channelID, limit, "", "", "")
	if err != nil {
		return nil, fmt.Errorf("failed to get channel messages: %w", err)
	}

	var result []DiscordMessage
	for _, msg := range messages {
		// Filter by user if specified
		if fromUserID != "" && msg.Author.ID != fromUserID {
			continue
		}

		// Skip bot messages and empty messages
		if msg.Author.Bot || strings.TrimSpace(msg.Content) == "" {
			continue
		}

		result = append(result, DiscordMessage{
			ID:        msg.ID,
			Content:   msg.Content,
			AuthorID:  msg.Author.ID,
			Author:    msg.Author.Username,
			Timestamp: msg.Timestamp.Format("2006-01-02 15:04:05"),
		})
	}

	// Reverse to get chronological order
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}

	return result, nil
}

// GetUserInfo gets information about a Discord user
func (d *DiscordExecutor) GetUserInfo(ctx context.Context, userID string) (*DiscordUserInfo, error) {
	if ctx != nil {
		select {
		case <-ctx.Done():
			return nil, apperrors.NewContextCancelled("GetUserInfo", ctx.Err())
		default:
		}
	}

	if d.session == nil {
		return nil, apperrors.ErrDiscordSessionUnavailable
	}

	user, err := d.session.User(userID)
	if err != nil {
		return nil, apperrors.NewDiscordUserNotFound(userID)
	}

	return &DiscordUserInfo{
		ID:            user.ID,
		Username:      user.Username,
		Discriminator: user.Discriminator,
		GlobalName:    user.GlobalName,
		Bot:           user.Bot,
		AvatarURL:     user.AvatarURL("256"),
	}, nil
}

// GetChannelInfo gets information about a Discord channel
func (d *DiscordExecutor) GetChannelInfo(ctx context.Context, channelID string) (*DiscordChannelInfo, error) {
	if ctx != nil {
		select {
		case <-ctx.Done():
			return nil, apperrors.NewContextCancelled("GetChannelInfo", ctx.Err())
		default:
		}
	}

	if d.session == nil {
		return nil, apperrors.ErrDiscordSessionUnavailable
	}

	channel, err := d.session.Channel(channelID)
	if err != nil {
		return nil, apperrors.NewDiscordChannelNotFound(channelID)
	}

	channelType := "unknown"
	switch channel.Type {
	case discordgo.ChannelTypeGuildText:
		channelType = "text"
	case discordgo.ChannelTypeDM:
		channelType = "dm"
	case discordgo.ChannelTypeGuildVoice:
		channelType = "voice"
	case discordgo.ChannelTypeGroupDM:
		channelType = "group_dm"
	case discordgo.ChannelTypeGuildCategory:
		channelType = "category"
	}

	return &DiscordChannelInfo{
		ID:      channel.ID,
		Name:    channel.Name,
		Topic:   channel.Topic,
		Type:    channelType,
		GuildID: channel.GuildID,
	}, nil
}

