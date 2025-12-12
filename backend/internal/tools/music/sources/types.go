package sources

// Song represents a track (local to sources package to avoid import cycles)
type Song struct {
	Title     string
	URL       string
	Duration  string
	Thumbnail string
	Requester string
	Source    string // "youtube", "spotify", "soundcloud", "twitch"
}

