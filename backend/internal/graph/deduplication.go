package graph

import (
	"regexp"
	"strings"
)

// ============================================================================
// Memory Deduplication Functions
// ============================================================================

// deduplicateFacts removes exact duplicates and very similar facts
func deduplicateFacts(facts []Fact) []Fact {
	if len(facts) <= 1 {
		return facts
	}

	seen := make(map[string]bool)
	var unique []Fact

	for _, fact := range facts {
		// Normalize content for comparison
		normalized := normalizeFactContent(fact.Content)
		
		// Check for exact duplicates
		if seen[normalized] {
			continue
		}

		// Check for very similar facts (simple string similarity)
		isDuplicate := false
		for seenContent := range seen {
			if areFactsSimilar(normalized, seenContent) {
				isDuplicate = true
				break
			}
		}

		if !isDuplicate {
			seen[normalized] = true
			unique = append(unique, fact)
		}
	}

	return unique
}

// normalizeFactContent normalizes fact content for comparison
func normalizeFactContent(content string) string {
	// Lowercase, trim, remove extra spaces
	content = strings.ToLower(strings.TrimSpace(content))
	content = regexp.MustCompile(`\s+`).ReplaceAllString(content, " ")
	// Remove trailing punctuation for better matching
	content = strings.TrimRight(content, ".,!?;:")
	return content
}

// areFactsSimilar checks if two facts are similar enough to be considered duplicates
func areFactsSimilar(content1, content2 string) bool {
	// Simple similarity check - can be enhanced with proper string similarity algorithms
	// For now, check if one contains the other (for very similar facts)
	if len(content1) < 10 || len(content2) < 10 {
		return false
	}

	// Check if they're very similar (one is substring of other with small differences)
	if strings.Contains(content1, content2) || strings.Contains(content2, content1) {
		// If one is 80%+ of the other, consider similar
		len1, len2 := len(content1), len(content2)
		ratio := float64(min(len1, len2)) / float64(max(len1, len2))
		return ratio >= 0.8
	}

	// Check for word overlap (if 70%+ words match, consider similar)
	words1 := strings.Fields(content1)
	words2 := strings.Fields(content2)
	if len(words1) == 0 || len(words2) == 0 {
		return false
	}

	// Count matching words
	matches := 0
	wordSet := make(map[string]bool)
	for _, word := range words1 {
		if len(word) > 3 { // Only consider words longer than 3 chars
			wordSet[word] = true
		}
	}
	for _, word := range words2 {
		if len(word) > 3 && wordSet[word] {
			matches++
		}
	}

	// If 70%+ of words match, consider similar
	avgWords := (len(words1) + len(words2)) / 2
	if avgWords > 0 {
		similarity := float64(matches) / float64(avgWords)
		return similarity >= 0.7
	}

	return false
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// max returns the maximum of two integers
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

