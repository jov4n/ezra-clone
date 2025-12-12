package errors

import (
	"fmt"
	"time"
)

// ErrorType represents the category of error
type ErrorType string

const (
	// ErrorTypeDiscord represents Discord-related errors
	ErrorTypeDiscord ErrorType = "discord"
	// ErrorTypeMusic represents music/source-related errors
	ErrorTypeMusic ErrorType = "music"
	// ErrorTypeAgent represents agent/LLM-related errors
	ErrorTypeAgent ErrorType = "agent"
	// ErrorTypeGraph represents graph database errors
	ErrorTypeGraph ErrorType = "graph"
	// ErrorTypeTool represents tool execution errors
	ErrorTypeTool ErrorType = "tool"
	// ErrorTypeConfig represents configuration errors
	ErrorTypeConfig ErrorType = "config"
	// ErrorTypeContext represents context cancellation/timeout errors
	ErrorTypeContext ErrorType = "context"
)

// BaseError is the base error type with common fields
type BaseError struct {
	Type      ErrorType
	Message   string
	Timestamp time.Time
	Err       error // Wrapped error
}

// Error implements the error interface
func (e *BaseError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Type, e.Message, e.Err)
	}
	return fmt.Sprintf("[%s] %s", e.Type, e.Message)
}

// Unwrap returns the wrapped error for error unwrapping
func (e *BaseError) Unwrap() error {
	return e.Err
}

// NewBaseError creates a new base error
func NewBaseError(errType ErrorType, message string, err error) *BaseError {
	return &BaseError{
		Type:      errType,
		Message:   message,
		Timestamp: time.Now(),
		Err:       err,
	}
}

// Discord Errors

// ErrDiscordSessionUnavailable is returned when Discord session is not available
var ErrDiscordSessionUnavailable = NewBaseError(ErrorTypeDiscord, "Discord session not available", nil)

// ErrDiscordChannelNotFound is returned when a Discord channel cannot be found
type ErrDiscordChannelNotFound struct {
	*BaseError
	ChannelID string
}

func NewDiscordChannelNotFound(channelID string) *ErrDiscordChannelNotFound {
	return &ErrDiscordChannelNotFound{
		BaseError: NewBaseError(ErrorTypeDiscord, fmt.Sprintf("channel not found: %s", channelID), nil),
		ChannelID: channelID,
	}
}

// ErrDiscordUserNotFound is returned when a Discord user cannot be found
type ErrDiscordUserNotFound struct {
	*BaseError
	UserID string
}

func NewDiscordUserNotFound(userID string) *ErrDiscordUserNotFound {
	return &ErrDiscordUserNotFound{
		BaseError: NewBaseError(ErrorTypeDiscord, fmt.Sprintf("user not found: %s", userID), nil),
		UserID:    userID,
	}
}

// ErrDiscordGuildNotFound is returned when a Discord guild cannot be found
type ErrDiscordGuildNotFound struct {
	*BaseError
	GuildID string
}

func NewDiscordGuildNotFound(guildID string) *ErrDiscordGuildNotFound {
	return &ErrDiscordGuildNotFound{
		BaseError: NewBaseError(ErrorTypeDiscord, fmt.Sprintf("guild not found: %s", guildID), nil),
		GuildID:   guildID,
	}
}

// ErrDiscordMessageSendFailed is returned when sending a Discord message fails
type ErrDiscordMessageSendFailed struct {
	*BaseError
	ChannelID string
}

func NewDiscordMessageSendFailed(channelID string, err error) *ErrDiscordMessageSendFailed {
	return &ErrDiscordMessageSendFailed{
		BaseError: NewBaseError(ErrorTypeDiscord, "failed to send message", err),
		ChannelID: channelID,
	}
}

// Music Errors (these wrap the existing errors from sources package)

// ErrMusicSongNotFound wraps sources.ErrSongNotFound
type ErrMusicSongNotFound struct {
	*BaseError
	Query string
}

