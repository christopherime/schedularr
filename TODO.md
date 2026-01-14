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

**Current Implementation**:

```go
// Custom file-based cache
type Cache struct {
    cacheDir      string
    cacheDuration time.Duration
}
func (c *Cache) Get(key string, v interface{}) (bool, error) { ... }
func (c *Cache) Set(key string, v interface{}) error { ... }
```

**Analysis**: File persistence is used for API response caching (`tunarr_programs.json`, `radarr_movies.json`). However, if in-memory caching is acceptable (data re-fetched on restart), this can be simplified.

**Candidate Libraries**:

| Library                           | Stars | Last Commit | Notes                                  |
| --------------------------------- | ----- | ----------- | -------------------------------------- |
| `github.com/patrickmn/go-cache`   | 8k+   | Stable      | Simple in-memory cache with TTL        |
| `github.com/dgraph-io/ristretto`  | 5k+   | Active      | High-performance with admission policy |
| `github.com/karlseguin/ccache/v3` | 2k+   | Active      | Concurrent cache with TTL              |

**Recommended**: `github.com/patrickmn/go-cache` - Simple API, proven, matches current functionality.

**Example Transformation**:

```go
// Before: 111 lines of file-based caching
cache := cache.New("/tmp/schedularr", 1*time.Hour)
cache.Set("programs", programs)

// After: 5 lines
import gocache "github.com/patrickmn/go-cache"
c := gocache.New(1*time.Hour, 10*time.Minute)
c.Set("programs", programs, gocache.DefaultExpiration)
```

**Tasks**:

- [x] Evaluate if file persistence is required (currently survives restarts) ✅
- [ ] If in-memory acceptable: replace with go-cache (~80 lines saved)
- [x] If persistence required: keep current or add go-cache as in-memory layer ✅

**Decision**: File persistence IS required. The cache stores API responses to disk to survive process restarts (important for daemon mode), reduce API load on external services, and enable cache inspection/debugging via file system.

**Status**: DEFERRED - Current implementation is appropriate for the use case

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
| Medium   | 8.2 In-Memory Cache         | 0           | N/A    | Deferred    |
| Medium   | 8.5 Cron Builder Extraction | ~170        | Medium | ✅ Complete |
| Low      | 8.4 History Tracker         | ~50         | Low    | Deferred    |
| Low      | 8.6 HTTP Client Middleware  | ~10         | Low    | ✅ Complete |

**Total Achieved Savings**: ~340 lines (8.1, 8.3, 8.5, 8.6)
**Remaining Potential**: 0 lines (all pending tasks deferred after evaluation)

---

## Already Completed Optimizations

These were previously implemented and should not be revisited:

- ✅ HTTP Client Consolidation with go-resty (~400 lines saved)
- ✅ Struct Validation with go-playground/validator
- ✅ Slice Utilities with samber/lo
- ✅ Test Assertions with testify
- ✅ Logging simplified to use slog directly
