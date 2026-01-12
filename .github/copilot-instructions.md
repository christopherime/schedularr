# Schedularr AI Coding Agent Instructions

## Project Overview

Schedularr is a Go CLI tool for automated content scheduling on Tunarr TV channels using cron-based recurring blocks with advanced filtering. It features a Bubble Tea TUI, CUE schema validation, SQLite state persistence, and priority-based schedule conflict resolution.

## Architecture & Component Boundaries

**Entry Point**: `main.go` → `cmd/` (Cobra commands) → `internal/` packages

### Key Components

- **`cmd/`**: Cobra CLI commands (`root`, `channels`, `config`, `generate`, `run`, `scheduler`, `tui`, `validate`)
- **`internal/scheduler/`**: Core scheduling engine with cron parsing, priority resolution, series state tracking
  - `engine.go`: Main scheduling logic with `GenerateForTimeRange()` and conflict resolution
  - `filter.go`: Content filtering by genre, rating, duration, year, title regex
  - `history.go`: Prevents re-scheduling content within rotation windows
  - `state.go`: SQLite-backed series episode progression tracking
- **`internal/tunarr/`**: API client with exponential backoff retry logic
  - ⚠️ **Critical**: Many endpoints are placeholders (see `docs/TUNARR_API_RESEARCH.md` before modifying)
- **`internal/config/`**: Viper-based config loading from `~/.schedularr.yaml` or `./.schedularr.yaml`
- **`internal/cueconfig/`**: CUE schema validation for type-safe configuration
- **`internal/tui/`**: Bubble Tea interactive terminal UI
- **`internal/store/`**: SQLite state persistence for series tracking

### Data Flow

1. **Config Load**: `cmd/root.go` → Viper → CUE validation → Domain types
2. **Schedule Generation**: CLI → Engine.GenerateForTimeRange() → Tunarr API → Filter → Priority Resolution → Tunarr Update
3. **Series State**: Engine → SQLite store → Track episode progression per series/block

## Critical Developer Workflows

### Build & Test

```bash
# Development build
go build -o schedularr main.go

# Optimized release build (required for distribution)
go build -ldflags="-s -w" -o schedularr main.go

# Run tests with race detection
go test -race -cover ./...

# Lint (strict rules enforced)
golangci-lint run

# Security scans
gosec ./...
govulncheck ./...
```

### Configuration Generation & Validation

```bash
# Generate default configs (CUE → YAML)
schedularr config generate config.yaml
schedularr scheduler init scheduler.yaml

# Validate configs against CUE schemas
schedularr validate config.yaml scheduler.yaml

# Manual CUE validation
cue vet configs/config.yaml cmd/schema/config.cue
```

### Debugging Schedule Generation

```bash
# Dry-run mode (preview without applying)
schedularr generate --scheduler scheduler.yaml --dry-run

# Enable debug logging
schedularr --config config.yaml generate --scheduler scheduler.yaml --log-level debug
```

## Project-Specific Conventions

### Configuration Schema Workflow

**DO NOT** manually edit YAML examples without updating CUE schemas first:

1. Edit schema: `cmd/schema/config.cue` or `cmd/schema/scheduler.cue`
2. Validate: `cue vet <yaml-file> <schema-file>`
3. Regenerate examples: `schedularr config generate` / `schedularr scheduler init`

**Rationale**: CUE schemas are the source of truth for defaults, types, and validation rules.

### Error Handling Pattern

```go
// ✅ Correct: Contextual wrapping with %w
if err != nil {
    return fmt.Errorf("failed to parse cron '%s' for block %s: %w", block.Cron, block.Name, err)
}

// ❌ Avoid: github.com/pkg/errors (blocked by depguard)
```

### Logging Standards

**Use `log/slog` with structured fields, not string formatting:**

```go
// ✅ Correct: Structured logging
logger.Info("block scheduled",
    "block_name", block.Name,
    "channel_id", channelID,
    "start_time", startTime,
    "program_count", len(programs))

// ❌ Avoid: Unstructured logging
logger.Info(fmt.Sprintf("Block %s scheduled on %s", block.Name, channelID))
```

