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
// the recency-dedup WINDOW (ScheduleHistory.WasRecentlyScheduled,
// StateStore.WasRecentlyScheduled) is measured against it.
// OccurrenceStart is the block occurrence's own cron-computed StartTime.
// It is the identity half of the (block_name, occurrence_start) key
// Engine.PlanBlock's idempotence check looks up (see its doc comment in
// engine.go) -- the same occurrence re-planned in a later apply must
// resolve to the same assignment, which requires a key that doesn't
// change between applies the way ScheduledAt (a fresh time.Now() every
// time) does -- and it is also the ORDER the in-memory dedup check reads
// entries by, so a run's own not-yet-aired occurrences cannot filter an
// earlier one's candidates (see ScheduleHistory.WasRecentlyScheduled).
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
	// ShowTitle is the SHOW this airing belongs to, where Title is the
	// EPISODE. Empty for anything that belongs to no show -- a movie, or a
	// program Tunarr could not resolve a show for -- and for every row
	// written before migration 000012, which cannot be backfilled.
	//
	// It is filled from tunarr.Program.ShowTitle rather than from the
	// block's series config on purpose: a FILTER block can select an
	// episode too, and that episode belongs to its show no matter which
	// kind of block put it on air. Keying off the block would have made
	// "remove this show" miss exactly those airings.
	ShowTitle string `db:"show_title"`
	Type      string `db:"type"`
	// RunID is the apply run (store.ApplyRun.ID) that committed this row,
	// or "" for rows written before apply runs were recorded (v0.5.7) --
	// runs cannot be backfilled, so an empty RunID means "unknown apply",
	// not "no apply". Stamped by Engine.Commit from EngineOptions.RunID,
	// never by the planner: a dry run produces entries too, and they must
	// not claim a run that never happened.
	RunID string `db:"run_id"`
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

// RecordScheduled records that a program was scheduled for one block
// occurrence. occurrenceStart is that occurrence's own cron-computed
// start time -- see WasRecentlyScheduled for why the entry has to carry
// it and not only the wall-clock scheduledAt.
func (sh *ScheduleHistory) RecordScheduled(programID, channelID, blockName string, scheduledAt, occurrenceStart time.Time) {
	sh.mu.Lock()
	defer sh.mu.Unlock()

	entry := ScheduleHistoryEntry{
		ProgramID:       programID,
		ChannelID:       channelID,
		ScheduledAt:     scheduledAt,
		BlockName:       blockName,
		OccurrenceStart: occurrenceStart,
	}

	sh.entries[programID] = append(sh.entries[programID], entry)
}

// RecordPrograms records multiple programs as scheduled for one block
// occurrence.
func (sh *ScheduleHistory) RecordPrograms(programs []tunarr.Program, channelID, blockName string, scheduledAt, occurrenceStart time.Time) {
	for _, p := range programs {
		sh.RecordScheduled(p.GetID(), channelID, blockName, scheduledAt, occurrenceStart)
	}
}

// WasRecentlyScheduled reports whether programID already went out on
// channelID inside the tracking window, for an occurrence that airs
// BEFORE occurrenceStart.
//
// The occurrence bound is what makes a plan independent of how wide a
// window it was asked for. This tracker is populated as a single run
// plans, block by block in e.blocks order and each block in its own
// chronological order, so by the time a mid-run occurrence is planned
// the tracker already holds entries for occurrences that air LATER than
// it -- every occurrence of every block planned earlier in the run. That
// set grows with the requested window, so without this bound a 7-day
// draft and a 28-day reading would filter the same occurrence's
// candidates from different sets and plan it differently, and the
// Guide's diff would read CHANGED on a draft nobody edited. Counting
// only strictly-earlier occurrences makes the filtered set a function of
// the occurrence alone. (The persisted twin, StateStore.
// WasRecentlyScheduled, needs no such bound: it only ever sees
// occurrences committed by an earlier apply.)
func (sh *ScheduleHistory) WasRecentlyScheduled(programID, channelID string, occurrenceStart time.Time) bool {
	sh.mu.RLock()
	defer sh.mu.RUnlock()

	entries, exists := sh.entries[programID]
	if !exists {
		return false
	}

	cutoff := time.Now().Add(-sh.window)

	for _, entry := range entries {
		if entry.ChannelID != channelID || !entry.ScheduledAt.After(cutoff) {
			continue
		}
		if entry.OccurrenceStart.Before(occurrenceStart) {
			return true
		}
	}

	return false
}
