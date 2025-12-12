package music

import (
	"encoding/binary"
	"io"
)

// WebMDemuxer extracts Opus audio packets from WebM container
type WebMDemuxer struct {
	reader       io.Reader
	trackNumber  int
	opusCodecID  string
	initialized  bool
	buffer       []byte
	bufferPos    int
	codecPrivate []byte
	headersSent  bool
	pageSeq      uint32
	granulePos   uint64
	// Seek support
	seekTargetMs  int64 // Target seek position in milliseconds (-1 = no seek)
	clusterTimeMs int64 // Current cluster timestamp in milliseconds
	currentTimeMs int64 // Current playback position in milliseconds
	seeking       bool  // Are we currently skipping frames?
	seekReady     bool  // Have we reached the seek target?
	// Loudness normalization
	analyzePackets [][]byte // Buffer for packets to analyze
	analyzed       bool     // Whether loudness analysis is complete
	outputGainDB   float64  // Calculated output gain in dB
	normalizeAudio bool     // Whether to apply normalization
}

// NewWebMDemuxer creates a new WebM demuxer with loudness normalization enabled
func NewWebMDemuxer(reader io.Reader) *WebMDemuxer {
	return &WebMDemuxer{
		reader:         reader,
		trackNumber:    -1,
		opusCodecID:    "A_OPUS",
		initialized:    false,
		buffer:         make([]byte, 0, 8192),
		bufferPos:      0,
		codecPrivate:   nil,
		headersSent:    false,
		pageSeq:        0,
		granulePos:     0,
		seekTargetMs:   -1,
		clusterTimeMs:  0,
		currentTimeMs:  0,
		seeking:        false,
		seekReady:      true,
		analyzePackets: make([][]byte, 0, AnalysisFrames),
		analyzed:       false,
		outputGainDB:   0,
		normalizeAudio: true, // Enable normalization by default
	}
}

// NewWebMDemuxerNoNormalize creates a WebM demuxer without loudness normalization
func NewWebMDemuxerNoNormalize(reader io.Reader) *WebMDemuxer {
	d := NewWebMDemuxer(reader)
	d.normalizeAudio = false
	d.analyzed = true // Skip analysis
	return d
}

// NewWebMDemuxerWithSeek creates a WebM demuxer that seeks to a position
func NewWebMDemuxerWithSeek(reader io.Reader, seekSeconds int) *WebMDemuxer {
	d := NewWebMDemuxer(reader)
	if seekSeconds > 0 {
		d.seekTargetMs = int64(seekSeconds) * 1000
		d.seeking = true
		d.seekReady = false
	}
	return d
}

// analyzeLoudness analyzes buffered Opus packets and calculates the required gain
// Uses heuristic estimation from packet characteristics (works with all Opus modes)
func (d *WebMDemuxer) analyzeLoudness() {
	if len(d.analyzePackets) == 0 {
		d.outputGainDB = 0
		d.analyzed = true
		return
	}

	// Analyze Opus packets for loudness
	if len(d.analyzePackets) > 0 && len(d.analyzePackets[0]) > 0 {
		GetOpusFrameInfo(d.analyzePackets[0][0])
	}

	// Use heuristic estimation (works with all Opus modes including CELT)
	estimatedRMSdB := EstimateLoudnessFromPackets(d.analyzePackets)

	// Calculate required gain to reach target LUFS
	d.outputGainDB = CalculateGainDB(estimatedRMSdB, TargetLUFS)
	d.analyzed = true
}

