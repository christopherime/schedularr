package scheduler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/geekxflood/schedularr/internal/tunarr"
)

func TestSlotsOverlap(t *testing.T) {
	tests := []struct {
		name     string
		slot1    ScheduledSlot
		slot2    ScheduledSlot
		expected bool
	}{
		{
			name: "overlapping slots",
			slot1: ScheduledSlot{
				StartTime: time.Date(2026, 1, 11, 10, 0, 0, 0, time.UTC),
				EndTime:   time.Date(2026, 1, 11, 12, 0, 0, 0, time.UTC),
			},
			slot2: ScheduledSlot{
				StartTime: time.Date(2026, 1, 11, 11, 0, 0, 0, time.UTC),
				EndTime:   time.Date(2026, 1, 11, 13, 0, 0, 0, time.UTC),
			},
			expected: true,
		},
		{
			name: "non-overlapping slots",
			slot1: ScheduledSlot{
				StartTime: time.Date(2026, 1, 11, 10, 0, 0, 0, time.UTC),
				EndTime:   time.Date(2026, 1, 11, 11, 0, 0, 0, time.UTC),
			},
			slot2: ScheduledSlot{
				StartTime: time.Date(2026, 1, 11, 11, 0, 0, 0, time.UTC),
				EndTime:   time.Date(2026, 1, 11, 12, 0, 0, 0, time.UTC),
			},
			expected: false,
		},
		{
			name: "slot1 contains slot2",
			slot1: ScheduledSlot{
				StartTime: time.Date(2026, 1, 11, 10, 0, 0, 0, time.UTC),
				EndTime:   time.Date(2026, 1, 11, 14, 0, 0, 0, time.UTC),
			},
			slot2: ScheduledSlot{
				StartTime: time.Date(2026, 1, 11, 11, 0, 0, 0, time.UTC),
				EndTime:   time.Date(2026, 1, 11, 13, 0, 0, 0, time.UTC),
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := slotsOverlap(tt.slot1, tt.slot2)
			if result != tt.expected {
				t.Errorf("slotsOverlap() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestResolveConflicts(t *testing.T) {
	block1 := Block{Name: "Low Priority", Priority: 10}
	block2 := Block{Name: "High Priority", Priority: 20}

	tests := []struct {
		name     string
		slots    []ScheduledSlot
		expected int
	}{
		{
			name:     "no conflicts",
			slots:    []ScheduledSlot{},
			expected: 0,
		},
		{
			name: "two non-overlapping slots",
			slots: []ScheduledSlot{
				{
					StartTime: time.Date(2026, 1, 11, 10, 0, 0, 0, time.UTC),
					EndTime:   time.Date(2026, 1, 11, 11, 0, 0, 0, time.UTC),
					Block:     block1,
				},
				{
					StartTime: time.Date(2026, 1, 11, 11, 0, 0, 0, time.UTC),
					EndTime:   time.Date(2026, 1, 11, 12, 0, 0, 0, time.UTC),
					Block:     block2,
				},
			},
			expected: 2,
		},
		{
			name: "two overlapping slots - high priority wins",
			slots: []ScheduledSlot{
				{
					StartTime: time.Date(2026, 1, 11, 10, 0, 0, 0, time.UTC),
					EndTime:   time.Date(2026, 1, 11, 12, 0, 0, 0, time.UTC),
					Block:     block1,
				},
				{
					StartTime: time.Date(2026, 1, 11, 11, 0, 0, 0, time.UTC),
					EndTime:   time.Date(2026, 1, 11, 13, 0, 0, 0, time.UTC),
					Block:     block2,
				},
			},
			expected: 1, // Only high priority should remain
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a minimal engine to call the method
			engine := &Engine{logger: slog.Default()}
			resolved := engine.resolveConflicts(tt.slots)
			if len(resolved) != tt.expected {
				t.Errorf("resolveConflicts() returned %d slots, expected %d", len(resolved), tt.expected)
			}

			// If there were conflicts, verify high priority won
			if tt.name == "two overlapping slots - high priority wins" && len(resolved) > 0 {
				if resolved[0].Block.Priority != 20 {
					t.Errorf("Expected high priority block to win, got priority %d", resolved[0].Block.Priority)
				}
			}
		})
	}
}

func TestPlanBlock_WithoutFiller(t *testing.T) {
	client := &tunarr.Client{}
	engine := NewEngine(client, []Block{}, NewMockStateStore(), slog.Default(), time.UTC)

	block := Block{
		Name:     "Test Block",
		Duration: 60, // 60 minutes
		Filter: Filter{
			Genres: []string{"Comedy"},
		},
	}

	availablePrograms := []tunarr.Program{
		{
			ID:       "prog1",
			Title:    "Show A",
			Duration: 1800000, // 30 minutes
			Genres:   []string{"Comedy"},
			Type:     "episode",
		},
		{
			ID:       "prog2",
			Title:    "Show B",
			Duration: 1800000, // 30 minutes
			Genres:   []string{"Comedy"},
			Type:     "episode",
		},
	}

	playlist, err := engine.PlanBlock(block, availablePrograms)
	if err != nil {
		t.Fatalf("PlanBlock returned error: %v", err)
	}

	if len(playlist) != 2 {
		t.Errorf("Expected 2 programs in playlist, got %d", len(playlist))
	}

	// Check total duration
	var totalDuration int64
	for _, p := range playlist {
		totalDuration += p.Duration
	}

	// Should be 60 minutes (3600000 ms)
	if totalDuration != 3600000 {
		t.Errorf("Expected total duration 3600000 ms, got %d ms", totalDuration)
	}
}

func TestPlanBlock_NoMatchingContent(t *testing.T) {
	client := &tunarr.Client{}
	engine := NewEngine(client, []Block{}, NewMockStateStore(), slog.Default(), time.UTC)

	block := Block{
		Name:     "Test Block",
		Duration: 60,
		Filter: Filter{
			Genres: []string{"SciFi"},
		},
	}

	availablePrograms := []tunarr.Program{
		{
			ID:       "prog1",
			Title:    "Show A",
			Duration: 1800000,
			Genres:   []string{"Comedy"},
			Type:     "episode",
		},
	}

	_, err := engine.PlanBlock(block, availablePrograms)
	if err == nil {
		t.Error("Expected error when no content matches filter, got nil")
	}
}

func TestPlanBlock_UsesStoreHistory(t *testing.T) {
	client := &tunarr.Client{}
	store := NewMockStateStore()
	store.History = []ScheduleHistoryEntry{
		{
			ProgramID:   "prog-1",
			ChannelID:   "channel-1",
			BlockName:   "Recent Block",
			ScheduledAt: time.Now(),
		},
	}
	engine := NewEngine(client, []Block{}, store, slog.Default(), time.UTC)

	block := Block{
		Name:      "History Block",
		Duration:  30,
		ChannelID: "channel-1",
		Filter:    Filter{},
	}

	availablePrograms := []tunarr.Program{
		{ID: "prog-1", Title: "Recent", Duration: 1800000, Type: "movie"},
		{ID: "prog-2", Title: "Fresh", Duration: 1800000, Type: "movie"},
	}

	playlist, err := engine.PlanBlock(block, availablePrograms)
	if err != nil {
		t.Fatalf("PlanBlock returned error: %v", err)
	}

	if len(playlist) != 1 {
		t.Fatalf("Expected 1 program, got %d", len(playlist))
	}
	if playlist[0].ID != "prog-2" {
		t.Errorf("Expected prog-2 after filtering, got %s", playlist[0].ID)
	}
}

func TestCommit_CleansUpScheduleHistory(t *testing.T) {
	client := &tunarr.Client{}
	store := NewMockStateStore()
	store.History = []ScheduleHistoryEntry{
		{
			ProgramID:   "old-prog",
			ChannelID:   "channel-1",
			BlockName:   "Old Block",
			ScheduledAt: time.Now().Add(-48 * time.Hour),
		},
		{
			ProgramID:   "new-prog",
			ChannelID:   "channel-1",
			BlockName:   "New Block",
			ScheduledAt: time.Now(),
		},
	}

	engine := NewEngineWithHistory(client, []Block{}, 24*time.Hour, store, slog.Default(), time.UTC)

	if err := engine.Commit(); err != nil {
		t.Fatalf("Commit returned error: %v", err)
	}

	if len(store.History) != 1 {
		t.Fatalf("expected 1 history entry after cleanup, got %d", len(store.History))
	}
	if store.History[0].ProgramID != "new-prog" {
		t.Errorf("expected new-prog to remain, got %s", store.History[0].ProgramID)
	}
}

func TestPlanBlock_Series(t *testing.T) {
	client := &tunarr.Client{}
	store := NewMockStateStore()
	engine := NewEngine(client, []Block{}, store, slog.Default(), time.UTC)

	block := Block{
		Name:     "Series Block",
		Type:     BlockTypeSeries,
		Duration: 120,
		Series: []SeriesConfig{
			{
				ShowTitle:        "Show A",
				EpisodesPerBlock: 2,
			},
		},
		Fallback: SeriesFallback{
			Mode: "none", // Disable fallback to test series logic only
		},
	}

	availablePrograms := []tunarr.Program{
		{ID: "p1", Title: "Ep 1", ShowTitle: "Show A", Season: 1, Episode: 1, Duration: 1800000, Type: "episode"},
		{ID: "p2", Title: "Ep 2", ShowTitle: "Show A", Season: 1, Episode: 2, Duration: 1800000, Type: "episode"},
		{ID: "p3", Title: "Ep 3", ShowTitle: "Show A", Season: 1, Episode: 3, Duration: 1800000, Type: "episode"},
	}

	playlist, err := engine.PlanBlock(block, availablePrograms)
	if err != nil {
		t.Fatalf("PlanBlock returned error: %v", err)
	}

	if len(playlist) != 2 {
		t.Errorf("Expected 2 episodes, got %d", len(playlist))
	}

	if playlist[0].Episode != 1 || playlist[1].Episode != 2 {
		t.Errorf("Expected Ep 1 and Ep 2, got %v", playlist)
	}

	// Verify pending state
	state, ok := engine.pendingStates["Show A"]
	if !ok {
		t.Fatal("Expected pending state for Show A")
	}
	if state.CurrentEpisode != 3 {
		t.Errorf("Expected next episode to be 3, got %d", state.CurrentEpisode)
	}
}

func TestPlanBlock_SeriesMarksCompleteWhenMissing(t *testing.T) {
	client := &tunarr.Client{}
	store := NewMockStateStore()
	engine := NewEngine(client, []Block{}, store, slog.Default(), time.UTC)

	block := Block{
		Name:     "Series Block",
		Type:     BlockTypeSeries,
		Duration: 30,
		Series: []SeriesConfig{
			{
				ShowTitle:        "Missing Show",
				EpisodesPerBlock: 1,
			},
		},
	}

	availablePrograms := []tunarr.Program{
		{ID: "p1", Title: "Other Show", ShowTitle: "Other Show", Season: 1, Episode: 1, Duration: 1800000, Type: "episode"},
	}

	playlist, err := engine.PlanBlock(block, availablePrograms)
	if err != nil {
		t.Fatalf("PlanBlock returned error: %v", err)
	}
	if len(playlist) != 0 {
		t.Fatalf("Expected empty playlist, got %d items", len(playlist))
	}

	state, ok := engine.pendingStates["Missing Show"]
	if !ok {
		t.Fatal("Expected pending state for Missing Show")
	}
	if !state.Completed {
		t.Error("Expected series to be marked completed")
	}
}

func TestSeriesCompletion_Restart(t *testing.T) {
	client := &tunarr.Client{}
	store := NewMockStateStore()
	engine := NewEngine(client, []Block{}, store, slog.Default(), time.UTC)

	// Set up initial state at end of series
	store.States["Test Show"] = &SeriesState{
		ShowTitle:      "Test Show",
		CurrentSeason:  2,
		CurrentEpisode: 10,
		Completed:      false,
		RunCount:       0,
	}

	block := Block{
		Name:     "Series Block",
		Type:     BlockTypeSeries,
		Duration: 30,
		Series: []SeriesConfig{
			{
				ShowTitle:        "Test Show",
				EpisodesPerBlock: 1,
				OnComplete:       CompletionActionRestart,
			},
		},
	}

	// No more episodes available - should trigger completion
	availablePrograms := []tunarr.Program{
		{ID: "p1", Title: "Test Show S01E01", ShowTitle: "Test Show", Season: 1, Episode: 1, Duration: 1800000, Type: "episode"},
	}

	_, err := engine.PlanBlock(block, availablePrograms)
	if err != nil {
		t.Fatalf("PlanBlock returned error: %v", err)
	}

	state, ok := engine.pendingStates["Test Show"]
	if !ok {
		t.Fatal("Expected pending state for Test Show")
	}

	if state.Completed {
		t.Error("Expected series to be restarted, not marked completed")
	}
	if state.CurrentSeason != 1 {
		t.Errorf("Expected season to be reset to 1, got %d", state.CurrentSeason)
	}
	if state.CurrentEpisode != 1 {
		t.Errorf("Expected episode to be reset to 1, got %d", state.CurrentEpisode)
	}
	if state.RunCount != 1 {
		t.Errorf("Expected run count to be 1, got %d", state.RunCount)
	}
}

func TestSeriesCompletion_Disable(t *testing.T) {
	client := &tunarr.Client{}
	store := NewMockStateStore()
	engine := NewEngine(client, []Block{}, store, slog.Default(), time.UTC)

	store.States["Test Show"] = &SeriesState{
		ShowTitle:      "Test Show",
		CurrentSeason:  1,
		CurrentEpisode: 10,
		Completed:      false,
	}

	block := Block{
		Name:     "Series Block",
		Type:     BlockTypeSeries,
		Duration: 30,
		Series: []SeriesConfig{
			{
				ShowTitle:        "Test Show",
				EpisodesPerBlock: 1,
				OnComplete:       CompletionActionDisable,
			},
		},
	}

	availablePrograms := []tunarr.Program{}

	_, err := engine.PlanBlock(block, availablePrograms)
	if err != nil {
		t.Fatalf("PlanBlock returned error: %v", err)
	}

	state, ok := engine.pendingStates["Test Show"]
	if !ok {
		t.Fatal("Expected pending state for Test Show")
	}

	if !state.Completed {
		t.Error("Expected series to be marked completed")
	}
	if !state.Disabled {
		t.Error("Expected series to be disabled")
	}
}

func TestSeriesCompletion_MaxRuns(t *testing.T) {
	client := &tunarr.Client{}
	store := NewMockStateStore()
	engine := NewEngine(client, []Block{}, store, slog.Default(), time.UTC)

	// Series has already run twice
	store.States["Test Show"] = &SeriesState{
		ShowTitle:      "Test Show",
		CurrentSeason:  1,
		CurrentEpisode: 10,
		Completed:      false,
		RunCount:       2,
	}

	block := Block{
		Name:     "Series Block",
		Type:     BlockTypeSeries,
		Duration: 30,
		Series: []SeriesConfig{
			{
				ShowTitle:        "Test Show",
				EpisodesPerBlock: 1,
				OnComplete:       CompletionActionRestart,
				MaxRuns:          3,
			},
		},
	}

	availablePrograms := []tunarr.Program{}

	_, err := engine.PlanBlock(block, availablePrograms)
	if err != nil {
		t.Fatalf("PlanBlock returned error: %v", err)
	}

	state, ok := engine.pendingStates["Test Show"]
	if !ok {
		t.Fatal("Expected pending state for Test Show")
	}

	if state.RunCount != 3 {
		t.Errorf("Expected run count to be 3, got %d", state.RunCount)
	}
	if !state.Disabled {
		t.Error("Expected series to be disabled after reaching max runs")
	}
}

func TestSeriesEpisodeSkipping(t *testing.T) {
	client := &tunarr.Client{}
	store := NewMockStateStore()
	engine := NewEngine(client, []Block{}, store, slog.Default(), time.UTC)

	store.States["Test Show"] = &SeriesState{
		ShowTitle:      "Test Show",
		CurrentSeason:  1,
		CurrentEpisode: 1,
		Completed:      false,
	}

	block := Block{
		Name:     "Series Block",
		Type:     BlockTypeSeries,
		Duration: 90, // 90 minutes
		Series: []SeriesConfig{
			{
				ShowTitle:        "Test Show",
				EpisodesPerBlock: 3,
				SkipEpisodes:     []string{"S01E02", "S01E04"},
			},
		},
	}

	availablePrograms := []tunarr.Program{
		{ID: "p1", Title: "Test Show S01E01", ShowTitle: "Test Show", Season: 1, Episode: 1, Duration: 1800000, Type: "episode"},
		{ID: "p2", Title: "Test Show S01E02", ShowTitle: "Test Show", Season: 1, Episode: 2, Duration: 1800000, Type: "episode"},
		{ID: "p3", Title: "Test Show S01E03", ShowTitle: "Test Show", Season: 1, Episode: 3, Duration: 1800000, Type: "episode"},
		{ID: "p4", Title: "Test Show S01E04", ShowTitle: "Test Show", Season: 1, Episode: 4, Duration: 1800000, Type: "episode"},
		{ID: "p5", Title: "Test Show S01E05", ShowTitle: "Test Show", Season: 1, Episode: 5, Duration: 1800000, Type: "episode"},
	}

	playlist, err := engine.PlanBlock(block, availablePrograms)
	if err != nil {
		t.Fatalf("PlanBlock returned error: %v", err)
	}

	// Should get E01, skip E02, get E03, get E05 (skipping E02 and E04)
	if len(playlist) != 3 {
		t.Fatalf("Expected 3 episodes (E01, E03, E05 - skipping E02 and E04), got %d", len(playlist))
	}

	if playlist[0].Episode != 1 {
		t.Errorf("Expected first episode to be E01, got E%02d", playlist[0].Episode)
	}
	if playlist[1].Episode != 3 {
		t.Errorf("Expected second episode to be E03 (skipped E02), got E%02d", playlist[1].Episode)
	}
	if playlist[2].Episode != 5 {
		t.Errorf("Expected third episode to be E05 (skipped E04), got E%02d", playlist[2].Episode)
	}

	state, ok := engine.pendingStates["Test Show"]
	if !ok {
		t.Fatal("Expected pending state for Test Show")
	}

	// Should be at E06 (next episode after E05)
	if state.CurrentEpisode != 6 {
		t.Errorf("Expected current episode to be 6, got %d", state.CurrentEpisode)
	}
}

func TestSeriesSkipDisabled(t *testing.T) {
	client := &tunarr.Client{}
	store := NewMockStateStore()
	engine := NewEngine(client, []Block{}, store, slog.Default(), time.UTC)

	// Series is disabled
	store.States["Test Show"] = &SeriesState{
		ShowTitle:      "Test Show",
		CurrentSeason:  1,
		CurrentEpisode: 1,
		Disabled:       true,
	}

	block := Block{
		Name:     "Series Block",
		Type:     BlockTypeSeries,
		Duration: 30,
		Series: []SeriesConfig{
			{
				ShowTitle:        "Test Show",
				EpisodesPerBlock: 1,
			},
		},
	}

	availablePrograms := []tunarr.Program{
		{ID: "p1", Title: "Test Show S01E01", ShowTitle: "Test Show", Season: 1, Episode: 1, Duration: 1800000, Type: "episode"},
	}

	playlist, err := engine.PlanBlock(block, availablePrograms)
	if err != nil {
		t.Fatalf("PlanBlock returned error: %v", err)
	}

	// Should get empty playlist because series is disabled
	if len(playlist) != 0 {
		t.Errorf("Expected empty playlist for disabled series, got %d items", len(playlist))
	}
}

func TestGenerateForTimeRange(t *testing.T) {
	client := &tunarr.Client{}
	store := NewMockStateStore()

	blocks := []Block{
		{
			Name:      "Morning Block",
			Type:      BlockTypeFilter,
			Cron:      "0 9 * * *", // Daily at 9 AM
			Duration:  60,          // 1 hour
			ChannelID: "channel-1",
			Priority:  10,
			Filter: Filter{
				Genres: []string{"Comedy"},
			},
		},
		{
			Name:      "Evening Block",
			Type:      BlockTypeFilter,
			Cron:      "0 20 * * *", // Daily at 8 PM
			Duration:  120,          // 2 hours
			ChannelID: "channel-1",
			Priority:  10,
			Filter: Filter{
				Genres: []string{"Drama"},
			},
		},
	}

	engine := NewEngine(client, blocks, store, slog.Default(), time.UTC)

	availablePrograms := []tunarr.Program{
		{ID: "p1", Title: "Comedy Show", Genres: []string{"Comedy"}, Duration: 1800000, Type: "episode"},
		{ID: "p2", Title: "Drama Show", Genres: []string{"Drama"}, Duration: 3600000, Type: "episode"},
	}

	// Test a 24-hour period
	start := time.Date(2026, 1, 12, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)

	schedule, err := engine.GenerateForTimeRange(start, end, availablePrograms)
	if err != nil {
		t.Fatalf("GenerateForTimeRange returned error: %v", err)
	}

	// Should have schedule for channel-1
	programs, ok := schedule["channel-1"]
	if !ok {
		t.Fatal("Expected schedule for channel-1")
	}

	// Should have programs from both blocks (morning and evening)
	if len(programs) == 0 {
		t.Error("Expected programs in schedule")
	}
}

func TestGenerateForTimeRange_InvalidCron(t *testing.T) {
	client := &tunarr.Client{}
	store := NewMockStateStore()

	blocks := []Block{
		{
			Name:      "Invalid Block",
			Type:      BlockTypeFilter,
			Cron:      "invalid cron",
			Duration:  60,
			ChannelID: "channel-1",
			Priority:  10,
		},
	}

	engine := NewEngine(client, blocks, store, slog.Default(), time.UTC)

	start := time.Date(2026, 1, 12, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)

	_, err := engine.GenerateForTimeRange(start, end, []tunarr.Program{})
	if err == nil {
		t.Error("Expected error for invalid cron expression")
	}
}

func TestGenerateForTimeRange_ConflictResolution(t *testing.T) {
	client := &tunarr.Client{}
	store := NewMockStateStore()

	blocks := []Block{
		{
			Name:      "Low Priority Block",
			Type:      BlockTypeFilter,
			Cron:      "0 9 * * *",
			Duration:  120, // 2 hours
			ChannelID: "channel-1",
			Priority:  5,
			Filter: Filter{
				Genres: []string{"Comedy"},
			},
		},
		{
			Name:      "High Priority Block",
			Type:      BlockTypeFilter,
			Cron:      "30 9 * * *", // Overlaps with low priority
			Duration:  60,           // 1 hour
			ChannelID: "channel-1",
			Priority:  10,
			Filter: Filter{
				Genres: []string{"Drama"},
			},
		},
	}

	engine := NewEngine(client, blocks, store, slog.Default(), time.UTC)

	availablePrograms := []tunarr.Program{
		{ID: "p1", Title: "Comedy Show", Genres: []string{"Comedy"}, Duration: 1800000, Type: "episode"},
		{ID: "p2", Title: "Drama Show", Genres: []string{"Drama"}, Duration: 1800000, Type: "episode"},
	}

	start := time.Date(2026, 1, 12, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)

	schedule, err := engine.GenerateForTimeRange(start, end, availablePrograms)
	if err != nil {
		t.Fatalf("GenerateForTimeRange returned error: %v", err)
	}

	programs, ok := schedule["channel-1"]
	if !ok {
		t.Fatal("Expected schedule for channel-1")
	}

	// Should have programs from both blocks, with high priority winning conflicts
	if len(programs) == 0 {
		t.Error("Expected programs in schedule")
	}
}

func TestCommit(t *testing.T) {
	client := &tunarr.Client{}
	store := NewMockStateStore()
	engine := NewEngine(client, []Block{}, store, slog.Default(), time.UTC)

	// Add some pending states
	engine.pendingStates["Show1"] = &SeriesState{
		ShowTitle:      "Show1",
		CurrentSeason:  1,
		CurrentEpisode: 5,
	}
	engine.pendingStates["Show2"] = &SeriesState{
		ShowTitle:      "Show2",
		CurrentSeason:  2,
		CurrentEpisode: 3,
	}

	err := engine.Commit()
	if err != nil {
		t.Fatalf("Commit returned error: %v", err)
	}

	// Verify states were saved to store
	if len(store.States) != 2 {
		t.Errorf("Expected 2 states in store, got %d", len(store.States))
	}

	state1, ok := store.States["Show1"]
	if !ok {
		t.Error("Expected Show1 in store")
	} else if state1.CurrentEpisode != 5 {
		t.Errorf("Expected Show1 episode 5, got %d", state1.CurrentEpisode)
	}

	// Verify pending states were cleared
	if len(engine.pendingStates) != 0 {
		t.Errorf("Expected pending states to be cleared, got %d", len(engine.pendingStates))
	}
}

func TestFilterByHistory(t *testing.T) {
	client := &tunarr.Client{}
	store := NewMockStateStore()

	// Add some history
	store.History = []ScheduleHistoryEntry{
		{
			ProgramID:   "p1",
			ChannelID:   "channel-1",
			ScheduledAt: time.Now().Add(-3 * 24 * time.Hour), // 3 days ago
			BlockName:   "Test Block",
		},
		{
			ProgramID:   "p2",
			ChannelID:   "channel-1",
			ScheduledAt: time.Now().Add(-10 * 24 * time.Hour), // 10 days ago
			BlockName:   "Test Block",
		},
	}

	engine := NewEngineWithHistory(client, []Block{}, 7*24*time.Hour, store, slog.Default(), time.UTC)

	availablePrograms := []tunarr.Program{
		{ID: "p1", Title: "Recent Show", Duration: 1800000, Type: "episode"},
		{ID: "p2", Title: "Old Show", Duration: 1800000, Type: "episode"},
		{ID: "p3", Title: "New Show", Duration: 1800000, Type: "episode"},
	}

	filtered := engine.filterByHistory(availablePrograms, "channel-1")

	// p1 should be filtered out (aired 3 days ago, within 7 day window)
	// p2 should be included (aired 10 days ago, outside 7 day window)
	// p3 should be included (never aired)
	if len(filtered) != 2 {
		t.Errorf("Expected 2 programs after filtering, got %d", len(filtered))
	}

	// Verify p1 was filtered out
	for _, p := range filtered {
		if p.ID == "p1" {
			t.Error("Expected p1 to be filtered out")
		}
	}
}

func TestGetFiller_Success(t *testing.T) {
	// Create a test server that returns filler content
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/filler-lists/filler-1/content" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		fillerContent := []tunarr.Program{
			{ID: "f1", Title: "Filler 1", Duration: 300000, Type: "track"},  // 5 min
			{ID: "f2", Title: "Filler 2", Duration: 600000, Type: "track"},  // 10 min
			{ID: "f3", Title: "Filler 3", Duration: 900000, Type: "track"},  // 15 min
			{ID: "f4", Title: "Filler 4", Duration: 1200000, Type: "track"}, // 20 min
		}
		json.NewEncoder(w).Encode(fillerContent)
	}))
	defer server.Close()

	client := tunarr.NewClient(tunarr.Config{URL: server.URL})
	store := NewMockStateStore()
	engine := NewEngine(client, []Block{}, store, slog.Default(), time.UTC)

	block := Block{
		Filler: FillerConfig{
			FillerListID: "filler-1",
		},
	}

	// Request 30 minutes of filler (1800000 ms)
	filler, err := engine.getFiller(block, 1800000)
	if err != nil {
		t.Fatalf("getFiller failed: %v", err)
	}

	if len(filler) == 0 {
		t.Error("Expected filler programs, got none")
	}

	totalDuration := int64(0)
	for _, f := range filler {
		totalDuration += f.Duration
	}

	if totalDuration > 1800000 {
		t.Errorf("Filler duration %d exceeds requested %d", totalDuration, 1800000)
	}
}

