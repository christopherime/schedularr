# Project TODOs

## Phase 1: Foundation & API Verification (PRIORITY)

### 1.1 Tunarr API Research & Verification

- [ ] **Research Tunarr API Documentation**: Study official Tunarr API docs to understand actual endpoints
- [ ] **Verify Channel Endpoints**: Test and confirm `/api/channels` endpoint structure
- [ ] **Verify Content Endpoints**: Identify correct endpoint for fetching library content (programs/media)
  - [ ] Test `/api/programs` endpoint
  - [ ] Test `/api/filler-lists` endpoint (if exists)
  - [ ] Test channel-specific content endpoints
- [ ] **Verify Schedule Update Endpoints**: Confirm how to update channel programming
  - [ ] Research schedule/programming API structure
  - [ ] Understand payload format for schedule updates
- [ ] **API Authentication**: Verify API key usage and authentication headers
- [ ] **Create API Integration Tests**: Write tests against real/mock Tunarr instance

### 1.2 Enhanced Error Handling & Resilience

- [ ] **Implement Retry Logic**: Add exponential backoff for API calls
- [ ] **Better Error Messages**: Improve error context and user-facing messages
- [ ] **API Response Validation**: Validate API responses match expected schema

## Phase 2: Scheduler File Architecture (NEW PRIORITY)

### 2.1 Configuration Separation

- [ ] **Separate Config Concerns**: Split configuration into app config and scheduler config
  - [ ] Keep `config.yaml` for: Tunarr connection, logging, app settings
  - [ ] Create `scheduler.yaml` structure for: scheduling blocks and rules
- [ ] **Update Config Package**: Modify config loading to support separate scheduler files
  - [ ] Add `SchedulerFile` field to main config
  - [ ] Create scheduler config loader
  - [ ] Support multiple scheduler file formats (YAML/JSON)
- [ ] **CLI Parameter Support**: Add `--scheduler <file>` flag to commands
  - [ ] Update `generate` command to accept scheduler file parameter
  - [ ] Update `validate` command to validate scheduler files
  - [ ] Add default scheduler file path resolution

### 2.2 Scheduler File Management Commands

- [ ] **Create `scheduler init` Command**: Generate boilerplate scheduler files
  - [ ] Create template with example blocks
  - [ ] Support different templates (basic, advanced, series-based)
  - [ ] Interactive prompts for initial setup
- [ ] **Create `scheduler validate` Command**: Validate scheduler file syntax
  - [ ] YAML/JSON syntax validation
  - [ ] Cron expression validation
  - [ ] Filter criteria validation
  - [ ] Channel ID validation against Tunarr
- [ ] **Create `scheduler list` Command**: List all configured blocks
  - [ ] Table output with block details
  - [ ] Filter by channel, priority, or status

### 2.3 Series-Based Scheduling (CORE FEATURE)

#### 2.3.1 Data Models

- [ ] **Define Series Block Type**: Create new block type for series scheduling
  - [ ] Add `SeriesBlock` struct with series list
  - [ ] Add `SeriesConfig` with show title, episodes per block, season/episode tracking
  - [ ] Support mixing series and filter-based blocks
- [ ] **Episode Tracking Schema**: Design state tracking structure
  - [ ] Track current season/episode per series
  - [ ] Track completion status
  - [ ] Track last aired date/time

#### 2.3.2 Persistence Layer

- [ ] **Implement SQLite State Store**: Create database for episode tracking
  - [ ] Design schema for series state table
  - [ ] Design schema for scheduling history
  - [ ] Create migration system
  - [ ] Add database initialization
- [ ] **State Management Functions**: CRUD operations for series state
  - [ ] Get current episode for series
  - [ ] Update episode progress
  - [ ] Mark series as complete
  - [ ] Reset series progress
- [ ] **State Backup/Export**: Allow exporting and importing state
  - [ ] Export to JSON
  - [ ] Import from JSON
  - [ ] Backup before major operations

#### 2.3.3 Series Scheduling Logic

- [ ] **Sequential Episode Selection**: Implement episode progression
  - [ ] Fetch next N episodes for each series
  - [ ] Handle season boundaries
  - [ ] Handle series completion
- [ ] **Smart Gap Filling**: Implement fallback logic
  - [ ] When series completes, redistribute time to remaining series
  - [ ] Fill remaining time with incomplete series
  - [ ] Support filler content as last resort
- [ ] **Completion Handling**: Graceful series completion
  - [ ] Log INFO when series completes all episodes
  - [ ] Mark series as complete in state
  - [ ] Option to auto-disable completed blocks
  - [ ] Option to restart series from beginning

#### 2.3.4 Example Scheduler File Format

- [ ] **Design YAML Schema**: Create comprehensive scheduler file format

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

### 3.1 Content Fetching Improvements

- [ ] **Content Source Integration**: Better content fetching
  - [ ] Implement proper library content fetching from Tunarr
  - [ ] Support Plex/Jellyfin/Emby integration via Tunarr
  - [ ] Cache content metadata locally
- [ ] **Series Episode Fetching**: Fetch episodes for specific series
  - [ ] Query by show title
  - [ ] Filter by season/episode range
  - [ ] Handle missing episodes gracefully

### 3.2 Duplicate Detection & History

- [ ] **Scheduling History**: Track what has been scheduled
  - [ ] Record scheduled content with timestamps
  - [ ] Prevent re-scheduling within X days (configurable)
  - [ ] History cleanup/archival
- [ ] **Smart Rotation**: Avoid repetition
  - [ ] Track last aired date per program
  - [ ] Prefer least recently aired content
  - [ ] Configurable rotation windows

### 3.3 Advanced Scheduling Features

- [ ] **Gap Filling Logic**: Handle partial block fills
  - [ ] Define filler content pools
  - [ ] Smart filler selection based on remaining time
  - [ ] Bumpers/commercials support
- [ ] **Strict Timing Mode**: Precise start time constraints
  - [ ] Ensure blocks start exactly at cron time
  - [ ] Handle time zone considerations
  - [ ] Daylight saving time handling
- [ ] **Priority-Based Conflict Resolution**: Handle overlapping blocks
  - [ ] Implement priority comparison
  - [ ] Merge or override based on priority
  - [ ] Warn about conflicts

## Phase 4: Operational Excellence

### 4.1 Testing & Quality

- [ ] **Unit Test Coverage**: Achieve >80% coverage
  - [ ] Test all scheduler logic
  - [ ] Test series progression
  - [ ] Test state management
  - [ ] Test API client
- [ ] **Integration Tests**: Test against real Tunarr instance
- [ ] **Linting Compliance**: Fix all linter issues
  - [ ] Run `golangci-lint run` and fix issues
  - [ ] Run `gosec ./...` and fix security issues
  - [ ] Run `govulncheck ./...` and address vulnerabilities

### 4.2 Deployment & Operations

- [ ] **Dockerization**: Container support
  - [ ] Create `Dockerfile`
  - [ ] Create `docker-compose.yml`
  - [ ] Multi-stage build for smaller images
  - [ ] Health check endpoint
- [ ] **Observability**: Monitoring and metrics
  - [ ] Add structured logging (slog)
  - [ ] Prometheus metrics for scheduling operations
  - [ ] Health check command/endpoint
  - [ ] Scheduling success/failure tracking

### 4.3 Documentation

- [ ] **API Documentation**: Document Tunarr integration
- [ ] **Scheduler File Reference**: Complete YAML schema documentation
- [ ] **Series Scheduling Guide**: Tutorial for series-based scheduling
- [ ] **Migration Guide**: Guide for upgrading from old config format

## Phase 5: UX Enhancements

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
