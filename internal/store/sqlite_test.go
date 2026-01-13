package store

import (
	"context"
	"testing"
	"time"

	"github.com/geekxflood/schedularr/internal/scheduler"
)

func TestStore_SeriesState(t *testing.T) {
	// Use in-memory database for testing
	s, err := New(":memory:")
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	showTitle := "Test Show"

	// 1. Get initial state (should be default)
	state, err := s.GetSeriesState(ctx, showTitle)
	if err != nil {
		t.Fatalf("GetSeriesState failed: %v", err)
	}
	if state.CurrentSeason != 1 || state.CurrentEpisode != 1 {
		t.Errorf("Expected default state S01E01, got S%02dE%02d", state.CurrentSeason, state.CurrentEpisode)
	}
	if state.Completed {
		t.Error("Expected not completed")
	}

	// 2. Update state
	newState := &scheduler.SeriesState{
		ShowTitle:      showTitle,
		CurrentSeason:  2,
		CurrentEpisode: 5,
		Completed:      false,
		LastAired:      time.Now(),
	}
	if err := s.UpdateSeriesState(ctx, newState); err != nil {
		t.Fatalf("UpdateSeriesState failed: %v", err)
	}

	// 3. Verify update
	updatedState, err := s.GetSeriesState(ctx, showTitle)
	if err != nil {
		t.Fatalf("GetSeriesState failed: %v", err)
	}
	if updatedState.CurrentSeason != 2 || updatedState.CurrentEpisode != 5 {
		t.Errorf("Expected S02E05, got S%02dE%02d", updatedState.CurrentSeason, updatedState.CurrentEpisode)
	}
	// Check time is close enough (sqlite stores time, but might lose some precision depending on format,
	// though go-sqlite3 usually handles it well. Let's just check it's not zero).
	if updatedState.LastAired.IsZero() {
		t.Error("Expected LastAired to be set")
	}

	// 4. Mark completed
	newState.Completed = true
	if err := s.UpdateSeriesState(ctx, newState); err != nil {
		t.Fatalf("UpdateSeriesState failed: %v", err)
	}

	finalState, err := s.GetSeriesState(ctx, showTitle)
	if err != nil {
		t.Fatalf("GetSeriesState failed: %v", err)
	}
	if !finalState.Completed {
		t.Error("Expected completed to be true")
	}
}

func TestStore_ScheduleHistory(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	now := time.Now()
	entry := scheduler.ScheduleHistoryEntry{
		ProgramID:   "prog-1",
		ChannelID:   "channel-1",
		BlockName:   "Morning Block",
		ScheduledAt: now,
	}

	if err := s.RecordScheduleHistory(ctx, []scheduler.ScheduleHistoryEntry{entry}); err != nil {
		t.Fatalf("RecordScheduleHistory failed: %v", err)
	}

	recent, err := s.WasRecentlyScheduled(ctx, "prog-1", "channel-1", 24*time.Hour)
	if err != nil {
		t.Fatalf("WasRecentlyScheduled failed: %v", err)
	}
	if !recent {
		t.Error("Expected program to be recently scheduled")
	}
}

func TestStore_ScheduleHistoryCleanup(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
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

	if err := s.RecordScheduleHistory(ctx, []scheduler.ScheduleHistoryEntry{oldEntry, newEntry}); err != nil {
		t.Fatalf("RecordScheduleHistory failed: %v", err)
	}

	removed, err := s.CleanupScheduleHistory(ctx, 24*time.Hour)
	if err != nil {
		t.Fatalf("CleanupScheduleHistory failed: %v", err)
	}
	if removed != 1 {
		t.Fatalf("Expected 1 entry removed, got %d", removed)
	}

	oldRecent, err := s.WasRecentlyScheduled(ctx, "old-prog", "channel-1", 24*time.Hour)
	if err != nil {
		t.Fatalf("WasRecentlyScheduled failed: %v", err)
	}
	if oldRecent {
		t.Error("Expected old entry to be removed")
	}

	newRecent, err := s.WasRecentlyScheduled(ctx, "new-prog", "channel-1", 24*time.Hour)
	if err != nil {
		t.Fatalf("WasRecentlyScheduled failed: %v", err)
	}
	if !newRecent {
		t.Error("Expected new entry to remain")
	}
}