func TestGetFiller_NoFillerListID(t *testing.T) {
	client := tunarr.NewClient(tunarr.Config{URL: "http://localhost:8000"})
	store := NewMockStateStore()
	engine := NewEngine(client, []Block{}, store, slog.Default(), time.UTC)

	block := Block{
		Filler: FillerConfig{
			FillerListID: "",
		},
	}

	_, err := engine.getFiller(block, 1800000)
	if err == nil {
		t.Error("Expected error for empty filler list ID, got nil")
	}
	if err != nil && !strings.Contains(err.Error(), "no filler list ID") {
		t.Errorf("Expected error about no filler list ID, got: %v", err)
	}
}

func TestGetFiller_EmptyFillerList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return empty array
		json.NewEncoder(w).Encode([]tunarr.Program{})
	}))
	defer server.Close()

	client := tunarr.NewClient(tunarr.Config{URL: server.URL})
	store := NewMockStateStore()
	engine := NewEngine(client, []Block{}, store, slog.Default(), time.UTC)

	block := Block{
		Filler: FillerConfig{
			FillerListID: "empty-filler",
		},
	}

	_, err := engine.getFiller(block, 1800000)
	if err == nil {
		t.Error("Expected error for empty filler list, got nil")
	}
	if err != nil && !strings.Contains(err.Error(), "is empty") {
		t.Errorf("Expected error about empty filler list, got: %v", err)
	}
}

