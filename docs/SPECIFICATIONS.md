# Schedularr Specifications

This document defines the detailed specifications for Schedularr's configuration format, block types, filter criteria, and operational behavior.

## Configuration Format

Schedularr uses two separate configuration files:

### Application Configuration (`config.yaml`)

Defines Tunarr connection, logging settings, and operational parameters.

```yaml
tunarr:
  url: string          # Required: Tunarr API base URL (default: "http://localhost:8000")
  api_key: string      # Optional: API authentication key

log:
  level: string        # Optional: "debug", "info", "warn", "error" (default: "info")
  format: string       # Optional: "json", "text" (default: "text")
  timezone: string     # Optional: IANA Time Zone name (e.g., "America/New_York", default: "Local")
```

Prometheus metrics are exposed at `GET /metrics` on the `schedularr serve`
command's own HTTP listener (`api.listen`, default `:8484`) -- there is no
separate `metrics_port` config key. See [README.md's Serve
section](../README.md#-serve-api-server--cron).

**Example:**

```yaml
tunarr:
  url: "http://localhost:8000"
  api_key: "your-api-key-here"

log:
  level: "info"
  format: "json"
  timezone: "Europe/London"
```

### Scheduler Configuration (`scheduler.yaml`)

Defines scheduling blocks and their rules.

```yaml
blocks:
  - name: string           # Required: Human-readable block identifier
    cron: string           # Required: Cron expression (5-field format)
    duration: int          # Required: Block duration in minutes
    channel_id: string     # Required: Target Tunarr channel ID
    priority: int          # Required: Conflict resolution priority (higher wins)
    max_duration_overflow_minutes: int # Optional: Max minutes a block's actual duration can exceed its planned duration (default: 0)

    # Choose ONE block type:

    # Option 1: Filter-based block
    filter:
      title_pattern: string # Optional: Regex pattern for title matching
      genres: []string      # Optional: List of genres (OR logic)
      ratings: []string     # Optional: List of content ratings
      year_from: int        # Optional: Minimum release year
      year_to: int          # Optional: Maximum release year
      min_duration: int     # Optional: Minimum duration in minutes
      max_duration: int     # Optional: Maximum duration in minutes
      tags: []string        # Optional: Custom tags

    # Option 2: Series-based block
    series:
      - show_title: string      # Required: Title of the TV show
        episodes_per_block: int # Required: Number of episodes to schedule per block occurrence
        start_season: int       # Optional: Starting season (default: 1)
        start_episode: int      # Optional: Starting episode (default: 1)
        on_complete: string     # Optional: Action when series completes ("continue", "restart", "disable", default: "continue")
        skip_episodes: []string # Optional: List of episodes to skip (e.g., "S01E05")
        max_runs: int           # Optional: Max times to run through series (0 = unlimited)

    # Optional: Fallback configuration for Series blocks
    fallback:
      mode: string            # Optional: Fallback mode ("redistribute" or "filler", default: "redistribute")
      filler_filter:          # Required if mode is "filler": Filter for fallback content
        genres: []string
        # ... other filter fields ...

    # Optional: Filler configuration (for both filter and series blocks)
    filler:
      enabled: bool        # Optional: Enable filler content (default: false)
      filler_list_id: string  # Required if enabled: Filler list ID from Tunarr
      max_filler_time: int    # Optional: Maximum filler duration in minutes (0 = unlimited)
      min_gap_time: int       # Optional: Minimum gap (minutes) before adding filler (default: 0)
```

## Block Types

### Filter-Based Block

Schedules content by applying filter criteria to available programs.

**Characteristics:**

- Applies filters in AND logic (all criteria must match)
- Randomizes matching content for variety
- Checks schedule history to prevent recent repeats
- Fills block duration using greedy selection
- Optionally adds filler content for time gaps

**Use Cases:**

- Morning cartoons (genre: Animation, rating: TV-Y)
- Prime time movies (genre: Action, duration: 90-150 min)
- Weekend classics (year: 1960-1990)
- Themed programming (title regex matching)

**Example:**

```yaml
blocks:
  - name: "Morning Cartoons"
    cron: "0 6 * * 6-7"      # Saturdays and Sundays at 6 AM
    duration: 240             # 4 hours
    channel_id: "channel-1"
    priority: 10
    filter:
      genres: ["Animation", "Family"]
      ratings: ["TV-Y", "TV-G"]
      max_duration: 30        # 30-minute episodes max
      year_from: 2000         # Modern content only
    filler:
      enabled: true
      filler_list_id: "bumpers-1"
      max_filler_time: 15     # Max 15 min of filler per block
```

### Series-Based Block

Schedules sequential episodes from one or more TV series with state tracking and flexible fallback options.

**Characteristics:**

- Configures multiple series within a single block.
- Maintains current season/episode position per series in a SQLite store.
- Fetches the next `episodes_per_block` episodes for each configured series.
- Automatically progresses through seasons.
- Provides configurable actions (`on_complete`) when a series finishes all its episodes: `continue`, `restart`, or `disable`.
- Supports `skip_episodes` for specific episode exclusions.
- Allows defining a `max_runs` limit for how many times a series can restart.
- Uses a `fallback` strategy when series content doesn't fill the block:
  - `redistribute`: Prioritizes remaining active series.
  - `filler`: Fills with content matching a specified filter.
- Atomic state updates (commit/rollback) ensure consistency.

**Use Cases:**

- Binge-watching blocks (watch series in order).
- Weekly series continuation.
- Scheduled series marathons.
- Thematic blocks featuring episodes from multiple related series.

**Example:**

```yaml
blocks:
  - name: "Saturday Evening Anime"
    cron: "0 20 * * 6"      # Every Saturday at 8 PM
    duration: 180             # 3-hour block
    channel_id: "anime-channel"
    priority: 10
    max_duration_overflow_minutes: 15 # Allow up to 15 minutes over
    series:
      - show_title: "My Hero Academia"
        episodes_per_block: 2
        start_season: 1
        start_episode: 1
        on_complete: "restart" # Restart from beginning when complete
        max_runs: 3            # Restart up to 3 times
      - show_title: "Attack on Titan"
        episodes_per_block: 1
        on_complete: "disable" # Disable block after completion
        skip_episodes: ["S01E03", "S02E07"] # Skip specific episodes
    fallback:
      mode: "filler" # If series content runs out, use filler
      filler_filter:
        genres: ["Animation", "Documentary"]
        min_duration: 5
        max_duration: 15
    filler: # General filler for any remaining gaps (after series and fallback)
      enabled: true
      filler_list_id: "anime-bumpers"
      min_gap_time: 5 # Only add filler if gap is at least 5 minutes
```

## Filter Criteria

All filter criteria use AND logic - a program must match ALL specified criteria to be included.

### Title Filter

**Type:** String (regex pattern)

**Behavior:** Matches program title using Go regex syntax

**Examples:**

```yaml
# Match titles starting with "Star"
title: "^Star"

# Match titles containing "Trek" or "Wars"
title: "(Trek|Wars)"

# Match titles ending with numbers
title: "\\d+$"
```

### Genre Filter

**Type:** Array of strings

**Behavior:** Matches if program has ANY of the specified genres (OR logic within genres)

**Common Values:**

- Animation, Comedy, Drama, Action, Adventure
- Sci-Fi, Fantasy, Horror, Thriller, Mystery
- Documentary, Reality, News, Sports
- Family, Kids, Romance

**Example:**

```yaml
filter:
  genres: ["Action", "Adventure", "Sci-Fi"]
  # Matches programs with genre = "Action" OR "Adventure" OR "Sci-Fi"
```

### Rating Filter

**Type:** Array of strings

**Behavior:** Matches if program has ANY of the specified ratings

**Common Values:**

- **TV Ratings:** TV-Y, TV-Y7, TV-G, TV-PG, TV-14, TV-MA
- **Movie Ratings:** G, PG, PG-13, R, NC-17
- **Other:** NR (Not Rated), Unrated

**Example:**

```yaml
filter:
  ratings: ["PG", "PG-13", "TV-PG"]
  # Matches family-friendly content
```

### Year Range Filter

**Type:** Integers (year_from, year_to)

**Behavior:** Matches programs released within the specified year range (inclusive)

**Example:**

```yaml
filter:
  year_from: 1980
  year_to: 1999
  # Matches content from the 80s and 90s
```

### Duration Filter

**Type:** Integers in minutes (min_duration, max_duration)

**Behavior:** Matches programs with duration within the specified range

**Notes:**

- Duration stored in Tunarr as milliseconds
- Filter values specified in minutes for user convenience
- Conversion: 1 minute = 60,000 milliseconds

**Example:**

```yaml
filter:
  min_duration: 90
  max_duration: 150
  # Matches movies between 1.5 and 2.5 hours
```

### Tag Filter (Future)

**Type:** Array of strings

**Behavior:** Matches programs with ALL specified tags (AND logic)

**Use Cases:**

- Custom categorization ("holiday", "award-winner")
- User-defined collections
- External metadata integration

**Example:**

```yaml
filter:
  tags: ["christmas", "family-favorite"]
  # Matches programs tagged with both "christmas" AND "family-favorite"
```

## Cron Expression Format

Schedularr uses standard 5-field cron expressions.

### Format

```text
┌─────────── minute (0 - 59)
│ ┌───────── hour (0 - 23)
│ │ ┌─────── day of month (1 - 31)
│ │ │ ┌───── month (1 - 12)
│ │ │ │ ┌─── day of week (0 - 7) (Sunday = 0 or 7)
│ │ │ │ │
* * * * *
```

### Special Characters

- `*` - Any value (wildcard)
- `,` - Value list separator (`1,3,5`)
- `-` - Range of values (`1-5`)
- `/` - Step values (`*/15` = every 15 units)

### Examples

```yaml
# Every day at 6 AM
cron: "0 6 * * *"

# Weekdays at 9 PM
cron: "0 21 * * 1-5"

# Weekends at 8 AM and 2 PM
cron: "0 8,14 * * 6-7"

# Every 2 hours
cron: "0 */2 * * *"

# First day of every month at midnight
cron: "0 0 1 * *"

# Every Monday, Wednesday, Friday at 7:30 PM
cron: "30 19 * * 1,3,5"
```

### Validation

- Cron expressions validated by `github.com/robfig/cron/v3`
- Invalid expressions cause configuration validation errors
- Use `./schedularr validate scheduler.yaml` to check syntax

## Series Progression

Series-based blocks maintain their state in a SQLite database to track episode progression across runs.

### State Schema

```sql
CREATE TABLE series_state (
    show_title TEXT PRIMARY KEY,         -- Unique identifier for the show title
    current_season INTEGER NOT NULL,     -- Current season number
    current_episode INTEGER NOT NULL,    -- Current episode number
    last_aired DATETIME,                 -- Last time an episode from this series was aired
    completed BOOLEAN NOT NULL DEFAULT FALSE, -- True if all episodes have been played
    run_count INTEGER NOT NULL DEFAULT 0,     -- Number of times the series has completed all episodes
    disabled BOOLEAN NOT NULL DEFAULT FALSE   -- True if the series block is disabled due to completion action
);
```

### Progression Logic

1. **Initial State**:
    - If a series is new, it starts at `current_season: 1`, `current_episode: 1`.
    - This can be overridden by `start_season` and `start_episode` in the `SeriesConfig`.
2. **Episode Selection**:
    - For each block occurrence, the scheduler attempts to fetch the next `episodes_per_block` episodes for each configured series.
    - Episodes specified in `skip_episodes` will be skipped over.
3. **State Update**:
    - After successfully scheduling an episode, the `current_episode` counter is incremented.
    - The `last_aired` timestamp is updated.
4. **Season Transition**:
    - When the last episode of a season is reached, the scheduler attempts to find episodes in the next season.
    - If found, `current_season` is incremented, and `current_episode` is reset to 1.
5. **Series Completion (`on_complete`)**:
    - When no more episodes are found for a series (after checking all seasons):
        - The series is marked as `completed`.
        - The `run_count` is incremented.
    - The `on_complete` action determines further behavior:
        - `continue` (default): The series is marked complete, but remains active. Subsequent scheduling attempts will fall through to the block's `fallback` logic.
        - `restart`: The series state is reset to `current_season: 1`, `current_episode: 1`. If `max_runs` is specified and exceeded, the series is `disabled`.
        - `disable`: The series is marked `disabled`, and will no longer be considered for scheduling in its block.

### Fallback Logic for Series Blocks

When a series block cannot fill its `duration` with series content (e.g., a series completes, or there aren't enough episodes to fill the desired `episodes_per_block`), the `fallback` configuration is used:

- `mode: "redistribute"`: (Default) The remaining time is implicitly available for other active series within the same block. If there are no other active series, the block will end earlier.
- `mode: "filler"`: The remaining time will be filled with content matching the `fallback.filler_filter`. This allows for specific types of content to be used as a "catch-all" when series content is exhausted.

### Transaction Handling

- Series state changes are **pending** in memory until the schedule is successfully applied to Tunarr.
- **Commit**: Saves pending state changes to SQLite after a successful Tunarr update.
- **Rollback**: Pending state changes are discarded if the Tunarr update fails (or if the application exits without committing).

### Example State Progression

```text
Block Config (with MaxRuns):
  show_title: "My Show"
  episodes_per_block: 2
  on_complete: "restart"
  max_runs: 2 # Series will restart once, then disable

Initial State:
  "My Show": S01E01, completed=false, run_count=0, disabled=false

Execution 1 (S01E01, S01E02 scheduled):
  "My Show": S01E03, completed=false, run_count=0, disabled=false

... (Series progresses through all seasons) ...

Execution N (Last episode of final season, e.g., S05E10 scheduled):
  "My Show": S05E11 (no more episodes), completed=true, run_count=1, disabled=false

Next Scheduling attempt for "My Show":
  - `on_complete` is "restart", `run_count` (1) < `max_runs` (2).
  - State is reset: "My Show": S01E01, completed=false, run_count=1, disabled=false

... (Series progresses again) ...

Execution M (Last episode of final season, second run):
  "My Show": S05E11 (no more episodes), completed=true, run_count=2, disabled=false

Next Scheduling attempt for "My Show":
  - `on_complete` is "restart", `run_count` (2) >= `max_runs` (2).
  - Series is `disabled`.
  - State becomes: "My Show": S01E01, completed=true, run_count=2, disabled=true

Future Scheduling attempts for "My Show":
  - Series will be skipped due to `disabled=true`.
  - Block will use `fallback` logic.
```

## Filler Content

Filler content fills time gaps when block duration exceeds available content.

### Configuration

```yaml
filler:
  enabled: bool               # Enable/disable filler
  filler_list_id: string      # Tunarr filler list ID
  max_filler_time: int        # Optional: Maximum filler duration in minutes (0 = unlimited)
  min_gap_time: int           # Optional: Minimum gap (minutes) before adding filler (default: 0)
```

### Behavior

1. **Gap Detection**: After scheduling main content, calculate remaining time
2. **Max Filler Check**: If `max_filler_time` set, cap filler duration
3. **Content Fetch**: Retrieve programs from specified filler list
4. **Random Selection**: Shuffle filler content
5. **Greedy Fill**: Add filler programs until gap filled or max reached

### Use Cases

- **Commercials**: Fill gaps with commercial content
- **Bumpers**: Short station identification clips
- **Promos**: Preview upcoming programming
- **Public Service Announcements**: Educational content

### Example

```yaml
blocks:
  - name: "Movie Night"
    cron: "0 20 * * *"
    duration: 180          # 3-hour block
    channel_id: "movies"
    priority: 10
    filter:
      genres: ["Action"]
      min_duration: 90
      max_duration: 120    # 90-120 min movies
    filler:
      enabled: true
      filler_list_id: "commercials-1"
      max_filler_time: 30  # Max 30 min of commercials per block

# Scenario:
#  - Movie duration: 115 minutes
#  - Block duration: 180 minutes
#  - Gap: 65 minutes
#  - Max filler: 30 minutes
#  - Result: 115 min movie + 30 min commercials = 145 min (35 min unfilled)
```

## Priority and Conflict Resolution

When multiple blocks schedule content for overlapping time periods, priority determines which block wins.

### Priority Rules

1. **Higher Priority Wins**: Block with higher priority value takes precedence
2. **Discard Lower Priority**: Conflicting lower-priority blocks are discarded entirely
3. **Logging**: All conflicts logged with winner/loser details

### Conflict Detection

Two blocks conflict if their time ranges overlap:

```text
Block A: [10:00 - 12:00], Priority: 10
Block B: [11:00 - 13:00], Priority: 5

Overlap: [11:00 - 12:00]
Winner: Block A (higher priority)
Result: Block A scheduled, Block B discarded
```

### Best Practices

**Priority Ranges:**

- **1-10**: Low priority (filler content, background programming)
- **11-50**: Normal priority (regular programming)
- **51-100**: High priority (special events, live content)

**Example:**

```yaml
blocks:
  # Regular programming
  - name: "Morning News"
    cron: "0 7 * * 1-5"
    priority: 20

  # Special event (overrides regular programming)
  - name: "Holiday Special"
    cron: "0 7 25 12 *"  # Dec 25 at 7 AM
    priority: 80

  # Low priority filler
  - name: "Generic Content"
    cron: "0 * * * *"     # Every hour
    priority: 5
```

## Schedule History

Schedule history prevents content repetition by tracking recently scheduled programs. It is both an in-memory dedup check during a single generate/apply cycle and a persisted `schedule_history` SQLite table (queryable via `GET /history?days=N`, see the API docs) that survives restarts.

### Configuration

- **Window**: `maintenance.history_retention` (default `"168h"`, i.e. 7 days) -- see `config.yaml`'s `maintenance` section
- **Storage**: Both an in-memory map, scoped to the current `scheduler.Engine` instance (cleared on restart), and the persisted `schedule_history` table (survives restarts, pruned to the retention window on every apply)
- **Key Format**: `channel_id:program_id` (in-memory); `(program_id, channel_id, scheduled_at)` rows in the `schedule_history` table

### Behavior

1. **Before Scheduling**: Check if program scheduled recently (in-memory check, then a `schedule_history` lookup via `store.WasRecentlyScheduled`)
2. **Exclusion**: Remove recently played programs from candidates
3. **Recording**: After scheduling, record program + timestamp (in-memory and, on `Engine.Commit()`, persisted to `schedule_history`)
4. **Cleanup**: `Engine.Commit()` deletes `schedule_history` rows older than `maintenance.history_retention` on every successful apply

### Example

```text
Current Time: 2026-01-12 10:00
History Window: 7 days (maintenance.history_retention: "168h")

History Entries:
  channel-1:prog-123 → 2026-01-10 14:00 (keep, within window)
  channel-1:prog-456 → 2026-01-04 08:00 (remove, outside window)
  channel-2:prog-789 → 2026-01-11 20:00 (keep, within window)

Scheduling Block for channel-1:
  Available: [prog-111, prog-123, prog-456, prog-789]
  Filter: Remove prog-123 (recently played on channel-1)
  Candidates: [prog-111, prog-456, prog-789]
```

### Limitations

- **Per-Channel**: Programs tracked separately per channel
- **Configurable Window**: Set `maintenance.history_retention` to widen or narrow the window. `GET /history?days=N` (`api/openapi.yaml`, `1..90`) can only ever return data as far back as `history_retention` allows -- e.g. `?days=90` needs `history_retention` set to at least `2160h` to actually have 90 days of persisted rows to return; the default `168h` limits queries to the last 7 days regardless of what `days` the caller requests. See [README.md's History Endpoint section](../README.md#history-endpoint).

## Validation

All configurations validated using CUE schemas before loading.

### Validation Commands

```bash
# Validate application config
./schedularr validate config.yaml

# Validate scheduler config
./schedularr validate scheduler.yaml

# Validate with custom schema (advanced)
cue vet config.yaml cmd/schema/config.cue
```

### Common Validation Errors

**Invalid Cron Expression:**

```text
Error: blocks[0].cron: invalid cron expression "60 25 * * *"
  minute must be 0-59
  hour must be 0-23
```

**Missing Required Field:**

```text
Error: blocks[0]: missing required field "channel_id"
```

**Invalid Priority:**

```text
Error: blocks[0].priority: value -5 not in range 1-100
```

**Invalid Duration Range:**

```text
Error: blocks[0].filter.max_duration: value 30 less than min_duration 90
```

## Examples

### Complete Configuration Example

**config.yaml:**

```yaml
tunarr:
  url: "http://localhost:8000"
  api_key: "secret-api-key"

log:
  level: "info"
  format: "json"
  timezone: "America/New_York" # New: Example timezone
```

**scheduler.yaml:**

```yaml
blocks:
  # Morning cartoons on weekends
  - name: "Saturday Morning Cartoons"
    cron: "0 7 * * 6"
    duration: 180
    channel_id: "kids-channel"
    priority: 30
    max_duration_overflow_minutes: 10 # New: Allow up to 10 minutes over
    filter:
      genres: ["Animation", "Family"]
      ratings: ["TV-Y", "TV-G"]
      max_duration: 30
      year_from: 2000
    filler:
      enabled: true
      filler_list_id: "bumpers-kids"
      max_filler_time: 10
      min_gap_time: 5 # New: Min gap for filler

  # Prime time action movies (filter-based)
  - name: "Action Movie Night"
    cron: "0 20 * * *"
    duration: 150
    channel_id: "movie-channel"
    priority: 40
    filter:
      genres: ["Action", "Adventure"]
      ratings: ["PG-13", "R"]
      min_duration: 90
      max_duration: 140
      year_from: 1990
    filler:
      enabled: true
      filler_list_id: "trailers"
      max_filler_time: 20
      min_gap_time: 3

  # Series: My Hero Academia marathon
  - name: "My Hero Academia Marathon"
    cron: "0 14 * * 0" # Every Sunday at 2 PM
    duration: 180
    channel_id: "anime-channel"
    priority: 50
    max_duration_overflow_minutes: 15 # Allow up to 15 minutes over
    series:
      - show_title: "My Hero Academia"
        episodes_per_block: 3
        start_season: 1
        start_episode: 1
        on_complete: "restart"
        max_runs: 2 # Restart series once
        skip_episodes: ["S01E01"] # Skip the very first episode
    fallback:
      mode: "filler"
      filler_filter:
        genres: ["Animation"]
        min_duration: 1
        max_duration: 10
    filler:
      enabled: true
      filler_list_id: "anime-commercials"
      min_gap_time: 2

  # Classic sitcom reruns
  - name: "Classic Comedy Hour"
    cron: "0 18 * * 1-5"
    duration: 60
    channel_id: "comedy-channel"
    priority: 25
    filter:
      genres: ["Comedy"]
      ratings: ["TV-PG", "TV-14"]
      max_duration: 30
      year_from: 1970
      year_to: 2000

  # Late night variety
  - name: "Late Night Mix"
    cron: "0 23 * * *"
    duration: 120
    channel_id: "variety-channel"
    priority: 15
    filter:
      genres: ["Comedy", "Talk Show", "Variety"]
      ratings: ["TV-14", "TV-MA"]
```

---

*For architectural details, see [ARCHITECTURE.md](ARCHITECTURE.md)*

*For CLI usage, see [CLI_REFERENCE.md](CLI_REFERENCE.md)*
