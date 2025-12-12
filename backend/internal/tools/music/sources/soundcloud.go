package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"
)

// FetchSoundCloudPlaylist fetches songs from a SoundCloud playlist/track and converts to YouTube
func FetchSoundCloudPlaylist(soundcloudURL, requester string, songChan chan<- Song) []Song {
	// Extract tracks using yt-dlp
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, YtdlpExecutable,
		"--dump-json",
		"--flat-playlist",
		"--no-download",
		"--quiet",
		soundcloudURL)

	output, err := cmd.Output()
	if err != nil {
		return []Song{}
	}

	var tracks []string
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		var info map[string]interface{}
		if err := json.Unmarshal([]byte(line), &info); err != nil {
			continue
		}

		title, _ := info["title"].(string)
		uploader, _ := info["uploader"].(string)

		if title != "" {
			if uploader != "" && !strings.Contains(title, uploader) {
				tracks = append(tracks, fmt.Sprintf("%s - %s", uploader, title))
			} else {
				tracks = append(tracks, title)
			}
		}
	}

	var songs []Song
	maxDurationSeconds := 360

	if len(tracks) > 50 {
		tracks = tracks[:50]
	}

	// Concurrent search
	type result struct {
		song Song
		idx  int
	}

	resultsChan := make(chan result, len(tracks))
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, 5)

	for i, track := range tracks {
		wg.Add(1)
		go func(idx int, query string) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			song := SearchYouTube(query, requester) // SearchYouTube is in youtube.go
			if song.Title != "" && IsSongDurationUnderLimit(song.Duration, maxDurationSeconds) {
				resultsChan <- result{song: song, idx: idx}
				if songChan != nil {
					songChan <- song
				}
			}
		}(i, track)
	}

	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	tempSongs := make([]Song, len(tracks))
	for res := range resultsChan {
		tempSongs[res.idx] = res.song
	}

	for _, s := range tempSongs {
		if s.Title != "" {
			songs = append(songs, s)
		}
	}

	return songs
}

func extractSoundCloudTrackName(url string) string {
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
		title := strings.ReplaceAll(matches[1], " | SoundCloud", "")
		return title
	}

	return ""
}
