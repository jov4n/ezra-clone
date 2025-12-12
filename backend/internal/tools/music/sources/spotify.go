package sources

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
)

// FetchSpotifyPlaylist fetches songs from a Spotify playlist/track and converts to YouTube
func FetchSpotifyPlaylist(spotifyURL, requester string, songChan chan<- Song) []Song {
	var tracks []string

	maxDurationSeconds := 360 // 6 minutes max for playlists
	if strings.Contains(spotifyURL, "track") {
		// Single track
		trackName := extractSpotifyTrackName(spotifyURL)
		if trackName != "" {
			song := SearchYouTube(trackName, requester) // SearchYouTube is in youtube.go
			if song.Title != "" {
				if IsSongDurationUnderLimit(song.Duration, maxDurationSeconds) {
					if songChan != nil {
						songChan <- song
					}
					return []Song{song}
				}
			}
		}
	} else if strings.Contains(spotifyURL, "playlist") || strings.Contains(spotifyURL, "album") {
		// Playlist/Album
		tracks = extractSpotifyPlaylist(spotifyURL)
	}

	var songs []Song
	if len(tracks) > 50 {
		tracks = tracks[:50]
	}

	// Concurrent search using errgroup with context
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(5) // Limit concurrency to 5

	tempSongs := make([]Song, len(tracks))
	var mu sync.Mutex

	for i, track := range tracks {
		idx := i
		query := track
		g.Go(func() error {
			// Check if context was cancelled
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			song := SearchYouTube(query, requester)
			if song.Title != "" && IsSongDurationUnderLimit(song.Duration, maxDurationSeconds) {
				mu.Lock()
				tempSongs[idx] = song
				mu.Unlock()
				if songChan != nil {
					select {
					case songChan <- song:
					case <-ctx.Done():
						return ctx.Err()
					}
				}
			}
			return nil
		})
	}

	// Wait for all goroutines to complete
	if err := g.Wait(); err != nil && err != context.Canceled {
	}

	for _, s := range tempSongs {
		if s.Title != "" {
			songs = append(songs, s)
		}
	}

	return songs
}

func extractSpotifyTrackName(url string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return ""
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	html := string(body)

	titleRegex := regexp.MustCompile(`<title>(.*?)</title>`)
	matches := titleRegex.FindStringSubmatch(html)
	if len(matches) > 1 {
		title := strings.ReplaceAll(matches[1], " | Spotify", "")
		title = strings.ReplaceAll(title, " | ", " ")
		return title
	}

	return ""
}

func extractSpotifyPlaylist(url string) []string {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Use curl.exe to fetch the page
	cmd := exec.CommandContext(ctx, "curl.exe", "-L", "-s", url)
	output, err := cmd.Output()
	if err != nil {
		return []string{}
	}

	html := string(output)
	var tracks []string

	// Regex to find track and artist
	trackRegex := regexp.MustCompile(`"name":"([^"]+)"[^}]*"artists":\[[^\]]*"name":"([^"]+)"`)
	matches := trackRegex.FindAllStringSubmatch(html, -1)

	for _, match := range matches {
		if len(match) >= 3 {
			artist := match[2]
			track := match[1]
			tracks = append(tracks, fmt.Sprintf("%s - %s", artist, track))
		}
	}

	return tracks
}
