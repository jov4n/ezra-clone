package discord

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/bwmarrin/discordgo"
	"ezra-clone/backend/internal/constants"
	"ezra-clone/backend/internal/graph"
	"ezra-clone/backend/internal/utils"
	"go.uber.org/zap"
)

// HandleLanguagePreferenceInstruction detects and processes language preference instructions
// Returns (success bool, targetUsername string) - targetUsername is empty if set for requester
func HandleLanguagePreferenceInstruction(ctx context.Context, s *discordgo.Session, m *discordgo.MessageCreate, content string, graphRepo *graph.Repository, log *zap.Logger) (bool, string) {
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

