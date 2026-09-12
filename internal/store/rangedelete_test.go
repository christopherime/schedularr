package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/christopherime/schedularr/internal/scheduler"
	"github.com/christopherime/schedularr/internal/store"
)

// airing builds one history row in a named block/occurrence, so a test can
// say which occurrence a row belongs to rather than relying on the shared
// historyEntry helper's fixed block.
func airing(programID, showTitle, block string, occurrence, at time.Time) scheduler.ScheduleHistoryEntry {
	// Same derivation as historyEntry's: distinct programs sharing a block
	// and occurrence need distinct sequences, as production writes them.
	seq := 0
	for _, b := range []byte(programID) {
		seq = seq*31 + int(b)
	}
	return scheduler.ScheduleHistoryEntry{
		ProgramID:       programID,
		ChannelID:       "ch1",
		BlockName:       block,
		ScheduledAt:     at,
		OccurrenceStart: occurrence,
		Sequence:        seq % 1000,
		Title:           programID,
		ShowTitle:       showTitle,
		Type:            "episode",
	}
}

func TestDeleteHistoryRange_DeletesOnlyInsideTheWindow(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()

	base := time.Date(2026, 5, 1, 20, 0, 0, 0, time.UTC)
	require.NoError(t, st.RecordScheduleHistory(ctx, []scheduler.ScheduleHistoryEntry{
		airing("old", "A", "Night", base.Add(-48*time.Hour), base.Add(-48*time.Hour)),
		airing("inside", "A", "Night", base, base),
		airing("new", "A", "Night", base.Add(48*time.Hour), base.Add(48*time.Hour)),
	}))

	report, err := st.DeleteHistoryRange(ctx, store.HistoryRange{
		From: base.Add(-time.Hour), To: base.Add(time.Hour),
	})
	require.NoError(t, err)
	assert.Equal(t, int64(1), report.Airings)

	left, err := st.ListScheduleHistory(ctx, time.Time{})
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"old", "new"}, programIDs(left), "only the row inside the window goes")
	assert.Equal(t, 1, sentinelCount(left), "its occurrence lost its only airing, so it stays committed and empty")
}

func TestDeleteHistoryRange_OpenEndedWindows(t *testing.T) {
	t.Parallel()
	base := time.Date(2026, 5, 1, 20, 0, 0, 0, time.UTC)

	t.Run("everything before to", func(t *testing.T) {
		t.Parallel()
		st := newTestStore(t)
		ctx := context.Background()
		require.NoError(t, st.RecordScheduleHistory(ctx, []scheduler.ScheduleHistoryEntry{
			airing("old", "A", "Night", base.Add(-48*time.Hour), base.Add(-48*time.Hour)),
			airing("new", "A", "Night", base.Add(48*time.Hour), base.Add(48*time.Hour)),
		}))

		report, err := st.DeleteHistoryRange(ctx, store.HistoryRange{To: base})
		require.NoError(t, err)
		assert.Equal(t, int64(1), report.Airings)

		left, err := st.ListScheduleHistory(ctx, time.Time{})
		require.NoError(t, err)
		assert.Equal(t, []string{"new"}, programIDs(left))
		assert.Equal(t, 1, sentinelCount(left))
	})

	t.Run("everything from onward", func(t *testing.T) {
		t.Parallel()
		st := newTestStore(t)
		ctx := context.Background()
		require.NoError(t, st.RecordScheduleHistory(ctx, []scheduler.ScheduleHistoryEntry{
			airing("old", "A", "Night", base.Add(-48*time.Hour), base.Add(-48*time.Hour)),
			airing("new", "A", "Night", base.Add(48*time.Hour), base.Add(48*time.Hour)),
		}))

		report, err := st.DeleteHistoryRange(ctx, store.HistoryRange{From: base})
		require.NoError(t, err)
		assert.Equal(t, int64(1), report.Airings)

		left, err := st.ListScheduleHistory(ctx, time.Time{})
		require.NoError(t, err)
		assert.Equal(t, []string{"old"}, programIDs(left))
		assert.Equal(t, 1, sentinelCount(left))
	})
}

