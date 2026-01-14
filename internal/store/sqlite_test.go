package store

import (
	"context"
	"testing"
	"time"

	"github.com/geekxflood/schedularr/internal/scheduler"
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
