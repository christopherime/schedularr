# Repository Guidelines

## Project Structure & Module Organization

- `cmd/schedularr/main.go` is the CLI entry point.
- `internal/cli/` holds Cobra command implementations (`root`, `channels`, `generate`, `tui`).
- `internal/scheduler/` contains the core scheduling engine and filter logic.
- `internal/tunarr/` is the Tunarr API client and models.
- `internal/tui/` contains the Bubble Tea terminal UI.
- `configs/config.yaml` is the example configuration; docs live in `docs/`.
- Tests are alongside code, e.g. `internal/scheduler/filter_test.go`.

## Build, Test, and Development Commands

- `go build -o schedularr cmd/schedularr/main.go` builds the binary.
- `go build -ldflags="-s -w" -o schedularr cmd/schedularr/main.go` builds an optimized release binary.
- `./schedularr --help` runs the CLI after a local build.
- `go test ./...` runs the full test suite.
- `go test -cover ./...` runs tests with coverage.
- `go fmt ./...` formats Go code.
- `golangci-lint run` runs lint checks (uses repo config).
- `gosec ./...` and `govulncheck ./...` run security scans.

## Coding Style & Naming Conventions

- Follow standard Go formatting (`go fmt`) and idioms.
- Package names are lowercase; exported identifiers use `CamelCase`.
- Tests use `_test.go` filenames and table-driven patterns when practical.
- Prefer contextual error wrapping (e.g., `fmt.Errorf("context: %w", err)`).

## Testing Guidelines

- Primary tests live in `internal/scheduler/` and `internal/tunarr/`.
- Name tests after behavior, e.g., `TestFilterPrograms_ExcludesShort`.
- For new features, add unit tests and mock Tunarr API responses when needed.

## Commit & Pull Request Guidelines

- Commit messages follow Conventional Commits: `type(scope): summary`.
  Example: `feat(tunarr): verify schedule endpoints`.
- PRs should include a short description, testing notes (commands + results),
  and screenshots when changing the TUI.
- Link related issues if applicable and update docs when behavior changes.

## Configuration & Security Notes

- Config is loaded from `~/.schedularr.yaml` or `./.schedularr.yaml`.
- Use `configs/config.yaml` as a starting point; do not commit secrets.
- Verify Tunarr endpoint behavior against `docs/TUNARR_API_RESEARCH.md` before
  changing client integration.
