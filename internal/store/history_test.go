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
