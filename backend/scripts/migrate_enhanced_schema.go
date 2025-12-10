package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"ezra-clone/backend/pkg/config"
	"ezra-clone/backend/pkg/logger"
	"go.uber.org/zap"
)

func main() {
	force := flag.Bool("force", false, "Force migration even if already applied")
	flag.Parse()

	// Initialize logger
	if err := logger.Init("development"); err != nil {
		panic(fmt.Sprintf("Failed to initialize logger: %v", err))
	}
	defer logger.Sync()

	log := logger.Get()
	log.Info("Starting Neo4j schema migration...")

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

	// Check if migration already applied
	if !*force {
		applied, err := checkMigrationApplied(ctx, driver)
		if err != nil {
			log.Fatal("Failed to check migration status", zap.Error(err))
		}
		if applied {
			log.Info("Migration already applied. Use -force to reapply.")
			os.Exit(0)
		}
	}

	// Run migrations
	if err := runMigrations(ctx, driver, log); err != nil {
		log.Fatal("Migration failed", zap.Error(err))
	}

	// Mark migration as applied
	if err := markMigrationApplied(ctx, driver); err != nil {
		log.Warn("Failed to mark migration as applied", zap.Error(err))
	}

	log.Info("Migration completed successfully!")
}

func checkMigrationApplied(ctx context.Context, driver neo4j.DriverWithContext) (bool, error) {
	session := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	query := `
		MATCH (m:Migration {version: 'enhanced_schema_v1'})
		RETURN m.applied_at as applied_at
	`

	result, err := session.Run(ctx, query, nil)
	if err != nil {
		return false, err
	}

	return result.Next(ctx), nil
}

func markMigrationApplied(ctx context.Context, driver neo4j.DriverWithContext) error {
	session := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	query := `
		MERGE (m:Migration {version: 'enhanced_schema_v1'})
		SET m.applied_at = datetime(),
		    m.description = 'Enhanced schema with Discord entities, user relationships, and weighted connections'
	`

	_, err := session.Run(ctx, query, nil)
	return err
}

