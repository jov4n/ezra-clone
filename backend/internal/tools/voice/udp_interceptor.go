package voice

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"ezra-clone/backend/internal/discordgo_voice"

	"github.com/bwmarrin/discordgo"
	"go.uber.org/zap"
)

// UDPInterceptor intercepts UDP packets from Discord voice connections
// Based on discord.js VoiceReceiver implementation
// Creates its own UDP socket to receive RTP packets from Discord
type UDPInterceptor struct {
	voiceConn  *discordgo.VoiceConnection
	udpConn    *net.UDPConn
	logger     *zap.Logger
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	packetChan chan []byte
	localIP    string
	localPort  int
	ssrc       uint32
	secretKey  []byte
	ownsConn   bool
}

// NewUDPInterceptor creates a new UDP interceptor
// Attempts to access discordgo's UDP connection using our wrapper
func NewUDPInterceptor(voiceConn *discordgo.VoiceConnection, logger *zap.Logger) (*UDPInterceptor, error) {
	ctx, cancel := context.WithCancel(context.Background())
	var ownsConn bool

	// Try to get the UDP connection from discordgo's VoiceConnection
	// This uses our wrapper that safely accesses the private field
	logger.Info("Attempting to access discordgo UDP connection")
	udpConn, err := discordgo_voice.SafeGetUDPConnection(voiceConn)
	if err != nil {
		logger.Warn("Could not access discordgo UDP connection, using fallback",
			zap.Error(err),
			zap.String("note", "Fallback listener will NOT receive Discord packets"))

		// Fallback: create our own UDP listener
		// This won't receive packets from Discord, but at least won't crash
		localIP, err := getLocalIP()
		if err != nil {
			cancel()
			return nil, fmt.Errorf("failed to get local IP: %w", err)
		}

		addr, err := net.ResolveUDPAddr("udp", "0.0.0.0:0")
		if err != nil {
			cancel()
			return nil, fmt.Errorf("failed to resolve UDP address: %w", err)
		}

		udpConn, err = net.ListenUDP("udp", addr)
		if err != nil {
			cancel()
			return nil, fmt.Errorf("failed to create UDP listener: %w", err)
		}

		ownsConn = true // We created it, we own it

		localAddr := udpConn.LocalAddr().(*net.UDPAddr)
		logger.Warn("Using fallback UDP listener - Discord sends packets to a different port",
			zap.String("local_addr", localAddr.String()),
			zap.String("local_ip", localIP))
	} else {
		ownsConn = false // Discordgo owns it
		logger.Info("Successfully accessed discordgo UDP connection",
			zap.String("local_addr", udpConn.LocalAddr().String()),
			zap.String("note", "Bot should now be able to receive audio packets from Discord"))
	}

	return &UDPInterceptor{
		voiceConn:  voiceConn,
		udpConn:    udpConn,
		logger:     logger,
		ctx:        ctx,
		cancel:     cancel,
		packetChan: make(chan []byte, 100),
		ownsConn:   ownsConn,
	}, nil
}

// getLocalIP gets the local IP address
// Similar to discord.js IP discovery
func getLocalIP() (string, error) {
	// Try to connect to a remote address to determine local IP
	// This is similar to how discord.js does IP discovery
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "", err
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String(), nil
}

// sendIPDiscovery sends an IP discovery packet to Discord
// This tells Discord where to send RTP packets
// Based on discord.js VoiceConnection.selectProtocol
func sendIPDiscovery(udpConn *net.UDPConn, vc *discordgo.VoiceConnection, localIP string, localPort int, logger *zap.Logger) error {
	// Get the voice server endpoint from the voice connection
	// discordgo stores this internally, but we can't access it directly
	// However, we can use the voice connection's internal state
	// The endpoint is typically stored in the Ready event data
	// For now, we'll need to get it from the voice connection's internal state
	// Since we can't access it directly, we'll use a workaround:
	// Discord voice servers typically use the same endpoint format
	// We'll need to extract it from the voice connection using reflection or
	// get it from the voice state update events

	// Actually, discordgo already handled IP discovery when the voice connection was established
	// The issue is that Discord is sending packets to discordgo's UDP socket, not ours
	// We need to either:
	// 1. Access discordgo's UDP socket (which we tried with unsafe)
	// 2. Create our own socket and somehow tell Discord to send packets there too
	// 3. Use packet capture to intercept all UDP traffic

	// For now, let's try a different approach: use the voice connection's internal UDP socket
	// by accessing it via the Ready event or by monitoring the voice connection state

	// Since we can't access the endpoint directly, we'll skip the IP discovery
	// and just create our UDP socket. Discord might send packets to multiple sockets,
	// or we might need to use packet capture.

	// For now, return nil to indicate we're not sending IP discovery
	// The socket will still be created and might receive packets if Discord
	// sends to multiple addresses or if we're on the same port
	logger.Warn("Skipping IP discovery - discordgo already handles this",
		zap.String("note", "Discord sends packets to discordgo's UDP socket, not ours"))
	return nil
}

// Start begins intercepting UDP packets
func (ui *UDPInterceptor) Start() {
	ui.wg.Add(1)
	go ui.receiveLoop()
}

// Stop stops intercepting UDP packets
func (ui *UDPInterceptor) Stop() {
	ui.cancel()
	if ui.ownsConn && ui.udpConn != nil {
		ui.udpConn.Close()
	}
	ui.wg.Wait()
}

// GetPacketChan returns the channel for receiving raw UDP packets
func (ui *UDPInterceptor) GetPacketChan() <-chan []byte {
	return ui.packetChan
}

// receiveLoop receives UDP packets from Discord
// Based on discord.js VoiceReceiver implementation
func (ui *UDPInterceptor) receiveLoop() {
	defer ui.wg.Done()

	buffer := make([]byte, 1500) // Standard MTU size

	for {
		select {
		case <-ui.ctx.Done():
			return
		default:
			// Set read deadline to allow context cancellation
			ui.udpConn.SetReadDeadline(time.Now().Add(1 * time.Second))

			n, remoteAddr, err := ui.udpConn.ReadFromUDP(buffer)
			if err != nil {
				if errors.Is(err, net.ErrClosed) {
					ui.logger.Debug("UDP connection closed, stopping interceptor")
					return
				}
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					// Timeout is expected, continue
					continue
				}
				if ui.ctx.Err() != nil {
					// Context cancelled
					return
				}
				// Also check for the string error just in case, though net.ErrClosed should catch it
				if err.Error() == "use of closed network connection" {
					ui.logger.Debug("UDP connection closed (string check), stopping interceptor")
					return
				}

				ui.logger.Debug("Error reading UDP packet", zap.Error(err))
				continue
			}

			// Check if this looks like an RTP packet
			// RTP packets typically start with version 2 (0x80-0x8F)
			if n >= 12 && (buffer[0]&0xC0) == 0x80 {
				packet := make([]byte, n)
				copy(packet, buffer[:n])

				ui.logger.Debug("Received RTP packet",
					zap.String("remote", remoteAddr.String()),
					zap.Int("size", n))

				// Send packet to channel (non-blocking)
				select {
				case ui.packetChan <- packet:
				default:
					// Channel full, drop packet
					ui.logger.Debug("UDP packet channel full, dropping packet")
				}
			} else {
				// Might be IP discovery response or other control packet
				ui.logger.Debug("Received non-RTP UDP packet",
					zap.String("remote", remoteAddr.String()),
					zap.Int("size", n),
					zap.String("first_bytes", fmt.Sprintf("%x", buffer[:min(n, 20)])))
			}
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
