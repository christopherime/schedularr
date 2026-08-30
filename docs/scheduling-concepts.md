# Scheduling Concepts

A **block** is a scheduling rule: a cron trigger, a duration, a target Tunarr channel, a priority, and either a content **filter** or a list of **series** to progress through. Blocks live in the SQLite store, not in a file — `scheduler.yaml` is a first-run *import* format only (see [Getting Started](getting-started.md)); manage blocks going forward through the `/api/v1/blocks` HTTP API or the [Web UI](web-ui-guide.md).

## Block structure

```yaml
blocks:
  - name: string # Required: human-readable block identifier
    type: string # "filter" or "series" -- required in scheduler.yaml (see note below)
    cron: string # Required: 5-field cron expression
    duration: int # Required: block duration in minutes
    channel_id: string # Required: target Tunarr channel ID
    priority: int # Required: conflict-resolution priority (higher wins)
    max_duration_overflow_minutes: int # Optional: max minutes actual duration can exceed planned duration (default 0)

    filter: { ... } # Choose one: filter-based block (see below)
    series: [ ... ] # Choose one: series-based block (see below)

    fallback: { ... } # Optional, series blocks only: strategy when series content doesn't fill the block
    filler: { ... } # Optional, either type: fills small residual gaps
```

Each block's `type` field (`filter` or `series`) has a CUE schema default, but the `scheduler.yaml` import path decodes each block into a Go struct before CUE-validating it, which turns an omitted `type` into an explicit empty string rather than an absent field — CUE only applies a default to a genuinely absent field, so an empty string fails the `"filter" | "series"` check. Always set `type` explicitly in `scheduler.yaml`. `POST /api/v1/blocks`'s JSON body doesn't have this problem; `type` there can be omitted.

## Filter-based blocks

Applies filter criteria to available programs, in AND logic across criteria (a program must match every specified criterion). Matching content is randomized, checked against schedule history to avoid recent repeats, and greedily selected to fill the block's duration.

```yaml
filter:
  title_pattern: string # Regex pattern for title matching (Go regex syntax)
  genres: []string # OR logic within the list -- matches ANY of these genres
  ratings: []string # OR logic within the list
  year_from: int # Minimum release year
  year_to: int # Maximum release year
  min_duration: int # Minimum duration, minutes
  max_duration: int # Maximum duration, minutes
  tags: []string # Custom tags, AND logic (matches ALL specified tags)
```

| Field | Example | Notes |
| --- | --- | --- |
| `title_pattern` | `"^Star"`, `"(Trek\|Wars)"`, `"\\d+$"` | Go regex |
| `genres` | `["Action", "Adventure", "Sci-Fi"]` | Matches any listed genre |
| `ratings` | `["PG", "PG-13", "TV-PG"]` | TV: TV-Y…TV-MA; movie: G…NC-17; or NR/Unrated |
| `year_from` / `year_to` | `1980` / `1999` | Inclusive range |
| `min_duration` / `max_duration` | `90` / `150` | Minutes; stored in Tunarr as milliseconds internally |
| `tags` | `["christmas", "family-favorite"]` | All listed tags must match |

**Example — Saturday night sci-fi marathon:**

```yaml
blocks:
  - name: "Saturday Night Sci-Fi"
    type: filter
    cron: "0 20 * * 6" # Saturdays at 8 PM
    duration: 360 # 6 hours
    channel_id: "channel-1"
    filter:
      genres: ["Science Fiction"]
      min_duration: 90
      year_from: 1980
```

## Series-based blocks

Schedules sequential episodes from one or more shows with state tracking and flexible fallback. Unlike filter blocks, which pick content at random, series blocks track exactly where each show is and play episodes in order — ideal for marathons, daily continuations, or thematic runs across multiple related shows.

```yaml
series:
  - show_title: string # Required: must match Tunarr's show title
    episodes_per_block: int # Required: episodes to attempt per block occurrence
    start_season: int # Optional, default 1
    start_episode: int # Optional, default 1
    on_complete: string # Optional: "continue" (default), "restart", or "disable"
    skip_episodes: []string # Optional: e.g. ["S01E03", "S02E07"]
    max_runs: int # Optional: cap on restarts before auto-disable (0 = unlimited)
```

### Multiple series per block

