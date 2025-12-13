package music

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"go.uber.org/zap"
)

// refillRadioQueue generates and adds new songs to the queue when radio mode is enabled
func refillRadioQueue(bot *MusicBot, session *discordgo.Session) {
	// Prevent concurrent refills
	bot.RadioMu.Lock()
	if bot.RadioRefilling || !bot.RadioEnabled {
		bot.RadioMu.Unlock()
		return
	}
	bot.RadioRefilling = true
	seed := bot.RadioSeed
	bot.RadioMu.Unlock()

	defer func() {
		bot.RadioMu.Lock()
		bot.RadioRefilling = false
		bot.RadioMu.Unlock()
	}()

	// Get recent songs for context
	recentSongs := bot.GetRecentRadioSongs(5)

	// Generate new song suggestions using LiteLLM adapter
	if bot.llmAdapter == nil {
		return
	}
	ctx := context.Background()
	suggestions := GenerateRadioSuggestions(ctx, bot.llmAdapter, seed, recentSongs)

	if len(suggestions) == 0 {
		return
	}

	// Search YouTube for each suggestion
	addedCount := 0
	maxToAdd := 6
	maxDurationSeconds := 420

	for _, query := range suggestions {
		if addedCount >= maxToAdd {
			break
		}

		song := SearchYouTube(query, "Radio")
		if song.Title == "" || bot.IsInRadioHistory(song.URL) {
			continue
		}

		if !IsSongDurationUnderLimit(song.Duration, maxDurationSeconds) {
			continue
		}

		bot.Playlist.Lock()
		bot.Playlist.Songs = append(bot.Playlist.Songs, song)
		bot.Playlist.Unlock()
		bot.AddToRadioHistory(song.URL)
		addedCount++
	}

	if addedCount > 0 {
	}
}

// updateGeneratingPlaylistMessage updates the generating playlist progress message
func updateGeneratingPlaylistMessage(bot *MusicBot, session *discordgo.Session, foundCount, totalCount int, query string) {
	bot.GeneratingPlaylistMu.Lock()
	msgID := bot.GeneratingPlaylistMsgID
	channelID := bot.GeneratingPlaylistChannelID
	bot.GeneratingPlaylistMu.Unlock()

	if msgID == "" || channelID == "" {
		return
	}

	// Create progress embed
	embed := &discordgo.MessageEmbed{
		Title:       "🎵 Generating Playlist",
		Description: fmt.Sprintf("Generating and queuing songs based on: *%s*\n\n**Found %d/%d songs...**", query, foundCount, totalCount),
		Color:       0x9b59b6, // Purple
		Footer: &discordgo.MessageEmbedFooter{
			Text: "Songs will start playing as they're found",
		},
		Timestamp: time.Now().Format(time.RFC3339),
	}

	// Update the message
	_, err := session.ChannelMessageEditEmbed(channelID, msgID, embed)
	if err != nil {
		// Note: This function doesn't have direct access to bot logger
		// We could pass it as parameter, but for now we'll skip logging here
		// as it's a non-critical operation
		_ = err
	}
}

