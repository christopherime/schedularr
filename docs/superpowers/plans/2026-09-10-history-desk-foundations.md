# History Desk Foundations Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make it safe to delete scheduling history. Nothing in this slice deletes anything the operator can reach from the UI — it builds the four things a deletion needs underneath it, and closes an on-air hole that is open in shipped code today.

**Architecture:** Four independent pieces of foundation work, each shippable and verifiable on its own. One exported on-air predicate replaces the guard that does not exist, and is applied to all three paths that can already change what is playing. The plan-sequence allocator stops depending on two processes not overlapping. Instants get one representation in the database instead of the writer's local offset. And `schedule_history` gains the column that "remove this show" needs to have anything to filter on.

**Tech Stack:** Go 1.2x (stdlib `time`, `database/sql` transactions, `github.com/robfig/cron/v3`, stdlib `testing` with `-race`), SQLite via `mattn/go-sqlite3` and `jmoiron/sqlx`, numbered migrations under `internal/store/migrations/`.

**Spec:** `docs/superpowers/specs/2026-08-30-v1-station-terminology-media-history-design.md` — its §3.3 invariant audit and the operator's Q1–Q11 answers in §5. **Note that every line number in that audit has drifted** (the provenance guard it cites as `engine.go:1125` is now `engine.go:1182`; only `sqlite.go:457-468` still resolves). The code it describes is intact; the citations are not.

---

## Global Constraints

- **Lean codebase (CLAUDE.md rule 1).** No deprecation aliases, no transition shims. An exported function with no caller is a defect.
- **Docs ship in the same commit as the code** (CLAUDE.md rule 3), including `CLAUDE.md`, `AGENTS.md` and `GEMINI.md` when the package tree changes.
- **Linting limits:** cyclomatic 15, cognitive 20, nesting 5, results 3, arguments 5. `golangci-lint run` must report 0 issues.
- **Blocked packages:** `github.com/pkg/errors`, `logrus`, `crypto/md5`, `crypto/sha1`, `io/ioutil`, `gopkg.in/yaml.v1`, `gopkg.in/yaml.v2`.
- **Error wrapping:** always `fmt.Errorf("...: %w", err)`. `slog`, snake_case keys.
- **`internal/api/gen/server.gen.go` and `web/assets/ts/gen/types.d.ts` are generated.** Never hand-edit; change `api/openapi.yaml` and run `make generate` / `make web-types`.
- **`make test` green before every commit.** `gosec` currently fails on this toolchain for reasons predating this work — confirm it fails identically on `main` before dismissing it.
- **Migrations are forward-only in practice.** Write a `.down.sql`, but assume nobody runs it against real data.

## Decisions taken before this plan was written

Settled by the operator on 2026-09-10 after a four-agent recon. Do not re-litigate.

1. **`schedule_history` gains a `show_title` column.** The table stores `program.Title` — the *episode* title — and `ShowTitle` is never persisted, so `DELETE /history?title=` as the intake spec sketches it has nothing to filter on. Resolving it through the live Tunarr catalog instead was rejected: removal would fail whenever Tunarr is down, and a catalog that changed since the airing would silently resolve the wrong rows. **Accepted cost:** rows written before this migration cannot be removed by title, and the docs must say so plainly.
2. **A removal refuses while any block still lists the show.** `backfillChainFromLive` re-adds any `block.Series` title missing from the chain, seeded from a fabricated S01E01 default — so a removal that does not also take the show out of every block undoes itself on the next apply *and* resets the cursor. Silently rewriting block specs from a history deletion was rejected as too large a blast radius for one click. The refusal is a `409` naming the blocks.
3. **The on-air guard covers all three paths, not just removal.** `DeleteBlock` and `PatchBlock{enabled:false | disabled_until}` already make an on-air occurrence vanish at the next apply, and if it was the channel's last block, `clearStaleChannels` pushes a flex-only lineup — dead air mid-episode. Shipping a guard on removal alone would be one locked door beside two open ones.
4. **The "safe at" instant goes in the problem `Detail` as RFC3339 prose.** `problem.Problem` is a closed struct shared with `internal/api/middleware`; widening it for one caller was rejected.
5. **`app_meta` is NOT built.** The roadmap's stated prerequisite — move the plan-sequence floor into `app_meta` so a deletion cannot lower it — solves a misdiagnosed invariant. `MaxPlanSeq` computes `MAX` over *surviving* rows and spans `series_state.cursor_plan_seq`, so the post-deletion floor is by construction ≥ every surviving cursor. The real hazard is a concurrent-allocation race (Task 3), and a persisted counter does not fix it. It would also turn every dry run into a database write, since `nextPlanSeq` sits on the `GenerateForTimeRange` path that `Run(Apply:false)` takes.
6. **The desk UI is not in this slice.** Bulk cursor operations, YAML import/export, range cleanup and the STORAGE strip move to v0.5.12. This slice ships no new UI.

