package radarr

// Movie represents a Radarr movie record.
type Movie struct {
	ID            int      `json:"id"`
	Title         string   `json:"title"`
	Year          int      `json:"year"`
	Overview      string   `json:"overview"`
	Runtime       int      `json:"runtime"` // minutes
	Genres        []string `json:"genres"`
	HasFile       bool     `json:"hasFile"`
	Monitored     bool     `json:"monitored"`
	Certification string   `json:"certification"`
}
