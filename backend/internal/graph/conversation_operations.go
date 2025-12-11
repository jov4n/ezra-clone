package graph

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// ============================================================================
// Conversation Operations
// ============================================================================

// LogMessage logs a message and links it to user and conversation
func (r *Repository) LogMessage(ctx context.Context, agentID, userID, channelID, content, role, platform string) error {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	msgID := uuid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)

	query := `
		MATCH (a:Agent {id: $agentID})
		MERGE (u:User {id: $userID})
		MERGE (c:Conversation {channel_id: $channelID})
		ON CREATE SET c.id = $convID, c.platform = $platform, c.started_at = datetime($now)
		
		CREATE (m:Message {
			id: $msgID,
			content: $content,
			role: $role,
			platform: $platform,
			timestamp: datetime($now)
		})
		
		MERGE (u)-[:PARTICIPATED_IN]->(c)
		MERGE (c)-[:CONTAINS]->(m)
		
		WITH m, u, a
		FOREACH (ignored IN CASE WHEN $role = 'user' THEN [1] ELSE [] END |
			MERGE (u)-[:SENT]->(m)
		)
		FOREACH (ignored IN CASE WHEN $role = 'agent' THEN [1] ELSE [] END |
			MERGE (a)-[:SENT]->(m)
		)
	`

	_, err := session.Run(ctx, query, map[string]interface{}{
		"agentID":   agentID,
		"userID":    userID,
		"channelID": channelID,
		"convID":    uuid.New().String(),
		"msgID":     msgID,
		"content":   content,
		"role":      role,
		"platform":  platform,
		"now":       now,
	})
	if err != nil {
		return fmt.Errorf("failed to log message: %w", err)
	}

	return nil
}

// GetConversationHistory retrieves recent messages from a conversation
func (r *Repository) GetConversationHistory(ctx context.Context, channelID string, limit int) ([]Message, error) {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	if limit < 1 {
		limit = 20
	}

	query := `
		MATCH (c:Conversation {channel_id: $channelID})-[:CONTAINS]->(m:Message)
		RETURN m.id as id, m.content as content, m.role as role, 
		       m.platform as platform, m.timestamp as timestamp
		ORDER BY m.timestamp DESC
		LIMIT $limit
	`

	result, err := session.Run(ctx, query, map[string]interface{}{
		"channelID": channelID,
		"limit":     limit,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get conversation history: %w", err)
	}

	var messages []Message
	for result.Next(ctx) {
		record := result.Record()
		messages = append(messages, Message{
			ID:       getStringFromRecord(record, "id"),
			Content:  getStringFromRecord(record, "content"),
			Role:     getStringFromRecord(record, "role"),
			Platform: getStringFromRecord(record, "platform"),
		})
	}

	// Reverse to get chronological order
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}

	return messages, nil
}

// LogMessageWithThreading logs a message with threading support
func (r *Repository) LogMessageWithThreading(ctx context.Context, agentID, userID, channelID, content, role, platform string, replyToMessageID string, mentionedUserIDs []string) error {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	msgID := uuid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)

	query := `
		MATCH (a:Agent {id: $agentID})
		MERGE (u:User {id: $userID})
		MERGE (c:Conversation {channel_id: $channelID})
		ON CREATE SET c.id = $convID, c.platform = $platform, c.started_at = datetime($now)
		
		CREATE (m:Message {
			id: $msgID,
			content: $content,
			role: $role,
			platform: $platform,
			timestamp: datetime($now)
		})
		
		MERGE (u)-[:PARTICIPATED_IN]->(c)
		MERGE (c)-[:CONTAINS]->(m)
		
		WITH m, u, a
		FOREACH (ignored IN CASE WHEN $role = 'user' THEN [1] ELSE [] END |
			MERGE (u)-[:SENT]->(m)
		)
		FOREACH (ignored IN CASE WHEN $role = 'agent' THEN [1] ELSE [] END |
			MERGE (a)-[:SENT]->(m)
		)
		
		WITH m
		FOREACH (ignored IN CASE WHEN $replyToMessageID <> '' THEN [1] ELSE [] END |
			OPTIONAL MATCH (replyTo:Message {id: $replyToMessageID})
			FOREACH (ignored2 IN CASE WHEN replyTo IS NOT NULL THEN [1] ELSE [] END |
				MERGE (m)-[:REPLIES_TO]->(replyTo)
			)
		)
		
		WITH m
		FOREACH (mentionedID IN $mentionedUserIDs |
			OPTIONAL MATCH (mentioned:User {id: mentionedID})
			FOREACH (ignored IN CASE WHEN mentioned IS NOT NULL THEN [1] ELSE [] END |
				MERGE (m)-[:MENTIONS]->(mentioned)
			)
		)
	`

	_, err := session.Run(ctx, query, map[string]interface{}{
		"agentID":          agentID,
		"userID":           userID,
		"channelID":        channelID,
		"convID":           uuid.New().String(),
		"msgID":            msgID,
		"content":          content,
		"role":             role,
		"platform":         platform,
		"replyToMessageID": replyToMessageID,
		"mentionedUserIDs": mentionedUserIDs,
		"now":              now,
	})
	if err != nil {
		return fmt.Errorf("failed to log message with threading: %w", err)
	}

	// Record mentions
	if role == "user" && len(mentionedUserIDs) > 0 {
		for _, mentionedID := range mentionedUserIDs {
			if mentionedID != userID {
				_ = r.RecordUserMention(ctx, userID, mentionedID, channelID)
			}
		}
	}

	return nil
}

