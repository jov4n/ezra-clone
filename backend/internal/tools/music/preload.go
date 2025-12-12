package music

import (
	"context"
	"io"
	"os/exec"
	"time"
)

// PreloadNextSong preloads the next song for seamless transitions
func PreloadNextSong(bot *MusicBot, song Song) {
	bot.PreloadMu.Lock()

	// Clean up existing preloaded song
	if bot.Preloaded != nil {
		cleanupPreloadedSong(bot.Preloaded)
	}
	bot.Preloaded = nil
	bot.PreloadMu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())

	var ytdlpCmd *exec.Cmd
	var audioOut io.ReadCloser
	var err error

	if song.Source == "twitch" {
		ytdlpCmd, audioOut, err = startTwitchStream(ctx, song.URL)
	} else {
		ytdlpCmd, audioOut, err = startYouTubeStream(ctx, song.URL)
	}

	if err != nil {
		cancel()
		return
	}

	var opusOut io.ReadCloser
	if song.Source == "twitch" {
		opusOut = audioOut
	} else {
		demuxer := NewWebMDemuxer(audioOut)
		opusOut = &readCloserWrapper{Reader: demuxer}
	}

	// Create the preloaded song with a done channel
	preloadedSong := &PreloadedSong{
		Song:        song,
		Preloaded:   true,
		Buffer:      make([]byte, 0, 1024*1024), // 1MB initial capacity
		BufferPos:   0,
		StreamReady: make(chan bool, 1),
		Cancel:      cancel,
		YtdlpCmd:    ytdlpCmd,
		OpusOut:     opusOut,
		Reading:     true, // Start as true since goroutine starts immediately
		ReadErr:     nil,
		Done:        make(chan struct{}),
	}

	// Store preloaded song atomically
	bot.PreloadMu.Lock()
	// Check if another preload started while we were setting up
	if bot.Preloaded != nil {
		// Another preload won, clean up ours
		bot.PreloadMu.Unlock()
		cancel()
		if ytdlpCmd.Process != nil {
			ytdlpCmd.Process.Kill()
		}
		opusOut.Close()
		return
	}
	bot.Preloaded = preloadedSong
	bot.PreloadMu.Unlock()

	// Start reading from pipe in background
	go func() {
		defer close(preloadedSong.Done)

		buf := make([]byte, 8192)
		totalRead := 0
		signaled := false

		for {
			// Check if context is cancelled
			select {
			case <-ctx.Done():
				preloadedSong.Mu.Lock()
				preloadedSong.Reading = false
				preloadedSong.ReadErr = ctx.Err()
				preloadedSong.Mu.Unlock()
				return
			default:
			}

			n, err := opusOut.Read(buf)
			if n > 0 {
				preloadedSong.Mu.Lock()
				preloadedSong.Buffer = append(preloadedSong.Buffer, buf[:n]...)
				totalRead += n
				preloadedSong.Mu.Unlock()

				if !signaled && totalRead >= 65536 {
					select {
					case preloadedSong.StreamReady <- true:
						signaled = true
					default:
					}
				}
			}
			if err != nil {
				preloadedSong.Mu.Lock()
				preloadedSong.Reading = false
				preloadedSong.ReadErr = err
				preloadedSong.Mu.Unlock()

				// Signal ready even on EOF (we have whatever we buffered)
				if !signaled {
					select {
					case preloadedSong.StreamReady <- true:
					default:
					}
				}

				if err != io.EOF {
					// Non-EOF error during preload, just silently abort
				}
				return
			}
		}
	}()
}

// cleanupPreloadedSong safely cleans up a preloaded song
func cleanupPreloadedSong(p *PreloadedSong) {
	if p == nil {
		return
	}

	// Cancel context first to signal goroutine to stop
	if p.Cancel != nil {
		p.Cancel()
	}

	// Kill the process
	if p.YtdlpCmd != nil {
		if cmd, ok := p.YtdlpCmd.(*exec.Cmd); ok && cmd.Process != nil {
			cmd.Process.Kill()
		}
	}

	// Close the output stream
	if p.OpusOut != nil {
		if closer, ok := p.OpusOut.(io.ReadCloser); ok {
			closer.Close()
		}
	}

	if p.Done != nil {
		select {
		case <-p.Done:
		case <-time.After(100 * time.Millisecond):
		}
	}
}