func NewMusicSongNotFound(query string) *ErrMusicSongNotFound {
	return &ErrMusicSongNotFound{
		BaseError: NewBaseError(ErrorTypeMusic, fmt.Sprintf("song not found: %s", query), nil),
		Query:     query,
	}
}

// ErrMusicPlaylistEmpty wraps sources.ErrPlaylistEmpty
type ErrMusicPlaylistEmpty struct {
	*BaseError
	URL string
}

func NewMusicPlaylistEmpty(url string) *ErrMusicPlaylistEmpty {
	return &ErrMusicPlaylistEmpty{
		BaseError: NewBaseError(ErrorTypeMusic, fmt.Sprintf("playlist is empty: %s", url), nil),
		URL:       url,
	}
}

// ErrMusicFetchFailed wraps sources.ErrFetchFailed
type ErrMusicFetchFailed struct {
	*BaseError
	Source string
	URL    string
}

func NewMusicFetchFailed(source, url string, err error) *ErrMusicFetchFailed {
	return &ErrMusicFetchFailed{
		BaseError: NewBaseError(ErrorTypeMusic, fmt.Sprintf("failed to fetch from %s: %s", source, url), err),
		Source:    source,
		URL:       url,
	}
}

// ErrMusicTimeout wraps sources.ErrTimeout
type ErrMusicTimeout struct {
	*BaseError
	Operation string
	Timeout   time.Duration
}

func NewMusicTimeout(operation string, timeout time.Duration) *ErrMusicTimeout {
	return &ErrMusicTimeout{
		BaseError: NewBaseError(ErrorTypeMusic, fmt.Sprintf("operation timed out: %s", operation), nil),
		Operation: operation,
		Timeout:   timeout,
	}
}

// Agent Errors

// ErrAgentIgnored is returned when the agent chooses to ignore a message
var ErrAgentIgnored = NewBaseError(ErrorTypeAgent, "agent ignored message", nil)

// ErrAgentLLMFailed is returned when LLM request fails
type ErrAgentLLMFailed struct {
	*BaseError
	Model     string
	Attempts  int
	Retryable bool
}

func NewAgentLLMFailed(model string, attempts int, retryable bool, err error) *ErrAgentLLMFailed {
	return &ErrAgentLLMFailed{
		BaseError: NewBaseError(ErrorTypeAgent, fmt.Sprintf("LLM request failed after %d attempts", attempts), err),
		Model:     model,
		Attempts:  attempts,
		Retryable: retryable,
	}
}

// ErrAgentNoResponse is returned when LLM returns no response
var ErrAgentNoResponse = NewBaseError(ErrorTypeAgent, "no response from LLM", nil)

// ErrAgentInvalidToolCall is returned when a tool call is invalid
type ErrAgentInvalidToolCall struct {
	*BaseError
	ToolName string
	Reason   string
}

func NewAgentInvalidToolCall(toolName, reason string) *ErrAgentInvalidToolCall {
	return &ErrAgentInvalidToolCall{
		BaseError: NewBaseError(ErrorTypeAgent, fmt.Sprintf("invalid tool call: %s", toolName), nil),
		ToolName: toolName,
		Reason:   reason,
	}
}

// Graph Errors

// ErrGraphConnectionFailed is returned when Neo4j connection fails
type ErrGraphConnectionFailed struct {
	*BaseError
	URI string
}

func NewGraphConnectionFailed(uri string, err error) *ErrGraphConnectionFailed {
	return &ErrGraphConnectionFailed{
		BaseError: NewBaseError(ErrorTypeGraph, fmt.Sprintf("failed to connect to Neo4j: %s", uri), err),
		URI:       uri,
	}
}

// ErrGraphQueryFailed is returned when a graph query fails
type ErrGraphQueryFailed struct {
	*BaseError
	Query string
}

func NewGraphQueryFailed(query string, err error) *ErrGraphQueryFailed {
	return &ErrGraphQueryFailed{
		BaseError: NewBaseError(ErrorTypeGraph, fmt.Sprintf("query failed: %s", query), err),
		Query:     query,
	}
}

// ErrGraphUserNotFound is returned when a user is not found in the graph
type ErrGraphUserNotFound struct {
	*BaseError
	UserID string
}

