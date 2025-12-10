package graph

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"go.uber.org/zap"
)

// ============================================================================
// Enhanced Graph Types
// ============================================================================

// User represents a user in the graph
type User struct {
	ID              string    `json:"id"`
	DiscordID       string    `json:"discord_id,omitempty"`
	DiscordUsername string    `json:"discord_username,omitempty"`
	WebID           string    `json:"web_id,omitempty"`
	PreferredLanguage string  `json:"preferred_language,omitempty"`
	FirstSeen       time.Time `json:"first_seen"`
	LastSeen        time.Time `json:"last_seen"`
}

// Fact represents a learned fact
type Fact struct {
	ID         string    `json:"id"`
	Content    string    `json:"content"`
	Source     string    `json:"source,omitempty"`
	Confidence float64   `json:"confidence"`
	CreatedAt  time.Time `json:"created_at"`
}

// Topic represents a topic/subject
type Topic struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// Conversation represents a conversation thread
type Conversation struct {
	ID        string    `json:"id"`
	ChannelID string    `json:"channel_id,omitempty"`
	Platform  string    `json:"platform"` // discord, web
	StartedAt time.Time `json:"started_at"`
}

// Message represents a single message
type Message struct {
	ID        string    `json:"id"`
	Content   string    `json:"content"`
	Role      string    `json:"role"` // user, agent
	Platform  string    `json:"platform"`
	Timestamp time.Time `json:"timestamp"`
}

// UserContext contains aggregated information about a user
type UserContext struct {
	User          User     `json:"user"`
	Topics        []Topic  `json:"topics"`
	Facts         []Fact   `json:"facts"`
	MessageCount  int64    `json:"message_count"`
	LastMessage   string   `json:"last_message,omitempty"`
	Conversations int64    `json:"conversations"`
}

// SearchResult represents a search result
type SearchResult struct {
	Type       string                 `json:"type"` // fact, topic, memory, user
	ID         string                 `json:"id"`
	Content    string                 `json:"content"`
	Score      float64                `json:"score"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
	Related    []string               `json:"related,omitempty"`
}

// Guild represents a Discord server/guild
type Guild struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	MemberCount int       `json:"member_count,omitempty"`
	CreatedAt   time.Time `json:"created_at,omitempty"`
}

// Channel represents a Discord channel
type Channel struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"` // text, voice, category, dm, group_dm
	Topic    string `json:"topic,omitempty"`
	GuildID  string `json:"guild_id,omitempty"`
}

// Role represents a Discord role
type Role struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Color       int      `json:"color,omitempty"`
	Permissions int64    `json:"permissions,omitempty"`
	GuildID     string   `json:"guild_id,omitempty"`
}

// ActivityPattern represents user activity patterns
type ActivityPattern struct {
	UserID           string    `json:"user_id"`
	DayOfWeek        string    `json:"day_of_week,omitempty"`
	HourOfDay        int       `json:"hour_of_day,omitempty"`
	ActivityCount    int       `json:"activity_count"`
	AvgMessageLength float64   `json:"avg_message_length,omitempty"`
	LastUpdated      time.Time `json:"last_updated"`
}

// UserSimilarity represents similarity between users
type UserSimilarity struct {
	User1ID        string   `json:"user1_id"`
	User2ID        string   `json:"user2_id"`
	SimilarityScore float64 `json:"similarity_score"`
	BasedOn        string   `json:"based_on"` // topics, facts, behavior
	SharedItems    []string `json:"shared_items,omitempty"`
}

// ============================================================================
// User Operations
// ============================================================================

