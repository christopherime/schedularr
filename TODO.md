# Project TODOs

## Ladder note (2026-08-30)

- **v0.5.5 became the token-at-rest security patch** (GitHub
  code-scanning alert #2, CodeQL `js/clear-text-storage-of-sensitive-data`
  — see CHANGELOG `[Unreleased]` → Security), so the Memory/`/history/`
  slice shifts again, to v0.5.7, with everything behind it moving one more
  number down. Numbers mark themes, not gates: a security patch is its own
  theme, never a rider on a feature slice. `docs/roadmap.md` carries the
  renumbered v0.5 train.
- **v0.5.6 shipped as draft & apply on the Guide** (2026-09-08): the
  Guide plans and applies, the Schedule page is gone, nav is
  `GUIDE · BLOCKS · SERIES`.
- **v0.5.7 shipped as Memory, landing at `/history/`** (2026-09-09):
  apply runs are persisted and readable, history rows name the apply that
  committed them, retention split per table, and one `/history/` page
  with TRACKED / AS-RUN / RUNS panes replaced `/series/` and
  `/dashboard/` outright. Nav is `GUIDE · BLOCKS · HISTORY`. Live link
  (SSE) is next in theme order — see `docs/roadmap.md`.
- **v0.5.8 shipped the live link's server half** (2026-09-10): the
  broadcast hub, `GET /api/v1/events`, the four change events, the
  reachability prober, and the `If-Match`/`PATCH` pair that closes the
  lost-update hole multi-writer visibility opens.
- **v0.5.9 shipped the client half** (2026-09-10): the fetch/ReadableStream
  reader and its ladder, the event bus with heartbeat-corrected
  `serverNow()`, the bezel's LINK legend, and per-page routing behind the
  two dirty guards. Block power tools are next in theme order.

## v1.0 product intake (2026-08-30)

Three operator-directed product streams, designed in
`docs/superpowers/specs/2026-08-30-v1-station-terminology-media-history-design.md`
and slotted into `docs/roadmap.md`. The Open Questions at the end of that
spec (Q1–Q11) were answered by the operator on 2026-09-08 (recorded in
its §5); nothing blocks these streams now.

- **Station lingo (→ v0.6.0).** "Block" validated against broadcast
  sources and kept; `type: "series"` → `"sequence"` (fixed); `type:
  "filter"` → `"selection"` (chosen at Q1); the criteria object renames
  with it, `filter:` → `criteria:` (Q3); UI and docs always say "sequence
  block" (Q11).
  Hard rename across OpenAPI + generated code, CUE, `scheduler.yaml`,
  `blocks.spec_json`, two DB tables, the web UI, and the docs — must land
  before the v0.9.0 freeze.
- **Any Tunarr media kind (→ v0.4.x, then v0.6.1).** Movies already reach
  the candidate pool; the gaps are the criteria — no media-kind field,
  `filter.tags` accepted but never evaluated (Q7: implement it next,
  ahead of enrichment), raw genres (Q6: enriched with raw fallback), no
  `/media/movies`. Converges with the v0.4 metadata theme (movie lookups,
  enrichment store, normalized genres/ratings, tags), with ordered movie
  sequences as a Sequence variant at v0.6.1.
- **`/series` → `/history` (→ v0.5.7, tools at v0.5.10).** One searchable
  page absorbing the planned `/log/`: TRACKED sequences, AS-RUN airings,
  apply RUNS. Adds remove-from-history and range cleanup, which need the
  plan-sequence floor moved into `app_meta` and a transactional delete
  path first — see the spec's I1–I5 invariant audit. Answered: scrub the
  title's key (Q5), refuse on-air deletions with a 409 (Q9), split
  retention per table (Q10).

## Library Adoption Analysis Summary

This document identifies opportunities to reduce custom code complexity through strategic adoption of well-maintained external libraries. All recommendations are refactoring-only - no behavior changes are proposed.

**Status: COMPLETED**

**Results:**

- 2 of 3 candidate areas successfully refactored
- Actual code reduction: ~90 lines (68 lines in cronbuilder + 42 lines in cache = 110 lines removed, ~20 lines added)
- Third task (map helpers) evaluated and determined not worth implementing

| Task             | Priority | Status          | Impact    |
| ---------------- | -------- | --------------- | --------- |
| Cron description | High     | Completed       | -46 lines |
| Cache wrapper    | Medium   | Completed       | -42 lines |
| Map helpers      | Low      | Not implemented | N/A       |

---

## Refactor: Cron Human-Readable Description Generation

**Priority**: High
**Status**: COMPLETED
**Actual Impact**: ~46 lines of code reduction (247 -> 203 lines)

