# Schedularr Series Scheduling Guide

This guide provides a comprehensive tutorial on how to use Schedularr's series-based scheduling feature, covering configuration, behavior, and best practices.

## 1. Introduction

Schedularr's series-based blocks allow you to automate the scheduling of TV show episodes in sequential order. Unlike filter-based blocks which randomly select content, series blocks intelligently track your progress through a show, ensuring episodes are played in the correct sequence. This is ideal for marathons, daily show schedules, or thematic programming that spans multiple series.

The scheduler maintains the current season and episode for each configured series in a persistent SQLite database, ensuring seamless progression even across application restarts.

## 2. Series Block Configuration

Series blocks are defined within your `scheduler.yaml` file under the `blocks` section. A series block uses `type: "series"` and contains a list of `series` configurations.

Here's an overview of the key fields:

```yaml
blocks:
  - name: "My Series Block"
    cron: "0 20 * * *"       # Schedule block daily at 8 PM
    duration: 120            # 2-hour planned duration
    channel_id: "my-channel"
    priority: 50
    max_duration_overflow_minutes: 15 # Allow programs to extend up to 15 mins over duration

    series:
      - show_title: string      # Required: Title of the TV show (matches Tunarr's show title)
        episodes_per_block: int # Required: Number of episodes to attempt to schedule from this series
        start_season: int       # Optional: Override starting season (default: 1)
        start_episode: int      # Optional: Override starting episode (default: 1)
        on_complete: string     # Optional: Action when series completes ("continue", "restart", "disable", default: "continue")
        skip_episodes: []string # Optional: List of episodes to skip (e.g., "S01E03", "S02E07")
        max_runs: int           # Optional: Max times to run through series before disabling (0 = unlimited)

    fallback:                 # Optional: Strategy if series content doesn't fill the block
      mode: string            # "redistribute" (default) or "filler"
      filler_filter:          # Required if mode: "filler": Filter criteria for fallback content
        # ... standard filter fields like genres, ratings, duration ...

    filler:                   # Optional: General filler for any remaining small gaps
      enabled: bool
      filler_list_id: string
      max_filler_time: int
      min_gap_time: int
```

### Multiple Series per Block

You can configure multiple `series` within a single series block. Schedularr will attempt to schedule episodes from each series sequentially until the block duration (plus `max_duration_overflow_minutes`) is met or all configured `episodes_per_block` from all series are considered. The order in which series are listed in the `scheduler.yaml` determines their priority within the block.

**Example:**

```yaml
    series:
      - show_title: "Series A"
        episodes_per_block: 2
      - show_title: "Series B"
        episodes_per_block: 1
      - show_title: "Series C"
        episodes_per_block: 2
```

In this example, Schedularr will first try to schedule 2 episodes of "Series A", then 1 episode of "Series B", then 2 episodes of "Series C", and so on, cyclically, until the block is filled.

### Initial Progression (`start_season`, `start_episode`)

By default, a new series will start from Season 1, Episode 1. You can override this using `start_season` and `start_episode` fields. This is useful if you've already watched part of a series or want to begin from a specific point.

**Example:**

```yaml
      - show_title: "My Series"
        episodes_per_block: 2
        start_season: 3  # Start from Season 3
        start_episode: 5 # Start from Episode 5 of Season 3
```

### Completion Actions (`on_complete`)

When a series runs out of episodes (i.e., all seasons and episodes have been scheduled), the `on_complete` action determines what Schedularr does next for that series:

- **`continue` (default)**: The series is marked as `completed`, but the series entry remains active in the block. Future scheduling attempts for this series within the block will immediately fall through to the block's `fallback` logic.
- **`restart`**: The series state is reset to `start_season`/`start_episode` (or S01E01 if not specified), and `completed` status is cleared. The `run_count` is incremented. The series becomes active again.
- **`disable`**: The series is marked `disabled`. It will no longer be considered for scheduling within this block. The block will use its `fallback` logic if no other series can fill the time.

