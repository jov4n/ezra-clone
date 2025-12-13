package tools

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"time"

	"ezra-clone/backend/internal/tools/music"

	"github.com/bwmarrin/discordgo"
	"go.uber.org/zap"
)

func (m *MusicExecutor) handlePlay(ctx context.Context, execCtx *ExecutionContext, bot *music.MusicBot, args map[string]interface{}) *ToolResult {
	query, _ := args["query"].(string)
	if query == "" {
		return &ToolResult{
			Success: false,
			Error:   "Query is required",
		}
	}

	// Get guild ID (should already be set on bot, but ensure we have it)
	guildID := bot.GuildID
	if guildID == "" && execCtx.ChannelID != "" {
		channel, err := m.session.Channel(execCtx.ChannelID)
		if err == nil && channel != nil {
			guildID = channel.GuildID
			bot.GuildID = guildID
		}
	}

	if guildID == "" {
		return &ToolResult{
			Success: false,
			Error:   "Could not determine guild ID. Please use a guild channel.",
		}
	}

	// Get voice channel ID - match original bot's simple approach with fallback
	channelID, _ := args["channel_id"].(string)
	if channelID == "" {
		m.logger.Debug("Attempting to detect user voice channel",
			zap.String("guild_id", guildID),
			zap.String("user_id", execCtx.UserID),
		)

		// Check if user is in a voice channel - exactly like original bot
		// The original bot just uses State.VoiceState directly - it works because
		// the state cache is populated by voice state update events
		vs, err := m.session.State.VoiceState(guildID, execCtx.UserID)
		if err != nil || vs == nil || vs.ChannelID == "" {
			// If state cache doesn't have it, the voice state might not be tracked yet
			// This can happen if the user joined before the bot started or state isn't synced
			// Try to get guild and check voice states - but note that Guild() API call
			// doesn't return voice states, only the state cache does
			m.logger.Debug("Voice state not in cache, checking if guild state has voice states",
				zap.String("guild_id", guildID),
				zap.String("user_id", execCtx.UserID),
				zap.Error(err),
			)

			// Try to get guild from state cache (which should have voice states if populated)
			guild, err := m.session.State.Guild(guildID)
			if err == nil && guild != nil && len(guild.VoiceStates) > 0 {
				m.logger.Debug("Found voice states in guild state cache",
					zap.Int("voice_state_count", len(guild.VoiceStates)),
				)
				for _, voiceState := range guild.VoiceStates {
					if voiceState.UserID == execCtx.UserID && voiceState.ChannelID != "" {
						channelID = voiceState.ChannelID
						m.logger.Info("Found voice channel from guild state cache",
							zap.String("channel_id", channelID),
							zap.String("user_id", execCtx.UserID),
						)
						break
					}
				}
			}

			if channelID == "" {
				m.logger.Warn("Could not find user voice channel - state cache may not be populated yet",
					zap.String("guild_id", guildID),
					zap.String("user_id", execCtx.UserID),
					zap.Bool("guild_in_cache", err == nil && guild != nil),
					zap.Int("voice_states_in_guild", func() int {
						if guild != nil {
							return len(guild.VoiceStates)
						}
						return 0
					}()),
				)
				return &ToolResult{
					Success: false,
					Error:   "You must be in a voice channel to play music. Please join a voice channel first or specify channel_id.",
				}
			}
		} else {
			channelID = vs.ChannelID
			m.logger.Debug("Found voice channel from session state",
				zap.String("channel_id", channelID),
			)
		}
	}

	// Connect to voice channel if not already connected - match original bot exactly
	if bot.VoiceConn == nil || bot.VoiceConn.ChannelID != channelID {
		// Disconnect existing connection if it exists and is in a different channel
		if bot.VoiceConn != nil && bot.VoiceConn.ChannelID != channelID {
			m.logger.Debug("Disconnecting from old voice channel", zap.String("channel_id", bot.VoiceConn.ChannelID))
			bot.VoiceConn.Disconnect()
			bot.VoiceConn = nil
			// Give it a moment to fully disconnect
			time.Sleep(200 * time.Millisecond)
		}

		vc, err := m.session.ChannelVoiceJoin(guildID, channelID, false, true)
		if err != nil {
			return &ToolResult{
				Success: false,
				Error:   fmt.Sprintf("Failed to join voice channel: %v", err),
			}
		}
		bot.VoiceConn = vc

		// Wait for voice connection to be ready (exactly like temp-music-botting)
		m.logger.Debug("Waiting for voice connection to be ready...")
		timeout := time.After(5 * time.Second)
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		timeoutReached := false
		for !bot.VoiceConn.Ready && !timeoutReached {
			select {
			case <-timeout:
				m.logger.Warn("Voice connection timeout, continuing anyway...")
				timeoutReached = true
			case <-ticker.C:
			}
		}
		m.logger.Info("Voice connection ready!")
	}

	// Fetch song based on query/URL
	var song music.Song
	var err error
	if music.IsYouTubeURL(query) {
		song = music.FetchYouTubeVideo(query, execCtx.UserID)
		if song.Title == "" {
			return &ToolResult{
				Success: false,
				Error:   fmt.Sprintf("Could not fetch YouTube video: %s", query),
			}
		}
	} else if music.IsSpotifyURL(query) {
		songs, fetchErr := music.FetchSpotifyPlaylist(ctx, query, execCtx.UserID, nil)
		if fetchErr != nil {
			return &ToolResult{
				Success: false,
				Error:   fmt.Sprintf("Could not fetch Spotify playlist: %v", fetchErr),
			}
		}
		if len(songs) > 0 {
			song = songs[0]
		}
	} else if music.IsSoundCloudURL(query) {
		songs, fetchErr := music.FetchSoundCloudPlaylist(ctx, query, execCtx.UserID, nil)
		if fetchErr != nil {
			return &ToolResult{
				Success: false,
				Error:   fmt.Sprintf("Could not fetch SoundCloud playlist: %v", fetchErr),
			}
		}
		if len(songs) > 0 {
			song = songs[0]
		}
	} else {
		// Search YouTube
		song = music.SearchYouTube(query, execCtx.UserID)
	}

	if song.Title == "" {
		return &ToolResult{
			Success: false,
			Error:   fmt.Sprintf("Could not find song: %s", query),
		}
	}
	_ = err // Suppress unused variable warning

	// Add to queue
	bot.Playlist.Lock()
	bot.Playlist.Songs = append(bot.Playlist.Songs, song)
	position := len(bot.Playlist.Songs)
	bot.Playlist.Unlock()

	// Start playback if not already playing
	bot.Mu.Lock()
	if !bot.IsPlaying {
		bot.Mu.Unlock()
		// Start playback in goroutine
		go music.PlayQueue(bot, m.session, execCtx.ChannelID)
	} else {
		bot.Mu.Unlock()
	}

	// Send confirmation embed
	go func() {
		embed := music.CreateSongAddedEmbed(song, position)
		_, err := m.session.ChannelMessageSendEmbed(execCtx.ChannelID, embed)
		if err != nil {
			m.logger.Warn("Failed to send song added embed", zap.Error(err))
		}
	}()

	return &ToolResult{
		Success: true,
		Message: fmt.Sprintf("Added to queue: %s (Position #%d)", song.Title, position),
		Data: map[string]interface{}{
			"title":    song.Title,
			"duration": song.Duration,
			"url":      song.URL,
			"position": position,
		},
	}
}

