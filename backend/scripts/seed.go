package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"ezra-clone/backend/internal/graph"
	"ezra-clone/backend/internal/state"
	"ezra-clone/backend/pkg/config"
	"ezra-clone/backend/pkg/logger"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"go.uber.org/zap"
)

func main() {
	agentID := flag.String("agent-id", "Ezra", "Agent ID to create")
	force := flag.Bool("force", false, "Force recreation even if agent exists")
	flag.Parse()

	// Initialize logger
	if err := logger.Init("development"); err != nil {
		panic(fmt.Sprintf("Failed to initialize logger: %v", err))
	}
	defer logger.Sync()

	log := logger.Get()
	log.Info("Starting database seeding...")

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatal("Failed to load configuration", zap.Error(err))
	}

	// Initialize Neo4j driver
	driver, err := neo4j.NewDriverWithContext(
		cfg.Neo4jURI,
		neo4j.BasicAuth(cfg.Neo4jUser, cfg.Neo4jPassword, ""),
	)
	if err != nil {
		log.Fatal("Failed to create Neo4j driver", zap.Error(err))
	}
	defer driver.Close(context.Background())

	// Verify connection
	ctx := context.Background()
	if err := driver.VerifyConnectivity(ctx); err != nil {
		log.Fatal("Failed to verify Neo4j connectivity", zap.Error(err))
	}

	repo := graph.NewRepository(driver)

	// Create constraints
	log.Info("Creating constraints...")
	if err := createConstraints(ctx, driver); err != nil {
		log.Warn("Failed to create some constraints (may already exist)", zap.Error(err))
	}

	// Create indexes for better performance
	log.Info("Creating indexes...")
	if err := createIndexes(ctx, driver); err != nil {
		log.Warn("Failed to create some indexes (may already exist)", zap.Error(err))
	}

	// Check if agent already exists
	existingState, err := repo.FetchState(ctx, *agentID)
	if err == nil && !*force {
		log.Info("Agent already exists, skipping creation (use -force to recreate)",
			zap.String("agent_id", *agentID),
			zap.Int("memory_blocks", len(existingState.CoreMemory)),
		)
		os.Exit(0)
	}

	// Create agent
	log.Info("Creating agent", zap.String("agent_id", *agentID))
	if err := repo.CreateAgent(ctx, *agentID, *agentID); err != nil {
		log.Fatal("Failed to create agent", zap.Error(err))
	}

	// Create agent identity
	identity := state.AgentIdentity{
		Name:        *agentID,
		Personality: "You are a helpful, curious, and personable AI assistant with the ability to remember and learn from interactions. You build relationships with users by remembering their preferences and interests.",
		Capabilities: []string{
			"chat",
			"memory_management",
			"fact_tracking",
			"topic_organization",
			"web_search",
			"github_integration",
		},
	}
	if err := repo.CreateAgentIdentity(ctx, *agentID, identity); err != nil {
		log.Fatal("Failed to create agent identity", zap.Error(err))
	}

	// Create default memory blocks
	defaultBlocks := []struct {
		name    string
		content string
	}{
		{
			name: "identity",
			content: fmt.Sprintf(`# %s - AI Agent Identity

I am %s, an intelligent AI agent with persistent memory.

## My Traits
- Helpful and friendly
- Curious about user interests
- Great at remembering facts and organizing knowledge
- Can search the web and GitHub for information

## What I Can Do
- Remember information about users and topics
- Organize knowledge using topics and relationships
- Search my memories for relevant information
- Look up current information online
- Explore GitHub repositories`, *agentID, *agentID),
		},
		{
			name: "instructions",
			content: `# Operating Instructions

## Memory Management
- Use create_fact when users share information or opinions
- Create topics to organize related knowledge
- Link facts to topics and users who shared them
- Use memory_search before claiming ignorance

## User Relationships
- Track user interests with link_user_to_topic
- Reference previous conversations when relevant
- Build on what you know about each user

## Tool Usage
- Always use send_message to respond after tool calls
- Use web_search for current events or unknown topics
- Use GitHub tools when discussing code or repositories

## Conversation Style
- Be conversational and personable
- Acknowledge when you're storing new information
- Reference things you've learned from users`,
		},
		{
			name: "persona",
			content: `# Personality Guidelines

## Communication Style
- Warm and engaging
- Uses casual but professional language
- Shows genuine interest in user topics
- Remembers and references past conversations

## Knowledge Organization
- I organize everything I learn into topics
- I track who told me what
- I build connections between related concepts

## Proactive Behaviors
- I note when users mention interests
- I remember preferences and opinions
- I link related information together`,
		},
	}

	for _, block := range defaultBlocks {
		log.Info("Creating memory block",
			zap.String("block_name", block.name),
		)
		if err := repo.UpdateMemory(ctx, *agentID, block.name, block.content); err != nil {
			log.Fatal("Failed to create memory block",
				zap.String("block_name", block.name),
				zap.Error(err),
			)
		}
	}

	// Create some initial topics
	initialTopics := []struct {
		name        string
		description string
	}{
		{"General", "General conversations and topics"},
		{"Technology", "Technology, programming, and software discussions"},
		{"Entertainment", "Movies, TV shows, games, and media"},
		{"Personal", "User personal information and preferences"},
	}

	for _, topic := range initialTopics {
		log.Info("Creating topic", zap.String("topic", topic.name))
		_, err := repo.CreateTopic(ctx, topic.name, topic.description)
		if err != nil {
			log.Warn("Failed to create topic (may already exist)",
				zap.String("topic", topic.name),
				zap.Error(err),
			)
		}
	}

	// Link topics
	topicLinks := []struct {
		topic1       string
		topic2       string
		relationship string
	}{
		{"Technology", "General", "SUBTOPIC_OF"},
		{"Entertainment", "General", "SUBTOPIC_OF"},
		{"Personal", "General", "SUBTOPIC_OF"},
	}

	for _, link := range topicLinks {
		if err := repo.LinkTopics(ctx, link.topic1, link.topic2, link.relationship); err != nil {
			log.Warn("Failed to link topics",
				zap.String("topic1", link.topic1),
				zap.String("topic2", link.topic2),
			)
		}
	}

	// Verify creation
	finalState, err := repo.FetchState(ctx, *agentID)
	if err != nil {
		log.Fatal("Failed to verify agent creation", zap.Error(err))
	}

	log.Info("Agent seeded successfully",
		zap.String("agent_id", *agentID),
		zap.String("name", finalState.Identity.Name),
		zap.Int("memory_blocks", len(finalState.CoreMemory)),
	)

	log.Info("Seed completed. The agent is ready to use!")
}