func TestGetFiller_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := tunarr.NewClient(tunarr.Config{URL: server.URL})
	store := NewMockStateStore()
	engine := NewEngine(client, []Block{}, store, slog.Default(), time.UTC)

	block := Block{
		Filler: FillerConfig{
			FillerListID: "filler-1",
		},
	}

	_, err := engine.getFiller(block, 1800000)
	if err == nil {
		t.Error("Expected error from API, got nil")
	}
	if err != nil && !strings.Contains(err.Error(), "failed to fetch filler") {
		t.Errorf("Expected API error, got: %v", err)
	}
}

func TestGetFiller_MaxFillerTime(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fillerContent := []tunarr.Program{
			{ID: "f1", Title: "Filler 1", Duration: 300000, Type: "track"}, // 5 min
			{ID: "f2", Title: "Filler 2", Duration: 300000, Type: "track"}, // 5 min
			{ID: "f3", Title: "Filler 3", Duration: 300000, Type: "track"}, // 5 min
			{ID: "f4", Title: "Filler 4", Duration: 300000, Type: "track"}, // 5 min
			{ID: "f5", Title: "Filler 5", Duration: 300000, Type: "track"}, // 5 min
		}
		json.NewEncoder(w).Encode(fillerContent)
	}))
	defer server.Close()

	client := tunarr.NewClient(tunarr.Config{URL: server.URL})
	store := NewMockStateStore()
	engine := NewEngine(client, []Block{}, store, slog.Default(), time.UTC)

	block := Block{
		Filler: FillerConfig{
			FillerListID:  "filler-1",
			MaxFillerTime: 10, // 10 minutes max
		},
	}

	// Request 30 minutes, but max filler is 10 minutes
	filler, err := engine.getFiller(block, 1800000)
	if err != nil {
		t.Fatalf("getFiller failed: %v", err)
	}

	totalDuration := int64(0)
	for _, f := range filler {
		totalDuration += f.Duration
	}

	// Should not exceed 10 minutes (600000 ms)
	if totalDuration > 600000 {
		t.Errorf("Filler duration %d exceeds max filler time %d", totalDuration, 600000)
	}
}