func (m *MusicExecutor) handlePlaylist(ctx context.Context, execCtx *ExecutionContext, bot *music.MusicBot, args map[string]interface{}) *ToolResult {
	query, _ := args["query"].(string)
	if query == "" {
		return &ToolResult{
			Success: false,
			Error:   "Query is required",
		}
	}

	// Get guild ID (should already be set on bot, but ensure we have it)
	guildID := bot.GuildID
	if guildID == "" && execCtx.ChannelID != "" {
		channel, err := m.session.Channel(execCtx.ChannelID)
		if err == nil && channel != nil {
			guildID = channel.GuildID
			bot.GuildID = guildID
		}
	}

	if guildID == "" {
		return &ToolResult{
			Success: false,
			Error:   "Could not determine guild ID. Please use a guild channel.",
		}
	}

	// Get voice channel ID - match original bot's simple approach with fallback
	channelID, _ := args["channel_id"].(string)
	if channelID == "" {
		m.logger.Debug("Attempting to detect user voice channel",
			zap.String("guild_id", guildID),
			zap.String("user_id", execCtx.UserID),
		)

		// Check if user is in a voice channel - exactly like original bot
		// The original bot just uses State.VoiceState directly - it works because
		// the state cache is populated by voice state update events
		vs, err := m.session.State.VoiceState(guildID, execCtx.UserID)
		if err != nil || vs == nil || vs.ChannelID == "" {
			// If state cache doesn't have it, the voice state might not be tracked yet
			// This can happen if the user joined before the bot started or state isn't synced
			// Try to get guild and check voice states - but note that Guild() API call
			// doesn't return voice states, only the state cache does
			m.logger.Debug("Voice state not in cache, checking if guild state has voice states",
				zap.String("guild_id", guildID),
				zap.String("user_id", execCtx.UserID),
				zap.Error(err),
			)

			// Try to get guild from state cache (which should have voice states if populated)
			guild, err := m.session.State.Guild(guildID)
			if err == nil && guild != nil && len(guild.VoiceStates) > 0 {
				m.logger.Debug("Found voice states in guild state cache",
					zap.Int("voice_state_count", len(guild.VoiceStates)),
				)
				for _, voiceState := range guild.VoiceStates {
					if voiceState.UserID == execCtx.UserID && voiceState.ChannelID != "" {
						channelID = voiceState.ChannelID
						m.logger.Info("Found voice channel from guild state cache",
							zap.String("channel_id", channelID),
							zap.String("user_id", execCtx.UserID),
						)
						break
					}
				}
			}

			if channelID == "" {
				m.logger.Warn("Could not find user voice channel - state cache may not be populated yet",
					zap.String("guild_id", guildID),
					zap.String("user_id", execCtx.UserID),
					zap.Bool("guild_in_cache", err == nil && guild != nil),
					zap.Int("voice_states_in_guild", func() int {
						if guild != nil {
							return len(guild.VoiceStates)
						}
						return 0
					}()),
				)
				return &ToolResult{
					Success: false,
					Error:   "You must be in a voice channel to play music. Please join a voice channel first or specify channel_id.",
				}
			}
		} else {
			channelID = vs.ChannelID
			m.logger.Debug("Found voice channel from session state",
				zap.String("channel_id", channelID),
			)
		}
	}

	// Connect to voice channel if not already connected - match original bot exactly
	if bot.VoiceConn == nil || bot.VoiceConn.ChannelID != channelID {
		// Disconnect existing connection if it exists and is in a different channel
		if bot.VoiceConn != nil && bot.VoiceConn.ChannelID != channelID {
			m.logger.Debug("Disconnecting from old voice channel", zap.String("channel_id", bot.VoiceConn.ChannelID))
			bot.VoiceConn.Disconnect()
			bot.VoiceConn = nil
			// Give it a moment to fully disconnect
			time.Sleep(200 * time.Millisecond)
		}

		// Try to join voice channel with retry logic
		var vc *discordgo.VoiceConnection
		var err error
		maxRetries := 2
		for attempt := 0; attempt < maxRetries; attempt++ {
			if attempt > 0 {
				m.logger.Debug("Retrying voice channel join...", zap.Int("attempt", attempt+1))
				time.Sleep(1 * time.Second) // Wait before retry
			}

			vc, err = m.session.ChannelVoiceJoin(guildID, channelID, false, true)
			if err != nil {
				if attempt < maxRetries-1 {
					m.logger.Warn("Failed to join voice channel, will retry", zap.Error(err))
					continue
				}
				return &ToolResult{
					Success: false,
					Error:   fmt.Sprintf("Failed to join voice channel after %d attempts: %v", maxRetries, err),
				}
			}
			bot.VoiceConn = vc
			break
		}

		// Wait for voice connection to be ready (exactly like temp-music-botting)
		m.logger.Debug("Waiting for voice connection to be ready...")
		timeout := time.After(5 * time.Second)
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		timeoutReached := false
		for !bot.VoiceConn.Ready && !timeoutReached {
			select {
			case <-timeout:
				m.logger.Warn("Voice connection timeout, continuing anyway...")
				timeoutReached = true
			case <-ticker.C:
			}
		}
		m.logger.Info("Voice connection ready!")
	}

	// Generate playlist using OpenRouter (if available) - streaming mode
	// This returns immediately, songs are added to queue as they're found
	// GenerateAndPlayPlaylist now handles sending and updating the progress message
	music.GenerateAndPlayPlaylist(query, execCtx.UserID, bot, m.session, execCtx.ChannelID)

	// Give it time for OpenRouter to generate queries and YouTube to find first song
	// OpenRouter API call + YouTube search can take 3-5 seconds
	time.Sleep(5 * time.Second)
	bot.Playlist.Lock()
	songCount := len(bot.Playlist.Songs)
	bot.Playlist.Unlock()

	// If still no songs after waiting, OpenRouter might have failed or YouTube searches are slow
	// But we'll return success anyway since the async process is running
	// The actual error will be handled when trying to play

	return &ToolResult{
		Success: true,
		Message: fmt.Sprintf("Generating playlist based on: %s (songs will start playing as they're found)", query),
		Data: map[string]interface{}{
			"song_count": songCount,
			"query":      query,
		},
	}
}

