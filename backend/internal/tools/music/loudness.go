package music

import (
	"encoding/binary"
	"math"
)

const (
	// TargetLUFS is the target loudness level (-14 LUFS, Spotify standard)
	TargetLUFS = -14.0

	// MaxGainDB is the maximum gain adjustment allowed (prevent extreme amplification)
	MaxGainDB = 12.0

	// MinGainDB is the minimum gain adjustment allowed (prevent extreme attenuation)
	MinGainDB = -12.0

	// SamplesPerFrame is the number of samples per Opus frame at 48kHz/20ms
	SamplesPerFrame = 960

	// AnalysisFrames is the number of frames to analyze (~1 second at 20ms/frame)
	AnalysisFrames = 50

	// ReferencePacketSize is the expected average packet size for -14 LUFS content
	// Based on empirical observation: ~100-150 bytes for typical YouTube music at 128kbps
	ReferencePacketSize = 130.0

	// ReferenceBitrate is the reference bitrate in kbps for normalization calculations
	ReferenceBitrate = 128.0
)

// CalculateRMSdB analyzes PCM samples (int16) and returns RMS level in dB
// Returns the RMS value relative to full scale (0 dBFS = max int16)
func CalculateRMSdB(samples []int16) float64 {
	if len(samples) == 0 {
		return -96.0 // Silence
	}

	var sumSquares float64
	for _, sample := range samples {
		// Normalize to -1.0 to 1.0 range
		normalized := float64(sample) / 32768.0
		sumSquares += normalized * normalized
	}

	rms := math.Sqrt(sumSquares / float64(len(samples)))
	if rms < 1e-10 {
		return -96.0 // Effectively silence
	}

	// Convert to dB (relative to full scale)
	return 20.0 * math.Log10(rms)
}

// CalculateRMSdBFloat32 analyzes PCM samples (float32) and returns RMS level in dB
func CalculateRMSdBFloat32(samples []float32) float64 {
	if len(samples) == 0 {
		return -96.0
	}

	var sumSquares float64
	for _, sample := range samples {
		sumSquares += float64(sample) * float64(sample)
	}

	rms := math.Sqrt(sumSquares / float64(len(samples)))
	if rms < 1e-10 {
		return -96.0
	}

	return 20.0 * math.Log10(rms)
}

// CalculateGainDB calculates the gain (in dB) needed to reach target loudness
// currentRMSdB: current RMS level in dB
// targetLUFS: target loudness (e.g., -14 LUFS)
// Returns gain in dB, clamped to safe range
func CalculateGainDB(currentRMSdB float64, targetLUFS float64) float64 {
	// LUFS is approximately RMS dB for music content
	// The relationship isn't exact, but close enough for normalization
	gainNeeded := targetLUFS - currentRMSdB

	// Clamp to safe range to prevent extreme adjustments
	if gainNeeded > MaxGainDB {
		gainNeeded = MaxGainDB
	}
	if gainNeeded < MinGainDB {
		gainNeeded = MinGainDB
	}

	return gainNeeded
}

// GainDBToQ78 converts a dB gain value to Opus Q7.8 fixed-point format
// Q7.8 means 7 bits for integer part, 8 bits for fractional part (signed)
// Formula: Q7.8_value = gainDB * 256
// The Opus decoder applies: linear_gain = 10^(Q7.8_value / (256 * 20))
func GainDBToQ78(gainDB float64) int16 {
	// Q7.8 format: multiply by 256 to get the fixed-point representation
	q78 := gainDB * 256.0

	// Clamp to int16 range (though in practice gain is much smaller)
	if q78 > 32767 {
		q78 = 32767
	}
	if q78 < -32768 {
		q78 = -32768
	}

	return int16(q78)
}

// SetOpusOutputGain modifies the Output Gain field in an OpusHead header
// opusHead: the OpusHead byte slice (at least 19 bytes)
// gainDB: the gain to apply in dB
// Returns true if successful
func SetOpusOutputGain(opusHead []byte, gainDB float64) bool {
	// OpusHead structure:
	// Bytes 0-7:   "OpusHead" magic
	// Byte 8:      Version (1)
	// Byte 9:      Channel count
	// Bytes 10-11: Pre-skip (little-endian)
	// Bytes 12-15: Input sample rate (little-endian)
	// Bytes 16-17: Output gain (Q7.8, little-endian) <-- This is what we modify
	// Byte 18:     Channel mapping family

	if len(opusHead) < 19 {
		return false
	}

	// Verify it's an OpusHead
	if string(opusHead[0:8]) != "OpusHead" {
		return false
	}

	// Convert gain to Q7.8 and write to bytes 16-17 (little-endian)
	q78Gain := GainDBToQ78(gainDB)
	binary.LittleEndian.PutUint16(opusHead[16:18], uint16(q78Gain))

	return true
}

