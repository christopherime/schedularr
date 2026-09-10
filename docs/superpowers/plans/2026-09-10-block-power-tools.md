# Block Power Tools Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
>
> **UI tasks (9–14) additionally REQUIRE the `impeccable` skill** — CLAUDE.md rule 4. Prose written for docs and the CHANGELOG gets cleaned with `stop-slop`.

**Goal:** Give the blocks page the answers an operator needs before they commit an edit — when this block next airs, what it will displace, and what the change will do to a series already mid-run — plus the two write operations that turn a block into a working set: duplicate, and go dark until a date.

**Architecture:** Occurrence math moves server-side and stays there. A new exported `scheduler.NextOccurrences` wraps the cron parser the engine already holds, and both `GET /cron/next` and each `BlockRecord.next_occurrence` read it, so the client never re-implements calendar semantics. `disabled_until` becomes a real column beside `enabled` rather than a field inside the spec blob, which puts both dark-switches in one row and one write. The single planning gate — `service.ActiveBlocks` — learns about it, and because every path (cron loop, CLI, API) already funnels through that one function, the gate is honoured everywhere by construction.

**Tech Stack:** Go 1.2x (stdlib `time`, `github.com/robfig/cron/v3` already vendored, stdlib `testing` with `-race`), chi router, oapi-codegen v2, CUE (config + block schema), SQLite with numbered migrations, Hugo + hand-written CSS + Alpine.js (vendored) + TypeScript (`node --test`).

**Spec:** `docs/superpowers/specs/2026-08-30-v0.5-web-overhaul-design.md` — section 9 item 4 ("Block power tools"), the endpoint sketches for `POST /api/v1/blocks/{id}/duplicate`, `PATCH /api/v1/blocks/{id}`, and `GET /api/v1/cron/next`, and section 10's rejection of client-side cron evaluation.

---

## Global Constraints

- **Lean codebase (CLAUDE.md rule 1).** No deprecation aliases, no transition shims, no commented-out remnants. An exported function with no caller is a defect, not scaffolding.
- **Docs ship in the same commit as the code** (CLAUDE.md rule 3). `docs/api-reference.md`, `docs/web-ui-guide.md`, `web/DESIGN.md`, `CHANGELOG.md`, and `docs/roadmap.md` are all in scope.
- **Linting limits:** cyclomatic complexity 15, cognitive complexity 20, nesting depth 5, function results 3, arguments 5. `golangci-lint run` must report 0 issues.
- **Blocked packages:** `github.com/pkg/errors`, `logrus`, `crypto/md5`, `crypto/sha1`, `io/ioutil`, `gopkg.in/yaml.v1`, `gopkg.in/yaml.v2`.
- **Error wrapping:** always `fmt.Errorf("...: %w", err)`. Structured logging via `slog`, snake_case keys.
- **`internal/api/gen/server.gen.go` is generated.** Never hand-edit it; change `api/openapi.yaml` and run `make generate`. Commit the regenerated file.
- **`web/assets/ts/gen/types.d.ts` is generated** by `make web-types` from the same contract. Never hand-edit.
- **No new npm dependencies.** Alpine.js and cronstrue are vendored under `web/assets/vendor/`; nothing else is permitted, and no CDN.
- **The client never evaluates cron semantics.** cronstrue renders prose only. Every instant shown to the operator is computed by the server. This is a spec-level rejection (section 10), not a preference.
- **`make test` green before every commit.** `make lint` runs golangci-lint, gosec, govulncheck, web-check and web-test; gosec currently fails on this toolchain for reasons unrelated to this work (a Go 1.27 incompatibility that reproduces on a clean `main`) — the other four must pass.

## Decisions taken before this plan was written

These were open in the spec and are settled here. Do not re-litigate them mid-implementation.

1. **`disabled_until` is a column on the block record, not a field on `BlockSpec`.** The spec sketched it on `BlockSpec`. That would put it in `spec_json` while `enabled` lives in a column — two storage locations that cannot be written atomically, and every disable-until write would become a full spec rewrite through the same path that carries `If-Match`. A column keeps both dark-switches in one row and one write. **Consequence, stated plainly:** `disabled_until` does not round-trip through `scheduler.yaml` export/import. That file is a first-run import format for block *specs*; a temporary dark window is operational state, not spec.
2. **`enabled` and `disabled_until` are independent axes.** `enabled: false` means off indefinitely and only an operator turns it back on. `disabled_until: <t>` means dark now, back by itself. A block is planned only when `enabled == true` **and** it is not currently dark. Setting one never writes the other.
3. **A duplicated block arrives disabled.** An exact copy carries the same cron, channel, and priority as its source, so landing it enabled would immediately contend for that channel at that time and conflict resolution would silently drop one of the two. The copy arrives as a draft the operator edits and then turns on.
4. **`next_occurrence` means "the next time this block will actually air".** It respects both switches: a dark block reports the first occurrence at or after it wakes, and a disabled block reports nothing at all. A column reading `NEXT THU 21:00` beside a block sitting dark until next month is a reading that lies.
5. **`GET /cron/next` evaluates in the configured `log.timezone`**, not a hardcoded `Europe/Zurich`. The spec's prose named the operator's own deployment value. The config default is the string `"Local"`, and `cmd/serve.go` already resolves it to a `*time.Location`; this plan threads that existing value into `api.Deps` rather than inventing a constant.
6. **`POST` and `PUT /blocks` reject an unparseable cron with `400`.** Today an invalid expression is accepted by CUE (which types `cron` as a bare string) and fails later inside the engine as a `502` at apply time. Rejecting at write time is a behaviour change and it is the right one: the operator finds out while looking at the field they typed.
7. **A block that is dark still counts as a competitor for the shared-show policy check.** It is defined and it will come back. Ignoring it would let a second block claim the same show, and the collision would surface the moment the dark one woke.
8. **`POST /blocks/{id}/duplicate` takes a body `{name}`**, required. The endpoint sketch and the UI note in the spec are compatible: the contract requires a name, and the client pre-fills `Copy of <source>`. The server never invents one, so a name collision is the caller's to resolve rather than something the server papers over with a counter.

## Explicitly out of scope

- **Bulk operations on blocks** (multi-select, bulk enable/disable). That is the desk slice's shape, applied to a different noun.
- **A `disabled_until` on series rows or channels.** Blocks only.
- **Recurring dark windows** ("dark every August"). One instant, one wake-up. A recurrence rule here would be a second scheduling language competing with cron.
- **Changing the name-collision rule to case-insensitive.** Today's SQLite `BINARY UNIQUE` stands; making it `COLLATE NOCASE` is a migration plus a decision about existing rows, and nothing in this slice needs it.
- **`GET /cron/next` returning end times.** It returns start instants. The consequence rail adds the block's own duration, which it already holds.

## File Structure

**Created:**

- `internal/store/migrations/000011_block_disabled_until.up.sql` / `.down.sql` — the column.
- `internal/scheduler/occurrence.go` — `NextOccurrences`, the one exported occurrence generator.
- `internal/scheduler/occurrence_test.go`
- `internal/api/cron.go` — the `GET /cron/next` handler.
- `internal/api/cron_test.go`
- `web/assets/ts/runtime/rank.ts` — the priority-rank helper, extracted from `guide.ts`'s inspector so the list and the inspector agree.
- `web/tests/rank.test.ts`
- `web/tests/blocks-power.test.ts` — pure predicates for the new row actions and the consequence rail.

**Modified:**

