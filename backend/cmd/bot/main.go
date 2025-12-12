package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"ezra-clone/backend/internal/adapter"
	"ezra-clone/backend/internal/agent"
	"ezra-clone/backend/internal/constants"
	"ezra-clone/backend/internal/graph"
	"ezra-clone/backend/internal/tools"
	"ezra-clone/backend/internal/utils"
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
	
	// Initialize ComfyUI executor (always initialize for prompt enhancement, RunPod optional for image generation)
	comfyExecutor := tools.NewComfyExecutor(llmAdapter, cfg)
	agentOrch.SetComfyExecutor(comfyExecutor)
	if cfg.RunPodAPIKey != "" && cfg.RunPodEndpointID != "" {
		log.Info("ComfyUI executor initialized with RunPod", zap.String("endpoint_id", cfg.RunPodEndpointID))
	} else {
		log.Info("ComfyUI executor initialized (prompt enhancement only, RunPod not configured)")
	}

	// Initialize Music executor
	musicExecutor := tools.NewMusicExecutor(dg, log, cfg.OpenRouterAPIKey)
	agentOrch.SetMusicExecutor(musicExecutor)
	log.Info("Music executor initialized")

	// Add message handler
	dg.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		handleMessage(s, m, agentOrch, graphRepo, log)
	})

	// Set intents (including voice state for music bot)
	// Required intents:
	// - IntentsGuilds: Access to guild information
	// - IntentsGuildMessages: Read messages in guild channels
	// - IntentsDirectMessages: Read DM messages
	// - IntentsGuildVoiceStates: Track voice state changes (REQUIRED for voice connections)
	dg.Identify.Intents = discordgo.IntentsGuilds | discordgo.IntentsGuildMessages | discordgo.IntentsDirectMessages | discordgo.IntentsGuildVoiceStates
	
	// Log intents for debugging
	log.Info("Discord bot intents configured",
		zap.Bool("guilds", (dg.Identify.Intents&discordgo.IntentsGuilds) != 0),
		zap.Bool("guild_messages", (dg.Identify.Intents&discordgo.IntentsGuildMessages) != 0),
		zap.Bool("direct_messages", (dg.Identify.Intents&discordgo.IntentsDirectMessages) != 0),
		zap.Bool("guild_voice_states", (dg.Identify.Intents&discordgo.IntentsGuildVoiceStates) != 0),
	)

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

	ctx := context.Background()

	// Ensure message author exists in database before processing
	_, err := graphRepo.GetOrCreateUser(ctx, m.Author.ID, m.Author.ID, m.Author.Username, "discord")
	if err != nil {
		log.Error("Failed to get/create user",
			zap.String("user_id", m.Author.ID),
			zap.Error(err),
		)
		// Continue anyway - user creation failure shouldn't block message processing
	}

	// Create users for any mentioned users (even if they haven't talked yet)
	for _, mention := range m.Mentions {
		// Skip bot mentions
		if mention.ID == s.State.User.ID {
			continue
		}
		
		// Create the mentioned user if they don't exist
		_, err := graphRepo.GetOrCreateUser(ctx, mention.ID, mention.ID, mention.Username, "discord")
		if err != nil {
			log.Warn("Failed to get/create mentioned user",
				zap.String("user_id", mention.ID),
				zap.String("username", mention.Username),
				zap.Error(err),
			)
			// Continue - don't block on mentioned user creation failure
		} else {
			log.Debug("Created/updated mentioned user",
				zap.String("user_id", mention.ID),
				zap.String("username", mention.Username),
			)
		}
	}

	// Also try to resolve text-based username mentions (e.g., "@bash wizard" in text)
	// Extract username patterns from message content
	usernamePattern := regexp.MustCompile(`@(\w+(?:\s+\w+)?)`)
	matches := usernamePattern.FindAllStringSubmatch(content, -1)
	for _, match := range matches {
		if len(match) > 1 {
			username := strings.TrimSpace(match[1])
			// Skip if it's a Discord ID mention (numeric)
			if matched, _ := regexp.MatchString(`^\d+$`, username); matched {
				continue
			}
			
			// Try to find this user in the guild/server
			if m.GuildID != "" {
				members, err := s.GuildMembersSearch(m.GuildID, username, 5)
				if err == nil && len(members) > 0 {
					// Found user(s) - create the first match
					for _, member := range members {
						if member.User != nil && !member.User.Bot {
							_, err := graphRepo.GetOrCreateUser(ctx, member.User.ID, member.User.ID, member.User.Username, "discord")
							if err != nil {
								log.Warn("Failed to get/create text-mentioned user",
									zap.String("user_id", member.User.ID),
									zap.String("username", member.User.Username),
									zap.String("searched_username", username),
									zap.Error(err),
								)
							} else {
								log.Debug("Created/updated text-mentioned user",
									zap.String("user_id", member.User.ID),
									zap.String("username", member.User.Username),
									zap.String("searched_username", username),
								)
							}
							break // Only create the first match
						}
					}
				}
			}
		}
	}

	// Check for language preference instructions before processing
	languagePreferenceSet, targetUserForLang := handleLanguagePreferenceInstruction(ctx, s, m, content, graphRepo, log)

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

		// If language preference was set but LLM failed, at least acknowledge that
		if languagePreferenceSet {
			_, _ = s.ChannelMessageSend(m.ChannelID, "✅ Language preference has been set! However, I encountered an error generating a response. Please check your API configuration.")
			return
		}

		// Optionally notify user of error
		_, _ = s.ChannelMessageSend(m.ChannelID, "Sorry, I encountered an error processing your message.")
		return
	}

	// Prepare message content
	messageContent := result.Content
	if len(messageContent) > constants.DiscordMaxMessageLength {
		messageContent = messageContent[:constants.DiscordMaxMessageLength-3] + "..."
	}

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

	// Prepare file attachment if image data is present
	var files []*discordgo.File
	var imageEmbed *discordgo.MessageEmbed
	if len(result.ImageData) > 0 {
		imageName := result.ImageName
		if imageName == "" {
			imageName = "image.png"
		}
		
		// Create a file attachment
		files = append(files, &discordgo.File{
			Name:   imageName,
			Reader: bytes.NewReader(result.ImageData),
		})
		
		// Create a nice embed for the image
		imageEmbed = &discordgo.MessageEmbed{
			Title:       "🎨 Generated Image",
			Description: messageContent,
			Color:       0x5865F2, // Discord blurple color
			Image: &discordgo.MessageEmbedImage{
				URL: fmt.Sprintf("attachment://%s", imageName), // Reference the attached file
			},
			Footer: &discordgo.MessageEmbedFooter{
				Text: "ComfyUI via RunPod",
			},
			Timestamp: time.Now().Format(time.RFC3339),
		}
		
		// Add metadata fields if available
		if result.ImageMeta != nil {
			var fields []*discordgo.MessageEmbedField
			
			if width, ok := result.ImageMeta["width"]; ok {
				if height, ok := result.ImageMeta["height"]; ok {
					fields = append(fields, &discordgo.MessageEmbedField{
						Name:   "Dimensions",
						Value:  fmt.Sprintf("%v × %v", width, height),
						Inline: true,
					})
				}
			}
			
			if seed, ok := result.ImageMeta["seed"]; ok {
				fields = append(fields, &discordgo.MessageEmbedField{
					Name:   "Seed",
					Value:  fmt.Sprintf("%v", seed),
					Inline: true,
				})
			}
			
			if elapsed, ok := result.ImageMeta["elapsed_seconds"]; ok {
				if elapsedFloat, ok := elapsed.(float64); ok {
					fields = append(fields, &discordgo.MessageEmbedField{
						Name:   "Generation Time",
						Value:  fmt.Sprintf("%.1fs", elapsedFloat),
						Inline: true,
					})
				}
			}
			
			if len(fields) > 0 {
				imageEmbed.Fields = fields
			}
		}
		
		// Add the image embed to the embeds list
		discordEmbeds = append(discordEmbeds, imageEmbed)
		
		log.Debug("Attaching image to Discord message",
			zap.String("filename", imageName),
			zap.Int("size_bytes", len(result.ImageData)),
		)
	}

	// Send message with embeds and/or file attachment
	if len(discordEmbeds) > 0 || len(files) > 0 {
		// If we have an image embed, don't send content separately (it's in the embed)
		sendContent := messageContent
		if imageEmbed != nil {
			sendContent = "" // Image embed already has the description
		}
		
		_, err := s.ChannelMessageSendComplex(m.ChannelID, &discordgo.MessageSend{
			Content: sendContent,
			Embeds:  discordEmbeds,
			Files:   files,
		})
		if err != nil {
			log.Error("Failed to send message with embeds/files",
				zap.Error(err),
				zap.String("channel_id", m.ChannelID),
			)
		}
	} else if messageContent != "" {
		// Plain text message - split if too long
		sendLongMessage(s, m.ChannelID, messageContent, log)
	}
}

