# Memory / `/history/` Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
>
> **UI tasks (10–14) additionally REQUIRE the `impeccable` skill** — CLAUDE.md rule 4: front-end work on the Hugo web UI goes through it. Prose written for docs/CHANGELOG gets cleaned with `stop-slop`.

**Goal:** Persist every apply as a durable run record with its conflict warnings, enrich the airing history with titles and run provenance, and land one searchable `/history/` page with TRACKED / AS-RUN / RUNS panes that replaces `/series/` and `/dashboard/` outright.

**Architecture:** Engine-and-contract first, UI second. A new migration adds `apply_runs` + `apply_run_warnings` and a `run_id` column on `schedule_history`; `service.Runner` mints a run ID per apply, writes a `running` row before it touches Tunarr, and finalizes it (status, counts, warnings, error) after — so UI, CLI, and cron-loop applies all land in the same table. The engine stamps the run ID onto the history rows it commits, making "which apply put this on air" a join instead of a guess. Retention splits into three per-table knobs in the same breakable-schema window. On the front end, one `/history/` route with three panes behind a shared filter toolbar absorbs the whole of `/series/` and `/dashboard/`, both of which are deleted with their layouts, content, TS, and CSS.

**Tech Stack:** Go 1.2x (stdlib `testing`, `-race`), `sqlx` + `mattn/go-sqlite3`, `golang-migrate` (embedded `iofs` source), `google/uuid`, oapi-codegen v2 (chi-server), CUE (config schema), Hugo + hand-written CSS + Alpine.js (vendored) + TypeScript (`node --test` for unit tests).

**Spec:**
- `docs/superpowers/specs/2026-08-30-v0.5-web-overhaul-design.md` — §2 (IA), §3.7 (the Log page's records, filters, states), §9.2 (the Memory slice's engine-first ordering and gate), §4 (component system deltas).
- `docs/superpowers/specs/2026-08-30-v1-station-terminology-media-history-design.md` — §3.1 (verified current state), §3.2 (the `/history/` merge and the three panes), §3.4 (retention), §5 (operator answers Q4, Q5, Q9, Q10).
- `docs/roadmap.md` — the "Then — Memory, landing as `/history/`" entry is this slice's scope statement.

## Global Constraints

- **Lean and clean (CLAUDE.md rule 1).** `/series/` and `/dashboard/` are **deleted**, not redirected: layouts, content front matter, page TS, their CSS rules, their `/kit/` fixtures, and every reference in docs — in the same commits that replace them. No deprecation aliases, no redirect stubs. The 404 page carries the nav (spec §3.2).
- **Docs in the same commit (CLAUDE.md rule 3).** README, CLAUDE.md, AGENTS.md, GEMINI.md, `docs/`, `mkdocs.yml` nav, `web/DESIGN.md`, CHANGELOG, `docs/roadmap.md`.
- **`make test` green before every commit** (CLAUDE.md rule 2). `make lint` green before the release commit.
- **Lint limits:** cyclomatic ≤ 15, cognitive ≤ 20, nesting ≤ 5, results ≤ 3, **arguments ≤ 5**. The argument limit is why `NewRunner` grows an options struct in Task 3 rather than three more parameters.
- **Error wrapping:** `fmt.Errorf("failed to X: %w", err)` — every returned error.
- **Logging:** `slog`, snake_case keys.
- **Blocked packages:** `github.com/pkg/errors`, `logrus`, `crypto/md5`, `crypto/sha1`, `io/ioutil`, `gopkg.in/yaml.v1`, `gopkg.in/yaml.v2`.
- **Contract-first:** `api/openapi.yaml` is edited before any handler; `make generate` regenerates `internal/api/gen/`, `make web-types` regenerates `web/assets/ts/gen/types.d.ts`. Generated files are never hand-edited.
- **CSP:** `style-src 'self'` — inline `style` attributes are silently dropped. Geometry goes through `element.style.setProperty` (CSSOM). No CDN, no external fetch.
- **Retention defaults (Q10, split per table):** `history_retention: "168h"`, `snapshot_retention: "168h"`, `apply_run_retention: "2160h"` (90 days, spec §3.7).
- **Run sources:** exactly `ui`, `cron`, `cli`; an unlabeled caller records `unknown` rather than failing the apply.
- **Out of scope for this slice** (they belong to the History-desk power-tools slice): `DELETE /history`, per-title removal, the STORAGE strip, the `app_meta.max_plan_seq` floor, bulk cursor operations, YAML import/export on the page. Nothing in this plan may implement them.

---

## File Structure

**Created**

| File | Responsibility |
| --- | --- |
| `internal/store/migrations/000010_apply_runs.up.sql` / `.down.sql` | `apply_runs`, `apply_run_warnings`, `schedule_history.run_id` |
| `internal/store/applies.go` | `ApplyRun`, `ApplyRunWarning`, start/finish/list/cleanup |
| `internal/store/applies_test.go` | store-level tests for the above |
| `internal/api/applies.go` | `GET /applies` handler + wire conversion |
| `internal/api/applies_test.go` | handler tests |
| `web/content/history/_index.md` | Hugo section front matter for `/history/` |
| `web/layouts/history/list.html` | the page: toolbar, segmented control, three panes |
| `web/assets/ts/pages/history.ts` | page bundle: filters, pane routing, three renderers |
| `web/tests/history.test.ts` | unit tests for the page's pure helpers |

**Modified**

| File | Change |
| --- | --- |
| `cmd/schema/config.cue` | `#MaintenanceConfig` splits into three retention knobs |
| `internal/config/config.go` | `MaintenanceSnapshotRetention`, `MaintenanceApplyRunRetention` |
| `internal/scheduler/history.go` | `ScheduleHistoryEntry.RunID` |
| `internal/scheduler/engine.go` | `Warning` gains `ChannelID`/`DurationMinutes`; `EngineOptions` gains `RunID`/`SnapshotRetention`; `Commit` stamps and prunes accordingly |
| `internal/scheduler/interfaces.go` | (doc only) `CleanupOccurrenceSnapshots` window is now its own knob |
| `internal/store/sqlite.go` | `run_id` in the `schedule_history` insert/select column lists |
| `internal/service/schedule.go` | `RunnerOptions`, `Options.Source`, run recording around the apply |
| `cmd/serve.go` | cron tick passes `Source: service.SourceCron`; `NewRunner` call site |
| `cmd/generate.go` | CLI passes `Source: service.SourceCLI`; `NewRunner` call site; cleanup uses the new knobs |
| `internal/api/schedule.go` | UI apply passes `Source: service.SourceUI` |
| `internal/api/history.go` | enriched `historyEntryToGen` |
| `api/openapi.yaml` | `/applies`; `ApplyRun`, `ApplyRunWarning`; enriched `HistoryEntry`; extended `Warning` |
| `web/layouts/partials/nav.html` | `GUIDE · BLOCKS · HISTORY` |
| `web/assets/css/main.css` | history page rules in, series/dashboard rules out |
| `web/layouts/kit/list.html`, `web/assets/ts/pages/kit.ts` | history fixtures in, series/dashboard fixtures out |
| `web/DESIGN.md` | the page, its idioms, WCAG evidence for new pairings |
| docs + CHANGELOG + roadmap + `mkdocs.yml` | see Task 15 |

**Deleted**

`web/content/series/_index.md`, `web/content/dashboard/_index.md`, `web/layouts/series/list.html`, `web/layouts/dashboard/list.html`, `web/assets/ts/pages/series.ts`, `web/assets/ts/pages/dashboard.ts`, and every CSS rule and doc reference that named them.

---

# Phase A — Contract and engine

## Task 1: The migration

**Files:**
- Create: `internal/store/migrations/000010_apply_runs.up.sql`
- Create: `internal/store/migrations/000010_apply_runs.down.sql`
- Test: `internal/store/applies_test.go`

**Interfaces:**
- Consumes: nothing (first task).
- Produces: tables `apply_runs (id, started_at, finished_at, source, scope, days, status, channel_count, slot_count, error)`, `apply_run_warnings (run_id, block_name, occurrence_start, blocking_block_name, channel_id, duration_minutes)`, and column `schedule_history.run_id TEXT NOT NULL DEFAULT ''`.

**Context an implementer needs:** migrations are embedded via `//go:embed migrations/*.sql` in `internal/store/migrate.go` and applied by `golang-migrate` on every `store.New`. There is **no `PRAGMA foreign_keys=ON`** (see `sqliteDSNParams` in `internal/store/sqlite.go`) — foreign keys are inert in this database, so `apply_run_warnings` carries no `REFERENCES` clause and cleanup deletes warnings explicitly rather than relying on a cascade. Existing migrations document *why* the table exists in a header comment; match that.

- [ ] **Step 1: Write the failing test**

Create `internal/store/applies_test.go`:

```go
package store

import (
	"context"
	"path/filepath"
	"testing"
)

// newTestStore opens a Store against a fresh temp-dir database, running
// every migration. Mirrors the helper idiom already used in
// internal/store/sqlite_test.go.
func newAppliesTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestMigration000010_CreatesApplyRunTables(t *testing.T) {
	s := newAppliesTestStore(t)
	ctx := context.Background()

	for _, table := range []string{"apply_runs", "apply_run_warnings"} {
		var name string
		err := s.db.GetContext(ctx, &name,
			`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, table)
		if err != nil {
			t.Fatalf("table %q missing after migration: %v", table, err)
		}
	}

	// schedule_history gained run_id, defaulted so pre-migration rows stay valid.
	var count int
	if err := s.db.GetContext(ctx, &count,
		`SELECT COUNT(*) FROM pragma_table_info('schedule_history') WHERE name = 'run_id'`); err != nil {
		t.Fatalf("failed to inspect schedule_history: %v", err)
	}
	if count != 1 {
		t.Fatalf("schedule_history.run_id: got %d matching columns, want 1", count)
	}
}
```

If `newTestStore`/`Close` already exist with these exact semantics in `sqlite_test.go`, reuse them and delete the local helper rather than shadowing.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -race ./internal/store -run TestMigration000010 -v`
Expected: FAIL — `table "apply_runs" missing after migration`.

- [ ] **Step 3: Write the migration**

`internal/store/migrations/000010_apply_runs.up.sql`:

```sql
-- apply_runs is the durable record of every apply -- UI, cron loop, and
-- CLI alike. Until now an apply left only a server-side log line and the
-- schedule_history rows it produced, so "why didn't X air last Tuesday"
-- had no answer that survived a restart: the conflict warnings that
-- explain a dropped occurrence were computed on every generate and
-- thrown away with the response.
--
-- A row is written with status 'running' BEFORE the apply touches
-- Tunarr, and finalized afterwards. A row left at 'running' therefore
-- means the process died mid-apply, which is information worth keeping
-- rather than a defect to hide.
--
-- finished_at is nullable for exactly that reason; every other column is
-- NOT NULL with a default so a partially-written run still reads cleanly.
CREATE TABLE apply_runs (
    id            TEXT PRIMARY KEY,
    started_at    TIMESTAMP NOT NULL,
    finished_at   TIMESTAMP,
    source        TEXT NOT NULL,
    scope         TEXT NOT NULL DEFAULT '',
    days          INTEGER NOT NULL DEFAULT 0,
    status        TEXT NOT NULL,
    channel_count INTEGER NOT NULL DEFAULT 0,
    slot_count    INTEGER NOT NULL DEFAULT 0,
    error         TEXT NOT NULL DEFAULT ''
);

-- The page reads runs newest-first inside a window; this is the only
-- access pattern.
CREATE INDEX idx_apply_runs_started_at ON apply_runs (started_at DESC);

-- apply_run_warnings persists what scheduler.Warning used to carry only
-- in a response body: one occurrence that was planned a slot and then
-- dropped by conflict resolution. No REFERENCES clause -- foreign keys
-- are not enabled on this database (store.sqliteDSNParams), so a
-- declared cascade would be silently inert; CleanupApplyRuns deletes
-- these rows explicitly instead.
CREATE TABLE apply_run_warnings (
    run_id              TEXT NOT NULL,
    block_name          TEXT NOT NULL,
    occurrence_start    TIMESTAMP NOT NULL,
    blocking_block_name TEXT NOT NULL,
    channel_id          TEXT NOT NULL DEFAULT '',
    duration_minutes    INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX idx_apply_run_warnings_run_id ON apply_run_warnings (run_id);

-- run_id ties an aired programme back to the apply that put it there.
-- Defaulted to '' rather than NULL so rows written before this migration
-- read as "no run recorded" without every consumer handling a NULL --
-- runs cannot be backfilled, which the page's empty state says plainly.
ALTER TABLE schedule_history ADD COLUMN run_id TEXT NOT NULL DEFAULT '';
```

