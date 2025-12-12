package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
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

// FetchUserMessages fetches messages from a user with proper pagination
func (d *DiscordExecutor) FetchUserMessages(ctx context.Context, channelID, userID string, target int) ([]*discordgo.Message, error) {
	if ctx != nil {
		select {
		case <-ctx.Done():
			return nil, apperrors.NewContextCancelled("FetchUserMessages", ctx.Err())
		default:
		}
	}

	if target <= 0 {
		target = 300
	}

	var out []*discordgo.Message
	beforeID := ""

	for len(out) < target {
		if ctx != nil {
			select {
			case <-ctx.Done():
				return out, apperrors.NewContextCancelled("FetchUserMessages", ctx.Err())
			default:
			}
		}

		batch, err := d.session.ChannelMessages(channelID, 100, beforeID, "", "")
		if err != nil {
			return out, err
		}

		if len(batch) == 0 {
			break
		}

		for _, m := range batch {
			if m.Author == nil || m.Author.Bot {
				continue
			}
			if userID != "" && m.Author.ID != userID {
				continue
			}
			if strings.TrimSpace(m.Content) == "" {
				continue
			}
			out = append(out, m)
			if len(out) >= target {
				break
			}
		}

		beforeID = batch[len(batch)-1].ID
	}

	// Reverse to chronological order
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}

	return out, nil
}

// FetchUserMessagesFromGuild fetches messages from all text channels in a guild
func (d *DiscordExecutor) FetchUserMessagesFromGuild(ctx context.Context, guildID, userID string, messagesPerChannel int) ([]*discordgo.Message, error) {
	if ctx != nil {
		select {
		case <-ctx.Done():
			return nil, apperrors.NewContextCancelled("FetchUserMessagesFromGuild", ctx.Err())
		default:
		}
	}

	if d.session == nil {
		return nil, apperrors.ErrDiscordSessionUnavailable
	}

	// Get all channels in the guild
	channels, err := d.session.GuildChannels(guildID)
	if err != nil {
		return nil, fmt.Errorf("failed to get guild channels: %w", err)
	}

	// Filter to only text channels (skip forum, voice, category, etc.)
	var textChannels []*discordgo.Channel
	for _, ch := range channels {
		// Only include standard text channels
		if ch.Type == discordgo.ChannelTypeGuildText {
			textChannels = append(textChannels, ch)
		} else {
			d.logger.Debug("Skipping non-text channel",
				zap.String("channel_id", ch.ID),
				zap.String("channel_name", ch.Name),
				zap.Int("channel_type", int(ch.Type)),
			)
		}
	}

	if len(textChannels) == 0 {
		return nil, fmt.Errorf("no text channels found in guild %s", guildID)
	}

	d.logger.Info("Fetching messages from all guild channels",
		zap.String("guild_id", guildID),
		zap.String("user_id", userID),
		zap.Int("total_channels", len(textChannels)),
		zap.Int("messages_per_channel", messagesPerChannel),
	)

	// Fetch messages from each channel
	var allMessages []*discordgo.Message
	for i, ch := range textChannels {
		if ctx != nil {
			select {
			case <-ctx.Done():
				return allMessages, apperrors.NewContextCancelled("FetchUserMessagesFromGuild", ctx.Err())
			default:
			}
		}

		d.logger.Debug("Fetching messages from channel",
			zap.String("channel_id", ch.ID),
			zap.String("channel_name", ch.Name),
			zap.Int("channel_index", i+1),
			zap.Int("total_channels", len(textChannels)),
		)

		messages, err := d.FetchUserMessages(ctx, ch.ID, userID, messagesPerChannel)
		if err != nil {
			// Check if it's an unsupported channel type error
			errStr := err.Error()
			if strings.Contains(errStr, "unknown component type") ||
				strings.Contains(errStr, "component type") ||
				strings.Contains(errStr, "unsupported") {
				d.logger.Debug("Skipping unsupported channel type",
					zap.String("channel_id", ch.ID),
					zap.String("channel_name", ch.Name),
					zap.String("error", errStr),
				)
			} else {
				d.logger.Warn("Failed to fetch messages from channel",
					zap.String("channel_id", ch.ID),
					zap.String("channel_name", ch.Name),
					zap.Error(err),
				)
			}
			// Continue with other channels even if one fails
			continue
		}

		allMessages = append(allMessages, messages...)
		d.logger.Debug("Fetched messages from channel",
			zap.String("channel_id", ch.ID),
			zap.String("channel_name", ch.Name),
			zap.Int("message_count", len(messages)),
			zap.Int("total_so_far", len(allMessages)),
		)
	}

	// Sort all messages by timestamp (oldest first)
	sort.Slice(allMessages, func(i, j int) bool {
		return allMessages[i].Timestamp.Before(allMessages[j].Timestamp)
	})

	d.logger.Info("Fetched messages from all guild channels",
		zap.String("guild_id", guildID),
		zap.String("user_id", userID),
		zap.Int("channels_searched", len(textChannels)),
		zap.Int("total_messages", len(allMessages)),
	)

	return allMessages, nil
}

