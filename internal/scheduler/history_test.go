package scheduler

import (
	"testing"
	"time"

	"github.com/geekxflood/schedularr/internal/external/tunarr"
)

func TestScheduleHistory_RecordAndCheck(t *testing.T) {
	history := NewScheduleHistory(24 * time.Hour)

	// Record a program
	history.RecordScheduled("prog1", "channel1", "Morning Block", time.Now())

	// Check if it was recently scheduled
	if !history.WasRecentlyScheduled("prog1", "channel1") {
		t.Error("Program should be marked as recently scheduled")
	}

	// Check different channel
	if history.WasRecentlyScheduled("prog1", "channel2") {
		t.Error("Program should not be marked as recently scheduled on different channel")
	}

	// Check non-existent program
	if history.WasRecentlyScheduled("prog2", "channel1") {
		t.Error("Non-existent program should not be marked as recently scheduled")
	}
}

func TestScheduleHistory_ExpirationWindow(t *testing.T) {
	history := NewScheduleHistory(1 * time.Hour)

	// Record a program 2 hours ago
	pastTime := time.Now().Add(-2 * time.Hour)
	history.RecordScheduled("prog1", "channel1", "Morning Block", pastTime)

	// Should not be considered recently scheduled (outside window)
	if history.WasRecentlyScheduled("prog1", "channel1") {
		t.Error("Program scheduled outside window should not be considered recent")
	}

	// Record same program now
	history.RecordScheduled("prog1", "channel1", "Afternoon Block", time.Now())

	// Should now be considered recently scheduled
	if !history.WasRecentlyScheduled("prog1", "channel1") {
		t.Error("Program scheduled within window should be considered recent")
	}
}

func TestScheduleHistory_GetLastScheduled(t *testing.T) {
	history := NewScheduleHistory(24 * time.Hour)

	// No history yet
	lastTime := history.GetLastScheduled("prog1", "channel1")
	if lastTime != nil {
		t.Error("Expected nil for program with no history")
	}

	// Record at different times
	time1 := time.Now().Add(-2 * time.Hour)
	time2 := time.Now().Add(-1 * time.Hour)

	history.RecordScheduled("prog1", "channel1", "Block1", time1)
	history.RecordScheduled("prog1", "channel1", "Block2", time2)

	// Should return most recent time
	lastTime = history.GetLastScheduled("prog1", "channel1")
	if lastTime == nil {
		t.Fatal("Expected non-nil last scheduled time")
	}

	if !lastTime.Equal(time2) {
		t.Errorf("Expected last time %v, got %v", time2, *lastTime)
	}
}

func TestScheduleHistory_RecordPrograms(t *testing.T) {
	history := NewScheduleHistory(24 * time.Hour)

	programs := []tunarr.Program{
		{ID: "prog1", Title: "Show A"},
		{ID: "prog2", Title: "Show B"},
		{ID: "prog3", Title: "Show C"},
	}

	history.RecordPrograms(programs, "channel1", "Morning Block", time.Now())

	// All programs should be marked as recently scheduled
	for _, p := range programs {
		if !history.WasRecentlyScheduled(p.ID, "channel1") {
			t.Errorf("Program %s should be marked as recently scheduled", p.ID)
		}
	}
}

func TestScheduleHistory_FilterByHistory(t *testing.T) {
	history := NewScheduleHistory(24 * time.Hour)

	programs := []tunarr.Program{
		{ID: "prog1", Title: "Show A"},
		{ID: "prog2", Title: "Show B"},
		{ID: "prog3", Title: "Show C"},
	}

	// Record prog1 and prog2 as recently scheduled
	history.RecordScheduled("prog1", "channel1", "Block1", time.Now())
	history.RecordScheduled("prog2", "channel1", "Block1", time.Now())

	// Filter should only return prog3
	filtered := history.FilterByHistory(programs, "channel1")

	if len(filtered) != 1 {
		t.Errorf("Expected 1 program after filtering, got %d", len(filtered))
	}

	if len(filtered) > 0 && filtered[0].ID != "prog3" {
		t.Errorf("Expected prog3, got %s", filtered[0].ID)
	}
}

func TestScheduleHistory_CleanupOldEntries(t *testing.T) {
	history := NewScheduleHistory(1 * time.Hour)

	// Record old and new entries
	oldTime := time.Now().Add(-2 * time.Hour)
	newTime := time.Now()

	history.RecordScheduled("prog1", "channel1", "Block1", oldTime)
	history.RecordScheduled("prog2", "channel1", "Block2", newTime)

	// Cleanup old entries
	history.CleanupOldEntries()

	// prog1 should be gone, prog2 should remain
	if history.WasRecentlyScheduled("prog1", "channel1") {
		t.Error("Old program should have been cleaned up")
	}

	if !history.WasRecentlyScheduled("prog2", "channel1") {
		t.Error("Recent program should still be in history")
	}

	// Check stats
	stats := history.GetStats()
	if stats["total_programs"] != 1 {
		t.Errorf("Expected 1 program in stats, got %d", stats["total_programs"])
	}
}

func TestScheduleHistory_GetStats(t *testing.T) {
	history := NewScheduleHistory(1 * time.Hour)

	// Record some entries
	history.RecordScheduled("prog1", "channel1", "Block1", time.Now())
	history.RecordScheduled("prog1", "channel1", "Block2", time.Now())
	history.RecordScheduled("prog2", "channel1", "Block1", time.Now())

	// Record old entry
	oldTime := time.Now().Add(-2 * time.Hour)
	history.RecordScheduled("prog3", "channel1", "Block1", oldTime)

	stats := history.GetStats()

	if stats["total_programs"] != 3 {
		t.Errorf("Expected 3 unique programs, got %d", stats["total_programs"])
	}

	if stats["total_entries"] != 4 {
		t.Errorf("Expected 4 total entries, got %d", stats["total_entries"])
	}

	if stats["active_entries"] != 3 {
		t.Errorf("Expected 3 active entries, got %d", stats["active_entries"])
	}
}

func TestScheduleHistory_MultipleChannels(t *testing.T) {
	history := NewScheduleHistory(24 * time.Hour)

	// Record same program on different channels
	history.RecordScheduled("prog1", "channel1", "Block1", time.Now())
	history.RecordScheduled("prog1", "channel2", "Block2", time.Now())

	// Should be recent on both channels
	if !history.WasRecentlyScheduled("prog1", "channel1") {
		t.Error("Program should be recent on channel1")
	}

	if !history.WasRecentlyScheduled("prog1", "channel2") {
		t.Error("Program should be recent on channel2")
	}

	// Should not be recent on channel3
	if history.WasRecentlyScheduled("prog1", "channel3") {
		t.Error("Program should not be recent on channel3")
	}
}
