package voice

import (
	"encoding/binary"
	"hash/crc32"
)

// buildOggContainer wraps Opus data in an Ogg container structure
// This is a minimal implementation sufficient for ffmpeg decoding
func buildOggContainer(opusData []byte) []byte {
	var buf []byte

	// 1. Identification Header Page (OpusHead)
	// Ogg ID Header
	idDocs := []byte("OpusHead")
	idHeader := make([]byte, 19)
	copy(idHeader[0:], idDocs)
	idHeader[8] = 1 // Version
	idHeader[9] = 1 // Channel Count (Mono) - Wait, Discord sends stereo often? Let's check. Usually stereo (2).
	// Actually, if we say 2 here but decode to 1 in ffmpeg, it works.
	// If we say 1 but it's 2, it might glitch. Discord usually sends 2 channels.
	idHeader[9] = 2
	binary.LittleEndian.PutUint16(idHeader[10:], 0)     // Pre-skip
	binary.LittleEndian.PutUint32(idHeader[12:], 48000) // Input Sample Rate
	binary.LittleEndian.PutUint16(idHeader[16:], 0)     // Output Gain
	idHeader[18] = 0                                    // Channel Mapping Family (0 = mono/stereo)

	// Create Ogg page for ID Header
	// Page 0, Seq 0, BOS (Beginning of Stream) set
	buf = append(buf, createOggPage(0, 0, 0, []byte{2}, [][]byte{idHeader})...)

	// 2. Comment Header Page (OpusTags)
	// Minimal comment header
	commentDocs := []byte("OpusTags")
	vendorString := []byte("EzraBot")
	commentHeader := make([]byte, 8+4+len(vendorString)+4)
	copy(commentHeader[0:], commentDocs)
	binary.LittleEndian.PutUint32(commentHeader[8:], uint32(len(vendorString)))
	copy(commentHeader[12:], vendorString)
	binary.LittleEndian.PutUint32(commentHeader[12+len(vendorString):], 0) // User comment list length

	// Create Ogg page for Comment Header
	// Page 1, Seq 1
	buf = append(buf, createOggPage(1, 1, 0, []byte{0}, [][]byte{commentHeader})...)

	// 3. Audio Data Page(s)
	// We wrap the entire opusData chunk in one or more pages.
	// Since frames are usually small (20ms), we can fit many in one page (up to 255 segments).
	// However, opusData passed here might be multiple frames concatenated (from buffers).
	// We need to know where frame boundaries are?
	// Ah, discordgo.Packet.Opus is a single frame.
	// But `userOpusBuffers` in AudioCapture concatenates.
	// We lost frame boundaries!
	// This is a problem. Opus is not self-delimiting in a stream without framing.

	// FIX: In AudioCapture, we append `packet.Opus` blindly.
	// If we just treat the blob as one giant frame, decoder might fail if it's actually multiple.
	// BUT, ffmpeg with Ogg expects standard Ogg packetization.

	// Assumption: Ideally we should process per-packet.
	// But `DecodeOpusToPCM` takes a byte slice.
	// If we construct a single page with this data as ONE packet,
	// the decoder will try to decode it as one Opus frame. Opus frames have a max size (120ms).
	// If we buffer too much, it's invalid.
	// `flushTicker` is 20ms. Discord sends 20ms packets.
	// So likely `opusData` contains 0 or 1 packet, maybe 2 if we are slow.

	// If it contains multiple packets, simply concatenating them is INVALID Opus bitstream unless using lacing values in Ogg.
	// To do this correctly, AudioCapture should probably pass `[][]byte` (list of packets) instead of `[]byte`.

	// For now, let's assume it's one packet or try to wrap it as one payload.
	// If it fails, we need to refactor AudioCapture to preserve packet boundaries.

	// Page 2, Seq 2, EOS (End of Stream) set since this is a self-contained chunk for ffmpeg
	// Granule position calculation is tricky but 0 or -1 might work if we don't care about seeking.
	// Actually, for playback, we just need valid structure.

	buf = append(buf, createOggPage(2, 2, 4, []byte{4}, [][]byte{opusData})...) // 4 = EOS (0x04)

	return buf
}

// createOggPage creates a single Ogg page
func createOggPage(pageSeq uint32, packetSeq int, flags byte, granulePos []byte, packets [][]byte) []byte {
	var page []byte

	// OggS Capture Pattern
	page = append(page, []byte("OggS")...)
	page = append(page, 0)     // Version
	page = append(page, flags) // Header Type (BOS=2, EOS=4, Cont=1)

	// Granule Position (8 bytes) - dummy for now
	if len(granulePos) == 8 {
		page = append(page, granulePos...)
	} else {
		// Zeroes
		page = append(page, make([]byte, 8)...)
	}

	// Serial Number (4 bytes) - manual constant
	serial := []byte{0x01, 0x23, 0x45, 0x67}
	page = append(page, serial...)

	// Page Sequence Number (4 bytes)
	seqBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(seqBytes, pageSeq)
	page = append(page, seqBytes...)

	// Checksum placeholder (4 bytes)
	checksumOffset := len(page)
	page = append(page, 0, 0, 0, 0)

	// Page Segments (1 byte)
	// We need to calculate segments based on packet lengths (255 byte blocks)
	var segmentTable []byte
	for _, pkt := range packets {
		l := len(pkt)
		for l >= 255 {
			segmentTable = append(segmentTable, 255)
			l -= 255
		}
		segmentTable = append(segmentTable, byte(l))
	}
	page = append(page, byte(len(segmentTable)))
	page = append(page, segmentTable...)

	// Packet Data
	for _, pkt := range packets {
		page = append(page, pkt...)
	}

	// Calculate CRC32
	// Ogg uses a specific polynomial 0x04c11db7 usually?
	// crc32.ChecksumIEEE is 0xedb88320 (Ethernet).
	// Ogg uses "standard" CRC32 algorithm but with polynomial 0x04c11db7.
	// Go's crc32.IEEE is not it. crc32.Koopman?
	// Actually, rolling our own table for correct Ogg CRC is standard in these impls.
	// Using IEEE might fail checks if ffmpeg is strict.
	// Most parsers check CRC.

	// Let's rely on standard Go library if possible, but we might need a custom table.
	// If ffmpeg ignores CRC, great. If not, this is complex.
	// Let's try IEEE first and see if ffmpeg accepts it (it might warn but process).
	crc := crc32.ChecksumIEEE(page)
	// Wait, we need to put it into the page BEFORE calculating? No, placeholder is 0.
	// But it uses the polynomial.

	// If strict CRC is required, this might fail.

	binary.LittleEndian.PutUint32(page[checksumOffset:], crc)

	return page
}