**Field naming**: Use `snake_case` for log fields (matches JSON output).

### Testing Patterns

**Use table-driven tests with meaningful scenario names:**

```go
func TestFilterPrograms_ExcludesShort(t *testing.T) {
    tests := []struct {
        name     string
        filter   Filter
        programs []Program
        want     int
    }{
        {
            name: "exclude programs under 20 minutes",
            filter: Filter{MinDuration: 20},
            programs: []Program{{Duration: 15}, {Duration: 25}},
            want: 1,
        },
    }
    // ...
}
```

**Mock Tunarr API**: Use `internal/scheduler/mock_store.go` pattern for interfaces.

### Code Complexity Limits (enforced by golangci-lint)

- **Cyclomatic complexity**: max 15
- **Cognitive complexity**: max 20
- **Function nesting**: max 5 levels
- **Function results**: max 3 return values
- **Arguments**: max 5 parameters

**Tip**: Extract helper functions when approaching limits (see `engine.go` for examples).

## Integration Points

### Tunarr API Client

**⚠️ CRITICAL**: Many endpoints in `internal/tunarr/client.go` are placeholders. **Always consult `docs/TUNARR_API_RESEARCH.md` before modifying API calls.**

**Known issues**:

- Programs endpoint (`/api/programs`) may not match actual API
- Schedule update endpoint structure unverified
- Requires runtime testing against actual Tunarr instance

**Retry logic**: Exponential backoff with 3 retries (1s → 30s) for all API calls.

### SQLite State Store

**Series progression tracking**: `internal/store/sqlite.go` tracks last-scheduled episode per series/block to maintain continuity across runs.

**Schema**: See `sqlite.go` migrations (auto-applied on first run).

## Commit & PR Conventions

**Conventional Commits required:**

```
feat(scheduler): add episode skip feature
fix(tunarr): correct schedule endpoint payload
docs(api): update Tunarr endpoint research
test(filter): add genre filtering edge cases
```

**PR Checklist**:

- [ ] Tests added/updated (`go test -cover ./...`)
- [ ] Linting passes (`golangci-lint run`)
- [ ] CUE schemas updated if config changes
- [ ] `docs/TUNARR_API_RESEARCH.md` updated if API changes
- [ ] TUI changes include screenshots

## Frequently Missed Details

1. **Entry point is `main.go` not `cmd/schedularr/main.go`** (common misconception from reading AGENTS.md)
2. **Config lookup order**: Flag → `$HOME/.schedularr.yaml` → `./.schedularr.yaml`
3. **Cron parser uses robfig/cron/v3 format**: minute, hour, day, month, weekday (no seconds)
4. **Priority resolution**: Higher priority blocks win overlaps; same priority = first defined
5. **History tracking**: `ScheduleHistory` prevents re-scheduling within 7-day default window
6. **CGO required**: SQLite dependency (`mattn/go-sqlite3`) requires `CGO_ENABLED=1`

## Quick Reference: Key Files

| File/Directory                                               | Purpose                               |
| ------------------------------------------------------------ | ------------------------------------- |
| [main.go](main.go)                                           | Entry point (calls `cmd.Execute()`)   |
| [cmd/root.go](cmd/root.go)                                   | Cobra root command & config init      |
| [internal/scheduler/engine.go](internal/scheduler/engine.go) | Core scheduling algorithm             |
| [internal/tunarr/client.go](internal/tunarr/client.go)       | Tunarr API client (many placeholders) |
| [cmd/schema/config.cue](cmd/schema/config.cue)               | Application config schema             |
| [cmd/schema/scheduler.cue](cmd/schema/scheduler.cue)         | Scheduler config schema               |
| [.golangci.yml](.golangci.yml)                               | Linting rules & complexity limits     |
| [docs/TUNARR_API_RESEARCH.md](docs/TUNARR_API_RESEARCH.md)   | API endpoint verification status      |

## VS Code Settings

Recommended `.vscode/settings.json`:

```json
{
  "go.testFlags": ["-v", "-race", "-cover"],
  "go.lintTool": "golangci-lint",
  "go.lintOnSave": "package",
  "go.buildOnSave": "package"
}
```
