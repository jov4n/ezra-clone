package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"ezra-clone/backend/internal/adapter"
	"ezra-clone/backend/internal/graph"
	"ezra-clone/backend/pkg/logger"
	"go.uber.org/zap"
)

// ExecutionContext holds context for tool execution
type ExecutionContext struct {
	AgentID   string
	UserID    string
	ChannelID string
	Platform  string // "discord", "web"
}

// ToolResult represents the result of a tool execution
type ToolResult struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
	Message string      `json:"message,omitempty"`
}

// MimicState holds the current personality mimic state
type MimicState struct {
	Active           bool                `json:"active"`
	OriginalPersonality string           `json:"original_personality"`
	MimicProfile     *PersonalityProfile `json:"mimic_profile,omitempty"`
}

// Executor handles tool execution
type Executor struct {
	repo            *graph.Repository
	httpClient      *http.Client
	logger          *zap.Logger
	discordExecutor *DiscordExecutor
	mimicStates     map[string]*MimicState // key: agentID
}

// NewExecutor creates a new tool executor
func NewExecutor(repo *graph.Repository) *Executor {
	return &Executor{
		repo: repo,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger:      logger.Get(),
		mimicStates: make(map[string]*MimicState),
	}
}

// SetDiscordExecutor sets the Discord executor for Discord-specific tools
func (e *Executor) SetDiscordExecutor(de *DiscordExecutor) {
	e.discordExecutor = de
}

// GetMimicState returns the current mimic state for an agent
func (e *Executor) GetMimicState(agentID string) *MimicState {
	return e.mimicStates[agentID]
}

// IsMimicking returns true if the agent is currently mimicking someone
func (e *Executor) IsMimicking(agentID string) bool {
	state := e.mimicStates[agentID]
	return state != nil && state.Active
}

// GetMimicPrompt returns the style prompt if mimicking, empty string otherwise
func (e *Executor) GetMimicPrompt(agentID string) string {
	state := e.mimicStates[agentID]
	if state != nil && state.Active && state.MimicProfile != nil {
		return state.MimicProfile.StylePrompt
	}
	return ""
}

