package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"ezra-clone/backend/internal/adapter"
	"ezra-clone/backend/internal/graph"
	"ezra-clone/backend/pkg/logger"
	"go.uber.org/zap"
)

// MemoryEvaluator automatically evaluates messages to determine if they should be saved to memory
type MemoryEvaluator struct {
	llm       *adapter.LLMAdapter
	graphRepo *graph.Repository
	logger    *zap.Logger
}

// MemoryDecision represents the evaluator's decision about what to save
type MemoryDecision struct {
	ShouldSave      bool     `json:"should_save"`
	MemoryType      string   `json:"memory_type"`      // "fact", "preference", "personal_info", "life_event", "none"
	Content         string   `json:"content"`           // What to save (rewritten clearly)
	Topics          []string `json:"topics"`             // Related topics
	Importance      int      `json:"importance"`       // 1-10 scale
	UpdatesExisting bool     `json:"updates_existing"` // Is this updating old info?
	ExistingID      string   `json:"existing_id"`     // ID of memory to update (if updating)
	Reasoning       string   `json:"reasoning"`        // Why this decision
}

// NewMemoryEvaluator creates a new memory evaluator
func NewMemoryEvaluator(llm *adapter.LLMAdapter, repo *graph.Repository) *MemoryEvaluator {
	return &MemoryEvaluator{
		llm:       llm,
		graphRepo: repo,
		logger:    logger.Get(),
	}
}

// EvaluateMessage analyzes a user message and determines if anything should be saved to memory
func (m *MemoryEvaluator) EvaluateMessage(ctx context.Context, agentID, userID, message string) (*MemoryDecision, error) {
	// Skip very short messages or obvious non-memory messages
	if len(strings.TrimSpace(message)) < 10 {
		return &MemoryDecision{ShouldSave: false}, nil
	}

	// Quick filter: skip greetings, questions, and commands
	if m.isNonMemoryMessage(message) {
		return &MemoryDecision{ShouldSave: false}, nil
	}

	// Get existing facts about this user for contradiction detection
	existingFacts, err := m.graphRepo.GetUserContext(ctx, userID)
	existingJSON := "[]"
	if err == nil && existingFacts != nil && len(existingFacts.Facts) > 0 {
		// Format existing facts for the LLM
		var factList []map[string]string
		for _, fact := range existingFacts.Facts {
			factList = append(factList, map[string]string{
				"id":      fact.ID,
				"content": fact.Content,
			})
		}
		if data, err := json.Marshal(factList); err == nil {
			existingJSON = string(data)
		}
	}

	// Build evaluation prompt
	prompt := fmt.Sprintf(`You are a memory evaluation system. Analyze this user message and decide if anything should be saved to memory.

User message: "%s"

Existing facts about this user:
%s

Respond with ONLY valid JSON (no markdown, no explanation):
{
  "should_save": true or false,
  "memory_type": "fact" or "preference" or "personal_info" or "life_event" or "none",
  "content": "The specific information to save, rewritten clearly and concisely",
  "topics": ["topic1", "topic2"],
  "importance": 1-10,
  "updates_existing": true or false,
  "existing_id": "fact id if updating, empty string otherwise",
  "reasoning": "Brief one-sentence explanation"
}

Guidelines:
- Save facts about the user: name, location, job, interests, opinions, relationships
- Save preferences: likes, dislikes, favorites, habits
- Save personal info: age, location, occupation, family
- Save life events: major changes, achievements, milestones
- DON'T save: greetings, questions to you, generic statements, temporary states
- Importance scale:
  * 8-10: Major life events, core identity, important relationships, critical preferences
  * 5-7: Preferences, interests, opinions, moderate importance facts
  * 1-4: Minor details, passing mentions, low importance
- CRITICAL: Check existing facts carefully for duplicates or conflicts:
  * If the new info is a duplicate (same meaning, different wording), set updates_existing=true and provide existing_id
  * If the new info contradicts existing info (e.g., "prefers English" vs "prefers Pig Latin"), set updates_existing=true and provide existing_id of the fact to replace
  * If the new info updates old info (e.g., age changes), set updates_existing=true and provide existing_id
- Only set should_save=true if importance >= 3
- Extract topics automatically (e.g., "I love pizza" -> topics: ["Food", "Preferences"])
- Rewrite content to be clear and standalone (e.g., "I love pizza" -> "User loves pizza")
- Be aggressive about detecting duplicates - if you see "User prefers X" and "User prefers to communicate in X", they are duplicates`, message, existingJSON)

	// Call LLM for evaluation
	response, err := m.llm.Generate(ctx, prompt, "Analyze and respond with JSON only. No markdown, no explanation, just the JSON object.", nil)
	if err != nil {
		m.logger.Warn("Memory evaluation LLM call failed",
			zap.String("user_id", userID),
			zap.Error(err),
		)
		return nil, fmt.Errorf("failed to evaluate memory: %w", err)
	}

	// Parse JSON response
	decision := &MemoryDecision{}
	
	// Try to extract JSON from response (handle markdown code blocks)
	jsonStr := response.Content
	jsonStr = strings.TrimSpace(jsonStr)
	
	// Remove markdown code blocks if present
	if strings.HasPrefix(jsonStr, "```") {
		lines := strings.Split(jsonStr, "\n")
		var jsonLines []string
		inCodeBlock := false
		for _, line := range lines {
			if strings.HasPrefix(line, "```") {
				inCodeBlock = !inCodeBlock
				continue
			}
			if inCodeBlock || (!strings.HasPrefix(jsonStr, "```") && len(jsonLines) == 0) {
				jsonLines = append(jsonLines, line)
			}
		}
		jsonStr = strings.Join(jsonLines, "\n")
	}
	
	// Find JSON object boundaries
	if start := strings.Index(jsonStr, "{"); start != -1 {
		if end := strings.LastIndex(jsonStr, "}"); end != -1 && end > start {
			jsonStr = jsonStr[start : end+1]
		}
	}

	if err := json.Unmarshal([]byte(jsonStr), decision); err != nil {
		m.logger.Warn("Failed to parse memory decision JSON",
			zap.String("user_id", userID),
			zap.String("response", response.Content),
			zap.Error(err),
		)
		return nil, fmt.Errorf("failed to parse memory decision: %w", err)
	}

	// Validate decision
	if decision.Importance < 3 {
		decision.ShouldSave = false
	}

	m.logger.Debug("Memory evaluation completed",
		zap.String("user_id", userID),
		zap.Bool("should_save", decision.ShouldSave),
		zap.String("memory_type", decision.MemoryType),
		zap.Int("importance", decision.Importance),
	)

	return decision, nil
}

