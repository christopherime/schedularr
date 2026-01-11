package scheduler

import (
	"sync"
	"time"

	"github.com/geekxflood/schedularr/internal/tunarr"
)

// ScheduleHistoryEntry represents a single program that was scheduled
type ScheduleHistoryEntry struct {
	ProgramID   string
	ChannelID   string
	ScheduledAt time.Time
	BlockName   string
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
		sh.RecordScheduled(p.ID, channelID, blockName, scheduledAt)
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

// GetLastScheduled returns the most recent time a program was scheduled on a channel
func (sh *ScheduleHistory) GetLastScheduled(programID string, channelID string) *time.Time {
	sh.mu.RLock()
	defer sh.mu.RUnlock()

	entries, exists := sh.entries[programID]
	if !exists {
		return nil
	}

	var lastTime *time.Time
	for _, entry := range entries {
		if entry.ChannelID == channelID {
			if lastTime == nil || entry.ScheduledAt.After(*lastTime) {
				t := entry.ScheduledAt
				lastTime = &t
			}
		}
	}

	return lastTime
}

// CleanupOldEntries removes entries older than the tracking window
func (sh *ScheduleHistory) CleanupOldEntries() {
	sh.mu.Lock()
	defer sh.mu.Unlock()

	cutoff := time.Now().Add(-sh.window)

	// Filter out old entries
	for programID, entries := range sh.entries {
		var kept []ScheduleHistoryEntry
		for _, entry := range entries {
			if entry.ScheduledAt.After(cutoff) {
				kept = append(kept, entry)
			}
		}

		if len(kept) == 0 {
			delete(sh.entries, programID)
		} else {
			sh.entries[programID] = kept
		}
	}
}

// GetStats returns statistics about the history
func (sh *ScheduleHistory) GetStats() map[string]int {
	sh.mu.RLock()
	defer sh.mu.RUnlock()

	stats := make(map[string]int)
	cutoff := time.Now().Add(-sh.window)
	totalEntries := 0
	activeEntries := 0

	for _, entries := range sh.entries {
		totalEntries += len(entries)
		for _, entry := range entries {
			if entry.ScheduledAt.After(cutoff) {
				activeEntries++
			}
		}
	}

	stats["total_programs"] = len(sh.entries)
	stats["total_entries"] = totalEntries
	stats["active_entries"] = activeEntries

	return stats
}

// FilterByHistory filters out programs that were recently scheduled
func (sh *ScheduleHistory) FilterByHistory(programs []tunarr.Program, channelID string) []tunarr.Program {
	sh.mu.RLock()
	defer sh.mu.RUnlock()

	var filtered []tunarr.Program
	for _, p := range programs {
		if !sh.WasRecentlyScheduled(p.ID, channelID) {
			filtered = append(filtered, p)
		}
	}

	return filtered
}