// Execute runs a tool call and returns the result
func (e *Executor) Execute(ctx context.Context, execCtx *ExecutionContext, toolCall adapter.ToolCall) *ToolResult {
	e.logger.Debug("Executing tool",
		zap.String("tool", toolCall.Name),
		zap.String("agent_id", execCtx.AgentID),
		zap.String("user_id", execCtx.UserID),
	)

	switch toolCall.Name {
	// Memory Tools
	case ToolCoreMemoryInsert, ToolCoreMemoryReplace:
		return e.executeMemoryUpdate(ctx, execCtx, toolCall.Arguments)
	case ToolArchivalInsert:
		return e.executeArchivalInsert(ctx, execCtx, toolCall.Arguments)
	case ToolArchivalSearch, ToolMemorySearch:
		return e.executeMemorySearch(ctx, execCtx, toolCall.Arguments)

	// Knowledge Tools
	case ToolCreateFact:
		return e.executeCreateFact(ctx, execCtx, toolCall.Arguments)
	case ToolSearchFacts:
		return e.executeSearchFacts(ctx, execCtx, toolCall.Arguments)
	case ToolGetUserContext:
		return e.executeGetUserContext(ctx, execCtx, toolCall.Arguments)

	// Topic Tools
	case ToolCreateTopic:
		return e.executeCreateTopic(ctx, execCtx, toolCall.Arguments)
	case ToolLinkTopics:
		return e.executeLinkTopics(ctx, execCtx, toolCall.Arguments)
	case ToolFindRelated:
		return e.executeFindRelated(ctx, execCtx, toolCall.Arguments)
	case ToolLinkUserTopic:
		return e.executeLinkUserTopic(ctx, execCtx, toolCall.Arguments)

	// Conversation Tools
	case ToolGetHistory:
		return e.executeGetHistory(ctx, execCtx, toolCall.Arguments)
	case ToolSendMessage:
		return e.executeSendMessage(ctx, execCtx, toolCall.Arguments)

	// Web Tools
	case ToolWebSearch:
		return e.executeWebSearch(ctx, toolCall.Arguments)
	case ToolFetchWebpage:
		return e.executeFetchWebpage(ctx, toolCall.Arguments)

	// GitHub Tools
	case ToolGitHubRepoInfo:
		return e.executeGitHubRepoInfo(ctx, toolCall.Arguments)
	case ToolGitHubSearch:
		return e.executeGitHubSearch(ctx, toolCall.Arguments)
	case ToolGitHubReadFile:
		return e.executeGitHubReadFile(ctx, toolCall.Arguments)
	case ToolGitHubListOrgRepos:
		return e.executeGitHubListOrgRepos(ctx, toolCall.Arguments)

	// Discord Tools
	case ToolDiscordReadHistory:
		return e.executeDiscordReadHistory(ctx, execCtx, toolCall.Arguments)
	case ToolDiscordGetUserInfo:
		return e.executeDiscordGetUserInfo(ctx, toolCall.Arguments)
	case ToolDiscordGetChannelInfo:
		return e.executeDiscordGetChannelInfo(ctx, execCtx, toolCall.Arguments)

	// Personality/Mimic Tools
	case ToolMimicPersonality:
		return e.executeMimicPersonality(ctx, execCtx, toolCall.Arguments)
	case ToolRevertPersonality:
		return e.executeRevertPersonality(ctx, execCtx)
	case ToolAnalyzeUserStyle:
		return e.executeAnalyzeUserStyle(ctx, execCtx, toolCall.Arguments)

	default:
		e.logger.Warn("Unknown tool", zap.String("tool", toolCall.Name))
		return &ToolResult{
			Success: false,
			Error:   fmt.Sprintf("Unknown tool: %s", toolCall.Name),
		}
	}
}

// ============================================================================
// Memory Tool Implementations
// ============================================================================

func (e *Executor) executeMemoryUpdate(ctx context.Context, execCtx *ExecutionContext, args map[string]interface{}) *ToolResult {
	name, _ := args["name"].(string)
	content, _ := args["content"].(string)

	if name == "" || content == "" {
		return &ToolResult{Success: false, Error: "name and content are required"}
	}

	err := e.repo.UpdateMemory(ctx, execCtx.AgentID, name, content)
	if err != nil {
		return &ToolResult{Success: false, Error: err.Error()}
	}

	return &ToolResult{
		Success: true,
		Message: fmt.Sprintf("Memory '%s' has been saved.", name),
	}
}

func (e *Executor) executeArchivalInsert(ctx context.Context, execCtx *ExecutionContext, args map[string]interface{}) *ToolResult {
	content, _ := args["content"].(string)
	if content == "" {
		return &ToolResult{Success: false, Error: "content is required"}
	}

	// For now, archival insert uses the same mechanism as memory
	// In a full implementation, this would go to a separate archival storage
	err := e.repo.UpdateMemory(ctx, execCtx.AgentID, fmt.Sprintf("archival_%d", time.Now().Unix()), content)
	if err != nil {
		return &ToolResult{Success: false, Error: err.Error()}
	}

	return &ToolResult{
		Success: true,
		Message: "Information archived successfully.",
	}
}

func (e *Executor) executeMemorySearch(ctx context.Context, execCtx *ExecutionContext, args map[string]interface{}) *ToolResult {
	query, _ := args["query"].(string)
	if query == "" {
		return &ToolResult{Success: false, Error: "query is required"}
	}

	limit := 10
	if l, ok := args["limit"].(float64); ok {
		limit = int(l)
	}

	results, err := e.repo.SearchMemory(ctx, execCtx.AgentID, query, limit)
	if err != nil {
		return &ToolResult{Success: false, Error: err.Error()}
	}

	return &ToolResult{
		Success: true,
		Data:    results,
		Message: fmt.Sprintf("Found %d results for '%s'", len(results), query),
	}
}