// ApplyDecision saves the memory based on the evaluation decision
func (m *MemoryEvaluator) ApplyDecision(ctx context.Context, agentID, userID string, decision *MemoryDecision) error {
	if !decision.ShouldSave || decision.Importance < 3 {
		return nil // Not important enough
	}

	// If updating existing fact (from LLM decision)
	if decision.UpdatesExisting && decision.ExistingID != "" {
		// Try to update existing fact
		if err := m.graphRepo.UpdateFact(ctx, decision.ExistingID, decision.Content); err != nil {
			m.logger.Warn("Failed to update existing fact, creating new one",
				zap.String("existing_id", decision.ExistingID),
				zap.Error(err),
			)
			// Fall through to check for duplicates and create new fact
		} else {
			m.logger.Info("Updated existing fact",
				zap.String("fact_id", decision.ExistingID),
				zap.String("user_id", userID),
			)
			return nil
		}
	}

	// Check for similar/duplicate facts BEFORE creating new one
	similarFacts, err := m.findSimilarFacts(ctx, userID, decision.Content)
	if err != nil {
		m.logger.Warn("Failed to check for similar facts", zap.Error(err))
		// Continue with creation if check fails
	} else if len(similarFacts) > 0 {
		// Found similar facts - update the most recent one instead of creating duplicate
		mostRecent := similarFacts[0]
		if err := m.graphRepo.UpdateFact(ctx, mostRecent.ID, decision.Content); err != nil {
			m.logger.Warn("Failed to update similar fact, creating new one",
				zap.String("existing_id", mostRecent.ID),
				zap.Error(err),
			)
			// Fall through to create new fact
		} else {
			m.logger.Info("Updated existing similar fact instead of creating duplicate",
				zap.String("fact_id", mostRecent.ID),
				zap.String("user_id", userID),
				zap.String("old_content", mostRecent.Content),
				zap.String("new_content", decision.Content),
			)
			return nil
		}
	}

	// Create new fact
	source := "auto-extracted"
	if decision.MemoryType == "life_event" {
		source = "life event"
	} else if decision.MemoryType == "preference" {
		source = "preference"
	} else if decision.MemoryType == "personal_info" {
		source = "personal information"
	}

	// Ensure we have at least one topic
	topics := decision.Topics
	if len(topics) == 0 {
		// Auto-assign topic based on memory type
		switch decision.MemoryType {
		case "preference":
			topics = []string{"Preferences"}
		case "personal_info":
			topics = []string{"Personal"}
		case "life_event":
			topics = []string{"Life Events"}
		default:
			topics = []string{"General"}
		}
	}

	fact, err := m.graphRepo.CreateFact(ctx, agentID, decision.Content, source, userID, topics)
	if err != nil {
		return fmt.Errorf("failed to create fact: %w", err)
	}

	m.logger.Info("Auto-saved memory",
		zap.String("fact_id", fact.ID),
		zap.String("user_id", userID),
		zap.String("type", decision.MemoryType),
		zap.Int("importance", decision.Importance),
		zap.Strings("topics", topics),
		zap.String("reasoning", decision.Reasoning),
	)

	return nil
}

