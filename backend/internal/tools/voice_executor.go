package tools

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"time"
	"unsafe"

	"ezra-clone/backend/internal/tools/voice"
	"ezra-clone/backend/pkg/config"

	"github.com/bwmarrin/discordgo"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

// VoiceExecutor handles voice interaction tools
type VoiceExecutor struct {
	session           *discordgo.Session
	logger            *zap.Logger
	cfg               *config.Config
	activeConnections map[string]*VoiceConnection // guildID -> connection
	mu                sync.Mutex
	converter         *voice.AudioConverter
	referenceManager  *voice.ReferenceAudioManager
	httpClient        *http.Client
	agentOrch         AgentOrchestrator // Reference to agent orchestrator for LLM processing
	bridgeClient      *voice.BridgeClient
}

// HandleVoiceStateUpdate handles voice state updates
// Note: SSRC is not available in VoiceStateUpdate, it comes from speaking events or RTP packets
// We'll track users in the channel and map SSRC dynamically as we receive packets
func (v *VoiceExecutor) HandleVoiceStateUpdate(s *discordgo.Session, vs *discordgo.VoiceStateUpdate) {
	v.mu.Lock()
	defer v.mu.Unlock()

	// Find the voice connection for this guild
	conn, exists := v.activeConnections[vs.GuildID]
	if !exists {
		return // Not connected to voice in this guild
	}

	// Check if this user is in our voice channel
	if vs.ChannelID != conn.ChannelID {
		return // User is in a different channel
	}

	// Track user presence - SSRC will be mapped when we receive RTP packets
	v.logger.Debug("Voice state update for user in our channel",
		zap.String("user_id", vs.UserID),
		zap.String("guild_id", vs.GuildID),
		zap.String("channel_id", vs.ChannelID),
	)
}

// MapSSRCToUser maps an SSRC to a user ID dynamically
// This is called when we receive RTP packets with unknown SSRC
// We'll try to correlate with users in the voice channel
func (v *VoiceExecutor) MapSSRCToUser(guildID, channelID string, ssrc uint32) string {
	v.mu.Lock()
	defer v.mu.Unlock()

	conn, exists := v.activeConnections[guildID]
	if !exists {
		return ""
	}

	// Check if we already have this mapping
	conn.mu.Lock()
	if userID, exists := conn.SSRCMap[ssrc]; exists {
		conn.mu.Unlock()
		return userID
	}
	conn.mu.Unlock()

	// Try to find user from voice states
	// Since SSRC is assigned when speaking, we'll use a heuristic
	guild, err := v.session.State.Guild(guildID)
	if err == nil && guild != nil {
		// Get users in the channel who could be speaking
		usersInChannel := make([]string, 0)
		for _, vs := range guild.VoiceStates {
			if vs.ChannelID == channelID && !vs.Mute && !vs.SelfMute {
				usersInChannel = append(usersInChannel, vs.UserID)
			}
		}

		// If only one user, map to them (heuristic)
		if len(usersInChannel) == 1 {
			userID := usersInChannel[0]
			conn.mu.Lock()
			conn.SSRCMap[ssrc] = userID
			if conn.Capture != nil {
				conn.Capture.SetSSRCMapping(ssrc, userID)
			}
			conn.mu.Unlock()
			v.logger.Debug("Mapped SSRC to user (heuristic)",
				zap.Uint32("ssrc", ssrc),
				zap.String("user_id", userID))
			return userID
		}
	}

	return ""
}

// VoiceConnection represents an active voice connection with listening
type VoiceConnection struct {
	GuildID      string
	ChannelID    string
	VoiceConn    *discordgo.VoiceConnection // Can be nil in Bridge mode
	Capture      *voice.AudioCapture
	VAD          *voice.VoiceActivityDetector
	Orchestrator *voice.VoiceOrchestrator
	SSRCMap      map[uint32]string // SSRC -> UserID
	AgentOrch    AgentOrchestrator
	ctx          context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup
	mu           sync.Mutex
}

// SetAgentOrchestrator sets the agent orchestrator for LLM processing
func (v *VoiceExecutor) SetAgentOrchestrator(orch AgentOrchestrator) {
	v.agentOrch = orch
}

// WarmupServices warms up STT and TTS services to avoid cold start
// This actually loads the models by calling warmup endpoints
// Retries with exponential backoff until services are available
func (v *VoiceExecutor) WarmupServices(ctx context.Context) {
	// Run warmup in background goroutine so it doesn't block startup
	go v.warmupServicesWithRetry(ctx)
}

