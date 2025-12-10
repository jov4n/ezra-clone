package graph

import (
	"context"
	"fmt"
	"time"

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
func (r *Repository) UpdateMemory(ctx context.Context, agentID, blockName, newContent string) error {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	query := `
		MATCH (a:Agent {id: $agentID})
		MERGE (a)-[:HAS_MEMORY]->(m:Memory {name: $blockName})
		SET m.content = $newContent,
		    m.updated_at = datetime()
		RETURN m.name as name
	`

	_, err := session.Run(ctx, query, map[string]interface{}{
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

func getStringFromMap(m map[string]interface{}, key string, defaultValue string) string {
	val, ok := m[key]
	if !ok {
		return defaultValue
	}
	if str, ok := val.(string); ok {
		return str
	}
	return defaultValue
}

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

func getFloat64FromMap(m map[string]interface{}, key string, defaultValue float64) float64 {
	val, ok := m[key]
	if !ok {
		return defaultValue
	}
	if f, ok := val.(float64); ok {
		return f
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