### Maximum Runs (`max_runs`)

The `max_runs` field (used with `on_complete: "restart"`) allows you to limit how many times a series can restart from the beginning. Once `run_count` (how many times the series has been fully completed and restarted) meets `max_runs`, the series will automatically be `disabled`. Set to `0` for unlimited restarts.

**Example:**

```yaml
      - show_title: "Seasonal Anime"
        episodes_per_block: 1
        on_complete: "restart"
        max_runs: 1 # Play through once, then restart once, then disable
```

### Skipping Episodes (`skip_episodes`)

You can specify a list of individual episodes to skip using the `skip_episodes` field. This is useful for avoiding filler episodes, problematic content, or episodes you've seen too many times.

**Format:** Episodes should be in the format `"SXXEYY"`, where `XX` is the season number (padded to two digits) and `YY` is the episode number (padded to two digits).

**Example:**

```yaml
      - show_title: "My Show"
        episodes_per_block: 1
        skip_episodes:
          - "S01E03" # Skips Season 1, Episode 3
          - "S02E07" # Skips Season 2, Episode 7
```

### Flexible Duration (`max_duration_overflow_minutes`)

The `max_duration_overflow_minutes` field defined at the block level allows the block's actual scheduled duration to exceed its `duration` by a specified number of minutes. This is crucial for series scheduling, where episode lengths can vary, and cutting an episode short is undesirable.

**Behavior:**

- The scheduler prioritizes fitting entire programs/episodes.
- If adding a program would push the total block duration beyond the `duration` but still within `duration + max_duration_overflow_minutes`, the program is included.
- Once a program causes the block to go into this "overflow" state, no further programs will be added from the series list (though `fallback` and `filler` might still be considered for any remaining time within the `duration` before the overflow item).

**Example:**

```yaml
  - name: "Evening Series"
    duration: 60
    max_duration_overflow_minutes: 10 # Block can run up to 70 minutes
    series:
      - show_title: "Shorts"
        episodes_per_block: 2
```

If two "Shorts" episodes are 35 minutes each, the block would run for 70 minutes, which is within the 10-minute overflow.

## 3. Fallback Strategies

Series blocks utilize a `fallback` strategy when their primary series content cannot fill the entire `duration`. This typically happens when all episodes for a series have been played and its `on_complete` action doesn't restart it, or if there aren't enough available episodes to meet `episodes_per_block`.

The `fallback` configuration is defined within the series block:

```yaml
    fallback:
      mode: string            # "redistribute" (default) or "filler"
      filler_filter:          # Required if mode is "filler": Filter criteria for fallback content
        genres: []string
        # ... other filter fields ...
```

### Redistribute Mode (`mode: "redistribute"`)

This is the default mode. If a series cannot provide enough content, the remaining time in the block is effectively "redistributed" to other active series within the *same block*. If there are no other active series, the block will simply end early. This mode aims to prioritize existing series.

### Filler Mode (`mode: "filler"`)

If `mode` is set to `"filler"`, Schedularr will use the `fallback.filler_filter` to select programs to fill any remaining time in the block after series content has been exhausted. This allows for a specific type of content to be used as a "catch-all" for gaps created by series completion or lack of episodes.

**Example:**

```yaml
    fallback:
      mode: "filler"
      filler_filter:
        genres: ["Documentary"]
        min_duration: 10
        max_duration: 30
```

In this example, if the series content finishes early, the remaining time will be filled with documentaries between 10 and 30 minutes long.

## 4. Filler Content for Gaps

In addition to series `fallback` logic, both filter-based and series-based blocks can have a general `filler` configuration. This is used to fill any *small, residual gaps* that remain after all primary content and series fallback logic has been applied. This is useful for short bumps, commercials, or promos.

