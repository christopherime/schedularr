// Package store provides persistence for scheduling state.
package store

import (
	"context"
	"database/sql"
	"errors"
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

// RecordScheduleHistory persists schedule history entries for future filtering.
func (s *Store) RecordScheduleHistory(ctx context.Context, entries []scheduler.ScheduleHistoryEntry) error {
	if len(entries) == 0 {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO schedule_history (program_id, channel_id, block_name, scheduled_at)
		VALUES (?, ?, ?, ?)
	`)
	if err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("failed to prepare schedule history insert: %w", err)
	}
	defer stmt.Close()

	for _, entry := range entries {
		if _, err := stmt.ExecContext(ctx, entry.ProgramID, entry.ChannelID, entry.BlockName, entry.ScheduledAt); err != nil {
			_ = tx.Rollback()
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
	query := `
		SELECT show_title, current_season, current_episode, completed, last_aired
		FROM series_state
		ORDER BY show_title
	`
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query series states: %w", err)
	}
	defer rows.Close()

	var states []scheduler.SeriesState
	for rows.Next() {
		var state scheduler.SeriesState
		var lastAired sql.NullTime

		if err := rows.Scan(&state.ShowTitle, &state.CurrentSeason, &state.CurrentEpisode, &state.Completed, &lastAired); err != nil {
			return nil, fmt.Errorf("failed to scan series state: %w", err)
		}

		if lastAired.Valid {
			state.LastAired = lastAired.Time
		}

		states = append(states, state)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating series states: %w", err)
	}

	return states, nil
}

// ImportSeriesStates imports series states from a slice, replacing existing states.
func (s *Store) ImportSeriesStates(ctx context.Context, states []scheduler.SeriesState) error {
	if len(states) == 0 {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO series_state (show_title, current_season, current_episode, completed, last_aired)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(show_title) DO UPDATE SET
			current_season = excluded.current_season,
			current_episode = excluded.current_episode,
			completed = excluded.completed,
			last_aired = excluded.last_aired
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare import statement: %w", err)
	}
	defer stmt.Close()

	for _, state := range states {
		if _, err := stmt.ExecContext(ctx, state.ShowTitle, state.CurrentSeason, state.CurrentEpisode, state.Completed, state.LastAired); err != nil {
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