func (m *MusicExecutor) handleQueue(ctx context.Context, execCtx *ExecutionContext, bot *music.MusicBot, args map[string]interface{}) *ToolResult {
	bot.Playlist.Lock()
	songs := bot.Playlist.Songs
	current := bot.Playlist.Current
	page := bot.QueuePage
	bot.Playlist.Unlock()

	// Send queue embed
	go func() {
		embed := music.CreateQueueEmbed(bot.Playlist, page)
		_, err := m.session.ChannelMessageSendEmbed(execCtx.ChannelID, embed)
		if err != nil {
			m.logger.Warn("Failed to send queue embed", zap.Error(err))
		}
	}()

	queueInfo := make([]map[string]interface{}, 0, len(songs))
	for i, song := range songs {
		queueInfo = append(queueInfo, map[string]interface{}{
			"position": i + 1,
			"title":    song.Title,
			"duration": song.Duration,
			"url":      song.URL,
			"current":  i == current,
		})
	}

	return &ToolResult{
		Success: true,
		Message: fmt.Sprintf("Queue: %d songs (currently playing #%d)", len(songs), current+1),
		Data: map[string]interface{}{
			"queue":   queueInfo,
			"current": current + 1,
			"total":   len(songs),
		},
	}
}

func (m *MusicExecutor) handleSkip(ctx context.Context, execCtx *ExecutionContext, bot *music.MusicBot, args map[string]interface{}) *ToolResult {
	select {
	case bot.SkipChan <- true:
	default:
	}

	return &ToolResult{
		Success: true,
		Message: "Skipped current song",
	}
}

