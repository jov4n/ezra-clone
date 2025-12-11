package graph

import (
	"context"
	"fmt"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// ============================================================================
// Similarity and Recommendation Operations
// ============================================================================

// CalculateUserSimilarity calculates similarity between users based on shared interests
func (r *Repository) CalculateUserSimilarity(ctx context.Context, user1ID, user2ID string) (*UserSimilarity, error) {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	query := `
		MATCH (u1:User {id: $user1ID})
		MATCH (u2:User {id: $user2ID})
		
		OPTIONAL MATCH (u1)-[:INTERESTED_IN]->(t:Topic)<-[:INTERESTED_IN]-(u2)
		WITH u1, u2, collect(DISTINCT t.name) as shared_topics
		
		OPTIONAL MATCH (u1)-[:TOLD_ME]->(f:Fact)<-[:TOLD_ME]-(u2)
		WITH u1, u2, shared_topics, count(DISTINCT f) as shared_facts
		
		OPTIONAL MATCH (u1)-[:PARTICIPATED_IN]->(c:Conversation)<-[:PARTICIPATED_IN]-(u2)
		WITH u1, u2, shared_topics, shared_facts, count(DISTINCT c) as shared_conversations
		
		WITH u1, u2, shared_topics, shared_facts, shared_conversations,
		     (size(shared_topics) * 0.4 + shared_facts * 0.3 + shared_conversations * 0.3) as similarity
		
		RETURN similarity, shared_topics, shared_facts, shared_conversations
	`

	result, err := session.Run(ctx, query, map[string]interface{}{
		"user1ID": user1ID,
		"user2ID": user2ID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to calculate similarity: %w", err)
	}

	if result.Next(ctx) {
		record := result.Record()
		similarity := getFloat64FromRecord(record, "similarity")
		sharedTopics := getStringSliceFromRecord(record, "shared_topics")
		
		basedOn := "topics"
		if len(sharedTopics) == 0 {
			basedOn = "behavior"
		}

		return &UserSimilarity{
			User1ID:        user1ID,
			User2ID:        user2ID,
			SimilarityScore: similarity,
			BasedOn:        basedOn,
			SharedItems:    sharedTopics,
		}, nil
	}

	return nil, fmt.Errorf("users not found")
}

// FindSimilarUsers finds users similar to a given user
func (r *Repository) FindSimilarUsers(ctx context.Context, userID string, limit int) ([]UserSimilarity, error) {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	if limit < 1 {
		limit = 10
	}

	query := `
		MATCH (u1:User {id: $userID})
		MATCH (u2:User)
		WHERE u1 <> u2
		
		OPTIONAL MATCH (u1)-[:INTERESTED_IN]->(t:Topic)<-[:INTERESTED_IN]-(u2)
		WITH u1, u2, collect(DISTINCT t.name) as shared_topics
		
		OPTIONAL MATCH (u1)-[:TOLD_ME]->(f:Fact)<-[:TOLD_ME]-(u2)
		WITH u1, u2, shared_topics, count(DISTINCT f) as shared_facts
		
		OPTIONAL MATCH (u1)-[:PARTICIPATED_IN]->(c:Conversation)<-[:PARTICIPATED_IN]-(u2)
		WITH u1, u2, shared_topics, shared_facts, count(DISTINCT c) as shared_conversations
		
		WITH u1, u2, shared_topics, shared_facts, shared_conversations,
		     (size(shared_topics) * 0.4 + shared_facts * 0.3 + shared_conversations * 0.3) as similarity
		
		WHERE similarity > 0
		RETURN u2.id as user2_id, similarity, shared_topics
		ORDER BY similarity DESC
		LIMIT $limit
	`

	result, err := session.Run(ctx, query, map[string]interface{}{
		"userID": userID,
		"limit":  limit,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to find similar users: %w", err)
	}

	var similarities []UserSimilarity
	for result.Next(ctx) {
		record := result.Record()
		sharedTopics := getStringSliceFromRecord(record, "shared_topics")
		
		similarities = append(similarities, UserSimilarity{
			User1ID:        userID,
			User2ID:        getStringFromRecord(record, "user2_id"),
			SimilarityScore: getFloat64FromRecord(record, "similarity"),
			BasedOn:        "topics",
			SharedItems:    sharedTopics,
		})
	}

	return similarities, nil
}

// GetRecommendationsForUser gets conversation/topic recommendations for a user
func (r *Repository) GetRecommendationsForUser(ctx context.Context, userID string, limit int) ([]SearchResult, error) {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	if limit < 1 {
		limit = 10
	}

	query := `
		MATCH (u:User {id: $userID})-[:INTERESTED_IN]->(t:Topic)
		OPTIONAL MATCH (t)<-[:ABOUT_TOPIC]-(m:Message)
		WHERE NOT (u)-[:PARTICIPATED_IN]->(m)
		WITH t, m, count(DISTINCT m) as relevance
		ORDER BY relevance DESC, m.timestamp DESC
		LIMIT $limit
		RETURN 
			'conversation' as type,
			m.id as id,
			m.content as content,
			relevance * 0.1 as score,
			{topic: t.name, timestamp: m.timestamp} as metadata
	`

	result, err := session.Run(ctx, query, map[string]interface{}{
		"userID": userID,
		"limit":  limit,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get recommendations: %w", err)
	}

	var results []SearchResult
	for result.Next(ctx) {
		record := result.Record()
		results = append(results, SearchResult{
			Type:    getStringFromRecord(record, "type"),
			ID:      getStringFromRecord(record, "id"),
			Content: getStringFromRecord(record, "content"),
			Score:   getFloat64FromRecord(record, "score"),
		})
	}

	return results, nil
}