func TestApplyBlockFiller_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fillerContent := []tunarr.Program{
			{ID: "f1", Title: "Filler 1", Duration: 300000, Type: "track"}, // 5 min
			{ID: "f2", Title: "Filler 2", Duration: 300000, Type: "track"}, // 5 min
		}
		json.NewEncoder(w).Encode(fillerContent)
	}))
	defer server.Close()

	client := tunarr.NewClient(tunarr.Config{URL: server.URL})
	store := NewMockStateStore()
	engine := NewEngine(client, []Block{}, store, slog.Default(), time.UTC)

	block := Block{
		Filler: FillerConfig{
			Enabled:      true,
			FillerListID: "filler-1",
			MinGapTime:   5, // 5 minutes minimum gap
		},
	}

	initialPlaylist := []tunarr.Program{
		{ID: "p1", Title: "Show 1", Duration: 1800000, Type: "episode"}, // 30 min
	}
	currentDuration := int64(1800000)
	targetDuration := int64(2400000) // 40 minutes target, 10 minute gap

	playlist, finalDuration := engine.applyBlockFiller(block, initialPlaylist, currentDuration, targetDuration)

	if len(playlist) <= len(initialPlaylist) {
		t.Error("Expected filler to be added to playlist")
	}

	if finalDuration <= currentDuration {
		t.Error("Expected duration to increase after adding filler")
	}

	if finalDuration > targetDuration {
		t.Errorf("Filler exceeded target duration: %d > %d", finalDuration, targetDuration)
	}
}