List several `series` entries in one block; Schedularr schedules episodes from each in listed order, cyclically, until the block's duration (plus `max_duration_overflow_minutes`) is met:

```yaml
series:
  - show_title: "Series A"
    episodes_per_block: 2
  - show_title: "Series B"
    episodes_per_block: 1
  - show_title: "Series C"
    episodes_per_block: 2
```

### Starting position

A new series starts at S01E01 by default. Override with `start_season`/`start_episode` to begin partway through — useful when you've already watched part of a show.

### Completion actions (`on_complete`)

When a series runs out of episodes (every season and episode scheduled):

- **`continue`** (default) — marked `completed`, stays active in the block; future attempts for this series fall straight through to `fallback`.
- **`restart`** — state resets to `start_season`/`start_episode` (or S01E01), `completed` clears, `run_count` increments, the series becomes active again.
- **`disable`** — marked `disabled`, no longer considered for scheduling in this block; the block falls through to `fallback` if no other series can fill the time.

`max_runs` (used with `on_complete: "restart"`) caps how many times a series restarts. Once `run_count` reaches `max_runs`, the series is disabled automatically. `0` means unlimited restarts.

### Skipping episodes

`skip_episodes` takes a list in `SxxEyy` form (season and episode padded to two digits), e.g. `["S01E03", "S02E07"]` — useful for filler episodes, problematic content, or anything watched too many times already.

### Flexible duration

`max_duration_overflow_minutes` (block-level) lets actual scheduled duration exceed `duration` by a set amount, since episode lengths vary and cutting one short is undesirable. The scheduler prioritizes fitting whole episodes: if adding one would push the block past `duration` but still within `duration + max_duration_overflow_minutes`, it's included; once that overflow happens, no further series programs are added (fallback/filler may still be considered for time remaining within `duration` before the overflow item).

### Fallback strategies

Used when series content can't fill the block's duration — a series completes without restarting, or there aren't enough episodes to meet `episodes_per_block`:

```yaml
fallback:
  mode: string # "redistribute" (default) or "filler"
  filler_filter: { ... } # Required if mode is "filler" -- same fields as a filter block's `filter`
```

- **`redistribute`** (default) — remaining time goes implicitly to other active series in the same block; with no other active series, the block ends early.
- **`filler`** — remaining time fills with content matching `fallback.filler_filter`, a targeted catch-all for gaps from series completion or exhaustion.

### State management

