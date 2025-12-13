package tools

import (
	"ezra-clone/backend/internal/adapter"
)

// GetPersonalityTools returns personality mimicking tools
func GetPersonalityTools() []adapter.Tool {
	return []adapter.Tool{
		{
			Type: "function",
			Function: adapter.FunctionDefinition{
				Name:        ToolMimicPersonality,
				Description: "Analyze a Discord user's message history and mimic their personality, speech patterns, vocabulary, and style. This will change how you communicate until reverted. CRITICAL: Check if you are already in mimic mode before calling this tool. If you are already mimicking someone (check the system prompt for 'PERSONALITY MIMIC MODE ACTIVE'), DO NOT call this tool - just respond to the user's message naturally in the mimicked style. Only call this tool when explicitly asked to START mimicking someone new.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"user_id": map[string]interface{}{
							"type":        "string",
							"description": "Discord user ID to mimic",
						},
						"username": map[string]interface{}{
							"type":        "string",
							"description": "Username of the person being mimicked (for reference)",
						},
						"channel_id": map[string]interface{}{
							"type":        "string",
							"description": "Channel ID to analyze messages from (leave empty for current)",
						},
						"message_count": map[string]interface{}{
							"type":        "integer",
							"description": "Number of messages to analyze (default: 300, more = better accuracy)",
						},
						"update": map[string]interface{}{
							"type":        "boolean",
							"description": "Force update the personality profile even if a cached version exists (default: false, uses cache if available)",
						},
					},
					"required": []string{"user_id"},
				},
			},
		},
		{
			Type: "function",
			Function: adapter.FunctionDefinition{
				Name:        ToolRevertPersonality,
				Description: "Stop mimicking and revert back to your original Ezra personality.",
				Parameters: map[string]interface{}{
					"type":       "object",
					"properties": map[string]interface{}{},
					"required":   []string{},
				},
			},
		},
		{
			Type: "function",
			Function: adapter.FunctionDefinition{
				Name:        ToolAnalyzeUserStyle,
				Description: "Analyze a user's communication style without mimicking. Returns insights about their vocabulary, tone, emoji usage, and speech patterns.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"user_id": map[string]interface{}{
							"type":        "string",
							"description": "Discord user ID to analyze",
						},
						"channel_id": map[string]interface{}{
							"type":        "string",
							"description": "Channel to analyze messages from",
						},
					},
					"required": []string{"user_id"},
				},
			},
		},
	}
}

