# Project TODOs

## Library Adoption Analysis Summary

This document identifies opportunities to reduce custom code complexity through strategic adoption of well-maintained external libraries. All recommendations are refactoring-only - no behavior changes are proposed.

**Status: COMPLETED**

**Results:**
- 2 of 3 candidate areas successfully refactored
- Actual code reduction: ~90 lines (68 lines in cronbuilder + 42 lines in cache = 110 lines removed, ~20 lines added)
- Third task (map helpers) evaluated and determined not worth implementing

| Task | Priority | Status | Impact |
|------|----------|--------|--------|
| Cron description | High | Completed | -46 lines |
| Cache wrapper | Medium | Completed | -42 lines |
| Map helpers | Low | Not implemented | N/A |

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

## v0.5.7 polish intake

Punted from the v0.5.0 bench-rebuild review fix round (2026-08-30) — real
findings, deliberately deferred to the v0.5.7 polish slice rather than
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
