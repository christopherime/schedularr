# Schedularr CLI Reference

## Overview

Schedularr provides a command-line interface for managing TV channel
scheduling with Tunarr, plus an HTTP API server and web UI
(`schedularr serve`; see the [API](../README.md#-api),
[Serve](../README.md#-serve-api-server--cron), and
[Web UI](../README.md#-web-ui) sections of the README). All commands
support the global `--config` flag to specify a custom app config file.

## Global Flags

- `--config <file>` - App config file. Resolution order: this flag, then
  `SCHEDULARR_CONFIG`, then legacy locations (`./config.yaml`,
  `./.schedularr.yaml`, `~/.config/.schedularr.yaml`, `~/.schedularr.yaml`),
  then the default `~/.schedularr/config.yaml`.
- `--help`, `-h` - Show help for any command

## Commands

### Configuration Management

#### `config generate [filename]`

Generate an application configuration file from the CUE schema with
default values. Also available as `generate config --output <filename>`
(same generator, different subcommand tree -- both exist in the code).

**Usage:**

```bash
# Generate default config.yaml
schedularr config generate

# Generate with custom filename
schedularr config generate my-config.yaml

# Generate JSON format
schedularr config generate config.json

# Override specific keys
schedularr config generate my-config.yaml --tunarr-url "http://my-tunarr:8000" --log-level debug
```

**Output:**

- Creates a YAML or JSON file (auto-detected from extension)
- Includes every key in `cmd/schema/config.cue` with its default value
- `tunarr.url` and `tunarr.api_key` default to literal `${SCHEDULARR_TUNARR_URL}`/
  `${SCHEDULARR_TUNARR_API_KEY}` placeholders (config files support `${VAR}`
  environment interpolation). Either export those variables, or edit the
  file and replace the placeholders with literal values -- **always quote
  the value**, even an empty one (`api_key: ""`). An unset `${VAR}`
  placeholder left unquoted expands to nothing, which YAML parses as
  `null` rather than `""`, and `null` fails CUE validation on load.

#### `config dump`

Print the currently loaded effective configuration (after resolving the
config file, defaults, and environment overrides) as YAML.

**Usage:**

```bash
schedularr --config config.yaml config dump
```

---

### Scheduler Configuration

#### `scheduler init [filename]`

Generate a `scheduler.yaml` block-import file from the CUE schema with an
example block. Blocks live in the SQLite store, not in this file --
`scheduler.yaml` is read once, on the first run against an empty store
(`internal/blockio.Bootstrap`), and ignored after that. Manage blocks
afterward through the `/api/v1/blocks` HTTP API (`schedularr serve`).

**Usage:**

```bash
# Generate default scheduler.yaml
schedularr scheduler init

# Generate with custom filename
schedularr scheduler init my-schedule.yaml

# Generate JSON format
schedularr scheduler init schedule.json

# Override the first block's fields
schedularr scheduler init my-schedule.yaml --name "Morning Cartoons" --cron "0 8 * * *" --duration 180 --channel-id "kids-channel" --priority 5
```

**Output:**

- Creates a YAML or JSON file with one example filter block
- Validate it before deploying: `schedularr validate my-schedule.yaml`

---

### Validation

#### `validate <file>`

Validate an application config file or a `scheduler.yaml` block-import
file against the CUE schemas in `cmd/schema/`.

File type is inferred from the filename: a name containing `scheduler`
(case-sensitive substring match) is validated as a block-import file
(strict YAML decode, duplicate-block-name check, per-block CUE
validation -- the same path `blockio.Bootstrap` and `POST
/api/v1/blocks/import` use); anything else is validated as an app config.
Name scheduler files accordingly (`scheduler.yaml`, `my-scheduler.yaml`,
...), or `validate` will check them against the wrong schema.

**Usage:**

```bash
# Validate application config
schedularr validate config.yaml

# Validate a scheduler.yaml block-import file
schedularr validate scheduler.yaml
```

**Note on `type`:** each block's `type` field (`filter` or `series`) has
a CUE schema default, but the scheduler-file validation path decodes each
block into a Go struct first, which turns an omitted `type` into an
explicit empty string rather than an absent field. CUE only applies a
default to a genuinely absent field, so an empty string fails the
`"filter" | "series"` enum check. Always set `type` explicitly in
`scheduler.yaml`. `POST /api/v1/blocks`'s JSON body does not have this
problem.

**Exit Codes:**

- `0` - Validation passed
- `1` - Validation failed (shows detailed errors)

---

### Schedule Generation

#### `generate`

Generate (and optionally apply) a schedule from the blocks currently in
the store. On an empty store, first bootstraps `scheduler_file`'s blocks
into it.

**Usage:**

```bash
# Dry run (preview only, never mutates the store or Tunarr)
schedularr generate

# Apply to Tunarr (--yes is mandatory; there is no interactive prompt)
schedularr generate --apply --yes

# Verbose output (raises the logger to debug)
schedularr generate --verbose
```

**Flags:**

- `--apply` - Push the generated schedule to Tunarr (requires `--yes`)
- `--yes` - Required alongside `--apply`; there is no interactive confirmation
- `--dry-run` - Preview only, even if `--apply` is also set
- `--verbose`, `-v` - Raise logging to debug

---

### Series State

#### `state export/import/reset/set/list/backup`

Manage series progression state (`internal/store`, table `series_state`).

**Usage:**

```bash
# List all tracked series
schedularr state list

# Jump a series to a specific season/episode
schedularr state set "My Favorite Show" --season 2 --episode 5

# Reset a series to S01E01
schedularr state reset "My Favorite Show"

# Export all series states to JSON
schedularr state export backup-2026-01-12.json

# Import series states from JSON (overwrites existing entries by show title)
schedularr state import backup-2026-01-12.json

# Whole-database SQLite backup (SQLite VACUUM INTO -- safe to run against a live database)
schedularr state backup full-backup-2026-01-12.db
```

---

### API Server + Cron

#### `serve`

Run the HTTP API server (blocks CRUD, schedule generate/apply, history,
series state, channels, status, `/openapi.json`) and the cron scheduling
loop together in one long-lived process.

**Usage:**

```bash
# Start the server (requires a bearer token -- via env or config)
SCHEDULARR_API_TOKEN=$(openssl rand -hex 32) schedularr serve --listen :8484

# Local development only -- disables bearer auth entirely
schedularr serve --insecure-no-auth

# Override the cron loop interval (default 6h, or the cron_interval config key)
schedularr serve --interval 1h
```

**Flags:**

- `--listen <addr>` - Address for the HTTP API server to listen on (default: `:8484`)
- `--insecure-no-auth` - Skip bearer-token auth on `/api/v1/*` (local development only)
- `--interval`, `-i` - Interval between cron-driven schedule generate-and-apply cycles (default: `6h`)

**Config keys:** `api.listen`, `api.token` (or the `SCHEDULARR_API_TOKEN`
env var, which always wins), `api.insecure_no_auth`, `cron_interval`
(same as `--interval`, a top-level key since it governs the cron loop, not
the HTTP server).

**Behavior:**

- `/healthz`, `/readyz`, `/metrics`, `/openapi.json` served unauthenticated;
  everything under `/api/v1/*` requires `Authorization: Bearer <token>`
- Every other path serves the embedded web UI (dashboard, blocks,
  schedule, series -- see [README's Web UI section](../README.md#-web-ui))
  at the same origin and port, unauthenticated for the static assets
  themselves; the UI's own API calls carry the same bearer token, pasted
  once into the browser and kept in `localStorage`
- Refuses to start if the effective token is empty or shorter than 32
  characters, unless `--insecure-no-auth`/`api.insecure_no_auth` is set
- Cron loop regenerates and applies the next day's schedule on the
  configured interval (and once immediately at startup)
- Graceful shutdown on SIGTERM/SIGINT: HTTP server drains (15s timeout),
  then the cron loop stops, then the store closes

---

### Standalone Health Probe

#### `health`

Starts a minimal HTTP server exposing `/healthz` and `/livez`, both always
returning `200 OK`. This is unrelated to `serve`'s own `/healthz`/`/readyz`
-- it exists for deployments that run Schedularr as a one-shot CLI
invocation (cron, a Kubernetes `Job`, ...) but still want a liveness probe
during that run.

**Usage:**

```bash
schedularr health --port 9600
```

---

### Channel Management

#### `channels`

List all available Tunarr channels.

**Usage:**

```bash
schedularr --config config.yaml channels
```

**Output:** a table of `ID`, `Number`, `Name`, `Group`. Used to look up
channel IDs for scheduler blocks.

---

## Examples

### Quick Start Workflow

```bash
# 1. Generate application config
schedularr config generate config.yaml

# 2. Edit config.yaml: set tunarr.url and quote tunarr.api_key (see note
#    on placeholders above)
vim config.yaml

# 3. Generate scheduler config
schedularr scheduler init scheduler.yaml

# 4. Edit scheduler blocks (set `type: filter` or `type: series` explicitly
#    on each block -- see the validate note above)
vim scheduler.yaml

# 5. Validate both
schedularr validate config.yaml
schedularr validate scheduler.yaml

# 6. Test schedule generation (dry-run; also bootstraps scheduler.yaml
#    into the store on first run)
schedularr --config config.yaml generate

# 7. Apply schedule to Tunarr
schedularr --config config.yaml generate --apply --yes

# 8. Start the API server + cron loop for continuous scheduling
SCHEDULARR_API_TOKEN=$(openssl rand -hex 32) schedularr --config config.yaml serve
```

---

## Configuration Files

### Application Config (`config.yaml`)

Generated with `schedularr config generate`. Full key reference is in the
[README's Configuration section](../README.md#️-configuration).

### Scheduler Config (`scheduler.yaml`)

Generated with `schedularr scheduler init`:

```yaml
blocks:
  - name: "Morning Cartoons"
    type: filter
    cron: "0 6 * * *"
    duration: 240
    channel_id: "channel-1"
    priority: 10
    filter:
      genres: ["Animation", "Family"]
      ratings: ["TV-Y", "TV-G"]
      max_duration: 30
      year_from: 2000
```

---

## Exit Codes

- `0` - Success
- `1` - Error (validation failure, runtime error, etc.)

---

## Environment Variables

- `SCHEDULARR_CONFIG` - Override default app config file location
- `SCHEDULARR_API_TOKEN` - Bearer token for `serve`'s `/api/v1/*` (always wins over the `api.token` config key)
- `${VAR}` placeholders inside a config file's string values are expanded from the process environment at load time

---

## See Also

- [README: API](../README.md#-api)
- [README: Serve](../README.md#-serve-api-server--cron)
- [Architecture Documentation](ARCHITECTURE.md)
- [Project TODO](../TODO.md)
