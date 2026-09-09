package cmd

import (
	"context"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/christopherime/schedularr/internal/events"
)

func probeTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func waitForProbeEvent(t *testing.T, sub <-chan events.Event, within time.Duration) events.Event {
	t.Helper()
	select {
	case ev := <-sub:
		return ev
	case <-time.After(within):
		t.Fatal("no event arrived")
		return events.Event{}
	}
}

func TestTunarrProbe_PublishesOnlyOnTransition(t *testing.T) {
	hub := events.NewHub(16)
	sub, cancel := hub.Subscribe(context.Background(), 0)
	defer cancel()

	var reachable atomic.Bool
	reachable.Store(true)

	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	startTunarrProbe(ctx, probeDeps{
		Hub:      hub,
		Interval: 10 * time.Millisecond,
		Logger:   probeTestLogger(),
		Check:    func(context.Context) bool { return reachable.Load() },
	})

	// The first probe establishes the baseline and announces it once, so
	// a tab connecting before any flip still learns the reading.
	first := waitForProbeEvent(t, sub, 2*time.Second)
	require.Equal(t, events.StatusChanged, first.Name)
	data, ok := first.Data.(map[string]any)
	require.True(t, ok, "payload shape: %T", first.Data)
	assert.Equal(t, true, data["tunarr_reachable"])

	// Steady state across many intervals: silence.
	select {
	case ev := <-sub:
		t.Fatalf("published %v with no state change", ev.Data)
	case <-time.After(150 * time.Millisecond):
	}

	// A flip is news.
	reachable.Store(false)
	second := waitForProbeEvent(t, sub, 2*time.Second)
	flipped, ok := second.Data.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, false, flipped["tunarr_reachable"])
}

func TestTunarrProbe_StopsWithItsContext(t *testing.T) {
	hub := events.NewHub(16)
	sub, cancel := hub.Subscribe(context.Background(), 0)
	defer cancel()

	ctx, stop := context.WithCancel(context.Background())
	var checks atomic.Int64
	startTunarrProbe(ctx, probeDeps{
		Hub:      hub,
		Interval: 10 * time.Millisecond,
		Logger:   probeTestLogger(),
		Check: func(context.Context) bool {
			checks.Add(1)
			return true
		},
	})

	waitForProbeEvent(t, sub, 2*time.Second) // baseline
	stop()

	settled := checks.Load()
	time.Sleep(120 * time.Millisecond)
	assert.LessOrEqual(t, checks.Load(), settled+1, "the prober stops probing once its context is done")
}

func TestTunarrProbe_WithoutAHubDoesNothing(t *testing.T) {
	// serve wires a hub always; a nil one must simply be inert rather
	// than panicking on a background goroutine nobody is watching.
	assert.NotPanics(t, func() {
		startTunarrProbe(context.Background(), probeDeps{Check: func(context.Context) bool { return true }})
		startTunarrProbe(context.Background(), probeDeps{Hub: events.NewHub(1)})
	})
}
