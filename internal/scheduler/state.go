package scheduler

import (
	"time"
)

// SeriesState tracks the playback progress of a specific series.
type SeriesState struct {
	ShowTitle      string     `json:"show_title" db:"show_title"`           // Primary key (or part of it)
	CurrentSeason  int        `json:"current_season" db:"current_season"`   // Next season to air
	CurrentEpisode int        `json:"current_episode" db:"current_episode"` // Next episode to air
	Completed      bool       `json:"completed" db:"completed"`             // True if all available episodes have aired
	LastAired      *time.Time `json:"last_aired" db:"last_aired"`           // Timestamp of last successful schedule (nullable)
	RunCount       int        `json:"run_count" db:"run_count"`             // Number of times series has been completed (for restart tracking)
	Disabled       bool       `json:"disabled" db:"disabled"`               // True if series has been disabled due to completion
}

// SeriesStateSnapshot is the per-show cursor captured immutably the FIRST
// time a series block's occurrence is ever planned, keyed by show title
// within a per-occurrence map (one snapshot entry per show the block's
// spec referenced at that moment) -- see Engine.PlanBlock's doc comment
// for the full mechanism this supports: idempotent re-apply of a
// not-yet-aired occurrence that still lets a block spec edited before it
// airs (series reordered, added, removed, episodes_per_block or duration
// changed) change that occurrence's content. It's a value-only subset of
// SeriesState -- no ShowTitle (implied by its map key) and no LastAired
// (irrelevant to a frozen planning input).
type SeriesStateSnapshot struct {
	CurrentSeason  int  `json:"current_season"`
	CurrentEpisode int  `json:"current_episode"`
	Completed      bool `json:"completed"`
	Disabled       bool `json:"disabled"`
	RunCount       int  `json:"run_count"`
}