See the [Filler Content](#filler-content) section in the main `SPECIFICATIONS.md` for full details. The key difference here is that the `fallback` mechanism is specific to series blocks running out of content, whereas the general `filler` is for any remaining short gaps.

## 5. State Management

Schedularr persists the progress of each series (`current_season`, `current_episode`, `run_count`, `completed`, `disabled` status) in an SQLite database (`schedularr.db`).

- **Atomic Updates**: Series state changes are only committed to the database after a successful update to the Tunarr API. If the API update fails, the state changes are rolled back.
- **Resilience**: This ensures that Schedularr can restart without losing track of series progress.

## 6. Examples

### Basic Series Marathon

Schedule a series to play for 4 hours every Sunday, restarting when complete.

```yaml
blocks:
  - name: "Sunday Sitcom Marathon"
    cron: "0 12 * * 0" # Every Sunday at 12 PM
    duration: 240      # 4-hour block
    channel_id: "comedy-channel"
    priority: 40
    series:
      - show_title: "The Office (US)"
        episodes_per_block: 4
        on_complete: "restart"
        max_runs: 0 # Unlimited restarts
    filler:
      enabled: true
      filler_list_id: "comedy-bumps"
      min_gap_time: 5
```

### Daily Series Continuation with Filler

Schedule 2 episodes of a documentary series each weekday evening, filling any gaps with short nature documentaries.

```yaml
blocks:
  - name: "Daily Docs"
    cron: "0 19 * * 1-5" # Weekdays at 7 PM
    duration: 90        # 90-minute planned block
    channel_id: "documentary-channel"
    priority: 60
    max_duration_overflow_minutes: 10 # Allow up to 10 minutes over
    series:
      - show_title: "Planet Earth"
        episodes_per_block: 2
        on_complete: "disable" # Play once, then stop
    fallback:
      mode: "filler"
      filler_filter:
        genres: ["Nature", "Documentary"]
        max_duration: 20
    filler:
      enabled: true
      filler_list_id: "doc-promos"
      min_gap_time: 2
```

### Thematic Block with Multiple Series

A block featuring episodes from two related fantasy series, with a general sci-fi filler if needed.

```yaml
blocks:
  - name: "Fantasy Adventure"
    cron: "0 21 * * 5" # Every Friday at 9 PM
    duration: 150
    channel_id: "fantasy-channel"
    priority: 70
    max_duration_overflow_minutes: 5 # Minor overflow allowed
    series:
      - show_title: "The Witcher"
        episodes_per_block: 1
        on_complete: "continue" # If finished, move to next series or fallback
      - show_title: "Game of Thrones"
        episodes_per_block: 1
        start_season: 4 # Start from a specific season
        skip_episodes: ["S05E09"] # Skip "that" episode
    fallback:
      mode: "redistribute" # Prioritize other active series first
    filler:
      enabled: true
      filler_list_id: "fantasy-trailers"
      max_filler_time: 15
```

## 7. Best Practices

- **Start Small**: Begin with simple series blocks and gradually add complexity.
- **Monitor Logs**: Pay attention to logs for series progression, completion, and fallback events.
- **Validate Configuration**: Always run `./schedularr validate scheduler.yaml` after making changes to catch errors early.
- **Balance `duration` and `episodes_per_block`**: Ensure your planned block duration is reasonable for the number and typical length of episodes you're trying to schedule.
- **Use `max_duration_overflow_minutes` wisely**: Set a reasonable overflow to avoid excessively long blocks, but enough to prevent cutting media.
- **Define `on_complete`**: Explicitly decide whether a series should `restart`, `disable`, or `continue` (default, leading to fallback) upon completion.
- **Leverage `fallback`**: Use `fallback.mode: "filler"` with a specific `filler_filter` for targeted content when series run out, or let it `redistribute` to other series in the block.
- **Utilize general `filler`**: For small gaps, use the block-level `filler` configuration with short bumps or promos.
- **Review `max_runs`**: Prevent infinite restarts if unintended.
