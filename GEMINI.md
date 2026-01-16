# Schedularr Context

## Project Overview

**Schedularr** is an intelligent automation tool for scheduling programming on [Tunarr](https://tunarr.com) TV channels. It enables users to define complex scheduling rules using cron syntax, filter content based on metadata (genres, ratings, years, etc.), and manage series progression (e.g., sequential episode scheduling) using a local SQLite database.

The project allows for "Set It and Forget It" channel management, ensuring fresh content rotation without manual intervention. It supports integration with Radarr, Sonarr, and Jellyfin for advanced filtering and metadata synchronization.

## Technology Stack

- **Language:** Go 1.25.5+
- **CLI Framework:** [Cobra](https://github.com/spf13/cobra)
- **TUI Framework:** [Bubble Tea](https://github.com/charmbracelet/bubbletea)
- **Configuration:** [Viper](https://github.com/spf13/viper) (loading) & [CUE](https://cuelang.org/) (validation)
- **Database:** SQLite (via `database/sql` + `github.com/mattn/go-sqlite3`)
- **HTTP Client:** [Resty](https://github.com/go-resty/resty) with retry logic
- **Scheduling:** [robfig/cron](https://github.com/robfig/cron)

## Build & Run Instructions

The project uses `make` for common tasks.

- **Build Binary:**

    ```bash
    make build
    # Output: ./bin/schedularr
    ```

- **Run (Dev):**

    ```bash
    go run main.go [command]
    ```

- **Run Tests:**

    ```bash
    make test        # Unit tests with race detection
    make e2e-test    # End-to-end tests (requires Docker)
    ```

- **Lint & Format:**

    ```bash
    make lint        # Runs golangci-lint, gosec, govulncheck
    make fmt         # Formats code
    ```

- **Validate Configs:**

    ```bash
    make validate    # Validates YAML configs against CUE schemas
    ```

- **Docker:**

    ```bash
    make docker-build
    ```

## Architecture & Code Structure

The project follows a standard Go project layout with a strong separation of concerns, heavily influenced by the "Athena" project patterns.

### Key Directories

- `cmd/`: Application entry points (CLI commands).

- `internal/`: Private application code.
  - `scheduler/`: Core scheduling logic (Engine, Filter, History).
  - `tunarr/`: Tunarr API client.
  - `store/`: SQLite persistence layer (Series state).
  - `config/`: Configuration loading and parsing.
  - `tui/`: Terminal User Interface implementation.
- `configs/`: Example configurations.
- `cmd/schema/`: **CUE schemas** for configuration validation.
- `e2e/`: End-to-end test suite (Docker Compose, shell scripts).

### Core Components

1. **Scheduling Engine (`internal/scheduler`):**
    - **Engine:** Orchestrates the planning process.
    - **Filter:** Applies rules (genre, rating, etc.) to select content.
    - **History:** Tracks recently played items to avoid repetition (default 7-day window).

2. **State Store (`internal/store`):**
    - Uses SQLite to track series progression (`season`, `episode`) for sequential blocks.
    - Ensures users can "pick up where they left off".

3. **Tunarr Client (`internal/tunarr`):**
    - Handles all communication with the Tunarr API.
    - Implements exponential backoff and retry logic.

## Development Conventions

- **Configuration Validation:**
  - All configuration files (`config.yaml`, `scheduler.yaml`) **MUST** be validated against CUE schemas defined in `cmd/schema/`.
  - Use `make validate` to ensure correctness.

- **Logging:**
  - Use `log/slog` for structured logging.
  - Logs should be machine-parseable (JSON in production).

- **Error Handling:**
  - Always wrap errors using `fmt.Errorf("context: %w", err)`.
  - Do not just return the error; add context (e.g., "failed to fetch channels").

- **Testing:**
  - Prefer **table-driven tests** for logic.
  - Use `e2e/` for integration tests involving the full stack (Tunarr mock).

- **Code Style:**
  - Strict linting rules (cyclomatic complexity < 15).
  - No deprecated packages (`io/ioutil`, `pkg/errors`).

## Key CLI Commands

- `schedularr generate`: Generates a schedule (add `--apply` to push to Tunarr).

- `schedularr channels`: Lists available channels.
- `schedularr tui`: Opens the interactive TUI.
- `schedularr validate [file]`: Validates a config file.
