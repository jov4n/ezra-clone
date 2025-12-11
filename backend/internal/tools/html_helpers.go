package tools

import (
	"fmt"
	"strings"
)

// ============================================================================
// Helper Functions for HTML Processing
// ============================================================================

// extractTextFromHTML extracts meaningful text content from HTML, removing scripts, styles, and other non-content elements
func extractTextFromHTML(html string) string {
	// First, remove script and style tags completely (including their content)
	html = removeTagContent(html, "script")
	html = removeTagContent(html, "style")
	html = removeTagContent(html, "noscript")
	html = removeTagContent(html, "iframe")
	html = removeTagContent(html, "svg")
	
	// Remove comments
	html = removeComments(html)
	
	// Remove all remaining HTML tags to get plain text
	content := stripHTMLTags(html)
	
	// Decode HTML entities
	content = decodeHTMLEntities(content)
	
	// Clean up whitespace - normalize to single spaces
	content = strings.Join(strings.Fields(content), " ")
	
	// Split into sentences/paragraphs and filter out noise
	words := strings.Fields(content)
	var meaningfulWords []string
	skipNext := false
	
	for i, word := range words {
		if skipNext {
			skipNext = false
			continue
		}
		
		// Skip very short words that are likely noise
		if len(word) < 2 {
			continue
		}
		
		// Skip common UI/navigation words in isolation
		wordLower := strings.ToLower(strings.Trim(word, ".,!?;:"))
		if isLikelyUINoise(wordLower) && i < len(words)/10 {
			// Only skip if it's early in the content (likely header/nav)
			continue
		}
		
		meaningfulWords = append(meaningfulWords, word)
	}
	
	content = strings.Join(meaningfulWords, " ")
	
	// Final cleanup - remove excessive repetition
	content = removeExcessiveRepetition(content)
	
	return content
}

// removeExcessiveRepetition removes repeated words/phrases that are likely noise
func removeExcessiveRepetition(text string) string {
	words := strings.Fields(text)
	if len(words) < 10 {
		return text
	}
	
	var result []string
	seen := make(map[string]int)
	
	for _, word := range words {
		wordLower := strings.ToLower(word)
		seen[wordLower]++
		
		// If a word appears more than 20 times in a short text, it's likely noise
		if seen[wordLower] > 20 && len(words) < 200 {
			continue
		}
		
		result = append(result, word)
	}
	
	return strings.Join(result, " ")
}

// removeTagContent removes a tag and all its content
func removeTagContent(html, tagName string) string {
	var result strings.Builder
	tagStart := fmt.Sprintf("<%s", tagName)
	
	i := 0
	for i < len(html) {
		// Find start of tag
		startIdx := strings.Index(strings.ToLower(html[i:]), tagStart)
		if startIdx == -1 {
			// No more tags, append rest
			result.WriteString(html[i:])
			break
		}
		startIdx += i
		
		// Find the closing >
		closeIdx := strings.Index(html[startIdx:], ">")
		if closeIdx == -1 {
			result.WriteString(html[i:])
			break
		}
		closeIdx += startIdx + 1
		
		// Check if it's a self-closing tag or find the closing tag
		tagContent := html[startIdx:closeIdx]
		if strings.HasSuffix(tagContent, "/>") {
			// Self-closing, just skip it
			result.WriteString(html[i:startIdx])
			i = closeIdx
			continue
		}
		
		// Find the closing tag
		endTag := fmt.Sprintf("</%s>", tagName)
		endIdx := strings.Index(strings.ToLower(html[closeIdx:]), endTag)
		if endIdx == -1 {
			// No closing tag found, skip to end
			result.WriteString(html[i:startIdx])
			break
		}
		endIdx += closeIdx + len(endTag)
		
		// Skip the entire tag and its content
		result.WriteString(html[i:startIdx])
		i = endIdx
	}
	
	return result.String()
}

// removeComments removes HTML comments
func removeComments(html string) string {
	var result strings.Builder
	i := 0
	for i < len(html) {
		commentStart := strings.Index(html[i:], "<!--")
		if commentStart == -1 {
			result.WriteString(html[i:])
			break
		}
		commentStart += i
		result.WriteString(html[i:commentStart])
		
		commentEnd := strings.Index(html[commentStart:], "-->")
		if commentEnd == -1 {
			break
		}
		i = commentStart + commentEnd + 3
	}
	return result.String()
}

// isLikelyUINoise checks if a string is likely UI noise (navigation, buttons, etc.)
func isLikelyUINoise(s string) bool {
	noisePatterns := []string{
		"cookie", "privacy", "terms", "menu", "nav", "button", "click", "login", "sign up",
		"subscribe", "follow", "share", "like", "comment", "search", "filter", "sort",
	}
	sLower := strings.ToLower(s)
	for _, pattern := range noisePatterns {
		if strings.Contains(sLower, pattern) && len(s) < 20 {
			return true
		}
	}
	return false
}

func stripHTMLTags(s string) string {
	// Very basic HTML tag removal
	var result strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
			result.WriteRune(' ')
		case !inTag:
			result.WriteRune(r)
		}
	}
	// Clean up whitespace
	text := result.String()
	text = strings.Join(strings.Fields(text), " ")
	return text
}

