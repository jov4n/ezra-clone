package tools

import (
	"ezra-clone/backend/internal/adapter"
)

// Tool names - Memory Tools
const (
	ToolCoreMemoryInsert  = "core_memory_insert"
	ToolCoreMemoryReplace = "core_memory_replace"
	ToolArchivalInsert    = "archival_memory_insert"
	ToolArchivalSearch    = "archival_memory_search"
	ToolMemorySearch      = "memory_search"
)

// Tool names - Fact & Knowledge Tools
const (
	ToolCreateFact     = "create_fact"
	ToolSearchFacts    = "search_facts"
	ToolLinkToUser     = "link_fact_to_user"
	ToolGetUserContext = "get_user_context"
)

// Tool names - Topic Tools
const (
	ToolCreateTopic    = "create_topic"
	ToolLinkTopics     = "link_topics"
	ToolFindRelated    = "find_related_topics"
	ToolLinkUserTopic  = "link_user_to_topic"
)

// Tool names - Conversation Tools
const (
	ToolGetHistory     = "get_conversation_history"
	ToolSendMessage    = "send_message"
)

// Tool names - Web & External Tools
const (
	ToolWebSearch        = "web_search"
	ToolFetchWebpage     = "fetch_webpage"
	ToolGitHubRepoInfo   = "github_repo_info"
	ToolGitHubSearch     = "github_search"
	ToolGitHubReadFile   = "github_read_file"
	ToolGitHubListOrgRepos = "github_list_org_repos"
)

// Tool names - Discord Tools
const (
	ToolDiscordReadHistory  = "discord_read_history"
	ToolDiscordGetUserInfo  = "discord_get_user_info"
	ToolDiscordSearchMessages = "discord_search_messages"
	ToolDiscordGetChannelInfo = "discord_get_channel_info"
)

// GetAllTools returns all available tools for the agent
func GetAllTools() []adapter.Tool {
	tools := []adapter.Tool{}
	
	// Memory Tools
	tools = append(tools, GetMemoryTools()...)
	
	// Knowledge Tools
	tools = append(tools, GetKnowledgeTools()...)
	
	// Topic Tools
	tools = append(tools, GetTopicTools()...)
	
	// Conversation Tools
	tools = append(tools, GetConversationTools()...)
	
	// Web & External Tools
	tools = append(tools, GetWebTools()...)
	
	// GitHub Tools
	tools = append(tools, GetGitHubTools()...)
	
	// Discord Tools
	tools = append(tools, GetDiscordTools()...)
	
	// Personality/Mimic Tools
	tools = append(tools, GetPersonalityTools()...)
	
	// Image Generation Tools
	tools = append(tools, GetImageGenerationTools()...)
	
	return tools
}

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

// GetWebTools returns web browsing/search tools
func GetWebTools() []adapter.Tool {
	return []adapter.Tool{
		{
			Type: "function",
			Function: adapter.FunctionDefinition{
				Name:        ToolWebSearch,
				Description: "Search the web for current information. IMPORTANT: Rewrite the user's question into an optimized search query with relevant keywords. Include the current month/year for time-sensitive queries. Example: 'what's happening with AI?' becomes 'artificial intelligence news [current month] [current year]'",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"query": map[string]interface{}{
							"type":        "string",
							"description": "An optimized search query with keywords (NOT the user's exact question). Use specific terms, add year if relevant, remove filler words.",
						},
						"original_question": map[string]interface{}{
							"type":        "string",
							"description": "The user's original question (for context in the response)",
						},
					},
					"required": []string{"query"},
				},
			},
		},
		{
			Type: "function",
			Function: adapter.FunctionDefinition{
				Name:        ToolFetchWebpage,
				Description: "Fetch and read the content of a webpage. USE THIS when a user asks 'what's on this page?', 'tell me about this URL', 'read this page', or provides any URL. Extract the text content and summarize it for the user.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"url": map[string]interface{}{
							"type":        "string",
							"description": "The URL to fetch (can be http:// or https://)",
						},
						"extract_text": map[string]interface{}{
							"type":        "boolean",
							"description": "Whether to extract just text content (default: true)",
						},
					},
					"required": []string{"url"},
				},
			},
		},
	}
}

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
	}
}