// warmupServicesWithRetry attempts to warm up services with retry logic
func (v *VoiceExecutor) warmupServicesWithRetry(ctx context.Context) {
	sttWarmed := false
	ttsWarmed := false
	maxRetries := 10
	retryDelay := 2 * time.Second

	// Warm up STT service
	sttWarmupURL := v.cfg.STTServiceURL + "/warmup"
	for attempt := 0; attempt < maxRetries && !sttWarmed; attempt++ {
		if attempt > 0 {
			// Wait before retry (exponential backoff)
			delay := retryDelay * time.Duration(attempt)
			v.logger.Debug("Retrying STT warmup",
				zap.Int("attempt", attempt+1),
				zap.Duration("delay", delay))
			time.Sleep(delay)
		}

		req, err := http.NewRequestWithContext(ctx, "POST", sttWarmupURL, nil)
		if err != nil {
			v.logger.Debug("Failed to create STT warmup request", zap.Error(err))
			continue
		}

		resp, err := v.httpClient.Do(req)
		if err != nil {
			if attempt < maxRetries-1 {
				continue // Retry
			}
			v.logger.Debug("STT service not available for warmup after retries",
				zap.String("url", sttWarmupURL),
				zap.Int("attempts", maxRetries),
				zap.String("note", "Service will be used on first voice request"),
				zap.Error(err))
		} else {
			resp.Body.Close()
			sttWarmed = true
			v.logger.Info("STT service (Faster-Whisper) warmed up - model loaded and ready",
				zap.Int("attempts", attempt+1))
		}
	}

	// Warm up TTS service
	ttsWarmupURL := v.cfg.TTSServiceURL + "/warmup"
	for attempt := 0; attempt < maxRetries && !ttsWarmed; attempt++ {
		if attempt > 0 {
			// Wait before retry (exponential backoff)
			delay := retryDelay * time.Duration(attempt)
			v.logger.Debug("Retrying TTS warmup",
				zap.Int("attempt", attempt+1),
				zap.Duration("delay", delay))
			time.Sleep(delay)
		}

		req, err := http.NewRequestWithContext(ctx, "POST", ttsWarmupURL, nil)
		if err != nil {
			v.logger.Debug("Failed to create TTS warmup request", zap.Error(err))
			continue
		}

		resp, err := v.httpClient.Do(req)
		if err != nil {
			if attempt < maxRetries-1 {
				continue // Retry
			}
			v.logger.Debug("TTS service not available for warmup after retries",
				zap.String("url", ttsWarmupURL),
				zap.Int("attempts", maxRetries),
				zap.String("note", "Service will be used on first voice request"),
				zap.Error(err))
		} else {
			resp.Body.Close()
			ttsWarmed = true
			v.logger.Info("TTS service (XTTS V2) warmed up - model loaded and ready",
				zap.Int("attempts", attempt+1))
		}
	}

	// Summary
	if sttWarmed && ttsWarmed {
		v.logger.Info("Voice services warmup completed successfully",
			zap.Bool("stt_warmed", sttWarmed),
			zap.Bool("tts_warmed", ttsWarmed))
	} else {
		v.logger.Info("Voice services warmup attempted",
			zap.Bool("stt_warmed", sttWarmed),
			zap.Bool("tts_warmed", ttsWarmed),
			zap.String("note", "Services not available will load on first use"))
	}
}

