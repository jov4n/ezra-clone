package discord

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	// Regex patterns compiled once at startup
	codeBlockPattern            = regexp.MustCompile("(?s)```.*?```")
	inlineCodePattern           = regexp.MustCompile("`([^`\n]+)`")
	h1Pattern                   = regexp.MustCompile(`(?m)^#\s+(.+)$`)
	h2Pattern                   = regexp.MustCompile(`(?m)^##\s+(.+)$`)
	h3Pattern                   = regexp.MustCompile(`(?m)^###\s+(.+)$`)
	h4Pattern                   = regexp.MustCompile(`(?m)^####\s+(.+)$`)
	h5Pattern                   = regexp.MustCompile(`(?m)^#####\s+(.+)$`)
	h6Pattern                   = regexp.MustCompile(`(?m)^######\s+(.+)$`)
	linkPattern                 = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	orderedListPattern          = regexp.MustCompile(`(?m)^(\d+)\.\s+(.+)$`)
	unorderedListPattern        = regexp.MustCompile(`(?m)^[-*]\s+(.+)$`)
	multipleNewlinesPattern     = regexp.MustCompile(`\n{3,}`)
	orderedListItemStartPattern = regexp.MustCompile(`^\d+\.`)
)

// FormatMarkdown converts standard markdown to Discord markdown format
//
// Conversions performed:
//   - Headers (# Header) → Bold (**Header**)
//   - Lists (- item) → Discord list format (• item)
//   - Links [text](url) → Preserved (Discord supports this)
//   - Code blocks ```code``` → Preserved exactly
//   - Inline code `code` → Preserved exactly
//   - Bold/italic/strikethrough → Preserved (already Discord-compatible)
//
// Discord supports: **bold**, *italic*, __underline__, ~~strikethrough~~, `code`, ```code blocks```
//
// Example:
//
//	Input:  "# Example\n\nHere's some `code` and a [link](https://example.com)"
//	Output: "**Example**\n\nHere's some `code` and a [link](https://example.com)"
func FormatMarkdown(content string) string {
	// First, protect code blocks from being modified
	content = protectCodeBlocks(content, func(protected string) string {
		// Process the non-code-block content
		formatted := protected

		// Convert markdown headers to bold (Discord doesn't support headers natively)
		formatted = formatHeaders(formatted)

		// Convert markdown links to Discord format [text](url) -> [text](url) (Discord supports this)
		formatted = formatLinks(formatted)

		// Convert markdown lists (already handled by Discord, but ensure proper formatting)
		formatted = formatLists(formatted)

		// Convert markdown emphasis (ensure proper Discord formatting)
		formatted = formatEmphasis(formatted)

		// Clean up extra whitespace while preserving intentional spacing
		formatted = cleanWhitespace(formatted)

		return formatted
	})

	return content
}

// protectCodeBlocks protects code blocks and inline code from modification
func protectCodeBlocks(content string, processor func(string) string) string {
	// Pattern to match code blocks: ```language\ncontent\n``` or ```content```
	// We use a general pattern to match anything between triple backticks to protect it
	// (?s) makes . match newlines, .*? is non-greedy
	// codeBlockPattern is now a package-level variable

	// Pattern to match inline code: `code`
	// Since we already protected triple code blocks, we simply match text surrounded by single backticks
	// Note: Go regex does not support lookarounds, so we rely on codeBlockPattern running first
	// inlineCodePattern is now a package-level variable

	// Store protected content
	type protectedItem struct {
		placeholder string
		content     string
	}
	var protected []protectedItem
	placeholderCounter := 0

	// Protect code blocks first (they can contain inline code)
	content = codeBlockPattern.ReplaceAllStringFunc(content, func(match string) string {
		placeholder := fmt.Sprintf("___CODEBLOCK_PLACEHOLDER_%d___", placeholderCounter)
		placeholderCounter++
		protected = append(protected, protectedItem{
			placeholder: placeholder,
			content:     match,
		})
		return placeholder
	})

	// Protect inline code (only if not already in a code block)
	content = inlineCodePattern.ReplaceAllStringFunc(content, func(match string) string {
		// Skip if this is part of a code block placeholder
		if strings.Contains(match, "___CODEBLOCK_PLACEHOLDER_") {
			return match
		}
		placeholder := fmt.Sprintf("___INLINECODE_PLACEHOLDER_%d___", placeholderCounter)
		placeholderCounter++
		protected = append(protected, protectedItem{
			placeholder: placeholder,
			content:     match,
		})
		return placeholder
	})

	// Process the content
	content = processor(content)

	// Restore protected content in reverse order (to avoid replacing placeholders)
	for i := len(protected) - 1; i >= 0; i-- {
		content = strings.Replace(content, protected[i].placeholder, protected[i].content, 1)
	}

	return content
}

// formatHeaders converts markdown headers to Discord bold
func formatHeaders(content string) string {
	// H1: # Header -> **Header**
	content = h1Pattern.ReplaceAllString(content, "**$1**")

	// H2: ## Header -> **Header**
	content = h2Pattern.ReplaceAllString(content, "**$1**")

	// H3: ### Header -> **Header**
	content = h3Pattern.ReplaceAllString(content, "**$1**")

	// H4-H6: Same treatment
	content = h4Pattern.ReplaceAllString(content, "**$1**")
	content = h5Pattern.ReplaceAllString(content, "**$1**")
	content = h6Pattern.ReplaceAllString(content, "**$1**")

	return content
}

