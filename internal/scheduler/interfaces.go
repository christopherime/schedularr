package scheduler

import (
	"context"
)

// StateStore defines the interface for persisting series state.
type StateStore interface {
	GetSeriesState(ctx context.Context, showTitle string) (*SeriesState, error)
	UpdateSeriesState(ctx context.Context, state *SeriesState) error
}
