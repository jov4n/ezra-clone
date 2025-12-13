package voice

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"go.uber.org/zap"
)

// AudioCapture handles capturing audio from Discord voice connections
type AudioCapture struct {
	voiceConn    *discordgo.VoiceConnection
	vad          *VoiceActivityDetector
	converter    *AudioConverter
	opusDecoder  *OpusDecoder
	rtpReceiver  *RTPReceiver
	logger       *zap.Logger
	ctx          context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup
	mu           sync.Mutex
	isCapturing  bool
	userStreams  map[string]*UserStream
	ssrcToUserID map[uint32]string // SSRC -> UserID mapping
	ssrcMu       sync.RWMutex
}

// UserStream represents an audio stream for a single user
type UserStream struct {
	UserID    string
	PCMChan   chan []int16 // Channel for PCM audio frames
	LastFrame time.Time
	mu        sync.Mutex
}

// NewAudioCapture creates a new audio capture instance
func NewAudioCapture(voiceConn *discordgo.VoiceConnection, vad *VoiceActivityDetector, converter *AudioConverter, opusDecoder *OpusDecoder, logger *zap.Logger) *AudioCapture {
	ctx, cancel := context.WithCancel(context.Background())
	return &AudioCapture{
		voiceConn:    voiceConn,
		vad:          vad,
		converter:    converter,
		opusDecoder:  opusDecoder,
		logger:       logger,
		ctx:          ctx,
		cancel:       cancel,
		userStreams:  make(map[string]*UserStream),
		ssrcToUserID: make(map[uint32]string),
	}
}

// Start begins capturing audio from the voice connection
func (ac *AudioCapture) Start() error {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	if ac.isCapturing {
		return fmt.Errorf("audio capture already started")
	}

	ac.isCapturing = true
	ac.wg.Add(1)

	go ac.captureLoop()

	ac.logger.Info("Started audio capture")
	return nil
}

// Stop stops capturing audio
func (ac *AudioCapture) Stop() {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	if !ac.isCapturing {
		return
	}

	ac.cancel()
	ac.wg.Wait()
	ac.isCapturing = false

	ac.logger.Info("Stopped audio capture")
}

// GetUserStream returns the audio stream for a user
func (ac *AudioCapture) GetUserStream(userID string) *UserStream {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	stream, exists := ac.userStreams[userID]
	if !exists {
		stream = &UserStream{
			UserID:  userID,
			PCMChan: make(chan []int16, 100), // Buffered channel
		}
		ac.userStreams[userID] = stream
	}

	return stream
}

// GetUserStreams returns all user streams
func (ac *AudioCapture) GetUserStreams() map[string]*UserStream {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	result := make(map[string]*UserStream)
	for k, v := range ac.userStreams {
		result[k] = v
	}
	return result
}

// SetSSRCMapping sets the mapping from SSRC to user ID
func (ac *AudioCapture) SetSSRCMapping(ssrc uint32, userID string) {
	ac.ssrcMu.Lock()
	defer ac.ssrcMu.Unlock()
	ac.ssrcToUserID[ssrc] = userID

	// Also set in RTP receiver if available
	if ac.rtpReceiver != nil {
		ac.rtpReceiver.SetSSRCMapping(ssrc, userID)
	}

	// Ensure stream exists for this user
	ac.mu.Lock()
	if _, exists := ac.userStreams[userID]; !exists {
		ac.userStreams[userID] = &UserStream{
			UserID:  userID,
			PCMChan: make(chan []int16, 100),
		}
	}
	ac.mu.Unlock()

	ac.logger.Debug("Set SSRC mapping", zap.Uint32("ssrc", ssrc), zap.String("user_id", userID))
}