// GetOrCreateUser gets or creates a user node
func (r *Repository) GetOrCreateUser(ctx context.Context, userID, discordID, discordUsername, platform string) (*User, error) {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	now := time.Now().UTC().Format(time.RFC3339)

	query := `
		MERGE (u:User {id: $userID})
		ON CREATE SET 
			u.discord_id = $discordID,
			u.discord_username = $discordUsername,
			u.platform = $platform,
			u.first_seen = datetime($now),
			u.last_seen = datetime($now)
		ON MATCH SET 
			u.last_seen = datetime($now),
			u.discord_username = CASE WHEN $discordUsername <> '' THEN $discordUsername ELSE u.discord_username END
		RETURN u.id as id, u.discord_id as discord_id, u.discord_username as discord_username,
		       u.preferred_language as preferred_language, u.first_seen as first_seen, u.last_seen as last_seen
	`

	result, err := session.Run(ctx, query, map[string]interface{}{
		"userID":          userID,
		"discordID":       discordID,
		"discordUsername": discordUsername,
		"platform":        platform,
		"now":             now,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get/create user: %w", err)
	}

	if result.Next(ctx) {
		record := result.Record()
		return &User{
			ID:              getStringFromRecord(record, "id"),
			DiscordID:       getStringFromRecord(record, "discord_id"),
			DiscordUsername: getStringFromRecord(record, "discord_username"),
			PreferredLanguage: getStringFromRecord(record, "preferred_language"),
		}, nil
	}

	return nil, fmt.Errorf("failed to create user")
}

// SetUserLanguagePreference sets the preferred language for a user
func (r *Repository) SetUserLanguagePreference(ctx context.Context, userID, language string) error {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	query := `
		MATCH (u:User {id: $userID})
		SET u.preferred_language = $language
	`

	_, err := session.Run(ctx, query, map[string]interface{}{
		"userID":   userID,
		"language": language,
	})
	if err != nil {
		return fmt.Errorf("failed to set language preference: %w", err)
	}

	return nil
}

// GetUserLanguagePreference retrieves the preferred language for a user
func (r *Repository) GetUserLanguagePreference(ctx context.Context, userID string) (string, error) {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	query := `
		MATCH (u:User {id: $userID})
		RETURN u.preferred_language as preferred_language
	`

	result, err := session.Run(ctx, query, map[string]interface{}{
		"userID": userID,
	})
	if err != nil {
		return "", fmt.Errorf("failed to get language preference: %w", err)
	}

	if result.Next(ctx) {
		record := result.Record()
		return getStringFromRecord(record, "preferred_language"), nil
	}

	return "", nil // No preference set
}

// FindUserByDiscordUsername finds a user by their Discord username (case-insensitive)
func (r *Repository) FindUserByDiscordUsername(ctx context.Context, username string) (*User, error) {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	query := `
		MATCH (u:User)
		WHERE toLower(u.discord_username) = toLower($username)
		RETURN u.id as id, u.discord_id as discord_id, u.discord_username as discord_username,
		       u.preferred_language as preferred_language, u.first_seen as first_seen, u.last_seen as last_seen
		LIMIT 1
	`

	result, err := session.Run(ctx, query, map[string]interface{}{
		"username": username,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to find user by username: %w", err)
	}

	if result.Next(ctx) {
		record := result.Record()
		return &User{
			ID:              getStringFromRecord(record, "id"),
			DiscordID:       getStringFromRecord(record, "discord_id"),
			DiscordUsername: getStringFromRecord(record, "discord_username"),
			PreferredLanguage: getStringFromRecord(record, "preferred_language"),
		}, nil
	}

	return nil, fmt.Errorf("user not found: %s", username)
}

// GetUserContext retrieves comprehensive context about a user
func (r *Repository) GetUserContext(ctx context.Context, userID string) (*UserContext, error) {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	query := `
		MATCH (u:User {id: $userID})
		OPTIONAL MATCH (u)-[:INTERESTED_IN]->(t:Topic)
		OPTIONAL MATCH (u)-[:TOLD_ME]->(f:Fact)
		OPTIONAL MATCH (u)-[:SENT]->(m:Message)
		OPTIONAL MATCH (u)-[:PARTICIPATED_IN]->(c:Conversation)
		WITH u, 
		     collect(DISTINCT {id: t.id, name: t.name}) as topics,
		     collect(DISTINCT {id: f.id, content: f.content}) as facts,
		     count(DISTINCT m) as msg_count,
		     count(DISTINCT c) as conv_count
		OPTIONAL MATCH (u)-[:SENT]->(lastMsg:Message)
		WITH u, topics, facts, msg_count, conv_count, lastMsg
		ORDER BY lastMsg.timestamp DESC
		LIMIT 1
		RETURN u.id as user_id, u.discord_id as discord_id, u.discord_username as discord_username, 
		       topics, facts, msg_count, conv_count, lastMsg.content as last_message, u.preferred_language as preferred_language
	`

	result, err := session.Run(ctx, query, map[string]interface{}{
		"userID": userID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get user context: %w", err)
	}

	if result.Next(ctx) {
		record := result.Record()

		preferredLang := getStringFromRecord(record, "preferred_language")
		discordID := getStringFromRecord(record, "discord_id")
		discordUsername := getStringFromRecord(record, "discord_username")
		uc := &UserContext{
			User: User{
				ID:               userID,
				DiscordID:        discordID,
				DiscordUsername:  discordUsername,
				PreferredLanguage: preferredLang,
			},
		}

		if msgCount, ok := record.Get("msg_count"); ok {
			if count, ok := msgCount.(int64); ok {
				uc.MessageCount = count
			}
		}

		if convCount, ok := record.Get("conv_count"); ok {
			if count, ok := convCount.(int64); ok {
				uc.Conversations = count
			}
		}

		if lastMsg, ok := record.Get("last_message"); ok {
			if msg, ok := lastMsg.(string); ok {
				uc.LastMessage = msg
			}
		}

		// Parse topics
		if topics, ok := record.Get("topics"); ok {
			if topicList, ok := topics.([]interface{}); ok {
				for _, t := range topicList {
					if tm, ok := t.(map[string]interface{}); ok {
						if name, ok := tm["name"].(string); ok && name != "" {
							uc.Topics = append(uc.Topics, Topic{
								ID:   getStringFromMap(tm, "id", ""),
								Name: name,
							})
						}
					}
				}
			}
		}

		// Parse facts
		if facts, ok := record.Get("facts"); ok {
			if factList, ok := facts.([]interface{}); ok {
				for _, f := range factList {
					if fm, ok := f.(map[string]interface{}); ok {
						if content, ok := fm["content"].(string); ok && content != "" {
							uc.Facts = append(uc.Facts, Fact{
								ID:      getStringFromMap(fm, "id", ""),
								Content: content,
							})
						}
					}
				}
			}
		}

		// Deduplicate facts before returning
		uc.Facts = deduplicateFacts(uc.Facts)

		return uc, nil
	}

	return nil, fmt.Errorf("user not found: %s", userID)
}

// ============================================================================
// Fact Operations
// ============================================================================

// CreateFact creates a new fact and links it to the agent and optionally a user/topic
func (r *Repository) CreateFact(ctx context.Context, agentID, content, source, userID string, topicNames []string) (*Fact, error) {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	factID := uuid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)

	// Create the fact and link to agent
	query := `
		MATCH (a:Agent {id: $agentID})
		CREATE (f:Fact {
			id: $factID,
			content: $content,
			source: $source,
			confidence: 1.0,
			created_at: datetime($now)
		})
		CREATE (a)-[:KNOWS_FACT]->(f)
		RETURN f.id as id
	`

	_, err := session.Run(ctx, query, map[string]interface{}{
		"agentID": agentID,
		"factID":  factID,
		"content": content,
		"source":  source,
		"now":     now,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create fact: %w", err)
	}

	// Link to user if provided
	if userID != "" {
		linkQuery := `
			MATCH (f:Fact {id: $factID})
			MATCH (u:User {id: $userID})
			MERGE (u)-[:TOLD_ME]->(f)
		`
		_, _ = session.Run(ctx, linkQuery, map[string]interface{}{
			"factID": factID,
			"userID": userID,
		})
	}

	// Link to topics
	for _, topicName := range topicNames {
		if topicName == "" {
			continue
		}
		topicQuery := `
			MATCH (f:Fact {id: $factID})
			MERGE (t:Topic {name: $topicName})
			ON CREATE SET t.id = $topicID, t.created_at = datetime($now)
			MERGE (f)-[:ABOUT]->(t)
		`
		_, _ = session.Run(ctx, topicQuery, map[string]interface{}{
			"factID":    factID,
			"topicName": topicName,
			"topicID":   uuid.New().String(),
			"now":       now,
		})
	}

	r.logger.Info("Fact created",
		zap.String("fact_id", factID),
		zap.String("agent_id", agentID),
		zap.String("source", source),
	)

	return &Fact{
		ID:        factID,
		Content:   content,
		Source:    source,
		CreatedAt: time.Now(),
	}, nil
}

// GetFactsAboutTopic retrieves all facts about a topic
func (r *Repository) GetFactsAboutTopic(ctx context.Context, topicName string) ([]Fact, error) {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	query := `
		MATCH (f:Fact)-[:ABOUT]->(t:Topic)
		WHERE toLower(t.name) CONTAINS toLower($topicName)
		OPTIONAL MATCH (u:User)-[:TOLD_ME]->(f)
		RETURN f.id as id, f.content as content, f.source as source, 
		       f.confidence as confidence, f.created_at as created_at,
		       u.discord_username as told_by
		ORDER BY f.created_at DESC
		LIMIT 20
	`

	result, err := session.Run(ctx, query, map[string]interface{}{
		"topicName": topicName,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get facts: %w", err)
	}

	var facts []Fact
	for result.Next(ctx) {
		record := result.Record()
		fact := Fact{
			ID:      getStringFromRecord(record, "id"),
			Content: getStringFromRecord(record, "content"),
			Source:  getStringFromRecord(record, "source"),
		}
		if toldBy := getStringFromRecord(record, "told_by"); toldBy != "" {
			fact.Source = fmt.Sprintf("Told by %s", toldBy)
		}
		facts = append(facts, fact)
	}

	return facts, nil
}

// UpdateFact updates the content of an existing fact
func (r *Repository) UpdateFact(ctx context.Context, factID, newContent string) error {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	now := time.Now().UTC().Format(time.RFC3339)

	query := `
		MATCH (f:Fact {id: $factID})
		SET f.content = $newContent,
		    f.updated_at = datetime($now)
		RETURN f.id as id
	`

	result, err := session.Run(ctx, query, map[string]interface{}{
		"factID":    factID,
		"newContent": newContent,
		"now":       now,
	})
	if err != nil {
		return fmt.Errorf("failed to update fact: %w", err)
	}

	if !result.Next(ctx) {
		return fmt.Errorf("fact not found: %s", factID)
	}

	r.logger.Info("Fact updated",
		zap.String("fact_id", factID),
	)
	return nil
}

// ============================================================================
// Topic Operations
// ============================================================================

// CreateTopic creates a new topic
func (r *Repository) CreateTopic(ctx context.Context, name, description string) (*Topic, error) {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	topicID := uuid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)

	query := `
		MERGE (t:Topic {name: $name})
		ON CREATE SET t.id = $topicID, t.description = $description, t.created_at = datetime($now)
		ON MATCH SET t.description = CASE WHEN $description <> '' THEN $description ELSE t.description END
		RETURN t.id as id, t.name as name, t.description as description
	`

	result, err := session.Run(ctx, query, map[string]interface{}{
		"name":        name,
		"description": description,
		"topicID":     topicID,
		"now":         now,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create topic: %w", err)
	}

	if result.Next(ctx) {
		record := result.Record()
		return &Topic{
			ID:          getStringFromRecord(record, "id"),
			Name:        getStringFromRecord(record, "name"),
			Description: getStringFromRecord(record, "description"),
		}, nil
	}

	return nil, fmt.Errorf("failed to create topic")
}

// LinkTopics creates a relationship between two topics
func (r *Repository) LinkTopics(ctx context.Context, topic1, topic2, relationship string) error {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	// Sanitize relationship type
	if relationship == "" {
		relationship = "RELATED_TO"
	}

	query := fmt.Sprintf(`
		MATCH (t1:Topic {name: $topic1})
		MATCH (t2:Topic {name: $topic2})
		MERGE (t1)-[:%s]->(t2)
	`, relationship)

	_, err := session.Run(ctx, query, map[string]interface{}{
		"topic1": topic1,
		"topic2": topic2,
	})
	if err != nil {
		return fmt.Errorf("failed to link topics: %w", err)
	}

	r.logger.Info("Topics linked",
		zap.String("topic1", topic1),
		zap.String("topic2", topic2),
		zap.String("relationship", relationship),
	)

	return nil
}

// LinkUserToTopic links a user's interest to a topic
func (r *Repository) LinkUserToTopic(ctx context.Context, userID, topicName string) error {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	query := `
		MATCH (u:User {id: $userID})
		MERGE (t:Topic {name: $topicName})
		ON CREATE SET t.id = $topicID, t.created_at = datetime($now)
		MERGE (u)-[:INTERESTED_IN]->(t)
	`

	now := time.Now().UTC().Format(time.RFC3339)
	_, err := session.Run(ctx, query, map[string]interface{}{
		"userID":    userID,
		"topicName": topicName,
		"topicID":   uuid.New().String(),
		"now":       now,
	})
	if err != nil {
		return fmt.Errorf("failed to link user to topic: %w", err)
	}

	return nil
}

// GetRelatedTopics finds topics related to a given topic
func (r *Repository) GetRelatedTopics(ctx context.Context, topicName string, depth int) ([]Topic, error) {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	if depth < 1 {
		depth = 1
	}
	if depth > 5 {
		depth = 5
	}

	query := fmt.Sprintf(`
		MATCH (t:Topic {name: $topicName})-[:RELATED_TO|SUBTOPIC_OF*1..%d]-(related:Topic)
		WHERE related.name <> $topicName
		RETURN DISTINCT related.id as id, related.name as name, related.description as description
		LIMIT 20
	`, depth)

	result, err := session.Run(ctx, query, map[string]interface{}{
		"topicName": topicName,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get related topics: %w", err)
	}

	var topics []Topic
	for result.Next(ctx) {
		record := result.Record()
		topics = append(topics, Topic{
			ID:          getStringFromRecord(record, "id"),
			Name:        getStringFromRecord(record, "name"),
			Description: getStringFromRecord(record, "description"),
		})
	}

	return topics, nil
}

// ============================================================================
// Search Operations
// ============================================================================

// SearchMemory performs a comprehensive search across the graph
func (r *Repository) SearchMemory(ctx context.Context, agentID, query string, limit int) ([]SearchResult, error) {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	if limit < 1 {
		limit = 10
	}

	// Search across facts, memories, and topics
	searchQuery := `
		MATCH (a:Agent {id: $agentID})
		OPTIONAL MATCH (a)-[:KNOWS_FACT]->(f:Fact)
		WHERE toLower(f.content) CONTAINS toLower($query)
		WITH collect({type: 'fact', id: f.id, content: f.content, score: 1.0}) as facts
		
		OPTIONAL MATCH (a)-[:HAS_MEMORY]->(m:Memory)
		WHERE toLower(m.content) CONTAINS toLower($query) OR toLower(m.name) CONTAINS toLower($query)
		WITH facts, collect({type: 'memory', id: m.name, content: m.content, score: 1.0}) as memories
		
		OPTIONAL MATCH (t:Topic)
		WHERE toLower(t.name) CONTAINS toLower($query) OR toLower(t.description) CONTAINS toLower($query)
		WITH facts, memories, collect({type: 'topic', id: t.id, content: t.name + ': ' + COALESCE(t.description, ''), score: 0.8}) as topics
		
		RETURN facts + memories + topics as results
	`

	result, err := session.Run(ctx, searchQuery, map[string]interface{}{
		"agentID": agentID,
		"query":   query,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to search memory: %w", err)
	}

	var results []SearchResult
	if result.Next(ctx) {
		record := result.Record()
		if resultList, ok := record.Get("results"); ok {
			if items, ok := resultList.([]interface{}); ok {
				for _, item := range items {
					if m, ok := item.(map[string]interface{}); ok {
						content := getStringFromMap(m, "content", "")
						if content != "" {
							results = append(results, SearchResult{
								Type:    getStringFromMap(m, "type", "unknown"),
								ID:      getStringFromMap(m, "id", ""),
								Content: content,
								Score:   getFloat64FromMap(m, "score", 0.0),
							})
						}
					}
				}
			}
		}
	}

	// Limit results
	if len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

// ============================================================================
// Conversation Operations
// ============================================================================

// LogMessage logs a message and links it to user and conversation
func (r *Repository) LogMessage(ctx context.Context, agentID, userID, channelID, content, role, platform string) error {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	msgID := uuid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)

	query := `
		MATCH (a:Agent {id: $agentID})
		MERGE (u:User {id: $userID})
		MERGE (c:Conversation {channel_id: $channelID})
		ON CREATE SET c.id = $convID, c.platform = $platform, c.started_at = datetime($now)
		
		CREATE (m:Message {
			id: $msgID,
			content: $content,
			role: $role,
			platform: $platform,
			timestamp: datetime($now)
		})
		
		MERGE (u)-[:PARTICIPATED_IN]->(c)
		MERGE (c)-[:CONTAINS]->(m)
		
		WITH m, u, a
		FOREACH (ignored IN CASE WHEN $role = 'user' THEN [1] ELSE [] END |
			MERGE (u)-[:SENT]->(m)
		)
		FOREACH (ignored IN CASE WHEN $role = 'agent' THEN [1] ELSE [] END |
			MERGE (a)-[:SENT]->(m)
		)
	`

	_, err := session.Run(ctx, query, map[string]interface{}{
		"agentID":   agentID,
		"userID":    userID,
		"channelID": channelID,
		"convID":    uuid.New().String(),
		"msgID":     msgID,
		"content":   content,
		"role":      role,
		"platform":  platform,
		"now":       now,
	})
	if err != nil {
		return fmt.Errorf("failed to log message: %w", err)
	}

	return nil
}

// GetConversationHistory retrieves recent messages from a conversation
func (r *Repository) GetConversationHistory(ctx context.Context, channelID string, limit int) ([]Message, error) {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	if limit < 1 {
		limit = 20
	}

	query := `
		MATCH (c:Conversation {channel_id: $channelID})-[:CONTAINS]->(m:Message)
		RETURN m.id as id, m.content as content, m.role as role, 
		       m.platform as platform, m.timestamp as timestamp
		ORDER BY m.timestamp DESC
		LIMIT $limit
	`

	result, err := session.Run(ctx, query, map[string]interface{}{
		"channelID": channelID,
		"limit":     limit,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get conversation history: %w", err)
	}

	var messages []Message
	for result.Next(ctx) {
		record := result.Record()
		messages = append(messages, Message{
			ID:       getStringFromRecord(record, "id"),
			Content:  getStringFromRecord(record, "content"),
			Role:     getStringFromRecord(record, "role"),
			Platform: getStringFromRecord(record, "platform"),
		})
	}

	// Reverse to get chronological order
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}

	return messages, nil
}

// ============================================================================
// Discord Entity Operations
// ============================================================================

// CreateOrUpdateGuild creates or updates a Discord guild
func (r *Repository) CreateOrUpdateGuild(ctx context.Context, guildID, name string, memberCount int) (*Guild, error) {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	now := time.Now().UTC().Format(time.RFC3339)

	query := `
		MERGE (g:Guild {id: $guildID})
		ON CREATE SET 
			g.name = $name,
			g.member_count = $memberCount,
			g.created_at = datetime($now)
		ON MATCH SET 
			g.name = $name,
			g.member_count = $memberCount
		RETURN g.id as id, g.name as name, g.member_count as member_count
	`

	result, err := session.Run(ctx, query, map[string]interface{}{
		"guildID":    guildID,
		"name":       name,
		"memberCount": memberCount,
		"now":        now,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create/update guild: %w", err)
	}

	if result.Next(ctx) {
		record := result.Record()
		return &Guild{
			ID:          getStringFromRecord(record, "id"),
			Name:        getStringFromRecord(record, "name"),
			MemberCount: getIntFromRecord(record, "member_count"),
		}, nil
	}

	return nil, fmt.Errorf("failed to create guild")
}

// CreateOrUpdateChannel creates or updates a Discord channel
func (r *Repository) CreateOrUpdateChannel(ctx context.Context, channelID, name, channelType, topic, guildID string) (*Channel, error) {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	query := `
		MERGE (c:Channel {id: $channelID})
		SET c.name = $name,
		    c.type = $channelType,
		    c.topic = $topic,
		    c.guild_id = $guildID
		WITH c
		OPTIONAL MATCH (g:Guild {id: $guildID})
		FOREACH (ignored IN CASE WHEN g IS NOT NULL THEN [1] ELSE [] END |
			MERGE (g)-[:HAS_CHANNEL]->(c)
		)
		RETURN c.id as id, c.name as name, c.type as type, c.topic as topic, c.guild_id as guild_id
	`

	result, err := session.Run(ctx, query, map[string]interface{}{
		"channelID":  channelID,
		"name":       name,
		"channelType": channelType,
		"topic":      topic,
		"guildID":    guildID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create/update channel: %w", err)
	}

	if result.Next(ctx) {
		record := result.Record()
		return &Channel{
			ID:      getStringFromRecord(record, "id"),
			Name:    getStringFromRecord(record, "name"),
			Type:    getStringFromRecord(record, "type"),
			Topic:   getStringFromRecord(record, "topic"),
			GuildID: getStringFromRecord(record, "guild_id"),
		}, nil
	}

	return nil, fmt.Errorf("failed to create channel")
}

// CreateOrUpdateRole creates or updates a Discord role
func (r *Repository) CreateOrUpdateRole(ctx context.Context, roleID, name, guildID string, color int, permissions int64) (*Role, error) {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	query := `
		MERGE (r:Role {id: $roleID})
		SET r.name = $name,
		    r.color = $color,
		    r.permissions = $permissions,
		    r.guild_id = $guildID
		WITH r
		OPTIONAL MATCH (g:Guild {id: $guildID})
		FOREACH (ignored IN CASE WHEN g IS NOT NULL THEN [1] ELSE [] END |
			MERGE (g)-[:HAS_ROLE]->(r)
		)
		RETURN r.id as id, r.name as name, r.color as color, r.permissions as permissions, r.guild_id as guild_id
	`

	result, err := session.Run(ctx, query, map[string]interface{}{
		"roleID":     roleID,
		"name":       name,
		"guildID":    guildID,
		"color":      color,
		"permissions": permissions,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create/update role: %w", err)
	}

	if result.Next(ctx) {
		record := result.Record()
		return &Role{
			ID:          getStringFromRecord(record, "id"),
			Name:        getStringFromRecord(record, "name"),
			Color:       getIntFromRecord(record, "color"),
			Permissions: getInt64FromRecord(record, "permissions"),
			GuildID:     getStringFromRecord(record, "guild_id"),
		}, nil
	}

	return nil, fmt.Errorf("failed to create role")
}

// LinkUserToGuild links a user to a guild with roles
func (r *Repository) LinkUserToGuild(ctx context.Context, userID, guildID string, roleIDs []string) error {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	now := time.Now().UTC().Format(time.RFC3339)

	query := `
		MATCH (u:User {id: $userID})
		MATCH (g:Guild {id: $guildID})
		MERGE (u)-[m:MEMBER_OF]->(g)
		ON CREATE SET m.joined_at = datetime($now)
		ON MATCH SET m.last_seen = datetime($now)
		WITH u, g, m
		FOREACH (roleID IN $roleIDs |
			OPTIONAL MATCH (r:Role {id: roleID})
			FOREACH (ignored IN CASE WHEN r IS NOT NULL THEN [1] ELSE [] END |
				MERGE (u)-[:HAS_ROLE]->(r)
			)
		)
	`

	_, err := session.Run(ctx, query, map[string]interface{}{
		"userID":  userID,
		"guildID": guildID,
		"roleIDs": roleIDs,
		"now":     now,
	})
	if err != nil {
		return fmt.Errorf("failed to link user to guild: %w", err)
	}

	return nil
}

// ============================================================================
// User-to-User Relationship Operations
// ============================================================================

// RecordUserMention records when a user mentions another user
func (r *Repository) RecordUserMention(ctx context.Context, fromUserID, toUserID, context string) error {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	now := time.Now().UTC().Format(time.RFC3339)

	query := `
		MATCH (u1:User {id: $fromUserID})
		MATCH (u2:User {id: $toUserID})
		MERGE (u1)-[m:MENTIONED]->(u2)
		ON CREATE SET 
			m.count = 1,
			m.first_mentioned = datetime($now),
			m.last_mentioned = datetime($now),
			m.contexts = [$context]
		ON MATCH SET 
			m.count = m.count + 1,
			m.last_mentioned = datetime($now),
			m.contexts = CASE 
				WHEN $context IN m.contexts THEN m.contexts 
				ELSE m.contexts + $context 
			END
	`

	_, err := session.Run(ctx, query, map[string]interface{}{
		"fromUserID": fromUserID,
		"toUserID":   toUserID,
		"context":    context,
		"now":        now,
	})
	if err != nil {
		return fmt.Errorf("failed to record user mention: %w", err)
	}

	return nil
}

// RecordUserReply records when a user replies to another user
func (r *Repository) RecordUserReply(ctx context.Context, fromUserID, toUserID string, responseTime time.Duration) error {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	now := time.Now().UTC().Format(time.RFC3339)
	responseTimeSeconds := int64(responseTime.Seconds())

	query := `
		MATCH (u1:User {id: $fromUserID})
		MATCH (u2:User {id: $toUserID})
		MERGE (u1)-[r:REPLIED_TO]->(u2)
		ON CREATE SET 
			r.count = 1,
			r.first_reply = datetime($now),
			r.last_reply = datetime($now),
			r.total_response_time = $responseTimeSeconds,
			r.avg_response_time = $responseTimeSeconds
		ON MATCH SET 
			r.count = r.count + 1,
			r.last_reply = datetime($now),
			r.total_response_time = r.total_response_time + $responseTimeSeconds,
			r.avg_response_time = r.total_response_time / r.count
	`

	_, err := session.Run(ctx, query, map[string]interface{}{
		"fromUserID":     fromUserID,
		"toUserID":       toUserID,
		"responseTimeSeconds": responseTimeSeconds,
		"now":            now,
	})
	if err != nil {
		return fmt.Errorf("failed to record user reply: %w", err)
	}

	return nil
}

// RecordSharedTopic records when users share interest in topics
func (r *Repository) RecordSharedTopic(ctx context.Context, user1ID, user2ID string, topicNames []string) error {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	now := time.Now().UTC().Format(time.RFC3339)
	strength := float64(len(topicNames)) * 0.1
	if strength > 1.0 {
		strength = 1.0
	}

	query := `
		MATCH (u1:User {id: $user1ID})
		MATCH (u2:User {id: $user2ID})
		MERGE (u1)-[s:SHARED_TOPIC]->(u2)
		ON CREATE SET 
			s.topics = $topicNames,
			s.strength = $strength,
			s.first_shared = datetime($now),
			s.last_shared = datetime($now)
		ON MATCH SET 
			s.topics = CASE 
				WHEN ALL(topic IN $topicNames WHERE topic IN s.topics) THEN s.topics
				ELSE [topic IN s.topics WHERE topic IN $topicNames] + [topic IN $topicNames WHERE NOT topic IN s.topics]
			END,
			s.strength = size(s.topics) * 0.1,
			s.last_shared = datetime($now)
	`

	_, err := session.Run(ctx, query, map[string]interface{}{
		"user1ID":    user1ID,
		"user2ID":    user2ID,
		"topicNames": topicNames,
		"strength":   strength,
		"now":        now,
	})
	if err != nil {
		return fmt.Errorf("failed to record shared topic: %w", err)
	}

	return nil
}

// RecordCollaboration records when users collaborate in conversations
func (r *Repository) RecordCollaboration(ctx context.Context, user1ID, user2ID, conversationID string) error {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	now := time.Now().UTC().Format(time.RFC3339)

	query := `
		MATCH (u1:User {id: $user1ID})
		MATCH (u2:User {id: $user2ID})
		MERGE (u1)-[c:COLLABORATED]->(u2)
		ON CREATE SET 
			c.conversations = [$conversationID],
			c.count = 1,
			c.first_collaboration = datetime($now),
			c.last_collaboration = datetime($now)
		ON MATCH SET 
			c.conversations = CASE 
				WHEN $conversationID IN c.conversations THEN c.conversations
				ELSE c.conversations + $conversationID
			END,
			c.count = size(c.conversations),
			c.last_collaboration = datetime($now)
	`

	_, err := session.Run(ctx, query, map[string]interface{}{
		"user1ID":       user1ID,
		"user2ID":       user2ID,
		"conversationID": conversationID,
		"now":           now,
	})
	if err != nil {
		return fmt.Errorf("failed to record collaboration: %w", err)
	}

	return nil
}

// ============================================================================
// Enhanced Message Logging with Threading
// ============================================================================

// LogMessageWithThreading logs a message with threading support
func (r *Repository) LogMessageWithThreading(ctx context.Context, agentID, userID, channelID, content, role, platform string, replyToMessageID string, mentionedUserIDs []string) error {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	msgID := uuid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)

	query := `
		MATCH (a:Agent {id: $agentID})
		MERGE (u:User {id: $userID})
		MERGE (c:Conversation {channel_id: $channelID})
		ON CREATE SET c.id = $convID, c.platform = $platform, c.started_at = datetime($now)
		
		CREATE (m:Message {
			id: $msgID,
			content: $content,
			role: $role,
			platform: $platform,
			timestamp: datetime($now)
		})
		
		MERGE (u)-[:PARTICIPATED_IN]->(c)
		MERGE (c)-[:CONTAINS]->(m)
		
		WITH m, u, a
		FOREACH (ignored IN CASE WHEN $role = 'user' THEN [1] ELSE [] END |
			MERGE (u)-[:SENT]->(m)
		)
		FOREACH (ignored IN CASE WHEN $role = 'agent' THEN [1] ELSE [] END |
			MERGE (a)-[:SENT]->(m)
		)
		
		WITH m
		FOREACH (ignored IN CASE WHEN $replyToMessageID <> '' THEN [1] ELSE [] END |
			OPTIONAL MATCH (replyTo:Message {id: $replyToMessageID})
			FOREACH (ignored2 IN CASE WHEN replyTo IS NOT NULL THEN [1] ELSE [] END |
				MERGE (m)-[:REPLIES_TO]->(replyTo)
			)
		)
		
		WITH m
		FOREACH (mentionedID IN $mentionedUserIDs |
			OPTIONAL MATCH (mentioned:User {id: mentionedID})
			FOREACH (ignored IN CASE WHEN mentioned IS NOT NULL THEN [1] ELSE [] END |
				MERGE (m)-[:MENTIONS]->(mentioned)
			)
		)
	`

	_, err := session.Run(ctx, query, map[string]interface{}{
		"agentID":          agentID,
		"userID":           userID,
		"channelID":        channelID,
		"convID":           uuid.New().String(),
		"msgID":            msgID,
		"content":          content,
		"role":             role,
		"platform":         platform,
		"replyToMessageID": replyToMessageID,
		"mentionedUserIDs": mentionedUserIDs,
		"now":              now,
	})
	if err != nil {
		return fmt.Errorf("failed to log message with threading: %w", err)
	}

	// Record mentions
	if role == "user" && len(mentionedUserIDs) > 0 {
		for _, mentionedID := range mentionedUserIDs {
			if mentionedID != userID {
				_ = r.RecordUserMention(ctx, userID, mentionedID, channelID)
			}
		}
	}

	return nil
}

// ============================================================================
// Enhanced Topic Interest with Weighted Relationships
// ============================================================================

// LinkUserToTopicWeighted links a user to a topic with weighted relationship
func (r *Repository) LinkUserToTopicWeighted(ctx context.Context, userID, topicName string, strength float64) error {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	now := time.Now().UTC().Format(time.RFC3339)

	if strength < 0 {
		strength = 0
	}
	if strength > 1.0 {
		strength = 1.0
	}

	query := `
		MATCH (u:User {id: $userID})
		MERGE (t:Topic {name: $topicName})
		ON CREATE SET t.id = $topicID, t.created_at = datetime($now)
		MERGE (u)-[r:INTERESTED_IN]->(t)
		ON CREATE SET 
			r.strength = $strength,
			r.first_interaction = datetime($now),
			r.last_interaction = datetime($now),
			r.interaction_count = 1,
			r.recency_score = 1.0
		ON MATCH SET 
			r.strength = ($strength + r.strength) / 2.0,
			r.last_interaction = datetime($now),
			r.interaction_count = r.interaction_count + 1,
			r.recency_score = 1.0 - (duration.between(r.last_interaction, datetime($now)).days / 365.0)
	`

	_, err := session.Run(ctx, query, map[string]interface{}{
		"userID":    userID,
		"topicName": topicName,
		"topicID":   uuid.New().String(),
		"strength":  strength,
		"now":       now,
	})
	if err != nil {
		return fmt.Errorf("failed to link user to topic: %w", err)
	}

	return nil
}

// ============================================================================
// Activity Pattern Operations
// ============================================================================

// RecordActivityPattern records user activity patterns
func (r *Repository) RecordActivityPattern(ctx context.Context, userID string, hourOfDay int, messageLength int) error {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	now := time.Now()
	dayOfWeek := now.Weekday().String()
	nowStr := now.UTC().Format(time.RFC3339)

	query := `
		MATCH (u:User {id: $userID})
		MERGE (u)-[:ACTIVE_AT]->(ap:ActivityPattern {
			day_of_week: $dayOfWeek,
			hour_of_day: $hourOfDay
		})
		ON CREATE SET 
			ap.activity_count = 1,
			ap.total_message_length = $messageLength,
			ap.avg_message_length = $messageLength,
			ap.last_updated = datetime($now)
		ON MATCH SET 
			ap.activity_count = ap.activity_count + 1,
			ap.total_message_length = ap.total_message_length + $messageLength,
			ap.avg_message_length = ap.total_message_length / ap.activity_count,
			ap.last_updated = datetime($now)
	`

	_, err := session.Run(ctx, query, map[string]interface{}{
		"userID":       userID,
		"dayOfWeek":    dayOfWeek,
		"hourOfDay":    hourOfDay,
		"messageLength": messageLength,
		"now":          nowStr,
	})
	if err != nil {
		return fmt.Errorf("failed to record activity pattern: %w", err)
	}

	return nil
}

// ============================================================================
// Similarity and Recommendation Operations
// ============================================================================

// CalculateUserSimilarity calculates similarity between users based on shared interests
func (r *Repository) CalculateUserSimilarity(ctx context.Context, user1ID, user2ID string) (*UserSimilarity, error) {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	query := `
		MATCH (u1:User {id: $user1ID})
		MATCH (u2:User {id: $user2ID})
		
		OPTIONAL MATCH (u1)-[:INTERESTED_IN]->(t:Topic)<-[:INTERESTED_IN]-(u2)
		WITH u1, u2, collect(DISTINCT t.name) as shared_topics
		
		OPTIONAL MATCH (u1)-[:TOLD_ME]->(f:Fact)<-[:TOLD_ME]-(u2)
		WITH u1, u2, shared_topics, count(DISTINCT f) as shared_facts
		
		OPTIONAL MATCH (u1)-[:PARTICIPATED_IN]->(c:Conversation)<-[:PARTICIPATED_IN]-(u2)
		WITH u1, u2, shared_topics, shared_facts, count(DISTINCT c) as shared_conversations
		
		WITH u1, u2, shared_topics, shared_facts, shared_conversations,
		     (size(shared_topics) * 0.4 + shared_facts * 0.3 + shared_conversations * 0.3) as similarity
		
		RETURN similarity, shared_topics, shared_facts, shared_conversations
	`

	result, err := session.Run(ctx, query, map[string]interface{}{
		"user1ID": user1ID,
		"user2ID": user2ID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to calculate similarity: %w", err)
	}

	if result.Next(ctx) {
		record := result.Record()
		similarity := getFloat64FromRecord(record, "similarity")
		sharedTopics := getStringSliceFromRecord(record, "shared_topics")
		
		basedOn := "topics"
		if len(sharedTopics) == 0 {
			basedOn = "behavior"
		}

		return &UserSimilarity{
			User1ID:        user1ID,
			User2ID:        user2ID,
			SimilarityScore: similarity,
			BasedOn:        basedOn,
			SharedItems:    sharedTopics,
		}, nil
	}

	return nil, fmt.Errorf("users not found")
}

// FindSimilarUsers finds users similar to a given user
func (r *Repository) FindSimilarUsers(ctx context.Context, userID string, limit int) ([]UserSimilarity, error) {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	if limit < 1 {
		limit = 10
	}

	query := `
		MATCH (u1:User {id: $userID})
		MATCH (u2:User)
		WHERE u1 <> u2
		
		OPTIONAL MATCH (u1)-[:INTERESTED_IN]->(t:Topic)<-[:INTERESTED_IN]-(u2)
		WITH u1, u2, collect(DISTINCT t.name) as shared_topics
		
		OPTIONAL MATCH (u1)-[:TOLD_ME]->(f:Fact)<-[:TOLD_ME]-(u2)
		WITH u1, u2, shared_topics, count(DISTINCT f) as shared_facts
		
		OPTIONAL MATCH (u1)-[:PARTICIPATED_IN]->(c:Conversation)<-[:PARTICIPATED_IN]-(u2)
		WITH u1, u2, shared_topics, shared_facts, count(DISTINCT c) as shared_conversations
		
		WITH u1, u2, shared_topics, shared_facts, shared_conversations,
		     (size(shared_topics) * 0.4 + shared_facts * 0.3 + shared_conversations * 0.3) as similarity
		
		WHERE similarity > 0
		RETURN u2.id as user2_id, similarity, shared_topics
		ORDER BY similarity DESC
		LIMIT $limit
	`

	result, err := session.Run(ctx, query, map[string]interface{}{
		"userID": userID,
		"limit":  limit,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to find similar users: %w", err)
	}

	var similarities []UserSimilarity
	for result.Next(ctx) {
		record := result.Record()
		sharedTopics := getStringSliceFromRecord(record, "shared_topics")
		
		similarities = append(similarities, UserSimilarity{
			User1ID:        userID,
			User2ID:        getStringFromRecord(record, "user2_id"),
			SimilarityScore: getFloat64FromRecord(record, "similarity"),
			BasedOn:        "topics",
			SharedItems:    sharedTopics,
		})
	}

	return similarities, nil
}

// GetRecommendationsForUser gets conversation/topic recommendations for a user
func (r *Repository) GetRecommendationsForUser(ctx context.Context, userID string, limit int) ([]SearchResult, error) {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	if limit < 1 {
		limit = 10
	}

	query := `
		MATCH (u:User {id: $userID})-[:INTERESTED_IN]->(t:Topic)
		OPTIONAL MATCH (t)<-[:ABOUT_TOPIC]-(m:Message)
		WHERE NOT (u)-[:PARTICIPATED_IN]->(m)
		WITH t, m, count(DISTINCT m) as relevance
		ORDER BY relevance DESC, m.timestamp DESC
		LIMIT $limit
		RETURN 
			'conversation' as type,
			m.id as id,
			m.content as content,
			relevance * 0.1 as score,
			{topic: t.name, timestamp: m.timestamp} as metadata
	`

	result, err := session.Run(ctx, query, map[string]interface{}{
		"userID": userID,
		"limit":  limit,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get recommendations: %w", err)
	}

	var results []SearchResult
	for result.Next(ctx) {
		record := result.Record()
		results = append(results, SearchResult{
			Type:    getStringFromRecord(record, "type"),
			ID:      getStringFromRecord(record, "id"),
			Content: getStringFromRecord(record, "content"),
			Score:   getFloat64FromRecord(record, "score"),
		})
	}

	return results, nil
}

// ============================================================================
// Enhanced Fact Relationships
// ============================================================================

// LinkFactRelationships links facts with support/contradict/related relationships
func (r *Repository) LinkFactRelationships(ctx context.Context, fact1ID, fact2ID, relationship string) error {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	// Validate relationship type
	validRelationships := map[string]bool{
		"SUPPORTS":    true,
		"CONTRADICTS": true,
		"RELATED_TO":  true,
	}
	if !validRelationships[relationship] {
		relationship = "RELATED_TO"
	}

	query := fmt.Sprintf(`
		MATCH (f1:Fact {id: $fact1ID})
		MATCH (f2:Fact {id: $fact2ID})
		MERGE (f1)-[r:%s]->(f2)
		ON CREATE SET r.created_at = datetime()
		ON MATCH SET r.last_updated = datetime()
	`, relationship)

	_, err := session.Run(ctx, query, map[string]interface{}{
		"fact1ID": fact1ID,
		"fact2ID": fact2ID,
	})
	if err != nil {
		return fmt.Errorf("failed to link fact relationships: %w", err)
	}

	return nil
}

// RecordFactVerification records when a user verifies or challenges a fact
func (r *Repository) RecordFactVerification(ctx context.Context, factID, userID string, verified bool) error {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	now := time.Now().UTC().Format(time.RFC3339)
	relType := "VERIFIED_BY"
	if !verified {
		relType = "CHALLENGED_BY"
	}

	query := fmt.Sprintf(`
		MATCH (f:Fact {id: $factID})
		MATCH (u:User {id: $userID})
		MERGE (f)<-[r:%s]-(u)
		ON CREATE SET 
			r.count = 1,
			r.first_%s = datetime($now),
			r.last_%s = datetime($now)
		ON MATCH SET 
			r.count = r.count + 1,
			r.last_%s = datetime($now)
	`, relType, relType, relType, relType)

	_, err := session.Run(ctx, query, map[string]interface{}{
		"factID": factID,
		"userID": userID,
		"now":    now,
	})
	if err != nil {
		return fmt.Errorf("failed to record fact verification: %w", err)
	}

	return nil
}

// ============================================================================
// Helper Functions
// ============================================================================

func getStringFromRecord(record *neo4j.Record, key string) string {
	val, ok := record.Get(key)
	if !ok || val == nil {
		return ""
	}
	if str, ok := val.(string); ok {
		return str
	}
	return ""
}

func getIntFromRecord(record *neo4j.Record, key string) int {
	val, ok := record.Get(key)
	if !ok || val == nil {
		return 0
	}
	if i, ok := val.(int64); ok {
		return int(i)
	}
	if i, ok := val.(int); ok {
		return i
	}
	return 0
}

func getInt64FromRecord(record *neo4j.Record, key string) int64 {
	val, ok := record.Get(key)
	if !ok || val == nil {
		return 0
	}
	if i, ok := val.(int64); ok {
		return i
	}
	if i, ok := val.(int); ok {
		return int64(i)
	}
	return 0
}

func getFloat64FromRecord(record *neo4j.Record, key string) float64 {
	val, ok := record.Get(key)
	if !ok || val == nil {
		return 0.0
	}
	if f, ok := val.(float64); ok {
		return f
	}
	if i, ok := val.(int64); ok {
		return float64(i)
	}
	return 0.0
}

func getStringSliceFromRecord(record *neo4j.Record, key string) []string {
	val, ok := record.Get(key)
	if !ok || val == nil {
		return []string{}
	}
	if slice, ok := val.([]interface{}); ok {
		result := make([]string, 0, len(slice))
		for _, v := range slice {
			if str, ok := v.(string); ok {
				result = append(result, str)
			}
		}
		return result
	}
	return []string{}
}

// ============================================================================
// Memory Deduplication Functions
// ============================================================================

// deduplicateFacts removes exact duplicates and very similar facts
func deduplicateFacts(facts []Fact) []Fact {
	if len(facts) <= 1 {
		return facts
	}

	seen := make(map[string]bool)
	var unique []Fact

	for _, fact := range facts {
		// Normalize content for comparison
		normalized := normalizeFactContent(fact.Content)
		
		// Check for exact duplicates
		if seen[normalized] {
			continue
		}

		// Check for very similar facts (simple string similarity)
		isDuplicate := false
		for seenContent := range seen {
			if areFactsSimilar(normalized, seenContent) {
				isDuplicate = true
				break
			}
		}

		if !isDuplicate {
			seen[normalized] = true
			unique = append(unique, fact)
		}
	}

	return unique
}

// normalizeFactContent normalizes fact content for comparison
func normalizeFactContent(content string) string {
	// Lowercase, trim, remove extra spaces
	content = strings.ToLower(strings.TrimSpace(content))
	content = regexp.MustCompile(`\s+`).ReplaceAllString(content, " ")
	// Remove trailing punctuation for better matching
	content = strings.TrimRight(content, ".,!?;:")
	return content
}

// areFactsSimilar checks if two facts are similar enough to be considered duplicates
func areFactsSimilar(content1, content2 string) bool {
	// Simple similarity check - can be enhanced with proper string similarity algorithms
	// For now, check if one contains the other (for very similar facts)
	if len(content1) < 10 || len(content2) < 10 {
		return false
	}

	// Check if they're very similar (one is substring of other with small differences)
	if strings.Contains(content1, content2) || strings.Contains(content2, content1) {
		// If one is 80%+ of the other, consider similar
		len1, len2 := len(content1), len(content2)
		ratio := float64(min(len1, len2)) / float64(max(len1, len2))
		return ratio >= 0.8
	}

	// Check for word overlap (if 70%+ words match, consider similar)
	words1 := strings.Fields(content1)
	words2 := strings.Fields(content2)
	if len(words1) == 0 || len(words2) == 0 {
		return false
	}

	// Count matching words
	matches := 0
	wordSet := make(map[string]bool)
	for _, word := range words1 {
		if len(word) > 3 { // Only consider words longer than 3 chars
			wordSet[word] = true
		}
	}
	for _, word := range words2 {
		if len(word) > 3 && wordSet[word] {
			matches++
		}
	}

	// If 70%+ of words match, consider similar
	avgWords := (len(words1) + len(words2)) / 2
	if avgWords > 0 {
		similarity := float64(matches) / float64(avgWords)
		return similarity >= 0.7
	}

	return false
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// max returns the maximum of two integers
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// DeleteFact deletes a fact by ID
func (r *Repository) DeleteFact(ctx context.Context, factID string) error {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	query := `
		MATCH (f:Fact {id: $factID})
		DETACH DELETE f
	`

	_, err := session.Run(ctx, query, map[string]interface{}{
		"factID": factID,
	})
	if err != nil {
		return fmt.Errorf("failed to delete fact: %w", err)
	}

	r.logger.Info("Fact deleted",
		zap.String("fact_id", factID),
	)
	return nil
}

