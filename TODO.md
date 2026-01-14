# Project TODOs

## Code Reduction via External Dependencies

**Goal**: Reduce codebase size while maintaining existing functionality by leveraging well-maintained external libraries.

**Constraints**:

- Libraries must be actively maintained
- Prefer stable, widely used libraries
- No functionality changes - refactoring only

---

### 8.1 TUI Form Handling with charmbracelet/huh (~150 lines saved)

**Problem**: `internal/tui/model.go` has manual form handling code including validation, focus management, and styling.

**Solution**: Replaced manual textinput-based form handling with `charmbracelet/huh`:

- Replaced 4 textinput fields with a single `huh.Form`
- Removed manual focus management (`focusIndex`, `moveFocus`, etc.)
- Removed manual validation functions (`validateInputs`, `validateName`, etc.)
- Huh handles validation inline with visual feedback
- Special key bindings (ctrl+b, ctrl+f) preserved for cron/filter builders

**Tasks**:

- [x] Add `github.com/charmbracelet/huh` to go.mod and depguard allowlist ✅
- [x] Replace textinput fields with huh form in Model struct ✅
- [x] Create `createBlockForm()` with inline validators ✅
- [x] Update `startEditBlock()` to populate form values ✅
- [x] Update `updateEditBlock()` to use huh form update/state handling ✅
- [x] Update `renderEditBlock()` to use `blockForm.View()` ✅
- [x] Update cron builder integration to use form values ✅
- [x] Remove old validation and focus management code ✅

**Actual Impact**: ~100 lines removed, cleaner declarative form definition

**Status**: ✅ COMPLETE

---

### 8.2 In-Memory Cache with TTL (~80 lines saved)

**Problem**: `internal/cache/cache.go` (111 lines) implements file-based caching with:

- JSON serialization to disk
- TTL based on file modification time
- Manual cleanup of expired entries

**Solution**: Replaced file-based cache with `github.com/patrickmn/go-cache`:

- Removed file I/O operations and JSON serialization
- Simplified `New()` function (no longer needs cacheDir parameter)
- Automatic TTL-based expiration with background cleanup
- Added `SetWithExpiration()` for custom TTL per item
- Added `ItemCount()` for cache statistics

**Tasks**:

- [x] Add `github.com/patrickmn/go-cache` dependency ✅
- [x] Replace file-based cache implementation with go-cache ✅
- [x] Update `New()` signature (remove cacheDir parameter) ✅
- [x] Update caller in `cmd/content_sources.go` ✅
- [x] Update tests to reflect new behavior ✅

**Actual Impact**: ~30 lines removed (111 → ~80), simpler API, automatic cleanup

**Status**: ✅ COMPLETE

---

### 8.3 Database Layer with sqlx (~100 lines saved)

**Problem**: `internal/store/sqlite.go` (339 lines) uses raw `database/sql` with:

- Manual prepared statements
- Repetitive transaction handling
- Raw SQL string queries
- Manual error handling patterns

**Current Implementation**:

```go
// Repeated patterns like:
stmt, err := db.PrepareContext(ctx, "SELECT ... FROM series_state WHERE ...")
defer stmt.Close()
rows, err := stmt.QueryContext(ctx, ...)
// ... manual scanning
```

**Candidate Libraries**:

| Library                   | Stars | Last Commit | Notes                               |
| ------------------------- | ----- | ----------- | ----------------------------------- |
| `github.com/jmoiron/sqlx` | 16k+  | Active      | Lightweight database/sql extensions |
| `github.com/uptrace/bun`  | 4k+   | Active      | Lightweight ORM with query builder  |
| `sqlc.dev`                | 14k+  | Active      | Generate type-safe code from SQL    |

**Recommended**: `github.com/jmoiron/sqlx` - Minimal overhead, extends database/sql, `NamedExec`, `Select`, `Get` helpers.

**Example Transformation**:

```go
// Before: ~20 lines per query
stmt, err := db.PrepareContext(ctx, query)
rows, err := stmt.QueryContext(ctx, args...)
for rows.Next() {
    rows.Scan(&field1, &field2, ...)
}

// After: 3 lines with sqlx
var states []SeriesState
err := db.SelectContext(ctx, &states, query, args...)
```

**Tasks**:

- [x] Add `github.com/jmoiron/sqlx` to go.mod ✅
- [x] Refactor `ExportAllSeriesStates` to use `SelectContext` ✅
- [x] Refactor `GetSeriesState` to use `GetContext` ✅
- [x] Refactor `UpdateSeriesState` to use `NamedExecContext` ✅
- [x] Replace manual transaction handling with sqlx transactions (`BeginTxx`) ✅
- [x] Add db tags to `ScheduleHistoryEntry` struct ✅

