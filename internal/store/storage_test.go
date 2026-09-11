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

func TestStorageCounts_EmptyDatabase(t *testing.T) {
	t.Parallel()
	counts, err := newTestStore(t).StorageCounts(context.Background())
	require.NoError(t, err)

	assert.Zero(t, counts.Airings)
	assert.Zero(t, counts.SeriesStates)
	assert.Nil(t, counts.OldestAiring, "no bounds to offer a range form")
	assert.Nil(t, counts.NewestAiring)
}

func TestStorageCounts_ReportsRowsAndBounds(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()

	first := time.Date(2026, 5, 1, 20, 0, 0, 0, time.UTC)
	last := time.Date(2026, 5, 9, 21, 30, 0, 0, time.UTC)
	require.NoError(t, st.RecordScheduleHistory(ctx, []scheduler.ScheduleHistoryEntry{
		airing("a", "Show", "Night", first, first),
		airing("b", "Show", "Night", last, last),
	}))
	require.NoError(t, st.UpdateSeriesState(ctx, &scheduler.SeriesState{
		ShowTitle: "Show", CurrentSeason: 1, CurrentEpisode: 2,
	}))

	counts, err := st.StorageCounts(ctx)
	require.NoError(t, err)

	assert.Equal(t, int64(2), counts.Airings)
	assert.Equal(t, int64(1), counts.SeriesStates)
	require.NotNil(t, counts.OldestAiring)
	require.NotNil(t, counts.NewestAiring)
	assert.Equal(t, first.Unix(), counts.OldestAiring.Unix())
	assert.Equal(t, last.Unix(), counts.NewestAiring.Unix())
}

// A sentinel is not an airing. Folding the two together would report rows
// the operator cannot delete as if they were rows they could, and the
// airing count would never reach zero.
func TestStorageCounts_CountsSentinelsApartFromAirings(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()

	at := time.Date(2026, 5, 1, 20, 0, 0, 0, time.UTC)
	require.NoError(t, st.RecordScheduleHistory(ctx, []scheduler.ScheduleHistoryEntry{
		airing("a", "Show", "Night", at, at),
	}))
	_, err := st.DeleteHistoryRange(ctx, store.HistoryRange{From: at.Add(-time.Hour), To: at.Add(time.Hour)})
	require.NoError(t, err)

	counts, err := st.StorageCounts(ctx)
	require.NoError(t, err)
	assert.Zero(t, counts.Airings, "the airing is gone")
	assert.Equal(t, int64(1), counts.Sentinels, "its occurrence stays committed and empty")
}
