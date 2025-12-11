package graph

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"ezra-clone/backend/internal/state"
	"ezra-clone/backend/pkg/logger"
	"go.uber.org/zap"
)

// Repository handles all Neo4j database operations
type Repository struct {
	driver neo4j.DriverWithContext
	logger *zap.Logger
}

// NewRepository creates a new graph repository
func NewRepository(driver neo4j.DriverWithContext) *Repository {
	return &Repository{
		driver: driver,
		logger: logger.Get(),
	}
}

// Close closes the Neo4j driver connection
func (r *Repository) Close() error {
	return r.driver.Close(context.Background())
}

// FetchState retrieves the complete context window for an agent
func (r *Repository) FetchState(ctx context.Context, agentID string) (*state.ContextWindow, error) {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	query := `
		MATCH (a:Agent {id: $agentID})
		OPTIONAL MATCH (a)-[:HAS_IDENTITY]->(id:AgentIdentity)
		OPTIONAL MATCH (a)-[:HAS_MEMORY]->(m:Memory)
		OPTIONAL MATCH (a)-[:HAS_ARCHIVAL]->(arch:Archival)
		RETURN 
			a.id as agent_id,
			a.name as agent_name,
			id.name as identity_name,
			id.personality as identity_personality,
			id.capabilities as identity_capabilities,
			collect(DISTINCT {
				name: m.name,
				content: m.content,
				updated_at: m.updated_at
			}) as memories,
			collect(DISTINCT {
				summary: arch.summary,
				timestamp: arch.timestamp,
				relevance_score: arch.relevance_score
			}) as archivals
	`

	result, err := session.Run(ctx, query, map[string]interface{}{
		"agentID": agentID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to execute query: %w", err)
	}

	if !result.Next(ctx) {
		if err := result.Err(); err != nil {
			return nil, fmt.Errorf("failed to fetch record: %w", err)
		}
		return nil, ErrAgentNotFound{AgentID: agentID}
	}

	record := result.Record()

	// Build ContextWindow from record
	cw := &state.ContextWindow{
		Identity: state.AgentIdentity{
			Name:        getString(record, "identity_name", getString(record, "agent_name", "")),
			Personality: getString(record, "identity_personality", ""),
			Capabilities: getStringSlice(record, "identity_capabilities"),
		},
		CoreMemory:   []state.MemoryBlock{},
		ArchivalRefs: []state.ArchivalPointer{},
		UserContext:  make(map[string]interface{}),
	}

	// Parse memories
	memories, _ := record.Get("memories")
	if memoriesList, ok := memories.([]interface{}); ok {
		for _, mem := range memoriesList {
			if memMap, ok := mem.(map[string]interface{}); ok {
				if name, ok := memMap["name"].(string); ok && name != "" {
					content := getStringFromMap(memMap, "content", "")
					updatedAt := getTimeFromMap(memMap, "updated_at", time.Now())
					cw.CoreMemory = append(cw.CoreMemory, state.MemoryBlock{
						Name:      name,
						Content:   content,
						UpdatedAt: updatedAt,
					})
				}
			}
		}
	}

	// Parse archivals
	archivals, _ := record.Get("archivals")
	if archivalsList, ok := archivals.([]interface{}); ok {
		for _, arch := range archivalsList {
			if archMap, ok := arch.(map[string]interface{}); ok {
				summary := getStringFromMap(archMap, "summary", "")
				if summary != "" {
					timestamp := getTimeFromMap(archMap, "timestamp", time.Now())
					score := getFloat64FromMap(archMap, "relevance_score", 0.0)
					cw.ArchivalRefs = append(cw.ArchivalRefs, state.ArchivalPointer{
						Summary:        summary,
						Timestamp:      timestamp,
						RelevanceScore: score,
					})
				}
			}
		}
	}

	return cw, nil
}

// UpdateMemory updates or creates a memory block for an agent
// If the agent doesn't exist, it will be created automatically
func (r *Repository) UpdateMemory(ctx context.Context, agentID, blockName, newContent string) error {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	// First, ensure the agent exists
	agentQuery := `
		MERGE (a:Agent {id: $agentID})
		ON CREATE SET a.name = $agentID
		RETURN a.id as id
	`
	result, err := session.Run(ctx, agentQuery, map[string]interface{}{
		"agentID": agentID,
	})
	if err != nil {
		return fmt.Errorf("failed to ensure agent exists: %w", err)
	}
	if !result.Next(ctx) {
		return fmt.Errorf("failed to create or find agent: %s", agentID)
	}

	// Now update/create the memory block
	query := `
		MATCH (a:Agent {id: $agentID})
		MERGE (a)-[:HAS_MEMORY]->(m:Memory {name: $blockName})
		SET m.content = $newContent,
		    m.updated_at = datetime()
		RETURN m.name as name
	`

	_, err = session.Run(ctx, query, map[string]interface{}{
		"agentID":   agentID,
		"blockName": blockName,
		"newContent": newContent,
	})
	if err != nil {
		return fmt.Errorf("failed to update memory: %w", err)
	}

	r.logger.Info("Memory block updated",
		zap.String("agent_id", agentID),
		zap.String("block_name", blockName),
	)
	return nil
}

// DeleteMemory deletes a memory block for an agent
func (r *Repository) DeleteMemory(ctx context.Context, agentID, blockName string) error {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	query := `
		MATCH (a:Agent {id: $agentID})-[:HAS_MEMORY]->(m:Memory {name: $blockName})
		DETACH DELETE m
		RETURN count(m) as deleted
	`

	result, err := session.Run(ctx, query, map[string]interface{}{
		"agentID":   agentID,
		"blockName": blockName,
	})
	if err != nil {
		return fmt.Errorf("failed to delete memory: %w", err)
	}

	if result.Next(ctx) {
		record := result.Record()
		deleted, _ := record.Get("deleted")
		if deletedCount, ok := deleted.(int64); ok && deletedCount == 0 {
			return fmt.Errorf("memory block not found")
		}
	}

	r.logger.Info("Memory block deleted",
		zap.String("agent_id", agentID),
		zap.String("block_name", blockName),
	)
	return nil
}

// LogInteraction logs an interaction between a user and an agent
func (r *Repository) LogInteraction(ctx context.Context, agentID, userID, message string, timestamp time.Time) error {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	// Convert to UTC and format as ISO 8601 string for Neo4j compatibility
	timestampStr := timestamp.UTC().Format(time.RFC3339)

	query := `
		MATCH (a:Agent {id: $agentID})
		MERGE (u:User {id: $userID})
		CREATE (i:Interaction {
			message: $message,
			timestamp: datetime($timestamp)
		})
		CREATE (a)<-[:WITH_AGENT]-(i)-[:FROM_USER]->(u)
		RETURN i
	`

	_, err := session.Run(ctx, query, map[string]interface{}{
		"agentID":   agentID,
		"userID":    userID,
		"message":   message,
		"timestamp": timestampStr,
	})
	if err != nil {
		return fmt.Errorf("failed to log interaction: %w", err)
	}

	return nil
}

// CreateAgent creates a new agent node in the graph
func (r *Repository) CreateAgent(ctx context.Context, agentID, name string) error {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	query := `
		MERGE (a:Agent {id: $agentID})
		SET a.name = $name,
		    a.created_at = datetime()
		RETURN a.id as id
	`

	result, err := session.Run(ctx, query, map[string]interface{}{
		"agentID": agentID,
		"name":    name,
	})
	if err != nil {
		return fmt.Errorf("failed to create agent: %w", err)
	}

	_, err = result.Single(ctx)
	if err != nil {
		return fmt.Errorf("failed to verify agent creation: %w", err)
	}

	r.logger.Info("Agent created",
		zap.String("agent_id", agentID),
		zap.String("name", name),
	)
	return nil
}

// CreateAgentIdentity creates or updates the identity for an agent
func (r *Repository) CreateAgentIdentity(ctx context.Context, agentID string, identity state.AgentIdentity) error {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	query := `
		MATCH (a:Agent {id: $agentID})
		MERGE (a)-[:HAS_IDENTITY]->(id:AgentIdentity)
		SET id.name = $name,
		    id.personality = $personality,
		    id.capabilities = $capabilities
		RETURN id
	`

	_, err := session.Run(ctx, query, map[string]interface{}{
		"agentID":     agentID,
		"name":        identity.Name,
		"personality": identity.Personality,
		"capabilities": identity.Capabilities,
	})
	if err != nil {
		return fmt.Errorf("failed to create agent identity: %w", err)
	}

	return nil
}

// Helper functions

func getString(record *neo4j.Record, key string, defaultValue string) string {
	val, ok := record.Get(key)
	if !ok {
		return defaultValue
	}
	if str, ok := val.(string); ok {
		return str
	}
	return defaultValue
}

func getStringSlice(record *neo4j.Record, key string) []string {
	val, ok := record.Get(key)
	if !ok {
		return []string{}
	}
	if slice, ok := val.([]interface{}); ok {
		result := make([]string, 0, len(slice))
		for _, v := range slice {
			if str, ok := v.(string); ok {
				result = append(result, str)
			}
		}
		return result
	}
	return []string{}
}

// getStringFromMap and getFloat64FromMap are defined in helpers.go

func getTimeFromMap(m map[string]interface{}, key string, defaultValue time.Time) time.Time {
	val, ok := m[key]
	if !ok {
		return defaultValue
	}
	// Neo4j datetime values come as time.Time
	if t, ok := val.(time.Time); ok {
		return t
	}
	return defaultValue
}

// getFloat64FromMap is defined in helpers.go

// ListAgents returns all agents with their metadata
func (r *Repository) ListAgents(ctx context.Context) ([]AgentInfo, error) {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	query := `
		MATCH (a:Agent)
		RETURN a.id as id, a.name as name, a.created_at as created_at
		ORDER BY a.created_at DESC
	`

	result, err := session.Run(ctx, query, map[string]interface{}{})
	if err != nil {
		return nil, fmt.Errorf("failed to list agents: %w", err)
	}

	var agents []AgentInfo
	for result.Next(ctx) {
		record := result.Record()
		createdAt := getTimeFromRecord(record, "created_at", time.Now())
		agents = append(agents, AgentInfo{
			ID:        getString(record, "id", ""),
			Name:      getString(record, "name", ""),
			CreatedAt: createdAt,
		})
	}

	return agents, nil
}

// AgentInfo represents basic agent information
type AgentInfo struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

// GetAgentConfig retrieves agent configuration (model, system_instructions)
func (r *Repository) GetAgentConfig(ctx context.Context, agentID string) (*AgentConfig, error) {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	query := `
		MATCH (a:Agent {id: $agentID})
		OPTIONAL MATCH (a)-[:HAS_IDENTITY]->(id:AgentIdentity)
		RETURN 
			a.model as model,
			a.system_instructions as system_instructions,
			id.personality as personality
	`

	result, err := session.Run(ctx, query, map[string]interface{}{
		"agentID": agentID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get agent config: %w", err)
	}

	if !result.Next(ctx) {
		return nil, ErrAgentNotFound{AgentID: agentID}
	}

	record := result.Record()
	model := getString(record, "model", "")
	systemInstructions := getString(record, "system_instructions", "")
	personality := getString(record, "personality", "")

	// If system_instructions is not set, use personality as fallback
	if systemInstructions == "" {
		systemInstructions = personality
	}

	return &AgentConfig{
		Model:              model,
		SystemInstructions: systemInstructions,
	}, nil
}

// AgentConfig represents agent configuration
type AgentConfig struct {
	Model              string `json:"model"`
	SystemInstructions string `json:"system_instructions"`
}

// UpdateAgentConfig updates agent configuration
func (r *Repository) UpdateAgentConfig(ctx context.Context, agentID string, config AgentConfig) error {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	query := `
		MATCH (a:Agent {id: $agentID})
		SET a.model = $model,
		    a.system_instructions = $system_instructions,
		    a.updated_at = datetime()
		RETURN a.id as id
	`

	_, err := session.Run(ctx, query, map[string]interface{}{
		"agentID":            agentID,
		"model":              config.Model,
		"system_instructions": config.SystemInstructions,
	})
	if err != nil {
		return fmt.Errorf("failed to update agent config: %w", err)
	}

	r.logger.Info("Agent config updated",
		zap.String("agent_id", agentID),
		zap.String("model", config.Model),
	)
	return nil
}

// GetArchivalMemories retrieves all archival memories for an agent
func (r *Repository) GetArchivalMemories(ctx context.Context, agentID string) ([]ArchivalMemory, error) {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	query := `
		MATCH (a:Agent {id: $agentID})-[:HAS_ARCHIVAL]->(arch:Archival)
		RETURN arch.id as id,
		       arch.summary as summary,
		       arch.timestamp as timestamp,
		       arch.relevance_score as relevance_score,
		       arch.content as content
		ORDER BY arch.timestamp DESC
	`

	result, err := session.Run(ctx, query, map[string]interface{}{
		"agentID": agentID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get archival memories: %w", err)
	}

	var memories []ArchivalMemory
	for result.Next(ctx) {
		record := result.Record()
		timestamp := getTimeFromRecord(record, "timestamp", time.Now())
		relevanceScore := getFloat64FromRecord(record, "relevance_score")
		memoryID := getString(record, "id", "")
		// If no ID exists, generate one from timestamp
		if memoryID == "" {
			memoryID = uuid.New().String()
		}
		memories = append(memories, ArchivalMemory{
			ID:             memoryID,
			Summary:        getString(record, "summary", ""),
			Content:        getString(record, "content", ""),
			Timestamp:      timestamp,
			RelevanceScore: relevanceScore,
		})
	}

	return memories, nil
}

// ArchivalMemory represents an archival memory entry
type ArchivalMemory struct {
	ID             string    `json:"id"`
	Summary        string    `json:"summary"`
	Content        string    `json:"content"`
	Timestamp      time.Time `json:"timestamp"`
	RelevanceScore float64   `json:"relevance_score"`
}

// DeleteArchivalMemory deletes an archival memory by ID
func (r *Repository) DeleteArchivalMemory(ctx context.Context, agentID string, memoryID string) error {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	query := `
		MATCH (a:Agent {id: $agentID})-[:HAS_ARCHIVAL]->(arch:Archival {id: $memoryID})
		DETACH DELETE arch
		RETURN count(arch) as deleted
	`

	result, err := session.Run(ctx, query, map[string]interface{}{
		"agentID":  agentID,
		"memoryID": memoryID,
	})
	if err != nil {
		return fmt.Errorf("failed to delete archival memory: %w", err)
	}

	if result.Next(ctx) {
		record := result.Record()
		deleted, _ := record.Get("deleted")
		if deletedCount, ok := deleted.(int64); ok && deletedCount == 0 {
			return fmt.Errorf("archival memory not found")
		}
	}

	r.logger.Info("Archival memory deleted",
		zap.String("agent_id", agentID),
		zap.String("memory_id", memoryID),
	)
	return nil
}

// CreateArchivalMemory creates a new archival memory
func (r *Repository) CreateArchivalMemory(ctx context.Context, agentID string, memory ArchivalMemory) error {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	timestampStr := memory.Timestamp.UTC().Format(time.RFC3339)
	
	// Generate ID if not provided
	if memory.ID == "" {
		memory.ID = uuid.New().String()
	}

	query := `
		MATCH (a:Agent {id: $agentID})
		CREATE (a)-[:HAS_ARCHIVAL]->(arch:Archival {
			id: $id,
			summary: $summary,
			content: $content,
			timestamp: datetime($timestamp),
			relevance_score: $relevance_score
		})
		RETURN arch
	`

	_, err := session.Run(ctx, query, map[string]interface{}{
		"agentID":        agentID,
		"id":             memory.ID,
		"summary":         memory.Summary,
		"content":         memory.Content,
		"timestamp":      timestampStr,
		"relevance_score": memory.RelevanceScore,
	})
	if err != nil {
		return fmt.Errorf("failed to create archival memory: %w", err)
	}

	r.logger.Info("Archival memory created",
		zap.String("agent_id", agentID),
		zap.String("summary", memory.Summary),
	)
	return nil
}

// GetContextStats estimates token usage for an agent's context window
func (r *Repository) GetContextStats(ctx context.Context, agentID string) (*ContextStats, error) {
	state, err := r.FetchState(ctx, agentID)
	if err != nil {
		return nil, err
	}

	// Simple token estimation: ~4 characters per token
	// This is a rough approximation
	totalChars := 0
	
	// Count identity
	totalChars += len(state.Identity.Name)
	totalChars += len(state.Identity.Personality)
	for _, cap := range state.Identity.Capabilities {
		totalChars += len(cap)
	}

	// Count core memory
	for _, block := range state.CoreMemory {
		totalChars += len(block.Name)
		totalChars += len(block.Content)
	}

	// Count archival refs
	for _, arch := range state.ArchivalRefs {
		totalChars += len(arch.Summary)
	}

	// Estimate tokens (rough: 4 chars per token)
	estimatedTokens := totalChars / 4

	// Default context window sizes (can be made configurable)
	totalTokens := 16384 // Default for most models
	if estimatedTokens > 8192 {
		totalTokens = 32768 // Larger models
	}

	return &ContextStats{
		UsedTokens:  estimatedTokens,
		TotalTokens: totalTokens,
	}, nil
}

// GetAllFacts retrieves all facts known by an agent
// Note: Fact type is defined in enhanced_repository.go
func (r *Repository) GetAllFacts(ctx context.Context, agentID string) ([]*Fact, error) {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	query := `
		MATCH (a:Agent {id: $agentID})-[:KNOWS_FACT]->(f:Fact)
		RETURN f.id as id, f.content as content, f.source as source,
		       f.confidence as confidence, f.created_at as created_at
		ORDER BY f.created_at DESC
	`

	result, err := session.Run(ctx, query, map[string]interface{}{
		"agentID": agentID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get facts: %w", err)
	}

	var facts []*Fact
	for result.Next(ctx) {
		record := result.Record()
		createdAt := getTimeFromRecord(record, "created_at", time.Now())
		confidence := getFloat64FromRecord(record, "confidence")
		facts = append(facts, &Fact{
			ID:         getString(record, "id", ""),
			Content:    getString(record, "content", ""),
			Source:     getString(record, "source", ""),
			Confidence: confidence,
			CreatedAt:  createdAt,
		})
	}

	return facts, nil
}

// GetAllTopics retrieves all topics related to an agent
// Note: Topic type is defined in enhanced_repository.go
func (r *Repository) GetAllTopics(ctx context.Context, agentID string) ([]*Topic, error) {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	query := `
		MATCH (a:Agent {id: $agentID})-[:KNOWS_FACT]->(f:Fact)-[:ABOUT]->(t:Topic)
		RETURN DISTINCT t.id as id, t.name as name, t.description as description
		ORDER BY t.name
	`

	result, err := session.Run(ctx, query, map[string]interface{}{
		"agentID": agentID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get topics: %w", err)
	}

	var topics []*Topic
	for result.Next(ctx) {
		record := result.Record()
		topics = append(topics, &Topic{
			ID:          getString(record, "id", ""),
			Name:        getString(record, "name", ""),
			Description: getString(record, "description", ""),
		})
	}

	return topics, nil
}

// GetAllMessages retrieves all messages sent by or to an agent
// Note: Message type is defined in enhanced_repository.go
func (r *Repository) GetAllMessages(ctx context.Context, agentID string, limit int) ([]*Message, error) {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	if limit < 1 {
		limit = 100
	}

	query := `
		MATCH (a:Agent {id: $agentID})-[:SENT]->(m:Message)
		RETURN m.id as id, m.content as content, m.role as role,
		       m.platform as platform, m.timestamp as timestamp
		ORDER BY m.timestamp DESC
		LIMIT $limit
	`

	result, err := session.Run(ctx, query, map[string]interface{}{
		"agentID": agentID,
		"limit":   limit,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get messages: %w", err)
	}

	var messages []*Message
	for result.Next(ctx) {
		record := result.Record()
		timestamp := getTimeFromRecord(record, "timestamp", time.Now())
		messages = append(messages, &Message{
			ID:        getString(record, "id", ""),
			Content:   getString(record, "content", ""),
			Role:      getString(record, "role", ""),
			Platform:  getString(record, "platform", ""),
			Timestamp: timestamp,
		})
	}

	return messages, nil
}

// GetAllConversations retrieves all conversations for an agent
// Note: Conversation type is defined in enhanced_repository.go
func (r *Repository) GetAllConversations(ctx context.Context, agentID string, limit int) ([]*Conversation, error) {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	if limit < 1 {
		limit = 50
	}

	query := `
		MATCH (a:Agent {id: $agentID})-[:SENT]->(m:Message)<-[:CONTAINS]-(c:Conversation)
		RETURN DISTINCT c.id as id, c.channel_id as channel_id,
		       c.platform as platform, c.started_at as started_at
		ORDER BY c.started_at DESC
		LIMIT $limit
	`

	result, err := session.Run(ctx, query, map[string]interface{}{
		"agentID": agentID,
		"limit":   limit,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get conversations: %w", err)
	}

	var conversations []*Conversation
	for result.Next(ctx) {
		record := result.Record()
		startedAt := getTimeFromRecord(record, "started_at", time.Now())
		conversations = append(conversations, &Conversation{
			ID:        getString(record, "id", ""),
			ChannelID: getString(record, "channel_id", ""),
			Platform:  getString(record, "platform", ""),
			StartedAt: startedAt,
		})
	}

	return conversations, nil
}

// GetAllUsers retrieves all users that have interacted with an agent
// Note: User type is defined in enhanced_repository.go
func (r *Repository) GetAllUsers(ctx context.Context, agentID string) ([]*User, error) {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	query := `
		MATCH (a:Agent {id: $agentID})-[:SENT]->(m:Message)<-[:SENT]-(u:User)
		RETURN DISTINCT u.id as id, u.discord_id as discord_id,
		       u.discord_username as discord_username, u.web_id as web_id,
		       u.preferred_language as preferred_language,
		       u.first_seen as first_seen, u.last_seen as last_seen
		ORDER BY u.last_seen DESC
	`

	result, err := session.Run(ctx, query, map[string]interface{}{
		"agentID": agentID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get users: %w", err)
	}

	var users []*User
	for result.Next(ctx) {
		record := result.Record()
		firstSeen := getTimeFromRecord(record, "first_seen", time.Now())
		lastSeen := getTimeFromRecord(record, "last_seen", time.Now())
		users = append(users, &User{
			ID:              getString(record, "id", ""),
			DiscordID:       getString(record, "discord_id", ""),
			DiscordUsername: getString(record, "discord_username", ""),
			WebID:           getString(record, "web_id", ""),
			PreferredLanguage: getString(record, "preferred_language", ""),
			FirstSeen:       firstSeen,
			LastSeen:        lastSeen,
		})
	}

	return users, nil
}

// ContextStats represents context window statistics
type ContextStats struct {
	UsedTokens  int `json:"used_tokens"`
	TotalTokens int `json:"total_tokens"`
}

// Helper functions for records

func getTimeFromRecord(record *neo4j.Record, key string, defaultValue time.Time) time.Time {
	val, ok := record.Get(key)
	if !ok {
		return defaultValue
	}
	if t, ok := val.(time.Time); ok {
		return t
	}
	return defaultValue
}

// Errors

type ErrAgentNotFound struct {
	AgentID string
}

func (e ErrAgentNotFound) Error() string {
	return fmt.Sprintf("agent not found: %s", e.AgentID)
}

