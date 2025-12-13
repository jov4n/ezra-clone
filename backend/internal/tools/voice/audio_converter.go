package voice

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"sync"
)

// AudioConverter handles conversion between Discord Opus and PCM formats
type AudioConverter struct {
	ffmpegPath string
	mu         sync.Mutex
}

// NewAudioConverter creates a new audio converter
func NewAudioConverter(ffmpegPath string) *AudioConverter {
	return &AudioConverter{
		ffmpegPath: ffmpegPath,
	}
}

// OpusToPCM converts Opus audio (from Discord) to PCM 16-bit, 48kHz mono
// Input: OGG Opus stream
// Output: PCM 16-bit, 48kHz, mono
func (ac *AudioConverter) OpusToPCM(ctx context.Context, opusData io.Reader) (io.ReadCloser, error) {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	// Use ffmpeg to convert Opus to PCM
	// Discord sends Opus in OGG container, 48kHz, stereo
	// We need PCM 16-bit, 48kHz, mono for faster-whisper
	cmd := exec.CommandContext(ctx, ac.ffmpegPath,
		"-f", "ogg",           // Input format: OGG
		"-i", "pipe:0",        // Read from stdin
		"-f", "s16le",         // Output format: PCM 16-bit little-endian
		"-ar", "48000",        // Sample rate: 48kHz
		"-ac", "1",            // Channels: mono
		"-acodec", "pcm_s16le", // Audio codec
		"pipe:1",              // Write to stdout
	)

	cmd.Stdin = opusData
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	cmd.Stderr = io.Discard // Suppress ffmpeg output

	if err := cmd.Start(); err != nil {
		stdout.Close()
		return nil, fmt.Errorf("failed to start ffmpeg: %w", err)
	}

	// Return a ReadCloser that closes the command when done
	return &cmdReadCloser{
		Reader: stdout,
		cmd:    cmd,
	}, nil
}

// PCMToOpus converts PCM audio to Opus (for sending to Discord)
// Input: PCM 16-bit, 48kHz, mono
// Output: OGG Opus, 48kHz, stereo (Discord format)
func (ac *AudioConverter) PCMToOpus(ctx context.Context, pcmData io.Reader) (io.ReadCloser, error) {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	// Use ffmpeg to convert PCM to Opus
	// Input: PCM 16-bit, 48kHz, mono
	// Output: OGG Opus, 48kHz, stereo (Discord format)
	cmd := exec.CommandContext(ctx, ac.ffmpegPath,
		"-f", "s16le",         // Input format: PCM 16-bit little-endian
		"-ar", "48000",        // Sample rate: 48kHz
		"-ac", "1",            // Input channels: mono
		"-i", "pipe:0",        // Read from stdin
		"-f", "ogg",           // Output format: OGG
		"-c:a", "libopus",     // Audio codec: Opus
		"-ar", "48000",        // Sample rate: 48kHz
		"-ac", "2",            // Output channels: stereo
		"-b:a", "128k",        // Bitrate: 128kbps
		"-application", "audio", // Opus application type
		"-frame_duration", "20", // Frame duration: 20ms
		"pipe:1",              // Write to stdout
	)

	cmd.Stdin = pcmData
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	cmd.Stderr = io.Discard // Suppress ffmpeg output

	if err := cmd.Start(); err != nil {
		stdout.Close()
		return nil, fmt.Errorf("failed to start ffmpeg: %w", err)
	}

	// Return a ReadCloser that closes the command when done
	return &cmdReadCloser{
		Reader: stdout,
		cmd:    cmd,
	}, nil
}

// ConvertAudioFile converts an audio file to WAV format suitable for XTTS V2
// Output: WAV, 24kHz, mono, 16-bit PCM
func (ac *AudioConverter) ConvertAudioFile(ctx context.Context, inputPath, outputPath string) error {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	cmd := exec.CommandContext(ctx, ac.ffmpegPath,
		"-i", inputPath,        // Input file
		"-ar", "24000",         // Sample rate: 24kHz (XTTS V2 requirement)
		"-ac", "1",             // Channels: mono
		"-c:a", "pcm_s16le",    // Audio codec: PCM 16-bit
		"-y",                   // Overwrite output file
		outputPath,             // Output file
	)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg conversion failed: %w, stderr: %s", err, stderr.String())
	}

	return nil
}

// cmdReadCloser wraps a Reader and ensures the command is cleaned up
type cmdReadCloser struct {
	io.Reader
	cmd *exec.Cmd
}

func (c *cmdReadCloser) Close() error {
	if c.cmd.Process != nil {
		c.cmd.Process.Kill()
	}
	return c.cmd.Wait()
}

