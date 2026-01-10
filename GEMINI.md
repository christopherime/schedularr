# Schedularr Project Context

## Project Overview

**Schedularr** is a CLI-based Go application designed to automate programming schedules for Tunarr TV channels. It interfaces with the Tunarr API to fetch content, filter it based on user-defined rules, and update channel schedules automatically.

## Architecture

The project follows a standard Go project layout:

-   **`cmd/schedularr`**: Contains the `main.go` entry point.
-   **`internal/`**: Contains the application logic, hidden from external import.
    -   **`internal/cli`**: Implements the CLI commands (using `cobra`).
    -   **`internal/config`**: Configuration management (using `viper`).
    -   **`internal/scheduler`**: Core logic for generating schedules using cron expressions and content filtering.
    -   **`internal/tunarr`**: Client library for the Tunarr REST API.

## Key Technologies

-   **Language**: Go 1.25.5
-   **CLI Framework**: `github.com/spf13/cobra`
-   **Configuration**: `github.com/spf13/viper` (YAML support)
-   **Scheduling**: `github.com/robfig/cron/v3`

## Development Conventions

-   **Dependency Management**: Modules are managed via `go.mod`.
-   **Configuration**: Configuration is loaded from `.schedularr.yaml` (defaulting to home or current directory). Structure is defined in `internal/config`.
-   **Error Handling**: Errors are propagated up to the CLI layer for logging/display.
-   **Testing**: Unit tests are located alongside code files (e.g., `internal/scheduler/filter_test.go`).

## Build and Run

### Build
```bash
go build -o schedularr cmd/schedularr/main.go
```

### Run
```bash
./schedularr --help
./schedularr channels
./schedularr generate --config config.yaml.example
```

### Test
```bash
go test ./...
```

## Current Status & Roadmap

-   **Implemented**: Basic structure, CLI skeleton, Tunarr client (basic auth/channels), Scheduling engine (filtering/cron).
-   **Pending**:
    -   Verification of Tunarr API endpoints (specifically for content fetching and schedule updates).
    -   Persistence layer (database) for duplicate prevention.
    -   Docker support.
    -   Integration with external content sources (Radarr/Sonarr).

Refer to `TODO.md` for a detailed task list.
