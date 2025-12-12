package music

import (
	"context"

	"ezra-clone/backend/internal/tools/music/sources"
)

// convertSong converts sources.Song to music.Song
func convertSong(s sources.Song) Song {
	return Song{
		Title:     s.Title,
		URL:       s.URL,
		Duration:  s.Duration,
		Thumbnail: s.Thumbnail,
		Requester: s.Requester,
		Source:    s.Source,
	}
}

// convertSongs converts []sources.Song to []music.Song
func convertSongs(ss []sources.Song) []Song {
	result := make([]Song, len(ss))
	for i, s := range ss {
		result[i] = convertSong(s)
	}
	return result
}

// FetchYouTubeVideo wraps sources.FetchYouTubeVideo
func FetchYouTubeVideo(url, requester string) Song {
	return convertSong(sources.FetchYouTubeVideo(url, requester))
}

// SearchYouTube wraps sources.SearchYouTube
func SearchYouTube(query, requester string) Song {
	return convertSong(sources.SearchYouTube(query, requester))
}

// FetchSpotifyPlaylist wraps sources.FetchSpotifyPlaylist
func FetchSpotifyPlaylist(ctx context.Context, spotifyURL, requester string, songChan chan<- Song) ([]Song, error) {
	// Create a channel for sources.Song and convert
	sourceChan := make(chan sources.Song, 10)
	go func() {
		for s := range sourceChan {
			if songChan != nil {
				songChan <- convertSong(s)
			}
		}
	}()

	songs, err := sources.FetchSpotifyPlaylist(ctx, spotifyURL, requester, sourceChan)
	close(sourceChan)
	if err != nil {
		return nil, err
	}
	return convertSongs(songs), nil
}

// FetchSoundCloudPlaylist wraps sources.FetchSoundCloudPlaylist
func FetchSoundCloudPlaylist(ctx context.Context, soundcloudURL, requester string, songChan chan<- Song) ([]Song, error) {
	// Create a channel for sources.Song and convert
	sourceChan := make(chan sources.Song, 10)
	go func() {
		for s := range sourceChan {
			if songChan != nil {
				songChan <- convertSong(s)
			}
		}
	}()

	songs, err := sources.FetchSoundCloudPlaylist(ctx, soundcloudURL, requester, sourceChan)
	close(sourceChan)
	if err != nil {
		return nil, err
	}
	return convertSongs(songs), nil
}

// GeneratePlaylistQueries wraps sources.GeneratePlaylistQueries
func GeneratePlaylistQueries(query string) []string {
	return sources.GeneratePlaylistQueries(query)
}

// GenerateRadioSuggestions wraps sources.GenerateRadioSuggestions
func GenerateRadioSuggestions(seed string, recentSongs []string) []string {
	return sources.GenerateRadioSuggestions(seed, recentSongs)
}

// SetOpenRouterAPIKey sets the OpenRouter API key for playlist generation
func SetOpenRouterAPIKey(key string) {
	sources.SetOpenRouterAPIKey(key)
}