// Tool names - Personality/Mimic Tools
const (
	ToolMimicPersonality   = "mimic_personality"
	ToolRevertPersonality  = "revert_personality"
	ToolAnalyzeUserStyle   = "analyze_user_style"
)

// Tool names - ComfyUI Image Generation Tools
const (
	ToolGenerateImageWithRunPod = "generate_image_with_runpod"
	ToolEnhancePrompt           = "enhance_prompt"
	ToolSelectWorkflow          = "select_workflow"
	ToolListWorkflows           = "list_workflows"
)

// GetPersonalityTools returns personality mimicking tools
func GetPersonalityTools() []adapter.Tool {
	return []adapter.Tool{
		{
			Type: "function",
			Function: adapter.FunctionDefinition{
				Name:        ToolMimicPersonality,
				Description: "Analyze a Discord user's message history and mimic their personality, speech patterns, vocabulary, and style. This will change how you communicate until reverted.",
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
							"description": "Number of messages to analyze (default: 50, more = better accuracy)",
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

// GetImageGenerationTools returns ComfyUI image generation tools
func GetImageGenerationTools() []adapter.Tool {
	return []adapter.Tool{
		{
			Type: "function",
			Function: adapter.FunctionDefinition{
				Name:        ToolGenerateImageWithRunPod,
				Description: "Generate an image using ComfyUI workflows on RunPod. This tool loads or creates a workflow, submits it to RunPod, polls for completion, and saves the generated image. Use this after enhancing the prompt and selecting a workflow.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"prompt": map[string]interface{}{
							"type":        "string",
							"description": "Enhanced prompt for image generation (should be enhanced using enhance_prompt tool first)",
						},
						"workflow_name": map[string]interface{}{
							"type":        "string",
							"description": "Name of workflow JSON file (optional, leave empty to use programmatic Z-Image Turbo workflow)",
						},
						"width": map[string]interface{}{
							"type":        "integer",
							"description": "Image width in pixels (default: 1280)",
						},
						"height": map[string]interface{}{
							"type":        "integer",
							"description": "Image height in pixels (default: 1440)",
						},
						"seed": map[string]interface{}{
							"type":        "integer",
							"description": "Random seed for reproducibility (optional, random if not provided)",
						},
					},
					"required": []string{"prompt"},
				},
			},
		},
		{
			Type: "function",
			Function: adapter.FunctionDefinition{
				Name:        ToolEnhancePrompt,
				Description: "Enhance a user's image generation prompt using Z-Image Turbo methodology. This optimizes the prompt for the Qwen 3.4B CLIP model used in Z-Image Turbo workflows. Call this FIRST before generating images.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"user_request": map[string]interface{}{
							"type":        "string",
							"description": "The original user request/prompt to enhance",
						},
					},
					"required": []string{"user_request"},
				},
			},
		},
		{
			Type: "function",
			Function: adapter.FunctionDefinition{
				Name:        ToolSelectWorkflow,
				Description: "Select the best workflow for image generation based on the user's request and enhanced prompt. By default, returns None to use the programmatic Z-Image Turbo workflow. Call this AFTER enhance_prompt.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"user_request": map[string]interface{}{
							"type":        "string",
							"description": "Original user request",
						},
						"enhanced_prompt": map[string]interface{}{
							"type":        "string",
							"description": "Enhanced prompt from enhance_prompt tool",
						},
					},
					"required": []string{"user_request", "enhanced_prompt"},
				},
			},
		},
		{
			Type: "function",
			Function: adapter.FunctionDefinition{
				Name:        ToolListWorkflows,
				Description: "List available ComfyUI workflow JSON files from the configured workflow directory.",
				Parameters: map[string]interface{}{
					"type":       "object",
					"properties": map[string]interface{}{},
					"required":   []string{},
				},
			},
		},
	}
}

