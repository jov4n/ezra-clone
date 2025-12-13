package tools

import (
	"ezra-clone/backend/internal/adapter"
)

// GetMemoryTools returns memory-related tools
func GetMemoryTools() []adapter.Tool {
	return []adapter.Tool{
		{
			Type: "function",
			Function: adapter.FunctionDefinition{
				Name:        ToolCoreMemoryInsert,
				Description: "Create a new core memory block. Use this to store important information, facts, or preferences that you want to remember permanently. Core memories are always available in your context.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"name": map[string]interface{}{
							"type":        "string",
							"description": "A unique name/label for this memory block (e.g., 'user_preferences', 'hazbin_hotel', 'project_details')",
						},
						"content": map[string]interface{}{
							"type":        "string",
							"description": "The content to store in this memory block",
						},
					},
					"required": []string{"name", "content"},
				},
			},
		},
		{
			Type: "function",
			Function: adapter.FunctionDefinition{
				Name:        ToolCoreMemoryReplace,
				Description: "Replace/update the content of an existing core memory block. Use this to modify information you've already stored.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"name": map[string]interface{}{
							"type":        "string",
							"description": "The name of the existing memory block to update",
						},
						"content": map[string]interface{}{
							"type":        "string",
							"description": "The new content to replace the existing content with",
						},
					},
					"required": []string{"name", "content"},
				},
			},
		},
		{
			Type: "function",
			Function: adapter.FunctionDefinition{
				Name:        ToolArchivalInsert,
				Description: "Insert information into archival memory for long-term storage. Archival memory is searchable but not always in context.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"content": map[string]interface{}{
							"type":        "string",
							"description": "The information to archive",
						},
						"tags": map[string]interface{}{
							"type":        "array",
							"items":       map[string]interface{}{"type": "string"},
							"description": "Optional tags to help categorize and search this information",
						},
					},
					"required": []string{"content"},
				},
			},
		},
		{
			Type: "function",
			Function: adapter.FunctionDefinition{
				Name:        ToolArchivalSearch,
				Description: "Search your archival memory for relevant information.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"query": map[string]interface{}{
							"type":        "string",
							"description": "The search query to find relevant archived information",
						},
						"limit": map[string]interface{}{
							"type":        "integer",
							"description": "Maximum number of results to return (default: 10)",
						},
					},
					"required": []string{"query"},
				},
			},
		},
		{
			Type: "function",
			Function: adapter.FunctionDefinition{
				Name:        ToolMemorySearch,
				Description: "Search across all your memories (core, archival, facts, topics) for relevant information.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"query": map[string]interface{}{
							"type":        "string",
							"description": "The search query",
						},
						"limit": map[string]interface{}{
							"type":        "integer",
							"description": "Maximum number of results (default: 10)",
						},
					},
					"required": []string{"query"},
				},
			},
		},
	}
}