Current season/episode, completion status, and run count persist per show in SQLite (`series_state` table), across restarts. State changes are pending in memory until the schedule applies successfully to Tunarr — commit on success, rollback (discard) on failure. See the [Web UI's Series page](web-ui-guide.md#series-series) for inline cursor editing, or `schedularr state` in the [CLI Reference](cli-reference.md#series-state) for the command-line equivalent.

### Idempotent apply and editing a block before it airs

A given block occurrence (its cron-computed start time) is only ever planned for real, advancing a series cursor, **once** — the first time any apply's window covers it. Because the default 6h cron interval re-applies more often than the 24h window it covers, the same not-yet-aired occurrence is re-examined by several consecutive applies; each of those re-examinations reuses what was already decided instead of re-planning from the live cursor, which is what makes repeated applies safe rather than silently skipping episodes ahead over time.

That reuse still lets you edit a block before an occurrence airs and see the change take effect:

- **Filter blocks**: once an occurrence's content is picked, it's frozen — reused verbatim on every later apply, aired or not. There's no "cursor" to re-derive for random content, so editing the filter criteria only affects occurrences not yet committed to (i.e. still outside every apply's window so far).
- **Series blocks**: a not-yet-aired occurrence's content is *re-derived* on every apply, from a fixed starting cursor (each show's season/episode as of when the occurrence was first reached) combined with the block's **current** spec. Reordering `series`, adding or removing an entry, or changing `episodes_per_block`/`duration` before the occurrence airs changes what it schedules — same episodes, or a different set, per what the new spec says — without advancing or duplicating anything.
- Once an occurrence's start time has passed, it's aired: frozen and replayed verbatim from then on, exactly like a filter block, regardless of any later spec edit. This includes an occurrence that's **currently on air** — still playing when an apply runs, its start time before the apply's own window but not yet finished — which every apply now explicitly keeps in the pushed lineup at its original start time instead of silently dropping mid-episode; see [Channel ownership](#channel-ownership) below for why that matters for the channel's playback anchor.
- Each occurrence's effect on the persisted per-show state (`series_state`) is decided exactly once, at plan time, and stored with the occurrence. Once the occurrence airs, that stored *post-state* — cursor, completion, disable, run count — is replayed into the persisted state, never reconstructed from the aired content and never re-planned against a since-edited spec. Replays apply in *plan order*: each carries the sequence of the plan that produced it, and a newer plan's post-state always wins — in either direction, so an `on_complete: restart` wrap (S01E05 → S01E01) lands rather than freezing the cursor at its pre-wrap high point — while a *stale* replay from an older plan (say, a slower block sharing the same show re-applying its on-air occurrence after a faster block moved on) is rejected outright.
- A `PATCH /state/series/{show_title}` (manual cursor edit, in either direction), `schedularr state set`/`state reset`/`state import`, or a block edit/delete invalidates every not-yet-*finished* occurrence snapshot for the affected block(s) — including one that's currently on air, not just occurrences that haven't started yet — so the change shapes the very next apply's re-derivations instead of being shadowed until every already-snapshotted occurrence ages out of the window on its own. An operator write always sticks, a *backward* jump included: operator writes are timestamped, and an aired occurrence planned *before* the write never overrides it — the persisted state stays where the operator put it, the on-air occurrence keeps replaying its already-aired content verbatim, and the new cursor takes effect on the next not-yet-aired occurrence. See the [API Reference](api-reference.md#series-state).

Conflict-dropped occurrences (see below) never reach this at all — they're excluded before planning, so they can't advance a cursor or get recorded.

**Example — Sunday sitcom marathon, unlimited restarts:**

```yaml
blocks:
  - name: "Sunday Sitcom Marathon"
    type: series
    cron: "0 12 * * 0" # Every Sunday at 12 PM
    duration: 240 # 4-hour block
    channel_id: "comedy-channel"
    priority: 40
    series:
      - show_title: "The Office (US)"
        episodes_per_block: 4
        on_complete: "restart"
        max_runs: 0
    filler:
      enabled: true
      filler_list_id: "comedy-bumps"
      min_gap_time: 5
```

**Example — daily documentary continuation with filler fallback:**

```yaml
blocks:
  - name: "Daily Docs"
    type: series
    cron: "0 19 * * 1-5" # Weekdays at 7 PM
    duration: 90
    channel_id: "documentary-channel"
    priority: 60
    max_duration_overflow_minutes: 10
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

## Filler content

Fills time gaps after primary content (and, for series blocks, `fallback`) has been scheduled — commercials, bumpers, promos, PSAs:

```yaml
filler:
  enabled: bool
  filler_list_id: string # Required if enabled -- a Tunarr filler list ID
  max_filler_time: int # Optional: cap filler duration, minutes (0 = unlimited)
  min_gap_time: int # Optional: minimum gap before adding filler, minutes (default 0)
```

Behavior: after scheduling main content, compute the remaining gap, cap it at `max_filler_time` if set, fetch and shuffle the filler list's programs, then greedily add filler until the gap is filled or the cap is reached.

## Cron scheduling

Standard 5-field cron expressions, validated by `github.com/robfig/cron/v3`:

```text
┌─────────── minute (0-59)
│ ┌───────── hour (0-23)
│ │ ┌─────── day of month (1-31)
│ │ │ ┌───── month (1-12)
│ │ │ │ ┌─── day of week (0-7, Sunday = 0 or 7)
* * * * *
```

`*` (any value), `,` (list, `1,3,5`), `-` (range, `1-5`), `/` (step, `*/15`).

```yaml
cron: "0 6 * * *"        # Every day at 6 AM
cron: "0 21 * * 1-5"     # Weekdays at 9 PM
cron: "0 8,14 * * 6-7"   # Weekends at 8 AM and 2 PM
cron: "0 */2 * * *"      # Every 2 hours
cron: "0 0 1 * *"        # First of the month at midnight
cron: "30 19 * * 1,3,5"  # Mon/Wed/Fri at 7:30 PM
```

Validate with `schedularr validate scheduler.yaml` before deploying — see the [CLI Reference](cli-reference.md#validation).

The [Web UI's blocks editor](web-ui-guide.md#schedule-picker) offers a Simple mode alternative to hand-writing cron: a frequency select, day-of-week checkboxes, and a time input that generate the cron string live. It parses back from an existing cron string when the pattern is representable in Simple mode (a fixed time, optionally restricted to weekdays or a single day-of-month); anything more complex — a day-of-month combined with a weekday restriction, a month restriction, a list/range/step on minute or hour — stays in Cron mode. A plain-language readback (cronstrue) renders under the field in both modes.

## Priority and conflict resolution

When multiple blocks schedule content for overlapping time periods, the higher `priority` value wins; the conflicting lower-priority block is discarded entirely. Every dropped occurrence is both logged server-side and reported in the API response's `warnings` array (`POST /generate` and `POST /apply`, see the [API Reference](api-reference.md#schedule)) — surfaced on the [Web UI's Schedule page](web-ui-guide.md#schedule-schedule) after every preview or apply, not just visible in a server log.

```text
Block A: [10:00-12:00], priority 10
Block B: [11:00-13:00], priority 5

Overlap: [11:00-12:00]
Winner: Block A
Result: Block A scheduled, Block B discarded
```

Suggested ranges: **1-10** low priority (filler, background programming), **11-50** normal priority (regular programming), **51-100** high priority (special events, live content).

## Channel ownership

**Every channel Schedularr applies to is Schedularr's alone, for its entire timeline.** Applying a schedule (`--apply`, the cron loop, or `POST /schedule/apply`) doesn't layer content into gaps in whatever a channel already has — it replaces the channel's whole Tunarr lineup, off-hours included, every single time:

- The apply window (`--days`, default 1) is fully covered end to end. Time your blocks don't schedule anything for isn't left alone — it's filled with **flex** (dead-air/offline) entries, so the pushed lineup always spans the entire window.
- The channel's own playback clock (Tunarr's `channel.startTime`) is reset on every apply, anchored to the start of that window — so the flex-padded lineup actually plays back at the wall-clock times its blocks were scheduled for rather than wherever Tunarr's internal position happened to be — **unless** something is currently on air on that channel at apply time, in which case the anchor shifts back to that occurrence's own original start time instead. Anchoring at the window's own start in that case would otherwise make Tunarr replay the on-air occurrence from its beginning the moment the new lineup takes effect (or, worse, replace it outright); anchoring at its real start lets Tunarr's wall-clock playback formula resolve to the correct position partway through it instead.
- This is a **full replacement**, not an append: anything on the channel that Schedularr didn't just schedule — a manual edit made through Tunarr's own UI, content left over from before the channel was handed to Schedularr — is gone after the next apply, without warning.

The practical rule: **don't hand a channel to Schedularr and then also edit its programming by hand.** Pick one owner per channel. A channel with occasional human-curated blocks alongside Schedularr's blocks isn't supported — the next apply erases the human edits; a channel Schedularr doesn't manage at all is completely unaffected (Schedularr only ever touches channel IDs its blocks reference).

This design keeps the apply model simple and its result fully predictable from the block configuration alone — what you'd get from re-running `--dry-run` is exactly what's on the channel after `--apply`, with no hidden state from a previous manual change or a prior apply's leftovers. The cost is that ownership is all-or-nothing per channel.

## Schedule history and retention

Schedule history prevents content repetition. It's both an in-memory dedup check during a single generate/apply cycle (cleared on restart, keyed `channel_id:program_id`) and a persisted `schedule_history` SQLite table, queryable via `GET /history?days=N` (see the [API Reference](api-reference.md#history)), that survives restarts.

- **Window**: `maintenance.history_retention` (default `168h`, 7 days) — see the [Deployment config reference](deployment.md#configuration-reference).
- **Before scheduling**: recently-played programs are excluded from candidates (in-memory check, then a `schedule_history` lookup).
- **After scheduling**: program + timestamp recorded, both in-memory and, on `Engine.Commit()`, persisted.
- **Cleanup**: every successful apply deletes `schedule_history` rows older than the retention window.

`GET /history?days=N` can only return data as far back as `history_retention` allows — `?days=90` needs `history_retention` set to at least `2160h` to actually have 90 days of persisted rows; the 7-day default limits queries to the last 7 days regardless of what `days` the caller requests.
