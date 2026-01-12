package scheduler

import (
	"context"
	"time"
)

// StateStore defines the interface for persisting series state.
type StateStore interface {
	GetSeriesState(ctx context.Context, showTitle string) (*SeriesState, error)
	UpdateSeriesState(ctx context.Context, state *SeriesState) error
	RecordScheduleHistory(ctx context.Context, entries []ScheduleHistoryEntry) error
	WasRecentlyScheduled(ctx context.Context, programID, channelID string, window time.Duration) (bool, error)
	CleanupScheduleHistory(ctx context.Context, window time.Duration) (int64, error)
}
