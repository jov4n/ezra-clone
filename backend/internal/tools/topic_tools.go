package tools

import (
	"ezra-clone/backend/internal/adapter"
)

// GetTopicTools returns topic management tools
func GetTopicTools() []adapter.Tool {
	return []adapter.Tool{
		{
			Type: "function",
			Function: adapter.FunctionDefinition{
				Name:        ToolCreateTopic,
				Description: "Create a new topic/subject to organize knowledge around.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"name": map[string]interface{}{
							"type":        "string",
							"description": "The topic name (e.g., 'Hazbin Hotel', 'Machine Learning', 'Gaming')",
						},
						"description": map[string]interface{}{
							"type":        "string",
							"description": "Optional description of the topic",
						},
					},
					"required": []string{"name"},
				},
			},
		},
		{
			Type: "function",
			Function: adapter.FunctionDefinition{
				Name:        ToolLinkTopics,
				Description: "Create a relationship between two topics (e.g., 'Animation' is related to 'Hazbin Hotel').",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"topic1": map[string]interface{}{
							"type":        "string",
							"description": "First topic name",
						},
						"topic2": map[string]interface{}{
							"type":        "string",
							"description": "Second topic name",
						},
						"relationship": map[string]interface{}{
							"type":        "string",
							"enum":        []string{"RELATED_TO", "SUBTOPIC_OF", "PART_OF"},
							"description": "Type of relationship between topics",
						},
					},
					"required": []string{"topic1", "topic2"},
				},
			},
		},
		{
			Type: "function",
			Function: adapter.FunctionDefinition{
				Name:        ToolFindRelated,
				Description: "Find topics related to a given topic using graph traversal.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"topic": map[string]interface{}{
							"type":        "string",
							"description": "The topic to find related topics for",
						},
						"depth": map[string]interface{}{
							"type":        "integer",
							"description": "How many relationship hops to traverse (1-5, default: 2)",
						},
					},
					"required": []string{"topic"},
				},
			},
		},
		{
			Type: "function",
			Function: adapter.FunctionDefinition{
				Name:        ToolLinkUserTopic,
				Description: "Record that a user is interested in a topic.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"user_id": map[string]interface{}{
							"type":        "string",
							"description": "User ID (leave empty for current user)",
						},
						"topic": map[string]interface{}{
							"type":        "string",
							"description": "Topic the user is interested in",
						},
					},
					"required": []string{"topic"},
				},
			},
		},
	}
}