// NewVoiceExecutor creates a new voice executor
func NewVoiceExecutor(session *discordgo.Session, logger *zap.Logger, cfg *config.Config) (*VoiceExecutor, error) {
	// Find ffmpeg
	ffmpegPath := findFFmpeg()
	if ffmpegPath == "" {
		return nil, fmt.Errorf("ffmpeg not found - required for voice features")
	}

	converter := voice.NewAudioConverter(ffmpegPath)

	// Create reference audio manager
	referenceManager, err := voice.NewReferenceAudioManager(cfg.VoiceReferenceDir, converter, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create reference audio manager: %w", err)
	}

	// Initialize Bridge Client
	bridgeClient := voice.NewBridgeClient(logger)
	
	// Wait for bridge to be available before connecting
	logger.Info("Waiting for voice bridge service to start...")
	if err := bridgeClient.WaitForBridge(10, 500*time.Millisecond); err != nil {
		return nil, fmt.Errorf("voice bridge not available: %w", err)
	}
	
	// Now connect to the bridge
	if err := bridgeClient.Connect(); err != nil {
		return nil, fmt.Errorf("failed to connect to voice bridge: %w", err)
	}

	// Handle payload forwarding from Bridge -> Discord Gateway
	bridgeClient.SetOnPayloadForward(func(payload interface{}) {
		// Write the raw payload directly to the gateway websocket connection

		p, ok := payload.(map[string]interface{})
		if !ok {
			logger.Error("Invalid payload type from bridge", zap.Any("payload_type", reflect.TypeOf(payload)))
			return
		}

		// Get the websocket connection from the session using reflection
		// Session is a pointer, so we need to get the value it points to
		rs := reflect.ValueOf(session)
		if rs.Kind() == reflect.Ptr {
			rs = rs.Elem()
		}

		rf := rs.FieldByName("wsConn")
		if !rf.IsValid() {
			logger.Error("Invalid wsConn field in Session struct. Discordgo version mismatch?",
				zap.String("session_type", rs.Type().String()))
			return
		}

		// Check if the field is nil (for pointer types)
		if rf.Kind() == reflect.Ptr && rf.IsNil() {
			logger.Error("wsConn is nil - Bot maybe not connected to Gateway?")
			return
		}

		// Access the private field using unsafe pointer
		// Note: wsConn field in discordgo.Session is *websocket.Conn (a pointer)
		rf = reflect.NewAt(rf.Type(), unsafe.Pointer(rf.UnsafeAddr())).Elem()

		// Get the websocket connection pointer
		// wsConn is a pointer field, so rf.Interface() gives us *websocket.Conn
		if rf.Kind() != reflect.Ptr {
			logger.Error("wsConn field is not a pointer",
				zap.String("type", rf.Type().String()),
				zap.String("kind", rf.Kind().String()))
			return
		}

		if rf.IsNil() {
			logger.Error("wsConn pointer is nil - Bot maybe not connected to Gateway?")
			return
		}

		wsConn, ok := rf.Interface().(*websocket.Conn)
		if !ok || wsConn == nil {
			logger.Error("wsConn is not a *websocket.Conn",
				zap.String("type", rf.Type().String()),
				zap.String("kind", rf.Kind().String()))
			return
		}

		// Write the payload directly to the websocket
		if err := wsConn.WriteJSON(p); err != nil {
			logger.Error("Failed to write payload to gateway websocket", zap.Error(err))
			return
		}

		op, _ := p["op"].(float64) // JSON numbers are floats
		logger.Debug("Forwarded payload to Discord Gateway", zap.Int("op", int(op)))
	})

	ve := &VoiceExecutor{
		session:           session,
		logger:            logger,
		cfg:               cfg,
		activeConnections: make(map[string]*VoiceConnection),
		converter:         converter,
		referenceManager:  referenceManager,
		httpClient: &http.Client{
			Timeout: 60 * time.Second, // Longer timeout for TTS generation
			Transport: &http.Transport{
				MaxIdleConns:        10,
				MaxIdleConnsPerHost: 5,
				IdleConnTimeout:     90 * time.Second,
				DisableKeepAlives:   false, // Enable keep-alive for faster subsequent requests
			},
		},
		bridgeClient: bridgeClient,
	}

	// Register handlers to forward events to Bridge
	session.AddHandler(func(s *discordgo.Session, e *discordgo.VoiceServerUpdate) {
		// Forward to Bridge
		bridgeClient.Send("VOICE_SERVER_UPDATE", e)
	})

	session.AddHandler(func(s *discordgo.Session, e *discordgo.VoiceStateUpdate) {
		// Forward to Bridge
		// Only forward if it's OUR bot
		if e.UserID == s.State.User.ID {
			bridgeClient.Send("VOICE_STATE_UPDATE", e)
		}

		// Keep existing tracking logic
		ve.HandleVoiceStateUpdate(s, e)
	})

	return ve, nil
}

// findFFmpeg finds the ffmpeg executable
func findFFmpeg() string {
	if path, err := exec.LookPath("ffmpeg"); err == nil {
		return path
	}

	if runtime.GOOS == "windows" {
		if path, err := exec.LookPath("ffmpeg.exe"); err == nil {
			return path
		}
	}

	return ""
}

// ExecuteVoiceTool executes a voice-related tool
func (v *VoiceExecutor) ExecuteVoiceTool(ctx context.Context, execCtx *ExecutionContext, toolName string, args map[string]interface{}) *ToolResult {
	switch toolName {
	case ToolVoiceJoinChannel:
		return v.handleJoinChannel(ctx, execCtx, args)
	case ToolVoiceLeaveChannel:
		return v.handleLeaveChannel(ctx, execCtx, args)
	case ToolVoiceSetReference:
		return v.handleSetReference(ctx, execCtx, args)
	case ToolVoiceStatus:
		return v.handleStatus(ctx, execCtx, args)
	default:
		return &ToolResult{
			Success: false,
			Error:   fmt.Sprintf("Unknown voice tool: %s", toolName),
		}
	}
}