// ============================================================================
// Knowledge Tool Implementations
// ============================================================================

func (e *Executor) executeCreateFact(ctx context.Context, execCtx *ExecutionContext, args map[string]interface{}) *ToolResult {
	content, _ := args["content"].(string)
	if content == "" {
		return &ToolResult{Success: false, Error: "content is required"}
	}

	source, _ := args["source"].(string)
	
	var topics []string
	if topicsArg, ok := args["topics"].([]interface{}); ok {
		for _, t := range topicsArg {
			if ts, ok := t.(string); ok {
				topics = append(topics, ts)
			}
		}
	}

	fact, err := e.repo.CreateFact(ctx, execCtx.AgentID, content, source, execCtx.UserID, topics)
	if err != nil {
		return &ToolResult{Success: false, Error: err.Error()}
	}

	return &ToolResult{
		Success: true,
		Data:    fact,
		Message: fmt.Sprintf("Fact stored: %s", content),
	}
}

func (e *Executor) executeSearchFacts(ctx context.Context, execCtx *ExecutionContext, args map[string]interface{}) *ToolResult {
	topic, _ := args["topic"].(string)
	if topic == "" {
		return &ToolResult{Success: false, Error: "topic is required"}
	}

	facts, err := e.repo.GetFactsAboutTopic(ctx, topic)
	if err != nil {
		return &ToolResult{Success: false, Error: err.Error()}
	}

	return &ToolResult{
		Success: true,
		Data:    facts,
		Message: fmt.Sprintf("Found %d facts about '%s'", len(facts), topic),
	}
}

func (e *Executor) executeGetUserContext(ctx context.Context, execCtx *ExecutionContext, args map[string]interface{}) *ToolResult {
	userID, _ := args["user_id"].(string)
	if userID == "" {
		userID = execCtx.UserID
	}

	userCtx, err := e.repo.GetUserContext(ctx, userID)
	if err != nil {
		return &ToolResult{Success: false, Error: err.Error()}
	}

	// Build a helpful message summarizing what was found
	message := fmt.Sprintf("Retrieved user context for user %s", userID)
	if userCtx != nil {
		if len(userCtx.Facts) > 0 {
			message += fmt.Sprintf(" - Found %d fact(s)", len(userCtx.Facts))
		}
		if len(userCtx.Topics) > 0 {
			message += fmt.Sprintf(" - %d interest(s)", len(userCtx.Topics))
		}
		if len(userCtx.Facts) == 0 && len(userCtx.Topics) == 0 {
			message += " - No information found yet"
		}
	}

	return &ToolResult{
		Success: true,
		Data:    userCtx,
		Message: message,
	}
}

// ============================================================================
// Topic Tool Implementations
// ============================================================================

func (e *Executor) executeCreateTopic(ctx context.Context, execCtx *ExecutionContext, args map[string]interface{}) *ToolResult {
	name, _ := args["name"].(string)
	if name == "" {
		return &ToolResult{Success: false, Error: "name is required"}
	}

	description, _ := args["description"].(string)

	topic, err := e.repo.CreateTopic(ctx, name, description)
	if err != nil {
		return &ToolResult{Success: false, Error: err.Error()}
	}

	return &ToolResult{
		Success: true,
		Data:    topic,
		Message: fmt.Sprintf("Topic '%s' created.", name),
	}
}