- `internal/store/blocks.go` — `DisabledUntil` on the record, in every scan and write.
- `internal/service/schedule.go` — `ActiveBlocks` takes a clock and applies the dark gate.
- `cmd/generate.go:253` — the second `ActiveBlocks` caller.
- `internal/scheduler/engine.go` — export the parser construction so `occurrence.go` shares exactly one parser configuration.
- `internal/api/blocks.go` — `duplicate`, `PATCH` extension, cron validation, `next_occurrence` in `toGen`.
- `internal/api/server.go` — `Location` and `Sched` on `Deps`.
- `cmd/serve.go` — wire the existing `loc` into `Deps`.
- `api/openapi.yaml` — two new paths, two new schema fields.
- `cmd/schema/scheduler.cue` — nothing. `disabled_until` is not spec.
- `web/layouts/blocks/list.html`, `web/assets/ts/pages/blocks.ts`, `web/assets/css/main.css`, `web/layouts/kit/list.html`, `web/DESIGN.md`.
- `web/assets/ts/pages/guide.ts` — only to import the extracted rank helper it previously owned.

---

# Phase A — The engine and the contract

## Task 1: The `disabled_until` column

**Files:**

- Create: `internal/store/migrations/000011_block_disabled_until.up.sql`, `internal/store/migrations/000011_block_disabled_until.down.sql`
- Modify: `internal/store/blocks.go`
- Test: `internal/store/blocks_test.go`

**Interfaces:**

- Produces: `store.BlockRecord.DisabledUntil *time.Time` — nil means "no dark window". A non-nil instant in the past is *stale, not cleared*: nothing sweeps it, and the gate in Task 2 simply stops suppressing once it passes. Readers that render a chip must compare against now rather than trusting non-nil to mean "dark".

- [ ] **Step 1: Write the failing test**

Add to `internal/store/blocks_test.go`:

```go
func TestBlockDisabledUntilRoundTrips(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()

	until := time.Date(2026, 12, 1, 9, 0, 0, 0, time.UTC)
	rec := store.BlockRecord{
		ID:            "b1",
		Name:          "Dark Block",
		Enabled:       true,
		DisabledUntil: &until,
		Spec:          scheduler.Block{Name: "Dark Block", Cron: "0 6 * * *", Duration: 60, ChannelID: "ch1"},
	}
	if err := st.CreateBlock(ctx, &rec); err != nil {
		t.Fatalf("CreateBlock: %v", err)
	}

	got, err := st.GetBlock(ctx, "b1")
	if err != nil {
		t.Fatalf("GetBlock: %v", err)
	}
	if got.DisabledUntil == nil {
		t.Fatal("DisabledUntil round-tripped as nil")
	}
	if !got.DisabledUntil.Equal(until) {
		t.Fatalf("DisabledUntil = %v, want %v", got.DisabledUntil, until)
	}
}

func TestBlockDisabledUntilClearsToNil(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()

	until := time.Date(2026, 12, 1, 9, 0, 0, 0, time.UTC)
	rec := store.BlockRecord{
		ID: "b1", Name: "Dark Block", Enabled: true, DisabledUntil: &until,
		Spec: scheduler.Block{Name: "Dark Block", Cron: "0 6 * * *", Duration: 60, ChannelID: "ch1"},
	}
	if err := st.CreateBlock(ctx, &rec); err != nil {
		t.Fatalf("CreateBlock: %v", err)
	}

	rec.DisabledUntil = nil
	if err := st.UpdateBlock(ctx, &rec); err != nil {
		t.Fatalf("UpdateBlock: %v", err)
	}

	got, err := st.GetBlock(ctx, "b1")
	if err != nil {
		t.Fatalf("GetBlock: %v", err)
	}
	if got.DisabledUntil != nil {
		t.Fatalf("DisabledUntil = %v, want nil after clear", got.DisabledUntil)
	}
}

func TestExistingBlocksMigrateToNullDisabledUntil(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()

	// A block written without ever mentioning the new column.
	rec := store.BlockRecord{
		ID: "b1", Name: "Plain", Enabled: true,
		Spec: scheduler.Block{Name: "Plain", Cron: "0 6 * * *", Duration: 60, ChannelID: "ch1"},
	}
	if err := st.CreateBlock(ctx, &rec); err != nil {
		t.Fatalf("CreateBlock: %v", err)
	}
	got, err := st.GetBlock(ctx, "b1")
	if err != nil {
		t.Fatalf("GetBlock: %v", err)
	}
	if got.DisabledUntil != nil {
		t.Fatalf("DisabledUntil = %v, want nil", got.DisabledUntil)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test -race ./internal/store/ -run TestBlockDisabledUntil`
Expected: FAIL — `rec.DisabledUntil undefined`.

- [ ] **Step 3: Write the migration**

`internal/store/migrations/000011_block_disabled_until.up.sql`:

```sql
-- A block can be dark until an instant without being disabled outright.
-- The two are independent: `enabled` is the indefinite switch an operator
-- must undo by hand, `disabled_until` is a timed one that expires on its
-- own. NULL means no dark window; a value in the past is stale rather than
-- wrong -- nothing sweeps it, and the planning gate simply stops
-- suppressing once the instant passes.
--
-- A column rather than a key inside spec_json on purpose: `enabled` is
-- already a column, and splitting the two dark-switches across a column
-- and a JSON blob makes them impossible to write in one statement.
ALTER TABLE blocks ADD COLUMN disabled_until DATETIME;
```

`internal/store/migrations/000011_block_disabled_until.down.sql`:

```sql
ALTER TABLE blocks DROP COLUMN disabled_until;
```

- [ ] **Step 4: Thread it through the store**

In `internal/store/blocks.go`, add the field to `BlockRecord`:

```go
	// DisabledUntil suppresses this block from schedule generation until
	// the instant it names, after which the block returns on its own.
	// Independent of Enabled: a block is planned only when Enabled is true
	// AND it is not currently dark. Nil means no dark window; a value in
	// the past is stale data that no longer suppresses anything, so a
	// caller rendering a "dark" affordance must compare against now rather
	// than test for non-nil.
	DisabledUntil *time.Time
```

Every `SELECT` in this file gains `disabled_until`, every scan gains a `*time.Time` target, and `CreateBlock`/`UpdateBlock` gain the column and its placeholder. Match the file's existing scan style exactly.

- [ ] **Step 5: Run the tests**

Run: `go test -race ./internal/store/...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/store/
git commit -m "feat(store): add the blocks.disabled_until column"
```

---

## Task 2: The planning gate

**Files:**

- Modify: `internal/service/schedule.go` (`ActiveBlocks`, and its call at line ~394), `cmd/generate.go` (the call at line ~253)
- Test: `internal/service/schedule_test.go`

**Interfaces:**

- Consumes: `store.BlockRecord.DisabledUntil` from Task 1.
- Produces: `service.ActiveBlocks(ctx context.Context, s *store.Store, now time.Time) ([]scheduler.Block, error)` — signature change, one new parameter.

**Why this is one function and not an audit.** `ActiveBlocks` is the only place in the codebase that decides whether a block is planned, and it has exactly two callers: `Runner.run` (which serves the API, the cron loop, and `generate --apply` alike) and the CLI's preflight. Adding the gate here covers every path by construction. Verify that claim before you change anything — `grep -rn 'ActiveBlocks' --include='*.go' .` — because if a third caller has appeared since this plan was written, it needs the same clock.

- [ ] **Step 1: Write the failing test**

