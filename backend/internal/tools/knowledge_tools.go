package tools

import (
	"ezra-clone/backend/internal/adapter"
)

// GetKnowledgeTools returns fact/knowledge management tools
func GetKnowledgeTools() []adapter.Tool {
	return []adapter.Tool{
		{
			Type: "function",
			Function: adapter.FunctionDefinition{
				Name:        ToolCreateFact,
				Description: "Store a new fact or piece of knowledge. Facts can be linked to users (who told you) and topics. Use this when someone shares information you want to remember.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"content": map[string]interface{}{
							"type":        "string",
							"description": "The fact or information to store",
						},
						"topics": map[string]interface{}{
							"type":        "array",
							"items":       map[string]interface{}{"type": "string"},
							"description": "Topics this fact is about (e.g., ['Hazbin Hotel', 'Animation'])",
						},
						"source": map[string]interface{}{
							"type":        "string",
							"description": "Where this fact came from (optional, will auto-link to current user)",
						},
					},
					"required": []string{"content"},
				},
			},
		},
		{
			Type: "function",
			Function: adapter.FunctionDefinition{
				Name:        ToolSearchFacts,
				Description: "Search for facts you've learned about a specific topic. Use this when asked about facts related to a particular subject (e.g., 'what do you know about pizza?' -> search_facts with topic 'pizza'). For user-specific questions like 'what do I love?', use get_user_context instead.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"topic": map[string]interface{}{
							"type":        "string",
							"description": "The topic to search facts about",
						},
					},
					"required": []string{"topic"},
				},
			},
		},
		{
			Type: "function",
			Function: adapter.FunctionDefinition{
				Name:        ToolGetUserContext,
				Description: "Get comprehensive information about a user including their interests, facts they've shared, preferences, and conversation history. USE THIS when asked 'what do I love?', 'what are my interests?', 'what do you know about me?', or any question about a user's preferences, likes, dislikes, or personal information. This tool returns all facts and topics associated with the user.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"user_id": map[string]interface{}{
							"type":        "string",
							"description": "The user ID to get context for (leave empty for current user)",
						},
					},
					"required": []string{},
				},
			},
		},
	}
}

