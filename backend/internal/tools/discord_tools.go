package tools

import (
	"ezra-clone/backend/internal/adapter"
)

// GetDiscordTools returns Discord-specific tools
func GetDiscordTools() []adapter.Tool {
	return []adapter.Tool{
		{
			Type: "function",
			Function: adapter.FunctionDefinition{
				Name:        ToolDiscordReadHistory,
				Description: "Read recent message history from a Discord channel. Use this to see what was discussed or to analyze a user's messages for personality mimicking.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"channel_id": map[string]interface{}{
							"type":        "string",
							"description": "Discord channel ID to read from (leave empty for current channel)",
						},
						"limit": map[string]interface{}{
							"type":        "integer",
							"description": "Number of messages to retrieve (default: 50, max: 100)",
						},
						"from_user_id": map[string]interface{}{
							"type":        "string",
							"description": "Only get messages from this specific user ID (for personality analysis)",
						},
					},
					"required": []string{},
				},
			},
		},
		{
			Type: "function",
			Function: adapter.FunctionDefinition{
				Name:        ToolDiscordGetUserInfo,
				Description: "Get information about a Discord user including their username, discriminator, and avatar.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"user_id": map[string]interface{}{
							"type":        "string",
							"description": "Discord user ID",
						},
					},
					"required": []string{"user_id"},
				},
			},
		},
		{
			Type: "function",
			Function: adapter.FunctionDefinition{
				Name:        ToolDiscordGetChannelInfo,
				Description: "Get information about a Discord channel including its name and topic.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"channel_id": map[string]interface{}{
							"type":        "string",
							"description": "Discord channel ID (leave empty for current channel)",
						},
					},
					"required": []string{},
				},
			},
		},
		{
			Type: "function",
			Function: adapter.FunctionDefinition{
				Name:        ToolReadCodebase,
				Description: "Read and search the bot's own codebase intelligently. ADMIN ONLY - Only works in DMs with the authorized admin user. Automatically filters out environment variables and sensitive files. Use this to understand how the codebase works, find specific functions, or analyze code structure.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"query": map[string]interface{}{
							"type":        "string",
							"description": "What to search for in the codebase (e.g., 'how does memory storage work', 'discord message handler', 'function name', 'file path'). Can be a semantic query or specific file/function name.",
						},
						"file_path": map[string]interface{}{
							"type":        "string",
							"description": "Optional: Specific file path to read (e.g., 'backend/internal/tools/discord_executor.go'). If provided, reads the entire file.",
						},
						"max_results": map[string]interface{}{
							"type":        "integer",
							"description": "Maximum number of code snippets to return (default: 5, max: 20)",
						},
					},
					"required": []string{},
				},
			},
		},
	}
}

