// Package store provides persistence for scheduling state.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/geekxflood/schedularr/internal/scheduler"
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
		last_aired DATETIME,
		run_count INTEGER NOT NULL DEFAULT 0,
		disabled BOOLEAN NOT NULL DEFAULT 0
	);
	CREATE TABLE IF NOT EXISTS schedule_history (
		program_id TEXT NOT NULL,
		channel_id TEXT NOT NULL,
		block_name TEXT NOT NULL,
		scheduled_at DATETIME NOT NULL,
		PRIMARY KEY (program_id, channel_id, scheduled_at)
	);
	CREATE INDEX IF NOT EXISTS idx_schedule_history_recent
		ON schedule_history (channel_id, scheduled_at);
	`
	if _, err := s.db.ExecContext(ctx, query); err != nil {
		return err
	}

	// Migrate existing tables to add new columns if they don't exist
	migrations := []string{
		`ALTER TABLE series_state ADD COLUMN run_count INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE series_state ADD COLUMN disabled BOOLEAN NOT NULL DEFAULT 0`,
	}

	for _, migration := range migrations {
		// Ignore errors for columns that already exist
		_, _ = s.db.ExecContext(ctx, migration)
	}

	return nil
}

// GetSeriesState retrieves the tracking state for a given show.
// If no state exists, it returns a default starting state (S01E01).
func (s *Store) GetSeriesState(ctx context.Context, showTitle string) (*scheduler.SeriesState, error) {
	var state scheduler.SeriesState
	err := s.db.GetContext(ctx, &state, `
		SELECT show_title, current_season, current_episode, completed, last_aired, run_count, disabled
		FROM series_state WHERE show_title = ?`, showTitle)

	if errors.Is(err, sql.ErrNoRows) {
		return &scheduler.SeriesState{
			ShowTitle:      showTitle,
			CurrentSeason:  1,
			CurrentEpisode: 1,
			Completed:      false,
			RunCount:       0,
			Disabled:       false,
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get series state: %w", err)
	}

	return &state, nil
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
		SELECT show_title, current_season, current_episode, completed, last_aired
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
			INSERT INTO series_state (show_title, current_season, current_episode, completed, last_aired)
			VALUES (:show_title, :current_season, :current_episode, :completed, :last_aired)
			ON CONFLICT(show_title) DO UPDATE SET
				current_season = excluded.current_season,
				current_episode = excluded.current_episode,
				completed = excluded.completed,
				last_aired = excluded.last_aired`, state); err != nil {
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
		SET current_season = 1, current_episode = 1, completed = 0, last_aired = NULL
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
