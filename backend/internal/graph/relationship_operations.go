package graph

import (
	"context"
	"fmt"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// ============================================================================
// User-to-User Relationship Operations
// ============================================================================

// RecordUserMention records when a user mentions another user
func (r *Repository) RecordUserMention(ctx context.Context, fromUserID, toUserID, context string) error {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	now := time.Now().UTC().Format(time.RFC3339)

	query := `
		MATCH (u1:User {id: $fromUserID})
		MATCH (u2:User {id: $toUserID})
		MERGE (u1)-[m:MENTIONED]->(u2)
		ON CREATE SET 
			m.count = 1,
			m.first_mentioned = datetime($now),
			m.last_mentioned = datetime($now),
			m.contexts = [$context]
		ON MATCH SET 
			m.count = m.count + 1,
			m.last_mentioned = datetime($now),
			m.contexts = CASE 
				WHEN $context IN m.contexts THEN m.contexts 
				ELSE m.contexts + $context 
			END
	`

	_, err := session.Run(ctx, query, map[string]interface{}{
		"fromUserID": fromUserID,
		"toUserID":   toUserID,
		"context":    context,
		"now":        now,
	})
	if err != nil {
		return fmt.Errorf("failed to record user mention: %w", err)
	}

	return nil
}

// RecordUserReply records when a user replies to another user
func (r *Repository) RecordUserReply(ctx context.Context, fromUserID, toUserID string, responseTime time.Duration) error {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	now := time.Now().UTC().Format(time.RFC3339)
	responseTimeSeconds := int64(responseTime.Seconds())

	query := `
		MATCH (u1:User {id: $fromUserID})
		MATCH (u2:User {id: $toUserID})
		MERGE (u1)-[r:REPLIED_TO]->(u2)
		ON CREATE SET 
			r.count = 1,
			r.first_reply = datetime($now),
			r.last_reply = datetime($now),
			r.total_response_time = $responseTimeSeconds,
			r.avg_response_time = $responseTimeSeconds
		ON MATCH SET 
			r.count = r.count + 1,
			r.last_reply = datetime($now),
			r.total_response_time = r.total_response_time + $responseTimeSeconds,
			r.avg_response_time = r.total_response_time / r.count
	`

	_, err := session.Run(ctx, query, map[string]interface{}{
		"fromUserID":     fromUserID,
		"toUserID":       toUserID,
		"responseTimeSeconds": responseTimeSeconds,
		"now":            now,
	})
	if err != nil {
		return fmt.Errorf("failed to record user reply: %w", err)
	}

	return nil
}

// RecordSharedTopic records when users share interest in topics
func (r *Repository) RecordSharedTopic(ctx context.Context, user1ID, user2ID string, topicNames []string) error {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	now := time.Now().UTC().Format(time.RFC3339)
	strength := float64(len(topicNames)) * 0.1
	if strength > 1.0 {
		strength = 1.0
	}

	query := `
		MATCH (u1:User {id: $user1ID})
		MATCH (u2:User {id: $user2ID})
		MERGE (u1)-[s:SHARED_TOPIC]->(u2)
		ON CREATE SET 
			s.topics = $topicNames,
			s.strength = $strength,
			s.first_shared = datetime($now),
			s.last_shared = datetime($now)
		ON MATCH SET 
			s.topics = CASE 
				WHEN ALL(topic IN $topicNames WHERE topic IN s.topics) THEN s.topics
				ELSE [topic IN s.topics WHERE topic IN $topicNames] + [topic IN $topicNames WHERE NOT topic IN s.topics]
			END,
			s.strength = size(s.topics) * 0.1,
			s.last_shared = datetime($now)
	`

	_, err := session.Run(ctx, query, map[string]interface{}{
		"user1ID":    user1ID,
		"user2ID":    user2ID,
		"topicNames": topicNames,
		"strength":   strength,
		"now":        now,
	})
	if err != nil {
		return fmt.Errorf("failed to record shared topic: %w", err)
	}

	return nil
}

// RecordCollaboration records when users collaborate in conversations
func (r *Repository) RecordCollaboration(ctx context.Context, user1ID, user2ID, conversationID string) error {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	now := time.Now().UTC().Format(time.RFC3339)

	query := `
		MATCH (u1:User {id: $user1ID})
		MATCH (u2:User {id: $user2ID})
		MERGE (u1)-[c:COLLABORATED]->(u2)
		ON CREATE SET 
			c.conversations = [$conversationID],
			c.count = 1,
			c.first_collaboration = datetime($now),
			c.last_collaboration = datetime($now)
		ON MATCH SET 
			c.conversations = CASE 
				WHEN $conversationID IN c.conversations THEN c.conversations
				ELSE c.conversations + $conversationID
			END,
			c.count = size(c.conversations),
			c.last_collaboration = datetime($now)
	`

	_, err := session.Run(ctx, query, map[string]interface{}{
		"user1ID":       user1ID,
		"user2ID":       user2ID,
		"conversationID": conversationID,
		"now":           now,
	})
	if err != nil {
		return fmt.Errorf("failed to record collaboration: %w", err)
	}

	return nil
}

// RecordActivityPattern records user activity patterns
func (r *Repository) RecordActivityPattern(ctx context.Context, userID string, hourOfDay int, messageLength int) error {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	now := time.Now()
	dayOfWeek := now.Weekday().String()
	nowStr := now.UTC().Format(time.RFC3339)

	query := `
		MATCH (u:User {id: $userID})
		MERGE (u)-[:ACTIVE_AT]->(ap:ActivityPattern {
			day_of_week: $dayOfWeek,
			hour_of_day: $hourOfDay
		})
		ON CREATE SET 
			ap.activity_count = 1,
			ap.total_message_length = $messageLength,
			ap.avg_message_length = $messageLength,
			ap.last_updated = datetime($now)
		ON MATCH SET 
			ap.activity_count = ap.activity_count + 1,
			ap.total_message_length = ap.total_message_length + $messageLength,
			ap.avg_message_length = ap.total_message_length / ap.activity_count,
			ap.last_updated = datetime($now)
	`

	_, err := session.Run(ctx, query, map[string]interface{}{
		"userID":       userID,
		"dayOfWeek":    dayOfWeek,
		"hourOfDay":    hourOfDay,
		"messageLength": messageLength,
		"now":          nowStr,
	})
	if err != nil {
		return fmt.Errorf("failed to record activity pattern: %w", err)
	}

	return nil
}

