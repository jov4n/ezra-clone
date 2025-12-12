package music

import (
	"context"
	"fmt"
	"io"
	"log"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
)

// readCloserWrapper wraps an io.Reader to implement io.ReadCloser
type readCloserWrapper struct {
	io.Reader
}

func (r *readCloserWrapper) Close() error {
	return nil
}

// bufferedPipeReader reads from buffer first, then from pipe
type bufferedPipeReader struct {
	buffer     []byte
	bufferPos  int
	pipe       io.ReadCloser
	pipeActive bool
	mu         sync.Mutex
}

func (r *bufferedPipeReader) Read(p []byte) (n int, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// First, read from buffer
	if r.bufferPos < len(r.buffer) {
		n = copy(p, r.buffer[r.bufferPos:])
		r.bufferPos += n
		return n, nil
	}

	// Buffer exhausted - read from pipe if active
	if r.pipeActive && r.pipe != nil {
		return r.pipe.Read(p)
	}

	// No more data
	return 0, io.EOF
}

func (r *bufferedPipeReader) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.pipe != nil {
		return r.pipe.Close()
	}
	return nil
}

// SeekError is returned when a seek is requested during playback
type SeekError struct {
	Position time.Duration
}

func (e *SeekError) Error() string {
	return fmt.Sprintf("seek requested to %v", e.Position)
}

// PlayQueue plays the queue of songs
func PlayQueue(bot *MusicBot, session *discordgo.Session, channelID string) {
	bot.Mu.Lock()
	if bot.IsPlaying {
		bot.Mu.Unlock()
		return
	}
	bot.IsPlaying = true
	bot.Mu.Unlock()

	defer func() {
		bot.Mu.Lock()
		bot.IsPlaying = false
		bot.Mu.Unlock()
	}()

	for {
		bot.Playlist.Lock()
		if bot.Playlist.Current >= len(bot.Playlist.Songs)-1 {
			if !bot.Playlist.Loop {
				// Check if radio mode is enabled before breaking
				bot.RadioMu.Lock()
				radioEnabled := bot.RadioEnabled
				bot.RadioMu.Unlock()

				if radioEnabled {
					// Trigger refill and wait for new songs
					bot.Playlist.Unlock()
					refillRadioQueue(bot, session)

					// Check if we got new songs
					bot.Playlist.Lock()
					if bot.Playlist.Current >= len(bot.Playlist.Songs)-1 {
						// Still no songs, break
						bot.Playlist.Unlock()
						break
					}
					bot.Playlist.Unlock()
					continue
				}

				bot.Playlist.Unlock()
				break
			}
			bot.Playlist.Current = -1
		}

		bot.Playlist.Current++
		if bot.Playlist.Current >= len(bot.Playlist.Songs) {
			bot.Playlist.Unlock()
			break
		}

		song := bot.Playlist.Songs[bot.Playlist.Current]

		// Add song to radio history if radio mode is enabled
		bot.RadioMu.Lock()
		if bot.RadioEnabled {
			bot.AddToRadioHistory(song.URL)
		}
		bot.RadioMu.Unlock()

		// Check if there's a next song to preload
		nextIndex := bot.Playlist.Current + 1
		if nextIndex < len(bot.Playlist.Songs) {
			nextSong := bot.Playlist.Songs[nextIndex]
			go PreloadNextSong(bot, nextSong)
		}

		// Check if we need to refill radio queue (when 2 or fewer songs remaining)
		bot.RadioMu.Lock()
		radioEnabled := bot.RadioEnabled
		refilling := bot.RadioRefilling
		bot.RadioMu.Unlock()

		remainingSongs := len(bot.Playlist.Songs) - bot.Playlist.Current - 1
		if radioEnabled && !refilling && remainingSongs <= 2 {
			go refillRadioQueue(bot, session)
		}

		bot.Playlist.Unlock()

		// Check voice connection before playing (like temp-music-botting - just check if nil, don't wait for Ready)
		if bot.VoiceConn == nil {
			return
		}

		// Play song
		err := PlaySong(bot, song)
		if err != nil {
			if strings.Contains(err.Error(), "stream closed") {
				bot.PreloadMu.Lock()
				bot.Preloaded = nil
				bot.PreloadMu.Unlock()
				err = PlaySong(bot, song)
			}
			if err != nil {
				log.Printf("Error playing song (retry failed): %v", err)
				if bot.VoiceConn != nil {
					bot.Mu.Lock()
					bot.VoiceConn.Speaking(false)
					bot.IsSpeaking = false
					bot.Mu.Unlock()
				}
				continue
			}
		}

		// Check for skip
		select {
		case <-bot.SkipChan:
			continue
		case <-bot.StopChan:
			if bot.VoiceConn != nil {
				bot.Mu.Lock()
				bot.VoiceConn.Speaking(false)
				bot.IsSpeaking = false
				bot.Mu.Unlock()
			}
			return
		default:
		}
	}

	// Set speaking to false when queue finishes
	if bot.VoiceConn != nil {
		bot.Mu.Lock()
		bot.VoiceConn.Speaking(false)
		bot.IsSpeaking = false
		bot.Mu.Unlock()
	}

	// Check if radio mode is enabled - if so, don't show "queue finished" message
	bot.RadioMu.Lock()
	radioEnabled := bot.RadioEnabled
	bot.RadioMu.Unlock()

	if !radioEnabled {
	}
}

