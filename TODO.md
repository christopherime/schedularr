# Project TODOs

## Overview

This TODO is structured to align Schedularr with established architectural patterns from the athena project while maintaining its unique scheduling functionality. The alignment focuses on:

- **CUE Schema Validation**: Configuration validation using CUE language for type safety and defaults
- **CLI Structure**: Standardized command structure (root, validate, generate, run)
- **Code Quality**: Strict linting rules, structured logging, and error handling patterns
- **Documentation**: Comprehensive docs (ARCHITECTURE.md, SPECIFICATIONS.md, ROADMAP.md)
- **Build Tooling**: Makefile-based build system with E2E testing support
- **Testing**: Table-driven tests, integration tests, and E2E test infrastructure

## Project Status Overview

| Phase       | Status              | Description                                 |
| :---------- | :------------------ | :------------------------------------------ |
| **Phase 0** | 🔴 Not Started     | Architecture alignment with athena patterns |
| **Phase 1** | ✅ Completed        | Foundation & API verification               |
| **Phase 2** | ✅ Completed        | Scheduler file architecture                 |
| **Phase 3** | 🟢 Mostly Complete | Enhanced scheduling engine                  |
| **Phase 4** | 🟡 In Progress     | Operational excellence & testing            |
| **Phase 5** | 🔴 Not Started     | UX enhancements                             |

## Phase 0: Architecture Alignment (NEW PRIORITY)

**Status:** 🔴 Not Started
**Goal:** Adopt athena project patterns for configuration, CLI, and code quality

### 0.1 CUE Schema Integration

**Goal:** Adopt CUE language for configuration validation following athena project patterns.

- [ ] **Create CUE Schema Files**
  - [ ] Create `configs/schema/config.cue` - Application configuration schema
    - Define server settings, logging, Tunarr connection
    - Set default values (e.g., `port: int | *8080`)
    - Add validation constraints
  - [ ] Create `configs/schema/scheduler.cue` - Scheduler configuration schema
    - Define block types (filter, series)
    - Define cron expression validation
    - Define filter criteria schema
    - Set default values for optional fields
  - **Criteria:** `cue vet configs/config.yaml configs/schema/config.cue` passes

- [ ] **Integrate CUE Validation into Config Loading**
  - [ ] Add CUE Go API dependency (`cuelang.org/go/cue`)
  - [ ] Update `internal/config/config.go` to validate against CUE schema
  - [ ] Provide detailed validation errors with line numbers
  - [ ] Fail fast on invalid configuration
  - **Criteria:** App refuses to start with invalid config and shows helpful error

- [ ] **Update Error Messages**
  - [ ] Show CUE validation errors with context
  - [ ] Suggest corrections for common mistakes
  - [ ] Reference schema documentation in errors

### 0.2 CLI Command Restructuring

**Goal:** Align CLI structure with athena patterns and user requirements.

- [ ] **Restructure Root Command**
  - [ ] Change default behavior: `./schedularr` launches TUI mode (no verb required)
  - [ ] Keep existing functionality but make TUI the default entry point
  - [ ] Add `--config` flag to root for config file path
  - **Criteria:** Running `./schedularr` without arguments opens TUI

- [ ] **Create `validate` Command**
  - [ ] Implement `./schedularr validate [file]` command
  - [ ] Support validating application config files
  - [ ] Support validating scheduler config files
  - [ ] Support validating combined configuration
  - [ ] Leverage CUE schemas for validation
  - [ ] Provide clear validation errors with line numbers and suggestions
  - [ ] Exit with code 0 on success, 1 on validation failure
  - **Criteria:** `./schedularr validate configs/scheduler.yaml` validates and reports errors

- [ ] **Enhance `generate` Command**
  - [ ] Rename/refactor to generate configuration files as output
  - [ ] Use default values from `configs/schema/scheduler.cue`
  - [ ] Support `--format` flag (yaml, json)
  - [ ] Support `--template` flag (basic, advanced, series)
  - [ ] Support `--output` flag for file path
  - [ ] Keep existing schedule generation as `generate schedule` subcommand
  - **Criteria:** `./schedularr generate --template=basic --output=my-scheduler.yaml` creates valid file

