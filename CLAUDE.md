# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Schedularr is a Go application that automates content scheduling for Tunarr TV channels. It uses cron-based recurring blocks with advanced filtering to intelligently program channel content without manual intervention.

## Common Commands

### Build & Run

```bash
# Build the binary (recommended - uses Makefile)
make build

# Run the application
./bin/schedularr --help
./bin/schedularr channels
./bin/schedularr generate --apply
./bin/schedularr tui

# Direct Go build (if needed)
go build -o schedularr main.go
```

### Testing

```bash
# Run all tests (recommended - uses Makefile)
make test

# Run tests manually
go test ./...

# Run with coverage
go test -cover ./...

# Run specific package tests
go test ./internal/scheduler/...

# Generate coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

### Code Quality

```bash
# Run all linters (recommended - uses Makefile)
make lint

# Individual linting tools
golangci-lint run  # Code quality and style
gosec ./...        # Security vulnerabilities
govulncheck ./...  # Known CVEs

# Format code
go fmt ./...
make fmt
```

### Other Makefile Targets

```bash
make clean      # Remove build artifacts
make validate   # Validate config files with CUE
make deps       # Download and tidy dependencies
make help       # Show all available targets
```

## Architecture Overview

### Project Structure

```
cmd/schedularr/          # Application entry point (main.go)
internal/
  ├── cli/               # Cobra-based CLI commands (root, channels, generate, tui)
  ├── config/            # Viper-based configuration management
  ├── scheduler/         # Core scheduling engine with filtering logic
  │   ├── engine.go      # Main scheduling engine (GenerateForTimeRange, PlanBlock)
  │   ├── filter.go      # Content filtering by genres, ratings, duration, etc.
  │   └── types.go       # Block, Filter, and Config types
  ├── tunarr/            # Tunarr API client
  │   ├── client.go      # HTTP client with auth (GetChannels, GetPrograms, UpdateSchedule)
  │   ├── models.go      # Channel and Program data models
  │   └── config.go      # Tunarr connection configuration
  └── tui/               # Bubble Tea terminal UI for block editing
configs/                 # Example configuration files
docs/                    # Architecture, specifications, and API research
```

### Key Components & Flow

**Configuration Loading:**

- Config files located at `~/.schedularr.yaml` or `./.schedularr.yaml`
- Viper loads: Tunarr connection, log settings, and scheduler blocks
- Each block defines: name, cron expression, duration, channel ID, priority, and filter criteria

**Scheduling Engine (`internal/scheduler/engine.go`):**

1. `GenerateForTimeRange()` iterates through blocks and their cron schedules
2. For each scheduled time slot, `PlanBlock()` selects content:
   - Filters programs using `FilterPrograms()` (genre, rating, year, duration, title regex)
   - Randomly shuffles candidates
   - Fills block duration using a simple greedy algorithm (knapsack-like)
3. Returns map of `ChannelID -> []Program`

**Tunarr Client (`internal/tunarr/client.go`):**

- REST API client with optional API key authentication (`X-API-Key` header)
- Main methods:
  - `GetChannels()` - Fetch all channels from `/api/channels`
  - `GetPrograms()` - Fetch available programs from `/api/programs` (placeholder endpoint)
  - `UpdateSchedule()` - POST schedule to `/api/channels/{id}/schedule` (placeholder endpoint)
- Note: Program fetching and schedule update endpoints need verification against actual Tunarr API

**Filter System (`internal/scheduler/filter.go`):**

- Supports filtering by: title regex, genres, ratings, year range, duration range, and tags
- All filter criteria use AND logic (all conditions must match)

### Important Data Structures

**Block (`internal/scheduler/types.go`):**

```go
type Block struct {
    Name      string  // Human-readable block name
    Cron      string  // Standard cron expression (minute hour dom month dow)
    Duration  int     // Block duration in minutes
    ChannelID string  // Target Tunarr channel ID
    Priority  int     // Higher priority blocks override overlapping blocks
    Filter    Filter  // Content selection criteria
}
```

**Program (`internal/tunarr/models.go`):**

```go
type Program struct {
    ID        string   // Tunarr program ID
    Title     string   // Title of movie/episode
    Duration  int64    // Duration in milliseconds
    Year      int      // Release year
    Rating    string   // Content rating (PG, PG-13, etc.)
    Genres    []string // Genre list
    Type      string   // "movie", "episode", or "track"
    ShowTitle string   // For episodes: show name
    Season    int      // For episodes: season number
    Episode   int      // For episodes: episode number
}
```

## Development Context

### Current State

- **Complete:** Basic CLI structure, Tunarr client skeleton, scheduling engine with cron and filtering, interactive TUI for block editing
- **In Progress:** Tunarr API endpoint verification (see TODO.md Phase 1)
- **Planned:** Series-based scheduling (sequential episode progression), episode state tracking with SQLite, separate scheduler file architecture

### Key Architectural Decisions

**Upcoming Major Changes (see TODO.md):**

1. **Config Separation:** Split app config (tunarr connection, logging) from scheduler config (blocks/rules)
   - Enable `--scheduler <file>` CLI parameter for different scheduling scenarios
   - Support multiple scheduler files for different programming strategies

2. **Series-Based Scheduling:** New block type for sequential episode progression
   - Track current episode per series using SQLite persistence
   - Support mixed series and filter-based blocks
   - Implement fallback logic when series completes (redistribute time or use filler)

3. **Enhanced Tunarr Integration:** Verify and implement actual API endpoints
   - Current endpoints are placeholders and need testing against real Tunarr instance
   - See `docs/TUNARR_API_RESEARCH.md` for API investigation notes

### API Client Considerations

- The Tunarr client uses placeholder endpoints that may not match the actual Tunarr API
- Always verify endpoint paths and payload structures when working on API integration
- Authentication uses optional `X-API-Key` header (not all Tunarr instances require it)
- Client has 10-second timeout for all requests

### Testing Notes

- Most tests are in `internal/scheduler/filter_test.go` and `internal/tunarr/client_test.go`
- Integration tests should use mocked Tunarr API responses
- When adding new features, follow existing test patterns with table-driven tests

## Configuration Reference

Configuration structure in `.schedularr.yaml`:

```yaml
tunarr:
  url: "http://localhost:8000"  # Tunarr API base URL
  api_key: ""                   # Optional API key

