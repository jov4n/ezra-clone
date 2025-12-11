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
// Fact Operations
// ============================================================================

// CreateFact creates a new fact and links it to the agent and optionally a user/topic
func (r *Repository) CreateFact(ctx context.Context, agentID, content, source, userID string, topicNames []string) (*Fact, error) {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	factID := uuid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)

	// Create the fact and link to agent
	query := `
		MATCH (a:Agent {id: $agentID})
		CREATE (f:Fact {
			id: $factID,
			content: $content,
			source: $source,
			confidence: 1.0,
			created_at: datetime($now)
		})
		CREATE (a)-[:KNOWS_FACT]->(f)
		RETURN f.id as id
	`

	_, err := session.Run(ctx, query, map[string]interface{}{
		"agentID": agentID,
		"factID":  factID,
		"content": content,
		"source":  source,
		"now":     now,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create fact: %w", err)
	}

	// Link to user if provided
	if userID != "" {
		linkQuery := `
			MATCH (f:Fact {id: $factID})
			MATCH (u:User {id: $userID})
			MERGE (u)-[:TOLD_ME]->(f)
		`
		_, _ = session.Run(ctx, linkQuery, map[string]interface{}{
			"factID": factID,
			"userID": userID,
		})
	}

	// Link to topics
	for _, topicName := range topicNames {
		if topicName == "" {
			continue
		}
		topicQuery := `
			MATCH (f:Fact {id: $factID})
			MERGE (t:Topic {name: $topicName})
			ON CREATE SET t.id = $topicID, t.created_at = datetime($now)
			MERGE (f)-[:ABOUT]->(t)
		`
		_, _ = session.Run(ctx, topicQuery, map[string]interface{}{
			"factID":    factID,
			"topicName": topicName,
			"topicID":   uuid.New().String(),
			"now":       now,
		})
	}

	r.logger.Info("Fact created",
		zap.String("fact_id", factID),
		zap.String("agent_id", agentID),
		zap.String("source", source),
	)

	return &Fact{
		ID:        factID,
		Content:   content,
		Source:    source,
		CreatedAt: time.Now(),
	}, nil
}

// GetFactsAboutTopic retrieves all facts about a topic
func (r *Repository) GetFactsAboutTopic(ctx context.Context, topicName string) ([]Fact, error) {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	query := `
		MATCH (f:Fact)-[:ABOUT]->(t:Topic)
		WHERE toLower(t.name) CONTAINS toLower($topicName)
		OPTIONAL MATCH (u:User)-[:TOLD_ME]->(f)
		RETURN f.id as id, f.content as content, f.source as source, 
		       f.confidence as confidence, f.created_at as created_at,
		       u.discord_username as told_by
		ORDER BY f.created_at DESC
		LIMIT 20
	`

	result, err := session.Run(ctx, query, map[string]interface{}{
		"topicName": topicName,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get facts: %w", err)
	}

	var facts []Fact
	for result.Next(ctx) {
		record := result.Record()
		fact := Fact{
			ID:      getStringFromRecord(record, "id"),
			Content: getStringFromRecord(record, "content"),
			Source:  getStringFromRecord(record, "source"),
		}
		if toldBy := getStringFromRecord(record, "told_by"); toldBy != "" {
			fact.Source = fmt.Sprintf("Told by %s", toldBy)
		}
		facts = append(facts, fact)
	}

	return facts, nil
}

// UpdateFact updates the content of an existing fact
func (r *Repository) UpdateFact(ctx context.Context, factID, newContent string) error {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	now := time.Now().UTC().Format(time.RFC3339)

	query := `
		MATCH (f:Fact {id: $factID})
		SET f.content = $newContent,
		    f.updated_at = datetime($now)
		RETURN f.id as id
	`

	result, err := session.Run(ctx, query, map[string]interface{}{
		"factID":    factID,
		"newContent": newContent,
		"now":       now,
	})
	if err != nil {
		return fmt.Errorf("failed to update fact: %w", err)
	}

	if !result.Next(ctx) {
		return fmt.Errorf("fact not found: %s", factID)
	}

	r.logger.Info("Fact updated",
		zap.String("fact_id", factID),
	)
	return nil
}

// DeleteFact deletes a fact by ID
func (r *Repository) DeleteFact(ctx context.Context, factID string) error {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	query := `
		MATCH (f:Fact {id: $factID})
		DETACH DELETE f
	`

	_, err := session.Run(ctx, query, map[string]interface{}{
		"factID": factID,
	})
	if err != nil {
		return fmt.Errorf("failed to delete fact: %w", err)
	}

	r.logger.Info("Fact deleted",
		zap.String("fact_id", factID),
	)
	return nil
}

// LinkFactRelationships links facts with support/contradict/related relationships
func (r *Repository) LinkFactRelationships(ctx context.Context, fact1ID, fact2ID, relationship string) error {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	// Validate relationship type
	validRelationships := map[string]bool{
		"SUPPORTS":    true,
		"CONTRADICTS": true,
		"RELATED_TO":  true,
	}
	if !validRelationships[relationship] {
		relationship = "RELATED_TO"
	}

	query := fmt.Sprintf(`
		MATCH (f1:Fact {id: $fact1ID})
		MATCH (f2:Fact {id: $fact2ID})
		MERGE (f1)-[r:%s]->(f2)
		ON CREATE SET r.created_at = datetime()
		ON MATCH SET r.last_updated = datetime()
	`, relationship)

	_, err := session.Run(ctx, query, map[string]interface{}{
		"fact1ID": fact1ID,
		"fact2ID": fact2ID,
	})
	if err != nil {
		return fmt.Errorf("failed to link fact relationships: %w", err)
	}

	return nil
}

// RecordFactVerification records when a user verifies or challenges a fact
func (r *Repository) RecordFactVerification(ctx context.Context, factID, userID string, verified bool) error {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	now := time.Now().UTC().Format(time.RFC3339)
	relType := "VERIFIED_BY"
	if !verified {
		relType = "CHALLENGED_BY"
	}

	query := fmt.Sprintf(`
		MATCH (f:Fact {id: $factID})
		MATCH (u:User {id: $userID})
		MERGE (f)<-[r:%s]-(u)
		ON CREATE SET 
			r.count = 1,
			r.first_%s = datetime($now),
			r.last_%s = datetime($now)
		ON MATCH SET 
			r.count = r.count + 1,
			r.last_%s = datetime($now)
	`, relType, relType, relType, relType)

	_, err := session.Run(ctx, query, map[string]interface{}{
		"factID": factID,
		"userID": userID,
		"now":    now,
	})
	if err != nil {
		return fmt.Errorf("failed to record fact verification: %w", err)
	}

	return nil
}

