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

**Current Test Coverage** (Updated 2026-01-14):

| Package           | Coverage | Target |
| ----------------- | -------- | ------ |
| cache             | 100%     | ✅     |
| config            | 94.6%    | ✅     |
| cronbuilder       | 98.8%    | ✅     |
| cueconfig         | 85.5%    | ✅     |
| external/jellyfin | 100%     | ✅     |
| external/radarr   | 87.5%    | ✅     |
| external/sonarr   | 86.7%    | ✅     |
| external/tunarr   | 90.8%    | ✅     |
| httpclient        | 89.7%    | ✅     |
| scheduler         | 91.9%    | ✅     |
| store             | 93.0%    | ✅     |

**Note**: TUI package has 0% coverage as interactive UI code is harder to unit test.

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

**Solution**: Two-pronged implementation:

1. **Direct cleanup in generate command** (`cmd/generate.go`):
   - Runs cleanup after `--apply` when `maintenance.cleanup_enabled` is true
   - Uses `history_retention` config to determine what to keep
   - Non-fatal - doesn't interrupt schedule application on failure

2. **Background Cleaner component** (`internal/store/cleaner.go`):
   - Designed for daemon/serve mode (future enhancement)
   - Runs as background goroutine with configurable interval
   - Uses Prometheus metrics to track cleanup operations
   - Supports graceful shutdown via Stop() or context cancellation
   - Can be tested via RunOnce() method

**Tasks**:

- [x] Create a background goroutine for periodic cleanup ✅
- [x] Make cleanup interval configurable (default: 24 hours) ✅
- [x] Add graceful shutdown handling ✅
- [x] Add metrics for cleanup operations ✅
- [x] Add tests for cleanup scheduling ✅
- [x] Integrate cleanup into generate --apply command ✅

**Configuration** (in `.schedularr.yaml`):

```yaml
maintenance:
  cleanup_enabled: true
  cleanup_interval: "24h"
  history_retention: "168h"  # 7 days
```

**Metrics** (for background Cleaner):

- `schedularr_cleanup_runs_total` - Total cleanup runs executed
- `schedularr_cleanup_entries_removed_total` - Entries removed by cleanup
- `schedularr_cleanup_duration_seconds` - Cleanup operation duration
- `schedularr_cleanup_errors_total` - Cleanup errors

**Future Enhancement**: The background `Cleaner` struct is available for use in a future daemon/serve mode.

**Status**: ✅ COMPLETE

---

### 4.4 End-to-End Testing

**Problem**: Minimal E2E tests for the complete schedule generation workflow.

**Solution**: Added comprehensive integration tests in `internal/scheduler/integration_test.go`:

**Tasks**:

- [x] Create E2E test for full schedule generation with mock Tunarr ✅
- [x] Add E2E test for series-based scheduling with state persistence ✅
- [x] Add E2E test for conflict resolution between blocks ✅
- [x] Add E2E test for mixed block types (filter + series) ✅

**Tests Added**:

- `TestIntegration_FullSchedulingWorkflow` - Full workflow with filter blocks
- `TestIntegration_ConflictResolution` - Priority-based conflict resolution
- `TestIntegration_SeriesSchedulingWithState` - Series blocks with state tracking
- `TestIntegration_SeriesStateRestoration` - State persistence across runs
- `TestIntegration_MixedBlockTypes` - Combined filter and series blocks
- `TestIntegration_FilterValidation` - Multi-criteria filter validation

**Status**: ✅ COMPLETE

---

## Phase 5: UX Enhancements

**Goal**: Improve user experience through better TUI interactions and visualizations.

---

### 5.1 TUI Block Editing CRUD Operations

**Problem**: TUI has basic scaffolding but incomplete CRUD operations.

**Solution**: Implemented full CRUD operations with persistence:

**Tasks**:

- [x] Complete block creation flow with validation ✅ (huh forms with validators)
- [x] Implement block deletion with confirmation ✅
- [x] Add block duplication feature ✅ (`D` key)
- [x] Implement block priority adjustment ✅ (`+`/`-` keys)
- [x] Add save to disk functionality ✅ (`ctrl+s`)
- [ ] Add undo/redo support for changes (deferred - lower priority)