`internal/store/migrations/000010_apply_runs.down.sql`:

```sql
DROP INDEX IF EXISTS idx_apply_run_warnings_run_id;
DROP TABLE IF EXISTS apply_run_warnings;
DROP INDEX IF EXISTS idx_apply_runs_started_at;
DROP TABLE IF EXISTS apply_runs;
-- DROP COLUMN needs SQLite >= 3.35; mattn/go-sqlite3 bundles well past it.
ALTER TABLE schedule_history DROP COLUMN run_id;
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test -race ./internal/store -run TestMigration000010 -v`
Expected: PASS.

- [ ] **Step 5: Verify every existing store test still passes**

Run: `go test -race ./internal/store`
Expected: `ok` — this is the spec's "migration tests from every 0.x database" gate in its cheapest form; the suite opens fresh databases and runs the full chain 000001→000010.

- [ ] **Step 6: Commit**

```bash
git add internal/store/migrations/000010_apply_runs.up.sql \
        internal/store/migrations/000010_apply_runs.down.sql \
        internal/store/applies_test.go
git commit -m "feat(store): add apply_runs, apply_run_warnings and schedule_history.run_id"
```

---

## Task 2: Store methods for apply runs

**Files:**
- Create: `internal/store/applies.go`
- Modify: `internal/store/applies_test.go` (add cases)
- Modify: `internal/store/sqlite.go` — `RecordScheduleHistory` and `ListScheduleHistory` column lists

**Interfaces:**
- Consumes: the tables from Task 1.
- Produces:
  - `store.ApplyRun{ID string; StartedAt time.Time; FinishedAt *time.Time; Source, Scope string; Days int; Status string; ChannelCount, SlotCount int; Error string; Warnings []ApplyRunWarning}`
  - `store.ApplyRunWarning{RunID, BlockName string; OccurrenceStart time.Time; BlockingBlockName, ChannelID string; DurationMinutes int}`
  - constants `ApplySourceUI/Cron/CLI/Unknown`, `ApplyStatusRunning/OK/Error`
  - `(*Store).StartApplyRun(ctx, ApplyRun) error`
  - `(*Store).FinishApplyRun(ctx, ApplyRun) error`
  - `(*Store).ListApplyRuns(ctx, since time.Time, limit int) ([]ApplyRun, error)`
  - `(*Store).CleanupApplyRuns(ctx, window time.Duration) (int64, error)`

- [ ] **Step 1: Write the failing tests**

Append to `internal/store/applies_test.go`:

```go
func TestStore_ApplyRunLifecycle(t *testing.T) {
	s := newAppliesTestStore(t)
	ctx := context.Background()
	started := time.Now().Add(-time.Minute).UTC().Truncate(time.Second)

	run := ApplyRun{
		ID: "run-1", StartedAt: started, Source: ApplySourceCron,
		Scope: "", Days: 1, Status: ApplyStatusRunning,
	}
	if err := s.StartApplyRun(ctx, run); err != nil {
		t.Fatalf("StartApplyRun: %v", err)
	}

	// A run in flight is already listable -- that is the point of writing
	// the row before the apply rather than after it.
	runs, err := s.ListApplyRuns(ctx, started.Add(-time.Hour), 10)
	if err != nil {
		t.Fatalf("ListApplyRuns: %v", err)
	}
	if len(runs) != 1 || runs[0].Status != ApplyStatusRunning {
		t.Fatalf("in-flight run: got %+v, want one running row", runs)
	}
	if runs[0].FinishedAt != nil {
		t.Fatalf("in-flight run: FinishedAt = %v, want nil", runs[0].FinishedAt)
	}

	finished := started.Add(30 * time.Second)
	run.FinishedAt = &finished
	run.Status = ApplyStatusOK
	run.ChannelCount = 2
	run.SlotCount = 7
	run.Warnings = []ApplyRunWarning{{
		RunID: "run-1", BlockName: "Late Movie",
		OccurrenceStart: started.Add(2 * time.Hour),
		BlockingBlockName: "News", ChannelID: "ch-1", DurationMinutes: 120,
	}}
	if err := s.FinishApplyRun(ctx, run); err != nil {
		t.Fatalf("FinishApplyRun: %v", err)
	}

	runs, err = s.ListApplyRuns(ctx, started.Add(-time.Hour), 10)
	if err != nil {
		t.Fatalf("ListApplyRuns after finish: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("got %d runs, want 1", len(runs))
	}
	got := runs[0]
	if got.Status != ApplyStatusOK || got.ChannelCount != 2 || got.SlotCount != 7 {
		t.Fatalf("finalized run: got %+v", got)
	}
	if got.FinishedAt == nil || !got.FinishedAt.Equal(finished) {
		t.Fatalf("FinishedAt: got %v, want %v", got.FinishedAt, finished)
	}
	if len(got.Warnings) != 1 || got.Warnings[0].BlockingBlockName != "News" {
		t.Fatalf("warnings: got %+v, want one News warning", got.Warnings)
	}
	if got.Warnings[0].DurationMinutes != 120 || got.Warnings[0].ChannelID != "ch-1" {
		t.Fatalf("warning enrichment: got %+v", got.Warnings[0])
	}
}

func TestStore_ListApplyRuns_WindowAndLimit(t *testing.T) {
	s := newAppliesTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	for i, age := range []time.Duration{time.Minute, time.Hour, 48 * time.Hour} {
		run := ApplyRun{
			ID: fmt.Sprintf("run-%d", i), StartedAt: now.Add(-age),
			Source: ApplySourceUI, Status: ApplyStatusOK, Days: 7,
		}
		if err := s.StartApplyRun(ctx, run); err != nil {
			t.Fatalf("StartApplyRun %d: %v", i, err)
		}
	}

	// The 48h-old run is outside a 24h window.
	runs, err := s.ListApplyRuns(ctx, now.Add(-24*time.Hour), 10)
	if err != nil {
		t.Fatalf("ListApplyRuns: %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("window: got %d runs, want 2", len(runs))
	}
	// Newest first.
	if runs[0].ID != "run-0" || runs[1].ID != "run-1" {
		t.Fatalf("order: got %s, %s -- want run-0, run-1", runs[0].ID, runs[1].ID)
	}

	runs, err = s.ListApplyRuns(ctx, now.Add(-72*time.Hour), 1)
	if err != nil {
		t.Fatalf("ListApplyRuns limited: %v", err)
	}
	if len(runs) != 1 || runs[0].ID != "run-0" {
		t.Fatalf("limit: got %+v, want just run-0", runs)
	}
}

func TestStore_CleanupApplyRuns_TakesWarningsWithIt(t *testing.T) {
	s := newAppliesTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	old := now.Add(-100 * 24 * time.Hour)

	run := ApplyRun{ID: "old", StartedAt: old, Source: ApplySourceCLI, Status: ApplyStatusOK}
	if err := s.StartApplyRun(ctx, run); err != nil {
		t.Fatalf("StartApplyRun: %v", err)
	}
	run.Status = ApplyStatusOK
	fin := old.Add(time.Second)
	run.FinishedAt = &fin
	run.Warnings = []ApplyRunWarning{{RunID: "old", BlockName: "B", OccurrenceStart: old, BlockingBlockName: "A"}}
	if err := s.FinishApplyRun(ctx, run); err != nil {
		t.Fatalf("FinishApplyRun: %v", err)
	}

	deleted, err := s.CleanupApplyRuns(ctx, 90*24*time.Hour)
	if err != nil {
		t.Fatalf("CleanupApplyRuns: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("deleted: got %d, want 1", deleted)
	}

	var orphans int
	if err := s.db.GetContext(ctx, &orphans, `SELECT COUNT(*) FROM apply_run_warnings`); err != nil {
		t.Fatalf("count warnings: %v", err)
	}
	if orphans != 0 {
		t.Fatalf("orphan warnings: got %d, want 0 -- foreign keys are off, cleanup must delete them", orphans)
	}
}
```

Add `"fmt"` and `"time"` to the test file's imports.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test -race ./internal/store -run 'TestStore_ApplyRun|TestStore_ListApplyRuns|TestStore_CleanupApplyRuns' -v`
Expected: FAIL — compile error, `s.StartApplyRun undefined`.

- [ ] **Step 3: Write the implementation**

Create `internal/store/applies.go`:

```go
package store

import (
	"context"
	"fmt"
	"time"
)

// Sources an apply can come from. Recorded verbatim in apply_runs.source
// and rendered as the RUNS pane's badge. An apply whose caller passed no
// source records ApplySourceUnknown rather than failing: a missing label
// is a code defect to notice later, never a reason to refuse to push a
// lineup.
const (
	ApplySourceUI      = "ui"
	ApplySourceCron    = "cron"
	ApplySourceCLI     = "cli"
	ApplySourceUnknown = "unknown"
)

// Statuses an apply run passes through. A row is written as
// ApplyStatusRunning before the apply touches Tunarr and finalized to OK
// or Error afterwards -- a row still at Running means the process died
// mid-apply, which the page shows rather than hides.
const (
	ApplyStatusRunning = "running"
	ApplyStatusOK      = "ok"
	ApplyStatusError   = "error"
)

// ApplyRun is one recorded apply. Scope is the channel ID an apply was
// narrowed to, or "" for every channel -- the same value
// service.Options.ChannelID carries. Warnings is hydrated by
// ListApplyRuns and written by FinishApplyRun; it is not a column.
type ApplyRun struct {
	ID           string     `db:"id"`
	StartedAt    time.Time  `db:"started_at"`
	FinishedAt   *time.Time `db:"finished_at"`
	Source       string     `db:"source"`
	Scope        string     `db:"scope"`
	Days         int        `db:"days"`
	Status       string     `db:"status"`
	ChannelCount int        `db:"channel_count"`
	SlotCount    int        `db:"slot_count"`
	Error        string     `db:"error"`

	Warnings []ApplyRunWarning `db:"-"`
}

// ApplyRunWarning is the persisted form of scheduler.Warning: one
// occurrence that was planned a slot and then dropped because a higher-
// (or equal-, first-come) priority occurrence on the same channel
// overlapped it. ChannelID and DurationMinutes are the enrichment the
// page needs to say what would have aired and where, without re-deriving
// the block spec.
type ApplyRunWarning struct {
	RunID             string    `db:"run_id"`
	BlockName         string    `db:"block_name"`
	OccurrenceStart   time.Time `db:"occurrence_start"`
	BlockingBlockName string    `db:"blocking_block_name"`
	ChannelID         string    `db:"channel_id"`
	DurationMinutes   int       `db:"duration_minutes"`
}

// StartApplyRun writes run as an in-flight record. Called before the
// apply pushes anything to Tunarr so that a crash mid-apply still leaves
// evidence the apply was attempted, and so schedule_history rows written
// during the apply reference a row that already exists.
func (s *Store) StartApplyRun(ctx context.Context, run ApplyRun) error {
	if _, err := s.db.NamedExecContext(ctx, `
		INSERT INTO apply_runs (id, started_at, finished_at, source, scope, days, status, channel_count, slot_count, error)
		VALUES (:id, :started_at, :finished_at, :source, :scope, :days, :status, :channel_count, :slot_count, :error)`,
		run); err != nil {
		return fmt.Errorf("failed to insert apply run: %w", err)
	}
	return nil
}

// FinishApplyRun finalizes run's outcome and persists its warnings, in
// one transaction: a run whose counts landed but whose warnings did not
// would understate what the apply dropped.
func (s *Store) FinishApplyRun(ctx context.Context, run ApplyRun) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.NamedExecContext(ctx, `
		UPDATE apply_runs
		SET finished_at = :finished_at, status = :status, channel_count = :channel_count,
		    slot_count = :slot_count, error = :error
		WHERE id = :id`, run); err != nil {
		return fmt.Errorf("failed to finalize apply run: %w", err)
	}

	for _, w := range run.Warnings {
		w.RunID = run.ID
		if _, err := tx.NamedExecContext(ctx, `
			INSERT INTO apply_run_warnings (run_id, block_name, occurrence_start, blocking_block_name, channel_id, duration_minutes)
			VALUES (:run_id, :block_name, :occurrence_start, :blocking_block_name, :channel_id, :duration_minutes)`,
			w); err != nil {
			return fmt.Errorf("failed to insert apply run warning: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit apply run: %w", err)
	}
	return nil
}

// ListApplyRuns returns runs started at or after since, newest first, at
// most limit of them, each with its warnings hydrated. Warnings are read
// in one second query and bucketed by run ID rather than one query per
// run -- a 90-day window at the cron default is a few hundred runs, and
// N+1 queries over that is gratuitous.
func (s *Store) ListApplyRuns(ctx context.Context, since time.Time, limit int) ([]ApplyRun, error) {
	var runs []ApplyRun
	if err := s.db.SelectContext(ctx, &runs, `
		SELECT id, started_at, finished_at, source, scope, days, status, channel_count, slot_count, error
		FROM apply_runs
		WHERE started_at >= ?
		ORDER BY started_at DESC
		LIMIT ?`, since, limit); err != nil {
		return nil, fmt.Errorf("failed to list apply runs: %w", err)
	}
	if len(runs) == 0 {
		return runs, nil
	}

	var warnings []ApplyRunWarning
	if err := s.db.SelectContext(ctx, &warnings, `
		SELECT run_id, block_name, occurrence_start, blocking_block_name, channel_id, duration_minutes
		FROM apply_run_warnings
		WHERE run_id IN (SELECT id FROM apply_runs WHERE started_at >= ? ORDER BY started_at DESC LIMIT ?)
		ORDER BY occurrence_start`, since, limit); err != nil {
		return nil, fmt.Errorf("failed to list apply run warnings: %w", err)
	}

	byRun := make(map[string][]ApplyRunWarning, len(runs))
	for _, w := range warnings {
		byRun[w.RunID] = append(byRun[w.RunID], w)
	}
	for i := range runs {
		runs[i].Warnings = byRun[runs[i].ID]
	}
	return runs, nil
}

// CleanupApplyRuns deletes runs that started more than window ago, and
// their warnings. Warnings go first and explicitly: foreign keys are not
// enabled on this database (see sqliteDSNParams), so a declared cascade
// would be inert and the rows would leak. Returns the number of RUNS
// deleted -- the unit the operator counts in.
func (s *Store) CleanupApplyRuns(ctx context.Context, window time.Duration) (int64, error) {
	cutoff := time.Now().Add(-window)

	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to start transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `
		DELETE FROM apply_run_warnings
		WHERE run_id IN (SELECT id FROM apply_runs WHERE started_at < ?)`, cutoff); err != nil {
		return 0, fmt.Errorf("failed to cleanup apply run warnings: %w", err)
	}

	result, err := tx.ExecContext(ctx, `DELETE FROM apply_runs WHERE started_at < ?`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("failed to cleanup apply runs: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to read cleanup result: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("failed to commit apply run cleanup: %w", err)
	}
	return affected, nil
}
```