// createConstraints creates Neo4j constraints for data integrity
func createConstraints(ctx context.Context, driver neo4j.DriverWithContext) error {
	session := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	constraints := []string{
		// Discord ID uniqueness (if not already exists)
		"CREATE CONSTRAINT user_discord_id_unique IF NOT EXISTS FOR (u:User) REQUIRE u.discord_id IS UNIQUE",

		// Guild constraints
		"CREATE CONSTRAINT guild_id_unique IF NOT EXISTS FOR (g:Guild) REQUIRE g.id IS UNIQUE",

		// Channel constraints
		"CREATE CONSTRAINT channel_id_unique IF NOT EXISTS FOR (c:Channel) REQUIRE c.id IS UNIQUE",

		// Role constraints
		"CREATE CONSTRAINT role_id_unique IF NOT EXISTS FOR (r:Role) REQUIRE r.id IS UNIQUE",
	}

	for _, constraint := range constraints {
		_, err := session.Run(ctx, constraint, nil)
		if err != nil {
			// Log but don't fail - constraints may already exist
			continue
		}
	}

	return nil
}

// createIndexes creates Neo4j indexes for better query performance
func createIndexes(ctx context.Context, driver neo4j.DriverWithContext) error {
	session := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	// Basic indexes
	indexes := []string{
		"CREATE INDEX agent_id IF NOT EXISTS FOR (a:Agent) ON (a.id)",
		"CREATE INDEX user_id IF NOT EXISTS FOR (u:User) ON (u.id)",
		"CREATE INDEX user_discord_id IF NOT EXISTS FOR (u:User) ON (u.discord_id)",
		"CREATE INDEX topic_name IF NOT EXISTS FOR (t:Topic) ON (t.name)",
		"CREATE INDEX fact_id IF NOT EXISTS FOR (f:Fact) ON (f.id)",
		"CREATE INDEX memory_name IF NOT EXISTS FOR (m:Memory) ON (m.name)",
		"CREATE INDEX message_timestamp IF NOT EXISTS FOR (m:Message) ON (m.timestamp)",
		"CREATE INDEX conversation_channel IF NOT EXISTS FOR (c:Conversation) ON (c.channel_id)",

		// Enhanced indexes for Discord entities
		"CREATE INDEX guild_id IF NOT EXISTS FOR (g:Guild) ON (g.id)",
		"CREATE INDEX channel_id IF NOT EXISTS FOR (c:Channel) ON (c.id)",
		"CREATE INDEX channel_guild IF NOT EXISTS FOR (c:Channel) ON (c.guild_id)",
		"CREATE INDEX role_id IF NOT EXISTS FOR (r:Role) ON (r.id)",
		"CREATE INDEX role_guild IF NOT EXISTS FOR (r:Role) ON (r.guild_id)",

		// Composite indexes
		"CREATE INDEX user_platform_discord IF NOT EXISTS FOR (u:User) ON (u.platform, u.discord_id)",
		"CREATE INDEX conversation_platform_channel IF NOT EXISTS FOR (c:Conversation) ON (c.platform, c.channel_id)",

		// Activity pattern indexes
		"CREATE INDEX activity_pattern_user IF NOT EXISTS FOR (ap:ActivityPattern) ON (ap.day_of_week, ap.hour_of_day)",

		// Fact indexes
		"CREATE INDEX fact_created_at IF NOT EXISTS FOR (f:Fact) ON (f.created_at)",
	}

	for _, idx := range indexes {
		_, err := session.Run(ctx, idx, nil)
		if err != nil {
			// Log but don't fail - indexes may already exist
			continue
		}
	}

	// Try to create full-text indexes (may not be supported in all Neo4j versions)
	fullTextIndexes := []string{
		"CREATE FULLTEXT INDEX fact_content IF NOT EXISTS FOR (f:Fact) ON EACH [f.content]",
		"CREATE FULLTEXT INDEX message_content IF NOT EXISTS FOR (m:Message) ON EACH [m.content]",
		"CREATE FULLTEXT INDEX topic_description IF NOT EXISTS FOR (t:Topic) ON EACH [t.name, t.description]",
	}

	for _, idx := range fullTextIndexes {
		_, err := session.Run(ctx, idx, nil)
		if err != nil {
			// Full-text indexes may not be supported - this is okay
			continue
		}
	}

	return nil
}