## Explicitly out of scope

- **Any user-reachable deletion.** No `DELETE` endpoint ships here. Task 5 builds the transactional primitive; the endpoint that calls it is v0.5.12's.
- **Making `Engine.Commit` transactional.** Open since v0.3.0 (`TODO.md:365`) and genuinely worth doing, but it is a separate change with its own risk, and this slice must not grow to include it. Task 5's deletion primitive is transactional on its own terms.
- **Backfilling `show_title` for rows whose program is gone from Tunarr.** Unrecoverable by construction. Documented, not worked around.
- **A `GET /onair` endpoint.** Ruled out for all of v0.5.x: Schedularr knows the applied lineup, not what Tunarr is emitting, and an instrument that renders a reading it cannot measure lies. Task 2's predicate is internal.

## File Structure

**Created:**

- `internal/store/migrations/000012_history_show_title.up.sql` / `.down.sql`
- `internal/store/migrations/000013_normalize_instants.up.sql` / `.down.sql`
- `internal/scheduler/onair.go` — the exported on-air predicate.
- `internal/scheduler/onair_test.go`
- `internal/store/removal.go` — the transactional removal primitive.
- `internal/store/removal_test.go`

**Modified:**

- `internal/scheduler/engine.go` — export the on-air computation; the allocator.
- `internal/scheduler/state.go`, `internal/scheduler/interfaces.go` — allocator contract.
- `internal/store/sqlite.go` — instant binding, `blockReferencesShow`, `MaxPlanSeq`.
- `internal/api/blocks.go` — the guard on `DeleteBlock` and `PatchBlock`.
- `internal/service/schedule.go` — history writes carry `show_title`.
- `CLAUDE.md`, `AGENTS.md`, `GEMINI.md`, `docs/architecture.md`, `docs/scheduling-concepts.md`, `CHANGELOG.md`, `docs/roadmap.md`, `TODO.md`.

---

# Phase A — Representation

## Task 1: One representation for stored instants

**Files:**

- Create: `internal/store/migrations/000013_normalize_instants.up.sql` / `.down.sql`
- Modify: `internal/store/sqlite.go`
- Test: `internal/store/sqlite_test.go`

**The defect, demonstrated.** SQLite stores these `DATETIME` columns as TEXT carrying the *writer's* offset, and compares them bytewise. Two rows that are the **same instant** written as `2026-06-01 08:00:00+00:00` and `2026-06-01 10:00:00+02:00`, queried with a cutoff of `09:00Z`, return **one row instead of two**. Verified against the real driver.

**Why it does not bite today, and why that is not a reason to leave it.** Same-zone data compares correctly because the date prefix dominates: `2026-01-15` sorts before `2026-03-01` whatever the offset suffix. It breaks when offsets *differ* and the values sit within the offset delta — which happens when the operator changes `log.timezone`, or when two writers run in different zones. v0.5.12's range cleanup hands the operator an arbitrary cutoff over data that may span exactly that, so this moves from latent to load-bearing. Fixing it now means touching stored instants once rather than twice.

**Interfaces:**