- [ ] **Step 4: Thread `run_id` through the schedule_history column lists**

In `internal/store/sqlite.go`, `RecordScheduleHistory`'s INSERT becomes:

```go
		if _, err := tx.NamedExecContext(ctx, `
			INSERT INTO schedule_history (program_id, channel_id, block_name, scheduled_at, occurrence_start, sequence, duration_ms, title, type, run_id)
			VALUES (:program_id, :channel_id, :block_name, :scheduled_at, :occurrence_start, :sequence, :duration_ms, :title, :type, :run_id)`, entry); err != nil {
```

and `ListScheduleHistory`'s SELECT becomes:

```go
	err := s.db.SelectContext(ctx, &entries, `
		SELECT program_id, channel_id, block_name, scheduled_at, occurrence_start, sequence, duration_ms, title, type, run_id
		FROM schedule_history
		WHERE scheduled_at >= ?
		ORDER BY scheduled_at DESC`, since)
```

Leave `GetCommittedOccurrence`'s projection alone — it reconstructs a `tunarr.Program` and has no use for the run. `ScheduleHistoryEntry.RunID` is added in Task 4; this step will not compile until then, so do Step 4 and Task 4's Step 3 together, or stage this edit and run the tests after Task 4. The commit below covers both.

- [ ] **Step 5: Run the tests**

Run: `go test -race ./internal/store -v -run 'ApplyRun'`
Expected: PASS (after `ScheduleHistoryEntry.RunID` exists — see Task 4).

- [ ] **Step 6: Commit**

```bash
git add internal/store/applies.go internal/store/applies_test.go internal/store/sqlite.go
git commit -m "feat(store): record, list and prune apply runs with their warnings"
```

---

## Task 3: Split retention per table

**Files:**
- Modify: `cmd/schema/config.cue:75-85`
- Modify: `internal/config/config.go:76-84`
- Modify: `internal/config/config_test.go`
- Modify: `configs/` sample config(s) and `config.yaml` template output if `cmd/config generate` embeds the keys

**Interfaces:**
- Consumes: nothing.
- Produces: `config.MaintenanceHistoryRetention(cfg) time.Duration` (unchanged name and meaning), `config.MaintenanceSnapshotRetention(cfg) time.Duration`, `config.MaintenanceApplyRunRetention(cfg) time.Duration`.

**Why now (Q10, answered 2026-09-08):** one knob governs `schedule_history` *and* `series_occurrence_snapshots` today, and `apply_runs` would have arrived with a hardcoded 90 days. The operator chose to split while the config schema is still breakable. This is a **breaking config change** only in the sense that new keys appear with defaults; an existing `config.yaml` that sets only `history_retention` keeps working and gets `168h`/`2160h` for the other two.

- [ ] **Step 1: Write the failing test**

In `internal/config/config_test.go`, extend `TestMaintenanceConfig`:

```go
	if got := MaintenanceSnapshotRetention(cfg); got != 168*time.Hour {
		t.Errorf("MaintenanceSnapshotRetention() = %v, want 168h", got)
	}
	if got := MaintenanceApplyRunRetention(cfg); got != 2160*time.Hour {
		t.Errorf("MaintenanceApplyRunRetention() = %v, want 2160h (90 days)", got)
	}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test -race ./internal/config -run TestMaintenanceConfig -v`
Expected: FAIL — `undefined: MaintenanceSnapshotRetention`.

- [ ] **Step 3: Update the CUE schema**

`cmd/schema/config.cue`, replacing `#MaintenanceConfig`:

```cue
// MaintenanceConfig defines background maintenance task settings.
//
// Retention is per table (v0.5.7): each of the three tables that grows
// with time is pruned on its own clock, because they answer different
// questions over different horizons -- history and snapshots feed the
// engine's replay and dedup machinery (a week is plenty), apply runs
// feed the operator's "why didn't X air last Tuesday" (a quarter is not).
#MaintenanceConfig: {
	// How long to keep schedule history (e.g., "168h" for 7 days). Also
	// governs the engine's in-memory recency-dedup window, and how much
	// of the persisted schedule_history table GET /history?days=N
	// (api/openapi.yaml, 1..90) can actually return -- see
	// internal/service/schedule.go's NewRunner doc comment.
	history_retention: string | *"168h"

	// How long to keep series occurrence snapshots. A snapshot for an
	// occurrence outside the SCHEDULE-HISTORY window can no longer be
	// replayed (its schedule_history rows are gone), so setting this
	// longer than history_retention keeps rows that serve no purpose.
	snapshot_retention: string | *"168h"

	// How long to keep apply-run records and their warnings. Defaults to
	// 90 days: a run card is the only durable answer to "why didn't X
	// air last Tuesday", and it cannot be backfilled.
	apply_run_retention: string | *"2160h"

	// Whether to enable automatic cleanup
	cleanup_enabled: bool | *true
}
```

- [ ] **Step 4: Add the accessors**

`internal/config/config.go`, after `MaintenanceHistoryRetention`:

```go
// MaintenanceSnapshotRetention returns how long series occurrence
// snapshots are kept. Split from history_retention in v0.5.7 so the two
// tables prune on their own clocks.
func MaintenanceSnapshotRetention(cfg *Config) time.Duration {
	return cfg.GetDuration("maintenance.snapshot_retention")
}

// MaintenanceApplyRunRetention returns how long apply-run records and
// their warnings are kept (default 90 days).
func MaintenanceApplyRunRetention(cfg *Config) time.Duration {
	return cfg.GetDuration("maintenance.apply_run_retention")
}
```

- [ ] **Step 5: Run the tests**

Run: `go test -race ./internal/config ./internal/cueconfig ./cmd && make validate`
Expected: PASS, and `make validate` accepts every config in `configs/` and `testdata/` unchanged (the new keys have defaults).

- [ ] **Step 6: Commit**

```bash
git add cmd/schema/config.cue internal/config/config.go internal/config/config_test.go
git commit -m "feat(config): split maintenance retention per table (history, snapshots, apply runs)"
```

---

## Task 4: Enrich the domain types

**Files:**
- Modify: `internal/scheduler/history.go:33-42` (`ScheduleHistoryEntry`)
- Modify: `internal/scheduler/engine.go:107-111` (`Warning`) and its construction site in `resolveConflicts`
- Test: `internal/scheduler/engine_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `ScheduleHistoryEntry.RunID string` (db tag `run_id`); `Warning.ChannelID string`, `Warning.DurationMinutes int`.

- [ ] **Step 1: Write the failing test**

Add to `internal/scheduler/engine_test.go`. Find the existing test that asserts on conflict warnings (grep for `Warnings` in that file) and add a sibling that checks the two new fields; if there is an existing table-driven conflict test, extend its expectations instead of adding a duplicate. A standalone version:

```go
func TestGenerateForTimeRange_WarningCarriesChannelAndDuration(t *testing.T) {
	// Two blocks on the SAME channel whose occurrences overlap; the
	// lower-priority one is dropped and must report where it would have
	// aired and for how long.
	winner := Block{
		ID: "b-win", Name: "News", Type: "filter", ChannelID: "ch-1",
		Cron: "0 20 * * *", Duration: 60, Priority: 100,
	}
	loser := Block{
		ID: "b-lose", Name: "Late Movie", Type: "filter", ChannelID: "ch-1",
		Cron: "0 20 * * *", Duration: 120, Priority: 10,
	}

	e := newTestEngine(t, []Block{winner, loser})
	_, warnings, err := e.GenerateForTimeRange(testWindowStart, testWindowStart.Add(48*time.Hour), testPrograms())
	if err != nil {
		t.Fatalf("GenerateForTimeRange: %v", err)
	}
	if len(warnings) == 0 {
		t.Fatal("expected at least one conflict warning")
	}
	w := warnings[0]
	if w.BlockName != "Late Movie" || w.BlockingBlockName != "News" {
		t.Fatalf("warning identity: got %+v", w)
	}
	if w.ChannelID != "ch-1" {
		t.Errorf("ChannelID = %q, want ch-1", w.ChannelID)
	}
	if w.DurationMinutes != 120 {
		t.Errorf("DurationMinutes = %d, want 120", w.DurationMinutes)
	}
}
```

Adapt `newTestEngine`, `testWindowStart`, and `testPrograms()` to whatever helpers `engine_test.go` already provides — reuse them, do not add parallel ones.

- [ ] **Step 2: Run it to verify it fails**

Run: `go test -race ./internal/scheduler -run WarningCarriesChannelAndDuration -v`
Expected: FAIL — `w.ChannelID undefined`.

- [ ] **Step 3: Extend the types**

`internal/scheduler/history.go`, in `ScheduleHistoryEntry`, after `Type`:

```go
	// RunID is the apply run (store.ApplyRun.ID) that committed this row,
	// or "" for rows written before apply runs were recorded (v0.5.7) --
	// runs cannot be backfilled, so an empty RunID means "unknown apply",
	// not "no apply". Stamped by Engine.Commit from EngineOptions.RunID,
	// never by the planner: a dry run produces entries too, and they must
	// not claim a run that never happened.
	RunID string `db:"run_id"`