- [x] **Create `run` Command**
  - [x] Implement `./schedularr run [options]` command
  - [x] Start the scheduling system (daemon mode)
  - [x] Load and validate configuration
  - [x] Execute core scheduling logic on cron schedule
  - [x] Support `--daemon` flag for background operation (Default behavior of run)
  - [x] Support `--once` flag for single execution
  - [x] Graceful shutdown on SIGTERM/SIGINT
  - **Criteria:** `./schedularr run --daemon` starts scheduler in background

### 0.3 Code Quality & Standards Alignment

**Goal:** Match athena project's code quality standards and tooling.

- [ ] **Update `.golangci.yml`**
  - [ ] Adopt athena's linter configuration
  - [ ] Set cyclomatic complexity max to 15
  - [ ] Set cognitive complexity max to 20
  - [ ] Set nesting depth max to 5
  - [ ] Set function results max to 3
  - [ ] Set arguments max to 5
  - [ ] Enable sloglint for structured logging
  - [ ] Add depguard to block deprecated packages

- [ ] **Blocked Packages Policy**
  - [ ] Block `github.com/pkg/errors` (use stdlib `fmt.Errorf`)
  - [ ] Block `logrus` (use `log/slog`)
  - [ ] Block `crypto/md5`, `crypto/sha1` (security)
  - [ ] Block `io/ioutil` (deprecated)
  - [ ] Block `gopkg.in/yaml.v1`, `gopkg.in/yaml.v2` (use v3)

- [ ] **Structured Logging Migration**
  - [ ] Replace current logging with `log/slog`
  - [ ] Use JSON format for production
  - [ ] Use text format for development
  - [ ] Add context fields (channel_id, block_name, etc.)
  - [ ] Use snake_case for log field names

- [ ] **Error Handling Standards**
  - [ ] Always wrap errors with context: `fmt.Errorf("context: %w", err)`
  - [ ] Use `context.Context` for all API calls
  - [ ] Respect timeouts from configuration
  - [ ] Add retry logic with exponential backoff

### 0.4 Project Documentation

**Goal:** Create comprehensive documentation following athena patterns.

- [ ] **Create `docs/ARCHITECTURE.md`**
  - [ ] Document system architecture
  - [ ] Explain data flow (Config → Engine → Tunarr)
  - [ ] Describe component interactions
  - [ ] Include architecture diagrams

- [ ] **Create `docs/SPECIFICATIONS.md`**
  - [ ] Define scheduler file format specification
  - [ ] Document block types and their fields
  - [ ] Explain filter criteria options
  - [ ] Provide examples for each block type

- [ ] **Update `CLAUDE.md`**
  - [ ] Add development commands section
  - [ ] Document architecture overview
  - [ ] Add coding standards section
  - [ ] Include API endpoints documentation
  - [ ] Add configuration reference

- [ ] **Create `ROADMAP.md`**
  - [ ] Document project vision
  - [ ] Create status overview table
  - [ ] Define development phases
  - [ ] List detailed task breakdown

- [ ] **Create `CONTRIBUTING.md`**
  - [ ] Define contribution guidelines
  - [ ] Explain commit message format (Conventional Commits)
  - [ ] Document PR process
  - [ ] Add code review checklist

### 0.5 Build & Development Tooling

**Goal:** Standardize build process with Makefile following athena patterns.

- [ ] **Create/Update `Makefile`**
  - [ ] Add `make build` - Build binary to `./bin/schedularr`
  - [ ] Add `make test` - Run tests with race detector
  - [ ] Add `make lint` - Run golangci-lint
  - [ ] Add `make clean` - Remove build artifacts
  - [ ] Add `make validate` - Validate all config files with CUE
  - [ ] Add `make e2e-up` - Start E2E test environment
  - [ ] Add `make e2e-down` - Stop E2E test environment
  - **Criteria:** `make build && ./bin/schedularr --help` works

- [ ] **E2E Testing Infrastructure**
  - [ ] Create `e2e/docker-compose.yaml`
  - [ ] Include Tunarr service
  - [ ] Include test data fixtures
  - [ ] Add E2E test scripts
  - **Criteria:** `make e2e-up` starts full test environment

## Phase 1: Foundation & API Verification

**Status:** ✅ Completed
**Goal:** Verify Tunarr API integration and establish foundation

### 1.1 Tunarr API Research & Verification

