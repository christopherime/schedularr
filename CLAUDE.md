# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Schedularr is a Go application that automates TV channel programming for [Tunarr](https://tunarr.com). It uses cron-based scheduling to generate and apply content schedules based on user-defined rules (blocks), with advanced filtering by genre, rating, year, duration, and title patterns.

## Development Commands

```bash
make build          # Compile binary to ./bin/schedularr
make test           # Run tests with race detector
make lint           # Run golangci-lint + gosec + govulncheck
make clean          # Remove build artifacts
make validate       # Validate YAML configs with CUE schemas
make fmt            # Format Go code
make deps           # Download and tidy dependencies

# Run a single test
go test -race -v ./internal/scheduler -run TestFilterPrograms

# Run tests in a specific package
go test -race -v ./internal/external/tunarr/...

# Run the binary
./bin/schedularr --config config.yaml generate --apply
```

## Architecture

```txt
cmd/                          # CLI commands (Cobra)
├── channels.go               # List Tunarr channels
├── content_sources.go        # Manage Radarr/Sonarr/Jellyfin
├── generate.go               # Generate & apply schedules
├── run.go                    # Daemon mode with cron
├── tui.go                    # Launch terminal UI
├── validate.go               # Config validation
├── state.go                  # Series state management
└── schema/                   # CUE schemas for validation
    ├── config.cue
    └── scheduler.cue
internal/
├── config/                   # Viper-based config management
├── scheduler/                # Core scheduling engine
│   ├── engine.go             # GenerateForTimeRange, PlanBlock, PlanSeriesBlock
│   ├── filter.go             # Genre/rating/year/duration/title filters
│   ├── history.go            # 7-day schedule history (prevent repeats)
│   └── types.go
├── external/                 # API clients
│   ├── tunarr/               # Tunarr REST API client
│   ├── radarr/               # Radarr client (movies)
│   ├── sonarr/               # Sonarr client (TV)
│   └── jellyfin/             # Jellyfin sync
├── store/                    # SQLite state persistence
│   └── migrations/           # DB migrations
├── tui/                      # Terminal UI (Bubble Tea)
├── cache/                    # In-memory caching
├── cronbuilder/              # Cron expression builder
└── httpclient/               # HTTP client with retry
```

**Data Flow:** Config → Scheduling Engine → Tunarr API → Channel Programming

**Key Integration Points:**

- **Tunarr:** Primary target - channels, programs, libraries
- **Radarr/Sonarr:** Optional - filter by content availability
- **Jellyfin:** Optional - refresh Live TV guide after schedule updates

## CLI Commands

```bash
schedularr generate [--apply]       # Generate schedule (--apply to push to Tunarr)
schedularr run [--daemon]           # Start daemon with cron-based execution
schedularr channels                 # List Tunarr channels
schedularr tui                      # Launch interactive block editor
schedularr validate <file>          # Validate config file
schedularr config generate [file]   # Generate config template
schedularr scheduler init [file]    # Create scheduler config
```

## Coding Standards

**Error Handling:** Always wrap with context:

```go
if err != nil {
    return fmt.Errorf("failed to query tunarr: %w", err)
}
```

**Logging:** Structured JSON logging with `slog`. Key naming uses snake_case.

**Linting Limits:**

- Cyclomatic complexity: max 15
- Cognitive complexity: max 20
- Nesting depth: max 5
- Function results: max 3
- Arguments: max 5

**Blocked Packages:** `github.com/pkg/errors`, `logrus`, `crypto/md5`, `crypto/sha1`, `io/ioutil`, `gopkg.in/yaml.v1`, `gopkg.in/yaml.v2`

## Configuration

Two YAML config files, both validated by CUE schemas:

- **`config.yaml`** - App settings (Tunarr URL, API keys, logging)
- **`scheduler.yaml`** - Scheduling blocks with cron expressions and filters

**Scheduling Block Structure:**

```yaml
blocks:
  - name: "Morning Cartoons"
    cron: "0 6 * * *"           # Daily at 6:00 AM
    duration: 240               # 4 hours (minutes)
    channel_id: "channel-1"
    priority: 10                # Higher = more important in conflicts
    filter:
      genres: ["Animation"]
      ratings: ["TV-Y", "TV-G"]
      year_from: 2000
      max_duration: 30
```

## Commit Messages

Follow [Conventional Commits](https://www.conventionalcommits.org/) specification.