```

`internal/scheduler/engine.go`, in `Warning`:

```go
type Warning struct {
	BlockName         string    // the block whose occurrence was dropped
	OccurrenceStart   time.Time // that occurrence's cron-computed start time
	BlockingBlockName string    // the block whose occurrence it lost to
	ChannelID         string    // the channel both occurrences contended for
	DurationMinutes   int       // how long the dropped occurrence would have run
}
```

In `resolveConflicts`, where the `Warning` literal is built, fill the two new fields from the dropped slot's own block:

```go
		warnings = append(warnings, Warning{
			BlockName:         slot.Block.Name,
			OccurrenceStart:   slot.StartTime,
			BlockingBlockName: winner.Block.Name,
			ChannelID:         slot.Block.ChannelID,
			DurationMinutes:   slot.Block.Duration,
		})
```

Match the existing variable names in `resolveConflicts` — `slot`/`winner` above are illustrative; read the function and use what is there.

- [ ] **Step 4: Run the tests**

Run: `go test -race ./internal/scheduler`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/scheduler/history.go internal/scheduler/engine.go internal/scheduler/engine_test.go
git commit -m "feat(scheduler): carry run_id on history entries and channel/duration on warnings"
```

---

## Task 5: Engine stamps the run and prunes on the split knobs

**Files:**
- Modify: `internal/scheduler/engine.go` — `EngineOptions`, `NewEngineWithOptions`, `Commit`
- Modify: `internal/scheduler/interfaces.go` — `CleanupOccurrenceSnapshots` doc comment
- Test: `internal/scheduler/engine_test.go`

**Interfaces:**
- Consumes: `ScheduleHistoryEntry.RunID` (Task 4), `config.MaintenanceSnapshotRetention` (Task 3).
- Produces: `EngineOptions.RunID string`, `EngineOptions.SnapshotRetention time.Duration`; `Engine.Commit()` stamps `RunID` on every entry it persists and prunes snapshots on `SnapshotRetention`.

**Why stamping happens in `Commit` and not in the planner:** `pendingHistory` is built during planning, which a dry run does too. Only `Commit` runs on an apply, and only an apply has a run. Stamping there is one loop and cannot mislabel a dry run.

- [ ] **Step 1: Write the failing test**

```go
func TestEngine_Commit_StampsRunIDOnHistory(t *testing.T) {
	st := newMockStore() // the existing mock in mock_store_test.go
	e := NewEngineWithOptions(context.Background(), nil, testBlocks(), st, EngineOptions{
		RunID:             "run-abc",
		SnapshotRetention: 48 * time.Hour,
	})
	if _, _, err := e.GenerateForTimeRange(testWindowStart, testWindowStart.Add(24*time.Hour), testPrograms()); err != nil {
		t.Fatalf("GenerateForTimeRange: %v", err)
	}
	if err := e.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if len(st.recordedHistory) == 0 {
		t.Fatal("expected committed history entries")
	}
	for _, entry := range st.recordedHistory {
		if entry.RunID != "run-abc" {
			t.Fatalf("entry %q: RunID = %q, want run-abc", entry.Title, entry.RunID)
		}
	}
	if st.snapshotCleanupWindow != 48*time.Hour {
		t.Errorf("snapshot cleanup window = %v, want 48h (its own knob, not the history window)",
			st.snapshotCleanupWindow)
	}
}
```

`newMockStore`, `recordedHistory`, and `snapshotCleanupWindow` come from `internal/scheduler/mock_store_test.go` — read it and add the two recording fields if they are not there yet (`CleanupOccurrenceSnapshots` currently discards its `window` argument in the mock; capture it).

- [ ] **Step 2: Run it to verify it fails**

Run: `go test -race ./internal/scheduler -run Commit_StampsRunID -v`
Expected: FAIL — `unknown field RunID in struct literal`.

- [ ] **Step 3: Extend `EngineOptions` and `NewEngineWithOptions`**

In `EngineOptions`:

```go
	// RunID is the apply run this engine's Commit belongs to
	// (store.ApplyRun.ID). Stamped onto every schedule_history row Commit
	// writes, so an aired programme can be traced back to the apply that
	// put it there. Empty for a dry-run engine, which never Commits.
	RunID string
	// SnapshotRetention bounds CleanupOccurrenceSnapshots at Commit time.
	// Split from the history window in v0.5.7
	// (maintenance.snapshot_retention); zero falls back to the history
	// window, which is what the single-knob behavior was.
	SnapshotRetention time.Duration
```

Store both on the `Engine` struct (`runID string`, `snapshotRetention time.Duration`) in `NewEngineWithOptions`, defaulting `snapshotRetention` to `e.history.Window()` when zero.

- [ ] **Step 4: Stamp and prune in `Commit`**

```go
	if len(e.pendingHistory) > 0 {
		for i := range e.pendingHistory {
			e.pendingHistory[i].RunID = e.runID
		}
		if err := e.store.RecordScheduleHistory(ctx, e.pendingHistory); err != nil {
			return fmt.Errorf("failed to record schedule history: %w", err)
		}
	}
```

and in the `pendingReplacements` loop, stamp each replacement's entries the same way before calling `ReplaceOccurrenceHistory`:

```go
	for _, rep := range e.pendingReplacements {
		for i := range rep.entries {
			rep.entries[i].RunID = e.runID
		}
		if err := e.store.ReplaceOccurrenceHistory(ctx, rep.blockName, rep.occurrenceStart, rep.entries); err != nil {
```

and change the snapshot cleanup call:

```go
	if _, err := e.store.CleanupOccurrenceSnapshots(ctx, e.snapshotRetention); err != nil {
```

- [ ] **Step 5: Update the interface doc**

In `internal/scheduler/interfaces.go`, `CleanupOccurrenceSnapshots`'s comment currently says it "mirrors CleanupScheduleHistory's retention window". Correct it:

```go
	// CleanupOccurrenceSnapshots deletes snapshot rows for occurrences that
	// started more than window before now. Since v0.5.7 window comes from
	// its own config knob (maintenance.snapshot_retention,
	// EngineOptions.SnapshotRetention) rather than the history window,
	// though the default value is the same: a snapshot for an occurrence
	// outside the SCHEDULE-HISTORY window can never be re-derived (its
	// schedule_history rows, if any, are gone too), so setting snapshot
	// retention longer than history retention keeps rows that serve no
	// purpose. Returns the number of rows deleted.
```

- [ ] **Step 6: Run the tests**

Run: `go test -race ./internal/scheduler`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/scheduler/engine.go internal/scheduler/interfaces.go \
        internal/scheduler/engine_test.go internal/scheduler/mock_store_test.go
