package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

var YtdlpExecutable = "yt-dlp"

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

// IsSongDurationUnderLimit checks if a formatted duration string is under the limit
func IsSongDurationUnderLimit(durationStr string, maxSeconds int) bool {
	if durationStr == "" || durationStr == "Unknown" {
		return true
	}

	parts := strings.Split(durationStr, ":")
	if len(parts) < 2 {
		return true
	}

	var totalSeconds int
	if len(parts) == 2 {
		minutes, _ := strconv.Atoi(parts[0])
		seconds, _ := strconv.Atoi(parts[1])
		totalSeconds = minutes*60 + seconds
	} else if len(parts) == 3 {
		hours, _ := strconv.Atoi(parts[0])
		minutes, _ := strconv.Atoi(parts[1])
		seconds, _ := strconv.Atoi(parts[2])
		totalSeconds = hours*3600 + minutes*60 + seconds
	} else {
		return true
	}

	return totalSeconds <= maxSeconds
}

// init sets YtdlpExecutable from music package
func init() {
	// Try to find yt-dlp
	if path, err := exec.LookPath("yt-dlp"); err == nil {
		YtdlpExecutable = path
	} else if path, err := exec.LookPath("ytdlp"); err == nil {
		YtdlpExecutable = path
	} else if os.Getenv("YTDLP_PATH") != "" {
		YtdlpExecutable = os.Getenv("YTDLP_PATH")
	}
}

// FetchYouTubeVideo gets video info from a YouTube URL
func FetchYouTubeVideo(url, requester string) Song {
	cmd := exec.Command(YtdlpExecutable, "--dump-json", "--no-playlist", url)
	output, err := cmd.Output()
	if err != nil {
		return Song{}
	}

	var videoInfo map[string]interface{}
	json.Unmarshal(output, &videoInfo)

	title, _ := videoInfo["title"].(string)
	duration := FormatDuration(videoInfo["duration"])
	thumbnail := ""
	if thumbnails, ok := videoInfo["thumbnail"].(string); ok {
		thumbnail = thumbnails
	}

	return Song{
		Title:     title,
		URL:       url,
		Duration:  duration,
		Thumbnail: thumbnail,
		Requester: requester,
		Source:    "youtube",
	}
}

// SearchYouTube searches for a single song on YouTube
func SearchYouTube(query, requester string) Song {

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, YtdlpExecutable, "--dump-json", "--default-search", "ytsearch1", query)
	output, err := cmd.Output()
	if err != nil {
		return Song{}
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) == 0 || lines[0] == "" {
		return Song{}
	}

	var videoInfo map[string]interface{}
	if err := json.Unmarshal([]byte(lines[0]), &videoInfo); err != nil {
		return Song{}
	}

	title, _ := videoInfo["title"].(string)
	if title == "" {
		return Song{}
	}

	videoID, ok := videoInfo["id"].(string)
	if !ok {
		return Song{}
	}
	videoURL := fmt.Sprintf("https://www.youtube.com/watch?v=%s", videoID)
	duration := FormatDuration(videoInfo["duration"])
	thumbnail := ""
	if thumbnails, ok := videoInfo["thumbnail"].(string); ok {
		thumbnail = thumbnails
	}

	return Song{
		Title:     title,
		URL:       videoURL,
		Duration:  duration,
		Thumbnail: thumbnail,
		Requester: requester,
		Source:    "youtube",
	}
}
