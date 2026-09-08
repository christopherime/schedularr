package scheduler

import (
	"testing"
	"time"

	"github.com/christopherime/schedularr/internal/external/tunarr"
)

// Every occurrence a test records airs at occ; every query asks about
// the next one an hour later, so the "airs before me" bound
// WasRecentlyScheduled applies is satisfied and the window itself is
// what each assertion is about.
var (
	occ  = time.Date(2026, 9, 10, 8, 0, 0, 0, time.UTC)
	next = occ.Add(time.Hour)
)

func TestScheduleHistory_RecordAndCheck(t *testing.T) {
	history := NewScheduleHistory(24 * time.Hour)

	// Record a program
	history.RecordScheduled("prog1", "channel1", "Morning Block", time.Now(), occ)

	// Check if it was recently scheduled
	if !history.WasRecentlyScheduled("prog1", "channel1", next) {
		t.Error("Program should be marked as recently scheduled")
	}

	// Check different channel
	if history.WasRecentlyScheduled("prog1", "channel2", next) {
		t.Error("Program should not be marked as recently scheduled on different channel")
	}

	// Check non-existent program
	if history.WasRecentlyScheduled("prog2", "channel1", next) {
		t.Error("Non-existent program should not be marked as recently scheduled")
	}
}

func TestScheduleHistory_ExpirationWindow(t *testing.T) {
	history := NewScheduleHistory(1 * time.Hour)

	// Record a program 2 hours ago
	pastTime := time.Now().Add(-2 * time.Hour)
	history.RecordScheduled("prog1", "channel1", "Morning Block", pastTime, occ)

	// Should not be considered recently scheduled (outside window)
	if history.WasRecentlyScheduled("prog1", "channel1", next) {
		t.Error("Program scheduled outside window should not be considered recent")
	}

	// Record same program now
	history.RecordScheduled("prog1", "channel1", "Afternoon Block", time.Now(), occ)

	// Should now be considered recently scheduled
	if !history.WasRecentlyScheduled("prog1", "channel1", next) {
		t.Error("Program scheduled within window should be considered recent")
	}
}

// A run plans block by block, so by the time a mid-run occurrence is
// planned the tracker already holds occurrences that air after it. Those
// may not filter it: a plan has to be the same whether the caller asked
// for 7 days or 28.
func TestScheduleHistory_IgnoresLaterOccurrences(t *testing.T) {
	history := NewScheduleHistory(24 * time.Hour)

	history.RecordScheduled("prog1", "channel1", "Late Block", time.Now(), next)

	if history.WasRecentlyScheduled("prog1", "channel1", occ) {
		t.Error("An occurrence that airs later must not filter an earlier one's candidates")
	}
	if history.WasRecentlyScheduled("prog1", "channel1", next) {
		t.Error("An occurrence must not filter itself")
	}
	if !history.WasRecentlyScheduled("prog1", "channel1", next.Add(time.Hour)) {
		t.Error("An occurrence that airs earlier should still be considered recent")
	}
}

func TestScheduleHistory_RecordPrograms(t *testing.T) {
	history := NewScheduleHistory(24 * time.Hour)

	programs := []tunarr.Program{
		{ID: "prog1", Title: "Show A"},
		{ID: "prog2", Title: "Show B"},
		{ID: "prog3", Title: "Show C"},
	}

	history.RecordPrograms(programs, "channel1", "Morning Block", time.Now(), occ)

	// All programs should be marked as recently scheduled
	for _, p := range programs {
		if !history.WasRecentlyScheduled(p.ID, "channel1", next) {
			t.Errorf("Program %s should be marked as recently scheduled", p.ID)
		}
	}
}

func TestScheduleHistory_MultipleChannels(t *testing.T) {
	history := NewScheduleHistory(24 * time.Hour)

	// Record same program on different channels
	history.RecordScheduled("prog1", "channel1", "Block1", time.Now(), occ)
	history.RecordScheduled("prog1", "channel2", "Block2", time.Now(), occ)

	// Should be recent on both channels
	if !history.WasRecentlyScheduled("prog1", "channel1", next) {
		t.Error("Program should be recent on channel1")
	}

	if !history.WasRecentlyScheduled("prog1", "channel2", next) {
		t.Error("Program should be recent on channel2")
	}

	// Should not be recent on channel3
	if history.WasRecentlyScheduled("prog1", "channel3", next) {
		t.Error("Program should not be recent on channel3")
	}
}