func TestApplyBlockFiller_GapTooSmall(t *testing.T) {
	client := tunarr.NewClient(tunarr.Config{URL: "http://localhost:8000"})
	store := NewMockStateStore()
	engine := NewEngine(client, []Block{}, store, slog.Default(), time.UTC)

	block := Block{
		Filler: FillerConfig{
			Enabled:    true,
			MinGapTime: 10, // 10 minutes minimum
		},
	}

	initialPlaylist := []tunarr.Program{
		{ID: "p1", Title: "Show 1", Duration: 1800000, Type: "episode"}, // 30 min
	}
	currentDuration := int64(1800000)
	targetDuration := int64(2100000) // 35 minutes target, 5 minute gap (less than minimum)

	playlist, finalDuration := engine.applyBlockFiller(block, initialPlaylist, currentDuration, targetDuration)

	// Should not add filler because gap is too small
	if len(playlist) != len(initialPlaylist) {
		t.Error("Expected no filler to be added (gap too small)")
	}

	if finalDuration != currentDuration {
		t.Error("Expected duration to remain unchanged")
	}
}

func TestApplyBlockFiller_FillerDisabled(t *testing.T) {
	client := tunarr.NewClient(tunarr.Config{URL: "http://localhost:8000"})
	store := NewMockStateStore()
	engine := NewEngine(client, []Block{}, store, slog.Default(), time.UTC)

	block := Block{
		Filler: FillerConfig{
			Enabled: false,
		},
	}

	initialPlaylist := []tunarr.Program{
		{ID: "p1", Title: "Show 1", Duration: 1800000, Type: "episode"},
	}
	currentDuration := int64(1800000)
	targetDuration := int64(3600000)

	playlist, finalDuration := engine.applyBlockFiller(block, initialPlaylist, currentDuration, targetDuration)

	if len(playlist) != len(initialPlaylist) {
		t.Error("Expected no filler to be added (filler disabled)")
	}

	if finalDuration != currentDuration {
		t.Error("Expected duration to remain unchanged")
	}
}

