# CLI Reference

```bash
schedularr [command] [flags]
```

![schedularr --help followed by schedularr validate against a fixture scheduler config](assets/cli.gif)

*`schedularr --help` and `schedularr validate` against a fixture in `testdata/configs/`.*

All commands support the global `--config` flag to specify a custom app config file.

## Global flags

- `--config <file>` — app config file. Resolution order: this flag, then `SCHEDULARR_CONFIG`, then legacy locations (`./config.yaml`, `./.schedularr.yaml`, `~/.config/.schedularr.yaml`, `~/.schedularr.yaml`), then the default `~/.schedularr/config.yaml`.
- `--help`, `-h` — show help for any command.

## Configuration management

### `config generate [filename]`

Generates an application configuration file from the CUE schema with default values. Also available as `generate config --output <filename>` (same generator, different subcommand tree).

```bash
schedularr config generate                 # Default config.yaml
schedularr config generate my-config.yaml  # Custom filename
schedularr config generate config.json     # JSON format

# Override specific keys
schedularr config generate my-config.yaml --tunarr-url "http://my-tunarr:8000" --log-level debug
```

Creates a YAML or JSON file (auto-detected from extension) with every key in `cmd/schema/config.cue` and its default value. `tunarr.url` and `tunarr.api_key` default to literal `${SCHEDULARR_TUNARR_URL}`/`${SCHEDULARR_TUNARR_API_KEY}` placeholders — either export those variables, or edit the file and replace the placeholders with literal values, always quoting the value even when empty (`api_key: ""`). An unset `${VAR}` placeholder left unquoted expands to nothing, which YAML parses as `null` rather than `""`, and `null` fails CUE validation on load.

### `config dump`

Prints the currently loaded effective configuration (after resolving the config file, defaults, and environment overrides) as YAML.

```bash
schedularr --config config.yaml config dump
```

## Scheduler configuration

### `scheduler init [filename]`

Generates a `scheduler.yaml` block-import file from the CUE schema with an example block. Blocks live in the SQLite store, not in this file — `scheduler.yaml` is read once, on the first run against an empty store (`internal/blockio.Bootstrap`), and ignored after that. Manage blocks afterward through the `/api/v1/blocks` HTTP API.

```bash
schedularr scheduler init                    # Default scheduler.yaml
schedularr scheduler init my-schedule.yaml   # Custom filename
schedularr scheduler init schedule.json      # JSON format

# Override the first block's fields
schedularr scheduler init my-schedule.yaml --name "Morning Cartoons" --cron "0 8 * * *" --duration 180 --channel-id "kids-channel" --priority 5
```

Validate the output before deploying: `schedularr validate my-schedule.yaml`.

## Validation

### `validate <file>`

Validates an application config file or a `scheduler.yaml` block-import file against the CUE schemas in `cmd/schema/`.

File type is inferred from the filename: a name containing `scheduler` (case-sensitive substring match) is validated as a block-import file (strict YAML decode, duplicate-block-name check, per-block CUE validation — the same path `blockio.Bootstrap` and `POST /api/v1/blocks/import` use); anything else is validated as an app config. Name scheduler files accordingly (`scheduler.yaml`, `my-scheduler.yaml`, ...), or `validate` checks them against the wrong schema.

```bash
schedularr validate config.yaml
schedularr validate scheduler.yaml
```

Each block's `type` field (`filter` or `series`) has a CUE schema default, but the scheduler-file validation path decodes each block into a Go struct first, turning an omitted `type` into an explicit empty string rather than an absent field. CUE only applies a default to a genuinely absent field, so an empty string fails the `"filter" | "series"` enum check — always set `type` explicitly in `scheduler.yaml`. `POST /api/v1/blocks`'s JSON body doesn't have this problem.

**Exit codes:** `0` validation passed, `1` validation failed (shows detailed errors).

## Schedule generation

### `generate`

Generates (and optionally applies) a schedule from the blocks currently in the store. On an empty store, first bootstraps `scheduler_file`'s blocks into it.

```bash
schedularr generate                    # Dry run (preview only, never mutates the store or Tunarr)
schedularr generate --apply --yes      # Apply to Tunarr (--yes is mandatory; there is no interactive prompt)
schedularr generate --verbose          # Raise logging to debug
```

| Flag | Description |
| --- | --- |
| `--apply` | Push the generated schedule to Tunarr (requires `--yes`) |
| `--yes` | Required alongside `--apply`; there is no interactive confirmation |
| `--dry-run` | Preview only, even if `--apply` is also set |
| `--verbose`, `-v` | Raise logging to debug |

## Series state

### `state export/import/reset/set/list/backup`

Manages series progression state (`internal/store`, table `series_state`).

