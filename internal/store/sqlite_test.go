package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/christopherime/schedularr/internal/scheduler"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStore_SeriesState(t *testing.T) {
	// Use in-memory database for testing
	s, err := New(":memory:")
	require.NoError(t, err, "Failed to create store")
	defer s.Close()

	ctx := context.Background()
	showTitle := "Test Show"

	// 1. Get initial state (should be default)
	state, err := s.GetSeriesState(ctx, showTitle)
	require.NoError(t, err, "GetSeriesState failed")
	assert.Equal(t, 1, state.CurrentSeason, "Expected default season 1")
	assert.Equal(t, 1, state.CurrentEpisode, "Expected default episode 1")
	assert.False(t, state.Completed, "Expected not completed")

	// 2. Update state
	now := time.Now()
	newState := &scheduler.SeriesState{
		ShowTitle:      showTitle,
		CurrentSeason:  2,
		CurrentEpisode: 5,
		Completed:      false,
		LastAired:      &now,
	}
	require.NoError(t, s.UpdateSeriesState(ctx, newState), "UpdateSeriesState failed")

	// 3. Verify update
	updatedState, err := s.GetSeriesState(ctx, showTitle)
	require.NoError(t, err, "GetSeriesState failed")
	assert.Equal(t, 2, updatedState.CurrentSeason)
	assert.Equal(t, 5, updatedState.CurrentEpisode)
	assert.NotNil(t, updatedState.LastAired, "Expected LastAired to be set")
	assert.False(t, updatedState.LastAired.IsZero(), "Expected LastAired to not be zero")

	// 4. Mark completed
	newState.Completed = true
	require.NoError(t, s.UpdateSeriesState(ctx, newState), "UpdateSeriesState failed")

	finalState, err := s.GetSeriesState(ctx, showTitle)
	require.NoError(t, err, "GetSeriesState failed")
	assert.True(t, finalState.Completed, "Expected completed to be true")
}

// TestStore_GetPersistedSeriesState covers the existence-aware counterpart
// to GetSeriesState added for Task 10 (internal/api/state.go's
// PatchSeriesState 404 semantics): unlike GetSeriesState, which fabricates
// a default S01E01 state for any show, GetPersistedSeriesState must report
// ErrNotFound for a show with no persisted row, and return the real row
// once one exists.
func TestStore_GetPersistedSeriesState(t *testing.T) {
	s, err := New(":memory:")
	require.NoError(t, err, "Failed to create store")
	defer s.Close()

	ctx := context.Background()
	showTitle := "Untracked Show"

	_, err = s.GetPersistedSeriesState(ctx, showTitle)
	require.Error(t, err, "expected ErrNotFound for an untracked show")
	assert.True(t, errors.Is(err, ErrNotFound), "want ErrNotFound, got %v", err)

	require.NoError(t, s.UpdateSeriesState(ctx, &scheduler.SeriesState{
		ShowTitle: showTitle, CurrentSeason: 4, CurrentEpisode: 9,
	}))

	state, err := s.GetPersistedSeriesState(ctx, showTitle)
	require.NoError(t, err, "GetPersistedSeriesState failed after the show was tracked")
	assert.Equal(t, 4, state.CurrentSeason)
	assert.Equal(t, 9, state.CurrentEpisode)
}

func TestStore_ScheduleHistory(t *testing.T) {
	s, err := New(":memory:")
	require.NoError(t, err, "Failed to create store")
	defer s.Close()

	ctx := context.Background()
	now := time.Now()
	entry := scheduler.ScheduleHistoryEntry{
		ProgramID:   "prog-1",
		ChannelID:   "channel-1",
		BlockName:   "Morning Block",
		ScheduledAt: now,
	}

	require.NoError(t, s.RecordScheduleHistory(ctx, []scheduler.ScheduleHistoryEntry{entry}), "RecordScheduleHistory failed")

	recent, err := s.WasRecentlyScheduled(ctx, "prog-1", "channel-1", 24*time.Hour)
	require.NoError(t, err, "WasRecentlyScheduled failed")
	assert.True(t, recent, "Expected program to be recently scheduled")
}

func TestStore_ScheduleHistoryCleanup(t *testing.T) {
	s, err := New(":memory:")
	require.NoError(t, err, "Failed to create store")
	defer s.Close()

	ctx := context.Background()
	oldEntry := scheduler.ScheduleHistoryEntry{
		ProgramID:   "old-prog",
		ChannelID:   "channel-1",
		BlockName:   "Old Block",
		ScheduledAt: time.Now().Add(-48 * time.Hour),
	}
	newEntry := scheduler.ScheduleHistoryEntry{
		ProgramID:   "new-prog",
		ChannelID:   "channel-1",
		BlockName:   "New Block",
		ScheduledAt: time.Now(),
	}

	require.NoError(t, s.RecordScheduleHistory(ctx, []scheduler.ScheduleHistoryEntry{oldEntry, newEntry}), "RecordScheduleHistory failed")

	removed, err := s.CleanupScheduleHistory(ctx, 24*time.Hour)
	require.NoError(t, err, "CleanupScheduleHistory failed")
	require.Equal(t, int64(1), removed, "Expected 1 entry removed")

	oldRecent, err := s.WasRecentlyScheduled(ctx, "old-prog", "channel-1", 24*time.Hour)
	require.NoError(t, err, "WasRecentlyScheduled failed")
	assert.False(t, oldRecent, "Expected old entry to be removed")

	newRecent, err := s.WasRecentlyScheduled(ctx, "new-prog", "channel-1", 24*time.Hour)
	require.NoError(t, err, "WasRecentlyScheduled failed")
	assert.True(t, newRecent, "Expected new entry to remain")
}

