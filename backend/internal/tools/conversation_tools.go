package tools

import (
	"ezra-clone/backend/internal/adapter"
)

// GetConversationTools returns conversation management tools
func GetConversationTools() []adapter.Tool {
	return []adapter.Tool{
		{
			Type: "function",
			Function: adapter.FunctionDefinition{
				Name:        ToolGetHistory,
				Description: "Retrieve recent conversation history from a channel.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"channel_id": map[string]interface{}{
							"type":        "string",
							"description": "The channel ID to get history for (leave empty for current channel)",
						},
						"limit": map[string]interface{}{
							"type":        "integer",
							"description": "Number of messages to retrieve (default: 20)",
						},
					},
					"required": []string{},
				},
			},
		},
		{
			Type: "function",
			Function: adapter.FunctionDefinition{
				Name:        ToolSendMessage,
				Description: "Send a message response to the user. Always use this to communicate with the user.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"message": map[string]interface{}{
							"type":        "string",
							"description": "The message to send to the user",
						},
					},
					"required": []string{"message"},
				},
			},
		},
	}
}

