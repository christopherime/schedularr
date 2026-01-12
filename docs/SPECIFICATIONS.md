# Schedularr Specifications

This document defines the detailed specifications for Schedularr's configuration format, block types, filter criteria, and operational behavior.

## Table of Contents

1. [Configuration Format](#configuration-format)
2. [Block Types](#block-types)
3. [Filter Criteria](#filter-criteria)
4. [Cron Expression Format](#cron-expression-format)
5. [Series Progression](#series-progression)
6. [Filler Content](#filler-content)
7. [Priority and Conflict Resolution](#priority-and-conflict-resolution)
8. [Schedule History](#schedule-history)

## Configuration Format

Schedularr uses two separate configuration files:

### Application Configuration (`config.yaml`)

Defines Tunarr connection and logging settings.

```yaml
tunarr:
  url: string          # Required: Tunarr API base URL
  api_key: string      # Optional: API authentication key

log:
  level: string        # Optional: "debug", "info", "warn", "error" (default: "info")
  format: string       # Optional: "json", "text" (default: "text")
```

**Example:**

```yaml
tunarr:
  url: "http://localhost:8000"
  api_key: "your-api-key-here"

log:
  level: "info"
  format: "json"
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

    # Choose ONE block type:

    # Option 1: Filter-based block
    filter:
      title: string        # Optional: Regex pattern for title matching
      genres: []string     # Optional: List of genres (OR logic)
      ratings: []string    # Optional: List of content ratings
      year_from: int       # Optional: Minimum release year
      year_to: int         # Optional: Maximum release year
      min_duration: int    # Optional: Minimum duration in minutes
      max_duration: int    # Optional: Maximum duration in minutes
      tags: []string       # Optional: Custom tags

    # Option 2: Series-based block (future)
    series:
      show_id: string      # Required: Tunarr show ID
      episodes_per_block: int  # Required: Number of episodes per occurrence
      restart_on_completion: bool  # Optional: Restart from S01E01 when complete

    # Optional: Filler configuration
    filler:
      enabled: bool        # Optional: Enable filler content (default: false)
      filler_list_id: string  # Required if enabled: Filler list ID from Tunarr
      max_filler_time: int    # Optional: Maximum filler duration in minutes
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

### Series-Based Block (Future Implementation)

Schedules sequential episodes from a TV series with state tracking.

**Characteristics:**

- Maintains current season/episode position in SQLite
- Fetches next N episodes from Tunarr
- Automatically progresses through seasons
- Optionally restarts series on completion
- Atomic state updates (commit/rollback)

**Use Cases:**

- Binge-watching blocks (watch series in order)
- Weekly series continuation
- Scheduled series marathons

**Example:**

```yaml
blocks:
  - name: "Star Trek TNG Marathon"
    cron: "0 19 * * 1-5"      # Weekdays at 7 PM
    duration: 120             # 2 hours (2-3 episodes)
    channel_id: "channel-2"
    priority: 20
    series:
      show_id: "show-123"
      episodes_per_block: 2
      restart_on_completion: true
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

Series-based blocks maintain state in SQLite to track episode progression.

### State Schema

```sql
CREATE TABLE series_state (
    block_id TEXT PRIMARY KEY,           -- Unique block identifier
    show_id TEXT NOT NULL,               -- Tunarr show ID
    season INTEGER NOT NULL,             -- Current season number
    episode INTEGER NOT NULL,            -- Current episode number
    last_updated INTEGER NOT NULL        -- Unix timestamp
);
```

### Progression Logic

1. **Initial State**: When block first runs, starts at S01E01
2. **Episode Fetch**: Retrieves next `episodes_per_block` episodes from Tunarr
3. **State Update**: Increments episode counter by number scheduled
4. **Season Transition**: When season ends, increments season and resets episode to 1
5. **Series Completion**: When all episodes exhausted:
   - If `restart_on_completion: true` → Reset to S01E01
   - If `restart_on_completion: false` → Use fallback content or skip block

### Transaction Handling

- State changes are **pending** until schedule successfully applied
- **Commit**: Saves state to SQLite after successful Tunarr update
- **Rollback**: Discards state changes if Tunarr update fails

### Example State Progression

```text
Block Config:
  show_id: "show-123"
  episodes_per_block: 2

Execution 1:
  Current State: S01E01
  Scheduled: S01E01, S01E02
  New State: S01E03

Execution 2:
  Current State: S01E03
  Scheduled: S01E03, S01E04
  New State: S01E05

...

Execution N (last episode of season):
  Current State: S01E23
  Scheduled: S01E23, S01E24
  New State: S02E01  (season transition)

Execution M (series complete):
  Current State: S05E24 (final episode)
  Scheduled: S05E24
  New State: S01E01 (if restart_on_completion: true)
```

## Filler Content

Filler content fills time gaps when block duration exceeds available content.

### Configuration

```yaml
filler:
  enabled: bool               # Enable/disable filler
  filler_list_id: string      # Tunarr filler list ID
  max_filler_time: int        # Optional: Maximum filler duration in minutes
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

Schedule history prevents content repetition by tracking recently scheduled programs.

### Configuration

- **Window**: 7 days (168 hours) by default
- **Storage**: In-memory map (cleared on restart)
- **Key Format**: `channel_id:program_id`

### Behavior

1. **Before Scheduling**: Check if program scheduled recently
2. **Exclusion**: Remove recently played programs from candidates
3. **Recording**: After scheduling, record program + timestamp
4. **Cleanup**: Automatically remove entries older than window

### Example

```text
Current Time: 2026-01-12 10:00
History Window: 7 days

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
- **In-Memory**: History lost on application restart
- **Fixed Window**: 7-day window not currently configurable
- **Future Enhancement**: Configurable window duration

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
    filter:
      genres: ["Animation", "Family"]
      ratings: ["TV-Y", "TV-G"]
      max_duration: 30
      year_from: 2000
    filler:
      enabled: true
      filler_list_id: "bumpers-kids"
      max_filler_time: 10

  # Prime time action movies
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
