package scheduler

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/geekxflood/schedularr/internal/external/tunarr"
)

// TestIntegration_FullSchedulingWorkflow tests the complete scheduling workflow
// from loading programs to generating and resolving a schedule.
func TestIntegration_FullSchedulingWorkflow(t *testing.T) {
	// Load test program data
	cartoonsData, err := os.ReadFile("../../testdata/programs/cartoons.json")
	if err != nil {
		t.Fatalf("Failed to load cartoons fixture: %v", err)
	}

	var programs []tunarr.Program
	if err := json.Unmarshal(cartoonsData, &programs); err != nil {
		t.Fatalf("Failed to parse cartoons JSON: %v", err)
	}

	if len(programs) == 0 {
		t.Fatal("No programs loaded from fixture")
	}

	// Create mock store
	mockStore := NewMockStateStore()

	// Define test blocks
	blocks := []Block{
		{
			Name:      "Morning Cartoons",
			Cron:      "0 6 * * *",
			Duration:  120, // 2 hours
			ChannelID: "channel-1",
			Priority:  10,
			Filter: Filter{
				Genres:      []string{"Animation", "Family"},
				Ratings:     []string{"TV-Y", "TV-Y7"},
				MaxDuration: 30,
			},
		},
		{
			Name:      "Afternoon Animation",
			Cron:      "0 14 * * *",
			Duration:  90, // 1.5 hours
			ChannelID: "channel-1",
			Priority:  5,
			Filter: Filter{
				Genres:      []string{"Animation"},
				MaxDuration: 30,
			},
		},
	}

	// Create engine - pass nil for client since we're passing programs directly
	engine := NewEngine(nil, blocks, mockStore, nil, time.UTC)
	if engine == nil {
		t.Fatal("Failed to create engine")
	}

	// Generate schedule for next 7 days
	startTime := time.Date(2026, 1, 13, 0, 0, 0, 0, time.UTC)
	endTime := startTime.Add(7 * 24 * time.Hour)

	schedule, err := engine.GenerateForTimeRange(startTime, endTime, programs)
	if err != nil {
		t.Fatalf("GenerateForTimeRange failed: %v", err)
	}

	// Verify schedule was generated
	if len(schedule) == 0 {
		t.Fatal("No schedule generated")
	}

	// Verify channel-1 has slots
	channelSlots, ok := schedule["channel-1"]
	if !ok {
		t.Fatal("No slots generated for channel-1")
	}

	if len(channelSlots) == 0 {
		t.Fatal("Channel-1 has no scheduled slots")
	}

	// Verify all slots have programs
	for i, slot := range channelSlots {
		if len(slot.Programs) == 0 {
			t.Errorf("Slot %d has no programs", i)
		}

		// Verify slot times are valid
		if slot.StartTime.IsZero() {
			t.Errorf("Slot %d has zero start time", i)
		}
		if slot.EndTime.IsZero() {
			t.Errorf("Slot %d has zero end time", i)
		}
		if !slot.StartTime.Before(slot.EndTime) {
			t.Errorf("Slot %d: start time %v is not before end time %v",
				i, slot.StartTime, slot.EndTime)
		}

		// Verify programs match filter criteria
		for j, prog := range slot.Programs {
			// Check genre filter
			hasMatchingGenre := false
			for _, genre := range prog.Genres {
				for _, filterGenre := range slot.Block.Filter.Genres {
					if genre == filterGenre {
						hasMatchingGenre = true
						break
					}
				}
				if hasMatchingGenre {
					break
				}
			}
			if !hasMatchingGenre && len(slot.Block.Filter.Genres) > 0 {
				t.Errorf("Slot %d, Program %d (%s) doesn't match genre filter %v",
					i, j, prog.Title, slot.Block.Filter.Genres)
			}

			// Check duration filter (convert ms to minutes)
			durationMinutes := int(prog.Duration / 60000)
			if slot.Block.Filter.MaxDuration > 0 && durationMinutes > slot.Block.Filter.MaxDuration {
				t.Errorf("Slot %d, Program %d (%s) exceeds max duration: %d > %d",
					i, j, prog.Title, durationMinutes, slot.Block.Filter.MaxDuration)
			}
		}
	}

	// Verify no overlaps (conflict resolution worked)
	for i := 0; i < len(channelSlots)-1; i++ {
		for j := i + 1; j < len(channelSlots); j++ {
			if slotsOverlap(channelSlots[i], channelSlots[j]) {
				t.Errorf("Slots %d and %d overlap: %v-%v and %v-%v",
					i, j,
					channelSlots[i].StartTime, channelSlots[i].EndTime,
					channelSlots[j].StartTime, channelSlots[j].EndTime)
			}
		}
	}

	// Verify commit works
	if err := engine.Commit(); err != nil {
		t.Fatalf("Commit failed: %v", err)
	}

	t.Logf("Successfully generated %d slots across %d channels",
		len(channelSlots), len(schedule))
}