- Produces: every `DATETIME` column stored as UTC, in one format. No Go API change — the fix is in how values are bound and in a one-time rewrite.

- [ ] **Step 1: Write the failing test**

```go
func TestInstantRangeIsIndependentOfWriterOffset(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()

	zurich, err := time.LoadLocation("Europe/Zurich")
	if err != nil {
		t.Skipf("tzdata unavailable: %v", err)
	}

	// The SAME instant, written twice, in two zones. A store that compares
	// instants returns both for any cutoff after them; a store that
	// compares text returns whichever one happens to sort right.
	instant := time.Date(2026, 6, 1, 8, 0, 0, 0, time.UTC)
	writeHistoryAt(t, st, "prog-utc", instant)
	writeHistoryAt(t, st, "prog-zur", instant.In(zurich))

	cutoff := instant.Add(time.Hour)
	n, err := st.CleanupHistory(ctx, time.Since(cutoff))
	if err != nil {
		t.Fatalf("CleanupHistory: %v", err)
	}
	if n != 2 {
		t.Fatalf("pruned %d rows, want 2 -- comparison is representation-dependent", n)
	}
}

func TestStoredInstantsAreUTC(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	zurich, err := time.LoadLocation("Europe/Zurich")
	if err != nil {
		t.Skipf("tzdata unavailable: %v", err)
	}
	writeHistoryAt(t, st, "p1", time.Date(2026, 6, 1, 10, 0, 0, 0, zurich))

	var raw string
	if err := rawDB(t, st).Get(&raw, `SELECT CAST(scheduled_at AS TEXT) FROM schedule_history WHERE program_id = 'p1'`); err != nil {
		t.Fatalf("read raw: %v", err)
	}
	if !strings.HasSuffix(raw, "+00:00") && !strings.HasSuffix(raw, "Z") {
		t.Fatalf("stored %q -- a local offset reached the column", raw)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test -race ./internal/store/ -run 'InstantRange|StoredInstantsAreUTC'`
Expected: FAIL — the first prunes 1 row, the second finds a `+02:00` suffix.

- [ ] **Step 3: Bind every instant as UTC**

Find every place this package binds a `time.Time` into a query and convert with `.UTC()` at the binding site. Do not convert at the *call* site — the point is that no caller has to remember. Add a comment recording why:

```go
// Every instant this package stores is bound as UTC, because SQLite keeps
// a DATETIME as TEXT carrying whatever offset the writer had and compares
// it BYTEWISE. Two rows one hour apart written in different zones can
// therefore order backwards, and a range predicate can miss one. Storing
// one representation makes the comparison mean what it reads like.
```

- [ ] **Step 4: Rewrite the existing rows**

`000013_normalize_instants.up.sql`. Every `DATETIME` column in `schedule_history`, `series_occurrence_snapshots`, `series_state`, `apply_runs`, and `blocks` is rewritten to UTC in one statement per column, using SQLite's `datetime(col)` which understands the offset suffix. The `.down.sql` cannot restore the original offsets — they are not recoverable — so it is a no-op with a comment saying so.

**Check before you write it:** any exact-match lookup on one of these columns (`WHERE occurrence_start = ?`) must still find its row after the rewrite. Grep for them and list them in your commit message.

- [ ] **Step 5: Run the tests**

Run: `make test`
Expected: PASS, including the existing retention tests.

- [ ] **Step 6: Commit**

---

## Task 2: `schedule_history.show_title`

**Files:**

- Create: `internal/store/migrations/000012_history_show_title.up.sql` / `.down.sql`
- Modify: `internal/store/sqlite.go`, `internal/scheduler/state.go` (the history entry type), `internal/service/schedule.go`
- Test: `internal/store/sqlite_test.go`

**Interfaces:**

- Produces: `ScheduleHistoryEntry.ShowTitle string` — the *show*, not the episode. Empty for a filter block's airings and for every row written before this migration.

- [ ] **Step 1: Write the failing test**