// TestStore_OccurrenceSnapshots exercises the real SQL behind
// GetOccurrenceSnapshot/SaveOccurrenceSnapshot against the migration
// 000005 schema (series_occurrence_snapshots keyed by block_id, with a
// recorded_at column) -- round 3 added these methods without a dedicated
// SQL-level test (only exercised indirectly via scheduler.MockStateStore
// in engine_test.go), and round 4 rewrote the key column and made the
// save an upsert, so this closes that gap: a wrong column name or a
// missing ON CONFLICT clause would compile fine and only surface here.
func TestStore_OccurrenceSnapshots(t *testing.T) {
	s, err := New(":memory:")
	require.NoError(t, err, "Failed to create store")
	defer s.Close()

	ctx := context.Background()
	occurrenceStart := time.Date(2026, 6, 1, 20, 0, 0, 0, time.UTC)

	// 1. No snapshot yet.
	_, ok, err := s.GetOccurrenceSnapshot(ctx, "block-a", occurrenceStart)
	require.NoError(t, err)
	assert.False(t, ok, "expected no snapshot before any save")

	// 2. Save, then read back.
	first := map[string]scheduler.SeriesStateSnapshot{
		"Show A": {CurrentSeason: 1, CurrentEpisode: 1, Seeded: false},
	}
	require.NoError(t, s.SaveOccurrenceSnapshot(ctx, "block-a", occurrenceStart, first))

	got, ok, err := s.GetOccurrenceSnapshot(ctx, "block-a", occurrenceStart)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, first, got)

	// 3. Saving again for the SAME (block_id, occurrence_start) must
	// upsert -- overwrite, not error on a duplicate primary key -- since
	// planSeriesOccurrences' chain mechanism rewrites a not-yet-aired
	// occurrence's snapshot when an earlier occurrence in the same block
	// is re-derived.
	second := map[string]scheduler.SeriesStateSnapshot{
		"Show A": {CurrentSeason: 1, CurrentEpisode: 6, Seeded: true},
	}
	require.NoError(t, s.SaveOccurrenceSnapshot(ctx, "block-a", occurrenceStart, second), "second save for the same key must upsert, not fail")

	got, ok, err = s.GetOccurrenceSnapshot(ctx, "block-a", occurrenceStart)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, second, got, "expected the upsert to have overwritten the first snapshot")

	// 4. A different block ID at the SAME occurrence_start is independent
	// -- proves the key is (block_id, occurrence_start), not just
	// occurrence_start (i.e. the migration 000005 rekey from block_name
	// to block_id actually took, and didn't collapse distinct blocks onto
	// each other by accident).
	_, ok, err = s.GetOccurrenceSnapshot(ctx, "block-b", occurrenceStart)
	require.NoError(t, err)
	assert.False(t, ok, "a different block ID must not see block-a's snapshot")
}

// TestStore_DeleteFutureOccurrenceSnapshots pins finding 3's fix: an
// operator's series_state PATCH (or a block edit/delete) must invalidate
// only the NOT-YET-AIRED occurrences of the affected block, leaving
// already-aired ones (occurrence_start <= now) and other blocks alone.
func TestStore_DeleteFutureOccurrenceSnapshots(t *testing.T) {
	s, err := New(":memory:")
	require.NoError(t, err, "Failed to create store")
	defer s.Close()

	ctx := context.Background()
	now := time.Now()
	past := now.Add(-1 * time.Hour)
	future1 := now.Add(1 * time.Hour)
	future2 := now.Add(2 * time.Hour)

	snapshot := map[string]scheduler.SeriesStateSnapshot{"Show A": {CurrentSeason: 1, CurrentEpisode: 1}}
	require.NoError(t, s.SaveOccurrenceSnapshot(ctx, "block-a", past, snapshot))
	require.NoError(t, s.SaveOccurrenceSnapshot(ctx, "block-a", future1, snapshot))
	require.NoError(t, s.SaveOccurrenceSnapshot(ctx, "block-a", future2, snapshot))
	require.NoError(t, s.SaveOccurrenceSnapshot(ctx, "block-b", future1, snapshot), "a different block's future snapshot must survive")

	require.NoError(t, s.DeleteFutureOccurrenceSnapshots(ctx, "block-a", now))

	_, ok, err := s.GetOccurrenceSnapshot(ctx, "block-a", past)
	require.NoError(t, err)
	assert.True(t, ok, "an already-aired occurrence's snapshot must survive")

	_, ok, err = s.GetOccurrenceSnapshot(ctx, "block-a", future1)
	require.NoError(t, err)
	assert.False(t, ok, "a not-yet-aired occurrence's snapshot must be deleted")

	_, ok, err = s.GetOccurrenceSnapshot(ctx, "block-a", future2)
	require.NoError(t, err)
	assert.False(t, ok, "a not-yet-aired occurrence's snapshot must be deleted")

	_, ok, err = s.GetOccurrenceSnapshot(ctx, "block-b", future1)
	require.NoError(t, err)
	assert.True(t, ok, "a different block's snapshot must be untouched")
}

