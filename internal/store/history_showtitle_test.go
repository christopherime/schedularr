package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/christopherime/schedularr/internal/scheduler"
)

func historyEntry(programID, title, showTitle string, at time.Time) scheduler.ScheduleHistoryEntry {
	return scheduler.ScheduleHistoryEntry{
		ProgramID:       programID,
		ChannelID:       "ch1",
		BlockName:       "Anime Night",
		ScheduledAt:     at,
		OccurrenceStart: at,
		Title:           title,
		ShowTitle:       showTitle,
		Type:            "episode",
	}
}

// The `title` column has always held the EPISODE title, so nothing in this
// schema identified a SHOW's airings -- which is why "remove everything
// from Bloody Mary" had no predicate to run against.
func TestScheduleHistory_CarriesTheShowTitle(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	at := time.Now().UTC().Truncate(time.Second)

	require.NoError(t, st.RecordScheduleHistory(ctx,
		[]scheduler.ScheduleHistoryEntry{historyEntry("p1", "The Empty Room", "Bloody Mary", at)}))

	rows, err := st.ListScheduleHistory(ctx, at.Add(-time.Hour))
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "Bloody Mary", rows[0].ShowTitle, "show_title did not round-trip")
	assert.Equal(t, "The Empty Room", rows[0].Title, "show_title overwrote the episode title")
}

// A movie belongs to no show, so empty is the honest value -- and a
// removal by title must never sweep an empty one, or "remove Bloody Mary"
// would take every movie ever aired with it.
func TestScheduleHistory_AShowlessProgramStoresAnEmptyShowTitle(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	at := time.Now().UTC().Truncate(time.Second)

	require.NoError(t, st.RecordScheduleHistory(ctx,
		[]scheduler.ScheduleHistoryEntry{historyEntry("p1", "Solaris", "", at)}))

	rows, err := st.ListScheduleHistory(ctx, at.Add(-time.Hour))
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Empty(t, rows[0].ShowTitle)
}

// ReplaceOccurrenceHistory is the second write path into this table (the
// idempotent-apply rewrite). A column threaded through one INSERT and not
// the other is the failure this pins: the field would work from a fresh
// apply and silently blank on a re-apply of the same occurrence.
func TestScheduleHistory_ReplaceKeepsTheShowTitle(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	at := time.Now().UTC().Truncate(time.Second)

	require.NoError(t, st.RecordScheduleHistory(ctx,
		[]scheduler.ScheduleHistoryEntry{historyEntry("p1", "The Empty Room", "Bloody Mary", at)}))

	require.NoError(t, st.ReplaceOccurrenceHistory(ctx, "Anime Night", at,
		[]scheduler.ScheduleHistoryEntry{historyEntry("p2", "Red Rain", "Bloody Mary", at)}))

	rows, err := st.ListScheduleHistory(ctx, at.Add(-time.Hour))
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "Bloody Mary", rows[0].ShowTitle, "the replace path dropped show_title")
	assert.Equal(t, "Red Rain", rows[0].Title)
}