```go
func TestHistoryCarriesTheShowTitle(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()

	// A series airing knows its show; the existing `title` column holds
	// the EPISODE, which is why removing "everything from Bloody Mary"
	// had nothing to match on.
	entry := scheduler.ScheduleHistoryEntry{
		ProgramID: "p1", ChannelID: "ch1", BlockName: "Anime Night",
		Title: "The Empty Room", ShowTitle: "Bloody Mary",
		ScheduledAt: time.Now().UTC(),
	}
	if err := st.RecordHistory(ctx, []scheduler.ScheduleHistoryEntry{entry}); err != nil {
		t.Fatalf("RecordHistory: %v", err)
	}

	rows, err := st.HistorySince(ctx, time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatalf("HistorySince: %v", err)
	}
	if len(rows) != 1 || rows[0].ShowTitle != "Bloody Mary" {
		t.Fatalf("show_title did not round-trip: %+v", rows)
	}
	if rows[0].Title != "The Empty Room" {
		t.Fatal("show_title overwrote the episode title")
	}
}

func TestFilterBlockAiringsHaveNoShowTitle(t *testing.T) {
	t.Parallel()
	// A filter block picks programs by criteria, not by show. Empty is the
	// honest value, and a removal by title must never match it.
	st := newTestStore(t)
	ctx := context.Background()
	entry := scheduler.ScheduleHistoryEntry{
		ProgramID: "p1", ChannelID: "ch1", BlockName: "Late Movies",
		Title: "Solaris", ScheduledAt: time.Now().UTC(),
	}
	if err := st.RecordHistory(ctx, []scheduler.ScheduleHistoryEntry{entry}); err != nil {
		t.Fatalf("RecordHistory: %v", err)
	}
	rows, _ := st.HistorySince(ctx, time.Now().Add(-time.Hour))
	if rows[0].ShowTitle != "" {
		t.Fatalf("ShowTitle = %q, want empty for a filter block", rows[0].ShowTitle)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test -race ./internal/store/ -run ShowTitle`
Expected: FAIL — `ShowTitle` undefined.

- [ ] **Step 3: Migrate and thread it**

```sql
-- The `title` column has always held the EPISODE title (program.Title),
-- so nothing in this schema identified a SHOW's airings -- which is why
-- "remove everything from Bloody Mary" had no predicate to run.
--
-- Empty is a real value here, not a missing one: a filter block schedules
-- by criteria and its airings belong to no show. A removal by title must
-- match on a non-empty value and never sweep these.
--
-- Rows written before this migration keep the default. They cannot be
-- backfilled: recovering a show from a program_id needs the live Tunarr
-- catalog, which may no longer carry that program, and guessing from the
-- block's spec is wrong for any block scheduling more than one show.
ALTER TABLE schedule_history ADD COLUMN show_title TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_schedule_history_show ON schedule_history (show_title);
```

Add `ShowTitle` to the history entry type and to both `INSERT` sites in `sqlite.go` (there are two — grep for `INSERT INTO schedule_history`). In `internal/scheduler`, populate it where a series occurrence's programs are recorded: the engine knows the show it is planning at that point.

- [ ] **Step 4: Run the tests**

Run: `make test`
Expected: PASS.

- [ ] **Step 5: Commit**

---

# Phase B — Safety

## Task 3: The allocator race

**Files:**

- Modify: `internal/scheduler/engine.go`, `internal/store/sqlite.go`
- Test: `internal/scheduler/engine_test.go`, `internal/store/sqlite_test.go`

**The defect.** `nextPlanSeq` is wall-clock nanos with a floor seeded from `MaxPlanSeq` at engine construction. `serve` and a CLI `generate --apply` are an explicitly supported concurrent pair — WAL and `_busy_timeout=5000` were added to `sqlite.go` for exactly that. Both can construct an engine, both read the same floor before either commits, and both allocate from the same nanosecond neighbourhood. Whichever commits second can produce a sequence `<=` the one already stored, and the provenance guard at `engine.go:1182` then **silently drops** that run's post-state:

