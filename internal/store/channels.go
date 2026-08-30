package store

import (
	"context"
	"fmt"
	"time"
)

// MarkChannelApplied records that an apply pushed a lineup to channelID at
// the given time, upserting the applied_channels row. The set of marked
// channels is what clearStaleChannels (internal/service) consults to find
// channels whose blocks have all since been deleted or disabled -- see
// migration 000008's comment for the full rationale.
func (s *Store) MarkChannelApplied(ctx context.Context, channelID string, at time.Time) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO applied_channels (channel_id, applied_at)
		VALUES (?, ?)
		ON CONFLICT(channel_id) DO UPDATE SET applied_at = excluded.applied_at`,
		channelID, at.UTC())
	if err != nil {
		return fmt.Errorf("failed to mark channel applied: %w", err)
	}
	return nil
}

// ListAppliedChannels returns the channel IDs a previous apply pushed a
// lineup to and that have not been cleared since, ordered for determinism.
func (s *Store) ListAppliedChannels(ctx context.Context) ([]string, error) {
	var ids []string
	if err := s.db.SelectContext(ctx, &ids,
		`SELECT channel_id FROM applied_channels ORDER BY channel_id`); err != nil {
		return nil, fmt.Errorf("failed to list applied channels: %w", err)
	}
	return ids, nil
}

// UnmarkChannelApplied removes channelID from the applied set -- called
// after a stale channel's lineup has been cleared, so it is cleared exactly
// once and a later manual takeover of the channel in Tunarr is never
// clobbered by subsequent applies. Removing an ID that is not present is a
// no-op, not an error.
func (s *Store) UnmarkChannelApplied(ctx context.Context, channelID string) error {
	if _, err := s.db.ExecContext(ctx,
		`DELETE FROM applied_channels WHERE channel_id = ?`, channelID); err != nil {
		return fmt.Errorf("failed to unmark channel applied: %w", err)
	}
	return nil
}
