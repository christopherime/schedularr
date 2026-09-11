package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/christopherime/schedularr/internal/scheduler"
)

// SQLite keeps a DATETIME as TEXT carrying whatever offset the writer had,
// and compares it BYTEWISE. Two rows that are the SAME INSTANT, written in
// different zones, must still both fall on the same side of a cutoff --
// otherwise retention prunes by text and a range cleanup deletes the wrong
// rows.
func TestScheduleHistory_RangeIsIndependentOfWriterOffset(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()

	zurich, err := time.LoadLocation("Europe/Zurich")
	if err != nil {
		t.Skipf("tzdata unavailable: %v", err)
	}

	// One instant, two representations, an hour before the cutoff.
	instant := time.Date(2026, 6, 1, 8, 0, 0, 0, time.UTC)
	entries := []scheduler.ScheduleHistoryEntry{
		historyEntry("p-utc", "A", "Show", instant),
		historyEntry("p-zur", "B", "Show", instant.In(zurich)),
	}
	require.NoError(t, st.RecordScheduleHistory(ctx, entries))

	// A window that puts the cutoff an hour AFTER both rows: both are old
	// enough to prune, whichever zone wrote them.
	window := time.Since(instant.Add(time.Hour))
	n, err := st.CleanupScheduleHistory(ctx, window)
	require.NoError(t, err)
	assert.Equal(t, int64(2), n, "pruned by text rather than by instant")
}

// The exact-match column. GetCommittedOccurrence and
// ReplaceOccurrenceHistory both look a row up by (block, occurrence_start),
// so a value written in one representation and searched for in another
// silently finds nothing -- which reads as missing data, not as a wrong
// answer.
func TestOccurrenceLookup_IsIndependentOfWriterOffset(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()

	zurich, err := time.LoadLocation("Europe/Zurich")
	if err != nil {
		t.Skipf("tzdata unavailable: %v", err)
	}

	occurrence := time.Date(2026, 6, 1, 21, 0, 0, 0, time.UTC)
	require.NoError(t, st.RecordScheduleHistory(ctx, []scheduler.ScheduleHistoryEntry{
		historyEntry("p1", "Ep", "Show", occurrence),
	}))

	// Same instant, asked for in the other zone.
	programs, ok, err := st.GetCommittedOccurrence(ctx, "Anime Night", occurrence.In(zurich))
	require.NoError(t, err)
	assert.True(t, ok, "the occurrence was not found when asked for in another zone")
	assert.Len(t, programs, 1)
}

// The sibling above pins that two representations of ONE instant land on
// the same side of a cutoff. This pins the case that actually broke: two
// DIFFERENT instants, written in zones far enough apart that the stored
// TEXT sorts backwards against the real order. A same-zone fixture can
// never reach it, which is why the ranges looked correct until range
// cleanup was about to hand an operator an arbitrary cutoff.
func TestScheduleHistory_RangeSplitsByInstantNotStoredText(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()

	plus14 := time.FixedZone("UTC+14", 14*3600)
	minus11 := time.FixedZone("UTC-11", -11*3600)

	base := time.Date(2026, 3, 14, 23, 30, 0, 0, time.UTC)
	early := base                   // stored as 2026-03-15T13:30+14:00
	late := base.Add(1 * time.Hour) // stored as 2026-03-14T13:30-11:00

	require.NoError(t, st.RecordScheduleHistory(ctx, []scheduler.ScheduleHistoryEntry{
		historyEntry("early", "Early", "Early", early.In(plus14)),
		historyEntry("late", "Late", "Late", late.In(minus11)),
	}))

	entries, err := st.ListScheduleHistory(ctx, base.Add(30*time.Minute))
	require.NoError(t, err)
	require.Len(t, entries, 1, "the cutoff splits by instant, not by stored text")
	assert.Equal(t, "late", entries[0].ProgramID)
}
