package tools

import (
	"fmt"
	"regexp"
	"strings"
)

// ============================================================================
// Structured Content Extraction
// ============================================================================

// ContentSection represents a section of content with optional heading
type ContentSection struct {
	Heading string   `json:"heading,omitempty"`
	Level   int      `json:"level,omitempty"` // 1-6 for h1-h6, 0 for no heading
	Content []string  `json:"content"`
}

// StructuredContent represents extracted structured content from a webpage
type StructuredContent struct {
	Title     string           `json:"title"`
	FullText  string           `json:"full_text"`
	Sections  []ContentSection `json:"sections"`
	Metadata  map[string]string `json:"metadata"`
	TextLength int             `json:"text_length"`
}

// extractStructuredContent extracts structured content from HTML with headings and sections
func extractStructuredContent(htmlContent string, maxLength int) *StructuredContent {
	result := &StructuredContent{
		Metadata: make(map[string]string),
		Sections: []ContentSection{},
	}

	// Extract title first
	result.Title = extractTitle(htmlContent)

	// Remove unwanted elements
	htmlContent = removeTagContent(htmlContent, "script")
	htmlContent = removeTagContent(htmlContent, "style")
	htmlContent = removeTagContent(htmlContent, "noscript")
	htmlContent = removeTagContent(htmlContent, "nav")
	htmlContent = removeTagContent(htmlContent, "footer")
	htmlContent = removeTagContent(htmlContent, "header")
	htmlContent = removeTagContent(htmlContent, "aside")
	htmlContent = removeComments(htmlContent)

	// Extract metadata
	result.Metadata = extractMetadata(htmlContent)

	// Try to find main content area
	contentHTML := findMainContent(htmlContent)

	// Extract structured sections
	sections := extractSections(contentHTML)
	
	// If structured extraction found very little, try using simple extraction as fallback
	// but still structure it
	if len(sections) == 0 || (len(sections) == 1 && len(sections[0].Content) == 0) {
		// Try simple extraction and structure it
		simpleText := extractTextFromHTMLSimple(contentHTML)
		if len(simpleText) > 500 {
			// Split into paragraphs based on double newlines or sentence boundaries
			parts := strings.Split(simpleText, ". ")
			var paragraphs []string
			currentPara := ""
			for _, part := range parts {
				part = strings.TrimSpace(part)
				if len(part) > 20 {
					if currentPara != "" {
						currentPara += ". " + part
					} else {
						currentPara = part
					}
					// Create paragraph every ~200 chars or at sentence boundaries
					if len(currentPara) > 200 {
						paragraphs = append(paragraphs, currentPara+".")
						currentPara = ""
					}
				}
			}
			if currentPara != "" {
				paragraphs = append(paragraphs, currentPara+".")
			}
			if len(paragraphs) > 0 {
				sections = []ContentSection{{Content: paragraphs}}
			}
		}
	}

	// Build full text with markdown formatting
	fullTextParts := []string{}
	if result.Title != "" {
		fullTextParts = append(fullTextParts, fmt.Sprintf("# %s\n", result.Title))
	}

	// Add metadata if available
	if result.Metadata["author"] != "" {
		fullTextParts = append(fullTextParts, fmt.Sprintf("**Author:** %s\n", result.Metadata["author"]))
	}
	if result.Metadata["date"] != "" {
		fullTextParts = append(fullTextParts, fmt.Sprintf("**Date:** %s\n", result.Metadata["date"]))
	}
	if len(result.Metadata) > 0 {
		fullTextParts = append(fullTextParts, "\n---\n\n")
	}

	// Add sections
	for _, section := range sections {
		if section.Heading != "" {
			headingPrefix := strings.Repeat("#", section.Level+1) // +1 because title is h1
			fullTextParts = append(fullTextParts, fmt.Sprintf("\n%s %s\n", headingPrefix, section.Heading))
		}
		for _, para := range section.Content {
			if strings.TrimSpace(para) != "" {
				fullTextParts = append(fullTextParts, para)
			}
		}
	}

	result.FullText = strings.Join(fullTextParts, "\n")

	// Truncate if needed
	if len(result.FullText) > maxLength {
		truncated := result.FullText[:maxLength]
		// Try to truncate at a sentence boundary
		if lastPeriod := strings.LastIndex(truncated, "."); lastPeriod > maxLength*3/4 {
			result.FullText = truncated[:lastPeriod+1] + "\n\n... [content truncated]"
		} else {
			result.FullText = truncated + "\n\n... [content truncated]"
		}
	}

	result.TextLength = len(result.FullText)
	result.Sections = sections

	// Limit sections to prevent overwhelming output
	if len(result.Sections) > 30 {
		result.Sections = result.Sections[:30]
	}

	return result
}

