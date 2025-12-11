package tools

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"go.uber.org/zap"
)

// ============================================================================
// Web Tool Implementations
// ============================================================================

func (e *Executor) executeWebSearch(ctx context.Context, args map[string]interface{}) *ToolResult {
	query, _ := args["query"].(string)
	if query == "" {
		return &ToolResult{Success: false, Error: "query is required"}
	}

	// Capture original question if provided (for better response context)
	originalQuestion, _ := args["original_question"].(string)

	e.logger.Debug("Web search",
		zap.String("optimized_query", query),
		zap.String("original_question", originalQuestion),
	)

	// Use DuckDuckGo HTML search (free, no API key needed)
	searchURL := fmt.Sprintf("https://html.duckduckgo.com/html/?q=%s", url.QueryEscape(query))

	req, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("Failed to create request: %v", err)}
	}

	// Set headers to look like a browser
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Accept", "text/html")

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("Search failed: %v", err)}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return &ToolResult{Success: false, Error: "Failed to read response"}
	}

	html := string(body)

	// Parse search results from HTML
	results := parseSearchResults(html)

	if len(results) == 0 {
		return &ToolResult{
			Success: true,
			Data:    map[string]interface{}{"results": []string{}, "query": query, "original_question": originalQuestion},
			Message: fmt.Sprintf("No results found for: %s", query),
		}
	}

	return &ToolResult{
		Success: true,
		Data:    map[string]interface{}{"results": results, "query": query, "original_question": originalQuestion},
		Message: fmt.Sprintf("Found %d results for: %s", len(results), query),
	}
}

// SearchResult represents a single search result
type SearchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

// parseSearchResults extracts search results from DuckDuckGo HTML
func parseSearchResults(html string) []SearchResult {
	var results []SearchResult

	// Find all result blocks - they're in <div class="result">
	// We'll use simple string parsing since we can't import goquery
	
	// Split by result divs
	parts := strings.Split(html, `class="result__a"`)
	
	for i := 1; i < len(parts) && len(results) < 5; i++ {
		part := parts[i]
		
		result := SearchResult{}
		
		// Extract URL - it's in href="..."
		if hrefStart := strings.Index(part, `href="`); hrefStart != -1 {
			hrefStart += 6
			if hrefEnd := strings.Index(part[hrefStart:], `"`); hrefEnd != -1 {
				rawURL := part[hrefStart : hrefStart+hrefEnd]
				// DuckDuckGo wraps URLs, extract the actual URL
				if uddg := strings.Index(rawURL, "uddg="); uddg != -1 {
					actualURL := rawURL[uddg+5:]
					if ampIdx := strings.Index(actualURL, "&"); ampIdx != -1 {
						actualURL = actualURL[:ampIdx]
					}
					if decoded, err := url.QueryUnescape(actualURL); err == nil {
						result.URL = decoded
					}
				} else if !strings.HasPrefix(rawURL, "/") {
					result.URL = rawURL
				}
			}
		}
		
		// Extract title - it's the text after the href, before </a>
		if titleEnd := strings.Index(part, "</a>"); titleEnd != -1 {
			titleStart := strings.Index(part, ">")
			if titleStart != -1 && titleStart < titleEnd {
				title := part[titleStart+1 : titleEnd]
				title = stripHTMLTags(title)
				title = decodeHTMLEntities(title)
				title = strings.TrimSpace(title)
				result.Title = title
			}
		}
		
		// Extract snippet - look for result__snippet
		if snippetIdx := strings.Index(part, `class="result__snippet"`); snippetIdx != -1 {
			snippetPart := part[snippetIdx:]
			if start := strings.Index(snippetPart, ">"); start != -1 {
				// Find the closing </a> or </span> tag
				endTag := strings.Index(snippetPart[start:], "</a>")
				if endTag == -1 {
					endTag = strings.Index(snippetPart[start:], "</span>")
				}
				if endTag == -1 {
					endTag = strings.Index(snippetPart[start:], "</div>")
				}
				if endTag != -1 {
					snippet := snippetPart[start+1 : start+endTag]
					snippet = stripHTMLTags(snippet)
					snippet = decodeHTMLEntities(snippet)
					snippet = strings.TrimSpace(snippet)
					// Clean up whitespace
					snippet = strings.Join(strings.Fields(snippet), " ")
					if len(snippet) > 200 {
						snippet = snippet[:200] + "..."
					}
					result.Snippet = snippet
				}
			}
		}
		
		// Only add if we have at least a title
		if result.Title != "" {
			results = append(results, result)
		}
	}

	return results
}

// decodeHTMLEntities decodes common HTML entities
func decodeHTMLEntities(s string) string {
	replacements := map[string]string{
		"&amp;":   "&",
		"&lt;":    "<",
		"&gt;":    ">",
		"&quot;":  "\"",
		"&#39;":   "'",
		"&apos;":  "'",
		"&nbsp;":  " ",
		"&mdash;": "—",
		"&ndash;": "–",
		"&hellip;": "...",
		"&copy;":  "©",
		"&reg;":   "®",
		"&trade;": "™",
	}
	
	for entity, char := range replacements {
		s = strings.ReplaceAll(s, entity, char)
	}
	
	return s
}

func (e *Executor) executeFetchWebpage(ctx context.Context, args map[string]interface{}) *ToolResult {
	urlStr, _ := args["url"].(string)
	if urlStr == "" {
		return &ToolResult{Success: false, Error: "url is required"}
	}

	// Validate URL
	if !strings.HasPrefix(urlStr, "http://") && !strings.HasPrefix(urlStr, "https://") {
		urlStr = "https://" + urlStr
	}

	req, err := http.NewRequestWithContext(ctx, "GET", urlStr, nil)
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("Invalid URL: %v", err)}
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; EzraBot/1.0)")

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("Failed to fetch: %v", err)}
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return &ToolResult{Success: false, Error: fmt.Sprintf("HTTP %d", resp.StatusCode)}
	}

	// Read limited content
	body, err := io.ReadAll(io.LimitReader(resp.Body, 50000)) // 50KB limit
	if err != nil {
		return &ToolResult{Success: false, Error: "Failed to read content"}
	}

	// Extract meaningful text content from HTML
	content := string(body)
	content = extractTextFromHTML(content)

	// Truncate if too long
	if len(content) > 5000 {
		content = content[:5000] + "... (truncated)"
	}

	return &ToolResult{
		Success: true,
		Data: map[string]interface{}{
			"url":     urlStr,
			"content": content,
		},
	}
}

