package graph

import (
	"context"
	"fmt"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

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

