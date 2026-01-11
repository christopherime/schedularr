# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Schedularr is a Go application that automates content scheduling for Tunarr TV channels. It uses cron-based recurring blocks with advanced filtering to intelligently program channel content without manual intervention.

## Common Commands

### Build & Run

```bash
# Build the binary
go build -o schedularr cmd/schedularr/main.go

# Production build with optimizations
go build -ldflags="-s -w" -o schedularr cmd/schedularr/main.go

# Run the application
./schedularr --help
./schedularr channels
./schedularr generate --apply
./schedularr tui
```

### Testing

```bash
# Run all tests
go test ./...

# Run with coverage
go test -cover ./...

# Run specific package tests
go test ./internal/scheduler/...
```

### Code Quality

```bash
# Format code
go fmt ./...

# Lint (uses .golangci.yml config)
golangci-lint run

# Security scan
gosec ./...

# Vulnerability check
govulncheck ./...
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

## Dependencies

Key external dependencies:

- `github.com/spf13/cobra` - CLI framework
- `github.com/spf13/viper` - Configuration management
- `github.com/robfig/cron/v3` - Cron expression parsing and scheduling
- `github.com/charmbracelet/bubbletea` - Terminal UI framework
- `github.com/charmbracelet/bubbles` - TUI components
- `gopkg.in/yaml.v3` - YAML parsing

Go version: 1.25.5 (specified in go.mod)
