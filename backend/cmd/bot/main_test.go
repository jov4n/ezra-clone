package main

import (
	"testing"

	"github.com/bwmarrin/discordgo"
	"github.com/stretchr/testify/assert"
)

func TestMessageFiltering(t *testing.T) {
	botUserID := "bot-123"
	otherUserID := "user-456"

	tests := []struct {
		name        string
		message     *discordgo.MessageCreate
		shouldReact bool
	}{
		{
			name: "Bot's own message - should ignore",
			message: &discordgo.MessageCreate{
				Message: &discordgo.Message{
					Author: &discordgo.User{ID: botUserID},
					Content: "Hello",
				},
			},
			shouldReact: false,
		},
		{
			name: "DM message - should react",
			message: &discordgo.MessageCreate{
				Message: &discordgo.Message{
					Author:  &discordgo.User{ID: otherUserID},
					Content: "Hello",
					GuildID: "", // DM
				},
			},
			shouldReact: true,
		},
		{
			name: "Mentioned message - should react",
			message: &discordgo.MessageCreate{
				Message: &discordgo.Message{
					Author:  &discordgo.User{ID: otherUserID},
					Content: "<@bot-123> Hello",
					GuildID: "guild-123",
					Mentions: []*discordgo.User{
						{ID: botUserID},
					},
				},
			},
			shouldReact: true,
		},
		{
			name: "Regular message without mention - should ignore",
			message: &discordgo.MessageCreate{
				Message: &discordgo.Message{
					Author:  &discordgo.User{ID: otherUserID},
					Content: "Hello",
					GuildID: "guild-123",
					Mentions: []*discordgo.User{},
				},
			},
			shouldReact: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the filtering logic
			isDM := tt.message.GuildID == ""
			isMentioned := false
			
			for _, mention := range tt.message.Mentions {
				if mention.ID == botUserID {
					isMentioned = true
					break
				}
			}

			shouldReact := isDM || isMentioned
			assert.Equal(t, tt.shouldReact, shouldReact, "Message filtering logic failed")
		})
	}
}

