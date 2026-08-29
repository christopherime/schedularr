package scheduler

import (
	"context"
	"sort"
	"time"

	"github.com/christopherime/schedularr/internal/external/tunarr"
)

// MockStateStore is a mock implementation of StateStore for testing.
type MockStateStore struct {
	States    map[string]*SeriesState
	History   []ScheduleHistoryEntry
	Snapshots map[occurrenceKey]map[string]SeriesStateSnapshot
	// SnapshotRecordedAt mirrors Store's series_occurrence_snapshots.recorded_at
	// column (real wall-clock write time, refreshed on every upsert) --
	// see CleanupOccurrenceSnapshots' doc comment for why cleanup keys off
	// this instead of occurrence_start.
	SnapshotRecordedAt map[occurrenceKey]time.Time
}

// NewMockStateStore creates a new MockStateStore with initialized maps.
func NewMockStateStore() *MockStateStore {
	return &MockStateStore{
		States:             make(map[string]*SeriesState),
		History:            []ScheduleHistoryEntry{},
		Snapshots:          make(map[occurrenceKey]map[string]SeriesStateSnapshot),
		SnapshotRecordedAt: make(map[occurrenceKey]time.Time),
	}
}

// GetSeriesState retrieves the series state from the mock store.
func (m *MockStateStore) GetSeriesState(_ context.Context, showTitle string) (*SeriesState, error) {
	if state, ok := m.States[showTitle]; ok {
		return state, nil
	}
	// Return default state
	return &SeriesState{
		ShowTitle:      showTitle,
		CurrentSeason:  1,
		CurrentEpisode: 1,
		Completed:      false,
	}, nil
}

// UpdateSeriesState updates the series state in the mock store.
func (m *MockStateStore) UpdateSeriesState(_ context.Context, state *SeriesState) error {
	m.States[state.ShowTitle] = state
	return nil
}

// RecordScheduleHistory records schedule history entries in memory.
func (m *MockStateStore) RecordScheduleHistory(_ context.Context, entries []ScheduleHistoryEntry) error {
	m.History = append(m.History, entries...)
	return nil
}

// WasRecentlyScheduled checks the in-memory history for a recent entry.
func (m *MockStateStore) WasRecentlyScheduled(_ context.Context, programID, channelID string, window time.Duration) (bool, error) {
	cutoff := time.Now().Add(-window)
	for _, entry := range m.History {
		if entry.ProgramID == programID && entry.ChannelID == channelID && entry.ScheduledAt.After(cutoff) {
			return true, nil
		}
	}
	return false, nil
}

// CleanupScheduleHistory removes history entries older than the window.
func (m *MockStateStore) CleanupScheduleHistory(_ context.Context, window time.Duration) (int64, error) {
	cutoff := time.Now().Add(-window)
	var kept []ScheduleHistoryEntry
	removed := int64(0)
	for _, entry := range m.History {
		if entry.ScheduledAt.After(cutoff) {
			kept = append(kept, entry)
		} else {
			removed++
		}
	}
	m.History = kept
	return removed, nil
}

// GetCommittedOccurrence reconstructs the program assignment previously
// committed for one occurrence of blockName starting at occurrenceStart,
// mirroring Store.GetCommittedOccurrence's SQL behavior (internal/store/
// sqlite.go) over the in-memory History slice instead.
func (m *MockStateStore) GetCommittedOccurrence(_ context.Context, blockName string, occurrenceStart time.Time) ([]tunarr.Program, bool, error) {
	var matches []ScheduleHistoryEntry
	for _, entry := range m.History {
		if entry.BlockName == blockName && entry.OccurrenceStart.Equal(occurrenceStart) {
			matches = append(matches, entry)
		}
	}
	if len(matches) == 0 {
		return nil, false, nil
	}

	sort.Slice(matches, func(i, j int) bool { return matches[i].Sequence < matches[j].Sequence })

	// A single sentinel entry (empty ProgramID) means "planned, zero
	// programs" -- see makeHistoryEntries' doc comment (engine.go).
	if len(matches) == 1 && matches[0].ProgramID == "" {
		return nil, true, nil
	}

	programs := make([]tunarr.Program, 0, len(matches))
	for _, entry := range matches {
		programs = append(programs, tunarr.Program{
			UUID:     entry.ProgramID,
			Title:    entry.Title,
			Duration: entry.DurationMs,
			Type:     entry.Type,
		})
	}
	return programs, true, nil
}

// GetOccurrenceSnapshot mirrors Store.GetOccurrenceSnapshot's SQL
// behavior over the in-memory Snapshots map instead. Keyed by blockID,
// like the real store -- occurrenceKey's "blockName" field is just a
// generic string-key slot reused here for that, not literally a block
// name (see keyFor, its other user, for the phase-2 conflict-resolution
// use where the field name is literal).
func (m *MockStateStore) GetOccurrenceSnapshot(_ context.Context, blockID string, occurrenceStart time.Time) (map[string]SeriesStateSnapshot, bool, error) {
	snapshot, ok := m.Snapshots[occurrenceKey{blockName: blockID, startUnixNano: occurrenceStart.UnixNano()}]
	return snapshot, ok, nil
}

// SaveOccurrenceSnapshot mirrors Store.SaveOccurrenceSnapshot: an upsert
// keyed by blockID that also refreshes SnapshotRecordedAt (mirroring the
// real store's recorded_at column).
func (m *MockStateStore) SaveOccurrenceSnapshot(_ context.Context, blockID string, occurrenceStart time.Time, snapshot map[string]SeriesStateSnapshot) error {
	key := occurrenceKey{blockName: blockID, startUnixNano: occurrenceStart.UnixNano()}
	m.Snapshots[key] = snapshot
	m.SnapshotRecordedAt[key] = time.Now()
	return nil
}

// CleanupOccurrenceSnapshots mirrors Store.CleanupOccurrenceSnapshots:
// prunes by SnapshotRecordedAt (real wall-clock write time), not
// occurrence_start -- see that field's doc comment.
func (m *MockStateStore) CleanupOccurrenceSnapshots(_ context.Context, window time.Duration) (int64, error) {
	cutoff := time.Now().Add(-window)
	removed := int64(0)
	for key, recordedAt := range m.SnapshotRecordedAt {
		if recordedAt.Before(cutoff) {
			delete(m.Snapshots, key)
			delete(m.SnapshotRecordedAt, key)
			removed++
		}
	}
	return removed, nil
}

// DeleteFutureOccurrenceSnapshots mirrors Store.DeleteFutureOccurrenceSnapshots.
func (m *MockStateStore) DeleteFutureOccurrenceSnapshots(_ context.Context, blockID string, now time.Time) error {
	for key := range m.Snapshots {
		if key.blockName == blockID && time.Unix(0, key.startUnixNano).After(now) {
			delete(m.Snapshots, key)
			delete(m.SnapshotRecordedAt, key)
		}
	}
	return nil
}

// ReplaceOccurrenceHistory mirrors Store.ReplaceOccurrenceHistory: drops
// every existing entry for (blockName, occurrenceStart) and appends
// entries in its place.
func (m *MockStateStore) ReplaceOccurrenceHistory(_ context.Context, blockName string, occurrenceStart time.Time, entries []ScheduleHistoryEntry) error {
	kept := make([]ScheduleHistoryEntry, 0, len(m.History))
	for _, entry := range m.History {
		if entry.BlockName == blockName && entry.OccurrenceStart.Equal(occurrenceStart) {
			continue
		}
		kept = append(kept, entry)
	}
	m.History = append(kept, entries...)
	return nil
}