func (e *Executor) executeLinkTopics(ctx context.Context, execCtx *ExecutionContext, args map[string]interface{}) *ToolResult {
	topic1, _ := args["topic1"].(string)
	topic2, _ := args["topic2"].(string)
	relationship, _ := args["relationship"].(string)

	if topic1 == "" || topic2 == "" {
		return &ToolResult{Success: false, Error: "topic1 and topic2 are required"}
	}

	if relationship == "" {
		relationship = "RELATED_TO"
	}

	err := e.repo.LinkTopics(ctx, topic1, topic2, relationship)
	if err != nil {
		return &ToolResult{Success: false, Error: err.Error()}
	}

	return &ToolResult{
		Success: true,
		Message: fmt.Sprintf("Linked '%s' to '%s' with relationship '%s'", topic1, topic2, relationship),
	}
}

func (e *Executor) executeFindRelated(ctx context.Context, execCtx *ExecutionContext, args map[string]interface{}) *ToolResult {
	topic, _ := args["topic"].(string)
	if topic == "" {
		return &ToolResult{Success: false, Error: "topic is required"}
	}

	depth := 2
	if d, ok := args["depth"].(float64); ok {
		depth = int(d)
	}

	topics, err := e.repo.GetRelatedTopics(ctx, topic, depth)
	if err != nil {
		return &ToolResult{Success: false, Error: err.Error()}
	}

	return &ToolResult{
		Success: true,
		Data:    topics,
		Message: fmt.Sprintf("Found %d related topics", len(topics)),
	}
}

func (e *Executor) executeLinkUserTopic(ctx context.Context, execCtx *ExecutionContext, args map[string]interface{}) *ToolResult {
	topic, _ := args["topic"].(string)
	if topic == "" {
		return &ToolResult{Success: false, Error: "topic is required"}
	}

	userID, _ := args["user_id"].(string)
	if userID == "" {
		userID = execCtx.UserID
	}

	err := e.repo.LinkUserToTopic(ctx, userID, topic)
	if err != nil {
		return &ToolResult{Success: false, Error: err.Error()}
	}

	return &ToolResult{
		Success: true,
		Message: fmt.Sprintf("Linked user to topic '%s'", topic),
	}
}

// ============================================================================
// Conversation Tool Implementations
// ============================================================================

func (e *Executor) executeGetHistory(ctx context.Context, execCtx *ExecutionContext, args map[string]interface{}) *ToolResult {
	channelID, _ := args["channel_id"].(string)
	if channelID == "" {
		channelID = execCtx.ChannelID
	}

	limit := 20
	if l, ok := args["limit"].(float64); ok {
		limit = int(l)
	}

	messages, err := e.repo.GetConversationHistory(ctx, channelID, limit)
	if err != nil {
		return &ToolResult{Success: false, Error: err.Error()}
	}

	return &ToolResult{
		Success: true,
		Data:    messages,
	}
}

func (e *Executor) executeSendMessage(ctx context.Context, execCtx *ExecutionContext, args map[string]interface{}) *ToolResult {
	message, _ := args["message"].(string)
	
	// This is handled specially - the message becomes the response content
	return &ToolResult{
		Success: true,
		Message: message,
	}
}

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

// ============================================================================
// GitHub Tool Implementations
// ============================================================================

func (e *Executor) executeGitHubRepoInfo(ctx context.Context, args map[string]interface{}) *ToolResult {
	owner, _ := args["owner"].(string)
	repo, _ := args["repo"].(string)

	if owner == "" || repo == "" {
		return &ToolResult{Success: false, Error: "owner and repo are required"}
	}

	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s", owner, repo)
	
	req, _ := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "EzraBot/1.0")

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("GitHub API error: %v", err)}
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return &ToolResult{Success: false, Error: "Repository not found"}
	}

	body, _ := io.ReadAll(resp.Body)
	var repoInfo map[string]interface{}
	if err := json.Unmarshal(body, &repoInfo); err != nil {
		return &ToolResult{Success: false, Error: "Failed to parse response"}
	}

	// Extract relevant info
	result := map[string]interface{}{
		"name":          repoInfo["name"],
		"full_name":     repoInfo["full_name"],
		"description":   repoInfo["description"],
		"stars":         repoInfo["stargazers_count"],
		"forks":         repoInfo["forks_count"],
		"language":      repoInfo["language"],
		"open_issues":   repoInfo["open_issues_count"],
		"url":           repoInfo["html_url"],
		"default_branch": repoInfo["default_branch"],
		"created_at":    repoInfo["created_at"],
		"updated_at":    repoInfo["updated_at"],
		"topics":        repoInfo["topics"],
	}

	return &ToolResult{
		Success: true,
		Data:    result,
	}
}

