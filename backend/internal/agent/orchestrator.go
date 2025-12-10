package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"ezra-clone/backend/internal/adapter"
	"ezra-clone/backend/internal/graph"
	"ezra-clone/backend/internal/state"
	"ezra-clone/backend/internal/tools"
	"ezra-clone/backend/pkg/logger"
	"go.uber.org/zap"
)

const maxRecursionDepth = 5

var (
	ErrIgnored      = errors.New("turn was ignored by agent")
	ErrMaxRecursion = errors.New("maximum recursion depth reached")
)

// Orchestrator manages the agent's reasoning and action loop
type Orchestrator struct {
	graphRepo       *graph.Repository
	llm             *adapter.LLMAdapter
	toolExecutor    *tools.Executor
	memoryEvaluator *MemoryEvaluator
	logger          *zap.Logger
}

// NewOrchestrator creates a new agent orchestrator
func NewOrchestrator(graphRepo *graph.Repository, llm *adapter.LLMAdapter) *Orchestrator {
	return &Orchestrator{
		graphRepo:       graphRepo,
		llm:             llm,
		toolExecutor:    tools.NewExecutor(graphRepo),
		memoryEvaluator: NewMemoryEvaluator(llm, graphRepo),
		logger:          logger.Get(),
	}
}

// SetDiscordExecutor sets the Discord executor for Discord-specific tools
func (o *Orchestrator) SetDiscordExecutor(de *tools.DiscordExecutor) {
	o.toolExecutor.SetDiscordExecutor(de)
}

// TurnResult represents the result of a single agent turn
type TurnResult struct {
	Content   string
	ToolCalls []adapter.ToolCall
	Ignored   bool
	Embeds    []Embed // Optional embeds for rich content
}

// Embed represents a Discord-style embed
type Embed struct {
	Title       string       `json:"title,omitempty"`
	Description string       `json:"description,omitempty"`
	URL         string       `json:"url,omitempty"`
	Color       int          `json:"color,omitempty"`
	Fields      []EmbedField `json:"fields,omitempty"`
	Footer      string       `json:"footer,omitempty"`
}

// EmbedField represents a field in an embed
type EmbedField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline,omitempty"`
}

// RunTurn executes a single turn of the agent's reasoning loop
func (o *Orchestrator) RunTurn(ctx context.Context, agentID, userID, message string) (*TurnResult, error) {
	return o.RunTurnWithContext(ctx, agentID, userID, "", "web", message)
}

// RunTurnWithContext executes a turn with full context
func (o *Orchestrator) RunTurnWithContext(ctx context.Context, agentID, userID, channelID, platform, message string) (*TurnResult, error) {
	execCtx := &tools.ExecutionContext{
		AgentID:   agentID,
		UserID:    userID,
		ChannelID: channelID,
		Platform:  platform,
	}
	return o.runTurnRecursive(ctx, execCtx, message, 0)
}