// AnalyzeUserPersonality analyzes a user's messages to create a personality profile
// If forceUpdate is false, it will check for a cached profile first
func (d *DiscordExecutor) AnalyzeUserPersonality(ctx context.Context, channelID, userID string, messageCount int, forceUpdate bool) (*PersonalityProfile, error) {
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

	if messageCount <= 0 {
		messageCount = 300 // Default to more messages for better analysis
	}

	// Get channel info to determine guild
	channelInfo, err := d.GetChannelInfo(ctx, channelID)
	if err != nil {
		return nil, fmt.Errorf("failed to get channel info: %w", err)
	}

	guildID := channelInfo.GuildID
	if guildID == "" {
		guildID = "dm" // Use "dm" as guild ID for DMs
	}

	// Check for cached profile if not forcing update
	if !forceUpdate && d.repo != nil {
		cachedProfileJSON, err := d.repo.GetUserPersonalityProfile(ctx, userID, guildID)
		if err == nil && cachedProfileJSON != "" {
			// Deserialize cached profile
			var profile PersonalityProfile
			if err := json.Unmarshal([]byte(cachedProfileJSON), &profile); err == nil {
				d.logger.Info("Using cached personality profile",
					zap.String("user_id", userID),
					zap.String("guild_id", guildID),
				)
				// Regenerate style prompt with RAG (in case memories were updated)
				profile.StylePrompt = d.generateStylePromptWithRAG(ctx, &profile, userID)
				return &profile, nil
			} else {
				d.logger.Warn("Failed to deserialize cached profile, re-analyzing",
					zap.String("user_id", userID),
					zap.Error(err),
				)
			}
		}
	}

	d.logger.Info("Analyzing user personality",
		zap.String("user_id", userID),
		zap.String("channel_id", channelID),
		zap.String("guild_id", guildID),
		zap.Int("requested_message_count", messageCount),
		zap.Bool("force_update", forceUpdate),
	)

	// Get user info
	userInfo, err := d.GetUserInfo(ctx, userID)
	if err != nil {
		return nil, err
	}

	var messages []*discordgo.Message

	// If we have a guild ID, fetch from all channels in the guild
	if channelInfo.GuildID != "" {
		d.logger.Info("Channel is in a guild, fetching from all channels",
			zap.String("guild_id", channelInfo.GuildID),
		)

		// Calculate messages per channel (distribute across channels)
		// First, get the number of text channels
		channels, err := d.session.GuildChannels(channelInfo.GuildID)
		if err == nil {
			textChannelCount := 0
			for _, ch := range channels {
				if ch.Type == discordgo.ChannelTypeGuildText {
					textChannelCount++
				}
			}

			if textChannelCount > 0 {
				// Fetch 300 messages from each channel in the guild
				messagesPerChannel := 300
				d.logger.Info("Fetching 300 messages from each channel in guild",
					zap.String("guild_id", channelInfo.GuildID),
					zap.Int("text_channels", textChannelCount),
					zap.Int("messages_per_channel", messagesPerChannel),
				)

				messages, err = d.FetchUserMessagesFromGuild(ctx, channelInfo.GuildID, userID, messagesPerChannel)
				if err != nil {
					d.logger.Warn("Failed to fetch from all guild channels, falling back to single channel",
						zap.Error(err),
					)
					// Fall back to single channel
					messages, err = d.FetchUserMessages(ctx, channelID, userID, messageCount)
					if err != nil {
						return nil, fmt.Errorf("failed to fetch messages: %w", err)
					}
				}
			} else {
				// No text channels, fall back to single channel
				messages, err = d.FetchUserMessages(ctx, channelID, userID, messageCount)
				if err != nil {
					return nil, fmt.Errorf("failed to fetch messages: %w", err)
				}
			}
		} else {
			d.logger.Warn("Failed to get guild channels, falling back to single channel",
				zap.Error(err),
			)
			// Fall back to single channel
			messages, err = d.FetchUserMessages(ctx, channelID, userID, messageCount)
			if err != nil {
				return nil, fmt.Errorf("failed to fetch messages: %w", err)
			}
		}
	} else {
		// Not a guild channel (DM), just fetch from this channel
		messages, err = d.FetchUserMessages(ctx, channelID, userID, messageCount)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch messages: %w", err)
		}
	}

	if len(messages) == 0 {
		return nil, fmt.Errorf("no messages found from user %s", userID)
	}

	d.logger.Info("Fetched messages for analysis",
		zap.String("user_id", userID),
		zap.Int("requested_count", messageCount),
		zap.Int("actual_count", len(messages)),
		zap.String("source", func() string {
			if channelInfo.GuildID != "" {
				return "all_guild_channels"
			}
			return "single_channel"
		}()),
	)

	// Extract message content strings
	var userMessages []string
	for _, msg := range messages {
		userMessages = append(userMessages, msg.Content)
	}

	// Analyze the messages
	profile := &PersonalityProfile{
		UserID:       userID,
		Username:     userInfo.Username,
		MessageCount: len(userMessages),
	}

	// Calculate average message length
	totalLength := 0
	lengths := make([]int, len(userMessages))
	for i, msg := range userMessages {
		lengths[i] = len(msg)
		totalLength += len(msg)
	}
	profile.AvgMessageLength = float64(totalLength) / float64(len(userMessages))

	// Calculate message length distribution
	profile.LengthDistribution = analyzeMessageLengthDistribution(lengths, messages)

	// Analyze capitalization style
	profile.Capitalization = analyzeCapitalization(userMessages)

	// Analyze punctuation
	profile.PunctuationStyle = analyzePunctuation(userMessages)

	// Extract common words
	profile.CommonWords = extractCommonWords(userMessages, 10)

	// Extract common phrases (bigrams)
	profile.CommonPhrases = extractCommonPhrases(userMessages, 2, 12)

	// Extract emoji usage
	profile.EmojiUsage = extractEmojis(userMessages)

	// Analyze formatting habits
	profile.FormatHabits = analyzeFormatHabits(userMessages)

	// Determine tone indicators
	profile.ToneIndicators = analyzeTone(userMessages)

	// Get sample messages (diverse selection)
	profile.SampleMessages = selectSampleMessages(userMessages, 5)

	// Extract and store personality facts/opinions for RAG (if repository available)
	if d.repo != nil {
		potentialFacts := extractPersonalityFactsFromMessages(userMessages)
		for _, fact := range potentialFacts {
			// Store as consented memory (user consented by using mimic_personality tool)
			_, err := d.repo.StoreUserPersonalityMemory(ctx, userID, fact, "discord_analysis", channelID, []string{"auto_extracted"}, true)
			if err != nil {
				d.logger.Warn("Failed to store personality memory", zap.Error(err))
			}
		}
	}

	// Generate the style prompt for the LLM (includes RAG memories if available)
	profile.StylePrompt = d.generateStylePromptWithRAG(ctx, profile, userID)

	// Cache the profile for future use
	if d.repo != nil {
		profileJSON, err := json.Marshal(profile)
		if err == nil {
			if err := d.repo.StoreUserPersonalityProfile(ctx, userID, guildID, string(profileJSON)); err != nil {
				d.logger.Warn("Failed to cache personality profile",
					zap.String("user_id", userID),
					zap.String("guild_id", guildID),
					zap.Error(err),
				)
			} else {
				d.logger.Info("Personality profile cached",
					zap.String("user_id", userID),
					zap.String("guild_id", guildID),
				)
			}
		}
	}

	return profile, nil
}

