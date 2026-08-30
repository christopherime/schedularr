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

// TestLastAppliedAt covers GET /status's last_applied_at source: nil on a
// fresh store, the max applied_at across rows once applies are recorded,
// and nil again once every applied channel has been unmarked.
func TestLastAppliedAt(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	at, err := s.LastAppliedAt(ctx)
	require.NoError(t, err)
	require.Nil(t, at, "fresh store should report no last apply")

	earlier := time.Date(2026, 8, 29, 6, 0, 0, 0, time.UTC)
	later := earlier.Add(12 * time.Hour)
	require.NoError(t, s.MarkChannelApplied(ctx, "channel-a", later))
	require.NoError(t, s.MarkChannelApplied(ctx, "channel-b", earlier))

	at, err = s.LastAppliedAt(ctx)
	require.NoError(t, err)
	require.NotNil(t, at)
	require.True(t, at.Equal(later), "expected the max applied_at, got %v", at)

	require.NoError(t, s.UnmarkChannelApplied(ctx, "channel-a"))
	require.NoError(t, s.UnmarkChannelApplied(ctx, "channel-b"))
	at, err = s.LastAppliedAt(ctx)
	require.NoError(t, err)
	require.Nil(t, at, "no tracked channels means no last apply to report")
}
