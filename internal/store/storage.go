package store

import (
	"context"
	"fmt"
	"time"
)

// StorageCounts is what is ACTUALLY stored, per table, right now.
//
// It reports rows, never policy. Retention says what should age out; a
// database whose retention was widened, or that has been running since
// before a knob existed, holds whatever it holds. Reporting the
// configured window here would be reporting an intention as a
// measurement -- the same failure the guide's draft mode refuses when it
// words every verdict "vs the reading taken at HH:MM".
//
// OldestAiring and NewestAiring are nil on an empty table. They cost one
// MIN/MAX in the same pass and they are what makes the range-cleanup form
// pickable instead of a guess.
type StorageCounts struct {
	SeriesStates int64
	Airings      int64
	Sentinels    int64
	Snapshots    int64
	ApplyRuns    int64
	Warnings     int64

	OldestAiring *time.Time
	NewestAiring *time.Time
}

// StorageCounts counts every table that grows with time.
func (s *Store) StorageCounts(ctx context.Context) (StorageCounts, error) {
	var counts StorageCounts

	// Sentinels are counted apart from airings. A strip that folded them
	// together would report rows the operator cannot delete as if they
	// were rows they could, and the number would never reach zero.
	for _, q := range []struct {
		dest  *int64
		query string
	}{
		{&counts.SeriesStates, `SELECT COUNT(*) FROM series_state`},
		{&counts.Airings, `SELECT COUNT(*) FROM schedule_history WHERE program_id <> ''`},
		{&counts.Sentinels, `SELECT COUNT(*) FROM schedule_history WHERE program_id = ''`},
		{&counts.Snapshots, `SELECT COUNT(*) FROM series_occurrence_snapshots`},
		{&counts.ApplyRuns, `SELECT COUNT(*) FROM apply_runs`},
		{&counts.Warnings, `SELECT COUNT(*) FROM apply_run_warnings`},
	} {
		if err := s.db.GetContext(ctx, q.dest, q.query); err != nil {
			return counts, fmt.Errorf("failed to count stored rows: %w", err)
		}
	}

	// Read as strings and parsed here: MIN/MAX over a DATETIME loses the
	// driver's type affinity and comes back bare, so scanning straight
	// into a *time.Time fails at run time -- the same trap emptiedSlot's
	// doc comment records.
	var bounds struct {
		Oldest *string `db:"oldest"`
		Newest *string `db:"newest"`
	}
	if err := s.db.GetContext(ctx, &bounds, `
		SELECT MIN(datetime(scheduled_at)) AS oldest, MAX(datetime(scheduled_at)) AS newest
		FROM schedule_history WHERE program_id <> ''`); err != nil {
		return counts, fmt.Errorf("failed to read the stored airing bounds: %w", err)
	}
	counts.OldestAiring = parseStoredInstant(bounds.Oldest)
	counts.NewestAiring = parseStoredInstant(bounds.Newest)

	return counts, nil
}

// parseStoredInstant reads what SQLite's datetime() renders -- UTC,
// second precision, no offset. A value that will not parse is dropped
// rather than guessed at: the bounds only prefill a form, and a wrong
// instant there would seed a destructive range.
func parseStoredInstant(raw *string) *time.Time {
	if raw == nil || *raw == "" {
		return nil
	}
	parsed, err := time.Parse("2006-01-02 15:04:05", *raw)
	if err != nil {
		return nil
	}
	return &parsed
}