// isNonMemoryMessage quickly filters out messages that are unlikely to contain memory-worthy content
func (m *MemoryEvaluator) isNonMemoryMessage(message string) bool {
	lower := strings.ToLower(strings.TrimSpace(message))
	
	// Very short messages
	if len(lower) < 10 {
		return true
	}

	// Greetings
	greetingPatterns := []string{
		"^hi", "^hello", "^hey", "^good morning", "^good afternoon", "^good evening",
		"^thanks", "^thank you", "^ty", "^thx",
		"^bye", "^goodbye", "^see you",
	}
	for _, pattern := range greetingPatterns {
		if matched, _ := regexp.MatchString(pattern, lower); matched {
			return true
		}
	}

	// Questions (simple ones)
	questionPatterns := []string{
		"^what", "^how", "^when", "^where", "^why", "^who",
		"^can you", "^could you", "^will you", "^would you",
		"^tell me", "^show me", "^help me",
	}
	for _, pattern := range questionPatterns {
		if matched, _ := regexp.MatchString(pattern, lower); matched {
			// But allow questions that might contain info ("what's my favorite color?" -> might be memory)
			// Only filter if it's clearly a question TO the bot
			if strings.Contains(lower, "?") && !strings.Contains(lower, "my ") && !strings.Contains(lower, "i ") {
				return true
			}
		}
	}

	// Commands
	commandPatterns := []string{
		"^/", "^!", "^@", "^search", "^find", "^get", "^show",
	}
	for _, pattern := range commandPatterns {
		if matched, _ := regexp.MatchString(pattern, lower); matched {
			return true
		}
	}

	return false
}

// findSimilarFacts checks for similar or duplicate facts using LLM
func (m *MemoryEvaluator) findSimilarFacts(ctx context.Context, userID, content string) ([]graph.Fact, error) {
	// Get all existing facts for this user
	userCtx, err := m.graphRepo.GetUserContext(ctx, userID)
	if err != nil {
		return nil, err
	}

	if len(userCtx.Facts) == 0 {
		return nil, nil
	}

	// Use LLM to find similar facts
	prompt := fmt.Sprintf(`Compare this new fact with existing facts and identify which ones are duplicates, conflicts, or updates:

New fact: "%s"

Existing facts:
%s

Respond with ONLY valid JSON array (no markdown, no explanation):
[
  {"id": "fact_id", "relationship": "duplicate|conflict|update|similar|none", "confidence": 0.0-1.0, "reason": "brief explanation"}
]

Guidelines:
- "duplicate": Same meaning, different wording (e.g., "User prefers English" vs "User prefers to communicate in English")
- "conflict": Contradictory information (e.g., "User prefers English" vs "User prefers Pig Latin")
- "update": Newer version of old information (e.g., "User is 25" vs "User is 26")
- "similar": Related but not identical (e.g., "User likes pizza" vs "User loves Italian food")
- "none": Not related

Only include facts where relationship is NOT "none" and confidence >= 0.7. Return the most similar/conflicting fact first.`, 
		content, 
		formatFactsForLLM(userCtx.Facts))

	response, err := m.llm.Generate(ctx, prompt, "Respond with JSON array only. No markdown, no explanation.", nil)
	if err != nil {
		m.logger.Warn("Failed to check for similar facts with LLM", zap.Error(err))
		return nil, err
	}

	// Parse response
	similarFacts := parseSimilarFactsResponse(response.Content, userCtx.Facts)
	return similarFacts, nil
}

// formatFactsForLLM formats facts for LLM analysis
func formatFactsForLLM(facts []graph.Fact) string {
	var parts []string
	for i, fact := range facts {
		parts = append(parts, fmt.Sprintf("%d. [ID: %s] %s", i+1, fact.ID, fact.Content))
	}
	return strings.Join(parts, "\n")
}

