package scheduler

import (
	"sync"
	"time"

	"github.com/christopherime/schedularr/internal/external/tunarr"
)

// ScheduleHistoryEntry represents a single program that was scheduled.
//
// ScheduledAt and OccurrenceStart serve two different purposes and are
// deliberately not the same value: ScheduledAt is the wall-clock instant
// planning happened (time.Now() at the moment Engine.PlanBlock ran) --
// what the recency-dedup window (ScheduleHistory.WasRecentlyScheduled,
// StateStore.WasRecentlyScheduled) checks against. OccurrenceStart is the
// block occurrence's own cron-computed StartTime -- the identity half of
// the (block_name, occurrence_start) key Engine.PlanBlock's idempotence
// check looks up (see its doc comment in engine.go): the same occurrence
// re-planned in a later apply must resolve to the same assignment, which
// requires a key that doesn't change between applies the way ScheduledAt
// (a fresh time.Now() every time) does.
//
// Sequence, DurationMs, Title, and Type exist purely to make an
// occurrence's assignment fully replayable without depending on the live
// Tunarr catalog still containing every program by ID: Sequence preserves
// playback order (a lineup's wall-clock anchoring depends on it -- see
// service.buildAnchoredLineup), and DurationMs/Title/Type are enough to
// reconstruct a valid tunarr.Program on their own.
type ScheduleHistoryEntry struct {
	ProgramID       string    `db:"program_id"`
	ChannelID       string    `db:"channel_id"`
	ScheduledAt     time.Time `db:"scheduled_at"`
	BlockName       string    `db:"block_name"`
	OccurrenceStart time.Time `db:"occurrence_start"`
	Sequence        int       `db:"sequence"`
	DurationMs      float64   `db:"duration_ms"`
	Title           string    `db:"title"`
	Type            string    `db:"type"`
}

// ScheduleHistory tracks what content has been scheduled to prevent repetition
type ScheduleHistory struct {
	mu      sync.RWMutex
	entries map[string][]ScheduleHistoryEntry // key is programID
	window  time.Duration                     // how far back to track
}

// NewScheduleHistory creates a new schedule history tracker
func NewScheduleHistory(window time.Duration) *ScheduleHistory {
	return &ScheduleHistory{
		entries: make(map[string][]ScheduleHistoryEntry),
		window:  window,
	}
}

// Window returns the configured tracking window for history entries.
func (sh *ScheduleHistory) Window() time.Duration {
	return sh.window
}

// RecordScheduled records that a program was scheduled
func (sh *ScheduleHistory) RecordScheduled(programID, channelID, blockName string, scheduledAt time.Time) {
	sh.mu.Lock()
	defer sh.mu.Unlock()

	entry := ScheduleHistoryEntry{
		ProgramID:   programID,
		ChannelID:   channelID,
		ScheduledAt: scheduledAt,
		BlockName:   blockName,
	}

	sh.entries[programID] = append(sh.entries[programID], entry)
}

// RecordPrograms records multiple programs as scheduled
func (sh *ScheduleHistory) RecordPrograms(programs []tunarr.Program, channelID, blockName string, scheduledAt time.Time) {
	for _, p := range programs {
		sh.RecordScheduled(p.GetID(), channelID, blockName, scheduledAt)
	}
}

// WasRecentlyScheduled checks if a program was scheduled within the tracking window
func (sh *ScheduleHistory) WasRecentlyScheduled(programID string, channelID string) bool {
	sh.mu.RLock()
	defer sh.mu.RUnlock()

	entries, exists := sh.entries[programID]
	if !exists {
		return false
	}

	cutoff := time.Now().Add(-sh.window)

	// Check if any entry for this program on this channel is within the window
	for _, entry := range entries {
		if entry.ChannelID == channelID && entry.ScheduledAt.After(cutoff) {
			return true
		}
	}

	return false
}
