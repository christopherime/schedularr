# Changelog

All notable changes to Schedularr will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added - 2026-08-28

#### HTTP API Server

- **`schedularr serve`**: new command running the HTTP API and the cron
  scheduling loop in one long-lived process, replacing the old daemon
  loop (see "Removed" below). Endpoints: blocks CRUD, block
  import/export, schedule generate/apply, schedule history, series
  state, channels, status, plus unauthenticated `/healthz`, `/readyz`,
  `/metrics`, and `/openapi.json`.
- **Contract-first API**: `api/openapi.yaml` (OpenAPI 3.0.3) is the
  source of truth; `internal/api/gen/server.gen.go` is generated from it
  via `make generate` (oapi-codegen) and must not be hand-edited.
- **Bearer-token auth**: `/api/v1/*` requires `Authorization: Bearer
  <token>`, constant-time compared. Token comes from
  `SCHEDULARR_API_TOKEN` (wins when set) or the `api.token` config key;
  `serve` refuses to start with a token under 32 characters unless
  `--insecure-no-auth`/`api.insecure_no_auth` is set.
- **`internal/service`**: extracted the generate/apply workflow out of
  the CLI so `cmd/generate.go` and the API's schedule endpoints share one
  implementation (`service.Runner`).
- **`internal/store`**: SQLite-backed persistence for blocks, series
  state, and schedule history (see "Changed" below).

### Changed - 2026-08-28

#### Module rename

- Module path changed from `github.com/geekxflood/schedularr` to
  `github.com/christopherime/schedularr` (repository transferred to a
  new owner). Every import path was rewritten in the same change;
  `schema.json`'s `$id` was missed at the time and corrected later in
  this same sub-project.

#### Blocks moved into the SQLite store

- Scheduling blocks now live in `internal/store`, not in a config file.
  `scheduler.yaml` is a **first-run import format only**: on an empty
  store, `internal/blockio.Bootstrap` imports its blocks once; the file
  is never read again afterward, and editing it post-bootstrap has no
  effect. Manage blocks going forward through `/api/v1/blocks`.
- `schedularr scheduler init` still authors a `scheduler.yaml` import
  file; `config.yaml`'s inline `scheduler:` block (legacy support) is no
  longer consulted by any code path -- config.cue documents the field but
  nothing reads it (flagged for cleanup, not yet removed).

#### `serve` replaces `run`

- The `run` daemon command (interval-based generate-and-apply loop,
  SIGHUP config reload) is gone; `serve` runs the same generate-and-apply
  cycle on a cron timer alongside the HTTP API, sharing one store
  connection and one graceful-shutdown path. SIGHUP reload and `--once`
  were not carried over -- `serve` has no config-reload story and is
  always long-running.
- Cadence is controlled by the `cron_interval` config key (default `6h`,
  a top-level key since it governs the scheduler, not the HTTP server) or
  `serve --interval`/`-i`, which overrides it when passed explicitly.

### Removed - 2026-08-28

#### Interactive TUI

- Deleted entirely: `internal/tui/`, `cmd/tui.go`, and the
  `charmbracelet/bubbletea`/`lipgloss`/`huh` dependencies. No deprecated
  alias was kept.
- `generate --apply` now requires an explicit `--yes` flag -- the
  `charmbracelet/huh` confirmation prompt it used to show is gone, and
  there is no other interactive confirmation. `--apply` without `--yes`
  fails fast with an error instead of running.

#### Jellyfin, Sonarr, and Radarr integrations

- Removed `internal/external/jellyfin/`, `internal/external/sonarr/`,
  `internal/external/radarr/`, `cmd/content_sources.go`, and their config
  sections in `cmd/schema/config.cue`. Tunarr is now the sole runtime
  integration; content availability filtering and the Jellyfin
  guide-refresh hook were removed along with the clients, not ported.

#### `run` command

- The interval-based daemon command is gone; see "`serve` replaces
  `run`" above.

### Added - 2026-01-12

#### CUE Schema Integration

- **CUE Schema Files**: Created comprehensive schemas for application and scheduler configurations
  - `cmd/schema/config.cue` - Application configuration schema with validation and defaults
  - `cmd/schema/scheduler.cue` - Scheduler configuration schema with block types and filters
  - Embedded schemas in `internal/cueconfig/schema/` for runtime use
- **Schema Validation Package**: New `internal/cueconfig` package for CUE-based validation
  - `ValidateConfig()` - Validates application configuration against schema
  - `ValidateScheduler()` - Validates scheduler configuration against schema
  - `GenerateConfig()` - Generates config files from schema with defaults
  - `GenerateScheduler()` - Generates scheduler files from schema with defaults
  - Support for both YAML and JSON formats

#### CLI Restructuring