### Current State

The `internal/cronbuilder/cronbuilder.go` file contains 246 lines of custom code for generating human-readable descriptions of cron expressions. The custom `Describe()` method and supporting functions (`describeMinute`, `describeHour`, `describeDayOfMonth`, `describeMonth`, `describeDayOfWeek`) implement basic cron-to-text conversion.

Current implementation limitations:

- Only handles simple patterns (wildcards, step values, single values)
- Limited range handling (only "1-5" for weekdays)
- No locale support
- Basic output format ("at minute X", "at hour Y")

### Proposed Change

Replace the custom `Describe()` method with `github.com/lnquy/cron` ([https://github.com/lnquy/cron](https://github.com/lnquy/cron)).

### Rationale

- **Battle-tested implementation**: Ported from cron-expression-descriptor (C#) via cRonstrue (JS), used across multiple language ecosystems
- **Comprehensive pattern support**: Handles all cron special characters (`* / , - ? L W #`) and 5/6/7-field expressions
- **Multi-locale support**: 26+ languages available (useful for future internationalization)
- **Natural language output**: Produces more readable descriptions like "Every 5 minutes" vs "every 5 minutes"
- **MIT licensed**: Compatible with project license

### Alternatives Considered

- **jsuar/go-cron-descriptor**: English-only, fewer GitHub stars (less community validation)
- **g-lib/cron-descriptor**: Less documentation, smaller community
- **Keep custom implementation**: Current code works but is limited and requires maintenance

### Affected Files

- `/Users/christophe/schedularr/internal/cronbuilder/cronbuilder.go` (lines 160-228 can be replaced)
- `/Users/christophe/schedularr/internal/cronbuilder/cronbuilder_test.go` (update tests for new library)

### Migration Notes

- The `lnquy/cron` library was last updated in November 2020 (v1.1.1) but is functionally complete and stable
- Keep the `Expression` struct and manipulation methods (`CycleNext`, `CyclePrev`, etc.) - only replace `Describe()`
- Library returns descriptions without leading capital; may need to capitalize first character for consistency
- Test edge cases: step expressions (`*/5`), ranges (`1-5`), and single values

### Code Example

```go
// Before (custom implementation)
func (e *Expression) Describe() string {
    parts := make([]string, 0, 5)
    parts = append(parts, describeMinute(e.Minute)...)
    // ... 70+ lines of custom logic
}

// After (using lnquy/cron)
import "github.com/lnquy/cron"

var cronDescriptor, _ = cron.NewDescriptor()

func (e *Expression) Describe() string {
    desc, err := cronDescriptor.ToDescription(e.String(), cron.Locale_en)
    if err != nil {
        return "invalid cron expression"
    }
    return desc
}
```

---

## Refactor: Simplify Cache Wrapper

**Priority**: Medium
**Status**: COMPLETED
**Actual Impact**: ~42 lines of code reduction (95 -> 53 lines)
**Estimated Impact**: ~40 lines of code reduction

### Current State

The `internal/cache/cache.go` file (95 lines) wraps `github.com/patrickmn/go-cache` but adds custom `copyValue()` logic (lines 59-88) to handle type assertions for cached values. This complexity exists because the wrapper tries to support generic type retrieval through an interface{} pointer.

The `copyValue()` function handles specific types (`[]interface{}`, `map[string]interface{}`, `string`, `int`) with type assertions, but the fallback case (line 87) always returns `true` regardless of whether the copy succeeded.

### Proposed Change

Simplify the cache wrapper by removing the `copyValue()` function and using `go-cache`'s native `Get()` return value directly. The calling code should handle type assertions.

This is not a library replacement but rather a simplification of how the existing library is used.

### Rationale

- **Reduces complexity**: The current `copyValue()` function doesn't properly handle all types and has unclear semantics
- **Clearer API**: Callers can use type assertions directly, which is the idiomatic Go pattern
- **Better error handling**: Current implementation silently fails on type mismatches
- **Less maintenance**: Removes custom type-handling code that's prone to edge cases

### Alternatives Considered

- **Replace with bluele/gcache**: Provides generics support, but `go-cache` is sufficient and already a dependency
- **Use otter or other modern caches**: Overkill for current usage; `go-cache` has been stable since 2014
- **Keep current implementation**: Works but adds unnecessary complexity

### Affected Files

- `/Users/christophe/schedularr/internal/cache/cache.go` (simplify `Get` method, remove `copyValue`)
- `/Users/christophe/schedularr/internal/cache/cache_test.go` (update tests for simplified API)
- Callers of cache (likely in external API clients) - need to handle type assertions

### Migration Notes

- Audit all cache callers to ensure they handle type assertions properly
- Consider using Go generics (`Cache[T]`) if Go 1.18+ is the minimum version
- The simplified API would return `(interface{}, bool)` like native `go-cache`
- Update tests to verify type assertion behavior

### Code Example

```go
// Before (complex wrapper)
func (c *Cache) Get(key string, v interface{}) (bool, error) {
    data, found := c.store.Get(key)
    if !found {
        return false, nil
    }
    // ... 30+ lines of type handling
    return copyValue(data, v)
}

// After (simplified)
func (c *Cache) Get(key string) (interface{}, bool) {
    return c.store.Get(key)
}

// Caller handles type assertion
if data, found := cache.Get("key"); found {
    if channels, ok := data.([]Channel); ok {
        // use channels
    }
}
```

---

## ~~Refactor: Extract Map Helper Functions to samber/lo~~ (Not Implemented)

**Priority**: Low
**Status**: Evaluated - Not worth implementing
**Estimated Impact**: ~~~35 lines of code reduction~~ Minimal

### Analysis Result

After evaluation, this refactoring was **not implemented** because:

1. **`samber/lo` doesn't have direct utilities for map type assertions**: The library is designed for slice operations (`Filter`, `Map`, `ContainsBy`, etc.), not for extracting typed values from `map[string]any`.

2. **`lo.ValueOr()` doesn't apply here**: This function is for pointer dereferencing with defaults, not for map access with type assertions. The type assertion must happen first.

3. **Current code is idiomatic and clear**: The helper functions are already concise (6-10 lines each) and follow standard Go patterns for type assertions.

4. **No consistency benefit**: `lo` is only used in `internal/scheduler/filter.go` for slice operations, not for map access patterns.

5. **Actual code example shows no `lo` usage**: The proposed "after" code in the original TODO doesn't actually use any `lo` functions - it just inlines the type assertion.

### What Would Actually Work

If code reduction is desired, consider:

- Using generics to create type-safe map accessors (Go 1.18+)
- Using `mapstructure` library for full struct unmarshaling
- Keeping the current implementation (recommended - it's already clean)

### Original Rationale (Superseded)

- ~~**Already a dependency**: `samber/lo` is already used in `internal/scheduler/filter.go`~~
- ~~**Type-safe**: Generic functions provide compile-time type checking~~
- ~~**Comprehensive utilities**: Includes `lo.ValueOr`, `lo.CoalesceOrEmpty`, map utilities~~

---

## Not Recommended for Library Replacement

### HTTP Client (`internal/httpclient/`)

**Reason**: Already uses `go-resty/resty/v2` which is well-maintained. The wrapper (208 lines) provides project-specific authentication handling and error types. This is appropriate custom code.

### SQLite Store (`internal/store/`)

**Reason**: Already uses `jmoiron/sqlx` and `golang-migrate/migrate`. The custom code (264 lines in `sqlite.go`) is domain-specific persistence logic that cannot be replaced by a library.

### Scheduler Filter (`internal/scheduler/filter.go`)

**Reason**: Already uses `samber/lo` for functional operations. The filtering logic (83 lines) is business logic specific to the application's needs.

### Schedule History (`internal/scheduler/history.go`)

**Reason**: Simple in-memory tracking (80 lines) with domain-specific logic. A generic cache library would not simplify this code.

---

## Implementation Checklist

### Completed Tasks

- [x] **Cron Description (High Priority)**: Replaced custom `Describe()` implementation with `github.com/lnquy/cron`
  - Reduced ~68 lines to ~22 lines
  - Improved description quality (natural language like "at 06:00 AM" vs "at minute 0, at hour 6")
  - Added library to depguard allowlist
  - Updated tests with new expected output

- [x] **Cache Wrapper (Medium Priority)**: Simplified `Get()` API by removing `copyValue()` complexity
  - Reduced 95 lines to 53 lines
  - Changed API from `Get(key, &target)` to `Get(key) (any, bool)`
  - Callers now use explicit type assertions (idiomatic Go)
  - Extracted cache loading helpers in callers to reduce nesting

- [x] **Map Helpers (Low Priority)**: Evaluated and determined NOT worth implementing
  - `samber/lo` doesn't have utilities for map type assertions
  - Current code is already idiomatic and concise
  - See analysis above for details

### Implementation Guidelines

When implementing future refactoring tasks:

- [x] Each change preserves exact behavior (write characterization tests first)
- [x] All suggested libraries have been verified for recent maintenance activity
- [x] No security advisories exist for recommended library versions
- [x] Update go.mod with any new dependencies
- [x] Run full test suite after each change
- [ ] Update CLAUDE.md if any architectural patterns change (N/A for these changes)

---

## Deferred (API server core close-out)

Recorded at the end of the API-server-core final fix wave
(`.superpowers/sdd/2026-08-28-api-server-core/`) so these don't get lost.
None block the current close-out; each is a known, scoped-out gap.

- **series_state channel-scoping.** `service.Runner.Run`'s `ChannelID`
  filtering scopes *blocks* (and therefore series-cursor advances) to the
  requested channel (see its doc comment in `internal/service/schedule.go`
  and `TestRunner_Run_ChannelScopedApply_LeavesOtherChannelStateUntouched`
  in `internal/service/schedule_test.go`), but `series_state` rows
  themselves are keyed only by `show_title` -- not `(channel_id,
  show_title)`. Two blocks on different channels tracking the same show
  title would collide on the same series-cursor row today. Revisit in
  sub-project 3.

- **kin-openapi unreached vulns.** `github.com/getkin/kin-openapi` (used
  by `internal/cueconfig`/`internal/api/gen` generation tooling, not at
  request-serving runtime) has had `govulncheck`-flagged advisories in its
  `openapi3filter` subpackage in the past; this project's code doesn't
  reach that subpackage, so `govulncheck`'s reachability analysis reports
  them as non-blocking. Monitor upstream and re-check
  `govulncheck ./...` output on every `kin-openapi` bump in case a future
  advisory does land in a reachable path.

## Deferred (Web UI sub-project close-out)

Recorded at the end of the web-UI final fix wave
(`.superpowers/sdd/2026-08-28-web-ui/`, FINAL whole-branch review) so
these don't get lost. None block the current close-out; each is a known,
scoped-out gap.

- **Spec deviation, accepted permanently: `make build` depends on a
  web-presence check, not the web build itself.** Spec Decision 4 (`docs/
  superpowers/specs/2026-08-28-web-ui-design.md`) describes `make build`
  as depending on the web build; the shipped Makefile instead makes
  `build` depend on `web-presence`, which writes a one-line placeholder
  on demand when `web/public/index.html` doesn't exist yet (untracked as
  of Task 1, `docs/superpowers/plans/2026-08-29-deploy.md` -- no
  committed blob involved), not on `web-build` itself running Hugo. This
  stays accepted: it keeps `go build`/`make build` working on a machine
  without Hugo installed, which the placeholder mechanism exists to
  guarantee in the first place -- forcing a real Hugo build on every
  `make build` would defeat that. The release-safety guarantee (a
  shipped binary never embeds the placeholder) is the Docker image's job
  instead: `Dockerfile`'s Hugo stage always runs the real `hugo --minify
  -s web` before the Go build stage, so a container image can never ship
  the placeholder as its embedded site.
- [ ] Transactional `Engine.Commit`: today it is a sequence of independent sqlite statements (UpdateSeriesState per show, RecordScheduleHistory, SaveOccurrenceSnapshot, ReplaceOccurrenceHistory, cleanups) — a SIGKILL (or the drain-timeout os.Exit) mid-sequence can persist an advanced cursor without its snapshot/history rows. Root fix: one transaction. (v0.3.0 review, MAJOR-class root cause; the drain path now exits without closing the store, which narrows but does not close the window.)
- [ ] `syncPostStates` stamps shows an occurrence never actually reached (post==pre rides through and gets LastAired/CursorPlanSeq written); newly visible for a series added to a full block — its `start_season`/`start_episode` is then silently never applied (initializeSeriesState early-returns once LastAired is set). Fix candidate: skip post==pre entries during sync. (v0.3.0 review, Minor.)
- [ ] Residue from v0.2.2 final gate, last open item: (a) backward-CAS coincidence — a wrap-lap landing exactly on a slow shared-show block's frozen baseline lands a partial-lap rewind (self-healing; needs restart + shared show_title + far-future occurrence). Address with the v0.3.x seed-machinery slice. ((b) contradictory on_complete now REJECTED at write time; (c) provenance stamp direction now test-pinned — both shipped in the v0.3.0 slice.)
- [ ] Overflow slots desync per-program `start_time` from actual Tunarr playback of the following slot (v0.5.1 review, contract lens). Block A (21:00, duration 60, `max_duration_overflow_minutes` 15) fills 70 min of content; the engine's shell still sets `EndTime = 22:00` (`internal/scheduler/engine.go` shell construction ignores the overflow in both the filter and series fill loops). `buildAnchoredLineup`'s `gap > 0` guards then skip both the end-of-slot pad and the next slot's gap, so adjacent block B's programs are appended at cumulative offset 22:10 and Tunarr plays them 10 minutes late — while the wire's per-program `start_time` (computed from B's nominal `StartTime` in `slotToGen`, `internal/api/schedule.go`) says 22:00. The guide renders air times up to the configured overflow early for every slot after an overflowing one until a real gap absorbs the shift. Pre-existing at slot level; the per-program field inherits and amplifies it. Root fix is engine-side: the shell `EndTime` must account for `max_duration_overflow_minutes` actually consumed.

## Deferred (v0.5.5 token-encryption review — APPROVED, fast-follows)

- Multi-tab first-hydration race: no cross-tab lock around key generation + migration; two tabs upgrading simultaneously can leave localStorage ciphertext under a key IndexedDB no longer holds → silent token loss, re-auth needed (no security exposure). Candidate: navigator.locks around obtainKey()+migration.
- Manual token-panel trigger click isn't gated on loadToken(): a click in the pre-hydration millisecond window shows an empty input despite a stored token (auto-open path IS gated).
- indexedDB.open has no onblocked handler/timeout; harmless at fixed version 1, but every request awaits this promise — guard before any future schema bump.
- clearToken() nulls the cache before the fallible removeItem — a failed remove shows "could not clear" while the session is already de-armed (cosmetic).

## Deferred (v0.5.3 week-grid review — all non-blocking, APPROVED)

- Silent cross-midnight slot pieces are browse-mode-discoverable buttons whose aria-label repeats the full base label + "continues across midnight" — trim to a short cross-reference or aria-hidden the silent piece (grid.ts buildSlotPiece / main.css .guide-slot--silent).
- Pre-existing: a 48h+ block's fully-transited middle day shows a rundown continuation label "…until HH:MM" with no date, implying same-day end (grid.ts rundownDaySlots). Block duration is UNBOUNDED (CUE has no ceiling; engine doesn't clamp) — the grid handles N-day slots correctly, the rundown label doesn't.
- No test exercises a 3+ segment (N>2 day) join/rundown case — add one alongside the label fix.
- Cold-pod 90s /schedule fetch straddling local midnight can silently drop a sub-90s programming sliver at the window start (guide.ts landedAt anchor) — negligible probability; comment + fix candidate: anchor on request-send time.

## Deferred (v0.5.7 memory / history)

Recorded at the close of the Memory slice (2026-09-09). Nothing here
blocks the release; each item names the gap and why it was left.

- **Apply-run recording is best-effort, not transactional with the
  apply.** `Runner.startApplyRun`/`finishApplyRun` log and continue when
  the store write fails, so a database problem produces a reporting gap
  rather than a refused apply. That is the right trade — refusing to
  schedule because the reporting table is unhappy would be worse — but it
  means the RUNS pane is not a guaranteed-complete ledger.
- **A run left at `running` is never reaped.** A process killed
  mid-apply leaves a row whose `finished_at` never arrives; the page
  reports it as in flight and the docs say what that means. A startup
  sweep that marks orphaned `running` rows as interrupted would be
  honest, but needs a way to tell "this process's own in-flight run" from
  "a previous process's corpse" — deferred rather than guessed at.
- **`GET /applies` has no cursor.** It takes `days` and `limit` and
  returns one page; at the 90-day default and a 6h cron cadence that is
  ~360 runs, comfortably inside the 500 cap. A busier deployment would
  silently see only the newest 500. Add keyset pagination when a real
  deployment reaches it.
- **The AS-RUN pane filters client-side.** Channel, block, and title
  narrow rows already fetched for the window, so a 90-day window loads
  every row before filtering. Server-side filter parameters on `GET
  /history` are the fix if that window ever gets large.
- **A history row carries no season/episode.** The wire has `title` and
  `type` but not the cursor position, so an as-run row cannot show
  `SxxEyy` without inventing it. Deliberately not shown; add the fields
  to `schedule_history` if the operator wants the marker.
- **The as-run block link goes to `/blocks/`, not to the block.** The
  blocks page has no per-row anchor yet — the same limitation the guide
  inspector's link already carries.
- **The mobile TRACKED table still scrolls horizontally.** Inherited
  unchanged from `/series/`: `.table-wrap` scrolls a six-column table
  inside a 390px viewport. A card layout below the table breakpoint is a
  polish-pass item, not a memory-slice one.
- **`gosec` cannot run in the current toolchain.** It fails with
  `internal error: package "strings" without types was imported from
  "command-line-arguments"` against Go 1.27 — on a clean tree as well, so
  it predates this slice. `make lint` reports it as a failed step;
  golangci-lint, govulncheck, web-check and web-test all pass. Needs a
  gosec upgrade.

## Deferred (v0.5.6 draft & apply)

Recorded at the close of the draft & apply slice (2026-09-08). Nothing
here blocks the release; each item names the gap and why it was left.

- **The diff baseline is the operator's last reading, not Tunarr's
  lineup.** `REMOVED` means "in the reading taken at HH:MM, not in this
  draft" — the bar, the confirm dialog, and the inspector all word it
  that way, because nothing on the client can see what Tunarr holds. The
  Memory slice's enriched history is what can make "removed" mean removed
  from Tunarr.
- **Retention prunes by WRITE time, so the apply window is capped at 7
  days.** `Engine.Commit` prunes occurrence snapshots and
  `schedule_history` by write time on every commit
  (`maintenance.history_retention`), so any apply window wider than that
  retention commits state the store forgets before it airs — the cron
  loop then re-plans day-of from an already-advanced cursor and skips
  episodes. Widening past 7 days (a 28-day apply matching the reading)
  needs pruning re-keyed on `occurrence_start` first.
- **The tape's `VIEW RUN` action and the bridge's `reaches Tunarr at`
  readout are unbuilt.** Both need surfaces that do not exist yet: the
  run link needs `/history/` (Memory), and the next-tick readout on the
  block-save bridge belongs with the block power tools slice.
- **Browser-clock skew can misclassify a slot that just ended.** The
  diff's cutoff is `Date.now()` at request time, so a slot that ended
  seconds before the draft was asked for reads as `REMOVED` on a
  browser whose clock runs behind the server's. The SSE heartbeat's
  skew correction fixes it for every clock-dependent surface at once.
- **A removed slot and a ghost of a different block can overlap in lane
  2.** Both render in the second lane at their own times; nothing
  reserves separate rows. Visible only when a dropped occurrence and a
  removed occurrence share a window on one channel.
- **The guide screenshot predates draft mode.**
  `docs/assets/screenshots/guide.png` shows committed mode only —
  re-capture it with the demo assets, with a draft armed.
- **`guide.ts` is ~1200 lines and wants a split.** Draft controller
  versus rendering is the seam; do it before the SSE slice adds a live
  layer to the same file.
- **The draft state machine has no DOM-level coverage.**
  `web/tests/guide.test.ts` drives the component through a Node harness
  (transitions, latches, focus counts, the fetch log), but `x-show`,
  real focus, and the sticky geometry need a browser harness the project
  does not have.
- **The Runner's apply mutex ignores `ctx`.** `service.Runner.applyMu`
  serializes applies, but a queued apply whose client already timed out
  still runs to completion. A ctx-aware semaphore (acquire or fail on
  `ctx.Done()`) is the follow-up; the plain mutex was the shape the plan
  mandated.
- **A pinned inspector can end up under the sticky draft bar.**
  `.guide-draftzone` sticks at `--z-sticky`; `.guide-inspector` sticks at
  `top: 4.5rem` with no `z-index` of its own, so on desktop widths
  narrower than about `--content-max` + the inspector's 24rem rail the
  bar can paint over the inspector's heading. Unconfirmed without a
  browser: the inspector rarely reaches its sticky threshold, because the
  draft viewport caps at `100dvh − 20rem`. The fix candidate is a top
  offset below the bar on `.guide[data-draft] .guide-inspector`, and it
  waits on a browser check rather than a guessed number.
- **A draft slot crossing the 7-day horizon can overlap a `beyond`
  reading slot in lane 1.** A drafted occurrence that starts inside the
  horizon and runs past it shares grid space with the reading slot that
  starts after it — a real future conflict the diff cannot express,
  since the draft never planned past the horizon to compare against.

- **Arm-press focus handoff fires for pointer presses too** (final
  re-review, minor). Chromium/Edge focus a `<button>` on mousedown, so the
  keyboard-oriented handoff in `guide.ts`'s `preview()` also moves a mouse
  user's focus when the Arm button disables; and a FAILED preview undoes
  the handoff (mode flips to `off`, the bar hides, focus lands on body).
  Fix candidate: gate the handoff on `:focus-visible` / a keyboard flag,
  and re-run `focusArmSoon()` on the preview failure path.
- **One vacuous assertion in `web/tests/guide.test.ts`** (~line 587): the
  `sessionStore.get(READING_STORAGE_KEY) === undefined` check passes with
  or without `dropStoredReading()` because the harness never seeds the
  store — seed it, then assert the drop.
- **`/kit/` fixture comment names the wrong order** for the three draft
  problem blocks (the live guide renders `Draft failed` above the bar and
  the two apply problems below it).
- **Filter-block planning is window-independent but still block-order
  dependent inside one run** (final fix wave, `b7a4e36`): the in-run
  recency check now counts only occurrences that air earlier, so a 7-day
  draft and a 28-day reading agree; but blocks are planned in name order,
  so the first block's mid-window occurrence never sees the second block's
  earlier ones. Stable across runs (the Guide's diff is sound); a fully
  order-free plan needs occurrences planned in global chronological order
  across blocks — a larger engine change.
- **`internal/store/sqlite_test.go` fails `gofmt -l`** (pre-existing on
  main; `make lint` passes regardless) — one-line `gofmt -w` commit.

## Deferred (v0.5.1 Guide review)

Recorded from the v0.5.1 Guide fix round (2026-08-30). Neither blocks
the slice; both are known, scoped-out gaps.

- **Season 0 (specials) is omitted from the wire, indistinguishable
  from "not an episode".** `programToGen` (`internal/api/schedule.go`)
  only emits `season` when `SeasonNumber > 0`, but Plex/Jellyfin
  specials live in season 0 — a scheduled special airs on the wire as
  `{type: "episode", episode: 5}` with no season, so the guide shows
  `E5` with no season marker. Mitigating: `SeasonNumber == 0` also
  means "not yet hydrated via SeasonID" (`internal/external/tunarr/
  models.go`), so 0 is an ambiguous sentinel internally and omission is
  the defensible projection; the wire just cannot distinguish a special
  from an unhydrated episode. Revisit if/when hydration makes 0
  unambiguous.
- **DST-day slot positions disagree with the ruler's hour labels.**
  `daySpan` (`web/assets/ts/runtime/grid.ts`) places slots by real
  elapsed minutes since local midnight while `buildRuler` labels the
  288-quantum track as fixed wall-clock hours: on the spring-forward
  day a 12:00 slot renders under the 11:00 cell; the fall-back day
  compresses an unlabeled extra hour (the now-line is clamped to the
  track since this round). Two days a year, read-only surface —
  documented at the `daySpan` doc comment; fix would be
  ruler-follows-offset rendering.

## v0.5.9 polish intake

Punted from the v0.5.0 bench-rebuild review fix round (2026-08-30) — real
findings, deliberately deferred to the v0.5.9 polish slice (the ladder
shifted twice on 2026-08-30) rather than
patched piecemeal mid-rebuild.

- [ ] **`aria-busy` on loading sections + skeleton announcement.** Every
  skeleton is `aria-hidden="true"` and no section carries `aria-busy`
  while loading, so a screen-reader user gets silence during load and no
  announcement when content arrives. Mark the loading section
  `aria-busy="true"` (cleared when content lands) and add a polite
  announcement for the arrival.
- [ ] **Toggle accessible name / row association.** `ui/toggle.html` puts
  the state label inside the `role="switch"` button, so it announces as
  "Completed, switch, on" — the accessible name changes with the value —
  and nothing associates the switch with its row (the show-title cell is
  a `<td>`, not `<th scope="row">`). Same shape on the blocks table.
- [ ] **Per-row expression evaluation costs.** `ui/problem.html` inlines
  its `.bind` expression three times, `ui/plate.html` twice,
  `ui/channel-select.html` calls `channelHint()` twice, and
  `blocks/list.html` runs `cronReadback()` twice per row (2N cronstrue
  parses per list render). Not a bug today; audit before the grid slice
  multiplies row counts.

## Deferred (live link)

Recorded when v0.5.9 closed the slice. None of these block anything; each
is a known, scoped-out gap.

- **The trace draw-in for newly arrived `/history/` rows.** Task 11 asked
  for it; the draw-in turned out to be guide-only (`@keyframes
  guide-drawin`), and giving History its own would mean designing the
  motion rather than reusing it. The event tape's own print animation
  marks arrival in the meantime.
- **The live refetch does not draw in on the Guide either.** The draw-in
  is structurally reachable only from a draft preview, not from a
  committed reading, so the Task 10 bullet asking for it describes
  something the grid cannot currently express.
- **A throwing subscriber kills its siblings.** Both switchboards —
  `bus.publishLocal` and shell's resume-handler loop — dispatch with a
  bare `for (const h of [...set]) h(data)`. Guard both at once or
  neither; guarding one would make them disagree about whether a handler
  may throw.
- **A hidden tab reports `linkState() === "live"`** while its stream is
  stopped, so the 60s poll stays suspended for a tab that is not
  streaming. Harmless — nobody is reading a hidden tab, and the resume
  path refetches everything — but the reading is not strictly honest.
- **No automated coverage of the full backoff walk** (LIVE → POLL →
  LINK LOST). It needs 7+ seconds of real backoff and the ladder has no
  injectable clock. Covered by the gate's by-hand browser pass.

## Deferred (block power tools)

Recorded when v0.5.10 closed the slice. Everything here was either scoped
out before implementation started or is a known gap in what shipped; none
of it blocks anything.

- **Recurring dark windows** ("dark every August"). One instant, one
  wake-up. A recurrence rule on `disabled_until` would be a second
  scheduling language competing with cron, and every question cron
  already answers badly (DST, month lengths, descriptors) would have to
  be answered again in a different vocabulary.
- **Bulk operations on blocks** (multi-select, bulk enable/disable, a
  bulk dark window). That is the history desk slice's shape applied to a
  different noun, and it wants the same selection model.
- **`disabled_until` on series rows or channels.** Blocks only. A dark
  channel is a different gate in a different place, and nothing asked for
  one.
- **Case-insensitive block-name collisions.** Today's SQLite `BINARY
  UNIQUE` stands, so `Morning Cartoons` and `morning cartoons` coexist
  and a duplicate can collide with neither. `COLLATE NOCASE` is a
  migration plus a decision about what to do with existing rows that
  differ only in case.
- **`disabled_until` does not round-trip through `scheduler.yaml`.** It
  is a column rather than a `BlockSpec` field, and that file is a
  first-run import format for specs; a dark window is operational state.
  A restored import therefore comes back awake. Deliberate, and the one
  visible cost of decision 1 — but it is a cost, so it is written down
  here rather than only in the plan.
- **Nothing sweeps a stale `disabled_until`.** A wake instant in the past
  stays in the row forever. Every reader compares against the clock so it
  suppresses nothing and paints nothing, but the value is still there for
  anyone reading the database directly, and `PATCH ... null` is the only
  thing that removes it.
- **`GET /cron/next` returns start instants only.** The editor's rail
  adds each block's own duration to get the end times it prints. A block
  is the only thing that knows its duration, so an endpoint that returned
  ends would have to be told one.
- **The save tape line still has no "reaches Tunarr at `<next_cron_tick>`"
  readout.** `GET /status` already carries `next_cron_tick` and
  `runtime/shell.ts` already renders it in the bezel, so this is one line
  in the editor's save path — and the comment above `printTape` in
  `pages/blocks.ts` still says it "arrives with the block power tools
  slice", which is now wrong. Fix the comment with the line.
- **`next_occurrence` parses one cron expression per record on every `GET
  /blocks`.** At tens of blocks that is nothing, and caching it would
  mean inventing an invalidation rule for a value that changes with the
  clock. If the list ever gets slow the fix is one batched computation
  over the whole list, not a cache.
- **A cursor rewind across several shows is not atomic.** The confirm is
  per-block and `PATCH /state/series/{show_title}` is per-show, so a
  block seeding three shows issues three independent writes and some can
  fail while others land. The tape reports per-title outcomes and offers
  the tracked view rather than claiming a single success, which is the
  honest reading, not a fix.

## Deferred (history desk foundations)

Recorded when v0.5.11 shipped the safety work. None blocks the desk UI;
each is a known gap with its reason.

- **Airings predating migration `000012` cannot be removed by title.**
  They carry an empty `show_title`, and recovering a show from a stored
  program id needs the live Tunarr catalogue — which may no longer carry
  that program, and guessing from the block that aired it is wrong for any
  block scheduling more than one show. They still age out through
  retention.
- **`Engine.Commit` is still not transactional** (open since v0.3.0,
  recorded above). `RemoveShow` is transactional on its own terms, so the
  desk does not need this — but a crash mid-`Commit` still leaves the same
  partial state it always has.
- **Range predicates on stored instants are not normalised.** The
  exact-match lookups were fixed; ranges were left alone because the date
  prefix dominates the bytewise comparison for same-zone data, and a test
  pins that retention prunes by instant. Range cleanup hands the operator
  an arbitrary cutoff over data that may span a `log.timezone` change,
  which is where that stops holding — fix it there, with the test that is
  already in place.
- **No endpoint calls `store.RemoveShow`.** Deliberate: the primitive and
  its tests ship here, the endpoint ships with the UI that drives it.
- **The removal is a two-step flow.** It refuses while any block still
  lists the show, so the operator edits those blocks first. The error names
  them; the desk UI should surface that first step rather than letting them
  hit the 409 cold.
