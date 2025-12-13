package agent

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"ezra-clone/backend/internal/adapter"
	"ezra-clone/backend/internal/constants"
	"ezra-clone/backend/internal/graph"
	"ezra-clone/backend/internal/tools"
	apperrors "ezra-clone/backend/pkg/errors"
	"ezra-clone/backend/pkg/logger"
	"go.uber.org/zap"
)

var (
	// ErrIgnored is returned when the agent chooses to ignore a message
	ErrIgnored = apperrors.ErrAgentIgnored
	// ErrMaxRecursion is returned when maximum recursion depth is reached
	ErrMaxRecursion = apperrors.NewBaseError(apperrors.ErrorTypeAgent, "maximum recursion depth reached", nil)
)

// Orchestrator manages the agent's reasoning and action loop
type Orchestrator struct {
	graphRepo         *graph.Repository
	llm               *adapter.LLMAdapter
	toolExecutor      *tools.Executor
	memoryEvaluator   *MemoryEvaluator
	toolResultProc    *ToolResultProcessor
	logger            *zap.Logger
}

// NewOrchestrator creates a new agent orchestrator
func NewOrchestrator(graphRepo *graph.Repository, llm *adapter.LLMAdapter) *Orchestrator {
	log := logger.Get()
	return &Orchestrator{
		graphRepo:       graphRepo,
		llm:             llm,
		toolExecutor:    tools.NewExecutor(graphRepo),
		memoryEvaluator: NewMemoryEvaluator(llm, graphRepo),
		toolResultProc:  NewToolResultProcessor(log),
		logger:          log,
	}
}

// SetDiscordExecutor sets the Discord executor for Discord-specific tools
func (o *Orchestrator) SetDiscordExecutor(de *tools.DiscordExecutor) {
	o.toolExecutor.SetDiscordExecutor(de)
}

// SetComfyExecutor sets the ComfyUI executor for image generation tools
func (o *Orchestrator) SetComfyExecutor(ce *tools.ComfyExecutor) {
	o.toolExecutor.SetComfyExecutor(ce)
}

// SetMusicExecutor sets the music executor for music playback tools
func (o *Orchestrator) SetMusicExecutor(me *tools.MusicExecutor) {
	o.toolExecutor.SetMusicExecutor(me)
}

// SetMimicBackgroundTask sets the background task manager for mimic mode
func (o *Orchestrator) SetMimicBackgroundTask(task *tools.MimicBackgroundTask) {
	o.toolExecutor.SetMimicBackgroundTask(task)
}

// SetSystemExecutor sets the system executor for system control tools
func (o *Orchestrator) SetSystemExecutor(se *tools.SystemExecutor) {
	o.toolExecutor.SetSystemExecutor(se)
}

// SetLLMAdapterForTools sets the LLM adapter for tools that need it (like website summarization)
func (o *Orchestrator) SetLLMAdapterForTools(llmAdapter *adapter.LLMAdapter) {
	o.toolExecutor.SetLLMAdapter(llmAdapter)
}

// GetToolExecutor returns the tool executor (for background tasks)
func (o *Orchestrator) GetToolExecutor() *tools.Executor {
	return o.toolExecutor
}

// TurnResult represents the result of a single agent turn
type TurnResult struct {
	Content   string
	ToolCalls []adapter.ToolCall
	Ignored   bool
	Embeds    []Embed // Optional embeds for rich content
	ImageData []byte  // Optional image data for Discord attachment
	ImageName string  // Optional image filename for Discord attachment
	ImageMeta map[string]interface{} // Optional image metadata (seed, dimensions, etc.)
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
	return o.runTurnRecursiveWithImage(ctx, execCtx, message, depth, nil, "", nil, nil)
}