// extractTitle extracts the page title
func extractTitle(htmlContent string) string {
	// Try <title> tag
	titleRegex := regexp.MustCompile(`(?i)<title[^>]*>(.*?)</title>`)
	if matches := titleRegex.FindStringSubmatch(htmlContent); len(matches) > 1 {
		title := matches[1]
		title = stripHTMLTags(title)
		title = decodeHTMLEntities(title)
		title = strings.TrimSpace(title)
		if title != "" {
			return title
		}
	}

	// Try <h1> tag
	h1Regex := regexp.MustCompile(`(?i)<h1[^>]*>(.*?)</h1>`)
	if matches := h1Regex.FindStringSubmatch(htmlContent); len(matches) > 1 {
		title := matches[1]
		title = stripHTMLTags(title)
		title = decodeHTMLEntities(title)
		title = strings.TrimSpace(title)
		if title != "" {
			return title
		}
	}

	return "Untitled"
}

// extractMetadata extracts metadata like author and date
func extractMetadata(htmlContent string) map[string]string {
	metadata := make(map[string]string)

	// Try to find author
	authorPatterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)<(?:span|div|p)[^>]*class="[^"]*(?:author|byline|writer)[^"]*"[^>]*>(.*?)</(?:span|div|p)>`),
		regexp.MustCompile(`(?i)<meta[^>]*name="author"[^>]*content="([^"]*)"`),
		regexp.MustCompile(`(?i)<meta[^>]*property="article:author"[^>]*content="([^"]*)"`),
	}

	for _, pattern := range authorPatterns {
		if matches := pattern.FindStringSubmatch(htmlContent); len(matches) > 1 {
			author := matches[1]
			author = stripHTMLTags(author)
			author = decodeHTMLEntities(author)
			author = strings.TrimSpace(author)
			if author != "" && len(author) < 200 {
				metadata["author"] = author
				break
			}
		}
	}

	// Try to find date
	datePatterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)<time[^>]*datetime="([^"]*)"`),
		regexp.MustCompile(`(?i)<time[^>]*>(.*?)</time>`),
		regexp.MustCompile(`(?i)<(?:span|div|p)[^>]*class="[^"]*(?:date|time|published)[^"]*"[^>]*>(.*?)</(?:span|div|p)>`),
		regexp.MustCompile(`(?i)<meta[^>]*property="article:published_time"[^>]*content="([^"]*)"`),
	}

	for _, pattern := range datePatterns {
		if matches := pattern.FindStringSubmatch(htmlContent); len(matches) > 1 {
			date := matches[1]
			date = stripHTMLTags(date)
			date = decodeHTMLEntities(date)
			date = strings.TrimSpace(date)
			if date != "" && len(date) < 100 {
				metadata["date"] = date
				break
			}
		}
	}

	return metadata
}

// findMainContent tries to find the main article content area
// Uses a more robust approach that handles nested tags
func findMainContent(htmlContent string) string {
	// Try to find <article> tag with proper nesting handling
	if content := extractNestedTag(htmlContent, "article"); content != "" {
		return content
	}

	// Try to find <main> tag
	if content := extractNestedTag(htmlContent, "main"); content != "" {
		return content
	}

	// Try to find div with article/content/post/blog class (with nesting)
	contentClassPatterns := []string{
		`(?i)class="[^"]*article[^"]*"`,
		`(?i)class="[^"]*content[^"]*"`,
		`(?i)class="[^"]*post[^"]*"`,
		`(?i)class="[^"]*blog[^"]*"`,
		`(?i)class="[^"]*entry[^"]*"`,
		`(?i)class="[^"]*main-content[^"]*"`,
	}
	
	for _, pattern := range contentClassPatterns {
		classRegex := regexp.MustCompile(`<div[^>]*` + pattern + `[^>]*>`)
		matches := classRegex.FindStringSubmatchIndex(htmlContent)
		if len(matches) > 0 {
			startIdx := matches[1] // End of opening tag
			// Find the matching closing </div> tag
			if content := extractNestedTagFromIndex(htmlContent, "div", startIdx); content != "" {
				return content
			}
		}
	}

	// Try to find <body> tag
	if content := extractNestedTag(htmlContent, "body"); content != "" {
		return content
	}

	// Fallback to entire HTML
	return htmlContent
}

