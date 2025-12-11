package graph

import (
	"context"
	"fmt"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// ============================================================================
// Search Operations
// ============================================================================

// SearchMemory performs a comprehensive search across the graph
func (r *Repository) SearchMemory(ctx context.Context, agentID, query string, limit int) ([]SearchResult, error) {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	if limit < 1 {
		limit = 10
	}

	// Search across facts, memories, and topics
	searchQuery := `
		MATCH (a:Agent {id: $agentID})
		OPTIONAL MATCH (a)-[:KNOWS_FACT]->(f:Fact)
		WHERE toLower(f.content) CONTAINS toLower($query)
		WITH collect({type: 'fact', id: f.id, content: f.content, score: 1.0}) as facts
		
		OPTIONAL MATCH (a)-[:HAS_MEMORY]->(m:Memory)
		WHERE toLower(m.content) CONTAINS toLower($query) OR toLower(m.name) CONTAINS toLower($query)
		WITH facts, collect({type: 'memory', id: m.name, content: m.content, score: 1.0}) as memories
		
		OPTIONAL MATCH (t:Topic)
		WHERE toLower(t.name) CONTAINS toLower($query) OR toLower(t.description) CONTAINS toLower($query)
		WITH facts, memories, collect({type: 'topic', id: t.id, content: t.name + ': ' + COALESCE(t.description, ''), score: 0.8}) as topics
		
		RETURN facts + memories + topics as results
	`

	result, err := session.Run(ctx, searchQuery, map[string]interface{}{
		"agentID": agentID,
		"query":   query,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to search memory: %w", err)
	}

	var results []SearchResult
	if result.Next(ctx) {
		record := result.Record()
		if resultList, ok := record.Get("results"); ok {
			if items, ok := resultList.([]interface{}); ok {
				for _, item := range items {
					if m, ok := item.(map[string]interface{}); ok {
						content := getStringFromMap(m, "content", "")
						if content != "" {
							results = append(results, SearchResult{
								Type:    getStringFromMap(m, "type", "unknown"),
								ID:      getStringFromMap(m, "id", ""),
								Content: content,
								Score:   getFloat64FromMap(m, "score", 0.0),
							})
						}
					}
				}
			}
		}
	}

	// Limit results
	if len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

