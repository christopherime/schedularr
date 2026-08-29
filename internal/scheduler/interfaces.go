package scheduler

import (
	"context"
	"time"

	"github.com/christopherime/schedularr/internal/external/tunarr"
)

// StateStore defines the interface for persisting series state.
type StateStore interface {
	GetSeriesState(ctx context.Context, showTitle string) (*SeriesState, error)
	UpdateSeriesState(ctx context.Context, state *SeriesState) error
	RecordScheduleHistory(ctx context.Context, entries []ScheduleHistoryEntry) error
	WasRecentlyScheduled(ctx context.Context, programID, channelID string, window time.Duration) (bool, error)
	CleanupScheduleHistory(ctx context.Context, window time.Duration) (int64, error)
	// GetCommittedOccurrence returns the program assignment previously
	// committed for one occurrence of blockName starting at
	// occurrenceStart, if any -- see Engine.PlanBlock's doc comment for
	// why this is the mechanism behind idempotent apply for filter blocks
	// (and for an aired series block occurrence). ok is false when no such
	// occurrence has ever been committed.
	GetCommittedOccurrence(ctx context.Context, blockName string, occurrenceStart time.Time) (programs []tunarr.Program, ok bool, err error)
	// GetOccurrenceSnapshot returns the per-show cursor snapshot captured
	// the first time a series block's occurrence (blockName,
	// occurrenceStart) was ever planned -- see SeriesStateSnapshot's and
	// Engine.PlanBlock's doc comments. ok is false when this occurrence has
	// never been planned before.
	GetOccurrenceSnapshot(ctx context.Context, blockName string, occurrenceStart time.Time) (snapshot map[string]SeriesStateSnapshot, ok bool, err error)
	// SaveOccurrenceSnapshot persists a series block occurrence's cursor
	// snapshot the first (and only) time it's planned -- it is never
	// overwritten afterward, which is what makes an already-seen
	// occurrence's re-derivation deterministic given an unchanged spec.
	SaveOccurrenceSnapshot(ctx context.Context, blockName string, occurrenceStart time.Time, snapshot map[string]SeriesStateSnapshot) error
	// ReplaceOccurrenceHistory atomically replaces every schedule_history
	// row for one occurrence (blockName, occurrenceStart) with entries --
	// used when a not-yet-aired series occurrence is re-derived against a
	// possibly-changed block spec, so its stored assignment (and any
	// re-derivation after it, via GetCommittedOccurrence once it airs)
	// reflects the latest re-derivation rather than accumulating stale
	// rows from every prior one.
	ReplaceOccurrenceHistory(ctx context.Context, blockName string, occurrenceStart time.Time, entries []ScheduleHistoryEntry) error
}
