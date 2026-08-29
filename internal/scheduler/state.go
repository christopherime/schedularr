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
// SeriesState -- no ShowTitle (implied by its map key) -- except Seeded,
// which stands in for LastAired.
//
// Seeded records whether the captured *SeriesState already had a non-nil
// LastAired (i.e. was already past its one-time start_season/start_episode
// initialization) at the moment of capture -- see
// Engine.initializeSeriesState: it only applies start_season/start_episode
// when LastAired is nil, treating that as "never initialized." A
// reconstructed snapshot used to always leave LastAired nil (see
// newSnapshotSeriesContext), which made initializeSeriesState re-apply
// start_season/start_episode on *every* re-derive of a not-yet-aired
// occurrence, silently re-pinning its cursor back to the configured start
// position regardless of how far the snapshot had actually progressed
// (e.g. start_episode: 5 configured, occurrence legitimately at E6 --
// every re-apply re-derived it back to E5). Seeded fixes that: a
// reconstructed SeriesState gets a non-nil marker LastAired exactly when
// the snapshot says the underlying cursor was already initialized, so
// initializeSeriesState correctly treats it as "already past the start
// position," same as the live cursor it was captured from.
type SeriesStateSnapshot struct {
	CurrentSeason  int  `json:"current_season"`
	CurrentEpisode int  `json:"current_episode"`
	Completed      bool `json:"completed"`
	Disabled       bool `json:"disabled"`
	RunCount       int  `json:"run_count"`
	Seeded         bool `json:"seeded"`
}
