package tools

import (
	"ezra-clone/backend/internal/adapter"
)

// GetSystemTools returns system control tools
func GetSystemTools() []adapter.Tool {
	return []adapter.Tool{
		{
			Type: "function",
			Function: adapter.FunctionDefinition{
				Name:        ToolBotShutdown,
				Description: "Shutdown the bot gracefully. Sends a goodbye message and then shuts down the bot. Use this when the user asks to shutdown, disconnect, or say goodnight.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"message": map[string]interface{}{
							"type":        "string",
							"description": "Optional goodbye message to send before shutting down (default: 'Good night! 🌙')",
						},
					},
					"required": []string{},
				},
			},
		},
	}
}

