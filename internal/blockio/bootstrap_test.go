package blockio_test

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/christopherime/schedularr/internal/blockio"
	"github.com/christopherime/schedularr/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newBootstrapTestStore opens a fresh temp-file backed store for a single
// test.
func newBootstrapTestStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.New(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err, "failed to create test store")
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// testLogger returns a slog.Logger that writes to buf so tests can assert
// on log output ("Bootstrap logs its import loudly").
func testLogger(buf *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

const validScheduler = `blocks:
  - type: filter
    name: Morning Cartoons
    cron: "0 6 * * *"
    duration: 120
    channel_id: channel-1
    priority: 10
  - type: series
    name: Saturday Night
    cron: "0 20 * * 6"
    duration: 90
    channel_id: channel-2
    priority: 20
    series:
      - show_title: Show A
        episodes_per_block: 1
        start_season: 1
        start_episode: 1
        on_complete: continue
`

const invalidScheduler = `blocks:
  - type: filter
    name: Valid Block
    cron: "0 6 * * *"
    duration: 120
    channel_id: channel-1
  - type: filter
    name: Bad Block
    cron: "0 7 * * *"
    duration: -10
    channel_id: channel-2
`

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

func TestBootstrap_EmptyDBImportsFileAndIsIdempotent(t *testing.T) {
	s := newBootstrapTestStore(t)
	ctx := context.Background()
	path := writeFile(t, t.TempDir(), "scheduler.yaml", validScheduler)

	var buf bytes.Buffer
	count, err := blockio.Bootstrap(ctx, s, path, testLogger(&buf))
	require.NoError(t, err)
	assert.Equal(t, 2, count, "should import both blocks")
	assert.NotEmpty(t, buf.String(), "bootstrap should log its import")

	dbCount, err := s.CountBlocks(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(2), dbCount)

	records, err := s.ListBlocks(ctx)
	require.NoError(t, err)
	require.Len(t, records, 2)
	for _, rec := range records {
		assert.NotEmpty(t, rec.ID, "imported record should have a generated ID")
		assert.Len(t, rec.ID, 36, "ID should look like a uuid string")
		assert.True(t, rec.Enabled, "imported record should be enabled")
	}

	// Second call: DB is no longer empty, so it must be a no-op.
	buf.Reset()
	count2, err := blockio.Bootstrap(ctx, s, path, testLogger(&buf))
	require.NoError(t, err)
	assert.Equal(t, 0, count2, "second call must be idempotent no-op")

	dbCount2, err := s.CountBlocks(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(2), dbCount2, "block count must not change on second call")
}

func TestBootstrap_NonEmptyDBNeverImports(t *testing.T) {
	s := newBootstrapTestStore(t)
	ctx := context.Background()

	// Seed the DB directly so it's non-empty before bootstrap ever runs.
	seeded := &store.BlockRecord{
		ID:      "seed-1",
		Name:    "Pre-existing Block",
		Enabled: true,
		Spec:    filterBlock(),
	}
	require.NoError(t, s.CreateBlock(ctx, seeded))

	path := writeFile(t, t.TempDir(), "scheduler.yaml", validScheduler)

	var buf bytes.Buffer
	count, err := blockio.Bootstrap(ctx, s, path, testLogger(&buf))
	require.NoError(t, err)
	assert.Equal(t, 0, count, "must not import when DB is already non-empty")

	dbCount, err := s.CountBlocks(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(1), dbCount, "only the pre-existing block should remain")
}

func TestBootstrap_MissingFileReturnsZero(t *testing.T) {
	s := newBootstrapTestStore(t)
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "does-not-exist.yaml")

	var buf bytes.Buffer
	count, err := blockio.Bootstrap(ctx, s, path, testLogger(&buf))
	require.NoError(t, err)
	assert.Equal(t, 0, count)

	dbCount, err := s.CountBlocks(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(0), dbCount)
}

func TestBootstrap_InvalidFileErrorsWithoutPartialImport(t *testing.T) {
	s := newBootstrapTestStore(t)
	ctx := context.Background()
	path := writeFile(t, t.TempDir(), "scheduler.yaml", invalidScheduler)

	var buf bytes.Buffer
	_, err := blockio.Bootstrap(ctx, s, path, testLogger(&buf))
	require.Error(t, err, "invalid file must fail bootstrap")

	dbCount, err := s.CountBlocks(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(0), dbCount, "no blocks should be imported when the file is invalid")
}

func TestBootstrap_NilLoggerDoesNotPanic(t *testing.T) {
	s := newBootstrapTestStore(t)
	ctx := context.Background()
	path := writeFile(t, t.TempDir(), "scheduler.yaml", validScheduler)

	assert.NotPanics(t, func() {
		_, err := blockio.Bootstrap(ctx, s, path, nil)
		require.NoError(t, err)
	})
}
