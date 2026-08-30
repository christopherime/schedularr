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