// PlaySong plays a single song
func PlaySong(bot *MusicBot, song Song) error {
	return PlaySongWithSeek(bot, song, 0)
}

// PlaySongWithSeek plays a single song starting from a specific position
func PlaySongWithSeek(bot *MusicBot, song Song, seekSeconds int) error {
	if bot.VoiceConn == nil {
		return fmt.Errorf("not connected to voice channel")
	}
	vc := bot.VoiceConn

	if seekSeconds > 0 {
		log.Printf("⏩ Seeking to: %d seconds (will skip frames in demuxer)", seekSeconds)
	}

	// Check if this song is preloaded (only use preload if not seeking)
	var opusOut io.ReadCloser
	var ytdlpCmd *exec.Cmd
	var cancel func()
	usePreloaded := false

	if seekSeconds == 0 {
		bot.PreloadMu.Lock()
		preloaded := bot.Preloaded

		if preloaded != nil && preloaded.Song.URL == song.URL && preloaded.Preloaded {
			// Wait for preload to be ready (up to 3 seconds)
			streamReady := preloaded.StreamReady
			bot.PreloadMu.Unlock()

			select {
			case <-streamReady:
			case <-time.After(3 * time.Second):
			}

			// Re-acquire lock and verify preload is still valid
			bot.PreloadMu.Lock()
			preloaded = bot.Preloaded
			if preloaded != nil && preloaded.Song.URL == song.URL && preloaded.Preloaded && preloaded.OpusOut != nil {
				preloaded.Mu.Lock()
				hasBuffer := len(preloaded.Buffer) > 0
				reading := preloaded.Reading
				readErr := preloaded.ReadErr
				preloaded.Mu.Unlock()

				// Use preload if we have buffer data
				if hasBuffer && (readErr == nil || readErr == io.EOF) {
					usePreloaded = true
					log.Printf("Using preloaded stream (buffer: %d bytes, reading: %v)", len(preloaded.Buffer), reading)

					// Take ownership of the preloaded data
					preloaded.Mu.Lock()
					buffer := make([]byte, len(preloaded.Buffer))
					copy(buffer, preloaded.Buffer)
					bufferPos := preloaded.BufferPos
					pipeReader := preloaded.OpusOut.(io.ReadCloser)
					ytdlpCmd = preloaded.YtdlpCmd.(*exec.Cmd)
					stillReading := preloaded.Reading
					preloaded.Mu.Unlock()

					// Create buffered reader that reads from buffer first, then pipe
					opusOut = &bufferedPipeReader{
						buffer:     buffer,
						bufferPos:  bufferPos,
						pipe:       pipeReader,
						pipeActive: stillReading,
					}

					// Clear the preload (we've taken ownership)
					bot.Preloaded = nil
				}
			}
			bot.PreloadMu.Unlock()
		} else {
			bot.PreloadMu.Unlock()
		}
	}

	if !usePreloaded {
		// Start fresh download
		log.Printf("Starting fresh stream (preload not available)")
		ctx, cancelFunc := context.WithCancel(context.Background())
		cancel = cancelFunc

		var audioOut io.ReadCloser
		var err error

		if song.Source == "twitch" {
			ytdlpCmd, audioOut, err = startTwitchStream(ctx, song.URL)
			if err != nil {
				cancel()
				return err
			}
			// Twitch streams: ffmpeg outputs OGG Opus directly
			opusOut = audioOut
		} else {
			ytdlpCmd, audioOut, err = startYouTubeStream(ctx, song.URL)
			if err != nil {
				cancel()
				return err
			}
			// Use WebM demuxer with seek support
			var demuxer *WebMDemuxer
			if seekSeconds > 0 {
				demuxer = NewWebMDemuxerWithSeek(audioOut, seekSeconds)
			} else {
				demuxer = NewWebMDemuxer(audioOut)
			}
			opusOut = &readCloserWrapper{Reader: demuxer}
		}
	}

	// Note: temp-music-botting doesn't check Ready here - it just tries to use the connection
	// If the websocket failed, it will fail when we try to send audio, and we'll handle that error

	// Set speaking - exactly like temp-music-botting (no Ready check before Speaking)
	bot.Mu.Lock()
	if !bot.IsSpeaking {
		vc.Speaking(true)
		bot.IsSpeaking = true
	}
	// Set initial position for seek
	if seekSeconds > 0 {
		bot.SongStartTime = time.Now().Add(-time.Duration(seekSeconds) * time.Second)
	} else {
		bot.SongStartTime = time.Now()
	}
	bot.Mu.Unlock()

	// Play the audio stream
	err := playAudioStream(bot, vc, opusOut, usePreloaded, ytdlpCmd, cancel)

	// Handle seek request
	if seekErr, ok := err.(*SeekError); ok {
		seekSecs := int(seekErr.Position.Seconds())
		return PlaySongWithSeek(bot, song, seekSecs)
	}

	if cancel != nil {
		cancel()
	}

	return err
}

