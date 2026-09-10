package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/christopherime/schedularr/internal/scheduler"
	"github.com/christopherime/schedularr/internal/store"
)

func seedAiring(t *testing.T, st *store.Store, programID, show string, at time.Time) {
	t.Helper()
	require.NoError(t, st.RecordScheduleHistory(context.Background(),
		[]scheduler.ScheduleHistoryEntry{historyEntry(programID, "Ep of "+show, show, at)}))
}

func TestRemoveShow_RefusesWhileABlockStillListsIt(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()

	rec := sampleBlock("b1", "Anime Night")
	rec.Spec.Type = scheduler.BlockTypeSeries
	rec.Spec.Series = []scheduler.SeriesConfig{{ShowTitle: "Bloody Mary"}}
	require.NoError(t, st.CreateBlock(ctx, rec))

	_, err := st.RemoveShow(ctx, "Bloody Mary")
	require.Error(t, err)
	assert.ErrorIs(t, err, store.ErrShowStillScheduled)

	var scheduled *store.ShowStillScheduledError
	require.True(t, errors.As(err, &scheduled))
	assert.Equal(t, []string{"Anime Night"}, scheduled.BlockNames,
		"the refusal must name what to edit first")
}

func TestRemoveShow_DeletesStateAiringsAndSnapshotKeys(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	at := time.Now().UTC().Truncate(time.Second)

	require.NoError(t, st.UpdateSeriesState(ctx, &scheduler.SeriesState{
		ShowTitle: "Bloody Mary", CurrentSeason: 2, CurrentEpisode: 5,
	}))
	seedAiring(t, st, "p1", "Bloody Mary", at)
	seedAiring(t, st, "p2", "Night Court", at)

	report, err := st.RemoveShow(ctx, "Bloody Mary")
	require.NoError(t, err)
	assert.Equal(t, int64(1), report.SeriesStates)
	assert.Equal(t, int64(1), report.Airings)

	rows, err := st.ListScheduleHistory(ctx, at.Add(-time.Hour))
	require.NoError(t, err)
	require.Len(t, rows, 1, "the other show's airing must survive")
	assert.Equal(t, "Night Court", rows[0].ShowTitle)
}

// A snapshot describes ONE occurrence of ONE block, which may have carried
// several shows. Deleting the row would take the others with it.
func TestRemoveShow_StripsTheKeyButKeepsTheSnapshotRow(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	occurrence := time.Now().UTC().Truncate(time.Minute)

	snap := scheduler.OccurrenceSnapshot{
		PreStates: map[string]scheduler.SeriesStateSnapshot{
			"Bloody Mary": {CurrentSeason: 2, CurrentEpisode: 5},
			"Night Court": {CurrentSeason: 1, CurrentEpisode: 12},
		},
		PostStates: map[string]scheduler.SeriesStateSnapshot{
			"Bloody Mary": {CurrentSeason: 2, CurrentEpisode: 6},
			"Night Court": {CurrentSeason: 1, CurrentEpisode: 13},
		},
		PlanSeq: 12345,
	}
	require.NoError(t, st.SaveOccurrenceSnapshot(ctx, "b1", occurrence, snap))

	report, err := st.RemoveShow(ctx, "Bloody Mary")
	require.NoError(t, err)
	assert.Equal(t, int64(1), report.Snapshots)

	got, ok, err := st.GetOccurrenceSnapshot(ctx, "b1", occurrence)
	require.NoError(t, err)
	require.True(t, ok, "the snapshot row was deleted, taking the other show with it")
	assert.NotContains(t, got.PreStates, "Bloody Mary")
	assert.NotContains(t, got.PostStates, "Bloody Mary")
	assert.Contains(t, got.PreStates, "Night Court")
	assert.Contains(t, got.PostStates, "Night Court")
	assert.Equal(t, int64(12345), got.PlanSeq,
		"rewriting the row must not disturb the provenance of an occurrence that already aired")
}