func (d *WebMDemuxer) Read(p []byte) (n int, err error) {
	if !d.initialized {
		if err := d.initialize(); err != nil {
			return 0, err
		}
		if len(d.codecPrivate) < 19 {
			d.codecPrivate = []byte{'O', 'p', 'u', 's', 'H', 'e', 'a', 'd', 1, 2, 0x00, 0x0F, 0x80, 0xBB, 0, 0, 0, 0, 0}
		}
		d.initialized = true
	}

	// If normalization is enabled and we haven't analyzed yet, buffer packets first
	if d.normalizeAudio && !d.analyzed {
		// Read packets until we have enough for analysis
		for len(d.analyzePackets) < AnalysisFrames {
			packets, err := d.readNextOpusPackets()
			if err != nil {
				// If we hit EOF before getting enough packets, analyze what we have
				if err == io.EOF {
					break
				}
				return 0, err
			}
			for _, pkt := range packets {
				if len(pkt) > 0 {
					// Make a copy of the packet
					pktCopy := make([]byte, len(pkt))
					copy(pktCopy, pkt)
					d.analyzePackets = append(d.analyzePackets, pktCopy)
					if len(d.analyzePackets) >= AnalysisFrames {
						break
					}
				}
			}
		}

		// Analyze the buffered packets
		d.analyzeLoudness()

		if d.outputGainDB != 0 && len(d.codecPrivate) >= 19 {
			SetOpusOutputGain(d.codecPrivate, d.outputGainDB)
		}
	}

	// Send OGG Headers first
	if !d.headersSent {
		// Page 0: OpusHead (with output gain if normalization was applied)
		d.buffer = append(d.buffer, d.createOGGPage(d.codecPrivate, 0, true, false)...)

		// Page 1: OpusTags
		vendor := []byte("GoWebMDemuxer")
		tags := make([]byte, 8+4+len(vendor)+4)
		copy(tags[0:], "OpusTags")
		binary.LittleEndian.PutUint32(tags[8:], uint32(len(vendor)))
		copy(tags[12:], vendor)

		d.buffer = append(d.buffer, d.createOGGPage(tags, 0, false, false)...)
		d.headersSent = true

		// If we buffered packets for analysis, emit them now
		if len(d.analyzePackets) > 0 {
			for _, packet := range d.analyzePackets {
				d.granulePos += 960
				d.buffer = append(d.buffer, d.createOGGPage(packet, d.granulePos, false, false)...)
			}
			// Clear the analyze buffer
			d.analyzePackets = nil
		}
	}

	// Flush buffer
	if d.bufferPos < len(d.buffer) {
		n = copy(p, d.buffer[d.bufferPos:])
		d.bufferPos += n
		if d.bufferPos >= len(d.buffer) {
			d.buffer = d.buffer[:0]
			d.bufferPos = 0
		}
		return n, nil
	}

	packets, err := d.readNextOpusPackets()
	if err != nil {
		return 0, err
	}

	// Encapsulate
	for _, packet := range packets {
		if len(packet) == 0 {
			continue
		}
		// 960 samples = 20ms @ 48kHz
		d.granulePos += 960
		d.buffer = append(d.buffer, d.createOGGPage(packet, d.granulePos, false, false)...)
	}

	// Output
	if d.bufferPos < len(d.buffer) {
		n = copy(p, d.buffer[d.bufferPos:])
		d.bufferPos += n
		if d.bufferPos >= len(d.buffer) {
			d.buffer = d.buffer[:0]
			d.bufferPos = 0
		}
		return n, nil
	}
	return 0, nil
}

func (d *WebMDemuxer) createOGGPage(packet []byte, granulePos uint64, bos bool, eos bool) []byte {
	segCount := (len(packet) + 255 - 1) / 255
	if segCount < 1 {
		segCount = 1
	}

	totalSize := 27 + segCount + len(packet)
	page := make([]byte, totalSize)

	copy(page[0:], "OggS")
	var flags byte
	if bos {
		flags |= 0x02
	}
	if eos {
		flags |= 0x04
	}
	page[5] = flags

	binary.LittleEndian.PutUint64(page[6:], granulePos)
	binary.LittleEndian.PutUint32(page[14:], 123456) // Serial
	binary.LittleEndian.PutUint32(page[18:], d.pageSeq)
	d.pageSeq++

	page[26] = byte(segCount)

	remaining := len(packet)
	segIdx := 27
	dataIdx := 27 + segCount

	for remaining > 0 {
		sz := remaining
		if sz > 255 {
			sz = 255
		}
		page[segIdx] = byte(sz)
		segIdx++
		copy(page[dataIdx:], packet[len(packet)-remaining:len(packet)-remaining+sz])
		dataIdx += sz
		remaining -= sz
	}

	crc := updateOggCRC(0, page)
	binary.LittleEndian.PutUint32(page[22:], crc)
	return page
}

