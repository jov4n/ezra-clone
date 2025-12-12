package music

import (
	"ezra-clone/backend/internal/tools/music/ui"
	"github.com/bwmarrin/discordgo"
)

// convertSongToUI converts music.Song to ui.Song
func convertSongToUI(song Song) ui.Song {
	return ui.Song{
		Title:     song.Title,
		URL:       song.URL,
		Duration:  song.Duration,
		Thumbnail: song.Thumbnail,
		Requester: song.Requester,
		Source:    song.Source,
	}
}

// convertPlaylistToUI converts music.Playlist to ui.Playlist
func convertPlaylistToUI(playlist *Playlist) *ui.Playlist {
	playlist.Lock()
	defer playlist.Unlock()

	uiSongs := make([]ui.Song, len(playlist.Songs))
	for i, s := range playlist.Songs {
		uiSongs[i] = convertSongToUI(s)
	}

	return &ui.Playlist{
		Songs:   uiSongs,
		Current: playlist.Current,
		Loop:    playlist.Loop,
		Shuffle: playlist.Shuffle,
	}
}

// CreateSongAddedEmbed wraps ui.CreateSongAddedEmbed
func CreateSongAddedEmbed(song Song, position int) *discordgo.MessageEmbed {
	return ui.CreateSongAddedEmbed(convertSongToUI(song), position)
}

// CreateQueueEmbed wraps ui.CreateQueueEmbed
func CreateQueueEmbed(playlist *Playlist, page int) *discordgo.MessageEmbed {
	uiPlaylist := convertPlaylistToUI(playlist)
	return ui.CreateQueueEmbed(uiPlaylist, page)
}