// Turning a NULL post_state_json into "{}" flips replayAiredOccurrence
// from "re-seed the chain from live state" to "apply nothing" -- a silent
// behaviour change for every OTHER title in that occurrence.
func TestRemoveShow_LeavesANullPostStateNull(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	occurrence := time.Now().UTC().Truncate(time.Minute)

	require.NoError(t, st.SaveOccurrenceSnapshot(ctx, "b1", occurrence, scheduler.OccurrenceSnapshot{
		PreStates: map[string]scheduler.SeriesStateSnapshot{
			"Bloody Mary": {CurrentSeason: 2, CurrentEpisode: 5},
		},
		PostStates: nil,
	}))

	_, err := st.RemoveShow(ctx, "Bloody Mary")
	require.NoError(t, err)

	got, ok, err := st.GetOccurrenceSnapshot(ctx, "b1", occurrence)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Nil(t, got.PostStates, "a NULL post-state must stay NULL")
}

// An occurrence that loses its last airing must stay "committed, produced
// nothing" rather than reading as never planned -- otherwise the next
// apply re-plans a slot that has already gone out.
func TestRemoveShow_KeepsAnEmptiedOccurrenceCommitted(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	at := time.Now().UTC().Truncate(time.Second)

	seedAiring(t, st, "p1", "Bloody Mary", at)

	report, err := st.RemoveShow(ctx, "Bloody Mary")
	require.NoError(t, err)
	assert.Equal(t, int64(1), report.EmptiedSlots)

	_, committed, err := st.GetCommittedOccurrence(ctx, "Anime Night", at)
	require.NoError(t, err)
	assert.True(t, committed, "the occurrence now reads as never planned, so the next apply would re-plan it")
}

// A movie belongs to no show. Matching an empty show_title would sweep
// every one of them.
func TestRemoveShow_NeverMatchesAShowlessAiring(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	at := time.Now().UTC().Truncate(time.Second)

	seedAiring(t, st, "p1", "", at)
	seedAiring(t, st, "p2", "Bloody Mary", at)

	_, err := st.RemoveShow(ctx, "")
	assert.Error(t, err, "an empty show title is not a removal target")

	_, err = st.RemoveShow(ctx, "Bloody Mary")
	require.NoError(t, err)

	rows, err := st.ListScheduleHistory(ctx, at.Add(-time.Hour))
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Empty(t, rows[0].ShowTitle, "the showless airing must survive")
}

// The transaction is the point of this primitive. A partial removal leaves
// a cursor pointing at airings that no longer exist, which is worse than
// no removal at all -- so a failure partway must leave NOTHING removed.
func TestRemoveShow_APartialFailureRemovesNothing(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	at := time.Now().UTC().Truncate(time.Second)

	require.NoError(t, st.UpdateSeriesState(ctx, &scheduler.SeriesState{
		ShowTitle: "Bloody Mary", CurrentSeason: 2, CurrentEpisode: 5,
	}))
	seedAiring(t, st, "p1", "Bloody Mary", at)

	// Corrupt one snapshot's JSON so the snapshot rewrite -- the LAST step
	// in the transaction -- fails after the state and airings are gone.
	occurrence := at.Truncate(time.Minute)
	require.NoError(t, st.SaveOccurrenceSnapshot(ctx, "b1", occurrence, scheduler.OccurrenceSnapshot{
		PreStates: map[string]scheduler.SeriesStateSnapshot{"Bloody Mary": {CurrentSeason: 1}},
	}))
	corruptSnapshotJSON(t, st, "b1")

	_, err := st.RemoveShow(ctx, "Bloody Mary")
	require.Error(t, err, "the corrupt snapshot should have failed the removal")

	// Everything must still be there.
	state, err := st.GetSeriesState(ctx, "Bloody Mary")
	require.NoError(t, err)
	assert.Equal(t, 5, state.CurrentEpisode, "the cursor was deleted despite the failure")

	rows, err := st.ListScheduleHistory(ctx, at.Add(-time.Hour))
	require.NoError(t, err)
	assert.Len(t, rows, 1, "the airing was deleted despite the failure")
}

// corruptSnapshotJSON writes invalid JSON into a snapshot row, so the
// removal's rewrite step fails. There is no supported way to do this
// through the store's own API, which is the point: it simulates a
// mid-transaction failure rather than a reachable input.
func corruptSnapshotJSON(t *testing.T, st *store.Store, blockID string) {
	t.Helper()
	require.NoError(t, st.ExecForTest(`UPDATE series_occurrence_snapshots SET snapshot_json = 'not json' WHERE block_id = ?`, blockID))
}
