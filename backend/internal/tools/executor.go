package tools

// This file has been refactored and split into multiple focused files by tool category:
//
// Core Executor:
// - executor_core.go: Core executor types, Execute() routing logic, and executor initialization
//
// Tool Implementations by Category:
// - memory_executor.go: Memory tool implementations (core_memory_insert, archival_memory_insert, memory_search)
// - knowledge_executor.go: Knowledge tool implementations (create_fact, search_facts, get_user_context)
// - topic_executor.go: Topic tool implementations (create_topic, link_topics, find_related, link_user_to_topic)
// - conversation_executor.go: Conversation tool implementations (get_history, send_message)
// - web_executor.go: Web tool implementations (web_search, fetch_webpage) + SearchResult type and parsing
// - github_executor.go: GitHub tool implementations (repo_info, search, read_file, list_org_repos)
// - discord_tools_executor.go: Discord tool implementations (read_history, get_user_info, get_channel_info)
// - personality_executor.go: Personality/mimic tool implementations (mimic_personality, revert_personality, analyze_user_style)
//
// Helper Functions:
// - html_helpers.go: HTML processing helper functions (extractTextFromHTML, stripHTMLTags, etc.)
//
// This refactoring reduces file size from 1302 lines to focused modules (max 300-400 lines each),
// improving maintainability and following the Single Responsibility Principle.