```go
if snap.PlanSeq <= existing.CursorPlanSeq {
    continue
}
```

No log, no warning, no error. A series cursor stops advancing and nothing says why.

**What to build.** The floor must be reserved, not observed. `MaxPlanSeq` becomes an allocation: one statement that raises the stored high-water mark and returns the value reserved, so two processes cannot be handed overlapping ranges. Reserve a *block* of sequences per engine rather than one per occurrence — the engine allocates at two call sites per occurrence and a round trip each would be absurd.

- [ ] **Step 1: Write the failing test**

```go
func TestConcurrentEnginesNeverShareASequence(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()

	// Two engines over ONE store, constructed before either commits --
	// the serve + `generate --apply` shape that WAL exists to support.
	const engines = 8
	const perEngine = 32

	var mu sync.Mutex
	seen := make(map[int64]int)
	var wg sync.WaitGroup
	for i := 0; i < engines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			e := scheduler.NewEngineWithOptions(ctx, nil, nil, st, scheduler.EngineOptions{})
			for j := 0; j < perEngine; j++ {
				seq := scheduler.ExportedNextPlanSeq(e)
				mu.Lock()
				seen[seq]++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	for seq, count := range seen {
		if count > 1 {
			t.Fatalf("sequence %d handed out %d times -- two runs can silently drop each other's post-state", seq, count)
		}
	}
	if len(seen) != engines*perEngine {
		t.Fatalf("got %d distinct sequences, want %d", len(seen), engines*perEngine)
	}
}

func TestAllocationSurvivesAReadFailure(t *testing.T) {
	t.Parallel()
	// Construction currently swallows a MaxPlanSeq error and falls back to
	// the wall clock. Decide what an ALLOCATION failure means and pin it:
	// silently falling back reopens the exact race this task closes.
	t.Skip("write this once the failure policy is chosen in Step 3")
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test -race ./internal/scheduler/ -run ConcurrentEngines`
Expected: FAIL — duplicate sequences, or a compile error for the missing export.

- [ ] **Step 3: Implement**

Decide and document the failure policy: an allocation that cannot reach the store is **not** the same as a floor that cannot be read. The current best-effort seeding is defensible because the wall clock covers it; a failed *reservation* is not, because two processes then both fall back and collide. Say which way you went and why, in the code.

Add a `sync.Mutex` around the in-memory hand-out. The current comment claims "engines are single-run, not concurrent, so no locking" — that is true of one engine and irrelevant to two.

- [ ] **Step 4: Run the tests**

Run: `go test -race ./internal/scheduler/... ./internal/store/...`
Expected: PASS, with `-race` clean.

- [ ] **Step 5: Make the drop audible**

The guard at `engine.go:1182` silently skips. Even with allocation fixed, a drop means something is wrong. Log it at `Warn` with `show_title`, both sequences, and the run id. A guard that protects a real invariant should say when it fires.

- [ ] **Step 6: Commit**

---

## Task 4: The on-air predicate, and the three paths that need it

**Files:**

- Create: `internal/scheduler/onair.go`, `internal/scheduler/onair_test.go`
- Modify: `internal/scheduler/engine.go` (`onAirOccurrenceStart` moves), `internal/store/sqlite.go` (`blockReferencesShow`), `internal/api/blocks.go`
- Test: `internal/api/blocks_test.go`

**Interfaces:**

- Produces: `scheduler.OnAirOccurrences(blocks []Block, now time.Time, loc *time.Location) []OnAir` where `OnAir{Block, Start, SafeAt}`. `SafeAt` is `Start + (Duration + MaxDurationOverflowMinutes)`.

**Three things the recon found that will make a naive implementation wrong:**

