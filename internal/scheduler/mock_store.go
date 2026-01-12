package scheduler

import (
	"context"
	"time"
)

// MockStateStore is a mock implementation of StateStore for testing.
type MockStateStore struct {
	States  map[string]*SeriesState
	History []ScheduleHistoryEntry
}

// NewMockStateStore creates a new in-memory mock store.
func NewMockStateStore() *MockStateStore {
	return &MockStateStore{
		States:  make(map[string]*SeriesState),
		History: nil,
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