// runTurnRecursive executes a turn with recursion tracking
func (o *Orchestrator) runTurnRecursive(ctx context.Context, execCtx *tools.ExecutionContext, message string, depth int) (*TurnResult, error) {
	if depth >= maxRecursionDepth {
		return nil, ErrMaxRecursion
	}

	o.logger.Debug("Starting agent turn",
		zap.String("agent_id", execCtx.AgentID),
		zap.String("user_id", execCtx.UserID),
		zap.Int("depth", depth),
	)

	// 1. Load State
	ctxWindow, err := o.graphRepo.FetchState(ctx, execCtx.AgentID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch state: %w", err)
	}

	// 2. Get user context if available
	userCtx, _ := o.graphRepo.GetUserContext(ctx, execCtx.UserID)

	// 3. Build System Prompt
	systemPrompt, err := o.buildSystemPrompt(ctxWindow, userCtx, execCtx)
	if err != nil {
		return nil, fmt.Errorf("failed to build system prompt: %w", err)
	}

	// 4. Get all tools
	allTools := tools.GetAllTools()

	// 5. Think - Call LLM
	llmResponse, err := o.llm.Generate(ctx, systemPrompt, message, allTools)
	if err != nil {
		return nil, fmt.Errorf("failed to generate LLM response: %w", err)
	}

	// 6. Act - Execute tool calls
	var toolResults []string
	var embeds []Embed
	if len(llmResponse.ToolCalls) > 0 {
		for _, toolCall := range llmResponse.ToolCalls {
			result := o.toolExecutor.Execute(ctx, execCtx, toolCall)

			if result.Success {
				o.logger.Info("Tool executed successfully",
					zap.String("tool", toolCall.Name),
					zap.String("message", result.Message),
				)

				// Capture tool results for context
				if result.Message != "" {
					toolResults = append(toolResults, fmt.Sprintf("[%s]: %s", toolCall.Name, result.Message))
				}

				// For informational tools, use result data to build response and embeds
				if isInformationalTool(toolCall.Name) && result.Data != nil {
					response, toolEmbeds := formatToolResponseWithEmbeds(toolCall.Name, result)
					if response != "" && llmResponse.Content == "" {
						llmResponse.Content = response
					}
					if len(toolEmbeds) > 0 {
						embeds = append(embeds, toolEmbeds...)
					}
				}

				// If send_message tool was used, capture the message as content
				if toolCall.Name == tools.ToolSendMessage && result.Message != "" {
					if llmResponse.Content == "" {
						llmResponse.Content = result.Message
					}
				}
			} else {
				o.logger.Warn("Tool execution failed",
					zap.String("tool", toolCall.Name),
					zap.String("error", result.Error),
				)
				toolResults = append(toolResults, fmt.Sprintf("[%s] ERROR: %s", toolCall.Name, result.Error))
			}
		}

		// If we have tool results but no content, and haven't hit max depth, recurse WITH tool context
		if llmResponse.Content == "" && depth < maxRecursionDepth-1 && len(toolResults) > 0 {
			// Include tool results in the next message so LLM knows what happened
			contextMessage := fmt.Sprintf("%s\n\n[Tool Results]:\n%s\n\nNow provide a helpful response to the user based on these results.",
				message, strings.Join(toolResults, "\n"))
			o.logger.Debug("Recursing with tool context",
				zap.Int("new_depth", depth+1),
				zap.Int("tool_results", len(toolResults)),
			)
			return o.runTurnRecursive(ctx, execCtx, contextMessage, depth+1)
		}

		// Default response if we hit max depth without content
		if llmResponse.Content == "" {
			if len(toolResults) > 0 {
				// Use the tool results as the response
				llmResponse.Content = strings.Join(toolResults, "\n")
			} else {
				llmResponse.Content = "I've completed the requested actions."
			}
		}
	}

	// 7. Log Interaction
	if err := o.graphRepo.LogInteraction(ctx, execCtx.AgentID, execCtx.UserID, message, time.Now()); err != nil {
		o.logger.Warn("Failed to log interaction", zap.Error(err))
	}

	// 8. Log message to conversation
	if execCtx.ChannelID != "" {
		_ = o.graphRepo.LogMessage(ctx, execCtx.AgentID, execCtx.UserID, execCtx.ChannelID, message, "user", execCtx.Platform)
		if llmResponse.Content != "" {
			_ = o.graphRepo.LogMessage(ctx, execCtx.AgentID, execCtx.UserID, execCtx.ChannelID, llmResponse.Content, "agent", execCtx.Platform)
		}
	}

	// 9. Auto-evaluate and save memory (async, non-blocking)
	go func() {
		evalCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		decision, err := o.memoryEvaluator.EvaluateMessage(evalCtx, execCtx.AgentID, execCtx.UserID, message)
		if err != nil {
			o.logger.Debug("Memory evaluation failed (non-critical)",
				zap.String("user_id", execCtx.UserID),
				zap.Error(err),
			)
			return
		}

		if decision != nil && decision.ShouldSave {
			if err := o.memoryEvaluator.ApplyDecision(evalCtx, execCtx.AgentID, execCtx.UserID, decision); err != nil {
				o.logger.Warn("Failed to auto-save memory",
					zap.String("user_id", execCtx.UserID),
					zap.String("memory_type", decision.MemoryType),
					zap.Error(err),
				)
			}
		}
	}()

	// Build result with any embeds
	turnResult := &TurnResult{
		Content:   llmResponse.Content,
		ToolCalls: llmResponse.ToolCalls,
		Ignored:   false,
		Embeds:    embeds,
	}

	return turnResult, nil
}

