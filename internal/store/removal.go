package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/christopherime/schedularr/internal/scheduler"
)

// ErrShowStillScheduled is returned by RemoveShow when a block still lists
// the show. Callers turn it into a 409 naming BlockNames.
var ErrShowStillScheduled = errors.New("show is still scheduled by a block")

// ShowStillScheduledError carries which blocks are keeping a show alive.
type ShowStillScheduledError struct {
	ShowTitle  string
	BlockNames []string
}

func (e *ShowStillScheduledError) Error() string {
	return fmt.Sprintf("%s: %q is listed by %v", ErrShowStillScheduled, e.ShowTitle, e.BlockNames)
}

func (e *ShowStillScheduledError) Unwrap() error { return ErrShowStillScheduled }

// RemovalReport counts what a removal actually touched, per table.
type RemovalReport struct {
	SeriesStates int64
	Airings      int64
	Snapshots    int64
	EmptiedSlots int64
}

// RemoveShow deletes every trace of one show's progression from the store,
// in ONE transaction: its series_state cursor, its airings in
// schedule_history, and its keys inside every occurrence snapshot.
//
// IT REFUSES WHILE A BLOCK STILL LISTS THE SHOW. backfillChainFromLive
// re-adds any block.Series title missing from the chain, seeded from a
// fabricated S01E01 -- so a removal that ran anyway would be undone by the
// next apply AND would reset the cursor it just deleted. Rewriting the
// operator's block specs from a history deletion was the alternative, and
// it is far too large a blast radius for one click: it can empty a block
// entirely. Refusing tells them exactly which blocks to edit first.
//
// SNAPSHOTS LOSE A KEY, NOT A ROW. A snapshot describes one occurrence of
// one block, which may have carried several shows; deleting the row would
// take the others with it. The two JSON maps are rewritten in place by a
// direct UPDATE rather than through SaveOccurrenceSnapshot, whose upsert
// sets plan_seq = excluded.plan_seq and would rewrite the provenance of an
// occurrence that already aired.
//
// A NULL post_state_json STAYS NULL. Turning it into "{}" flips
// replayAiredOccurrence from "re-seed the chain from live state" to "apply
// nothing", which silently changes behavior for every OTHER title in that
// occurrence.
//
// AN EMPTIED OCCURRENCE KEEPS ITS SENTINEL. Deleting the last airing of an
// occurrence would leave GetCommittedOccurrence answering "never planned",
// and the next apply would re-plan a slot that has already gone out --
// rewriting history rather than removing from it. The empty-ProgramID
// sentinel row keeps it "committed, produced nothing", which is what
// actually happened.
//
// No endpoint calls this yet; the UI that drives it is the next slice.
func (s *Store) RemoveShow(ctx context.Context, showTitle string) (RemovalReport, error) {
	var report RemovalReport
	if showTitle == "" {
		return report, errors.New("show title must not be empty")
	}

	blocking, err := s.blocksScheduling(ctx, showTitle)
	if err != nil {
		return report, err
	}
	if len(blocking) > 0 {
		return report, &ShowStillScheduledError{ShowTitle: showTitle, BlockNames: blocking}
	}

	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return report, fmt.Errorf("failed to start transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx, `DELETE FROM series_state WHERE show_title = ?`, showTitle)
	if err != nil {
		return report, fmt.Errorf("failed to remove series state for %q: %w", showTitle, err)
	}
	report.SeriesStates, _ = res.RowsAffected()

	// Non-empty on purpose: an empty show_title means "belongs to no show"
	// (a movie, or a row predating migration 000012), and matching it would
	// sweep every one of them.
	emptied, err := occurrencesLosingEveryAiring(ctx, tx, showTitle)
	if err != nil {
		return report, err
	}

	res, err = tx.ExecContext(ctx,
		`DELETE FROM schedule_history WHERE show_title = ? AND show_title <> ''`, showTitle)
	if err != nil {
		return report, fmt.Errorf("failed to remove airings for %q: %w", showTitle, err)
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

	report.Snapshots, err = stripShowFromSnapshots(ctx, tx, showTitle)
	if err != nil {
		return report, err
	}

	if err := tx.Commit(); err != nil {
		return report, fmt.Errorf("failed to commit removal of %q: %w", showTitle, err)
	}
	return report, nil
}

// blocksScheduling names every block whose spec lists showTitle.
func (s *Store) blocksScheduling(ctx context.Context, showTitle string) ([]string, error) {
	records, err := s.ListBlocks(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list blocks: %w", err)
	}
	var names []string
	for _, rec := range records {
		if blockReferencesShow(rec.Spec, showTitle) {
			names = append(names, rec.Name)
		}
	}
	return names, nil
}

// emptiedSlot identifies an occurrence about to lose its last airing.
//
// Only the GROUPED columns are selected. An aggregate over a DATETIME
// (MIN(scheduled_at)) comes back as a bare string because the driver's
// type affinity does not survive the aggregation, and scanning it into a
// time.Time fails at run time -- so the sentinel takes its stamps from
// occurrence_start instead, which is the instant that actually identifies
// the slot.
type emptiedSlot struct {
	ChannelID       string    `db:"channel_id"`
	BlockName       string    `db:"block_name"`
	OccurrenceStart time.Time `db:"occurrence_start"`
}

// occurrencesLosingEveryAiring finds the occurrences whose only remaining
// airings belong to showTitle, so the caller can keep them committed
// rather than letting them read as never planned.
func occurrencesLosingEveryAiring(ctx context.Context, tx *sqlx.Tx, showTitle string) ([]emptiedSlot, error) {
	var slots []emptiedSlot
	err := tx.SelectContext(ctx, &slots, `
		SELECT channel_id, block_name, occurrence_start
		FROM schedule_history
		GROUP BY block_name, occurrence_start, channel_id
		HAVING SUM(CASE WHEN show_title = ? THEN 0 ELSE 1 END) = 0`, showTitle)
	if err != nil {
		return nil, fmt.Errorf("failed to find occurrences losing every airing: %w", err)
	}
	return slots, nil
}

// stripShowFromSnapshots removes showTitle's key from every snapshot's two
// JSON maps, rewriting only the rows that actually carried it.
func stripShowFromSnapshots(ctx context.Context, tx *sqlx.Tx, showTitle string) (int64, error) {
	var rows []struct {
		BlockID         string    `db:"block_id"`
		OccurrenceStart time.Time `db:"occurrence_start"`
		SnapshotJSON    string    `db:"snapshot_json"`
		PostStateJSON   *string   `db:"post_state_json"`
	}
	if err := tx.SelectContext(ctx, &rows, `
		SELECT block_id, occurrence_start, snapshot_json, post_state_json
		FROM series_occurrence_snapshots`); err != nil {
		return 0, fmt.Errorf("failed to read occurrence snapshots: %w", err)
	}

	var touched int64
	for _, row := range rows {
		pre, preHad, err := withoutKey(row.SnapshotJSON, showTitle)
		if err != nil {
			return 0, fmt.Errorf("failed to rewrite snapshot for block %q: %w", row.BlockID, err)
		}
		post := row.PostStateJSON
		postHad := false
		if row.PostStateJSON != nil {
			rewritten, had, err := withoutKey(*row.PostStateJSON, showTitle)
			if err != nil {
				return 0, fmt.Errorf("failed to rewrite post-state for block %q: %w", row.BlockID, err)
			}
			post, postHad = &rewritten, had
		}
		if !preHad && !postHad {
			continue
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE series_occurrence_snapshots
			SET snapshot_json = ?, post_state_json = ?
			WHERE block_id = ? AND datetime(occurrence_start) = datetime(?)`,
			pre, post, row.BlockID, row.OccurrenceStart); err != nil {
			return 0, fmt.Errorf("failed to update snapshot for block %q: %w", row.BlockID, err)
		}
		touched++
	}
	return touched, nil
}

// withoutKey drops one key from a JSON object, reporting whether it was
// there. The map is re-marshaled rather than string-edited so a show title
// containing a quote or a dot cannot corrupt the document.
func withoutKey(raw, key string) (string, bool, error) {
	var m map[string]scheduler.SeriesStateSnapshot
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return "", false, fmt.Errorf("failed to decode snapshot map: %w", err)
	}
	if _, ok := m[key]; !ok {
		return raw, false, nil
	}
	delete(m, key)
	out, err := json.Marshal(m)
	if err != nil {
		return "", false, fmt.Errorf("failed to encode snapshot map: %w", err)
	}
	return string(out), true, nil
}
