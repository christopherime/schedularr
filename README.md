<div align="center">
<img src="assets/logo.svg" alt="Schedularr Logo" width="150"/>

# Schedularr

## Intelligent Content Scheduling for Tunarr

[![Go Version](https://img.shields.io/badge/Go-1.25.5+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![PRs Welcome](https://img.shields.io/badge/PRs-welcome-brightgreen.svg)](https://github.com/christopherime/schedularr/pulls)

**Automate your TV channel programming with powerful rule-based scheduling, advanced content filtering, and seamless Tunarr integration.**

[Features](#-features) • [Quick Start](#-quick-start) • [Configuration](#️-configuration) • [Examples](#-examples) • [Contributing](#-contributing)

</div>

---

## 🎯 Overview

Schedularr is a sophisticated Go application that transforms how you manage content scheduling for [Tunarr](https://tunarr.com). Say goodbye to manual programming and hello to intelligent, automated channel management with cron-based recurring blocks and multi-criteria content filtering.

### Why Schedularr?

- 🤖 **Set It and Forget It**: Define your programming rules once, let Schedularr handle the rest
- 🎨 **Smart Filtering**: Match content by title patterns, genres, ratings, release years, and duration
- ⏰ **Flexible Scheduling**: Use familiar cron syntax for daily, weekly, or custom recurring blocks
- 🔍 **Dry Run Mode**: Preview schedules before applying them to your channels
- 🚀 **Lightweight & Fast**: Built in Go for performance and reliability

---

## ✨ Features

### Core Capabilities

| Feature                      | Description                                                                   |
| ---------------------------- | ----------------------------------------------------------------------------- |
| **🔌 Tunarr Integration**   | Seamless API communication with your Tunarr instance                          |
| **🎯 Advanced Filtering**   | Regex title matching, genre/rating filters, year ranges, duration constraints |
| **📅 Cron Scheduling**      | Standard cron expressions for flexible recurring programming                  |
| **⚡ CLI Commands**          | Powerful command-line tools for automation and scripting                      |
| **🔍 Dry Run Mode**         | Test and preview schedules before applying changes                            |
| **📊 Priority System**      | Handle overlapping blocks with configurable priorities                        |
| **🏷️ Tag Support**        | Organize and filter content using custom tags                                 |

---

## 🚀 Quick Start

### Prerequisites

- **Go 1.25+** - [Download](https://go.dev/dl/)
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

Edit `config.yaml` and set your Tunarr instance details:

```yaml
tunarr:
  url: "http://localhost:8000"
  api_key: "your-api-key-here"  # Optional, if authentication is enabled
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

Schedularr uses a YAML configuration file located at:

- `~/.schedularr.yaml` (user-level)
- `./.schedularr.yaml` (project-level)

### Configuration Reference

#### Tunarr Connection

```yaml
tunarr:
  url: "http://localhost:8000"  # Tunarr API endpoint
  api_key: ""                   # Optional API key for authentication
```

#### Logging

```yaml
log:
  level: "info"    # Options: debug, info, warn, error
  format: "text"   # Options: text, json
```

#### Scheduling Blocks

```yaml
scheduler:
  blocks:
    - name: "Morning Cartoons"
      cron: "0 6 * * *"           # Daily at 6:00 AM
      duration: 240               # 4 hours (in minutes)
      channel_id: "channel-1"     # Target channel
      priority: 10                # Higher = more important
      filter:
        genres: ["Animation", "Family"]
        max_duration: 30          # Max 30 min per show
        ratings: ["TV-Y", "TV-G"]
        year_from: 2000

    - name: "Prime Time Movies"
      cron: "0 20 * * *"          # Daily at 8:00 PM
      duration: 180               # 3 hours
      channel_id: "channel-1"
      priority: 20
      filter:
        genres: ["Action", "Drama"]
        min_duration: 90          # Feature-length films
        year_from: 2010
        ratings: ["PG-13", "R"]
```

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

**📚 For complete CLI documentation, see [CLI Reference](docs/CLI_REFERENCE.md)**

### Available Commands

#### ⚙️ Configuration Management

Generate and validate configuration files:

```bash
# Generate application config
schedularr config generate [filename]

# Generate scheduler config
schedularr scheduler init [filename]

# Validate any config file
schedularr validate <file>

# List scheduler blocks
schedularr scheduler list [filename]
```

#### 📋 List Channels

View all available channels from your Tunarr instance:

```bash
schedularr channels
```

**Output:**

```txt
ID              Number  Name                    Enabled
channel-1       1       Classic Movies          true
channel-2       2       Kids Programming        true
channel-3       3       Sports & News           false
```

#### 🎬 Generate Schedule

Generate a schedule for the next 24 hours based on your configuration:

```bash
# Dry run (preview only)
schedularr generate

# Apply to Tunarr (requires --yes; there is no interactive confirmation)
schedularr generate --apply --yes
```

**Example Output:**

```txt
Channel channel-1: 12 items scheduled
 - The Incredibles (115 min)
 - Finding Nemo (100 min)
 - Toy Story (81 min)
 ...
```

#### 🔧 Configuration Management

```bash
# Validate configuration
schedularr validate

# Show current configuration
schedularr config show

# Edit configuration in $EDITOR
schedularr config edit
```

---

## 💡 Examples

### Example 1: Weekend Movie Marathon

```yaml
scheduler:
  blocks:
    - name: "Saturday Night Sci-Fi"
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
scheduler:
  blocks:
    - name: "Weekday Morning Cartoons"
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
scheduler:
  blocks:
    - name: "Film Noir Fridays"
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
scheduler:
  blocks:
    - name: "Christmas Movies"
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
├── cmd/
│   └── schedularr/          # Application entry point
│       └── main.go
├── internal/
│   ├── cli/                 # CLI commands (Cobra)
│   │   ├── root.go
│   │   ├── channels.go
│   │   └── generate.go
│   ├── config/              # Configuration management (Viper)
│   │   └── config.go
│   ├── scheduler/           # Core scheduling engine
│   │   ├── engine.go
│   │   ├── filter.go
│   │   └── types.go
│   └── tunarr/              # Tunarr API client
│       ├── client.go
│       └── types.go
├── configs/
│   └── config.yaml          # Example configuration
└── docs/
    ├── ARCHITECTURE.md
    └── SPECIFICATIONS.md
```

### Key Technologies

- **Language**: Go 1.25.5
- **CLI Framework**: [Cobra](https://github.com/spf13/cobra)
- **Configuration**: [Viper](https://github.com/spf13/viper)
- **Cron Parsing**: [robfig/cron](https://github.com/robfig/cron)

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
body (`internal/api/problem.go`).

The API is not yet wired to an HTTP server; that lands in a later task,
which will also serve the contract itself at `/openapi.json`.

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
| POST   | `/blocks/import` | 200     | 400, 409    |
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

System endpoints served outside `internal/api` — `/healthz`, `/livez`
(from `schedularr health`), and `/metrics` (from `schedularr run`) — are
not part of the OpenAPI contract and do not go through bearer auth.

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
go build -o schedularr cmd/schedularr/main.go

# Production build with optimizations
go build -ldflags="-s -w" -o schedularr cmd/schedularr/main.go

# Cross-compilation
GOOS=linux GOARCH=amd64 go build -o schedularr-linux cmd/schedularr/main.go
```

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

- [Tunarr](https://tunarr.com) - The amazing TV channel management platform
- [Cobra](https://github.com/spf13/cobra) - Powerful CLI framework

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