// buildSystemPrompt creates a comprehensive system prompt with all context
func (o *Orchestrator) buildSystemPrompt(ctxWindow *state.ContextWindow, userCtx *graph.UserContext, execCtx *tools.ExecutionContext) (string, error) {
	// Serialize agent state
	agentStateJSON, err := json.MarshalIndent(ctxWindow, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal context window: %w", err)
	}

	// Build user context section
	userSection := ""
	if userCtx != nil {
		userInfo := map[string]interface{}{
			"user_id":           userCtx.User.ID,
			"discord_id":        userCtx.User.DiscordID,
			"username":          userCtx.User.DiscordUsername,
			"preferred_language": userCtx.User.PreferredLanguage,
			"message_count":     userCtx.MessageCount,
			"interests":          []string{},
			"known_facts":        []string{},
		}

		for _, t := range userCtx.Topics {
			if interests, ok := userInfo["interests"].([]string); ok {
				userInfo["interests"] = append(interests, t.Name)
			}
		}

		for _, f := range userCtx.Facts {
			if facts, ok := userInfo["known_facts"].([]string); ok {
				userInfo["known_facts"] = append(facts, f.Content)
			}
		}

		userJSON, _ := json.MarshalIndent(userInfo, "", "  ")
		userSection = fmt.Sprintf(`
## Current User Context
%s
`, string(userJSON))
	}

	// Check if we're in mimic mode
	mimicSection := ""
	if o.toolExecutor.IsMimicking(execCtx.AgentID) {
		mimicPrompt := o.toolExecutor.GetMimicPrompt(execCtx.AgentID)
		if mimicPrompt != "" {
			mimicSection = fmt.Sprintf(`
## ⚠️ PERSONALITY MIMIC MODE ACTIVE ⚠️

%s

IMPORTANT: While in mimic mode:
- Completely adopt the communication style described above
- Maintain this style in ALL responses until asked to revert
- You still have access to all your tools and knowledge
- If asked to "revert", "stop mimicking", or "be yourself", use the revert_personality tool
`, mimicPrompt)
		}
	}

	// Check for language preference - from user property or facts
	// Default to English if no preference is set
	languageSection := ""
	var preferredLang string
	var langName string
	
	if userCtx != nil {
		// First check user's preferred language property
		preferredLang = userCtx.User.PreferredLanguage
		
		// If not set, check facts for language preferences
		if preferredLang == "" && len(userCtx.Facts) > 0 {
			preferredLang, langName = extractLanguageFromFacts(userCtx.Facts)
		}
		
		// Default to English if no preference found
		if preferredLang == "" {
			preferredLang = "en"
			langName = "English"
		} else {
			if langName == "" {
				langName = getLanguageName(preferredLang)
			}
		}
		
		// Only add language section if preference is NOT English (English is the default)
		if preferredLang != "en" && preferredLang != "" {
			langCodeSuffix := ""
			if preferredLang != langName {
				langCodeSuffix = fmt.Sprintf(" (language code: %s)", preferredLang)
			}
			
			languageSection = fmt.Sprintf(`
## 🌍 LANGUAGE PREFERENCE

IMPORTANT: The current user prefers to communicate in %s%s.

You MUST respond in %s unless:
- The user explicitly asks you to respond in a different language
- The user says "don't speak %s", "speak english", or similar override requests

This is a persistent preference that should be remembered for all future conversations with this user.
`, langName, langCodeSuffix, langName, strings.ToLower(langName))
		}
		// If preferredLang is "en" or empty, no language section is added (English is default)
	}

	// Get current date for context
	currentDate := time.Now().Format("Monday, January 2, 2006")
	currentYear := time.Now().Year()
	currentMonth := time.Now().Format("January")

	prompt := fmt.Sprintf(`# Ezra - AI Agent System

You are Ezra, an intelligent AI agent with persistent memory and the ability to learn and remember information about users.

## Current Date
Today is %s. When searching for current events or news, use "%s %d" or similar date context in your queries.
%s%s
## Your Core State
%s
%s
## Platform Information
- Platform: %s
- Channel ID: %s

## Your Capabilities

You have access to a comprehensive set of tools:

### Memory Tools
- **core_memory_insert**: Create new memory blocks to store important information permanently
- **core_memory_replace**: Update existing memory blocks
- **archival_memory_insert**: Archive information for long-term storage
- **archival_memory_search**: Search your archived memories
- **memory_search**: Search across all your memories

### Knowledge Management
- **create_fact**: Store facts and link them to topics and users
- **search_facts**: Search for facts about specific topics
- **get_user_context**: Get comprehensive information about a user

### Topic Management
- **create_topic**: Create topics to organize knowledge
- **link_topics**: Create relationships between topics
- **find_related_topics**: Find topics related to a given topic
- **link_user_to_topic**: Record a user's interest in a topic

### Conversation Tools
- **get_conversation_history**: Retrieve recent messages
- **send_message**: Send a response to the user

### Discord Tools (when on Discord)
- **discord_read_history**: Read message history from a Discord channel
- **discord_get_user_info**: Get information about a Discord user
- **discord_get_channel_info**: Get information about a Discord channel

### Personality/Mimic Tools
- **mimic_personality**: Analyze a user's messages and mimic their communication style
- **revert_personality**: Stop mimicking and return to your normal personality
- **analyze_user_style**: Analyze a user's communication style without mimicking

### External Tools
- **web_search**: Search the web for information
- **fetch_webpage**: Read content from a URL. USE THIS when user asks "what's on this page?", "tell me about this URL", or provides any URL
- **github_repo_info**: Get information about a GitHub repository
- **github_search**: Search GitHub for repositories, code, or issues
- **github_read_file**: Read a file from a GitHub repository
- **github_list_org_repos**: List an organization's repos sorted by most recently updated

## CRITICAL: ACTION-FIRST BEHAVIOR

**DO NOT ASK CLARIFYING QUESTIONS. USE TOOLS IMMEDIATELY.**

When a user asks something that can be answered with a tool, USE THE TOOL FIRST:
- "What was the last repo updated?" → Use github_list_org_repos with the org they mentioned
- "Tell me about system-nebula" → Use github_list_org_repos for system-nebula
- "What's happening with X repo?" → Use github_repo_info
- "Search for Y" → Use web_search or github_search
- "What's on this page? [URL]" → Use fetch_webpage with the URL
- "Tell me about [URL]" → Use fetch_webpage with the URL
- Any URL provided → Use fetch_webpage to read it

**NEVER say "what repo are you looking for?" or "can you clarify?"**
If you can make a reasonable guess about what they want, JUST DO IT.

## Important Instructions

1. **ACT FIRST, ASK LATER**: Use tools immediately when you can reasonably infer the intent
2. **Remember context**: If someone mentioned "system-nebula" earlier, assume future questions are about that org
3. **Use tools proactively**: When users share information, store it using create_fact or core_memory_insert
4. **Link information**: When learning something, create topics and link facts to them
5. **Remember user interests**: Track what users are interested in using link_user_to_topic
6. **Always respond with results**: After using tools, summarize what you found in plain language
7. **Be direct**: Don't be overly conversational. Answer the question with the data you retrieved.
8. **Mimic on request**: If a user says "mimic @user personality" or similar, use mimic_personality with their user ID
9. **Revert on request**: If user says "revert", "stop mimicking", "be yourself", use revert_personality
10. **URL handling**: If a user provides a URL or asks about a webpage, IMMEDIATELY use fetch_webpage with that URL

## User Information Queries

**CRITICAL**: When a user asks about themselves or another user (e.g., "what do I love?", "what are my interests?", "what do you know about @user?"), you MUST:
1. Use **get_user_context** tool immediately (no parameters needed for current user)
2. Read the returned facts and topics
3. Format a clear, friendly response listing what you found
4. If no information is found, say so honestly

**Examples:**
- "what do I love?" → Use get_user_context → Respond with list of preferences/interests
- "what are my interests?" → Use get_user_context → Respond with topics they're interested in
- "what do you know about me?" → Use get_user_context → Summarize all facts about them

## Response Format

USE TOOLS FIRST. Then provide a direct, helpful response with the information you found.
`, currentDate, currentMonth, currentYear, mimicSection, languageSection, string(agentStateJSON), userSection, execCtx.Platform, execCtx.ChannelID)

	return prompt, nil
}