```go
func TestActiveBlocksSkipsCurrentlyDarkBlocks(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)

	future := now.Add(48 * time.Hour)
	past := now.Add(-48 * time.Hour)

	mustCreate(t, st, "dark", "Dark", true, &future)
	mustCreate(t, st, "expired", "Expired", true, &past)
	mustCreate(t, st, "plain", "Plain", true, nil)
	mustCreate(t, st, "off", "Off", false, nil)
	// Both switches at once: still off, because the axes are independent.
	mustCreate(t, st, "offdark", "OffAndDark", false, &future)

	blocks, err := service.ActiveBlocks(ctx, st, now)
	if err != nil {
		t.Fatalf("ActiveBlocks: %v", err)
	}

	got := make([]string, 0, len(blocks))
	for _, b := range blocks {
		got = append(got, b.Name)
	}
	sort.Strings(got)
	want := []string{"Expired", "Plain"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("active = %v, want %v", got, want)
	}
}

func TestActiveBlocksWakesExactlyAtTheInstant(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	wake := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	mustCreate(t, st, "b1", "Waking", true, &wake)

	// One second before: still dark.
	before, err := service.ActiveBlocks(ctx, st, wake.Add(-time.Second))
	if err != nil {
		t.Fatalf("ActiveBlocks: %v", err)
	}
	if len(before) != 0 {
		t.Fatalf("block active %v before its wake instant", len(before))
	}

	// Exactly at the instant: awake. "Until" names the moment it returns,
	// not the last moment it is dark.
	at, err := service.ActiveBlocks(ctx, st, wake)
	if err != nil {
		t.Fatalf("ActiveBlocks: %v", err)
	}
	if len(at) != 1 {
		t.Fatalf("block not active at its wake instant (got %d)", len(at))
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test -race ./internal/service/ -run TestActiveBlocks`
Expected: FAIL — `not enough arguments in call to service.ActiveBlocks`.

- [ ] **Step 3: Implement the gate**

```go
// ActiveBlocks returns the Spec of every block that should be planned as of
// now: enabled, and not currently inside a dark window.
//
// The two switches are independent. Enabled is the indefinite one, undone
// only by an operator. DisabledUntil is the timed one, undone by the clock.
// A block needs both to be clear, so setting either suppresses it.
//
// `now` is a parameter rather than a time.Now() call so a run plans against
// the same instant it generates for, and so this is testable without a
// clock stub. DisabledUntil comparison is instant-vs-instant, so the
// location `now` carries does not affect the result.
//
// This is the ONLY place that decides whether a block is planned -- the
// cron loop, the API, and the CLI all reach the engine through here. A new
// suppression rule belongs in this function and nowhere else.
func ActiveBlocks(ctx context.Context, s *store.Store, now time.Time) ([]scheduler.Block, error) {
	records, err := s.ListBlocks(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list blocks from store: %w", err)
	}

	blocks := make([]scheduler.Block, 0, len(records))
	for _, rec := range records {
		if !rec.Enabled {
			continue
		}
		// "Until" names the instant the block returns, so a wake time equal
		// to now is already awake.
		if rec.DisabledUntil != nil && now.Before(*rec.DisabledUntil) {
			continue
		}
		spec := rec.Spec
		spec.ID = rec.ID
		blocks = append(blocks, spec)
	}
	return blocks, nil
}
```

- [ ] **Step 4: Update both callers**

`internal/service/schedule.go` (in `Runner.run`): pass the run's own clock, the same value the window is generated from — find the existing `start`/`r.now()` and use it, so a block cannot be dark for the gate and awake for the window.

`cmd/generate.go`: pass `time.Now()`. This is a preflight count for a CLI message, so a fresh read is correct.

- [ ] **Step 5: Run the tests**

Run: `go test -race ./internal/service/... ./cmd/...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/service/ cmd/generate.go
git commit -m "feat(service): skip currently-dark blocks at the single planning gate"
```

---

## Task 3: The occurrence generator

**Files:**

- Create: `internal/scheduler/occurrence.go`, `internal/scheduler/occurrence_test.go`
- Modify: `internal/scheduler/engine.go` (extract the parser construction)

**Interfaces:**

- Produces:
  - `scheduler.NewCronParser() cron.Parser` — the one parser configuration, shared by the engine and by `NextOccurrences`, so the two can never disagree about what a valid expression is.
  - `scheduler.NextOccurrences(expr string, from time.Time, count int) ([]time.Time, error)` — the next `count` start instants strictly after `from`, in `from`'s location.

**The two library edge cases this must respect.** `robfig`'s `SpecSchedule.Next` returns the **zero** `time.Time` when no match exists within five years — a `0 0 30 2 *` (February 30th) never fires. That must terminate the loop and shorten the slice, never serialize as `0001-01-01T00:00:00Z`. And `@every 1h30m` yields a `ConstantDelaySchedule` that is relative to its argument rather than to a calendar, which is correct but worth a comment so nobody "fixes" it later.

- [ ] **Step 1: Write the failing test**

```go
func TestNextOccurrencesReturnsCountStartsAfterFrom(t *testing.T) {
	t.Parallel()
	from := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	got, err := scheduler.NextOccurrences("0 6 * * *", from, 3)
	if err != nil {
		t.Fatalf("NextOccurrences: %v", err)
	}
	want := []time.Time{
		time.Date(2026, 9, 11, 6, 0, 0, 0, time.UTC),
		time.Date(2026, 9, 12, 6, 0, 0, 0, time.UTC),
		time.Date(2026, 9, 13, 6, 0, 0, 0, time.UTC),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestNextOccurrencesIsStrictlyAfterFrom(t *testing.T) {
	t.Parallel()
	// from sits exactly on an occurrence: it must not be returned again.
	from := time.Date(2026, 9, 10, 6, 0, 0, 0, time.UTC)
	got, err := scheduler.NextOccurrences("0 6 * * *", from, 1)
	if err != nil {
		t.Fatalf("NextOccurrences: %v", err)
	}
	want := time.Date(2026, 9, 11, 6, 0, 0, 0, time.UTC)
	if len(got) != 1 || !got[0].Equal(want) {
		t.Fatalf("got %v, want [%v]", got, want)
	}
}

func TestNextOccurrencesEvaluatesInFromsLocation(t *testing.T) {
	t.Parallel()
	zurich, err := time.LoadLocation("Europe/Zurich")
	if err != nil {
		t.Skipf("tzdata unavailable: %v", err)
	}
	from := time.Date(2026, 9, 10, 12, 0, 0, 0, zurich)
	got, err := scheduler.NextOccurrences("0 6 * * *", from, 1)
	if err != nil {
		t.Fatalf("NextOccurrences: %v", err)
	}
	if got[0].Location() != zurich {
		t.Fatalf("location = %v, want %v", got[0].Location(), zurich)
	}
	if h := got[0].Hour(); h != 6 {
		t.Fatalf("hour = %d, want 6 local", h)
	}
}

func TestNextOccurrencesShortensWhenTheScheduleRunsOut(t *testing.T) {
	t.Parallel()
	// February 30th never happens. robfig returns the zero time rather
	// than an error, which must end the walk and never reach a caller.
	from := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	got, err := scheduler.NextOccurrences("0 0 30 2 *", from, 3)
	if err != nil {
		t.Fatalf("NextOccurrences: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %v, want no occurrences", got)
	}
	for _, o := range got {
		if o.IsZero() {
			t.Fatal("a zero time reached the caller")
		}
	}
}

func TestNextOccurrencesRejectsAnUnparseableExpression(t *testing.T) {
	t.Parallel()
	_, err := scheduler.NextOccurrences("not a cron", time.Now(), 1)
	if err == nil {
		t.Fatal("expected an error for an unparseable expression")
	}
}

func TestNextOccurrencesRejectsANonPositiveCount(t *testing.T) {
	t.Parallel()
	if _, err := scheduler.NextOccurrences("0 6 * * *", time.Now(), 0); err == nil {
		t.Fatal("expected an error for count 0")
	}
}

func TestNextOccurrencesAcceptsDescriptors(t *testing.T) {
	t.Parallel()
	from := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	got, err := scheduler.NextOccurrences("@daily", from, 2)
	if err != nil {
		t.Fatalf("NextOccurrences: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d occurrences, want 2", len(got))
	}
	if got[0].Hour() != 0 {
		t.Fatalf("@daily fired at %v, want midnight", got[0])
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test -race ./internal/scheduler/ -run TestNextOccurrences`
Expected: FAIL — undefined `scheduler.NextOccurrences`.