// parseSimilarFactsResponse parses LLM response to extract similar facts
func parseSimilarFactsResponse(response string, allFacts []graph.Fact) []graph.Fact {
	// Extract JSON from response
	jsonStr := strings.TrimSpace(response)
	if strings.HasPrefix(jsonStr, "```") {
		lines := strings.Split(jsonStr, "\n")
		var jsonLines []string
		inCodeBlock := false
		for _, line := range lines {
			if strings.HasPrefix(line, "```") {
				inCodeBlock = !inCodeBlock
				continue
			}
			if inCodeBlock {
				jsonLines = append(jsonLines, line)
			}
		}
		jsonStr = strings.Join(jsonLines, "\n")
	}

	// Find JSON array
	if start := strings.Index(jsonStr, "["); start != -1 {
		if end := strings.LastIndex(jsonStr, "]"); end != -1 && end > start {
			jsonStr = jsonStr[start : end+1]
		}
	}

	var results []map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &results); err != nil {
		return nil
	}

	// Map to facts
	factMap := make(map[string]graph.Fact)
	for _, fact := range allFacts {
		factMap[fact.ID] = fact
	}

	var similarFacts []graph.Fact
	for _, result := range results {
		if id, ok := result["id"].(string); ok {
			if fact, exists := factMap[id]; exists {
				if conf, ok := result["confidence"].(float64); ok && conf >= 0.7 {
					rel, _ := result["relationship"].(string)
					// Prioritize duplicates and conflicts
					if rel == "duplicate" || rel == "conflict" || rel == "update" {
						similarFacts = append(similarFacts, fact)
					} else if rel == "similar" && conf >= 0.85 {
						// Only include "similar" if very high confidence
						similarFacts = append(similarFacts, fact)
					}
				}
			}
		}
	}

	return similarFacts
}

// CleanupUserMemories periodically cleans up duplicate/conflicting memories for a user
func (m *MemoryEvaluator) CleanupUserMemories(ctx context.Context, userID string) error {
	// Get all facts for this user
	userCtx, err := m.graphRepo.GetUserContext(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get user context: %w", err)
	}

	if len(userCtx.Facts) < 2 {
		return nil // No duplicates possible
	}

	// Group facts by similarity using LLM
	duplicateGroups := m.findDuplicateGroups(ctx, userCtx.Facts)
	
	// Process each group - keep the most recent, delete others
	for _, group := range duplicateGroups {
		if len(group) < 2 {
			continue
		}
		
		// Keep the first fact (should be most recent), delete the rest
		keepID := group[0]
		for i := 1; i < len(group); i++ {
			if err := m.graphRepo.DeleteFact(ctx, group[i]); err != nil {
				m.logger.Warn("Failed to delete duplicate fact",
					zap.String("fact_id", group[i]),
					zap.Error(err),
				)
			} else {
				m.logger.Info("Deleted duplicate fact",
					zap.String("fact_id", group[i]),
					zap.String("kept_id", keepID),
					zap.String("user_id", userID),
				)
			}
		}
	}

	return nil
}

// findDuplicateGroups uses LLM to group duplicate/conflicting facts
func (m *MemoryEvaluator) findDuplicateGroups(ctx context.Context, facts []graph.Fact) [][]string {
	if len(facts) < 2 {
		return nil
	}

	// Build fact list for LLM
	var factList []string
	for i, fact := range facts {
		factList = append(factList, fmt.Sprintf("%d. [ID: %s] %s", i+1, fact.ID, fact.Content))
	}

	prompt := fmt.Sprintf(`You are a memory deduplication system. Analyze these facts and group them by duplicates or conflicts.

Facts:
%s

Respond with ONLY valid JSON array (no markdown, no explanation):
[
  {"group": ["fact_id1", "fact_id2", ...], "type": "duplicate|conflict", "reason": "why they're grouped"}
]

Guidelines:
- Group facts that are clearly duplicates (same meaning, different wording)
- Group facts that are conflicts (contradictory information about the same topic)
- Each fact ID should appear in at most one group
- Only create groups with 2+ facts
- Return empty array if no duplicates/conflicts found`, strings.Join(factList, "\n"))

	response, err := m.llm.Generate(ctx, prompt, "Respond with JSON array only. No markdown, no explanation.", nil)
	if err != nil {
		m.logger.Warn("Failed to analyze duplicates with LLM", zap.Error(err))
		return nil
	}

	// Parse response
	jsonStr := strings.TrimSpace(response.Content)
	if strings.HasPrefix(jsonStr, "```") {
		lines := strings.Split(jsonStr, "\n")
		var jsonLines []string
		inCodeBlock := false
		for _, line := range lines {
			if strings.HasPrefix(line, "```") {
				inCodeBlock = !inCodeBlock
				continue
			}
			if inCodeBlock {
				jsonLines = append(jsonLines, line)
			}
		}
		jsonStr = strings.Join(jsonLines, "\n")
	}

	// Find JSON array
	if start := strings.Index(jsonStr, "["); start != -1 {
		if end := strings.LastIndex(jsonStr, "]"); end != -1 && end > start {
			jsonStr = jsonStr[start : end+1]
		}
	}

	var results []map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &results); err != nil {
		m.logger.Warn("Failed to parse duplicate groups JSON", zap.Error(err))
		return nil
	}

	// Extract groups
	var groups [][]string
	for _, result := range results {
		if groupList, ok := result["group"].([]interface{}); ok {
			var group []string
			for _, item := range groupList {
				if id, ok := item.(string); ok {
					group = append(group, id)
				}
			}
			if len(group) >= 2 {
				groups = append(groups, group)
			}
		}
	}

	return groups
}