1. **The envelope.** Two incompatible definitions exist. `onAirOccurrenceStart` uses `Duration` alone; `store.InvalidationCutoff` adds `MaxDurationOverflowMinutes` and its doc comment argues that overflow *is* airing. Use the **wider** one — the same removal must also invalidate snapshots, and that path already uses this envelope. The nominal one would let the guard clear while the invalidation path still classes the occurrence as live.
2. **The timezone.** `GenerateForTimeRange` does `start = start.In(e.location)` *before* walking the cron, because a `SpecSchedule` matches calendar fields against whatever location its argument carries. A guard computing on-air from a bare `time.Now()` in a container with no `TZ` picks a different occurrence than the apply will. The exported function takes the location explicitly so a caller cannot forget.
3. **`blockReferencesShow` only sees series blocks.** It scans `spec.Series[].ShowTitle` (`sqlite.go:444-451`). A filter block schedules titles that appear nowhere in its spec, so a guard driven off it silently misses that case. For the on-air check, scan **every active block's** occurrence, not only those that name the show — an occurrence is on air or it is not, regardless of how its content was chosen.

- [ ] **Step 1: Write the failing tests**

Cover: an occurrence containing `now` is on air; one that ended is not; one starting later is not; an occurrence still inside its overflow window **is**; `SafeAt` equals start plus the widened envelope; a dark or disabled block contributes nothing (it will not be generated at the next apply, so deleting its history cannot change playback); and the location argument actually changes which occurrence is found across a DST boundary.

- [ ] **Step 2: Run to verify they fail**

- [ ] **Step 3: Implement, and move the unexported helper**

`onAirOccurrenceStart` moves into `onair.go` unchanged. The exported wrapper is thin: parse each block's cron with `NewCronParser`, ask the helper, and return those that are live with their `SafeAt`.

- [ ] **Step 4: Guard all three paths**

`DeleteBlock` and `PatchBlock` (both the `enabled:false` and the future-`disabled_until` cases) gain the same check. A refusal is:

```text
409  title:  "block is on air"
     detail: "Anime Night is airing until 21:47. Deleting it now would
              cut the current episode. Try again after that."
```

The instant is RFC3339-derived prose in `Detail` (decision 4). Extract one helper that formats the refusal so the three sites cannot drift.

**Ask before you write the copy:** the existing 409s name a colliding *name*; none names a *time*. Read `blocks.go:249`'s 412 for the house voice — it names what changed and the remedy.

- [ ] **Step 5: Run the tests**

Run: `make test`
Expected: PASS. Existing `DeleteBlock` tests that delete a block whose cron happens to be live now get a 409 — fix the fixtures' crons, do not weaken the guard.

- [ ] **Step 6: Commit**

---

## Task 5: The transactional removal primitive

**Files:**

- Create: `internal/store/removal.go`, `internal/store/removal_test.go`
- Modify: `internal/store/sqlite.go`

**Interfaces:**

- Produces: `(*Store).RemoveShow(ctx, showTitle string) (RemovalReport, error)` — one transaction across `series_state`, `series_occurrence_snapshots` and `schedule_history`. `RemovalReport` carries per-table counts.

**No endpoint calls this yet.** That is deliberate: the primitive is testable on its own, and the endpoint belongs to v0.5.12 with the UI that drives it. It is not speculative under rule 1 — its test is its caller, and the slice that consumes it is the next one.

**Two subtleties from Q5 that a straightforward `DELETE` gets wrong:**

- **Aired snapshots lose only the removed title's key**, rather than the row. Read how a snapshot is stored before writing this: if it is a JSON blob keyed by title, the removal *edits* stored data rather than deleting a row, and `SaveOccurrenceSnapshot`'s upsert currently sets `plan_seq = excluded.plan_seq` — rewriting a snapshot must not disturb its provenance.
- **A `NULL` `post_state_json` must stay `NULL`.** Turning it into `{}` flips `replayAiredOccurrence` from "re-seed the chain from live state" to "apply nothing", a silent behaviour change for every co-scheduled title in that occurrence.

- [ ] **Step 1: Write the failing tests**

Cover: all three tables lose the show's rows in one transaction; a failure partway leaves **nothing** removed; another show's rows in the same occurrence survive; an aired snapshot keeps its row and its `plan_seq` but loses the title's key; a `NULL` `post_state_json` is still `NULL` afterwards; and rows with an empty `show_title` (filter-block airings, and everything predating migration 000012) are never matched.

