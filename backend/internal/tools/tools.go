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

// Tool names - System Tools
const (
	ToolBotShutdown = "bot_shutdown"
)

// Tool names - Web & External Tools
const (
	ToolWebSearch        = "web_search"
	ToolFetchWebpage     = "fetch_webpage"
	ToolSummarizeWebsite = "summarize_website"
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
	ToolReadCodebase = "read_codebase"
)

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

// Tool names - Music Tools
const (
	ToolMusicPlay      = "music_play"
	ToolMusicPlaylist  = "music_playlist"
	ToolMusicQueue     = "music_queue"
	ToolMusicSkip      = "music_skip"
	ToolMusicPause     = "music_pause"
	ToolMusicResume    = "music_resume"
	ToolMusicStop      = "music_stop"
	ToolMusicVolume    = "music_volume"
	ToolMusicRadio     = "music_radio"
	ToolMusicDisconnect = "music_disconnect"
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
	
	// Music Tools
	tools = append(tools, GetMusicTools()...)
	
	// System Tools
	tools = append(tools, GetSystemTools()...)
	
	return tools
}