// Helper functions for personality analysis

func analyzeCapitalization(messages []string) string {
	lowercase := 0
	uppercase := 0
	normal := 0

	for _, msg := range messages {
		if msg == strings.ToLower(msg) {
			lowercase++
		} else if msg == strings.ToUpper(msg) {
			uppercase++
		} else if len(msg) > 0 && msg[0] >= 'A' && msg[0] <= 'Z' {
			normal++
		}
	}

	total := len(messages)
	if lowercase > total*70/100 {
		return "lowercase"
	}
	if uppercase > total*50/100 {
		return "uppercase"
	}
	if normal > total*50/100 {
		return "normal"
	}
	return "mixed"
}

func analyzePunctuation(messages []string) string {
	punctCount := 0
	totalChars := 0

	for _, msg := range messages {
		totalChars += len(msg)
		for _, c := range msg {
			if c == '!' || c == '?' || c == '.' || c == ',' || c == ';' || c == ':' {
				punctCount++
			}
		}
	}

	if totalChars == 0 {
		return "minimal"
	}

	ratio := float64(punctCount) / float64(totalChars)
	if ratio < 0.02 {
		return "minimal"
	}
	if ratio > 0.08 {
		return "heavy"
	}
	return "normal"
}

func extractCommonWords(messages []string, limit int) []string {
	wordCount := make(map[string]int)
	stopWords := map[string]bool{
		"the": true, "a": true, "an": true, "is": true, "are": true,
		"was": true, "were": true, "be": true, "been": true, "being": true,
		"have": true, "has": true, "had": true, "do": true, "does": true,
		"did": true, "will": true, "would": true, "could": true, "should": true,
		"may": true, "might": true, "must": true, "shall": true,
		"i": true, "you": true, "he": true, "she": true, "it": true,
		"we": true, "they": true, "me": true, "him": true, "her": true,
		"us": true, "them": true, "my": true, "your": true, "his": true,
		"its": true, "our": true, "their": true, "this": true, "that": true,
		"these": true, "those": true, "and": true, "but": true, "or": true,
		"so": true, "if": true, "then": true, "than": true, "of": true,
		"in": true, "on": true, "at": true, "to": true, "for": true,
		"with": true, "by": true, "from": true, "up": true, "about": true,
		"into": true, "through": true, "during": true, "before": true,
		"after": true, "above": true, "below": true, "between": true,
		"under": true, "again": true, "further": true, "once": true,
		"just": true, "like": true, "dont": true, "im": true,
	}

	wordRegex := regexp.MustCompile(`[a-zA-Z]+`)

	for _, msg := range messages {
		words := wordRegex.FindAllString(strings.ToLower(msg), -1)
		for _, word := range words {
			if len(word) > 2 && !stopWords[word] {
				wordCount[word]++
			}
		}
	}

	// Sort by frequency
	type wordFreq struct {
		word  string
		count int
	}
	var sorted []wordFreq
	for word, count := range wordCount {
		sorted = append(sorted, wordFreq{word, count})
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].count > sorted[j].count
	})

	var result []string
	for i := 0; i < limit && i < len(sorted); i++ {
		result = append(result, sorted[i].word)
	}
	return result
}