// captureLoop listens for PCM audio from the Node.js bridge via UDP :4000
func (ac *AudioCapture) captureLoop() {
	defer ac.wg.Done()

	addr, err := net.ResolveUDPAddr("udp", ":4000")
	if err != nil {
		ac.logger.Error("Failed to resolve UDP address", zap.Error(err))
		return
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		ac.logger.Error("Failed to start UDP bridge listener", zap.Error(err))
		return
	}
	defer conn.Close()

	ac.logger.Info("Listening for audio from bridge on :4000")

	buffer := make([]byte, 4096)

	for {
		select {
		case <-ac.ctx.Done():
			return
		default:
			conn.SetReadDeadline(time.Now().Add(1 * time.Second))
			n, _, err := conn.ReadFromUDP(buffer)
			if err != nil {
				if opErr, ok := err.(*net.OpError); ok && opErr.Timeout() {
					continue
				}
				if ac.ctx.Err() != nil {
					return
				}
				ac.logger.Error("Bridge UDP read error", zap.Error(err))
				continue
			}

			// Parse packet from bridge: [UserIDLen(1)][UserID][PCMData]
			if n < 2 {
				continue
			}

			idLen := int(buffer[0])
			if n < 1+idLen {
				continue
			}

			userID := string(buffer[1 : 1+idLen])
			pcmData := buffer[1+idLen : n]

			// Convert PCM bytes to int16 samples
			samples := bytesToInt16(pcmData)

			// Process audio frame
			ac.processAudioFrame(samples, userID)
		}
	}
}

// processAudioFrame processes a frame of PCM audio
func (ac *AudioCapture) processAudioFrame(samples []int16, userID string) {
	// Process frame through VAD
	isSpeaking := ac.vad.ProcessFrame(userID, samples)

	if isSpeaking {
		// Send to user stream
		stream := ac.GetUserStream(userID)
		select {
		case stream.PCMChan <- samples:
			stream.mu.Lock()
			stream.LastFrame = time.Now()
			stream.mu.Unlock()
		default:
			// Channel full, drop frame
			// ac.logger.Debug("User stream buffer full, dropping frame", zap.String("user_id", userID))
		}
	}
}

// getUserIDFromSSRC maps SSRC (Synchronization Source) to user ID
func (ac *AudioCapture) getUserIDFromSSRC(ssrc uint32) (string, bool) {
	ac.ssrcMu.RLock()
	defer ac.ssrcMu.RUnlock()

	userID, exists := ac.ssrcToUserID[ssrc]
	return userID, exists
}

// bytesToInt16 converts a byte slice to int16 samples (little-endian)
func bytesToInt16(data []byte) []int16 {
	if len(data)%2 != 0 {
		// Odd number of bytes, pad with zero
		data = append(data, 0)
	}

	samples := make([]int16, len(data)/2)
	for i := 0; i < len(samples); i++ {
		samples[i] = int16(binary.LittleEndian.Uint16(data[i*2 : (i+1)*2]))
	}

	return samples
}

// ConvertPCMToWAV converts PCM samples to WAV format for STT service
func (ac *AudioCapture) ConvertPCMToWAV(samples []int16, sampleRate int) ([]byte, error) {
	var buf bytes.Buffer

	// WAV header
	numChannels := 1 // Mono
	bitsPerSample := 16
	byteRate := sampleRate * numChannels * bitsPerSample / 8
	blockAlign := numChannels * bitsPerSample / 8
	dataSize := len(samples) * 2 // 2 bytes per sample
	fileSize := 36 + dataSize

	// Write WAV header
	buf.WriteString("RIFF")
	binary.Write(&buf, binary.LittleEndian, uint32(fileSize))
	buf.WriteString("WAVE")
	buf.WriteString("fmt ")
	binary.Write(&buf, binary.LittleEndian, uint32(16)) // fmt chunk size
	binary.Write(&buf, binary.LittleEndian, uint16(1))  // audio format (PCM)
	binary.Write(&buf, binary.LittleEndian, uint16(numChannels))
	binary.Write(&buf, binary.LittleEndian, uint32(sampleRate))
	binary.Write(&buf, binary.LittleEndian, uint32(byteRate))
	binary.Write(&buf, binary.LittleEndian, uint16(blockAlign))
	binary.Write(&buf, binary.LittleEndian, uint16(bitsPerSample))
	buf.WriteString("data")
	binary.Write(&buf, binary.LittleEndian, uint32(dataSize))

	// Write PCM data
	for _, sample := range samples {
		binary.Write(&buf, binary.LittleEndian, sample)
	}

	return buf.Bytes(), nil
}
