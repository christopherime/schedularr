# Schedularr - Context

## What Is It

Cron-based content scheduling for [Tunarr](https://tunarr.com) TV channels. Generates and applies TV channel schedules from rule-based blocks (time slot + cron + channel + content filter), including series-based sequential episode progression. Runs as a CLI or as a long-lived `serve` process with HTTP API, web UI, and cron loop.

Version: v0.5.5 (2026-08-31).

## Tech Stack

| Layer | Technology |
|-------|-----------|
| Language | Go 1.27 |
| CLI | Cobra |
| HTTP API | chi router + oapi-codegen (from api/openapi.yaml) |
| Config | CUE schemas (cmd/schema/), no Viper |
| Database | SQLite (sqlx + mattn/go-sqlite3, CGO) |
| HTTP Client | Resty with retry (internal/httpclient) |
| Scheduling | robfig/cron |
| Metadata | TMDB v3, TheTVDB v4 clients |
| Web UI | Hugo (static) + Alpine.js (vendored) + cronstrue (vendored) |
| Codegen | oapi-codegen (Go handlers), openapi-typescript (TS types) |
| CI | GitHub Actions (lint, security, tests, drift, Docker) |
| Container | Multi-stage Dockerfile (Alpine, CGO=musl) |

## External Dependencies

### Go (go.mod)

**Direct:**

- `cuelang.org/go` — config schema validation
- `github.com/getkin/kin-openapi` — OpenAPI spec parsing
- `github.com/go-chi/chi/v5` — HTTP router
- `github.com/go-playground/validator/v10` — request validation
- `github.com/go-resty/resty/v2` — HTTP client
- `github.com/golang-migrate/migrate/v4` — DB migrations
- `github.com/google/uuid` — block IDs
- `github.com/jedib0t/go-pretty/v6` — CLI tables
- `github.com/jmoiron/sqlx` — SQLite queries
- `github.com/mattn/go-sqlite3` — SQLite driver (CGO)
- `github.com/oapi-codegen/oapi-codegen/v2` — Go handler generation
- `github.com/oapi-codegen/runtime` — OpenAPI runtime helpers
- `github.com/patrickmn/go-cache` — in-memory cache
- `github.com/prometheus/client_golang` — metrics
- `github.com/robfig/cron/v3` — cron scheduling
- `github.com/samber/lo` — functional slice helpers
- `github.com/spf13/cobra` — CLI framework
- `gopkg.in/yaml.v3` — YAML parsing

**Web (web/package.json):**

- `typescript@5.9.3` — type checking
- `openapi-typescript@7.13.0` — TS types from OpenAPI
- `@types/node@22.20.1` — Node types

### Local Libraries

None. This project is self-contained. No `replace` directives in go.mod.

## Naming Conventions

- Go: CamelCase exports, role-based filenames (`engine.go`, `filter.go`).
- Config/CLI: kebab-case keys mirrored by CLI flags.
- Tests: `TestComponent_Scenario`, table-driven.
- Commits: `type(scope): imperative summary` (Conventional Commits).
- Log keys: snake_case.

## Configuration Reference

### config.yaml (app config)

| Key | Default | Description |
|-----|---------|-------------|
| `tunarr.url` | `${SCHEDULARR_TUNARR_URL}` | Tunarr API base URL |
| `tunarr.api_key` | `${SCHEDULARR_TUNARR_API_KEY}` | Tunarr API key |
| `tunarr.timeout` | `30s` | Request timeout |
| `database` | `schedularr.db` | SQLite DB path |
| `scheduler_file` | `scheduler.yaml` | Import file path |
| `cron_interval` | `6h` | Serve cron loop interval |
| `log.level` | `info` | debug/info/warn/error |
| `log.format` | `text` | text/json |
| `log.timezone` | `Local` | IANA timezone |
| `maintenance.history_retention` | `168h` | History retention (7 days) |
| `maintenance.cleanup_enabled` | `true` | Enable background cleanup |
| `api.listen` | `:8484` | HTTP listen address |
| `api.token` | `""` | Bearer token (env var preferred) |
| `api.insecure_no_auth` | `false` | Skip auth (dev only) |

Schema source: `cmd/schema/config.cue`.

### scheduler.yaml (first-run import only)

Blocks define: name, cron, duration, channel_id, priority, type (filter/series), filter criteria, series config, fallback, filler. After bootstrap, blocks are managed via the HTTP API. Schema: `cmd/schema/scheduler.cue`.

## Development Workflow

```
make build    → compile
make test     → race tests
make lint     → lint + security
make validate → CUE config validation
make fmt      → go fmt
make generate → OpenAPI codegen
make web      → types + tsc + Hugo build
make docker-build VERSION=x.y.z → container
```

Run: `go run main.go [cmd]` or `./bin/schedularr [cmd]`.

## Important Rules

1. **Lean codebase** — delete superseded code in the same change.
2. **Feature first** — implement, accompany with tests, green `make test` before commit.
3. **Docs current** — update docs in the same commit as code changes.
4. **No local lib replace directives** — project is self-contained.
5. **Blocked packages** — no `pkg/errors`, `logrus`, `crypto/md5`, `crypto/sha1`, `io/ioutil`, `yaml.v1/v2`.
6. **scheduler.yaml is import-only** — post-bootstrap, use the HTTP API.
7. **Token via env var** — `SCHEDULARR_API_TOKEN` always wins over config.
8. **Web UI vendored** — no CDN; Alpine.js and cronstrue vendored.
9. **CGO required** — `mattn/go-sqlite3` means Alpine musl, not distroless.
10. **OpenAPI is the contract** — edit `api/openapi.yaml`, regenerate with `make generate` and `make web-types`.