- [ ] **Step 3: Implement**

`internal/scheduler/occurrence.go`:

```go
package scheduler

import (
	"fmt"
	"time"

	"github.com/robfig/cron/v3"
)

// maxOccurrences bounds one NextOccurrences call. The contract caps count
// at 10; this is the belt to that braces, so a future caller cannot walk a
// five-year search 10,000 times by passing a large number.
const maxOccurrences = 100

// NewCronParser builds the project's ONE cron parser configuration:
// standard 5-field expressions plus descriptors (@daily, @every 1h30m).
//
// It exists so the engine and NextOccurrences cannot drift apart about
// what a valid expression is. A block whose cron the planner accepts must
// be one the UI can show occurrences for, and the reverse -- two parsers
// with different option sets would produce exactly that lie.
func NewCronParser() cron.Parser {
	return cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
}

// NextOccurrences returns the next `count` start instants strictly after
// `from`, in from's own location.
//
// Strictly after, so a `from` that sits exactly on an occurrence does not
// return that occurrence again -- the caller is asking what happens NEXT.
// This is deliberately a different convention from the engine's window
// walk, which seeds at from-1s precisely so an occurrence exactly at the
// window start IS included; a window is inclusive of its edge, "next" is
// not.
//
// The result may be SHORTER than count. robfig's SpecSchedule.Next returns
// the zero time when it finds no match within five years -- February 30th
// is a well-formed expression that never fires -- so the walk stops there
// rather than letting a zero instant reach a caller and serialize as
// 0001-01-01.
//
// A @every descriptor yields a ConstantDelaySchedule measured from the
// argument rather than from a calendar. That is the library's intent, not
// a bug to correct.
func NextOccurrences(expr string, from time.Time, count int) ([]time.Time, error) {
	if count <= 0 {
		return nil, fmt.Errorf("count must be positive, got %d", count)
	}
	if count > maxOccurrences {
		return nil, fmt.Errorf("count must be at most %d, got %d", maxOccurrences, count)
	}

	schedule, err := NewCronParser().Parse(expr)
	if err != nil {
		return nil, fmt.Errorf("invalid cron %q: %w", expr, err)
	}

	out := make([]time.Time, 0, count)
	next := from
	for range count {
		next = schedule.Next(next)
		if next.IsZero() {
			break
		}
		out = append(out, next)
	}
	return out, nil
}
```

In `internal/scheduler/engine.go`, replace the inline `cron.NewParser(...)` in `NewEngineWithOptions` with `NewCronParser()`. Leave a one-line comment at the call site saying the configuration is shared on purpose.

- [ ] **Step 4: Run the tests**

Run: `go test -race ./internal/scheduler/...`
Expected: PASS, including the pre-existing engine tests — the parser extraction must not change behaviour.

- [ ] **Step 5: Commit**

```bash
git add internal/scheduler/
git commit -m "feat(scheduler): add NextOccurrences over the engine's own cron parser"
```

---

## Task 4: `GET /api/v1/cron/next`

**Files:**

- Modify: `api/openapi.yaml`
- Create: `internal/api/cron.go`, `internal/api/cron_test.go`
- Modify: `internal/api/server.go` (`Deps.Location`), `cmd/serve.go` (wire it), `internal/api/router.go` if the generated registration needs it

**Interfaces:**

- Consumes: `scheduler.NextOccurrences` from Task 3.
- Produces: `GET /api/v1/cron/next?expr=&count=&from=` → `200 {"occurrences": ["<RFC3339>", ...]}`; `400` on an unparseable `expr`, a `count` outside 1–10, or an unparseable `from`.

- [ ] **Step 1: Document it in the contract**

In `api/openapi.yaml`, add under `paths:`:

```yaml
  /cron/next:
    get:
      operationId: nextCronOccurrences
      summary: Upcoming occurrences of a cron expression
      description: >-
        The next occurrences of `expr`, evaluated server-side in the
        configured `log.timezone`. It exists so the client never
        re-implements calendar semantics: DST transitions, month lengths,
        and descriptor handling all belong to one parser, the same one the
        scheduling engine uses.

        Occurrences are strictly AFTER `from`, so a `from` sitting exactly
        on an occurrence does not return it again. The array may be shorter
        than `count`: a well-formed expression that never fires (February
        30th) returns an empty array rather than an error, because the
        expression is valid and the answer is genuinely "never".

        Stateless -- it reads no blocks and touches no store.
      parameters:
        - name: expr
          in: query
          required: true
          schema:
            type: string
          description: A standard 5-field cron expression, or a descriptor such as `@daily`.
        - name: count
          in: query
          required: false
          schema:
            type: integer
            minimum: 1
            maximum: 10
            default: 3
        - name: from
          in: query
          required: false
          schema:
            type: string
            format: date-time
          description: RFC3339 instant to search after. Defaults to now.
      responses:
        "200":
          description: The upcoming occurrences, oldest first.
          content:
            application/json:
              schema:
                type: object
                required: [occurrences]
                properties:
                  occurrences:
                    type: array
                    items:
                      type: string
                      format: date-time
        "400":
          $ref: "#/components/responses/Problem"
```

Run `make generate` and commit the regenerated `internal/api/gen/server.gen.go`.

- [ ] **Step 2: Write the failing test**

```go
func TestNextCronOccurrencesReturnsThreeByDefault(t *testing.T) {
	t.Parallel()
	h := newTestHandlers(t)
	rr := doGet(t, h, "/api/v1/cron/next?expr=0+6+*+*+*&from=2026-09-10T12:00:00Z")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", rr.Code, rr.Body)
	}
	var body struct {
		Occurrences []string `json:"occurrences"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Occurrences) != 3 {
		t.Fatalf("got %d occurrences, want 3", len(body.Occurrences))
	}
}

func TestNextCronOccurrencesRejectsAnUnparseableExpression(t *testing.T) {
	t.Parallel()
	h := newTestHandlers(t)
	rr := doGet(t, h, "/api/v1/cron/next?expr=nonsense")
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
}

func TestNextCronOccurrencesRejectsCountOutOfRange(t *testing.T) {
	t.Parallel()
	h := newTestHandlers(t)
	for _, count := range []string{"0", "11", "-1"} {
		rr := doGet(t, h, "/api/v1/cron/next?expr=0+6+*+*+*&count="+count)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("count=%s: status = %d, want 400", count, rr.Code)
		}
	}
}