func TestApplySeriesFallback_FillerMode(t *testing.T) {
	client := tunarr.NewClient(tunarr.Config{URL: "http://localhost:8000"})
	store := NewMockStateStore()
	engine := NewEngine(client, []Block{}, store, slog.Default(), time.UTC)

	availablePrograms := []tunarr.Program{
		{ID: "f1", Title: "Filler Show", Duration: 600000, Type: "episode", Genres: []string{"Comedy"}},
		{ID: "f2", Title: "Another Filler", Duration: 600000, Type: "episode", Genres: []string{"Comedy"}},
	}

	block := Block{
		Fallback: SeriesFallback{
			Mode: FallbackModeFiller,
			FillerFilter: Filter{
				Genres: []string{"Comedy"},
			},
		},
	}

	initialPlaylist := []tunarr.Program{
		{ID: "p1", Title: "Series Episode", Duration: 1800000, Type: "episode"},
	}
	currentDuration := int64(1800000)
	targetDuration := int64(3000000) // 50 minutes, needs 20 minutes more

	playlist, finalDuration := engine.applySeriesFallback(block, availablePrograms, initialPlaylist, currentDuration, targetDuration)

	if len(playlist) <= len(initialPlaylist) {
		t.Error("Expected fallback filler to be added")
	}

	if finalDuration <= currentDuration {
		t.Error("Expected duration to increase after fallback")
	}
}

