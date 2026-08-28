// Package store provides persistence for scheduling state.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/christopherime/schedularr/internal/scheduler"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3" // SQLite driver
)

// Store implements the persistence layer for Schedularr.
type Store struct {
	db *sqlx.DB
}

// New creates a new Store instance connected to the specified SQLite database.
func New(dsn string) (*Store, error) {
	db, err := sqlx.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Verify connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// Run migrations
	if err := runMigrations(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	return &Store{db: db}, nil
}

// Close closes the database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// seriesStateRow queries the raw persisted row for showTitle, returning
// (nil, nil) if no row exists -- distinct from a query error. It backs both
// GetSeriesState (which fabricates a default S01E01 state as a convenience
// for scheduling callers that want a usable state for any show, tracked or
// not) and GetPersistedSeriesState (which reports ErrNotFound instead, for
// callers -- the state API's PatchSeriesState handler -- that need to
// distinguish "no row for this show" from "row exists and is at S01E01").
func (s *Store) seriesStateRow(ctx context.Context, showTitle string) (*scheduler.SeriesState, error) {
	var state scheduler.SeriesState
	err := s.db.GetContext(ctx, &state, `
		SELECT show_title, current_season, current_episode, completed, last_aired, run_count, disabled
		FROM series_state WHERE show_title = ?`, showTitle)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get series state: %w", err)
	}

	return &state, nil
}

// GetSeriesState retrieves the tracking state for a given show.
// If no state exists, it returns a default starting state (S01E01).
func (s *Store) GetSeriesState(ctx context.Context, showTitle string) (*scheduler.SeriesState, error) {
	state, err := s.seriesStateRow(ctx, showTitle)
	if err != nil {
		return nil, err
	}
	if state == nil {
		return &scheduler.SeriesState{
			ShowTitle:      showTitle,
			CurrentSeason:  1,
			CurrentEpisode: 1,
			Completed:      false,
			RunCount:       0,
			Disabled:       false,
		}, nil
	}

	return state, nil
}

// GetPersistedSeriesState retrieves the tracking state for a given show,
// returning ErrNotFound if no row is persisted for it. This is the
// existence-aware counterpart to GetSeriesState: GetSeriesState always
// succeeds with a fabricated S01E01 default for an untracked show (useful
// for the scheduler engine, which needs a starting point regardless), which
// makes it unsuitable for callers that must map "unknown show" to a 404 --
// currently the state API's PatchSeriesState handler (internal/api/state.go).
func (s *Store) GetPersistedSeriesState(ctx context.Context, showTitle string) (*scheduler.SeriesState, error) {
	state, err := s.seriesStateRow(ctx, showTitle)
	if err != nil {
		return nil, err
	}
	if state == nil {
		return nil, ErrNotFound
	}

	return state, nil
}

// UpdateSeriesState updates or inserts the tracking state for a show.
func (s *Store) UpdateSeriesState(ctx context.Context, state *scheduler.SeriesState) error {
	_, err := s.db.NamedExecContext(ctx, `
		INSERT INTO series_state (show_title, current_season, current_episode, completed, last_aired, run_count, disabled)
		VALUES (:show_title, :current_season, :current_episode, :completed, :last_aired, :run_count, :disabled)
		ON CONFLICT(show_title) DO UPDATE SET
			current_season = excluded.current_season,
			current_episode = excluded.current_episode,
			completed = excluded.completed,
			last_aired = excluded.last_aired,
			run_count = excluded.run_count,
			disabled = excluded.disabled`, state)
	if err != nil {
		return fmt.Errorf("failed to update series state: %w", err)
	}
	return nil
}

// RecordScheduleHistory persists schedule history entries for future filtering.
func (s *Store) RecordScheduleHistory(ctx context.Context, entries []scheduler.ScheduleHistoryEntry) error {
	if len(entries) == 0 {
		return nil
	}

	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for _, entry := range entries {
		if _, err := tx.NamedExecContext(ctx, `
			INSERT INTO schedule_history (program_id, channel_id, block_name, scheduled_at)
			VALUES (:program_id, :channel_id, :block_name, :scheduled_at)`, entry); err != nil {
			return fmt.Errorf("failed to insert schedule history: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit schedule history: %w", err)
	}

	return nil
}

// WasRecentlyScheduled returns true if a program was scheduled within the provided window.
func (s *Store) WasRecentlyScheduled(ctx context.Context, programID, channelID string, window time.Duration) (bool, error) {
	if programID == "" || channelID == "" {
		return false, errors.New("program ID and channel ID are required")
	}

	cutoff := time.Now().Add(-window)
	query := `
		SELECT 1
		FROM schedule_history
		WHERE program_id = ? AND channel_id = ? AND scheduled_at > ?
		LIMIT 1
	`
	row := s.db.QueryRowContext(ctx, query, programID, channelID, cutoff)

	var exists int
	if err := row.Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("failed to query schedule history: %w", err)
	}

	return true, nil
}

