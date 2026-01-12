package scheduler

import (
	"time"
)

// SeriesState tracks the playback progress of a specific series.
type SeriesState struct {
	ShowTitle      string    `json:"show_title" db:"show_title"`           // Primary key (or part of it)
	CurrentSeason  int       `json:"current_season" db:"current_season"`   // Next season to air
	CurrentEpisode int       `json:"current_episode" db:"current_episode"` // Next episode to air
	Completed      bool      `json:"completed" db:"completed"`             // True if all available episodes have aired
	LastAired      time.Time `json:"last_aired" db:"last_aired"`           // Timestamp of last successful schedule
	RunCount       int       `json:"run_count" db:"run_count"`             // Number of times series has been completed (for restart tracking)
	Disabled       bool      `json:"disabled" db:"disabled"`               // True if series has been disabled due to completion
}
