package voice

import (
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

// BridgeClient handles communication with the Node.js Voice Bridge service
type BridgeClient struct {
	conn   *websocket.Conn
	logger *zap.Logger
	mu     sync.Mutex
	url    string
	// Callback for forwarding payloads to Discord Gateway
	onPayloadForward func(payload interface{})
}

// OpCode constants for bridge communication
const (
	OpJoin              = "JOIN"
	OpPlay              = "PLAY"
	OpVoiceServerUpdate = "VOICE_SERVER_UPDATE"
	OpVoiceStateUpdate  = "VOICE_STATE_UPDATE"
	OpForwardPayload    = "FORWARD_PAYLOAD"
)

// BridgeMessage represents a message sent to/from the bridge
type BridgeMessage struct {
	Op   string      `json:"op"`
	Data interface{} `json:"data"`
}

// NewBridgeClient creates a new bridge client
func NewBridgeClient(logger *zap.Logger) *BridgeClient {
	return &BridgeClient{
		logger: logger,
		url:    "ws://localhost:5000",
	}
}

// SetOnPayloadForward sets the callback for forwarding payloads
func (c *BridgeClient) SetOnPayloadForward(cb func(payload interface{})) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onPayloadForward = cb
}

// WaitForBridge waits for the bridge service to be available
// It will retry connecting with exponential backoff up to maxAttempts times
func (c *BridgeClient) WaitForBridge(maxAttempts int, initialDelay time.Duration) error {
	u, err := url.Parse(c.url)
	if err != nil {
		return fmt.Errorf("invalid url: %w", err)
	}

	c.logger.Info("Waiting for voice bridge to be available", zap.String("url", u.String()))

	delay := initialDelay
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		// Try to connect
		conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
		if err == nil {
			// Success! Close the test connection and return
			conn.Close()
			c.logger.Info("Voice bridge is available", zap.Int("attempt", attempt))
			return nil
		}

		if attempt < maxAttempts {
			c.logger.Debug("Voice bridge not ready, retrying...",
				zap.Int("attempt", attempt),
				zap.Int("max_attempts", maxAttempts),
				zap.Duration("retry_delay", delay),
				zap.Error(err))
			time.Sleep(delay)
			// Exponential backoff: double the delay each time (capped at 5 seconds)
			delay *= 2
			if delay > 5*time.Second {
				delay = 5 * time.Second
			}
		}
	}

	return fmt.Errorf("voice bridge not available after %d attempts", maxAttempts)
}

// Connect connects to the bridge service
func (c *BridgeClient) Connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	u, err := url.Parse(c.url)
	if err != nil {
		return fmt.Errorf("invalid url: %w", err)
	}

	c.logger.Info("Connecting to voice bridge", zap.String("url", u.String()))

	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		return fmt.Errorf("failed to dial: %w", err)
	}

	c.conn = conn

	// Start reading (required to process ping/pong and receiving messages)
	go c.readLoop()

	return nil
}

// Send sends a message to the bridge
func (c *BridgeClient) Send(op string, data interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil {
		return fmt.Errorf("not connected")
	}

	msg := BridgeMessage{
		Op:   op,
		Data: data,
	}

	if err := c.conn.WriteJSON(msg); err != nil {
		return fmt.Errorf("write json failed: %w", err)
	}

	return nil
}

// readLoop reads messages from the connection
func (c *BridgeClient) readLoop() {
	for {
		var msg BridgeMessage
		if err := c.conn.ReadJSON(&msg); err != nil {
			c.logger.Error("Bridge read error", zap.Error(err))
			c.mu.Lock()
			c.conn = nil
			c.mu.Unlock()
			return
		}

		if msg.Op == OpForwardPayload {
			c.logger.Debug("Bridge requested payload forward")
			c.mu.Lock()
			cb := c.onPayloadForward
			c.mu.Unlock()
			if cb != nil {
				cb(msg.Data)
			}
		}
	}
}

// Close closes the connection
func (c *BridgeClient) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}
}

// JoinChannel sends a JOIN command to the bridge
func (c *BridgeClient) JoinChannel(guildID, channelID string) error {
	return c.Send(OpJoin, map[string]interface{}{
		"guildId":   guildID,
		"channelId": channelID,
		"selfDeaf":  false,
		"selfMute":  false,
	})
}

// PlayAudio sends a PLAY command to the bridge
func (c *BridgeClient) PlayAudio(guildID, path string) error {
	return c.Send(OpPlay, map[string]interface{}{
		"guildId": guildID,
		"type":    "file", // simplified
		"path":    path,
	})
}
