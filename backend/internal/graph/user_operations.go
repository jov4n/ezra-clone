package graph

import (
	"context"
	"fmt"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

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