func extractCommonPhrases(messages []string, ngram int, limit int) []string {
	count := make(map[string]int)
	tokenize := regexp.MustCompile(`[a-zA-Z0-9']+`).FindAllString

	for _, msg := range messages {
		words := tokenize(strings.ToLower(msg), -1)
		if len(words) < ngram {
			continue
		}
		for i := 0; i <= len(words)-ngram; i++ {
			ng := strings.Join(words[i:i+ngram], " ")
			// Skip tiny / mostly stop-gram
			if len(ng) < 4 {
				continue
			}
			count[ng]++
		}
	}

	type kv struct {
		k string
		v int
	}
	var arr []kv
	for k, v := range count {
		arr = append(arr, kv{k, v})
	}
	sort.Slice(arr, func(i, j int) bool {
		return arr[i].v > arr[j].v
	})

	out := []string{}
	for _, it := range arr {
		out = append(out, it.k)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func analyzeFormatHabits(messages []string) FormatHabits {
	var codeTicks, codeBlock, emph, quote, multi, ell int

	for _, m := range messages {
		if strings.Contains(m, "```") {
			codeBlock++
		}
		if strings.Contains(m, "`") {
			codeTicks++
		}
		if strings.Contains(m, "**") || strings.Contains(m, "*") {
			emph++
		}
		lines := strings.Split(m, "\n")
		if len(lines) > 1 {
			multi++
		}
		for _, ln := range lines {
			if strings.HasPrefix(strings.TrimSpace(ln), ">") {
				quote++
				break
			}
		}
		if strings.Contains(m, "...") {
			ell++
		}
	}

	total := float64(len(messages))
	if total == 0 {
		return FormatHabits{}
	}

	return FormatHabits{
		CodeTicksRate: float64(codeTicks) / total,
		CodeBlockRate: float64(codeBlock) / total,
		EmphasisRate:  float64(emph) / total,
		QuoteRate:     float64(quote) / total,
		MultiLineRate: float64(multi) / total,
		EllipsisRate:  float64(ell) / total,
	}
}

func analyzeMessageLengthDistribution(lengths []int, messages []*discordgo.Message) MessageLengthDistribution {
	if len(lengths) == 0 {
		return MessageLengthDistribution{}
	}

	// Calculate percentiles
	sorted := make([]int, len(lengths))
	copy(sorted, lengths)
	sort.Ints(sorted)

	p25 := sorted[len(sorted)/4]
	p50 := sorted[len(sorted)/2]
	p75 := sorted[len(sorted)*3/4]

	// Calculate burstiness (messages within 60s windows)
	burstiness := 0.0
	if len(messages) > 1 {
		burstCount := 0
		for i := 1; i < len(messages); i++ {
			// Parse timestamps and check if within 60s
			if messages[i].Timestamp.After(messages[i-1].Timestamp) {
				diff := messages[i].Timestamp.Sub(messages[i-1].Timestamp)
				if diff.Seconds() < 60 {
					burstCount++
				}
			}
		}
		burstiness = float64(burstCount) / float64(len(messages)-1)
	}

	return MessageLengthDistribution{
		P25:        float64(p25),
		P50:        float64(p50),
		P75:        float64(p75),
		Burstiness: burstiness,
	}
}

func extractEmojis(messages []string) []string {
	emojiCount := make(map[string]int)

	// Common emoji patterns
	emojiRegex := regexp.MustCompile(`[\x{1F600}-\x{1F64F}]|[\x{1F300}-\x{1F5FF}]|[\x{1F680}-\x{1F6FF}]|[\x{2600}-\x{26FF}]|[\x{2700}-\x{27BF}]|:\w+:|<:\w+:\d+>`)

	for _, msg := range messages {
		emojis := emojiRegex.FindAllString(msg, -1)
		for _, emoji := range emojis {
			emojiCount[emoji]++
		}
	}

	// Sort by frequency
	type emojiFreq struct {
		emoji string
		count int
	}
	var sorted []emojiFreq
	for emoji, count := range emojiCount {
		sorted = append(sorted, emojiFreq{emoji, count})
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].count > sorted[j].count
	})

	var result []string
	for i := 0; i < 5 && i < len(sorted); i++ {
		result = append(result, sorted[i].emoji)
	}
	return result
}

