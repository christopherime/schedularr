package scheduler

import (
	"context"
	"errors"
	"sync"
	"testing"
)

// Two engines over ONE store, neither having committed -- the `serve` plus
// `generate --apply` shape this store's WAL exists to support. Every
// sequence handed out must be unique, or the later commit is silently
// discarded by the provenance guard in syncPostStates.
func TestNextPlanSeq_ConcurrentEnginesNeverShareASequence(t *testing.T) {
	t.Parallel()
	store := NewMockStateStore()
	ctx := context.Background()

	const engines = 6
	const perEngine = 64

	var mu sync.Mutex
	seen := make(map[int64]int)
	var wg sync.WaitGroup
	for range engines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			e := NewEngineWithOptions(ctx, nil, nil, store, EngineOptions{})
			for range perEngine {
				seq := e.nextPlanSeq()
				mu.Lock()
				seen[seq]++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	for seq, n := range seen {
		if n > 1 {
			t.Fatalf("sequence %d handed out %d times -- one run's post-state would be silently dropped", seq, n)
		}
	}
	if len(seen) != engines*perEngine {
		t.Fatalf("got %d distinct sequences, want %d", len(seen), engines*perEngine)
	}
}

// Sequences must be strictly increasing within one engine: the provenance
// guard compares them, so a repeat or a step backwards drops a post-state.
func TestNextPlanSeq_IsStrictlyIncreasing(t *testing.T) {
	t.Parallel()
	e := NewEngineWithOptions(context.Background(), nil, nil, NewMockStateStore(), EngineOptions{})
	prev := int64(0)
	for i := range 128 {
		seq := e.nextPlanSeq()
		if seq <= prev {
			t.Fatalf("allocation %d went backwards: %d after %d", i, seq, prev)
		}
		prev = seq
	}
}

// A reservation that cannot be written must NOT fail engine construction:
// that would take a scheduling run down over bookkeeping. It degrades to
// the wall clock, which is what this allocator did before reservations.
func TestNextPlanSeq_AFailedReservationFallsBackRatherThanFailing(t *testing.T) {
	t.Parallel()
	store := NewMockStateStore()
	store.PlanSeqReserveErr = errors.New("database is locked")

	e := NewEngineWithOptions(context.Background(), nil, nil, store, EngineOptions{})
	if e == nil {
		t.Fatal("engine construction failed on a reservation error")
	}
	prev := int64(0)
	for range 8 {
		seq := e.nextPlanSeq()
		if seq <= prev {
			t.Fatalf("fallback allocation went backwards: %d after %d", seq, prev)
		}
		prev = seq
	}
}

// Exhausting the reserved block must keep producing increasing sequences
// rather than repeating the last one -- the fallback is a degradation, not
// a failure.
func TestNextPlanSeq_SurvivesBlockExhaustion(t *testing.T) {
	t.Parallel()
	e := NewEngineWithOptions(context.Background(), nil, nil, NewMockStateStore(), EngineOptions{})

	// Drain the reservation, then keep going.
	e.planSeqMu.Lock()
	e.planSeqNext = e.planSeqLast
	e.planSeqMu.Unlock()

	prev := e.nextPlanSeq()
	for range 8 {
		seq := e.nextPlanSeq()
		if seq <= prev {
			t.Fatalf("post-exhaustion allocation went backwards: %d after %d", seq, prev)
		}
		prev = seq
	}
}
