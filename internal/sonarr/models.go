package sonarr

// Series represents a Sonarr series record.
type Series struct {
	ID       int      `json:"id"`
	Title    string   `json:"title"`
	Year     int      `json:"year"`
	Overview string   `json:"overview"`
	Genres   []string `json:"genres"`
	Runtime  int      `json:"runtime"` // minutes
}

// Episode represents a Sonarr episode record.
type Episode struct {
	ID            int    `json:"id"`
	SeriesID      int    `json:"seriesId"`
	Title         string `json:"title"`
	Overview      string `json:"overview"`
	SeasonNumber  int    `json:"seasonNumber"`
	EpisodeNumber int    `json:"episodeNumber"`
	Runtime       int    `json:"runtime"` // minutes
	HasFile       bool   `json:"hasFile"`
}