func TestNextCronOccurrencesReturnsEmptyForAScheduleThatNeverFires(t *testing.T) {
	t.Parallel()
	h := newTestHandlers(t)
	rr := doGet(t, h, "/api/v1/cron/next?expr=0+0+30+2+*")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 -- the expression is valid, it just never fires", rr.Code)
	}
	var body struct {
		Occurrences []string `json:"occurrences"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Occurrences) != 0 {
		t.Fatalf("got %v, want an empty array", body.Occurrences)
	}
	if strings.Contains(rr.Body.String(), "0001-01-01") {
		t.Fatal("a zero time serialized into the response")
	}
}

func TestNextCronOccurrencesEvaluatesInTheConfiguredLocation(t *testing.T) {
	t.Parallel()
	zurich, err := time.LoadLocation("Europe/Zurich")
	if err != nil {
		t.Skipf("tzdata unavailable: %v", err)
	}
	h := newTestHandlersWithLocation(t, zurich)
	rr := doGet(t, h, "/api/v1/cron/next?expr=0+6+*+*+*&from=2026-09-10T12:00:00Z&count=1")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	var body struct {
		Occurrences []string `json:"occurrences"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// 06:00 Zurich in September is 04:00Z.
	got, err := time.Parse(time.RFC3339, body.Occurrences[0])
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got.UTC().Hour() != 4 {
		t.Fatalf("occurrence %s is not 06:00 Zurich", body.Occurrences[0])
	}
}
```

- [ ] **Step 3: Run to verify they fail**

Run: `go test -race ./internal/api/ -run TestNextCronOccurrences`
Expected: FAIL — the handler does not exist.

- [ ] **Step 4: Implement**

Add `Location *time.Location` to `api.Deps` (documented as: nil falls back to `time.Local`, matching the engine's own nil-handling), and wire `cmd/serve.go`'s existing `loc` into it — the value is already resolved there for the Runner, so this is one field, not new config.

`internal/api/cron.go`:

```go
// NextCronOccurrences implements gen.ServerInterface.
//
// Stateless and store-free: the only endpoint in the contract that reads
// nothing. It exists so the client never evaluates cron itself -- the UI
// vendors cronstrue for PROSE, and asks here for instants.
func (h *Handlers) NextCronOccurrences(w http.ResponseWriter, r *http.Request, params gen.NextCronOccurrencesParams) {
	count := 3
	if params.Count != nil {
		count = *params.Count
	}
	// Range-checked here rather than trusting the contract: oapi-codegen
	// does not enforce the schema's own minimum/maximum, the same gap
	// GET /schedule's `days` works around.
	if count < 1 || count > 10 {
		WriteProblem(w, r, http.StatusBadRequest, "count out of range",
			"count must be between 1 and 10")
		return
	}

	loc := h.d.Location
	if loc == nil {
		loc = time.Local
	}

	from := time.Now().In(loc)
	if params.From != nil {
		from = params.From.In(loc)
	}

	occurrences, err := scheduler.NextOccurrences(params.Expr, from, count)
	if err != nil {
		// The only failure NextOccurrences can produce here is a parse
		// error -- count is already range-checked -- and that is the
		// operator's expression, so it is a 400 and the detail is safe to
		// echo.
		WriteProblem(w, r, http.StatusBadRequest, "invalid cron expression", err.Error())
		return
	}

	out := make([]string, 0, len(occurrences))
	for _, o := range occurrences {
		out = append(out, o.Format(time.RFC3339))
	}
	writeJSON(w, http.StatusOK, map[string]any{"occurrences": out})
}
```

- [ ] **Step 5: Run the tests**

Run: `go test -race ./internal/api/...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add api/openapi.yaml internal/api/ cmd/serve.go
git commit -m "feat(api): add GET /cron/next so the client never evaluates cron"
```

---

## Task 5: `next_occurrence` on the block record

**Files:**

- Modify: `api/openapi.yaml` (BlockRecord schema), `internal/api/blocks.go` (`toGen`)
- Test: `internal/api/blocks_test.go`

**Interfaces:**

- Consumes: `scheduler.NextOccurrences`, `store.BlockRecord.DisabledUntil`.
- Produces: `BlockRecord.next_occurrence` — an optional RFC3339 string. **Absent** when the block is disabled, when its cron never fires, or when the expression will not parse. Present otherwise, and it is the next instant the block will **actually air** — for a dark block, the first occurrence at or after it wakes.

**Cost note.** `toGen` runs per record on every `GET /blocks`, so this parses N cron expressions per list. At the scale this application runs (tens of blocks) that is nothing, and caching it would mean inventing an invalidation rule for a value that changes with the clock. If a list ever gets slow, the fix is a single batched computation, not a cache.

- [ ] **Step 1: Write the failing test**

```go
func TestBlockRecordCarriesItsNextOccurrence(t *testing.T) {
	t.Parallel()
	h := newTestHandlers(t)
	id := createBlock(t, h, blockWrite{Name: "Morning", Cron: "0 6 * * *", Duration: 60, ChannelID: "ch1"})

	rec := getBlock(t, h, id)
	if rec.NextOccurrence == nil {
		t.Fatal("next_occurrence absent for an enabled block")
	}
	got, err := time.Parse(time.RFC3339, *rec.NextOccurrence)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got.Before(time.Now()) {
		t.Fatalf("next_occurrence %v is in the past", got)
	}
}

func TestDisabledBlockReportsNoNextOccurrence(t *testing.T) {
	t.Parallel()
	h := newTestHandlers(t)
	id := createBlock(t, h, blockWrite{Name: "Off", Cron: "0 6 * * *", Duration: 60, ChannelID: "ch1"})
	patchBlock(t, h, id, `{"enabled":false}`)

	rec := getBlock(t, h, id)
	if rec.NextOccurrence != nil {
		t.Fatalf("next_occurrence = %v, want absent for a disabled block", *rec.NextOccurrence)
	}
}

func TestDarkBlockReportsTheFirstOccurrenceAfterItWakes(t *testing.T) {
	t.Parallel()
	h := newTestHandlers(t)
	id := createBlock(t, h, blockWrite{Name: "Dark", Cron: "0 6 * * *", Duration: 60, ChannelID: "ch1"})

	// Dark for the next ten days.
	wake := time.Now().Add(10 * 24 * time.Hour).UTC().Format(time.RFC3339)
	patchBlock(t, h, id, `{"disabled_until":"`+wake+`"}`)

	rec := getBlock(t, h, id)
	if rec.NextOccurrence == nil {
		t.Fatal("next_occurrence absent for a dark block -- it comes back, so it has one")
	}
	got, err := time.Parse(time.RFC3339, *rec.NextOccurrence)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	wakeAt, _ := time.Parse(time.RFC3339, wake)
	if got.Before(wakeAt) {
		t.Fatalf("next_occurrence %v is before the block wakes at %v", got, wakeAt)
	}
}