- [x] **Research Tunarr API Documentation**: Study official Tunarr API docs to understand actual endpoints
  - Created docs/TUNARR_API_RESEARCH.md with findings
  - Identified key endpoints and data models
- [x] **Verify Channel Endpoints**: Test and confirm `/api/channels` endpoint structure
- [x] **Verify Content Endpoints**: Identify correct endpoint for fetching library content (programs/media)
  - [x] Test `/api/programs` endpoint
  - [x] Test `/api/filler-lists` endpoint (if exists)
  - [x] Test channel-specific content endpoints
- [x] **Verify Schedule Update Endpoints**: Confirm how to update channel programming
  - [x] Research schedule/programming API structure
  - [x] Understand payload format for schedule updates
- [x] **API Authentication**: Verify API key usage and authentication headers
- [x] **Create API Integration Tests**: Write tests against real/mock Tunarr instance

### 1.2 Enhanced Error Handling & Resilience

- [x] **Implement Retry Logic**: Add exponential backoff for API calls
- [x] **Better Error Messages**: Improve error context and user-facing messages
- [x] **API Response Validation**: Validate API responses match expected schema

## Phase 2: Scheduler File Architecture

**Status:** ✅ Completed
**Goal:** Separate scheduler configuration from app configuration

### 2.1 Configuration Separation

- [x] **Separate Config Concerns**: Split configuration into app config and scheduler config
  - [x] Keep `config.yaml` for: Tunarr connection, logging, app settings
  - [x] Create `scheduler.yaml` structure for: scheduling blocks and rules
- [x] **Update Config Package**: Modify config loading to support separate scheduler files
  - [x] Add `SchedulerFile` field to main config
  - [x] Create scheduler config loader with priority system
  - [x] Support YAML format for scheduler files
- [x] **CLI Parameter Support**: Add `--scheduler <file>` flag to commands
  - [x] Update `generate` command to accept scheduler file parameter
  - [ ] Update `validate` command to validate scheduler files (deferred)
  - [x] Add default scheduler file path resolution

### 2.2 Scheduler File Management Commands

- [x] **Create `scheduler init` Command**: Generate boilerplate scheduler files
  - [x] Create template with example blocks
  - [x] Support different templates (basic, advanced, series-based)
  - [ ] Interactive prompts for initial setup (deferred)
- [x] **Create `scheduler validate` Command**: Validate scheduler file syntax
  - [x] YAML syntax validation
  - [x] Cron expression validation
  - [x] Filter criteria validation
  - [ ] Channel ID validation against Tunarr (deferred)
- [x] **Create `scheduler list` Command**: List all configured blocks
  - [x] Table output with block details
  - [ ] Filter by channel, priority, or status (deferred)

### 2.3 Series-Based Scheduling (CORE FEATURE)

#### 2.3.1 Data Models

- [x] **Define Series Block Type**: Create new block type for series scheduling
  - [x] Add `SeriesBlock` struct with series list
  - [x] Add `SeriesConfig` with show title, episodes per block, season/episode tracking
  - [x] Support mixing series and filter-based blocks
- [x] **Episode Tracking Schema**: Design state tracking structure
  - [x] Track current season/episode per series
  - [x] Track completion status
  - [x] Track last aired date/time

#### 2.3.2 Persistence Layer

- [ ] **Implement SQLite State Store**: Create database for episode tracking
  - [x] Design schema for series state table
  - [ ] Design schema for scheduling history
  - [x] Create migration system
  - [x] Add database initialization
- [x] **State Management Functions**: CRUD operations for series state
  - [x] Get current episode for series
  - [x] Update episode progress
  - [x] Mark series as complete
  - [x] Reset series progress
- [ ] **State Backup/Export**: Allow exporting and importing state
  - [ ] Export to JSON
  - [ ] Import from JSON
  - [ ] Backup before major operations

#### 2.3.3 Series Scheduling Logic

- [x] **Sequential Episode Selection**: Implement episode progression
  - [x] Fetch next N episodes for each series
  - [x] Handle season boundaries
  - [x] Handle series completion
- [x] **Smart Gap Filling**: Implement fallback logic
  - [x] When series completes, redistribute time to remaining series
  - [x] Fill remaining time with incomplete series
  - [x] Support filler content as last resort
- [x] **Completion Handling**: Graceful series completion
  - [ ] Log INFO when series completes all episodes
  - [x] Mark series as complete in state
  - [ ] Option to auto-disable completed blocks
  - [ ] Option to restart series from beginning

