package voice

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"go.uber.org/zap"
)

// RTPReceiver handles receiving and parsing RTP packets from Discord
type RTPReceiver struct {
	udpConn    *net.UDPConn
	logger     *zap.Logger
	ssrcMap    map[uint32]string // SSRC -> UserID
	ssrcMu     sync.RWMutex
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	packetChan chan *RTPPacket
}

// RTPPacket represents a parsed RTP packet
type RTPPacket struct {
	SSRC      uint32
	Payload   []byte
	Header    []byte
	SeqNum    uint16
	Timestamp uint32
}

// NewRTPReceiver creates a new RTP receiver from UDP connection
func NewRTPReceiver(udpConn *net.UDPConn, logger *zap.Logger) *RTPReceiver {
	ctx, cancel := context.WithCancel(context.Background())
	return &RTPReceiver{
		udpConn:    udpConn,
		logger:     logger,
		ssrcMap:    make(map[uint32]string),
		ctx:        ctx,
		cancel:     cancel,
		packetChan: make(chan *RTPPacket, 100),
	}
}

// NewRTPReceiverFromPackets creates a new RTP receiver from a packet channel
// This is used when packets come from packet capture instead of UDP connection
func NewRTPReceiverFromPackets(packetChan <-chan []byte, logger *zap.Logger) *RTPReceiver {
	ctx, cancel := context.WithCancel(context.Background())
	r := &RTPReceiver{
		udpConn:    nil, // No UDP connection, packets come from channel
		logger:     logger,
		ssrcMap:    make(map[uint32]string),
		ctx:        ctx,
		cancel:     cancel,
		packetChan: make(chan *RTPPacket, 100),
	}

	// Start processing packets from the channel
	r.wg.Add(1)
	go r.processPacketChannel(packetChan)

	return r
}

// processPacketChannel processes packets from a channel (from packet capture)
func (r *RTPReceiver) processPacketChannel(packetChan <-chan []byte) {
	defer r.wg.Done()

	for {
		select {
		case <-r.ctx.Done():
			return
		case packet, ok := <-packetChan:
			if !ok {
				return
			}

			// Parse RTP packet
			rtpPacket, err := r.parseRTPPacket(packet)
			if err != nil {
				r.logger.Debug("Failed to parse captured packet", zap.Error(err))
				continue
			}

			// Send to packet channel (non-blocking)
			select {
			case r.packetChan <- rtpPacket:
			default:
				// Channel full, drop packet
				r.logger.Debug("RTP packet channel full, dropping packet")
			}
		}
	}
}

// SetSSRCMapping sets the mapping from SSRC to user ID
func (r *RTPReceiver) SetSSRCMapping(ssrc uint32, userID string) {
	r.ssrcMu.Lock()
	defer r.ssrcMu.Unlock()
	r.ssrcMap[ssrc] = userID
	r.logger.Debug("Set SSRC mapping", zap.Uint32("ssrc", ssrc), zap.String("user_id", userID))
}

// GetUserIDFromSSRC returns the user ID for a given SSRC
func (r *RTPReceiver) GetUserIDFromSSRC(ssrc uint32) (string, bool) {
	r.ssrcMu.RLock()
	defer r.ssrcMu.RUnlock()
	userID, exists := r.ssrcMap[ssrc]
	return userID, exists
}

// Start begins receiving RTP packets
func (r *RTPReceiver) Start() {
	r.wg.Add(1)
	go r.receiveLoop()
}

// Stop stops receiving RTP packets
func (r *RTPReceiver) Stop() {
	r.cancel()
	r.wg.Wait()
}

// GetPacketChan returns the channel for receiving RTP packets
func (r *RTPReceiver) GetPacketChan() <-chan *RTPPacket {
	return r.packetChan
}

// receiveLoop is the main loop for receiving RTP packets
func (r *RTPReceiver) receiveLoop() {
	defer r.wg.Done()

	buffer := make([]byte, 1500) // Standard MTU size

	for {
		select {
		case <-r.ctx.Done():
			return
		default:
			// Set read deadline to allow context cancellation
			r.udpConn.SetReadDeadline(time.Now().Add(1 * time.Second))

			n, err := r.udpConn.Read(buffer)
			if err != nil {
				if errors.Is(err, net.ErrClosed) {
					r.logger.Debug("UDP connection closed, stopping receiver")
					return
				}
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					// Timeout is expected, continue
					continue
				}
				if r.ctx.Err() != nil {
					// Context cancelled
					return
				}
				// Also check for the string error just in case
				if err.Error() == "use of closed network connection" {
					r.logger.Debug("UDP connection closed (string check), stopping receiver")
					return
				}

				r.logger.Error("Error reading UDP packet", zap.Error(err))
				continue
			}

			// Parse RTP packet
			packet, err := r.parseRTPPacket(buffer[:n])
			if err != nil {
				r.logger.Debug("Failed to parse RTP packet", zap.Error(err))
				continue
			}

			// Send to packet channel (non-blocking)
			select {
			case r.packetChan <- packet:
			default:
				// Channel full, drop packet
				r.logger.Debug("RTP packet channel full, dropping packet")
			}
		}
	}
}

// parseRTPPacket parses an RTP packet according to RFC 3550
func (r *RTPReceiver) parseRTPPacket(data []byte) (*RTPPacket, error) {
	if len(data) < 12 {
		return nil, fmt.Errorf("RTP packet too short: %d bytes", len(data))
	}

	// RTP Header (12 bytes minimum)
	// 0                   1                   2                   3
	// 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
	// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	// |V=2|P|X|  CC   |M|     PT      |       sequence number         |
	// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	// |                           timestamp                             |
	// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	// |           synchronization source (SSRC) identifier              |
	// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+

	version := (data[0] >> 6) & 0x3
	if version != 2 {
		return nil, fmt.Errorf("invalid RTP version: %d", version)
	}

	padding := (data[0] >> 5) & 0x1
	extension := (data[0] >> 4) & 0x1
	cc := data[0] & 0xF      // CSRC count
	_ = (data[1] >> 7) & 0x1 // marker (unused but part of RTP header)
	_ = data[1] & 0x7F       // payloadType (unused but part of RTP header)

	seqNum := binary.BigEndian.Uint16(data[2:4])
	timestamp := binary.BigEndian.Uint32(data[4:8])
	ssrc := binary.BigEndian.Uint32(data[8:12])

	// Calculate header length
	headerLen := 12 + (4 * int(cc)) // Base header + CSRCs
	if extension == 1 {
		if len(data) < headerLen+4 {
			return nil, fmt.Errorf("RTP packet too short for extension header")
		}
		extLen := int(binary.BigEndian.Uint16(data[headerLen+2:headerLen+4])) * 4
		headerLen += 4 + extLen
	}

	// Extract payload
	payloadStart := headerLen
	if padding == 1 && len(data) > 0 {
		paddingLen := int(data[len(data)-1])
		payloadEnd := len(data) - paddingLen
		if payloadEnd > payloadStart {
			payload := make([]byte, payloadEnd-payloadStart)
			copy(payload, data[payloadStart:payloadEnd])
			return &RTPPacket{
				SSRC:      ssrc,
				Payload:   payload,
				Header:    data[:headerLen],
				SeqNum:    seqNum,
				Timestamp: timestamp,
			}, nil
		}
	}

	payload := make([]byte, len(data)-payloadStart)
	copy(payload, data[payloadStart:])

	return &RTPPacket{
		SSRC:      ssrc,
		Payload:   payload,
		Header:    data[:headerLen],
		SeqNum:    seqNum,
		Timestamp: timestamp,
	}, nil
}
