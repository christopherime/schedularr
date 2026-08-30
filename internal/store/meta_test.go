package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestLastApplyAt covers GET /status's last_applied_at source: nil on a
// fresh store, a set/read round-trip (UTC-normalized, second precision),
// and overwrite-on-set. Unlike the applied_channels tracking set this key
// replaced as the source, the recorded instant survives channels being
// unmarked -- it only ever moves when SetLastApplyAt writes it.
func TestLastApplyAt(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	at, err := s.LastApplyAt(ctx)
	require.NoError(t, err)
	require.Nil(t, at, "fresh store should report no last apply")

	first := time.Date(2026, 8, 29, 6, 0, 0, 0, time.UTC)
	require.NoError(t, s.SetLastApplyAt(ctx, first))
	at, err = s.LastApplyAt(ctx)
	require.NoError(t, err)
	require.NotNil(t, at)
	require.True(t, at.Equal(first), "expected the recorded instant, got %v", at)

	// A later apply overwrites the single row (upsert, never a second row).
	second := first.Add(12 * time.Hour)
	require.NoError(t, s.SetLastApplyAt(ctx, second))
	at, err = s.LastApplyAt(ctx)
	require.NoError(t, err)
	require.NotNil(t, at)
	require.True(t, at.Equal(second), "expected the overwritten instant, got %v", at)

	// Unmarking applied channels must not disturb the recorded instant --
	// the exact failure mode of deriving it from the tracking set
	// (v0.5.0 review, MAJOR-1).
	require.NoError(t, s.MarkChannelApplied(ctx, "channel-a", second))
	require.NoError(t, s.UnmarkChannelApplied(ctx, "channel-a"))
	at, err = s.LastApplyAt(ctx)
	require.NoError(t, err)
	require.NotNil(t, at)
	require.True(t, at.Equal(second), "clearing tracked channels must not erase the last apply instant")
}

// TestLastApplyAt_NormalizesToUTC pins the storage encoding: a non-UTC
// input is written as (and read back in) UTC, same instant.
func TestLastApplyAt_NormalizesToUTC(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	zurich := time.FixedZone("CEST", 2*60*60)
	local := time.Date(2026, 8, 29, 8, 0, 0, 0, zurich) // 06:00 UTC
	require.NoError(t, s.SetLastApplyAt(ctx, local))

	at, err := s.LastApplyAt(ctx)
	require.NoError(t, err)
	require.NotNil(t, at)
	require.True(t, at.Equal(local), "expected the same instant back, got %v", at)
	require.Equal(t, time.UTC, at.Location(), "stored value must read back as UTC")
}
