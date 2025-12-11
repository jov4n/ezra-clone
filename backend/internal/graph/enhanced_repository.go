package graph

// This file has been refactored and split into multiple focused files:
//
// Core Repository:
// - repository.go: Repository struct, initialization, and core state management
//
// Type Definitions:
// - types.go: All type definitions (User, Fact, Topic, Conversation, Message, etc.)
//
// Helper Functions:
// - helpers.go: Helper functions for record parsing (getStringFromRecord, etc.)
//
// Operations by Category:
// - user_operations.go: User-related operations (GetOrCreateUser, GetUserContext, etc.)
// - fact_operations.go: Fact-related operations (CreateFact, GetFactsAboutTopic, etc.)
// - deduplication.go: Fact deduplication logic
// - topic_operations.go: Topic-related operations (CreateTopic, LinkTopics, etc.)
// - conversation_operations.go: Conversation/message operations (LogMessage, GetConversationHistory, etc.)
// - discord_operations.go: Discord entity operations (CreateOrUpdateGuild, CreateOrUpdateChannel, etc.)
// - relationship_operations.go: User-to-user relationship operations (RecordUserMention, RecordCollaboration, etc.)
// - search_operations.go: Search operations (SearchMemory)
// - similarity_operations.go: User similarity and recommendations (CalculateUserSimilarity, FindSimilarUsers, etc.)
//
// This refactoring reduces file size from 1778 lines to focused modules (max 300-400 lines each),
// improving maintainability and following the Single Responsibility Principle.
