package sources

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const OpenRouterAPIURL = "https://openrouter.ai/api/v1/chat/completions"

var OpenRouterAPIKey = ""

// SetOpenRouterAPIKey sets the OpenRouter API key
func SetOpenRouterAPIKey(key string) {
	OpenRouterAPIKey = key
}

// GeneratePlaylistQueries generates song search queries using OpenRouter API
func GeneratePlaylistQueries(query string) []string {
	if OpenRouterAPIKey == "" {
		return []string{}
	}

	// Use a free-tier model
	model := "google/gemini-2.5-flash" // Automatically selects the best free model
	// Alternative free models: "mistralai/mixtral-8x7b-instruct", "meta-llama/llama-3-8b-instruct"

	systemPrompt := "You are a music playlist generator. Generate song suggestions based on similarity and songs the user may like in the format 'Artist - Song Title', one per line. Only output the song suggestions, nothing else."
	userPrompt := fmt.Sprintf("Generate 20-25 song suggestions for a playlist based on: %s", query)

	requestBody := map[string]interface{}{
		"model":       model,
		"messages":    []map[string]string{{"role": "system", "content": systemPrompt}, {"role": "user", "content": userPrompt}},
		"max_tokens":  DefaultOpenRouterPlaylistMaxTokens,
		"temperature": 0.8,
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return []string{}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", OpenRouterAPIURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return []string{}
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", OpenRouterAPIKey))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("HTTP-Referer", "https://github.com/System-Nebula/music-botting/tree/refactored")
	req.Header.Set("X-Title", "Ezra Music Bot - Playlist Generator")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return []string{}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		io.ReadAll(resp.Body)
		return []string{}
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return []string{}
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return []string{}
	}

	if result.Error.Message != "" {
		return []string{}
	}

	if len(result.Choices) == 0 || result.Choices[0].Message.Content == "" {
		return []string{}
	}

	content := result.Choices[0].Message.Content
	queries := parseSongQueries(content)

	return queries
}

// GenerateRadioSuggestions generates song suggestions based on seed and recently played songs
func GenerateRadioSuggestions(seed string, recentSongs []string) []string {
	if OpenRouterAPIKey == "" {
		return []string{}
	}

	model := "google/gemini-2.5-flash"

	// Build context from recent songs
	recentContext := ""
	if len(recentSongs) > 0 {
		recentContext = fmt.Sprintf("\nRecently played songs:\n%s\n\nGenerate songs similar to these but avoid suggesting any of them.", strings.Join(recentSongs, "\n"))
	}

	systemPrompt := "You are a music radio DJ. Generate song suggestions that flow well together, maintaining a consistent mood and style. Format each suggestion as 'Artist - Song Title', one per line. Only output the song suggestions, nothing else."
	userPrompt := fmt.Sprintf("The listener started a radio station based on: %s%s\n\nGenerate 8-10 new song suggestions that would fit this radio station perfectly.", seed, recentContext)

	requestBody := map[string]interface{}{
		"model":       model,
		"messages":    []map[string]string{{"role": "system", "content": systemPrompt}, {"role": "user", "content": userPrompt}},
		"max_tokens":  DefaultOpenRouterMaxTokens,
		"temperature": 0.9, // Slightly higher temperature for more variety
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return []string{}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", OpenRouterAPIURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return []string{}
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", OpenRouterAPIKey))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("HTTP-Referer", "https://github.com/System-Nebula/music-botting/tree/refactored")
	req.Header.Set("X-Title", "Ezra Music Bot - Infinite Radio")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return []string{}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		io.ReadAll(resp.Body)
		return []string{}
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return []string{}
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return []string{}
	}

	if result.Error.Message != "" {
		return []string{}
	}

	if len(result.Choices) == 0 || result.Choices[0].Message.Content == "" {
		return []string{}
	}

	content := result.Choices[0].Message.Content
	queries := parseSongQueries(content)

	return queries
}

// parseSongQueries parses the AI response to extract song queries in "Artist - Song" format
func parseSongQueries(content string) []string {
	var queries []string
	lines := strings.Split(content, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Remove common prefixes like "-", "*", etc.
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "-") {
			line = strings.TrimSpace(line[1:])
		}
		if strings.HasPrefix(line, "*") {
			line = strings.TrimSpace(line[1:])
		}

		// Remove numbered prefixes (e.g., "1. ", "2. ", "10. ", etc.)
		// Match pattern: one or more digits followed by ". "
		for i := 0; i < len(line); i++ {
			if i > 0 && line[i] == '.' && i+1 < len(line) && line[i+1] == ' ' {
				// Check if all characters before '.' are digits
				allDigits := true
				for j := 0; j < i; j++ {
					if line[j] < '0' || line[j] > '9' {
						allDigits = false
						break
					}
				}
				if allDigits {
					line = strings.TrimSpace(line[i+2:])
					break
				}
			}
		}

		line = strings.TrimSpace(line)

		// Skip lines that don't look like "Artist - Song" format
		if !strings.Contains(line, " - ") {
			// Try to extract if it's in a different format
			// Some models might format as "Song by Artist"
			if strings.Contains(strings.ToLower(line), " by ") {
				parts := strings.SplitN(line, " by ", 2)
				if len(parts) == 2 {
					line = fmt.Sprintf("%s - %s", strings.TrimSpace(parts[1]), strings.TrimSpace(parts[0]))
				} else {
					continue
				}
			} else {
				// Skip lines that don't match expected format
				continue
			}
		}

		// Validate the format
		parts := strings.SplitN(line, " - ", 2)
		if len(parts) == 2 {
			artist := strings.TrimSpace(parts[0])
			song := strings.TrimSpace(parts[1])
			if artist != "" && song != "" {
				queries = append(queries, line)
			}
		}
	}

	return queries
}
