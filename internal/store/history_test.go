package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/christopherime/schedularr/internal/scheduler"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListScheduleHistory_FiltersAndOrdersDesc(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	since := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)

	entries := []scheduler.ScheduleHistoryEntry{
		// Before the cutoff -- must be excluded.
		{ProgramID: "old-1", ChannelID: "ch1", BlockName: "block-a", ScheduledAt: since.Add(-2 * 24 * time.Hour)},
		{ProgramID: "old-2", ChannelID: "ch1", BlockName: "block-a", ScheduledAt: since.Add(-1 * time.Hour)},
		// At or after the cutoff -- must be included.
		{ProgramID: "new-1", ChannelID: "ch1", BlockName: "block-b", ScheduledAt: since},
		{ProgramID: "new-2", ChannelID: "ch2", BlockName: "block-b", ScheduledAt: since.Add(2 * time.Hour)},
	}
	require.NoError(t, s.RecordScheduleHistory(ctx, entries))

	got, err := s.ListScheduleHistory(ctx, since)
	require.NoError(t, err)
	require.Len(t, got, 2, "only entries at/after the cutoff should be returned")

	// DESC order: most recently scheduled first.
	assert.Equal(t, "new-2", got[0].ProgramID)
	assert.Equal(t, "ch2", got[0].ChannelID)
	assert.Equal(t, "new-1", got[1].ProgramID)
	assert.Equal(t, "ch1", got[1].ChannelID)
}

func TestListScheduleHistory_Empty(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	got, err := s.ListScheduleHistory(ctx, time.Now().Add(-7*24*time.Hour))
	require.NoError(t, err)
	assert.Empty(t, got)
}

// The same program may legitimately land on one channel more than once in
// a single apply: once per occurrence of a multi-day window, and even
// twice within one long occurrence when the library is small. All three
// rows share the same planning wall-clock ScheduledAt -- the pre-v0.5.13
// PRIMARY KEY rejected exactly this.
func TestRecordScheduleHistory_SameProgramAcrossOccurrences(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	planned := time.Date(2026, 9, 12, 9, 0, 0, 0, time.UTC)
	day1 := time.Date(2026, 9, 12, 22, 0, 0, 0, time.UTC)
	day2 := day1.Add(24 * time.Hour)

	entries := []scheduler.ScheduleHistoryEntry{
		{ProgramID: "movie-1", ChannelID: "ch1", BlockName: "horror", ScheduledAt: planned, OccurrenceStart: day1, Sequence: 0},
		{ProgramID: "movie-1", ChannelID: "ch1", BlockName: "horror", ScheduledAt: planned, OccurrenceStart: day1, Sequence: 1},
		{ProgramID: "movie-1", ChannelID: "ch1", BlockName: "horror", ScheduledAt: planned, OccurrenceStart: day2, Sequence: 0},
	}
	require.NoError(t, s.RecordScheduleHistory(ctx, entries))

	got, err := s.ListScheduleHistory(ctx, planned.Add(-time.Hour))
	require.NoError(t, err)
	assert.Len(t, got, 3)
}

// A genuine double-commit of one occurrence row is still an integrity
// error: the rebuilt identity is (block_name, occurrence_start, sequence).
func TestRecordScheduleHistory_RejectsDuplicateOccurrenceRow(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	occ := time.Date(2026, 9, 12, 22, 0, 0, 0, time.UTC)
	row := scheduler.ScheduleHistoryEntry{
		ProgramID: "movie-1", ChannelID: "ch1", BlockName: "horror",
		ScheduledAt: occ, OccurrenceStart: occ, Sequence: 0,
	}
	require.NoError(t, s.RecordScheduleHistory(ctx, []scheduler.ScheduleHistoryEntry{row}))
	require.Error(t, s.RecordScheduleHistory(ctx, []scheduler.ScheduleHistoryEntry{row}))
}