func TestBlockWithAnUnfireableCronReportsNoNextOccurrence(t *testing.T) {
	t.Parallel()
	h := newTestHandlers(t)
	id := createBlock(t, h, blockWrite{Name: "Never", Cron: "0 0 30 2 *", Duration: 60, ChannelID: "ch1"})

	rec := getBlock(t, h, id)
	if rec.NextOccurrence != nil {
		t.Fatalf("next_occurrence = %v, want absent -- February 30th never comes", *rec.NextOccurrence)
	}
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `go test -race ./internal/api/ -run 'NextOccurrence'`
Expected: FAIL — `rec.NextOccurrence undefined`.

- [ ] **Step 3: Extend the contract**

In `api/openapi.yaml`'s `BlockRecord` schema, add alongside the existing properties:

```yaml
        disabled_until:
          type: string
          format: date-time
          description: >-
            The instant this block returns to the schedule. Absent when
            there is no dark window. Independent of `enabled`: a block is
            planned only when it is enabled AND not currently dark. A value
            in the past is stale rather than meaningful -- it no longer
            suppresses anything.
        next_occurrence:
          type: string
          format: date-time
          description: >-
            The next instant this block will actually air, honouring both
            `enabled` and `disabled_until`. Absent when the block is
            disabled, when its cron expression never fires, or when the
            expression will not parse. For a dark block this is the first
            occurrence at or after it wakes, not the next raw cron tick --
            a NEXT reading beside a dark block would otherwise be a lie.
```

Run `make generate` and `make web-types`.

- [ ] **Step 4: Implement in `toGen`**

`toGen` currently takes only the record. Give it the clock and the location it needs, computing the search anchor before delegating:

```go
// nextOccurrenceFor computes what a block will actually do next, which is
// not the same question as what its cron says next.
//
// A disabled block has no answer: it is off until an operator says
// otherwise, and inventing an instant for it would put a NEXT reading
// beside a row the operator has deliberately switched off.
//
// A dark block DOES have one -- it comes back on its own -- so the search
// starts at its wake instant rather than now. Anything else would report
// a tick that the planning gate is going to skip.
//
// Returns nil for an unparseable or never-firing expression. Both are
// answers rather than errors: the list must render a block whose cron is
// wrong, so it can be the thing the operator goes and fixes.
func nextOccurrenceFor(rec store.BlockRecord, now time.Time, loc *time.Location) *string {
	if !rec.Enabled {
		return nil
	}
	from := now.In(loc)
	if rec.DisabledUntil != nil && from.Before(*rec.DisabledUntil) {
		// One second back, so an occurrence landing exactly on the wake
		// instant counts -- NextOccurrences is strictly-after.
		from = rec.DisabledUntil.In(loc).Add(-time.Second)
	}
	occurrences, err := scheduler.NextOccurrences(rec.Spec.Cron, from, 1)
	if err != nil || len(occurrences) == 0 {
		return nil
	}
	out := occurrences[0].Format(time.RFC3339)
	return &out
}
```

- [ ] **Step 5: Run the tests**

Run: `make test`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add api/openapi.yaml internal/api/ web/assets/ts/gen/
git commit -m "feat(api): report each block's next airing, honouring both dark switches"
```

---

## Task 6: `PATCH` gains `disabled_until`, and writes validate cron

**Files:**

- Modify: `api/openapi.yaml` (`BlockPatch`), `internal/api/blocks.go`
- Test: `internal/api/blocks_test.go`

**Interfaces:**

- Produces: `PATCH /blocks/{id}` accepts `{enabled?, disabled_until?}`. `POST`/`PUT /blocks` return `400` for an unparseable cron.

**Clearing the dark window.** `*time.Time` cannot distinguish an absent key from an explicit `null` under the current decoding style, and the operator needs both "leave it alone" and "clear it". Decode into `json.RawMessage` for this one field so absent, `null`, and an instant are three distinguishable cases. `null` clears.

- [ ] **Step 1: Write the failing tests**

```go
func TestPatchSetsAndClearsDisabledUntil(t *testing.T) {
	t.Parallel()
	h := newTestHandlers(t)
	id := createBlock(t, h, blockWrite{Name: "B", Cron: "0 6 * * *", Duration: 60, ChannelID: "ch1"})

	until := time.Now().Add(48 * time.Hour).UTC().Format(time.RFC3339)
	patchBlock(t, h, id, `{"disabled_until":"`+until+`"}`)
	if rec := getBlock(t, h, id); rec.DisabledUntil == nil {
		t.Fatal("disabled_until not set")
	}

	patchBlock(t, h, id, `{"disabled_until":null}`)
	if rec := getBlock(t, h, id); rec.DisabledUntil != nil {
		t.Fatalf("disabled_until = %v, want cleared by an explicit null", *rec.DisabledUntil)
	}
}

func TestPatchingOnlyEnabledLeavesTheDarkWindowAlone(t *testing.T) {
	t.Parallel()
	h := newTestHandlers(t)
	id := createBlock(t, h, blockWrite{Name: "B", Cron: "0 6 * * *", Duration: 60, ChannelID: "ch1"})

	until := time.Now().Add(48 * time.Hour).UTC().Format(time.RFC3339)
	patchBlock(t, h, id, `{"disabled_until":"`+until+`"}`)
	patchBlock(t, h, id, `{"enabled":false}`)

	rec := getBlock(t, h, id)
	if rec.DisabledUntil == nil {
		t.Fatal("an absent disabled_until key cleared the dark window -- the axes are independent")
	}
	if rec.Enabled {
		t.Fatal("enabled not applied")
	}
}

func TestPatchWithNoFieldsIsRejected(t *testing.T) {
	t.Parallel()
	h := newTestHandlers(t)
	id := createBlock(t, h, blockWrite{Name: "B", Cron: "0 6 * * *", Duration: 60, ChannelID: "ch1"})
	rr := doPatch(t, h, id, `{}`)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
}

func TestCreateRejectsAnUnparseableCron(t *testing.T) {
	t.Parallel()
	h := newTestHandlers(t)
	rr := doPost(t, h, `{"spec":{"type":"filter","name":"Bad","cron":"not a cron","duration":60,"channel_id":"ch1"}}`)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 -- a bad cron must be caught at write time, not at apply time", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "cron") {
		t.Fatalf("problem body does not name the field: %s", rr.Body)
	}
}