// extractNestedTag extracts content from a tag, handling nested tags properly
func extractNestedTag(htmlContent, tagName string) string {
	openTag := regexp.MustCompile(fmt.Sprintf(`(?i)<%s[^>]*>`, tagName))
	matches := openTag.FindStringSubmatchIndex(htmlContent)
	if len(matches) == 0 {
		return ""
	}
	startIdx := matches[1] // End of opening tag
	return extractNestedTagFromIndex(htmlContent, tagName, startIdx)
}

// extractNestedTagFromIndex extracts content from a tag starting at a given index, handling nesting
func extractNestedTagFromIndex(htmlContent, tagName string, startIdx int) string {
	if startIdx >= len(htmlContent) {
		return ""
	}
	
	openTagPattern := fmt.Sprintf(`(?i)<%s[^>]*>`, tagName)
	closeTagPattern := fmt.Sprintf(`(?i)</%s>`, tagName)
	
	openTagRegex := regexp.MustCompile(openTagPattern)
	closeTagRegex := regexp.MustCompile(closeTagPattern)
	
	depth := 1
	i := startIdx
	
	for i < len(htmlContent) {
		// Find next open or close tag
		nextOpen := openTagRegex.FindStringIndex(htmlContent[i:])
		nextClose := closeTagRegex.FindStringIndex(htmlContent[i:])
		
		var nextTagIdx int = len(htmlContent)
		var isOpen bool
		
		if nextOpen != nil && nextClose != nil {
			if nextOpen[0] < nextClose[0] {
				nextTagIdx = i + nextOpen[0]
				isOpen = true
			} else {
				nextTagIdx = i + nextClose[0]
				isOpen = false
			}
		} else if nextOpen != nil {
			nextTagIdx = i + nextOpen[0]
			isOpen = true
		} else if nextClose != nil {
			nextTagIdx = i + nextClose[0]
			isOpen = false
		} else {
			// No more tags found, return what we have
			break
		}
		
		if isOpen {
			depth++
		} else {
			depth--
			if depth == 0 {
				// Found matching closing tag
				return htmlContent[startIdx:nextTagIdx]
			}
		}
		
		i = nextTagIdx + 1
	}
	
	// Didn't find matching closing tag, return everything from start
	return htmlContent[startIdx:]
}