func (e *Executor) executeGitHubSearch(ctx context.Context, args map[string]interface{}) *ToolResult {
	query, _ := args["query"].(string)
	if query == "" {
		return &ToolResult{Success: false, Error: "query is required"}
	}

	searchType, _ := args["type"].(string)
	if searchType == "" {
		searchType = "repositories"
	}

	limit := 5
	if l, ok := args["limit"].(float64); ok {
		limit = int(l)
	}

	apiURL := fmt.Sprintf("https://api.github.com/search/%s?q=%s&per_page=%d",
		searchType, url.QueryEscape(query), limit)

	req, _ := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "EzraBot/1.0")

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("GitHub API error: %v", err)}
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var searchResult map[string]interface{}
	if err := json.Unmarshal(body, &searchResult); err != nil {
		return &ToolResult{Success: false, Error: "Failed to parse response"}
	}

	return &ToolResult{
		Success: true,
		Data:    searchResult,
		Message: fmt.Sprintf("Found %v results", searchResult["total_count"]),
	}
}

func (e *Executor) executeGitHubListOrgRepos(ctx context.Context, args map[string]interface{}) *ToolResult {
	org, _ := args["org"].(string)
	if org == "" {
		return &ToolResult{Success: false, Error: "org is required"}
	}

	limit := 5
	if l, ok := args["limit"].(float64); ok {
		limit = int(l)
	}

	// GitHub API: list org repos sorted by most recently updated
	apiURL := fmt.Sprintf("https://api.github.com/orgs/%s/repos?sort=updated&direction=desc&per_page=%d",
		url.QueryEscape(org), limit)

	req, _ := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "EzraBot/1.0")

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("GitHub API error: %v", err)}
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return &ToolResult{Success: false, Error: fmt.Sprintf("Organization '%s' not found", org)}
	}

	body, _ := io.ReadAll(resp.Body)
	var repos []map[string]interface{}
	if err := json.Unmarshal(body, &repos); err != nil {
		return &ToolResult{Success: false, Error: "Failed to parse response"}
	}

	if len(repos) == 0 {
		return &ToolResult{
			Success: true,
			Message: fmt.Sprintf("No public repositories found for organization '%s'", org),
		}
	}

	// Format the results nicely
	var results []map[string]interface{}
	for _, repo := range repos {
		results = append(results, map[string]interface{}{
			"name":        repo["name"],
			"full_name":   repo["full_name"],
			"description": repo["description"],
			"language":    repo["language"],
			"updated_at":  repo["updated_at"],
			"pushed_at":   repo["pushed_at"],
			"url":         repo["html_url"],
			"stars":       repo["stargazers_count"],
		})
	}

	// Get the most recently updated repo for a nice summary
	mostRecent := results[0]
	
	return &ToolResult{
		Success: true,
		Data:    results,
		Message: fmt.Sprintf("Found %d repos. Most recently updated: %s (updated: %v)", 
			len(results), mostRecent["name"], mostRecent["updated_at"]),
	}
}