// CleanupScheduleHistory removes schedule history entries older than the window.
func (s *Store) CleanupScheduleHistory(ctx context.Context, window time.Duration) (int64, error) {
	cutoff := time.Now().Add(-window)
	result, err := s.db.ExecContext(ctx, `DELETE FROM schedule_history WHERE scheduled_at < ?`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("failed to cleanup schedule history: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to read cleanup result: %w", err)
	}

	return affected, nil
}

// ExportAllSeriesStates exports all series states to a slice for backup/export.
func (s *Store) ExportAllSeriesStates(ctx context.Context) ([]scheduler.SeriesState, error) {
	var states []scheduler.SeriesState
	err := s.db.SelectContext(ctx, &states, `
		SELECT show_title, current_season, current_episode, completed, last_aired, run_count, disabled
		FROM series_state ORDER BY show_title`)
	if err != nil {
		return nil, fmt.Errorf("failed to query series states: %w", err)
	}
	return states, nil
}

// ImportSeriesStates imports series states from a slice, replacing existing states.
func (s *Store) ImportSeriesStates(ctx context.Context, states []scheduler.SeriesState) error {
	if len(states) == 0 {
		return nil
	}

	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for _, state := range states {
		if _, err := tx.NamedExecContext(ctx, `
			INSERT INTO series_state (show_title, current_season, current_episode, completed, last_aired, run_count, disabled)
			VALUES (:show_title, :current_season, :current_episode, :completed, :last_aired, :run_count, :disabled)
			ON CONFLICT(show_title) DO UPDATE SET
				current_season = excluded.current_season,
				current_episode = excluded.current_episode,
				completed = excluded.completed,
				last_aired = excluded.last_aired,
				run_count = excluded.run_count,
				disabled = excluded.disabled`, state); err != nil {
			return fmt.Errorf("failed to import state for %s: %w", state.ShowTitle, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit import: %w", err)
	}

	return nil
}

// ResetSeriesState resets a series to its starting state (S01E01, not completed).
func (s *Store) ResetSeriesState(ctx context.Context, showTitle string) error {
	query := `
		UPDATE series_state
		SET current_season = 1, current_episode = 1, completed = 0, last_aired = NULL, run_count = 0, disabled = 0
		WHERE show_title = ?
	`
	result, err := s.db.ExecContext(ctx, query, showTitle)
	if err != nil {
		return fmt.Errorf("failed to reset series state: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check reset result: %w", err)
	}

	if affected == 0 {
		// Series doesn't exist in database, nothing to reset
		return nil
	}

	return nil
}

// SetSeriesState sets a series to a specific season and episode.
// If the series doesn't exist, it creates a new entry.
func (s *Store) SetSeriesState(ctx context.Context, showTitle string, season, episode int) error {
	if season < 1 || episode < 1 {
		return fmt.Errorf("season and episode must be >= 1, got S%02dE%02d", season, episode)
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO series_state (show_title, current_season, current_episode, completed, run_count, disabled)
		VALUES (?, ?, ?, 0, 0, 0)
		ON CONFLICT(show_title) DO UPDATE SET
			current_season = excluded.current_season,
			current_episode = excluded.current_episode,
			completed = 0,
			disabled = 0`, showTitle, season, episode)
	if err != nil {
		return fmt.Errorf("failed to set series state: %w", err)
	}

	return nil
}

// Backup creates a safe backup of the database to the specified path using VACUUM INTO.
func (s *Store) Backup(ctx context.Context, destPath string) error {
	// VACUUM INTO was introduced in SQLite 3.27.0
	// It creates a transactionally consistent copy of the database
	_, err := s.db.ExecContext(ctx, "VACUUM INTO ?", destPath)
	if err != nil {
		return fmt.Errorf("failed to backup database: %w", err)
	}
	return nil
}