func TestDeleteHistoryRange_NarrowsByChannelBlockAndShow(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()

	at := time.Date(2026, 5, 1, 20, 0, 0, 0, time.UTC)
	rows := []scheduler.ScheduleHistoryEntry{
		airing("keep-block", "A", "Other", at, at),
		airing("kill", "A", "Night", at, at),
		airing("keep-show", "B", "Night", at, at),
	}
	require.NoError(t, st.RecordScheduleHistory(ctx, rows))

	report, err := st.DeleteHistoryRange(ctx, store.HistoryRange{
		From: at.Add(-time.Hour), To: at.Add(time.Hour),
		BlockName: "Night", ShowTitle: "A",
	})
	require.NoError(t, err)
	assert.Equal(t, int64(1), report.Airings)

	left, err := st.ListScheduleHistory(ctx, time.Time{})
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"keep-block", "keep-show"}, programIDs(left))
}

// An occurrence that loses every airing must stay COMMITTED and empty.
// Without the sentinel GetCommittedOccurrence answers "never planned" and
// the next apply re-plans a slot that has already gone out -- rewriting
// history instead of removing from it.
func TestDeleteHistoryRange_EmptiedOccurrenceKeepsItsSentinel(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()

	occurrence := time.Date(2026, 5, 1, 20, 0, 0, 0, time.UTC)
	require.NoError(t, st.RecordScheduleHistory(ctx, []scheduler.ScheduleHistoryEntry{
		airing("one", "A", "Night", occurrence, occurrence),
		airing("two", "A", "Night", occurrence, occurrence.Add(time.Minute)),
	}))

	_, ok, err := st.GetCommittedOccurrence(ctx, "Night", occurrence)
	require.NoError(t, err)
	require.True(t, ok, "committed before the delete")

	report, err := st.DeleteHistoryRange(ctx, store.HistoryRange{
		From: occurrence.Add(-time.Hour), To: occurrence.Add(time.Hour),
	})
	require.NoError(t, err)
	assert.Equal(t, int64(2), report.Airings)
	assert.Equal(t, int64(1), report.EmptiedSlots)

	programs, ok, err := st.GetCommittedOccurrence(ctx, "Night", occurrence)
	require.NoError(t, err)
	assert.True(t, ok, "the occurrence stays committed, so the next apply will not re-plan it")
	assert.Empty(t, programs, "and it produced nothing, which is what actually happened")
}

// A second pass over the same window must be a no-op: the sentinel the
// first pass left is not an airing, so it is neither deleted nor counted,
// and it does not collect a second sentinel.
func TestDeleteHistoryRange_IsIdempotent(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()

	occurrence := time.Date(2026, 5, 1, 20, 0, 0, 0, time.UTC)
	require.NoError(t, st.RecordScheduleHistory(ctx, []scheduler.ScheduleHistoryEntry{
		airing("one", "A", "Night", occurrence, occurrence),
	}))

	window := store.HistoryRange{From: occurrence.Add(-time.Hour), To: occurrence.Add(time.Hour)}

	first, err := st.DeleteHistoryRange(ctx, window)
	require.NoError(t, err)
	assert.Equal(t, int64(1), first.Airings)
	assert.Equal(t, int64(1), first.EmptiedSlots)

	second, err := st.DeleteHistoryRange(ctx, window)
	require.NoError(t, err)
	assert.Zero(t, second.Airings, "the sentinel is not an airing")
	assert.Zero(t, second.EmptiedSlots, "and does not collect another sentinel")

	left, err := st.ListScheduleHistory(ctx, time.Time{})
	require.NoError(t, err)
	assert.Empty(t, programIDs(left), "no airings survive")
	assert.Equal(t, 1, sentinelCount(left), "exactly one sentinel survives both passes")
}