log:
  level: "info"    # debug, info, warn, error
  format: "text"   # text or json

scheduler:
  blocks:
    - name: "Morning Cartoons"
      cron: "0 6 * * *"         # Daily at 6 AM
      duration: 240             # Minutes
      channel_id: "channel-1"   # From Tunarr
      priority: 10              # Higher = more important
      filter:
        genres: ["Animation", "Family"]
        max_duration: 30        # Max episode length in minutes
        ratings: ["TV-Y", "TV-G"]
        year_from: 2000
```

## Code Patterns

### Error Handling

- Errors are wrapped with context using `fmt.Errorf("context: %w", err)`
- CLI commands handle errors at the top level and exit with status 1
- API client returns detailed errors with HTTP status codes

### Cron Parsing

- Uses `github.com/robfig/cron/v3` with standard 5-field format
- Parser initialized with: `cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)`
- Cron validation happens in the engine during schedule generation

### Configuration Loading

- Viper searches for `.schedularr.yaml` in home directory and current directory
- Default values set in `internal/config/config.go:New()`
- Config can be overridden with `--config` flag on any command

## Coding Standards

### Code Style

Follow standard Go conventions plus project-specific standards from the athena project:

**Package Comments:**

```go
// Package scheduler provides the core scheduling engine for Schedularr.
package scheduler
```

**Exported Functions:**

```go
// NewEngine creates a new scheduling engine with the given parameters.
// The logger parameter is optional; if nil, slog.Default() will be used.
func NewEngine(client *tunarr.Client, blocks []Block, store StateStore, logger *slog.Logger) *Engine
```

**Structured Logging:**

```go
// ✅ GOOD - Use slog with snake_case fields
logger.Info("schedule generated",
    "block_name", block.Name,
    "program_count", len(programs),
    "duration_minutes", duration)

// ❌ BAD - Don't use log.Printf
log.Printf("Generated %d programs for %s", len(programs), block.Name)
```

**Error Wrapping:**

```go
// ✅ GOOD - Wrap errors with context
if err != nil {
    return fmt.Errorf("failed to fetch library %s: %w", libID, err)
}

// ❌ BAD - Don't return raw errors
if err != nil {
    return err
}
```

### Complexity Limits

Enforced by golangci-lint (.golangci.yml):

- Cyclomatic complexity: max 15
- Cognitive complexity: max 20
- Nesting depth: max 5
- Function parameters: max 5
- Function return values: max 3

**Refactoring strategies when exceeding limits:**

- Extract helper functions for complex logic
- Use early returns to reduce nesting
- Split large functions into smaller focused functions
- Use table-driven approaches for multiple similar cases

### Blocked Packages

Never use these packages (enforced by depguard):

- `github.com/pkg/errors` → Use `fmt.Errorf` with `%w`
- `github.com/sirupsen/logrus` → Use `log/slog`
- `crypto/md5`, `crypto/sha1` → Security vulnerabilities
- `io/ioutil` → Deprecated, use `os` and `io` packages
- `gopkg.in/yaml.v1`, `gopkg.in/yaml.v2` → Use `gopkg.in/yaml.v3`

### Testing Standards

**Table-Driven Tests:**

```go
func TestFunction(t *testing.T) {
    tests := []struct {
        name    string
        input   InputType
        want    OutputType
        wantErr bool
    }{
        {
            name: "descriptive test case",
            input: InputType{/* ... */},
            want: OutputType{/* ... */},
            wantErr: false,
        },
        // more cases
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := Function(tt.input)
            if (err != nil) != tt.wantErr {
                t.Errorf("error = %v, wantErr %v", err, tt.wantErr)
                return
            }
            if !reflect.DeepEqual(got, tt.want) {
                t.Errorf("got %v, want %v", got, tt.want)
            }
        })
    }
}
```

**Test Coverage Goals:**

- Target: >80% coverage across all packages
- Current status: Run `go test -cover ./...` to check
- Priority areas: scheduler engine, filter logic, state management, API client

### Commit Message Format

Use [Conventional Commits](https://www.conventionalcommits.org/):

```text
<type>(<scope>): <subject>

<body>

<footer>
```

**Types:** feat, fix, docs, style, refactor, test, chore

**Example:**

```text
feat(scheduler): add episode skip functionality

Allow users to skip specific episodes in series blocks.

- Add SkipEpisodes field to SeriesConfig
- Update planSeriesBlock to check skip list
- Add tests for episode skipping

Closes #123
```

## Dependencies

Key external dependencies:

- `github.com/spf13/cobra` - CLI framework
- `github.com/spf13/viper` - Configuration management
- `github.com/robfig/cron/v3` - Cron expression parsing and scheduling
- `github.com/charmbracelet/bubbletea` - Terminal UI framework
- `github.com/charmbracelet/bubbles` - TUI components
- `gopkg.in/yaml.v3` - YAML parsing

Go version: 1.25.5 (specified in go.mod)
