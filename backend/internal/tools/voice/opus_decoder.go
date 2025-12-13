package voice

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"os/exec"
	"time"
)

// OpusDecoder decodes Opus audio to PCM
type OpusDecoder struct {
	ffmpegPath string
}

// NewOpusDecoder creates a new Opus decoder
func NewOpusDecoder(ffmpegPath string) *OpusDecoder {
	return &OpusDecoder{
		ffmpegPath: ffmpegPath,
	}
}

// DecodeOpusToPCM decodes Opus audio data to PCM 16-bit, 48kHz, mono
// Note: For better performance, consider using a native Opus library like github.com/hajimehoshi/oto
// For now, we use ffmpeg which works but has higher latency
// DecodeOpusToPCM decodes Opus audio data to PCM 16-bit, 48kHz, mono
func (od *OpusDecoder) DecodeOpusToPCM(ctx context.Context, opusData []byte) ([]int16, error) {
	if len(opusData) == 0 {
		return nil, fmt.Errorf("empty opus data")
	}

	// Use ffmpeg to decode Opus to PCM
	// We must wrap the raw Opus frames in an Ogg container for ffmpeg to accept them via pipe
	oggBytes := buildOggContainer(opusData)

	cmd := exec.CommandContext(ctx, od.ffmpegPath,
		"-f", "ogg", // Input format: Ogg
		"-i", "pipe:0",
		"-f", "s16le", // Output format
		"-ar", "48000",
		"-ac", "1",
		"pipe:1",
	)

	cmd.Stdin = bytes.NewReader(oggBytes)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	// Capture stderr for debugging
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start ffmpeg: %w", err)
	}

	// Read all PCM data
	pcmData, err := io.ReadAll(stdout)
	if err != nil {
		cmd.Process.Kill()
		return nil, fmt.Errorf("failed to read PCM data: %w", err)
	}

	if err := cmd.Wait(); err != nil {
		// ffmpeg often exits with error when piping raw streams if it can't find header
		// But if we got data, we might be okay.
		if len(pcmData) == 0 {
			return nil, fmt.Errorf("ffmpeg decode failed: %v, stderr: %s", err, stderr.String())
		}
	}

	// Convert bytes to int16 samples (little-endian)
	samples := make([]int16, len(pcmData)/2)
	for i := 0; i < len(samples); i++ {
		samples[i] = int16(binary.LittleEndian.Uint16(pcmData[i*2 : i*2+2]))
	}

	return samples, nil
}

// DecodeOpusStream decodes a stream of Opus packets to PCM
// This is more efficient for continuous streams
func (od *OpusDecoder) DecodeOpusStream(ctx context.Context, opusPackets <-chan []byte) (<-chan []int16, error) {
	pcmChan := make(chan []int16, 50)

	go func() {
		defer close(pcmChan)

		// Buffer multiple Opus packets for better efficiency
		buffer := make([]byte, 0, 4096)
		ticker := time.NewTicker(20 * time.Millisecond) // 20ms frames
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case packet, ok := <-opusPackets:
				if !ok {
					// Decode remaining buffer
					if len(buffer) > 0 {
						samples, err := od.DecodeOpusToPCM(ctx, buffer)
						if err == nil {
							select {
							case pcmChan <- samples:
							case <-ctx.Done():
								return
							}
						}
					}
					return
				}
				buffer = append(buffer, packet...)
			case <-ticker.C:
				// Decode buffered packets every 20ms
				if len(buffer) > 0 {
					samples, err := od.DecodeOpusToPCM(ctx, buffer)
					if err == nil {
						select {
						case pcmChan <- samples:
						case <-ctx.Done():
							return
						}
					}
					buffer = buffer[:0] // Clear buffer
				}
			}
		}
	}()

	return pcmChan, nil
}
