<div align="center">
<img src="assets/logo.svg" alt="Schedularr Logo" width="150"/>

# Schedularr

## Content Scheduling for Tunarr

[![Go Version](https://img.shields.io/badge/Go-1.27+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![PRs Welcome](https://img.shields.io/badge/PRs-welcome-brightgreen.svg)](https://github.com/christopherime/schedularr/pulls)

**Cron-based content scheduling for [Tunarr](https://tunarr.com) TV channels, driven by rule-based blocks and content filters.**

[Features](#-features) • [Quick Start](#-quick-start) • [Configuration](#️-configuration) • [Examples](#-examples) • [API](#-api) • [Serve](#-serve-api-server--cron) • [Contributing](#-contributing)

</div>

---

## 🎯 Overview

Schedularr generates and applies TV channel schedules for [Tunarr](https://tunarr.com). Scheduling rules ("blocks") define a time slot, a cron expression, a target channel, and a content filter; Schedularr resolves each block against Tunarr's library and pushes the result back to Tunarr.

### What it does

- Defines programming rules as blocks: cron expression, duration, target channel, priority.
- Filters content by title pattern (regex), genre, rating, release year range, and duration.
- Tracks per-show season/episode progression for series-based blocks.
- Previews a schedule (`generate`, no `--apply`) before pushing it to Tunarr.
- Runs as a one-shot CLI command or as a long-lived process (`serve`) exposing an HTTP API and a cron-driven schedule cycle.

---

## ✨ Features

### Core Capabilities

| Feature               | Description                                                                    |
| ----------------------- | -------------------------------------------------------------------------------- |
| **Tunarr integration** | Reads channels and library content from Tunarr, pushes schedules back            |
| **Content filtering**  | Regex title matching, genre/rating filters, year ranges, duration constraints    |
| **Cron scheduling**    | Standard cron expressions for recurring blocks                                   |
| **Series blocks**      | Sequential episode progression per show, with season/episode state persisted     |
| **HTTP API**           | Blocks CRUD, generate/apply, history, series state, channels, status             |
| **Dry run**            | `generate` without `--apply` previews a schedule without pushing it to Tunarr    |
| **Priority system**    | Resolves overlapping blocks by configurable priority                             |
| **Tag support**        | Filter content by custom tags                                                    |

---

## 🚀 Quick Start

### Prerequisites

- **Go 1.27+** - [Download](https://go.dev/dl/)
- **A C toolchain** - the SQLite driver (`mattn/go-sqlite3`) is cgo-based; `CGO_ENABLED=1` (the default when a C compiler is on `PATH`) is required to build
- **Tunarr Instance** - [Setup Guide](https://tunarr.com/api-docs.html#latest)

### Installation

#### Option 1: Build from Source

```bash
# Clone the repository
git clone https://github.com/christopherime/schedularr.git
cd schedularr

# Build the binary
go build -o schedularr main.go

# Optional: Install to your PATH
sudo mv schedularr /usr/local/bin/
```

#### Option 2: Using Go Install

```bash
go install github.com/christopherime/schedularr@latest
```

### Initial Setup

1. **Generate configuration files:**

```bash
# Generate application config
schedularr config generate config.yaml

# Generate scheduler config
schedularr scheduler init scheduler.yaml
```

1. **Configure Tunarr connection:**

`config generate` writes `tunarr.url`/`tunarr.api_key` as literal
`${SCHEDULARR_TUNARR_URL}`/`${SCHEDULARR_TUNARR_API_KEY}` placeholders
(CUE-loaded config files support `${VAR}` env interpolation -- see
[Configuration](#️-configuration)). Either export those two environment
variables, or edit `config.yaml` directly and replace both with literal
values -- an unset `${VAR}` placeholder left in the file expands to an
empty, *unquoted* YAML value, which parses as `null` rather than `""` and
fails config loading:

```yaml
tunarr:
  url: "http://localhost:8000"
  api_key: ""  # quote it explicitly, even when empty
log:
  level: "info"
  format: "text"
```

1. **Validate your configuration:**

```bash
# Validate both configs
schedularr validate config.yaml
schedularr validate scheduler.yaml
```

1. **Verify Tunarr connection:**

```bash
schedularr --config config.yaml channels
```

---

## ⚙️ Configuration

`schedularr` resolves the app config file path in this order:

1. `--config <file>` (global flag)
2. `SCHEDULARR_CONFIG` environment variable
3. Legacy locations, in order: `./config.yaml`, `./.schedularr.yaml`,
   `~/.config/.schedularr.yaml`, `~/.schedularr.yaml`
4. `~/.schedularr/config.yaml` (default)

The file is validated against the CUE schema in `cmd/schema/config.cue`
(also embedded in the binary as `internal/cueconfig`). String values
support `${VAR}` environment variable interpolation.

### Configuration Reference

Full key list, with CUE defaults:

```yaml
tunarr:
  url: ""                  # Tunarr API endpoint (required)
  api_key: ""               # Optional API key
  timeout: "30s"

log:
  level: "info"              # debug, info, warn, error
  format: "text"             # text, json
  timezone: "Local"          # IANA time zone name

database: "schedularr.db"    # SQLite database path
scheduler_file: "scheduler.yaml"  # First-run block import file, see below
cron_interval: "6h"          # `serve`'s cron loop cadence; `serve --interval`/`-i` overrides it

maintenance:
  history_retention: "168h"  # how long schedule_history rows are kept; also bounds
                              # GET /history?days=N -- see the History Endpoint section
  cleanup_enabled: true

api:                          # the `serve` command's HTTP server
  listen: ":8484"
  token: ""                  # bearer token for /api/v1/*; SCHEDULARR_API_TOKEN env var wins when set
  insecure_no_auth: false     # skip bearer auth entirely -- local development only
```

There is no `metrics_port` config key: `schedularr serve` exposes Prometheus
metrics at `GET /metrics` on the same listener as everything else
(`--listen`/`api.listen`, default `:8484`) -- see [Serve](#-serve-api-server--cron)
below.

#### Scheduling Blocks

Blocks live in the SQLite store (`database` above), not in a config file.
`scheduler_file` (default `scheduler.yaml`) is a **first-run import
format only**: the first time the store is empty, its blocks are imported
once; editing the file after that has no effect. Generate one with
`schedularr scheduler init`, then either let it bootstrap on the next
`generate`/`serve` run, or manage blocks going forward through the
`/api/v1/blocks` HTTP API (see [API](#-api)):

```yaml
blocks:
  - name: "Morning Cartoons"
    type: filter                 # required in scheduler.yaml -- see note below
    cron: "0 6 * * *"           # Daily at 6:00 AM
    duration: 240                # 4 hours (in minutes)
    channel_id: "channel-1"      # Target channel
    priority: 10                 # Higher = more important
    filter:
      genres: ["Animation", "Family"]
      max_duration: 30           # Max 30 min per show
      ratings: ["TV-Y", "TV-G"]
      year_from: 2000

  - name: "Prime Time Movies"
    type: filter
    cron: "0 20 * * *"          # Daily at 8:00 PM
    duration: 180                # 3 hours
    channel_id: "channel-1"
    priority: 20
    filter:
      genres: ["Action", "Drama"]
      min_duration: 90           # Feature-length films
      year_from: 2010
      ratings: ["PG-13", "R"]
```

`type` (`filter` or `series`) has a schema default in `cmd/schema/scheduler.cue`,
but `schedularr validate`/the `scheduler.yaml` import path decodes each
block into a Go struct before CUE-validating it, which turns an omitted
`type` into an explicit empty string rather than an absent field -- CUE
only applies the default to a genuinely absent field, so an empty string
fails the `"filter" | "series"` check. Always set `type` explicitly in
`scheduler.yaml`. `POST /api/v1/blocks`'s JSON body does not have this
problem; `type` there can be omitted.

### Filter Options

| Field           | Type     | Description                      | Example                   |
| --------------- | -------- | -------------------------------- | ------------------------- |
| `title_pattern` | string   | Regex pattern for title matching | `"^Star.*"`               |
| `genres`        | []string | List of genres to include        | `["Comedy", "Drama"]`     |
| `ratings`       | []string | Content ratings filter           | `["PG", "PG-13"]`         |
| `year_from`     | int      | Minimum release year             | `2000`                    |
| `year_to`       | int      | Maximum release year             | `2023`                    |
| `min_duration`  | int      | Minimum duration (minutes)       | `90`                      |
| `max_duration`  | int      | Maximum duration (minutes)       | `120`                     |
| `tags`          | []string | Custom tags filter               | `["favorite", "classic"]` |

---

## 📖 Usage

### Command Overview

```bash
schedularr [command] [flags]
```

**For per-command flags and a full walkthrough, see [CLI Reference](docs/CLI_REFERENCE.md).**

### Available Commands

| Command | Purpose |
| --- | --- |
| `config generate [file]` | Write an app config file from the CUE schema defaults (`--tunarr-url`, `--log-level`, ... override individual keys) |
| `config dump` | Print the currently loaded effective config as YAML |
| `scheduler init [file]` | Write a `scheduler.yaml` block-import file from the CUE schema defaults |
| `validate <file>` | Validate an app config or `scheduler.yaml` file against its CUE schema (file type is inferred: a filename containing `scheduler` is validated as a block-import file) |
| `channels` | List Tunarr channels |
| `generate [--apply] [--yes] [--dry-run] [-v]` | Generate (and optionally apply) the next schedule cycle |
| `generate config --output <file>` | Same config generation as `config generate`, under `generate` instead |
| `state export/import/reset/set/list/backup` | Manage series progression state (`internal/store`) |
| `health [--port 9600]` | Standalone `/healthz`/`/livez` probe server (unrelated to `serve`'s own health endpoints) |
| `serve [--listen] [--insecure-no-auth] [--interval/-i]` | Run the HTTP API and the cron scheduling loop -- see [Serve](#-serve-api-server--cron) |

#### List Channels

```bash
schedularr channels
```

Output (`ID`/`Number`/`Name`/`Group`, from a live Tunarr instance):

```txt
┌───────────┬────────┬──────────────────┬────────┐
│ ID        │ NUMBER │ NAME             │ GROUP  │
├───────────┼────────┼──────────────────┼────────┤
│ channel-1 │      1 │ Classic Movies   │ Movies │
│ channel-2 │      2 │ Kids Programming │ Kids   │
└───────────┴────────┴──────────────────┴────────┘
```

#### Generate Schedule

Generates a schedule from the enabled blocks in the store (bootstrapping
`scheduler_file` into the store first, on an empty store):

```bash
# Dry run (preview only, never mutates the store or Tunarr)
schedularr generate

# Apply to Tunarr (requires --yes; there is no interactive confirmation)
schedularr generate --apply --yes
```

`generate` prints a per-channel table (start/end time, block, program,
duration, type, show/season/episode) followed by a totals summary; see
`displaySchedule`/`displayChannelSchedule` in `cmd/generate.go`.

#### Series State

```bash
schedularr state list                                    # table of all tracked series
schedularr state set "My Show" --season 2 --episode 5     # jump to S02E05
schedularr state reset "My Show"                          # back to S01E01
schedularr state export backup.json                       # all series states to JSON
schedularr state import backup.json                       # restore from JSON
schedularr state backup full-backup.db                    # whole-database SQLite backup (VACUUM INTO)
```

---

## 💡 Examples

### Example 1: Weekend Movie Marathon

```yaml
blocks:
  - name: "Saturday Night Sci-Fi"
    type: filter
    cron: "0 20 * * 6"  # Saturdays at 8 PM
    duration: 360       # 6 hours
    channel_id: "channel-1"
    filter:
      genres: ["Science Fiction"]
      min_duration: 90
      year_from: 1980
```

### Example 2: Weekday Morning Kids Block

```yaml
blocks:
  - name: "Weekday Morning Cartoons"
    type: filter
    cron: "0 7 * * 1-5"  # Monday-Friday at 7 AM
    duration: 120
    channel_id: "channel-2"
    filter:
      genres: ["Animation"]
      ratings: ["TV-Y", "TV-Y7"]
      max_duration: 30
```

### Example 3: Classic Film Noir Night

```yaml
blocks:
  - name: "Film Noir Fridays"
    type: filter
    cron: "0 22 * * 5"  # Fridays at 10 PM
    duration: 240
    channel_id: "channel-1"
    filter:
      title_pattern: ".*Noir.*|.*Detective.*"
      year_from: 1940
      year_to: 1959
      genres: ["Crime", "Mystery"]
```

### Example 4: Holiday Special Programming

```yaml
blocks:
  - name: "Christmas Movies"
    type: filter
    cron: "0 18 1-25 12 *"  # Dec 1-25 at 6 PM
    duration: 180
    channel_id: "channel-1"
    priority: 100  # Override other blocks
    filter:
      title_pattern: ".*Christmas.*|.*Holiday.*"
      genres: ["Family", "Comedy"]
```

---

## 🏗️ Architecture

### Project Structure

```txt
schedularr/
├── main.go                    # Entry point
├── api/
│   └── openapi.yaml           # OpenAPI 3.0.3 contract (source of truth for internal/api/gen)
├── cmd/                        # CLI commands (Cobra)
│   ├── root.go
│   ├── channels.go             # List Tunarr channels
│   ├── generate.go             # Generate & apply schedules
│   ├── serve.go                # HTTP API server + cron scheduling loop
│   ├── validate.go             # Config validation
│   ├── state.go                # Series state management
│   ├── config.go               # App config generation/dump
│   ├── health.go               # Standalone healthz/livez probe server
│   ├── scheduler.go            # scheduler.yaml import-file authoring
│   └── schema/                 # CUE schemas for validation
│       ├── config.cue
│       └── scheduler.cue
├── internal/
│   ├── config/                 # CUE-based config loading (see internal/cueconfig)
│   ├── cueconfig/               # CUE schema compilation, validation, generation
│   ├── scheduler/                # Core scheduling engine
│   │   ├── engine.go             # GenerateForTimeRange, PlanBlock, PlanSeriesBlock
│   │   ├── filter.go             # Genre/rating/year/duration/title filters
│   │   ├── history.go            # Schedule history (prevent repeats)
│   │   └── types.go
│   ├── external/tunarr/          # Tunarr REST API client
│   ├── store/                    # SQLite persistence (blocks, series state, history)
│   │   └── migrations/
│   ├── api/                      # HTTP API: router, handlers, generated gen.ServerInterface
│   │   └── gen/                  # server.gen.go -- generated, do not hand-edit
│   ├── service/                   # Schedule generate/apply workflow (shared by CLI + API)
│   ├── blockio/                   # scheduler.yaml parse/render + first-run store import
│   ├── problem/                   # RFC 7807 application/problem+json helpers
│   ├── metrics/                   # Prometheus metrics registration
│   ├── cache/                     # In-memory caching
│   └── httpclient/                # HTTP client with retry
├── configs/
│   └── config.yaml              # Example configuration
├── e2e/                          # Docker-based acceptance tests against a real Tunarr
└── docs/
    ├── ARCHITECTURE.md
    └── SPECIFICATIONS.md
```

### Key Technologies

- **Language**: Go 1.27
- **CLI framework**: [Cobra](https://github.com/spf13/cobra)
- **Configuration**: [CUE](https://cuelang.org/) schemas (`cmd/schema/`, `internal/cueconfig`), not Viper
- **HTTP API**: [chi](https://github.com/go-chi/chi) router, generated from `api/openapi.yaml` via [oapi-codegen](https://github.com/oapi-codegen/oapi-codegen)
- **Persistence**: SQLite (`mattn/go-sqlite3`), migrated with [golang-migrate](https://github.com/golang-migrate/migrate)
- **Cron parsing**: [robfig/cron](https://github.com/robfig/cron)

---

## 🔌 API

Schedularr exposes an HTTP API defined by an OpenAPI 3.0.3 contract at
[`api/openapi.yaml`](api/openapi.yaml). The contract covers blocks CRUD,
block import/export, schedule generation and application, history, series
state, channels, and status.

Server code is generated from the contract with
[oapi-codegen](https://github.com/oapi-codegen/oapi-codegen) v2:

```bash
make generate
```

This writes `internal/api/gen/server.gen.go`, which is committed and must
not be edited by hand — change `api/openapi.yaml` and regenerate instead.
Handlers live in `internal/api/` and implement the generated
`gen.ServerInterface`. Errors use an RFC 7807 `application/problem+json`
body (`internal/api/problem.go`, backed by `internal/problem`).

The API is served by `schedularr serve` (`internal/api/router.go`,
`cmd/serve.go`) — see [Serve](#-serve-api-server--cron) below.

### Blocks Endpoints

Blocks CRUD (`internal/api/blocks.go`) is implemented against
`gen.ServerInterface` and backed by the sqlite block store
(`internal/store`). Every write path (`POST`/`PUT`) validates the block
spec against the CUE scheduler schema via `blockio.ValidateBlocks` before
touching the store, and every response body is `application/json` (or
`application/problem+json` for errors).

| Method | Path           | Success | Error codes   |
| ------ | -------------- | ------- | ------------- |
| GET    | `/blocks`      | 200     | —             |
| POST   | `/blocks`      | 201     | 400, 409      |
| GET    | `/blocks/{id}` | 200     | 404           |
| PUT    | `/blocks/{id}` | 200     | 400, 404, 409 |
| DELETE | `/blocks/{id}` | 204     | 404           |

Notes:

- `POST`/`PUT` return `400` for a spec that fails CUE validation (e.g. a
  missing `cron` or a non-positive `duration`) or a malformed JSON body.
- `POST` returns `409` for a duplicate block name; `PUT` returns `409` if
  the request body's `spec.name` differs from the existing block's name
  and collides with another block. A `PUT` whose `spec.name` differs from
  the current name without colliding renames the block.
- `POST`/`PUT` also return `400` for a series block (`spec.type: series`)
  whose `series[].show_title` is empty. The CUE scheduler schema types
  `show_title` as a bare `string` with no non-empty constraint, so this
  case would otherwise pass CUE validation; the check is applied in Go on
  both block-ingestion paths (blocks CRUD here, and `blocks/import` below).

### Import/Export Endpoints

Block import/export (`internal/api/importexport.go`) round-trips the same
sqlite-backed blocks through `blockio.ParseYAML`/`blockio.RenderYAML`
(`internal/blockio`) used for the on-disk `scheduler.yaml` bootstrap path.
Both endpoints exchange raw YAML text, not JSON: oapi-codegen does not
generate a Go type for an `application/yaml` request or response body, so
the handlers read/write `[]byte` directly.

| Method | Path             | Success | Error codes |
| ------ | ---------------- | ------- | ----------- |
| POST   | `/blocks/import` | 200     | 400, 409, 413 |
| GET    | `/blocks/export` | 200     | —           |

Notes:

- `POST /blocks/import` takes an `application/yaml` body (capped at 1MiB
  via `http.MaxBytesReader`; an oversized body gets `413`) and an optional
  `?dry_run=true` query parameter (default `false`). The body is strictly
  decoded and CUE-validated by `blockio.ParseYAML`, including duplicate
  block-name and empty-`show_title` rejection -- any failure there is
  `400` with the CUE detail included. Every parsed block's name is then
  checked against every block already in the store; any collision is
  `409` (listing the colliding name(s)) with **zero writes**, even for the
  non-colliding blocks in the same batch. `dry_run=true` stops after that
  check and reports what would have been imported (`{imported, dry_run,
  names}`) without writing anything; otherwise every block is created with
  a fresh UUID and `enabled: true`.
- `GET /blocks/export` renders every stored block's spec as YAML via
  `blockio.RenderYAML` -- **including disabled blocks**, since export
  doubles as a backup mechanism and silently dropping disabled blocks
  would make a restored import lossy. The response has no `enabled` state
  of its own (that lives on the store record, not the spec); re-importing
  an exported file creates every block as enabled.

### Schedule Endpoints

Schedule generation and application (`internal/api/schedule.go`) delegate to
`internal/service.Runner` (`Deps.Sched`, a `service.ScheduleRunner`), which
is the extraction of what used to be `cmd/generate.go`'s inline
generate/apply flow: load the enabled blocks (`service.ActiveBlocks`, moved
from the CLI's former `loadActiveBlocks`), fetch available Tunarr content,
run `scheduler.Engine.GenerateForTimeRange` over a `days`-wide window
starting now, and -- only when applying -- push the result to Tunarr per
channel via `UpdateSchedule` and commit the engine's pending state via
`Engine.Commit()`. `cmd/generate.go` now calls the same `service.Runner`,
so the CLI and the API share one implementation.

| Method | Path               | Success | Error codes |
| ------ | ------------------ | ------- | ----------- |
| POST   | `/generate`        | 200     | 400, 502    |
| POST   | `/apply`           | 200     | 400, 502    |
| GET    | `/schedule?days=N` | 200     | 400, 502    |

Notes:

- `POST /generate` and `POST /apply` share the same optional
  `GenerateRequest` body (`days`, `channel_id`); `GET /schedule` takes the
  same `days` as a query parameter. `days` defaults to `7` and (matching
  the `GetHistory` convention -- oapi-codegen's generated bindings don't
  enforce the OpenAPI schema's default/minimum/maximum) is range-checked by
  the handler itself against `[1, 30]`, returning `400` outside that range.
- `POST /generate` always runs a dry run (`applied: false` in the response)
  regardless of the request body -- it never mutates the store or Tunarr.
  Only `POST /apply` (`applied: true` on success) pushes anything.
- `channel_id`, when set, restricts *which blocks get planned at all* --
  not just which channels appear in the response or get pushed via
  `UpdateSchedule`. `Runner.Run` filters the active blocks down to that
  channel's before handing them to `scheduler.Engine`, so a channel-scoped
  `POST /apply` never touches Tunarr, schedule history, or series-cursor
  state for any other channel: `scheduler.Engine` mutates pending
  series-state and history for every block it plans, so filtering only the
  result map after planning (an earlier version of this behavior) would
  still have let `Engine.Commit()` persist state for channels the request
  never asked about, even though nothing was pushed to Tunarr for them.
- A `Runner.Run` failure (loading blocks, fetching Tunarr content,
  generating the schedule, or -- on apply -- `UpdateSchedule`/`Commit`)
  returns `502` (`title: "schedule generation failed"`) with a short, fixed
  detail: the underlying error can wrap a raw store/driver error, so
  (matching the internal-error convention used elsewhere in this package)
  it's logged server-side only, never echoed in the response body.

### History Endpoint

History (`internal/api/history.go`) lists `schedule_history` rows (backed
by `store.ListScheduleHistory`, ordered by `scheduled_at` DESC) scheduled
within the last `days` days.

| Method | Path      | Success | Error codes |
| ------ | --------- | ------- | ----------- |
| GET    | `/history?days=N` | 200 | 400 |

Notes:

- `days` defaults to `7` when omitted. The OpenAPI schema declares
  `minimum: 1` / `maximum: 90` for `days`, but oapi-codegen's chi-server
  generator does not enforce a schema's default/minimum/maximum at the
  binding layer -- it only type-checks the raw query value. The handler
  applies the default and range check itself, returning `400` for any
  `days` outside `[1, 90]`.
- `days` only has data to return as far back as `maintenance.history_retention`
  (default `168h`/7 days) allows: `service.NewRunner` threads that config
  value into every `scheduler.Engine` it builds
  (`scheduler.EngineOptions.HistoryWindow`), and `Engine.Commit()` prunes
  `schedule_history` to that same window on every apply. Requesting
  `?days=90` when `history_retention` is still at its 7-day default returns
  only the last 7 days' worth of rows -- set `history_retention` to at
  least the widest `days` value you intend to query (e.g. `2160h` for a
  full 90-day window) for `GET /history?days=90` to actually have 90 days
  of data available.

### Series State Endpoints

Series state (`internal/api/state.go`) lists and patches the per-show
`series_state` tracking rows (current season/episode, completion, and the
disabled flag the scheduler sets once a non-restarting series runs out of
episodes) backed by the same sqlite store as blocks.

| Method | Path                         | Success | Error codes |
| ------ | ---------------------------- | ------- | ----------- |
| GET    | `/state/series`              | 200     | —           |
| PATCH  | `/state/series/{show_title}` | 200     | 400, 404    |

Notes:

- `PATCH` applies a partial update: only fields present in the request body
  (`current_season`, `current_episode`, `completed`, `disabled`) change,
  and a body with none of them set returns `400`, as does a malformed JSON
  body.
- `PATCH` returns `404` for a `show_title` with no persisted `series_state`
  row. This intentionally does not reuse `store.GetSeriesState`, which
  fabricates a default S01E01 state for any show (tracked or not) as a
  convenience for scheduling callers; the handler instead uses
  `store.GetPersistedSeriesState`, added alongside it, which returns
  `ErrNotFound` when no row exists.

### Channels and Status Endpoints

Channels and status (`internal/api/tunarr.go`) are the Tunarr boundary:
`ListChannels` proxies `GET /api/channels` on the configured Tunarr instance
via a `TunarrAPI` interface (`GetChannels(ctx) ([]tunarr.Channel, error)`)
added to `Deps`, and `GetStatus` reports overall service health, probing
Tunarr reachability through the same interface. Production wiring passes a
`*tunarr.Client` (it satisfies `TunarrAPI` unmodified); `Deps.Tunarr` may
also be `nil`, meaning "no Tunarr integration configured" -- both handlers
treat that the same as a Tunarr connectivity failure, not a programming
error.

| Method | Path        | Success | Error codes |
| ------ | ----------- | ------- | ----------- |
| GET    | `/channels` | 200     | 502         |
| GET    | `/status`   | 200     | —           |

Notes:

- `GET /channels` returns `502` (`title: "tunarr unreachable"`) both when
  `Deps.Tunarr` is `nil` (`detail: "tunarr not configured"`) and when the
  configured client's `GetChannels` call fails (`detail` carries the
  wrapped connectivity error, e.g. a dial failure -- an upstream
  reachability message, not an internal leak).
- `GET /status` never returns a `5xx`. It always responds `200` with
  `version` (`Deps.Version`), `tunarr_reachable` (a live probe via
  `GetChannels`), `tunarr_error` (set whenever `tunarr_reachable` is
  `false`, whether from a nil client or a failed probe), and `blocks`
  (`store.CountBlocks`). A `CountBlocks` failure is logged server-side and
  `blocks` is simply omitted from the response rather than failing the
  request.

### Middleware

`internal/api/middleware` provides the four pieces of middleware every
`/api/v1/*` route will run through once the router is wired up:

- **Request ID** — generates a fresh identifier for every request (an
  inbound `X-Request-Id` header is never trusted), returns it on the
  response as `X-Request-Id`, and makes it available to
  `api.WriteProblem` so every problem+json body includes the same id.
- **Logging** — writes one structured `slog` line per request with the
  keys `method`, `path`, `status`, `duration_ms`, `request_id`.
- **Recovery** — turns a panic in a handler into a `500`
  `application/problem+json` response and logs it with a stack trace,
  instead of crashing the process.
- **Bearer auth** — requires `Authorization: Bearer <token>` on protected
  routes. The token is compared as a SHA-256 digest via
  `crypto/subtle.ConstantTimeCompare`, and a token under 32 characters is
  rejected when the middleware is constructed. A missing or wrong token
  gets a `401` problem+json response.

`schedularr serve`'s own system endpoints — `/healthz`, `/readyz`,
`/metrics`, `/openapi.json` — are not part of the OpenAPI contract and sit
outside this middleware chain entirely (see [Serve](#-serve-api-server--cron)
below). The separate `schedularr health` command's `/healthz`/`/livez`
endpoints are a standalone, unrelated probe server, unaffected by this
package.

---

## 🚀 Serve (API server + cron)

`schedularr serve` runs the HTTP API and the cron scheduling loop in one
long-lived process — the API server, the block store, and the periodic
schedule generate-and-apply cycle all share one process and one graceful
shutdown path (`internal/api/router.go`, `cmd/serve.go`).

```bash
SCHEDULARR_API_TOKEN=$(openssl rand -hex 32) schedularr serve --listen :8484
```

### Flags

| Flag                    | Default | Description                                                       |
| ------------------------ | ------- | --------------------------------------------------------------------- |
| `--listen`                | `:8484`  | Address the HTTP API server listens on                              |
| `--insecure-no-auth`     | `false`  | Skip bearer-token auth on `/api/v1/*` — local development only, never a real deployment (logs a `WARN` at startup when set) |
| `--interval`/`-i`         | `6h`     | Interval between cron-driven schedule generate-and-apply cycles     |

### Environment and config keys

| Config key              | Env var                 | Default | Description                                                         |
| ------------------------ | ------------------------ | ------- | --------------------------------------------------------------------- |
| `api.listen`              | —                          | `:8484`  | Same as `--listen`; the flag wins when explicitly passed              |
| `api.token`                | `SCHEDULARR_API_TOKEN` | `""`     | Bearer token required on `/api/v1/*`. The env var always wins over this key when both are set |
| `api.insecure_no_auth` | —                          | `false`  | Same as `--insecure-no-auth`; either source turning it on disables auth |
| `cron_interval`            | —                          | `6h`     | Same as `--interval`/`-i`; the flag wins when explicitly passed. Top-level key, not under `api.*` -- it governs the cron loop, not the HTTP server |

`schedularr serve` refuses to start if the effective token is empty (or
shorter than 32 characters) and `--insecure-no-auth`/`api.insecure_no_auth`
is not set.

### Endpoints

| Method | Path             | Auth | Description                                     |
| ------ | ---------------- | ---- | ------------------------------------------------ |
| GET    | `/healthz`         | none | Process liveness only, no dependency checks     |
| GET    | `/readyz`           | none | Liveness plus a store round-trip (`SELECT 1`)    |
| GET    | `/metrics`          | none | Prometheus text exposition                        |
| GET    | `/openapi.json`    | none | The OpenAPI 3.0 contract as JSON                  |
| \*      | `/api/v1/*`         | bearer | Blocks CRUD, import/export, generate/apply/schedule, history, series state, channels, status — see [API](#-api) above |

### Cron loop

On an interval (`--interval`/`-i`, or the `cron_interval` config key,
default `6h` either way -- flag wins when passed explicitly) and once
immediately at startup, `serve` regenerates and applies the next day's
schedule — the same `service.Runner.Run(ctx, Options{Days: 1, Apply:
true})` call `schedularr generate --apply --yes` makes, just on a timer
instead of a one-shot CLI invocation. A failed tick is logged and does not
stop the server; the next tick tries again.

### Shutdown

`serve` listens for `SIGINT`/`SIGTERM`. On either, it stops accepting new
HTTP connections, waits up to 15s for in-flight requests to finish
(`http.Server.Shutdown`), then stops the cron loop and closes the store —
in that order, so an in-progress schedule apply isn't cut off mid-write by
the HTTP shutdown deadline.

### Running it

`serve` is a plain long-lived process: run it under whatever supervisor
manages the rest of your deployment (a systemd unit with `Restart=on-failure`,
a Docker/Podman container, a Kubernetes `Deployment`, ...). It has no
built-in daemonization — foreground it and let the supervisor handle
restarts and logging, the same way you'd run any other Go server binary.

---

## 🧪 Development

### Running Tests

```bash
# Run all tests
go test ./...

# Run with coverage
go test -cover ./...

# Run specific package tests
go test ./internal/scheduler/...
```

### Building

```bash
# Development build
go build -o schedularr main.go

# Stripped build with a version stamp (reported by GET /api/v1/status)
go build -ldflags="-s -w -X github.com/christopherime/schedularr/cmd.Version=1.2.3" -o schedularr main.go

# Cross-compilation
GOOS=linux GOARCH=amd64 go build -o schedularr-linux main.go
```

`make build` runs the equivalent of the first form (see `Makefile`).

### Code Quality

```bash
# Format code
go fmt ./...

# Lint
golangci-lint run

# Vet
go vet ./...
```

---

## 🤝 Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

### Development Workflow

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add some amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

### Guidelines

- Write tests for new features
- Follow Go best practices and idioms
- Update documentation as needed
- Ensure all tests pass before submitting PR

---

## 📝 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

---

## 🙏 Acknowledgments

- [Tunarr](https://tunarr.com) - the TV channel management platform this project schedules for
- [Cobra](https://github.com/spf13/cobra) - CLI framework

---

## 📞 Support

- 🐛 **Issues**: [GitHub Issues](https://github.com/christopherime/schedularr/issues)
- 💬 **Discussions**: [GitHub Discussions](https://github.com/christopherime/schedularr/discussions)
- 📧 **Email**: [Contact](mailto:christopherime@me.com)

---

<div align="center">

**Made with ❤️ by [christopherime](https://github.com/christopherime)**

⭐ Star this repo if you find it useful!

</div>
