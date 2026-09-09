package store

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/christopherime/schedularr/internal/scheduler"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMigration000010_CreatesApplyRunTables(t *testing.T) {
	s, err := New(":memory:")
	require.NoError(t, err, "Failed to create store")
	defer s.Close()

	ctx := context.Background()

	for _, table := range []string{"apply_runs", "apply_run_warnings"} {
		var name string
		err := s.db.GetContext(ctx, &name,
			`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, table)
		require.NoErrorf(t, err, "table %q missing after migration", table)
	}

	// schedule_history gained run_id, defaulted so pre-migration rows stay valid.
	var count int
	require.NoError(t, s.db.GetContext(ctx, &count,
		`SELECT COUNT(*) FROM pragma_table_info('schedule_history') WHERE name = 'run_id'`))
	assert.Equal(t, 1, count, "schedule_history.run_id column")
}

func TestStore_ApplyRunLifecycle(t *testing.T) {
	s, err := New(":memory:")
	require.NoError(t, err, "Failed to create store")
	defer s.Close()

	ctx := context.Background()
	started := time.Now().Add(-time.Minute).UTC().Truncate(time.Second)

	run := ApplyRun{
		ID: "run-1", StartedAt: started, Source: ApplySourceCron,
		Scope: "", Days: 1, Status: ApplyStatusRunning,
	}
	require.NoError(t, s.StartApplyRun(ctx, run))

	// A run in flight is already listable -- that is the point of writing
	// the row before the apply rather than after it.
	runs, err := s.ListApplyRuns(ctx, started.Add(-time.Hour), 10)
	require.NoError(t, err)
	require.Len(t, runs, 1, "in-flight run should be listable")
	assert.Equal(t, ApplyStatusRunning, runs[0].Status)
	assert.Nil(t, runs[0].FinishedAt, "in-flight run has no finished_at")

	finished := started.Add(30 * time.Second)
	run.FinishedAt = &finished
	run.Status = ApplyStatusOK
	run.ChannelCount = 2
	run.SlotCount = 7
	run.Warnings = []ApplyRunWarning{{
		BlockName:         "Late Movie",
		OccurrenceStart:   started.Add(2 * time.Hour),
		BlockingBlockName: "News", ChannelID: "ch-1", DurationMinutes: 120,
	}}
	require.NoError(t, s.FinishApplyRun(ctx, run))

	runs, err = s.ListApplyRuns(ctx, started.Add(-time.Hour), 10)
	require.NoError(t, err)
	require.Len(t, runs, 1)

	got := runs[0]
	assert.Equal(t, ApplyStatusOK, got.Status)
	assert.Equal(t, 2, got.ChannelCount)
	assert.Equal(t, 7, got.SlotCount)
	require.NotNil(t, got.FinishedAt)
	assert.True(t, got.FinishedAt.Equal(finished), "FinishedAt round-trip")

	require.Len(t, got.Warnings, 1)
	assert.Equal(t, "News", got.Warnings[0].BlockingBlockName)
	assert.Equal(t, "ch-1", got.Warnings[0].ChannelID)
	assert.Equal(t, 120, got.Warnings[0].DurationMinutes)
	assert.Equal(t, "run-1", got.Warnings[0].RunID, "FinishApplyRun stamps the run ID")
}

func TestStore_ListApplyRuns_WindowAndLimit(t *testing.T) {
	s, err := New(":memory:")
	require.NoError(t, err, "Failed to create store")
	defer s.Close()

	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	for i, age := range []time.Duration{time.Minute, time.Hour, 48 * time.Hour} {
		require.NoError(t, s.StartApplyRun(ctx, ApplyRun{
			ID: fmt.Sprintf("run-%d", i), StartedAt: now.Add(-age),
			Source: ApplySourceUI, Status: ApplyStatusOK, Days: 7,
		}))
	}

	// The 48h-old run is outside a 24h window.
	runs, err := s.ListApplyRuns(ctx, now.Add(-24*time.Hour), 10)
	require.NoError(t, err)
	require.Len(t, runs, 2, "window excludes the 48h-old run")
	assert.Equal(t, "run-0", runs[0].ID, "newest first")
	assert.Equal(t, "run-1", runs[1].ID)

	runs, err = s.ListApplyRuns(ctx, now.Add(-72*time.Hour), 1)
	require.NoError(t, err)
	require.Len(t, runs, 1, "limit caps the page")
	assert.Equal(t, "run-0", runs[0].ID)
}

func TestStore_CleanupApplyRuns_TakesWarningsWithIt(t *testing.T) {
	s, err := New(":memory:")
	require.NoError(t, err, "Failed to create store")
	defer s.Close()

	ctx := context.Background()
	old := time.Now().Add(-100 * 24 * time.Hour).UTC().Truncate(time.Second)

	run := ApplyRun{ID: "old", StartedAt: old, Source: ApplySourceCLI, Status: ApplyStatusRunning}
	require.NoError(t, s.StartApplyRun(ctx, run))

	fin := old.Add(time.Second)
	run.FinishedAt = &fin
	run.Status = ApplyStatusOK
	run.Warnings = []ApplyRunWarning{{BlockName: "B", OccurrenceStart: old, BlockingBlockName: "A"}}
	require.NoError(t, s.FinishApplyRun(ctx, run))

	deleted, err := s.CleanupApplyRuns(ctx, 90*24*time.Hour)
	require.NoError(t, err)
	assert.Equal(t, int64(1), deleted, "one run pruned")

	var orphans int
	require.NoError(t, s.db.GetContext(ctx, &orphans, `SELECT COUNT(*) FROM apply_run_warnings`))
	assert.Zero(t, orphans, "foreign keys are off -- cleanup must delete warnings explicitly")
}

func TestStore_ScheduleHistory_RoundTripsRunID(t *testing.T) {
	s, err := New(":memory:")
	require.NoError(t, err, "Failed to create store")
	defer s.Close()

	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	require.NoError(t, s.RecordScheduleHistory(ctx, []scheduler.ScheduleHistoryEntry{{
		ProgramID: "p-1", ChannelID: "ch-1", BlockName: "Morning Cartoons",
		ScheduledAt: now, OccurrenceStart: now, Sequence: 0,
		DurationMs: 1_800_000, Title: "Pilot", Type: "episode", RunID: "run-xyz",
	}}))

	entries, err := s.ListScheduleHistory(ctx, now.Add(-time.Hour))
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "run-xyz", entries[0].RunID, "run_id must survive the round trip")
	assert.Equal(t, "Pilot", entries[0].Title)
}
