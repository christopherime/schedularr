package tunarr

// Channel represents a Tunarr TV channel.
type Channel struct {
	ID      string `json:"id" validate:"required"`
	Number  int    `json:"number"`
	Name    string `json:"name" validate:"required"`
	Icon    string `json:"icon"`
	Group   string `json:"groupTitle"`
	Enabled bool   `json:"enabled"`
}

// Program represents a media program (movie, episode, or track) in Tunarr.
type Program struct {
	ID        string   `json:"id" validate:"required"`
	Title     string   `json:"title" validate:"required"`
	Year      int      `json:"year"`
	Summary   string   `json:"summary"`
	Duration  int64    `json:"duration" validate:"gt=0"` // in milliseconds
	Rating    string   `json:"rating"`
	Genres    []string `json:"genres"`
	Type      string   `json:"type" validate:"omitempty,oneof=movie episode track"` // movie, episode, track
	ShowTitle string   `json:"showTitle,omitempty"`
	Season    int      `json:"season,omitempty"`
	Episode   int      `json:"episode,omitempty"`
}

// Library represents a media library (Plex/Jellyfin/Emby).
type Library struct {
	ID     string `json:"id" validate:"required"`
	Name   string `json:"name" validate:"required"`
	Type   string `json:"type"`   // movie, show, music
	Server string `json:"server"` // plex, jellyfin, emby
}

// Show represents a TV show with metadata.
type Show struct {
	ID       string   `json:"id" validate:"required"`
	Title    string   `json:"title" validate:"required"`
	Year     int      `json:"year"`
	Summary  string   `json:"summary"`
	Genres   []string `json:"genres"`
	Rating   string   `json:"rating"`
	Seasons  int      `json:"seasons"`
	Episodes int      `json:"episodes"`
}

// FillerList represents a collection of filler content.
type FillerList struct {
	ID    string `json:"id" validate:"required"`
	Name  string `json:"name" validate:"required"`
	Count int    `json:"count"` // number of items
}