// extractPersonalityFactsFromMessages extracts potential facts/opinions from user messages
// This is a helper that can be called during personality analysis to auto-extract memories
func extractPersonalityFactsFromMessages(messages []string) []string {
	var facts []string
	opinionPatterns := []string{
		"i hate", "i love", "i dislike", "i prefer", "i always", "i never",
		"i think", "i believe", "i feel", "my favorite", "my least favorite",
		"i'm a fan of", "i can't stand", "i'm into", "i'm not into",
	}

	for _, msg := range messages {
		lowerMsg := strings.ToLower(msg)
		for _, pattern := range opinionPatterns {
			if strings.Contains(lowerMsg, pattern) {
				// Extract the sentence containing the opinion
				sentences := strings.Split(msg, ".")
				for _, sentence := range sentences {
					if strings.Contains(strings.ToLower(sentence), pattern) {
						trimmed := strings.TrimSpace(sentence)
						if len(trimmed) > 10 && len(trimmed) < 200 {
							facts = append(facts, trimmed)
							break
						}
					}
				}
			}
		}
	}

	// Deduplicate
	seen := make(map[string]bool)
	var uniqueFacts []string
	for _, fact := range facts {
		if !seen[fact] {
			seen[fact] = true
			uniqueFacts = append(uniqueFacts, fact)
		}
	}

	return uniqueFacts
}

