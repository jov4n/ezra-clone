package voice

import (
	"math"
	"sync"
	"time"
)

// VADConfig holds configuration for voice activity detection
type VADConfig struct {
	EnergyThreshold float64       // Energy threshold for voice detection
	SilenceDuration time.Duration // Duration of silence before considering speech ended
	FrameSize       int           // Audio frame size in samples
	SampleRate      int           // Audio sample rate
}

// DefaultVADConfig returns a default VAD configuration
func DefaultVADConfig() *VADConfig {
	return &VADConfig{
		EnergyThreshold: 0.01,
		SilenceDuration: 1000 * time.Millisecond,
		FrameSize:       480,  // 10ms at 48kHz
		SampleRate:      48000,
	}
}

// VoiceActivityDetector detects voice activity in audio streams
type VoiceActivityDetector struct {
	config       *VADConfig
	mu           sync.Mutex
	activeUsers  map[string]*UserVoiceState
	lastActivity map[string]time.Time
}

// UserVoiceState tracks voice state for a single user
type UserVoiceState struct {
	IsSpeaking    bool
	LastActivity  time.Time
	SpeechStart   time.Time
	SpeechBuffer  []int16 // Buffer for current speech segment
	SilenceStart  time.Time
}

// NewVoiceActivityDetector creates a new VAD
func NewVoiceActivityDetector(config *VADConfig) *VoiceActivityDetector {
	return &VoiceActivityDetector{
		config:       config,
		activeUsers:  make(map[string]*UserVoiceState),
		lastActivity: make(map[string]time.Time),
	}
}

// ProcessFrame processes an audio frame and returns whether voice is detected
// frame: PCM 16-bit samples
func (vad *VoiceActivityDetector) ProcessFrame(userID string, frame []int16) bool {
	vad.mu.Lock()
	defer vad.mu.Unlock()

	// Calculate energy (RMS) of the frame
	energy := calculateEnergy(frame)

	// Get or create user state
	state, exists := vad.activeUsers[userID]
	if !exists {
		state = &UserVoiceState{
			SpeechBuffer: make([]int16, 0, 48000), // Pre-allocate for ~1 second
		}
		vad.activeUsers[userID] = state
	}

	now := time.Now()
	voiceDetected := energy > vad.config.EnergyThreshold

	if voiceDetected {
		// Voice detected
		if !state.IsSpeaking {
			// Start of speech
			state.IsSpeaking = true
			state.SpeechStart = now
			state.SpeechBuffer = state.SpeechBuffer[:0] // Clear buffer
		}
		state.LastActivity = now
		vad.lastActivity[userID] = now

		// Append frame to buffer
		state.SpeechBuffer = append(state.SpeechBuffer, frame...)
	} else {
		// Silence detected
		if state.IsSpeaking {
			// Check if silence has been long enough
			if state.SilenceStart.IsZero() {
				state.SilenceStart = now
			}

			silenceDuration := now.Sub(state.SilenceStart)
			if silenceDuration >= vad.config.SilenceDuration {
				// Speech has ended
				state.IsSpeaking = false
				state.SilenceStart = time.Time{}
				return false // Speech ended
			}

			// Still in speech, but add silence samples to buffer
			// (we keep some silence to help with transcription)
			state.SpeechBuffer = append(state.SpeechBuffer, frame...)
		}
	}

	return state.IsSpeaking
}

// IsUserSpeaking returns whether a user is currently speaking
func (vad *VoiceActivityDetector) IsUserSpeaking(userID string) bool {
	vad.mu.Lock()
	defer vad.mu.Unlock()

	state, exists := vad.activeUsers[userID]
	return exists && state.IsSpeaking
}

// GetSpeechBuffer returns the current speech buffer for a user and clears it
func (vad *VoiceActivityDetector) GetSpeechBuffer(userID string) []int16 {
	vad.mu.Lock()
	defer vad.mu.Unlock()

	state, exists := vad.activeUsers[userID]
	if !exists || len(state.SpeechBuffer) == 0 {
		return nil
	}

	// Copy and clear buffer
	buffer := make([]int16, len(state.SpeechBuffer))
	copy(buffer, state.SpeechBuffer)
	state.SpeechBuffer = state.SpeechBuffer[:0]

	return buffer
}

// ClearUserState clears the voice state for a user
func (vad *VoiceActivityDetector) ClearUserState(userID string) {
	vad.mu.Lock()
	defer vad.mu.Unlock()

	delete(vad.activeUsers, userID)
	delete(vad.lastActivity, userID)
}

// GetActiveUsers returns a list of users who have been active recently
func (vad *VoiceActivityDetector) GetActiveUsers(since time.Duration) []string {
	vad.mu.Lock()
	defer vad.mu.Unlock()

	now := time.Now()
	var active []string

	for userID, lastActivity := range vad.lastActivity {
		if now.Sub(lastActivity) <= since {
			active = append(active, userID)
		}
	}

	return active
}

// calculateEnergy calculates the RMS (Root Mean Square) energy of an audio frame
func calculateEnergy(frame []int16) float64 {
	if len(frame) == 0 {
		return 0.0
	}

	var sumSquares float64
	for _, sample := range frame {
		// Normalize to [-1, 1] range
		normalized := float64(sample) / 32768.0
		sumSquares += normalized * normalized
	}

	// RMS = sqrt(mean of squares)
	rms := math.Sqrt(sumSquares / float64(len(frame)))
	return rms
}

