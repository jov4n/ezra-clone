package tools

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
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

	// Set headers to look like a real browser
	// Note: We accept gzip but will decompress it manually
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Accept-Encoding", "gzip, deflate") // Accept gzip but we'll decompress
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Upgrade-Insecure-Requests", "1")

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("Failed to fetch: %v", err)}
	}
	defer resp.Body.Close()

	// Handle redirects (but limit to prevent infinite loops)
	redirectCount := 0
	maxRedirects := 5
	for resp.StatusCode >= 300 && resp.StatusCode < 400 && redirectCount < maxRedirects {
		location := resp.Header.Get("Location")
		if location == "" {
			return &ToolResult{Success: false, Error: fmt.Sprintf("HTTP %d (redirect without location)", resp.StatusCode)}
		}
		
		// Handle relative redirects
		if !strings.HasPrefix(location, "http://") && !strings.HasPrefix(location, "https://") {
			baseURL, err := url.Parse(urlStr)
			if err == nil {
				location = baseURL.ResolveReference(&url.URL{Path: location}).String()
			}
		}
		
		redirectCount++
		urlStr = location
		
		// Create new request for redirect
		req, err = http.NewRequestWithContext(ctx, "GET", urlStr, nil)
		if err != nil {
			return &ToolResult{Success: false, Error: fmt.Sprintf("Invalid redirect URL: %v", err)}
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")
		req.Header.Set("Accept-Language", "en-US,en;q=0.9")
		req.Header.Set("Accept-Encoding", "gzip, deflate") // Accept gzip but we'll decompress
		req.Header.Set("Connection", "keep-alive")
		req.Header.Set("Upgrade-Insecure-Requests", "1")
		
		resp, err = e.httpClient.Do(req)
		if err != nil {
			return &ToolResult{Success: false, Error: fmt.Sprintf("Failed to follow redirect: %v", err)}
		}
		defer resp.Body.Close()
	}
	
	if redirectCount >= maxRedirects {
		return &ToolResult{Success: false, Error: "Too many redirects"}
	}

	if resp.StatusCode != 200 {
		return &ToolResult{Success: false, Error: fmt.Sprintf("HTTP %d: %s", resp.StatusCode, resp.Status)}
	}

	// Check content type - be lenient (some servers don't set it correctly)
	contentType := resp.Header.Get("Content-Type")
	isHTML := strings.Contains(strings.ToLower(contentType), "text/html") || 
	          strings.Contains(strings.ToLower(contentType), "application/xhtml") ||
	          strings.Contains(strings.ToLower(contentType), "text/plain") ||
	          contentType == ""
	
	if !isHTML && contentType != "" {
		// Log warning but continue - might still be HTML
		e.logger.Debug("Unexpected content type, attempting to parse as HTML anyway",
			zap.String("content_type", contentType),
			zap.String("url", urlStr),
		)
	}

	// Handle compressed content (gzip, deflate, br)
	var reader io.Reader = resp.Body
	contentEncoding := resp.Header.Get("Content-Encoding")
	
	if strings.Contains(strings.ToLower(contentEncoding), "gzip") {
		gzipReader, err := gzip.NewReader(resp.Body)
		if err != nil {
			return &ToolResult{Success: false, Error: fmt.Sprintf("Failed to decompress gzip: %v", err)}
		}
		defer gzipReader.Close()
		reader = gzipReader
		e.logger.Debug("Decompressing gzip content", zap.String("url", urlStr))
	} else if strings.Contains(strings.ToLower(contentEncoding), "br") {
		// Brotli compression - would need additional library
		// For now, try to read as-is (some content might still be readable)
		e.logger.Debug("Brotli compression detected but not supported, attempting to read anyway", zap.String("url", urlStr))
	}

	// Read content with larger limit for articles (500KB instead of 50KB)
	body, err := io.ReadAll(io.LimitReader(reader, 500000)) // 500KB limit for articles
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("Failed to read content: %v", err)}
	}

	if len(body) == 0 {
		return &ToolResult{Success: false, Error: "Empty response from server"}
	}
	
	// Check if content looks like binary/garbled (might be compressed but not detected)
	// Try to detect if it's gzip (starts with 0x1f 0x8b)
	if len(body) >= 2 && body[0] == 0x1f && body[1] == 0x8b {
		// It's gzip but wasn't detected in Content-Encoding, try to decompress
		gzipReader, err := gzip.NewReader(bytes.NewReader(body))
		if err == nil {
			decompressed, err := io.ReadAll(gzipReader)
			gzipReader.Close()
			if err == nil && len(decompressed) > 0 {
				body = decompressed
				e.logger.Debug("Auto-detected and decompressed gzip content", zap.String("url", urlStr))
			}
		}
	}

	// Extract structured content from HTML
	htmlContent := string(body)
	originalLength := len(htmlContent)
	
	// Use structured extraction (max 50,000 chars for full text)
	structuredContent := extractStructuredContent(htmlContent, 50000)
	
	// Log extraction stats for debugging
	e.logger.Debug("Structured HTML extraction",
		zap.String("url", urlStr),
		zap.Int("original_bytes", originalLength),
		zap.Int("extracted_chars", structuredContent.TextLength),
		zap.Int("num_sections", len(structuredContent.Sections)),
		zap.String("title", structuredContent.Title),
	)

	// Validate extraction - check if extraction is too small relative to original HTML
	// If we got less than 0.5% of the original HTML size (and original is > 10KB), it's likely a failed extraction
	extractionRatio := float64(structuredContent.TextLength) / float64(originalLength)
	shouldFallback := structuredContent.TextLength == 0 || 
	                  structuredContent.FullText == "" ||
	                  (originalLength > 10000 && extractionRatio < 0.005) ||
	                  (originalLength > 1000 && structuredContent.TextLength < 100 && len(structuredContent.Sections) == 0)
	
	if shouldFallback {
		// Fallback to simple extraction if structured extraction fails
		e.logger.Debug("Structured extraction failed or insufficient, trying fallback",
			zap.Int("original", originalLength),
			zap.Int("extracted", structuredContent.TextLength),
			zap.Float64("ratio", extractionRatio),
		)
		
		fallbackContent := extractTextFromHTMLSimple(htmlContent)
		
		if len(fallbackContent) == 0 || (originalLength > 1000 && len(fallbackContent) < originalLength/100) {
			return &ToolResult{Success: false, Error: "Could not extract text content from webpage (may be JavaScript-rendered or empty)"}
		}
		
		// Use fallback content - but try to structure it better
		// Build a simple structured response with the fallback content
		title := structuredContent.Title
		if title == "" || title == "Untitled" {
			// Extract title using regex (simple method)
			titleRegex := regexp.MustCompile(`(?i)<title[^>]*>(.*?)</title>`)
			if matches := titleRegex.FindStringSubmatch(htmlContent); len(matches) > 1 {
				title = matches[1]
				title = stripHTMLTags(title)
				title = decodeHTMLEntities(title)
				title = strings.TrimSpace(title)
			}
			if title == "" {
				title = "Untitled"
			}
		}
		
		// Format fallback content as markdown
		formattedContent := title
		if title != "" && title != "Untitled" {
			formattedContent = fmt.Sprintf("# %s\n\n", title)
		}
		formattedContent += fallbackContent
		
		// Truncate if too long (increased limit for articles)
		if len(formattedContent) > 50000 {
			truncated := formattedContent[:50000]
			if lastPeriod := strings.LastIndex(truncated, "."); lastPeriod > 45000 {
				formattedContent = truncated[:lastPeriod+1] + "\n\n... [content truncated]"
			} else {
				formattedContent = truncated + "\n\n... [content truncated]"
			}
		}
		
		return &ToolResult{
			Success: true,
			Data: map[string]interface{}{
				"url":         urlStr,
				"title":       title,
				"content":     formattedContent,
				"full_text":   formattedContent,
				"text_length": len(formattedContent),
				"num_sections": 0,
				"fallback_used": true,
			},
			Message: fmt.Sprintf("Extracted %d characters using fallback extraction from %s", len(formattedContent), urlStr),
		}
	}

	// Build response with structured content
	responseData := map[string]interface{}{
		"url":         urlStr,
		"title":       structuredContent.Title,
		"content":     structuredContent.FullText, // Full markdown-formatted text
		"full_text":   structuredContent.FullText,  // Alias for compatibility
		"sections":    structuredContent.Sections,
		"metadata":    structuredContent.Metadata,
		"text_length": structuredContent.TextLength,
		"num_sections": len(structuredContent.Sections),
	}

	// Add source URL to metadata
	if structuredContent.Metadata == nil {
		structuredContent.Metadata = make(map[string]string)
	}
	structuredContent.Metadata["source_url"] = urlStr

	message := fmt.Sprintf("Extracted %d characters in %d sections from %s", 
		structuredContent.TextLength, 
		len(structuredContent.Sections), 
		urlStr)
	
	// If content is long, suggest using summarize_website for better summarization
	if structuredContent.TextLength > 8000 {
		message += fmt.Sprintf(". Note: For AI-powered summarization of this long article (%d chars), consider using summarize_website tool.", structuredContent.TextLength)
	}

	return &ToolResult{
		Success: true,
		Data:    responseData,
		Message: message,
	}
}

