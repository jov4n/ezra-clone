package tools

import (
	"context"
	"fmt"
	"sort"
	"strings"

	apperrors "ezra-clone/backend/pkg/errors"

	"github.com/bwmarrin/discordgo"
	"go.uber.org/zap"
)

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

