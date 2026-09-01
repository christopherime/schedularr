# AGENTS.md

This file provides guidance to AI coding agents working in the Schedularr repository.

## Project Overview

Schedularr is a Go application that automates TV channel programming for [Tunarr](https://tunarr.com). It uses cron-based scheduling to generate and apply content schedules based on user-defined rules (blocks), with advanced filtering by genre, rating, year, duration, and title patterns, plus series-based sequential episode progression.

There is no interactive TUI. Schedularr is a CLI (one-shot commands) plus a long-lived `serve` process hosting an HTTP API, a web UI (Hugo + Alpine.js), and a cron scheduling loop. Blocks are managed via the `/api/v1/blocks` HTTP API or the `scheduler.yaml` first-run import file.

**Current version:** v0.5.5 (2026-08-31). Latest release was the token-at-rest encryption security patch.

**Documentation site:** <https://christopherime.github.io/schedularr/>

## Operator Rules (non-negotiable)

1. **Lean codebase.** Only live code. Superseded code, config, and deps are deleted in the same change — no legacy, no aliases, no dead exports.
2. **Feature first.** Implementation leads; tests accompany and `make test` stays green before commit, but test ceremony never blocks delivery.
3. **Docs always current.** README, AGENTS.md, CLAUDE.md, GEMINI.md, and `docs/` change in the same commit as the code they describe. Documentation drift is a defect.
4. **Skills.** Use `impeccable` for front-end/UI work and `stop-slop` to clean generated prose. Load relevant Go skills (`golang`, `golang-testing`, `golang-error-handling`, etc.) before touching Go code.

## Development Commands

```bash
make build            # Compile binary to ./bin/schedularr (depends on web-presence)
make test             # Run tests with race detector: go test -race -cover ./...
make lint             # golangci-lint + gosec + govulncheck + web-check + web-test
make clean            # Remove build artifacts
make validate         # Validate YAML configs with CUE schemas
make fmt              # Format Go code (go fmt ./...)
make deps             # Download and tidy dependencies
make generate         # Regenerate OpenAPI server code from api/openapi.yaml
make docker-build     # Build Docker image (VERSION=... to stamp version)
make web              # Full web build: types + type-check + Hugo site to web/public
make web-types        # Regenerate web/assets/ts/gen/types.d.ts from api/openapi.yaml
make web-check        # Type-check web TS (skips if npm absent)
make web-test         # Run web unit tests (skips if node absent)
```

Focused test commands:

```bash
go test -race -v ./internal/scheduler -run TestFilterPrograms
go test -race -cover ./internal/external/tunarr/...
```

Run the binary:

```bash
./bin/schedularr --config config.yaml generate --apply --yes
./bin/schedularr serve --listen :8484
```

## Architecture

### Directory Structure

```txt
main.go                           # Entry point — calls cmd.Execute()
cmd/                              # CLI commands (Cobra)
├── root.go                       # Root command, global --config flag
├── generate.go                   # Generate and apply schedules
├── serve.go                      # HTTP API server + cron scheduling loop
├── channels.go                   # List Tunarr channels
├── validate.go                   # Config validation
├── config.go                     # Config generation from CUE defaults
├── scheduler.go                  # scheduler.yaml init (import-file authoring)
├── state.go                      # Series state management (export/import/reset)
├── health.go                     # Health check command
├── logger.go                     # slog setup
└── schema/                       # CUE schemas (embedded into binary)
    ├── embed.go                  # go:embed all:*.cue
    ├── config.cue                # config.cue schema (single source of truth)
    └── scheduler.cue             # scheduler.yaml schema

internal/
├── config/                       # Config loading + typed accessors over cueconfig
├── cueconfig/                    # CUE schema compilation, validation, defaults
├── scheduler/                    # Core scheduling engine
│   ├── engine.go                 # GenerateForTimeRange, PlanBlock, PlanSeriesBlock
│   ├── filter.go                 # Genre/rating/year/duration/title/tag filters
│   ├── history.go                # 7-day schedule history (prevent repeats)
│   ├── state.go                  # Series state tracking
│   ├── types.go                  # Block, Filter, SeriesBlock types
│   └── interfaces.go             # Store/Tunarr interfaces (for mocking)
├── external/
│   └── tunarr/                   # Tunarr REST API client (Resty-backed)
├── metadata/                     # Show metadata providers + canonical genres
│   ├── normalize.go              # NormalizeGenre/NormalizeGenres
│   ├── types.go                  # Metadata types
│   ├── tmdb/                     # The Movie Database v3 client
│   └── tvdb/                     # TheTVDB v4 client
├── store/                        # SQLite state persistence
│   ├── sqlite.go                 # DB open, connection config (WAL, busy_timeout)
│   ├── blocks.go                 # Block CRUD
│   ├── channels.go               # Applied channels tracking
│   ├── history.go                # Schedule history rows
│   ├── meta.go                   # app_meta key-value store
│   ├── migrate.go                # golang-migrate wrapper
│   └── migrations/               # SQL migrations (001-009)
├── api/                          # HTTP API
│   ├── router.go                 # chi router setup
│   ├── server.go                 # HTTP server lifecycle
│   ├── ui.go                     # Embedded web UI serving (NotFound handler)
│   ├── blocks.go                 # Blocks CRUD handlers
│   ├── schedule.go               # Generate/apply/get handlers
│   ├── history.go                # History endpoint
│   ├── state.go                  # Series state handlers
│   ├── tunarr.go                 # Tunarr passthrough (channels)
│   ├── media.go                  # Media enrichment endpoints
│   ├── importexport.go           # YAML import/export
│   ├── problem.go                # RFC 7807 problem+json responses
│   ├── media.go                  # Media lookup/enrichment
│   ├── gen/                      # Generated from api/openapi.yaml (oapi-codegen)
│   │   └── server.gen.go         # Generated handler interface
│   └── middleware/               # Auth, logging, recovery, request ID
├── service/                      # Schedule generate/apply workflow (shared by CLI + API)
│   ├── schedule.go               # Runner: orchestrate engine + store + Tunarr
│   └── media.go                  # Media enrichment service
├── blockio/                      # scheduler.yaml parse/render + first-run bootstrap
├── cache/                        # In-memory caching (go-cache wrapper)
├── httpclient/                   # HTTP client with retry (Resty wrapper)
├── problem/                      # RFC 7807 error body types
└── metrics/                      # Prometheus metrics

web/                              # Hugo web UI, embedded into binary via go:embed
├── embed.go                      # package web — //go:embed all:public, Site()
├── DESIGN.md                     # Design system: tokens, components, WCAG, Alpine
├── hugo.toml                     # Site config (via config/_default/hugo.toml)
├── package.json                  # devDeps: typescript, openapi-typescript
├── tsconfig.json                 # Strict TS, noEmit, moduleResolution bundler
├── layouts/                      # Hugo templates
│   ├── _default/baseof.html      # Shell: header/nav/footer
│   ├── partials/                 # Nav, toggles, partials
│   ├── index.html                # Dashboard
│   ├── blocks/list.html          # Blocks management
│   ├── schedule/list.html        # Schedule preview (week grid)
│   ├── series/list.html          # Series state
│   └── 404.html                  # Styled not-found
├── content/                      # Hugo section front matter
├── assets/
│   ├── css/main.css              # Single CSS file, custom properties (light/dark)
│   ├── ts/                       # API client, token, pages
│   │   ├── runtime/              # api.ts, token.ts, shell.ts, cron.ts, etc.
│   │   ├── pages/                # dashboard.ts, blocks.ts, schedule.ts, series.ts
│   │   └── gen/                  # types.d.ts (generated by make web-types)
│   └── vendor/                   # Alpine.js + cronstrue (vendored, no CDN)
├── static/                       # Favicon, brand images
└── public/                       # Hugo build output — gitignored, untracked

api/
└── openapi.yaml                  # OpenAPI 3.0.3 spec (source of truth for API contract)
    └── oapi-codegen.yaml         # Code generation config

configs/                          # Published config samples
├── config.yaml                   # Example app config (redacted credentials)
└── scheduler.yaml                # Example scheduler import file

testdata/                         # Deterministic test fixtures
├── channels/                     # Mock channel responses
├── programs/                     # Mock program lists (sitcoms, movies, cartoons)
└── configs/                      # Valid/invalid config files for tests

e2e/                              # Dockerized acceptance tests
├── docker-compose.yaml
├── test.sh
└── fixtures/                     # Test configs

docs/                             # Documentation site source (mkdocs)
├── architecture.md
├── deployment.md
├── getting-started.md
├── api-reference.md
├── design-system.md
├── metadata.md
├── roadmap.md
├── superpowers/                  # Design specs and plans
└── assets/                       # Screenshots, demo GIFs

.github/workflows/                # CI/CD
├── ci.yaml                       # Go lint/security/tests, codegen drift, web, Docker
├── release.yaml                  # Release automation
├── pages.yaml                    # Docs site deployment
└── demo.yaml                     # Demo GIF regeneration
```

### Data Flow

```txt
config.yaml + scheduler.yaml (import) → SQLite block store
    ↓
Scheduling Engine (filter/series planning)
    ↓
Tunarr API (read library → push schedule)
    ↓
Channel Programming
```

### Key Integration Points

- **Tunarr:** Sole external integration. Channels, programs, libraries, schedule push.
- **TMDB/TvDB:** Metadata enrichment (genres, ratings, images). Optional — only when configured.

### Configuration

- `config.yaml` is the live app config, validated by CUE schema in `cmd/schema/config.cue`.
- CUE schema is the single source of truth for config keys and defaults.
- `scheduler.yaml` is a **first-run import format only** — on startup, if the block store is empty, `blockio.Bootstrap` imports its blocks into the SQLite store once. After that, the file is never read again. Manage blocks via the HTTP API (`/api/v1/blocks`) post-bootstrap.
- Environment variable interpolation: `${VAR_NAME}` syntax supported in config files.
- Config path priority: `--config` flag > `SCHEDULARR_CONFIG` env > legacy locations > `~/.schedularr/config.yaml`.
- `SCHEDULARR_API_TOKEN` env var always wins over `api.token` config key.

### Web UI Architecture

- Hugo static site with Alpine.js (vendored) for interactivity.
- Embedded into the Go binary via `web/embed.go` (`go:embed all:public`).
- Served by the API router's `NotFound` handler: system routes and `/api/v1/*` win first, everything else falls back to the embedded site.
- `make build` only needs `web/public/index.html` to exist (placeholder written by `web-presence` if missing). Hugo/Node are only needed for `make web`.
- Docker build always runs the real Hugo build — never ships the placeholder.
- TypeScript types generated from `api/openapi.yaml` via `openapi-typescript`.
- No CDN: all JS (Alpine.js, cronstrue) is vendored.

### SQLite Database

- Opened with `_busy_timeout=5000&_journal_mode=WAL` (serve is a long-lived writer that may share the DB with concurrent CLI invocations).
- Migrations managed by `golang-migrate/migrate` in `internal/store/migrations/` (9 migrations).
- Tables: blocks, series_state, schedule_history, occurrence_assignments, occurrence_snapshots, applied_channels, plan_provenance, post_state_replay, app_meta.

## Coding Style

- **Target:** Go 1.27.
- **Formatting:** `go fmt ./...` (tabs). Run `make fmt` before committing.
- **Doc comments:** First word matches the symbol. Exported identifiers use CamelCase.
- **File naming:** Role-based (`engine.go`, `filter.go`, `model.go`).
- **YAML/CLI casing:** kebab-case in config files, mirrored by CLI flags.
- **Error handling:** Always wrap with context: `fmt.Errorf("failed to X: %w", err)`.
- **Logging:** Structured JSON logging with `slog`. Key naming uses snake_case.

### Linting Limits (enforced by .golangci.yml)

| Rule                  | Limit  |
| --------------------- | ------ |
| Cyclomatic complexity | max 15 |
| Cognitive complexity  | max 20 |
| Nesting depth         | max 5  |
| Function results      | max 3  |
| Arguments             | max 5  |

### Blocked Packages

`github.com/pkg/errors`, `logrus`, `crypto/md5`, `crypto/sha1`, `io/ioutil`, `gopkg.in/yaml.v1`, `gopkg.in/yaml.v2`

## Testing

- Unit tests beside their package in `*_test.go` files.
- Naming: `TestComponent_Scenario`.
- Favor table-driven cases for scheduler filters and CLI parsing.
- `make test` must pass before any commit.
- `go test -cover ./...` to track coverage.
- Golden data in `testdata/`.
- E2E: `make e2e-up && make e2e-test && make e2e-down`.

## Security

- Never commit real Tunarr credentials — keep examples redacted.
- API token set via `SCHEDULARR_API_TOKEN` env var.
- Token encrypted at rest in the web UI (AES-GCM-256, WebCrypto key in IndexedDB).
- `insecure_no_auth` is for local development only.
- Run `make validate` around schema changes.
- `govulncheck` runs in CI and `make lint`.

## CI/CD Pipeline

GitHub Actions (`ci.yaml`):

1. **Go job:** golangci-lint, gosec, govulncheck, go test -race
2. **Drift job:** regenerate OpenAPI server code and TS types, check for uncommitted diffs
3. **Web job:** `tsc --noEmit` + `node --test`
4. **Docker job:** build image (no push)

Pages workflow (`pages.yaml`): deploy docs site on docs push.
Release workflow (`release.yaml`): publish binary and Docker image.

## Commit Messages

Conventional Commits: `type(scope): imperative summary`

- Types: feat, fix, chore, docs, refactor, test, security
- Reference issues: `Fixes #ID` or `Refs #ID`
- Single-purpose commits
- Note migrations or schema updates in the body

## Docker

- Multi-stage build (4 stages): webtypes → hugo/webui → golang builder → alpine runtime
- CGO_ENABLED=1 required (mattn/go-sqlite3 is a C binding, no pure-Go build tag).
- Final image: Alpine with musl libc (must match build stage).
- Non-root user `schedularr` (uid/gid 1001).
- Health check: `wget http://127.0.0.1:8484/healthz`.
- Entrypoint: `schedularr serve --config /etc/schedularr/config.yaml`.
- Volume: `/data` for SQLite DB + configs.
- Port: 8484.

## Current State & Pending Work

See `TODO.md` for the full backlog. Key themes:

- **v0.5.7:** `/series` → `/history` page (merge tracked sequences, AS-RUN airings, apply runs)
- **v0.6.0:** Station terminology rename (filter→rotation, series→sequence)
- **v0.6.1:** Any Tunarr media kind support (movies, tags, media-kind criteria)
- **Deferred issues:** Transactional Engine.Commit, channel-scoped series_state, overflow slots desync

Open questions (Q1–Q11) in `docs/superpowers/specs/2026-08-30-v1-station-terminology-media-history-design.md` block the v1.0 streams.

## Key Files

| File                           | Purpose                                                  |
| ------------------------------ | -------------------------------------------------------- |
| `cmd/schema/config.cue`        | Single source of truth for config structure and defaults |
| `api/openapi.yaml`             | API contract — source for generated code                 |
| `internal/scheduler/engine.go` | Core scheduling logic                                    |
| `internal/scheduler/filter.go` | Content filtering rules                                  |
| `internal/store/sqlite.go`     | DB connection and schema                                 |
| `internal/service/schedule.go` | Generate/apply orchestration                             |
| `internal/api/router.go`       | HTTP router setup                                        |
| `web/DESIGN.md`                | Design system documentation                              |
| `.golangci.yml`                | Linter rules and blocked packages                        |
| `Dockerfile`                   | Multi-stage container build                              |
| `configs/config.yaml`          | Example app config                                       |
| `TODO.md`                      | Project backlog and deferred items                       |
| `CHANGELOG.md`                 | Version history                                          |
| `mkdocs.yml`                   | Docs site navigation                                     |