// GetOpusOutputGain reads the current Output Gain from an OpusHead header
// Returns the gain in dB
func GetOpusOutputGain(opusHead []byte) float64 {
	if len(opusHead) < 18 {
		return 0.0
	}

	q78Gain := int16(binary.LittleEndian.Uint16(opusHead[16:18]))
	return float64(q78Gain) / 256.0
}

// EstimateLoudnessFromPackets estimates loudness from Opus packet data without decoding
// This is a heuristic approach that analyzes packet characteristics
// Returns estimated RMS in dB (relative to reference level)
func EstimateLoudnessFromPackets(packets [][]byte) float64 {
	if len(packets) == 0 {
		return -20.0 // Default assumption for unknown content
	}

	var totalSize int
	var totalByteEnergy float64
	var validPackets int

	for _, packet := range packets {
		if len(packet) < 2 {
			continue
		}

		totalSize += len(packet)
		validPackets++

		// Analyze byte energy (variance from midpoint)
		// Higher variance in byte values often correlates with louder audio
		var byteSum float64
		for _, b := range packet[1:] { // Skip TOC byte
			// Measure deviation from 128 (midpoint)
			deviation := float64(b) - 128.0
			byteSum += deviation * deviation
		}
		if len(packet) > 1 {
			totalByteEnergy += math.Sqrt(byteSum / float64(len(packet)-1))
		}
	}

	if validPackets == 0 {
		return -20.0
	}

	// Calculate average packet size
	avgPacketSize := float64(totalSize) / float64(validPackets)

	// Calculate average byte energy
	avgByteEnergy := totalByteEnergy / float64(validPackets)

	// Heuristic: Combine packet size and byte energy metrics
	// Larger packets and higher byte energy suggest louder content
	
	// Packet size factor: compare to reference size
	// Log scale to convert to dB-like value
	sizeFactor := 20.0 * math.Log10(avgPacketSize/ReferencePacketSize+0.01)

	// Byte energy factor: normalize to a dB-like scale
	// Reference energy ~40-60 for typical music
	energyFactor := 20.0 * math.Log10(avgByteEnergy/50.0+0.01)

	// Combine factors with weighting
	// Energy is more reliable indicator, size is secondary
	estimatedRMS := (energyFactor*0.7 + sizeFactor*0.3) - 14.0

	// Clamp to reasonable range
	if estimatedRMS > 0 {
		estimatedRMS = 0
	}
	if estimatedRMS < -40 {
		estimatedRMS = -40
	}

	return estimatedRMS
}

// GetOpusFrameInfo extracts information from an Opus TOC byte
// Returns: mode (0=SILK, 1=Hybrid, 2=CELT), bandwidth, frameSize in ms
func GetOpusFrameInfo(tocByte byte) (mode int, bandwidth int, frameSizeMs float64) {
	config := (tocByte >> 3) & 0x1F

	// Determine mode and bandwidth from configuration
	switch {
	case config <= 11: // SILK-only
		mode = 0
		if config <= 3 {
			bandwidth = 0 // Narrowband
		} else if config <= 7 {
			bandwidth = 1 // Medium-band
		} else {
			bandwidth = 2 // Wideband
		}
	case config <= 15: // Hybrid
		mode = 1
		if config <= 13 {
			bandwidth = 3 // Super-wideband
		} else {
			bandwidth = 4 // Fullband
		}
	default: // CELT-only
		mode = 2
		if config <= 19 {
			bandwidth = 0 // Narrowband
		} else if config <= 23 {
			bandwidth = 2 // Wideband
		} else if config <= 27 {
			bandwidth = 3 // Super-wideband
		} else {
			bandwidth = 4 // Fullband
		}
	}

	// Frame size from configuration
	frameCode := config & 0x03
	switch mode {
	case 0: // SILK
		frameSizes := []float64{10, 20, 40, 60}
		frameSizeMs = frameSizes[frameCode]
	case 1: // Hybrid
		frameSizes := []float64{10, 20, 10, 20}
		frameSizeMs = frameSizes[frameCode]
	case 2: // CELT
		frameSizes := []float64{2.5, 5, 10, 20}
		frameSizeMs = frameSizes[frameCode]
	}

	return mode, bandwidth, frameSizeMs
}

