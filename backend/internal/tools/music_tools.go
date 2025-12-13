package tools

import (
	"ezra-clone/backend/internal/adapter"
)

// GetMusicTools returns music playback and generation tools
func GetMusicTools() []adapter.Tool {
	return []adapter.Tool{
		{
			Type: "function",
			Function: adapter.FunctionDefinition{
				Name:        ToolMusicPlay,
				Description: "Play music from a URL or search query. Supports YouTube, Spotify, and SoundCloud. If a query is provided, searches YouTube for the song.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"query": map[string]interface{}{
							"type":        "string",
							"description": "Song URL (YouTube, Spotify, SoundCloud) or search query",
						},
						"guild_id": map[string]interface{}{
							"type":        "string",
							"description": "Discord guild ID (leave empty for current guild)",
						},
						"channel_id": map[string]interface{}{
							"type":        "string",
							"description": "Voice channel ID to join (leave empty to use user's current voice channel)",
						},
					},
					"required": []string{"query"},
				},
			},
		},
		{
			Type: "function",
			Function: adapter.FunctionDefinition{
				Name:        ToolMusicPlaylist,
				Description: "Generate and play a smart playlist based on a query (e.g., 'cyberpunk synthwave', 'chill lofi beats', 'upbeat workout music'). Uses AI to generate song suggestions. The bot will automatically detect and join the user's voice channel if they are in one. Always use this tool when the user asks for a playlist - do not ask them to join a voice channel first, just use the tool and it will handle voice channel detection automatically.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"query": map[string]interface{}{
							"type":        "string",
							"description": "Playlist theme or description (e.g., 'chill lofi beats', 'upbeat workout music')",
						},
						"guild_id": map[string]interface{}{
							"type":        "string",
							"description": "Discord guild ID (leave empty for current guild)",
						},
						"channel_id": map[string]interface{}{
							"type":        "string",
							"description": "Voice channel ID to join. If the user provides a channel ID in their message (a long number like '549642809574162484'), use that here. Otherwise, leave empty to automatically detect the user's current voice channel.",
						},
					},
					"required": []string{"query"},
				},
			},
		},
		{
			Type: "function",
			Function: adapter.FunctionDefinition{
				Name:        ToolMusicQueue,
				Description: "View the current music queue for a guild.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"guild_id": map[string]interface{}{
							"type":        "string",
							"description": "Discord guild ID (leave empty for current guild)",
						},
					},
					"required": []string{},
				},
			},
		},
		{
			Type: "function",
			Function: adapter.FunctionDefinition{
				Name:        ToolMusicSkip,
				Description: "Skip the currently playing song.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"guild_id": map[string]interface{}{
							"type":        "string",
							"description": "Discord guild ID (leave empty for current guild)",
						},
					},
					"required": []string{},
				},
			},
		},
		{
			Type: "function",
			Function: adapter.FunctionDefinition{
				Name:        ToolMusicPause,
				Description: "Pause the currently playing song.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"guild_id": map[string]interface{}{
							"type":        "string",
							"description": "Discord guild ID (leave empty for current guild)",
						},
					},
					"required": []string{},
				},
			},
		},
		{
			Type: "function",
			Function: adapter.FunctionDefinition{
				Name:        ToolMusicResume,
				Description: "Resume the paused song.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"guild_id": map[string]interface{}{
							"type":        "string",
							"description": "Discord guild ID (leave empty for current guild)",
						},
					},
					"required": []string{},
				},
			},
		},
		{
			Type: "function",
			Function: adapter.FunctionDefinition{
				Name:        ToolMusicStop,
				Description: "Stop playback and clear the queue.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"guild_id": map[string]interface{}{
							"type":        "string",
							"description": "Discord guild ID (leave empty for current guild)",
						},
					},
					"required": []string{},
				},
			},
		},
		{
			Type: "function",
			Function: adapter.FunctionDefinition{
				Name:        ToolMusicVolume,
				Description: "Set the playback volume (0-100).",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"volume": map[string]interface{}{
							"type":        "integer",
							"description": "Volume level (0-100, default: 100)",
						},
						"guild_id": map[string]interface{}{
							"type":        "string",
							"description": "Discord guild ID (leave empty for current guild)",
						},
					},
					"required": []string{"volume"},
				},
			},
		},
		{
			Type: "function",
			Function: adapter.FunctionDefinition{
				Name:        ToolMusicRadio,
				Description: "Start or stop infinite radio mode. Radio mode continuously generates and plays songs based on a seed.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"action": map[string]interface{}{
							"type":        "string",
							"enum":        []string{"start", "stop"},
							"description": "Start or stop radio mode",
						},
						"seed": map[string]interface{}{
							"type":        "string",
							"description": "Seed query for radio mode (required when starting)",
						},
						"guild_id": map[string]interface{}{
							"type":        "string",
							"description": "Discord guild ID (leave empty for current guild)",
						},
					},
					"required": []string{"action"},
				},
			},
		},
		{
			Type: "function",
			Function: adapter.FunctionDefinition{
				Name:        ToolMusicDisconnect,
				Description: "Disconnect from the voice channel. Stops all playback and leaves the voice channel.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"guild_id": map[string]interface{}{
							"type":        "string",
							"description": "Discord guild ID (leave empty for current guild)",
						},
					},
					"required": []string{},
				},
			},
		},
	}
}

