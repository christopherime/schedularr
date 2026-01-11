package tunarr

// Channel represents a Tunarr TV channel.
type Channel struct {
	ID      string `json:"id"`
	Number  int    `json:"number"`
	Name    string `json:"name"`
	Icon    string `json:"icon"`
	Group   string `json:"groupTitle"`
	Enabled bool   `json:"enabled"`
}

// Program represents a media program (movie, episode, or track) in Tunarr.
type Program struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Year      int      `json:"year"`
	Summary   string   `json:"summary"`
	Duration  int64    `json:"duration"` // in milliseconds
	Rating    string   `json:"rating"`
	Genres    []string `json:"genres"`
	Type      string   `json:"type"` // movie, episode, track
	ShowTitle string   `json:"showTitle,omitempty"`
	Season    int      `json:"season,omitempty"`
	Episode   int      `json:"episode,omitempty"`
}
