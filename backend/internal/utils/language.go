package utils

import (
	"strings"

	"ezra-clone/backend/internal/graph"
)

// LanguagePatterns maps language codes to detection patterns
var LanguagePatterns = map[string][]string{
	"fr":        {"france", "speak french", "respond in french", "lang=fr", "language=french", "only speaks french", "only speak french"},
	"en":        {"english", "speak english", "respond in english", "lang=en", "language=english", "preferred language is english", "prefers to speak in english"},
	"pig_latin": {"pig latin", "speaks pig latin", "only speaks pig latin", "only speak pig latin"},
	"es":        {"spanish", "speaks spanish", "only speaks spanish", "only speak spanish", "lang=es"},
	"de":        {"german", "speaks german", "only speaks german", "only speak german", "lang=de"},
	"it":        {"italian", "speaks italian", "only speaks italian", "only speak italian", "lang=it"},
	"pt":        {"portuguese", "speaks portuguese", "only speaks portuguese", "only speak portuguese", "lang=pt"},
	"ja":        {"japanese", "speaks japanese", "only speaks japanese", "only speak japanese", "lang=ja"},
	"zh":        {"chinese", "speaks chinese", "only speaks chinese", "only speak chinese", "lang=zh"},
	"ko":        {"korean", "speaks korean", "only speaks korean", "only speak korean", "lang=ko"},
	"ru":        {"russian", "speaks russian", "only speaks russian", "only speak russian", "lang=ru"},
}

// LanguageNames maps language codes to display names
var LanguageNames = map[string]string{
	"fr":        "French",
	"en":        "English",
	"es":        "Spanish",
	"de":        "German",
	"it":        "Italian",
	"pt":        "Portuguese",
	"ja":        "Japanese",
	"zh":        "Chinese",
	"ko":        "Korean",
	"ru":        "Russian",
	"pig_latin": "Pig Latin",
}

// ExtractLanguageFromMessage extracts the language code from a message
func ExtractLanguageFromMessage(content string) string {
	lowerContent := strings.ToLower(content)
	
	for langCode, patterns := range LanguagePatterns {
		for _, pattern := range patterns {
			if strings.Contains(lowerContent, pattern) {
				return langCode
			}
		}
	}
	return ""
}

// GetLanguageName returns the display name for a language code
func GetLanguageName(langCode string) string {
	if name, ok := LanguageNames[langCode]; ok {
		return name
	}
	return langCode // Return code if name not found
}

// ExtractLanguageFromFacts searches user facts for language preferences
// Only considers facts that are ABOUT the current user, not facts mentioning other users
func ExtractLanguageFromFacts(facts []graph.Fact) (string, string) {
	// First, look for explicit language preference facts (highest priority)
	for _, fact := range facts {
		lowerFact := strings.ToLower(fact.Content)
		
		// Check for explicit preference statements
		if strings.Contains(lowerFact, "prefers to communicate in") || 
		   strings.Contains(lowerFact, "preferred language") ||
		   strings.Contains(lowerFact, "prefers to speak in") {
			// This is an explicit language preference fact
			for langCode, patterns := range LanguagePatterns {
				for _, pattern := range patterns {
					if strings.Contains(lowerFact, pattern) {
						return langCode, GetLanguageName(langCode)
					}
				}
			}
		}
	}
	
	// Second, look for facts about the user speaking a language
	// But exclude facts that mention other users (like "@alexei only speaks...")
	for _, fact := range facts {
		lowerFact := strings.ToLower(fact.Content)
		
		// Skip facts that mention other users (they start with @username or contain mentions)
		// Facts about the current user typically start with "User" or "The user" or don't have @mentions
		if strings.HasPrefix(lowerFact, "@") {
			// This fact mentions another user, skip it
			continue
		}
		
		// Check if this fact is about the user speaking a language
		// Look for patterns like "user only speaks X" or "only speaks X" (without @mention)
		hasUserReference := strings.HasPrefix(lowerFact, "user") || 
		                    strings.HasPrefix(lowerFact, "the user") ||
		                    strings.Contains(lowerFact, " only speaks") ||
		                    strings.Contains(lowerFact, " only speak")
		
		if hasUserReference {
			for langCode, patterns := range LanguagePatterns {
				for _, pattern := range patterns {
					if strings.Contains(lowerFact, pattern) {
						return langCode, GetLanguageName(langCode)
					}
				}
			}
		}
	}
	
	return "", ""
}

