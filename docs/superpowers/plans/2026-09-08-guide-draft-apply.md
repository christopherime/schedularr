# Guide Draft & Apply Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move preview/apply into the Guide as a draft mode — a 7-day `POST /generate` diff overlay on the week grid with a sticky draft bar (APPLY / DISCARD), armed-signature apply, the applied tape line, a `PREVIEW ON GUIDE` bridge from a block save — make filter-block lineups deterministic so the previewed plan is the applied plan, and delete the Schedule page; nav becomes `GUIDE · BLOCKS · SERIES`. Ships as v0.5.6.

**Architecture:** The guide keeps one **reading** (the committed-mode plan: `GET /schedule?days=28`, always ALL channels, stamped with its request time, mirrored trimmed to `sessionStorage`) and, in draft mode, one **draft** (`POST /generate {days: 7, channel_id?}` for the SCOPE) rendered on the same grid with per-slot verdicts computed client-side against the reading (`new` / `changed` / `same` / `removed`; reading slots past the draft horizon are `beyond`). A new pure runtime module `web/assets/ts/runtime/draft.ts` owns the diff, the storage codec, and every line of draft copy; `guide.ts` wires state; `grid.ts` renders the verdict as `data-draft` + a text chip. One small engine change makes filter-block lineups a pure function of (block, occurrence) so a dry run and the apply that follows plan the same content, and the Runner serializes applies. No contract change.

**Tech Stack:** Go 1.27 (engine + service), Hugo + Alpine.js (vendored), TypeScript (strict, `tsc --noEmit`), Node built-in test runner (`node --test`, native type stripping), hand-written CSS with custom properties.

**Spec:** `docs/superpowers/specs/2026-08-30-v0.5-web-overhaul-design.md` — §1 non-negotiables, §2 (IA + first-run acceptance flow), §3.1 (toolbar/pager amendments), §3.3 (draft & apply mode), §7 (mobile parity), §9 slice item "Draft & apply on the Guide", §11 (Q1, Q3, Q6 answered). The controller's design note with every ruling behind this plan: `.superpowers/sdd/2026-09-08-guide-draft-apply/design-note.md` (read its "Rulings after the three-lens critique" section — those rulings are binding where the spec is silent).

## Global Constraints

- **No contract change in this slice** — `api/openapi.yaml`, `internal/api/gen`, `web/assets/ts/gen/types.d.ts` stay untouched; the drift job must stay green.
- **No-legacy-code policy:** superseded pages, bundles, styles, helpers, and doc sentences are deleted outright in the same change — no aliases, no redirect stubs (the 404 page carries the nav), no dual paths.
- **CSP `style-src 'self'`:** zero inline `style` attributes; all dynamic geometry via CSSOM (`el.style.setProperty`) or classes/`data-*` attributes.
- **Feedback idiom:** no toasts, no spinners. Errors are inline `.problem` blocks through `ui/problem.html` (title + detail + muted `REF <request_id>`); successes are tape lines through `printTape`; busy states are `[aria-busy]`.
- **SC 1.4.1:** color never carries meaning alone — every verdict that changes something has a text chip (`NEW` / `CHANGED` / `REMOVED`) and an aria-label prefix.
- **Motion:** 150–250ms, state-change only, never on initial load; `prefers-reduced-motion` parity (data still lands, transitions suppressed); `forced-colors` fallbacks for every box-shadow/hatch-borne state.
- **Stack rules:** Hugo + Alpine; the grid DOM is built in TS (`runtime/grid.ts`); Alpine drives only toolbar, draft bar, and inspector. The client never parses cron. Every page bundle is one esbuild bundle through `ui/page-js.html`.
- **Armed-signature discipline (spec §3.3):** APPLY sends exactly `draftRequestBody(signature)` of the draft on the glass, never a body rebuilt from live controls; a SCOPE change replaces the draft (re-preview).
- **Honesty boundary:** nothing on the client knows Tunarr's current lineup. Every verdict and every count is "vs the reading taken HH:MM" and the copy says so; copy never claims a slot is "in Tunarr" or "removed from Tunarr". An empty draft is not "nothing": applying pushes an empty lineup to any channel Schedularr last applied in scope, and the confirm says so.
- **The committed reading is always ALL channels and 28 days;** SCOPE is a draft control only; the draft/apply window is 7 days (`DRAFT_DAYS = 7`). After DISCARD or a successful apply the SCOPE control snaps back to All channels.
- **Exact copy:** draft bar `7-day draft — 14 slots across 3 channels · 2 new · 1 changed · 1 removed · 1 dropped · vs reading 21:02` (the CSS uppercases; zero verdict counts omitted; `no changes` when all three are zero; `dropped` only when > 0); confirm title `Apply ALL channels` / `Apply <plate text>`; confirm body `This applies 14 slots across 3 channels to Tunarr — 2 new, 1 changed, 1 removed vs the 21:02 reading.` and for an empty draft `This draft schedules nothing for CH 04 · HORROR. Applying pushes an empty lineup to any channel Schedularr last applied in this scope — 3 slots removed vs the 21:02 reading.`; tape `Applied — 14 slots / 3 channels`; block save tape `Block saved — <name>` with the single action `Preview on guide`.
- **Nav becomes `GUIDE · BLOCKS · SERIES`.** The first-run acceptance flow in spec §2 (token → guide empty state → blocks → save → `PREVIEW ON GUIDE` → draft → APPLY → tape) must be walkable with zero dead ends, including on mobile (SCOPE is hidden under 640px; the `Arm draft` button is the on-page entry there).
- **Docs in the same change:** `web/DESIGN.md`, `docs/web-ui-guide.md`, `docs/api-reference.md`, `docs/scheduling-concepts.md`, `docs/roadmap.md`, `TODO.md`, `CHANGELOG.md`, `CLAUDE.md`, `AGENTS.md`, `.ai/AGENTS.md`, `docs/index.md`, `README.md` alt text, the spec's §3.1 amendment note. Generated prose is cleaned with the `stop-slop` skill.
- **Gates before every commit:** `make web-check` (tsc) and `make web-test` (node) for web tasks; `make test` (Go, `-race`) for the engine task and the final task; `make lint` in the final task. `make web` (real Hugo production build) must succeed at the end of every task that touches `web/`.
- **Commits:** Conventional Commits, single-purpose, ending with the attribution trailer the session provides.
- **UI edits go through the `impeccable` skill's craft floor:** before editing any file under `web/`, read `/Users/christophe/.claude/skills/impeccable/reference/craft-floor.md` and honor `web/DESIGN.md` (the committed visual world: CRT signal bench, coded legends, monospace, graticule).
- **Go edits follow CLAUDE.md:** wrap errors with context, `slog` snake_case keys, linter limits (cyclomatic 15, cognitive 20, nesting 5, results 3, args 5), doc comments whose first word is the symbol.

---

## File map

| File | Responsibility in this slice |
| --- | --- |
| `internal/scheduler/engine.go` | `planFilterBlock` shuffles with the occurrence-seeded rng over ID-sorted candidates; filler uses the same rng. |
| `internal/scheduler/engine_test.go` | Determinism test. |
| `internal/service/schedule.go`, `schedule_test.go` | `Runner` serializes applies (`applyMu`); test. |
| `web/assets/ts/runtime/draft.ts` (new) | Pure draft model: diff, verdict copy, storage codec, request body, URL param, draft href. No DOM, no fetch. |
| `web/tests/draft.test.ts` (new) | Unit tests for every export of `draft.ts`. |
| `web/assets/ts/runtime/grid.ts` | `GuideSlot.draft` verdict; renderer emits `data-draft`, the verdict chip, aria prefix, draw-in class, lane-2 placement for removed; rundown parity. |
| `web/assets/ts/runtime/api.ts` | `apiSend` gains an optional timeout; `LONG_SEND_TIMEOUT_MS`. |
| `web/assets/ts/runtime/shell.ts` | `--bezel-h` on the root via `ResizeObserver`. |
| `web/assets/ts/pages/guide.ts` | Draft state machine: preview, arm, discard, apply, storage, `?draft` arrival with the `/status` staleness guard, inspector verdict, announcements, focus. |
| `web/layouts/index.html` | Toolbar `Arm draft` button, draft bar, preview/apply problems, draft empty state, confirm dialog, inspector verdict line. |
| `web/assets/css/main.css` | Draft bar, verdict treatments, lane-2 removed, dimmed `same`, draw-in/settle, reduced-motion/forced-colors; delete the Schedule page's rules. |
| `web/assets/ts/pages/blocks.ts` | `Preview on guide` tape action after a save. |
| `web/layouts/partials/nav.html` | Three items; comments name themes, not numbers. |
| `web/layouts/schedule/list.html`, `web/assets/ts/pages/schedule.ts`, `web/content/schedule/_index.md` | Deleted. |
| `web/assets/ts/runtime/format.ts`, `web/tests/runtime.test.ts` | `clampDays` deleted (last caller dies). |
| `web/layouts/kit/list.html`, `web/assets/ts/pages/kit.ts` | Draft bar states + verdict slots in the gallery; the Schedule warnings fixture deleted. |
| Docs listed under Global Constraints | Updated; Schedule page prose deleted; v0.5.6 CHANGELOG. |

---

### Task 1: Deterministic filter-block lineups and serialized applies (Go)

**Files:**
- Modify: `internal/scheduler/engine.go` (`planFilterBlock`, ~lines 1199–1290)
- Modify: `internal/scheduler/engine_test.go` (append one test)
- Modify: `internal/service/schedule.go` (`Runner` struct ~line 80, `Run` ~line 191)
- Modify: `internal/service/schedule_test.go` (append one test)

**Interfaces:**
- Consumes: `occurrenceRand(blockID string, occurrenceStart time.Time) *rand.Rand` and `shuffleWith(rng, n, swap)` (engine.go ~lines 142, 159); `getFiller(block, remainingDuration, rng)` (~line 1828); `tunarr.Program.GetID()`.
- Produces: no API change. Behavior: two dry runs (or a dry run and an apply) over the same store state, library, and window produce identical filter-block lineups; `Runner.Run` with `Apply: true` never overlaps another apply on the same Runner.

- [ ] **Step 1: Write the failing determinism test**

