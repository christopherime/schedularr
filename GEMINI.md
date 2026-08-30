# Schedularr Context

## Project Overview

**Schedularr** is an intelligent automation tool for scheduling programming on [Tunarr](https://tunarr.com) TV channels. It enables users to define complex scheduling rules using cron syntax, filter content based on metadata (genres, ratings, years, etc.), and manage series progression (e.g., sequential episode scheduling) using a local SQLite database.

The project allows for "Set It and Forget It" channel management, ensuring fresh content rotation without manual intervention. Tunarr is the sole integration; content availability filtering is driven entirely by Tunarr's own library data.

There is no interactive UI: Schedularr is a CLI (one-shot commands like `generate`, `state`) plus, via `schedularr serve`, a long-lived process hosting an HTTP API and a cron scheduling loop. A former Bubble Tea TUI (`schedularr tui`, block-editing forms) was removed; blocks are now managed through the `/api/v1/blocks` HTTP API or `scheduler.yaml` first-run import.

## Technology Stack

- **Language:** Go 1.27
- **CLI Framework:** [Cobra](https://github.com/spf13/cobra)
- **HTTP API:** [chi](https://github.com/go-chi/chi) router with a handler layer generated from `api/openapi.yaml` via [oapi-codegen](https://github.com/oapi-codegen/oapi-codegen) (`internal/api/gen`)
- **Configuration:** [CUE](https://cuelang.org/) for both schema validation and default values (`cmd/schema/`, `internal/cueconfig`) -- no Viper
- **Database:** SQLite (via `github.com/jmoiron/sqlx` + `github.com/mattn/go-sqlite3`)
- **HTTP Client:** [Resty](https://github.com/go-resty/resty) with retry logic (`internal/httpclient`, used by the Tunarr client in `internal/external/tunarr` and the metadata providers in `internal/metadata`)
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

The project follows a standard Go project layout with a strong separation of concerns.

### Key Directories

- `cmd/`: Application entry points (CLI commands) plus `cmd/schema/` (embedded CUE schemas).

- `internal/`: Private application code.
  - `scheduler/`: Core scheduling logic (Engine, Filter, History).
  - `external/tunarr/`: Tunarr API client.
  - `metadata/`: Show metadata providers (`tmdb/`, `tvdb/`) plus the canonical genre vocabulary (`normalize.go`); see `docs/metadata.md`.
  - `store/`: SQLite persistence layer (blocks, series state, schedule history).
  - `config/`: Configuration loading and typed accessors over `internal/cueconfig`.
  - `cueconfig/`: CUE schema compilation, validation, and default-config generation.
  - `service/`: The schedule generate/apply workflow shared by the CLI (`cmd/generate.go`) and the HTTP API.
  - `api/`: HTTP API -- router, handlers, RFC 7807 problem responses, and `internal/api/gen` (generated from `api/openapi.yaml`; edit only via `make generate`).
  - `blockio/`: `scheduler.yaml` parse/render plus first-run store import (`blockio.Bootstrap`).
  - `cache/`: In-memory content caching used by `service.Runner`.
  - `metrics/`: Prometheus metrics registration, served at `GET /metrics` on `serve`'s own HTTP listener.
- `configs/`: Example configuration (`config.yaml`).
- `cmd/schema/`: **CUE schemas** for configuration validation.
- `e2e/`: End-to-end test suite (Docker Compose, shell scripts).

### Core Components

1. **Scheduling Engine (`internal/scheduler`):**
    - **Engine:** Orchestrates the planning process (`GenerateForTimeRange`, `PlanBlock`, `PlanSeriesBlock`).
    - **Filter:** Applies rules (genre, rating, year, duration, title, tags) to select content.
    - **History:** Tracks recently played items to avoid repetition. The window defaults to 7 days but is configurable via `maintenance.history_retention` (threaded through `service.NewRunner` -> `scheduler.EngineOptions.HistoryWindow`); it also bounds how far back `GET /history?days=N` can return data.

2. **State Store (`internal/store`):**
    - Uses SQLite to track series progression (`season`, `episode`), scheduling blocks, and schedule history.
    - Ensures users can "pick up where they left off".
    - Opened with `_busy_timeout=5000&_journal_mode=WAL` (see `internal/store/sqlite.go`), since `serve` is a long-lived writer that may share the database file with concurrent CLI invocations.

3. **Tunarr Client (`internal/external/tunarr`):**
    - Handles all communication with the Tunarr API.
    - Implements exponential backoff and retry logic via `internal/httpclient`.

4. **HTTP API (`internal/api`, hosted by `schedularr serve`):**
    - Blocks CRUD and YAML import/export, schedule generate/apply/get, schedule history, series state, Tunarr channels, and status -- see `api/openapi.yaml` for the full contract.
    - Also serves `/healthz`, `/readyz`, `/metrics`, and `/openapi.json` outside the versioned `/api/v1/*` surface.

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

- `schedularr generate [--apply --yes]`: Generates a schedule (add `--apply --yes` to push it to Tunarr).
- `schedularr serve [--listen :8484]`: Runs the HTTP API server and the cron scheduling loop in one process.
- `schedularr channels`: Lists available Tunarr channels.
- `schedularr validate <file>`: Validates a config or `scheduler.yaml` file.
- `schedularr config generate [file]` / `schedularr scheduler init [file]`: Write a config or `scheduler.yaml` template from the CUE schema defaults.
- `schedularr state <export|import|reset|set|list|backup>`: Manage series progression state.

There is no `schedularr tui` command; block management happens through the `/api/v1/blocks` HTTP API (`schedularr serve`) or the `scheduler.yaml` first-run import.