func startYouTubeStream(ctx context.Context, url string) (*exec.Cmd, io.ReadCloser, error) {
	args := []string{
		"-f", "251/250/bestaudio[ext=webm]/bestaudio/best",
		"-o", "-",
		"--no-playlist",
		url,
	}

	cmd := exec.CommandContext(ctx, YtdlpExecutable, args...)

	audioOut, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, err
	}

	log.Printf("Starting yt-dlp (preferring opus format in WebM)...")
	if err := cmd.Start(); err != nil {
		return nil, nil, err
	}

	return cmd, audioOut, nil
}

func startTwitchStream(ctx context.Context, url string) (*exec.Cmd, io.ReadCloser, error) {
	// Check if ffmpeg is available
	if FfmpegExecutable == "" {
		return nil, nil, fmt.Errorf("ffmpeg not found - required for Twitch streams")
	}

	// Start yt-dlp with live stream options
	ytdlpCmd := exec.CommandContext(ctx, YtdlpExecutable,
		"-o", "-",
		"--no-playlist",
		"-f", "bestaudio/best",
		"--no-live-from-start",
		"--no-part",
		"--no-cache-dir",
		url)

	ytdlpOut, err := ytdlpCmd.StdoutPipe()
	if err != nil {
		return nil, nil, err
	}

	ytdlpCmd.Stderr = io.Discard

	if err := ytdlpCmd.Start(); err != nil {
		return nil, nil, err
	}
	log.Printf("Started yt-dlp process (PID: %d)", ytdlpCmd.Process.Pid)

	// Start ffmpeg - output OGG Opus directly
	ffmpegCmd := exec.CommandContext(ctx, FfmpegExecutable,
		"-hide_banner",
		"-loglevel", "warning",
		"-i", "pipe:0",
		"-vn",
		"-c:a", "libopus",
		"-b:a", "128k",
		"-ar", "48000",
		"-ac", "2",
		"-application", "audio",
		"-frame_duration", "20",
		"-f", "ogg",
		"pipe:1")

	ffmpegCmd.Stdin = ytdlpOut
	ffmpegOut, err := ffmpegCmd.StdoutPipe()
	if err != nil {
		ytdlpCmd.Process.Kill()
		return nil, nil, err
	}

	ffmpegCmd.Stderr = io.Discard

	if err := ffmpegCmd.Start(); err != nil {
		ytdlpCmd.Process.Kill()
		return nil, nil, err
	}
	log.Printf("Started ffmpeg process (PID: %d)", ffmpegCmd.Process.Pid)

	// Clean up yt-dlp when ffmpeg exits
	go func() {
		ffmpegCmd.Wait()
		if ytdlpCmd.Process != nil {
			ytdlpCmd.Process.Kill()
		}
	}()

	return ffmpegCmd, ffmpegOut, nil
}

