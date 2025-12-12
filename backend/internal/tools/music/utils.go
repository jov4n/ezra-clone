package music

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

var YtdlpExecutable = "yt-dlp"
var FfmpegExecutable = "ffmpeg"

// CheckDependencies checks for required external tools
func CheckDependencies() error {
	// Check for yt-dlp
	ytdlpPath := FindExecutable("yt-dlp")
	if ytdlpPath == "" {
		// Try alternative names
		ytdlpPath = FindExecutable("ytdlp")
		if ytdlpPath == "" {
			return fmt.Errorf("yt-dlp not found in PATH")
		} else {
			YtdlpExecutable = ytdlpPath
		}
	} else {
		YtdlpExecutable = ytdlpPath
	}

	// Check for ffmpeg (optional, only needed for Twitch)
	ffmpegPath := FindExecutable("ffmpeg")
	if ffmpegPath != "" {
		FfmpegExecutable = ffmpegPath
	}

	return nil
}

// FindExecutable finds an executable in PATH or common locations
func FindExecutable(name string) string {
	// Try direct lookup first
	if path, err := exec.LookPath(name); err == nil {
		return path
	}

	// On Windows, try with .exe extension
	if runtime.GOOS == "windows" {
		if path, err := exec.LookPath(name + ".exe"); err == nil {
			return path
		}
		// Try common installation locations
		commonPaths := []string{
			filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "Python", "Python*", "Scripts", name+".exe"),
			filepath.Join(os.Getenv("APPDATA"), "Local", "Programs", "Python", "Python*", "Scripts", name+".exe"),
			filepath.Join(os.Getenv("ProgramFiles"), "yt-dlp", name+".exe"),
			filepath.Join(os.Getenv("ProgramFiles(x86)"), "yt-dlp", name+".exe"),
		}
		for _, pattern := range commonPaths {
			matches, _ := filepath.Glob(pattern)
			if len(matches) > 0 {
				return matches[0]
			}
		}
	}

	return ""
}

// IsYouTubeURL checks if a string is a YouTube URL
func IsYouTubeURL(str string) bool {
	parsed, err := url.Parse(str)
	return err == nil && (parsed.Host == "youtube.com" || parsed.Host == "www.youtube.com" || parsed.Host == "youtu.be" || parsed.Host == "m.youtube.com")
}

// IsSpotifyURL checks if a string is a Spotify URL
func IsSpotifyURL(str string) bool {
	parsed, err := url.Parse(str)
	return err == nil && (parsed.Host == "open.spotify.com" || parsed.Host == "spotify.com")
}

// IsSoundCloudURL checks if a string is a SoundCloud URL
func IsSoundCloudURL(str string) bool {
	parsed, err := url.Parse(str)
	return err == nil && (parsed.Host == "soundcloud.com" || parsed.Host == "www.soundcloud.com")
}

// FormatDuration formats a duration from seconds to MM:SS or H:MM:SS
func FormatDuration(d interface{}) string {
	var seconds float64
	switch v := d.(type) {
	case float64:
		seconds = v
	case int:
		seconds = float64(v)
	case int64:
		seconds = float64(v)
	default:
		return "Unknown"
	}

	hours := int(seconds) / 3600
	minutes := (int(seconds) % 3600) / 60
	secs := int(seconds) % 60

	if hours > 0 {
		return fmt.Sprintf("%d:%02d:%02d", hours, minutes, secs)
	}
	return fmt.Sprintf("%d:%02d", minutes, secs)
}

// GetDurationSeconds extracts duration in seconds from videoInfo
func GetDurationSeconds(d interface{}) float64 {
	switch v := d.(type) {
	case float64:
		return v
	case int:
		return float64(v)
	case int64:
		return float64(v)
	default:
		return 0
	}
}

// IsSongDurationUnderLimit checks if a formatted duration string is under the limit
func IsSongDurationUnderLimit(durationStr string, maxSeconds int) bool {
	if durationStr == "" || durationStr == "Unknown" {
		return true // Allow unknown durations
	}

	parts := strings.Split(durationStr, ":")
	if len(parts) < 2 {
		return true // Invalid format, allow it
	}

	var totalSeconds int
	if len(parts) == 2 {
		// Format: "M:SS"
		minutes, _ := strconv.Atoi(parts[0])
		seconds, _ := strconv.Atoi(parts[1])
		totalSeconds = minutes*60 + seconds
	} else if len(parts) == 3 {
		// Format: "H:MM:SS"
		hours, _ := strconv.Atoi(parts[0])
		minutes, _ := strconv.Atoi(parts[1])
		seconds, _ := strconv.Atoi(parts[2])
		totalSeconds = hours*3600 + minutes*60 + seconds
	} else {
		return true // Invalid format, allow it
	}

	return totalSeconds <= maxSeconds
}

// Truncate truncates a string to maxLen characters
func Truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// HTMLUnescape unescapes HTML entities
func HTMLUnescape(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(s, "&amp;", "&"), "&lt;", "<"), "&gt;", ">")
}

// ParseTimestamp parses a timestamp string (M:SS, H:MM:SS, or just seconds) into total seconds
func ParseTimestamp(timestamp string) (int, error) {
	timestamp = strings.TrimSpace(timestamp)

	// Try parsing as just seconds first
	if seconds, err := strconv.Atoi(timestamp); err == nil {
		if seconds < 0 {
			return 0, fmt.Errorf("timestamp cannot be negative")
		}
		return seconds, nil
	}

	// Parse as M:SS or H:MM:SS format
	parts := strings.Split(timestamp, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, fmt.Errorf("invalid timestamp format")
	}

	var totalSeconds int
	if len(parts) == 2 {
		// M:SS format
		minutes, err := strconv.Atoi(parts[0])
		if err != nil {
			return 0, fmt.Errorf("invalid minutes")
		}
		seconds, err := strconv.Atoi(parts[1])
		if err != nil {
			return 0, fmt.Errorf("invalid seconds")
		}
		totalSeconds = minutes*60 + seconds
	} else if len(parts) == 3 {
		// H:MM:SS format
		hours, err := strconv.Atoi(parts[0])
		if err != nil {
			return 0, fmt.Errorf("invalid hours")
		}
		minutes, err := strconv.Atoi(parts[1])
		if err != nil {
			return 0, fmt.Errorf("invalid minutes")
		}
		seconds, err := strconv.Atoi(parts[2])
		if err != nil {
			return 0, fmt.Errorf("invalid seconds")
		}
		totalSeconds = hours*3600 + minutes*60 + seconds
	}

	if totalSeconds < 0 {
		return 0, fmt.Errorf("timestamp cannot be negative")
	}

	return totalSeconds, nil
}

// FormatDurationFromSeconds formats seconds into M:SS or H:MM:SS format
func FormatDurationFromSeconds(seconds int) string {
	if seconds < 0 {
		seconds = 0
	}

	hours := seconds / 3600
	minutes := (seconds % 3600) / 60
	secs := seconds % 60

	if hours > 0 {
		return fmt.Sprintf("%d:%02d:%02d", hours, minutes, secs)
	}
	return fmt.Sprintf("%d:%02d", minutes, secs)
}

