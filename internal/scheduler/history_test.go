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