// GenerateAndPlayPlaylist generates a playlist and starts playback (streaming mode)
func GenerateAndPlayPlaylist(query, requester string, bot *MusicBot, session *discordgo.Session, channelID string) []Song {
	bot.logger.Info("Generating playlist", zap.String("query", query), zap.String("mode", "streaming"))

	// Generate playlist queries first to know expected count
	ctx := context.Background()
	var queries []string
	if bot.llmAdapter != nil {
		queries = GeneratePlaylistQueries(ctx, bot.llmAdapter, query)
	}
	expectedCount := len(queries)
	if expectedCount == 0 {
		// Fallback: will search directly, expect 1 song
		expectedCount = 1
	}

	// Send initial "generating playlist" message
	initialEmbed := &discordgo.MessageEmbed{
		Title:       "🎵 Generating Playlist",
		Description: fmt.Sprintf("Generating and queuing songs based on: *%s*\n\n**Found 0/%d songs...**", query, expectedCount),
		Color:       0x9b59b6, // Purple
		Footer: &discordgo.MessageEmbedFooter{
			Text: "Songs will start playing as they're found",
		},
		Timestamp: time.Now().Format(time.RFC3339),
	}

	msg, err := session.ChannelMessageSendEmbed(channelID, initialEmbed)
	if err != nil {
		bot.logger.Warn("Failed to send generating playlist message", zap.Error(err))
	} else {
		// Store message ID and channel ID
		bot.GeneratingPlaylistMu.Lock()
		bot.GeneratingPlaylistMsgID = msg.ID
		bot.GeneratingPlaylistChannelID = channelID
		bot.GeneratingPlaylistMu.Unlock()
	}

	// Create channel for streaming songs
	songChan := make(chan Song, 20)
	startedPlaying := false
	foundCount := 0
	var foundCountMu sync.Mutex

	// Generate playlist queries using OpenRouter in background
	go func() {
		defer close(songChan)

		if len(queries) == 0 {
			// Fallback: search YouTube directly
			song := SearchYouTube(query, requester)
			if song.Title != "" {
				foundCountMu.Lock()
				foundCount++
				foundCountMu.Unlock()
				updateGeneratingPlaylistMessage(bot, session, foundCount, expectedCount, query)
				select {
				case songChan <- song:
				default:
				}
			}
			return
		}

		// Search YouTube for each query and stream results
		maxDurationSeconds := 360 // 6 minutes max
		for _, queryStr := range queries {
			song := SearchYouTube(queryStr, requester)
			if song.Title != "" && IsSongDurationUnderLimit(song.Duration, maxDurationSeconds) {
				foundCountMu.Lock()
				foundCount++
				currentFound := foundCount
				foundCountMu.Unlock()

				// Update progress message
				updateGeneratingPlaylistMessage(bot, session, currentFound, expectedCount, query)

				select {
				case songChan <- song:
				default:
				}
			}
		}
	}()

	// Add songs to queue as they come in and start playback immediately
	go func() {
		for song := range songChan {
			bot.Playlist.Lock()
			wasEmpty := len(bot.Playlist.Songs) == 0
			bot.Playlist.Songs = append(bot.Playlist.Songs, song)
			bot.Playlist.Unlock()

			// Start playback if queue was empty and we haven't started yet
			bot.Mu.Lock()
			isCurrentlyPlaying := bot.IsPlaying
			bot.Mu.Unlock()

			if wasEmpty && !startedPlaying && !isCurrentlyPlaying {
				startedPlaying = true
				go PlayQueue(bot, session, channelID)
			}
		}

		// If channel closed and we haven't started, start now
		bot.Playlist.Lock()
		hasSongs := len(bot.Playlist.Songs) > 0
		bot.Playlist.Unlock()

		bot.Mu.Lock()
		isCurrentlyPlaying := bot.IsPlaying
		bot.Mu.Unlock()

		if hasSongs && !startedPlaying && !isCurrentlyPlaying {
			go PlayQueue(bot, session, channelID)
		}

		// Final update to show completion
		foundCountMu.Lock()
		finalCount := foundCount
		foundCountMu.Unlock()

		// Update message one final time with completion status
		bot.GeneratingPlaylistMu.Lock()
		msgID := bot.GeneratingPlaylistMsgID
		bot.GeneratingPlaylistMu.Unlock()

		if msgID != "" {
			finalEmbed := &discordgo.MessageEmbed{
				Title:       "🎵 Playlist Generated",
				Description: fmt.Sprintf("Generated and queued **%d songs** based on: *%s*\n\nSongs are now playing! 🎶", finalCount, query),
				Color:       0x2ecc71, // Green
				Footer: &discordgo.MessageEmbedFooter{
					Text: "Enjoy your playlist!",
				},
				Timestamp: time.Now().Format(time.RFC3339),
			}
			_, err := session.ChannelMessageEditEmbed(channelID, msgID, finalEmbed)
			if err != nil {
				// Note: This is in a goroutine, we don't have direct access to bot logger
				// We could pass it, but for now we'll skip logging as it's non-critical
				_ = err
			}
		}
	}()

	// Return empty slice initially, songs are added via channel
	return []Song{}
}

// StartRadioMode starts infinite radio mode
func StartRadioMode(bot *MusicBot, session *discordgo.Session, seed, channelID string) {
	bot.RadioMu.Lock()
	bot.RadioEnabled = true
	bot.RadioSeed = seed
	bot.RadioChannelID = channelID
	bot.RadioMu.Unlock()

	// Generate initial playlist
	ctx := context.Background()
	var queries []string
	if bot.llmAdapter != nil {
		queries = GeneratePlaylistQueries(ctx, bot.llmAdapter, seed)
	}
	maxDurationSeconds := 420

	for _, queryStr := range queries {
		song := SearchYouTube(queryStr, "Radio")
		if song.Title != "" && IsSongDurationUnderLimit(song.Duration, maxDurationSeconds) {
			if !bot.IsInRadioHistory(song.URL) {
				bot.Playlist.Lock()
				bot.Playlist.Songs = append(bot.Playlist.Songs, song)
				bot.Playlist.Unlock()
				bot.AddToRadioHistory(song.URL)
			}
		}
	}

	// Start playback if not already playing
	bot.Mu.Lock()
	if !bot.IsPlaying {
		bot.Mu.Unlock()
		go PlayQueue(bot, session, channelID)
	} else {
		bot.Mu.Unlock()
	}
}

