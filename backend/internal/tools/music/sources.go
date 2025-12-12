package music

import (
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
func FetchSpotifyPlaylist(spotifyURL, requester string, songChan chan<- Song) []Song {
	// Create a channel for sources.Song and convert
	sourceChan := make(chan sources.Song, 10)
	go func() {
		for s := range sourceChan {
			if songChan != nil {
				songChan <- convertSong(s)
			}
		}
	}()

	songs := sources.FetchSpotifyPlaylist(spotifyURL, requester, sourceChan)
	close(sourceChan)
	return convertSongs(songs)
}

// FetchSoundCloudPlaylist wraps sources.FetchSoundCloudPlaylist
func FetchSoundCloudPlaylist(soundcloudURL, requester string, songChan chan<- Song) []Song {
	// Create a channel for sources.Song and convert
	sourceChan := make(chan sources.Song, 10)
	go func() {
		for s := range sourceChan {
			if songChan != nil {
				songChan <- convertSong(s)
			}
		}
	}()

	songs := sources.FetchSoundCloudPlaylist(soundcloudURL, requester, sourceChan)
	close(sourceChan)
	return convertSongs(songs)
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