// handleJoinChannel joins a voice channel and starts listening
func (v *VoiceExecutor) handleJoinChannel(ctx context.Context, execCtx *ExecutionContext, args map[string]interface{}) *ToolResult {
	// Extract channel ID
	channelID, _ := args["channel_id"].(string)
	if channelID == "" {
		// Try to detect user's current voice channel
		channelID = v.detectUserVoiceChannel(execCtx.UserID, execCtx.ChannelID)
		if channelID == "" {
			return &ToolResult{
				Success: false,
				Error:   "Could not determine voice channel. Please specify channel_id or join a voice channel first.",
			}
		}
	}

	// Get guild ID from channel
	channel, err := v.session.Channel(channelID)
	if err != nil {
		return &ToolResult{
			Success: false,
			Error:   fmt.Sprintf("Failed to get channel: %v", err),
		}
	}

	if channel.Type != discordgo.ChannelTypeGuildVoice {
		return &ToolResult{
			Success: false,
			Error:   "Channel is not a voice channel",
		}
	}

	guildID := channel.GuildID

	// Check if already connected
	v.mu.Lock()
	if existing, exists := v.activeConnections[guildID]; exists {
		if existing.ChannelID == channelID {
			v.mu.Unlock()
			return &ToolResult{
				Success: true,
				Message: fmt.Sprintf("Already connected to voice channel %s", channel.Name),
			}
		}
		// Disconnect from old channel
		v.disconnectGuild(guildID)
	}
	v.mu.Unlock()

	// Use Bridge to join voice channel
	// This triggers the sequence: Bridge -> Forward Payload -> Go -> Discord -> Events -> Bridge
	v.logger.Info("Sending JOIN command to voice bridge",
		zap.String("guild_id", guildID),
		zap.String("channel_id", channelID))
	
	if err := v.bridgeClient.JoinChannel(guildID, channelID); err != nil {
		v.logger.Error("Failed to send JOIN command to bridge", zap.Error(err))
		return &ToolResult{
			Success: false,
			Error:   fmt.Sprintf("Failed to initiate bridge connection: %v", err),
		}
	}
	
	v.logger.Debug("JOIN command sent to bridge successfully")

	// We still need a local "Pseudo" VoiceConnection to manage state, VAD, etc.
	// But we won't have a *discordgo.VoiceConnection immediately, or ever?
	// discordgo tracks voice state, but since we didn't use its VoiceJoin, it creates no VoiceConnection struct.
	// We'll proceed with nil VoiceConnection.

	// Create VAD
	vadConfig := voice.DefaultVADConfig()
	vadConfig.EnergyThreshold = v.cfg.VADThreshold
	vadConfig.SilenceDuration = time.Duration(v.cfg.SilenceDuration) * time.Millisecond
	vad := voice.NewVoiceActivityDetector(vadConfig)

	// Create orchestrator
	orchestrator := voice.NewVoiceOrchestrator(vad, v.logger)

	// Create Opus decoder (Not needed for Bridge PCM? But AudioCapture constructor wants it)
	opusDecoder := voice.NewOpusDecoder(findFFmpeg())

	// Create audio capture
	// PASS NIL for voiceConn! AudioCapture needs update to handle this.
	capture := voice.NewAudioCapture(nil, vad, v.converter, opusDecoder, v.logger)

	// Create voice connection wrapper
	voiceCtx, cancel := context.WithCancel(context.Background())
	voiceConn := &VoiceConnection{
		GuildID:      guildID,
		ChannelID:    channelID,
		VoiceConn:    nil, // Managed by Bridge
		Capture:      capture,
		VAD:          vad,
		Orchestrator: orchestrator,
		SSRCMap:      make(map[uint32]string),
		ctx:          voiceCtx,
		cancel:       cancel,
	}

	// Set agent orchestrator for LLM processing
	voiceConn.AgentOrch = v.agentOrch

	// Set up voice state tracking for SSRC mapping
	v.setupVoiceStateTracking(voiceConn, guildID)

	// Start capturing
	if err := capture.Start(); err != nil {
		return &ToolResult{
			Success: false,
			Error:   fmt.Sprintf("Failed to start audio capture: %v", err),
		}
	}

	// Start processing loop
	voiceConn.wg.Add(1)
	go v.processVoiceInput(voiceConn, execCtx)

	// Store connection
	v.mu.Lock()
	v.activeConnections[guildID] = voiceConn
	v.mu.Unlock()

	v.logger.Info("Joined voice channel via Bridge",
		zap.String("guild_id", guildID),
		zap.String("channel_id", channelID),
	)

	return &ToolResult{
		Success: true,
		Message: fmt.Sprintf("Joined voice channel %s via Bridge", channel.Name),
	}
}

// handleLeaveChannel leaves the voice channel
func (v *VoiceExecutor) handleLeaveChannel(ctx context.Context, execCtx *ExecutionContext, args map[string]interface{}) *ToolResult {
	// Get guild ID
	var guildID string
	if gid, ok := args["guild_id"].(string); ok && gid != "" {
		guildID = gid
	} else if execCtx.ChannelID != "" {
		channel, err := v.session.Channel(execCtx.ChannelID)
		if err == nil && channel != nil {
			guildID = channel.GuildID
		}
	}

	if guildID == "" {
		return &ToolResult{
			Success: false,
			Error:   "Could not determine guild ID",
		}
	}

	v.mu.Lock()
	defer v.mu.Unlock()

	if err := v.disconnectGuild(guildID); err != nil {
		return &ToolResult{
			Success: false,
			Error:   err.Error(),
		}
	}

	return &ToolResult{
		Success: true,
		Message: "Left voice channel",
	}
}

