package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"

	"github.com/bwmarrin/discordgo"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"ezra-clone/backend/internal/adapter"
	"ezra-clone/backend/internal/agent"
	"ezra-clone/backend/internal/graph"
	"ezra-clone/backend/internal/tools"
	"ezra-clone/backend/pkg/config"
	"ezra-clone/backend/pkg/logger"
	"go.uber.org/zap"
)

func main() {
	// Initialize logger
	if err := logger.Init("development"); err != nil {
		panic(fmt.Sprintf("Failed to initialize logger: %v", err))
	}
	defer logger.Sync()

	log := logger.Get()
	log.Info("Starting Discord bot...")

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatal("Failed to load configuration", zap.Error(err))
	}

	if cfg.DiscordBotToken == "" {
		log.Fatal("DISCORD_BOT_TOKEN is required")
	}

	// Initialize Neo4j driver
	driver, err := neo4j.NewDriverWithContext(
		cfg.Neo4jURI,
		neo4j.BasicAuth(cfg.Neo4jUser, cfg.Neo4jPassword, ""),
	)
	if err != nil {
		log.Fatal("Failed to create Neo4j driver", zap.Error(err))
	}
	defer driver.Close(context.Background())

	// Verify Neo4j connection
	ctx := context.Background()
	if err := driver.VerifyConnectivity(ctx); err != nil {
		log.Fatal("Failed to verify Neo4j connectivity", zap.Error(err))
	}

	// Initialize dependencies
	graphRepo := graph.NewRepository(driver)
	llmAdapter := adapter.NewLLMAdapter(cfg.LiteLLMURL, cfg.OpenRouterAPIKey, cfg.ModelID)
	agentOrch := agent.NewOrchestrator(graphRepo, llmAdapter)

	// Create Discord session
	dg, err := discordgo.New("Bot " + cfg.DiscordBotToken)
	if err != nil {
		log.Fatal("Failed to create Discord session", zap.Error(err))
	}

	// Create Discord executor for Discord-specific tools
	discordExecutor := tools.NewDiscordExecutor(dg, log)
	agentOrch.SetDiscordExecutor(discordExecutor)

	// Add message handler
	dg.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		handleMessage(s, m, agentOrch, graphRepo, log)
	})

	// Set intents
	dg.Identify.Intents = discordgo.IntentsGuildMessages | discordgo.IntentsDirectMessages

	// Open connection
	if err := dg.Open(); err != nil {
		log.Fatal("Failed to open Discord connection", zap.Error(err))
	}
	defer dg.Close()

	log.Info("Discord bot is running. Press CTRL-C to exit.")

	// Wait for interrupt signal
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc

	log.Info("Shutting down Discord bot...")
}

func handleMessage(s *discordgo.Session, m *discordgo.MessageCreate, agentOrch *agent.Orchestrator, graphRepo *graph.Repository, log *zap.Logger) {
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

	log.Info("Processing Discord message",
		zap.String("user_id", m.Author.ID),
		zap.String("channel_id", m.ChannelID),
		zap.Bool("is_dm", isDM),
	)

	// Check for language preference instructions before processing
	ctx := context.Background()
	handleLanguagePreferenceInstruction(ctx, s, m, content, graphRepo, log)

	// Run agent turn with full context
	agentID := "Ezra" // Default agent ID
	channelID := m.ChannelID
	platform := "discord"
	result, err := agentOrch.RunTurnWithContext(ctx, agentID, m.Author.ID, channelID, platform, content)

	if err != nil {
		if err == agent.ErrIgnored {
			// Agent chose to ignore - do nothing (lurker mode)
			log.Debug("Agent ignored message",
				zap.String("user_id", m.Author.ID),
			)
			return
		}

		log.Error("Failed to process message",
			zap.Error(err),
			zap.String("user_id", m.Author.ID),
		)

		// Optionally notify user of error
		_, _ = s.ChannelMessageSend(m.ChannelID, "Sorry, I encountered an error processing your message.")
		return
	}

	// Send response - use embeds if available, otherwise plain text
	if len(result.Embeds) > 0 {
		// Convert agent embeds to Discord embeds
		var discordEmbeds []*discordgo.MessageEmbed
		for _, e := range result.Embeds {
			embed := &discordgo.MessageEmbed{
				Title:       e.Title,
				Description: e.Description,
				URL:         e.URL,
				Color:       e.Color,
			}
			
			// Add fields if present
			for _, f := range e.Fields {
				embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
					Name:   f.Name,
					Value:  f.Value,
					Inline: f.Inline,
				})
			}
			
			// Add footer if present
			if e.Footer != "" {
				embed.Footer = &discordgo.MessageEmbedFooter{
					Text: e.Footer,
				}
			}
			
			discordEmbeds = append(discordEmbeds, embed)
		}
		
		// Send message with embeds - truncate content if needed
		content := result.Content
		if len(content) > 2000 {
			content = content[:1997] + "..."
		}
		
		_, err := s.ChannelMessageSendComplex(m.ChannelID, &discordgo.MessageSend{
			Content: content,
			Embeds:  discordEmbeds,
		})
		if err != nil {
			log.Error("Failed to send embed message",
				zap.Error(err),
				zap.String("channel_id", m.ChannelID),
			)
		}
	} else if result.Content != "" {
		// Plain text message - split if too long
		sendLongMessage(s, m.ChannelID, result.Content, log)
	}
}