func TestApplySeriesFallback_NoFallbackNeeded(t *testing.T) {
	client := tunarr.NewClient(tunarr.Config{URL: "http://localhost:8000"})
	store := NewMockStateStore()
	engine := NewEngine(client, []Block{}, store, slog.Default(), time.UTC)

	initialPlaylist := []tunarr.Program{
		{ID: "p1", Title: "Series Episode", Duration: 1800000, Type: "episode"},
	}
	currentDuration := int64(1800000)
	targetDuration := int64(1800000) // Already at target

	playlist, finalDuration := engine.applySeriesFallback(Block{}, []tunarr.Program{}, initialPlaylist, currentDuration, targetDuration)

	if len(playlist) != len(initialPlaylist) {
		t.Error("Expected no fallback when at target duration")
	}

	if finalDuration != currentDuration {
		t.Error("Expected duration to remain unchanged")
	}
}

func TestApplySeriesFallback_NotFillerMode(t *testing.T) {
	client := tunarr.NewClient(tunarr.Config{URL: "http://localhost:8000"})
	store := NewMockStateStore()
	engine := NewEngine(client, []Block{}, store, slog.Default(), time.UTC)

	availablePrograms := []tunarr.Program{
		{ID: "f1", Title: "Filler Show", Duration: 600000, Type: "episode"},
	}

	block := Block{
		Fallback: SeriesFallback{
			Mode: "", // Not filler mode
		},
	}

	initialPlaylist := []tunarr.Program{
		{ID: "p1", Title: "Series Episode", Duration: 1800000, Type: "episode"},
	}
	currentDuration := int64(1800000)
	targetDuration := int64(3000000)

	playlist, finalDuration := engine.applySeriesFallback(block, availablePrograms, initialPlaylist, currentDuration, targetDuration)

	if len(playlist) != len(initialPlaylist) {
		t.Error("Expected no fallback when not in filler mode")
	}

	if finalDuration != currentDuration {
		t.Error("Expected duration to remain unchanged")
	}
}

