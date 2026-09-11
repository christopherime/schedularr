package store

import (
	"context"
	"fmt"
	"time"
)

// Sources an apply can come from. Recorded verbatim in apply_runs.source
// and rendered as the RUNS pane's badge. An apply whose caller passed no
// source records ApplySourceUnknown rather than failing: a missing label
// is a code defect to notice later, never a reason to refuse to push a
// lineup.
const (
	ApplySourceUI      = "ui"
	ApplySourceCron    = "cron"
	ApplySourceCLI     = "cli"
	ApplySourceUnknown = "unknown"
)

// Statuses an apply run passes through. A row is written as
// ApplyStatusRunning before the apply touches Tunarr and finalized to OK
// or Error afterwards -- a row still at Running means the process died
// mid-apply, which the page shows rather than hides.
const (
	ApplyStatusRunning = "running"
	ApplyStatusOK      = "ok"
	ApplyStatusError   = "error"
)

// ApplyRun is one recorded apply. Scope is the channel ID an apply was
// narrowed to, or "" for every channel -- the same value
// service.Options.ChannelID carries. Warnings is hydrated by
// ListApplyRuns and written by FinishApplyRun; it is not a column.
type ApplyRun struct {
	ID           string     `db:"id"`
	StartedAt    time.Time  `db:"started_at"`
	FinishedAt   *time.Time `db:"finished_at"`
	Source       string     `db:"source"`
	Scope        string     `db:"scope"`
	Days         int        `db:"days"`
	Status       string     `db:"status"`
	ChannelCount int        `db:"channel_count"`
	SlotCount    int        `db:"slot_count"`
	Error        string     `db:"error"`

	Warnings []ApplyRunWarning `db:"-"`
}

// ApplyRunWarning is the persisted form of scheduler.Warning: one
// occurrence that was planned a slot and then dropped because a higher-
// (or equal-, first-come) priority occurrence on the same channel
// overlapped it. ChannelID and DurationMinutes are the enrichment the
// page needs to say what would have aired and where, without re-deriving
// the block spec.
type ApplyRunWarning struct {
	RunID             string    `db:"run_id"`
	BlockName         string    `db:"block_name"`
	OccurrenceStart   time.Time `db:"occurrence_start"`
	BlockingBlockName string    `db:"blocking_block_name"`
	ChannelID         string    `db:"channel_id"`
	DurationMinutes   int       `db:"duration_minutes"`
}

// StartApplyRun writes run as an in-flight record. Called before the
// apply pushes anything to Tunarr so that a crash mid-apply still leaves
// evidence the apply was attempted, and so the schedule_history rows
// written during the apply reference a row that already exists.
func (s *Store) StartApplyRun(ctx context.Context, run ApplyRun) error {
	if _, err := s.db.NamedExecContext(ctx, `
		INSERT INTO apply_runs (id, started_at, finished_at, source, scope, days, status, channel_count, slot_count, error)
		VALUES (:id, :started_at, :finished_at, :source, :scope, :days, :status, :channel_count, :slot_count, :error)`,
		run); err != nil {
		return fmt.Errorf("failed to insert apply run: %w", err)
	}
	return nil
}

// FinishApplyRun finalizes run's outcome and persists its warnings, in
// one transaction: a run whose counts landed but whose warnings did not
// would understate what the apply dropped.
func (s *Store) FinishApplyRun(ctx context.Context, run ApplyRun) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.NamedExecContext(ctx, `
		UPDATE apply_runs
		SET finished_at = :finished_at, status = :status, channel_count = :channel_count,
		    slot_count = :slot_count, error = :error
		WHERE id = :id`, run); err != nil {
		return fmt.Errorf("failed to finalize apply run: %w", err)
	}

	for _, w := range run.Warnings {
		w.RunID = run.ID
		if _, err := tx.NamedExecContext(ctx, `
			INSERT INTO apply_run_warnings (run_id, block_name, occurrence_start, blocking_block_name, channel_id, duration_minutes)
			VALUES (:run_id, :block_name, :occurrence_start, :blocking_block_name, :channel_id, :duration_minutes)`,
			w); err != nil {
			return fmt.Errorf("failed to insert apply run warning: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit apply run: %w", err)
	}
	return nil
}

// ListApplyRuns returns runs started at or after since, newest first, at
// most limit of them, each with its warnings hydrated. Warnings are read
// in one second query and bucketed by run ID rather than one query per
// run -- a 90-day window at the cron default is a few hundred runs, and
// N+1 queries over that is gratuitous.
func (s *Store) ListApplyRuns(ctx context.Context, since time.Time, limit int) ([]ApplyRun, error) {
	var runs []ApplyRun
	if err := s.db.SelectContext(ctx, &runs, `
		SELECT id, started_at, finished_at, source, scope, days, status, channel_count, slot_count, error
		FROM apply_runs
		WHERE datetime(started_at) >= datetime(?)
		ORDER BY datetime(started_at) DESC
		LIMIT ?`, since, limit); err != nil {
		return nil, fmt.Errorf("failed to list apply runs: %w", err)
	}
	if len(runs) == 0 {
		return runs, nil
	}

	var warnings []ApplyRunWarning
	if err := s.db.SelectContext(ctx, &warnings, `
		SELECT run_id, block_name, occurrence_start, blocking_block_name, channel_id, duration_minutes
		FROM apply_run_warnings
		WHERE run_id IN (SELECT id FROM apply_runs WHERE datetime(started_at) >= datetime(?) ORDER BY datetime(started_at) DESC LIMIT ?)
		ORDER BY occurrence_start`, since, limit); err != nil {
		return nil, fmt.Errorf("failed to list apply run warnings: %w", err)
	}

	byRun := make(map[string][]ApplyRunWarning, len(runs))
	for _, w := range warnings {
		byRun[w.RunID] = append(byRun[w.RunID], w)
	}
	for i := range runs {
		runs[i].Warnings = byRun[runs[i].ID]
	}
	return runs, nil
}

// CleanupApplyRuns deletes runs that started more than window ago, and
// their warnings. Warnings go first and explicitly: foreign keys are not
// enabled on this database (see sqliteDSNParams), so a declared cascade
// would be inert and the rows would leak. Returns the number of RUNS
// deleted -- the unit the operator counts in.
func (s *Store) CleanupApplyRuns(ctx context.Context, window time.Duration) (int64, error) {
	cutoff := time.Now().Add(-window)

	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to start transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `
		DELETE FROM apply_run_warnings
		WHERE run_id IN (SELECT id FROM apply_runs WHERE datetime(started_at) < datetime(?))`, cutoff); err != nil {
		return 0, fmt.Errorf("failed to cleanup apply run warnings: %w", err)
	}

	result, err := tx.ExecContext(ctx, `DELETE FROM apply_runs WHERE datetime(started_at) < datetime(?)`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("failed to cleanup apply runs: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to read cleanup result: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("failed to commit apply run cleanup: %w", err)
	}
	return affected, nil
}
