# Repository Guidelines

## Project Structure & Module Organization

Entry point sits in `cmd/schedularr` and `main.go`, while `internal/scheduler`, `cli`, `tui`, `tunarr`, and `config` hold scheduling logic, user surfaces, API bindings, and configuration plumbing. Config samples live in `configs/` with CUE schemas in `cmd/schema/`; deterministic fixtures belong in `testdata/`, docs in `docs/`, and dockerized acceptance assets in `e2e/`.

## Build, Test, and Development Commands

- `make build`: compile a stripped `bin/schedularr`; run `make clean` if artifacts linger.
- `make test`: run `go test -race -cover ./...`; use `go test ./internal/scheduler/...` for focused loops.
- `make lint`: run `golangci-lint`, `gosec`, and `govulncheck`.
- `make validate`: `cue vet` the files in `configs/` to keep published samples legal.
- `make e2e-up && make e2e-test && make e2e-down`: start docker-compose, execute `e2e/test.sh`, then tear down; `make e2e-clean` wipes volumes.

## Coding Style & Naming Conventions

Target Go 1.25.5; run `go fmt ./...` (or `make fmt`) before committing. Use Go's default tabs, CamelCase exported identifiers, and doc comments whose first word matches the symbol. Keep file names role-based (`engine.go`, `filter.go`, `model.go`) and align new ones with their siblings. YAML configs use kebab-case keys, mirrored by CLI flags, so keep casing consistent across both surfaces.

## Testing Guidelines

Place unit tests beside their package in `*_test.go` files with `TestComponent_Scenario` naming, and favor table-driven cases for scheduler filters or CLI parsing. Pull requests must pass `make test`; also watch `go test -cover ./...` so coverage targets in `TODO.md` do not slide, and keep golden data in `testdata/`. For cross-service behaviour, run `make e2e-up`, execute `make e2e-test`, and shut things down immediately with `make e2e-down` or `make e2e-clean`.

## Commit & Pull Request Guidelines

History favors `type(scope): imperative summary` (e.g., `feat(tui): …`), so keep commits single-purpose and reference issues with `Fixes #ID` or `Refs #ID`, noting migrations or schema updates in the body. Pull requests should outline the problem, solution, and validation steps (commands, screenshots, config diffs) and mention new dependencies or config keys. Run `make build`, `make test`, and `make lint` before requesting review.

## Security & Configuration Tips

Never commit real Tunarr credentials — keep examples redacted in `configs/` and store machine-specific overrides in `~/.schedularr.yaml`. Run `make validate` around schema changes so CUE stays synced with the samples. Docker resources assume localhost; if you expose ports externally, audit `docker-compose.yml` env values, rotate keys, and delete stray environment files before merging.
