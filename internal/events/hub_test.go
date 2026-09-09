package events

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHub_FansOutToEverySubscriber(t *testing.T) {
	h := NewHub(8)
	ctx := context.Background()

	a, closeA := h.Subscribe(ctx, 0)
	defer closeA()
	b, closeB := h.Subscribe(ctx, 0)
	defer closeB()

	h.Publish(StatusChanged, map[string]any{"tunarr_reachable": true})

	for name, ch := range map[string]<-chan Event{"a": a, "b": b} {
		select {
		case ev := <-ch:
			assert.Equal(t, StatusChanged, ev.Name, "subscriber %s", name)
			assert.Equal(t, int64(1), ev.ID, "ids start at 1")
		case <-time.After(time.Second):
			t.Fatalf("subscriber %s received nothing", name)
		}
	}
}

func TestHub_SlowSubscriberNeverBlocksPublish(t *testing.T) {
	// Buffer of 1, and nobody reads: publishing must still return.
	h := NewHub(1)
	_, cancel := h.Subscribe(context.Background(), 0)
	defer cancel()

	done := make(chan struct{})
	go func() {
		for i := range 100 {
			h.Publish(Heartbeat, map[string]any{"n": i})
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Publish blocked on a subscriber that stopped reading")
	}
	assert.Equal(t, int64(100), h.LastID(), "every publish still took an id")
}

func TestHub_ResumeReplaysWhatWasMissed(t *testing.T) {
	h := NewHub(16)
	h.Publish(StatusChanged, map[string]any{"n": 1})
	h.Publish(StatusChanged, map[string]any{"n": 2})
	h.Publish(StatusChanged, map[string]any{"n": 3})

	ch, cancel := h.Subscribe(context.Background(), 1) // saw 1, wants 2 and 3
	defer cancel()

	var got []int64
	for range 2 {
		select {
		case ev := <-ch:
			got = append(got, ev.ID)
		case <-time.After(time.Second):
			t.Fatal("resume replayed nothing")
		}
	}
	assert.Equal(t, []int64{2, 3}, got)
}

func TestHub_ResumeBeyondTheRingReplaysOnlyWhatItHolds(t *testing.T) {
	// A gap the ring cannot fill is not silently papered over: the
	// subscriber gets only what survives, and refetches for the rest.
	h := NewHub(4)
	h.ringCap = 2
	for i := range 10 {
		h.Publish(StatusChanged, map[string]any{"n": i})
	}

	ch, cancel := h.Subscribe(context.Background(), 1)
	defer cancel()

	select {
	case ev := <-ch:
		assert.GreaterOrEqual(t, ev.ID, int64(9), "only what the ring still holds")
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected the ring's surviving entries to replay")
	}
}

func TestHub_UnsubscribeIsIdempotentAndRaceFree(t *testing.T) {
	h := NewHub(4)
	ch, cancel := h.Subscribe(context.Background(), 0)

	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() { defer wg.Done(); cancel() }()
	}
	wg.Wait()

	h.Publish(StatusChanged, nil) // must not panic on a closed subscriber
	_, open := <-ch
	require.False(t, open, "channel closes on unsubscribe")
}

func TestHub_ContextCancellationUnsubscribes(t *testing.T) {
	h := NewHub(4)
	ctx, cancel := context.WithCancel(context.Background())
	ch, _ := h.Subscribe(ctx, 0)

	cancel()
	assert.Eventually(t, func() bool {
		_, open := <-ch
		return !open
	}, time.Second, 10*time.Millisecond, "a cancelled context drops the subscriber")
}

func TestHub_PublishToNoSubscribersStillAdvancesIDs(t *testing.T) {
	h := NewHub(4)
	h.Publish(StatusChanged, nil)
	h.Publish(SeriesChanged, nil)
	assert.Equal(t, int64(2), h.LastID())
}