- **New CLI Structure**: Migrated from `cmd/schedularr/main.go` + `internal/cli/` to standard Cobra layout
  - Entry point: `main.go`
  - Commands: `cmd/` package
  - Removed old `internal/cli/` directory
- **New Commands**:
  - `config generate [filename]` - Generate application config from CUE schema
  - `validate <file>` - Validate any config file against CUE schemas
  - `scheduler init [filename]` - Generate scheduler config from CUE schema (updated)
  - `scheduler validate [filename]` - Validate scheduler config
  - `scheduler list [filename]` - List all configured blocks
- **Enhanced Root Command**:
  - Updated descriptions and examples
  - Integrated Viper for configuration management
  - Added `--config` global flag for custom config files
  - Auto-detection of config files in home and current directories

#### Documentation

- **CLI Reference**: Comprehensive CLI documentation in `docs/CLI_REFERENCE.md`
  - Complete command reference with examples
  - Configuration file templates
  - Quick start workflows
  - Exit codes and environment variables
- **Updated README**:
  - New installation instructions
  - Updated quick start guide with new commands
  - Added reference to CLI documentation
  - Updated configuration examples
- **Updated TODO**: Marked Phase 0.1 and 0.2 as completed with detailed notes

### Changed

#### Build Process

- **Build Command**: Updated from `go build -o schedularr cmd/schedularr/main.go` to `go build -o schedularr main.go`
- **Import Paths**: All CLI commands now use `cmd` package instead of `internal/cli`

#### Configuration Generation

- **Scheduler Init**: Changed from template-based to CUE schema-based generation
  - Removed hardcoded templates (basic, advanced, series)
  - Now generates from schema defaults with example blocks
  - Auto-detects output format from file extension

#### Validation

- **Config Loading**: Removed inline CUE validation from config loading
  - Validation now explicit via `validate` command
  - Runtime validation can be added separately if needed

### Removed

- **Old CLI Structure**: Removed `internal/cli/` directory and all files
- **Old Entry Point**: Removed `cmd/schedularr/main.go` and `schedularr/` directory
- **Template Files**: Removed hardcoded scheduler templates from `scheduler.go`
- **Validator File**: Removed `internal/config/validator.go` (replaced by `internal/cueconfig`)

### Technical Details

#### Dependencies

- Added `cuelang.org/go/cue` for CUE schema support
- Added `cuelang.org/go/cue/cuecontext` for CUE context management
- Using `gopkg.in/yaml.v3` for YAML marshaling

#### File Structure

```txt
schedularr/
├── main.go                          # Entry point
├── cmd/
│   ├── root.go                      # Root command
│   ├── config.go                    # Config management (NEW)
│   ├── validate.go                  # Validation (NEW)
│   ├── scheduler.go                 # Scheduler management (UPDATED)
│   ├── generate.go                  # Schedule generation
│   ├── run.go                       # Daemon mode
│   ├── tui.go                       # Interactive TUI
│   ├── channels.go                  # Channel listing
│   └── schema/                      # CUE schemas
│       ├── config.cue               # App config schema
│       └── scheduler.cue            # Scheduler config schema
├── internal/
│   ├── cueconfig/                   # CUE validation (NEW)
│   │   ├── schema.go
│   │   └── schema/
│   │       ├── config.cue           # Embedded
│   │       └── scheduler.cue        # Embedded
│   ├── config/                      # Config loading
│   ├── scheduler/                   # Scheduling engine
│   ├── store/                       # State persistence
│   ├── tui/                         # TUI components
│   └── tunarr/                      # Tunarr API client
└── docs/
    └── CLI_REFERENCE.md             # CLI documentation (NEW)
```

#### Testing

- ✅ All existing tests pass
- ✅ Config generation tested with YAML and JSON
- ✅ Scheduler generation tested with YAML and JSON
- ✅ Validation tested with valid and invalid configs
- ✅ Build succeeds without errors

### Migration Guide

For users upgrading from previous versions:

1. **Update Build Command**:

   ```bash
   # Old
   go build -o schedularr cmd/schedularr/main.go

   # New
   go build -o schedularr main.go
   ```

2. **Generate New Configs**:

   ```bash
   # Generate application config
   schedularr config generate config.yaml

   # Generate scheduler config
   schedularr scheduler init scheduler.yaml
   ```

3. **Validate Existing Configs**:

   ```bash
   schedularr validate ~/.schedularr.yaml
   schedularr validate scheduler.yaml
   ```

4. **Update Scripts**: If you have automation scripts, update command paths and imports

---

## [0.1.0] - 2026-01-XX (Previous Release)

### Added

- Initial release with basic scheduling functionality
- Tunarr API integration
- Filter-based content selection
- Cron-based scheduling
- Interactive TUI
- CLI commands: channels, generate, run, tui

[Unreleased]: https://github.com/geekxflood/schedularr/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/geekxflood/schedularr/releases/tag/v0.1.0
