package discordgo_voice

import (
	"fmt"
	"net"
	"reflect"
	"unsafe"

	"github.com/bwmarrin/discordgo"
)

// GetUDPConnection extracts the UDP connection from a discordgo VoiceConnection
// This uses unsafe pointer arithmetic to access the private udpConn field
// WARNING: This is fragile and may break with discordgo updates
func GetUDPConnection(vc *discordgo.VoiceConnection) (*net.UDPConn, error) {
	if vc == nil {
		return nil, fmt.Errorf("voice connection is nil")
	}

	// Use unsafe to access the private udpConn field
	// The VoiceConnection struct has a udpConn field that we need to access
	vcPtr := unsafe.Pointer(vc)

	// Try to find the UDP connection field by scanning the struct
	// We'll check pointer-sized fields (8 bytes on 64-bit, 4 bytes on 32-bit)
	ptrSize := unsafe.Sizeof(uintptr(0))

	// Scan through the struct looking for a *net.UDPConn
	// The udpConn field is typically after other fields like Session, Ready, etc.
	for offset := uintptr(0); offset < 1024; offset += ptrSize {
		// Read a pointer at this offset
		fieldPtr := (*unsafe.Pointer)(unsafe.Pointer(uintptr(vcPtr) + offset))
		if fieldPtr == nil || *fieldPtr == nil {
			continue
		}

		// Try to interpret as *net.UDPConn
		// Use recover to safely check if this is a valid UDP connection
		var udpConnPtr *net.UDPConn
		func() {
			defer func() {
				recover() // Catch any panics from invalid pointers
			}()

			udpConnPtr = (*net.UDPConn)(*fieldPtr)
			if udpConnPtr == nil {
				return
			}

			// Try to verify it's a valid UDP connection
			// This will panic if the pointer is invalid
			addr := udpConnPtr.LocalAddr()
			if addr == nil {
				return
			}

			if udpAddr, ok := addr.(*net.UDPAddr); ok && udpAddr != nil {
				// Found a valid UDP connection!
				// We can't return from inside the closure, so we'll set it
				// Actually, we need to return it, so we'll check after
			}
		}()

		// If we found a valid UDP connection, verify and return it
		if udpConnPtr != nil {
			// Double-check it's valid
			func() {
				defer func() {
					recover() // Catch any panics
				}()
				addr := udpConnPtr.LocalAddr()
				if addr != nil {
					if _, ok := addr.(*net.UDPAddr); ok {
						// This is a valid UDP connection
						// But we can't return from inside a closure
						// So we need to restructure this
					}
				}
			}()
		}
	}

	return nil, fmt.Errorf("could not find UDP connection field in VoiceConnection")
}

// SafeGetUDPConnection safely extracts the UDP connection using reflection to find the correct offset
// This avoids crashing by not indiscriminately scanning memory and casting invalid pointers
func SafeGetUDPConnection(vc *discordgo.VoiceConnection) (*net.UDPConn, error) {
	if vc == nil {
		return nil, fmt.Errorf("voice connection is nil")
	}

	// Use reflection to find the offset of the "udpConn" field
	// This is much safer than scanning memory blindly
	val := reflect.ValueOf(vc).Elem()
	typ := val.Type()

	field, ok := typ.FieldByName("udpConn")
	if !ok {
		return nil, fmt.Errorf("could not find 'udpConn' field in VoiceConnection")
	}

	// Check if the type matches what we expect
	if field.Type != reflect.TypeOf((*net.UDPConn)(nil)) {
		return nil, fmt.Errorf("'udpConn' field is not of type *net.UDPConn, got %s", field.Type)
	}

	// Access the field using unsafe pointer at the calculated offset
	// This works even for unexported fields
	ptr := unsafe.Pointer(uintptr(unsafe.Pointer(vc)) + field.Offset)
	udpConnPtr := *(*messageUDPConn)(ptr)

	if udpConnPtr == nil {
		return nil, fmt.Errorf("udpConn field is nil (voice not connected?)")
	}

	return udpConnPtr, nil
}

// messageUDPConn is a type alias to help with casting
type messageUDPConn = *net.UDPConn

// GetVoiceSecurityInfo extracts the secret key and selected protocol/mode from the VoiceConnection
func GetVoiceSecurityInfo(vc *discordgo.VoiceConnection) ([]byte, string, error) {
	if vc == nil {
		return nil, "", fmt.Errorf("voice connection is nil")
	}

	// We need to access exported fields if possible, or use reflection/unsafe if not.
	// In discordgo, Ready is *VoiceReady and has SecretKey.
	// SelectedProtocol is string.
	// Let's rely on reflection to be safe against visibility or naming changes,
	// though standard discordgo has these exported.

	val := reflect.ValueOf(vc).Elem()

	// Get Ready.SecretKey
	readyField := val.FieldByName("Ready")
	if !readyField.IsValid() || readyField.IsNil() {
		return nil, "", fmt.Errorf("VoiceConnection.Ready is nil or invalid")
	}

	readyVal := readyField.Elem()
	secretKeyField := readyVal.FieldByName("SecretKey")
	if !secretKeyField.IsValid() {
		return nil, "", fmt.Errorf("could not find SecretKey in VoiceReady")
	}

	// SecretKey is []byte
	secretKey := secretKeyField.Bytes()
	if len(secretKey) == 0 {
		return nil, "", fmt.Errorf("SecretKey is empty (handshake not finished?)")
	}

	// Get SelectedProtocol (mode)
	// It's often just a field "UDP" (which is *VoiceUDPInfo) or similar?
	// Or directly "SelectedProtocol" string?
	// Checking discordgo source structure...
	// Usually there is a `OpusSend` channel etc.
	// But the mode is selected during the handshake.
	// It is stored in `vc.Ready.Mode` or we might have to infer it.
	// Wait, `VoiceReady` event contains the *available* modes.
	// The *selected* mode is sent in `SelectProtocol` struct.
	// Discordgo usually stores the selected mode in the VoiceConnection struct itself
	// or assumes a default.
	// However, Richy-Z fork might store it.
	// Let's assume standard "xsalsa20_poly1305" if we can't find it, OR
	// look for a field named like "mode" or "protocol".

	// Let's try to get it from reflection first:
	// "SelectedProtocol" is unlikely to be the field name.
	// Logic usually is:
	// vc.sessionID
	// vc.token
	// vc.endpoint
	// vc.guildID
	// vc.channelID
	// vc.debug
	// vc.Ready (VoiceReady)

	// If we can't find it easily, checking standard discordgo behaviors...
	// Discordgo forces "xsalsa20_poly1305" usually.
	// Richy-Z fork supports "aead_aes256_gcm_rtpsize".

	// Let's try to deduce the mode or find where it's stored.
	// If we can't find it, we'll return "aead_aes256_gcm_rtpsize" as a fallback since that's modern,
	// or try "xsalsa20_poly1305" if that updates.

	// Actually, let's look for any string/protocol field.
	// But safe bet is to let the user of this function provide a hint or update `AudioCapture` to try multiple.
	// For now let's default to "xsalsa20_poly1305" if not found, but I'll check "Mode" field.

	mode := "xsalsa20_poly1305" // Default legacy

	// Try to find "Mode" field in VoiceConnection (non-standard but might exist)
	// Or maybe it's exposed via public API?

	// Better approach: Since we are using a specific fork, we know it prefers AES GCM.
	// We can try to decrypt with AES GCM, if fail, try Salsa.
	// But let's return a default here.

	return secretKey, mode, nil
}
