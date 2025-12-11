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
// Topic Operations
// ============================================================================

// CreateTopic creates a new topic
func (r *Repository) CreateTopic(ctx context.Context, name, description string) (*Topic, error) {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	topicID := uuid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)

	query := `
		MERGE (t:Topic {name: $name})
		ON CREATE SET t.id = $topicID, t.description = $description, t.created_at = datetime($now)
		ON MATCH SET t.description = CASE WHEN $description <> '' THEN $description ELSE t.description END
		RETURN t.id as id, t.name as name, t.description as description
	`

	result, err := session.Run(ctx, query, map[string]interface{}{
		"name":        name,
		"description": description,
		"topicID":     topicID,
		"now":         now,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create topic: %w", err)
	}

	if result.Next(ctx) {
		record := result.Record()
		return &Topic{
			ID:          getStringFromRecord(record, "id"),
			Name:        getStringFromRecord(record, "name"),
			Description: getStringFromRecord(record, "description"),
		}, nil
	}

	return nil, fmt.Errorf("failed to create topic")
}

// LinkTopics creates a relationship between two topics
func (r *Repository) LinkTopics(ctx context.Context, topic1, topic2, relationship string) error {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	// Sanitize relationship type
	if relationship == "" {
		relationship = "RELATED_TO"
	}

	query := fmt.Sprintf(`
		MATCH (t1:Topic {name: $topic1})
		MATCH (t2:Topic {name: $topic2})
		MERGE (t1)-[:%s]->(t2)
	`, relationship)

	_, err := session.Run(ctx, query, map[string]interface{}{
		"topic1": topic1,
		"topic2": topic2,
	})
	if err != nil {
		return fmt.Errorf("failed to link topics: %w", err)
	}

	r.logger.Info("Topics linked",
		zap.String("topic1", topic1),
		zap.String("topic2", topic2),
		zap.String("relationship", relationship),
	)

	return nil
}

// LinkUserToTopic links a user's interest to a topic
func (r *Repository) LinkUserToTopic(ctx context.Context, userID, topicName string) error {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	query := `
		MATCH (u:User {id: $userID})
		MERGE (t:Topic {name: $topicName})
		ON CREATE SET t.id = $topicID, t.created_at = datetime($now)
		MERGE (u)-[:INTERESTED_IN]->(t)
	`

	now := time.Now().UTC().Format(time.RFC3339)
	_, err := session.Run(ctx, query, map[string]interface{}{
		"userID":    userID,
		"topicName": topicName,
		"topicID":   uuid.New().String(),
		"now":       now,
	})
	if err != nil {
		return fmt.Errorf("failed to link user to topic: %w", err)
	}

	return nil
}

// GetRelatedTopics finds topics related to a given topic
func (r *Repository) GetRelatedTopics(ctx context.Context, topicName string, depth int) ([]Topic, error) {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	if depth < 1 {
		depth = 1
	}
	if depth > 5 {
		depth = 5
	}

	query := fmt.Sprintf(`
		MATCH (t:Topic {name: $topicName})-[:RELATED_TO|SUBTOPIC_OF*1..%d]-(related:Topic)
		WHERE related.name <> $topicName
		RETURN DISTINCT related.id as id, related.name as name, related.description as description
		LIMIT 20
	`, depth)

	result, err := session.Run(ctx, query, map[string]interface{}{
		"topicName": topicName,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get related topics: %w", err)
	}

	var topics []Topic
	for result.Next(ctx) {
		record := result.Record()
		topics = append(topics, Topic{
			ID:          getStringFromRecord(record, "id"),
			Name:        getStringFromRecord(record, "name"),
			Description: getStringFromRecord(record, "description"),
		})
	}

	return topics, nil
}

// LinkUserToTopicWeighted links a user to a topic with weighted relationship
func (r *Repository) LinkUserToTopicWeighted(ctx context.Context, userID, topicName string, strength float64) error {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	now := time.Now().UTC().Format(time.RFC3339)

	if strength < 0 {
		strength = 0
	}
	if strength > 1.0 {
		strength = 1.0
	}

	query := `
		MATCH (u:User {id: $userID})
		MERGE (t:Topic {name: $topicName})
		ON CREATE SET t.id = $topicID, t.created_at = datetime($now)
		MERGE (u)-[r:INTERESTED_IN]->(t)
		ON CREATE SET 
			r.strength = $strength,
			r.first_interaction = datetime($now),
			r.last_interaction = datetime($now),
			r.interaction_count = 1,
			r.recency_score = 1.0
		ON MATCH SET 
			r.strength = ($strength + r.strength) / 2.0,
			r.last_interaction = datetime($now),
			r.interaction_count = r.interaction_count + 1,
			r.recency_score = 1.0 - (duration.between(r.last_interaction, datetime($now)).days / 365.0)
	`

	_, err := session.Run(ctx, query, map[string]interface{}{
		"userID":    userID,
		"topicName": topicName,
		"topicID":   uuid.New().String(),
		"strength":  strength,
		"now":       now,
	})
	if err != nil {
		return fmt.Errorf("failed to link user to topic: %w", err)
	}

	return nil
}