func TestDeleteHistoryRange_RefusesAnUnboundedWindow(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)

	_, err := st.DeleteHistoryRange(context.Background(), store.HistoryRange{})
	require.ErrorIs(t, err, store.ErrUnboundedRange, "no window is not 'delete everything'")
}

func TestDeleteHistoryRange_RefusesABackwardWindow(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)

	at := time.Date(2026, 5, 1, 20, 0, 0, 0, time.UTC)
	_, err := st.DeleteHistoryRange(context.Background(), store.HistoryRange{
		From: at.Add(time.Hour), To: at,
	})
	require.ErrorIs(t, err, store.ErrBackwardRange)
}

func TestCountHistoryRange_CountsWhatTheDeleteWouldRemove(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()

	occurrence := time.Date(2026, 5, 1, 20, 0, 0, 0, time.UTC)
	require.NoError(t, st.RecordScheduleHistory(ctx, []scheduler.ScheduleHistoryEntry{
		airing("one", "A", "Night", occurrence, occurrence),
		airing("two", "A", "Night", occurrence, occurrence.Add(time.Minute)),
		airing("far", "A", "Night", occurrence.Add(72*time.Hour), occurrence.Add(72*time.Hour)),
	}))

	window := store.HistoryRange{From: occurrence.Add(-time.Hour), To: occurrence.Add(time.Hour)}

	preview, err := st.CountHistoryRange(ctx, window)
	require.NoError(t, err)

	left, err := st.ListScheduleHistory(ctx, time.Time{})
	require.NoError(t, err)
	require.Len(t, left, 3, "a dry run changes nothing")

	actual, err := st.DeleteHistoryRange(ctx, window)
	require.NoError(t, err)

	// The operator confirms against the preview, so it must be the same
	// number the delete reports.
	assert.Equal(t, preview.Airings, actual.Airings)
	assert.Equal(t, preview.EmptiedSlots, actual.EmptiedSlots)
}

// The operator's cutoff is arbitrary, and stored instants carry their
// writer's offset -- so a range must split by instant, not by text.
func TestDeleteHistoryRange_IsInstantIndependent(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()

	plus14 := time.FixedZone("UTC+14", 14*3600)
	minus11 := time.FixedZone("UTC-11", -11*3600)

	base := time.Date(2026, 3, 14, 23, 30, 0, 0, time.UTC)
	early := base
	late := base.Add(time.Hour)

	require.NoError(t, st.RecordScheduleHistory(ctx, []scheduler.ScheduleHistoryEntry{
		airing("early", "A", "Night", early.In(plus14), early.In(plus14)),
		airing("late", "A", "Night", late.In(minus11), late.In(minus11)),
	}))

	report, err := st.DeleteHistoryRange(ctx, store.HistoryRange{To: base.Add(30 * time.Minute)})
	require.NoError(t, err)
	assert.Equal(t, int64(1), report.Airings)

	left, err := st.ListScheduleHistory(ctx, time.Time{})
	require.NoError(t, err)
	assert.Equal(t, []string{"late"}, programIDs(left), "the cutoff split by instant")
	assert.Equal(t, 1, sentinelCount(left))
}

// programIDs lists the real airings left, skipping sentinels. A sentinel
// is not an airing -- it is the marker saying an occurrence was committed
// and produced nothing -- so a test about WHICH ROWS a window selects
// should not have to spell them out. sentinelCount asserts on them
// directly where they are the point.
func programIDs(entries []scheduler.ScheduleHistoryEntry) []string {
	ids := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.ProgramID == "" {
			continue
		}
		ids = append(ids, e.ProgramID)
	}
	return ids
}

func sentinelCount(entries []scheduler.ScheduleHistoryEntry) int {
	n := 0
	for _, e := range entries {
		if e.ProgramID == "" {
			n++
		}
	}
	return n
}
