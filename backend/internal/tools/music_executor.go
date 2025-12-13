package tools

import (
	"context"
	"fmt"

	"ezra-clone/backend/internal/adapter"
	"ezra-clone/backend/internal/tools/music"

	"github.com/bwmarrin/discordgo"
	"go.uber.org/zap"
)

// MusicExecutor handles music-related tool execution
type MusicExecutor struct {
	manager   *music.MusicManager
	session   *discordgo.Session
	logger    *zap.Logger
	llmAdapter *adapter.LLMAdapter
}

// NewMusicExecutor creates a new music executor
func NewMusicExecutor(session *discordgo.Session, logger *zap.Logger, llmAdapter *adapter.LLMAdapter) *MusicExecutor {
	// Check dependencies
	if err := music.CheckDependencies(); err != nil {
		logger.Warn("Music dependencies not found", zap.Error(err))
	}

	if llmAdapter != nil {
		logger.Info("LLM adapter set for music playlist generation")
	} else {
		logger.Warn("LLM adapter not set - playlist generation will be limited")
	}

	return &MusicExecutor{
		manager:    music.NewMusicManager(llmAdapter, logger),
		session:   session,
		logger:    logger,
		llmAdapter: llmAdapter,
	}
}

// SetSession updates the Discord session
func (m *MusicExecutor) SetSession(session *discordgo.Session) {
	m.session = session
}

// ExecuteMusicTool executes a music tool call
func (m *MusicExecutor) ExecuteMusicTool(ctx context.Context, execCtx *ExecutionContext, toolName string, args map[string]interface{}) *ToolResult {
	if m.session == nil {
		return &ToolResult{
			Success: false,
			Error:   "Discord session not available",
		}
	}

	// Extract guild ID from context or args
	var guildID string
	if gid, ok := args["guild_id"].(string); ok && gid != "" {
		guildID = gid
	} else if execCtx.ChannelID != "" {
		// Resolve guild ID from channel
		channel, err := m.session.Channel(execCtx.ChannelID)
		if err == nil && channel != nil {
			guildID = channel.GuildID
		}
	}

	if guildID == "" {
		return &ToolResult{
			Success: false,
			Error:   "Could not determine guild ID. Please specify guild_id or use a guild channel.",
		}
	}

	// Get or create bot for this guild
	bot := m.manager.GetBot(guildID, m.session)

	switch toolName {
	case ToolMusicPlay:
		return m.handlePlay(ctx, execCtx, bot, args)
	case ToolMusicPlaylist:
		return m.handlePlaylist(ctx, execCtx, bot, args)
	case ToolMusicQueue:
		return m.handleQueue(ctx, execCtx, bot, args)
	case ToolMusicSkip:
		return m.handleSkip(ctx, execCtx, bot, args)
	case ToolMusicPause:
		return m.handlePause(ctx, execCtx, bot, args)
	case ToolMusicResume:
		return m.handleResume(ctx, execCtx, bot, args)
	case ToolMusicStop:
		return m.handleStop(ctx, execCtx, bot, args)
	case ToolMusicVolume:
		return m.handleVolume(ctx, execCtx, bot, args)
	case ToolMusicRadio:
		return m.handleRadio(ctx, execCtx, bot, args)
	case ToolMusicDisconnect:
		return m.handleDisconnect(ctx, execCtx, bot, args)
	default:
		return &ToolResult{
			Success: false,
			Error:   fmt.Sprintf("Unknown music tool: %s", toolName),
		}
	}
}