func (e *Executor) executeGitHubReadFile(ctx context.Context, args map[string]interface{}) *ToolResult {
	owner, _ := args["owner"].(string)
	repo, _ := args["repo"].(string)
	path, _ := args["path"].(string)
	branch, _ := args["branch"].(string)

	if owner == "" || repo == "" || path == "" {
		return &ToolResult{Success: false, Error: "owner, repo, and path are required"}
	}

	if branch == "" {
		branch = "main"
	}

	// Use raw content URL
	rawURL := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s/%s",
		owner, repo, branch, path)

	req, _ := http.NewRequestWithContext(ctx, "GET", rawURL, nil)
	req.Header.Set("User-Agent", "EzraBot/1.0")

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("Failed to fetch file: %v", err)}
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		// Try with 'master' branch
		if branch == "main" {
			args["branch"] = "master"
			return e.executeGitHubReadFile(ctx, args)
		}
		return &ToolResult{Success: false, Error: "File not found"}
	}

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 100000)) // 100KB limit
	content := string(body)

	// Truncate if too long
	if len(content) > 10000 {
		content = content[:10000] + "\n... (truncated)"
	}

	return &ToolResult{
		Success: true,
		Data: map[string]interface{}{
			"path":    path,
			"content": content,
		},
	}
}

// ============================================================================
// Discord Tool Implementations
// ============================================================================

func (e *Executor) executeDiscordReadHistory(ctx context.Context, execCtx *ExecutionContext, args map[string]interface{}) *ToolResult {
	if e.discordExecutor == nil {
		return &ToolResult{Success: false, Error: "Discord not available (only works in Discord bot context)"}
	}

	channelID, _ := args["channel_id"].(string)
	if channelID == "" {
		channelID = execCtx.ChannelID
	}
	if channelID == "" {
		return &ToolResult{Success: false, Error: "channel_id is required"}
	}

	limit := 50
	if l, ok := args["limit"].(float64); ok {
		limit = int(l)
	}

	fromUserID, _ := args["from_user_id"].(string)

	messages, err := e.discordExecutor.ReadChannelHistory(ctx, channelID, limit, fromUserID)
	if err != nil {
		return &ToolResult{Success: false, Error: err.Error()}
	}

	return &ToolResult{
		Success: true,
		Data:    messages,
		Message: fmt.Sprintf("Retrieved %d messages from channel", len(messages)),
	}
}

func (e *Executor) executeDiscordGetUserInfo(ctx context.Context, args map[string]interface{}) *ToolResult {
	if e.discordExecutor == nil {
		return &ToolResult{Success: false, Error: "Discord not available"}
	}

	userID, _ := args["user_id"].(string)
	if userID == "" {
		return &ToolResult{Success: false, Error: "user_id is required"}
	}

	userInfo, err := e.discordExecutor.GetUserInfo(ctx, userID)
	if err != nil {
		return &ToolResult{Success: false, Error: err.Error()}
	}

	return &ToolResult{
		Success: true,
		Data:    userInfo,
	}
}

func (e *Executor) executeDiscordGetChannelInfo(ctx context.Context, execCtx *ExecutionContext, args map[string]interface{}) *ToolResult {
	if e.discordExecutor == nil {
		return &ToolResult{Success: false, Error: "Discord not available"}
	}

	channelID, _ := args["channel_id"].(string)
	if channelID == "" {
		channelID = execCtx.ChannelID
	}
	if channelID == "" {
		return &ToolResult{Success: false, Error: "channel_id is required"}
	}

	channelInfo, err := e.discordExecutor.GetChannelInfo(ctx, channelID)
	if err != nil {
		return &ToolResult{Success: false, Error: err.Error()}
	}

	return &ToolResult{
		Success: true,
		Data:    channelInfo,
	}
}

// ============================================================================
// Personality/Mimic Tool Implementations
// ============================================================================