func (m *MusicExecutor) handlePause(ctx context.Context, execCtx *ExecutionContext, bot *music.MusicBot, args map[string]interface{}) *ToolResult {
	select {
	case bot.PauseChan <- true:
	default:
	}

	return &ToolResult{
		Success: true,
		Message: "Paused playback",
	}
}

func (m *MusicExecutor) handleResume(ctx context.Context, execCtx *ExecutionContext, bot *music.MusicBot, args map[string]interface{}) *ToolResult {
	select {
	case bot.ResumeChan <- true:
	default:
	}

	return &ToolResult{
		Success: true,
		Message: "Resumed playback",
	}
}

func (m *MusicExecutor) handleStop(ctx context.Context, execCtx *ExecutionContext, bot *music.MusicBot, args map[string]interface{}) *ToolResult {
	select {
	case bot.StopChan <- true:
	default:
	}

	// Clear queue
	bot.Playlist.Lock()
	bot.Playlist.Songs = []music.Song{}
	bot.Playlist.Current = -1
	bot.Playlist.Unlock()

	return &ToolResult{
		Success: true,
		Message: "Stopped playback and cleared queue",
	}
}

func (m *MusicExecutor) handleVolume(ctx context.Context, execCtx *ExecutionContext, bot *music.MusicBot, args map[string]interface{}) *ToolResult {
	volume, ok := args["volume"].(float64)
	if !ok {
		volInt, ok := args["volume"].(int)
		if !ok {
			return &ToolResult{
				Success: false,
				Error:   "Volume must be a number",
			}
		}
		volume = float64(volInt)
	}

	if volume < 0 || volume > 100 {
		return &ToolResult{
			Success: false,
			Error:   "Volume must be between 0 and 100",
		}
	}

	// Note: Discord voice connections don't support volume control directly
	// This would need to be implemented at the audio processing level
	return &ToolResult{
		Success: true,
		Message: fmt.Sprintf("Volume set to %.0f%% (note: volume control not yet implemented)", volume),
	}
}