// sendLongMessage splits a message into chunks if it exceeds Discord's 2000 character limit
func sendLongMessage(s *discordgo.Session, channelID, content string, log *zap.Logger) {
	const maxLength = 2000
	const chunkPrefix = "```\n"
	const chunkSuffix = "\n```"
	const maxChunkLength = maxLength - len(chunkPrefix) - len(chunkSuffix)
	
	if len(content) <= maxLength {
		// Message fits in one chunk
		_, err := s.ChannelMessageSend(channelID, content)
		if err != nil {
			log.Error("Failed to send message",
				zap.Error(err),
				zap.String("channel_id", channelID),
			)
		}
		return
	}
	
	// Split into chunks
	chunks := splitMessage(content, maxChunkLength)
	
	for i, chunk := range chunks {
		var message string
		if len(chunks) > 1 {
			// Add chunk indicator for multi-part messages
			message = fmt.Sprintf("%s%s%s\n*(Part %d/%d)*", chunkPrefix, chunk, chunkSuffix, i+1, len(chunks))
		} else {
			message = chunk
		}
		
		// Ensure we don't exceed limit even with prefix
		if len(message) > maxLength {
			message = message[:maxLength-3] + "..."
		}
		
		_, err := s.ChannelMessageSend(channelID, message)
		if err != nil {
			log.Error("Failed to send message chunk",
				zap.Error(err),
				zap.String("channel_id", channelID),
				zap.Int("chunk", i+1),
				zap.Int("total_chunks", len(chunks)),
			)
			// Stop sending if we hit an error
			break
		}
		
		// Small delay between messages to avoid rate limiting
		if i < len(chunks)-1 {
			// Don't delay on last message
		}
	}
}

// splitMessage splits a message into chunks of maxLength, trying to break at word boundaries
func splitMessage(content string, maxLength int) []string {
	if len(content) <= maxLength {
		return []string{content}
	}
	
	var chunks []string
	current := ""
	lines := strings.Split(content, "\n")
	
	for _, line := range lines {
		// If adding this line would exceed the limit, start a new chunk
		if len(current) > 0 && len(current)+len(line)+1 > maxLength {
			chunks = append(chunks, current)
			current = ""
		}
		
		// If a single line is too long, split it
		if len(line) > maxLength {
			// Save current chunk if any
			if current != "" {
				chunks = append(chunks, current)
				current = ""
			}
			
			// Split the long line
			for len(line) > maxLength {
				chunks = append(chunks, line[:maxLength])
				line = line[maxLength:]
			}
			current = line
		} else {
			if current != "" {
				current += "\n"
			}
			current += line
		}
	}
	
	// Add remaining content
	if current != "" {
		chunks = append(chunks, current)
	}
	
	return chunks
}

