# Schedularr API Server Core — Design

**Date:** 2026-08-28
**Status:** Approved (design review with operator)
**Sub-project:** 1 of 4 (API core → Web UI → Tag/metadata engine → Container/CI/GitOps deploy)

## Context

Schedularr today is a Go CLI/TUI tool that generates cron-based content
schedules and applies them to Tunarr. It is being evolved into a full
self-hosted web application. This sub-project builds the foundation: a
secure, contract-first REST API served by the existing binary, with the
scheduling-block configuration moving from YAML files to the SQLite
store so a web UI can edit it.

Repo: `christopherime/schedularr` (transferred from `geekxflood/schedularr`;
local `origin` must be repointed). No CI pipelines exist yet — they are
sub-project 4.

## Goals

- REST API exposing the scheduling engine: blocks CRUD, generate/apply,
  series state, history, Tunarr channel listing.
- Blocks stored in SQLite as the source of truth; YAML import/export
  retained for bootstrap, backup, and GitOps-style workflows.
- App-level bearer-token authentication; portable beyond this cluster.
- Truly agnostic: **Tunarr is the only runtime integration.** Jellyfin,
  Sonarr, and Radarr integrations are removed entirely; content
  metadata will come from online sources (TMDB/TVDB — sub-project 3).
- Foundation the web UI (sub-project 2) can generate a typed client from.
- Built on the current Go toolchain (1.27); module path renamed to the
  transferred repo.

## Non-goals (deferred to later sub-projects)

- Web UI (sub-project 2 — consumes this API; TUI deletion happens there).
- Tag/metadata enrichment from TMDB/TVDB etc. (sub-project 3).
- Dockerfile hardening, GitHub Actions, Helm chart, cluster deployment,
  oauth2-proxy/Keycloak SSO fronting (sub-project 4).
- Multi-user identity, roles, audit trails (single-operator homelab;
  SSO fronting supplies user identity later if ever needed).

## Decisions (settled with operator)

1. **Auth model:** app enforces a static bearer token; cluster-grade SSO
   (oauth2-proxy/Keycloak) fronts the UI at the gateway later. Keeps the
   app agnostic and testable.
2. **Config source of truth:** SQLite for blocks; CUE validation on every
   write; YAML import/export endpoints; first-run auto-import of
   `scheduler.yaml` if the DB holds no blocks.
3. **Contract-first (Approach B):** `api/openapi.yaml` is authoritative;
   `oapi-codegen` generates Go server interfaces/types; chi for routing.
   The same spec later generates the web UI's TypeScript client.
4. **TUI:** deprecated now (startup warning, removed from README);
   deleted in sub-project 2 when the web UI MVP replaces it.
5. **Integration removals:** Jellyfin (`internal/external/jellyfin/`,
   the Live-TV guide-refresh hook), Sonarr, and Radarr
   (`internal/external/sonarr/`, `internal/external/radarr/`, the
   availability-filter code paths in the engine, and
   `cmd/content_sources.go`) are all removed in this phase, along with
   their config keys and docs. Jellyfin refreshes its guide on its own
   schedule from Tunarr's XMLTV; per-title availability filtering via
   *arr apps is retired — future enrichment comes from online metadata
   sources (sub-project 3).
6. **Credentials** (Tunarr API key) remain env/file configuration only.
   No API endpoint reads or writes them; the status endpoint reports
   connectivity/health only.
7. **Toolchain and module path:** `go 1.27` in `go.mod` (current
   stable); module renamed `github.com/geekxflood/schedularr` →
   `github.com/christopherime/schedularr` with all imports rewritten.
8. **Web UI frontend preference (recorded for sub-project 2):** Hugo —
   a static Hugo-built frontend embedded in the binary via `go:embed`,
   with JavaScript consuming this API.

## Architecture

### Runtime shape

- Single binary. New `schedularr serve` command runs the cron scheduler
  engine and the HTTP API (default `:8484`) with graceful shutdown
  (SIGTERM drains in-flight requests, stops cron).
- `run --daemon` becomes a deprecated alias for `serve --no-api`.
- Structured `slog` JSON logging as today; every request logged with a
  request ID (middleware-assigned, echoed in error responses).

### Package layout

