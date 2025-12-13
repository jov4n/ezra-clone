package tools

import (
	"ezra-clone/backend/internal/adapter"
)

// GetGitHubTools returns GitHub-related tools
func GetGitHubTools() []adapter.Tool {
	return []adapter.Tool{
		{
			Type: "function",
			Function: adapter.FunctionDefinition{
				Name:        ToolGitHubRepoInfo,
				Description: "Get information about a GitHub repository including description, stars, language, and recent activity.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"owner": map[string]interface{}{
							"type":        "string",
							"description": "Repository owner (username or organization)",
						},
						"repo": map[string]interface{}{
							"type":        "string",
							"description": "Repository name",
						},
					},
					"required": []string{"owner", "repo"},
				},
			},
		},
		{
			Type: "function",
			Function: adapter.FunctionDefinition{
				Name:        ToolGitHubSearch,
				Description: "Search GitHub for repositories, code, issues, or users. Use 'org:orgname' in query to search within an organization. Use 'sort:updated' to get most recently updated.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"query": map[string]interface{}{
							"type":        "string",
							"description": "Search query. Examples: 'org:microsoft sort:updated' for org repos, 'react hooks' for general search",
						},
						"type": map[string]interface{}{
							"type":        "string",
							"enum":        []string{"repositories", "code", "issues", "users"},
							"description": "What to search for (default: repositories)",
						},
						"limit": map[string]interface{}{
							"type":        "integer",
							"description": "Number of results (default: 5)",
						},
					},
					"required": []string{"query"},
				},
			},
		},
		{
			Type: "function",
			Function: adapter.FunctionDefinition{
				Name:        ToolGitHubListOrgRepos,
				Description: "List repositories for a GitHub organization, sorted by most recently updated. USE THIS when someone asks about an org's repos or 'what was last updated'.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"org": map[string]interface{}{
							"type":        "string",
							"description": "GitHub organization name (e.g., 'microsoft', 'system-nebula')",
						},
						"limit": map[string]interface{}{
							"type":        "integer",
							"description": "Number of repos to return (default: 5)",
						},
					},
					"required": []string{"org"},
				},
			},
		},
		{
			Type: "function",
			Function: adapter.FunctionDefinition{
				Name:        ToolGitHubReadFile,
				Description: "Read a file from a GitHub repository.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"owner": map[string]interface{}{
							"type":        "string",
							"description": "Repository owner",
						},
						"repo": map[string]interface{}{
							"type":        "string",
							"description": "Repository name",
						},
						"path": map[string]interface{}{
							"type":        "string",
							"description": "Path to the file (e.g., 'README.md', 'src/main.go')",
						},
						"branch": map[string]interface{}{
							"type":        "string",
							"description": "Branch name (default: main)",
						},
					},
					"required": []string{"owner", "repo", "path"},
				},
			},
		},
	}
}

