package discord

import (
	"context"
	"regexp"
	"strings"

	"github.com/bwmarrin/discordgo"
	"go.uber.org/zap"
)

// createMentionedUsers creates user records for any users mentioned in the message
func (h *Handler) createMentionedUsers(ctx context.Context, s *discordgo.Session, m *discordgo.MessageCreate) {
	// Create users for any mentioned users (even if they haven't talked yet)
	for _, mention := range m.Mentions {
		// Skip bot mentions
		if mention.ID == s.State.User.ID {
			continue
		}

		// Create the mentioned user if they don't exist
		_, err := h.graphRepo.GetOrCreateUser(ctx, mention.ID, mention.ID, mention.Username, "discord")
		if err != nil {
			h.logger.Warn("Failed to get/create mentioned user",
				zap.String("user_id", mention.ID),
				zap.String("username", mention.Username),
				zap.Error(err),
			)
			// Continue - don't block on mentioned user creation failure
		} else {
			h.logger.Debug("Created/updated mentioned user",
				zap.String("user_id", mention.ID),
				zap.String("username", mention.Username),
			)
		}
	}

	// Also try to resolve text-based username mentions (e.g., "@bash wizard" in text)
	// Extract username patterns from message content
	usernamePattern := regexp.MustCompile(`@(\w+(?:\s+\w+)?)`)
	matches := usernamePattern.FindAllStringSubmatch(m.Content, -1)
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
							_, err := h.graphRepo.GetOrCreateUser(ctx, member.User.ID, member.User.ID, member.User.Username, "discord")
							if err != nil {
								h.logger.Warn("Failed to get/create text-mentioned user",
									zap.String("user_id", member.User.ID),
									zap.String("username", member.User.Username),
									zap.String("searched_username", username),
									zap.Error(err),
								)
							} else {
								h.logger.Debug("Created/updated text-mentioned user",
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
}