- [ ] **Step 2: Run to verify they fail**

- [ ] **Step 3: Implement**

Use the package's transaction helper if one exists; if every write is currently autocommit, introduce the helper here and say so in the commit message — it is a real addition, not a refactor.

- [ ] **Step 4: Add the block-reference refusal**

`RemoveShow` returns a typed error when any block still lists the show, carrying the block names. Decision 2: the removal refuses rather than rewriting specs. `blockReferencesShow` is the right check *here* — unlike the on-air guard, this question really is "does a block name this show".

- [ ] **Step 5: Run the tests**

Run: `make test`

- [ ] **Step 6: Commit**

---

# Phase C — Documentation and release

## Task 6: Docs

- [ ] `docs/scheduling-concepts.md`: what "on air" means to the guard, the widened envelope and why, and that a removal refuses while a block still lists the show.
- [ ] `docs/api-reference.md`: the new `409` on `DELETE /blocks/{id}` and `PATCH /blocks/{id}`, with the `Detail` shape.
- [ ] `docs/architecture.md`: `internal/scheduler/onair.go` and `internal/store/removal.go` in the tree; a note that instants are stored UTC and why.
- [ ] `docs/deployment.md`: migration 000013 rewrites every stored instant. Back up the database first — this is the first migration that touches existing row *values* rather than adding a column.
- [ ] `CLAUDE.md`, `AGENTS.md`, `GEMINI.md`: the new files in the architecture tree.
- [ ] `CHANGELOG.md`: a `[0.5.11]` section. Say plainly that no user-reachable deletion ships, and that the on-air guard closes a hole that was open in `DeleteBlock` and `PatchBlock` since they existed.
- [ ] `docs/roadmap.md`: split the desk entry — foundations shipped at v0.5.11, the desk UI pending at v0.5.12. Record that `app_meta` was dropped and why.
- [ ] `TODO.md`: a "Deferred (history desk foundations)" section, including `Engine.Commit`'s non-transactional shape and the unbackfillable pre-000012 rows.
- [ ] Run `stop-slop` over the prose and `markdownlint-cli2` against the baseline.
- [ ] Paste text for the cluster wiki and the Obsidian note. **Migration 000013 is an operational change worth naming:** it rewrites stored values, so the operator wants a backup before the image bump.

---

## Self-Review

**Spec coverage.** The roadmap's four v0.5.11 deliverables: bulk cursor operations → **deferred to v0.5.12** (decision 6); YAML import/export → **deferred**; remove a show or movie from history → **foundations only**, Tasks 2 and 5, with no endpoint; range cleanup → **deferred**, but its blocker is fixed in Task 1. The two engine prerequisites: the plan-sequence floor → **replaced** by Task 3 after the recon showed I5 misdiagnosed (decision 5); the transactional delete → Task 5.

**Placeholder scan.** One deliberate `t.Skip` in Task 3 Step 1, which Step 3 removes once the failure policy is chosen — the choice is real and the test cannot be written before it. Task 5's tests are described rather than written out because their shape depends on the snapshot storage format, which Step 1 requires reading first; writing test bodies against a guessed format would be worse than naming what they must cover.

**Type consistency.** `ScheduleHistoryEntry.ShowTitle` (Task 2) is filtered on by `RemoveShow` (Task 5). `scheduler.OnAirOccurrences` (Task 4) is called by `DeleteBlock` and `PatchBlock` in the same task. `RemovalReport` is returned by Task 5 and consumed by nothing until v0.5.12 — stated openly above.

**The risk this plan does not remove.** Task 1 rewrites every stored instant in place. If any exact-match lookup on those columns is missed, it silently stops finding its rows — and unlike a range bug, an exact-match miss looks like missing data rather than a wrong answer. Step 4 requires grepping and listing them; treat that list as the review's focus, not the migration SQL.