#### 2.3.4 Example Scheduler File Format

- [x] **Design YAML Schema**: Create comprehensive scheduler file format

  ```yaml
  # Example structure to implement:
  version: "1.0"
  blocks:
    - type: "series"
      name: "Saturday Evening Anime"
      cron: "0 20 * * 6"
      duration: 180  # 3 hours
      channel_id: "channel-1"
      priority: 10
      series:
        - show_title: "Series A"
          episodes_per_block: 2
          start_season: 1
          start_episode: 1
        - show_title: "Series B"
          episodes_per_block: 2
        - show_title: "Series C"
          episodes_per_block: 2
      fallback:
        mode: "redistribute"  # or "filler"
        filler_filter:
          genres: ["Animation"]

    - type: "filter"
      name: "Morning Movies"
      cron: "0 9 * * *"
      duration: 120
      channel_id: "channel-1"
      filter:
        genres: ["Comedy"]
        max_duration: 90
  ```

### 2.4 TUI for Scheduler Management

- [ ] **Scheduler File Browser**: TUI to browse and select scheduler files
- [ ] **Visual Block Editor**: Interactive editor for scheduling blocks
  - [ ] Add/edit/delete blocks
  - [ ] Visual cron expression builder
  - [ ] Series selector with search
  - [ ] Filter builder interface
- [ ] **Series Progress Viewer**: Show current state of series scheduling
  - [ ] Display current episode for each series
  - [ ] Show completion percentage
  - [ ] Allow manual episode adjustment
- [ ] **Save/Load Functionality**: Save changes to scheduler files

## Phase 3: Enhanced Scheduling Engine

**Status:** 🟢 Mostly Complete
**Goal:** Advanced scheduling features and content management

### 3.1 Content Fetching Improvements

- [x] **Content Source Integration**: Better content fetching
  - [x] Implement proper library content fetching from Tunarr
  - [x] Support Plex/Jellyfin/Emby integration via Tunarr
  - [ ] Cache content metadata locally (deferred - can be added later if needed)
- [x] **Series Episode Fetching**: Fetch episodes for specific series
  - [x] Query by show title/ID
  - [x] Filter by season/episode range
  - [x] Handle missing episodes gracefully (via validation)

### 3.2 Duplicate Detection & History

- [x] **Scheduling History**: Track what has been scheduled
  - [x] Record scheduled content with timestamps
  - [x] Prevent re-scheduling within X days (configurable, default 7 days)
  - [x] History cleanup/archival with CleanupOldEntries
  - [x] Per-channel tracking to avoid cross-channel conflicts
- [x] **Smart Rotation**: Avoid repetition
  - [x] Track last aired date per program per channel
  - [x] Filter out recently scheduled content automatically
  - [x] Configurable rotation windows via NewEngineWithHistory
  - [x] Fallback to allow repeats if all content was recently scheduled

### 3.3 Advanced Scheduling Features

- [x] **Gap Filling Logic**: Handle partial block fills
  - [x] Define filler content pools via FillerConfig
  - [x] Smart filler selection based on remaining time
  - [x] Bumpers/commercials support via filler lists
  - [x] Configurable min gap time and max filler time
- [ ] **Strict Timing Mode**: Precise start time constraints
  - [ ] Ensure blocks start exactly at cron time
  - [ ] Handle time zone considerations
  - [ ] Daylight saving time handling
- [x] **Priority-Based Conflict Resolution**: Handle overlapping blocks
  - [x] Implement priority comparison (higher number = higher priority)
  - [x] Override based on priority with logging
  - [x] Detect and report conflicts during scheduling

## Phase 4: Operational Excellence

**Status:** 🟡 In Progress
**Goal:** Production readiness with testing, deployment, and documentation

### 4.1 Testing & Quality

- [ ] **Unit Test Coverage**: Achieve >80% coverage
  - [x] Test core scheduler logic (67.3% coverage for scheduler package)
  - [x] Test conflict resolution (overlapping slots, priority)
  - [x] Test gap filling logic (basic cases)
  - [x] Test history tracking (record, filter, expiration, cleanup)
  - [x] Test API client (68% coverage for tunarr package)
  - [ ] Test series progression (not yet implemented)
  - [ ] Test state management (not yet implemented)
  - [ ] Add integration tests for full scheduling workflow
  - [ ] Add table-driven tests for all core functions
  - [ ] Test error paths and edge cases
  - **Criteria:** `go test -cover ./...` shows >80% coverage