Append to `internal/scheduler/engine_test.go` (use the package's existing helpers for a filter `Block` and a program list — look at `TestFilterByHistory` around line 2427 for the fixture style; the engine is constructed with `NewEngineWithOptions` against the in-memory/fake store the neighbors use):

```go
// TestPlanFilterBlock_DeterministicPerOccurrence pins the draft-mode
// contract: a filter occurrence's lineup is a pure function of (block,
// occurrence start, candidates) -- a dry run and the apply that follows
// it pick the same programs, whatever order the library arrived in.
func TestPlanFilterBlock_DeterministicPerOccurrence(t *testing.T) {
	programs := make([]tunarr.Program, 0, 40)
	for i := 0; i < 40; i++ {
		programs = append(programs, tunarr.Program{
			ID:       fmt.Sprintf("prog-%02d", i),
			Title:    fmt.Sprintf("Program %02d", i),
			Type:     "movie",
			Duration: 30 * 60 * 1000,
			Genres:   []string{"Comedy"},
		})
	}
	block := Block{ID: "blk-1", Name: "Comedy Hour", ChannelID: "ch-1", Duration: 120, Type: "filter", Filter: Filter{Genres: []string{"Comedy"}}}
	occ := time.Date(2026, 9, 10, 21, 0, 0, 0, time.UTC)

	newEngine := func() *Engine {
		return NewEngineWithOptions(context.Background(), nil, []Block{block}, newFakeStore(), EngineOptions{})
	}
	first, err := newEngine().planFilterBlock(block, programs, occ)
	if err != nil {
		t.Fatalf("first plan: %v", err)
	}
	second, err := newEngine().planFilterBlock(block, programs, occ)
	if err != nil {
		t.Fatalf("second plan: %v", err)
	}
	if !equalIDs(first, second) {
		t.Fatalf("same inputs planned different lineups:\n%v\n%v", ids(first), ids(second))
	}

	// Library order must not matter: the same catalog reversed plans the
	// same lineup.
	reversed := make([]tunarr.Program, len(programs))
	for i, p := range programs {
		reversed[len(programs)-1-i] = p
	}
	third, err := newEngine().planFilterBlock(block, reversed, occ)
	if err != nil {
		t.Fatalf("reversed plan: %v", err)
	}
	if !equalIDs(first, third) {
		t.Fatalf("library order changed the lineup:\n%v\n%v", ids(first), ids(third))
	}

	// A different occurrence of the same block still varies.
	other, err := newEngine().planFilterBlock(block, programs, occ.Add(24*time.Hour))
	if err != nil {
		t.Fatalf("other occurrence: %v", err)
	}
	if equalIDs(first, other) {
		t.Fatalf("two occurrences a day apart planned the identical lineup %v", ids(first))
	}
}
```

Write `equalIDs`/`ids` as tiny local helpers if the file has none (check for existing ones first — `programIDs` or similar). Replace `newFakeStore()` with whatever the neighboring tests construct for `StateStore` (grep `NewEngineWithOptions(` in the test file and copy its store argument).

- [ ] **Step 2: Run it to verify it fails**

Run: `go test -race ./internal/scheduler -run TestPlanFilterBlock_DeterministicPerOccurrence -v`
Expected: FAIL on "same inputs planned different lineups" (the global shuffle differs between the two engines) — or, if the two calls happen to agree by chance, on the reversed-library assertion.

- [ ] **Step 3: Make `planFilterBlock` deterministic**

In `planFilterBlock`, replace the "Simple random shuffle and fill" block and the filler call:

```go
	// Deterministic per occurrence (draft-mode contract, v0.5.6): sort
	// the candidates by program ID so the library's arrival order cannot
	// leak in, then shuffle with the occurrence-seeded rng series
	// planning already uses. A dry run (GET /schedule, POST /generate)
	// and the apply that follows it therefore pick the SAME lineup for
	// the same block, occurrence, candidates, and history -- the plan the
	// operator previewed is the plan that lands. Variety still comes
	// from the seed varying per occurrence.
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].GetID() < candidates[j].GetID() })
	rng := occurrenceRand(block.ID, occurrenceStart)
	shuffleWith(rng, len(candidates), func(i, j int) {
		candidates[i], candidates[j] = candidates[j], candidates[i]
	})
```

and pass `rng` instead of `nil` to `e.getFiller(block, gapDuration, rng)`, rewriting the comment above that call:

```go
		// The occurrence-seeded rng, not nil/global: filler must be as
		// reproducible as the main lineup for a previewed plan to equal
		// the applied one (the first-commit freeze still applies after
		// apply; this makes the dry runs before it agree too).
```

Add `"sort"` to the imports if absent. Keep the `#nosec G404` comment on `occurrenceRand`'s definition (it is already there); the `rand.Shuffle` call in `shuffleWith`'s nil branch stays for callers that still pass nil.

- [ ] **Step 4: Run the engine tests**

Run: `go test -race ./internal/scheduler/...`
Expected: PASS, including the new test. If an existing test asserted that two plans differ, read it: it is asserting the old global-rand behavior, and this slice changes that on purpose — update the assertion to the new contract and say so in your report.

- [ ] **Step 5: Serialize applies on the Runner**

In `internal/service/schedule.go` add to the `Runner` struct:

```go
	// applyMu serializes applying Runs: the serve cron tick and a UI
	// apply share one Runner and both push lineups and Commit engine
	// state; letting them interleave was an accepted single-writer
	// assumption that the guide's draft mode makes routine to violate.
	// Dry runs never take it.
	applyMu sync.Mutex
```

and at the top of `Run`:

```go
	if o.Apply {
		r.applyMu.Lock()
		defer r.applyMu.Unlock()
	}
```

Import `"sync"`. Append to `internal/service/schedule_test.go` a test that builds a Runner the way its neighbors do (a fake Tunarr HTTP server + temp store), makes the fake's channel-programming handler sleep 30ms while counting in-flight requests with an atomic, launches two `Run(ctx, Options{Days: 1, Apply: true})` goroutines, and asserts the observed maximum in-flight count is 1 and both runs return without error. Name it `TestRunner_Run_SerializesApplies`.

- [ ] **Step 6: Run the service tests and the full suite**

Run: `go test -race ./internal/service/... && make test`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/scheduler/engine.go internal/scheduler/engine_test.go internal/service/schedule.go internal/service/schedule_test.go
git commit -m "fix(scheduler): deterministic filter-block lineups per occurrence; serialize applies"
```

---

### Task 2: The draft model (`runtime/draft.ts`), verdict rendering (`runtime/grid.ts`), send timeouts (`runtime/api.ts`)

**Files:**
- Create: `web/assets/ts/runtime/draft.ts`
- Create: `web/tests/draft.test.ts`
- Modify: `web/assets/ts/runtime/grid.ts` (the file header; `GuideSlot` ~line 248; `GridCallbacks` ~line 403; after `slotAriaLabel` ~line 465; `buildSlotPiece` ~line 482; `renderGuideWeek`'s `hasGhost` ~line 591 and `updateNow` ~line 715; `renderRundown`'s row body ~line 787)
- Modify: `web/assets/ts/runtime/api.ts` (`apiSend` ~line 301; the timeout constants ~line 229)
- Modify: `web/tests/grid.test.ts` (header comment line 4 mentions the schedule page's DAYS clamp — rewrite)

**Interfaces:**
- Consumes: `GuideRow`, `GuideSlot`, `GuideProgram`, `PlateParts` from `runtime/grid.ts`/`channels.ts`; `ApiRequestJSON<"generateSchedule">` from `runtime/api.ts`; `formatClock`, `plural` from `runtime/format.ts`.
- Produces (Tasks 3 and 4 depend on these exact names):

```ts
// runtime/grid.ts additions
export type DraftVerdict = "new" | "changed" | "same" | "removed" | "beyond";
export interface GuideSlot { /* existing fields */ draft?: DraftVerdict; }
export interface GridCallbacks { /* existing */ drawIn?: boolean; }
export function draftAriaPrefix(v: DraftVerdict | undefined): string;

// runtime/api.ts
export const LONG_SEND_TIMEOUT_MS = 120_000;
export function apiSend<T>(method: string, path: string, body?: unknown, timeoutMs?: number): Promise<T>;

// runtime/draft.ts
export const DRAFT_DAYS = 7;
export const DRAFT_HORIZON_MS = DRAFT_DAYS * 86_400_000;
export interface DraftSignature { days: number; channelId: string }
export function draftRequestBody(sig: DraftSignature): GenerateBody;
export function draftScopeFromSearch(search: string): string | null;
export function draftHref(channelId: string | undefined): string;
export interface DiffCounts { new: number; changed: number; same: number; removed: number }
export interface DiffOptions { requestedAt: number; scopeChannelId: string }
export function slotKey(s: GuideSlot): string;
export function lineupSignature(s: GuideSlot): string;
export function diffRows(readingRows: GuideRow[], draftRows: GuideRow[], opts: DiffOptions): { rows: GuideRow[]; counts: DiffCounts };
export interface StoredSlot { blockName: string; blockType: string; cron: string; priority: number; startMs: number; endMs: number; programs: GuideProgram[] }
export interface StoredReading { requestedAt: number; rows: { channelId: string; slots: StoredSlot[] }[] }
export const READING_STORAGE_KEY = "schedularr_guide_reading";
export const READING_MAX_AGE_MS = 24 * 60 * 60 * 1000;
export const READING_MAX_CHARS = 2_000_000;
export function serializeReading(requestedAt: number, rows: GuideRow[]): string | null;
export function parseStoredReading(raw: string | null, nowMs: number, lastAppliedAt?: string | null): StoredReading | null;
export function rowsFromStored(stored: StoredReading, plateFor: (channelId: string) => PlateParts): GuideRow[];
export function planSlotCount(plan: PlanResult): number;
export function planChannelCount(plan: PlanResult): number;
export function draftBarLine(p: { slots: number; channels: number; counts: DiffCounts; dropped: number; readingRequestedAt: number }): string;
export function draftingLine(scopeLabel: string): string;
export function applyingLine(slots: number, channels: number): string;
export function applyConfirmTitle(scopeLabel: string): string;
export function applyConfirmBody(p: { slots: number; channels: number; counts: DiffCounts; scopeLabel: string; readingRequestedAt: number }): string;
export function appliedTapeLine(slots: number, channels: number): string;
export function draftVerdictLine(v: DraftVerdict, readingRequestedAt: number, onAir: boolean): string;
```

- [ ] **Step 1: Write the failing tests**

Create `web/tests/draft.test.ts`:

```ts
// Unit tests for the guide's draft model (runtime/draft.ts): the
// reading-vs-draft diff, the storage codec, the request body, the URL
// param, and every line of draft copy. Pure module -- no DOM stubs.
// Instants use local-time Date constructors so the assertions hold in
// any timezone (same convention as grid.test.ts).
import assert from "node:assert/strict";
import test from "node:test";

const {
  DRAFT_DAYS,
  DRAFT_HORIZON_MS,
  READING_MAX_AGE_MS,
  READING_MAX_CHARS,
  appliedTapeLine,
  applyConfirmBody,
  applyConfirmTitle,
  applyingLine,
  diffRows,
  draftBarLine,
  draftHref,
  draftRequestBody,
  draftScopeFromSearch,
  draftVerdictLine,
  draftingLine,
  lineupSignature,
  parseStoredReading,
  planChannelCount,
  planSlotCount,
  rowsFromStored,
  serializeReading,
  slotKey,
} = await import("../assets/ts/runtime/draft.ts");
const { draftAriaPrefix } = await import("../assets/ts/runtime/grid.ts");

const at = (h: number, m = 0) => new Date(2026, 8, 8, h, m, 0, 0).getTime();
const dayAt = (d: number, h: number) => new Date(2026, 8, 8 + d, h, 0, 0, 0).getTime();

function prog(title: string, minutes: number, season?: number, episode?: number) {
  return { title, type: season === undefined ? "movie" : "episode", season, episode, durationMs: minutes * 60_000, startMs: 0 };
}

function slot(channelId: string, blockName: string, startMs: number, minutes: number, programs = [prog("A", minutes)]) {
  return {
    kind: "slot" as const,
    channelId,
    blockName,
    blockType: "filter",
    cron: "0 21 * * *",
    priority: 50,
    startMs,
    endMs: startMs + minutes * 60_000,
    programs,
  };
}

function ghost(channelId: string, blockName: string, startMs: number) {
  return {
    kind: "ghost" as const,
    channelId,
    blockName,
    blockType: "",
    cron: "",
    priority: 0,
    startMs,
    endMs: startMs + 30 * 60_000,
    programs: [],
    lostTo: "Winner",
  };
}

const plate = { ch: "CH 01", name: "Horror" };
const row = (channelId: string, slots: (ReturnType<typeof slot> | ReturnType<typeof ghost>)[]) => ({ channelId, plate, slots });
const opts = (requestedAt: number, scopeChannelId = "") => ({ requestedAt, scopeChannelId });

// ---- keys ------------------------------------------------------------------

test("slotKey is channel|block|start and lineupSignature is end + the ordered program identity", () => {
  const s = slot("c1", "Night", at(21), 60, [prog("Pilot", 30, 1, 1), prog("Feature", 30)]);
  assert.equal(slotKey(s), `c1|Night|${at(21)}`);
  assert.equal(lineupSignature(s), `${at(22)}#Pilot|1800000|1|1;Feature|1800000|-|-`);
});

// ---- diff ------------------------------------------------------------------

test("diffRows marks identical rows same and counts them", () => {
  const reading = [row("c1", [slot("c1", "Night", at(21), 60)])];
  const draft = [row("c1", [slot("c1", "Night", at(21), 60)])];
  const { rows, counts } = diffRows(reading, draft, opts(at(12)));
  assert.deepEqual(counts, { new: 0, changed: 0, same: 1, removed: 0 });
  assert.equal(rows[0].slots[0].draft, "same");
});

test("diffRows marks a draft-only slot new and a lineup change changed", () => {
  const reading = [row("c1", [slot("c1", "Night", at(21), 60, [prog("E1", 60, 1, 1)])])];
  const draft = [row("c1", [slot("c1", "Night", at(21), 60, [prog("E2", 60, 1, 2)]), slot("c1", "Late", at(23), 60)])];
  const { rows, counts } = diffRows(reading, draft, opts(at(12)));
  assert.deepEqual(counts, { new: 1, changed: 1, same: 0, removed: 0 });
  assert.equal(rows[0].slots.find((s) => s.blockName === "Night")?.draft, "changed");
  assert.equal(rows[0].slots.find((s) => s.blockName === "Late")?.draft, "new");
});

test("diffRows treats an end-time change as changed", () => {
  const reading = [row("c1", [slot("c1", "Night", at(21), 60)])];
  const draft = [row("c1", [slot("c1", "Night", at(21), 90)])];
  assert.equal(diffRows(reading, draft, opts(at(12))).counts.changed, 1);
});

test("diffRows marks reading-only slots removed when they overlap the draft window, drops the past, keeps beyond-horizon slots plain", () => {
  const reading = [
    row("c1", [
      slot("c1", "Morning", at(6), 60), // ended before the draft: past, dropped
      slot("c1", "OnAir", at(11, 30), 60), // airing at 12:00: removed (cut)
      slot("c1", "Night", at(21), 60), // still planned: same
      slot("c1", "Late", at(23), 60), // future, absent: removed
      slot("c1", "NextWeek", dayAt(8, 21), 60), // past the 7-day horizon: beyond
    ]),
  ];
  const draft = [row("c1", [slot("c1", "Night", at(21), 60)])];
  const { rows, counts } = diffRows(reading, draft, opts(at(12)));
  assert.deepEqual(counts, { new: 0, changed: 0, same: 1, removed: 2 });
  assert.deepEqual(
    rows[0].slots.map((s) => [s.blockName, s.draft]),
    [
      ["OnAir", "removed"],
      ["Night", "same"],
      ["Late", "removed"],
      ["NextWeek", "beyond"],
    ],
  );
  assert.equal(rows[0].slots[2].endMs, at(24));
});

test("diffRows bounds the horizon at exactly requestedAt + DRAFT_HORIZON_MS", () => {
  const start = at(12);
  const reading = [row("c1", [slot("c1", "Edge", start + DRAFT_HORIZON_MS, 60), slot("c1", "Inside", start + DRAFT_HORIZON_MS - 60_000, 60)])];
  const { rows, counts } = diffRows(reading, [row("c1", [])], opts(start));
  assert.deepEqual(rows[0].slots.map((s) => [s.blockName, s.draft]), [
    ["Inside", "removed"],
    ["Edge", "beyond"],
  ]);
  assert.equal(counts.removed, 1);
  assert.equal(DRAFT_DAYS, 7);
});

test("diffRows orders a removed slot before a new slot at the same start", () => {
  const reading = [row("c1", [slot("c1", "Old", at(21), 60)])];
  const draft = [row("c1", [slot("c1", "New", at(21), 60)])];
  const { rows } = diffRows(reading, draft, opts(at(12)));
  assert.deepEqual(rows[0].slots.map((s) => s.draft), ["removed", "new"]);
});

test("diffRows lets a draft ghost stand for a displaced reading slot (dropped, not removed)", () => {
  const reading = [row("c1", [slot("c1", "Loser", at(21), 60)])];
  const draft = [row("c1", [slot("c1", "Winner", at(21), 60), ghost("c1", "Loser", at(21))])];
  const { rows, counts } = diffRows(reading, draft, opts(at(12)));
  assert.deepEqual(counts, { new: 1, changed: 0, same: 0, removed: 0 });
  assert.deepEqual(rows[0].slots.map((s) => [s.kind, s.draft]), [
    ["slot", "new"],
    ["ghost", undefined],
  ]);
});

test("diffRows with ALL scope keeps a reading-only channel as an all-removed row", () => {
  const reading = [row("c1", [slot("c1", "Night", at(21), 60)]), row("c2", [slot("c2", "Kids", at(7), 60), slot("c2", "Prime", at(20), 60)])];
  const draft = [row("c1", [slot("c1", "Night", at(21), 60)])];
  const { rows, counts } = diffRows(reading, draft, opts(at(12)));
  assert.equal(rows.length, 2);
  assert.deepEqual(rows[1].slots.map((s) => s.draft), ["removed"], "07:00 ended before the draft: dropped");
  assert.deepEqual(counts, { new: 0, changed: 0, same: 1, removed: 1 });
});

test("diffRows with a channel scope diffs only that channel", () => {
  const reading = [row("c1", [slot("c1", "Night", at(21), 60)]), row("c2", [slot("c2", "Prime", at(20), 60)])];
  const draft = [row("c1", [slot("c1", "Night", at(21), 60)])];
  const { rows, counts } = diffRows(reading, draft, opts(at(12), "c1"));
  assert.equal(rows.length, 1);
  assert.equal(rows[0].channelId, "c1");
  assert.deepEqual(counts, { new: 0, changed: 0, same: 1, removed: 0 });
});

test("diffRows treats a channel absent from the reading as all new, and an empty reading as all new", () => {
  const draft = [row("c9", [slot("c9", "Fresh", at(21), 60)])];
  assert.equal(diffRows([], draft, opts(at(12))).rows[0].slots[0].draft, "new");
  assert.deepEqual(diffRows([row("c1", [])], draft, opts(at(12))).counts, { new: 1, changed: 0, same: 0, removed: 0 });
});

test("diffRows does not mutate its inputs", () => {
  const reading = [row("c1", [slot("c1", "Night", at(21), 60)])];
  const draft = [row("c1", [slot("c1", "Night", at(21), 60)])];
  diffRows(reading, draft, opts(at(12)));
  assert.equal(reading[0].slots[0].draft, undefined);
  assert.equal(draft[0].slots[0].draft, undefined);
});

// ---- request body / URL ----------------------------------------------------

test("draftRequestBody omits channel_id for ALL and sends it otherwise", () => {
  assert.deepEqual(draftRequestBody({ days: DRAFT_DAYS, channelId: "" }), { days: 7 });
  assert.deepEqual(draftRequestBody({ days: DRAFT_DAYS, channelId: "abc" }), { days: 7, channel_id: "abc" });
});

test("draftScopeFromSearch reads ?draft=all|<id> and nothing else", () => {
  assert.equal(draftScopeFromSearch(""), null);
  assert.equal(draftScopeFromSearch("?edit=x"), null);
  assert.equal(draftScopeFromSearch("?draft=all"), "");
  assert.equal(draftScopeFromSearch("?draft=ch%20one"), "ch one");
  assert.equal(draftScopeFromSearch("?draft="), null);
});

test("draftHref builds the guide arrival URL", () => {
  assert.equal(draftHref(undefined), "/?draft=all");
  assert.equal(draftHref(""), "/?draft=all");
  assert.equal(draftHref("ch one"), "/?draft=ch%20one");
});

// ---- storage codec ---------------------------------------------------------

test("stored reading round-trips trimmed rows, re-plates on restore, and expires", () => {
  const rows = [row("c1", [slot("c1", "Night", at(21), 60, [prog("E1", 60, 1, 1)]), ghost("c1", "Loser", at(22))])];
  const raw = serializeReading(at(12), rows);
  assert.ok(raw);
  const stored = parseStoredReading(raw, at(13));
  assert.ok(stored);
  assert.equal(stored.requestedAt, at(12));
  assert.deepEqual(stored.rows, [
    {
      channelId: "c1",
      slots: [{ blockName: "Night", blockType: "filter", cron: "0 21 * * *", priority: 50, startMs: at(21), endMs: at(22), programs: [prog("E1", 60, 1, 1)] }],
    },
  ]);
  const restored = rowsFromStored(stored, () => ({ ch: "CH 09", name: "Restored" }));
  assert.deepEqual(restored[0].plate, { ch: "CH 09", name: "Restored" });
  assert.equal(restored[0].slots[0].kind, "slot");
  assert.equal(restored[0].slots[0].channelId, "c1");
  assert.equal(parseStoredReading(raw, at(12) + READING_MAX_AGE_MS + 1), null);
  assert.equal(parseStoredReading(null, at(13)), null);
  assert.equal(parseStoredReading("{not json", at(13)), null);
  assert.equal(parseStoredReading(JSON.stringify({ requestedAt: "x", rows: [] }), at(13)), null);
  assert.equal(parseStoredReading(JSON.stringify({ requestedAt: at(12) }), at(13)), null);
});

test("a stored reading older than the last apply is not a baseline", () => {
  const raw = serializeReading(at(12), [row("c1", [])]);
  assert.ok(raw);
  assert.ok(parseStoredReading(raw, at(13), new Date(at(11)).toISOString()));
  assert.equal(parseStoredReading(raw, at(13), new Date(at(12, 30)).toISOString()), null);
  assert.ok(parseStoredReading(raw, at(13), "not a date"), "an unparseable stamp does not invalidate");
  assert.ok(parseStoredReading(raw, at(13), null));
});

test("serializeReading refuses a payload over READING_MAX_CHARS", () => {
  const programs = [];
  for (let i = 0; i < 30_000; i++) programs.push(prog(`Program ${i} with a fairly long title to inflate the payload`, 1));
  const rows = [row("c1", [slot("c1", "Huge", at(21), 60, programs)])];
  assert.equal(serializeReading(at(12), rows), null);
  assert.ok(READING_MAX_CHARS >= 1_000_000);
});

// ---- counts and copy -------------------------------------------------------

const wire = (start: number, name: string) => ({
  start_time: new Date(start).toISOString(),
  end_time: new Date(start + 3_600_000).toISOString(),
  block: { name, cron: "0 21 * * *", duration: 60, channel_id: "c1", type: "filter" as const },
  programs: [],
});

test("planSlotCount and planChannelCount read the wire shape", () => {
  const plan = { applied: false, channels: { c1: [wire(at(21), "A"), wire(at(23), "B")], c2: [wire(at(20), "C")] } };
  assert.equal(planSlotCount(plan), 3);
  assert.equal(planChannelCount(plan), 2);
});

test("draftBarLine prints counts, omitting zero verdicts, with the reading clock", () => {
  assert.equal(
    draftBarLine({ slots: 14, channels: 3, counts: { new: 2, changed: 1, same: 11, removed: 1 }, dropped: 1, readingRequestedAt: at(21, 2) }),
    "7-day draft — 14 slots across 3 channels · 2 new · 1 changed · 1 removed · 1 dropped · vs reading 21:02",
  );
  assert.equal(
    draftBarLine({ slots: 1, channels: 1, counts: { new: 0, changed: 0, same: 1, removed: 0 }, dropped: 0, readingRequestedAt: at(9, 5) }),
    "7-day draft — 1 slot across 1 channel · no changes · vs reading 09:05",
  );
  assert.equal(
    draftBarLine({ slots: 0, channels: 0, counts: { new: 0, changed: 0, same: 0, removed: 2 }, dropped: 0, readingRequestedAt: at(9, 5) }),
    "7-day draft — 0 slots across 0 channels · 2 removed · vs reading 09:05",
  );
});

test("drafting and applying lines", () => {
  assert.equal(draftingLine("ALL channels"), "Drafting — planning ALL channels against Tunarr…");
  assert.equal(draftingLine("CH 04 · HORROR"), "Drafting — planning CH 04 · HORROR against Tunarr…");
  assert.equal(applyingLine(14, 3), "Applying — 14 slots across 3 channels…");
});

test("apply confirm copy names scope, real counts, and the reading it is measured against", () => {
  assert.equal(applyConfirmTitle("ALL channels"), "Apply ALL channels");
  assert.equal(applyConfirmTitle("CH 04 · HORROR"), "Apply CH 04 · HORROR");
  assert.equal(
    applyConfirmBody({ slots: 14, channels: 3, counts: { new: 2, changed: 1, same: 11, removed: 1 }, scopeLabel: "ALL channels", readingRequestedAt: at(21, 2) }),
    "This applies 14 slots across 3 channels to Tunarr — 2 new, 1 changed, 1 removed vs the 21:02 reading.",
  );
  assert.equal(
    applyConfirmBody({ slots: 14, channels: 3, counts: { new: 0, changed: 0, same: 14, removed: 0 }, scopeLabel: "ALL channels", readingRequestedAt: at(21, 2) }),
    "This applies 14 slots across 3 channels to Tunarr — no changes vs the 21:02 reading.",
  );
  assert.equal(
    applyConfirmBody({ slots: 0, channels: 0, counts: { new: 0, changed: 0, same: 0, removed: 3 }, scopeLabel: "CH 04 · HORROR", readingRequestedAt: at(21, 2) }),
    "This draft schedules nothing for CH 04 · HORROR. Applying pushes an empty lineup to any channel Schedularr last applied in this scope — 3 slots removed vs the 21:02 reading.",
  );
  assert.equal(
    applyConfirmBody({ slots: 0, channels: 0, counts: { new: 0, changed: 0, same: 0, removed: 0 }, scopeLabel: "ALL channels", readingRequestedAt: at(21, 2) }),
    "This draft schedules nothing for ALL channels. Applying pushes an empty lineup to any channel Schedularr last applied in this scope.",
  );
});

test("appliedTapeLine is the spec's printout", () => {
  assert.equal(appliedTapeLine(14, 3), "Applied — 14 slots / 3 channels");
  assert.equal(appliedTapeLine(1, 1), "Applied — 1 slot / 1 channel");
});

test("draftVerdictLine explains each verdict against the timestamped reading", () => {
  assert.equal(draftVerdictLine("new", at(21, 2), false), "Draft: new — not in the 21:02 reading; the draft adds it.");
  assert.equal(draftVerdictLine("changed", at(21, 2), false), "Draft: changed — the lineup differs from the 21:02 reading.");
  assert.equal(draftVerdictLine("removed", at(21, 2), false), "Draft: removed — in the 21:02 reading, not in this draft; the draft does not schedule it.");
  assert.equal(draftVerdictLine("removed", at(21, 2), true), "Draft: removed — airing now in the 21:02 reading, not in this draft; applying cuts it.");
  assert.equal(draftVerdictLine("same", at(21, 2), false), "Draft: unchanged from the 21:02 reading.");
  assert.equal(draftVerdictLine("beyond", at(21, 2), false), "Beyond the 7-day draft — from the 21:02 reading, not part of this apply.");
});

test("draftAriaPrefix prefixes only the verdicts that change something", () => {
  assert.equal(draftAriaPrefix("new"), "Draft new — ");
  assert.equal(draftAriaPrefix("changed"), "Draft changed — ");
  assert.equal(draftAriaPrefix("removed"), "Draft removed — ");
  assert.equal(draftAriaPrefix("same"), "");
  assert.equal(draftAriaPrefix("beyond"), "");
  assert.equal(draftAriaPrefix(undefined), "");
});
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd web && node --disable-warning=MODULE_TYPELESS_PACKAGE_JSON --test tests/draft.test.ts`
Expected: FAIL — `Cannot find module '../assets/ts/runtime/draft.ts'`.

- [ ] **Step 3: `grid.ts` — the verdict**

1. After the `GuideProgram` interface:

```ts
/** A slot's verdict in draft mode (spec §3.3), computed client-side by
 * runtime/draft.ts against the committed reading: new / changed / same
 * inside the draft window, removed (in the reading, not in the draft),
 * beyond (a reading slot past the draft horizon, shown plain). Undefined
 * in committed mode and on ghosts. */
export type DraftVerdict = "new" | "changed" | "same" | "removed" | "beyond";
```

2. In `GuideSlot`, after `lostTo?: string;`: `/** draft mode only: the diff verdict. */ draft?: DraftVerdict;`.

3. In `GridCallbacks`:

```ts
  /** Draft mode entered from a committed grid already on the glass: new
   * and changed slots take the 200ms draw-in (motion inventory item 3).
   * Never set on an initial load; CSS suppresses it under reduced
   * motion. */
  drawIn?: boolean;
```

4. After `slotAriaLabel`:

```ts
/** The aria-label prefix for a verdict that changes something -- same,
 * beyond, and committed-mode slots get none (SC 1.4.1: the verdict chip
 * is the visible text; this is its spoken twin). */
export function draftAriaPrefix(v: DraftVerdict | undefined): string {
  if (v === "new" || v === "changed" || v === "removed") return `Draft ${v} — `;
  return "";
}

/** The visible verdict chip (`NEW` / `CHANGED` / `REMOVED`); null
 * otherwise. Text carries the fact -- the accent edge and the removed
 * hatch are secondary scan aids. */
function verdictChip(v: DraftVerdict | undefined): HTMLElement | null {
  if (v !== "new" && v !== "changed" && v !== "removed") return null;
  return el("span", "guide-slot__verdict", v.toUpperCase());
}
```

5. In `buildSlotPiece`: after `btn.dataset.type = slot.blockType;` add `if (slot.draft) btn.dataset.draft = slot.draft;` and `if (cb.drawIn && (slot.draft === "new" || slot.draft === "changed")) btn.classList.add("guide-slot--drawin");`. Change the aria-label base to `const base = draftAriaPrefix(slot.draft) + slotAriaLabel(slot);`. In the non-ghost branch, before the name span: `const chip = verdictChip(slot.draft); if (chip) btn.appendChild(chip);`.

6. In `renderGuideWeek`: `const hasLane2 = row.slots.some((s) => s.kind === "ghost" || s.draft === "removed");` replaces `hasGhost` (rename the variable and its comment: "A channel with any ghost OR any removed draft slot gets the two-lane template on EVERY segment — removed slots live in lane 2 next to the ghosts, so a replacement never paints over what it replaces"). In `updateNow`, the on-air glow skips removed slots: `const onAir = p.slot.draft !== "removed" && p.slot.startMs <= nowMs && nowMs < p.slot.endMs;`.

7. In `renderRundown`'s row loop: after `btn.dataset.type = slot.blockType;` add `if (slot.draft) btn.dataset.draft = slot.draft;`; prefix both aria-label branches with `draftAriaPrefix(slot.draft) +`; in the non-ghost body branch, before the name span, append the chip the same way. In its `updateNow`, the same on-air exclusion for removed slots.

8. The file header's first paragraph gains: "In draft mode (spec §3.3) every slot carries a verdict (`GuideSlot.draft`) that renders as `data-draft` plus a text chip; the diff itself lives in runtime/draft.ts." Rewrite the three stale forward references at ~lines 271, 288, 417 to name themes ("the Memory slice", "the SSE live-link slice"), not versions.

- [ ] **Step 4: `api.ts` — send timeout**

Replace the timeout constants block with:

```ts
// Reads never take this long against a same-LAN instance. Writes get more
// headroom because /generate and /apply do real planning work against
// Tunarr before they answer. The guide's GET /schedule and its draft
// mode's POST /generate and POST /apply are the sanctioned long calls:
// the first plan after a cold pod start (Tunarr itself waking, empty
// caches) has been observed to exceed a minute, and an apply also pushes
// every channel's lineup after planning -- a client-side abort mid-push
// would leave a half-landed apply behind, so the guide passes
// LONG_SEND_TIMEOUT_MS explicitly. Every other read stays on 15s and
// every other write on 60s.
const GET_TIMEOUT_MS = 15_000;
export const LONG_GET_TIMEOUT_MS = 90_000;
const SEND_TIMEOUT_MS = 60_000;
export const LONG_SEND_TIMEOUT_MS = 120_000;
```

and give `apiSend` a fourth parameter `timeoutMs: number = SEND_TIMEOUT_MS`, forwarded to `request(...)`. Update its doc comment: "timeoutMs is the write tier -- omit it everywhere except the guide's draft preview/apply (LONG_SEND_TIMEOUT_MS)."

- [ ] **Step 5: `draft.ts`**

```ts
// The guide's draft model (spec §3.3): pure functions behind draft mode.
// A DRAFT is a 7-day POST /generate result for a SCOPE, armed for apply;
// the READING is the committed-mode plan the guide already shows (always
// ALL channels, 28 days). This module diffs the two, keeps every line of
// draft copy in one place, and codes the reading to/from sessionStorage
// for the Blocks-page round trip. No DOM, no fetch -- guide.ts wires it;
// web/tests/draft.test.ts pins it.
//
// Honesty boundary: nothing here knows what Tunarr currently holds. A
// verdict is always "vs the reading taken at HH:MM" -- the bar, the
// confirm, and the inspector say so in text -- never a claim about
// Tunarr's lineup. That question belongs to the Memory slice's enriched
// history.
//
// Why 7 days: the engine prunes occurrence snapshots and history by
// write time on every commit (history_retention, 7 days by default), so
// an apply wider than the retention window commits state the store
// forgets before it airs. Seven days is the CLI default, the old
// Schedule page's default, and the spec's confirmed default (§11 Q6).
import type { ApiRequestJSON } from "./api.ts";
import type { PlateParts } from "./channels.ts";
import { formatClock, plural } from "./format.ts";
import type { DraftVerdict, GuideProgram, GuideRow, GuideSlot } from "./grid.ts";
import type { components } from "../gen/types";

type PlanResult = components["schemas"]["PlanResult"];
type GenerateBody = ApiRequestJSON<"generateSchedule">;

/** The draft/apply window. */
export const DRAFT_DAYS = 7;
export const DRAFT_HORIZON_MS = DRAFT_DAYS * 86_400_000;

/** Exactly what a draft was generated from; apply() sends this, verbatim. */
export interface DraftSignature {
  days: number;
  channelId: string;
}

/** The GenerateRequest body for a signature -- channel_id omitted (never
 * sent as "") for ALL channels. Shared by preview and apply so the
 * armed-signature rule holds by construction. */
export function draftRequestBody(sig: DraftSignature): GenerateBody {
  const body: GenerateBody = { days: sig.days };
  if (sig.channelId !== "") body.channel_id = sig.channelId;
  return body;
}

/** `?draft=all` -> "" (ALL channels), `?draft=<id>` -> the id, absent or
 * empty -> null (no draft requested). */
export function draftScopeFromSearch(search: string): string | null {
  const value = new URLSearchParams(search).get("draft");
  if (value === null || value === "") return null;
  return value === "all" ? "" : value;
}

/** The guide URL a block save's PREVIEW ON GUIDE action navigates to. */
export function draftHref(channelId: string | undefined): string {
  return `/?draft=${encodeURIComponent(channelId ? channelId : "all")}`;
}

// ---- diff -------------------------------------------------------------------

export interface DiffCounts {
  new: number;
  changed: number;
  same: number;
  removed: number;
}

export interface DiffOptions {
  /** When the draft request was SENT (browser clock): the diff cutoff
   * and the start of the 7-day horizon. */
  requestedAt: number;
  /** "" for ALL channels, else the one channel the draft planned. */
  scopeChannelId: string;
}

/** Occurrence identity: the engine keys an occurrence by (block, start)
 * on one channel. Block names are unique in the store. */
export function slotKey(s: GuideSlot): string {
  return `${s.channelId}|${s.blockName}|${s.startMs}`;
}

/** Content identity: the end time plus the ordered lineup. Season and
 * episode print as "-" when absent so a movie and an episode never
 * collide by accident. */
export function lineupSignature(s: GuideSlot): string {
  const programs = s.programs
    .map((p) => `${p.title}|${p.durationMs}|${p.season ?? "-"}|${p.episode ?? "-"}`)
    .join(";");
  return `${s.endMs}#${programs}`;
}

function withVerdict(s: GuideSlot, draft: DraftVerdict): GuideSlot {
  return { ...s, draft };
}

function rank(s: GuideSlot): number {
  return s.draft === "removed" ? 0 : 1;
}

/**
 * Diffs the draft's rows against the reading's rows and returns NEW rows
 * (inputs untouched):
 *
 *   - every draft slot carries new / changed / same;
 *   - a reading slot the draft no longer plans is `removed` when it
 *     overlaps the draft window (ends after requestedAt, starts before
 *     the horizon) -- the on-air slot included, since applying cuts it;
 *     `beyond` when it starts at or past the horizon (shown plain, not
 *     counted -- the draft never planned that far); dropped silently when
 *     it ended before the draft was requested (the past, not a removal);
 *   - a reading slot a draft ghost stands for (same channel, block, and
 *     start) is left to the ghost: it is DROPPED, not removed, so one
 *     displaced occurrence is never counted twice;
 *   - ghosts pass through unverdicted and uncounted.
 *
 * With scopeChannelId "" every reading channel is in scope (a channel the
 * draft dropped entirely becomes an all-removed row); with a channel
 * scope only that channel is compared and returned. Slots sort by start,
 * `removed` first at a tie so a replacement never paints over what it
 * replaces (removed slots render in the second lane anyway).
 */
export function diffRows(
  readingRows: GuideRow[],
  draftRows: GuideRow[],
  opts: DiffOptions,
): { rows: GuideRow[]; counts: DiffCounts } {
  const horizonMs = opts.requestedAt + DRAFT_HORIZON_MS;
  const counts: DiffCounts = { new: 0, changed: 0, same: 0, removed: 0 };
  const readingByChannel = new Map(readingRows.map((r) => [r.channelId, r]));
  const draftByChannel = new Map(draftRows.map((r) => [r.channelId, r]));
  const channelIds = new Set<string>();
  for (const r of draftRows) channelIds.add(r.channelId);
  if (opts.scopeChannelId === "") for (const r of readingRows) channelIds.add(r.channelId);
  else if (readingByChannel.has(opts.scopeChannelId)) channelIds.add(opts.scopeChannelId);

  const rows: GuideRow[] = [];
  for (const channelId of channelIds) {
    const reading = readingByChannel.get(channelId);
    const draft = draftByChannel.get(channelId);
    const readingSlots = new Map<string, GuideSlot>();
    for (const s of reading?.slots ?? []) if (s.kind === "slot") readingSlots.set(slotKey(s), s);

    const slots: GuideSlot[] = [];
    const seen = new Set<string>();
    for (const s of draft?.slots ?? []) {
      const key = slotKey(s);
      if (s.kind !== "slot") {
        // A ghost stands for the reading slot it displaced.
        seen.add(key);
        slots.push(s);
        continue;
      }
      seen.add(key);
      const before = readingSlots.get(key);
      let verdict: DraftVerdict;
      if (!before) verdict = "new";
      else if (lineupSignature(before) !== lineupSignature(s)) verdict = "changed";
      else verdict = "same";
      counts[verdict]++;
      slots.push(withVerdict(s, verdict));
    }
    for (const [key, before] of readingSlots) {
      if (seen.has(key)) continue;
      if (before.startMs >= horizonMs) {
        slots.push(withVerdict(before, "beyond"));
        continue;
      }
      if (before.endMs <= opts.requestedAt) continue;
      counts.removed++;
      slots.push(withVerdict(before, "removed"));
    }
    slots.sort((a, b) => a.startMs - b.startMs || rank(a) - rank(b));
    rows.push({ channelId, plate: (draft ?? reading)!.plate, slots });
  }
  return { rows, counts };
}

// ---- reading persistence ----------------------------------------------------

/** One reading slot as stored: the fields the diff and the inspector
 * need, nothing from the wire (no ISO strings, no block spec, no plate
 * -- plates are re-resolved on restore). Ghosts are not stored. */
export interface StoredSlot {
  blockName: string;
  blockType: string;
  cron: string;
  priority: number;
  startMs: number;
  endMs: number;
  programs: GuideProgram[];
}

/** The committed reading, mirrored to sessionStorage so a Blocks-page
 * round trip (save -> PREVIEW ON GUIDE) can diff against what the
 * operator last saw. It seeds the diff baseline ONLY -- a restored
 * reading is never shown as current: DISCARD and apply re-fetch. */
export interface StoredReading {
  requestedAt: number;
  rows: { channelId: string; slots: StoredSlot[] }[];
}

export const READING_STORAGE_KEY = "schedularr_guide_reading";
export const READING_MAX_AGE_MS = 24 * 60 * 60 * 1000;
/** Above this many characters the mirror is skipped (sessionStorage
 * quotas sit around 5 MB; a 20-channel four-week plan can exceed it). */
export const READING_MAX_CHARS = 2_000_000;

/** null when the payload would be too large to store. */
export function serializeReading(requestedAt: number, rows: GuideRow[]): string | null {
  const stored: StoredReading = {
    requestedAt,
    rows: rows.map((r) => ({
      channelId: r.channelId,
      slots: r.slots
        .filter((s) => s.kind === "slot")
        .map((s) => ({
          blockName: s.blockName,
          blockType: s.blockType,
          cron: s.cron,
          priority: s.priority,
          startMs: s.startMs,
          endMs: s.endMs,
          programs: s.programs,
        })),
    })),
  };
  const raw = JSON.stringify(stored);
  return raw.length > READING_MAX_CHARS ? null : raw;
}

/**
 * null for anything missing, malformed, older than READING_MAX_AGE_MS,
 * or taken before the server's last apply (lastAppliedAt =
 * Status.last_applied_at) -- a reading from before an apply is not what
 * the operator would be diffing against. An unparseable stamp does not
 * invalidate (the server simply has not recorded one).
 */
export function parseStoredReading(raw: string | null, nowMs: number, lastAppliedAt?: string | null): StoredReading | null {
  if (raw === null) return null;
  let parsed: unknown;
  try {
    parsed = JSON.parse(raw);
  } catch {
    return null;
  }
  if (typeof parsed !== "object" || parsed === null) return null;
  const { requestedAt, rows } = parsed as { requestedAt?: unknown; rows?: unknown };
  if (typeof requestedAt !== "number" || !Number.isFinite(requestedAt)) return null;
  if (!Array.isArray(rows)) return null;
  if (nowMs - requestedAt > READING_MAX_AGE_MS) return null;
  if (lastAppliedAt) {
    const applied = Date.parse(lastAppliedAt);
    if (!Number.isNaN(applied) && applied > requestedAt) return null;
  }
  return { requestedAt, rows: rows as StoredReading["rows"] };
}

/** Rebuilds renderable rows from a stored reading, re-resolving plates
 * through the live channel cache. */
export function rowsFromStored(stored: StoredReading, plateFor: (channelId: string) => PlateParts): GuideRow[] {
  return stored.rows.map((r) => ({
    channelId: r.channelId,
    plate: plateFor(r.channelId),
    slots: r.slots.map((s) => ({ kind: "slot" as const, channelId: r.channelId, ...s })),
  }));
}

// ---- counts and copy --------------------------------------------------------

export function planSlotCount(plan: PlanResult): number {
  return Object.values(plan.channels).reduce((n, slots) => n + slots.length, 0);
}

export function planChannelCount(plan: PlanResult): number {
  return Object.keys(plan.channels).length;
}

function verdictParts(counts: DiffCounts): string[] {
  const parts: string[] = [];
  if (counts.new > 0) parts.push(`${counts.new} new`);
  if (counts.changed > 0) parts.push(`${counts.changed} changed`);
  if (counts.removed > 0) parts.push(`${counts.removed} removed`);
  return parts;
}

/** The draft bar's readout. Zero verdict counts are omitted; a draft
 * with no verdict at all prints "no changes". The reading's clock keeps
 * the comparison honest: it is a diff against THAT reading. */
export function draftBarLine(p: {
  slots: number;
  channels: number;
  counts: DiffCounts;
  dropped: number;
  readingRequestedAt: number;
}): string {
  const parts = [`${DRAFT_DAYS}-day draft — ${plural(p.slots, "slot")} across ${plural(p.channels, "channel")}`];
  const verdicts = verdictParts(p.counts);
  parts.push(...(verdicts.length > 0 ? verdicts : ["no changes"]));
  if (p.dropped > 0) parts.push(`${p.dropped} dropped`);
  parts.push(`vs reading ${formatClock(p.readingRequestedAt)}`);
  return parts.join(" · ");
}

export function draftingLine(scopeLabel: string): string {
  return `Drafting — planning ${scopeLabel} against Tunarr…`;
}

export function applyingLine(slots: number, channels: number): string {
  return `Applying — ${plural(slots, "slot")} across ${plural(channels, "channel")}…`;
}

export function applyConfirmTitle(scopeLabel: string): string {
  return `Apply ${scopeLabel}`;
}

/** Real counts from the draft on the glass, never a guess. An empty
 * draft is not "nothing": the server pushes an empty lineup to every
 * channel it last applied inside the scope, so the body says so. */
export function applyConfirmBody(p: {
  slots: number;
  channels: number;
  counts: DiffCounts;
  scopeLabel: string;
  readingRequestedAt: number;
}): string {
  const when = formatClock(p.readingRequestedAt);
  if (p.channels === 0) {
    const head = `This draft schedules nothing for ${p.scopeLabel}. Applying pushes an empty lineup to any channel Schedularr last applied in this scope`;
    return p.counts.removed > 0 ? `${head} — ${plural(p.counts.removed, "slot")} removed vs the ${when} reading.` : `${head}.`;
  }
  const verdicts = verdictParts(p.counts);
  const tail = verdicts.length > 0 ? verdicts.join(", ") : "no changes";
  return `This applies ${plural(p.slots, "slot")} across ${plural(p.channels, "channel")} to Tunarr — ${tail} vs the ${when} reading.`;
}

export function appliedTapeLine(slots: number, channels: number): string {
  return `Applied — ${plural(slots, "slot")} / ${plural(channels, "channel")}`;
}

/** The inspector's verdict line for a draft slot. */
export function draftVerdictLine(v: DraftVerdict, readingRequestedAt: number, onAir: boolean): string {
  const when = formatClock(readingRequestedAt);
  switch (v) {
    case "new":
      return `Draft: new — not in the ${when} reading; the draft adds it.`;
    case "changed":
      return `Draft: changed — the lineup differs from the ${when} reading.`;
    case "removed":
      return onAir
        ? `Draft: removed — airing now in the ${when} reading, not in this draft; applying cuts it.`
        : `Draft: removed — in the ${when} reading, not in this draft; the draft does not schedule it.`;
    case "beyond":
      return `Beyond the ${DRAFT_DAYS}-day draft — from the ${when} reading, not part of this apply.`;
    default:
      return `Draft: unchanged from the ${when} reading.`;
  }
}
```

- [ ] **Step 6: Run the tests, then type-check**

Run: `cd web && node --disable-warning=MODULE_TYPELESS_PACKAGE_JSON --test tests/draft.test.ts tests/grid.test.ts tests/runtime.test.ts`
Expected: all PASS, output pristine.
Run: `make web-check`
Expected: clean. (If the `lineupSignature` expectation and the implementation disagree on the `endMs` prefix, the implementation above is the authority — fix the test's expected value.)

- [ ] **Step 7: Commit**

```bash
git add web/assets/ts/runtime/draft.ts web/tests/draft.test.ts web/assets/ts/runtime/grid.ts web/assets/ts/runtime/api.ts web/tests/grid.test.ts
git commit -m "feat(web): draft model — reading-vs-draft diff, storage codec, verdict rendering, long send tier"
```

---

### Task 3: Draft mode on the Guide (`guide.ts`, `index.html`, `main.css`, `shell.ts`)

**Files:**
- Modify: `web/assets/ts/pages/guide.ts` (whole component)
- Modify: `web/layouts/index.html` (root element, toolbar, draft bar + problems + draft empty state, inspector, confirm dialog)
- Modify: `web/assets/css/main.css` (new rules after `.guide-droplegend` ~line 2050; after the `.guide-slot--ghost` hover rule ~line 2540; the `.guide-viewport` rule ~line 2117; the `forced-colors` block near the end; the reduced-motion block)
- Modify: `web/assets/ts/runtime/shell.ts` (`--bezel-h`)
- Read first: `/Users/christophe/.claude/skills/impeccable/reference/craft-floor.md`; `web/DESIGN.md` sections "The guide grid", "Component-state floor", "Content-Security-Policy"; `web/assets/ts/pages/schedule.ts` (the page whose armed-signature discipline moves here; it is deleted in Task 4); `web/layouts/partials/ui/confirm.html`.

**Interfaces:**
- Consumes (Task 2): everything exported from `runtime/draft.ts`; `DraftVerdict`, `GridCallbacks.drawIn`, `GuideSlot.draft` from `runtime/grid.ts`; `apiSend(..., LONG_SEND_TIMEOUT_MS)` from `runtime/api.ts`; `printTape` from `runtime/tape.ts`; `problemLine` from `runtime/errors.ts`.
- Produces: `#guide-arm` (the arming button), `#guide-draftbar`, `data-draft` on the guide root, `#guide-viewport` and `#guide-rundown` while a draft is on the glass; the `--bezel-h` root property. Task 5's kit copies the draft bar markup.

**Behavior contract (binding):**

1. **Reading.** State: `readingRows: GuideRow[]`, `readingRequestedAt: number`, `readingRestored: boolean`, `plan: PlanResult | null` (the wire plan, for `hasBlocks`-style checks only). `reload()` records `requestedAt = Date.now()` BEFORE sending `GET /schedule?days=28` (no `channel_id`, `LONG_GET_TIMEOUT_MS`); on success sets `plan`, `readingRows = projectPlan(plan).rows`, `readingRequestedAt = requestedAt`, `readingRestored = false`, and mirrors the reading (`serializeReading(requestedAt, readingRows)`; skip on null; try/catch around the storage accessor and `setItem`) on EVERY landing, an empty plan included. `windowStartMs`/`loadedDays` from the landing time as today. GET /blocks in parallel as today. `reload()` returns after rendering; if `draft.pendingScope` is set, it clears it and calls `preview()`.
2. **Draft state** (`this.draft`): `mode: "off" | "previewing" | "armed" | "applying"`, `signature: DraftSignature | null`, `rows: GuideRow[] | null` (projected draft rows), `plan: PlanResult | null`, `requestedAt: number`, `counts: DiffCounts | null`, `dropped: number`, `previewError: ProblemView | null`, `applyError: ProblemView | null`, `applyTimedOut: boolean`, `applyFailed: boolean`, `seq: number`, `pendingScope: boolean`. `emptyDraft(seq)` resets everything but `seq`.
3. **Entry points.** (a) SCOPE `@change` → `requestDraft()`; (b) `#guide-arm` click → `requestDraft()`; (c) `init()`: `scope = draftScopeFromSearch(window.location.search)`; when non-null: `history.replaceState(null, "", "/")` immediately; `controls.channelId = scope`; fetch `GET /status` (15s tier, catch → null) and read `sessionStorage[READING_STORAGE_KEY]` (try/catch → null); `stored = parseStoredReading(raw, Date.now(), status?.last_applied_at ?? null)`. If `stored`: `readingRows = rowsFromStored(stored, (id) => channelPlate(id, this.channels))` (re-projected when channels land: key the rows memo on `channels` identity), `readingRequestedAt = stored.requestedAt`, `plan = null`, `readingRestored = true`, `loading = false`, fire the silent `GET /blocks` enrichment (same catch as `reload()`), then `preview()`. If not: `await reload()` then `preview()`. The stored reading is never rendered as committed: any path that leaves draft mode from a restored baseline calls `reload()`.
4. **`requestDraft()`**: if `loading || draft.mode === "previewing" || draft.mode === "applying"` → `draft.pendingScope = true`; else `void preview()`.
5. **`preview()`**: `signature = { days: DRAFT_DAYS, channelId: controls.channelId.trim() }`; `requestedAt = Date.now()` BEFORE sending; `seq = ++draft.seq`; `wasGridOnGlass = !loading && !problem && rows().length > 0`; mode `previewing`; clear `previewError`/`applyError`/`applyTimedOut`; `statusLine = draftingLine(scopeLabelFor(signature.channelId))`; `apiSend<PlanResult>("POST", apiPath("/generate"), draftRequestBody(signature), LONG_SEND_TIMEOUT_MS)`. On landing, if `seq !== draft.seq` → ignore. Success: `draft.plan`, `draft.rows = projectPlan(result).rows`, `draft.dropped = projectPlan(result).dropped`, `draft.signature`, `draft.requestedAt = requestedAt`, `draft.counts = diffRows(readingRows, draft.rows, { requestedAt, scopeChannelId }).counts`, `windowStartMs = localDayStart(requestedAt)`, `loadedDays = windowDayCount(requestedAt, 28)` (the reading's later weeks still render as `beyond`), `weekPage = 0`, mode `armed`, `statusLine = draftBarLine(...)`, `closeInspector(false)`, `renderAll({ drawIn: wasGridOnGlass })`. Failure: `previewError = toProblemView(err)`, mode `off`, `draft.rows/plan/counts = null`, `statusLine = "Draft failed — " + problemLine(previewError)`, `renderAll()`; if `readingRestored` and no committed plan is on the glass, call `reload()` so committed mode has a reading (the restored one is never shown as current). Either way, fire the latch if set.
6. **`rows()`** = draft on the glass (`draft.rows !== null && draft.mode !== "off"`) ? `diffRows(readingRows, draft.rows, { requestedAt: draft.requestedAt, scopeChannelId: draft.signature.channelId }).rows` (memoized on `(readingRows, draft.rows)` identity) : `readingRows`. `droppedWarnings()` = draft on the glass ? `draft.dropped` : the reading projection's `dropped`. `hasAnySlots()` = `rows().some((r) => r.slots.length > 0)`. The `projection()` body becomes `projectPlan(plan)` (memoized per plan identity + blocksByName + channels), reused for both plans.
7. **Rendering.** `renderAll(opts?: { drawIn?: boolean; settle?: boolean })` forwards `drawIn` through `GridCallbacks`; toggles `dataset.draft` on `#guide-viewport`, `#guide-rundown`, and the `.guide` root (`this.$root`) according to draft-on-glass; with `settle` it adds `guide-sheet--settle` to the freshly rendered `.guide-sheet` (the 200ms committed-grid settle after a non-refetch DISCARD).
8. **Apply.** `canApply()` = `draft.mode === "armed" && draft.plan && draft.signature`. `requestApply()` opens `$refs.confirmDialog` (no-op unless `canApply()`). `confirmApply()`: mode `applying`, `statusLine = applyingLine(...)`, the dialog STAYS OPEN (its confirm button carries `aria-busy`, Escape/backdrop are guarded by `busy`); `apiSend<PlanResult>("POST", apiPath("/apply"), draftRequestBody(draft.signature), LONG_SEND_TIMEOUT_MS)`. Success: `cancelApply(true)` (close), `printTape(appliedTapeLine(planSlotCount(result), planChannelCount(result)))`, `statusLine = "Applied — …"`, `this.draft = emptyDraft(seq)`, `controls.channelId = ""`, `closeInspector(false)`, focus `#guide-arm`, then `void reload()` (the reading is always refetched after an apply; the skeleton and its honest note show meanwhile). Failure: `cancelApply(true)`, mode `armed`, `applyError = toProblemView(err)`, `applyTimedOut = err instanceof ApiError && err.status === 0`, `applyFailed = true`, focus `#guide-arm`; the problem renders under the bar — label `Apply timed out — it may have partially landed` with retry `requestDraft()` (re-preview first) when timed out, else label `Apply failed` with retry `requestApply()`. `cancelApply(force = false)` refuses to close while applying unless forced.
9. **Discard.** `discardDraft()`: clear the draft (keep `seq`), `controls.channelId = ""`, `closeInspector(false)`, focus `#guide-arm`; if `readingRestored || applyFailed-before-clear` → `void reload()`; else `renderAll({ settle: true })` and `statusLine = "Draft discarded — reading restored"`.
10. **Scope label.** `scopeLabelFor(channelId)` = `"ALL channels"` when `""`, else the plate text `CH 04 · HORROR` (`channelPlate`; `ch` null → the name alone, which is the shortened raw id when unresolved). `scopeLabel()` uses the draft signature's channel when armed, else the control's.
11. **Inspector.** When `inspector.slot.draft` is set, the panel body starts with `<p class="guide-inspector__verdict-line" x-text="inspectorVerdictLine()">` = `draftVerdictLine(slot.draft, readingRequestedAt, slot.startMs <= Date.now() && Date.now() < slot.endMs)`.
12. **Announcements.** Every transition updates `statusLine`; the draft bar carries `aria-busy` while previewing/applying.
13. **Re-auth.** `onReauth`: `reload()` if `problem`, `preview()` if `previewError`.
14. **Week pager** never touches the draft. `pageWeek` re-renders `rows()`.
15. **Escape.** The window Escape handler closes the inspector only when the confirm dialog is not open.
16. **Arm button** is disabled only while previewing or applying (never while loading — presses latch), so it can always take focus.
17. Comments: rewrite the header and every inline comment that says the old Schedule page owns preview/apply; forward references name themes ("the SSE live-link slice", "the Memory slice"), never numbers.

- [ ] **Step 1: `shell.ts` — `--bezel-h`**

At the end of `initShell()`:

```ts
  // The draft bar (guide) sticks under the bezel, whose height varies as
  // its rows wrap: publish it as a root custom property via CSSOM (the
  // CSP forbids inline styles, not CSSOM writes). ResizeObserver is
  // universal in the supported browsers; guard anyway for the test stubs.
  const bezel = document.querySelector<HTMLElement>(".bezel");
  if (bezel && typeof ResizeObserver !== "undefined") {
    const publish = (): void => {
      document.documentElement.style.setProperty("--bezel-h", `${bezel.offsetHeight}px`);
    };
    publish();
    new ResizeObserver(publish).observe(bezel);
  }
```

- [ ] **Step 2: `guide.ts`**

Implement the contract. Skeleton (existing members not shown stay):

```ts
import { ApiError, LONG_GET_TIMEOUT_MS, LONG_SEND_TIMEOUT_MS, apiGet, apiPath, apiSend, onReauth } from "../runtime/api.ts";
import type { ApiResponse } from "../runtime/api.ts";
import {
  DRAFT_DAYS,
  READING_STORAGE_KEY,
  appliedTapeLine,
  applyConfirmBody,
  applyConfirmTitle,
  applyingLine,
  diffRows,
  draftBarLine,
  draftRequestBody,
  draftScopeFromSearch,
  draftVerdictLine,
  draftingLine,
  parseStoredReading,
  planChannelCount,
  planSlotCount,
  rowsFromStored,
  serializeReading,
} from "../runtime/draft.ts";
import type { DiffCounts, DraftSignature } from "../runtime/draft.ts";
import { problemLine, toProblemView } from "../runtime/errors.ts";
import { printTape } from "../runtime/tape.ts";

type PlanResult = ApiResponse<"getSchedule", 200>;
type Status = ApiResponse<"getStatus", 200>;

/** The reading fetched by the guide (spec §3.1): the whole 28-day window,
 * ALL channels, paged client-side. Drafts plan DRAFT_DAYS of it. */
const FETCH_DAYS = 28;

type DraftMode = "off" | "previewing" | "armed" | "applying";

interface DraftState {
  mode: DraftMode;
  signature: DraftSignature | null;
  plan: PlanResult | null;
  rows: GuideRow[] | null;
  dropped: number;
  /** When the draft request was sent: the diff cutoff and horizon start. */
  requestedAt: number;
  counts: DiffCounts | null;
  previewError: ProblemView | null;
  applyError: ProblemView | null;
  /** A client-side abort (status 0): the apply may have partially landed. */
  applyTimedOut: boolean;
  /** Any apply failure: the reading may no longer match Tunarr, so
   * DISCARD re-fetches instead of restoring it. */
  applyFailed: boolean;
  /** In-flight guard: a preview that lands after a newer one started is
   * dropped on the floor. */
  seq: number;
  /** A SCOPE change or Arm press that landed during a reload, preview,
   * or apply -- re-fired once the flight lands. */
  pendingScope: boolean;
}

function emptyDraft(seq = 0): DraftState {
  return {
    mode: "off", signature: null, plan: null, rows: null, dropped: 0, requestedAt: 0, counts: null,
    previewError: null, applyError: null, applyTimedOut: false, applyFailed: false, seq, pendingScope: false,
  };
}

function storeReading(requestedAt: number, rows: GuideRow[]): void {
  try {
    const raw = serializeReading(requestedAt, rows);
    if (raw !== null) window.sessionStorage.setItem(READING_STORAGE_KEY, raw);
  } catch {
    // Quota or a privacy lockdown: the mirror is a convenience for the
    // Blocks round trip, never load-bearing.
  }
}

function readStoredReading(nowMs: number, lastAppliedAt: string | null) {
  try {
    return parseStoredReading(window.sessionStorage.getItem(READING_STORAGE_KEY), nowMs, lastAppliedAt);
  } catch {
    return null;
  }
}

function focusArm(): void {
  document.getElementById("guide-arm")?.focus();
}
```

Write `init`, `reload`, `requestDraft`, `preview`, `rows`, `renderAll`, `canApply`, `requestApply`, `confirmApply`, `cancelApply`, `discardDraft`, `scopeLabelFor`, `scopeLabel`, `draftBarLine`, `applyConfirmTitle`, `applyConfirmBody`, `inspectorVerdictLine`, `draftOnGlass` per the contract. `preview()` in full:

```ts
      async preview() {
        const signature: DraftSignature = { days: DRAFT_DAYS, channelId: this.controls.channelId.trim() };
        const requestedAt = Date.now();
        const seq = ++this.draft.seq;
        const wasGridOnGlass = !this.loading && !this.problem && this.rows().length > 0;
        this.draft.mode = "previewing";
        this.draft.previewError = null;
        this.draft.applyError = null;
        this.draft.applyTimedOut = false;
        this.statusLine = draftingLine(this.scopeLabelFor(signature.channelId));
        try {
          const result = await apiSend<PlanResult>(
            "POST",
            apiPath("/generate"),
            draftRequestBody(signature),
            LONG_SEND_TIMEOUT_MS,
          );
          if (seq !== this.draft.seq) return;
          const projected = this.projectPlan(result);
          this.draft.plan = result;
          this.draft.rows = projected.rows;
          this.draft.dropped = projected.dropped;
          this.draft.signature = signature;
          this.draft.requestedAt = requestedAt;
          this.windowStartMs = localDayStart(requestedAt);
          this.loadedDays = windowDayCount(requestedAt, FETCH_DAYS);
          this.weekPage = 0;
          this.draft.mode = "armed";
          this.draft.counts = this.draftDiff().counts;
          this.statusLine = this.draftBarLine();
          this.closeInspector(false);
          this.renderAll({ drawIn: wasGridOnGlass });
        } catch (err) {
          if (seq !== this.draft.seq) return;
          this.draft.previewError = toProblemView(err);
          this.draft.mode = "off";
          this.draft.plan = null;
          this.draft.rows = null;
          this.draft.counts = null;
          this.statusLine = `Draft failed — ${problemLine(this.draft.previewError)}`;
          if (this.readingRestored) void this.reload();
          else this.renderAll();
        } finally {
          if (seq === this.draft.seq && this.draft.pendingScope && this.draft.mode !== "previewing") {
            this.draft.pendingScope = false;
            void this.preview();
          }
        }
      },
```

`draftDiff()` memoizes `diffRows(this.readingRows, this.draft.rows, { requestedAt: this.draft.requestedAt, scopeChannelId: this.draft.signature.channelId })` on the identity of `readingRows` and `draft.rows`. `confirmApply()` in full:

```ts
      async confirmApply() {
        const sig = this.draft.signature;
        const plan = this.draft.plan;
        if (!sig || !plan || this.draft.mode !== "armed") {
          this.cancelApply();
          return;
        }
        this.draft.mode = "applying";
        this.draft.applyError = null;
        this.draft.applyTimedOut = false;
        this.statusLine = applyingLine(planSlotCount(plan), planChannelCount(plan));
        try {
          const result = await apiSend<PlanResult>("POST", apiPath("/apply"), draftRequestBody(sig), LONG_SEND_TIMEOUT_MS);
          this.cancelApply(true);
          const slots = planSlotCount(result);
          const channels = planChannelCount(result);
          printTape(appliedTapeLine(slots, channels));
          this.statusLine = `Applied — ${plural(slots, "slot")} across ${plural(channels, "channel")}; re-reading the guide`;
          this.draft = emptyDraft(this.draft.seq);
          this.controls.channelId = "";
          this.closeInspector(false);
          focusArm();
          // The reading is always re-fetched after an apply: the server
          // now replays what it just committed, and a merge of the old
          // reading with the applied scope would carry two ages.
          void this.reload();
        } catch (err) {
          this.cancelApply(true);
          this.draft.mode = "armed";
          this.draft.applyError = toProblemView(err);
          this.draft.applyTimedOut = err instanceof ApiError && err.status === 0;
          this.draft.applyFailed = true;
          this.statusLine = `Apply failed — ${problemLine(this.draft.applyError)}`;
          focusArm();
        }
      },
```

- [ ] **Step 3: `index.html`**

Root: `<div x-data="guide" class="guide" @keydown.escape.window="if (!$refs.confirmDialog.open) closeInspector()">`.

Toolbar: change the SCOPE partial's `"change"` argument to `"requestDraft()"` and add after it:

```html
      <div class="form-field guide-toolbar__arm">
        <span class="form-field__label-spacer" aria-hidden="true"></span>
        <button
          type="button"
          class="btn btn--ghost"
          id="guide-arm"
          :disabled="draft.mode === 'previewing' || draft.mode === 'applying'"
          @click="requestDraft()"
          aria-describedby="guide-arm-hint"
        >Arm draft</button>
        <p class="form-field__hint" id="guide-arm-hint">Plan the next 7 days for this scope as a draft you can apply to Tunarr.</p>
      </div>
```

Directly under the toolbar (inside the top `.guide-chrome`, so it stays visible in every grid state):

```html
    <template x-if="draft.previewError">
      {{ partial "ui/problem.html" (dict "label" "Draft failed" "bind" "draft.previewError" "retry" "requestDraft()") }}
    </template>

    <div
      class="guide-draftbar"
      id="guide-draftbar"
      role="region"
      aria-label="Draft"
      :aria-busy="draft.mode === 'previewing' || draft.mode === 'applying'"
      x-show="draft.mode !== 'off'"
      x-cloak
    >
      <p class="guide-draftbar__line" x-text="draftBarLine()"></p>
      <div class="guide-draftbar__actions">
        <button type="button" class="btn btn--ghost btn--sm" :disabled="draft.mode === 'applying'" @click="discardDraft()">Discard</button>
        <button
          type="button"
          class="btn btn--primary btn--sm"
          :disabled="!canApply()"
          :aria-busy="draft.mode === 'applying'"
          @click="requestApply()"
          x-text="draft.mode === 'applying' ? 'Applying…' : 'Apply'"
        ></button>
      </div>
    </div>

    <template x-if="draft.applyError && draft.applyTimedOut">
      {{ partial "ui/problem.html" (dict "label" "Apply timed out — it may have partially landed" "bind" "draft.applyError" "retry" "requestDraft()" "retryLabel" "Re-draft") }}
    </template>
    <template x-if="draft.applyError && !draft.applyTimedOut">
      {{ partial "ui/problem.html" (dict "label" "Apply failed" "bind" "draft.applyError" "retry" "requestApply()" "retryDisabled" "!canApply()") }}
    </template>
```

`draftBarLine()` returns `draftingLine(scopeLabel())` while previewing, the `draftBarLine` readout while armed, and `applyingLine(...)` while applying.

Empty states: gate the two committed-mode teaching states on `draft.mode === 'off'` and drop the `plan &&` guard (the plan is null for a restored reading); add a draft-mode empty state:

```html
    <template x-if="!loading && draftOnGlass() && !hasAnySlots()">
      {{ partial "ui/empty.html" (dict
        "legend" "Nothing drafted"
        "text" "This draft schedules nothing for the scope. Applying pushes an empty lineup to any channel Schedularr last applied in it — discard, or review the blocks for this channel."
        "actionLabel" "Review blocks"
        "actionHref" "/blocks/") }}
    </template>
```

`.guide-body`'s `x-show` becomes `!loading && !problem && hasAnySlots()`.

Inspector: as the first child of `.panel__body`, `<p class="guide-inspector__verdict-line" x-show="inspector.slot.draft" x-text="inspectorVerdictLine()"></p>`.

Confirm dialog, before the root's closing `</div>`:

```html
  {{ partial "ui/confirm.html" (dict
    "title" "applyConfirmTitle()"
    "body" "applyConfirmBody()"
    "confirmText" "draft.mode === 'applying' ? 'Applying…' : 'Confirm Apply'"
    "confirm" "confirmApply()"
    "cancel" "cancelApply()"
    "busy" "draft.mode === 'applying'") }}
```

Intro copy: "The planned programme across every channel — what airs, what didn't, and why. Change the scope or arm a draft to preview the next 7 days on the grid, then apply it to Tunarr." Update the template's header comment.

- [ ] **Step 4: CSS**

After `.guide-droplegend`:

```css
/* The draft bar (spec §3.3): the armed control, sticky under the bezel
   (--bezel-h is published by runtime/shell.ts via CSSOM) so APPLY and
   DISCARD stay reachable while the page scrolls. The accent edge names
   "armed" the way the token dot's glow does; while drafting or applying
   ([aria-busy]) it quiets to the interactive border. */
.guide-draftbar {
  position: sticky;
  top: var(--bezel-h, 4.5rem);
  z-index: var(--z-sticky);
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-3);
  margin-bottom: var(--space-4);
  padding: var(--space-2) var(--space-3);
  border: var(--border-width) solid var(--color-accent);
  border-left-width: var(--border-width-thick);
  border-radius: var(--radius-sm);
  background: color-mix(in srgb, var(--color-accent) 8%, var(--color-bg-raised));
}

.guide-draftbar[aria-busy="true"] {
  border-color: var(--color-border-interactive);
}

.guide-draftbar__line {
  flex: 1 1 20rem;
  margin: 0;
  font-size: var(--text-xs);
  font-weight: 600;
  letter-spacing: var(--tracking-label);
  text-transform: uppercase;
  color: var(--color-ink);
  font-variant-numeric: tabular-nums;
}

.guide-draftbar__actions {
  display: flex;
  gap: var(--space-2);
}

/* The arming control sits beside SCOPE at the select's height; on mobile
   it is the one on-page draft entry (the SCOPE select hides there). */
.guide-toolbar__arm .form-field__label-spacer {
  display: block;
  height: 1.2em;
  margin-bottom: var(--space-1);
}

/* A draft on the glass adds the bar (and possibly a problem) above the
   grid: widen the viewport's chrome budget so its bottom stays on screen. */
.guide[data-draft] .guide-viewport {
  max-height: calc(100dvh - 20rem);
}
```

After the `.guide-slot--ghost` hover rule:

```css
/* -- draft verdicts (spec §3.3) ---------------------------------------------
   Text carries the fact (the NEW / CHANGED / REMOVED chip, SC 1.4.1); the
   accent left edge and the removed hatch are scan aids. Unchanged slots
   dim while a draft is on the glass so the diff is the content; beyond-
   horizon reading slots render plain. Precedence, later wins: removed >
   same > past. WCAG (computed, recorded in DESIGN.md): accent chip on
   bg-inset and on the series tint; danger chip and name on the removed
   hatch stripe and on surface-danger -- both palettes. */
.guide-slot__verdict {
  font-size: var(--text-xs);
  font-weight: 700;
  letter-spacing: var(--tracking-label);
  color: var(--color-accent);
  white-space: nowrap;
}

.guide-slot[data-draft="new"]:not([data-join="left"]):not([data-join="both"]),
.guide-slot[data-draft="changed"]:not([data-join="left"]):not([data-join="both"]) {
  border-left-width: var(--border-width-thick);
  border-left-color: var(--color-accent);
}

.guide-viewport[data-draft] .guide-slot[data-draft="same"],
.guide-viewport[data-draft] .guide-slot[data-draft="same"].is-past,
.guide-rundown[data-draft] .rundown-slot[data-draft="same"],
.guide-rundown[data-draft] .rundown-slot[data-draft="same"].is-past {
  opacity: 0.5;
}

/* Removed: the second lane (next to the ghosts), danger dashed edge, a
   reversed hatch, the name struck through. Never the on-air glow. */
.guide-slot[data-draft="removed"] {
  grid-row: 2;
  opacity: 1;
  border-color: var(--color-danger);
  border-style: dashed;
  color: var(--color-danger);
  box-shadow: none;
  background:
    repeating-linear-gradient(
      -45deg,
      color-mix(in srgb, var(--color-danger) 14%, transparent) 0 6px,
      transparent 6px 12px
    ),
    var(--surface-danger);
}

.rundown-slot[data-draft="removed"] {
  border-color: var(--color-danger);
  border-style: dashed;
  color: var(--color-danger);
  background:
    repeating-linear-gradient(
      -45deg,
      color-mix(in srgb, var(--color-danger) 14%, transparent) 0 6px,
      transparent 6px 12px
    ),
    var(--surface-danger);
}

[data-draft="removed"] .guide-slot__name {
  text-decoration: line-through;
}

[data-draft="removed"] .guide-slot__verdict,
[data-draft="removed"] .guide-slot__meta,
[data-draft="removed"] .guide-slot__prog,
[data-draft="removed"] .guide-slot__more {
  color: var(--color-danger);
}

/* Trace draw-in (motion inventory item 3): a new or changed slot draws
   in left-to-right ONLY when draft mode replaces a committed grid already
   on the glass -- never on an initial load. The settle is the committed
   sheet fading back after a discard. */
@keyframes guide-drawin {
  from {
    clip-path: inset(0 100% 0 0);
  }
  to {
    clip-path: inset(0);
  }
}

@keyframes guide-settle {
  from {
    opacity: 0;
  }
  to {
    opacity: 1;
  }
}

.guide-slot--drawin {
  animation: guide-drawin var(--duration-slow) var(--ease-out) both;
}

.guide-sheet--settle {
  animation: guide-settle var(--duration-slow) var(--ease-out) both;
}

@media (prefers-reduced-motion: reduce) {
  .guide-slot--drawin,
  .guide-sheet--settle {
    animation: none;
  }
}
```

In the existing `@media (forced-colors: active)` block add: `.guide-draftbar { border-color: Highlight; }`, `.guide-slot[data-draft="new"], .guide-slot[data-draft="changed"] { border-left-width: var(--border-width-thick); }`, `[data-draft="removed"] { border-style: dashed; }` (the line-through and chips already survive). Verify the removed hatch rule sits AFTER `.guide-slot.is-past`/`.is-on-air` in source order (it must, for the precedence stated in the comment). Compute the WCAG pairs named in the comment for both palettes (`assets/css/main.css` `:root`/dark tokens) and put the numbers in your report for Task 6.

- [ ] **Step 5: Type-check, unit tests, build**

Run: `make web-check && make web-test && make web`
Expected: clean; `web/public/index.html` contains `guide-draftbar` and `id="guide-arm"`.
No browser is available to you; state in the report which states you could and could not exercise and why.

- [ ] **Step 6: Commit**

```bash
git add web/assets/ts/pages/guide.ts web/layouts/index.html web/assets/css/main.css web/assets/ts/runtime/shell.ts
git commit -m "feat(web): draft & apply on the Guide — 7-day diff overlay, draft bar, armed-signature apply"
```

---

### Task 4: Blocks bridge, Schedule page deletion, nav, `clampDays`, stale comments

**Files:**
- Modify: `web/assets/ts/pages/blocks.ts` (~line 1138, the save tape line; imports)
- Delete: `web/layouts/schedule/list.html`, `web/assets/ts/pages/schedule.ts`, `web/content/schedule/_index.md`, `docs/assets/screenshots/schedule.png`, `assets/screenshots/schedule.png`
- Modify: `web/layouts/partials/nav.html`
- Modify: `web/assets/ts/runtime/format.ts` (delete `clampDays` + its doc comment), `web/tests/runtime.test.ts` (delete the two `clampDays` tests and its import)
- Modify: `web/assets/css/main.css` — delete ONLY the Schedule PAGE's rules: the `.schedule-warnings*` block (~lines 1097–1125, comment included) and the `.schedule-controls*` / `.schedule-status*` / `.schedule-channel` / `.schedule-programs*` block with its 640px media rule (~lines 1622–1712, comments included). KEEP the blocks editor's `.schedule-field*`, `.schedule-mode-toggle*`, `.schedule-picker*`, `.schedule-days*` rules (~lines 1784–1850). Update the comments at ~lines 411/445 of the table section that list "history/blocks/schedule/series".
- Modify: `web/layouts/kit/list.html` (delete the "Warning panel" section ~lines 316–327 that uses `.schedule-warnings`), `web/assets/ts/pages/kit.ts` (~line 265 comment: drop "schedule cancelApply")
- Modify: comments that mention the schedule page, `schedule.ts`, or a version number for a pending slice — `web/assets/ts/runtime/channels.ts` (~lines 5, 39, 96), `web/assets/ts/pages/series.ts` (~line 51), `web/assets/ts/runtime/format.ts` (~line 89), `web/assets/ts/runtime/shell.ts` (~line 14), `web/layouts/dashboard/list.html` (~lines 6, 13), `web/assets/css/main.css` (~line 388), `web/layouts/_default/baseof.html` (~line 73)

**Interfaces:**
- Consumes (Task 2): `draftHref` from `runtime/draft.ts`.

- [ ] **Step 1: The bridge**

In `blocks.ts`, import `draftHref` and replace the save tape line:

```ts
          // Success is printed, not toasted. The one action is the
          // create→apply bridge (spec §2 step 4): PREVIEW ON GUIDE opens
          // the guide in draft mode scoped to this block's channel, diffed
          // against the reading the operator last saw. The "reaches
          // Tunarr at <next_cron_tick>" readout arrives with the block
          // power tools slice.
          printTape(`Block saved — ${spec.name}`, {
            label: "Preview on guide",
            run: () => {
              window.location.href = draftHref(spec.channel_id);
            },
          });
```

- [ ] **Step 2: Delete the Schedule page and its remnants**

```bash
git rm web/layouts/schedule/list.html web/assets/ts/pages/schedule.ts web/content/schedule/_index.md docs/assets/screenshots/schedule.png assets/screenshots/schedule.png
```

`nav.html`: three items (`Guide`, `Blocks`, `Series`); replace the IA comment with:

```
{{- /* IA since the draft & apply slice: the Guide is home and owns
       preview/apply as its draft mode; the old dashboard survives on an
       unlinked /dashboard/ route until the Memory slice's /history/
       absorbs its table. Active state is server-rendered (aria-current,
       no JS) so it's correct even if a page's script fails to load. */ -}}
```

Delete `clampDays` and its tests/import; delete the two Schedule-page CSS blocks and the kit warnings fixture; rewrite the listed comments so forward references name the theme ("the Memory slice", "the SSE live-link slice", "the block power tools slice"), never a version number.

- [ ] **Step 3: Verify nothing references the dead page**

Run: `grep -rn "schedule/list\|pages/schedule\|clampDays\|schedule-controls\|schedule-table\|schedule-warnings\|schedule\.png" web docs README.md CLAUDE.md AGENTS.md .ai GEMINI.md mkdocs.yml | grep -v "docs/superpowers\|CHANGELOG"`
Expected: only doc prose lines that Task 6 rewrites (`docs/web-ui-guide.md`, `docs/api-reference.md`, `docs/scheduling-concepts.md`, `CLAUDE.md`, `AGENTS.md`, `.ai/AGENTS.md`, `web/DESIGN.md`). List them in your report.
Run: `grep -n "schedule-field\|schedule-picker\|schedule-days\|schedule-mode-toggle" web/layouts/blocks/list.html web/assets/css/main.css | wc -l` — must be > 0 on both (the blocks editor's picker survived).
Run: `make web-check && make web-test && make web`
Expected: clean; `web/public/schedule/` does not exist after the build.

- [ ] **Step 4: Commit**

```bash
git add -A web docs/assets assets/screenshots
git commit -m "feat(web): PREVIEW ON GUIDE bridge; delete the Schedule page, nav GUIDE · BLOCKS · SERIES"
```

---

### Task 5: `/kit/` gallery — draft states

**Files:**
- Modify: `web/layouts/kit/list.html` (a new section after "Guide week pager")
- Modify: `web/assets/ts/pages/kit.ts` (a second fixture band rendered with verdicts)

**Interfaces:**
- Consumes: `GuideSlot.draft`, `renderGuideWeek` with `drawIn: false` (Task 2); the draft bar markup from Task 3 (copied as static fixtures).

- [ ] **Step 1: Markup**

Add a section `kit-draft` titled "Guide draft mode" with one paragraph (what a verdict is; that text carries it; removed slots live in the second lane; `same` dims), three static draft bars — armed (`7-DAY DRAFT — 14 SLOTS ACROSS 3 CHANNELS · 2 NEW · 1 CHANGED · 1 REMOVED · 1 DROPPED · VS READING 21:02`, Discard/Apply enabled), drafting (`aria-busy="true"`, `DRAFTING — PLANNING ALL CHANNELS AGAINST TUNARR…`, Apply disabled), applying (`aria-busy="true"`, `APPLYING — 14 SLOTS ACROSS 3 CHANNELS…`, both disabled, Apply `aria-busy="true"`) — and a second viewport: `<div class="guide kit-guide" data-draft><div class="guide-viewport" id="kit-draft-viewport" data-draft aria-label="Fixture draft grid"></div></div>`.

- [ ] **Step 2: Fixture**

In `kit.ts`, add `fixtureDraftRows()` returning the guide fixture's first row with verdicts: Morning Creatures `same` (past), Matinee Massacre `changed`, Spooky Saturday Night `new`, a "Cancelled Cartoons" slot at now+5h for 60 min with `draft: "removed"`, the ghost untouched, and a `beyond` slot "Next Week Matinee" on day 2 of the band. Render it in `init()` next to the existing band with `{ onOpen: (slot) => printTape(\`Inspector would open — ${slot.blockName}\`), drawIn: false }`.

- [ ] **Step 3: Verify**

Run: `make web-check && hugo -s web -e development -d public-dev && grep -c "kit-draft-viewport" web/public-dev/kit/index.html && rm -rf web/public-dev`
Expected: tsc clean, count `1`.

- [ ] **Step 4: Commit**

```bash
git add web/layouts/kit/list.html web/assets/ts/pages/kit.ts
git commit -m "feat(web): /kit/ gallery — draft bar states and verdict slots"
```

---

### Task 6: Documentation, changelog, roadmap, spec amendment

**Files:**
- Modify: `docs/web-ui-guide.md` — the Guide section gains a "Draft & apply" subsection; the "Schedule (`/schedule/`)" section is deleted; the Toolbar bullet, the Blocks sentence "the same confirm idiom the Schedule page's Apply uses", and the Event tape paragraph are rewritten; the page-list sentences at the top
- Modify: `docs/api-reference.md` (~lines 51, 56: link targets → `web-ui-guide.md#the-guide`; "the web Guide's SCOPE control"; "the Guide's draft mode surfaces them as NO SIGNAL ghosts")
- Modify: `docs/scheduling-concepts.md` (~line 242 link target; add one sentence in the filter-block section: since v0.5.6 a filter occurrence's lineup is deterministic per (block, occurrence) so a preview equals the apply that follows it)
- Modify: `docs/index.md` (~lines 5, 26, 48), `README.md` (~line 17 alt text), `PRODUCT.md` (the "schedule preview + apply" capability line → "the Guide's draft & apply mode")
- Modify: `web/DESIGN.md` — "The guide grid" section gains a "Draft mode (v0.5.6)" bullet block (draft bar + `--bezel-h`, verdict chips and edges, lane-2 removed, dimmed same, beyond, draw-in/settle, the reading/sessionStorage mirror and its honesty boundary, the CSP note that `data-draft` is an attribute not a style); the WCAG evidence table gains the pairs Task 3 computed; the CSP page list (~line 849) drops `/schedule/`; the table mentions (~lines 411, 445, 510, 749) drop schedule
- Modify: `docs/superpowers/specs/2026-08-30-v0.5-web-overhaul-design.md` — §3.1 gains an amendment note (2026-09-08): the toolbar carries SCOPE and one `Arm draft` control (SCOPE hides on mobile, so the button is the on-page draft entry there; an explicit press also arms ALL channels from a fresh load); §3.3 gains a note that the draft window is 7 days inside the 28-day reading, with the retention reason; the §9 renumber block gains one line saying draft & apply shipped as v0.5.6
- Modify: `docs/roadmap.md` — the "Next — Draft & apply on the Guide: pending" bullet becomes `**v0.5.6 — Draft & apply on the Guide: SHIPPED (2026-09-08).**` with a summary in the style of the v0.5.3 entry (7-day diff overlay vs the timestamped reading, draft bar, armed-signature apply, PREVIEW ON GUIDE bridge, `?draft=` arrival with the mirrored reading and the last-apply staleness guard, deterministic filter lineups, serialized applies, Schedule page deleted, nav); the "Last shaped" line updates; the v0.3.0 section's "Seed-preserving" area gains nothing (do not invent)
- Modify: `TODO.md` — the ladder note gains one line (v0.5.6 shipped as draft & apply; memory/`/history/` is next); a new "Deferred (v0.5.6 draft & apply)" section: (a) the diff baseline is the operator's last reading, not Tunarr's lineup — the Memory slice's enriched history can make "removed" mean removed from Tunarr; (b) retention prunes occurrence snapshots and history by WRITE time, so any apply window wider than `history_retention` commits state the store forgets before it airs (the cron loop then re-plans day-of from an already-advanced cursor and skips episodes) — a 28-day apply needs pruning re-keyed on `occurrence_start` first; (c) the `VIEW RUN` tape action and the `reaches Tunarr at` bridge readout wait for `/history/` and the block power tools slice; (d) browser-clock skew can misclassify a slot that ended seconds before the draft as removed until the SSE heartbeat corrects the clock; (e) a removed slot and a ghost of a different block can overlap in lane 2; (f) the guide screenshot predates draft mode — re-capture with the demo assets
- Modify: `CHANGELOG.md` — new `## [0.5.6] - 2026-09-08` under `[Unreleased]` with `### Added` (draft mode on the Guide; PREVIEW ON GUIDE; `runtime/draft.ts` + tests; kit fixtures; `--bezel-h`), `### Changed` (SCOPE arms a draft instead of re-fetching; the reading is always ALL channels; `apiSend` timeout tier; nav; comments name themes not numbers), `### Fixed` (filter-block lineups deterministic per occurrence — a dry run and the apply that follows plan the same content; Runner serializes applies), `### Removed` (Schedule page, `schedule.ts`, `clampDays`, the page's styles, `schedule.png`, the kit warnings fixture), `### Documentation`; plus the compare link `[0.5.6]: https://github.com/christopherime/schedularr/compare/v0.5.5...v0.5.6`
- Modify: `CLAUDE.md` (the `web/` tree: drop `schedule/list.html` and `schedule.ts`, add `runtime/draft.ts`; the nav/pages line), `AGENTS.md` (the page list sentence), `.ai/AGENTS.md` (version line → `v0.5.6 (2026-09-08)`, the tree, the "Current State & Pending Work" list), `GEMINI.md` (only if it names the schedule page — verify; it does not today)
- Modify (outside the repo, operator-mandated by CLAUDE.md rule 3): `~/Documents/main/PERSO/k8s-home/GXF Schedularr & Tunarr.md` line 43's ladder sentence — append `**v0.5.6: draft & apply on the Guide** (SCOPE / Arm draft → a 7-day POST /generate diff overlay vs the last reading: NEW/CHANGED/REMOVED chips, sticky draft bar, confirm-gated apply, PREVIEW ON GUIDE from a block save; filter lineups deterministic per occurrence; Schedule page deleted, nav GUIDE·BLOCKS·SERIES)` and rewrite the trailing ladder in theme order (memory/`/history/` next, then SSE, block tools, history desk, polish — numbered at ship time). Do not touch the pinned-version line (the cluster bump is the operator's release step).

- [ ] **Step 1: Write the docs** — every file above, in one pass. The Guide section's new "Draft & apply" subsection must cover: the three entry points (SCOPE, Arm draft, PREVIEW ON GUIDE); reading / draft / verdict in one sentence each, with the honesty boundary stated plainly ("vs the reading taken at HH:MM", never Tunarr's lineup); the 7-day window and why; the draft bar line's exact format; what each verdict means, where removed slots render, what `beyond` is; DISCARD semantics (a restored or post-failure reading is re-fetched); the confirm dialog, the empty-draft wording, and the applied tape line; that the reading is re-fetched after an apply; mobile parity; keyboard reach (the bar sits in the tab order before the pager); what a preview/apply failure looks like, including the timeout wording. Name the three views the Schedule page had and where each answer now lives (per-channel window list → the inspector rundown per slot, and the Memory slice's `/history/`; DAYS control → gone by spec; warnings list → the ghost lane and the drop legend).

- [ ] **Step 2: Clean the prose** — invoke the `stop-slop` skill on every paragraph you wrote; apply its findings.

- [ ] **Step 3: Verify** — `grep -rn "Schedule page\|schedule-schedule\|/schedule/" docs README.md CLAUDE.md AGENTS.md .ai web/DESIGN.md PRODUCT.md | grep -v "docs/superpowers\|CHANGELOG\|api/v1/schedule"` → empty. `mkdocs build --strict` if mkdocs is installed (else say so). `make lint` → clean.

- [ ] **Step 4: Commit**

```bash
git add docs web/DESIGN.md README.md PRODUCT.md CLAUDE.md AGENTS.md .ai/AGENTS.md GEMINI.md TODO.md CHANGELOG.md
git commit -m "docs: v0.5.6 — draft & apply on the Guide; Schedule page prose removed; spec §3.1 amendment"
```

---

### Task 7: Integration verification (no new code)

**Files:** none modified unless a gate fails (then the fix goes in a `fix(...):` commit naming the failing command).

- [ ] **Step 1:** `make test` → all Go packages pass with `-race`.
- [ ] **Step 2:** `make lint` → golangci-lint, gosec, govulncheck, `web-check`, `web-test` all pass.
- [ ] **Step 3:** `make generate && git diff --exit-code -- internal/api/gen/` and `make web-types && git diff --exit-code -- web/assets/ts/gen/` → no drift.
- [ ] **Step 4:** `make web && make build`; write a temp config (copy `configs/config.yaml`, set `api.insecure_no_auth: true`, `database: /tmp/sdd-guide.db`, `tunarr.url: http://127.0.0.1:9`, `tunarr.api_key: x`, listen on `:18484`); start `./bin/schedularr serve --config <it>` in the background; verify with curl: `/` → 200 containing `guide-draftbar`, `id="guide-arm"`, `id="guide-scope"`; `/blocks/` → 200; `/series/` → 200; `/schedule/` → 404 and the body contains at least three `nav__link` anchors (the styled 404); `/kit/` → 404; `POST /api/v1/generate` with `{"days":7}` → 502 problem+json titled `schedule generation failed` (the draft error path's real payload); `GET /api/v1/schedule?days=28` → 502. Stop the server; delete the temp DB and config.
- [ ] **Step 5:** `git status` clean; `git log --oneline main..HEAD` lists the six feature commits.
- [ ] **Step 6:** Report the exact commands and outputs.
