# Scheduling Concepts

A **block** is a scheduling rule: a cron trigger, a duration, a target Tunarr channel, a priority, and either a content **filter** or a list of **series** to progress through. Blocks live in the SQLite store, not in a file — `scheduler.yaml` is a first-run *import* format only (see [Getting Started](getting-started.md)); manage blocks going forward through the `/api/v1/blocks` HTTP API or the [Web UI](web-ui-guide.md).

## Block structure

```yaml
blocks:
  - name: string # Required: human-readable block identifier
    type: string # Optional: "filter" (default) or "series"
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

Each block's `type` field (`filter` or `series`) defaults to `filter` and can be omitted — in `scheduler.yaml` and in `POST /api/v1/blocks`'s JSON body alike (the import path validates the raw YAML against CUE before decoding, so the schema default applies to an absent field). An explicit `type: ""` is rejected.

## Filter-based blocks

Applies filter criteria to available programs, in AND logic across criteria (a program must match every specified criterion). Matching content is checked against schedule history to avoid recent repeats, shuffled, and greedily selected to fill the block's duration.

Since v0.5.6 that shuffle is **deterministic per (block, occurrence)**: the candidates (and any filler) are put in a total order and then shuffled with a seed derived from the block and the occurrence's start, so re-planning the same occurrence against the same library and the same committed history yields the same lineup. A dry run and the apply that follows it therefore schedule the same content, so the plan the operator previewed is the plan that lands. Variety across occurrences is unaffected: the seed changes with each one.

The recency check behind that shuffle counts only occurrences that air **earlier** than the one being planned. A single run fills its own in-memory history as it goes, block by block, so a mid-window occurrence would otherwise be filtered against occurrences that have not aired yet — and how many of those exist depends on how far ahead the caller asked the engine to plan. Bounding the check by air time is what lets a 7-day plan and a 28-day plan schedule the shared days identically, which is the whole basis of the [Guide's draft diff](web-ui-guide.md#draft-apply): without it, an untouched draft reads `CHANGED` on any channel where two filter blocks share a content pool.

```yaml
filter:
  title_pattern: string # Regex pattern for title matching (Go regex syntax)
  genres: []string # OR logic within the list -- matches ANY of these genres
  ratings: []string # OR logic within the list
  year_from: int # Minimum release year
  year_to: int # Maximum release year
  min_duration: int # Minimum duration, minutes
  max_duration: int # Maximum duration, minutes
  tags: []string # ACCEPTED BUT NOT YET EVALUATED -- no matcher consumes tags today (operator directive 2026-09-08: implemented next, ahead of the metadata enrichment; see docs/roadmap.md)
