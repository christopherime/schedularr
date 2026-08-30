# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Schedularr is a Go application that automates TV channel programming for [Tunarr](https://tunarr.com). It uses cron-based scheduling to generate and apply content schedules based on user-defined rules (blocks), with advanced filtering by genre, rating, year, duration, and title patterns.

## Development Principles (operator rules — non-negotiable)

1. **Lean and clean codebase.** Only code the project uses right now. Superseded code is deleted outright — commands, config keys, schema sections, and dependencies in the same change. No deprecation aliases, no transition shims, no commented-out remnants.
2. **Feature first — never block on tests.** Implementation leads; tests accompany the feature and `make test` must be green before every commit, but no test-ceremony (strict TDD ritual) may stall delivery.
3. **Bespoke, up-to-date documentation — always.** README, this file, AGENTS.md, and `docs/` are updated in the SAME commit as the code they describe. Documentation drift is a defect.
   Four surfaces hang off `docs/` (the single source of truth): the **documentation site** (<https://christopherime.github.io/schedularr/>, built by `.github/workflows/pages.yaml` on every docs push — new pages must be added to `mkdocs.yml`'s `nav`), the **cluster wiki page** (applicationset repo, `wiki/docs/applications/media/schedularr.md`, updated on every image-pin bump), and the operator's **Obsidian note** (`PERSO/k8s-home/GXF Schedularr & Tunarr.md` in the vault at `~/Documents/main` — update on each release or operational change).
4. **Use the configured skills.** Front-end/UI work (the Hugo web UI) goes through the `impeccable` skill; generated prose (docs, READMEs, release notes) gets cleaned with `stop-slop`.

## Development Commands

```bash
make build          # Compile binary to ./bin/schedularr
make test           # Run tests with race detector
make lint           # Run golangci-lint + gosec + govulncheck
make clean          # Remove build artifacts
make validate       # Validate YAML configs with CUE schemas
make fmt            # Format Go code
make deps           # Download and tidy dependencies

# Run a single test
go test -race -v ./internal/scheduler -run TestFilterPrograms

# Run tests in a specific package
go test -race -v ./internal/external/tunarr/...

# Run the binary
./bin/schedularr --config config.yaml generate --apply --yes
```

## Architecture

```txt
cmd/                          # CLI commands (Cobra)
├── channels.go               # List Tunarr channels
├── generate.go               # Generate & apply schedules
├── serve.go                  # HTTP API server + cron scheduling loop
├── validate.go               # Config validation
├── state.go                  # Series state management
├── scheduler.go              # scheduler.yaml import-file authoring (`scheduler init`)
└── schema/                   # CUE schemas for validation
    ├── config.cue
    └── scheduler.cue
internal/
├── config/                   # CUE-based config loading (see internal/cueconfig)
├── cueconfig/                # CUE schema validation, generation, loading (schemas in cmd/schema/)
├── scheduler/                # Core scheduling engine
│   ├── engine.go             # GenerateForTimeRange, PlanBlock, PlanSeriesBlock
│   ├── filter.go             # Genre/rating/year/duration/title filters
│   ├── history.go            # 7-day schedule history (prevent repeats)
│   └── types.go
├── external/                 # API clients
│   └── tunarr/               # Tunarr REST API client
├── metadata/                 # Show metadata providers + canonical genre vocabulary
│   ├── normalize.go          # NormalizeGenre/NormalizeGenres (see docs/metadata.md)
│   ├── tmdb/                 # The Movie Database v3 client
│   └── tvdb/                 # TheTVDB v4 client
├── store/                    # SQLite state persistence (blocks, series state, history)
│   └── migrations/           # DB migrations
├── api/                      # HTTP API: router, handlers, generated gen.ServerInterface
├── problem/                  # RFC 7807 problem+json error body (shared by api + middleware)
├── metrics/                  # Prometheus metrics instrumentation
├── service/                  # Schedule generate/apply workflow (shared by CLI + API)
├── blockio/                  # scheduler.yaml parse/render + first-run store import
├── cache/                    # In-memory caching
└── httpclient/               # HTTP client with retry
web/                           # Hugo web UI, embedded into the binary via go:embed
├── DESIGN.md                   # Shipped design system: tokens, components, WCAG evidence, Alpine rules
├── hugo.toml                    # Site config (baseURL "/", disabled taxonomy/term/rss/sitemap)
├── package.json                  # devDeps: typescript, openapi-typescript (pinned exact)
├── tsconfig.json                  # Strict TS, noEmit, moduleResolution bundler
├── embed.go                       # package web -- //go:embed all:public, exports FS + Site()
├── layouts/                       # Hugo templates
│   ├── _default/baseof.html         # Shell: header/nav/token-panel/footer
│   ├── partials/nav.html            # Primary channel-select nav
│   ├── index.html                   # Dashboard ("/")
│   ├── 404.html                     # Styled not-found page, composed through baseof
│   ├── blocks/list.html              # Blocks ("/blocks/")
│   ├── schedule/list.html             # Schedule ("/schedule/")
│   └── series/list.html               # Series ("/series/")
├── content/                       # Hugo section front matter (blocks/, schedule/, series/ _index.md)
├── assets/
│   ├── css/main.css                 # Hand-written, one file, CSS custom properties (light/dark)
│   ├── ts/                          # api.ts, token.ts, main.ts; gen/types.d.ts generated by `make web-types`
│   │   └── pages/                     # dashboard.ts, blocks.ts, schedule.ts, series.ts
│   └── vendor/                      # Alpine.js + cronstrue, both vendored, pinned, no CDN (see web/DESIGN.md)
└── public/                        # Hugo build output -- fully gitignored (untracked); `make web-presence` writes a placeholder index.html here when missing
```

`cmd/serve.go` passes `web.Site()` into `internal/api.Config.UI`;
`internal/api/router.go` mounts it as the router's catch-all `NotFound`
handler (`internal/api/ui.go`) so `schedularr serve` serves it directly --
system routes and `/api/v1/*` still win first. `make build` depends on
`web-presence`, which only requires `web/public/index.html` to exist,
writing a one-line placeholder when it doesn't; Hugo and Node are only
needed to run `make web`, never to build the Go
binary itself.

**Data Flow:** Config + SQLite block store → Scheduling Engine → Tunarr API → Channel Programming

**Key Integration Points:**

- **Tunarr:** Sole integration - channels, programs, libraries

## CLI Commands

```bash
schedularr generate [--apply --yes] # Generate schedule (--apply --yes to push to Tunarr)
schedularr serve [--listen :8484]   # Run the HTTP API server + cron loop
schedularr channels                 # List Tunarr channels
schedularr validate <file>          # Validate config file
schedularr config generate [file]   # Generate config template
schedularr scheduler init [file]    # Create scheduler.yaml import file
```

## Coding Standards

**Error Handling:** Always wrap with context:

```go
if err != nil {
    return fmt.Errorf("failed to query tunarr: %w", err)
}
```

**Logging:** Structured JSON logging with `slog`. Key naming uses snake_case.

**Linting Limits:**

- Cyclomatic complexity: max 15
- Cognitive complexity: max 20
- Nesting depth: max 5
- Function results: max 3
- Arguments: max 5

**Blocked Packages:** `github.com/pkg/errors`, `logrus`, `crypto/md5`, `crypto/sha1`, `io/ioutil`, `gopkg.in/yaml.v1`, `gopkg.in/yaml.v2`

## Configuration

`config.yaml` is the one live app config file, validated by the CUE schema
in `cmd/schema/config.cue`: Tunarr URL/API key, logging, database path,
cache/maintenance settings, `cron_interval` (the `serve` command's cron
loop cadence, default `6h`), and `api.*` (the `serve` command's HTTP
server: `listen`, `token`, `insecure_no_auth` -- see `cmd/serve.go`,
`internal/api/router.go`).

**Scheduling blocks live in the SQLite store (`internal/store`), not in a
file.** `scheduler.yaml` is a first-run **import** format only: on startup,
if the block store is still empty, `internal/blockio.Bootstrap` imports
its blocks into the store once; after that the file is never read again --
editing it post-bootstrap has no effect. `schedularr scheduler init`
authors a new `scheduler.yaml` import file (validate it with `schedularr
validate` before deploying); it does not read or manage existing store
state. Manage blocks going forward via the `/api/v1/blocks` HTTP API
(`schedularr serve`), not by hand-editing `scheduler.yaml`.

**Scheduling Block Structure** (as authored for `scheduler.yaml` import, or
the JSON body of `POST /api/v1/blocks`):

```yaml
blocks:
  - name: "Morning Cartoons"
    cron: "0 6 * * *"           # Daily at 6:00 AM
    duration: 240               # 4 hours (minutes)
    channel_id: "channel-1"
    priority: 10                # Higher = more important in conflicts
    filter:
      genres: ["Animation"]
      ratings: ["TV-Y", "TV-G"]
      year_from: 2000
      max_duration: 30
```

## Commit Messages

Follow [Conventional Commits](https://www.conventionalcommits.org/) specification.
