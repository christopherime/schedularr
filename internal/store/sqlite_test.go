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
