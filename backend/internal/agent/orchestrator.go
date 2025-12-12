package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"ezra-clone/backend/internal/adapter"
	"ezra-clone/backend/internal/constants"
	"ezra-clone/backend/internal/graph"
	"ezra-clone/backend/internal/tools"
	"ezra-clone/backend/pkg/logger"
	"go.uber.org/zap"
)

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

// SetComfyExecutor sets the ComfyUI executor for image generation tools
func (o *Orchestrator) SetComfyExecutor(ce *tools.ComfyExecutor) {
	o.toolExecutor.SetComfyExecutor(ce)
}

// SetMusicExecutor sets the music executor for music playback tools
func (o *Orchestrator) SetMusicExecutor(me *tools.MusicExecutor) {
	o.toolExecutor.SetMusicExecutor(me)
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
	return o.runTurnRecursiveWithImage(ctx, execCtx, message, depth, nil, "", nil)
}

// runTurnRecursiveWithImage executes a turn with recursion tracking and preserves image data
func (o *Orchestrator) runTurnRecursiveWithImage(ctx context.Context, execCtx *tools.ExecutionContext, message string, depth int, preservedImageData []byte, preservedImageName string, preservedImageMeta map[string]interface{}) (*TurnResult, error) {
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

	// 6. Get all tools
	allTools := tools.GetAllTools()

	// 7. Think - Call LLM
	llmResponse, err := o.llm.Generate(ctx, systemPrompt, message, allTools)
	if err != nil {
		return nil, fmt.Errorf("failed to generate LLM response: %w", err)
	}

	// 6. Act - Execute tool calls
	var toolResults []string
	var embeds []Embed
	// Start with preserved image data from previous turns
	imageData := preservedImageData
	imageName := preservedImageName
	imageMeta := preservedImageMeta
	if imageMeta == nil {
		imageMeta = make(map[string]interface{})
	}
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

				// Check for image data from image generation tool
				if toolCall.Name == tools.ToolGenerateImageWithRunPod && result.Data != nil {
					if dataMap, ok := result.Data.(map[string]interface{}); ok {
						if imgData, ok := dataMap["image_data"].([]byte); ok && len(imgData) > 0 {
							imageData = imgData
							if format, ok := dataMap["image_format"].(string); ok {
								imageName = fmt.Sprintf("image.%s", format)
							} else {
								imageName = "image.png"
							}
							
							// Extract metadata for embed
							imageMeta = make(map[string]interface{})
							if seed, ok := dataMap["seed"]; ok {
								imageMeta["seed"] = seed
							}
							if width, ok := dataMap["width"]; ok {
								imageMeta["width"] = width
							}
							if height, ok := dataMap["height"]; ok {
								imageMeta["height"] = height
							}
							if workflow, ok := dataMap["workflow"]; ok {
								imageMeta["workflow"] = workflow
							}
							if elapsed, ok := dataMap["elapsed_seconds"]; ok {
								imageMeta["elapsed_seconds"] = elapsed
							}
							
							o.logger.Debug("Captured image data from tool result",
								zap.Int("image_size", len(imageData)),
								zap.String("image_name", imageName),
							)
						}
					}
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
		if llmResponse.Content == "" && depth < constants.MaxRecursionDepth-1 && len(toolResults) > 0 {
			// Include tool results in the next message so LLM knows what happened
			contextMessage := fmt.Sprintf("%s\n\n[Tool Results]:\n%s\n\nNow provide a helpful response to the user based on these results.",
				message, strings.Join(toolResults, "\n"))
			o.logger.Debug("Recursing with tool context",
				zap.Int("new_depth", depth+1),
				zap.Int("tool_results", len(toolResults)),
			)
			// Preserve image data through recursive call
			return o.runTurnRecursiveWithImage(ctx, execCtx, contextMessage, depth+1, imageData, imageName, imageMeta)
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
		ImageData: imageData,
		ImageName: imageName,
		ImageMeta: imageMeta,
	}

	return turnResult, nil
}

// buildSystemPrompt is defined in prompt_builder.go
// formatToolResponseWithEmbeds is defined in response_formatter.go
// isInformationalTool is defined in response_formatter.go
// formatTimestamp is defined in response_formatter.go
