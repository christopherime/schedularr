// Package store provides persistence for scheduling state.
package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/geekxflood/schedularr/internal/scheduler"
	_ "github.com/mattn/go-sqlite3" // SQLite driver
)

// Store implements the persistence layer for Schedularr.
type Store struct {
	db *sql.DB
}

// New creates a new Store instance connected to the specified SQLite database.
func New(dsn string) (*Store, error) {
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Verify connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	s := &Store{db: db}
	if err := s.initSchema(context.Background()); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to init schema: %w", err)
	}

	return s, nil
}

// Close closes the database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) initSchema(ctx context.Context) error {
	query := `
	CREATE TABLE IF NOT EXISTS series_state (
		show_title TEXT PRIMARY KEY,
		current_season INTEGER NOT NULL DEFAULT 1,
		current_episode INTEGER NOT NULL DEFAULT 1,
		completed BOOLEAN NOT NULL DEFAULT 0,
		last_aired DATETIME
	);
	`
	_, err := s.db.ExecContext(ctx, query)
	return err
}

// GetSeriesState retrieves the tracking state for a given show.
// If no state exists, it returns a default starting state (S01E01).
func (s *Store) GetSeriesState(ctx context.Context, showTitle string) (*scheduler.SeriesState, error) {
	query := `
	SELECT show_title, current_season, current_episode, completed, last_aired
	FROM series_state
	WHERE show_title = ?
	`
	row := s.db.QueryRowContext(ctx, query, showTitle)

	var state scheduler.SeriesState
	var lastAired sql.NullTime

	err := row.Scan(&state.ShowTitle, &state.CurrentSeason, &state.CurrentEpisode, &state.Completed, &lastAired)
	if err == sql.ErrNoRows {
		// Return default state if not found
		return &scheduler.SeriesState{
			ShowTitle:      showTitle,
			CurrentSeason:  1,
			CurrentEpisode: 1,
			Completed:      false,
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan series state: %w", err)
	}

	if lastAired.Valid {
		state.LastAired = lastAired.Time
	}

	return &state, nil
}

// UpdateSeriesState updates or inserts the tracking state for a show.
func (s *Store) UpdateSeriesState(ctx context.Context, state *scheduler.SeriesState) error {
	query := `
	INSERT INTO series_state (show_title, current_season, current_episode, completed, last_aired)
	VALUES (?, ?, ?, ?, ?)
	ON CONFLICT(show_title) DO UPDATE SET
		current_season = excluded.current_season,
		current_episode = excluded.current_episode,
		completed = excluded.completed,
		last_aired = excluded.last_aired
	`
	_, err := s.db.ExecContext(ctx, query, state.ShowTitle, state.CurrentSeason, state.CurrentEpisode, state.Completed, state.LastAired)
	if err != nil {
		return fmt.Errorf("failed to update series state: %w", err)
	}
	return nil
}