func NewGraphUserNotFound(userID string) *ErrGraphUserNotFound {
	return &ErrGraphUserNotFound{
		BaseError: NewBaseError(ErrorTypeGraph, fmt.Sprintf("user not found: %s", userID), nil),
		UserID:    userID,
	}
}

// Tool Errors

// ErrToolExecutionFailed is returned when tool execution fails
type ErrToolExecutionFailed struct {
	*BaseError
	ToolName string
	Reason   string
}

func NewToolExecutionFailed(toolName, reason string, err error) *ErrToolExecutionFailed {
	return &ErrToolExecutionFailed{
		BaseError: NewBaseError(ErrorTypeTool, fmt.Sprintf("tool execution failed: %s", toolName), err),
		ToolName:  toolName,
		Reason:    reason,
	}
}

// ErrToolNotFound is returned when a requested tool is not found
type ErrToolNotFound struct {
	*BaseError
	ToolName string
}

func NewToolNotFound(toolName string) *ErrToolNotFound {
	return &ErrToolNotFound{
		BaseError: NewBaseError(ErrorTypeTool, fmt.Sprintf("tool not found: %s", toolName), nil),
		ToolName:  toolName,
	}
}

// Context Errors

// ErrContextCancelled is returned when context is cancelled
type ErrContextCancelled struct {
	*BaseError
	Operation string
}

func NewContextCancelled(operation string, err error) *ErrContextCancelled {
	return &ErrContextCancelled{
		BaseError: NewBaseError(ErrorTypeContext, fmt.Sprintf("context cancelled: %s", operation), err),
		Operation: operation,
	}
}

// ErrContextTimeout is returned when context times out
type ErrContextTimeout struct {
	*BaseError
	Operation string
	Timeout   time.Duration
}

func NewContextTimeout(operation string, timeout time.Duration) *ErrContextTimeout {
	return &ErrContextTimeout{
		BaseError: NewBaseError(ErrorTypeContext, fmt.Sprintf("context timeout: %s (timeout: %v)", operation, timeout), nil),
		Operation: operation,
		Timeout:   timeout,
	}
}

// Config Errors

// ErrConfigValidationFailed is returned when configuration validation fails
type ErrConfigValidationFailed struct {
	*BaseError
	Field   string
	Reason  string
}

func NewConfigValidationFailed(field, reason string) *ErrConfigValidationFailed {
	return &ErrConfigValidationFailed{
		BaseError: NewBaseError(ErrorTypeConfig, fmt.Sprintf("config validation failed: %s - %s", field, reason), nil),
		Field:    field,
		Reason:   reason,
	}
}

// ErrConfigMissingRequired is returned when a required config value is missing
type ErrConfigMissingRequired struct {
	*BaseError
	Field string
}

func NewConfigMissingRequired(field string) *ErrConfigMissingRequired {
	return &ErrConfigMissingRequired{
		BaseError: NewBaseError(ErrorTypeConfig, fmt.Sprintf("missing required config: %s", field), nil),
		Field:     field,
	}
}

// Helper functions

// IsErrorType checks if an error is of a specific type
func IsErrorType(err error, errType ErrorType) bool {
	if baseErr, ok := err.(*BaseError); ok {
		return baseErr.Type == errType
	}
	// Check wrapped errors
	if baseErr, ok := err.(interface{ Unwrap() error }); ok {
		return IsErrorType(baseErr.Unwrap(), errType)
	}
	return false
}

// IsRetryable checks if an error is retryable
func IsRetryable(err error) bool {
	// Context errors are not retryable
	if IsErrorType(err, ErrorTypeContext) {
		return false
	}
	// Check for specific retryable error types
	if llmErr, ok := err.(*ErrAgentLLMFailed); ok {
		return llmErr.Retryable
	}
	// Timeout errors might be retryable
	if IsErrorType(err, ErrorTypeMusic) {
		// Music timeouts might be retryable
		return true
	}
	// Graph connection errors are retryable
	if IsErrorType(err, ErrorTypeGraph) {
		return true
	}
	return false
}

