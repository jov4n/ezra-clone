package agent

import (
	"fmt"
	"strings"
	"time"

	"ezra-clone/backend/internal/graph"
	"ezra-clone/backend/internal/tools"
)

// isInformationalTool returns true for tools that fetch/return data (not actions)
func isInformationalTool(toolName string) bool {
	informationalTools := map[string]bool{
		tools.ToolGitHubListOrgRepos: true,
		tools.ToolGitHubRepoInfo:     true,
		tools.ToolGitHubSearch:       true,
		tools.ToolGitHubReadFile:     true,
		tools.ToolWebSearch:          true,
		tools.ToolFetchWebpage:       true,
		tools.ToolSearchFacts:        true,
		tools.ToolGetUserContext:     true,
		tools.ToolFindRelated:        true,
		tools.ToolArchivalSearch:     true,
		tools.ToolMemorySearch:       true,
		tools.ToolGetHistory:         true,
		tools.ToolDiscordReadHistory: true,
		tools.ToolAnalyzeUserStyle:   true,
	}
	return informationalTools[toolName]
}

// formatToolResponseWithEmbeds formats tool results into a response and optional embeds
func formatToolResponseWithEmbeds(toolName string, result *tools.ToolResult) (string, []Embed) {
	switch toolName {
	case tools.ToolGitHubListOrgRepos:
		if repos, ok := result.Data.([]map[string]interface{}); ok && len(repos) > 0 {
			// Most recent repo is first
			mostRecent := repos[0]
			name := mostRecent["name"]
			desc := mostRecent["description"]
			updated := formatTimestamp(mostRecent["updated_at"])
			
			if len(repos) == 1 {
				if desc != nil && desc != "" {
					return fmt.Sprintf("The most recently updated repo is **%v** - %v. It was last updated %v.", name, desc, updated), nil
				}
				return fmt.Sprintf("The most recently updated repo is **%v**, last updated %v.", name, updated), nil
			}
			
			// Multiple repos
			var others []string
			for i := 1; i < len(repos) && i < 4; i++ {
				others = append(others, fmt.Sprintf("%v", repos[i]["name"]))
			}
			
			response := fmt.Sprintf("The most recently updated repo is **%v**", name)
			if desc != nil && desc != "" {
				response += fmt.Sprintf(" (%v)", desc)
			}
			response += fmt.Sprintf(", last updated %v.", updated)
			
			if len(others) > 0 {
				response += fmt.Sprintf(" Other recent repos: %s.", strings.Join(others, ", "))
			}
			return response, nil
		}
		return result.Message, nil

	case tools.ToolGitHubRepoInfo:
		if info, ok := result.Data.(map[string]interface{}); ok {
			name := info["full_name"]
			desc := info["description"]
			stars := info["stars"]
			lang := info["language"]
			updated := formatTimestamp(info["updated_at"])
			
			response := fmt.Sprintf("**%v**", name)
			if desc != nil && desc != "" {
				response += fmt.Sprintf(" - %v", desc)
			}
			response += fmt.Sprintf("\n\nIt has %v stars", stars)
			if lang != nil && lang != "" {
				response += fmt.Sprintf(", written in %v", lang)
			}
			response += fmt.Sprintf(", and was last updated %v.", updated)
			return response, nil
		}
		return result.Message, nil

	case tools.ToolGitHubSearch:
		if searchData, ok := result.Data.(map[string]interface{}); ok {
			if items, ok := searchData["items"].([]interface{}); ok && len(items) > 0 {
				var results []string
				for i, item := range items {
					if i >= 3 {
						break
					}
					if repo, ok := item.(map[string]interface{}); ok {
						repoName := repo["full_name"]
						desc := repo["description"]
						if desc == nil || desc == "" {
							desc = "No description"
						}
						results = append(results, fmt.Sprintf("• **%v** - %v", repoName, desc))
					}
				}
				if len(results) > 0 {
					return fmt.Sprintf("Here's what I found on GitHub:\n\n%s", strings.Join(results, "\n")), nil
				}
			}
		}
		return result.Message, nil

	case tools.ToolWebSearch:
		if searchData, ok := result.Data.(map[string]interface{}); ok {
			if resultsRaw, ok := searchData["results"]; ok {
				// Handle the new search results format - create embeds!
				if results, ok := resultsRaw.([]tools.SearchResult); ok && len(results) > 0 {
					var embeds []Embed
					
					for i, r := range results {
						if i >= 5 {
							break
						}
						embed := Embed{
							Title:       r.Title,
							URL:         r.URL,
							Description: r.Snippet,
							Color:       0x5865F2, // Discord blurple
						}
						embeds = append(embeds, embed)
					}
					
					// Use original question for more natural response, fallback to optimized query
					displayText := ""
					if origQ, ok := searchData["original_question"].(string); ok && origQ != "" {
						displayText = origQ
					} else if q, ok := searchData["query"]; ok {
						displayText = fmt.Sprintf("%v", q)
					}
					
					return fmt.Sprintf("Here's what I found for \"%s\":", displayText), embeds
				}
			}
			
			// Fallback for empty results
			displayText := ""
			if origQ, ok := searchData["original_question"].(string); ok && origQ != "" {
				displayText = origQ
			} else if q, ok := searchData["query"]; ok {
				displayText = fmt.Sprintf("%v", q)
			}
			if displayText != "" {
				return fmt.Sprintf("I couldn't find any results for \"%s\". Try rephrasing your search.", displayText), nil
			}
		}
		return result.Message, nil

	case tools.ToolGetUserContext:
		// Don't pre-format - let the LLM format it conversationally based on the raw data
		// The tool result message already contains the structured data
		return "", nil

	case tools.ToolSearchFacts:
		// Format facts search results
		if facts, ok := result.Data.([]graph.Fact); ok && len(facts) > 0 {
			var parts []string
			parts = append(parts, fmt.Sprintf("I found %d fact(s):", len(facts)))
			for _, fact := range facts {
				parts = append(parts, fmt.Sprintf("• %s", fact.Content))
			}
			return strings.Join(parts, "\n"), nil
		}
		return "I couldn't find any facts about that topic.", nil

	case tools.ToolFetchWebpage:
		// Format webpage content - keep it concise for Discord
		if webpageData, ok := result.Data.(map[string]interface{}); ok {
			url := webpageData["url"]
			content := webpageData["content"]
			
			if contentStr, ok := content.(string); ok && contentStr != "" {
				// For Discord, we need to be more aggressive with truncation
				// Keep it under 1800 chars to leave room for prefix text
				maxContentLength := 1800
				intro := fmt.Sprintf("I fetched the webpage content from %v. Here's what I found:\n\n", url)
				maxContentLength -= len(intro)
				
				if len(contentStr) > maxContentLength {
					// Try to truncate at a sentence boundary
					truncated := contentStr[:maxContentLength]
					if lastPeriod := strings.LastIndex(truncated, "."); lastPeriod > maxContentLength-200 {
						truncated = truncated[:lastPeriod+1]
					} else if lastNewline := strings.LastIndex(truncated, "\n"); lastNewline > maxContentLength-200 {
						truncated = truncated[:lastNewline]
					}
					return fmt.Sprintf("%s%s\n\n[Content truncated - page is %d characters long]", intro, truncated, len(contentStr)), nil
				}
				return fmt.Sprintf("%s%s", intro, contentStr), nil
			}
		}
		// Fallback to message if data format is unexpected
		if result.Message != "" {
			return result.Message, nil
		}
		return "I fetched the webpage but couldn't extract the content.", nil

	default:
		return "", nil // Let LLM handle other tools
	}
}

// formatTimestamp converts ISO timestamp to a more readable format
func formatTimestamp(ts interface{}) string {
	if ts == nil {
		return "recently"
	}
	tsStr := fmt.Sprintf("%v", ts)
	
	// Try to parse ISO format
	t, err := time.Parse(time.RFC3339, tsStr)
	if err != nil {
		// Try without timezone
		t, err = time.Parse("2006-01-02T15:04:05Z", tsStr)
		if err != nil {
			return tsStr
		}
	}
	
	// Calculate relative time
	now := time.Now()
	diff := now.Sub(t)
	
	switch {
	case diff < time.Hour:
		return fmt.Sprintf("%d minutes ago", int(diff.Minutes()))
	case diff < 24*time.Hour:
		return fmt.Sprintf("%d hours ago", int(diff.Hours()))
	case diff < 48*time.Hour:
		return "yesterday"
	case diff < 7*24*time.Hour:
		return fmt.Sprintf("%d days ago", int(diff.Hours()/24))
	case diff < 30*24*time.Hour:
		return fmt.Sprintf("%d weeks ago", int(diff.Hours()/(24*7)))
	default:
		return t.Format("January 2, 2006")
	}
}