func analyzeTone(messages []string) []string {
	var indicators []string

	// Count various indicators
	exclamations := 0
	questions := 0
	lol := 0
	caps := 0
	longMessages := 0

	for _, msg := range messages {
		if strings.Contains(msg, "!") {
			exclamations++
		}
		if strings.Contains(msg, "?") {
			questions++
		}
		lower := strings.ToLower(msg)
		if strings.Contains(lower, "lol") || strings.Contains(lower, "lmao") || strings.Contains(lower, "haha") {
			lol++
		}
		if msg == strings.ToUpper(msg) && len(msg) > 3 {
			caps++
		}
		if len(msg) > 200 {
			longMessages++
		}
	}

	total := len(messages)
	if total == 0 {
		return []string{"neutral"}
	}

	if exclamations > total*30/100 {
		indicators = append(indicators, "enthusiastic")
	}
	if questions > total*20/100 {
		indicators = append(indicators, "inquisitive")
	}
	if lol > total*20/100 {
		indicators = append(indicators, "humorous")
	}
	if caps > total*10/100 {
		indicators = append(indicators, "expressive")
	}
	if longMessages > total*30/100 {
		indicators = append(indicators, "detailed")
	} else if longMessages < total*10/100 {
		indicators = append(indicators, "concise")
	}

	if len(indicators) == 0 {
		indicators = append(indicators, "casual")
	}

	return indicators
}

func selectSampleMessages(messages []string, count int) []string {
	if len(messages) <= count {
		return messages
	}

	// Select diverse messages (short, medium, long)
	var short, medium, long []string
	for _, msg := range messages {
		l := len(msg)
		if l < 50 {
			short = append(short, msg)
		} else if l < 150 {
			medium = append(medium, msg)
		} else {
			long = append(long, msg)
		}
	}

	var result []string
	// Try to get a mix
	if len(short) > 0 {
		result = append(result, short[0])
	}
	if len(medium) > 0 {
		result = append(result, medium[0])
	}
	if len(long) > 0 {
		result = append(result, long[0])
	}

	// Fill remaining from medium (most representative)
	for i := 1; len(result) < count && i < len(medium); i++ {
		result = append(result, medium[i])
	}

	return result
}