func (e *Executor) executeMimicPersonality(ctx context.Context, execCtx *ExecutionContext, args map[string]interface{}) *ToolResult {
	if e.discordExecutor == nil {
		return &ToolResult{Success: false, Error: "Discord not available - mimicking only works in Discord"}
	}

	userID, _ := args["user_id"].(string)
	if userID == "" {
		return &ToolResult{Success: false, Error: "user_id is required"}
	}

	channelID, _ := args["channel_id"].(string)
	if channelID == "" {
		channelID = execCtx.ChannelID
	}
	if channelID == "" {
		return &ToolResult{Success: false, Error: "channel_id is required for personality analysis"}
	}

	messageCount := 50
	if mc, ok := args["message_count"].(float64); ok {
		messageCount = int(mc)
	}

	// Save the original personality before mimicking
	originalPersonality := ""
	state, err := e.repo.FetchState(ctx, execCtx.AgentID)
	if err == nil && state != nil {
		originalPersonality = state.Identity.Personality
	}

	// Analyze the user's personality
	profile, err := e.discordExecutor.AnalyzeUserPersonality(ctx, channelID, userID, messageCount)
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("Failed to analyze personality: %v", err)}
	}

	// Store the mimic state
	e.mimicStates[execCtx.AgentID] = &MimicState{
		Active:              true,
		OriginalPersonality: originalPersonality,
		MimicProfile:        profile,
	}

	e.logger.Info("Mimic mode activated",
		zap.String("agent_id", execCtx.AgentID),
		zap.String("mimicking_user", profile.Username),
		zap.Int("messages_analyzed", profile.MessageCount),
	)

	return &ToolResult{
		Success: true,
		Data: map[string]interface{}{
			"mimicking":          profile.Username,
			"messages_analyzed":  profile.MessageCount,
			"style":              profile.ToneIndicators,
			"capitalization":     profile.Capitalization,
			"avg_message_length": profile.AvgMessageLength,
		},
		Message: fmt.Sprintf("Now mimicking %s's personality based on %d messages. Use revert_personality to stop.", profile.Username, profile.MessageCount),
	}
}

func (e *Executor) executeRevertPersonality(ctx context.Context, execCtx *ExecutionContext) *ToolResult {
	state := e.mimicStates[execCtx.AgentID]
	if state == nil || !state.Active {
		return &ToolResult{
			Success: true,
			Message: "Not currently mimicking anyone.",
		}
	}

	mimickedUser := ""
	if state.MimicProfile != nil {
		mimickedUser = state.MimicProfile.Username
	}

	// Clear the mimic state
	delete(e.mimicStates, execCtx.AgentID)

	e.logger.Info("Mimic mode deactivated",
		zap.String("agent_id", execCtx.AgentID),
		zap.String("was_mimicking", mimickedUser),
	)

	return &ToolResult{
		Success: true,
		Message: fmt.Sprintf("Stopped mimicking %s. Reverted to original personality.", mimickedUser),
	}
}

func (e *Executor) executeAnalyzeUserStyle(ctx context.Context, execCtx *ExecutionContext, args map[string]interface{}) *ToolResult {
	if e.discordExecutor == nil {
		return &ToolResult{Success: false, Error: "Discord not available"}
	}

	userID, _ := args["user_id"].(string)
	if userID == "" {
		return &ToolResult{Success: false, Error: "user_id is required"}
	}

	channelID, _ := args["channel_id"].(string)
	if channelID == "" {
		channelID = execCtx.ChannelID
	}
	if channelID == "" {
		return &ToolResult{Success: false, Error: "channel_id is required"}
	}

	profile, err := e.discordExecutor.AnalyzeUserPersonality(ctx, channelID, userID, 100)
	if err != nil {
		return &ToolResult{Success: false, Error: err.Error()}
	}

	return &ToolResult{
		Success: true,
		Data: map[string]interface{}{
			"username":           profile.Username,
			"messages_analyzed":  profile.MessageCount,
			"avg_message_length": profile.AvgMessageLength,
			"capitalization":     profile.Capitalization,
			"punctuation":        profile.PunctuationStyle,
			"tone":               profile.ToneIndicators,
			"common_words":       profile.CommonWords,
			"emoji_usage":        profile.EmojiUsage,
			"sample_messages":    profile.SampleMessages,
		},
		Message: fmt.Sprintf("Analyzed %d messages from %s", profile.MessageCount, profile.Username),
	}
}

// ============================================================================
// Helper Functions
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

