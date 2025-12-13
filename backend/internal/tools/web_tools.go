package tools

import (
	"ezra-clone/backend/internal/adapter"
)

// GetWebTools returns web browsing/search tools
func GetWebTools() []adapter.Tool {
	return []adapter.Tool{
		{
			Type: "function",
			Function: adapter.FunctionDefinition{
				Name:        ToolWebSearch,
				Description: "Search the web for current information. IMPORTANT: Rewrite the user's question into an optimized search query with relevant keywords. Include the current month/year for time-sensitive queries. Example: 'what's happening with AI?' becomes 'artificial intelligence news [current month] [current year]'",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"query": map[string]interface{}{
							"type":        "string",
							"description": "An optimized search query with keywords (NOT the user's exact question). Use specific terms, add year if relevant, remove filler words.",
						},
						"original_question": map[string]interface{}{
							"type":        "string",
							"description": "The user's original question (for context in the response)",
						},
					},
					"required": []string{"query"},
				},
			},
		},
		{
			Type: "function",
			Function: adapter.FunctionDefinition{
				Name:        ToolFetchWebpage,
				Description: "Fetch and intelligently extract structured content from a webpage. This tool parses article content with headings, sections, and metadata (title, author, date). USE THIS when a user asks 'what's on this page?', 'tell me about this URL', 'read this page', or provides any URL. IMPORTANT: If the user asks to 'summarize' or wants a 'summary', use summarize_website tool instead - it provides AI-powered summaries. CRITICAL: When summarizing articles from search results, fetch the ACTUAL INDIVIDUAL ARTICLE URLs from the search results (the URLs listed under 'ARTICLE 1', 'ARTICLE 2', etc.), NOT article list pages, digest pages, or search results pages. DO NOT fetch URLs ending in patterns like '/ai-news-december-2025' or '/monthly-digest' as these are usually article list pages, not individual articles. The tool returns structured content with sections, headings, and metadata that can be used for detailed analysis and question answering.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"url": map[string]interface{}{
							"type":        "string",
							"description": "The URL to fetch (can be http:// or https://). IMPORTANT: When summarizing articles from search results, use the article URLs from the search results, not the search results page URL.",
						},
						"extract_text": map[string]interface{}{
							"type":        "boolean",
							"description": "Whether to extract structured text content (default: true). The tool automatically extracts structured content with headings, sections, and metadata.",
						},
					},
					"required": []string{"url"},
				},
			},
		},
		{
			Type: "function",
			Function: adapter.FunctionDefinition{
				Name:        ToolSummarizeWebsite,
				Description: "Summarize a website by fetching its content and generating a concise AI-powered summary using OpenRouter. This tool uses smart multi-stage summarization: it chunks long articles (>8000 chars), extracts important information from each chunk, and synthesizes a comprehensive final summary. MANDATORY: USE THIS tool whenever the user asks to 'summarize', 'give me a summary', 'summarize the articles', 'what's this about', or wants a quick overview. DO NOT use fetch_webpage for summarization tasks - this tool handles both fetching AND summarization. For long articles, it automatically chunks the content and extracts vital information from each section before creating the final summary. This is the ONLY tool that provides AI-generated summaries.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"url": map[string]interface{}{
							"type":        "string",
							"description": "The URL to summarize (can be http:// or https://).",
						},
					},
					"required": []string{"url"},
				},
			},
		},
	}
}