func TestStore_ExportImportSeriesStates(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer s.Close()

	ctx := context.Background()

	// Create some test states
	states := []scheduler.SeriesState{
		{
			ShowTitle:      "Show A",
			CurrentSeason:  2,
			CurrentEpisode: 5,
			Completed:      false,
			LastAired:      time.Now().Add(-24 * time.Hour),
		},
		{
			ShowTitle:      "Show B",
			CurrentSeason:  1,
			CurrentEpisode: 10,
			Completed:      false,
			LastAired:      time.Now().Add(-12 * time.Hour),
		},
		{
			ShowTitle:      "Show C",
			CurrentSeason:  3,
			CurrentEpisode: 1,
			Completed:      true,
			LastAired:      time.Now().Add(-48 * time.Hour),
		},
	}

	// Import states
	if err := s.ImportSeriesStates(ctx, states); err != nil {
		t.Fatalf("ImportSeriesStates failed: %v", err)
	}

	// Export states
	exported, err := s.ExportAllSeriesStates(ctx)
	if err != nil {
		t.Fatalf("ExportAllSeriesStates failed: %v", err)
	}

	// Verify count
	if len(exported) != len(states) {
		t.Fatalf("Expected %d states, got %d", len(states), len(exported))
	}

	// Verify each state (order should be by show_title due to ORDER BY)
	expectedOrder := []string{"Show A", "Show B", "Show C"}
	for i, expected := range expectedOrder {
		if exported[i].ShowTitle != expected {
			t.Errorf("Expected state %d to be %s, got %s", i, expected, exported[i].ShowTitle)
		}
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
		if original == nil {
			t.Errorf("Exported state %s not found in original states", state.ShowTitle)
			continue
		}

		if state.CurrentSeason != original.CurrentSeason {
			t.Errorf("Season mismatch for %s: expected %d, got %d", state.ShowTitle, original.CurrentSeason, state.CurrentSeason)
		}
		if state.CurrentEpisode != original.CurrentEpisode {
			t.Errorf("Episode mismatch for %s: expected %d, got %d", state.ShowTitle, original.CurrentEpisode, state.CurrentEpisode)
		}
		if state.Completed != original.Completed {
			t.Errorf("Completed mismatch for %s: expected %v, got %v", state.ShowTitle, original.Completed, state.Completed)
		}
	}
}

func TestStore_ResetSeriesState(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	showTitle := "Test Show"

	// Create a state with progress
	state := &scheduler.SeriesState{
		ShowTitle:      showTitle,
		CurrentSeason:  3,
		CurrentEpisode: 15,
		Completed:      true,
		LastAired:      time.Now(),
	}
	if err := s.UpdateSeriesState(ctx, state); err != nil {
		t.Fatalf("UpdateSeriesState failed: %v", err)
	}

	// Reset the state
	if err := s.ResetSeriesState(ctx, showTitle); err != nil {
		t.Fatalf("ResetSeriesState failed: %v", err)
	}

	// Verify reset
	resetState, err := s.GetSeriesState(ctx, showTitle)
	if err != nil {
		t.Fatalf("GetSeriesState failed: %v", err)
	}

	if resetState.CurrentSeason != 1 {
		t.Errorf("Expected season 1 after reset, got %d", resetState.CurrentSeason)
	}
	if resetState.CurrentEpisode != 1 {
		t.Errorf("Expected episode 1 after reset, got %d", resetState.CurrentEpisode)
	}
	if resetState.Completed {
		t.Error("Expected completed to be false after reset")
	}
	if !resetState.LastAired.IsZero() {
		t.Error("Expected LastAired to be zero after reset")
	}
}

func TestStore_ImportSeriesStates_Empty(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer s.Close()

	ctx := context.Background()

	// Import empty slice should not error
	if err := s.ImportSeriesStates(ctx, []scheduler.SeriesState{}); err != nil {
		t.Errorf("ImportSeriesStates with empty slice should not error: %v", err)
	}

	// Import nil should not error
	if err := s.ImportSeriesStates(ctx, nil); err != nil {
		t.Errorf("ImportSeriesStates with nil should not error: %v", err)
	}
}

func TestStore_RecordScheduleHistory_Empty(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer s.Close()

	ctx := context.Background()

	// Recording empty slice should not error
	if err := s.RecordScheduleHistory(ctx, []scheduler.ScheduleHistoryEntry{}); err != nil {
		t.Errorf("RecordScheduleHistory with empty slice should not error: %v", err)
	}

	// Recording nil should not error
	if err := s.RecordScheduleHistory(ctx, nil); err != nil {
		t.Errorf("RecordScheduleHistory with nil should not error: %v", err)
	}
}

func TestStore_WasRecentlyScheduled_InvalidInput(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer s.Close()

	ctx := context.Background()

	// Empty program ID
	_, err = s.WasRecentlyScheduled(ctx, "", "channel-1", 24*time.Hour)
	if err == nil {
		t.Error("Expected error for empty program ID, got nil")
	}

	// Empty channel ID
	_, err = s.WasRecentlyScheduled(ctx, "prog-1", "", 24*time.Hour)
	if err == nil {
		t.Error("Expected error for empty channel ID, got nil")
	}

	// Both empty
	_, err = s.WasRecentlyScheduled(ctx, "", "", 24*time.Hour)
	if err == nil {
		t.Error("Expected error for both empty, got nil")
	}
}

func TestStore_WasRecentlyScheduled_NotFound(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer s.Close()

	ctx := context.Background()

	// Check for program that was never scheduled
	recent, err := s.WasRecentlyScheduled(ctx, "nonexistent", "channel-1", 24*time.Hour)
	if err != nil {
		t.Fatalf("WasRecentlyScheduled failed: %v", err)
	}
	if recent {
		t.Error("Expected program to not be recently scheduled")
	}
}

