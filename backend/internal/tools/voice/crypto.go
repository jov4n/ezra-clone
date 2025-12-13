package voice

import (
	"crypto/aes"
	"crypto/cipher"
	"fmt"

	"golang.org/x/crypto/nacl/secretbox"
)

// DecryptPacket decrypts an RTP packet based on the encryption mode
func DecryptPacket(packet *RTPPacket, secretKey []byte, mode string) ([]byte, error) {
	if len(secretKey) == 0 {
		return nil, fmt.Errorf("secret key is empty")
	}

	var key [32]byte
	if len(secretKey) == 32 {
		copy(key[:], secretKey)
	} else {
		// Does this happen? Discord sends 32 bytes usually.
		// If it's different, we might be in trouble for xsalsa20
		// For AES-256-GCM, we need 32 bytes too.
		if len(secretKey) != 32 {
			return nil, fmt.Errorf("invalid secret key length: %d", len(secretKey))
		}
		copy(key[:], secretKey)
	}

	switch mode {
	case "xsalsa20_poly1305":
		// Nonce is the RTP header (12 bytes) + 12 zero bytes
		var nonce [24]byte
		if len(packet.Header) > 12 {
			// If header has extensions, we might need to be careful?
			// Standard says generated from header. Usually just the first 12 bytes.
			copy(nonce[:], packet.Header[:12])
		} else {
			copy(nonce[:], packet.Header)
		}

		decrypted, ok := secretbox.Open(nil, packet.Payload, &nonce, &key)
		if !ok {
			return nil, fmt.Errorf("decryption failed")
		}
		return decrypted, nil

	case "xsalsa20_poly1305_suffix":
		// Nonce is the last 24 bytes of the payload
		// Payload = Ciphertext + Nonce
		if len(packet.Payload) < 24 {
			return nil, fmt.Errorf("packet too short for suffix mode")
		}
		ciphertextLen := len(packet.Payload) - 24
		ciphertext := packet.Payload[:ciphertextLen]

		var nonce [24]byte
		copy(nonce[:], packet.Payload[ciphertextLen:])

		decrypted, ok := secretbox.Open(nil, ciphertext, &nonce, &key)
		if !ok {
			return nil, fmt.Errorf("decryption failed")
		}
		return decrypted, nil

	case "xsalsa20_poly1305_lite":
		// Nonce is 4 bytes at the end of the payload + extended to 24 bytes
		if len(packet.Payload) < 4 {
			return nil, fmt.Errorf("packet too short for lite mode")
		}
		ciphertextLen := len(packet.Payload) - 4
		ciphertext := packet.Payload[:ciphertextLen]

		var nonce [24]byte
		// First 4 bytes from end of payload
		copy(nonce[:], packet.Payload[ciphertextLen:])

		decrypted, ok := secretbox.Open(nil, ciphertext, &nonce, &key)
		if !ok {
			return nil, fmt.Errorf("decryption failed")
		}
		return decrypted, nil

	case "aead_aes256_gcm_rtpsize":
		// Uses AES-GCM
		block, err := aes.NewCipher(key[:])
		if err != nil {
			return nil, err
		}

		gcm, err := cipher.NewGCM(block)
		if err != nil {
			return nil, err
		}

		// Nonce is generated from header + extended
		// For this mode, nonce is usually header (12 bytes)
		// NOTE: discordgo/discord.js implementation details vary.
		// Looking at Richy-Z fork or djs:
		// IV is usually the header.
		if len(packet.Header) < 12 {
			return nil, fmt.Errorf("header too short for GCM")
		}

		nonce := make([]byte, 12)
		copy(nonce, packet.Header[:12])

		// Additional Authenticated Data is the header (optional? usually not used in Discord voice)
		// But wait, the standard RTP GCM spec says AAD is header.
		// Discord implementation:
		// nonce = header
		// AAD = header (unclear if Discord checks this)
		// Let's try standard GCM Open with just nonce and ciphertext.

		decrypted, err := gcm.Open(nil, nonce, packet.Payload, nil)
		if err != nil {
			// Try with AAD = header
			decryptedWithAAD, err2 := gcm.Open(nil, nonce, packet.Payload, packet.Header)
			if err2 != nil {
				return nil, fmt.Errorf("decryption failed: %v (and %v)", err, err2)
			}
			return decryptedWithAAD, nil
		}
		return decrypted, nil

	default:
		return nil, fmt.Errorf("unsupported encryption mode: %s", mode)
	}
}
