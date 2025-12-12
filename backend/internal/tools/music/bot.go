package music

import (
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
)

// Constants for music bot configuration
const (
	// MaxRadioHistorySize is the maximum number of songs to keep in radio history
	MaxRadioHistorySize = 100

	// DefaultMaxQueueSize is the default maximum size for the music queue
	DefaultMaxQueueSize = 500
)

// Song represents a track in the queue
type Song struct {
	Title     string
	URL       string
	Duration  string
	Thumbnail string
	Requester string
	Source    string // "youtube", "spotify", "soundcloud", "twitch"
}

// Playlist represents a queue of songs
type Playlist struct {
	Songs   []Song
	Current int
	mu      sync.Mutex
	Loop    bool
	Shuffle bool
}

// Lock locks the playlist mutex
func (p *Playlist) Lock() {
	p.mu.Lock()
}

// Unlock unlocks the playlist mutex
func (p *Playlist) Unlock() {
	p.mu.Unlock()
}

// PreloadedSong holds preloaded audio data for seamless transitions
type PreloadedSong struct {
	Song        Song
	Preloaded   bool
	Buffer      []byte
	BufferPos   int
	StreamReady chan bool
	Cancel      func()
	YtdlpCmd    interface{}
	OpusOut     interface{}
	Mu          sync.Mutex
	Reading     bool
	ReadErr     error
	Done        chan struct{} // Signals when background goroutine is done
}

// MusicBot represents a Discord music bot instance for a single guild
type MusicBot struct {
	GuildID         string
	Session         *discordgo.Session
	VoiceConn       *discordgo.VoiceConnection
	Playlist        *Playlist
	IsPlaying       bool
	IsSpeaking      bool
	SkipChan        chan bool
	StopChan        chan bool
	Preloaded       *PreloadedSong
	Mu              sync.Mutex
	PreloadMu       sync.Mutex
	NowPlayingMsgID string
	QueueMsgID      string
	QueueChannelID  string
	QueuePage       int
	QueueMu         sync.Mutex

	// Pause/Resume control
	IsPaused   bool
	PauseChan  chan bool
	ResumeChan chan bool

	// Seek control
	SeekChan      chan time.Duration
	CurrentPos    time.Duration // Current playback position
	SongStartTime time.Time     // When the current song started playing
	PausedAt      time.Duration // Position when paused

	// Radio mode fields
	RadioEnabled    bool
	RadioSeed       string
	RadioHistoryMap map[string]struct{} // URLs of songs already played (O(1) lookup)
	RadioChannelID  string              // Channel to send radio notifications
	RadioMu         sync.Mutex
	RadioRefilling  bool // Prevents concurrent refills

	// Playlist generation progress tracking
	GeneratingPlaylistMsgID     string     // Message ID for "generating playlist" message
	GeneratingPlaylistChannelID string     // Channel ID for the generating message
	GeneratingPlaylistMu        sync.Mutex // Mutex for generating playlist message updates
}

// NewMusicBot creates a new MusicBot instance for a guild
func NewMusicBot(guildID string, session *discordgo.Session) *MusicBot {
	return &MusicBot{
		GuildID:         guildID,
		Session:         session,
		Playlist:        &Playlist{Songs: make([]Song, 0), Current: -1},
		SkipChan:        make(chan bool, 1),
		StopChan:        make(chan bool, 1),
		PauseChan:       make(chan bool, 1),
		ResumeChan:      make(chan bool, 1),
		SeekChan:        make(chan time.Duration, 1),
		RadioHistoryMap: make(map[string]struct{}),
	}
}

// ClearRadioState disables radio mode and clears history
func (b *MusicBot) ClearRadioState() {
	b.RadioMu.Lock()
	defer b.RadioMu.Unlock()
	b.RadioEnabled = false
	b.RadioSeed = ""
	b.RadioHistoryMap = make(map[string]struct{})
	b.RadioChannelID = ""
	b.RadioRefilling = false
}

// AddToRadioHistory adds a song URL to the radio history
func (b *MusicBot) AddToRadioHistory(url string) {
	b.RadioMu.Lock()
	defer b.RadioMu.Unlock()
	b.RadioHistoryMap[url] = struct{}{}
	// Keep history limited to prevent memory growth
	if len(b.RadioHistoryMap) > MaxRadioHistorySize {
		// Remove a random entry (maps don't guarantee order, so just delete first found)
		for k := range b.RadioHistoryMap {
			delete(b.RadioHistoryMap, k)
			break
		}
	}
}

// IsInRadioHistory checks if a URL has already been played (O(1) lookup)
func (b *MusicBot) IsInRadioHistory(url string) bool {
	b.RadioMu.Lock()
	defer b.RadioMu.Unlock()
	_, exists := b.RadioHistoryMap[url]
	return exists
}

// GetRadioHistoryCount returns the number of songs in radio history
func (b *MusicBot) GetRadioHistoryCount() int {
	b.RadioMu.Lock()
	defer b.RadioMu.Unlock()
	return len(b.RadioHistoryMap)
}

// GetRecentRadioSongs returns the titles of recently played songs for context
func (b *MusicBot) GetRecentRadioSongs(count int) []string {
	b.Playlist.Lock()
	defer b.Playlist.Unlock()

	var recent []string
	start := b.Playlist.Current - count + 1
	if start < 0 {
		start = 0
	}

	for i := start; i <= b.Playlist.Current && i < len(b.Playlist.Songs); i++ {
		recent = append(recent, b.Playlist.Songs[i].Title)
	}
	return recent
}

// MusicManager manages music bot instances per guild
type MusicManager struct {
	bots map[string]*MusicBot
	mu   sync.RWMutex
}

// NewMusicManager creates a new music manager
func NewMusicManager() *MusicManager {
	return &MusicManager{
		bots: make(map[string]*MusicBot),
	}
}

// GetBot gets or creates a music bot for a guild
func (m *MusicManager) GetBot(guildID string, session *discordgo.Session) *MusicBot {
	m.mu.Lock()
	defer m.mu.Unlock()

	if bot, exists := m.bots[guildID]; exists {
		return bot
	}

	bot := NewMusicBot(guildID, session)
	m.bots[guildID] = bot
	return bot
}

// RemoveBot removes a music bot for a guild (cleanup)
func (m *MusicManager) RemoveBot(guildID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if bot, exists := m.bots[guildID]; exists {
		// Cleanup voice connection if exists
		if bot.VoiceConn != nil {
			bot.VoiceConn.Disconnect()
		}
		delete(m.bots, guildID)
	}
}
