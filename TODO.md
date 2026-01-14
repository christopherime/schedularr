# Project TODOs

## Code Reduction via External Dependencies

**Goal**: Reduce codebase size while maintaining existing functionality by leveraging well-maintained external libraries.

**Constraints**:

- Libraries must be actively maintained
- Prefer stable, widely used libraries
- No functionality changes - refactoring only

---

### 8.1 TUI Form Handling with charmbracelet/huh (~150 lines saved)

**Problem**: `internal/tui/model.go` (1,888 lines) has extensive manual form handling code:

- Custom input validation (`validateField`, `validateInputs` methods)
- Manual focus management (`moveFocus`, `updateInputFocusStyles`)
- Repeated style definitions with magic color codes (205, 240, 252)
- Hand-rolled navigation logic (tab/shift+tab/enter handling)

**Current Implementation**:

```go
// Lines 232-460: Manual form handling
func (m Model) updateEditBlock(msg tea.KeyMsg) (tea.Model, tea.Cmd) { ... }
func (m *Model) validateField(index int) { ... }
func (m *Model) moveFocus(delta int) { ... }
func (m *Model) updateInputFocusStyles() tea.Cmd { ... }
```

**Candidate Library**:

| Library                        | Stars | Last Commit | Notes                                  |
| ------------------------------ | ----- | ----------- | -------------------------------------- |
| `github.com/charmbracelet/huh` | 5k+   | Active      | High-level form builder for Bubble Tea |

**Recommended**: `charmbracelet/huh` - Built by same team as Bubble Tea, provides form groups, validation, theming out of the box.

**Example Transformation**:

```go
// Before: ~230 lines of manual form handling
m.inputs[0].SetValue(b.Name)
m.inputs[1].SetValue(b.Cron)
// ... manual validation, focus management

// After: ~30 lines with huh
form := huh.NewForm(
    huh.NewGroup(
        huh.NewInput().Title("Name").Value(&name).Validate(notEmpty),
        huh.NewInput().Title("Cron").Value(&cron).Validate(validCron),
        huh.NewInput().Title("Duration").Value(&duration).Validate(isPositiveInt),
        huh.NewInput().Title("Channel ID").Value(&channelID).Validate(notEmpty),
    ),
)
```

**Tasks**:

- [ ] Add `github.com/charmbracelet/huh` to go.mod
- [ ] Create form definitions for block editing
- [ ] Create form definitions for filter builder
- [ ] Migrate validation logic to huh validators
- [ ] Extract color constants to a `styles.go` file
- [ ] Remove manual focus/navigation code

**Estimated Impact**: ~150 lines removed, cleaner form handling, built-in accessibility

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

**Problem**: `internal/scheduler/history.go` (168 lines) implements custom in-memory tracking:

- Manual sync.RWMutex for concurrency
- Custom TTL-based cleanup (`CleanupOldEntries`)
- Per-program, per-channel history maps

**Current Implementation**:

```go
type HistoryTracker struct {
    mu      sync.RWMutex
    entries map[string]map[string][]HistoryEntry
    window  time.Duration
}
```

**Candidate Libraries**:

| Library                              | Stars | Last Commit | Notes                                   |
| ------------------------------------ | ----- | ----------- | --------------------------------------- |
| `github.com/karlseguin/ccache/v3`    | 2k+   | Active      | Concurrent cache with TTL and callbacks |
| `github.com/hashicorp/golang-lru/v2` | 5k+   | Active      | Thread-safe LRU with generics           |

**Analysis**: Current implementation is domain-specific (per-program, per-channel). A generic cache may not fit perfectly but could simplify the TTL logic.

**Tasks**:

- [ ] Evaluate if generic cache fits domain requirements
- [ ] If yes: replace internal map with ccache or golang-lru
- [ ] If no: keep custom implementation, consider simplifying cleanup logic

**Estimated Impact**: ~50 lines if applicable, but may lose domain-specific features

---

### 8.5 Cron Builder Extraction (~100 lines saved via data-driven approach)

**Problem**: `internal/tui/model.go` lines 765-1018 (~250 lines) contain a custom cron expression builder:

- Manual field cycling through preset values
- Hardcoded preset arrays
- String parsing and rebuilding
- Human-readable description generation

**Current Implementation**:

```go
var minutePresets = []string{"*", "0", "15", "30", "45", "0,30"}
var hourPresets = []string{"*", "0", "6", "8", "12", "18", "20", "22"}
// ... repeated for each field
```

**Approach**: Extract to a reusable package with data-driven configuration.

**Tasks**:

- [ ] Extract cron builder to `internal/cronbuilder/` package
- [ ] Use struct-based presets instead of parallel arrays
- [ ] Consider using `github.com/robfig/cron/v3` descriptor utilities
- [ ] Make presets configurable via YAML

**Estimated Impact**: ~100 lines saved through consolidation, reusable outside TUI

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

| Priority | Task                             | Lines Saved | Effort |
| -------- | -------------------------------- | ----------- | ------ |
| High     | 8.1 TUI Form Handling (huh)      | ~150        | Medium |
| High     | 8.3 Database Layer (sqlx) ✅     | ~60         | Medium |
| Medium   | 8.2 In-Memory Cache (deferred)   | 0           | N/A    |
| Medium   | 8.5 Cron Builder Extraction      | ~100        | Medium |
| Low      | 8.4 History Tracker              | ~50         | Low    |
| Low      | 8.6 HTTP Client Middleware ✅    | ~10         | Low    |

**Total Potential Savings**: ~510 lines

---

## Already Completed Optimizations

These were previously implemented and should not be revisited:

- ✅ HTTP Client Consolidation with go-resty (~400 lines saved)
- ✅ Struct Validation with go-playground/validator
- ✅ Slice Utilities with samber/lo
- ✅ Test Assertions with testify
- ✅ Logging simplified to use slog directly