func playAudioStream(bot *MusicBot, vc *discordgo.VoiceConnection, opusOut io.ReadCloser, usePreloaded bool, ytdlpCmd *exec.Cmd, cancel func()) error {
	// Note: temp-music-botting doesn't check Ready here - it just tries to use the connection
	// If the websocket failed, errors will occur when trying to send data, and we'll handle them

	frameCount := 0
	oggHeader := make([]byte, 27)

	// Track song start time for position tracking
	bot.Mu.Lock()
	if bot.SongStartTime.IsZero() {
		bot.SongStartTime = time.Now()
	}
	bot.CurrentPos = 0
	bot.IsPaused = false
	bot.Mu.Unlock()

	// Helper to cleanup stream resources
	cleanupStream := func() {
		if !usePreloaded && cancel != nil {
			cancel()
			if ytdlpCmd != nil && ytdlpCmd.Process != nil {
				ytdlpCmd.Process.Kill()
			}
		} else {
			if opusOut != nil {
				opusOut.Close()
			}
		}
	}

	// Note: temp-music-botting doesn't check Ready here - it just tries to use the connection
	// If the websocket failed, errors will occur when trying to send data, and we'll handle them

	for {
		// Check for pause state
		bot.Mu.Lock()
		isPaused := bot.IsPaused
		bot.Mu.Unlock()

		if isPaused {
			// Wait for resume or other control signals
			select {
			case <-bot.ResumeChan:
				bot.Mu.Lock()
				bot.IsPaused = false
				bot.SongStartTime = time.Now().Add(-bot.PausedAt)
				vc.Speaking(true)
				bot.IsSpeaking = true
				bot.Mu.Unlock()
				continue
			case <-bot.SkipChan:
				cleanupStream()
				return nil
			case <-bot.StopChan:
				cleanupStream()
				return nil
			case seekPos := <-bot.SeekChan:
				cleanupStream()
				return &SeekError{Position: seekPos}
			case <-time.After(100 * time.Millisecond):
				continue
			}
		}

		select {
		case <-bot.SkipChan:
			cleanupStream()
			return nil
		case <-bot.StopChan:
			bot.PreloadMu.Lock()
			if bot.Preloaded != nil {
				cleanupPreloadedSong(bot.Preloaded)
				bot.Preloaded = nil
			}
			bot.PreloadMu.Unlock()
			cleanupStream()
			return nil
		case <-bot.PauseChan:
			bot.Mu.Lock()
			bot.IsPaused = true
			bot.PausedAt = time.Since(bot.SongStartTime)
			vc.Speaking(false)
			bot.IsSpeaking = false
			bot.Mu.Unlock()
			continue
		case seekPos := <-bot.SeekChan:
			cleanupStream()
			return &SeekError{Position: seekPos}
		default:
			// Read OGG page header
			_, err := io.ReadFull(opusOut, oggHeader)
			if err != nil {
				if err == io.EOF || err == io.ErrUnexpectedEOF {
					if !usePreloaded {
						if ytdlpCmd != nil {
							ytdlpCmd.Wait()
						}
						if cancel != nil {
							cancel()
						}
					} else {
						if opusOut != nil {
							opusOut.Close()
						}
						if ytdlpCmd != nil {
							ytdlpCmd.Wait()
						}
					}
					return nil
				}
				if strings.Contains(err.Error(), "file already closed") || strings.Contains(err.Error(), "use of closed") || strings.Contains(err.Error(), "stream closed") {
					log.Printf("Stream was closed (likely due to skip or premature closure)")
					return fmt.Errorf("stream closed, will retry")
				}
				return err
			}

			// Check OGG magic number
			if string(oggHeader[0:4]) != "OggS" {
				return fmt.Errorf("invalid OGG header")
			}

			// Read segment table
			segCount := int(oggHeader[26])
			if segCount == 0 {
				continue
			}

			segTable := make([]byte, segCount)
			_, err = io.ReadFull(opusOut, segTable)
			if err != nil {
				return err
			}

			// Read segments and send opus packets
			currentPacket := make([]byte, 0, 4000)
			for i := 0; i < segCount; i++ {
				segLen := int(segTable[i])
				if segLen > 0 {
					segData := make([]byte, segLen)
					n, err := io.ReadFull(opusOut, segData)
					if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
						return err
					}
					if n > 0 {
						currentPacket = append(currentPacket, segData[:n]...)
					}

					if segLen < 255 && len(currentPacket) > 0 {
						frameCount++
						packetData := make([]byte, len(currentPacket))
						copy(packetData, currentPacket)

						// Update position every 50 frames (~1 second at 20ms per frame)
						if frameCount%50 == 0 {
							bot.Mu.Lock()
							bot.CurrentPos = time.Since(bot.SongStartTime)
							bot.Mu.Unlock()
						}

						// Send to voice connection - blocks until channel is ready (like old bot)
						// No default case - we want to block and wait for the channel
						select {
						case vc.OpusSend <- packetData:
							// Successfully sent packet
						case <-bot.SkipChan:
							return nil
						case <-bot.StopChan:
							return nil
						case <-bot.PauseChan:
							bot.Mu.Lock()
							bot.IsPaused = true
							bot.PausedAt = time.Since(bot.SongStartTime)
							if vc != nil {
								vc.Speaking(false)
							}
							bot.IsSpeaking = false
							bot.Mu.Unlock()
						case seekPos := <-bot.SeekChan:
							cleanupStream()
							return &SeekError{Position: seekPos}
						}

						currentPacket = currentPacket[:0]
					}
				}
			}

			// Send remaining packet if any
			if len(currentPacket) > 0 {
				frameCount++
				packetData := make([]byte, len(currentPacket))
				copy(packetData, currentPacket)

				select {
				case vc.OpusSend <- packetData:
					// Successfully sent packet
				case <-bot.SkipChan:
					return nil
				case <-bot.StopChan:
					return nil
				}
			}
		}
	}
}

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

	// Generate new song suggestions using OpenRouter
	// Import sources for GenerateRadioSuggestions
	suggestions := GenerateRadioSuggestions(seed, recentSongs)

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
		log.Printf("Failed to update generating playlist message: %v", err)
	}
}

// GenerateAndPlayPlaylist generates a playlist and starts playback (streaming mode)
func GenerateAndPlayPlaylist(query, requester string, bot *MusicBot, session *discordgo.Session, channelID string) []Song {
	log.Printf("Generating playlist for query: %s (streaming mode)", query)

	// Generate playlist queries first to know expected count
	queries := GeneratePlaylistQueries(query)
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
		log.Printf("Failed to send generating playlist message: %v", err)
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
				log.Printf("Failed to update final playlist message: %v", err)
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
	queries := GeneratePlaylistQueries(seed)
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