// extractSections extracts structured sections with headings and paragraphs
func extractSections(htmlContent string) []ContentSection {
	sections := []ContentSection{}
	currentSection := ContentSection{}

	// Pattern to match headings and paragraphs
	// We'll process the HTML sequentially
	content := htmlContent

	// Extract all headings and their following content
	// Use non-greedy matching with DOTALL equivalent (handling multiline)
	headingRegex := regexp.MustCompile(`(?is)<(h[1-6])[^>]*>(.*?)</h[1-6]>`)
	allMatches := headingRegex.FindAllStringSubmatchIndex(content, -1)

	if len(allMatches) == 0 {
		// No headings found, extract all paragraphs as a single section
		paragraphs := extractParagraphs(content)
		if len(paragraphs) > 0 {
			return []ContentSection{{Content: paragraphs}}
		}
		// If no paragraphs either, try extracting any text content
		// This handles cases where content might be in divs without proper structure
		textContent := extractTextFromHTMLSimple(content)
		if len(textContent) > 100 {
			// Split into sentences/paragraphs
			parts := strings.Split(textContent, ". ")
			var paragraphs []string
			for _, part := range parts {
				part = strings.TrimSpace(part)
				if len(part) > 20 {
					paragraphs = append(paragraphs, part+".")
				}
			}
			if len(paragraphs) > 0 {
				return []ContentSection{{Content: paragraphs}}
			}
		}
		return sections
	}

	// Process content between headings
	for i, match := range allMatches {
		headingLevel := int(content[match[2]+1] - '0') // Extract number from h1-h6
		headingText := content[match[4]:match[5]]
		headingText = stripHTMLTags(headingText)
		headingText = decodeHTMLEntities(headingText)
		headingText = strings.TrimSpace(headingText)

		// Save previous section if it has content
		if currentSection.Heading != "" || len(currentSection.Content) > 0 {
			sections = append(sections, currentSection)
		}

		// Start new section
		currentSection = ContentSection{
			Heading: headingText,
			Level:   headingLevel,
			Content: []string{},
		}

		// Extract content between this heading and next heading (or end)
		contentStart := match[1] // End of current heading tag
		contentEnd := len(content)
		if i+1 < len(allMatches) {
			contentEnd = allMatches[i+1][0] // Start of next heading
		}

		sectionContent := content[contentStart:contentEnd]
		paragraphs := extractParagraphs(sectionContent)
		currentSection.Content = paragraphs
	}

	// Add last section
	if currentSection.Heading != "" || len(currentSection.Content) > 0 {
		sections = append(sections, currentSection)
	}

	// Filter out sections with no meaningful content
	filteredSections := []ContentSection{}
	for _, section := range sections {
		hasContent := false
		for _, para := range section.Content {
			if strings.TrimSpace(para) != "" && len(strings.TrimSpace(para)) > 10 {
				hasContent = true
				break
			}
		}
		if hasContent || section.Heading != "" {
			filteredSections = append(filteredSections, section)
		}
	}

	return filteredSections
}

// extractParagraphs extracts paragraph text from HTML
func extractParagraphs(htmlContent string) []string {
	paragraphs := []string{}

	// Extract <p> tags (use DOTALL mode to handle multiline)
	pRegex := regexp.MustCompile(`(?is)<p[^>]*>(.*?)</p>`)
	matches := pRegex.FindAllStringSubmatch(htmlContent, -1)
	for _, match := range matches {
		if len(match) > 1 {
			text := match[1]
			text = stripHTMLTags(text)
			text = decodeHTMLEntities(text)
			text = strings.TrimSpace(text)
			if text != "" && len(text) > 10 {
				paragraphs = append(paragraphs, text)
			}
		}
	}

	// Extract <li> tags (list items)
	liRegex := regexp.MustCompile(`(?is)<li[^>]*>(.*?)</li>`)
	matches = liRegex.FindAllStringSubmatch(htmlContent, -1)
	for _, match := range matches {
		if len(match) > 1 {
			text := match[1]
			text = stripHTMLTags(text)
			text = decodeHTMLEntities(text)
			text = strings.TrimSpace(text)
			if text != "" && len(text) > 10 {
				paragraphs = append(paragraphs, text)
			}
		}
	}

	// If no paragraphs found, try extracting text from divs with substantial content
	// But be more selective - only divs that look like content (not navigation, etc.)
	if len(paragraphs) == 0 {
		// Try to find content divs (avoid nav, header, footer, etc.)
		divRegex := regexp.MustCompile(`(?is)<div[^>]*>(.*?)</div>`)
		matches = divRegex.FindAllStringSubmatch(htmlContent, -1)
		for _, match := range matches {
			if len(match) > 1 {
				// Check if this div is likely content (not navigation/header/footer)
				divTag := match[0]
				divTagLower := strings.ToLower(divTag)
				if strings.Contains(divTagLower, "nav") || 
				   strings.Contains(divTagLower, "header") || 
				   strings.Contains(divTagLower, "footer") ||
				   strings.Contains(divTagLower, "sidebar") ||
				   strings.Contains(divTagLower, "menu") {
					continue
				}
				
				text := match[1]
				text = stripHTMLTags(text)
				text = decodeHTMLEntities(text)
				text = strings.TrimSpace(text)
				// Only add divs with substantial content (more than 50 chars)
				if text != "" && len(text) > 50 {
					paragraphs = append(paragraphs, text)
				}
			}
		}
	}

	return paragraphs
}