// extractLanguageFromFacts searches user facts for language preferences
// Only considers facts that are ABOUT the current user, not facts mentioning other users
func extractLanguageFromFacts(facts []graph.Fact) (string, string) {
	languagePatterns := map[string][]string{
		"pig_latin": {"pig latin", "speaks pig latin", "only speaks pig latin", "only speak pig latin"},
		"fr":        {"french", "speaks french", "only speaks french", "only speak french", "from france"},
		"es":        {"spanish", "speaks spanish", "only speaks spanish", "only speak spanish"},
		"de":        {"german", "speaks german", "only speaks german", "only speak german"},
		"it":        {"italian", "speaks italian", "only speaks italian", "only speak italian"},
		"pt":        {"portuguese", "speaks portuguese", "only speaks portuguese", "only speak portuguese"},
		"ja":        {"japanese", "speaks japanese", "only speaks japanese", "only speak japanese"},
		"zh":        {"chinese", "speaks chinese", "only speaks chinese", "only speak chinese"},
		"ko":        {"korean", "speaks korean", "only speaks korean", "only speak korean"},
		"ru":        {"russian", "speaks russian", "only speaks russian", "only speak russian"},
		"en":        {"english", "speaks english", "only speaks english", "only speak english", "prefers to speak in english", "preferred language is english"},
	}
	
	// First, look for explicit language preference facts (highest priority)
	for _, fact := range facts {
		lowerFact := strings.ToLower(fact.Content)
		
		// Check for explicit preference statements
		if strings.Contains(lowerFact, "prefers to communicate in") || 
		   strings.Contains(lowerFact, "preferred language") ||
		   strings.Contains(lowerFact, "prefers to speak in") {
			// This is an explicit language preference fact
			for langCode, patterns := range languagePatterns {
				for _, pattern := range patterns {
					if strings.Contains(lowerFact, pattern) {
						return langCode, getLanguageName(langCode)
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
			for langCode, patterns := range languagePatterns {
				for _, pattern := range patterns {
					if strings.Contains(lowerFact, pattern) {
						return langCode, getLanguageName(langCode)
					}
				}
			}
		}
	}
	
	return "", ""
}

// getLanguageName returns the display name for a language code
func getLanguageName(langCode string) string {
	langNames := map[string]string{
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
	
	if name, ok := langNames[langCode]; ok {
		return name
	}
	return langCode // Return code if name not found
}

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
		// Format user context into a readable response
		if userCtx, ok := result.Data.(*graph.UserContext); ok {
			var parts []string
			
			if len(userCtx.Facts) > 0 {
				parts = append(parts, "Here's what I know about you:")
				for _, fact := range userCtx.Facts {
					parts = append(parts, fmt.Sprintf("• %s", fact.Content))
				}
			}
			
			if len(userCtx.Topics) > 0 {
				var topicNames []string
				for _, topic := range userCtx.Topics {
					topicNames = append(topicNames, topic.Name)
				}
				if len(parts) > 0 {
					parts = append(parts, "")
				}
				parts = append(parts, fmt.Sprintf("Your interests: %s", strings.Join(topicNames, ", ")))
			}
			
			if len(parts) == 0 {
				return "I don't have much information about you yet. Feel free to share something about yourself!", nil
			}
			
			return strings.Join(parts, "\n"), nil
		}
		return result.Message, nil

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