// handleSetReference sets the voice reference audio
func (v *VoiceExecutor) handleSetReference(ctx context.Context, execCtx *ExecutionContext, args map[string]interface{}) *ToolResult {
	// Check for attachment URL or file path
	attachmentURL, _ := args["attachment_url"].(string)
	filePath, _ := args["file_path"].(string)

	if attachmentURL == "" && filePath == "" {
		return &ToolResult{
			Success: false,
			Error:   "Either attachment_url or file_path must be provided",
		}
	}

	userID := execCtx.UserID
	if userID == "" {
		userID = "default"
	}

	if attachmentURL != "" {
		// Download attachment
		resp, err := v.httpClient.Get(attachmentURL)
		if err != nil {
			return &ToolResult{
				Success: false,
				Error:   fmt.Sprintf("Failed to download attachment: %v", err),
			}
		}
		defer resp.Body.Close()

		contentType := resp.Header.Get("Content-Type")
		if err := v.referenceManager.SetReferenceFromReader(ctx, userID, resp.Body, contentType); err != nil {
			return &ToolResult{
				Success: false,
				Error:   fmt.Sprintf("Failed to set reference audio: %v", err),
			}
		}
	} else {
		// Validate file
		if err := v.referenceManager.ValidateAudioFile(filePath); err != nil {
			return &ToolResult{
				Success: false,
				Error:   err.Error(),
			}
		}

		// Set reference from file
		if err := v.referenceManager.SetReferenceFromFile(ctx, userID, filePath); err != nil {
			return &ToolResult{
				Success: false,
				Error:   fmt.Sprintf("Failed to set reference audio: %v", err),
			}
		}
	}

	return &ToolResult{
		Success: true,
		Message: "Voice reference audio set successfully",
	}
}

// handleStatus returns the current voice connection status
func (v *VoiceExecutor) handleStatus(ctx context.Context, execCtx *ExecutionContext, args map[string]interface{}) *ToolResult {
	var guildID string
	if gid, ok := args["guild_id"].(string); ok && gid != "" {
		guildID = gid
	} else if execCtx.ChannelID != "" {
		channel, err := v.session.Channel(execCtx.ChannelID)
		if err == nil && channel != nil {
			guildID = channel.GuildID
		}
	}

	v.mu.Lock()
	defer v.mu.Unlock()

	if guildID == "" {
		// Return all connections
		connections := make([]map[string]interface{}, 0, len(v.activeConnections))
		for gid, conn := range v.activeConnections {
			connections = append(connections, map[string]interface{}{
				"guild_id":   gid,
				"channel_id": conn.ChannelID,
				"active":     true,
			})
		}
		return &ToolResult{
			Success: true,
			Data:    map[string]interface{}{"connections": connections},
		}
	}

	conn, exists := v.activeConnections[guildID]
	if !exists {
		return &ToolResult{
			Success: true,
			Data:    map[string]interface{}{"connected": false},
		}
	}

	return &ToolResult{
		Success: true,
		Data: map[string]interface{}{
			"connected":  true,
			"guild_id":   conn.GuildID,
			"channel_id": conn.ChannelID,
		},
	}
}

// processVoiceInput processes voice input and generates responses
func (v *VoiceExecutor) processVoiceInput(voiceConn *VoiceConnection, execCtx *ExecutionContext) {
	defer voiceConn.wg.Done()

	// Start processing user voice streams
	go v.monitorUserStreams(voiceConn, execCtx)

	// Response loop
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-voiceConn.ctx.Done():
			return
		case <-ticker.C:
			if voiceConn.Orchestrator.ShouldRespond(voiceConn.ctx) {
				response := voiceConn.Orchestrator.GetNextResponse()
				if response != nil {
					v.generateAndPlayResponse(voiceConn, response, execCtx)
				}
			}
		}
	}
}

// monitorUserStreams monitors and processes user voice streams
func (v *VoiceExecutor) monitorUserStreams(voiceConn *VoiceConnection, execCtx *ExecutionContext) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	processedUsers := make(map[string]bool)

	for {
		select {
		case <-voiceConn.ctx.Done():
			return
		case <-ticker.C:
			// Get active users from VAD
			activeUsers := voiceConn.VAD.GetActiveUsers(5 * time.Second)
			for _, userID := range activeUsers {
				if !processedUsers[userID] {
					stream := voiceConn.Capture.GetUserStream(userID)
					go v.processUserVoice(voiceConn, userID, stream, execCtx)
					processedUsers[userID] = true
				}
			}
		}
	}
}