func TestInitializeSeriesState_NewStateWithStartSeasonAndEpisode(t *testing.T) {
	client := tunarr.NewClient(tunarr.Config{URL: "http://localhost:8000"})
	store := NewMockStateStore()
	engine := NewEngine(client, []Block{}, store, slog.Default(), time.UTC)

	state := &SeriesState{
		ShowTitle:      "Test Show",
		CurrentSeason:  1,
		CurrentEpisode: 1,
		LastAired:      time.Time{}, // Zero time, indicating new state
	}

	config := SeriesConfig{
		ShowTitle:    "Test Show",
		StartSeason:  3,
		StartEpisode: 5,
	}

	engine.initializeSeriesState(state, config)

	if state.CurrentSeason != 3 {
		t.Errorf("Expected CurrentSeason to be 3, got %d", state.CurrentSeason)
	}

	if state.CurrentEpisode != 5 {
		t.Errorf("Expected CurrentEpisode to be 5, got %d", state.CurrentEpisode)
	}
}

func TestInitializeSeriesState_ExistingStateNotModified(t *testing.T) {
	client := tunarr.NewClient(tunarr.Config{URL: "http://localhost:8000"})
	store := NewMockStateStore()
	engine := NewEngine(client, []Block{}, store, slog.Default(), time.UTC)

	state := &SeriesState{
		ShowTitle:      "Test Show",
		CurrentSeason:  2,
		CurrentEpisode: 7,
		LastAired:      time.Now(), // Non-zero time, indicating existing state
	}

	config := SeriesConfig{
		ShowTitle:    "Test Show",
		StartSeason:  3,
		StartEpisode: 5,
	}

	engine.initializeSeriesState(state, config)

	// State should not be modified when LastAired is not zero
	if state.CurrentSeason != 2 {
		t.Errorf("Expected CurrentSeason to remain 2, got %d", state.CurrentSeason)
	}

	if state.CurrentEpisode != 7 {
		t.Errorf("Expected CurrentEpisode to remain 7, got %d", state.CurrentEpisode)
	}
}

func TestInitializeSeriesState_StartSeasonOnly(t *testing.T) {
	client := tunarr.NewClient(tunarr.Config{URL: "http://localhost:8000"})
	store := NewMockStateStore()
	engine := NewEngine(client, []Block{}, store, slog.Default(), time.UTC)

	state := &SeriesState{
		ShowTitle:      "Test Show",
		CurrentSeason:  1,
		CurrentEpisode: 1,
		LastAired:      time.Time{},
	}

	config := SeriesConfig{
		ShowTitle:   "Test Show",
		StartSeason: 4,
		// StartEpisode not specified (0)
	}

	engine.initializeSeriesState(state, config)

	if state.CurrentSeason != 4 {
		t.Errorf("Expected CurrentSeason to be 4, got %d", state.CurrentSeason)
	}

	if state.CurrentEpisode != 1 {
		t.Errorf("Expected CurrentEpisode to remain 1, got %d", state.CurrentEpisode)
	}
}

func TestInitializeSeriesState_StartEpisodeOnly(t *testing.T) {
	client := tunarr.NewClient(tunarr.Config{URL: "http://localhost:8000"})
	store := NewMockStateStore()
	engine := NewEngine(client, []Block{}, store, slog.Default(), time.UTC)

	state := &SeriesState{
		ShowTitle:      "Test Show",
		CurrentSeason:  1,
		CurrentEpisode: 1,
		LastAired:      time.Time{},
	}

	config := SeriesConfig{
		ShowTitle:    "Test Show",
		StartEpisode: 10,
		// StartSeason not specified (0)
	}

	engine.initializeSeriesState(state, config)

	if state.CurrentSeason != 1 {
		t.Errorf("Expected CurrentSeason to remain 1, got %d", state.CurrentSeason)
	}

	if state.CurrentEpisode != 10 {
		t.Errorf("Expected CurrentEpisode to be 10, got %d", state.CurrentEpisode)
	}
}

func TestInitializeSeriesState_NoStartConfig(t *testing.T) {
	client := tunarr.NewClient(tunarr.Config{URL: "http://localhost:8000"})
	store := NewMockStateStore()
	engine := NewEngine(client, []Block{}, store, slog.Default(), time.UTC)

	state := &SeriesState{
		ShowTitle:      "Test Show",
		CurrentSeason:  1,
		CurrentEpisode: 1,
		LastAired:      time.Time{},
	}

	config := SeriesConfig{
		ShowTitle: "Test Show",
		// No StartSeason or StartEpisode
	}

	engine.initializeSeriesState(state, config)

	// State should remain unchanged when no start config is provided
	if state.CurrentSeason != 1 {
		t.Errorf("Expected CurrentSeason to remain 1, got %d", state.CurrentSeason)
	}

	if state.CurrentEpisode != 1 {
		t.Errorf("Expected CurrentEpisode to remain 1, got %d", state.CurrentEpisode)
	}
}