**Actual Impact**: ~60 lines removed (339 → 279), cleaner struct-based queries

**Status**: ✅ COMPLETE

---

### 8.4 History Tracker with Cache Library (~50 lines saved)

**Problem**: `internal/scheduler/history.go` (168 lines) implements custom in-memory tracking.

**Analysis**: After evaluation, the current implementation is well-suited for its purpose:

- Nested data structure: `map[programID][]ScheduleHistoryEntry`
- Per-channel filtering within each program's history
- TTL applies to individual entries, not entire cache keys
- Domain-specific methods like `FilterByHistory`, `WasRecentlyScheduled`

**Evaluation**: Generic cache libraries (ccache, golang-lru) don't fit well because:

1. Cache key would need to be composite (programID + channelID)
2. TTL on entries within a slice, not the slice itself
3. Custom filtering logic needed for channel-specific checks
4. Current implementation is clean, well-tested, and only 168 lines

**Decision**: DEFERRED - Current implementation is appropriate and purpose-built.

**Status**: DEFERRED - Domain-specific requirements make generic caching unsuitable

---

### 8.5 Cron Builder Extraction (~170 lines saved via data-driven approach)

**Problem**: `internal/tui/model.go` lines 765-1018 (~250 lines) contained a custom cron expression builder:

- Manual field cycling through preset values
- Hardcoded preset arrays
- String parsing and rebuilding
- Human-readable description generation

**Solution**: Extracted to `internal/cronbuilder/` package with:

- `Expression` struct with all 5 cron fields
- `FieldPresets` map for preset cycling
- `Describe()` method for human-readable descriptions
- `CommonPresets()` for common cron patterns
- `Fields()` for UI rendering metadata

**Tasks**:

- [x] Extract cron builder to `internal/cronbuilder/` package ✅
- [x] Use struct-based presets instead of parallel arrays ✅
- [x] Add human-readable description generation ✅
- [ ] Consider using `github.com/robfig/cron/v3` descriptor utilities (deferred - current approach works well)
- [ ] Make presets configurable via YAML (future enhancement)

**Actual Impact**: ~170 lines removed from TUI model, reusable package created

**Status**: ✅ COMPLETE

---

### 8.6 HTTP Client Middleware Simplification (~30 lines saved)

**Problem**: `internal/httpclient/client.go` (218 lines) wraps go-resty but adds custom:

- Authentication header injection
- Error type handling
- Request building

**Current Implementation**:

```go
func (c *Client) newRequest(ctx context.Context) *resty.Request {
    req := c.client.R().SetContext(ctx)
    if c.apiKey != "" {
        req.SetHeader("X-Api-Key", c.apiKey)
    }
    return req
}
```

**Analysis**: go-resty has built-in middleware/interceptor support that could replace custom code.

**Tasks**:

- [x] Use resty `OnBeforeRequest` hook for auth headers ✅
- [ ] Use resty `OnAfterResponse` hook for error handling (deferred - current approach is cleaner)
- [x] Simplify `newRequest` to just `SetContext` ✅

**Estimated Impact**: ~10 lines saved, cleaner separation of concerns

**Status**: ✅ COMPLETE - Auth header injection moved to middleware

---

## Priority Summary

| Priority | Task                        | Lines Saved | Effort | Status      |
| -------- | --------------------------- | ----------- | ------ | ----------- |
| High     | 8.1 TUI Form Handling (huh) | ~100        | Medium | ✅ Complete |
| High     | 8.3 Database Layer (sqlx)   | ~60         | Medium | ✅ Complete |
| Medium   | 8.2 In-Memory Cache         | ~30         | Low    | ✅ Complete |
| Medium   | 8.5 Cron Builder Extraction | ~170        | Medium | ✅ Complete |
| Low      | 8.4 History Tracker         | ~50         | Low    | Deferred    |
| Low      | 8.6 HTTP Client Middleware  | ~10         | Low    | ✅ Complete |

**Total Achieved Savings**: ~370 lines (8.1, 8.2, 8.3, 8.5, 8.6)
**Remaining Potential**: 0 lines (all pending tasks deferred after evaluation)

---

## Already Completed Optimizations

These were previously implemented and should not be revisited:

- ✅ HTTP Client Consolidation with go-resty (~400 lines saved)
- ✅ Struct Validation with go-playground/validator
- ✅ Slice Utilities with samber/lo
- ✅ Test Assertions with testify
- ✅ Logging simplified to use slog directly

---

## Phase 4: Operational Excellence & Testing

