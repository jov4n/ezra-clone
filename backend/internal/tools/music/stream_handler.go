package music

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"go.uber.org/zap"
)

func startYouTubeStream(ctx context.Context, url string, logger *zap.Logger) (*exec.Cmd, io.ReadCloser, error) {
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

	logger.Debug("Starting yt-dlp (preferring opus format in WebM)")
	if err := cmd.Start(); err != nil {
		return nil, nil, err
	}

	return cmd, audioOut, nil
}

func startTwitchStream(ctx context.Context, url string, logger *zap.Logger) (*exec.Cmd, io.ReadCloser, error) {
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
	logger.Debug("Started yt-dlp process", zap.Int("pid", ytdlpCmd.Process.Pid))

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
	logger.Debug("Started ffmpeg process", zap.Int("pid", ffmpegCmd.Process.Pid))

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
					bot.logger.Debug("Stream was closed (likely due to skip or premature closure)")
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