func (m *MusicExecutor) handleRadio(ctx context.Context, execCtx *ExecutionContext, bot *music.MusicBot, args map[string]interface{}) *ToolResult {
	action, _ := args["action"].(string)
	if action == "stop" {
		bot.ClearRadioState()
		return &ToolResult{
			Success: true,
			Message: "Radio mode stopped",
		}
	}

	if action != "start" {
		return &ToolResult{
			Success: false,
			Error:   "Action must be 'start' or 'stop'",
		}
	}

	seed, _ := args["seed"].(string)
	if seed == "" {
		return &ToolResult{
			Success: false,
			Error:   "Seed is required when starting radio mode",
		}
	}

	// Start radio mode
	bot.RadioMu.Lock()
	bot.RadioEnabled = true
	bot.RadioSeed = seed
	bot.RadioChannelID = execCtx.ChannelID
	bot.RadioMu.Unlock()

	// Start radio playback
	go music.StartRadioMode(bot, m.session, seed, execCtx.ChannelID)

	return &ToolResult{
		Success: true,
		Message: fmt.Sprintf("Radio mode started with seed: %s", seed),
	}
}

func (m *MusicExecutor) handleDisconnect(ctx context.Context, execCtx *ExecutionContext, bot *music.MusicBot, args map[string]interface{}) *ToolResult {
	// Stop playback
	select {
	case bot.StopChan <- true:
	default:
	}

	// Clear queue
	bot.Playlist.Lock()
	bot.Playlist.Songs = []music.Song{}
	bot.Playlist.Current = -1
	bot.Playlist.Unlock()

	// Clear radio state
	bot.ClearRadioState()

	// Disconnect from voice channel
	if bot.VoiceConn != nil {
		bot.Mu.Lock()
		if bot.IsSpeaking {
			bot.VoiceConn.Speaking(false)
			bot.IsSpeaking = false
		}
		vc := bot.VoiceConn
		bot.VoiceConn = nil
		bot.Mu.Unlock()

		// Clean up preload
		bot.PreloadMu.Lock()
		if bot.Preloaded != nil {
			preloaded := bot.Preloaded
			// Cancel the context to stop the goroutine
			if preloaded.Cancel != nil {
				preloaded.Cancel()
			}
			// Kill the process
			if preloaded.YtdlpCmd != nil {
				if cmd, ok := preloaded.YtdlpCmd.(*exec.Cmd); ok && cmd.Process != nil {
					cmd.Process.Kill()
				}
			}
			// Close the output stream
			if preloaded.OpusOut != nil {
				if closer, ok := preloaded.OpusOut.(io.ReadCloser); ok {
					closer.Close()
				}
			}
			bot.Preloaded = nil
		}
		bot.PreloadMu.Unlock()

		// Disconnect
		if vc != nil {
			vc.Disconnect()
			m.logger.Info("Disconnected from voice channel", zap.String("guild_id", bot.GuildID))
		}
	}

	return &ToolResult{
		Success: true,
		Message: "Disconnected from voice channel",
	}
}