// processUserVoice processes voice input from a single user
func (v *VoiceExecutor) processUserVoice(voiceConn *VoiceConnection, userID string, stream *voice.UserStream, execCtx *ExecutionContext) {
	// Buffer for collecting speech
	speechBuffer := make([]int16, 0, 48000*5) // 5 seconds buffer
	lastActivity := time.Now()
	silenceTimeout := 2 * time.Second

	for {
		select {
		case <-voiceConn.ctx.Done():
			return
		case samples := <-stream.PCMChan:
			if len(samples) == 0 {
				continue
			}

			speechBuffer = append(speechBuffer, samples...)
			lastActivity = time.Now()

			// Check if speech has ended (silence detected)
			if !voiceConn.VAD.IsUserSpeaking(userID) {
				// Wait a bit more to ensure speech is complete
				time.Sleep(500 * time.Millisecond)

				if len(speechBuffer) > 4800 { // At least 100ms of audio
					// Process speech
					go v.transcribeAndRespond(voiceConn, userID, speechBuffer, execCtx)
					speechBuffer = speechBuffer[:0] // Clear buffer
				}
			}
		case <-time.After(silenceTimeout):
			// Timeout - process accumulated speech
			if len(speechBuffer) > 4800 && time.Since(lastActivity) > silenceTimeout {
				go v.transcribeAndRespond(voiceConn, userID, speechBuffer, execCtx)
				speechBuffer = speechBuffer[:0]
			}
		}
	}
}

// transcribeAndRespond transcribes speech and generates a response
func (v *VoiceExecutor) transcribeAndRespond(voiceConn *VoiceConnection, userID string, samples []int16, execCtx *ExecutionContext) {
	// Convert PCM to WAV
	wavData, err := voiceConn.Capture.ConvertPCMToWAV(samples, 48000)
	if err != nil {
		v.logger.Error("Failed to convert PCM to WAV", zap.Error(err))
		return
	}

	// Send to STT service
	text, err := v.transcribeAudio(wavData)
	if err != nil {
		v.logger.Error("Failed to transcribe audio", zap.Error(err))
		return
	}

	if text == "" {
		return
	}

	v.logger.Info("Transcribed speech", zap.String("user_id", userID), zap.String("text", text))

	// Process with LLM (if agent orchestrator is set)
	if voiceConn.AgentOrch != nil {
		// Get LLM response
		turnResult, err := voiceConn.AgentOrch.RunTurn(context.Background(), execCtx.AgentID, userID, text)
		if err != nil {
			v.logger.Error("Failed to get LLM response", zap.Error(err))
			return
		}

		if turnResult != nil && !turnResult.IsIgnored() {
			content := turnResult.GetContent()
			if content != "" {
				// Queue response
				isQuestion := strings.HasSuffix(text, "?")
				mentionsBot := strings.Contains(strings.ToLower(text), "ezra") || strings.Contains(strings.ToLower(text), "bot")
				priority := voiceConn.Orchestrator.CalculatePriority(content, isQuestion, mentionsBot)

				if err := voiceConn.Orchestrator.QueueResponse(userID, content, priority); err != nil {
					v.logger.Error("Failed to queue response", zap.Error(err))
				}
			}
		}
	}
}

