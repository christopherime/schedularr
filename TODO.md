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

- **Transactional batch import.** `internal/api/importexport.go`'s
  `ImportBlocks` inserts blocks one at a time (via `store.CreateBlock` in a
  loop, not a single DB transaction), so a name collision or store error
  partway through a batch import leaves the earlier blocks in that same
  request already persisted -- there is no rollback. `ImportBlocks`'s own
  doc comment already flags this ("A batch transaction that could roll
  back a partial write on such a race stays out of scope"). Fix: wrap the
  insert loop in a single `*sql.Tx` (mirroring `RecordScheduleHistory`'s
  and `ImportSeriesStates`'s existing transactional pattern in
  `internal/store/sqlite.go`) so an import either fully succeeds or fully
  rolls back.

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

- **`${VAR}`-unset-\>null config bug.** An unset `${VAR}` placeholder left
  in a config file expands to an empty, unquoted YAML scalar, which YAML
  parses as `null` rather than `""` -- documented as a footgun in
  README.md's Quick Start ("Environment Variable Interpolation") section,
  worked around today by always quoting interpolated values explicitly.
  The root fix belongs in `internal/cueconfig`'s `${VAR}` interpolation
  step (`internal/cueconfig/schema.go`): substitute an unset variable with
  an explicitly-quoted empty string instead of a bare empty token, so the
  YAML never round-trips through `null`.

- **YAML `type:`-omission CUE bug.** Documented in README.md's Scheduling
  Blocks section: `scheduler.yaml`'s block `type` field has a CUE default
  (`"filter" | "series" | *"filter"`, `cmd/schema/scheduler.cue`), but the
  import path decodes each block into a Go struct *before* CUE-validating
  it, which turns an omitted `type` into an explicit `""` rather than a
  genuinely absent field -- CUE only applies `*default` to an absent
  field, so the empty string fails the disjunction check. Workaround
  today: always set `type` explicitly in `scheduler.yaml` (the JSON
  `POST /api/v1/blocks` path doesn't have this problem -- see
  `internal/api/blocks.go`'s `fromGen`). Fix: give `internal/blockio` a
  raw-bytes validation path (validate the YAML bytes against CUE directly,
  before decoding into the Go struct) plus `omitempty` tags on the
  decode-target struct's optional fields, so an omitted field stays
  genuinely absent through to CUE.

- **Duplicate `generate config` command.** `cmd/config.go`'s
  `config generate` and `cmd/generate.go`'s `generate config` are the same
  operation (`cueconfig.NewValidator().GenerateConfigWithOverrides`)
  reachable two ways; README.md's Available Commands table documents both
  as intentional aliases today. Fast follow: delete one (`generate
  config`, the less discoverable of the two nested under an unrelated
  verb) and point users at `config generate`.

- **`/status` probe timeout.** `GetStatus` (`internal/api/tunarr.go`)
  probes Tunarr liveness via a live `GetChannels` call with no
  request-scoped timeout of its own beyond whatever `r.Context()` and the
  underlying `internal/httpclient` retry/backoff budget already impose --
  a slow-but-not-dead Tunarr instance can make `/status` itself slow to
  respond. Fix: wrap the probe in an explicit short `context.WithTimeout`
  so `/status` has a bounded worst-case latency independent of Tunarr's.

- **Bounded `cronDone` wait.** `cmd/serve.go`'s `serveUntil` waits on
  `<-cronDone` with no timeout after `cancelCron()` -- see the comment
  added at that line in this fix wave. In practice this returns almost
  immediately (the cron loop's own `ctx.Done()` select fires as soon as
  `cancelCron` runs) except when a schedule tick is already in flight, in
  which case shutdown blocks until that `Runner.Run` call finishes end to
  end. The current backstop is external (Kubernetes SIGKILLs the process
  once its termination grace period elapses). Fix, if this needs a
  self-contained bound: race `<-cronDone` against a
  `time.After(someBound)` and log (not force-kill) if the bound is hit,
  since there's no way to cancel a `Runner.Run` already past its own
  ctx-check points.
- `configs/config.yaml` carries a `log.file` key that `#LogConfig` does not define and no code reads; `make validate` misses it because `cue vet` runs without `-d '#Config'` closedness — remove the key and consider tightening the validate target.
- CLAUDE.md's `internal/` architecture tree omits `cueconfig/`, `metrics/`, and `problem/` — add on next docs touch.

## Deferred (Web UI sub-project close-out)

Recorded at the end of the web-UI final fix wave
(`.superpowers/sdd/2026-08-28-web-ui/`, FINAL whole-branch review) so
these don't get lost. None block the current close-out; each is a known,
scoped-out gap.

- **`initConfig` error-swallowing.** `cmd/root.go:69-73` -- inside
  `initConfig()`, `cfg, err := config.Load(configPath); if err != nil {
  return }` discards `err` entirely: a config file that exists but fails
  CUE validation or fails to parse leaves `appConfig` nil with zero
  diagnostic, and every downstream command that needs config either
  panics on a nil dereference or silently behaves as if no config file
  was ever given. Standalone chore, owner: next session, ~3 lines --
  log the error (or `cobra.CheckErr`/`os.Exit` it, matching how the rest
  of `cmd/` treats a fatal startup condition) instead of a bare `return`.

- **SP4 inherits: `types.d.ts` drift check in CI.** `web/assets/ts/gen/types.d.ts`
  is committed and regenerated by `make web-types`, but nothing today
  fails CI if `api/openapi.yaml` changes without someone re-running
  `make web-types` and committing the regenerated file -- the two can
  silently drift. Add a CI step that runs `make web-types` and fails the
  build on a resulting `git diff`.

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
- [ ] Enforce or document the reapply-interval vs apply-window constraint: `cron_interval` > the serve loop's 24h apply window exhausts the pushed lineup and Tunarr loops it back to the anchor (wall-clock drift). Clamp Days from the interval or cap the interval; at minimum document next to cron_interval in the CUE schema + docs/deployment.md. (re-review 5227c5f, Minor)
- [ ] Committed TS test infra: the web UI has no checked-in jsdom/unit harness (picker + reorder logic were verified with throwaway scripts). Add a minimal committed harness + CI step so UI logic tests accumulate. (flagged during reorder-UI task)
- [ ] Residue from v0.2.2 final gate (adjudicated non-blocking): (a) backward-CAS coincidence — a wrap-lap landing exactly on a slow shared-show block's frozen baseline lands a partial-lap rewind (self-healing; needs restart + shared show_title + far-future occurrence); (b) shared-show blocks with contradictory on_complete policies let the newest plan re-enable a show the other disabled (document-or-validate); (c) the max() half of the provenance stamp (engine.go:835) is unpinned by tests (unreachable-by-construction today).
