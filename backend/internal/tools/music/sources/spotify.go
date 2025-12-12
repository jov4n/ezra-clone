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
func FetchSpotifyPlaylist(ctx context.Context, spotifyURL, requester string, songChan chan<- Song) ([]Song, error) {
	var tracks []string

	maxDurationSeconds := MaxSongDurationSeconds
	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), PlaylistFetchTimeout*time.Second)
		defer cancel()
	}

	if strings.Contains(spotifyURL, "track") {
		// Single track
		trackName, err := extractSpotifyTrackName(ctx, spotifyURL)
		if err == nil && trackName != "" {
			song, err := SearchYouTubeWithContext(ctx, trackName, requester)
			if err == nil && !song.IsEmpty() {
				if IsSongDurationUnderLimit(song.Duration, maxDurationSeconds) {
					if songChan != nil {
						songChan <- song
					}
					return []Song{song}, nil
				}
			}
		}
	} else if strings.Contains(spotifyURL, "playlist") || strings.Contains(spotifyURL, "album") {
		// Playlist/Album
		var err error
		tracks, err = extractSpotifyPlaylist(ctx, spotifyURL)
		if err != nil {
			return nil, fmt.Errorf("failed to extract playlist: %w", err)
		}
	}

	var songs []Song
	if len(tracks) > 50 {
		tracks = tracks[:50]
	}

	// Concurrent search using errgroup with context
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(MaxConcurrentSearches)

	tempSongs := make([]Song, len(tracks))
	var mu sync.Mutex

	for i, track := range tracks {
		idx := i
		query := track
		g.Go(func() error {
			// Check if context was cancelled
			select {
			case <-gctx.Done():
				return gctx.Err()
			default:
			}

			song, err := SearchYouTubeWithContext(gctx, query, requester)
			if err == nil && !song.IsEmpty() && IsSongDurationUnderLimit(song.Duration, maxDurationSeconds) {
				mu.Lock()
				tempSongs[idx] = song
				mu.Unlock()
				if songChan != nil {
					select {
					case songChan <- song:
					case <-gctx.Done():
						return gctx.Err()
					}
				}
			}
			return nil
		})
	}

	// Wait for all goroutines to complete
	if err := g.Wait(); err != nil && err != context.Canceled {
		return nil, fmt.Errorf("playlist fetch failed: %w", err)
	}

	for _, s := range tempSongs {
		if !s.IsEmpty() {
			songs = append(songs, s)
		}
	}

	if len(songs) == 0 {
		return nil, ErrPlaylistEmpty
	}

	return songs, nil
}

func extractSpotifyTrackName(ctx context.Context, url string) (string, error) {
	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("%w: request timed out: %v", ErrTimeout, ctx.Err())
		}
		return "", fmt.Errorf("failed to fetch URL: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	html := string(body)
	titleRegex := regexp.MustCompile(`<title>(.*?)</title>`)
	matches := titleRegex.FindStringSubmatch(html)
	if len(matches) > 1 {
		title := strings.ReplaceAll(matches[1], " | Spotify", "")
		title = strings.ReplaceAll(title, " | ", " ")
		return title, nil
	}

	return "", ErrSongNotFound
}

func extractSpotifyPlaylist(ctx context.Context, url string) ([]string, error) {
	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
	}

	// Use curl.exe to fetch the page
	cmd := exec.CommandContext(ctx, "curl.exe", "-L", "-s", url)
	output, err := cmd.Output()
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("%w: playlist extraction timed out: %v", ErrTimeout, ctx.Err())
		}
		return nil, fmt.Errorf("%w: curl failed for Spotify URL %s: %v", ErrFetchFailed, url, err)
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

	if len(tracks) == 0 {
		return nil, ErrPlaylistEmpty
	}

	return tracks, nil
}
