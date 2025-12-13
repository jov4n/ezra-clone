package tools

import (
	"context"
	"fmt"
	"time"

	"github.com/bwmarrin/discordgo"
	"go.uber.org/zap"
)

// SystemExecutor handles system control tool execution
type SystemExecutor struct {
	session     *discordgo.Session
	logger      *zap.Logger
	shutdownFunc func() // Function to trigger shutdown
}

// NewSystemExecutor creates a new system executor
func NewSystemExecutor(session *discordgo.Session, logger *zap.Logger, shutdownFunc func()) *SystemExecutor {
	return &SystemExecutor{
		session:      session,
		logger:       logger,
		shutdownFunc: shutdownFunc,
	}
}

// SetSession updates the Discord session
func (s *SystemExecutor) SetSession(session *discordgo.Session) {
	s.session = session
}

// ExecuteSystemTool executes a system tool call
func (s *SystemExecutor) ExecuteSystemTool(ctx context.Context, execCtx *ExecutionContext, toolName string, args map[string]interface{}) *ToolResult {
	if s.session == nil {
		return &ToolResult{
			Success: false,
			Error:   "Discord session not available",
		}
	}

	switch toolName {
	case ToolBotShutdown:
		return s.handleShutdown(ctx, execCtx, args)
	default:
		return &ToolResult{
			Success: false,
			Error:   fmt.Sprintf("Unknown system tool: %s", toolName),
		}
	}
}

func (s *SystemExecutor) handleShutdown(ctx context.Context, execCtx *ExecutionContext, args map[string]interface{}) *ToolResult {
	// Check if user is admin
	if !s.isAdmin(execCtx) {
		return &ToolResult{
			Success: false,
			Error:   "Unauthorized: Only administrators can shutdown the bot",
		}
	}

	// Get optional message
	message, _ := args["message"].(string)
	if message == "" {
		message = "Good night! 🌙"
	}

	// Send goodbye message
	if execCtx.ChannelID != "" {
		_, err := s.session.ChannelMessageSend(execCtx.ChannelID, message)
		if err != nil {
			s.logger.Warn("Failed to send shutdown message", zap.Error(err))
		}
	}

	// Trigger shutdown after a short delay to allow message to be sent
	go func() {
		time.Sleep(1 * time.Second)
		if s.shutdownFunc != nil {
			s.shutdownFunc()
		}
	}()

	return &ToolResult{
		Success: true,
		Message: "Shutting down...",
	}
}

// isAdmin checks if the user is an administrator
func (s *SystemExecutor) isAdmin(execCtx *ExecutionContext) bool {
	// Check if user is the hardcoded admin user ID
	if execCtx.UserID == AdminUserID {
		return true
	}

	// If it's a DM, only the admin user ID can shutdown
	if execCtx.ChannelID != "" {
		channel, err := s.session.Channel(execCtx.ChannelID)
		if err == nil && channel != nil {
			// If it's a DM, only allow the admin user ID
			if channel.Type == discordgo.ChannelTypeDM || channel.Type == discordgo.ChannelTypeGroupDM {
				return execCtx.UserID == AdminUserID
			}

			// For guild channels, check if user has administrator permission
			if channel.GuildID != "" {
				member, err := s.session.GuildMember(channel.GuildID, execCtx.UserID)
				if err != nil {
					s.logger.Warn("Failed to get guild member for admin check", zap.Error(err))
					return false
				}

				// Get guild to check permissions
				guild, err := s.session.Guild(channel.GuildID)
				if err != nil {
					s.logger.Warn("Failed to get guild for admin check", zap.Error(err))
					return false
				}

				// Check if user is the guild owner
				if guild.OwnerID == execCtx.UserID {
					return true
				}

				// Check if user has administrator permission
				permissions, err := s.session.UserChannelPermissions(execCtx.UserID, execCtx.ChannelID)
				if err != nil {
					s.logger.Warn("Failed to get user permissions for admin check", zap.Error(err))
					return false
				}

				// Check for administrator permission
				if permissions&discordgo.PermissionAdministrator != 0 {
					return true
				}

				// Also check member roles for administrator permission
				for _, roleID := range member.Roles {
					role, err := s.session.State.Role(channel.GuildID, roleID)
					if err == nil && role != nil {
						if role.Permissions&discordgo.PermissionAdministrator != 0 {
							return true
						}
					}
				}
			}
		}
	}

	return false
}

