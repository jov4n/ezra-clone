package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/bwmarrin/discordgo"
	"ezra-clone/backend/internal/adapter"
	"ezra-clone/backend/pkg/config"
	"go.uber.org/zap"
)

// MimicBackgroundTask manages automatic posting when mimic mode is active
type MimicBackgroundTask struct {
	executor      *Executor
	llm           *adapter.LLMAdapter
	discordSession *discordgo.Session
	config        *config.Config
	logger        *zap.Logger
	stopChan      chan struct{}
	running       bool
	agentID       string
}

// NewMimicBackgroundTask creates a new background task manager
func NewMimicBackgroundTask(executor *Executor, llm *adapter.LLMAdapter, discordSession *discordgo.Session, cfg *config.Config, logger *zap.Logger) *MimicBackgroundTask {
	return &MimicBackgroundTask{
		executor:      executor,
		llm:           llm,
		discordSession: discordSession,
		config:        cfg,
		logger:        logger,
		stopChan:      make(chan struct{}),
		running:       false,
	}
}

// Start begins the background task for an agent
func (m *MimicBackgroundTask) Start(agentID string) {
	if m.running {
		m.logger.Warn("Background task already running", zap.String("agent_id", agentID))
		return
	}

	m.agentID = agentID
	m.running = true
	m.stopChan = make(chan struct{})

	// Register message handler for responding to posts
	m.discordSession.AddHandler(m.handleChannelMessage)

	m.logger.Info("Mimic background task started",
		zap.String("agent_id", agentID),
		zap.String("mimic_channel_id", m.config.MimicChannelID),
	)
}

// Stop stops the background task
func (m *MimicBackgroundTask) Stop() {
	if !m.running {
		return
	}

	m.running = false
	if m.stopChan != nil {
		close(m.stopChan)
	}

	// Note: Discord handlers can't be easily removed, but we check m.running in handleChannelMessage
	// so it will effectively stop responding

	m.logger.Info("Mimic background task stopped",
		zap.String("agent_id", m.agentID),
	)
}