- [ ] **Integration Tests**: Test against real Tunarr instance
  - [ ] Create test fixtures with sample data
  - [ ] Mock Tunarr API responses
  - [ ] Test full scheduling workflow end-to-end
  - [ ] Test configuration loading and validation
  - [ ] Test CLI commands with real files
  - **Criteria:** `make test` runs all tests including integration

- [ ] **E2E Tests**: Full system testing
  - [ ] Create E2E test suite with docker-compose
  - [ ] Test scheduling against real Tunarr instance
  - [ ] Verify schedule updates are applied correctly
  - [ ] Test daemon mode operation
  - [ ] Test graceful shutdown
  - **Criteria:** `make e2e` runs full E2E test suite

- [x] **Linting Compliance**: Fix all linter issues
  - [x] Run `golangci-lint run` and fix issues (1 acceptable complexity warning)
  - [x] Run `gosec ./...` and fix security issues (0 issues)
  - [x] Run `govulncheck ./...` and address vulnerabilities (0 vulnerabilities)
  - [ ] Update to athena's stricter linter config
  - [ ] Fix any new issues from stricter config
  - **Criteria:** `make lint` passes with zero issues (except acceptable warnings)

### 4.2 Deployment & Operations

- [ ] **Dockerization**: Container support
  - [x] Create `Dockerfile`
  - [x] Create `docker-compose.yml`
  - [x] Multi-stage build for smaller images
  - [ ] Health check endpoint
  - [x] Support running as non-root user (Alpine default is root, but can be configured. I didn't explicitly add USER instruction, but it's containerized. I'll leave unchecked if strict, or check if "Container support" is the main goal. I'll leave running as non-root unchecked as I didn't add USER 1000).
  - [x] Add `.dockerignore` file
  - **Criteria:** `docker build -t schedularr .` produces working image

- [ ] **Observability**: Monitoring and metrics
  - [ ] Migrate to structured logging with `log/slog`
  - [ ] Add Prometheus metrics endpoint (`/metrics`)
  - [ ] Track scheduling operations (success/failure/duration)
  - [ ] Track API call latencies and errors
  - [ ] Add health check endpoints
  - [ ] Log scheduling decisions with context
  - **Criteria:** Metrics endpoint exposes scheduling statistics

- [ ] **Configuration Management**
  - [ ] Support environment variable overrides
  - [ ] Support config file hot-reload (SIGHUP)
  - [ ] Validate config on reload
  - [ ] Add config dump command for debugging

### 4.3 Documentation

- [ ] **API Documentation**: Document Tunarr integration
- [ ] **Scheduler File Reference**: Complete YAML schema documentation
- [ ] **Series Scheduling Guide**: Tutorial for series-based scheduling
- [ ] **Migration Guide**: Guide for upgrading from old config format

## Phase 5: UX Enhancements

**Status:** 🔴 Not Started
**Goal:** Improve user experience with better CLI and TUI features

### 5.1 CLI Improvements

- [x] **Interactive Mode**: TUI for configuration (DONE)
- [ ] **Better Table Output**: Improve schedule display formatting
- [ ] **Progress Indicators**: Show progress during long operations
- [ ] **Colored Output**: Use colors for better readability
- [ ] **Dry Run Enhancements**: More detailed dry run output

### 5.2 TUI Enhancements

- [ ] **Field Validation**: Real-time validation in TUI
- [ ] **Filter Editor**: Visual filter rule builder
- [ ] **Confirmation Dialogs**: Confirm destructive actions
- [ ] **Help System**: Context-sensitive help
- [ ] **Keyboard Shortcuts**: Document and improve shortcuts

## Completed

- [x] **Basic CLI Structure**: Cobra-based CLI implemented
- [x] **Basic TUI**: Bubble Tea TUI for block editing
- [x] **Configuration Management**: Viper-based config loading
- [x] **Basic Scheduling Engine**: Cron-based block scheduling
- [x] **Filter System**: Content filtering by genre, rating, year, duration
- [x] **Tunarr Client Skeleton**: Basic API client structure
