# Project TODOs

## Recent Updates (2026-01-14)

### ✅ Radarr/Sonarr/Jellyfin Integration (2026-01-14)

- **Radarr Availability Filtering**: Filter Tunarr movie programs to titles with files in Radarr
- **Sonarr Availability Filtering**: Filter Tunarr episode programs to episodes with files in Sonarr
- **Jellyfin Live TV Refresh**: Optional guide refresh after schedule apply
- **Docs Update**: Added media API research and config references

---

## Recent Updates (2026-01-12)

### ✅ Phase 0.1 & 0.2 Completed - CUE Schema & CLI Restructuring

**Major Changes:**

- **CUE Schema Integration**: Full schema-based validation and generation for both app and scheduler configs
- **CLI Migration**: Moved from `cmd/schedularr/main.go` + `internal/cli/` to standard Cobra structure (`main.go` + `cmd/`)
- **New Commands**: `config generate`, `validate`, updated `scheduler init` to use CUE schemas
- **Embedded Schemas**: Runtime schema validation without external files

**New CLI Structure:**

```text
main.go                    # Entry point
cmd/
  ├── root.go             # Root command with Viper config
  ├── config.go           # Config management (NEW)
  ├── validate.go         # CUE validation (NEW)
  ├── scheduler.go        # Updated to use CUE generation
  ├── generate.go         # Schedule generation
  ├── run.go              # Daemon mode
  ├── tui.go              # Interactive TUI
  ├── channels.go         # List channels
  └── schema/             # CUE schemas
      ├── config.cue      # App config schema
      └── scheduler.cue   # Scheduler config schema
```

**Testing Status:**

- ✅ All existing tests pass
- ✅ Config generation tested and working
- ✅ Scheduler generation tested and working
- ✅ Validation tested with valid and invalid configs
- ✅ Build succeeds without errors

---

## Overview

This TODO is structured to align Schedularr with established architectural patterns from the athena project while maintaining its unique scheduling functionality. The alignment focuses on:

- **CUE Schema Validation**: Configuration validation using CUE language for type safety and defaults ✅
- **CLI Structure**: Standardized command structure (root, validate, generate, run) ✅
- **Code Quality**: Strict linting rules, structured logging, and error handling patterns
- **Documentation**: Comprehensive docs (ARCHITECTURE.md, SPECIFICATIONS.md, ROADMAP.md)
- **Build Tooling**: Makefile-based build system with E2E testing support
- **Testing**: Table-driven tests, integration tests, and E2E test infrastructure

## Project Status Overview

| Phase       | Status          | Description                                 |
| :---------- | :-------------- | :------------------------------------------ |
| **Phase 0** | ✅ Completed    | Architecture alignment with athena patterns |
| **Phase 1** | ✅ Completed    | Foundation & API verification               |
| **Phase 2** | ✅ Completed    | Scheduler file architecture                 |
| **Phase 3** | ✅ Completed    | Enhanced scheduling engine                  |
| **Phase 4** | 🟡 In Progress | Operational excellence & testing            |
| **Phase 5** | ✅ Completed    | UX enhancements                             |

## Phase 2: Scheduler File Architecture

### 2.1 Configuration Separation

- [x] Update `validate` command to validate scheduler files (deferred)

### 2.2 Scheduler File Management Commands

- [x] Interactive prompts for initial setup (partially completed - configurable flags added to config and scheduler init commands)
- [x] Channel ID validation against Tunarr (deferred)
- [x] Filter by channel, priority, or status (deferred)

### 2.3 Series-Based Scheduling (CORE FEATURE)

#### 2.3.2 Persistence Layer

- [x] Backup before major operations (deferred)

### 2.4 TUI for Scheduler Management (Optional Advanced Features)

- [ ] Scheduler File Browser: TUI to browse and select scheduler files (optional)
- [ ] Visual cron expression builder (optional)
- [ ] Series selector with search (optional)
- [ ] Filter builder interface (optional)
- [ ] Series Progress Viewer: Display current episode for each series (optional)
- [ ] Series Progress Viewer: Show completion percentage (optional)
- [ ] Series Progress Viewer: Allow manual episode adjustment (optional)

## Phase 3: Enhanced Scheduling Engine

### 3.1 Content Fetching Improvements

- [x] Cache content metadata locally (deferred)
- [x] Radarr: add availability filtering configuration toggles (include/exclude missing files)
- [x] Sonarr: add availability filtering configuration toggles (include/exclude missing files)
- [x] Jellyfin: support optional Live TV refresh retries/backoff and log failures without blocking apply

## Phase 4: Operational Excellence

### 4.1 Testing & Quality

#### Current Coverage: 43.6% overall

- internal/logging: 100.0% ✅
- internal/config: 94.4% ✅
- internal/cache: 81.0% ✅
- internal/scheduler: 76.8% ✅
- internal/tunarr: 73.2% ✅
- internal/store: 72.8% ✅
- internal/radarr: 72.1% ✅
- internal/jellyfin: 72.0% ✅
- internal/sonarr: 70.7% ✅
- internal/cueconfig: 69.6%
- cmd: 6.1% (CLI commands - hard to test)
- internal/tui: 0.0% (TUI - hard to test)

#### Tasks

- [ ] Unit test coverage >80% (currently ~45% overall, core packages >70%)
- [x] Table-driven tests for all core functions
- [x] Error path and edge case tests (mostly done)
- [x] Integration tests: full scheduling workflow
- [x] Create test fixtures with sample data
- [x] Mock Tunarr API responses (partially done)
- [x] Test configuration loading and validation
- [ ] Integration tests against real Tunarr instance
- [ ] Test CLI commands with real files
- [ ] E2E tests: scheduling against real Tunarr instance
- [ ] E2E tests: verify schedule updates
- [ ] E2E tests: daemon mode operation
- [ ] E2E tests: graceful shutdown

### 4.2 Deployment & Operations

- [x] Dockerization: health check endpoint (cmd/health.go implemented)
- [x] Observability: migrate remaining logging to slog (complete)

## Phase 5: UX Enhancements

### 5.2 TUI Enhancements

- [ ] Filter editor: visual filter rule builder
- [ ] Keyboard shortcuts: document and improve shortcuts
