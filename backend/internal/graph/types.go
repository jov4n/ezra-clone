package graph

import "time"

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

