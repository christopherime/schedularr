# Schedularr Migration Guide

This guide assists users in upgrading their Schedularr configuration files from older formats to the latest version, especially after significant schema changes like the CUE schema integration.

## Table of Contents

- [Schedularr Migration Guide](#schedularr-migration-guide)
  - [Table of Contents](#table-of-contents)
  - [1. Introduction](#1-introduction)
  - [2. Key Configuration Changes](#2-key-configuration-changes)
    - [Application Configuration (`config.yaml`)](#application-configuration-configyaml)
    - [Scheduler Configuration (`scheduler.yaml`)](#scheduler-configuration-scheduleryaml)
  - [3. Migration Steps](#3-migration-steps)
    - [Step 1: Backup Existing Configuration](#step-1-backup-existing-configuration)
    - [Step 2: Generate New Configuration Templates](#step-2-generate-new-configuration-templates)
    - [Step 3: Transfer Settings from Old Config](#step-3-transfer-settings-from-old-config)
    - [Step 4: Validate New Configuration](#step-4-validate-new-configuration)
    - [Step 5: Review and Adjust Scheduler Blocks](#step-5-review-and-adjust-scheduler-blocks)
  - [4. Example Migration](#4-example-migration)
    - [Old `config.yaml`](#old-configyaml)
    - [New `config.yaml`](#new-configyaml)
    - [Old `scheduler.yaml` (or inline config)](#old-scheduleryaml-or-inline-config)
    - [New `scheduler.yaml`](#new-scheduleryaml)
  - [5. Troubleshooting Common Migration Issues](#5-troubleshooting-common-migration-issues)

## 1. Introduction

With the integration of CUE schemas, Schedularr's configuration has become more robust, type-safe, and flexible. This also means some breaking changes or restructuring might be necessary if you are upgrading from an older version. This guide will walk you through the process of migrating your existing configurations to align with the new schema.

The primary change involves:

- **Separation of Concerns**: Application-level settings (like Tunarr connection and logging) are now distinct from scheduler-specific block definitions.
- **Enhanced Block Definitions**: Introduction of Series-based blocks, flexible duration overflow, and more granular control over filler and fallback content.
- **Timezone Support**: Cron expressions are now parsed with explicit timezone awareness.
- **Metrics Exposure**: New configuration for exposing Prometheus metrics.

## 2. Key Configuration Changes

### Application Configuration (`config.yaml`)

**Old Structure (before CUE integration):**
Previously, all settings, including scheduler blocks, might have been in a single `config.yaml` or a less structured format.

**New Structure:**

```yaml
tunarr:
  url: string
  api_key: string

log:
  level: string
  format: string
  timezone: string # New: IANA Time Zone name (e.g., "America/New_York", default: "Local")

metrics_port: int  # New: Port for Prometheus metrics and health check endpoints (default: 9090)

database: string   # New: Path to SQLite database file (e.g., "schedularr.db")

# Scheduler blocks are now typically in a separate file, but can be inlined for compatibility:
# scheduler:
#   blocks:
#     - ... (scheduler block definitions)
scheduler_file: string # New: Path to your separate scheduler.yaml file
```

**Notable Additions/Changes:**

- `log.timezone`: Explicitly set the timezone for cron parsing. Defaults to `Local` (system's local time).
- `metrics_port`: Defines the port where Prometheus metrics (`/metrics`) and health checks (`/healthz`) are exposed.
- `database`: Path to the SQLite database for state persistence (series progression, history).
- `scheduler_file`: Preferred way to specify your scheduling rules, pointing to a separate `scheduler.yaml`. The `scheduler` inline section is still supported for backward compatibility but less recommended.

### Scheduler Configuration (`scheduler.yaml`)

**Old Structure (if separate scheduler files existed):**
Might have contained simpler `blocks` definitions without `type`, `max_duration_overflow_minutes`, detailed `series` config, `fallback`, or `min_gap_time`.

**New Structure:**

```yaml
blocks:
  - name: string
    cron: string
    duration: int
    channel_id: string
    priority: int
    max_duration_overflow_minutes: int # New: Max minutes a block's actual duration can exceed its planned duration

    # Block type (now explicit):
    type: "filter" | "series" # New: Explicitly define block type

    filter:                  # Used if type: "filter"
      # ... standard filter fields ...
      title_pattern: string  # `title` field renamed to `title_pattern` for clarity

    series:                  # Used if type: "series". Now a list of SeriesConfig.
      - show_title: string
        episodes_per_block: int
        start_season: int
        start_episode: int
        on_complete: string  # "continue", "restart", "disable"
        skip_episodes: []string # List of "SXXEYY"
        max_runs: int        # 0 = unlimited

    fallback:                # New: For type: "series" when content runs out
      mode: string           # "redistribute" or "filler"
      filler_filter:         # If mode: "filler", filter for content
        # ... standard filter fields ...

    filler:                  # For any small gaps in both filter and series blocks
      enabled: bool
      filler_list_id: string
      max_filler_time: int
      min_gap_time: int      # New: Minimum gap before adding filler
```

**Notable Additions/Changes:**

- `type`: Blocks now explicitly declare their type (`filter` or `series`).
- `max_duration_overflow_minutes`: Allows for flexible block durations to prioritize content completeness.
- `title_pattern`: The `filter.title` field has been renamed to `filter.title_pattern` for better clarity that it expects a regex pattern.
- `series` block has been greatly expanded to support multiple series configurations within a single block, with fields like `start_season`, `start_episode`, `on_complete`, `skip_episodes`, and `max_runs`.
- `fallback`: A new section for series blocks to define what happens when series content is exhausted.
- `filler.min_gap_time`: Minimum gap duration required before filler content is inserted.

## 3. Migration Steps

Follow these steps to safely migrate your existing Schedularr configuration.

### Step 1: Backup Existing Configuration

Before making any changes, make a copy of your existing `config.yaml` (and any separate scheduler files) to a safe location.

```bash
cp config.yaml config.yaml.bak
cp scheduler.yaml scheduler.yaml.bak # If you have one
```

### Step 2: Generate New Configuration Templates

Use the Schedularr CLI to generate fresh configuration files based on the latest CUE schemas. This will give you valid, up-to-date templates with all default values and new fields.

```bash
./schedularr config generate new-config.yaml
./schedularr scheduler init new-scheduler.yaml
```

### Step 3: Transfer Settings from Old Config

Open your `config.yaml.bak` and `new-config.yaml` side-by-side. Carefully transfer your specific settings (e.g., `tunarr.url`, `tunarr.api_key`, `log.level`, `log.format`) to `new-config.yaml`.
Remember to add the new fields like `log.timezone`, `metrics_port`, and `database` as per your operational environment.

If your old scheduler blocks were inline in `config.yaml`, copy them to `new-scheduler.yaml`.

### Step 4: Validate New Configuration

Once you've transferred your settings, validate the new files using the Schedularr CLI. This will catch any syntax errors or missing required fields.

```bash
./schedularr validate new-config.yaml
./schedularr validate new-scheduler.yaml
```

Address any errors reported by the validator.

### Step 5: Review and Adjust Scheduler Blocks

Open your `scheduler.yaml.bak` (or the scheduler blocks from your old `config.yaml`) and `new-scheduler.yaml` side-by-side.

- For **filter-based blocks**:
  - Ensure `type: "filter"` is explicitly set.
  - If you used `filter.title`, rename it to `filter.title_pattern`.
  - Consider adding `max_duration_overflow_minutes` if you need flexibility in block duration.
  - Review `filler` settings, especially `min_gap_time`.

- For **series-based blocks**:
  - These are significantly changed. You will need to redefine your series blocks using the new `series` list structure.
  - Map your old `show_id` and `episodes_per_block` to the new `series` list.
  - Decide on `start_season`, `start_episode`, `on_complete`, `skip_episodes`, and `max_runs` for each series.
  - Configure `fallback` logic (either `redistribute` or `filler` with a `filler_filter`).
  - Consider `max_duration_overflow_minutes` for these blocks.

## 4. Example Migration

Let's illustrate with an example.

### Old `config.yaml`

```yaml
tunarr:
  url: "http://localhost:8000"
  api_key: "old-api-key"
log:
  level: "debug"
  format: "text"
# Old scheduler blocks might have been inline here, or in a separate file implicitly loaded.
```

### New `config.yaml`

```yaml
tunarr:
  url: "http://localhost:8000"
  api_key: "old-api-key" # Retain your API key

log:
  level: "debug"         # Retain log level
  format: "text"         # Retain log format
  timezone: "America/Los_Angeles" # New: Add your desired timezone

metrics_port: 9090       # New: Default metrics port, adjust if needed
database: "schedularr.db" # New: Default database file

scheduler_file: "scheduler.yaml" # New: Point to your new scheduler file
```

### Old `scheduler.yaml` (or inline config)

Assume a simple structure:

```yaml
blocks:
  - name: "Old Morning Block"
    cron: "0 8 * * *"
    duration: 60
    channel_id: "channel-alpha"
    priority: 10
    filter:
      genres: ["Comedy"]
      max_duration: 30
    filler:
      enabled: true
      filler_list_id: "old-filler-list"
      max_filler_time: 10
```

### New `scheduler.yaml`

```yaml
blocks:
  - name: "Old Morning Block"
    cron: "0 8 * * *"
    duration: 60
    channel_id: "channel-alpha"
    priority: 10
    max_duration_overflow_minutes: 5 # New: Added a small overflow
    type: "filter"                   # New: Explicitly define block type
    filter:
      genres: ["Comedy"]
      max_duration: 30
    filler:
      enabled: true
      filler_list_id: "old-filler-list"
      max_filler_time: 10
      min_gap_time: 2                # New: Added min_gap_time

  # New series block example, reflecting previous inline "future" example
  - name: "Star Trek TNG Evening"
    cron: "0 19 * * 1-5"      # Weekdays at 7 PM
    duration: 120             # 2 hours
    channel_id: "channel-beta"
    priority: 20
    type: "series"            # Explicitly "series" type
    max_duration_overflow_minutes: 15
    series:
      - show_title: "Star Trek: The Next Generation"
        episodes_per_block: 2
        on_complete: "restart"
        max_runs: 0 # Unlimited restarts
    fallback:
      mode: "redistribute" # Default fallback for series
    filler:
      enabled: true
      filler_list_id: "scifi-promos"
      min_gap_time: 3
```

## 5. Troubleshooting Common Migration Issues

- **`Error: blocks[X]: missing required field "type"`**: You need to explicitly set `type: "filter"` or `type: "series"` for each block.
- **`Error: log.timezone: invalid value "EST"`**: Ensure you are using a valid IANA Time Zone name (e.g., "America/New_York", "Europe/London"), not abbreviations.
- **`Error: blocks[X].filter.title: field not found`**: The `filter.title` field was renamed to `filter.title_pattern`. Update your configuration accordingly.
- **`Error: blocks[X].filler.min_gap_time: value ...`**: Ensure `min_gap_time` is a valid integer.
- **`Error: blocks[X].series: expected a list of objects`**: The `series` field now expects a list of `SeriesConfig` objects, not a single `show_id`.
- **Scheduler fails to start with `schedularr.db` error**: Ensure the `database` path in `config.yaml` is accessible and writable.

If you encounter persistent issues, refer to the [SPECIFICATIONS.md](SPECIFICATIONS.md) for the latest schema details or consult the project's issue tracker.
