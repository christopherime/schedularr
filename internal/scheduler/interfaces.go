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
	// the first time a series block's occurrence (blockID,
	// occurrenceStart) was ever planned -- see SeriesStateSnapshot's and
	// Engine.planSeriesOccurrences' doc comments. ok is false when this
	// occurrence has never been planned before. Keyed by blockID (the
	// block's stable store UUID, scheduler.Block.ID) rather than the
	// block's name, which is renameable -- a rename must not orphan a
	// not-yet-aired occurrence's stored cursor snapshot.
	GetOccurrenceSnapshot(ctx context.Context, blockID string, occurrenceStart time.Time) (snapshot map[string]SeriesStateSnapshot, ok bool, err error)
	// SaveOccurrenceSnapshot persists a series block occurrence's cursor
	// snapshot. It is an upsert: the first time an occurrence is planned
	// this creates its row, and re-deriving a not-yet-aired occurrence
	// (see planSeriesOccurrences' chain mechanism) overwrites it with the
	// occurrence's new baseline -- carried forward from whatever the
	// block's PRECEDING occurrence actually ended up at, not necessarily
	// what was stored before. An aired occurrence's snapshot, once
	// written, is never touched again (aired occurrences aren't
	// re-derived at all).
	SaveOccurrenceSnapshot(ctx context.Context, blockID string, occurrenceStart time.Time, snapshot map[string]SeriesStateSnapshot) error
	// CleanupOccurrenceSnapshots deletes snapshot rows for occurrences that
	// started more than window before now -- mirrors
	// CleanupScheduleHistory's retention window, since a snapshot for an
	// occurrence outside the history retention window can never be
	// re-derived (its schedule_history rows, if any, are gone too) and so
	// serves no further purpose. Returns the number of rows deleted.
	CleanupOccurrenceSnapshots(ctx context.Context, window time.Duration) (int64, error)
	// DeleteFutureOccurrenceSnapshots deletes every occurrence snapshot for
	// blockID with occurrence_start > now. Used whenever something that
	// invalidates a not-yet-aired occurrence's cached baseline happens
	// outside the normal apply flow -- an operator PATCHing series_state
	// (api.PatchSeriesState), or a block being edited or deleted
	// (api.UpdateBlock / api.DeleteBlock) -- so the next apply treats
	// those occurrences as unseen again and re-derives them from current
	// state/spec rather than silently keeping a now-stale snapshot.
	DeleteFutureOccurrenceSnapshots(ctx context.Context, blockID string, now time.Time) error
	// ReplaceOccurrenceHistory atomically replaces every schedule_history
	// row for one occurrence (blockName, occurrenceStart) with entries --
	// used when a not-yet-aired series occurrence is re-derived against a
	// possibly-changed block spec, so its stored assignment (and any
	// re-derivation after it, via GetCommittedOccurrence once it airs)
	// reflects the latest re-derivation rather than accumulating stale
	// rows from every prior one. Still keyed by blockName (unlike the
	// occurrence snapshot table): schedule_history is out of scope for
	// the blockID rekey, see store/sqlite.go's schema doc comment.
	ReplaceOccurrenceHistory(ctx context.Context, blockName string, occurrenceStart time.Time, entries []ScheduleHistoryEntry) error
}
