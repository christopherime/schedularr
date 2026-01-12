# Schedularr CLI Reference

## Overview

Schedularr provides a comprehensive command-line interface for managing TV channel scheduling with Tunarr. All commands support the `--config` flag to specify a custom configuration file.

## Global Flags

- `--config <file>` - Specify config file (default: `$HOME/.schedularr.yaml`)
- `--help`, `-h` - Show help for any command

## Commands

### Configuration Management

#### `config generate [filename]`

Generate an application configuration file from the CUE schema with default values.

**Usage:**
```bash
# Generate default config.yaml
schedularr config generate

# Generate with custom filename
schedularr config generate my-config.yaml

# Generate JSON format
schedularr config generate config.json
```

**Output:**
- Creates a YAML or JSON file (auto-detected from extension)
- Includes all configuration options with defaults from CUE schema
- File includes: Tunarr connection settings, logging configuration

---

### Scheduler Configuration

#### `scheduler init [filename]`

Generate a scheduler configuration file from the CUE schema with example blocks.

**Usage:**
```bash
# Generate default scheduler.yaml
schedularr scheduler init

# Generate with custom filename
schedularr scheduler init my-schedule.yaml

# Generate JSON format
schedularr scheduler init schedule.json
```

**Output:**
- Creates a YAML or JSON file with example scheduling blocks
- Includes default settings for rotation, filler, and gap management
- Example block includes filter-based scheduling with common genres

#### `scheduler validate [filename]`

Validate a scheduler configuration file against the CUE schema.

**Usage:**
```bash
# Validate specific file
schedularr scheduler validate my-schedule.yaml

# Auto-detect scheduler.yaml in current or home directory
schedularr scheduler validate
```

**Exit Codes:**
- `0` - Validation passed
- `1` - Validation failed (shows detailed errors)

#### `scheduler list [filename]`

Display all configured scheduling blocks in a table format.

**Usage:**
```bash
schedularr scheduler list
schedularr scheduler list my-schedule.yaml
```

**Output:**
- Table showing: Name, Cron, Duration, Channel, Priority, Filters
- Summary of total blocks configured

---

### Validation

#### `validate <file>`

Validate any configuration file (app config or scheduler config) against CUE schemas.

**Usage:**
```bash
# Validate application config
schedularr validate config.yaml

# Validate scheduler config
schedularr validate scheduler.yaml

# Validate with full path
schedularr validate ~/.schedularr.yaml
```

**Features:**
- Auto-detects file type based on filename
- Provides detailed validation errors from CUE engine
- Shows field paths and constraint violations

**Exit Codes:**
- `0` - Validation passed
- `1` - Validation failed

---

### Schedule Generation

#### `generate`

Generate TV channel schedules based on scheduler configuration.

**Usage:**
```bash
# Generate schedule (dry-run)
schedularr generate --scheduler my-schedule.yaml

# Generate and apply to Tunarr
schedularr generate --scheduler my-schedule.yaml --apply

# Generate for specific time range
schedularr generate --scheduler my-schedule.yaml --from "2026-01-15" --to "2026-01-22"
```

**Flags:**
- `--scheduler <file>` - Path to scheduler configuration file
- `--apply` - Apply generated schedule to Tunarr (default: dry-run)
- `--from <date>` - Start date for schedule generation
- `--to <date>` - End date for schedule generation

---

### Daemon Mode

#### `run`

Start the scheduling daemon to automatically generate and apply schedules.

**Usage:**
```bash
# Start daemon (runs continuously)
schedularr run --scheduler my-schedule.yaml

# Run once and exit
schedularr run --scheduler my-schedule.yaml --once
```

**Flags:**
- `--scheduler <file>` - Path to scheduler configuration file
- `--once` - Run once and exit (default: continuous)
- `--daemon` - Run in background (default behavior)

**Features:**
- Graceful shutdown on SIGTERM/SIGINT
- Automatic schedule generation based on cron expressions
- Continuous monitoring and updates

---

### Interactive TUI

#### `tui`

Launch the interactive terminal user interface for managing schedules.

**Usage:**
```bash
schedularr tui
```

**Features:**
- Visual block editor
- Real-time schedule preview
- Interactive configuration management

---

### Channel Management

#### `channels`

List all available Tunarr channels.

**Usage:**
```bash
schedularr channels
```

**Output:**
- Table of channels with ID, name, and number
- Used to identify channel IDs for scheduler configuration

---

## Examples

### Quick Start Workflow

```bash
# 1. Generate application config
schedularr config generate

# 2. Edit config with your Tunarr URL
vim config.yaml

# 3. Generate scheduler config
schedularr scheduler init

# 4. Edit scheduler blocks
vim scheduler.yaml

# 5. Validate both configs
schedularr validate config.yaml
schedularr validate scheduler.yaml

# 6. Test schedule generation (dry-run)
schedularr generate --scheduler scheduler.yaml

# 7. Apply schedule to Tunarr
schedularr generate --scheduler scheduler.yaml --apply

# 8. Start daemon for continuous scheduling
schedularr run --scheduler scheduler.yaml
```

### Validation Workflow

```bash
# Validate all configs before deployment
schedularr validate ~/.schedularr.yaml
schedularr validate scheduler.yaml

# Check scheduler syntax
schedularr scheduler validate scheduler.yaml

# List configured blocks
schedularr scheduler list scheduler.yaml
```

---

## Configuration Files

### Application Config (`config.yaml`)

Generated with `schedularr config generate`:

```yaml
tunarr:
  url: http://localhost:8000
  api_key: ""  # Optional
log:
  level: info
  format: text
```

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

settings:
  rotation_window_days: 7
  min_gap_minutes: 5
  max_filler_minutes: 30
```

---

## Exit Codes

- `0` - Success
- `1` - Error (validation failure, runtime error, etc.)

---

## Environment Variables

- `SCHEDULARR_CONFIG` - Override default config file location
- Standard Viper environment variable support (prefix: `SCHEDULARR_`)

---

## See Also

- [Architecture Documentation](ARCHITECTURE.md)
- [Tunarr API Research](TUNARR_API_RESEARCH.md)
- [Project TODO](../TODO.md)

