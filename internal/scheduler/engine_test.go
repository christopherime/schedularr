package scheduler

import (
	"log/slog"
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
	engine := NewEngine(client, []Block{}, NewMockStateStore(), slog.Default())

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
	engine := NewEngine(client, []Block{}, NewMockStateStore(), slog.Default())

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
	engine := NewEngine(client, []Block{}, store, slog.Default())

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

	engine := NewEngineWithHistory(client, []Block{}, 24*time.Hour, store, slog.Default())

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
	engine := NewEngine(client, []Block{}, store, slog.Default())

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
	engine := NewEngine(client, []Block{}, store, slog.Default())

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
	engine := NewEngine(client, []Block{}, store, slog.Default())

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
	engine := NewEngine(client, []Block{}, store, slog.Default())

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
	engine := NewEngine(client, []Block{}, store, slog.Default())

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
	engine := NewEngine(client, []Block{}, store, slog.Default())

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
	engine := NewEngine(client, []Block{}, store, slog.Default())

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