git commit -m "feat(scheduler): stamp the apply run on committed history, prune snapshots on their own knob"
```

---

## Task 6: The Runner records every apply

**Files:**
- Modify: `internal/service/schedule.go` — `Options`, `RunnerOptions`, `NewRunner`, `Run`
- Test: `internal/service/schedule_test.go`

**Interfaces:**
- Consumes: `store.StartApplyRun`/`FinishApplyRun` (Task 2), `EngineOptions.RunID`/`SnapshotRetention` (Task 5).
- Produces:
  - `service.Source` string constants: `SourceUI = store.ApplySourceUI`, `SourceCron`, `SourceCLI`
  - `service.Options{Days int; ChannelID string; Apply bool; Source string}`
  - `service.RunnerOptions{Logger *slog.Logger; Location *time.Location; HistoryRetention, SnapshotRetention, ApplyRunRetention time.Duration}`
  - `service.NewRunner(st *store.Store, tc *tunarr.Client, o RunnerOptions) *Runner`
  - `Result` gains `RunID string` (empty on a dry run)

**Why `RunnerOptions`:** `NewRunner` is already at the 5-argument lint ceiling; two more retention values would breach it. The house already has this shape in `scheduler.EngineOptions`.

- [ ] **Step 1: Write the failing test**

In `internal/service/schedule_test.go` (it already builds Runners against a real temp-dir store and a stubbed Tunarr client — reuse those helpers):

```go
func TestRunner_Run_RecordsApplyRun(t *testing.T) {
	st, tc := newTestRunnerDeps(t) // existing helper
	r := NewRunner(st, tc, RunnerOptions{HistoryRetention: 168 * time.Hour})

	res, err := r.Run(context.Background(), Options{Days: 1, Apply: true, Source: SourceCron})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.RunID == "" {
		t.Fatal("Result.RunID is empty on an apply")
	}

	runs, err := st.ListApplyRuns(context.Background(), time.Now().Add(-time.Hour), 10)
	if err != nil {
		t.Fatalf("ListApplyRuns: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("got %d recorded runs, want 1", len(runs))
	}
	got := runs[0]
	if got.ID != res.RunID {
		t.Errorf("recorded run ID = %q, want %q", got.ID, res.RunID)
	}
	if got.Source != store.ApplySourceCron {
		t.Errorf("source = %q, want cron", got.Source)
	}
	if got.Status != store.ApplyStatusOK {
		t.Errorf("status = %q, want ok", got.Status)
	}
	if got.FinishedAt == nil {
		t.Error("FinishedAt is nil on a completed run")
	}
	if got.Days != 1 {
		t.Errorf("days = %d, want 1", got.Days)
	}
}

func TestRunner_Run_DryRunRecordsNothing(t *testing.T) {
	st, tc := newTestRunnerDeps(t)
	r := NewRunner(st, tc, RunnerOptions{HistoryRetention: 168 * time.Hour})

	res, err := r.Run(context.Background(), Options{Days: 1, Apply: false, Source: SourceUI})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.RunID != "" {
		t.Errorf("dry run reported RunID %q, want empty", res.RunID)
	}

	runs, err := st.ListApplyRuns(context.Background(), time.Now().Add(-time.Hour), 10)
	if err != nil {
		t.Fatalf("ListApplyRuns: %v", err)
	}
	if len(runs) != 0 {
		t.Fatalf("dry run recorded %d runs, want 0 -- a dry run applies nothing", len(runs))
	}
}

func TestRunner_Run_RecordsFailureWithDetail(t *testing.T) {
	st, tc := newFailingTunarrRunnerDeps(t) // stub whose UpdateSchedule returns an error
	r := NewRunner(st, tc, RunnerOptions{HistoryRetention: 168 * time.Hour})

	if _, err := r.Run(context.Background(), Options{Days: 1, Apply: true, Source: SourceUI}); err == nil {
		t.Fatal("expected Run to fail")
	}

	runs, err := st.ListApplyRuns(context.Background(), time.Now().Add(-time.Hour), 10)
	if err != nil {
		t.Fatalf("ListApplyRuns: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("got %d recorded runs, want 1 -- a failed apply is still a run", len(runs))
	}
	if runs[0].Status != store.ApplyStatusError {
		t.Errorf("status = %q, want error", runs[0].Status)
	}
	if runs[0].Error == "" {
		t.Error("failed run recorded no error detail")
	}
}
```

`newFailingTunarrRunnerDeps` may not exist; build it from the existing stub by making its schedule-update call return an error (the package's existing Tunarr stub pattern shows how).

- [ ] **Step 2: Run them to verify they fail**

Run: `go test -race ./internal/service -run 'RecordsApplyRun|DryRunRecordsNothing|RecordsFailureWithDetail' -v`
Expected: FAIL — `too many arguments in call to NewRunner` / `unknown field Source`.

- [ ] **Step 3: Restructure `Options`, `RunnerOptions`, `NewRunner`, `Result`**

```go
// Sources an apply can be attributed to, mirrored from the store's
// constants so callers outside internal/store name them in service terms.
const (
	SourceUI   = store.ApplySourceUI
	SourceCron = store.ApplySourceCron
	SourceCLI  = store.ApplySourceCLI
)

type Options struct {
	Days      int
	ChannelID string
	Apply     bool
	// Source attributes an apply to the surface that asked for it -- one
	// of SourceUI, SourceCron, SourceCLI -- and is recorded on the run.
	// Ignored on a dry run. An empty Source on an apply records
	// store.ApplySourceUnknown: a missing label is a defect to notice in
	// the RUNS pane, never a reason to refuse to push a lineup.
	Source string
}

// RunnerOptions carries a Runner's construction settings. A struct
// rather than more parameters: NewRunner was already at the five-argument
// lint ceiling before retention split per table.
type RunnerOptions struct {
	Logger   *slog.Logger
	Location *time.Location
	// HistoryRetention is forwarded to every scheduler.Engine Run builds
	// (EngineOptions.HistoryWindow), governing both the in-memory
	// recency-dedup window and Commit-time schedule_history pruning.
	// Zero falls back to scheduler's own default (7 days).
	HistoryRetention time.Duration
	// SnapshotRetention bounds occurrence-snapshot pruning
	// (EngineOptions.SnapshotRetention). Zero falls back to
	// HistoryRetention.
	SnapshotRetention time.Duration
	// ApplyRunRetention bounds apply-run pruning, run after each
	// successful apply. Zero disables it -- runs then accumulate until an
	// operator prunes them, which is the honest behavior for an unset
	// knob rather than silently picking a horizon.
	ApplyRunRetention time.Duration
}
```

`Result` gains:

```go
	// RunID is the apply run this Run was recorded as
	// (store.ApplyRun.ID), or "" on a dry run.
	RunID string
```

`NewRunner` becomes `func NewRunner(st *store.Store, tc *tunarr.Client, o RunnerOptions) *Runner`, applying the same nil-defaults for `Logger`/`Location` it does today and storing the three durations on the Runner.

- [ ] **Step 4: Wrap `Run` with run recording**

Rename the existing body to `run` (unexported), taking the run ID, and make `Run` the recording wrapper:

```go
// Run executes one generate-(and-maybe-apply) cycle. An apply is
// recorded as a store.ApplyRun: the row is written BEFORE anything is
// pushed to Tunarr (so a crash mid-apply still leaves evidence, and so
// the schedule_history rows Commit writes reference a row that already
// exists) and finalized afterwards with its outcome, counts and
// warnings. A dry run records nothing -- it applies nothing.
//
// Recording never fails an apply: a store write that fails here is
// logged and dropped. Losing the record of an apply is a reporting
// gap; refusing to schedule because the reporting table is unhappy
// would be worse.
func (r *Runner) Run(ctx context.Context, o Options) (*Result, error) {
	if !o.Apply {
		return r.run(ctx, o, "")
	}

	r.applyMu.Lock()
	defer r.applyMu.Unlock()

	runID := uuid.NewString()
	started := r.now()
	r.startRun(ctx, runID, started, o)

	res, err := r.run(ctx, o, runID)
	r.finishRun(ctx, runID, started, o, res, err)
	if res != nil {
		res.RunID = runID
	}
	return res, err
}

func (r *Runner) startRun(ctx context.Context, runID string, started time.Time, o Options) {
	source := o.Source
	if source == "" {
		source = store.ApplySourceUnknown
	}
	if err := r.store.StartApplyRun(ctx, store.ApplyRun{
		ID: runID, StartedAt: started, Source: source, Scope: o.ChannelID,
		Days: o.Days, Status: store.ApplyStatusRunning,
	}); err != nil {
		r.logger.Warn("failed to record apply run start", "error", err, "run_id", runID)
	}
}

func (r *Runner) finishRun(ctx context.Context, runID string, started time.Time, o Options, res *Result, runErr error) {
	finished := r.now()
	rec := store.ApplyRun{
		ID: runID, StartedAt: started, FinishedAt: &finished,
		Source: o.Source, Scope: o.ChannelID, Days: o.Days,
		Status: store.ApplyStatusOK,
	}
	if runErr != nil {
		rec.Status = store.ApplyStatusError
		rec.Error = runErr.Error()
	}
	if res != nil {
		rec.ChannelCount = len(res.Channels)
		for _, slots := range res.Channels {
			rec.SlotCount += len(slots)
		}
		rec.Warnings = applyRunWarnings(runID, res.Warnings)
	}
	if err := r.store.FinishApplyRun(ctx, rec); err != nil {
		r.logger.Warn("failed to record apply run outcome", "error", err, "run_id", runID)
		return
	}
	if r.applyRunRetention > 0 && runErr == nil {
		if _, err := r.store.CleanupApplyRuns(ctx, r.applyRunRetention); err != nil {
			r.logger.Warn("failed to prune apply runs", "error", err)
		}
	}
}

// applyRunWarnings converts the engine's in-memory warnings into their
// persisted form.
func applyRunWarnings(runID string, warnings []scheduler.Warning) []store.ApplyRunWarning {
	out := make([]store.ApplyRunWarning, 0, len(warnings))
	for _, w := range warnings {
		out = append(out, store.ApplyRunWarning{
			RunID: runID, BlockName: w.BlockName, OccurrenceStart: w.OccurrenceStart,
			BlockingBlockName: w.BlockingBlockName, ChannelID: w.ChannelID,
			DurationMinutes: w.DurationMinutes,
		})
	}
	return out
}
```

`run` keeps the existing body minus the `applyMu` locking (now the wrapper's job) and passes the run ID through to the engine:

```go
	engine := scheduler.NewEngineWithOptions(ctx, r.tunarr, blocks, r.store, scheduler.EngineOptions{
		Logger:            r.logger,
		Location:          r.loc,
		HistoryWindow:     r.historyRetention,
		SnapshotRetention: r.snapshotRetention,
		RunID:             runID,
	})
```

and returns `&Result{Applied: o.Apply, Channels: channels, Warnings: warnings}` unchanged (the wrapper sets `RunID`).

Add `"github.com/google/uuid"` to the imports.

- [ ] **Step 5: Run the tests**

Run: `go test -race ./internal/service`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/service/schedule.go internal/service/schedule_test.go
git commit -m "feat(service): record every apply as a run with its counts, warnings and outcome"
```

---

## Task 7: Every caller labels its apply

**Files:**
- Modify: `cmd/serve.go:418-425` (cron tick) and its `NewRunner` call site
- Modify: `cmd/generate.go:141` and its `NewRunner` call site, plus the cleanup at `:191`
- Modify: `internal/api/schedule.go:115`
- Test: `cmd/*_test.go`, `internal/api/schedule_test.go`

**Interfaces:**
- Consumes: `service.RunnerOptions`, `service.Options.Source`, `service.Source*` (Task 6); `config.Maintenance*Retention` (Task 3).
- Produces: no new symbols; three call sites now compile against the new signatures.

- [ ] **Step 1: Update `cmd/serve.go`**

The `NewRunner` construction becomes:

```go
	runner := service.NewRunner(st, tc, service.RunnerOptions{
		Logger:            logger,
		Location:          loc,
		HistoryRetention:  config.MaintenanceHistoryRetention(cfg),
		SnapshotRetention: config.MaintenanceSnapshotRetention(cfg),
		ApplyRunRetention: config.MaintenanceApplyRunRetention(cfg),
	})
```

(match the local variable names actually in scope), and the cron tick:

```go
	if _, err := runner.Run(ctx, service.Options{Days: days, Apply: true, Source: service.SourceCron}); err != nil {
```

Update the doc comment above it that currently reads `service.Options{Days: 1, Apply: true}` so it names the source too.

- [ ] **Step 2: Update `cmd/generate.go`**

Same `RunnerOptions` construction, and:

```go
	result, err := runner.Run(context.Background(), service.Options{Days: 1, Apply: applyNow, Source: service.SourceCLI})
```

The standalone cleanup near line 191 currently prunes history with a single retention value. Extend it to prune all three tables on their own knobs:

```go
	removed, err := st.CleanupScheduleHistory(ctx, config.MaintenanceHistoryRetention(cfg))
	if err != nil {
		return fmt.Errorf("failed to cleanup schedule history: %w", err)
	}
	removedSnapshots, err := st.CleanupOccurrenceSnapshots(ctx, config.MaintenanceSnapshotRetention(cfg))
	if err != nil {
		return fmt.Errorf("failed to cleanup occurrence snapshots: %w", err)
	}
	removedRuns, err := st.CleanupApplyRuns(ctx, config.MaintenanceApplyRunRetention(cfg))
	if err != nil {
		return fmt.Errorf("failed to cleanup apply runs: %w", err)
	}
	logger.Info("maintenance cleanup complete",
		"history_rows", removed, "snapshot_rows", removedSnapshots, "apply_runs", removedRuns)
```

Read the surrounding function first and keep its existing variable names, logger, and error style; the block above shows the shape, not a literal paste target.

- [ ] **Step 3: Update `internal/api/schedule.go`**

```go
	result, err := h.d.Sched.Run(r.Context(), service.Options{
		Days: days, ChannelID: channelID, Apply: req.apply, Source: service.SourceUI,
	})
```

The API's `Source` is always `SourceUI` — the HTTP surface is the UI's, and the CLI never goes through it.

- [ ] **Step 4: Run the tests**

Run: `go test -race ./cmd ./internal/api ./internal/service`
Expected: PASS. Fake `ScheduleRunner` implementations in `internal/api` tests take `service.Options` by value and keep compiling; if a test asserts on the exact `Options` it received, update the expectation to include `Source: service.SourceUI`.

- [ ] **Step 5: Full suite**

Run: `make test`
Expected: all packages `ok`.

- [ ] **Step 6: Commit**

```bash
git add cmd/serve.go cmd/generate.go internal/api/schedule.go
git commit -m "feat(cmd,api): attribute every apply to its source and prune each table on its own knob"
```

---

## Task 8: The contract — `GET /applies` and the enriched shapes

**Files:**
- Modify: `api/openapi.yaml` — new path `/applies`; new schemas `ApplyRun`, `ApplyRunWarning`; `HistoryEntry` and `Warning` extended
- Create: `internal/api/applies.go`
- Create: `internal/api/applies_test.go`
- Modify: `internal/api/history.go` — `historyEntryToGen`
- Modify: `internal/api/history_test.go`
- Modify: `internal/api/schedule.go` — warning conversion gains the two fields
- Regenerated: `internal/api/gen/server.gen.go`, `web/assets/ts/gen/types.d.ts`

**Interfaces:**
- Consumes: `store.ListApplyRuns` (Task 2), enriched domain types (Task 4).
- Produces: `GET /api/v1/applies?days=&limit=` → `ApplyRun[]`; `HistoryEntry` with `title, type, duration_ms, occurrence_start, sequence, run_id`; `Warning` with `channel_id, duration_minutes`.

- [ ] **Step 1: Edit the contract**

In `api/openapi.yaml`, after the `/history` path:

```yaml
  /applies:
    get:
      operationId: listApplyRuns
      description: >-
        Recorded applies, newest first. One row per apply -- UI, cron
        loop, and CLI alike -- with the conflict warnings that apply
        dropped. Runs cannot be backfilled: nothing exists before the
        v0.5.7 migration that created the table.
      parameters:
        - name: days
          in: query
          schema:
            type: integer
            default: 7
            minimum: 1
            maximum: 90
        - name: limit
          in: query
          schema:
            type: integer
            default: 100
            minimum: 1
            maximum: 500
      responses:
        "200":
          description: apply runs
          content:
            application/json:
              schema:
                type: array
                items:
                  $ref: "#/components/schemas/ApplyRun"
        "400":
          $ref: "#/components/responses/Problem"
```

New schemas alongside `Warning`:

```yaml
    ApplyRun:
      type: object
      required: [id, started_at, source, status]
      properties:
        id:
          type: string
          description: The run's identifier, also stamped on the schedule_history rows it committed.
        started_at:
          type: string
          format: date-time
        finished_at:
          type: string
          format: date-time
          nullable: true
          description: >-
            Null while a run is in flight, and permanently null for a run
            whose process died mid-apply -- which is why status can read
            "running" on a row that will never finish.
        source:
          type: string
          enum: [ui, cron, cli, unknown]
        scope:
          type: string
          description: The channel ID the apply was narrowed to, or "" for every channel.
        days:
          type: integer
        status:
          type: string
          enum: [running, ok, error]
        channel_count:
          type: integer
        slot_count:
          type: integer
        error:
          type: string
        warnings:
          type: array
          items:
            $ref: "#/components/schemas/ApplyRunWarning"

    ApplyRunWarning:
      type: object
      description: >-
        One occurrence this run planned a slot for and then dropped by
        conflict resolution -- the persisted form of Warning.
      properties:
        block_name:
          type: string
        occurrence_start:
          type: string
          format: date-time
        blocking_block_name:
          type: string
        channel_id:
          type: string
        duration_minutes:
          type: integer
```

Extend `Warning` with:

```yaml
        channel_id:
          type: string
          description: The channel both occurrences contended for.
        duration_minutes:
          type: integer
          description: How long the dropped occurrence would have run.
```

Extend `HistoryEntry` with:

```yaml
        title:
          type: string
          description: The programme's title -- stored since migration 000003, exposed since v0.5.7.
        type:
          type: string
          description: The Tunarr program type (e.g. episode, movie).
        duration_ms:
          type: number
          format: double
        occurrence_start:
          type: string
          format: date-time
          description: >-
            The block occurrence's own cron-computed start time -- the
            identity half of the (block_name, occurrence_start) key, not
            the wall-clock instant planning happened (that is scheduled_at).
        sequence:
          type: integer
          description: Playback order within the occurrence.
        run_id:
          type: string
          description: >-
            The apply run that committed this row, or "" for rows written
            before runs were recorded. Runs are not backfilled.
```

- [ ] **Step 2: Regenerate**

Run: `make generate && make web-types`
Expected: `internal/api/gen/server.gen.go` gains `ListApplyRuns`, `GetAppliesParams`/`ListApplyRunsParams`, `ApplyRun`, `ApplyRunWarning`, and the new `HistoryEntry`/`Warning` fields; `web/assets/ts/gen/types.d.ts` gains the `/applies` path. Do not hand-edit either file.

- [ ] **Step 3: Write the failing handler test**

Create `internal/api/applies_test.go`, following `history_test.go`'s idiom exactly (it builds a `Handlers` over a temp-dir store and drives it through the router):

```go
func TestListApplyRuns_ReturnsRunsWithWarnings(t *testing.T) {
	h, st := newTestHandlers(t) // existing helper in the package's tests
	ctx := context.Background()
	started := time.Now().Add(-time.Minute)

	run := store.ApplyRun{ID: "r1", StartedAt: started, Source: store.ApplySourceUI, Days: 7, Status: store.ApplyStatusRunning}
	if err := st.StartApplyRun(ctx, run); err != nil {
		t.Fatalf("StartApplyRun: %v", err)
	}
	fin := started.Add(time.Second)
	run.FinishedAt = &fin
	run.Status = store.ApplyStatusOK
	run.ChannelCount, run.SlotCount = 1, 4
	run.Warnings = []store.ApplyRunWarning{{
		BlockName: "Late Movie", OccurrenceStart: started, BlockingBlockName: "News",
		ChannelID: "ch-1", DurationMinutes: 120,
	}}
	if err := st.FinishApplyRun(ctx, run); err != nil {
		t.Fatalf("FinishApplyRun: %v", err)
	}

	rec := doRequest(t, h, http.MethodGet, "/api/v1/applies?days=7", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	var got []gen.ApplyRun
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d runs, want 1", len(got))
	}
	if got[0].Id != "r1" || got[0].Status != "ok" || got[0].Source != "ui" {
		t.Fatalf("run: %+v", got[0])
	}
	if got[0].Warnings == nil || len(*got[0].Warnings) != 1 {
		t.Fatalf("warnings: %+v", got[0].Warnings)
	}
	if w := (*got[0].Warnings)[0]; *w.BlockingBlockName != "News" || *w.DurationMinutes != 120 {
		t.Fatalf("warning: %+v", w)
	}
}

func TestListApplyRuns_RejectsOutOfRangeDays(t *testing.T) {
	h, _ := newTestHandlers(t)
	rec := doRequest(t, h, http.MethodGet, "/api/v1/applies?days=200", nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}
```

Adapt `newTestHandlers`/`doRequest` to the package's actual helper names, and the generated field casing to whatever `make generate` produced (`Id` vs `ID`, pointer vs value) — read `gen/server.gen.go` after regenerating rather than guessing.

- [ ] **Step 4: Run it to verify it fails**

Run: `go test -race ./internal/api -run ListApplyRuns -v`
Expected: FAIL — `*Handlers does not implement gen.ServerInterface (missing ListApplyRuns)`.

- [ ] **Step 5: Write the handler**

Create `internal/api/applies.go`:

```go
package api

import (
	"fmt"
	"net/http"
	"time"

	"github.com/christopherime/schedularr/internal/api/gen"
	"github.com/christopherime/schedularr/internal/store"
)

// Window and page bounds for GET /applies. These mirror the OpenAPI
// schema (api/openapi.yaml), which oapi-codegen's chi-server generator
// does not enforce at the binding layer -- see GetHistory's doc comment
// in history.go for the full explanation of why every query parameter is
// re-validated here.
const (
	defaultApplyDays  = 7
	minApplyDays      = 1
	maxApplyDays      = 90
	defaultApplyLimit = 100
	minApplyLimit     = 1
	maxApplyLimit     = 500
)

// ListApplyRuns implements gen.ServerInterface.
func (h *Handlers) ListApplyRuns(w http.ResponseWriter, r *http.Request, params gen.ListApplyRunsParams) {
	days := defaultApplyDays
	if params.Days != nil {
		days = *params.Days
	}
	if days < minApplyDays || days > maxApplyDays {
		WriteProblem(w, r, http.StatusBadRequest, "invalid days parameter",
			fmt.Sprintf("days must be between %d and %d, got %d", minApplyDays, maxApplyDays, days))
		return
	}

	limit := defaultApplyLimit
	if params.Limit != nil {
		limit = *params.Limit
	}
	if limit < minApplyLimit || limit > maxApplyLimit {
		WriteProblem(w, r, http.StatusBadRequest, "invalid limit parameter",
			fmt.Sprintf("limit must be between %d and %d, got %d", minApplyLimit, maxApplyLimit, limit))
		return
	}

	since := time.Now().Add(-time.Duration(days) * 24 * time.Hour)
	runs, err := h.d.Store.ListApplyRuns(r.Context(), since, limit)
	if err != nil {
		h.logAndWriteInternalError(w, r, "list_apply_runs", err)
		return
	}

	list := make([]gen.ApplyRun, 0, len(runs))
	for _, run := range runs {
		list = append(list, applyRunToGen(run))
	}
	writeJSON(w, http.StatusOK, list)
}

// applyRunToGen converts a store.ApplyRun into its wire shape. Warnings
// is always a non-nil slice so a run with none serializes as [] rather
// than null -- the page renders "no conflicts" from an empty array and
// would have to special-case null otherwise.
func applyRunToGen(run store.ApplyRun) gen.ApplyRun {
	warnings := make([]gen.ApplyRunWarning, 0, len(run.Warnings))
	for _, w := range run.Warnings {
		blockName := w.BlockName
		occurrenceStart := w.OccurrenceStart
		blocking := w.BlockingBlockName
		channelID := w.ChannelID
		duration := w.DurationMinutes
		warnings = append(warnings, gen.ApplyRunWarning{
			BlockName:         &blockName,
			OccurrenceStart:   &occurrenceStart,
			BlockingBlockName: &blocking,
			ChannelId:         &channelID,
			DurationMinutes:   &duration,
		})
	}

	scope := run.Scope
	days := run.Days
	channelCount := run.ChannelCount
	slotCount := run.SlotCount
	errText := run.Error

	return gen.ApplyRun{
		Id:           run.ID,
		StartedAt:    run.StartedAt,
		FinishedAt:   run.FinishedAt,
		Source:       run.Source,
		Scope:        &scope,
		Days:         &days,
		Status:       run.Status,
		ChannelCount: &channelCount,
		SlotCount:    &slotCount,
		Error:        &errText,
		Warnings:     &warnings,
	}
}
```

Field names and pointer-ness must match the regenerated `gen` package — read it, then adjust. The `&local` pattern (never `&run.Field`) is the house idiom already used in `historyEntryToGen`.

- [ ] **Step 6: Enrich `historyEntryToGen`**

In `internal/api/history.go`:

```go
// historyEntryToGen converts a scheduler.ScheduleHistoryEntry (the
// persisted domain representation) into a gen.HistoryEntry (the API wire
// shape). Every wire field is a pointer (OpenAPI marks them optional) but
// always populated here since this only ever converts an already-persisted
// row. Since v0.5.7 the enrichment migration 000003 already stored --
// title, type, duration_ms, occurrence_start, sequence -- is exposed too,
// along with run_id: the page shows programme names, not UUIDs.
func historyEntryToGen(e scheduler.ScheduleHistoryEntry) gen.HistoryEntry {
	programID := e.ProgramID
	channelID := e.ChannelID
	blockName := e.BlockName
	scheduledAt := e.ScheduledAt
	occurrenceStart := e.OccurrenceStart
	sequence := e.Sequence
	durationMs := e.DurationMs
	title := e.Title
	kind := e.Type
	runID := e.RunID

	return gen.HistoryEntry{
		ProgramId:       &programID,
		ChannelId:       &channelID,
		BlockName:       &blockName,
		ScheduledAt:     &scheduledAt,
		OccurrenceStart: &occurrenceStart,
		Sequence:        &sequence,
		DurationMs:      &durationMs,
		Title:           &title,
		Type:            &kind,
		RunId:           &runID,
	}
}
```

- [ ] **Step 7: Extend the warning conversion in `internal/api/schedule.go`**

Find where `scheduler.Warning` becomes `gen.Warning` and add `ChannelId` and `DurationMinutes` from the domain fields, using the same `&local` pattern.

- [ ] **Step 8: Add a history-enrichment test**

In `internal/api/history_test.go`, extend the existing round-trip test to assert `Title`, `Type`, `Sequence`, `OccurrenceStart`, `DurationMs`, and `RunId` all survive the wire — the whole point of the slice is that `GET /history` stops being UUID-headed.

- [ ] **Step 9: Run the tests**

Run: `go test -race ./internal/api && make test`
Expected: PASS.

- [ ] **Step 10: Commit**

```bash
git add api/openapi.yaml internal/api/ web/assets/ts/gen/types.d.ts
git commit -m "feat(api): add GET /applies and enrich HistoryEntry and Warning"
```

---

## Task 9: Phase A gate

**Files:** none created; this task verifies.

- [ ] **Step 1: Full lint and test**

Run: `make lint && make test`
Expected: both green. Fix anything the linters flag (complexity on the new `Run`/`finishRun` split is the likely candidate — extract, don't suppress).

- [ ] **Step 2: Prove a cron-tick apply writes a run (spec §9.2 gate)**

Run the binary against the e2e fixtures:

```bash
make build
./bin/schedularr --config e2e/fixtures/test-config.yaml generate --apply --yes
sqlite3 "$(grep '^database:' e2e/fixtures/test-config.yaml | awk '{print $2}')" \
  'SELECT id, source, status, channel_count, slot_count FROM apply_runs ORDER BY started_at DESC LIMIT 3;'
```

Expected: at least one row with `source = cli`, `status = ok`. If the e2e config points at a Tunarr the environment does not have, run `e2e/test.sh` instead and read its output — record in the commit message which of the two was actually run, and do not claim the gate passed if neither did.

- [ ] **Step 3: Prove history rows carry the run**

```bash
sqlite3 <db> 'SELECT COUNT(*) FROM schedule_history WHERE run_id != "";'
```

Expected: non-zero after the apply above.

- [ ] **Step 4: Commit any fixes**

```bash
git commit -am "fix: Phase A gate corrections"
```

(Skip if nothing needed fixing.)

---

# Phase B — The `/history/` page

> **Every task in this phase invokes the `impeccable` skill first** (CLAUDE.md rule 4). The design contract is `web/DESIGN.md`; the shared partials (`skeleton`, `problem`, `empty`, `plate`, `tape`, `confirm`, `channel-select`, `page-js`) and the runtime modules (`api.ts`, `channels.ts`, `format.ts`, `errors.ts`, `tape.ts`) already exist — **reuse them, do not re-implement**. `/kit/` is where each new component gets a fixture.

## Task 10: The route, the nav, and the deletions

**Files:**
- Create: `web/content/history/_index.md`
- Create: `web/layouts/history/list.html` (shell only in this task)
- Create: `web/assets/ts/pages/history.ts` (shell only)
- Modify: `web/layouts/partials/nav.html`
- Delete: `web/content/series/_index.md`, `web/content/dashboard/_index.md`, `web/layouts/series/list.html`, `web/layouts/dashboard/list.html`, `web/assets/ts/pages/series.ts`, `web/assets/ts/pages/dashboard.ts`
- Modify: `web/assets/css/main.css` (remove series/dashboard rules), `web/layouts/kit/list.html` + `web/assets/ts/pages/kit.ts` (remove their fixtures), `web/layouts/_default/baseof.html` (drop the dashboard reference)

**Interfaces:**
- Consumes: `GET /state/series`, `GET /history`, `GET /applies` (Phase A).
- Produces: the route `/history/` with a segmented control whose three panes (`tracked`, `asrun`, `runs`) are deep-linkable as `/history/?view=<pane>`; `tracked` is the default.

**Why the deletions land here and not later:** rule 1. A `/series/` route that still works while `/history/` exists is two answers to one question, and the nav can only point at one of them.

- [ ] **Step 1: Read the design contract**

Read `web/DESIGN.md` end to end and `web/layouts/blocks/list.html` as the closest structural precedent (toolbar + table + row actions). Note the `page-js` partial's cronstrue-before-page ordering constraint.

- [ ] **Step 2: Create the section front matter**

`web/content/history/_index.md`:

```markdown
---
title: "History"
---
```

Match the exact front-matter fields `web/content/blocks/_index.md` uses.

- [ ] **Step 3: Write the page shell**

`web/layouts/history/list.html`: `{{ define "main" }}` with

- a filter toolbar: window select (1/7/30/90 days), the shared `channel-select` partial, a block-name text filter, and a search input;
- a segmented control (`role="tablist"`, three `role="tab"` buttons: TRACKED / AS-RUN / RUNS, `aria-selected`, `aria-controls`);
- three `role="tabpanel"` regions, two hidden, each containing the `skeleton` partial as its initial state;
- the `tape` region;
- `{{ partial "ui/page-js.html" (dict "page" "history") }}` (match the partial's actual signature).

Pane selection is server-agnostic: the TS reads `?view=` on load and writes it back with `history.replaceState` on switch, so a pane is linkable without a reload.

- [ ] **Step 4: Write the page bundle shell**

`web/assets/ts/pages/history.ts` — for this task, only: read `?view=`, wire the segmented control (click + arrow-key roving tabindex per the WAI-ARIA tabs pattern), and render each pane's empty state. Pane content arrives in Tasks 11–13.

```ts
/** The three panes, in tab order. The query value is the pane id. */
export const PANES = ["tracked", "asrun", "runs"] as const;
export type Pane = (typeof PANES)[number];

/**
 * Resolves the pane a URL asks for. An unknown or absent ?view= falls
 * back to "tracked": a mistyped deep link should land somewhere useful
 * rather than on a blank page.
 */
export function paneFromSearch(search: string): Pane {
  const value = new URLSearchParams(search).get("view");
  return (PANES as readonly string[]).includes(value ?? "") ? (value as Pane) : "tracked";
}
```

- [ ] **Step 5: Update the nav**

`web/layouts/partials/nav.html` — replace the stale comment block (it still describes the dashboard surviving "until the Memory slice's `/history/` absorbs its table") and the items:

```go-html-template
{{- /* IA since the Memory slice: the Guide is home and owns
       preview/apply as its draft mode; History is the one searchable
       record -- tracked sequences, as-run airings, and apply runs behind
       one filter toolbar. /series/ and /dashboard/ were deleted with it,
       no redirect stubs. Active state is server-rendered (aria-current,
       no JS) so it's correct even if a page's script fails to load. */ -}}
{{- $items := slice
  (dict "href" "/" "label" "Guide")
  (dict "href" "/blocks/" "label" "Blocks")
  (dict "href" "/history/" "label" "History")
-}}
```

- [ ] **Step 6: Delete the old surfaces**

```bash
git rm web/content/series/_index.md web/content/dashboard/_index.md \
       web/layouts/series/list.html web/layouts/dashboard/list.html \
       web/assets/ts/pages/series.ts web/assets/ts/pages/dashboard.ts
```

Then grep for every remaining reference and remove it:

```bash
grep -rn "dashboard\|/series/" web/ docs/ README.md CLAUDE.md AGENTS.md GEMINI.md mkdocs.yml
```

Delete the CSS rules that only served those pages from `web/assets/css/main.css`, and their `/kit/` fixtures from `web/layouts/kit/list.html` and `web/assets/ts/pages/kit.ts`. `web/layouts/_default/baseof.html`'s dashboard mention goes too. **`/api/v1/state/series` stays** — that is the API path the TRACKED pane calls, not a UI route.

- [ ] **Step 7: Write the pane-routing test**

`web/tests/history.test.ts`:

```ts
import assert from "node:assert/strict";
import test from "node:test";

const { PANES, paneFromSearch } = await import("../assets/ts/pages/history.ts");

test("paneFromSearch reads a deep link", () => {
  assert.equal(paneFromSearch("?view=runs"), "runs");
  assert.equal(paneFromSearch("?view=asrun&block=News"), "asrun");
});

test("paneFromSearch falls back to tracked", () => {
  assert.equal(paneFromSearch(""), "tracked");
  assert.equal(paneFromSearch("?view=nonsense"), "tracked");
  assert.equal(PANES[0], "tracked");
});
```

- [ ] **Step 8: Verify**

Run: `make web-check && make web-test && make web-build`
Expected: type-check clean, tests pass, Hugo builds with no `/series/` or `/dashboard/` output directory.

- [ ] **Step 9: Commit**

```bash
git add -A web/
git commit -m "feat(web): add the /history/ route and delete /series/ and /dashboard/"
```

---

## Task 11: The TRACKED pane

**Files:**
- Modify: `web/layouts/history/list.html`, `web/assets/ts/pages/history.ts`, `web/assets/css/main.css`, `web/tests/history.test.ts`

**Interfaces:**
- Consumes: `GET /api/v1/state/series` → `SeriesState[]`; `PATCH /api/v1/state/series/{show_title}` → `SeriesState`.
- Produces: a table of tracked sequences with cursor editing.

**What it must carry over from the deleted `/series/` page** (read `git show HEAD~1:web/assets/ts/pages/series.ts` for the shipped behavior before writing anything): the cursor S/E dirty-armed edit behind Save, the Completed/Disabled instant toggles that print an undoable tape line, per-row error handling with refresh recovery, and the teaching empty state. **Not** carried over (later slice): bulk selection, YAML import/export, removal.

- [ ] **Step 1: Write the failing test**

Add to `web/tests/history.test.ts` a test for the pane's pure filter helper:

```ts
const { filterTracked } = await import("../assets/ts/pages/history.ts");

test("filterTracked matches title case-insensitively", () => {
  const rows = [
    { show_title: "Supernatural", current_season: 1, current_episode: 2 },
    { show_title: "The Wire", current_season: 3, current_episode: 1 },
  ];
  assert.deepEqual(filterTracked(rows, "wire").map((r) => r.show_title), ["The Wire"]);
  assert.equal(filterTracked(rows, "").length, 2);
});
```

- [ ] **Step 2: Run it to verify it fails**

Run: `npm test --prefix web`
Expected: FAIL — `filterTracked is not a function`.

- [ ] **Step 3: Implement the pane**

Markup: a table inside the `tracked` tabpanel — columns TITLE, CURSOR (S/E number inputs), COMPLETED, DISABLED, RUNS, LAST AIRED. Rows use the `plate` idiom where a channel appears; the skeleton partial's `table-row` variant is the loading state; the `empty` partial carries "Rows appear after a sequence block first airs — create one on Blocks."

TS: fetch via `apiGet("/state/series")`, render, wire the toggles and the armed cursor save through `apiSend`, and export:

```ts
/**
 * Narrows tracked rows to those whose title contains query, ignoring
 * case. An empty query matches everything -- the toolbar's search is a
 * filter, not a required argument.
 */
export function filterTracked<T extends { show_title: string }>(rows: T[], query: string): T[] {
  const needle = query.trim().toLowerCase();
  if (needle === "") return rows;
  return rows.filter((r) => r.show_title.toLowerCase().includes(needle));
}
```

- [ ] **Step 4: Run the tests**

Run: `make web-check && make web-test`
Expected: PASS.

- [ ] **Step 5: Add the `/kit/` fixture**

Add a TRACKED-row fixture to `web/layouts/kit/list.html` covering: normal row, disabled row, completed row, error row, skeleton, empty state.

- [ ] **Step 6: Commit**

```bash
git add web/
git commit -m "feat(web): TRACKED pane on /history/, absorbing the series desk"
```

---

## Task 12: The AS-RUN pane

**Files:** `web/layouts/history/list.html`, `web/assets/ts/pages/history.ts`, `web/assets/css/main.css`, `web/tests/history.test.ts`

**Interfaces:**
- Consumes: `GET /api/v1/history?days=N` → the enriched `HistoryEntry[]` from Task 8.
- Produces: airings grouped by local day, newest first.

- [ ] **Step 1: Write the failing test**

```ts
const { groupByDay } = await import("../assets/ts/pages/history.ts");

test("groupByDay buckets airings by local date, newest day first", () => {
  const entries = [
    { scheduled_at: "2026-09-08T21:00:00Z", title: "A" },
    { scheduled_at: "2026-09-08T22:30:00Z", title: "B" },
    { scheduled_at: "2026-09-07T20:00:00Z", title: "C" },
  ];
  const groups = groupByDay(entries);
  assert.equal(groups.length, 2);
  assert.deepEqual(groups[0].entries.map((e) => e.title), ["A", "B"]);
  assert.deepEqual(groups[1].entries.map((e) => e.title), ["C"]);
});

test("groupByDay tolerates an empty list", () => {
  assert.deepEqual(groupByDay([]), []);
});
```

- [ ] **Step 2: Run it to verify it fails**

Run: `npm test --prefix web`
Expected: FAIL — `groupByDay is not a function`.

- [ ] **Step 3: Implement**

```ts
/** One local day's airings, in air order. */
export interface DayGroup<T> {
  /** Local YYYY-MM-DD, the group's stable key and heading source. */
  day: string;
  entries: T[];
}

/**
 * Buckets airings by the LOCAL day they were scheduled on -- the
 * operator reads a station log in station time, not UTC -- newest day
 * first, entries within a day in ascending air order.
 */
export function groupByDay<T extends { scheduled_at?: string }>(entries: T[]): DayGroup<T>[] {
  const buckets = new Map<string, T[]>();
  for (const entry of entries) {
    if (!entry.scheduled_at) continue;
    const d = new Date(entry.scheduled_at);
    const day = `${d.getFullYear()}-${pad2(d.getMonth() + 1)}-${pad2(d.getDate())}`;
    const bucket = buckets.get(day);
    if (bucket) bucket.push(entry);
    else buckets.set(day, [entry]);
  }
  return [...buckets.entries()]
    .sort((a, b) => (a[0] < b[0] ? 1 : a[0] > b[0] ? -1 : 0))
    .map(([day, list]) => ({
      day,
      entries: list.sort((a, b) => (a.scheduled_at ?? "").localeCompare(b.scheduled_at ?? "")),
    }));
}
```

`pad2` comes from `runtime/format.ts` — import it, do not redefine it. Rows render: local time (`formatClock`), channel `plate`, block name linking to `/blocks/#<id>` where the block is known, title + `sxxeyy` (also from `format.ts`). The window select and channel/block/search filters narrow client-side over the fetched window.

The empty state must say plainly that history is bounded by `maintenance.history_retention`, so a 90-day window can legitimately return seven days of rows.

- [ ] **Step 4: Run the tests**

Run: `make web-check && make web-test`
Expected: PASS.

- [ ] **Step 5: `/kit/` fixture + commit**

```bash
git add web/
git commit -m "feat(web): AS-RUN pane on /history/, airings grouped by local day"
```

---

## Task 13: The RUNS pane

**Files:** `web/layouts/history/list.html`, `web/assets/ts/pages/history.ts`, `web/assets/css/main.css`, `web/tests/history.test.ts`

**Interfaces:**
- Consumes: `GET /api/v1/applies?days=N&limit=M` → `ApplyRun[]` from Task 8.
- Produces: run cards with expandable warnings.

**Spec (§3.7, §3.2):** bordered instrument cards — timestamp, source badge `UI`/`CRON`/`CLI`, scope, slot × channel counts, warnings expandable inline naming loser, winner, and would-have-aired time with both blocks linked. Failed runs carry the error detail and a muted `REF` line. Expansion is a 200ms disclosure (`--duration-slow` is 250ms; use the 200ms token or add one per DESIGN.md's evidence discipline). A run links to the Guide at its window.

- [ ] **Step 1: Write the failing test**

```ts
const { runSummary } = await import("../assets/ts/pages/history.ts");

test("runSummary reads counts, scope and outcome", () => {
  assert.equal(
    runSummary({ status: "ok", source: "cron", scope: "", days: 7, channel_count: 3, slot_count: 14 }),
    "14 SLOTS ACROSS 3 CHANNELS · 7 DAYS · ALL CHANNELS",
  );
  assert.equal(
    runSummary({ status: "ok", source: "ui", scope: "ch-1", days: 1, channel_count: 1, slot_count: 1 }),
    "1 SLOT ACROSS 1 CHANNEL · 1 DAY · SCOPED",
  );
});

test("runSummary says what a failed or in-flight run means", () => {
  assert.equal(runSummary({ status: "error", source: "cli", days: 1, channel_count: 0, slot_count: 0 }), "FAILED");
  assert.equal(runSummary({ status: "running", source: "ui", days: 1, channel_count: 0, slot_count: 0 }), "IN FLIGHT");
});
```

- [ ] **Step 2: Run it to verify it fails**

Run: `npm test --prefix web`
Expected: FAIL — `runSummary is not a function`.

- [ ] **Step 3: Implement**

```ts
/** The shape runSummary reads -- a subset of the wire ApplyRun. */
export interface RunSummaryInput {
  status: string;
  source: string;
  scope?: string;
  days?: number;
  channel_count?: number;
  slot_count?: number;
}

/**
 * The card's one-line readout, in the guide's draft-bar register
 * (uppercase, counts first). A failed run reports the failure rather
 * than counts that never landed, and a run still marked running is
 * reported as in flight -- which, on a row whose process died, is the
 * honest reading of a finished_at that will never arrive.
 */
export function runSummary(run: RunSummaryInput): string {
  if (run.status === "error") return "FAILED";
  if (run.status === "running") return "IN FLIGHT";
  const slots = run.slot_count ?? 0;
  const channels = run.channel_count ?? 0;
  const days = run.days ?? 0;
  return [
    `${slots} ${plural(slots, "SLOT")} ACROSS ${channels} ${plural(channels, "CHANNEL")}`,
    `${days} ${plural(days, "DAY")}`,
    run.scope ? "SCOPED" : "ALL CHANNELS",
  ].join(" · ");
}
```

`plural` comes from `runtime/format.ts`; confirm it uppercases or wrap accordingly (the test above pins the expected strings — make the implementation match the test, or fix both together if `plural`'s contract differs).

Warnings render inside a `<details>`-backed disclosure (native, keyboard-accessible, no Alpine needed): each line names the dropped block, the blocking block, the would-have-aired time, the channel plate, and the duration.

The empty state teaches: the cron loop writes here too, and runs cannot be backfilled from before the migration.

- [ ] **Step 4: Run the tests**

Run: `make web-check && make web-test`
Expected: PASS.

- [ ] **Step 5: `/kit/` fixture + commit**

Fixtures: ok run, scoped run, failed run with `REF` line, running run, run with three warnings expanded, empty state, skeleton.

```bash
git add web/
git commit -m "feat(web): RUNS pane on /history/, apply cards with persisted warnings"
```

---

## Task 14: Design system, accessibility, and the page gate

**Files:** `web/DESIGN.md`, `web/assets/css/main.css`, `web/layouts/history/list.html`

- [ ] **Step 1: WCAG evidence for every new pairing**

Compute contrast ratios in **both** palettes for: the source badges (UI/CRON/CLI), the failed-run danger treatment, the warning chips, the segmented control's selected state, and the day-group headings. Record the computed values in `web/DESIGN.md` in the format the existing evidence tables use. Any pairing below 4.5:1 for text (3:1 for large text and UI boundaries) gets fixed, not documented as an exception.

- [ ] **Step 2: Keyboard and screen-reader pass**

Verify: the tablist supports Left/Right/Home/End with roving tabindex; every pane switch moves focus into the newly shown panel; the disclosure is reachable and its expanded state is announced; every interactive control has a visible focus ring that survives `forced-colors`; the table has real `<th scope=...>` headers.

- [ ] **Step 3: Document the page in DESIGN.md**

Add the `/history/` section: the three panes, the shared toolbar, the segmented-control idiom, the disclosure timing, the badge vocabulary, and the deletions (`/series/`, `/dashboard/`) with the date.

- [ ] **Step 4: Full front-end gate**

Run: `make web && make lint`
Expected: types clean, tests pass, Hugo builds, linters green.

- [ ] **Step 5: Run it against a real server**

```bash
make build && ./bin/schedularr --config config.yaml serve --listen :8484
```

Open `/history/`, `/history/?view=asrun`, `/history/?view=runs`. Confirm: a `GET /applies` populates the RUNS pane after one apply; a mistyped `?view=` lands on TRACKED; `/series/` and `/dashboard/` return the styled 404 with the new nav.

- [ ] **Step 6: Commit**

```bash
git add web/
git commit -m "docs(web): record the /history/ page in DESIGN.md with WCAG evidence"
```

---

# Phase C — Documentation and release

## Task 15: Documentation, changelog, roadmap

**Files:**
- Modify: `README.md`, `CLAUDE.md`, `AGENTS.md`, `GEMINI.md`
- Modify: `docs/web-ui-guide.md`, `docs/api-reference.md`, `docs/scheduling-concepts.md`, `docs/deployment.md`, `docs/architecture.md`, `docs/cli-reference.md`, `docs/roadmap.md`, `docs/index.md`
- Modify: `mkdocs.yml` (only if a page is added or renamed)
- Modify: `CHANGELOG.md`
- Modify: `configs/` sample config(s)

**Interfaces:** none — prose and configuration samples.

- [ ] **Step 1: Sweep for stale references**

```bash
grep -rn "/series/\|/dashboard/\|history_retention" README.md CLAUDE.md AGENTS.md GEMINI.md docs/ configs/ mkdocs.yml
```

Every hit is either a correct reference to the `/api/v1/state/series` **endpoint** (keep) or a reference to a deleted **UI route** / the old single retention knob (fix).

- [ ] **Step 2: Update `CLAUDE.md`'s architecture tree**

The `web/layouts/` and `web/assets/ts/pages/` listings name `series/list.html`, `series.ts`, and the dashboard. Replace with `history/list.html` and `history.ts`. The `## Configuration` section's description of `maintenance` gains the three retention keys.

- [ ] **Step 3: Update `docs/web-ui-guide.md`**

Replace the Series and Dashboard sections with one History section: the three panes, the shared filter toolbar, deep links (`/history/?view=asrun&block=…`), what each pane can and cannot do in this slice, and the honest limits — runs are not backfilled, and the window a pane can show is bounded by that table's retention.

- [ ] **Step 4: Update `docs/api-reference.md`**

Document `GET /applies` (parameters, response, the `running` status and why it can persist), the enriched `HistoryEntry` fields, and the extended `Warning`.

- [ ] **Step 5: Update `docs/deployment.md` and `docs/scheduling-concepts.md`**

`deployment.md`: the three retention knobs, their defaults, and what each bounds. `scheduling-concepts.md:278` currently explains that `history_retention` bounds `GET /history?days=N`; it must now also say that snapshots and apply runs prune on their own knobs, and that setting `snapshot_retention` beyond `history_retention` keeps rows that can never be replayed.

- [ ] **Step 6: Update the roadmap**

In `docs/roadmap.md`, convert the "**Then — Memory, landing as `/history/`:** pending." entry into a shipped entry in the same voice as the v0.5.6 one above it: what shipped, what was deleted, what was deliberately left to the History-desk slice (deletion tools, STORAGE strip, `app_meta.max_plan_seq`, bulk operations).

- [ ] **Step 7: Update `TODO.md`**

The Ladder note's second bullet says "Memory/`/history/` is next" — replace it with a v0.5.7 shipped line and name what is next. Add a "Deferred (v0.5.7 memory/history)" section for anything the implementation deliberately parked.

- [ ] **Step 8: Write the CHANGELOG entry**

Under `[Unreleased]` → a new `## [0.5.7]` heading, in the file's existing style, with `Added` / `Changed` / `Removed` sections. `Removed` must name `/series/` and `/dashboard/` explicitly. Note the config change (three retention keys) under `Changed` with the migration-free upgrade path (defaults apply).

- [ ] **Step 9: Clean the prose**

Run the `stop-slop` skill over every paragraph written in Steps 3–8.

- [ ] **Step 10: Verify and commit**

Run: `make lint && make test && make validate && make web`
Expected: all green.

```bash
git add -A
git commit -m "docs: v0.5.7 -- memory lands as /history/; /series/ and /dashboard/ removed"
```

- [ ] **Step 11: The two external surfaces (CLAUDE.md rule 3)**

These live outside this repo and are the operator's to apply — do not attempt to edit them from here. Report to the operator, with the text to paste:

1. **Cluster wiki** — `wiki/docs/applications/media/schedularr.md` in the applicationset repo: the new config keys, updated on the next image-pin bump.
2. **Obsidian note** — `PERSO/k8s-home/GXF Schedularr & Tunarr.md` in the vault at `~/Documents/main`: the `/history/` route replacing `/series/`, and the three retention knobs.

---

## Self-Review

**Spec coverage.**

| Spec requirement | Task |
| --- | --- |
| Apply-run persistence migration (v0.5 §9.2) | 1 |
| Runs written by UI, CLI **and cron-loop** applies alike | 6, 7 |
| 90-day run pruning in the same migration/slice | 2 (`CleanupApplyRuns`), 3 (default), 6 (call site) |
| `GET /applies` | 8 |
| Enriched `HistoryEntry` + `run_id` | 4, 5, 8 |
| Extended `Warning` (`channel_id`, `duration_minutes`) | 4, 8 |
| One `/history/` page, three panes, one filter toolbar (Q4) | 10–13 |
| Deep-linkable `?view=` | 10 |
| TRACKED replaces the whole `/series/` page | 11 |
| AS-RUN airings grouped by day, titles not UUIDs | 12 |
| RUNS cards with source badge, scope, counts, expandable warnings, failed-run detail + REF | 13 |
| `/series/` and `/dashboard/` deleted, no redirect stubs | 10 |
| Nav becomes `GUIDE · BLOCKS · HISTORY` | 10 |
| Retention split per table (Q10) | 3, 5, 6, 7 |
| Teaching empty states (runs not backfillable; history bounded) | 12, 13 |
| Gate: migrations from every 0.x database | 1 (Step 5) |
| Gate: an apply writes a run, end to end | 9 |
| WCAG evidence for new pairings | 14 |

**Deliberately out of scope** (History-desk slice, stated in Global Constraints): `DELETE /history`, `DELETE /state/sequences/{key}`, the STORAGE strip, `app_meta.max_plan_seq`, the I1–I5 deletion guards, bulk cursor operations, YAML import/export, SSE-driven prepending (Live-link slice).

**Placeholders:** none — every code step carries the code. The two places that say "read the existing helper and match it" (`resolveConflicts`'s local variable names, the `gen` package's generated field casing) are pointing at facts that only exist after `make generate` runs, not deferring design decisions.

**Type consistency:** `store.ApplyRun`/`ApplyRunWarning` field names are identical in Tasks 2, 6, and 8. `EngineOptions.RunID`/`SnapshotRetention` are named the same in Tasks 5 and 6. `service.RunnerOptions.HistoryRetention` maps to `EngineOptions.HistoryWindow` — deliberately different names for the same value, called out in Task 6's code. `Result.RunID` is set in `Run`, not `run` (Task 6), and read in Task 6's test only.