// findUserFromMentionsOrUsername finds a user from message mentions or by username
func findUserFromMentionsOrUsername(ctx context.Context, s *discordgo.Session, m *discordgo.MessageCreate, targetUsername string, graphRepo *graph.Repository) (*graph.User, error) {
	// First, try to find in mentions
	for _, mention := range m.Mentions {
		// Skip bot mentions
		if mention.ID == s.State.User.ID {
			continue
		}
		// Check if username matches (case-insensitive)
		if strings.EqualFold(mention.Username, targetUsername) {
			// Get or create user in graph
			user, err := graphRepo.GetOrCreateUser(ctx, mention.ID, mention.ID, mention.Username, "discord")
			if err != nil {
				return nil, err
			}
			return user, nil
		}
	}

	// If not found in mentions, try to find by username in database
	user, err := graphRepo.FindUserByDiscordUsername(ctx, targetUsername)
	if err == nil {
		return user, nil
	}

	return nil, fmt.Errorf("user not found: %s", targetUsername)
}

// getLanguageNameFromCode returns the display name for a language code
func getLanguageNameFromCode(langCode string) string {
	langNames := map[string]string{
		"fr":        "French",
		"en":        "English",
		"es":        "Spanish",
		"de":        "German",
		"it":        "Italian",
		"pt":        "Portuguese",
		"ja":        "Japanese",
		"zh":        "Chinese",
		"ko":        "Korean",
		"ru":        "Russian",
		"pig_latin": "Pig Latin",
	}
	
	if name, ok := langNames[langCode]; ok {
		return name
	}
	return langCode // Return code if name not found
}