```

| Field                           | Example                                | Notes                                                            |          |
| ------------------------------- | -------------------------------------- | ---------------------------------------------------------------- |          |
| `title_pattern`                 | `"^Star"`, `"(Trek\                    | Wars)"`, `"\\d+$"`                                               | Go regex |
| `genres`                        | `["Action", "Adventure", "Sci-Fi"]`    | Matches any listed genre                                         |          |
| `ratings`                       | `["PG", "PG-13", "TV-PG"]`             | TV: TV-Y…TV-MA; movie: G…NC-17; or NR/Unrated                    |          |
| `year_from` / `year_to`         | `1980` / `1999`                        | Inclusive range                                                  |          |
| `min_duration` / `max_duration` | `90` / `150`                           | Minutes; stored in Tunarr as milliseconds internally             |          |
| `tags`                          | `["christmas", "family-favorite"]`     | Accepted, **not yet evaluated** — next, ahead of enrichment (Q7) |          |

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

**Shared shows must agree on their policy — enforced.** Series state is keyed by show title, shared across every block that schedules the show — `on_complete` included. Two blocks giving the same show *contradictory* policies (one `disable`, another `restart` or `continue`) would fight over that shared state, so every write path rejects the contradiction with a `400` naming the show and both blocks: block create/update (against every *enabled* block — a disabled block plans nothing and fights nobody, but re-enabling it re-validates, and a block that is merely [dark](#a-dark-block-is-still-a-competitor) is checked throughout, since it comes back on its own), `POST /blocks/import`, and `scheduler.yaml` import. `POST /blocks/{id}/duplicate` runs the same check even though the copy arrives disabled — a contradiction it introduced would otherwise surface the moment somebody enabled it, which is the worst moment to learn of it. The compared policy is the pair `on_complete` + (for `restart`) `max_runs` — a per-block `max_runs` cap counts against the show's shared `run_count`, so disagreeing caps are the same fight. An omitted `on_complete` counts as the default, `continue`. Relatedly, `skip_episodes` also acts on the shared cursor: a skip in one block skips for every block sharing the show.

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

Current season/episode, completion status, and run count persist per show in SQLite (`series_state` table), across restarts. State changes are pending in memory until the schedule applies successfully to Tunarr — commit on success, rollback (discard) on failure. See the [Web UI's History page](web-ui-guide.md#history-history) for inline cursor editing, or `schedularr state` in the [CLI Reference](cli-reference.md#series-state) for the command-line equivalent.

### Idempotent apply and editing a block before it airs

A given block occurrence (its cron-computed start time) is only ever planned for real, advancing a series cursor, **once** — the first time any apply's window covers it. Because the default 6h cron interval re-applies more often than the 24h window it covers, the same not-yet-aired occurrence is re-examined by several consecutive applies; each of those re-examinations reuses what was already decided instead of re-planning from the live cursor, which is what makes repeated applies safe rather than silently skipping episodes ahead over time.

That reuse still lets you edit a block before an occurrence airs and see the change take effect:

- **Filter blocks**: once an occurrence's content is picked, it's frozen — reused verbatim on every later apply, aired or not. There's no "cursor" to re-derive for random content, so editing the filter criteria only affects occurrences not yet committed to (i.e. still outside every apply's window so far).
- **Series blocks**: a not-yet-aired occurrence's content is *re-derived* on every apply, from a fixed starting cursor (each show's season/episode as of when the occurrence was first reached) combined with the block's **current** spec. Reordering `series`, adding or removing an entry, or changing `episodes_per_block`/`duration` before the occurrence airs changes what it schedules — same episodes, or a different set, per what the new spec says — without advancing the persisted cursor. A show **added** to a block whose pending occurrence already has a seed plans from its *live* cursor (the seed doesn't know it, so the live state is the only truth there is), resuming where it left off rather than re-airing its pilot.
- Once an occurrence's start time has passed, it's aired: frozen and replayed verbatim from then on, exactly like a filter block, regardless of any later spec edit. This includes an occurrence that's **currently on air** — still playing when an apply runs, its start time before the apply's own window but not yet finished — which every apply now explicitly keeps in the pushed lineup at its original start time instead of silently dropping mid-episode; see [Channel ownership](#channel-ownership) below for why that matters for the channel's playback anchor.
- Each occurrence's effect on the persisted per-show state (`series_state`) is decided exactly once, at plan time, and stored with the occurrence. Once the occurrence airs, that stored *post-state* — cursor, completion, disable, run count — is replayed into the persisted state, never reconstructed from the aired content and never re-planned against a since-edited spec. Replays apply in *plan order*: each carries the sequence of the plan that produced it, and a replay from a plan *older* than whatever last wrote the cursor is always rejected. For a replay from a *newer* plan, direction matters: a **forward** move always wins, while a **backward** move (an `on_complete: restart` wrap, S01E05 → S01E01) additionally requires the plan's own starting baseline to match the live cursor — a wrap that planned *from* the current high-water mark lands; a wrap-shaped result derived from some long-stale baseline does not.
- The same `show_title` may appear in more than one series block — nothing forbids it, and the blocks then share that show's single cursor, advancing it cooperatively: each block plans its occurrences from wherever the cursor stands when it plans, forward advances land in plan order, and the baseline-agreement rule above is what keeps sharing safe — a slower block's occurrence, planned long ago from an old cursor, can never *rewind* the cursor a faster block has since advanced (its baseline no longer matches), so the next occurrence anywhere continues from the furthest point actually reached. What sharing does **not** give you is per-block independent progress: one cursor means the blocks interleave one continuous run of the show, not two parallel runs.
- Spec edits and cursor edits differ deliberately: a **spec edit** (`PUT /blocks/{id}` — reorder, add/remove a series, change `episodes_per_block`/`duration`) is *seed-preserving* — the pending occurrence keeps its captured starting cursor and simply re-derives against the new spec, so it plays the **same episodes, re-arranged**, never skipping ahead to wherever the live cursor has moved. **`cron` is the exception**: changing it *moves* occurrences to new start keys that have no seed, so an occurrence already committed for the old start is abandoned and its replacement plans from the live cursor — skipping the committed round. If you must change a block's `cron` while an occurrence is committed, follow it with a compensating `PATCH /state/series` rewind for that block's shows; a **cursor edit** (`PATCH /state/series/{show_title}`, `schedularr state set`/`state reset`/`state import`) is *seed-overriding* — it invalidates every not-yet-*finished* occurrence snapshot for the affected block(s), including one that's currently on air, so the re-derive starts from the operator's new cursor on the very next apply instead of being shadowed until the old snapshots age out. (A block *delete* also clears its snapshots, as orphan cleanup.) An operator cursor write always sticks, a *backward* jump included: operator writes are timestamped, and an aired occurrence planned *before* the write never overrides it — the persisted state stays where the operator put it, the on-air occurrence keeps replaying its already-aired content verbatim, and the new cursor takes effect on the next not-yet-aired occurrence. See the [API Reference](api-reference.md#series-state).

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

## Enabled, dark, and the single planning gate

Two switches decide whether a block is planned at all, and they answer different operator questions:

| Switch           | Written by                                                      | Undone by                       | The question it answers                             |
| ---------------- | --------------------------------------------------------------- | ------------------------------- | --------------------------------------------------- |
| `enabled`        | `POST`/`PUT /blocks` (defaults to `true`), `PATCH /blocks/{id}` | an operator, by hand            | Is this rule part of my lineup at all?              |
| `disabled_until` | `PATCH /blocks/{id}` only                                       | the clock, at the instant named | Skip it while I'm away, and put it back without me. |

They are independent axes: a block is planned only when it is enabled **and** now is at or past its `disabled_until`, so setting either one suppresses it and setting one never writes the other. A `PATCH` carrying only `enabled` finds its dark window exactly where it left it, and a full `PUT /blocks/{id}` spec save doesn't disturb it either — `disabled_until` isn't part of a spec body. Bringing a block back early is an explicit `{"disabled_until": null}`; an absent key means "leave it alone", so `null` is the only way to say "clear it".

**"Until" names the instant the block returns**, not the last moment it is dark — a wake time equal to now is already awake. A `disabled_until` in the past is *stale*, not cleared: nothing sweeps the column, and nothing needs to, because the gate compares it against now rather than testing whether a value is present. Anything that renders a "dark" affordance has to make that same comparison, which is why a window that lapsed last week paints nothing on the [Blocks page](web-ui-guide.md#blocks-blocks).

### One gate, every path

`service.ActiveBlocks(ctx, store, now)` is the only place in the codebase that decides whether a block reaches the engine. The `serve` cron loop, `POST /api/v1/generate`, `POST /api/v1/apply` and `schedularr generate` all load their blocks through it, so both switches hold on every path by construction rather than by three separate checks agreeing with each other. A new suppression rule belongs in that function and nowhere else.

Its clock is a parameter rather than a `time.Now()` inside it. An apply passes the same instant it derives its window from, so a block can never be dark for the gate and awake for the window — two independent reads would sit a program fetch apart.

### The dark window doesn't round-trip through `scheduler.yaml`

`disabled_until` is a column on the block row beside `enabled`, not a field inside the stored spec, which keeps both switches in one row and one write. The consequence is worth stating plainly because it will surprise someone: `GET /blocks/export` renders block **specs**, so neither `disabled_until` nor `enabled` survives an export. Export, wipe the store, re-import, and every block comes back **awake and enabled** — `POST /blocks/import` and first-run `scheduler.yaml` bootstrap both create blocks enabled with no dark window.

That split is deliberate. `scheduler.yaml` describes scheduling rules; a dark window is operational state about one deployment at one moment, which a rules file has no business carrying. If a re-import is part of a restore, re-apply the dark windows by hand afterwards. Nothing has to be re-derived to do it: a dark window is one instant and one wake-up, never a recurrence.

### A dark block is still a competitor

The [shared-show policy check](#completion-actions-on_complete) treats a dark block as live, because it is defined and it comes back. A write is validated against every *enabled* block, dark ones included, so a second block can't claim a contradictory `on_complete` policy for a show while the first sits dark and then collide with it the moment it wakes.

Conflict resolution is the opposite case, and for the same reason. A dark block contributes no occurrences at all — it never reaches the engine — so it can't displace a lower-priority block on its channel while it's out. It stops competing for airtime and keeps competing for shared series state.

## What "on air" means, and why some changes are refused

An occurrence is **on air** when now falls inside `[start, start + duration + max_duration_overflow_minutes)`. The overflow is part of the window on purpose: content that legitimately runs past its slot is still playing, and the snapshot-invalidation path already treats it that way.

Since v0.5.11, a write that would take an airing block out of the next plan is refused with a `409` naming when it becomes safe — deleting it, switching it off, giving it a dark window, or moving its `cron`, channel or duration. Editing its *filter* is not refused, because that leaves the occurrence exactly where it is.

This matters more than it looks, because the damage is deferred rather than absent. Nothing reaches Tunarr at write time; the effect lands at the next apply, which the cron loop performs unattended at process start and then every `cron_interval`. And the failure is not a shortened lineup. Tunarr plays a pushed lineup as `elapsed = (now − channel.startTime) % channel.duration`, so a channel is only ever anchored, never partially updated: an occurrence that generates no shell leaves nothing to anchor at, the channel re-anchors at now, and **its whole lineup restarts**. If the block was that channel's last, the channel is pushed a flex-only lineup instead — dead air, mid-episode.

The predicate asks whether an *occurrence* is live, never how its content was chosen, so a filter block is covered exactly like a series one. It also evaluates in the configured `log.timezone` rather than the host's: a block's cron carries no zone of its own, so reading the same expression in UTC would answer for a different hour entirely.

## Removing a show from history

`series_state`, the occurrence snapshots and the airings in `schedule_history` are three tables holding one show's progression, and they are removed together in a single transaction or not at all. A partial removal would leave a cursor pointing at airings that no longer exist, which is worse than leaving everything in place.

**A removal refuses while any block still lists the show.** This is not caution; it is the difference between removing a show and appearing to. The engine re-adds any `series[].show_title` that is missing from a block's chain, seeded from the default first episode — so a removal that ran anyway would be undone by the next apply *and* would reset the cursor it had just deleted. The refusal names the blocks, so the order is: edit those blocks first, then remove.

Three things a removal deliberately does not do:

- **It does not delete snapshot rows**, only the show's key inside them. A snapshot describes one occurrence of one block, which may have carried several shows.
- **It does not touch airings that belong to no show** — movies, and every row written before the `show_title` column existed. Those carry an empty show title, and matching it would sweep all of them.
- **It does not let an emptied occurrence read as never planned.** An occurrence that loses its last airing keeps a marker saying it was planned and produced nothing, which is what actually happened. Without it the next apply would re-plan a slot that has already gone out — rewriting history rather than removing from it.

**Airings written before the `show_title` column cannot be removed by title.** Recovering a show from a stored program id needs the live Tunarr catalogue, which may no longer carry that program at all, and guessing from the block that aired it is wrong for any block scheduling more than one show. Those rows still age out through retention.

## Priority and conflict resolution

When multiple blocks schedule content for overlapping time periods, the higher `priority` value wins; the conflicting lower-priority block is discarded entirely. Every dropped occurrence is both logged server-side and reported in the API response's `warnings` array (`POST /generate` and `POST /apply`, see the [API Reference](api-reference.md#schedule)) — surfaced on the [Guide](web-ui-guide.md#the-guide) as NO SIGNAL ghost slots at the time each would have aired, not just visible in a server log.

```text
Block A: [10:00-12:00], priority 10
Block B: [11:00-13:00], priority 5