```txt
api/openapi.yaml            # Authoritative OpenAPI 3 contract
internal/api/               # Handlers (thin HTTP↔engine/store glue)
internal/api/gen/           # oapi-codegen output (committed, CI-verified)
internal/api/middleware/    # auth, request-id, logging, recovery
```

- `make generate` runs oapi-codegen via a pinned `go run` tool
  dependency. Generated code is committed; a later CI step re-runs
  codegen and fails on diff.
- Handlers contain no business logic: they call the engine/store through
  the existing interfaces (`internal/scheduler/interfaces.go` pattern).

### API surface v1

All routes under `/api/v1`, bearer-token auth unless noted.

| Area | Endpoints | Notes |
|---|---|---|
| Blocks | `GET/POST /blocks`, `GET/PUT/DELETE /blocks/{id}` | CUE-validated writes; `enabled` toggle |
| Schedule | `POST /generate` (dry-run, returns plan), `POST /apply`, `GET /schedule`, `GET /history` | mirrors existing CLI semantics |
| Series state | `GET /state/series`, `PATCH /state/series/{id}` | adjust cursor / start episode / skips |
| Integrations | `GET /channels` (Tunarr), `GET /status` | Tunarr connectivity/version/health; never credentials |
| Import/export | `POST /blocks/import` (YAML, `dry_run` flag), `GET /blocks/export` (YAML) | GitOps escape hatch |
| System | `GET /healthz`, `GET /readyz`, `GET /metrics`, `GET /openapi.json` | unauthenticated |

### Persistence

- New migrations in `internal/store/migrations/`: `blocks` table —
  `id`, `name`, `cron`, `duration_minutes`, `channel_id`, `priority`,
  `type` (filter/series), `filter_json`, `series_json`, `enabled`,
  `created_at`, `updated_at`.
- Engine reads blocks from the store instead of `scheduler.yaml`.
- First-run bootstrap: DB empty of blocks + `scheduler.yaml` present →
  auto-import with a prominent log line.
- CLI commands (`generate`, `state`, `channels`, `validate`) keep
  working; `validate` continues to validate YAML (now the import format).

### Security

- Token from `SCHEDULARR_API_TOKEN` (env; config-file fallback).
  Startup fails if the token is set but shorter than 32 chars; API
  refuses to start without a token unless `--insecure-no-auth` is
  passed explicitly (for local dev only, loudly logged).
- Constant-time token comparison; 401 with `problem+json` body.
- TLS terminates at the ingress/gateway; app serves plain HTTP.
- CORS: same-origin only (UI will be embedded in the binary).
- No secrets in logs; API responses carry no credential material in
  any form.
- `make lint` (golangci-lint + gosec + govulncheck) remains mandatory.

### Error handling

- RFC 7807 `application/problem+json` for all API errors; single
  problem schema defined in the spec; `request_id` included.
- Internal wrapping stays house-style: `fmt.Errorf("...: %w", err)`.

### Testing

- TDD. Handler tests: `httptest` against the generated router with
  mocked store/engine interfaces; table-driven.
- Migration tests for the new tables (existing sqlite test patterns).
- YAML import/export round-trip tests (CUE validation both directions).
- Auth middleware tests: missing/short/wrong/valid token,
  constant-time path.
- Race detector via existing `make test`.

## Housekeeping (this phase)

- Repoint `origin` to `git@github.com:christopherime/schedularr.git`.
- Module path rename + `go 1.27` bump land as the first implementation
  commit (mechanical import rewrite, verified by `make build test lint`).
- Operator commits or stashes the in-flight TUI WIP
  (`internal/tui/calendar_day.go`, `internal/tui/model.go`, untracked
  `schedules/`) before implementation starts.

## Out of scope, recorded for later

- Sub-project 2: web UI — Hugo-built static frontend (operator
  preference) embedded via `go:embed`, JS/typed client generated from
  `api/openapi.yaml`, TUI deletion.
- Sub-project 3: tag engine (TMDB/TVDB; note IMDb has no official free
  API), tags as first-class filter criteria.
- Sub-project 4: hardened image, GitHub Actions (lint/test/security/
  codegen-drift/build/release to GHCR), Helm chart, ArgoCD Application,
  oauth2-proxy SSO fronting, LAN-only HTTPRoute.