// handleChannelMessage handles messages in the mimic channel and intelligently responds
func (m *MimicBackgroundTask) handleChannelMessage(s *discordgo.Session, msg *discordgo.MessageCreate) {
	// Log ALL messages for debugging (to see if handler is being called)
	m.logger.Debug("Mimic handler checking message",
		zap.String("channel_id", msg.ChannelID),
		zap.String("expected_channel_id", m.config.MimicChannelID),
		zap.String("author", msg.Author.Username),
		zap.Bool("is_match", msg.ChannelID == m.config.MimicChannelID),
		zap.Bool("running", m.running),
	)

	// Log messages in the target channel at Info level
	if msg.ChannelID == m.config.MimicChannelID {
		m.logger.Info("Mimic handler received message in target channel",
			zap.String("channel_id", msg.ChannelID),
			zap.String("author", msg.Author.Username),
			zap.String("content", msg.Content),
			zap.Bool("running", m.running),
		)
	}

	// Only process messages in the configured mimic channel
	if msg.ChannelID != m.config.MimicChannelID {
		return
	}

	// Ignore if not running
	if !m.running {
		return
	}

	// Ignore messages from the bot itself
	if msg.Author.ID == s.State.User.ID {
		return
	}

	// Ignore bot messages
	if msg.Author.Bot {
		return
	}

	// Ignore empty messages
	if strings.TrimSpace(msg.Content) == "" {
		return
	}

	// Get mimic state
	mimicState := m.executor.GetMimicState(m.agentID)
	if mimicState == nil || !mimicState.Active || mimicState.MimicProfile == nil {
		m.logger.Info("Mimic state not active, skipping",
			zap.String("agent_id", m.agentID),
			zap.Bool("state_exists", mimicState != nil),
			zap.Bool("state_active", mimicState != nil && mimicState.Active),
			zap.Bool("has_profile", mimicState != nil && mimicState.MimicProfile != nil),
		)
		return
	}

	profile := mimicState.MimicProfile
	ctx := context.Background()

	// Check if this is a direct reply to the bot
	isDirectReply := false
	if msg.MessageReference != nil && msg.MessageReference.MessageID != "" {
		// Fetch the referenced message to check if it's from the bot
		referencedMsg, err := s.ChannelMessage(msg.ChannelID, msg.MessageReference.MessageID)
		if err == nil && referencedMsg != nil && referencedMsg.Author != nil {
			if referencedMsg.Author.ID == s.State.User.ID {
				isDirectReply = true
				m.logger.Info("Detected direct reply to bot",
					zap.String("agent_id", m.agentID),
					zap.String("channel_id", msg.ChannelID),
					zap.String("responding_to_user", msg.Author.Username),
				)
			}
		}
	}

	// Always respond if directly replied to, otherwise let LM decide
	if !isDirectReply {
		// Use LM to decide if we should respond
		shouldRespond, err := m.shouldRespondToMessage(ctx, profile, msg.Content, msg.ChannelID)
		if err != nil {
			m.logger.Warn("Failed to determine if should respond",
				zap.Error(err),
			)
			// On error, don't respond to avoid spam
			return
		}
		if !shouldRespond {
			return
		}
	}

	m.logger.Info("Mimic responding to message",
		zap.String("agent_id", m.agentID),
		zap.String("channel_id", msg.ChannelID),
		zap.String("responding_to_user", msg.Author.Username),
		zap.Bool("is_direct_reply", isDirectReply),
	)

	// Get recent channel context for intelligent responses
	channelContext, err := m.getChannelContext(ctx, msg.ChannelID, 10)
	if err != nil {
		m.logger.Warn("Failed to get channel context, continuing without it",
			zap.Error(err),
		)
		channelContext = ""
	}

	// Generate response in user's style with channel context
	response, err := m.generateResponseToMessage(ctx, profile, msg.Content, msg.Author.Username, channelContext)
	if err != nil {
		m.logger.Error("Failed to generate response",
			zap.Error(err),
		)
		return
	}

	// Post response
	_, err = s.ChannelMessageSend(msg.ChannelID, response)
	if err != nil {
		m.logger.Error("Failed to send response",
			zap.Error(err),
		)
		return
	}

	m.logger.Info("Mimic response sent",
		zap.String("agent_id", m.agentID),
		zap.String("channel_id", msg.ChannelID),
	)
}

// shouldRespondToMessage uses the LM to decide if we should respond to a message
func (m *MimicBackgroundTask) shouldRespondToMessage(ctx context.Context, profile *PersonalityProfile, messageContent, channelID string) (bool, error) {
	// Get recent channel context
	channelContext, err := m.getChannelContext(ctx, channelID, 10)
	if err != nil {
		// If we can't get context, still try to decide
		channelContext = ""
	}

	// Create the ignore_message tool
	ignoreTool := adapter.Tool{
		Type: "function",
		Function: adapter.FunctionDefinition{
			Name:        "ignore_message",
			Description: "ONLY call this tool if the message is completely irrelevant, spam, or something you would absolutely never respond to. For normal conversation, questions, greetings, or anything you might naturally engage with, DO NOT call this tool.",
			Parameters: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
				"required":   []string{},
			},
		},
	}

	contextSection := ""
	if channelContext != "" {
		contextSection = fmt.Sprintf("\n\nRecent channel context:\n%s", channelContext)
	}

	// Use the personality profile's perspective - you ARE this person, not a bot
	prompt := fmt.Sprintf(`Someone posted this message in the Discord channel:

"%s"%s

Think about whether YOU (as yourself) would naturally respond to this. Consider:
- Would you respond to a greeting or casual conversation?
- Would you respond to a question (even if not directly at you)?
- Would you respond if someone mentioned you or your name?
- Would you respond if it's about something you're interested in?
- Would you respond if it's part of a conversation you're following?

ONLY call ignore_message if:
- The message is spam or completely irrelevant to you
- It's a bot command or system message
- It's something you would absolutely never respond to

For normal conversation, questions, or anything you might naturally engage with, DO NOT call ignore_message - just don't call any tool and we'll generate a response.`,
		messageContent,
		contextSection,
	)

	// Use the style prompt as system prompt - this makes the LLM think AS the person, not as a bot
	// The style prompt already says "You ARE [username]" and includes all their personality traits
	response, err := m.llm.Generate(ctx, profile.StylePrompt, prompt, []adapter.Tool{ignoreTool})
	if err != nil {
		return false, err
	}

	// Check if the ignore_message tool was called
	for _, toolCall := range response.ToolCalls {
		if toolCall.Name == "ignore_message" {
			m.logger.Debug("LM decided to ignore message",
				zap.String("agent_id", m.agentID),
				zap.String("channel_id", channelID),
				zap.String("message_preview", truncateString(messageContent, 50)),
			)
			return false, nil
		}
	}

	// No ignore tool was called, so we should respond
	m.logger.Debug("LM decided to respond to message",
		zap.String("agent_id", m.agentID),
		zap.String("channel_id", channelID),
		zap.String("message_preview", truncateString(messageContent, 50)),
	)
	return true, nil
}