func TestStore_WasRecentlyScheduled_OutsideWindow(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer s.Close()

	ctx := context.Background()

	// Record an old entry
	oldEntry := scheduler.ScheduleHistoryEntry{
		ProgramID:   "old-prog",
		ChannelID:   "channel-1",
		BlockName:   "Old Block",
		ScheduledAt: time.Now().Add(-72 * time.Hour), // 3 days ago
	}

	if err := s.RecordScheduleHistory(ctx, []scheduler.ScheduleHistoryEntry{oldEntry}); err != nil {
		t.Fatalf("RecordScheduleHistory failed: %v", err)
	}

	// Check with 24 hour window (should not find it)
	recent, err := s.WasRecentlyScheduled(ctx, "old-prog", "channel-1", 24*time.Hour)
	if err != nil {
		t.Fatalf("WasRecentlyScheduled failed: %v", err)
	}
	if recent {
		t.Error("Expected old program to not be within 24h window")
	}

	// Check with 96 hour window (should find it)
	recent96, err := s.WasRecentlyScheduled(ctx, "old-prog", "channel-1", 96*time.Hour)
	if err != nil {
		t.Fatalf("WasRecentlyScheduled failed: %v", err)
	}
	if !recent96 {
		t.Error("Expected old program to be within 96h window")
	}
}

func TestStore_GetSeriesState_MultipleShows(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
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
		state := &scheduler.SeriesState{
			ShowTitle:      show.title,
			CurrentSeason:  show.season,
			CurrentEpisode: show.episode,
			Completed:      false,
			LastAired:      time.Now(),
		}
		if err := s.UpdateSeriesState(ctx, state); err != nil {
			t.Fatalf("UpdateSeriesState failed for %s: %v", show.title, err)
		}
	}

	// Verify each show has correct state
	for _, show := range shows {
		state, err := s.GetSeriesState(ctx, show.title)
		if err != nil {
			t.Fatalf("GetSeriesState failed for %s: %v", show.title, err)
		}
		if state.CurrentSeason != show.season || state.CurrentEpisode != show.episode {
			t.Errorf("State mismatch for %s: expected S%02dE%02d, got S%02dE%02d",
				show.title, show.season, show.episode, state.CurrentSeason, state.CurrentEpisode)
		}
	}
}

func TestStore_ResetSeriesState_NonExistent(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer s.Close()

	ctx := context.Background()

	// Reset a show that doesn't exist should not error
	if err := s.ResetSeriesState(ctx, "NonExistent Show"); err != nil {
		t.Errorf("ResetSeriesState for non-existent show should not error: %v", err)
	}

	// Verify the state is now default
	state, err := s.GetSeriesState(ctx, "NonExistent Show")
	if err != nil {
		t.Fatalf("GetSeriesState failed: %v", err)
	}
	if state.CurrentSeason != 1 || state.CurrentEpisode != 1 {
		t.Errorf("Expected default state S01E01, got S%02dE%02d", state.CurrentSeason, state.CurrentEpisode)
	}
}

func TestStore_CleanupScheduleHistory_EmptyDatabase(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer s.Close()

	ctx := context.Background()

	// Cleanup on empty database should not error and return 0
	removed, err := s.CleanupScheduleHistory(ctx, 24*time.Hour)
	if err != nil {
		t.Fatalf("CleanupScheduleHistory failed: %v", err)
	}
	if removed != 0 {
		t.Errorf("Expected 0 entries removed from empty database, got %d", removed)
	}
}

func TestStore_ExportAllSeriesStates_Empty(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer s.Close()

	ctx := context.Background()

	// Export from empty database should return empty slice
	states, err := s.ExportAllSeriesStates(ctx)
	if err != nil {
		t.Fatalf("ExportAllSeriesStates failed: %v", err)
	}
	if len(states) != 0 {
		t.Errorf("Expected 0 states from empty database, got %d", len(states))
	}
}

func TestStore_Backup(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer s.Close()

	ctx := context.Background()

	// Add some data
	state := &scheduler.SeriesState{
		ShowTitle:      "Test Show",
		CurrentSeason:  2,
		CurrentEpisode: 5,
		Completed:      false,
		LastAired:      time.Now(),
	}
	if err := s.UpdateSeriesState(ctx, state); err != nil {
		t.Fatalf("UpdateSeriesState failed: %v", err)
	}

	// Create backup to a temporary file
	backupPath := t.TempDir() + "/backup.db"
	if err := s.Backup(ctx, backupPath); err != nil {
		t.Fatalf("Backup failed: %v", err)
	}

	// Open the backup and verify data
	backupStore, err := New(backupPath)
	if err != nil {
		t.Fatalf("Failed to open backup: %v", err)
	}
	defer backupStore.Close()

	// Verify the data was backed up
	restoredState, err := backupStore.GetSeriesState(ctx, "Test Show")
	if err != nil {
		t.Fatalf("GetSeriesState from backup failed: %v", err)
	}
	if restoredState.CurrentSeason != 2 || restoredState.CurrentEpisode != 5 {
		t.Errorf("Backup data mismatch: expected S02E05, got S%02dE%02d",
			restoredState.CurrentSeason, restoredState.CurrentEpisode)
	}
}
