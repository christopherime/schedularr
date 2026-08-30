package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestAppliedChannels_MarkListUnmark covers the applied_channels round-trip
// clearStaleChannels (internal/service) depends on: mark is an upsert, list
// is deterministic, unmark is idempotent.
func TestAppliedChannels_MarkListUnmark(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	ids, err := s.ListAppliedChannels(ctx)
	require.NoError(t, err)
	require.Empty(t, ids)

	// channel-b is deliberately inserted BEFORE channel-a: raw rowid scan
	// order would then return [b, a], so the [a, b] assertion below only
	// holds while ListAppliedChannels actually carries its ORDER BY
	// channel_id -- the insert order is what pins the ordering clause.
	now := time.Now()
	require.NoError(t, s.MarkChannelApplied(ctx, "channel-b", now))
	require.NoError(t, s.MarkChannelApplied(ctx, "channel-a", now))
	// Re-marking an already-tracked channel upserts rather than erroring.
	require.NoError(t, s.MarkChannelApplied(ctx, "channel-b", now.Add(time.Hour)))

	ids, err = s.ListAppliedChannels(ctx)
	require.NoError(t, err)
	require.Equal(t, []string{"channel-a", "channel-b"}, ids)

	require.NoError(t, s.UnmarkChannelApplied(ctx, "channel-b"))
	// Unmarking an absent channel is a no-op, not an error.
	require.NoError(t, s.UnmarkChannelApplied(ctx, "channel-b"))

	ids, err = s.ListAppliedChannels(ctx)
	require.NoError(t, err)
	require.Equal(t, []string{"channel-a"}, ids)
}
