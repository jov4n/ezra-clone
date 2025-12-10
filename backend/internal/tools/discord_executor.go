package tools

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

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
	ID       string `json:"id"`
	Name     string `json:"name"`
	Topic    string `json:"topic,omitempty"`
	Type     string `json:"type"`
	GuildID  string `json:"guild_id,omitempty"`
}

// PersonalityProfile represents an analyzed user's communication style
type PersonalityProfile struct {
	UserID           string   `json:"user_id"`
	Username         string   `json:"username"`
	MessageCount     int      `json:"message_count"`
	AvgMessageLength float64  `json:"avg_message_length"`
	CommonWords      []string `json:"common_words"`
	CommonPhrases    []string `json:"common_phrases"`
	EmojiUsage       []string `json:"emoji_usage"`
	Capitalization   string   `json:"capitalization"` // "normal", "lowercase", "uppercase", "mixed"
	PunctuationStyle string   `json:"punctuation_style"` // "minimal", "normal", "heavy"
	ToneIndicators   []string `json:"tone_indicators"` // e.g., "casual", "formal", "enthusiastic"
	SampleMessages   []string `json:"sample_messages"`
	StylePrompt      string   `json:"style_prompt"` // Generated prompt for LLM to mimic
}

// DiscordExecutor handles Discord-specific tool execution
type DiscordExecutor struct {
	session *discordgo.Session
	logger  *zap.Logger
}

// NewDiscordExecutor creates a new Discord executor
func NewDiscordExecutor(session *discordgo.Session, logger *zap.Logger) *DiscordExecutor {
	return &DiscordExecutor{
		session: session,
		logger:  logger,
	}
}

// SetSession updates the Discord session (useful for late binding)
func (d *DiscordExecutor) SetSession(session *discordgo.Session) {
	d.session = session
}

// ReadChannelHistory reads messages from a Discord channel
func (d *DiscordExecutor) ReadChannelHistory(ctx context.Context, channelID string, limit int, fromUserID string) ([]DiscordMessage, error) {
	if d.session == nil {
		return nil, fmt.Errorf("Discord session not available")
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
	if d.session == nil {
		return nil, fmt.Errorf("Discord session not available")
	}

	user, err := d.session.User(userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
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
	if d.session == nil {
		return nil, fmt.Errorf("Discord session not available")
	}

	channel, err := d.session.Channel(channelID)
	if err != nil {
		return nil, fmt.Errorf("failed to get channel: %w", err)
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

// AnalyzeUserPersonality analyzes a user's messages to create a personality profile
func (d *DiscordExecutor) AnalyzeUserPersonality(ctx context.Context, channelID, userID string, messageCount int) (*PersonalityProfile, error) {
	if d.session == nil {
		return nil, fmt.Errorf("Discord session not available")
	}

	if messageCount <= 0 {
		messageCount = 50
	}

	// Get user info
	userInfo, err := d.GetUserInfo(ctx, userID)
	if err != nil {
		return nil, err
	}

	// Get messages from the user - we may need to fetch more to get enough from this user
	var userMessages []string
	fetchedCount := 0
	beforeID := ""
	maxFetches := 10 // Prevent infinite loops

	for len(userMessages) < messageCount && fetchedCount < maxFetches {
		messages, err := d.session.ChannelMessages(channelID, 100, beforeID, "", "")
		if err != nil {
			break
		}

		if len(messages) == 0 {
			break
		}

		for _, msg := range messages {
			if msg.Author.ID == userID && !msg.Author.Bot && strings.TrimSpace(msg.Content) != "" {
				userMessages = append(userMessages, msg.Content)
			}
		}

		beforeID = messages[len(messages)-1].ID
		fetchedCount++
	}

	if len(userMessages) == 0 {
		return nil, fmt.Errorf("no messages found from user %s", userID)
	}

	// Analyze the messages
	profile := &PersonalityProfile{
		UserID:       userID,
		Username:     userInfo.Username,
		MessageCount: len(userMessages),
	}

	// Calculate average message length
	totalLength := 0
	for _, msg := range userMessages {
		totalLength += len(msg)
	}
	profile.AvgMessageLength = float64(totalLength) / float64(len(userMessages))

	// Analyze capitalization style
	profile.Capitalization = analyzeCapitalization(userMessages)

	// Analyze punctuation
	profile.PunctuationStyle = analyzePunctuation(userMessages)

	// Extract common words
	profile.CommonWords = extractCommonWords(userMessages, 10)

	// Extract emoji usage
	profile.EmojiUsage = extractEmojis(userMessages)

	// Determine tone indicators
	profile.ToneIndicators = analyzeTone(userMessages)

	// Get sample messages (diverse selection)
	profile.SampleMessages = selectSampleMessages(userMessages, 5)

	// Generate the style prompt for the LLM
	profile.StylePrompt = generateStylePrompt(profile)

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

func generateStylePrompt(profile *PersonalityProfile) string {
	var parts []string

	parts = append(parts, fmt.Sprintf("You are now mimicking the communication style of %s.", profile.Username))

	// Capitalization
	switch profile.Capitalization {
	case "lowercase":
		parts = append(parts, "Write entirely in lowercase letters, never capitalize.")
	case "uppercase":
		parts = append(parts, "Write in ALL CAPS frequently to show emphasis.")
	case "mixed":
		parts = append(parts, "Use casual capitalization, sometimes starting sentences without capitals.")
	}

	// Punctuation
	switch profile.PunctuationStyle {
	case "minimal":
		parts = append(parts, "Use minimal punctuation, often omitting periods at the end of messages.")
	case "heavy":
		parts = append(parts, "Use punctuation expressively, with multiple exclamation marks or question marks when excited.")
	}

	// Tone
	if len(profile.ToneIndicators) > 0 {
		parts = append(parts, fmt.Sprintf("Your tone is: %s.", strings.Join(profile.ToneIndicators, ", ")))
	}

	// Common words
	if len(profile.CommonWords) > 0 {
		parts = append(parts, fmt.Sprintf("Frequently use words like: %s.", strings.Join(profile.CommonWords, ", ")))
	}

	// Emoji usage
	if len(profile.EmojiUsage) > 0 {
		parts = append(parts, fmt.Sprintf("Use emojis like: %s.", strings.Join(profile.EmojiUsage, " ")))
	} else {
		parts = append(parts, "Rarely or never use emojis.")
	}

	// Message length
	if profile.AvgMessageLength < 50 {
		parts = append(parts, "Keep messages short and concise.")
	} else if profile.AvgMessageLength > 150 {
		parts = append(parts, "Write longer, more detailed messages.")
	}

	// Sample messages
	if len(profile.SampleMessages) > 0 {
		parts = append(parts, "\nHere are example messages from this person to mimic:")
		for _, sample := range profile.SampleMessages {
			// Truncate very long samples
			if len(sample) > 200 {
				sample = sample[:200] + "..."
			}
			parts = append(parts, fmt.Sprintf("- \"%s\"", sample))
		}
	}

	parts = append(parts, "\nMimic their exact style, vocabulary, and mannerisms in all your responses.")

	return strings.Join(parts, "\n")
}

