package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// metaKeyLastApplyAt is the app_meta key holding the instant the most
// recent apply pushed at least one lineup to Tunarr (planned pushes and
// stale-channel clears alike). Stored as an RFC 3339 UTC string, not a
// TIMESTAMP column: app_meta.value is shared by every future key, so the
// column stays TEXT and each key owns its own encoding.
const metaKeyLastApplyAt = "last_apply_at"

// SetLastApplyAt records at (normalized to UTC, second precision) as the
// most recent apply instant, upserting the app_meta row. Written by
// service.Runner.applyChannels at the end of any apply that pushed at
// least one lineup -- deliberately NOT derived from applied_channels,
// whose rows are a tracking set that clearStaleChannels removes again
// (see migration 000009's comment for the full rationale).
func (s *Store) SetLastApplyAt(ctx context.Context, at time.Time) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO app_meta (key, value)
		VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		metaKeyLastApplyAt, at.UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("failed to record last apply time: %w", err)
	}
	return nil
}

// LastApplyAt returns when the most recent apply pushed a lineup to
// Tunarr, or nil when no apply has been recorded (a fresh install, or a
// database predating migration 000009 with no apply since). Feeds GET
// /status's last_applied_at field (internal/api/tunarr.go's GetStatus).
func (s *Store) LastApplyAt(ctx context.Context) (*time.Time, error) {
	var value string
	err := s.db.GetContext(ctx, &value,
		`SELECT value FROM app_meta WHERE key = ?`, metaKeyLastApplyAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read last apply time: %w", err)
	}
	at, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil, fmt.Errorf("failed to parse stored last apply time %q: %w", value, err)
	}
	at = at.UTC()
	return &at, nil
}

// metaKeyMaxPlanSeq is the app_meta key holding the highest plan-provenance
// sequence ever RESERVED -- not the highest still stored in a row.
//
// That distinction is the whole point. MaxPlanSeq derives a floor from
// MAX() over surviving snapshot and cursor rows, which is sound for one
// process and unsound for two: `serve` and a concurrent `generate --apply`
// (a deployment this store's WAL and _busy_timeout exist to support) can
// both read that floor before either commits, allocate from the same
// nanosecond neighborhood, and the second commit is then silently
// discarded by the provenance guard in Engine.syncPostStates -- a series
// cursor that stops advancing with nothing saying why.
const metaKeyMaxPlanSeq = "max_plan_seq"

// ReservePlanSeqBlock atomically reserves `count` consecutive plan
// sequences and returns the first and last of them. No other caller, in
// this process or any other, can be handed a sequence in that range.
//
// One statement, not a read-then-write transaction: transactions in this
// package are DEFERRED, so a read that later upgrades to a write can hit
// SQLITE_BUSY on the upgrade rather than waiting out the busy timeout --
// the classic SQLite deadlock. An upsert with RETURNING sidesteps it.
//
// The reserved range starts at max(stored, wall clock), so the sequence
// keeps the property its original design chose it for: it moves forward
// with real time, and cannot be dragged backwards by a stale row. Both
// sides of that comparison are CAST to INTEGER because app_meta.value is
// TEXT, where '9' > '10'.
func (s *Store) ReservePlanSeqBlock(ctx context.Context, count int64) (first, last int64, err error) {
	if count <= 0 {
		return 0, 0, fmt.Errorf("plan-sequence block size must be positive, got %d", count)
	}
	wall := time.Now().UnixNano()

	err = s.db.GetContext(ctx, &last, `
		INSERT INTO app_meta (key, value) VALUES (?, CAST(? AS TEXT))
		ON CONFLICT(key) DO UPDATE
		  SET value = CAST(MAX(CAST(app_meta.value AS INTEGER), ?) + ? AS TEXT)
		RETURNING CAST(value AS INTEGER)`,
		metaKeyMaxPlanSeq, wall+count, wall, count)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to reserve plan-sequence block: %w", err)
	}
	return last - count + 1, last, nil
}
