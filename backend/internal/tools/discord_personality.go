package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	apperrors "ezra-clone/backend/pkg/errors"

	"github.com/bwmarrin/discordgo"
	"go.uber.org/zap"
)

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

