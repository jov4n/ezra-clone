package voice

import (
	"context"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"go.uber.org/zap"
)

// ReferenceAudioManager manages voice reference audio files
type ReferenceAudioManager struct {
	referenceDir string
	converter    *AudioConverter
	logger       *zap.Logger
	mu           sync.RWMutex
	references   map[string]string // userID -> reference file path
}

// NewReferenceAudioManager creates a new reference audio manager
func NewReferenceAudioManager(referenceDir string, converter *AudioConverter, logger *zap.Logger) (*ReferenceAudioManager, error) {
	// Create reference directory if it doesn't exist
	if err := os.MkdirAll(referenceDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create reference directory: %w", err)
	}

	ram := &ReferenceAudioManager{
		referenceDir: referenceDir,
		converter:    converter,
		logger:       logger,
		references:   make(map[string]string),
	}

	// Load default reference if it exists
	defaultPath := filepath.Join(referenceDir, "default.wav")
	if _, err := os.Stat(defaultPath); err == nil {
		ram.references["default"] = defaultPath
		logger.Info("Loaded default voice reference", zap.String("path", defaultPath))
	}

	return ram, nil
}

// GetReference returns the path to the reference audio for a user
// Falls back to default if user-specific reference doesn't exist
func (ram *ReferenceAudioManager) GetReference(userID string) (string, error) {
	ram.mu.RLock()
	defer ram.mu.RUnlock()

	// Check for user-specific reference
	if path, exists := ram.references[userID]; exists {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
		// File doesn't exist, remove from map
		delete(ram.references, userID)
	}

	// Fall back to default
	if path, exists := ram.references["default"]; exists {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	return "", fmt.Errorf("no reference audio found for user %s and no default available", userID)
}

// SetReferenceFromFile sets a reference audio from a local file
func (ram *ReferenceAudioManager) SetReferenceFromFile(ctx context.Context, userID, filePath string) error {
	// Convert to XTTS V2 format
	outputPath := filepath.Join(ram.referenceDir, fmt.Sprintf("%s.wav", userID))
	
	if err := ram.converter.ConvertAudioFile(ctx, filePath, outputPath); err != nil {
		return fmt.Errorf("failed to convert reference audio: %w", err)
	}

	ram.mu.Lock()
	ram.references[userID] = outputPath
	ram.mu.Unlock()

	ram.logger.Info("Set voice reference", zap.String("user_id", userID), zap.String("path", outputPath))
	return nil
}

// SetReferenceFromReader sets a reference audio from a reader (e.g., Discord attachment)
func (ram *ReferenceAudioManager) SetReferenceFromReader(ctx context.Context, userID string, reader io.Reader, contentType string) error {
	// Determine file extension from content type
	ext := getExtensionFromContentType(contentType)
	if ext == "" {
		// Try to detect from content
		ext = ".wav" // Default
	}

	// Save to temporary file first
	tempPath := filepath.Join(ram.referenceDir, fmt.Sprintf("temp_%s%s", userID, ext))
	defer os.Remove(tempPath) // Clean up temp file

	// Write to temp file
	file, err := os.Create(tempPath)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}

	if _, err := io.Copy(file, reader); err != nil {
		file.Close()
		return fmt.Errorf("failed to write temp file: %w", err)
	}
	file.Close()

	// Convert and set reference
	return ram.SetReferenceFromFile(ctx, userID, tempPath)
}

// ValidateAudioFile checks if a file is a valid audio file
func (ram *ReferenceAudioManager) ValidateAudioFile(filePath string) error {
	// Check file extension
	ext := strings.ToLower(filepath.Ext(filePath))
	validExts := map[string]bool{
		".wav": true,
		".mp3": true,
		".ogg": true,
		".m4a": true,
		".flac": true,
		".aac": true,
	}

	if !validExts[ext] {
		return fmt.Errorf("unsupported audio format: %s (supported: wav, mp3, ogg, m4a, flac, aac)", ext)
	}

	// Check if file exists and is readable
	if _, err := os.Stat(filePath); err != nil {
		return fmt.Errorf("file not found or not readable: %w", err)
	}

	return nil
}

// getExtensionFromContentType extracts file extension from MIME type
func getExtensionFromContentType(contentType string) string {
	exts, err := mime.ExtensionsByType(contentType)
	if err != nil || len(exts) == 0 {
		return ""
	}
	// Return the first extension (usually the most common)
	return exts[0]
}