```bash
schedularr state list                                       # List all tracked series
schedularr state set "My Favorite Show" --season 2 --episode 5  # Jump to S02E05
schedularr state reset "My Favorite Show"                    # Reset to S01E01
schedularr state export backup-2026-01-12.json               # Export all series states to JSON
schedularr state import backup-2026-01-12.json               # Import (overwrites existing entries by show title)
schedularr state backup full-backup-2026-01-12.db             # Whole-database SQLite backup (VACUUM INTO, safe against a live database)
```

`set` and `reset` both invalidate every not-yet-*finished* occurrence's cursor snapshot for every block that schedules the show — including one currently on air — the same way `PATCH /state/series/{show_title}` does (see the [API Reference](api-reference.md#series-state)), so the change takes effect on the very next apply and stays in effect. A failure at that invalidation step (rare — the show/season/episode change itself has already been saved by then) prints a warning to stderr rather than failing the command.

## API server + cron

### `serve`

Runs the HTTP API server (blocks CRUD, schedule generate/apply, history, series state, channels, status, `/openapi.json`), the embedded web UI, and the cron scheduling loop together in one long-lived process — see the [Web UI Guide](web-ui-guide.md) and [API Reference](api-reference.md).

```bash
# Start the server (requires a bearer token -- via env or config)
SCHEDULARR_API_TOKEN=$(openssl rand -hex 32) schedularr serve --listen :8484

# Local development only -- disables bearer auth entirely
schedularr serve --insecure-no-auth

# Override the cron loop interval (default 6h, or the cron_interval config key)
schedularr serve --interval 1h
```

| Flag | Description |
| --- | --- |
| `--listen <addr>` | Address for the HTTP API server to listen on (default `:8484`) |
| `--insecure-no-auth` | Skip bearer-token auth on `/api/v1/*` (local development only) |
| `--interval`, `-i` | Interval between cron-driven schedule generate-and-apply cycles (default `6h`) |

**Config keys:** `api.listen`, `api.token` (or `SCHEDULARR_API_TOKEN`, which always wins), `api.insecure_no_auth`, `cron_interval` — see the [Deployment config reference](deployment.md#configuration-reference).

**Behavior:**

- `/healthz`, `/readyz`, `/metrics`, `/openapi.json` served unauthenticated; everything under `/api/v1/*` requires `Authorization: Bearer <token>`.
- Every other path serves the embedded web UI, unauthenticated for the static assets themselves — the UI's own API calls carry the same bearer token, pasted once into the browser and kept in `localStorage`.
- Refuses to start if the effective token is empty or shorter than 32 characters, unless `--insecure-no-auth`/`api.insecure_no_auth` is set.
- Cron loop regenerates and applies the next day's schedule on the configured interval (and once immediately at startup).
- Graceful shutdown on SIGTERM/SIGINT: HTTP server drains (15s timeout), then the cron loop stops, then the store closes.

## Standalone health probe

### `health`

Starts a minimal HTTP server exposing `/healthz` and `/livez`, both always returning `200 OK`. Unrelated to `serve`'s own `/healthz`/`/readyz` — this exists for deployments that run Schedularr as a one-shot CLI invocation (cron, a Kubernetes `Job`, ...) but still want a liveness probe during that run.

```bash
schedularr health --port 9600
```

## Channel management

### `channels`

Lists all available Tunarr channels.

```bash
schedularr --config config.yaml channels
```

Output: a table of `ID`, `Number`, `Name`, `Group`. Used to look up channel IDs for scheduler blocks.

## Quick start workflow

```bash
# 1. Generate application config
schedularr config generate config.yaml

# 2. Edit config.yaml: set tunarr.url and quote tunarr.api_key
vim config.yaml

# 3. Generate scheduler config
schedularr scheduler init scheduler.yaml

# 4. Edit scheduler blocks (set `type: filter` or `type: series` explicitly on each block)
vim scheduler.yaml

# 5. Validate both
schedularr validate config.yaml
schedularr validate scheduler.yaml

# 6. Test schedule generation (dry-run; also bootstraps scheduler.yaml into the store on first run)
schedularr --config config.yaml generate

# 7. Apply schedule to Tunarr
schedularr --config config.yaml generate --apply --yes

# 8. Start the API server + cron loop for continuous scheduling
SCHEDULARR_API_TOKEN=$(openssl rand -hex 32) schedularr --config config.yaml serve
```

## Exit codes

- `0` — success
- `1` — error (validation failure, runtime error, etc.)

## Environment variables

- `SCHEDULARR_CONFIG` — override the default app config file location.
- `SCHEDULARR_API_TOKEN` — bearer token for `serve`'s `/api/v1/*` (always wins over the `api.token` config key).
- `${VAR}` placeholders inside a config file's string values are expanded from the process environment at load time.