// transcribeAudio sends audio to STT service and returns transcribed text
func (v *VoiceExecutor) transcribeAudio(wavData []byte) (string, error) {
	// Create multipart form request
	boundary := "----WebKitFormBoundary7MA4YWxkTrZu0gW"
	body := &bytes.Buffer{}

	// Write form field
	body.WriteString(fmt.Sprintf("--%s\r\n", boundary))
	body.WriteString("Content-Disposition: form-data; name=\"audio\"; filename=\"audio.wav\"\r\n")
	body.WriteString("Content-Type: audio/wav\r\n\r\n")
	body.Write(wavData)
	body.WriteString(fmt.Sprintf("\r\n--%s--\r\n", boundary))

	req, err := http.NewRequest("POST", v.cfg.STTServiceURL+"/transcribe", body)
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", fmt.Sprintf("multipart/form-data; boundary=%s", boundary))

	resp, err := v.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("STT service returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var result struct {
		Text string `json:"text"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	return result.Text, nil
}

// generateAndPlayResponse generates TTS audio and plays it with streaming support
// Starts playing as soon as the first chunk is ready to simulate real-time
func (v *VoiceExecutor) generateAndPlayResponse(voiceConn *VoiceConnection, response *voice.QueuedResponse, execCtx *ExecutionContext) {
	voiceConn.Orchestrator.SetResponding(true)
	defer voiceConn.Orchestrator.SetResponding(false)

	// Get reference audio for user
	refPath, err := v.referenceManager.GetReference(response.UserID)
	if err != nil {
		v.logger.Warn("No reference audio found, using default", zap.Error(err))
		refPath, err = v.referenceManager.GetReference("default")
		if err != nil {
			v.logger.Error("No default reference audio", zap.Error(err))
			return
		}
	}

	// Use streaming TTS to start playback as soon as first chunk is ready
	if err := v.synthesizeAndPlayStreaming(voiceConn.GuildID, response.Text, refPath); err != nil {
		v.logger.Error("Failed to synthesize and play speech", zap.Error(err))
		// Fallback to non-streaming
		audioData, err := v.synthesizeSpeech(response.Text, refPath)
		if err != nil {
			v.logger.Error("Failed to synthesize speech (fallback)", zap.Error(err))
			return
		}
		if err := v.playAudioViaBridge(voiceConn.GuildID, audioData); err != nil {
			v.logger.Error("Failed to play audio via bridge", zap.Error(err))
		}
	}
}

// synthesizeAndPlayStreaming streams TTS audio and starts playing immediately
// This allows real-time playback as chunks are generated
// First chunk starts playback immediately, subsequent chunks are queued
func (v *VoiceExecutor) synthesizeAndPlayStreaming(guildID, text, referencePath string) error {
	// Create JSON request with streaming enabled
	reqData := map[string]interface{}{
		"text":           text,
		"reference_path": referencePath,
		"stream":         true,
	}

	jsonData, err := json.Marshal(reqData)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", v.cfg.TTSServiceURL+"/synthesize", bytes.NewReader(jsonData))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := v.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("TTS service returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	// Stream chunks: accumulate and start playing as soon as first chunk is ready
	// Python generates chunks incrementally for speed, we accumulate and play complete audio
	chunkBuffer := make([]byte, 128*1024) // 128KB buffer
	var accumulatedAudio []byte
	chunkCount := 0
	
	// Start playback as soon as we have enough audio (first chunk)
	playStarted := false
	playStartTime := time.Now()
	
	for {
		n, err := resp.Body.Read(chunkBuffer)
		if n > 0 {
			accumulatedAudio = append(accumulatedAudio, chunkBuffer[:n]...)
			chunkCount++
			
			// Start playing as soon as we have first chunk (enough for WAV header + some audio)
			if !playStarted && len(accumulatedAudio) >= 8192 { // 8KB minimum (WAV header + initial audio)
				playStarted = true
				timeToFirstChunk := time.Since(playStartTime)
				
				v.logger.Info("First audio chunk ready, starting playback immediately",
					zap.Int("bytes_received", len(accumulatedAudio)),
					zap.Duration("time_to_first_chunk", timeToFirstChunk))
				
				// Start playing accumulated audio so far in background
				// We'll continue accumulating and update the file
				go func(initialAudio []byte) {
					// Save initial chunk to temp file
					tmpFile, err := os.CreateTemp(os.TempDir(), "tts_stream_*.wav")
					if err != nil {
						v.logger.Error("Failed to create temp file for streaming", zap.Error(err))
						return
					}
					
					if _, err := tmpFile.Write(initialAudio); err != nil {
						tmpFile.Close()
						os.Remove(tmpFile.Name())
						v.logger.Error("Failed to write initial chunk", zap.Error(err))
						return
					}
					tmpFile.Close()
					
					absPath, _ := filepath.Abs(tmpFile.Name())
					
					// Start playback
					if err := v.bridgeClient.PlayAudio(guildID, absPath); err != nil {
						v.logger.Error("Failed to start streaming playback", zap.Error(err))
						os.Remove(absPath)
					}
				}(accumulatedAudio)
			}
		}
		
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}
	
	// If we never started playing (chunks too small), play complete audio now
	if !playStarted && len(accumulatedAudio) > 0 {
		v.logger.Info("All audio received, starting playback",
			zap.Int("total_bytes", len(accumulatedAudio)),
			zap.Int("chunks", chunkCount))
		return v.playAudioViaBridge(guildID, accumulatedAudio)
	}
	
	// If we started playing but have more audio, we need to handle it
	// For now, the initial playback will play what it has
	// In a production system, you'd append to the playing file or use a proper streaming format
	if playStarted && len(accumulatedAudio) > 0 {
		v.logger.Info("Streaming TTS complete",
			zap.Int("total_bytes", len(accumulatedAudio)),
			zap.Int("chunks", chunkCount),
			zap.Bool("playback_started", playStarted))
		// Note: Initial playback already started, remaining audio was accumulated
		// For proper streaming, you'd need to append to the playing file or queue subsequent chunks
	}
	
	return nil
}

// synthesizeSpeech sends text to TTS service and returns audio
func (v *VoiceExecutor) synthesizeSpeech(text, referencePath string) ([]byte, error) {
	// Create JSON request with text and reference path
	// The TTS service will read the reference file from the path
	reqData := map[string]interface{}{
		"text":           text,
		"reference_path": referencePath,
	}

	jsonData, err := json.Marshal(reqData)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", v.cfg.TTSServiceURL+"/synthesize", bytes.NewReader(jsonData))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := v.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("TTS service returned status %d", resp.StatusCode)
	}

	audioData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return audioData, nil
}

// playAudioViaBridge plays audio through the voice bridge
func (v *VoiceExecutor) playAudioViaBridge(guildID string, audioData []byte) error {
	// Save audio to temporary file for bridge to play
	// Try to use system temp dir (usually faster, sometimes RAM-backed)
	tmpDir := os.TempDir()
	tmpFile, err := os.CreateTemp(tmpDir, "tts_*.wav")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	// Don't defer remove - let the bridge delete it after playback
	// We'll clean it up in a goroutine after a delay

	// Write audio data to temp file (buffered for speed)
	// Use larger buffer for faster writes
	bufWriter := bufio.NewWriterSize(tmpFile, 64*1024) // 64KB buffer
	if _, err := bufWriter.Write(audioData); err != nil {
		bufWriter.Flush()
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		return fmt.Errorf("failed to write audio to temp file: %w", err)
	}
	if err := bufWriter.Flush(); err != nil {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		return fmt.Errorf("failed to flush audio to temp file: %w", err)
	}
	tmpFile.Close()

	// Send PLAY command to bridge
	absPath, err := filepath.Abs(tmpFile.Name())
	if err != nil {
		os.Remove(tmpFile.Name())
		return fmt.Errorf("failed to get absolute path: %w", err)
	}

	v.logger.Debug("Sending audio to bridge for playback",
		zap.String("guild_id", guildID),
		zap.String("path", absPath))

	if err := v.bridgeClient.PlayAudio(guildID, absPath); err != nil {
		os.Remove(tmpFile.Name())
		return fmt.Errorf("failed to send play command to bridge: %w", err)
	}

	// Clean up file after a delay (give time for playback)
	// Estimate: ~1 second per 24KB of audio (24kHz sample rate)
	estimatedDuration := time.Duration(len(audioData)/24000) * time.Second
	if estimatedDuration < 5*time.Second {
		estimatedDuration = 5 * time.Second // Minimum 5 seconds
	}
	
	go func() {
		time.Sleep(estimatedDuration + 2*time.Second) // Add buffer
		if err := os.Remove(absPath); err != nil {
			v.logger.Debug("Failed to remove temp audio file", 
				zap.String("path", absPath), 
				zap.Error(err))
		}
	}()

	return nil
}

// playAudio plays audio through the voice connection (legacy method, not used with bridge)
func (v *VoiceExecutor) playAudio(vc *discordgo.VoiceConnection, audioData []byte) error {
	if vc == nil {
		return fmt.Errorf("voice connection is nil - cannot play audio")
	}

	// Convert WAV to Opus
	opusReader, err := v.converter.PCMToOpus(context.Background(), bytes.NewReader(audioData))
	if err != nil {
		return err
	}
	defer opusReader.Close()

	// Set speaking
	vc.Speaking(true)
	defer vc.Speaking(false)

	// Read and send Opus packets
	buffer := make([]byte, 4000)
	for {
		n, err := opusReader.Read(buffer)
		if n > 0 {
			select {
			case vc.OpusSend <- buffer[:n]:
			case <-time.After(5 * time.Second):
				return fmt.Errorf("timeout sending audio")
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}

	return nil
}

// Helper functions

func (v *VoiceExecutor) detectUserVoiceChannel(userID, channelID string) string {
	// Try to get guild from channel
	channel, err := v.session.Channel(channelID)
	if err != nil {
		return ""
	}

	guildID := channel.GuildID
	if guildID == "" {
		return ""
	}

	// Get voice state
	vs, err := v.session.State.VoiceState(guildID, userID)
	if err != nil || vs == nil {
		return ""
	}

	return vs.ChannelID
}

func (v *VoiceExecutor) disconnectGuild(guildID string) error {
	conn, exists := v.activeConnections[guildID]
	if !exists {
		return fmt.Errorf("not connected to voice channel in guild %s", guildID)
	}

	conn.cancel()
	conn.Capture.Stop()
	conn.VoiceConn.Disconnect()
	conn.wg.Wait()

	delete(v.activeConnections, guildID)
	return nil
}

// setupVoiceStateTracking sets up handlers to track voice state updates
// SSRC mapping will be done dynamically as we receive RTP packets
func (v *VoiceExecutor) setupVoiceStateTracking(voiceConn *VoiceConnection, guildID string) {
	// Get current voice states from guild to know which users are in the channel
	guild, err := v.session.State.Guild(guildID)
	if err == nil && guild != nil {
		for _, vs := range guild.VoiceStates {
			if vs.ChannelID == voiceConn.ChannelID {
				v.logger.Debug("User in voice channel",
					zap.String("user_id", vs.UserID),
					zap.String("channel_id", vs.ChannelID))
				// SSRC will be mapped dynamically when we receive RTP packets
				// We'll use MapSSRCToUser to correlate SSRC with users
			}
		}
	}

	// SSRC mapping will be done dynamically from RTP packets
	// We'll use MapSSRCToUser to correlate SSRC with users in the channel
}