Overlap: [11:00-12:00]
Winner: Block A
Result: Block A scheduled, Block B discarded
```

Suggested ranges: **1-10** low priority (filler, background programming), **11-50** normal priority (regular programming), **51-100** high priority (special events, live content).

## Channel ownership

**Every channel Schedularr applies to is Schedularr's alone, for its entire timeline.** Applying a schedule (`--apply`, the cron loop, or `POST /api/v1/apply`) doesn't layer content into gaps in whatever a channel already has — it replaces the channel's whole Tunarr lineup, off-hours included, every single time:

- The apply window (one day for `schedularr generate`, which registers no `--days` flag; the request's `days` for `POST /api/v1/apply`, and seven for a draft applied from the Guide) is fully covered end to end. Time your blocks don't schedule anything for isn't left alone — it's filled with **flex** (dead-air/offline) entries, so the pushed lineup always spans the entire window.
- The channel's own playback clock (Tunarr's `channel.startTime`) is reset on every apply, anchored to the start of that window — so the flex-padded lineup actually plays back at the wall-clock times its blocks were scheduled for rather than wherever Tunarr's internal position happened to be — **unless** something is currently on air on that channel at apply time, in which case the anchor shifts back to that occurrence's own original start time instead. Anchoring at the window's own start in that case would otherwise make Tunarr replay the on-air occurrence from its beginning the moment the new lineup takes effect (or, worse, replace it outright); anchoring at its real start lets Tunarr's wall-clock playback formula resolve to the correct position partway through it instead.
- This is a **full replacement**, not an append: anything on the channel that Schedularr didn't just schedule — a manual edit made through Tunarr's own UI, content left over from before the channel was handed to Schedularr — is gone after the next apply, without warning.

The practical rule: **don't hand a channel to Schedularr and then also edit its programming by hand.** Pick one owner per channel. A channel with occasional human-curated blocks alongside Schedularr's blocks isn't supported — the next apply erases the human edits; a channel Schedularr doesn't manage at all is completely unaffected (Schedularr only ever touches channel IDs its blocks reference).

**Ownership ends with a single clear, not a lingering lineup.** When a channel's last block is deleted or disabled, the channel drops out of the plan — but the previous apply's lineup would otherwise keep airing in Tunarr indefinitely, since "no longer pushed to" isn't the same as "cleared". Schedularr tracks every channel it pushes a lineup to (the `applied_channels` table); the next apply after a channel drops out of the plan pushes one flex-only (dead-air) lineup to it and stops tracking it. That clear happens exactly once per hand-back: afterwards the channel is genuinely unmanaged again, so taking it over manually in Tunarr — or just leaving it empty — is safe, and re-adding a block for it simply resumes normal ownership. A channel-scoped apply (`channel_id` set) only ever clears that one channel.

This design keeps the apply model simple and its result fully predictable from the block configuration alone — what you'd get from re-running `--dry-run` is exactly what's on the channel after `--apply`, with no hidden state from a previous manual change or a prior apply's leftovers. The cost is that ownership is all-or-nothing per channel.

## Schedule history and retention

Schedule history prevents content repetition. It's both an in-memory dedup check during a single generate/apply cycle (cleared on restart, keyed `channel_id:program_id`) and a persisted `schedule_history` SQLite table, queryable via `GET /history?days=N` (see the [API Reference](api-reference.md#history)), that survives restarts.

- **Window**: `maintenance.history_retention` (default `168h`, 7 days) — see the [Deployment config reference](deployment.md#configuration-reference).
- **Before scheduling**: recently-played programs are excluded from candidates (in-memory check, then a `schedule_history` lookup).
- **After scheduling**: program + timestamp recorded, both in-memory and, on `Engine.Commit()`, persisted. Each row also carries the `run_id` of the apply that committed it.
- **Cleanup**: every successful apply deletes `schedule_history` rows older than the retention window.

`GET /history?days=N` can only return data as far back as `history_retention` allows — `?days=90` needs `history_retention` set to at least `2160h` to actually have 90 days of persisted rows; the 7-day default limits queries to the last 7 days regardless of what `days` the caller requests.

### Retention is per table

Three tables grow with time, and each prunes on its own knob:

| Table                               | Knob                              | Default           | What it bounds                                                 |
|-------------------------------------|-----------------------------------|-------------------|----------------------------------------------------------------|
| `schedule_history`                  | `maintenance.history_retention`   | `168h`            | The recency-dedup window and `GET /history?days=N`             |
| `series_occurrence_snapshots`       | `maintenance.snapshot_retention`  | `168h`            | How long a not-yet-aired occurrence's cursor snapshot survives |
| `apply_runs` + `apply_run_warnings` | `maintenance.apply_run_retention` | `2160h` (90 days) | `GET /applies?days=N`                                          |

Setting `snapshot_retention` **longer** than `history_retention` keeps rows that serve no purpose: an occurrence outside the schedule-history window can never be replayed, because the `schedule_history` rows it would replay from are already gone.

Apply runs default to a far longer horizon than the other two because they answer a different question. History and snapshots feed the engine's own replay and dedup machinery, where a week is ample; a run card is the operator's only durable answer to "why didn't X air last Tuesday", and it cannot be backfilled — nothing exists from before the migration that created the table.

### Apply runs

Every apply — from the web UI, the `serve` cron loop, or `schedularr generate --apply` — is recorded as a row in `apply_runs`, together with the conflict warnings that apply had to drop.

- The row is written **before** the apply pushes anything to Tunarr, with status `running`, and finalized afterwards. A row still reading `running` means the process died mid-apply; that is information, not a defect to hide.
- A **failed** apply is still a run, carrying its error detail.
- Every `schedule_history` row the apply commits is stamped with that run's ID, so an aired program traces back to the apply that put it there. Rows written before this table existed carry an empty `run_id` — runs are never backfilled.

Read them at `GET /api/v1/applies` (see the [API Reference](api-reference.md#apply-runs)) or on the web UI's [History page](web-ui-guide.md#history-history).