// TestStore_CleanupOccurrenceSnapshots pins finding 4's GC fix, and
// specifically that it prunes by recorded_at (a real wall-clock write
// timestamp), not occurrence_start -- see migration 000005's recorded_at
// column doc comment for why the latter would be wrong (it would prune a
// legitimately just-created snapshot outright whenever occurrence_start
// itself is far from the real clock, e.g. a schedule-generation window
// that doesn't start at "now"). The stale row here is inserted directly
// via SQL (bypassing SaveOccurrenceSnapshot, which always stamps
// recorded_at = time.Now()) specifically to control recorded_at without
// a real sleep.
func TestStore_CleanupOccurrenceSnapshots(t *testing.T) {
	s, err := New(":memory:")
	require.NoError(t, err, "Failed to create store")
	defer s.Close()

	ctx := context.Background()

	// Fixed occurrenceStart values, reused at every lookup below (rather
	// than calling time.Now() again at each use), so a lookup can't
	// spuriously miss from two separate time.Now() calls landing
	// nanoseconds apart.
	staleOccurrenceStart := time.Now().Add(30 * 24 * time.Hour)
	freshOccurrenceStart := time.Now().Add(1 * time.Hour)

	// A stale row: occurrence_start far in the future (so an
	// occurrence_start-based cutoff would NOT prune it), but recorded_at
	// far in the past (so a recorded_at-based cutoff, the correct
	// behavior, DOES).
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO series_occurrence_snapshots (block_id, occurrence_start, snapshot_json, recorded_at)
		VALUES (?, ?, ?, ?)`,
		"block-stale", staleOccurrenceStart, `{}`, time.Now().Add(-48*time.Hour))
	require.NoError(t, err)

	// A fresh row via the normal path -- recorded_at is "now".
	snapshot := map[string]scheduler.SeriesStateSnapshot{"Show A": {CurrentSeason: 1, CurrentEpisode: 1}}
	require.NoError(t, s.SaveOccurrenceSnapshot(ctx, "block-fresh", freshOccurrenceStart, snapshot))

	removed, err := s.CleanupOccurrenceSnapshots(ctx, 24*time.Hour)
	require.NoError(t, err)
	require.Equal(t, int64(1), removed, "expected exactly the stale row to be removed")

	_, ok, err := s.GetOccurrenceSnapshot(ctx, "block-stale", staleOccurrenceStart)
	require.NoError(t, err)
	assert.False(t, ok, "the stale (old recorded_at) row must be gone")

	_, ok, err = s.GetOccurrenceSnapshot(ctx, "block-fresh", freshOccurrenceStart)
	require.NoError(t, err)
	assert.True(t, ok, "the fresh row must survive")
}

func TestStore_ExportImportSeriesStates(t *testing.T) {
	s, err := New(":memory:")
	require.NoError(t, err, "Failed to create store")
	defer s.Close()

	ctx := context.Background()

	// Create some test states
	t1 := time.Now().Add(-24 * time.Hour)
	t2 := time.Now().Add(-12 * time.Hour)
	t3 := time.Now().Add(-48 * time.Hour)
	states := []scheduler.SeriesState{
		{
			ShowTitle:      "Show A",
			CurrentSeason:  2,
			CurrentEpisode: 5,
			Completed:      false,
			LastAired:      &t1,
		},
		{
			ShowTitle:      "Show B",
			CurrentSeason:  1,
			CurrentEpisode: 10,
			Completed:      false,
			LastAired:      &t2,
		},
		{
			ShowTitle:      "Show C",
			CurrentSeason:  3,
			CurrentEpisode: 1,
			Completed:      true,
			LastAired:      &t3,
		},
	}

	// Import states
	require.NoError(t, s.ImportSeriesStates(ctx, states), "ImportSeriesStates failed")

	// Export states
	exported, err := s.ExportAllSeriesStates(ctx)
	require.NoError(t, err, "ExportAllSeriesStates failed")
	require.Len(t, exported, len(states), "Expected %d states", len(states))

	// Verify each state (order should be by show_title due to ORDER BY)
	expectedOrder := []string{"Show A", "Show B", "Show C"}
	for i, expected := range expectedOrder {
		assert.Equal(t, expected, exported[i].ShowTitle, "State %d should be %s", i, expected)
	}

	// Verify specific state details
	for _, state := range exported {
		var original *scheduler.SeriesState
		for i := range states {
			if states[i].ShowTitle == state.ShowTitle {
				original = &states[i]
				break
			}
		}
		require.NotNil(t, original, "Exported state %s not found in original states", state.ShowTitle)
		assert.Equal(t, original.CurrentSeason, state.CurrentSeason, "Season mismatch for %s", state.ShowTitle)
		assert.Equal(t, original.CurrentEpisode, state.CurrentEpisode, "Episode mismatch for %s", state.ShowTitle)
		assert.Equal(t, original.Completed, state.Completed, "Completed mismatch for %s", state.ShowTitle)
	}
}

func TestStore_ResetSeriesState(t *testing.T) {
	s, err := New(":memory:")
	require.NoError(t, err, "Failed to create store")
	defer s.Close()

	ctx := context.Background()
	showTitle := "Test Show"

	// Create a state with progress
	now := time.Now()
	state := &scheduler.SeriesState{
		ShowTitle:      showTitle,
		CurrentSeason:  3,
		CurrentEpisode: 15,
		Completed:      true,
		LastAired:      &now,
	}
	require.NoError(t, s.UpdateSeriesState(ctx, state), "UpdateSeriesState failed")

	// Reset the state
	require.NoError(t, s.ResetSeriesState(ctx, showTitle), "ResetSeriesState failed")

	// Verify reset
	resetState, err := s.GetSeriesState(ctx, showTitle)
	require.NoError(t, err, "GetSeriesState failed")

	assert.Equal(t, 1, resetState.CurrentSeason, "Expected season 1 after reset")
	assert.Equal(t, 1, resetState.CurrentEpisode, "Expected episode 1 after reset")
	assert.False(t, resetState.Completed, "Expected completed to be false after reset")
	assert.Nil(t, resetState.LastAired, "Expected LastAired to be nil after reset")
}

func TestStore_ImportSeriesStates_Empty(t *testing.T) {
	s, err := New(":memory:")
	require.NoError(t, err, "Failed to create store")
	defer s.Close()

	ctx := context.Background()

	// Import empty slice should not error
	assert.NoError(t, s.ImportSeriesStates(ctx, []scheduler.SeriesState{}), "ImportSeriesStates with empty slice should not error")

	// Import nil should not error
	assert.NoError(t, s.ImportSeriesStates(ctx, nil), "ImportSeriesStates with nil should not error")
}

func TestStore_RecordScheduleHistory_Empty(t *testing.T) {
	s, err := New(":memory:")
	require.NoError(t, err, "Failed to create store")
	defer s.Close()

	ctx := context.Background()

	// Recording empty slice should not error
	assert.NoError(t, s.RecordScheduleHistory(ctx, []scheduler.ScheduleHistoryEntry{}), "RecordScheduleHistory with empty slice should not error")

	// Recording nil should not error
	assert.NoError(t, s.RecordScheduleHistory(ctx, nil), "RecordScheduleHistory with nil should not error")
}

func TestStore_WasRecentlyScheduled_InvalidInput(t *testing.T) {
	s, err := New(":memory:")
	require.NoError(t, err, "Failed to create store")
	defer s.Close()

	ctx := context.Background()

	// Empty program ID
	_, err = s.WasRecentlyScheduled(ctx, "", "channel-1", 24*time.Hour)
	assert.Error(t, err, "Expected error for empty program ID")

	// Empty channel ID
	_, err = s.WasRecentlyScheduled(ctx, "prog-1", "", 24*time.Hour)
	assert.Error(t, err, "Expected error for empty channel ID")

	// Both empty
	_, err = s.WasRecentlyScheduled(ctx, "", "", 24*time.Hour)
	assert.Error(t, err, "Expected error for both empty")
}

func TestStore_WasRecentlyScheduled_NotFound(t *testing.T) {
	s, err := New(":memory:")
	require.NoError(t, err, "Failed to create store")
	defer s.Close()

	ctx := context.Background()

	// Check for program that was never scheduled
	recent, err := s.WasRecentlyScheduled(ctx, "nonexistent", "channel-1", 24*time.Hour)
	require.NoError(t, err, "WasRecentlyScheduled failed")
	assert.False(t, recent, "Expected program to not be recently scheduled")
}

func TestStore_WasRecentlyScheduled_OutsideWindow(t *testing.T) {
	s, err := New(":memory:")
	require.NoError(t, err, "Failed to create store")
	defer s.Close()

	ctx := context.Background()

	// Record an old entry
	oldEntry := scheduler.ScheduleHistoryEntry{
		ProgramID:   "old-prog",
		ChannelID:   "channel-1",
		BlockName:   "Old Block",
		ScheduledAt: time.Now().Add(-72 * time.Hour), // 3 days ago
	}

	require.NoError(t, s.RecordScheduleHistory(ctx, []scheduler.ScheduleHistoryEntry{oldEntry}), "RecordScheduleHistory failed")

	// Check with 24 hour window (should not find it)
	recent, err := s.WasRecentlyScheduled(ctx, "old-prog", "channel-1", 24*time.Hour)
	require.NoError(t, err, "WasRecentlyScheduled failed")
	assert.False(t, recent, "Expected old program to not be within 24h window")

	// Check with 96 hour window (should find it)
	recent96, err := s.WasRecentlyScheduled(ctx, "old-prog", "channel-1", 96*time.Hour)
	require.NoError(t, err, "WasRecentlyScheduled failed")
	assert.True(t, recent96, "Expected old program to be within 96h window")
}

func TestStore_GetSeriesState_MultipleShows(t *testing.T) {
	s, err := New(":memory:")
	require.NoError(t, err, "Failed to create store")
	defer s.Close()

	ctx := context.Background()

	// Update multiple shows
	shows := []struct {
		title   string
		season  int
		episode int
	}{
		{"Show A", 1, 5},
		{"Show B", 2, 10},
		{"Show C", 3, 1},
	}

	for _, show := range shows {
		now := time.Now()
		state := &scheduler.SeriesState{
			ShowTitle:      show.title,
			CurrentSeason:  show.season,
			CurrentEpisode: show.episode,
			Completed:      false,
			LastAired:      &now,
		}
		require.NoError(t, s.UpdateSeriesState(ctx, state), "UpdateSeriesState failed for %s", show.title)
	}

	// Verify each show has correct state
	for _, show := range shows {
		state, err := s.GetSeriesState(ctx, show.title)
		require.NoError(t, err, "GetSeriesState failed for %s", show.title)
		assert.Equal(t, show.season, state.CurrentSeason, "Season mismatch for %s", show.title)
		assert.Equal(t, show.episode, state.CurrentEpisode, "Episode mismatch for %s", show.title)
	}
}

func TestStore_ResetSeriesState_NonExistent(t *testing.T) {
	s, err := New(":memory:")
	require.NoError(t, err, "Failed to create store")
	defer s.Close()

	ctx := context.Background()

	// Reset a show that doesn't exist should not error
	assert.NoError(t, s.ResetSeriesState(ctx, "NonExistent Show"), "ResetSeriesState for non-existent show should not error")

	// Verify the state is now default
	state, err := s.GetSeriesState(ctx, "NonExistent Show")
	require.NoError(t, err, "GetSeriesState failed")
	assert.Equal(t, 1, state.CurrentSeason, "Expected default season 1")
	assert.Equal(t, 1, state.CurrentEpisode, "Expected default episode 1")
}

func TestStore_CleanupScheduleHistory_EmptyDatabase(t *testing.T) {
	s, err := New(":memory:")
	require.NoError(t, err, "Failed to create store")
	defer s.Close()

	ctx := context.Background()

	// Cleanup on empty database should not error and return 0
	removed, err := s.CleanupScheduleHistory(ctx, 24*time.Hour)
	require.NoError(t, err, "CleanupScheduleHistory failed")
	assert.Equal(t, int64(0), removed, "Expected 0 entries removed from empty database")
}

func TestStore_ExportAllSeriesStates_Empty(t *testing.T) {
	s, err := New(":memory:")
	require.NoError(t, err, "Failed to create store")
	defer s.Close()

	ctx := context.Background()

	// Export from empty database should return empty slice
	states, err := s.ExportAllSeriesStates(ctx)
	require.NoError(t, err, "ExportAllSeriesStates failed")
	assert.Empty(t, states, "Expected 0 states from empty database")
}

func TestStore_Backup(t *testing.T) {
	s, err := New(":memory:")
	require.NoError(t, err, "Failed to create store")
	defer s.Close()

	ctx := context.Background()

	// Add some data
	now := time.Now()
	state := &scheduler.SeriesState{
		ShowTitle:      "Test Show",
		CurrentSeason:  2,
		CurrentEpisode: 5,
		Completed:      false,
		LastAired:      &now,
	}
	require.NoError(t, s.UpdateSeriesState(ctx, state), "UpdateSeriesState failed")

	// Create backup to a temporary file
	backupPath := t.TempDir() + "/backup.db"
	require.NoError(t, s.Backup(ctx, backupPath), "Backup failed")

	// Open the backup and verify data
	backupStore, err := New(backupPath)
	require.NoError(t, err, "Failed to open backup")
	defer backupStore.Close()

	// Verify the data was backed up
	restoredState, err := backupStore.GetSeriesState(ctx, "Test Show")
	require.NoError(t, err, "GetSeriesState from backup failed")
	assert.Equal(t, 2, restoredState.CurrentSeason, "Backup data mismatch: season")
	assert.Equal(t, 5, restoredState.CurrentEpisode, "Backup data mismatch: episode")
}

func TestStore_Backup_InvalidPath(t *testing.T) {
	s, err := New(":memory:")
	require.NoError(t, err, "Failed to create store")
	defer s.Close()

	ctx := context.Background()

	// Backup to an invalid path should return an error
	err = s.Backup(ctx, "/nonexistent/directory/backup.db")
	assert.Error(t, err, "Expected error for backup to invalid path")
}

func TestStore_SeriesState_RunCountAndDisabled(t *testing.T) {
	s, err := New(":memory:")
	require.NoError(t, err, "Failed to create store")
	defer s.Close()

	ctx := context.Background()
	showTitle := "Test Show With RunCount"

	// Create a state with RunCount and Disabled fields
	now := time.Now()
	state := &scheduler.SeriesState{
		ShowTitle:      showTitle,
		CurrentSeason:  1,
		CurrentEpisode: 1,
		Completed:      false,
		LastAired:      &now,
		RunCount:       5,
		Disabled:       true,
	}
	require.NoError(t, s.UpdateSeriesState(ctx, state), "UpdateSeriesState failed")

	// Verify RunCount and Disabled are persisted
	retrievedState, err := s.GetSeriesState(ctx, showTitle)
	require.NoError(t, err, "GetSeriesState failed")
	assert.Equal(t, 5, retrievedState.RunCount, "Expected RunCount to be 5")
	assert.True(t, retrievedState.Disabled, "Expected Disabled to be true")

	// Update RunCount
	state.RunCount = 10
	state.Disabled = false
	require.NoError(t, s.UpdateSeriesState(ctx, state), "UpdateSeriesState failed")

	retrievedState, err = s.GetSeriesState(ctx, showTitle)
	require.NoError(t, err, "GetSeriesState failed")
	assert.Equal(t, 10, retrievedState.RunCount, "Expected RunCount to be 10")
	assert.False(t, retrievedState.Disabled, "Expected Disabled to be false")
}

func TestStore_OperationsOnClosedStore(t *testing.T) {
	s, err := New(":memory:")
	require.NoError(t, err, "Failed to create store")

	ctx := context.Background()

	// Close the store
	require.NoError(t, s.Close(), "Close failed")

	// All operations on a closed store should fail
	_, err = s.GetSeriesState(ctx, "Test")
	assert.Error(t, err, "GetSeriesState on closed store should error")

	err = s.UpdateSeriesState(ctx, &scheduler.SeriesState{ShowTitle: "Test"})
	assert.Error(t, err, "UpdateSeriesState on closed store should error")

	err = s.RecordScheduleHistory(ctx, []scheduler.ScheduleHistoryEntry{
		{ProgramID: "p1", ChannelID: "c1", BlockName: "b1", ScheduledAt: time.Now()},
	})
	assert.Error(t, err, "RecordScheduleHistory on closed store should error")

	_, err = s.WasRecentlyScheduled(ctx, "p1", "c1", time.Hour)
	assert.Error(t, err, "WasRecentlyScheduled on closed store should error")

	_, err = s.CleanupScheduleHistory(ctx, time.Hour)
	assert.Error(t, err, "CleanupScheduleHistory on closed store should error")

	_, err = s.ExportAllSeriesStates(ctx)
	assert.Error(t, err, "ExportAllSeriesStates on closed store should error")

	err = s.ImportSeriesStates(ctx, []scheduler.SeriesState{{ShowTitle: "Test"}})
	assert.Error(t, err, "ImportSeriesStates on closed store should error")

	err = s.ResetSeriesState(ctx, "Test")
	assert.Error(t, err, "ResetSeriesState on closed store should error")
}

func TestNew_InvalidDSN(t *testing.T) {
	// Opening a database at an invalid path should fail
	_, err := New("/nonexistent/path/to/db.sqlite")
	assert.Error(t, err, "Expected error for invalid DSN path")
}

func TestStore_CleanupScheduleHistory_MultipleCutoffs(t *testing.T) {
	s, err := New(":memory:")
	require.NoError(t, err, "Failed to create store")
	defer s.Close()

	ctx := context.Background()

	// Create entries at different times
	entries := []scheduler.ScheduleHistoryEntry{
		{ProgramID: "prog-1", ChannelID: "ch-1", BlockName: "b1", ScheduledAt: time.Now().Add(-7 * 24 * time.Hour)}, // 7 days ago
		{ProgramID: "prog-2", ChannelID: "ch-1", BlockName: "b1", ScheduledAt: time.Now().Add(-3 * 24 * time.Hour)}, // 3 days ago
		{ProgramID: "prog-3", ChannelID: "ch-1", BlockName: "b1", ScheduledAt: time.Now().Add(-1 * 24 * time.Hour)}, // 1 day ago
		{ProgramID: "prog-4", ChannelID: "ch-1", BlockName: "b1", ScheduledAt: time.Now().Add(-12 * time.Hour)},     // 12 hours ago
		{ProgramID: "prog-5", ChannelID: "ch-1", BlockName: "b1", ScheduledAt: time.Now()},                          // now
	}
	require.NoError(t, s.RecordScheduleHistory(ctx, entries), "RecordScheduleHistory failed")

	// Cleanup with 2-day window (should remove prog-1, prog-2)
	removed, err := s.CleanupScheduleHistory(ctx, 2*24*time.Hour)
	require.NoError(t, err, "CleanupScheduleHistory failed")
	assert.Equal(t, int64(2), removed, "Expected 2 entries removed")

	// Verify remaining entries
	for _, prog := range []string{"prog-3", "prog-4", "prog-5"} {
		recent, err := s.WasRecentlyScheduled(ctx, prog, "ch-1", 30*24*time.Hour)
		require.NoError(t, err)
		assert.True(t, recent, "Expected %s to still exist", prog)
	}

	for _, prog := range []string{"prog-1", "prog-2"} {
		recent, err := s.WasRecentlyScheduled(ctx, prog, "ch-1", 30*24*time.Hour)
		require.NoError(t, err)
		assert.False(t, recent, "Expected %s to be removed", prog)
	}
}

func TestStore_WasRecentlyScheduled_DifferentChannels(t *testing.T) {
	s, err := New(":memory:")
	require.NoError(t, err, "Failed to create store")
	defer s.Close()

	ctx := context.Background()

	// Record same program on different channels
	entries := []scheduler.ScheduleHistoryEntry{
		{ProgramID: "prog-1", ChannelID: "channel-A", BlockName: "b1", ScheduledAt: time.Now()},
		{ProgramID: "prog-1", ChannelID: "channel-B", BlockName: "b1", ScheduledAt: time.Now().Add(-48 * time.Hour)},
	}
	require.NoError(t, s.RecordScheduleHistory(ctx, entries), "RecordScheduleHistory failed")

	// prog-1 should be recent on channel-A but not on channel-B (with 24h window)
	recentA, err := s.WasRecentlyScheduled(ctx, "prog-1", "channel-A", 24*time.Hour)
	require.NoError(t, err)
	assert.True(t, recentA, "Expected prog-1 to be recent on channel-A")

	recentB, err := s.WasRecentlyScheduled(ctx, "prog-1", "channel-B", 24*time.Hour)
	require.NoError(t, err)
	assert.False(t, recentB, "Expected prog-1 to not be recent on channel-B with 24h window")

	// With 72h window, should find on channel-B
	recentB72, err := s.WasRecentlyScheduled(ctx, "prog-1", "channel-B", 72*time.Hour)
	require.NoError(t, err)
	assert.True(t, recentB72, "Expected prog-1 to be recent on channel-B with 72h window")
}

func TestStore_ImportSeriesStates_UpdateExisting(t *testing.T) {
	s, err := New(":memory:")
	require.NoError(t, err, "Failed to create store")
	defer s.Close()

	ctx := context.Background()

	// Initial import
	t1 := time.Now()
	initial := []scheduler.SeriesState{
		{ShowTitle: "Show A", CurrentSeason: 1, CurrentEpisode: 1, Completed: false, LastAired: &t1},
		{ShowTitle: "Show B", CurrentSeason: 1, CurrentEpisode: 5, Completed: false, LastAired: &t1},
	}
	require.NoError(t, s.ImportSeriesStates(ctx, initial), "Initial import failed")

	// Update via import (should use ON CONFLICT DO UPDATE)
	t2 := time.Now()
	updated := []scheduler.SeriesState{
		{ShowTitle: "Show A", CurrentSeason: 2, CurrentEpisode: 10, Completed: true, LastAired: &t2},
		{ShowTitle: "Show C", CurrentSeason: 1, CurrentEpisode: 1, Completed: false, LastAired: &t2}, // new show
	}
	require.NoError(t, s.ImportSeriesStates(ctx, updated), "Update import failed")

	// Verify Show A was updated
	stateA, err := s.GetSeriesState(ctx, "Show A")
	require.NoError(t, err)
	assert.Equal(t, 2, stateA.CurrentSeason, "Show A season should be updated")
	assert.Equal(t, 10, stateA.CurrentEpisode, "Show A episode should be updated")
	assert.True(t, stateA.Completed, "Show A should be completed")

	// Verify Show B was not modified
	stateB, err := s.GetSeriesState(ctx, "Show B")
	require.NoError(t, err)
	assert.Equal(t, 1, stateB.CurrentSeason, "Show B should not be modified")
	assert.Equal(t, 5, stateB.CurrentEpisode, "Show B should not be modified")

	// Verify Show C was added
	stateC, err := s.GetSeriesState(ctx, "Show C")
	require.NoError(t, err)
	assert.Equal(t, 1, stateC.CurrentSeason, "Show C should exist")
}

func TestStore_SetSeriesState(t *testing.T) {
	s, err := New(":memory:")
	require.NoError(t, err, "Failed to create store")
	defer s.Close()

	ctx := context.Background()
	showTitle := "Test Show"

	// Set state for a new series
	require.NoError(t, s.SetSeriesState(ctx, showTitle, 2, 5), "SetSeriesState failed")

	// Verify the state
	state, err := s.GetSeriesState(ctx, showTitle)
	require.NoError(t, err, "GetSeriesState failed")
	assert.Equal(t, 2, state.CurrentSeason, "Expected season 2")
	assert.Equal(t, 5, state.CurrentEpisode, "Expected episode 5")
	assert.False(t, state.Completed, "Expected not completed")
	assert.False(t, state.Disabled, "Expected not disabled")
	assert.Equal(t, 0, state.RunCount, "Expected run count 0")

	// Update the state to a different position
	require.NoError(t, s.SetSeriesState(ctx, showTitle, 3, 10), "SetSeriesState failed")

	// Verify the update
	state, err = s.GetSeriesState(ctx, showTitle)
	require.NoError(t, err, "GetSeriesState failed")
	assert.Equal(t, 3, state.CurrentSeason, "Expected season 3")
	assert.Equal(t, 10, state.CurrentEpisode, "Expected episode 10")
}

func TestStore_SetSeriesState_ClearsCompletedAndDisabled(t *testing.T) {
	s, err := New(":memory:")
	require.NoError(t, err, "Failed to create store")
	defer s.Close()

	ctx := context.Background()
	showTitle := "Completed Show"

	// First create a completed and disabled state
	now := time.Now()
	state := &scheduler.SeriesState{
		ShowTitle:      showTitle,
		CurrentSeason:  5,
		CurrentEpisode: 20,
		Completed:      true,
		Disabled:       true,
		RunCount:       3,
		LastAired:      &now,
	}
	require.NoError(t, s.UpdateSeriesState(ctx, state), "UpdateSeriesState failed")

	// Verify it's completed and disabled
	state, err = s.GetSeriesState(ctx, showTitle)
	require.NoError(t, err)
	assert.True(t, state.Completed, "Expected completed to be true")
	assert.True(t, state.Disabled, "Expected disabled to be true")
	assert.Equal(t, 3, state.RunCount, "Expected run count 3")

	// Use SetSeriesState to jump to a specific episode
	require.NoError(t, s.SetSeriesState(ctx, showTitle, 1, 1), "SetSeriesState failed")

	// Verify completed and disabled are cleared, but run count is preserved
	state, err = s.GetSeriesState(ctx, showTitle)
	require.NoError(t, err)
	assert.Equal(t, 1, state.CurrentSeason, "Expected season 1")
	assert.Equal(t, 1, state.CurrentEpisode, "Expected episode 1")
	assert.False(t, state.Completed, "Expected completed to be cleared")
	assert.False(t, state.Disabled, "Expected disabled to be cleared")
}

func TestStore_SetSeriesState_InvalidInput(t *testing.T) {
	s, err := New(":memory:")
	require.NoError(t, err, "Failed to create store")
	defer s.Close()

	ctx := context.Background()

	// Season < 1
	err = s.SetSeriesState(ctx, "Test Show", 0, 5)
	assert.Error(t, err, "Expected error for season < 1")

	// Episode < 1
	err = s.SetSeriesState(ctx, "Test Show", 1, 0)
	assert.Error(t, err, "Expected error for episode < 1")

	// Negative values
	err = s.SetSeriesState(ctx, "Test Show", -1, 5)
	assert.Error(t, err, "Expected error for negative season")

	err = s.SetSeriesState(ctx, "Test Show", 1, -5)
	assert.Error(t, err, "Expected error for negative episode")
}

func TestStore_SetSeriesState_OnClosedStore(t *testing.T) {
	s, err := New(":memory:")
	require.NoError(t, err, "Failed to create store")

	ctx := context.Background()

	// Close the store
	require.NoError(t, s.Close(), "Close failed")

	// SetSeriesState on closed store should fail
	err = s.SetSeriesState(ctx, "Test Show", 1, 1)
	assert.Error(t, err, "SetSeriesState on closed store should error")
}

func TestStore_Ping(t *testing.T) {
	s, err := New(":memory:")
	require.NoError(t, err, "Failed to create store")
	defer s.Close()

	require.NoError(t, s.Ping(context.Background()), "Ping on a live store should succeed")
}

func TestStore_Ping_OnClosedStore(t *testing.T) {
	s, err := New(":memory:")
	require.NoError(t, err, "Failed to create store")
	require.NoError(t, s.Close(), "Close failed")

	err = s.Ping(context.Background())
	assert.Error(t, err, "Ping on a closed store should error")
}
