package graph

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"go.uber.org/zap"
)

// ============================================================================
// Personality Memory Operations (RAG for User Facts/Opinions)
// ============================================================================

// UserPersonalityMemory represents a stored memory/fact about a user
type UserPersonalityMemory struct {
	ID          string    `json:"id"`
	UserID      string    `json:"user_id"`
	Content     string    `json:"content"`
	Source      string    `json:"source"`      // e.g., "discord_channel_123", "manual_entry"
	ChannelID   string    `json:"channel_id"` // Discord channel where this was observed
	Tags        []string  `json:"tags"`       // Optional tags for categorization
	CreatedAt   time.Time `json:"created_at"`
	Consented   bool      `json:"consented"` // Whether user explicitly consented to this memory
}

// StoreUserPersonalityMemory stores a consented memory/fact about a user
// This is used for RAG retrieval to maintain consistency in personality mimicry
func (r *Repository) StoreUserPersonalityMemory(ctx context.Context, userID, content, source, channelID string, tags []string, consented bool) (*UserPersonalityMemory, error) {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	memoryID := uuid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)

	// Ensure user exists
	ensureUserQuery := `
		MERGE (u:User {id: $userID})
		ON CREATE SET u.created_at = datetime($now)
		RETURN u.id as id
	`
	_, err := session.Run(ctx, ensureUserQuery, map[string]interface{}{
		"userID": userID,
		"now":    now,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to ensure user exists: %w", err)
	}

	// Create the personality memory node
	query := `
		MATCH (u:User {id: $userID})
		CREATE (m:UserPersonalityMemory {
			id: $memoryID,
			content: $content,
			source: $source,
			channel_id: $channelID,
			tags: $tags,
			consented: $consented,
			created_at: datetime($now)
		})
		CREATE (u)-[:HAS_PERSONALITY_MEMORY]->(m)
		RETURN m.id as id, m.content as content, m.source as source,
		       m.channel_id as channel_id, m.tags as tags,
		       m.consented as consented, m.created_at as created_at
	`

	result, err := session.Run(ctx, query, map[string]interface{}{
		"userID":    userID,
		"memoryID":  memoryID,
		"content":   content,
		"source":    source,
		"channelID": channelID,
		"tags":      tags,
		"consented": consented,
		"now":       now,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to store personality memory: %w", err)
	}

	if !result.Next(ctx) {
		return nil, fmt.Errorf("failed to retrieve created memory")
	}

	record := result.Record()
	createdAt, _ := time.Parse(time.RFC3339, getStringFromRecord(record, "created_at"))

	memory := &UserPersonalityMemory{
		ID:        getStringFromRecord(record, "id"),
		UserID:    userID,
		Content:   getStringFromRecord(record, "content"),
		Source:    getStringFromRecord(record, "source"),
		ChannelID: getStringFromRecord(record, "channel_id"),
		Tags:      getStringSliceFromRecord(record, "tags"),
		Consented: getBoolFromRecord(record, "consented"),
		CreatedAt: createdAt,
	}

	r.logger.Info("Personality memory stored",
		zap.String("memory_id", memoryID),
		zap.String("user_id", userID),
		zap.Bool("consented", consented),
	)

	return memory, nil
}

// RetrieveUserPersonalityMemories retrieves relevant memories for a user based on query similarity
// Uses text-based similarity search (can be upgraded to vector search)
func (r *Repository) RetrieveUserPersonalityMemories(ctx context.Context, userID, query string, limit int) ([]UserPersonalityMemory, error) {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	if limit <= 0 {
		limit = 5
	}

	// Text-based similarity search using CONTAINS and relevance scoring
	// For better results, this can be upgraded to use Neo4j vector search or external embedding API
	searchQuery := `
		MATCH (u:User {id: $userID})-[:HAS_PERSONALITY_MEMORY]->(m:UserPersonalityMemory)
		WHERE m.consented = true
		AND (
			toLower(m.content) CONTAINS toLower($query)
			OR ANY(tag IN m.tags WHERE toLower(tag) CONTAINS toLower($query))
		)
		WITH m, 
		     CASE 
		       WHEN toLower(m.content) CONTAINS toLower($query) THEN 2.0
		       ELSE 1.0
		     END as relevance
		ORDER BY relevance DESC, m.created_at DESC
		LIMIT $limit
		RETURN m.id as id, m.content as content, m.source as source,
		       m.channel_id as channel_id, m.tags as tags,
		       m.consented as consented, m.created_at as created_at
	`

	result, err := session.Run(ctx, searchQuery, map[string]interface{}{
		"userID": userID,
		"query":  query,
		"limit":  limit,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve personality memories: %w", err)
	}

	var memories []UserPersonalityMemory
	for result.Next(ctx) {
		record := result.Record()
		createdAtStr := getStringFromRecord(record, "created_at")
		createdAt, _ := time.Parse(time.RFC3339, createdAtStr)

		memory := UserPersonalityMemory{
			ID:        getStringFromRecord(record, "id"),
			UserID:    userID,
			Content:   getStringFromRecord(record, "content"),
			Source:    getStringFromRecord(record, "source"),
			ChannelID: getStringFromRecord(record, "channel_id"),
			Tags:      getStringSliceFromRecord(record, "tags"),
			Consented: getBoolFromRecord(record, "consented"),
			CreatedAt: createdAt,
		}
		memories = append(memories, memory)
	}

	return memories, nil
}

// GetAllUserPersonalityMemories retrieves all consented memories for a user
func (r *Repository) GetAllUserPersonalityMemories(ctx context.Context, userID string) ([]UserPersonalityMemory, error) {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	query := `
		MATCH (u:User {id: $userID})-[:HAS_PERSONALITY_MEMORY]->(m:UserPersonalityMemory)
		WHERE m.consented = true
		ORDER BY m.created_at DESC
		RETURN m.id as id, m.content as content, m.source as source,
		       m.channel_id as channel_id, m.tags as tags,
		       m.consented as consented, m.created_at as created_at
	`

	result, err := session.Run(ctx, query, map[string]interface{}{
		"userID": userID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve all personality memories: %w", err)
	}

	var memories []UserPersonalityMemory
	for result.Next(ctx) {
		record := result.Record()
		createdAtStr := getStringFromRecord(record, "created_at")
		createdAt, _ := time.Parse(time.RFC3339, createdAtStr)

		memory := UserPersonalityMemory{
			ID:        getStringFromRecord(record, "id"),
			UserID:    userID,
			Content:   getStringFromRecord(record, "content"),
			Source:    getStringFromRecord(record, "source"),
			ChannelID: getStringFromRecord(record, "channel_id"),
			Tags:      getStringSliceFromRecord(record, "tags"),
			Consented: getBoolFromRecord(record, "consented"),
			CreatedAt: createdAt,
		}
		memories = append(memories, memory)
	}

	return memories, nil
}

// DeleteUserPersonalityMemory deletes a personality memory
func (r *Repository) DeleteUserPersonalityMemory(ctx context.Context, memoryID string) error {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	query := `
		MATCH (m:UserPersonalityMemory {id: $memoryID})
		DETACH DELETE m
	`

	_, err := session.Run(ctx, query, map[string]interface{}{
		"memoryID": memoryID,
	})
	if err != nil {
		return fmt.Errorf("failed to delete personality memory: %w", err)
	}

	r.logger.Info("Personality memory deleted",
		zap.String("memory_id", memoryID),
	)

	return nil
}

// ============================================================================
// Personality Profile Caching
// ============================================================================

// StoreUserPersonalityProfile stores a cached personality profile for a user
func (r *Repository) StoreUserPersonalityProfile(ctx context.Context, userID, guildID string, profileJSON string) error {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	now := time.Now().UTC().Format(time.RFC3339)

	// Ensure user exists
	ensureUserQuery := `
		MERGE (u:User {id: $userID})
		ON CREATE SET u.created_at = datetime($now)
		RETURN u.id as id
	`
	_, err := session.Run(ctx, ensureUserQuery, map[string]interface{}{
		"userID": userID,
		"now":    now,
	})
	if err != nil {
		return fmt.Errorf("failed to ensure user exists: %w", err)
	}

	// Store or update the personality profile
	query := `
		MATCH (u:User {id: $userID})
		MERGE (p:UserPersonalityProfile {user_id: $userID, guild_id: $guildID})
		SET p.profile_data = $profileJSON,
		    p.updated_at = datetime($now)
		CREATE (u)-[:HAS_PERSONALITY_PROFILE]->(p)
		RETURN p.user_id as user_id
	`

	_, err = session.Run(ctx, query, map[string]interface{}{
		"userID":      userID,
		"guildID":     guildID,
		"profileJSON": profileJSON,
		"now":         now,
	})
	if err != nil {
		return fmt.Errorf("failed to store personality profile: %w", err)
	}

	r.logger.Info("Personality profile stored",
		zap.String("user_id", userID),
		zap.String("guild_id", guildID),
	)

	return nil
}

// GetUserPersonalityProfile retrieves a cached personality profile for a user
func (r *Repository) GetUserPersonalityProfile(ctx context.Context, userID, guildID string) (string, error) {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	query := `
		MATCH (u:User {id: $userID})-[:HAS_PERSONALITY_PROFILE]->(p:UserPersonalityProfile)
		WHERE p.user_id = $userID AND p.guild_id = $guildID
		RETURN p.profile_data as profile_data, p.updated_at as updated_at
		ORDER BY p.updated_at DESC
		LIMIT 1
	`

	result, err := session.Run(ctx, query, map[string]interface{}{
		"userID":  userID,
		"guildID": guildID,
	})
	if err != nil {
		return "", fmt.Errorf("failed to retrieve personality profile: %w", err)
	}

	if !result.Next(ctx) {
		return "", nil // No cached profile found
	}

	record := result.Record()
	profileData := getStringFromRecord(record, "profile_data")

	return profileData, nil
}

// DeleteUserPersonalityProfile deletes a cached personality profile
func (r *Repository) DeleteUserPersonalityProfile(ctx context.Context, userID, guildID string) error {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	query := `
		MATCH (p:UserPersonalityProfile {user_id: $userID, guild_id: $guildID})
		DETACH DELETE p
	`

	_, err := session.Run(ctx, query, map[string]interface{}{
		"userID":  userID,
		"guildID": guildID,
	})
	if err != nil {
		return fmt.Errorf("failed to delete personality profile: %w", err)
	}

	r.logger.Info("Personality profile deleted",
		zap.String("user_id", userID),
		zap.String("guild_id", guildID),
	)

	return nil
}