var crcTable [256]uint32

func init() {
	const poly = 0x04c11db7
	for i := 0; i < 256; i++ {
		r := uint32(i) << 24
		for j := 0; j < 8; j++ {
			if r&0x80000000 != 0 {
				r = (r << 1) ^ poly
			} else {
				r <<= 1
			}
		}
		crcTable[i] = r
	}
}

func updateOggCRC(crc uint32, data []byte) uint32 {
	for _, b := range data {
		crc = (crc << 8) ^ crcTable[byte(crc>>24)^b]
	}
	return crc
}

// --- WebM Parsing ---

func (d *WebMDemuxer) initialize() error {
	for {
		id, size, err := readEBMLElement(d.reader)
		if err != nil {
			return err
		}

		if id == 0x1A45DFA3 { // Header
			io.CopyN(io.Discard, d.reader, int64(size))
		} else if id == 0x18538067 { // Segment
			return d.readSegmentHeader(size)
		} else {
			io.CopyN(io.Discard, d.reader, int64(size))
		}
	}
}

func (d *WebMDemuxer) readSegmentHeader(size uint64) error {
	remaining := int64(size)
	if remaining < 0 {
		remaining = 1 << 62
	}

	for remaining > 0 {
		id, sz, err := readEBMLElement(d.reader)
		if err != nil {
			return err
		}

		consumed := int64(sz) + int64(getEBMLSizeLength(sz)) + int64(getEBMLIDLength(id))
		remaining -= consumed

		if id == 0x1654AE6B { // Tracks
			return d.readTracks(sz)
		} else {
			io.CopyN(io.Discard, d.reader, int64(sz))
		}
	}
	return nil
}

func (d *WebMDemuxer) readTracks(size uint64) error {
	remaining := int64(size)
	for remaining > 0 {
		id, sz, err := readEBMLElement(d.reader)
		if err != nil {
			return err
		}

		consumed := int64(sz) + int64(getEBMLSizeLength(sz)) + int64(getEBMLIDLength(id))
		remaining -= consumed

		if id == 0xAE { // TrackEntry
			tNum, codec, priv, err := d.readTrackEntry(sz)
			if err != nil {
				return err
			}

			if codec == d.opusCodecID {
				d.trackNumber = tNum
				d.codecPrivate = priv
				if remaining > 0 {
					io.CopyN(io.Discard, d.reader, remaining)
				}
				return nil
			}
		} else {
			io.CopyN(io.Discard, d.reader, int64(sz))
		}
	}
	return nil
}

func (d *WebMDemuxer) readTrackEntry(size uint64) (int, string, []byte, error) {
	remaining := int64(size)
	tNum := -1
	codec := ""
	var priv []byte

	for remaining > 0 {
		id, sz, err := readEBMLElement(d.reader)
		if err != nil {
			return 0, "", nil, err
		}

		consumed := int64(sz) + int64(getEBMLSizeLength(sz)) + int64(getEBMLIDLength(id))
		remaining -= consumed

		if id == 0xD7 {
			b := make([]byte, sz)
			io.ReadFull(d.reader, b)
			tNum = int(readEBMLUint(b))
		} else if id == 0x86 {
			b := make([]byte, sz)
			io.ReadFull(d.reader, b)
			codec = string(b)
		} else if id == 0x63A2 {
			priv = make([]byte, sz)
			io.ReadFull(d.reader, priv)
		} else {
			io.CopyN(io.Discard, d.reader, int64(sz))
		}
	}
	return tNum, codec, priv, nil
}

