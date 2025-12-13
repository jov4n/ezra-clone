package voice

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// ResponseQueue manages queued responses for voice interactions
type ResponseQueue struct {
	items []*QueuedResponse
	mu    sync.Mutex
}

// QueuedResponse represents a queued voice response
type QueuedResponse struct {
	UserID    string
	Text      string
	Priority  int // Higher priority = respond sooner
	Timestamp time.Time
}

// NewResponseQueue creates a new response queue
func NewResponseQueue() *ResponseQueue {
	return &ResponseQueue{
		items: make([]*QueuedResponse, 0),
	}
}

// Enqueue adds a response to the queue
func (rq *ResponseQueue) Enqueue(response *QueuedResponse) {
	rq.mu.Lock()
	defer rq.mu.Unlock()

	rq.items = append(rq.items, response)
	// Sort by priority (higher first), then by timestamp
	// Simple insertion sort for small queues
	for i := len(rq.items) - 1; i > 0; i-- {
		if rq.items[i].Priority > rq.items[i-1].Priority ||
			(rq.items[i].Priority == rq.items[i-1].Priority && rq.items[i].Timestamp.Before(rq.items[i-1].Timestamp)) {
			rq.items[i], rq.items[i-1] = rq.items[i-1], rq.items[i]
		} else {
			break
		}
	}
}

// Dequeue removes and returns the highest priority response
func (rq *ResponseQueue) Dequeue() *QueuedResponse {
	rq.mu.Lock()
	defer rq.mu.Unlock()

	if len(rq.items) == 0 {
		return nil
	}

	item := rq.items[0]
	rq.items = rq.items[1:]
	return item
}

// Peek returns the highest priority response without removing it
func (rq *ResponseQueue) Peek() *QueuedResponse {
	rq.mu.Lock()
	defer rq.mu.Unlock()

	if len(rq.items) == 0 {
		return nil
	}

	return rq.items[0]
}

// Clear removes all items from the queue
func (rq *ResponseQueue) Clear() {
	rq.mu.Lock()
	defer rq.mu.Unlock()

	rq.items = rq.items[:0]
}

// Length returns the number of items in the queue
func (rq *ResponseQueue) Length() int {
	rq.mu.Lock()
	defer rq.mu.Unlock()

	return len(rq.items)
}

// VoiceOrchestrator coordinates voice interactions and responses
type VoiceOrchestrator struct {
	responseQueue *ResponseQueue
	vad           *VoiceActivityDetector
	logger        *zap.Logger
	mu            sync.Mutex
	isResponding  bool
	lastResponse  time.Time
	config        *OrchestratorConfig
}

// OrchestratorConfig holds configuration for the orchestrator
type OrchestratorConfig struct {
	MinPauseBeforeResponse time.Duration // Minimum pause before responding
	MaxQueueSize           int           // Maximum queue size
	ResponseCooldown       time.Duration // Cooldown between responses
}

// DefaultOrchestratorConfig returns default orchestrator configuration
func DefaultOrchestratorConfig() *OrchestratorConfig {
	return &OrchestratorConfig{
		MinPauseBeforeResponse: 500 * time.Millisecond,
		MaxQueueSize:           10,
		ResponseCooldown:       2 * time.Second,
	}
}

// NewVoiceOrchestrator creates a new voice orchestrator
func NewVoiceOrchestrator(vad *VoiceActivityDetector, logger *zap.Logger) *VoiceOrchestrator {
	return &VoiceOrchestrator{
		responseQueue: NewResponseQueue(),
		vad:           vad,
		logger:        logger,
		config:        DefaultOrchestratorConfig(),
	}
}

// ShouldRespond determines if the bot should respond now
func (vo *VoiceOrchestrator) ShouldRespond(ctx context.Context) bool {
	vo.mu.Lock()
	defer vo.mu.Unlock()

	// Don't respond if already responding
	if vo.isResponding {
		return false
	}

	// Check cooldown
	if time.Since(vo.lastResponse) < vo.config.ResponseCooldown {
		return false
	}

	// Check if there's a response in queue
	if vo.responseQueue.Length() == 0 {
		return false
	}

	// Check if any users are currently speaking
	activeUsers := vo.vad.GetActiveUsers(2 * time.Second)
	if len(activeUsers) > 0 {
		// Wait for pause
		return false
	}

	return true
}

// QueueResponse adds a response to the queue
func (vo *VoiceOrchestrator) QueueResponse(userID, text string, priority int) error {
	vo.mu.Lock()
	defer vo.mu.Unlock()

	if vo.responseQueue.Length() >= vo.config.MaxQueueSize {
		return fmt.Errorf("response queue is full")
	}

	response := &QueuedResponse{
		UserID:    userID,
		Text:      text,
		Priority:  priority,
		Timestamp: time.Now(),
	}

	vo.responseQueue.Enqueue(response)
	vo.logger.Debug("Queued voice response", zap.String("user_id", userID), zap.Int("priority", priority))

	return nil
}

// GetNextResponse gets the next response to process
func (vo *VoiceOrchestrator) GetNextResponse() *QueuedResponse {
	return vo.responseQueue.Dequeue()
}

// SetResponding marks the orchestrator as currently responding
func (vo *VoiceOrchestrator) SetResponding(responding bool) {
	vo.mu.Lock()
	defer vo.mu.Unlock()

	vo.isResponding = responding
	if responding {
		vo.lastResponse = time.Now()
	}
}

// IsResponding returns whether the bot is currently responding
func (vo *VoiceOrchestrator) IsResponding() bool {
	vo.mu.Lock()
	defer vo.mu.Unlock()

	return vo.isResponding
}

// ClearQueue clears the response queue
func (vo *VoiceOrchestrator) ClearQueue() {
	vo.responseQueue.Clear()
	vo.logger.Debug("Cleared voice response queue")
}

// CalculatePriority calculates response priority based on context
func (vo *VoiceOrchestrator) CalculatePriority(text string, isQuestion bool, mentionsBot bool) int {
	priority := 1 // Default priority

	if mentionsBot {
		priority += 3 // High priority if bot is mentioned
	}

	if isQuestion {
		priority += 2 // Higher priority for questions
	}

	// Check for urgent keywords
	urgentKeywords := []string{"help", "error", "stop", "cancel", "urgent"}
	textLower := fmt.Sprintf("%v", text) // Convert to lowercase
	for _, keyword := range urgentKeywords {
		if contains(textLower, keyword) {
			priority += 1
			break
		}
	}

	return priority
}

// contains checks if a string contains a substring (case-insensitive)
func contains(s, substr string) bool {
	// Simple case-insensitive contains check
	sLower := ""
	substrLower := ""
	for _, r := range s {
		sLower += string(r | 32) // Convert to lowercase
	}
	for _, r := range substr {
		substrLower += string(r | 32)
	}
	
	for i := 0; i <= len(sLower)-len(substrLower); i++ {
		if sLower[i:i+len(substrLower)] == substrLower {
			return true
		}
	}
	return false
}