// formatLinks ensures Discord-compatible link formatting
func formatLinks(content string) string {
	// Discord supports [text](url) format, so we just ensure it's properly formatted
	// Convert markdown links that might be missing brackets
	// This already matches Discord format, so we just validate it
	content = linkPattern.ReplaceAllStringFunc(content, func(match string) string {
		// Ensure URL is valid
		return match
	})

	return content
}

// formatLists ensures proper list formatting for Discord
func formatLists(content string) string {
	// Discord supports both - and * for unordered lists, and numbers for ordered
	// Ensure consistent formatting

	// Convert markdown ordered lists (1. item) to Discord format
	content = orderedListPattern.ReplaceAllString(content, "$1. $2")

	// Ensure unordered lists use consistent markers
	content = unorderedListPattern.ReplaceAllString(content, "• $1")

	return content
}

// formatEmphasis ensures proper Discord emphasis formatting
func formatEmphasis(content string) string {
	// Discord uses:
	// **bold** or __bold__
	// *italic* or _italic_
	// ***bold italic*** or ___bold italic___
	// ~~strikethrough~~

	// Convert markdown bold **text** (already Discord compatible)
	// Convert markdown italic *text* (already Discord compatible)
	// But ensure we don't double-format

	// Note: Markdown emphasis is already Discord-compatible
	// **text** for bold, *text* for italic
	// We're conservative here to avoid breaking existing formatting

	return content
}

// cleanWhitespace removes excessive whitespace while preserving intentional spacing
func cleanWhitespace(content string) string {
	// Remove trailing whitespace from lines (but preserve intentional spacing)
	lines := strings.Split(content, "\n")
	var cleaned []string
	for _, line := range lines {
		// Preserve empty lines (intentional spacing)
		if strings.TrimSpace(line) == "" {
			cleaned = append(cleaned, "")
			continue
		}
		// Remove trailing spaces but preserve leading spaces (for indentation)
		cleaned = append(cleaned, strings.TrimRight(line, " \t"))
	}
	return strings.Join(cleaned, "\n")
}

// FormatCodeBlock formats code for Discord code blocks with optional language
func FormatCodeBlock(code string, language string) string {
	if language != "" {
		return "```" + language + "\n" + code + "\n```"
	}
	return "```\n" + code + "\n```"
}

// FormatInlineCode formats inline code for Discord
func FormatInlineCode(code string) string {
	return "`" + code + "`"
}

// FormatBold formats text as bold in Discord
func FormatBold(text string) string {
	return "**" + text + "**"
}

// FormatItalic formats text as italic in Discord
func FormatItalic(text string) string {
	return "*" + text + "*"
}

// FormatUnderline formats text as underlined in Discord
func FormatUnderline(text string) string {
	return "__" + text + "__"
}

// FormatStrikethrough formats text as strikethrough in Discord
func FormatStrikethrough(text string) string {
	return "~~" + text + "~~"
}

// FormatSpoiler formats text as spoiler in Discord
func FormatSpoiler(text string) string {
	return "||" + text + "||"
}

// FormatQuote formats text as a quote block in Discord
func FormatQuote(text string) string {
	lines := strings.Split(text, "\n")
	var quoted []string
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			quoted = append(quoted, "> "+line)
		} else {
			quoted = append(quoted, "")
		}
	}
	return strings.Join(quoted, "\n")
}

// FormatList formats items as a Discord list
func FormatList(items []string, ordered bool) string {
	var list []string
	for i, item := range items {
		if ordered {
			list = append(list, fmt.Sprintf("%d. %s", i+1, item))
		} else {
			list = append(list, "• "+item)
		}
	}
	return strings.Join(list, "\n")
}

// SmartFormat intelligently formats content for Discord, detecting code blocks and preserving them
func SmartFormat(content string) string {
	// Always apply FormatMarkdown - it will protect code blocks internally
	// This ensures code blocks are preserved while formatting the rest of the content
	formatted := FormatMarkdown(content)

	// Ensure proper line breaks for readability
	formatted = ensureProperLineBreaks(formatted)

	return formatted
}

// ensureProperLineBreaks ensures proper line breaks for Discord readability
func ensureProperLineBreaks(content string) string {
	// Ensure double line breaks between paragraphs (but not more than 2)
	content = multipleNewlinesPattern.ReplaceAllString(content, "\n\n")

	// Ensure proper spacing after list items
	// Go regex doesn't support lookahead, so we'll process line by line
	lines := strings.Split(content, "\n")
	var result []string

	for i, line := range lines {
		result = append(result, line)

		// Check if this is a list item
		trimmed := strings.TrimSpace(line)
		isListItem := strings.HasPrefix(trimmed, "•") || orderedListItemStartPattern.MatchString(trimmed)

		if isListItem && i < len(lines)-1 {
			// Check if next line is not empty and not a list item
			nextLine := ""
			if i+1 < len(lines) {
				nextLine = strings.TrimSpace(lines[i+1])
			}

			nextIsListItem := nextLine != "" && (strings.HasPrefix(nextLine, "•") || orderedListItemStartPattern.MatchString(nextLine))

			// If next line is not a list item and not empty, add a blank line for spacing
			if nextLine != "" && !nextIsListItem && i+1 < len(lines) {
				// Check if there's already a blank line
				if i+1 < len(lines) && strings.TrimSpace(lines[i+1]) != "" {
					// No blank line, but we'll let it be - Discord handles list spacing fine
				}
			}
		}
	}

	return strings.Join(result, "\n")
}