func runMigrations(ctx context.Context, driver neo4j.DriverWithContext, log *zap.Logger) error {
	session := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	migrations := []struct {
		name        string
		description string
		query       string
	}{
		{
			name:        "Create Constraints",
			description: "Create unique constraints for Discord IDs and other entities",
			query: `
				// Discord ID uniqueness constraint
				CREATE CONSTRAINT user_discord_id_unique IF NOT EXISTS FOR (u:User) REQUIRE u.discord_id IS UNIQUE;
				
				// Guild constraints
				CREATE CONSTRAINT guild_id_unique IF NOT EXISTS FOR (g:Guild) REQUIRE g.id IS UNIQUE;
				
				// Channel constraints
				CREATE CONSTRAINT channel_id_unique IF NOT EXISTS FOR (c:Channel) REQUIRE c.id IS UNIQUE;
				
				// Role constraints
				CREATE CONSTRAINT role_id_unique IF NOT EXISTS FOR (r:Role) REQUIRE r.id IS UNIQUE;
			`,
		},
		{
			name:        "Create Indexes",
			description: "Create indexes for better query performance",
			query: `
				// Composite indexes
				CREATE INDEX user_platform_discord IF NOT EXISTS FOR (u:User) ON (u.platform, u.discord_id);
				CREATE INDEX conversation_platform_channel IF NOT EXISTS FOR (c:Conversation) ON (c.platform, c.channel_id);
				CREATE INDEX channel_guild IF NOT EXISTS FOR (c:Channel) ON (c.guild_id);
				CREATE INDEX role_guild IF NOT EXISTS FOR (r:Role) ON (r.guild_id);
				
				// Activity pattern indexes
				CREATE INDEX activity_pattern_user IF NOT EXISTS FOR (ap:ActivityPattern) ON (ap.day_of_week, ap.hour_of_day);
			`,
		},
		{
			name:        "Create Full-Text Indexes",
			description: "Create full-text search indexes for content search",
			query: `
				// Full-text search indexes (if supported by Neo4j version)
				CREATE FULLTEXT INDEX fact_content IF NOT EXISTS FOR (f:Fact) ON EACH [f.content];
				CREATE FULLTEXT INDEX message_content IF NOT EXISTS FOR (m:Message) ON EACH [m.content];
				CREATE FULLTEXT INDEX topic_description IF NOT EXISTS FOR (t:Topic) ON EACH [t.name, t.description];
			`,
		},
		{
			name:        "Add Relationship Properties",
			description: "Add temporal and weighted properties to existing relationships",
			query: `
				// Enhance INTERESTED_IN relationships with weighted properties
				MATCH (u:User)-[r:INTERESTED_IN]->(t:Topic)
				WHERE r.strength IS NULL
				SET r.strength = 0.5,
				    r.first_interaction = COALESCE(r.first_interaction, datetime()),
				    r.last_interaction = COALESCE(r.last_interaction, datetime()),
				    r.interaction_count = COALESCE(r.interaction_count, 1),
				    r.recency_score = 1.0;
				
				// Enhance TOLD_ME relationships
				MATCH (u:User)-[r:TOLD_ME]->(f:Fact)
				WHERE r.fact_count IS NULL
				SET r.fact_count = 1,
				    r.first_fact = COALESCE(r.first_fact, datetime()),
				    r.last_fact = COALESCE(r.last_fact, datetime()),
				    r.trust_score = 0.5;
				
				// Enhance PARTICIPATED_IN relationships
				MATCH (u:User)-[r:PARTICIPATED_IN]->(c:Conversation)
				WHERE r.message_count IS NULL
				WITH u, c, r, count{(u)-[:SENT]->(:Message)-[:CONTAINS]-(c)} as msg_count
				SET r.message_count = msg_count,
				    r.first_message = COALESCE(r.first_message, datetime()),
				    r.last_message = COALESCE(r.last_message, datetime()),
				    r.activity_score = CASE 
				    	WHEN msg_count > 10 THEN 0.8
				    	WHEN msg_count > 5 THEN 0.6
				    	ELSE 0.4
				    END;
			`,
		},
		{
			name:        "Create User Similarity Relationships",
			description: "Calculate and create similarity relationships between users",
			query: `
				// Calculate user similarities based on shared topics
				MATCH (u1:User)-[:INTERESTED_IN]->(t:Topic)<-[:INTERESTED_IN]-(u2:User)
				WHERE u1 <> u2 AND id(u1) < id(u2)
				WITH u1, u2, collect(DISTINCT t.name) as shared_topics, count(DISTINCT t) as topic_count
				MERGE (u1)-[s:SIMILAR_TO]->(u2)
				ON CREATE SET 
					s.similarity_score = topic_count * 0.1,
					s.based_on = 'topics',
					s.shared_items = shared_topics,
					s.created_at = datetime()
				ON MATCH SET 
					s.similarity_score = topic_count * 0.1,
					s.shared_items = shared_topics,
					s.updated_at = datetime();
			`,
		},
		{
			name:        "Add Conversation Metadata",
			description: "Add metadata to conversation nodes",
			query: `
				// Add message count and participant count to conversations
				MATCH (c:Conversation)
				OPTIONAL MATCH (c)-[:CONTAINS]->(m:Message)
				OPTIONAL MATCH (u:User)-[:PARTICIPATED_IN]->(c)
				WITH c, count(DISTINCT m) as msg_count, count(DISTINCT u) as participant_count
				SET c.message_count = msg_count,
				    c.participant_count = participant_count,
				    c.last_activity = COALESCE(c.last_activity, c.started_at);
			`,
		},
		{
			name:        "Create Fact Relationship Indexes",
			description: "Add indexes for fact relationship queries",
			query: `
				// Indexes for fact relationships (if needed)
				CREATE INDEX fact_created_at IF NOT EXISTS FOR (f:Fact) ON (f.created_at);
			`,
		},
	}

	for i, migration := range migrations {
		log.Info("Running migration",
			zap.Int("step", i+1),
			zap.Int("total", len(migrations)),
			zap.String("name", migration.name),
			zap.String("description", migration.description),
		)

		// Split query by semicolons and execute each statement
		statements := splitStatements(migration.query)
		for j, stmt := range statements {
			if stmt == "" {
				continue
			}
			_, err := session.Run(ctx, stmt, nil)
			if err != nil {
				// Some errors are expected (e.g., constraints/indexes already exist)
				log.Warn("Migration step had an error (may be expected)",
					zap.String("migration", migration.name),
					zap.Int("statement", j+1),
					zap.Error(err),
				)
				// Continue anyway - many of these are idempotent
			}
		}

		log.Info("Migration step completed", zap.String("name", migration.name))
	}

	return nil
}

// splitStatements splits a Cypher script into individual statements
// Simple approach: split by semicolon and trim whitespace
func splitStatements(script string) []string {
	// Remove single-line comments
	lines := strings.Split(script, "\n")
	var cleanedLines []string
	for _, line := range lines {
		// Remove // comments
		if idx := strings.Index(line, "//"); idx >= 0 {
			line = line[:idx]
		}
		cleanedLines = append(cleanedLines, line)
	}
	cleanedScript := strings.Join(cleanedLines, "\n")
	
	// Split by semicolon
	parts := strings.Split(cleanedScript, ";")
	var statements []string
	for _, part := range parts {
		stmt := strings.TrimSpace(part)
		// Remove multi-line comments
		stmt = removeMultiLineComments(stmt)
		stmt = strings.TrimSpace(stmt)
		if stmt != "" {
			statements = append(statements, stmt)
		}
	}
	
	return statements
}

// removeMultiLineComments removes /* */ style comments
func removeMultiLineComments(text string) string {
	for {
		start := strings.Index(text, "/*")
		if start < 0 {
			break
		}
		end := strings.Index(text[start+2:], "*/")
		if end < 0 {
			break
		}
		text = text[:start] + text[start+end+4:]
	}
	return text
}

