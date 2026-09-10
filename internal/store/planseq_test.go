package store_test

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReservePlanSeqBlock_HandsOutDisjointRanges(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()

	f1, l1, err := st.ReservePlanSeqBlock(ctx, 16)
	require.NoError(t, err)
	f2, l2, err := st.ReservePlanSeqBlock(ctx, 16)
	require.NoError(t, err)

	assert.Equal(t, int64(16), l1-f1+1)
	assert.Equal(t, int64(16), l2-f2+1)
	assert.Greater(t, f2, l1, "the second block overlaps the first")
}

// The failure this whole task exists for: two engines over one store,
// neither having committed. Every sequence handed out must be unique, or
// the later commit is silently dropped by the provenance guard.
func TestReservePlanSeqBlock_ConcurrentReserversNeverOverlap(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()

	const reservers = 8
	const perReserver = 8
	const blockSize = 4

	var mu sync.Mutex
	seen := make(map[int64]int)
	var wg sync.WaitGroup
	for i := 0; i < reservers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < perReserver; j++ {
				first, last, err := st.ReservePlanSeqBlock(ctx, blockSize)
				if err != nil {
					t.Errorf("reserve: %v", err)
					return
				}
				mu.Lock()
				for seq := first; seq <= last; seq++ {
					seen[seq]++
				}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	for seq, n := range seen {
		if n > 1 {
			t.Fatalf("sequence %d reserved %d times -- two runs can silently drop each other's post-state", seq, n)
		}
	}
	assert.Len(t, seen, reservers*perReserver*blockSize)
}

// app_meta.value is TEXT, where '9' > '10'. A reservation that compared
// without CAST would let the high-water mark walk backwards.
func TestReservePlanSeqBlock_NeverGoesBackwards(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()

	_, last1, err := st.ReservePlanSeqBlock(ctx, 8)
	require.NoError(t, err)
	first2, _, err := st.ReservePlanSeqBlock(ctx, 8)
	require.NoError(t, err)
	assert.Greater(t, first2, last1)
}

func TestReservePlanSeqBlock_RejectsANonPositiveCount(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	_, _, err := st.ReservePlanSeqBlock(context.Background(), 0)
	assert.Error(t, err)
}