func (d *WebMDemuxer) readNextOpusPackets() ([][]byte, error) {
	for {
		id, size, err := readEBMLElement(d.reader)
		if err != nil {
			return nil, err
		}

		if id == 0x1F43B675 { // Cluster
			packets, found, err := d.readCluster(size)
			if err != nil {
				if err == io.EOF {
					continue
				}
				return nil, err
			}
			if found && len(packets) > 0 {
				return packets, nil
			}
		} else if id == 0x1654AE6B { // Tracks re-appearance
			d.readTracks(size)
		} else {
			io.CopyN(io.Discard, d.reader, int64(size))
		}
	}
}

func (d *WebMDemuxer) readCluster(size uint64) ([][]byte, bool, error) {
	remaining := int64(size)
	unknown := (size == 0xFFFFFFFFFFFFFFFF)
	var allPackets [][]byte

	for remaining > 0 || unknown {
		id, sz, err := readEBMLElement(d.reader)
		if err != nil {
			if err == io.EOF && unknown {
				return allPackets, true, nil
			}
			return nil, false, err
		}

		if !unknown {
			consumed := int64(sz) + int64(getEBMLSizeLength(sz)) + int64(getEBMLIDLength(id))
			remaining -= consumed
			if remaining < 0 {
				remaining = 0
			}
		}

		if id == 0xE7 { // Cluster Timecode
			b := make([]byte, sz)
			io.ReadFull(d.reader, b)
			d.clusterTimeMs = int64(readEBMLUint(b))
		} else if id == 0xA3 { // SimpleBlock
			pkts, found, err := d.readSimpleBlock(sz)
			if err != nil {
				if err == io.EOF {
					return allPackets, true, nil
				}
				return nil, false, err
			}
			if found {
				allPackets = append(allPackets, pkts...)
			}
		} else if id == 0xA0 { // BlockGroup
			pkts, found, err := d.readBlockGroup(sz)
			if err != nil {
				if err == io.EOF {
					return allPackets, true, nil
				}
				return nil, false, err
			}
			if found {
				allPackets = append(allPackets, pkts...)
			}
		} else {
			io.CopyN(io.Discard, d.reader, int64(sz))
		}
	}
	return allPackets, true, nil
}

func (d *WebMDemuxer) readBlockGroup(size uint64) ([][]byte, bool, error) {
	remaining := int64(size)
	for remaining > 0 {
		id, sz, err := readEBMLElement(d.reader)
		if err != nil {
			return nil, false, err
		}

		consumed := int64(sz) + int64(getEBMLSizeLength(sz)) + int64(getEBMLIDLength(id))
		remaining -= consumed

		if id == 0xA1 {
			return d.readSimpleBlock(sz)
		} else {
			io.CopyN(io.Discard, d.reader, int64(sz))
		}
	}
	return nil, false, nil
}