// generateStylePromptWithRAG generates style prompt with RAG memory context
func (d *DiscordExecutor) generateStylePromptWithRAG(ctx context.Context, profile *PersonalityProfile, userID string) string {
	basePrompt := generateStylePrompt(profile)

	// If repository available, retrieve relevant memories
	if d.repo != nil {
		// Use a general query to get user's preferences/opinions
		memories, err := d.repo.RetrieveUserPersonalityMemories(ctx, userID, "preference opinion fact", 5)
		if err == nil && len(memories) > 0 {
			var b strings.Builder
			b.WriteString(basePrompt)
			b.WriteString("\n\nREFERENCE MEMORIES (approved facts - do not invent new ones):\n")
			for _, mem := range memories {
				if mem.Content != "" {
					b.WriteString(fmt.Sprintf("- %s\n", mem.Content))
				}
			}
			b.WriteString("\nUse these memories to stay consistent. Do not claim knowledge beyond these approved memories.\n")
			return b.String()
		}
	}

	return basePrompt
}

func generateStylePrompt(profile *PersonalityProfile) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("You ARE %s. You are writing as yourself on Discord.\n", profile.Username))
	b.WriteString("Write exactly as you normally would. Be authentic to your own communication style.\n\n")

	b.WriteString("STYLE RULES:\n")
	switch profile.Capitalization {
	case "lowercase":
		b.WriteString("- lowercase style: mostly lowercase, minimal sentence caps.\n")
	case "uppercase":
		b.WriteString("- emphasis caps: occasional ALL CAPS for emphasis.\n")
	case "mixed":
		b.WriteString("- mixed casual capitalization.\n")
	default:
		b.WriteString("- normal capitalization.\n")
	}

	b.WriteString(fmt.Sprintf("- punctuation: %s\n", profile.PunctuationStyle))

	if len(profile.ToneIndicators) > 0 {
		b.WriteString("- tone: " + strings.Join(profile.ToneIndicators, ", ") + "\n")
	}

	if len(profile.CommonWords) > 0 {
		b.WriteString("- common words: " + strings.Join(profile.CommonWords, ", ") + "\n")
	}

	if len(profile.CommonPhrases) > 0 {
		b.WriteString("- common phrases: " + strings.Join(profile.CommonPhrases, ", ") + "\n")
	}

	if len(profile.EmojiUsage) > 0 {
		b.WriteString("- emoji set: " + strings.Join(profile.EmojiUsage, " ") + "\n")
	} else {
		b.WriteString("- emoji: rarely\n")
	}

	// Formatting habits summary
	b.WriteString("- formatting habits:\n")
	b.WriteString(fmt.Sprintf("  - code ticks rate ~%.2f, code blocks ~%.2f, multiline ~%.2f, ellipses ~%.2f\n",
		profile.FormatHabits.CodeTicksRate,
		profile.FormatHabits.CodeBlockRate,
		profile.FormatHabits.MultiLineRate,
		profile.FormatHabits.EllipsisRate,
	))

	// Message length guidance
	if profile.AvgMessageLength < 50 {
		b.WriteString("- message length: short and concise\n")
	} else if profile.AvgMessageLength > 150 {
		b.WriteString("- message length: longer, detailed messages\n")
	}

	b.WriteString("\nIMPORTANT GUIDELINES:\n")
	b.WriteString("- Write naturally and authentically in your own style.\n")
	b.WriteString("- Do NOT quote the provided examples verbatim - use them as style reference only.\n")
	b.WriteString("- Stay true to your communication patterns and vocabulary.\n")
	b.WriteString("- Be authentic to yourself in every response.\n")

	// Optional: provide 3–5 short examples but instruct "pattern only"
	if len(profile.SampleMessages) > 0 {
		b.WriteString("\nEXAMPLES (pattern only, never copy):\n")
		for _, s := range profile.SampleMessages {
			s = strings.ReplaceAll(s, "\n", " ")
			if len(s) > 140 {
				s = s[:140] + "…"
			}
			b.WriteString("- " + s + "\n")
		}
	}

	b.WriteString("\nRespond naturally as yourself. Be authentic to your communication style in every message.\n")

	return b.String()
}
