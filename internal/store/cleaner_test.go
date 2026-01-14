package store

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/geekxflood/schedularr/internal/scheduler"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCleaner(t *testing.T) {
	s, err := New(":memory:")
	require.NoError(t, err)
	defer s.Close()

	interval := 1 * time.Hour
	retention := 7 * 24 * time.Hour

	cleaner := NewCleaner(s, interval, retention)
	require.NotNil(t, cleaner)
	assert.Equal(t, interval, cleaner.interval)
	assert.Equal(t, retention, cleaner.retention)
	assert.False(t, cleaner.IsRunning())
}

func TestNewCleaner_WithLogger(t *testing.T) {
	s, err := New(":memory:")
	require.NoError(t, err)
	defer s.Close()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	cleaner := NewCleaner(s, time.Hour, time.Hour, WithLogger(logger))

	require.NotNil(t, cleaner)
	assert.Equal(t, logger, cleaner.logger)
}

func TestCleaner_RunOnce(t *testing.T) {
	s, err := New(":memory:")
	require.NoError(t, err)
	defer s.Close()

	ctx := context.Background()

	// Add some old entries
	entries := []scheduler.ScheduleHistoryEntry{
		{ProgramID: "prog-1", ChannelID: "ch-1", BlockName: "b1", ScheduledAt: time.Now().Add(-48 * time.Hour)},
		{ProgramID: "prog-2", ChannelID: "ch-1", BlockName: "b1", ScheduledAt: time.Now().Add(-72 * time.Hour)},
		{ProgramID: "prog-3", ChannelID: "ch-1", BlockName: "b1", ScheduledAt: time.Now()},
	}
	require.NoError(t, s.RecordScheduleHistory(ctx, entries))

	// Create cleaner with 24-hour retention
	cleaner := NewCleaner(s, time.Hour, 24*time.Hour)

	// Run cleanup once
	removed, err := cleaner.RunOnce(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(2), removed, "Expected 2 old entries to be removed")

	// Verify the recent entry still exists
	recent, err := s.WasRecentlyScheduled(ctx, "prog-3", "ch-1", 24*time.Hour)
	require.NoError(t, err)
	assert.True(t, recent, "Expected recent entry to still exist")

	// Verify old entries are gone
	old1, err := s.WasRecentlyScheduled(ctx, "prog-1", "ch-1", 96*time.Hour)
	require.NoError(t, err)
	assert.False(t, old1, "Expected old entry to be removed")
}

func TestCleaner_StartStop(t *testing.T) {
	s, err := New(":memory:")
	require.NoError(t, err)
	defer s.Close()

	// Use a short interval for testing
	cleaner := NewCleaner(s, 50*time.Millisecond, 24*time.Hour)

	ctx := context.Background()

	// Start the cleaner
	require.NoError(t, cleaner.Start(ctx))
	assert.True(t, cleaner.IsRunning())

	// Starting again should be a no-op
	require.NoError(t, cleaner.Start(ctx))
	assert.True(t, cleaner.IsRunning())

	// Let it run for a bit
	time.Sleep(100 * time.Millisecond)

	// Stop the cleaner
	cleaner.Stop()
	assert.False(t, cleaner.IsRunning())

	// Stopping again should be a no-op
	cleaner.Stop()
	assert.False(t, cleaner.IsRunning())
}

func TestCleaner_ContextCancellation(t *testing.T) {
	s, err := New(":memory:")
	require.NoError(t, err)
	defer s.Close()

	cleaner := NewCleaner(s, 50*time.Millisecond, 24*time.Hour)

	ctx, cancel := context.WithCancel(context.Background())

	// Start the cleaner
	require.NoError(t, cleaner.Start(ctx))
	assert.True(t, cleaner.IsRunning())

	// Cancel the context
	cancel()

	// Give it time to shut down
	time.Sleep(100 * time.Millisecond)

	// The internal goroutine should have exited, but IsRunning may still be true
	// because we didn't call Stop(). The doneChan should be closed though.
	select {
	case <-cleaner.doneChan:
		// Expected - goroutine exited
	case <-time.After(time.Second):
		t.Fatal("Expected cleaner goroutine to exit on context cancellation")
	}
}

func TestCleaner_PeriodicCleanup(t *testing.T) {
	s, err := New(":memory:")
	require.NoError(t, err)

	ctx := context.Background()

	// Add old entries
	entries := []scheduler.ScheduleHistoryEntry{
		{ProgramID: "old-prog", ChannelID: "ch-1", BlockName: "b1", ScheduledAt: time.Now().Add(-48 * time.Hour)},
	}
	require.NoError(t, s.RecordScheduleHistory(ctx, entries))

	// Verify entry exists before cleanup
	exists, err := s.WasRecentlyScheduled(ctx, "old-prog", "ch-1", 96*time.Hour)
	require.NoError(t, err)
	assert.True(t, exists, "Expected old entry to exist before cleanup")

	// Create cleaner with very short interval
	cleaner := NewCleaner(s, 50*time.Millisecond, 24*time.Hour)

	// Start the cleaner (it runs cleanup immediately on start)
	require.NoError(t, cleaner.Start(ctx))

	// Wait for initial cleanup to run
	time.Sleep(100 * time.Millisecond)

	// Stop the cleaner BEFORE checking the result to ensure no concurrent access
	cleaner.Stop()

	// Verify entry is cleaned up
	exists, err = s.WasRecentlyScheduled(ctx, "old-prog", "ch-1", 96*time.Hour)
	require.NoError(t, err)
	assert.False(t, exists, "Expected old entry to be cleaned up")

	// Now safe to close the store
	s.Close()
}

func TestCleaner_NoEntriesRemoved(t *testing.T) {
	s, err := New(":memory:")
	require.NoError(t, err)
	defer s.Close()

	ctx := context.Background()

	// Only add recent entries
	entries := []scheduler.ScheduleHistoryEntry{
		{ProgramID: "recent-prog", ChannelID: "ch-1", BlockName: "b1", ScheduledAt: time.Now()},
	}
	require.NoError(t, s.RecordScheduleHistory(ctx, entries))

	// Create cleaner with 24-hour retention
	cleaner := NewCleaner(s, time.Hour, 24*time.Hour)

	// Run cleanup once
	removed, err := cleaner.RunOnce(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(0), removed, "Expected no entries to be removed")

	// Verify entry still exists
	exists, err := s.WasRecentlyScheduled(ctx, "recent-prog", "ch-1", 24*time.Hour)
	require.NoError(t, err)
	assert.True(t, exists, "Expected recent entry to still exist")
}

func TestCleaner_EmptyDatabase(t *testing.T) {
	s, err := New(":memory:")
	require.NoError(t, err)
	defer s.Close()

	ctx := context.Background()

	// Create cleaner on empty database
	cleaner := NewCleaner(s, time.Hour, 24*time.Hour)

	// Run cleanup once
	removed, err := cleaner.RunOnce(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(0), removed, "Expected no entries to be removed from empty database")
}

func TestCleaner_RestartAfterStop(t *testing.T) {
	s, err := New(":memory:")
	require.NoError(t, err)
	defer s.Close()

	ctx := context.Background()

	cleaner := NewCleaner(s, 50*time.Millisecond, 24*time.Hour)

	// Start, stop, start again
	require.NoError(t, cleaner.Start(ctx))
	assert.True(t, cleaner.IsRunning())

	cleaner.Stop()
	assert.False(t, cleaner.IsRunning())

	// Start again
	require.NoError(t, cleaner.Start(ctx))
	assert.True(t, cleaner.IsRunning())

	cleaner.Stop()
	assert.False(t, cleaner.IsRunning())
}
