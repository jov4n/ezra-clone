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

// YouTubeSource implements the AudioSource interface for YouTube
type YouTubeSource struct{}

// NewYouTubeSource creates a new YouTube audio source
func NewYouTubeSource() *YouTubeSource {
	return &YouTubeSource{}
}

// Name returns the source name
func (y *YouTubeSource) Name() string {
	return "youtube"
}

// SupportsURL returns true if the URL is a YouTube URL
func (y *YouTubeSource) SupportsURL(url string) bool {
	return strings.Contains(url, "youtube.com") || strings.Contains(url, "youtu.be")
}

// Search searches for a song on YouTube
func (y *YouTubeSource) Search(ctx context.Context, query, requester string) (Song, error) {
	return SearchYouTubeWithContext(ctx, query, requester)
}

// FetchByURL fetches song metadata from a YouTube URL
func (y *YouTubeSource) FetchByURL(ctx context.Context, url, requester string) (Song, error) {
	return FetchYouTubeVideoWithContext(ctx, url, requester)
}

// FetchPlaylist fetches songs from a YouTube playlist (not implemented yet)
func (y *YouTubeSource) FetchPlaylist(ctx context.Context, url, requester string, songChan chan<- Song) ([]Song, error) {
	// For now, just fetch a single video
	song, err := y.FetchByURL(ctx, url, requester)
	if err != nil {
		return nil, err
	}
	if songChan != nil {
		songChan <- song
	}
	return []Song{song}, nil
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

// FetchYouTubeVideoWithContext gets video info from a YouTube URL with context support
func FetchYouTubeVideoWithContext(ctx context.Context, url, requester string) (Song, error) {
	cmd := exec.CommandContext(ctx, YtdlpExecutable, "--dump-json", "--no-playlist", url)
	output, err := cmd.Output()
	if err != nil {
		if ctx.Err() != nil {
			return Song{}, fmt.Errorf("%w: %v", ErrTimeout, ctx.Err())
		}
		return Song{}, fmt.Errorf("%w: yt-dlp failed for URL %s: %v", ErrFetchFailed, url, err)
	}

	var videoInfo map[string]interface{}
	if err := json.Unmarshal(output, &videoInfo); err != nil {
		return Song{}, fmt.Errorf("%w: failed to parse video info: %v", ErrFetchFailed, err)
	}

	title, _ := videoInfo["title"].(string)
	if title == "" {
		return Song{}, ErrSongNotFound
	}

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
	}, nil
}

// FetchYouTubeVideo gets video info from a YouTube URL (legacy wrapper for backward compatibility)
// Deprecated: Use FetchYouTubeVideoWithContext instead
func FetchYouTubeVideo(url, requester string) Song {
	song, err := FetchYouTubeVideoWithContext(context.Background(), url, requester)
	if err != nil {
		return Song{}
	}
	return song
}

// SearchYouTubeWithContext searches for a single song on YouTube with context support
func SearchYouTubeWithContext(ctx context.Context, query, requester string) (Song, error) {
	// Use provided context or create one with timeout
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, DefaultSearchTimeout*time.Second)
		defer cancel()
	}

	cmd := exec.CommandContext(ctx, YtdlpExecutable, "--dump-json", "--default-search", "ytsearch1", query)
	output, err := cmd.Output()
	if err != nil {
		if ctx.Err() != nil {
			return Song{}, fmt.Errorf("%w: search timed out for query %q", ErrTimeout, query)
		}
		return Song{}, fmt.Errorf("%w: yt-dlp search failed for query %q: %v", ErrFetchFailed, query, err)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) == 0 || lines[0] == "" {
		return Song{}, fmt.Errorf("%w: no results for query %q", ErrSongNotFound, query)
	}

	var videoInfo map[string]interface{}
	if err := json.Unmarshal([]byte(lines[0]), &videoInfo); err != nil {
		return Song{}, fmt.Errorf("%w: failed to parse search results: %v", ErrFetchFailed, err)
	}

	title, _ := videoInfo["title"].(string)
	if title == "" {
		return Song{}, fmt.Errorf("%w: no valid result for query %q", ErrSongNotFound, query)
	}

	videoID, ok := videoInfo["id"].(string)
	if !ok {
		return Song{}, fmt.Errorf("%w: no video ID found for query %q", ErrSongNotFound, query)
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
	}, nil
}

// SearchYouTube searches for a single song on YouTube (legacy wrapper for backward compatibility)
// Deprecated: Use SearchYouTubeWithContext instead
func SearchYouTube(query, requester string) Song {
	song, err := SearchYouTubeWithContext(context.Background(), query, requester)
	if err != nil {
		return Song{}
	}
	return song
}
