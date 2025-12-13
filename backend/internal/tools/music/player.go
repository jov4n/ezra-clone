package music

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"go.uber.org/zap"
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
				bot.logger.Error("Error playing song (retry failed)", zap.Error(err))
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
		bot.logger.Info("Seeking to position", zap.Int("seconds", seekSeconds))
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

			streamReadyReceived := false
			select {
			case <-streamReady:
				streamReadyReceived = true
			case <-time.After(3 * time.Second):
			}

			// Re-acquire lock and verify preload is still valid
			bot.PreloadMu.Lock()
			preloaded = bot.Preloaded
			if preloaded != nil && preloaded.Song.URL == song.URL && preloaded.Preloaded && preloaded.OpusOut != nil {
				preloaded.Mu.Lock()
				bufferSize := len(preloaded.Buffer)
				reading := preloaded.Reading
				readErr := preloaded.ReadErr
				preloaded.Mu.Unlock()

				// Use preload if:
				// 1. Stream is ready (signaled) and still reading, OR
				// 2. We have buffer data and no errors (or just EOF)
				// This allows using preload even with small buffers if the stream is active
				canUsePreload := false
				if streamReadyReceived && reading && readErr == nil {
					// Stream is ready and actively reading - use it even with small buffer
					canUsePreload = true
					bot.logger.Debug("Using preloaded stream (stream ready)", zap.Int("buffer_bytes", bufferSize), zap.Bool("reading", reading))
				} else if bufferSize > 0 && (readErr == nil || readErr == io.EOF) {
					// We have buffered data and no errors
					canUsePreload = true
					bot.logger.Debug("Using preloaded stream", zap.Int("buffer_bytes", bufferSize), zap.Bool("reading", reading), zap.Error(readErr))
				} else {
					// Log why we're not using the preload
					bot.logger.Debug("Preload available but not usable", 
						zap.Int("buffer_bytes", bufferSize), 
						zap.Bool("reading", reading), 
						zap.Error(readErr), 
						zap.Bool("stream_ready", streamReadyReceived))
				}

				if canUsePreload {
					usePreloaded = true

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
			} else {
				// Preload doesn't match or was cleared
				if preloaded == nil {
					bot.logger.Debug("Preload was cleared before use")
				} else if preloaded.Song.URL != song.URL {
					bot.logger.Debug("Preload URL mismatch", zap.String("expected", song.URL), zap.String("got", preloaded.Song.URL))
				} else if !preloaded.Preloaded {
					bot.logger.Debug("Preload marked as not preloaded")
				} else if preloaded.OpusOut == nil {
					bot.logger.Debug("Preload has nil OpusOut")
				}
			}
			bot.PreloadMu.Unlock()
		} else {
			if preloaded == nil {
				bot.logger.Debug("No preload available for song", zap.String("url", song.URL))
			} else if preloaded.Song.URL != song.URL {
				bot.logger.Debug("Preload URL mismatch", zap.String("expected", song.URL), zap.String("got", preloaded.Song.URL))
			} else if !preloaded.Preloaded {
				bot.logger.Debug("Preload exists but not marked as preloaded")
			}
			bot.PreloadMu.Unlock()
		}
	}

	if !usePreloaded {
		// Start fresh download
		bot.logger.Debug("Starting fresh stream (preload not available)")
		ctx, cancelFunc := context.WithCancel(context.Background())
		cancel = cancelFunc

		var audioOut io.ReadCloser
		var err error

		if song.Source == "twitch" {
			ytdlpCmd, audioOut, err = startTwitchStream(ctx, song.URL, bot.logger)
			if err != nil {
				cancel()
				return err
			}
			// Twitch streams: ffmpeg outputs OGG Opus directly
			opusOut = audioOut
		} else {
			ytdlpCmd, audioOut, err = startYouTubeStream(ctx, song.URL, bot.logger)
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