// handleLanguagePreferenceInstruction detects and processes language preference instructions
func handleLanguagePreferenceInstruction(ctx context.Context, s *discordgo.Session, m *discordgo.MessageCreate, content string, graphRepo *graph.Repository, log *zap.Logger) {
	// Normalize content for pattern matching
	lowerContent := strings.ToLower(content)

	// Detect language preferences from various patterns
	// Pattern 1: "never forget that @user is from france speak french when he talks to you"
	// Pattern 2: "set lang=fr" for a mentioned user
	// Pattern 3: "speak french" or "respond in french" for a mentioned user
	// Pattern 4: "only speaks X" or "speaks X" for a mentioned user

	// Language detection patterns
	languagePatterns := map[string][]string{
		"fr":        {"france", "speak french", "respond in french", "lang=fr", "language=french", "only speaks french", "only speak french"},
		"en":        {"english", "speak english", "respond in english", "lang=en", "language=english", "preferred language is english", "prefers to speak in english"},
		"pig_latin": {"pig latin", "speaks pig latin", "only speaks pig latin", "only speak pig latin"},
		"es":        {"spanish", "speaks spanish", "only speaks spanish", "only speak spanish", "lang=es"},
		"de":        {"german", "speaks german", "only speaks german", "only speak german", "lang=de"},
		"it":        {"italian", "speaks italian", "only speaks italian", "only speak italian", "lang=it"},
		"pt":        {"portuguese", "speaks portuguese", "only speaks portuguese", "only speak portuguese", "lang=pt"},
		"ja":        {"japanese", "speaks japanese", "only speaks japanese", "only speak japanese", "lang=ja"},
		"zh":        {"chinese", "speaks chinese", "only speaks chinese", "only speak chinese", "lang=zh"},
		"ko":        {"korean", "speaks korean", "only speaks korean", "only speak korean", "lang=ko"},
		"ru":        {"russian", "speaks russian", "only speaks russian", "only speak russian", "lang=ru"},
	}

	// Check for any language indicator
	var detectedLang string
	for langCode, patterns := range languagePatterns {
		for _, pattern := range patterns {
			if strings.Contains(lowerContent, pattern) {
				detectedLang = langCode
				break
			}
		}
		if detectedLang != "" {
			break
		}
	}

	if detectedLang == "" {
		return
	}

	// Extract target user - prioritize mentioned users
	var targetUsername string
	var targetUserID string

	// First, check if there are any user mentions (excluding the bot)
	// If there are mentions, use the first mentioned user as the target
	for _, mention := range m.Mentions {
		if mention.ID != s.State.User.ID {
			// Found a mentioned user - this is our target
			targetUserID = mention.ID
			targetUsername = mention.Username
			break
		}
	}

	// If no mentions found, try to extract username from text patterns
	if targetUserID == "" {
		// Pattern: "never forget that @bash wizard" or "never forget that bash wizard"
		pattern1 := regexp.MustCompile(`(?i)never forget that\s+@?(\w+(?:\s+\w+)?)`)
		matches := pattern1.FindStringSubmatch(content)
		if len(matches) > 1 {
			targetUsername = strings.TrimSpace(matches[1])
		}

		// Pattern: "set lang=XX for @user" or "set lang=XX for user"
		if targetUsername == "" {
			pattern2 := regexp.MustCompile(`(?i)set lang=\w+\s+(?:for|to)\s+@?(\w+(?:\s+\w+)?)`)
			matches = pattern2.FindStringSubmatch(content)
			if len(matches) > 1 {
				targetUsername = strings.TrimSpace(matches[1])
			}
		}

		// Pattern: "@user only speaks X" or "user only speaks X"
		if targetUsername == "" {
			pattern3 := regexp.MustCompile(`(?i)@?(\w+(?:\s+\w+)?)\s+(?:only\s+)?speaks?\s+(?:pig\s+)?latin|french|spanish|german|italian|portuguese|japanese|chinese|korean|russian`)
			matches = pattern3.FindStringSubmatch(content)
			if len(matches) > 1 {
				targetUsername = strings.TrimSpace(matches[1])
			}
		}
	}

	// If we found a target user from mentions or patterns, set the preference for them
	// Otherwise, if no target found, set it for the requester (the person making the request)
	if targetUsername != "" || targetUserID != "" {
		var user *graph.User
		var err error

		if targetUserID != "" {
			// User found from mentions
			user, err = graphRepo.GetOrCreateUser(ctx, targetUserID, targetUserID, targetUsername, "discord")
		} else {
			// Try to find user by username
			user, err = findUserFromMentionsOrUsername(ctx, s, m, targetUsername, graphRepo)
		}

		if err != nil {
			log.Debug("Could not find user for language preference",
				zap.String("username", targetUsername),
				zap.Error(err),
			)
			return
		}

		// Set language preference for the target user
		if err := graphRepo.SetUserLanguagePreference(ctx, user.ID, detectedLang); err != nil {
			log.Error("Failed to set language preference",
				zap.String("user_id", user.ID),
				zap.Error(err),
			)
			return
		}

		// Get language name for fact
		langName := getLanguageNameFromCode(detectedLang)
		
		// Create a fact about the language preference
		agentID := "Ezra"
		factContent := fmt.Sprintf("User prefers to communicate in %s", langName)
		_, err = graphRepo.CreateFact(ctx, agentID, factContent, "language_preference", user.ID, []string{"Language Preferences"})
		if err != nil {
			log.Warn("Failed to create language preference fact",
				zap.String("user_id", user.ID),
				zap.Error(err),
			)
		}

		log.Info("Language preference set",
			zap.String("target_user_id", user.ID),
			zap.String("target_username", user.DiscordUsername),
			zap.String("requester_id", m.Author.ID),
			zap.String("requester_username", m.Author.Username),
			zap.String("language", detectedLang),
		)
	} else {
		// No target user found - set preference for the requester (person making the request)
		requesterUser, err := graphRepo.GetOrCreateUser(ctx, m.Author.ID, m.Author.ID, m.Author.Username, "discord")
		if err != nil {
			log.Debug("Could not get/create requester user",
				zap.String("user_id", m.Author.ID),
				zap.Error(err),
			)
			return
		}

		// Set language preference for requester
		if err := graphRepo.SetUserLanguagePreference(ctx, requesterUser.ID, detectedLang); err != nil {
			log.Error("Failed to set language preference for requester",
				zap.String("user_id", requesterUser.ID),
				zap.Error(err),
			)
			return
		}

		// Get language name for fact
		langName := getLanguageNameFromCode(detectedLang)
		
		// Create a fact about the language preference
		agentID := "Ezra"
		factContent := fmt.Sprintf("User prefers to communicate in %s", langName)
		_, err = graphRepo.CreateFact(ctx, agentID, factContent, "language_preference", requesterUser.ID, []string{"Language Preferences"})
		if err != nil {
			log.Warn("Failed to create language preference fact",
				zap.String("user_id", requesterUser.ID),
				zap.Error(err),
			)
		}

		log.Info("Language preference set for requester",
			zap.String("user_id", requesterUser.ID),
			zap.String("username", requesterUser.DiscordUsername),
			zap.String("language", detectedLang),
		)
	}
}