**New Features**:

- `D` - Duplicate selected block
- `+`/`-` - Adjust block priority
- `ctrl+s` - Save configuration to scheduler.yaml
- Unsaved changes indicator in status bar
- Priority displayed in block list

**Files Modified**:

- `internal/tui/model.go` - CRUD operations, priority adjustment, save functionality
- `internal/config/config.go` - Added `SaveSchedulerConfig()` function
- `cmd/tui.go` - Updated to pass scheduler file path

**Status**: ✅ COMPLETE (undo/redo deferred)

---

### 5.2 Schedule Preview Visualization

**Problem**: No way to preview what a schedule would look like before applying.

**Solution**: Implemented both CLI dry-run and TUI timeline views.

**Tasks**:

- [x] Implement `--dry-run` mode for schedule generation ✅ (already exists in generate command)
- [x] Create TUI view for schedule preview ✅ (block timeline view)
- [x] Show program titles, durations, and time slots ✅
- [x] Highlight potential conflicts or gaps ✅ (conflict detection)

**CLI Preview**:
The `generate --dry-run` flag provides full schedule preview with actual content.
Use `--verbose` for detailed filtering and history output.

**TUI Timeline** (in `internal/tui/model.go`):

- Press 't' from block list to view 24-hour schedule timeline
- Shows: Block name, start/end times, channel ID, priority
- Highlights conflicts when blocks overlap on the same channel
- Navigate with j/k or arrows, refresh with 'r'
- Parses cron expressions to calculate block occurrences

**Status**: ✅ COMPLETE

---

### 5.3 Series State Visualization

**Problem**: No visibility into current episode tracking state.

**Solution**: Implemented comprehensive series state management with CLI and TUI.

**Tasks**:

- [x] Add CLI command to view series state (`schedularr state list`) ✅
- [x] Create TUI panel for series state overview ✅ (press 's' in block list)
- [x] Show current episode per series ✅
- [x] Add ability to manually adjust episode position ✅ (press 'e' to edit)

**CLI Commands** (in `cmd/state.go`):

- `schedularr state list` - List all series with episode, status, run count
- `schedularr state set <show> -s <season> -e <episode>` - Set series position
- `schedularr state reset <show>` - Reset series to S01E01
- `schedularr state export <file>` - Export states to JSON
- `schedularr state import <file>` - Import states from JSON
- `schedularr state backup <file>` - Full database backup

**TUI Features** (in `internal/tui/model.go`):

- Press 's' from block list to view series progress
- Displays: Show title, current episode, run count, status, last aired
- Press 'e' to edit selected series position
- Press 'r' to refresh from database
- Color-coded status: Active (white), Completed (green), Disabled (gray)

**Files Modified**:

- `cmd/state.go` - Added `set` command, improved `list` output
- `internal/store/sqlite.go` - Added `SetSeriesState` method, fixed export/import
- `internal/tui/model.go` - Added series edit view (stateSeriesEdit)

**Status**: ✅ COMPLETE

---

## Code Quality Notes

### Unused Code Analysis (via deadcode)

The following code is reported as "unreachable" by deadcode but is intentionally kept:

**Public API Methods** (designed for consumers or future use):

- `internal/cache/cache.go`: `SetWithExpiration`, `Clear`, `ClearAll`, `ItemCount`
- `internal/config/config.go`: `New`, `GetSchedulerConfig`
- `internal/external/tunarr/client.go`: `GetShows`, `GetShowEpisodes`, `SearchPrograms`, `GetFillerLists`
- `internal/httpclient/client.go`: `Put`, `Delete`, `SetBaseURL`, `ValidateRequired`

**Engine Variants** (constructor options):

- `internal/scheduler/engine.go`: `NewEngineWithOptions`, `NewEngineWithHistory`

**In-Memory History** (complete API surface, tested):

- `internal/scheduler/history.go`: `GetLastScheduled`, `CleanupOldEntries`, `GetStats`, `FilterByHistory`
- Note: Core methods `Window()`, `RecordPrograms()`, `WasRecentlyScheduled()` ARE used