// getChannelContext retrieves recent messages from the channel for context
func (m *MimicBackgroundTask) getChannelContext(ctx context.Context, channelID string, limit int) (string, error) {
	if m.executor.discordExecutor == nil {
		return "", fmt.Errorf("discord executor not available")
	}

	messages, err := m.executor.discordExecutor.ReadChannelHistory(ctx, channelID, limit, "")
	if err != nil {
		return "", err
	}

	if len(messages) == 0 {
		return "No recent messages in channel.", nil
	}

	var contextLines []string
	for _, msg := range messages {
		// Skip very long messages
		content := msg.Content
		if len(content) > 200 {
			content = content[:200] + "..."
		}
		contextLines = append(contextLines, fmt.Sprintf("- %s: %s", msg.Author, content))
	}

	return strings.Join(contextLines, "\n"), nil
}


// generateResponseToMessage generates a response to a message in the user's style with channel context
func (m *MimicBackgroundTask) generateResponseToMessage(ctx context.Context, profile *PersonalityProfile, originalMessage, authorUsername, channelContext string) (string, error) {
	contextSection := ""
	if channelContext != "" {
		contextSection = fmt.Sprintf(`

Recent channel context (for understanding the conversation):
%s

`, channelContext)
	}

	// Write from the perspective of BEING this person, not mimicking them
	prompt := fmt.Sprintf(`Someone posted this message in the Discord channel:

"%s" (by %s)
%s
Write a short response (1-2 sentences max) that YOU would naturally post. This could be:
- A reaction or comment
- Agreement or disagreement
- A question
- A related thought
- A casual response
- Something relevant to the ongoing conversation

Write naturally as yourself - be authentic to your own communication style and respond as you normally would.`,
		originalMessage,
		authorUsername,
		contextSection,
	)

	// Use the style prompt as system prompt and the response request as user message
	response, err := m.llm.Generate(ctx, profile.StylePrompt, prompt, []adapter.Tool{})
	if err != nil {
		return "", err
	}

	message := response.Content
	if message == "" {
		// Fallback response based on common phrases
		if len(profile.CommonPhrases) > 0 {
			message = profile.CommonPhrases[0]
		} else {
			message = "interesting"
		}
	}

	return message, nil
}

// Helper functions
func joinStrings(strs []string, sep string) string {
	if len(strs) == 0 {
		return "none"
	}
	result := ""
	for i, s := range strs {
		if i > 0 {
			result += sep
		}
		result += s
	}
	return result
}

func cleanQuery(query string) string {
	// Remove quotes, newlines, and extra spaces
	query = strings.TrimSpace(query)
	query = strings.Trim(query, `"'`)
	query = strings.ReplaceAll(query, "\n", " ")
	query = strings.ReplaceAll(query, "\r", " ")
	
	// Remove extra spaces
	words := strings.Fields(query)
	return strings.Join(words, " ")
}

