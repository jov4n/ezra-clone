package voice

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"go.uber.org/zap"
)

// PacketCapture uses raw sockets to capture all UDP packets
// This allows us to intercept Discord voice packets even though
// discordgo doesn't expose its UDP connection
type PacketCapture struct {
	logger     *zap.Logger
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	packetChan chan []byte
	ports      map[int]bool // Track which ports we're interested in
	portsMu    sync.RWMutex
}

// NewPacketCapture creates a new packet capture instance
func NewPacketCapture(logger *zap.Logger) *PacketCapture {
	ctx, cancel := context.WithCancel(context.Background())
	return &PacketCapture{
		logger:     logger,
		ctx:        ctx,
		cancel:     cancel,
		packetChan: make(chan []byte, 100),
		ports:      make(map[int]bool),
	}
}

// AddPort adds a port to monitor for packets
func (pc *PacketCapture) AddPort(port int) {
	pc.portsMu.Lock()
	defer pc.portsMu.Unlock()
	pc.ports[port] = true
	pc.logger.Info("Added port to monitor", zap.Int("port", port))
}

// Start begins capturing packets
func (pc *PacketCapture) Start() error {
	// On Windows, raw socket access requires admin privileges
	// We'll use a different approach: listen on common Discord voice ports
	// Discord voice typically uses ports in the range 50000-65535
	
	pc.logger.Info("Starting packet capture - listening on common Discord voice ports")
	
	// Try to listen on multiple ports in the Discord voice range
	// We'll bind to several ports and see which one receives packets
	startPort := 50000
	endPort := 50100 // Try first 100 ports in range
	
	for port := startPort; port < endPort; port++ {
		pc.wg.Add(1)
		go pc.listenOnPort(port)
	}
	
	return nil
}

// listenOnPort listens for UDP packets on a specific port
func (pc *PacketCapture) listenOnPort(port int) {
	defer pc.wg.Done()
	
	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf(":%d", port))
	if err != nil {
		pc.logger.Debug("Failed to resolve UDP address", zap.Int("port", port), zap.Error(err))
		return
	}
	
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		// Port might be in use, skip it
		pc.logger.Debug("Port in use or unavailable", zap.Int("port", port))
		return
	}
	defer conn.Close()
	
	pc.logger.Debug("Listening for packets", zap.Int("port", port))
	
	buffer := make([]byte, 1500)
	
	for {
		select {
		case <-pc.ctx.Done():
			return
		default:
			conn.SetReadDeadline(time.Now().Add(1 * time.Second))
			
			n, remoteAddr, err := conn.ReadFromUDP(buffer)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					continue
				}
				if pc.ctx.Err() != nil {
					return
				}
				continue
			}
			
			// Check if this looks like an RTP packet (starts with 0x80-0x8F typically)
			if n >= 12 && (buffer[0]&0xC0) == 0x80 {
				packet := make([]byte, n)
				copy(packet, buffer[:n])
				
				pc.logger.Debug("Received potential RTP packet",
					zap.Int("port", port),
					zap.String("remote", remoteAddr.String()),
					zap.Int("size", n))
				
				select {
				case pc.packetChan <- packet:
				default:
					// Channel full, drop packet
				}
			}
		}
	}
}

// Stop stops capturing packets
func (pc *PacketCapture) Stop() {
	pc.cancel()
	pc.wg.Wait()
}

// GetPacketChan returns the channel for receiving packets
func (pc *PacketCapture) GetPacketChan() <-chan []byte {
	return pc.packetChan
}

