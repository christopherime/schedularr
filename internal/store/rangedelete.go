package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
)

// HistoryRange selects airings for counting or deletion: a time window,
// optionally narrowed to one channel, block, or show.
//
// The window is over scheduled_at, NOT occurrence_start. That is the axis
// GET /history filters on and the axis the AS-RUN pane groups its days
// by, so a dry run counts the rows the operator is actually looking at. A
// range measured on the other column would preview one set and delete
// another, and they would confirm against the wrong number.
//
// A zero From means "everything up to To"; a zero To means "everything
// from From onward". Both zero is refused rather than treated as "all
// history": a destructive default with no operator intent behind it is
// how an accident happens.
type HistoryRange struct {
	From      time.Time
	To        time.Time
	ChannelID string
	BlockName string
	ShowTitle string
}

// ErrUnboundedRange is returned when neither end of the window is set.
var ErrUnboundedRange = errors.New("a history range needs at least one of from or to")

// ErrBackwardRange is returned when From is after To.
var ErrBackwardRange = errors.New("a history range's from must not be after its to")

// rangeWhere builds the shared predicate, WITHOUT its leading WHERE.
//
// CountHistoryRange and DeleteHistoryRange both use it, and must: a dry
// run that counts different rows than the delete removes is worse than no
// dry run at all, because the operator confirms against it.
//
// Sentinel rows are excluded. A sentinel (empty program_id) is not an
// airing, it is the marker saying "this occurrence was committed and
// produced nothing" -- deleting it would make the next apply re-plan a
// slot that has already gone out, which is rewriting history rather than
// removing from it. See RemoveShow's doc comment.
//
// Instants are compared through datetime() on both sides for the reason
// ListScheduleHistory's comment gives: a stored instant keeps its
// writer's offset, and the operator's cutoff is arbitrary.
//
// The returned fragment is safe to concatenate into a query: every clause
// is a fixed literal chosen by this function, and every operator-supplied
// value goes into args as a bound parameter. Nothing from the caller ever
// reaches the SQL text.
func rangeWhere(r HistoryRange) (string, []any, error) {
	if r.From.IsZero() && r.To.IsZero() {
		return "", nil, ErrUnboundedRange
	}
	if !r.From.IsZero() && !r.To.IsZero() && r.From.After(r.To) {
		return "", nil, ErrBackwardRange
	}

	clauses := []string{"program_id <> ''"}
	var args []any

	if !r.From.IsZero() {
		clauses = append(clauses, "datetime(scheduled_at) >= datetime(?)")
		args = append(args, r.From)
	}
	if !r.To.IsZero() {
		clauses = append(clauses, "datetime(scheduled_at) < datetime(?)")
		args = append(args, r.To)
	}
	if r.ChannelID != "" {
		clauses = append(clauses, "channel_id = ?")
		args = append(args, r.ChannelID)
	}
	if r.BlockName != "" {
		clauses = append(clauses, "block_name = ?")
		args = append(args, r.BlockName)
	}
	if r.ShowTitle != "" {
		// Non-empty on purpose, exactly as RemoveShow does it: an empty
		// show_title means "belongs to no show" (a movie, or a row
		// predating migration 000012), and matching it would sweep every
		// one of them.
		clauses = append(clauses, "show_title = ? AND show_title <> ''")
		args = append(args, r.ShowTitle)
	}

	return strings.Join(clauses, " AND "), args, nil
}

// CountHistoryRange reports what DeleteHistoryRange would remove, without
// removing it. This is the dry run every destructive confirm is built on.
func (s *Store) CountHistoryRange(ctx context.Context, r HistoryRange) (RemovalReport, error) {
	var report RemovalReport

	where, args, err := rangeWhere(r)
	if err != nil {
		return report, err
	}

	if err := s.db.GetContext(ctx, &report.Airings,
		`SELECT COUNT(*) FROM schedule_history WHERE `+where, args...); err != nil {
		return report, fmt.Errorf("failed to count airings in range: %w", err)
	}

	slots, err := occurrencesEmptiedBy(ctx, s.db, where, args)
	if err != nil {
		return report, err
	}
	report.EmptiedSlots = int64(len(slots))

	return report, nil
}

// DeleteHistoryRange removes the airings a range selects, in ONE
// transaction, keeping every occurrence it empties committed.
//
// IT DOES NOT TOUCH SNAPSHOTS OR CURSORS. A date range is not a statement
// about any particular show, and a snapshot describes one occurrence of
// one block that may have carried several. Removing a show's progression
// is RemoveShow's job, and it scrubs the snapshot keys itself; snapshots
// outside that flow age out on maintenance.snapshot_retention.
//
// AN EMPTIED OCCURRENCE KEEPS ITS SENTINEL, for the reason RemoveShow's
// doc comment gives: without it GetCommittedOccurrence answers "never
// planned" and the next apply re-plans a slot that already went out.
func (s *Store) DeleteHistoryRange(ctx context.Context, r HistoryRange) (RemovalReport, error) {
	var report RemovalReport

	where, args, err := rangeWhere(r)
	if err != nil {
		return report, err
	}

	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return report, fmt.Errorf("failed to start transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Found BEFORE the delete: afterwards the rows that would identify
	// them are gone.
	emptied, err := occurrencesEmptiedBy(ctx, tx, where, args)
	if err != nil {
		return report, err
	}

	res, err := tx.ExecContext(ctx, `DELETE FROM schedule_history WHERE `+where, args...)
	if err != nil {
		return report, fmt.Errorf("failed to delete airings in range: %w", err)
	}
	report.Airings, _ = res.RowsAffected()

	for _, slot := range emptied {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO schedule_history (program_id, channel_id, block_name, scheduled_at, occurrence_start, sequence, duration_ms, title, show_title, type, run_id)
			VALUES ('', ?, ?, ?, ?, 0, 0, '', '', '', '')`,
			slot.ChannelID, slot.BlockName, slot.OccurrenceStart, slot.OccurrenceStart); err != nil {
			return report, fmt.Errorf("failed to keep occurrence %q committed: %w", slot.BlockName, err)
		}
	}
	report.EmptiedSlots = int64(len(emptied))

	if err := tx.Commit(); err != nil {
		return report, fmt.Errorf("failed to commit range deletion: %w", err)
	}
	return report, nil
}

// queryer is the shared read surface of *sqlx.DB and *sqlx.Tx, so the dry
// run and the delete can run the same query against their own handle.
type queryer interface {
	SelectContext(ctx context.Context, dest any, query string, args ...any) error
}

// occurrencesEmptiedBy finds the occurrences whose every remaining airing
// matches where -- the ones that would be left with nothing.
//
// The args are bound twice because the predicate appears twice: once to
// decide which rows count as "being removed", and once more is not needed
// here since the outer query has no filter of its own. Sentinels already
// fail `program_id <> ”`, so an occurrence holding only a sentinel is not
// reported as emptied and does not collect a second one.
func occurrencesEmptiedBy(ctx context.Context, q queryer, where string, args []any) ([]emptiedSlot, error) {
	var slots []emptiedSlot
	query := `
		SELECT channel_id, block_name, occurrence_start
		FROM schedule_history
		GROUP BY block_name, occurrence_start, channel_id
		HAVING SUM(CASE WHEN ` + where + ` THEN 0 ELSE 1 END) = 0`
	if err := q.SelectContext(ctx, &slots, query, args...); err != nil {
		return nil, fmt.Errorf("failed to find occurrences emptied by range: %w", err)
	}
	return slots, nil
}

var _ queryer = (*sqlx.DB)(nil)
var _ queryer = (*sqlx.Tx)(nil)
