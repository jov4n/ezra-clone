package sources

import (
	"context"
	"errors"
)

// Configuration constants
const (
	// MaxPlaylistTracks is the maximum number of tracks to fetch from a playlist
	MaxPlaylistTracks = 50

	// MaxSongDurationSeconds is the default maximum song duration for playlists (6 minutes)
	MaxSongDurationSeconds = 360

	// DefaultSearchTimeout is the default timeout for search operations
	DefaultSearchTimeout = 30

	// MaxConcurrentSearches is the maximum number of concurrent YouTube searches
	MaxConcurrentSearches = 5

	// PlaylistFetchTimeout is the timeout for fetching playlist metadata
	PlaylistFetchTimeout = 60

	// DefaultOpenRouterMaxTokens is the default max tokens for OpenRouter API calls
	DefaultOpenRouterMaxTokens = 500

	// DefaultOpenRouterPlaylistMaxTokens is the max tokens for playlist generation
	DefaultOpenRouterPlaylistMaxTokens = 600
)

// Common errors for music sources
var (
	// ErrSongNotFound is returned when a song cannot be found
	ErrSongNotFound = errors.New("song not found")

	// ErrPlaylistEmpty is returned when a playlist has no valid tracks
	ErrPlaylistEmpty = errors.New("playlist is empty or has no valid tracks")

	// ErrFetchFailed is returned when fetching audio metadata fails
	ErrFetchFailed = errors.New("failed to fetch audio metadata")

	// ErrInvalidURL is returned when the provided URL is invalid
	ErrInvalidURL = errors.New("invalid URL format")

	// ErrTimeout is returned when an operation times out
	ErrTimeout = errors.New("operation timed out")

	// ErrSourceUnavailable is returned when a source (YouTube, Spotify, etc.) is unavailable
	ErrSourceUnavailable = errors.New("audio source unavailable")
)

// Song represents a track (local to sources package to avoid import cycles)
type Song struct {
	Title     string
	URL       string
	Duration  string
	Thumbnail string
	Requester string
	Source    string // "youtube", "spotify", "soundcloud", "twitch"
}

// IsEmpty returns true if the song has no title (indicating an invalid/empty song)
func (s Song) IsEmpty() bool {
	return s.Title == ""
}

// AudioSource defines the interface for music source providers
type AudioSource interface {
	// Name returns the name of the source (e.g., "youtube", "spotify")
	Name() string

	// Search searches for a song by query and returns the best match
	Search(ctx context.Context, query, requester string) (Song, error)

	// FetchByURL fetches song metadata from a direct URL
	FetchByURL(ctx context.Context, url, requester string) (Song, error)

	// FetchPlaylist fetches songs from a playlist URL
	// If songChan is provided, songs are streamed as they're found
	FetchPlaylist(ctx context.Context, url, requester string, songChan chan<- Song) ([]Song, error)

	// SupportsURL returns true if this source can handle the given URL
	SupportsURL(url string) bool
}
