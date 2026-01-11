package scheduler

import (
	"context"
)

// MockStateStore is a mock implementation of StateStore for testing.
type MockStateStore struct {
	States map[string]*SeriesState
}

// NewMockStateStore creates a new in-memory mock store.
func NewMockStateStore() *MockStateStore {
	return &MockStateStore{
		States: make(map[string]*SeriesState),
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