**Test Helpers**:

- `internal/scheduler/mock_store.go`: All methods (used in tests)

**Future Features**:

- `internal/store/cleaner.go`: Background `Cleaner` struct for daemon mode

**Rationale for keeping**:

1. Public API completeness for external consumers
2. Well-tested code that may be used in future features
3. Standard patterns (HTTP methods, constructors with options)
4. No runtime overhead when unused

---

## Phase 9: Further Code Reduction via External Libraries

**Goal**: Reduce custom code by leveraging well-maintained external libraries where the trade-off (dependency vs. code reduction) is favorable.

**Analysis Date**: 2026-01-14

---

### 9.1 CLI Table Output with go-pretty (~80 lines saved)

**Problem**: Multiple CLI commands use `text/tabwriter` with manual column formatting:

- `cmd/generate.go`: ~60 lines for schedule display with manual width calculation
- `cmd/channels.go`: ~20 lines for channel listing
- `cmd/state.go`: ~15 lines for series state listing

**Current Implementation**:

```go
w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
fmt.Fprintln(w, "ID\tNumber\tName\tEnabled")
fmt.Fprintf(w, "%s\t%d\t%s\t%t\n", ch.ID, ch.Number, ch.Name, ch.Enabled)
w.Flush()
```

**Candidate Library**: `github.com/jedib0t/go-pretty/v6/table` (8K+ stars, actively maintained)

**Benefits**:

- Automatic column width calculation
- Built-in borders and styling (consistent with lipgloss usage)
- Color support, sorting, filtering
- Markdown/HTML/CSV output modes
- Reduces boilerplate significantly

**Example After**:

```go
t := table.NewWriter()
t.SetOutputMirror(os.Stdout)
t.AppendHeader(table.Row{"ID", "Number", "Name", "Enabled"})
t.AppendRow(table.Row{ch.ID, ch.Number, ch.Name, ch.Enabled})
t.Render()
```

**Tasks**:

- [x] Add `github.com/jedib0t/go-pretty/v6` to go.mod ✅
- [x] Refactor `displayChannelSchedule()` in cmd/generate.go ✅
- [x] Refactor `formatProgramRow()` in cmd/generate.go (merged into displayChannelSchedule) ✅
- [x] Refactor channel listing in cmd/channels.go ✅
- [x] Refactor state listing in cmd/state.go ✅
- [x] Remove manual truncation functions (go-pretty handles this) ✅

**Actual Impact**: ~60 lines removed, better UX with auto-sizing columns and consistent table styling

**Status**: ✅ COMPLETE

---

### 9.2 Database Migrations with golang-migrate (~40 lines saved)

**Problem**: `internal/store/sqlite.go` uses manual migration pattern with ignored errors:

```go
migrations := []string{
    `ALTER TABLE series_state ADD COLUMN run_count INTEGER NOT NULL DEFAULT 0`,
    `ALTER TABLE series_state ADD COLUMN disabled BOOLEAN NOT NULL DEFAULT 0`,
}
for _, migration := range migrations {
    _, _ = s.db.ExecContext(ctx, migration)  // Errors silently ignored!
}
```

**Issues with current approach**:

1. Errors are silently ignored (dangerous for production)
2. No version tracking - can't tell which migrations ran
3. No rollback support
4. Migrations embedded in Go code, harder to audit

**Candidate Library**: `github.com/golang-migrate/migrate/v4` (14K+ stars, actively maintained)

**Benefits**:

- Versioned migrations with up/down support
- Proper error handling and transaction safety
- SQL files separated from Go code
- CLI tool for manual migration management
- Automatic version tracking in database

**Example Structure After**:

```
migrations/
├── 000001_initial_schema.up.sql
├── 000001_initial_schema.down.sql
├── 000002_add_run_count_disabled.up.sql
└── 000002_add_run_count_disabled.down.sql
```

**Tasks**:

- [x] Add `github.com/golang-migrate/migrate/v4` to go.mod ✅
- [x] Create `migrations/` directory with SQL files ✅
- [x] Extract current schema to `000001_initial_schema.up.sql` ✅
- [x] Add down migrations for rollback support ✅
- [x] Refactor `initSchema()` to use migrate library ✅
- [ ] Add migration CLI commands (optional: `schedularr db migrate`) - deferred, not needed for basic usage

**Implementation Notes**:
- Used Go's embed.FS to embed migrations in the binary (no external files needed)
- Single migration file with complete schema (since columns already exist in current schema)
- Migrations are automatically applied on Store initialization
- Version tracking via golang-migrate's schema_migrations table

**Actual Impact**: ~35 lines of inline schema code replaced with proper migration system

**Status**: ✅ COMPLETE

---

### 9.3 Retry Logic Consolidation with avast/retry-go (~20 lines saved)

**Problem**: Custom retry logic in `cmd/generate.go`:

```go
func refreshJellyfinWithRetries(client *jellyfin.Client) error {
    maxRetries := 3
    baseDelay := 2 * time.Second
    var err error
    for i := 0; i < maxRetries; i++ {
        err = client.RefreshLiveTVGuide(context.Background())
        if err == nil {
            return nil
        }
        if i < maxRetries-1 {
            delay := baseDelay * time.Duration(1<<i)
            time.Sleep(delay)
        }
    }
    return fmt.Errorf("after %d attempts: %w", maxRetries, err)
}
```

This duplicates retry logic already in `internal/httpclient/client.go`.

**Candidate Library**: `github.com/avast/retry-go/v4` (2K+ stars, actively maintained)

**Benefits**:

- Declarative retry configuration
- Built-in exponential backoff, jitter
- Context support for cancellation
- Retry condition functions
- Already a dependency pattern (httpclient uses resty retries)

**Example After**:

```go
err := retry.Do(
    func() error { return client.RefreshLiveTVGuide(ctx) },
    retry.Attempts(3),
    retry.Delay(2*time.Second),
    retry.DelayType(retry.BackOffDelay),
    retry.OnRetry(func(n uint, err error) {
        fmt.Printf("Retry %d: %v\n", n, err)
    }),
)
```

**Tasks**:

- [x] Add `github.com/avast/retry-go/v4` to go.mod ✅
- [x] Refactor `refreshJellyfinWithRetries()` to use retry-go ✅
- [ ] Consider using retry-go in httpclient for consistency (optional - deferred, httpclient uses resty's built-in retries)

**Actual Impact**: ~8 lines removed, cleaner declarative retry logic

**Status**: ✅ COMPLETE

---

### Priority Summary - Phase 9

| Priority | Task                        | Lines Saved | Effort | Status      |
| -------- | --------------------------- | ----------- | ------ | ----------- |
| High     | 9.1 CLI Tables (go-pretty)  | ~60         | Medium | ✅ Complete |
| Medium   | 9.2 DB Migrations (migrate) | ~35         | Medium | ✅ Complete |
| Low      | 9.3 Retry Logic (retry-go)  | ~8          | Low    | ✅ Complete |

**Total Achieved Savings**: ~103 lines (9.1, 9.2, 9.3)
**Phase 9 Status**: ✅ COMPLETE

---

### Libraries Already Well-Utilized (No Changes Recommended)

The codebase already makes excellent use of libraries:

| Area          | Current Library           | Assessment                     |
| ------------- | ------------------------- | ------------------------------ |
| TUI Framework | Bubble Tea, Lipgloss, Huh | Best-in-class, no replacement  |
| Config        | Viper, yaml.v3            | Industry standard              |
| HTTP Client   | go-resty                  | Well-integrated with retries   |
| Caching       | go-cache                  | Appropriate for use case       |
| Cron Parsing  | robfig/cron               | Industry standard              |
| Validation    | go-playground/validator   | Industry standard              |
| Functional    | samber/lo                 | Good use of modern Go patterns |
| Metrics       | Prometheus client         | Industry standard              |
| Database      | sqlx                      | Already migrated from raw sql  |
| Schema        | CUE                       | Specialized, appropriate       |
