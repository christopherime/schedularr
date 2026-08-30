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
	// OperatorUpdatedAt is when an operator last wrote this row directly
	// (PATCH /state/series, or the CLI's `state set`/`state reset`/
	// `state import`) -- nil if never. Engine.syncPostStates skips any
	// aired-occurrence post-state replay whose own commit predates this
	// stamp, so an operator write -- a BACKWARD cursor jump included --
	// is never re-advanced by an occurrence planned before it.
	// Engine-side writes (Commit's UpdateSeriesState calls) carry the
	// stamp through unchanged; only the operator entry points ever set
	// it.
	OperatorUpdatedAt *time.Time `json:"operator_updated_at,omitempty" db:"operator_updated_at"`
	// CursorPlanSeq is the provenance of the current cursor value: the
	// plan sequence (Engine.nextPlanSeq) of the plan whose post-state
	// last wrote this row through Engine.syncPostStates. An
	// aired-occurrence replay wins exactly when its own
	// OccurrenceSnapshot.PlanSeq is newer than this -- in either
	// direction, so an on_complete:restart wrap (S01E05 -> S01E01) lands
	// instead of being dropped as "backward" -- while a stale replay from
	// an OLDER plan (e.g. a slower block sharing the show) is rejected.
	// Zero means no post-state replay (or plan-time real-plan write) has
	// stamped this row yet. Operator writes leave it untouched: they are
	// protected by OperatorUpdatedAt, not by provenance.
	CursorPlanSeq int64 `json:"cursor_plan_seq,omitempty" db:"cursor_plan_seq"`
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
// statesFromSnapshot), which made initializeSeriesState re-apply
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

// OccurrenceSnapshot is everything persisted per series-block occurrence
// in series_occurrence_snapshots: the per-show cursor the occurrence
// plans FROM (PreStates -- the seed, captured at first plan and
// rewritten only when an earlier occurrence's re-derivation shifts this
// occurrence's baseline), the per-show cursor it ends AT (PostStates,
// captured at that same plan time), and when that plan was last written
// (RecordedAt -- the occurrence's commit stamp, refreshed on every
// upsert; an aired occurrence's row is never written again, so it
// freezes at the plan the occurrence actually aired with).
//
// PostStates is what advances the persisted series_state cursor once the
// occurrence airs: Engine.planSeriesOccurrences' aired branch replays it
// (see Engine.syncPostStates for the guards) instead of re-deriving the
// advance from committed content metadata or a re-plan -- an
// occurrence's effect on the global cursor is decided exactly once, at
// plan time, when the full planning context (on_complete restarts,
// skips, season rollovers) is still in hand. A nil PostStates marks a
// legacy row written before migration 000006 added the column: its
// occurrence still replays committed content verbatim if aired (and
// re-derives if future) but contributes no cursor advance of its own.
type OccurrenceSnapshot struct {
	PreStates  map[string]SeriesStateSnapshot
	PostStates map[string]SeriesStateSnapshot
	RecordedAt time.Time
	// PlanSeq is the engine-allocated, strictly monotonic sequence
	// (Engine.nextPlanSeq) of the plan generation that produced
	// PostStates -- the replay-ordering provenance
	// Engine.syncPostStates compares against SeriesState.CursorPlanSeq.
	// Re-deriving a not-yet-aired occurrence allocates a fresh, higher
	// sequence; an aired occurrence's row is never rewritten, freezing
	// the sequence of the plan it actually aired with. Zero marks a
	// legacy row written before migration 000007 (no cursor advance of
	// its own -- see the migration's doc comment).
	PlanSeq int64
}