**Goal**: Achieve production-readiness through comprehensive testing, automated maintenance, and observability.

**Current Test Coverage**:
| Package | Coverage | Target |
|---------|----------|--------|
| cache | 100% | ✅ |
| config | 94.4% | ✅ |
| cronbuilder | 98.8% | ✅ |
| external/jellyfin | 86.7% | ✅ |
| external/radarr | 100% | ✅ |
| external/sonarr | 100% | ✅ |
| external/tunarr | 90.8% | ✅ |
| scheduler | 91.9% | ✅ |
| store | 89.5% | ✅ |

---

### 4.1 Store Package Test Coverage

**Problem**: Store package at 78.9% coverage, needs additional tests for edge cases.

**Tasks**:
- [x] Add tests for `CleanupScheduleHistory()` with various cutoff scenarios ✅
- [x] Add tests for concurrent access patterns (via closed store operations) ✅
- [x] Add tests for database migration edge cases (RunCount, Disabled fields) ✅
- [x] Add error injection tests for database failures (closed store, invalid paths) ✅

**Actual Impact**: Coverage improved from 78.9% to 89.5%

**Status**: ✅ COMPLETE

---

### 4.2 Cache Package Test Coverage

**Problem**: Cache package at 77.8% coverage, missing edge case tests.

**Tasks**:
- [x] Add tests for TTL expiration behavior ✅
- [x] Add tests for concurrent access ✅
- [x] Add tests for `ItemCount()` accuracy ✅
- [x] Add tests for `SetWithExpiration()` custom TTL ✅
- [x] Add tests for `copyValue()` all type paths ✅
- [x] Add tests for `Get()` with interface pointer ✅

**Actual Impact**: Coverage improved from 77.8% to 100%

**Status**: ✅ COMPLETE

---

### 4.3 Automatic Schedule History Cleanup

**Problem**: `CleanupScheduleHistory()` exists but is not automatically scheduled.

**Solution**: Implemented a `Cleaner` component in `internal/store/cleaner.go` that:
- Runs as a background goroutine with configurable interval
- Uses Prometheus metrics to track cleanup operations
- Supports graceful shutdown via Stop() or context cancellation
- Can be tested via RunOnce() method

**Tasks**:
- [x] Create a background goroutine for periodic cleanup ✅
- [x] Make cleanup interval configurable (default: 24 hours) ✅
- [x] Add graceful shutdown handling ✅
- [x] Add metrics for cleanup operations ✅
- [x] Add tests for cleanup scheduling ✅

**Configuration** (in `.schedularr.yaml`):
```yaml
maintenance:
  cleanup_enabled: true
  cleanup_interval: "24h"
  history_retention: "168h"  # 7 days
```

**Metrics**:
- `schedularr_cleanup_runs_total` - Total cleanup runs executed
- `schedularr_cleanup_entries_removed_total` - Entries removed by cleanup
- `schedularr_cleanup_duration_seconds` - Cleanup operation duration
- `schedularr_cleanup_errors_total` - Cleanup errors

**Status**: ✅ COMPLETE

---

### 4.4 End-to-End Testing

**Problem**: Minimal E2E tests for the complete schedule generation workflow.

**Tasks**:
- [ ] Create E2E test for full schedule generation with mock Tunarr
- [ ] Add E2E test for series-based scheduling with state persistence
- [ ] Add E2E test for conflict resolution between blocks
- [ ] Add E2E test for filler content integration

**Status**: 🔲 PENDING

---

## Phase 5: UX Enhancements

**Goal**: Improve user experience through better TUI interactions and visualizations.

---

### 5.1 TUI Block Editing CRUD Operations

**Problem**: TUI has basic scaffolding but incomplete CRUD operations.

**Tasks**:
- [ ] Complete block creation flow with validation
- [ ] Implement block deletion with confirmation
- [ ] Add block duplication feature
- [ ] Implement block reordering by priority
- [ ] Add undo/redo support for changes

**Status**: 🔲 PENDING

---

### 5.2 Schedule Preview Visualization

**Problem**: No way to preview what a schedule would look like before applying.

**Tasks**:
- [ ] Implement `--dry-run` mode for schedule generation
- [ ] Create TUI view for schedule preview
- [ ] Show program titles, durations, and time slots
- [ ] Highlight potential conflicts or gaps

**Status**: 🔲 PENDING

---

### 5.3 Series State Visualization

**Problem**: No visibility into current episode tracking state.

**Tasks**:
- [ ] Add CLI command to view series state (`schedularr series list`)
- [ ] Create TUI panel for series state overview
- [ ] Show current episode per series
- [ ] Add ability to manually adjust episode position

**Status**: 🔲 PENDING