func (d *WebMDemuxer) readSimpleBlock(size uint64) ([][]byte, bool, error) {
	if d.trackNumber < 0 {
		io.CopyN(io.Discard, d.reader, int64(size))
		return nil, false, nil
	}

	tNum, tLen, err := readEBMLVarInt(d.reader)
	if err != nil {
		return nil, false, err
	}

	if int(tNum) != d.trackNumber {
		io.CopyN(io.Discard, d.reader, int64(size)-int64(tLen))
		return nil, false, nil
	}

	// Read block timecode (relative to cluster, signed 16-bit)
	timecodeBytes := make([]byte, 2)
	io.ReadFull(d.reader, timecodeBytes)
	blockTimecode := int16(binary.BigEndian.Uint16(timecodeBytes))

	// Calculate absolute timestamp
	d.currentTimeMs = d.clusterTimeMs + int64(blockTimecode)

	flags := make([]byte, 1)
	io.ReadFull(d.reader, flags)
	lacing := (flags[0] & 0x06) >> 1

	headerSize := int64(tLen) + 3
	dataSize := int64(size) - headerSize
	if dataSize <= 0 {
		return nil, false, nil
	}

	// Check if we're seeking and haven't reached target yet
	if d.seeking && d.seekTargetMs > 0 {
		if d.currentTimeMs < d.seekTargetMs {
			// Skip this block - we haven't reached seek target
			io.CopyN(io.Discard, d.reader, dataSize)
			return nil, false, nil
		} else {
			// We've reached the seek target!
			if !d.seekReady {
				d.seekReady = true
				d.seeking = false
			}
		}
	}

	if lacing == 0 {
		pkt := make([]byte, dataSize)
		io.ReadFull(d.reader, pkt)
		return [][]byte{pkt}, true, nil
	}

	// Handle Lacing
	frameCountByte := make([]byte, 1)
	io.ReadFull(d.reader, frameCountByte)
	numFrames := int(frameCountByte[0]) + 1
	dataSize--

	var packets [][]byte

	if lacing == 1 { // Xiph
		sizes := make([]int, numFrames)
		totalLacingBytes := 0
		for i := 0; i < numFrames-1; i++ {
			sz := 0
			for {
				b := make([]byte, 1)
				io.ReadFull(d.reader, b)
				totalLacingBytes++
				sz += int(b[0])
				if b[0] < 255 {
					break
				}
			}
			sizes[i] = sz
		}

		used := 0
		for i := 0; i < numFrames-1; i++ {
			used += sizes[i]
		}
		sizes[numFrames-1] = int(dataSize) - totalLacingBytes - used

		for i := 0; i < numFrames; i++ {
			p := make([]byte, sizes[i])
			io.ReadFull(d.reader, p)
			packets = append(packets, p)
		}
		return packets, true, nil
	}

	// Skip complex lacing
	io.CopyN(io.Discard, d.reader, dataSize)
	return nil, false, nil
}

// --- Low Level EBML ---

func readEBMLElement(r io.Reader) (uint32, uint64, error) {
	b := make([]byte, 1)
	if _, err := io.ReadFull(r, b); err != nil {
		return 0, 0, err
	}

	first := b[0]
	idLen := 1
	if first&0x80 == 0 {
		if first&0x40 != 0 {
			idLen = 2
		} else if first&0x20 != 0 {
			idLen = 3
		} else {
			idLen = 4
		}
	}

	fullID := make([]byte, idLen)
	fullID[0] = first
	if idLen > 1 {
		io.ReadFull(r, fullID[1:])
	}

	pad := make([]byte, 4)
	copy(pad[4-idLen:], fullID)
	id := binary.BigEndian.Uint32(pad)

	sz, _, err := readEBMLVarInt(r)
	return id, sz, err
}

func getEBMLIDLength(id uint32) int {
	if id >= 0x80 {
		return 1
	}
	if id >= 0x4000 {
		return 2
	}
	if id >= 0x200000 {
		return 3
	}
	return 4
}

func readEBMLVarInt(r io.Reader) (uint64, int, error) {
	b := make([]byte, 1)
	if _, err := io.ReadFull(r, b); err != nil {
		return 0, 0, err
	}

	first := b[0]
	length := 0
	var mask byte = 0x80
	for i := 0; i < 8; i++ {
		if first&mask != 0 {
			length = i + 1
			break
		}
		mask >>= 1
	}
	if length == 0 {
		length = 8
	}

	raw := make([]byte, length)
	raw[0] = first
	if length > 1 {
		io.ReadFull(r, raw[1:])
	}

	raw[0] &= ^byte(0x80 >> (length - 1))

	val := uint64(0)
	for i := 0; i < length; i++ {
		val = (val << 8) | uint64(raw[i])
	}
	return val, length, nil
}

func readEBMLUint(b []byte) uint64 {
	var v uint64
	for _, x := range b {
		v = (v << 8) | uint64(x)
	}
	return v
}

func getEBMLSizeLength(s uint64) int {
	if s < 0x80 {
		return 1
	}
	if s < 0x4000 {
		return 2
	}
	if s < 0x200000 {
		return 3
	}
	if s < 0x10000000 {
		return 4
	}
	if s < 0x800000000 {
		return 5
	}
	if s < 0x40000000000 {
		return 6
	}
	if s < 0x2000000000000 {
		return 7
	}
	return 8
}