// runTurnRecursiveWithImage executes a turn with recursion tracking and preserves image data
func (o *Orchestrator) runTurnRecursiveWithImage(ctx context.Context, execCtx *tools.ExecutionContext, message string, depth int, preservedImageData []byte, preservedImageName string, preservedImageMeta map[string]interface{}, preservedFetchedURLs []string) (*TurnResult, error) {
	if depth >= constants.MaxRecursionDepth {
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

	// 2. Get agent config to use the correct model
	agentConfig, err := o.graphRepo.GetAgentConfig(ctx, execCtx.AgentID)
	if err == nil && agentConfig.Model != "" {
		// Temporarily set the model for this agent's turn
		originalModel := o.llm.GetModel()
		o.llm.SetModel(agentConfig.Model)
		defer func() {
			// Restore original model after the turn
			o.llm.SetModel(originalModel)
		}()
	}

	// 3. Get user context if available
	userCtx, _ := o.graphRepo.GetUserContext(ctx, execCtx.UserID)

	// 4. Get recent conversation history for context (if channel ID is available)
	var conversationHistory []graph.Message
	if execCtx.ChannelID != "" {
		history, err := o.graphRepo.GetConversationHistory(ctx, execCtx.ChannelID, 15)
		if err == nil {
			conversationHistory = history
		} else {
			o.logger.Debug("Failed to fetch conversation history", zap.Error(err))
		}
	}

	// 5. Build System Prompt
	systemPrompt, err := o.buildSystemPrompt(ctxWindow, userCtx, execCtx, conversationHistory)
	if err != nil {
		return nil, fmt.Errorf("failed to build system prompt: %w", err)
	}

	// 6. Get all tools, but filter out mimic_personality if already mimicking
	allTools := tools.GetAllTools()
	
	// If already mimicking, remove mimic_personality tool unless user explicitly wants to mimic someone
	if o.toolExecutor.IsMimicking(execCtx.AgentID) {
		// Check if user explicitly mentions wanting to mimic someone (different user or update)
		messageLower := strings.ToLower(message)
		shouldAllowMimicTool := strings.Contains(messageLower, "mimic") || 
		                        strings.Contains(messageLower, "update personality") ||
		                        strings.Contains(messageLower, "refresh personality")
		
		if !shouldAllowMimicTool {
			// Filter out mimic_personality tool
			filteredTools := make([]adapter.Tool, 0, len(allTools))
			for _, tool := range allTools {
				if tool.Function.Name != tools.ToolMimicPersonality {
					filteredTools = append(filteredTools, tool)
				}
			}
			allTools = filteredTools
			o.logger.Debug("Filtered out mimic_personality tool - already in mimic mode",
				zap.String("agent_id", execCtx.AgentID),
			)
		}
	}

	// 7. Think - Call LLM
	llmResponse, err := o.llm.Generate(ctx, systemPrompt, message, allTools)
	if err != nil {
		return nil, fmt.Errorf("failed to generate LLM response: %w", err)
	}

	// 6. Act - Execute tool calls
	var toolResults []string
	var embeds []Embed
	var fetchWebpageCount int
	var imageData []byte
	var imageName string
	var imageMeta map[string]interface{}
	var fetchedURLs []string

	if len(llmResponse.ToolCalls) > 0 {
		toolResults, imageData, imageName, imageMeta, fetchedURLs, embeds, fetchWebpageCount = o.toolResultProc.ProcessToolResults(
			ctx,
			llmResponse.ToolCalls,
			execCtx,
			o.toolExecutor,
			llmResponse,
			preservedImageData,
			preservedImageName,
			preservedImageMeta,
			preservedFetchedURLs,
		)

		// Check if user asked for multiple articles but we only fetched one
		messageLower := strings.ToLower(message)
		requestedMultipleArticles := strings.Contains(messageLower, "summarize") && 
			(strings.Contains(messageLower, "article") || strings.Contains(messageLower, "result") || strings.Contains(messageLower, "first") || strings.Contains(messageLower, "most interesting"))
		
		numArticlesRequested := 2 // default
		// Detect number of articles requested
		if strings.Contains(messageLower, "first 2") || strings.Contains(messageLower, "2 articles") || strings.Contains(messageLower, "2 most") {
			numArticlesRequested = 2
		} else if strings.Contains(messageLower, "first 3") || strings.Contains(messageLower, "3 articles") || strings.Contains(messageLower, "3 most") {
			numArticlesRequested = 3
		} else if strings.Contains(messageLower, "first 4") || strings.Contains(messageLower, "4 articles") || strings.Contains(messageLower, "4 most") {
			numArticlesRequested = 4
		} else if strings.Contains(messageLower, "first") || strings.Contains(messageLower, "most interesting") {
			numArticlesRequested = 2
		}
		
		// If we have tool results but no content, and haven't hit max depth, recurse WITH tool context
		shouldRecurse := llmResponse.Content == "" && depth < constants.MaxRecursionDepth-1 && len(toolResults) > 0
		
		// Also recurse if user asked for multiple articles but we haven't fetched enough yet
		// BUT: if we have enough articles, STOP recursing and force summarization
		if requestedMultipleArticles {
			if fetchWebpageCount < numArticlesRequested && depth < constants.MaxRecursionDepth-1 {
				// Need more articles - force recursion
				shouldRecurse = true
				// Add instruction about needing more articles
				if len(toolResults) == 0 {
					toolResults = append(toolResults, "[Status]: Need to fetch more articles")
				}
			} else if fetchWebpageCount >= numArticlesRequested {
				// We have enough articles - STOP fetching, only recurse if we need to summarize
				// Don't recurse if we already have content (LLM already responded)
				if llmResponse.Content == "" {
					// No content yet, recurse once more to get summary
					shouldRecurse = true
				} else {
					// We have content, don't recurse
					shouldRecurse = false
				}
			}
		}
		
		if shouldRecurse {
			// Include tool results in the next message so LLM knows what happened
			// Add a summary of fetched URLs at the top for clarity
			toolResultsWithSummary := make([]string, 0)
			if len(fetchedURLs) > 0 {
				toolResultsWithSummary = append(toolResultsWithSummary, fmt.Sprintf("[SUMMARY]: You have already fetched %d article(s):", len(fetchedURLs)))
				for i, url := range fetchedURLs {
					toolResultsWithSummary = append(toolResultsWithSummary, fmt.Sprintf("  Article %d: %s", i+1, url))
				}
				toolResultsWithSummary = append(toolResultsWithSummary, "")
			}
			toolResultsWithSummary = append(toolResultsWithSummary, toolResults...)
			
			contextMessage := fmt.Sprintf("%s\n\n[Tool Results]:\n%s\n\nNow provide a helpful response to the user based on these results.",
				message, strings.Join(toolResultsWithSummary, "\n"))
			
			// If user asked to summarize articles, add explicit instruction
			if requestedMultipleArticles {
				if fetchWebpageCount < numArticlesRequested {
					contextMessage += fmt.Sprintf("\n\nCRITICAL: You have only fetched %d article(s), but the user asked for %d articles. ", fetchWebpageCount, numArticlesRequested)
					contextMessage += fmt.Sprintf("You MUST call fetch_webpage %d more time(s) to fetch DIFFERENT articles.\n", numArticlesRequested-fetchWebpageCount)
					
					// List already fetched URLs to prevent duplicates - make it VERY explicit
					if len(fetchedURLs) > 0 {
						contextMessage += fmt.Sprintf("\n🚫 ALREADY FETCHED URLs - DO NOT FETCH THESE AGAIN:\n")
						for i, url := range fetchedURLs {
							contextMessage += fmt.Sprintf("  %d. %s\n", i+1, url)
						}
						contextMessage += fmt.Sprintf("\n⚠️ WARNING: If you fetch any of these URLs again, you will waste tokens and not get new information!\n")
					}
					
					contextMessage += fmt.Sprintf("\nCRITICAL INSTRUCTIONS - READ CAREFULLY:\n")
					contextMessage += fmt.Sprintf("- The user asked to summarize %d ARTICLES\n", numArticlesRequested)
					contextMessage += fmt.Sprintf("- You have already fetched %d article(s)\n", fetchWebpageCount)
					contextMessage += fmt.Sprintf("- You need to fetch %d MORE DIFFERENT article(s)\n", numArticlesRequested-fetchWebpageCount)
					contextMessage += fmt.Sprintf("- Use the ARTICLE URLs from the search results above (the URLs listed under 'ARTICLE 1', 'ARTICLE 2', etc.)\n")
					contextMessage += fmt.Sprintf("- BEFORE calling fetch_webpage, check the 'ALREADY FETCHED URLs' list above\n")
					contextMessage += fmt.Sprintf("- The URL you fetch MUST be DIFFERENT from all URLs in that list\n")
					contextMessage += "- DO NOT fetch the search results page URL (html.duckduckgo.com)\n"
					contextMessage += "- DO NOT fetch URLs that contain 'duckduckgo.com', 'search', or look like article list pages\n"
					contextMessage += "- DO NOT fetch URLs ending in '/ai-news-december-2025', '/monthly-digest', '/in-depth-and-concise', or similar list/digest pages\n"
					contextMessage += "- DO NOT fetch URLs that are article collections, digests, or roundups\n"
					contextMessage += fmt.Sprintf("- You need to fetch ARTICLE %d URL (use fetch_webpage with that URL - make sure it's DIFFERENT from URLs already fetched)\n", fetchWebpageCount+1)
					if numArticlesRequested > fetchWebpageCount+1 {
						contextMessage += fmt.Sprintf("- Then fetch ARTICLE %d URL (use fetch_webpage again with that URL - also DIFFERENT)\n", fetchWebpageCount+2)
					}
				} else {
					// We have enough articles - STRONGLY instruct to summarize and STOP fetching
					contextMessage += fmt.Sprintf("\n\n✅ SUCCESS: You have fetched %d article(s) as requested. ", fetchWebpageCount)
					contextMessage += fmt.Sprintf("You MUST NOW PROVIDE A SUMMARY - DO NOT FETCH ANY MORE ARTICLES.\n\n")
					contextMessage += fmt.Sprintf("CRITICAL INSTRUCTIONS:\n")
					contextMessage += fmt.Sprintf("1. You have all the article content you need in the tool results above\n")
					contextMessage += fmt.Sprintf("2. DO NOT call fetch_webpage again - you already have %d articles\n", fetchWebpageCount)
					contextMessage += fmt.Sprintf("3. DO NOT call web_search again\n")
					contextMessage += fmt.Sprintf("4. You MUST provide a well-formatted summary that:\n")
					contextMessage += fmt.Sprintf("   - Summarizes each of the %d articles you fetched\n", numArticlesRequested)
					contextMessage += fmt.Sprintf("   - Highlights the key points from each article\n")
					contextMessage += fmt.Sprintf("   - Formats the summary nicely with clear sections\n")
					contextMessage += fmt.Sprintf("   - Uses the send_message tool to send the summary to the user\n")
					contextMessage += fmt.Sprintf("5. Your response should be a complete, formatted summary - not just raw content\n")
				}
			}
			o.logger.Debug("Recursing with tool context",
				zap.Int("new_depth", depth+1),
				zap.Int("tool_results", len(toolResults)),
			)
			// Preserve image data through recursive call
			return o.runTurnRecursiveWithImage(ctx, execCtx, contextMessage, depth+1, imageData, imageName, imageMeta, fetchedURLs)
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
	turnResult := BuildTurnResult(llmResponse, embeds, imageData, imageName, imageMeta)

	return turnResult, nil
}

// smartChunkContent intelligently splits content into chunks at natural boundaries
// It tries to split at paragraph breaks first, then sentence breaks, avoiding mid-word splits
func smartChunkContent(content string, maxChunkSize int) []string {
	if len(content) <= maxChunkSize {
		return []string{content}
	}

	var chunks []string
	remaining := content

	for len(remaining) > maxChunkSize {
		// Try to find a good split point
		chunk := remaining[:maxChunkSize]
		
		// First, try to split at a paragraph break (double newline)
		if idx := strings.LastIndex(chunk, "\n\n"); idx > maxChunkSize*3/4 {
			chunks = append(chunks, remaining[:idx+2])
			remaining = strings.TrimSpace(remaining[idx+2:])
			continue
		}
		
		// Then try to split at a single newline (paragraph end)
		if idx := strings.LastIndex(chunk, "\n"); idx > maxChunkSize*3/4 {
			chunks = append(chunks, remaining[:idx+1])
			remaining = strings.TrimSpace(remaining[idx+1:])
			continue
		}
		
		// Try to split at sentence boundaries (period, exclamation, question mark followed by space)
		sentenceEnd := regexp.MustCompile(`[.!?]\s+`)
		matches := sentenceEnd.FindAllStringIndex(chunk, -1)
		if len(matches) > 0 {
			// Use the last sentence boundary that's in the last quarter of the chunk
			for i := len(matches) - 1; i >= 0; i-- {
				idx := matches[i][1]
				if idx > maxChunkSize*3/4 {
					chunks = append(chunks, remaining[:idx])
					remaining = strings.TrimSpace(remaining[idx:])
					goto nextChunk
				}
			}
		}
		
		// Last resort: split at word boundary (space)
		if idx := strings.LastIndex(chunk, " "); idx > maxChunkSize*2/3 {
			chunks = append(chunks, remaining[:idx])
			remaining = strings.TrimSpace(remaining[idx:])
		} else {
			// No good split point, force split (shouldn't happen often)
			chunks = append(chunks, remaining[:maxChunkSize])
			remaining = remaining[maxChunkSize:]
		}
		
	nextChunk:
		continue
	}

	if len(remaining) > 0 {
		chunks = append(chunks, remaining)
	}

	return chunks
}

// buildSystemPrompt is defined in prompt_builder.go
// formatToolResponseWithEmbeds is defined in response_formatter.go
// isInformationalTool is defined in response_formatter.go
// formatTimestamp is defined in response_formatter.go
