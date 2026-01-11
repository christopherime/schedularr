# Project TODOs

## Core Functionality & API

- [ ] **Verify API Endpoints**: Confirm the exact endpoints for `GetPrograms` and `UpdateSchedule` with Tunarr API (v1/v2 differences).
- [ ] **Content Source Integration**:
  - [ ] Implement proper fetching of library content (Plex/Jellyfin/Emby integration via Tunarr or direct).
  - [ ] Support fetching content from Radarr/Sonarr for availability checks.
- [ ] **Enhanced Error Handling**: Improve retry logic for API calls using exponential backoff.

## Scheduling Engine

- [ ] **Duplicate Detection**:
  - [ ] Implement a persistence layer (SQLite/BoltDB) to track history.
  - [ ] Add logic to prevent re-scheduling content aired within `X` days.
- [ ] **Gap Filling**: Add logic to handle "filler" content when a block isn't perfectly filled by selected programs.
- [ ] **Strict Timing**:f Implement strict start time constraints for precise programming slots.
- [ ] **Sequential Playback**: Support sequential episode ordering for TV shows.

## Operational

- [ ] **Dockerization**: Create a `Dockerfile` and `docker-compose.yml` for easy deployment.
- [ ] **Health Checks**: Add a health check command/endpoint.
- [ ] **Metrics**: Expose Prometheus metrics for scheduling success/failure rates.

## UX / CLI

- [x] **Interactive Mode**: Add interactive prompts for configuration setup (TUI implemented).
- [ ] **Table Output**: Improve CLI output formatting for generated schedules.
- [ ] **Validation**: Add command to validate configuration file syntax and logic.
- [ ] **TUI Improvements**:
  - [ ] Add field validation in the editor.
  - [ ] Support editing Filter rules (Genres, Tags, etc.) in TUI.
  - [ ] Add confirmation dialog before quitting without saving.