func TestUpdateRejectsAnUnparseableCron(t *testing.T) {
	t.Parallel()
	h := newTestHandlers(t)
	id := createBlock(t, h, blockWrite{Name: "B", Cron: "0 6 * * *", Duration: 60, ChannelID: "ch1"})
	rec := getBlock(t, h, id)
	rr := doPutWithIfMatch(t, h, id, rec.UpdatedAt,
		`{"spec":{"type":"filter","name":"B","cron":"75 99 * * *","duration":60,"channel_id":"ch1"}}`)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `go test -race ./internal/api/ -run 'DisabledUntil|UnparseableCron|NoFields'`
Expected: FAIL.

- [ ] **Step 3: Implement**

Extend `BlockPatch` in the contract with `disabled_until` (`type: string, format: date-time, nullable: true`), regenerate, and in `PatchBlock` decode the raw body so the three cases stay distinct. In `CreateBlock` and `UpdateBlock`, validate the cron with `scheduler.NewCronParser().Parse(spec.Cron)` alongside the existing CUE validation, returning `400` with a detail naming the expression. Add a comment recording that CUE types `cron` as a bare string and cannot express this, which is why the check is in Go — the same reasoning the empty-`show_title` check already uses.

- [ ] **Step 4: Run the tests**

Run: `make test`
Expected: PASS. Existing tests that create blocks with placeholder crons may now fail — fix the fixtures, do not weaken the check.

- [ ] **Step 5: Commit**

```bash
git add api/openapi.yaml internal/api/ web/assets/ts/gen/
git commit -m "feat(api): patch a block's dark window, and reject a bad cron at write time"
```

---

## Task 7: `POST /blocks/{id}/duplicate`

**Files:**

- Modify: `api/openapi.yaml`, `internal/api/blocks.go`
- Test: `internal/api/blocks_test.go`

**Interfaces:**

- Produces: `POST /blocks/{id}/duplicate` with body `{"name": "<string>"}` → `201 BlockRecord`; `404` when the source is gone, `400` for an empty name, `409` on a name collision.

**What is and is not copied.** The spec is copied whole, including series seeds and filter configuration — that is what makes it a duplicate. Nothing keyed by the source's block id comes along: series *cursors* and occurrence *snapshots* are records of what the source has already aired, and a copy has aired nothing. The new block gets a fresh UUID and fresh timestamps, and arrives **disabled** (decision 3).

- [ ] **Step 1: Write the failing tests**

```go
func TestDuplicateCopiesTheSpecAndArrivesDisabled(t *testing.T) {
	t.Parallel()
	h := newTestHandlers(t)
	id := createBlock(t, h, blockWrite{Name: "Morning", Cron: "0 6 * * *", Duration: 60, ChannelID: "ch1"})

	rr := doPost(t, h, "/api/v1/blocks/"+id+"/duplicate", `{"name":"Copy of Morning"}`)
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (body %s)", rr.Code, rr.Body)
	}
	var copied gen.BlockRecord
	if err := json.Unmarshal(rr.Body.Bytes(), &copied); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if copied.Id == id {
		t.Fatal("the copy reused the source's id")
	}
	if copied.Name != "Copy of Morning" {
		t.Fatalf("name = %q", copied.Name)
	}
	if copied.Enabled {
		t.Fatal("the copy arrived enabled -- it would contend with its source at the same cron on the same channel")
	}
	if copied.Spec.Cron != "0 6 * * *" || copied.Spec.ChannelId != "ch1" {
		t.Fatalf("spec not copied: %+v", copied.Spec)
	}
}

func TestDuplicateCarriesSeriesSeeds(t *testing.T) {
	t.Parallel()
	h := newTestHandlers(t)
	id := createSeriesBlock(t, h, "Anime Night", "ch1", []seriesSeed{
		{ShowTitle: "Bloody Mary", Season: 2, Episode: 5},
	})

	rr := doPost(t, h, "/api/v1/blocks/"+id+"/duplicate", `{"name":"Anime Night B"}`)
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rr.Code)
	}
	var copied gen.BlockRecord
	if err := json.Unmarshal(rr.Body.Bytes(), &copied); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if copied.Spec.Series == nil || len(*copied.Spec.Series) != 1 {
		t.Fatalf("series seeds not copied: %+v", copied.Spec.Series)
	}
	if (*copied.Spec.Series)[0].ShowTitle != "Bloody Mary" {
		t.Fatalf("seed not copied: %+v", (*copied.Spec.Series)[0])
	}
}

func TestDuplicateRejectsANameCollision(t *testing.T) {
	t.Parallel()
	h := newTestHandlers(t)
	id := createBlock(t, h, blockWrite{Name: "Morning", Cron: "0 6 * * *", Duration: 60, ChannelID: "ch1"})
	createBlock(t, h, blockWrite{Name: "Taken", Cron: "0 7 * * *", Duration: 60, ChannelID: "ch1"})

	rr := doPost(t, h, "/api/v1/blocks/"+id+"/duplicate", `{"name":"Taken"}`)
	if rr.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rr.Code)
	}
}

func TestDuplicateRejectsAnEmptyName(t *testing.T) {
	t.Parallel()
	h := newTestHandlers(t)
	id := createBlock(t, h, blockWrite{Name: "Morning", Cron: "0 6 * * *", Duration: 60, ChannelID: "ch1"})
	rr := doPost(t, h, "/api/v1/blocks/"+id+"/duplicate", `{"name":""}`)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
}

func TestDuplicateOfAMissingBlockIs404(t *testing.T) {
	t.Parallel()
	h := newTestHandlers(t)
	rr := doPost(t, h, "/api/v1/blocks/00000000-0000-0000-0000-000000000000/duplicate", `{"name":"X"}`)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rr.Code)
	}
}

func TestDuplicateAnnouncesOnTheLiveLink(t *testing.T) {
	t.Parallel()
	h, hub := newTestHandlersWithHub(t)
	id := createBlock(t, h, blockWrite{Name: "Morning", Cron: "0 6 * * *", Duration: 60, ChannelID: "ch1"})
	before := hub.LastID()

	rr := doPost(t, h, "/api/v1/blocks/"+id+"/duplicate", `{"name":"Copy"}`)
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d", rr.Code)
	}
	if hub.LastID() == before {
		t.Fatal("duplicate did not announce plan.invalidated -- other tabs would not see the new block")
	}
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `go test -race ./internal/api/ -run TestDuplicate`
Expected: FAIL — no such route.

- [ ] **Step 3: Extend the contract and implement**

Add the path to `api/openapi.yaml` with a `BlockDuplicate` request schema (`{name: string, minLength: 1}`), regenerate, then implement `DuplicateBlock`. It reads the source, copies `rec.Spec` by value, mints a fresh UUID and timestamps, sets `Enabled: false` and `DisabledUntil: nil`, runs the same name-collision check `CreateBlock` uses, and runs `checkSharedShowPolicies` — a duplicated series block competes for the same shows as its source, and arriving disabled does not exempt it (decision 7). Publish `plan.invalidated` on success, exactly as the other block writes do.

- [ ] **Step 4: Run the tests**

Run: `make test`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add api/openapi.yaml internal/api/ web/assets/ts/gen/
git commit -m "feat(api): duplicate a block, disabled, spec and seeds intact"
```

---

## Task 8: Phase A gate

- [ ] **Step 1: Lint and test**

Run: `make test && make lint && make validate`
Expected: golangci-lint 0 issues, all Go packages pass, config validates. `gosec` fails on this toolchain for reasons predating this work — confirm it fails identically on `main` before dismissing it.

- [ ] **Step 2: Migrate a real database**

Copy a pre-migration database aside, run the binary against it, and confirm `000011` applies cleanly and every existing block reads back with `disabled_until` nil.

- [ ] **Step 3: Exercise the endpoints by hand**

Start `schedularr serve` and verify, with `curl`:

- `GET /api/v1/cron/next?expr=0+6+*+*+*` returns three ascending instants in the configured zone.
- `GET /api/v1/cron/next?expr=0+0+30+2+*` returns `{"occurrences":[]}` with status 200, and the body contains no `0001-01-01`.
- `PATCH` a block's `disabled_until` to two days out, then `GET /blocks` and confirm `next_occurrence` is at or after that instant.
- `POST /blocks/{id}/duplicate` returns 201 with `enabled: false` and the source's spec.
- `POST /blocks` with `"cron": "nonsense"` returns 400.

- [ ] **Step 4: Prove the gate**

Set a block's `disabled_until` into the future, run `POST /generate`, and confirm the block contributes no slots. Set it into the past and confirm it does.

- [ ] **Step 5: Commit any fixes**

---

# Phase B — The blocks page

> Every task here invokes the `impeccable` skill first. Reuse the shared runtime and existing partials; do not re-implement them.

## Task 9: The list columns

**Files:**

- Create: `web/assets/ts/runtime/rank.ts`, `web/tests/rank.test.ts`
- Modify: `web/layouts/blocks/list.html`, `web/assets/ts/pages/blocks.ts`, `web/assets/ts/pages/guide.ts`, `web/assets/css/main.css`, `web/DESIGN.md`

**Interfaces:**

- Produces: `priorityRank(priority: number, peers: {priority: number; enabled: boolean}[]): {rank: number; of: number}` — the shared rank computation.

The guide's inspector already renders `50 · 2nd of 5`. Extract that computation into `rank.ts` and have both the inspector and the new column call it, so the two surfaces cannot disagree about what rank a block holds. Rank is among **enabled same-channel peers**, matching the inspector's existing rule; a disabled or dark block shows its priority without a rank, because it is not currently contending.

- [ ] **Step 1:** Failing tests for `priorityRank` — ties, a single peer, a block absent from its own peer list, and the disabled-peer exclusion. Then extract, verify the guide's inspector still renders identically, and commit.
- [ ] **Step 2:** Add the NEXT, Duration, PRI-rank, Channel and DARK UNTIL columns. NEXT renders `next_occurrence` through the existing `relativeTime`/`formatLocal` helpers reading `serverNow()`, never `Date.now()`. DARK UNTIL renders only while `disabled_until` is in the future — a past instant is stale and must not paint a chip.
- [ ] **Step 3:** Record the new column rhythm and any new chip in `web/DESIGN.md`, with contrast evidence for every new pairing in both palettes. Add `/kit/` fixtures for a dark row and a rankless row.

---

## Task 10: Duplicate and Disable-until actions

**Files:** `web/layouts/blocks/list.html`, `web/assets/ts/pages/blocks.ts`, `web/assets/css/main.css`, `web/tests/blocks-power.test.ts`

**Three constraints the recon surfaced, all of which will cause silent bugs if missed:**

1. **`pendingId` is a single global row-action slot.** Both new actions must set and clear it, and must refuse to arm while another action is in flight — exactly as `requestDelete` does. Otherwise a disable-until popover can be opened over an in-flight toggle and its write silently dropped.
2. **The open-editor freeze is an `If-Match` safety mechanism, not a nicety.** Duplicate opens the editor on the copy, which freezes the list. Any new action that mutates `this.blocks` while a panel is open must route through `planInvalidatedReaction`'s reasoning, or `pages-live.test.ts` will not catch the regression.
3. **There is no popover component.** `--z-popover` exists as a token used once, for the guide's mobile bottom sheet. Disable-until needs a disclosure; build it from the existing dialog vocabulary rather than inventing a third pattern, and record the choice in `DESIGN.md`.

- [ ] Failing tests for the pure predicates — which actions may arm given a pending action and an open editor, and the preset-to-instant mapping for the disable-until choices. Implement, verify, commit.
- [ ] Duplicate POSTs, then opens the editor on the returned record in **edit** mode, so `If-Match` is armed against the copy the server actually created rather than against a client-side guess.

---

## Task 11: The editor consequence rail

**Files:** `web/layouts/blocks/list.html`, `web/assets/ts/pages/blocks.ts`, `web/assets/css/main.css`

The rail answers "what will this do" before the operator commits: the next three occurrences with computed end times (from `GET /cron/next` plus the block's own duration — the endpoint returns starts only), the priority siblings on that channel, and for a series block the projected lineup when rows are reordered.

- [ ] Debounce the `/cron/next` call on cron-field edits; an invalid expression renders the 400's detail inline rather than an empty rail. Failing tests for the debounce and the end-time computation, implement, verify, commit.

---

## Task 12: Collapsed series rows

**Files:** `web/layouts/blocks/list.html`, `web/assets/ts/pages/blocks.ts`, `web/assets/css/main.css`

A series block with a dozen shows makes the editor unreadable. Rows collapse to a summary line and expand on demand. Decide and document what an **incomplete** row summarises to — the spec's example assumes a filled row, and a blank `show_title` must not render an empty line the operator cannot find again.

- [ ] Failing test for the summary-line function (including the incomplete case), implement, verify, commit.

---

## Task 13: The cron-footgun confirm

**Files:** `web/layouts/blocks/list.html`, `web/assets/ts/pages/blocks.ts`, `web/layouts/partials/ui/confirm.html`

Editing the cron of a series block that has already aired changes which episode lands when. The confirm names the consequence and offers to rewind the cursor.

**The shared-partial collision.** `ui/confirm.html` hard-codes `x-ref="confirmDialog"` and blocks already instantiates it for Delete. A second instance on the same page collides on that ref and the last one silently wins — the delete dialog would stop working. Either multiplex the one dialog behind a `confirmKind` discriminator, or parameterise the partial's ref name. Both are contract changes to a partial the guide also uses, so whichever is chosen must be verified on the guide too.

**The rewind writes through `PATCH /state/series/{show_title}`**, which already invalidates snapshots. It is per-show and the confirm is per-block, so a block seeding three shows means three writes; report per-title outcomes rather than claiming a single success.

- [ ] Failing tests for the "should this confirm fire" predicate — a series block with a committed cursor versus one that has never aired, and a filter block, which never fires it. Implement, verify, commit.

---

## Task 14: Phase B gate

- [ ] `make web` clean; `npm test --prefix web` green; real `tsc` exit 0 via `./node_modules/.bin/tsc --noEmit` from `web/` — `npm run check --prefix web` is filtered in this environment and its exit status is not reliable.
- [ ] Run the real binary and verify by hand: duplicate a series block and confirm the copy is dark with its seeds intact; set a dark window and watch the row's DARK UNTIL chip and NEXT column agree; edit a cron and watch the rail update; confirm the delete dialog still works after the cron-footgun confirm was added.
- [ ] Batched screenshot round, desktop and mobile, light and dark.
- [ ] Run the mechanical detector once over the changed UI files.

---

# Phase C — Documentation and release

## Task 15: Docs, changelog, roadmap

- [ ] `docs/api-reference.md`: `GET /cron/next`; `POST /blocks/{id}/duplicate`; `disabled_until` on `PATCH`, including how `null` clears it; `next_occurrence` and what its absence means; the new `400` on an unparseable cron at write time.
- [ ] `docs/web-ui-guide.md`: the new columns, both row actions, the consequence rail, collapsed series rows, and the cron-footgun confirm.
- [ ] `docs/scheduling-concepts.md`: `disabled_until` as the timed dark switch, its independence from `enabled`, and the fact that it does not round-trip through `scheduler.yaml`.
- [ ] `docs/architecture.md`: `internal/scheduler/occurrence.go` as the one occurrence generator, and why the client never evaluates cron.
- [ ] `web/DESIGN.md`: the new columns, the DARK UNTIL chip, the disclosure pattern for collapsed rows, and WCAG evidence for every new pairing.
- [ ] `docs/roadmap.md`: convert the pending block-power-tools entry into a shipped one, naming what was deliberately left out (recurring dark windows, bulk block operations, case-insensitive names).
- [ ] `TODO.md`: a "Deferred (block power tools)" section.
- [ ] `CHANGELOG.md`: a new version heading with Added / Changed / Fixed, plus the compare link at the foot.
- [ ] Run `stop-slop` over every paragraph written here, and `markdownlint-cli2` to confirm no new findings against the baseline.
- [ ] Hand the operator the paste text for the cluster wiki page and the Obsidian note. **`disabled_until` is an operational change worth naming there:** a block can now be dark without being disabled, and an operator reading only the `enabled` column will not see why a block is not airing.

---

## Self-Review

**Spec coverage.** Section 9 item 4 lists four contract items and six UI items. Contract: `POST /blocks/{id}/duplicate` → Task 7; `BlockSpec.disabled_until` + PATCH extension → Tasks 1, 2, 6 (relocated to a column, with the reasoning recorded in decision 1); `BlockRecord.next_occurrence(s)` → Task 5 (singular chosen; the rail reads `/cron/next` for its three, so a plural field would be a second way to ask the same question); `GET /cron/next` → Task 4. UI: NEXT/Duration/PRI-rank/Channel/DARK UNTIL columns → Task 9; Duplicate + Disable-until actions → Task 10; consequence rail → Task 11; collapsed series rows → Task 12; cron-footgun confirm with rewind → Task 13. The create→apply bridge tape line with the real `next_cron_tick` is **not** covered by a task — `next_cron_tick` already ships on `GET /status`, so it is one tape line in Task 10's editor-save path; it is called out here so it is not lost.

**Placeholder scan.** No TBDs. Phase A tasks carry complete test bodies and implementations. Phase B tasks 9–13 carry constraints, decisions, and test subjects rather than full code — the UI shape depends on the `impeccable` pass those tasks require, and pre-writing markup that skill will redo would be waste. Each still names its files, its predicates, and its failure modes.

**Type consistency.** `store.BlockRecord.DisabledUntil *time.Time` (Task 1) is read by `service.ActiveBlocks` (Task 2) and `nextOccurrenceFor` (Task 5). `scheduler.NextOccurrences(expr, from, count)` (Task 3) is called by the `/cron/next` handler (Task 4) and `nextOccurrenceFor` (Task 5) with the same signature. `scheduler.NewCronParser()` (Task 3) is used by the engine and by Task 6's write validation. `priorityRank` (Task 9) is called by both the blocks list and the guide inspector.

**One risk this plan does not eliminate.** Task 6 changes `POST`/`PUT /blocks` to reject crons that were previously accepted. Any existing block in a live database with an unparseable cron becomes uneditable until its cron is fixed — the write path will refuse it. That is the correct behaviour and it is also a migration hazard worth checking in the Phase A gate: list every stored block's cron through the parser before shipping, and if any fails, the operator needs to know before they discover it mid-edit.
