package scheduler

import (
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
			resolved := resolveConflicts(tt.slots)
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
	engine := NewEngine(client, []Block{}, NewMockStateStore())

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
	engine := NewEngine(client, []Block{}, NewMockStateStore())

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

func TestPlanBlock_Series(t *testing.T) {
	client := &tunarr.Client{}
	store := NewMockStateStore()
	engine := NewEngine(client, []Block{}, store)

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