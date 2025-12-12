package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

// Song represents a track (duplicated here to avoid import cycle)
type Song struct {
	Title     string
	URL       string
	Duration  string
	Thumbnail string
	Requester string
	Source    string
}

// Playlist represents a queue (duplicated here to avoid import cycle)
type Playlist struct {
	Songs   []Song
	Current int
	Loop    bool
	Shuffle bool
	mu      interface{} // Placeholder for sync.Mutex
}

const (
	// Embed colors
	ColorSuccess = 0x2ecc71  // Green
	ColorError   = 0xe74c3c  // Red
	ColorInfo    = 0x3498db  // Blue
	ColorWarning = 0xf39c12  // Orange
	ColorPurple  = 0x9b59b6  // Purple
	ColorGray    = 0x95a5a6  // Gray
)

// getSourceIcon returns an emoji/icon for the source
func getSourceIcon(source string) string {
	switch source {
	case "youtube":
		return "▶️"
	case "spotify":
		return "🎵"
	case "soundcloud":
		return "🎧"
	case "twitch":
		return "📺"
	default:
		return "🎶"
	}
}

// getSourceName returns a formatted source name
func getSourceName(source string) string {
	switch source {
	case "youtube":
		return "YouTube"
	case "spotify":
		return "Spotify"
	case "soundcloud":
		return "SoundCloud"
	case "twitch":
		return "Twitch"
	default:
		return "Unknown"
	}
}

// CreateNowPlayingEmbed creates a now playing embed
func CreateNowPlayingEmbed(song Song, position, total int) *discordgo.MessageEmbed {
	sourceIcon := getSourceIcon(song.Source)
	sourceName := getSourceName(song.Source)

	var image *discordgo.MessageEmbedImage
	var thumbnail *discordgo.MessageEmbedThumbnail
	if song.Thumbnail != "" {
		image = &discordgo.MessageEmbedImage{
			URL: song.Thumbnail,
		}
		thumbnail = &discordgo.MessageEmbedThumbnail{
			URL: song.Thumbnail,
		}
	}

	embed := &discordgo.MessageEmbed{
		Title:       "🎵 Now Playing",
		Description: fmt.Sprintf("**[%s](%s)**", song.Title, song.URL),
		Color:       ColorSuccess,
		Image:       image,
		Thumbnail:   thumbnail,
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:   "⏱️ Duration",
				Value:  song.Duration,
				Inline: true,
			},
			{
				Name:   "👤 Requested by",
				Value:  song.Requester,
				Inline: true,
			},
			{
				Name:   fmt.Sprintf("%s Source", sourceIcon),
				Value:  sourceName,
				Inline: true,
			},
		},
		Footer: &discordgo.MessageEmbedFooter{
			Text: fmt.Sprintf("📊 Position: %d/%d", position, total),
		},
		Timestamp: time.Now().Format(time.RFC3339),
	}

	return embed
}

// CreateSongAddedEmbed creates a song added embed
func CreateSongAddedEmbed(song Song, position int) *discordgo.MessageEmbed {
	sourceIcon := getSourceIcon(song.Source)
	sourceName := getSourceName(song.Source)

	var thumbnail *discordgo.MessageEmbedThumbnail
	if song.Thumbnail != "" {
		thumbnail = &discordgo.MessageEmbedThumbnail{
			URL: song.Thumbnail,
		}
	}

	return &discordgo.MessageEmbed{
		Title:       "✅ Song Added to Queue",
		Description: fmt.Sprintf("**[%s](%s)**", song.Title, song.URL),
		Color:       ColorSuccess,
		Thumbnail:   thumbnail,
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:   "⏱️ Duration",
				Value:  song.Duration,
				Inline: true,
			},
			{
				Name:   "📍 Position",
				Value:  fmt.Sprintf("#%d", position),
				Inline: true,
			},
			{
				Name:   fmt.Sprintf("%s Source", sourceIcon),
				Value:  sourceName,
				Inline: true,
			},
		},
		Footer: &discordgo.MessageEmbedFooter{
			Text: "🎵 Music will start playing automatically",
		},
		Timestamp: time.Now().Format(time.RFC3339),
	}
}

// CreateQueueEmbed creates a queue embed
func CreateQueueEmbed(playlist *Playlist, page int) *discordgo.MessageEmbed {
	// Note: playlist.Lock() would be called by caller if needed

	if len(playlist.Songs) == 0 {
		return &discordgo.MessageEmbed{
			Title:       "📋 Queue",
			Description: "The queue is empty! Use music tools to add songs.",
			Color:       ColorGray,
			Timestamp:   time.Now().Format(time.RFC3339),
			Footer: &discordgo.MessageEmbedFooter{
				Text: "💡 Tip: Add songs with music_play or music_playlist",
			},
		}
	}

	const songsPerPage = 10
	totalSongs := len(playlist.Songs)
	totalPages := (totalSongs + songsPerPage - 1) / songsPerPage

	// Clamp page to valid range
	if page < 0 {
		page = 0
	}
	if page >= totalPages {
		page = totalPages - 1
	}

	startIdx := page * songsPerPage
	endIdx := startIdx + songsPerPage
	if endIdx > totalSongs {
		endIdx = totalSongs
	}

	var queueText strings.Builder
	queueText.WriteString(fmt.Sprintf("**📊 Total Songs:** %d\n\n", totalSongs))

	for i := startIdx; i < endIdx; i++ {
		song := playlist.Songs[i]
		marker := "▫️"
		if i == playlist.Current {
			marker = "▶️"
		}
		sourceIcon := getSourceIcon(song.Source)
		title := song.Title
		if len(title) > 50 {
			title = title[:47] + "..."
		}
		queueText.WriteString(fmt.Sprintf("%s **%d.** [%s](%s) %s *%s*\n",
			marker, i+1, title, song.URL, sourceIcon, song.Duration))
	}

	footerText := fmt.Sprintf("Page %d/%d", page+1, totalPages)
	if totalPages <= 1 {
		footerText = "Use music_queue to refresh"
	}

	return &discordgo.MessageEmbed{
		Title:       "📋 Queue",
		Description: queueText.String(),
		Color:       ColorInfo,
		Footer: &discordgo.MessageEmbedFooter{
			Text: footerText,
		},
		Timestamp: time.Now().Format(time.RFC3339),
	}
}

// CreateErrorEmbed creates an error embed
func CreateErrorEmbed(message string) *discordgo.MessageEmbed {
	return &discordgo.MessageEmbed{
		Title:       "❌ Error",
		Description: message,
		Color:       ColorError,
		Timestamp:   time.Now().Format(time.RFC3339),
		Footer: &discordgo.MessageEmbedFooter{
			Text: "If this issue persists, please contact support",
		},
	}
}

// CreateEmbed creates a basic embed
func CreateEmbed(title, description, footer string, color int) *discordgo.MessageEmbed {
	embed := &discordgo.MessageEmbed{
		Title:       title,
		Description: description,
		Color:       color,
		Timestamp:   time.Now().Format(time.RFC3339),
	}

	if footer != "" {
		embed.Footer = &discordgo.MessageEmbedFooter{
			Text: footer,
		}
	}

	return embed
}

