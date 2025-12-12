package tools

// Registry provides a centralized way to manage and access all tools.
// This file serves as a registry pattern implementation for tool organization.
//
// Current Structure:
// - tools.go: Contains all tool definitions (GetMemoryTools, GetDiscordTools, etc.)
// - executor_core.go: Core executor types and routing
// - *_executor.go: Tool implementation files organized by category
//
// Recommended Future Structure (for incremental refactoring):
//   internal/tools/
//   ├── registry.go          # Tool registration & GetAllTools (this file)
//   ├── executor.go           # Base executor interface and core logic
//   ├── executor_core.go      # Execution context and result types
//   ├── memory/
//   │   ├── executor.go       # Memory tools executor
//   │   └── tools.go          # Memory tool definitions
//   ├── discord/
//   │   ├── executor.go       # Discord tools executor (discord_tools_executor.go)
//   │   ├── tools.go          # Discord tool definitions
//   │   └── discord_executor.go # Discord API executor (already exists)
//   ├── music/
//   │   └── ...               # (already organized)
//   ├── comfy/
//   │   ├── executor.go       # ComfyUI executor
//   │   ├── prompts.go        # Prompt enhancement
//   │   └── workflows.go      # Workflow definitions
//   ├── web/
//   │   ├── executor.go       # Web tools executor
//   │   ├── tools.go          # Web tool definitions
//   │   └── html_helpers.go   # HTML processing helpers
//   ├── github/
//   │   ├── executor.go       # GitHub tools executor
//   │   └── tools.go          # GitHub tool definitions
//   ├── knowledge/
//   │   ├── executor.go       # Knowledge/facts executor
//   │   └── tools.go          # Knowledge tool definitions
//   ├── topic/
//   │   ├── executor.go       # Topic executor
//   │   └── tools.go          # Topic tool definitions
//   ├── conversation/
//   │   ├── executor.go       # Conversation executor
//   │   └── tools.go          # Conversation tool definitions
//   └── personality/
//       ├── executor.go       # Personality/mimic executor
//       └── tools.go          # Personality tool definitions
//
// This structure would improve:
// - Separation of concerns (each category in its own package)
// - Easier testing (can test each category independently)
// - Better maintainability (smaller, focused files)
// - Clearer dependencies (explicit imports show what's used)
//
// Migration Strategy:
// 1. Create subdirectories one at a time
// 2. Move files incrementally
// 3. Update imports as files are moved
// 4. Keep backward compatibility during transition
// 5. Update all callers once migration is complete