// TestIntegration_ConflictResolution tests that higher priority blocks
// override lower priority blocks when there are scheduling conflicts.
func TestIntegration_ConflictResolution(t *testing.T) {
	// Load test programs
	data, err := os.ReadFile("../../testdata/programs/sitcoms.json")
	if err != nil {
		t.Fatalf("Failed to load sitcoms fixture: %v", err)
	}

	var programs []tunarr.Program
	if err := json.Unmarshal(data, &programs); err != nil {
		t.Fatalf("Failed to parse sitcoms JSON: %v", err)
	}

	mockStore := NewMockStateStore()

	// Create overlapping blocks with different priorities
	blocks := []Block{
		{
			Name:      "Low Priority Block",
			Cron:      "0 19 * * *", // 7 PM
			Duration:  120,
			ChannelID: "channel-1",
			Priority:  5, // Lower priority
			Filter: Filter{
				Genres: []string{"Comedy"},
			},
		},
		{
			Name:      "High Priority Block",
			Cron:      "30 19 * * *", // 7:30 PM (overlaps with previous)
			Duration:  90,
			ChannelID: "channel-1",
			Priority:  20, // Higher priority
			Filter: Filter{
				Genres: []string{"Comedy"},
			},
		},
	}

	engine := NewEngine(nil, blocks, mockStore, nil, time.UTC)

	startTime := time.Date(2026, 1, 13, 0, 0, 0, 0, time.UTC)
	endTime := startTime.Add(24 * time.Hour)

	schedule, err := engine.GenerateForTimeRange(startTime, endTime, programs)
	if err != nil {
		t.Fatalf("GenerateForTimeRange failed: %v", err)
	}

	channelSlots, ok := schedule["channel-1"]
	if !ok || len(channelSlots) == 0 {
		t.Fatal("No slots generated")
	}

	// Verify no overlaps - conflict should be resolved
	for i := 0; i < len(channelSlots)-1; i++ {
		for j := i + 1; j < len(channelSlots); j++ {
			if slotsOverlap(channelSlots[i], channelSlots[j]) {
				t.Errorf("Found overlapping slots after conflict resolution")
			}
		}
	}

	// Verify high priority block exists in schedule
	foundHighPriority := false
	for _, slot := range channelSlots {
		if slot.Block.Priority == 20 {
			foundHighPriority = true
			break
		}
	}

	if !foundHighPriority {
		t.Error("High priority block not found in resolved schedule")
	}
}

// TestIntegration_FilterValidation tests that filtering works correctly
// with real test data across different filter criteria.
func TestIntegration_FilterValidation(t *testing.T) {
	// Load movies data
	moviesData, err := os.ReadFile("../../testdata/programs/movies.json")
	if err != nil {
		t.Fatalf("Failed to load movies fixture: %v", err)
	}

	var programs []tunarr.Program
	if err := json.Unmarshal(moviesData, &programs); err != nil {
		t.Fatalf("Failed to parse movies JSON: %v", err)
	}

	mockStore := NewMockStateStore()

	// Test block with multiple filter criteria
	blocks := []Block{
		{
			Name:      "Action Movies",
			Cron:      "0 20 * * 5", // Friday 8 PM
			Duration:  180,          // 3 hours
			ChannelID: "channel-2",
			Priority:  15,
			Filter: Filter{
				Genres:      []string{"Action"},
				Ratings:     []string{"PG-13", "R"},
				MinDuration: 90,
				YearFrom:    2000,
			},
		},
	}

	engine := NewEngine(nil, blocks, mockStore, nil, time.UTC)

	// Friday, Jan 16, 2026
	startTime := time.Date(2026, 1, 16, 0, 0, 0, 0, time.UTC)
	endTime := startTime.Add(24 * time.Hour)

	schedule, err := engine.GenerateForTimeRange(startTime, endTime, programs)
	if err != nil {
		t.Fatalf("GenerateForTimeRange failed: %v", err)
	}

	channelSlots, ok := schedule["channel-2"]
	if !ok || len(channelSlots) == 0 {
		t.Fatal("No slots generated for action movies")
	}

	// Verify all programs meet filter criteria
	for _, slot := range channelSlots {
		for _, prog := range slot.Programs {
			// Must have "Action" genre
			hasAction := false
			for _, genre := range prog.Genres {
				if genre == "Action" {
					hasAction = true
					break
				}
			}
			if !hasAction {
				t.Errorf("Program %s missing Action genre", prog.Title)
			}

			// Must be PG-13 or R
			if prog.Rating != "PG-13" && prog.Rating != "R" {
				t.Errorf("Program %s has invalid rating: %s", prog.Title, prog.Rating)
			}

			// Must be at least 90 minutes
			durationMinutes := int(prog.Duration / 60000)
			if durationMinutes < 90 {
				t.Errorf("Program %s too short: %d minutes", prog.Title, durationMinutes)
			}

			// Must be from 2000 or later
			if prog.Year < 2000 {
				t.Errorf("Program %s too old: %d", prog.Title, prog.Year)
			}
		}
	}
}
