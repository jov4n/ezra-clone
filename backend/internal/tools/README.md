# Tools Package Organization

This package contains all tool definitions and executors for the agent system.

## Current Structure

The tools package is organized as follows:

```
internal/tools/
├── tools.go                 # All tool definitions (GetMemoryTools, GetDiscordTools, etc.)
├── executor_core.go         # Core executor types and ExecutionContext
├── executor.go              # Main executor routing logic
│
├── memory_executor.go       # Memory tool implementations
├── knowledge_executor.go    # Knowledge/fact tool implementations
├── topic_executor.go        # Topic tool implementations
├── conversation_executor.go # Conversation tool implementations
├── web_executor.go          # Web tool implementations
├── html_helpers.go          # HTML processing helpers
├── github_executor.go       # GitHub tool implementations
├── discord_tools_executor.go # Discord tool implementations
├── discord_executor.go      # Discord API executor (separate from tools)
├── personality_executor.go  # Personality/mimic tool implementations
├── music_executor.go        # Music tool implementations
├── comfy_executor.go        # ComfyUI/image generation executor
├── comfy_prompts.go         # Prompt enhancement
├── comfy_workflows.go       # Workflow definitions
├── runpod_client.go         # RunPod API client
├── prompt_enhancer.go       # LLM prompt enhancement
├── mimic_background_task.go # Background task for mimic mode
│
└── music/                   # Music subsystem (already organized)
    ├── bot.go
    ├── player.go
    ├── preload.go
    ├── sources/
    │   ├── types.go
    │   ├── youtube.go
    │   ├── spotify.go
    │   ├── soundcloud.go
    │   └── openrouter.go
    └── ui/
        └── embeds.go
```

## Tool Categories

### Memory Tools
- `core_memory_insert` - Create new core memory blocks
- `core_memory_replace` - Update existing core memory blocks
- `archival_memory_insert` - Store information in archival memory
- `archival_memory_search` - Search archival memory
- `memory_search` - Search all memories

### Knowledge Tools
- `create_fact` - Create a new fact in the knowledge graph
- `search_facts` - Search for facts
- `link_fact_to_user` - Associate a fact with a user
- `get_user_context` - Get user's context and preferences

### Topic Tools
- `create_topic` - Create a new topic
- `link_topics` - Link related topics
- `find_related_topics` - Find topics related to a given topic
- `link_user_to_topic` - Associate a user with a topic

### Conversation Tools
- `get_conversation_history` - Retrieve conversation history
- `send_message` - Send a message (platform-specific)

### Web Tools
- `web_search` - Search the web
- `fetch_webpage` - Fetch and parse a webpage

### GitHub Tools
- `github_repo_info` - Get repository information
- `github_search` - Search GitHub repositories
- `github_read_file` - Read a file from a repository
- `github_list_org_repos` - List organization repositories

### Discord Tools
- `discord_read_history` - Read Discord channel history
- `discord_get_user_info` - Get Discord user information
- `discord_search_messages` - Search Discord messages
- `discord_get_channel_info` - Get Discord channel information

### Personality Tools
- `mimic_personality` - Mimic a user's communication style
- `revert_personality` - Revert to original personality
- `analyze_user_style` - Analyze a user's communication style

### Image Generation Tools
- `generate_image` - Generate an image using ComfyUI

### Music Tools
- `play_music` - Play music in Discord voice channel
- `stop_music` - Stop music playback
- `skip_music` - Skip current track
- `queue_music` - Add song to queue
- `get_queue` - Get current queue
- `set_volume` - Set music volume

## Future Organization

For better maintainability, consider organizing into subdirectories:

```
internal/tools/
├── executor.go
├── executor_core.go
├── memory/
│   ├── executor.go
│   └── tools.go
├── discord/
│   ├── executor.go
│   ├── tools.go
│   └── discord_executor.go
├── music/ (already organized)
├── comfy/
├── web/
├── github/
├── knowledge/
├── topic/
├── conversation/
└── personality/
```

This would provide:
- Better separation of concerns
- Easier testing
- Clearer dependencies
- Smaller, more focused files

## Adding New Tools

1. Add tool definition to `tools.go` in the appropriate `Get*Tools()` function
2. Add tool constant to `tools.go` 
3. Add executor implementation in the appropriate `*_executor.go` file
4. Register the executor in `executor.go`'s `Execute()` method

## Testing

Each tool category can be tested independently. The executor pattern allows for easy mocking of dependencies.

