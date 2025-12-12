package tools

import (
	"context"
	"fmt"
	"net/http"
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
	repo                *graph.Repository
	httpClient          *http.Client
	logger              *zap.Logger
	discordExecutor     *DiscordExecutor
	comfyExecutor       *ComfyExecutor
	musicExecutor       *MusicExecutor
	mimicStates         map[string]*MimicState // key: agentID
	mimicBackgroundTask *MimicBackgroundTask
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

// SetComfyExecutor sets the ComfyUI executor for image generation tools
func (e *Executor) SetComfyExecutor(ce *ComfyExecutor) {
	e.comfyExecutor = ce
}

// SetMusicExecutor sets the music executor for music playback tools
func (e *Executor) SetMusicExecutor(me *MusicExecutor) {
	e.musicExecutor = me
}

// SetMimicBackgroundTask sets the background task manager for mimic mode
func (e *Executor) SetMimicBackgroundTask(task *MimicBackgroundTask) {
	e.mimicBackgroundTask = task
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
	case ToolReadCodebase:
		return e.executeReadCodebase(ctx, execCtx, toolCall.Arguments)

	// Personality/Mimic Tools
	case ToolMimicPersonality:
		return e.executeMimicPersonality(ctx, execCtx, toolCall.Arguments)
	case ToolRevertPersonality:
		return e.executeRevertPersonality(ctx, execCtx)
	case ToolAnalyzeUserStyle:
		return e.executeAnalyzeUserStyle(ctx, execCtx, toolCall.Arguments)

	// ComfyUI Image Generation Tools
	case ToolGenerateImageWithRunPod:
		return e.executeGenerateImageWithRunPod(ctx, execCtx, toolCall.Arguments)
	case ToolEnhancePrompt:
		return e.executeEnhancePrompt(ctx, execCtx, toolCall.Arguments)
	case ToolSelectWorkflow:
		return e.executeSelectWorkflow(ctx, execCtx, toolCall.Arguments)
	case ToolListWorkflows:
		return e.executeListWorkflows(ctx, execCtx, toolCall.Arguments)

	// Music Tools
	case ToolMusicPlay, ToolMusicPlaylist, ToolMusicQueue, ToolMusicSkip,
		ToolMusicPause, ToolMusicResume, ToolMusicStop, ToolMusicVolume, ToolMusicRadio:
		return e.executeMusicTool(ctx, execCtx, toolCall)

	default:
		e.logger.Warn("Unknown tool", zap.String("tool", toolCall.Name))
		return &ToolResult{
			Success: false,
			Error:   fmt.Sprintf("Unknown tool: %s", toolCall.Name),
		}
	}
}

// executeMusicTool executes a music-related tool
func (e *Executor) executeMusicTool(ctx context.Context, execCtx *ExecutionContext, toolCall adapter.ToolCall) *ToolResult {
	if e.musicExecutor == nil {
		return &ToolResult{
			Success: false,
			Error:   "Music executor not initialized",
		}
	}

	// Parse arguments - Arguments is already a map[string]interface{}
	args := make(map[string]interface{})
	if toolCall.Arguments != nil {
		args = toolCall.Arguments
	}

	return e.musicExecutor.ExecuteMusicTool(ctx, execCtx, toolCall.Name, args)
}

