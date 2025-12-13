package sources

import (
	"context"
	"fmt"
	"strings"
	"time"

	"ezra-clone/backend/internal/adapter"
)

// GeneratePlaylistQueries generates song search queries using LiteLLM adapter
func GeneratePlaylistQueries(ctx context.Context, llmAdapter *adapter.LLMAdapter, query string) []string {
	if llmAdapter == nil {
		return []string{}
	}

	// Create context with timeout
	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	systemPrompt := "You are a music playlist generator. Generate song suggestions based on similarity and songs the user may like in the format 'Artist - Song Title', one per line. Only output the song suggestions, nothing else."
	userPrompt := fmt.Sprintf("Generate 20-25 song suggestions for a playlist based on: %s", query)

	response, err := llmAdapter.Generate(reqCtx, systemPrompt, userPrompt, []adapter.Tool{})
	if err != nil {
		return []string{}
	}

	if response.Content == "" {
		return []string{}
	}

	queries := parseSongQueries(response.Content)
	return queries
}

// GenerateRadioSuggestions generates song suggestions based on seed and recently played songs using LiteLLM adapter
func GenerateRadioSuggestions(ctx context.Context, llmAdapter *adapter.LLMAdapter, seed string, recentSongs []string) []string {
	if llmAdapter == nil {
		return []string{}
	}

	// Create context with timeout
	reqCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	// Build context from recent songs
	recentContext := ""
	if len(recentSongs) > 0 {
		recentContext = fmt.Sprintf("\nRecently played songs:\n%s\n\nGenerate songs similar to these but avoid suggesting any of them.", strings.Join(recentSongs, "\n"))
	}

	systemPrompt := "You are a music radio DJ. Generate song suggestions that flow well together, maintaining a consistent mood and style. Format each suggestion as 'Artist - Song Title', one per line. Only output the song suggestions, nothing else."
	userPrompt := fmt.Sprintf("The listener started a radio station based on: %s%s\n\nGenerate 8-10 new song suggestions that would fit this radio station perfectly.", seed, recentContext)

	response, err := llmAdapter.Generate(reqCtx, systemPrompt, userPrompt, []adapter.Tool{})
	if err != nil {
		return []string{}
	}

	if response.Content == "" {
		return []string{}
	}

	queries := parseSongQueries(response.Content)
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