// sendLongMessage splits a message into chunks if it exceeds Discord's character limit
func sendLongMessage(s *discordgo.Session, channelID, content string, log *zap.Logger) {
	maxLength := constants.DiscordMaxMessageLength
	const chunkPrefix = "```\n"
	const chunkSuffix = "\n```"
	maxChunkLength := maxLength - len(chunkPrefix) - len(chunkSuffix)
	
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


// handleLanguagePreferenceInstruction detects and processes language preference instructions
// Returns (success bool, targetUsername string) - targetUsername is empty if set for requester
func handleLanguagePreferenceInstruction(ctx context.Context, s *discordgo.Session, m *discordgo.MessageCreate, content string, graphRepo *graph.Repository, log *zap.Logger) (bool, string) {
	// Normalize content for pattern matching
	_ = strings.ToLower(content) // Reserved for future pattern matching

	// Detect language preferences from various patterns
	// Pattern 1: "never forget that @user is from france speak french when he talks to you"
	// Pattern 2: "set lang=fr" for a mentioned user
	// Pattern 3: "speak french" or "respond in french" for a mentioned user
	// Pattern 4: "only speaks X" or "speaks X" for a mentioned user

	// Check for any language indicator
	detectedLang := utils.ExtractLanguageFromMessage(content)

	if detectedLang == "" {
		return false, ""
	}

	log.Debug("Language preference detected",
		zap.String("language", detectedLang),
		zap.String("content", content),
	)

	// Extract target user - prioritize mentioned users
	var targetUsername string
	var targetUserID string

	// First, check if there are any user mentions (excluding the bot)
	// If there are mentions, use the first mentioned user as the target
	log.Debug("Checking mentions",
		zap.Int("mention_count", len(m.Mentions)),
		zap.String("bot_id", s.State.User.ID),
	)
	
	for _, mention := range m.Mentions {
		log.Debug("Processing mention",
			zap.String("mention_id", mention.ID),
			zap.String("mention_username", mention.Username),
			zap.Bool("is_bot", mention.ID == s.State.User.ID),
		)
		if mention.ID != s.State.User.ID {
			// Found a mentioned user - this is our target
			targetUserID = mention.ID
			targetUsername = mention.Username
			log.Info("Found target user from mentions",
				zap.String("user_id", targetUserID),
				zap.String("username", targetUsername),
			)
			break
		}
	}

	// If no mentions found, try to extract username from text patterns
	if targetUserID == "" {
		// Pattern: "set language for @user to X" or "set language for user to X"
		pattern1 := regexp.MustCompile(`(?i)set\s+language\s+for\s+@?(\w+(?:\s+\w+)?)\s+to`)
		matches := pattern1.FindStringSubmatch(content)
		if len(matches) > 1 {
			targetUsername = strings.TrimSpace(matches[1])
		}

		// Pattern: "set @user language to X" or "set user language to X"
		if targetUsername == "" {
			pattern2 := regexp.MustCompile(`(?i)set\s+@?(\w+(?:\s+\w+)?)\s+language\s+to`)
			matches = pattern2.FindStringSubmatch(content)
			if len(matches) > 1 {
				targetUsername = strings.TrimSpace(matches[1])
			}
		}

		// Pattern: "set language to X for @user" or "set language to X for user"
		if targetUsername == "" {
			pattern3 := regexp.MustCompile(`(?i)set\s+language\s+to\s+\w+\s+(?:for|to)\s+@?(\w+(?:\s+\w+)?)`)
			matches = pattern3.FindStringSubmatch(content)
			if len(matches) > 1 {
				targetUsername = strings.TrimSpace(matches[1])
			}
		}

		// Pattern: "never forget that @bash wizard" or "never forget that bash wizard"
		if targetUsername == "" {
			pattern3 := regexp.MustCompile(`(?i)never forget that\s+@?(\w+(?:\s+\w+)?)`)
			matches = pattern3.FindStringSubmatch(content)
			if len(matches) > 1 {
				targetUsername = strings.TrimSpace(matches[1])
			}
		}

		// Pattern: "set lang=XX for @user" or "set lang=XX for user"
		if targetUsername == "" {
			pattern4 := regexp.MustCompile(`(?i)set lang=\w+\s+(?:for|to)\s+@?(\w+(?:\s+\w+)?)`)
			matches = pattern4.FindStringSubmatch(content)
			if len(matches) > 1 {
				targetUsername = strings.TrimSpace(matches[1])
			}
		}

		// Pattern: "@user only speaks X" or "user only speaks X"
		if targetUsername == "" {
			pattern5 := regexp.MustCompile(`(?i)@?(\w+(?:\s+\w+)?)\s+(?:only\s+)?speaks?\s+(?:pig\s+)?latin|french|spanish|german|italian|portuguese|japanese|chinese|korean|russian`)
			matches = pattern5.FindStringSubmatch(content)
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
			// User found from mentions - use the mention ID directly
			user, err = graphRepo.GetOrCreateUser(ctx, targetUserID, targetUserID, targetUsername, "discord")
			if err != nil {
				log.Error("Failed to get/create mentioned user",
					zap.String("user_id", targetUserID),
					zap.String("username", targetUsername),
					zap.Error(err),
				)
				return false, ""
			}
		} else if targetUsername != "" {
			// Try to find user by username from text patterns
			// First check if username matches any mention in the message
			for _, mention := range m.Mentions {
				if mention.ID != s.State.User.ID && strings.EqualFold(mention.Username, targetUsername) {
					// Found matching mention - use it
					user, err = graphRepo.GetOrCreateUser(ctx, mention.ID, mention.ID, mention.Username, "discord")
					if err != nil {
						log.Error("Failed to get/create matched mention user",
							zap.String("user_id", mention.ID),
							zap.String("username", mention.Username),
							zap.Error(err),
						)
						return false, ""
					}
					targetUserID = mention.ID // Update for logging
					break
				}
			}
			
			// If not found in mentions, try to find by username in database or search guild
			if user == nil {
				user, err = findUserFromMentionsOrUsername(ctx, s, m, targetUsername, graphRepo)
				if err != nil {
			log.Warn("Could not find user for language preference",
				zap.String("username", targetUsername),
				zap.String("user_id", targetUserID),
				zap.Error(err),
			)
			return false, ""
		}
			}
		}

		// Set the preference for the target user
		if user == nil {
			// We had a target but couldn't find the user - don't set for requester
			log.Warn("Target user specified but not found",
				zap.String("target_username", targetUsername),
				zap.String("target_user_id", targetUserID),
			)
			return false, ""
		}

		log.Info("Found target user for language preference",
			zap.String("user_id", user.ID),
			zap.String("username", user.DiscordUsername),
			zap.String("target_username", targetUsername),
		)

		// Set language preference for the target user
		if err := graphRepo.SetUserLanguagePreference(ctx, user.ID, detectedLang); err != nil {
			log.Error("Failed to set language preference",
				zap.String("user_id", user.ID),
				zap.Error(err),
			)
			return false, ""
		}

		// Get language name for fact
		langName := utils.GetLanguageName(detectedLang)
		
		// Create a fact about the language preference
		agentID := constants.DefaultAgentID
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
		// Return the target username for confirmation message
		return true, user.DiscordUsername
	} else {
		// No target user found - set preference for the requester (person making the request)
		requesterUser, err := graphRepo.GetOrCreateUser(ctx, m.Author.ID, m.Author.ID, m.Author.Username, "discord")
		if err != nil {
			log.Debug("Could not get/create requester user",
				zap.String("user_id", m.Author.ID),
				zap.Error(err),
			)
			return false, ""
		}

		// Set language preference for requester
		if err := graphRepo.SetUserLanguagePreference(ctx, requesterUser.ID, detectedLang); err != nil {
			log.Error("Failed to set language preference for requester",
				zap.String("user_id", requesterUser.ID),
				zap.Error(err),
			)
			return false, ""
		}

		// Get language name for fact
		langName := utils.GetLanguageName(detectedLang)
		
		// Create a fact about the language preference
		agentID := constants.DefaultAgentID
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
		// Return empty string for targetUserForLang since it was set for requester
		return true, ""
	}
	
	return false, ""
}

