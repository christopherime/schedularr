# API Reference

Schedularr exposes an HTTP API defined by an OpenAPI 3.0.3 contract at [`api/openapi.yaml`](https://github.com/christopherime/schedularr/blob/main/api/openapi.yaml) in the repository. The contract covers blocks CRUD, block import/export, cron occurrence lookup, schedule generation and application, history, series state, channels, and status.

!!! note "The live contract"
    A running `schedularr serve` instance serves the same contract as JSON at `GET /openapi.json` (unauthenticated) — that endpoint is the source of truth for whatever version you're actually running. The tables below describe the contract as of this page's writing; `/openapi.json` never drifts from your binary.

Server code is generated from the contract with [oapi-codegen](https://github.com/oapi-codegen/oapi-codegen) v2 (`make generate`), writing `internal/api/gen/server.gen.go` — committed, and not hand-edited. Handlers live in `internal/api/` and implement the generated `gen.ServerInterface`. Errors use an RFC 7807 `application/problem+json` body.

The API is served by `schedularr serve` — see the [CLI Reference](cli-reference.md#serve) for flags and startup behavior.

## Blocks

Every full-spec write path (`POST`/`PUT`) validates the block spec against the CUE scheduler schema, plus the two rules CUE cannot express (a non-empty `show_title`, a parseable `cron`), before touching the store; every response body is `application/json` (or `application/problem+json` for errors).

| Method | Path                     | Success | Error codes        |
|--------|--------------------------|---------|--------------------|
| GET    | `/blocks`                | 200     | —                  |
| POST   | `/blocks`                | 201     | 400, 409           |
| GET    | `/blocks/{id}`           | 200     | 404                |
| PUT    | `/blocks/{id}`           | 200     | 400, 404, 409, 412 |
| PATCH  | `/blocks/{id}`           | 200     | 400, 404, 409      |
| POST   | `/blocks/{id}/duplicate` | 201     | 400, 404, 409      |
| DELETE | `/blocks/{id}`           | 204     | 404, 409           |

- `POST`/`PUT` return `400` for a spec that fails CUE validation (e.g. a missing `cron` or a non-positive `duration`) or a malformed JSON body.
- `POST` returns `409` for a duplicate block name; `PUT` returns `409` if the request body's `spec.name` differs from the existing block's name and collides with another block. A `PUT` whose `spec.name` differs from the current name without colliding renames the block.
- `POST`/`PUT` also return `400` for a series block (`spec.type: series`) whose `series[].show_title` is empty. The CUE scheduler schema types `show_title` as a bare `string` with no non-empty constraint, so this case would otherwise pass CUE validation — the check is applied in Go on both block-ingestion paths (here, and `blocks/import` below).
- `POST`, `PUT`, **and** `POST /blocks/import` return `400` (`title: "block validation failed"`) for a `cron` that will not parse, naming the offending expression — or, for import, every offending block's name. CUE types `cron` as a bare `string` and cannot express "parses as a cron expression", so the check runs in Go, through `scheduler.NewCronParser()`: the same parser configuration the engine plans with, so it can never reject an expression the planner would have accepted nor accept one the planner would later choke on. Previously a typo was accepted at write time and surfaced much later as a `502` from the engine at apply time, in a place that cannot point at the field the operator typed. Import matters most of the three, because imported blocks are created **enabled** — a bad expression there went straight into schedule generation.
- **Live-data hazard from that check.** A block already sitting in the store with an unparseable cron cannot be saved through `PUT` until its cron is fixed: the write path now refuses a body it will happily hand you from `GET`. Repair the expression in the same edit. `PATCH` is unaffected (it validates no spec), so enabling, disabling and dark windows still work on such a block, and its `next_occurrence` is simply absent. `POST /blocks/{id}/duplicate` does not re-validate the source's cron either, so a copy inherits the bad expression as-is.
- `PUT` requires an `If-Match` header carrying the block's current `updated_at`, which you read from a prior `GET`. A missing or unparseable header is `400`; a value that no longer matches the stored `updated_at` is `412` (`title: "block changed since you loaded it"`). This is lost-update protection: `PUT` replaces a block's entire spec, so without it two operators editing one block silently discard the slower one's work. The value is compared as an instant rather than as a string — a client that round-trips `updated_at` through JSON may re-render it equivalently but not identically — and surrounding quotes are accepted, since callers reasonably treat it as an entity tag. The correct response to a `412` is to reload the block and re-apply the edit, never a silent retry.
- `PATCH /blocks/{id}` is the field-scoped complement to `PUT`: only fields present in the body change (`enabled` and `disabled_until` today), so an enable/disable toggle cannot clobber a spec edit made elsewhere. It takes **no** `If-Match` for that reason — there is no unrelated state for it to overwrite. A body with neither field set is `400` (`title: "empty patch"`), matching `PATCH /state/series/{show_title}`. Re-enabling a block re-enters the same shared-show policy check a create or full update runs, which fails as a `400` (`title: "block validation failed"`); disabling never triggers it.
- `enabled` and `disabled_until` are **independent axes**, and `PATCH` writes them independently. `enabled: false` is the indefinite switch: only an operator undoes it. `disabled_until: <instant>` is the timed one: the clock undoes it. A block is planned only when both are clear — `service.ActiveBlocks` is the single gate every path reaches the engine through, so the cron loop, the API and the CLI all honor it identically. "Until" names the instant the block **returns**, so a wake time equal to now is already awake. Setting one never writes the other: a `PATCH` carrying only `enabled` leaves the dark window exactly where it was, and `PUT` never touches it at all.
- Because `disabled_until` is a column on the block record rather than a field on its spec, it does **not** round-trip through `/blocks/export` and `/blocks/import`. Those exchange block *specs*; a temporary dark window is operational state, and an exported file re-imported elsewhere brings back the blocks without their dark windows.
- On `disabled_until`, an explicit `null` and an absent key are deliberately different. `null` **clears** the window — the only way to bring a block back early — and an absent key leaves it alone; the handler decodes that one field as raw JSON precisely to keep absent, `null`, and an instant apart, which a nullable timestamp alone cannot. A value that is not a valid RFC 3339 instant is `400` (`title: "invalid disabled_until"`) rather than a silent no-op, because dropping it would leave the block airing while the operator believes they just took it dark. An instant in the past is accepted and inert: nothing sweeps it, so a client rendering a "dark" affordance must compare against now rather than test for the field's presence.
- `next_occurrence` on a `BlockRecord` is the next instant the block will **actually air**, which is not the same question as what its cron says next. It honors both switches: for a block currently dark, the search starts at its wake instant, so the value is the first occurrence at or after the block returns rather than the next raw cron tick — a NEXT reading beside a block dark until next month would be a reading that lies. It is computed per record on every read, against the server's clock, in the configured `log.timezone`.
- `next_occurrence` is **absent** in three different situations: the block is disabled, its cron parses but never fires (`0 0 30 2 *`), or its cron will not parse at all. A caller cannot tell those apart from this field alone — read `enabled` and the block's own `spec.cron` to distinguish them. The list deliberately still renders a block whose cron is broken, so it can be the thing the operator goes and fixes.
- `PUT`/`DELETE` on a series block also invalidate every not-yet-*finished* occurrence's cursor snapshot for that block — including one currently on air, not just occurrences that haven't started yet (see [Scheduling Concepts' idempotent-apply section](scheduling-concepts.md#idempotent-apply-and-editing-a-block-before-it-airs)) — so the next apply re-derives those occurrences against the spec you just changed instead of a snapshot captured under the old one.
- `POST /blocks/{id}/duplicate` takes `{"name": "…"}` and returns `201` with the new `BlockRecord`. The name is required and never invented server-side: an empty or whitespace-only name is `400`, a name already taken is `409`, and a missing source is `404`. There is no server-side counter appending `(2)`, because a collision belongs to the caller — they are the one who can see what the other block is. The web UI pre-fills `Copy of <source>` and only prompts when the `409` comes back.
- The copy carries the source's spec **whole** — type, cron, channel, priority, filter, filler, and every series seed — which is what makes it a duplicate rather than a new block wearing a borrowed name. Nothing keyed by the source's id comes with it: series cursors and occurrence snapshots record what the *source* has already aired, and a copy has aired nothing. The copy gets a fresh UUID, fresh timestamps, and no dark window.
- The copy arrives `enabled: false`. An exact copy shares its source's cron, channel and priority, so landing it enabled would put two blocks in contention for one channel at one time and conflict resolution would silently drop one of the two; the copy is a draft the operator edits and then turns on. Arriving disabled does **not** exempt it from the shared-show policy check, though — it is defined and it will come back, so a duplicated series block that would contradict a live block's completion policy is refused now with a `400` rather than at the moment somebody enables it.

### The on-air guard

Three of those writes can change what a viewer is watching, and since v0.5.11 they refuse to while the block is airing:

| Write                                                                                               | Refused when        |
|-----------------------------------------------------------------------------------------------------|---------------------|
| `DELETE /blocks/{id}`                                                                               | the block is on air |
| `PATCH /blocks/{id}` with `enabled: false`, or a future `disabled_until`                            | the block is on air |
| `PUT /blocks/{id}` that changes `cron`, `channel_id`, `duration` or `max_duration_overflow_minutes` | the block is on air |

The refusal is `409` with `title: "block is on air"`, and the detail names both the block and the local time it becomes safe:

```text
Anime Night is airing until 21:47. Deleting it now would cut the current
program. Try again after that.
```

**Why a refusal and not a warning.** Nothing is pushed to Tunarr at write time, so none of these has an immediate effect. That is not a reprieve: `serve`'s cron loop applies at process start and then every `cron_interval`, unattended, so the operator cannot avoid the consequence by declining to apply. And what lands is worse than a shortened lineup — an occurrence that generates no shell leaves the channel with nothing to anchor, so its **whole lineup restarts from the top**; if the block was that channel's last, the channel gets a flex-only lineup, which is dead air mid-episode.

**What is deliberately still allowed.** A `PUT` that changes only a block's *filter* keeps the occurrence exactly where it is — same slot, same length, same channel — so it is never refused. Only a change that moves the block is. Switching a block back **on**, or clearing its dark window, adds to the next lineup rather than removing from it, and is never refused either. Neither is a change to a block that is already disabled or already dark: it generates no shell at the next apply regardless, so nothing can be cut off.

**The airing window includes overflow.** A block is treated as on air for `duration + max_duration_overflow_minutes`, not just `duration`. Content that legitimately overruns its slot is still on air, and the snapshot-invalidation path already uses that wider envelope — a guard that cleared first would refuse to refuse on exactly the content that overran.

**The consequence worth knowing.** While a block is airing you cannot delete it, switch it off, or move its schedule, for up to `duration + max_duration_overflow_minutes`. You can still edit its filter, and you can always wait for the occurrence to finish.

## Import / export

Round-trips the same sqlite-backed blocks through the same YAML parse/render used for the on-disk `scheduler.yaml` bootstrap path. Both endpoints exchange raw YAML text, not JSON.

| Method | Path             | Success | Error codes   |
|--------|------------------|---------|---------------|
| POST   | `/blocks/import` | 200     | 400, 409, 413 |
| GET    | `/blocks/export` | 200     | —             |

- `POST /blocks/import` takes an `application/yaml` body (capped at 1MiB; an oversized body gets `413`) and an optional `?dry_run=true` query parameter (default `false`). The body is strictly decoded and CUE-validated, including duplicate block-name, empty-`show_title`, and unparseable-`cron` rejection — any failure is `400` with the detail included. The cron and `show_title` checks aggregate every offending block's name into one detail rather than stopping at the first, since this is the batch path. Every parsed block's name is checked against every block already in the store; any collision is `409` (listing the colliding name(s)) with **zero writes**, even for the non-colliding blocks in the same batch. `dry_run=true` stops after that check and reports what would have been imported (`{imported, dry_run, names}`) without writing anything; otherwise every block is created with a fresh UUID and `enabled: true`.
- `GET /blocks/export` renders every stored block's spec as YAML — **including disabled blocks**, since export doubles as a backup mechanism. The response has no `enabled` state of its own, and no `disabled_until` either (both live on the store record, not in the spec); re-importing an exported file creates every block as enabled and with no dark window.

## Cron occurrences

Evaluates a cron expression and hands back its next start instants. It exists so no client ever re-implements calendar semantics: DST transitions, month lengths, and descriptor handling belong to one parser — `scheduler.NewCronParser()`, the same configuration the scheduling engine plans with and the same one every block write validates against.

| Method | Path                            | Success | Error codes |
|--------|---------------------------------|---------|-------------|
| GET    | `/cron/next?expr=&count=&from=` | 200     | 400         |

The body is `{"occurrences": ["<RFC 3339>", …]}`, oldest first.

- Stateless — the only endpoint in the contract that reads no store. It knows nothing about blocks, so `expr` need not belong to one; the web UI's block editor calls it against a cron field the operator is still typing.
- `expr` is required: a standard 5-field expression, or a descriptor (`@daily`, `@every 1h30m`). `count` defaults to `3` and is range-checked against `[1, 10]` by the handler itself (`title: "count out of range"`), because oapi-codegen's generated bindings don't enforce the OpenAPI schema's own default/minimum/maximum — the same gap `GET /schedule`'s `days` works around. `from` defaults to now.
- Evaluated in the configured `log.timezone` (the config default is `"Local"`), and the returned instants carry that zone's offset. Cron fields are wall clock: `0 6 * * *` evaluated anywhere but the zone the channel airs in hands back a different hour than the operator will see. A `from` supplied in some other zone is converted first, so the same instant always yields the same answer.
- **Start instants only.** A caller that wants an end adds the block's own `duration` to each start — that is a property of the block, not of the expression, and taking a duration parameter here would make this endpoint pretend otherwise.
- Occurrences are strictly **after** `from`, so a `from` sitting exactly on an occurrence does not return that occurrence again: the question being asked is what happens *next*. This is deliberately a different convention from the engine's own window walk, which seeds at `from` minus one second precisely so an occurrence landing on the window's start edge **is** planned — a window is inclusive of its edge, "next" is not. Anything lining this endpoint's output up against a generated schedule needs to know which of the two it is holding.
- The array may be **shorter** than `count`, empty included. `0 0 30 2 *` is a well-formed expression that never fires; the parser accepts it and gives up after a five-year search. That is a `200` with `{"occurrences": []}`, not an error — the expression is valid and the answer is genuinely "never". The zero timestamp that search returns is dropped rather than serialized, so no `0001-01-01` ever reaches a client.
- An expression that will not parse is `400` (`title: "invalid cron expression"`), with the parser's own complaint echoed in the detail — it is the operator's own text, so there is nothing to withhold. A missing `expr`, or a `count`/`from` that will not bind to its declared type, is also `400`, but it comes from the generated parameter binding rather than from a handler, so its body is plain text instead of `problem+json`. `expr` is the contract's only required query parameter.

## Schedule

Delegates to the same runner the CLI's `generate`/`generate --apply` uses, so the CLI and the API share one implementation: load the enabled blocks, fetch available Tunarr content, run the scheduling engine over a `days`-wide window starting now, and — only when applying — push the result to Tunarr per channel and commit pending state.

| Method | Path                            | Success | Error codes |
|--------|---------------------------------|---------|-------------|
| POST   | `/generate`                     | 200     | 400, 502    |
| POST   | `/apply`                        | 200     | 400, 502    |
| GET    | `/schedule?days=N&channel_id=…` | 200     | 400, 502    |

- `POST /generate` and `POST /apply` share the same optional `GenerateRequest` body (`days`, `channel_id`); `GET /schedule` takes the same `days` and `channel_id` as query parameters (parity with the POST body — `GET /schedule` is what the [web Guide](web-ui-guide.md#the-guide) reads on open, and `POST /generate` is what its SCOPE control drafts with). `days` defaults to `7` and is range-checked by the handler itself against `[1, 30]`, returning `400` outside that range (oapi-codegen's generated bindings don't enforce the OpenAPI schema's own default/minimum/maximum).
- Every slot's `programs` array carries the typed `ScheduledProgram` shape — `title`, `duration_ms`, and a per-program `start_time` (the slot's start plus the cumulative durations of everything before it in the lineup) always present; `type`, `season`, and `episode` present only when the source program carries them. This replaced the untyped `additionalProperties: true` passthrough outright in v0.5.1 (no dual-shape transition, per the no-legacy policy); the same shape flows through `POST /generate`, `POST /apply`, and `GET /schedule` identically.
- `POST /generate` always runs a dry run (`applied: false` in the response) regardless of the request body — it never mutates the store or Tunarr. Only `POST /apply` (`applied: true` on success) pushes anything.
- `channel_id`, when set, restricts *which blocks get planned at all* — not just which channels appear in the response or get pushed. A channel-scoped `POST /apply` never touches Tunarr, schedule history, or series-cursor state for any other channel.
- A failure (loading blocks, fetching Tunarr content, generating the schedule, or — on apply — pushing/committing) returns `502` (`title: "schedule generation failed"`) with a short, fixed detail; the underlying error is logged server-side only, never echoed in the response body.
- The response's `warnings` array (present, non-empty only when at least one occurrence was dropped) lists every occurrence that was planned a time slot but then lost conflict resolution to an overlapping, higher- (or equal-, first-come-) priority occurrence on the same channel — `block_name`, `occurrence_start`, `blocking_block_name`, plus the `channel_id` both occurrences contended for and the `duration_minutes` the dropped one would have run, for each. Both `POST /generate` and `POST /apply` populate it identically (conflict resolution happens during generation either way, not only on apply); the [Guide's draft mode](web-ui-guide.md#draft-apply) surfaces them as NO SIGNAL ghost slots, drawn at the time each dropped occurrence would have aired.

## History

Lists `schedule_history` rows, ordered by `scheduled_at` descending, scheduled within the last `days` days.

| Method | Path              | Success | Error codes |
|--------|-------------------|---------|-------------|
| GET    | `/history?days=N` | 200     | 400         |

`days` defaults to `7`; the handler applies the default and range-checks against `[1, 90]` itself, returning `400` outside that range. `days` only has data to return as far back as `maintenance.history_retention` allows — see [Scheduling Concepts' history retention section](scheduling-concepts.md#schedule-history-and-retention).

Each entry carries the program's identity and the occurrence it belonged to:

| Field                                    | Meaning                                                                                   |
|------------------------------------------|-------------------------------------------------------------------------------------------|
| `program_id`, `channel_id`, `block_name` | What was scheduled, where, and by which block                                             |
| `scheduled_at`                           | The wall-clock instant planning happened — the value `days` is measured against           |
| `occurrence_start`                       | The block occurrence's own cron-computed start time, **not** `scheduled_at`               |
| `sequence`                               | Playback order within that occurrence                                                     |
| `title`, `type`, `duration_ms`           | Enough to name and size the program without the live Tunarr catalog                       |
| `run_id`                                 | The apply run that committed this row, or `""` for rows written before runs were recorded |

## Apply runs

Lists recorded applies, newest first — one row per apply, from the web UI, the `serve` cron loop, and the CLI alike, each with the conflict warnings that apply dropped.

| Method | Path                      | Success | Error codes |
|--------|---------------------------|---------|-------------|
| GET    | `/applies?days=N&limit=M` | 200     | 400         |

`days` defaults to `7` and is range-checked against `[1, 90]`; `limit` defaults to `100` and is range-checked against `[1, 500]`. Both return `400` outside their range. The window is bounded by `maintenance.apply_run_retention` (default 90 days).

| Field                         | Meaning                                                                                                     |
|-------------------------------|-------------------------------------------------------------------------------------------------------------|
| `id`                          | The run's identifier, also stamped on the `schedule_history` rows it committed                              |
| `started_at`                  | When the apply began — the value `days` is measured against                                                 |
| `finished_at`                 | When it ended, or `null` while in flight                                                                    |
| `source`                      | `ui`, `cron`, `cli`, or `unknown`                                                                           |
| `scope`                       | The channel ID the apply was narrowed to, or `""` for every channel                                         |
| `days`                        | The window the apply planned                                                                                |
| `status`                      | `running`, `ok`, or `error`                                                                                 |
| `channel_count`, `slot_count` | What the apply pushed; both `0` unless it succeeded                                                         |
| `error`                       | The failure detail when `status` is `error`; `""` otherwise                                                 |
| `warnings`                    | Always an array — `block_name`, `occurrence_start`, `blocking_block_name`, `channel_id`, `duration_minutes` |

The row is written **before** the apply pushes anything to Tunarr and finalized afterwards, so a row still reading `running` long after its timestamp means the process died mid-apply: its `finished_at` will never arrive. A failed apply is still a run, carrying its error. Runs are never backfilled: nothing exists from before the migration that created the table.

## Deleting history

Two destructive endpoints and one readout. Both deletions take `dry_run`, and both run the same predicates in preview as they do for real — a preview that counted different rows than the delete removes would be worse than none, because the operator confirms against it.

**Nothing here can be undone.** There is no backfill: the data is gone.

| Method | Path                                  | Success | Error codes |
|--------|---------------------------------------|---------|-------------|
| DELETE | `/state/series/{show_title}?dry_run=` | 200     | 404, 409    |
| DELETE | `/history?…&dry_run=`                 | 200     | 400, 409    |
| GET    | `/storage`                            | 200     | —           |

### Removing one show

`DELETE /state/series/{show_title}` removes the show's cursor, its airings, and its keys inside every occurrence snapshot, in one transaction. A snapshot loses the show's KEY, not the row — a snapshot describes one occurrence of one block, which may have carried several shows.

It is **refused with 409 while any block still lists the show**. The next apply would re-add it from the block spec and reset the cursor the removal just deleted, so the refusal names the blocks to edit first in the problem's `detail`. The web UI reads `GET /blocks` itself and shows such a row as blocked before the operator clicks; the 409 is the backstop for a block added in between.

`404` means no tracked cursor exists for that title.

### Deleting a range

`DELETE /history` takes exactly one window form — `before=<RFC3339>`, or `from=<RFC3339>&to=<RFC3339>` — optionally narrowed by `channel_id`, `block_name`, and `show_title`. Neither form, or both, is a `400`; so is `from` after `to`. Sending no window is never read as "delete everything".

The window is measured on `scheduled_at`, the same column `GET /history?days=N` filters on, so a preview counts the rows the AS-RUN pane is showing.

Cursors and snapshots are untouched: a date range is not a statement about any particular show.

`409` means the range covers an occurrence that is **on air right now**. Deleting the record of what is playing is the one deletion that changes what a viewer sees, because the next unattended apply would re-plan the slot mid-program. The problem names when it becomes safe.

### The empty-slot marker

An occurrence that loses every airing keeps one row with an empty `program_id`. Without it the engine would answer "this occurrence was never planned" and the next apply would re-plan a slot that already went out — rewriting history rather than removing from it. The marker says "committed, produced nothing", which is what actually happened.

Markers are not airings: a range deletion never removes one, so running the same window twice is a no-op, and `GET /storage` counts them separately.

### What is stored

`GET /storage` reports row counts per table plus the span the stored airings cover. It reports what IS stored, never what retention says should be — a database whose retention was widened holds whatever it holds.

## Series state

Lists and patches the per-show `series_state` tracking rows (current season/episode, completion, and the disabled flag the scheduler sets once a non-restarting series runs out of episodes).

| Method | Path                         | Success | Error codes |
|--------|------------------------------|---------|-------------|
| GET    | `/state/series`              | 200     | —           |
| PATCH  | `/state/series/{show_title}` | 200     | 400, 404    |

- `PATCH` applies a partial update: only fields present in the request body (`current_season`, `current_episode`, `completed`, `disabled`) change, and a body with none of them set returns `400`, as does a malformed JSON body.
- `PATCH` returns `404` for a `show_title` with no persisted `series_state` row — the store fabricates nothing for the API.
- A successful `PATCH` also invalidates every not-yet-*finished* occurrence snapshot for every block that schedules `show_title` — including a currently on-air occurrence — so a manual cursor reset takes effect on the very next apply and stays in effect, instead of being shadowed by (or overwritten back to) an already-captured snapshot for up to the schedule-generation window. `schedularr state reset`/`state set` (see the [CLI Reference](cli-reference.md)) do the same invalidation directly against the store. See [Scheduling Concepts' idempotent-apply section](scheduling-concepts.md#idempotent-apply-and-editing-a-block-before-it-airs).

## Channels and status

The Tunarr boundary: `ListChannels` proxies `GET /api/channels` on the configured Tunarr instance; `GetStatus` reports overall service health, probing Tunarr reachability the same way.

| Method | Path        | Success | Error codes |
|--------|-------------|---------|-------------|
| GET    | `/channels` | 200     | 502         |
| GET    | `/status`   | 200     | —           |

- `GET /channels` returns `502` (`title: "tunarr unreachable"`) both when Tunarr isn't configured (`detail: "tunarr not configured"`) and when the configured client's call fails (`detail` carries the wrapped connectivity error).
- `GET /status` never returns a `5xx`. It always responds `200` with `version`, `tunarr_reachable` (a live probe), `tunarr_error` (set whenever `tunarr_reachable` is `false`), `blocks` (the current block count, omitted rather than failing the request if the count itself errors), `last_applied_at` (when the most recent apply pushed at least one lineup to Tunarr — planned pushes and stale-channel clears alike, sampled at push time; omitted when no apply has been recorded), and `next_cron_tick` (when `serve`'s cron loop will next generate and apply; omitted when no cron loop is running). The two timestamp fields feed the web UI's bezel telemetry strip.

## Media discovery

Exposes what Tunarr's synced library actually contains — shows and the distinct genre/rating values observed across it. Both endpoints share a 1h cache with schedule generation: a call that finds the cache already warm issues no Tunarr HTTP requests at all.

| Method | Path           | Success | Error codes |
|--------|----------------|---------|-------------|
| GET    | `/media/shows` | 200     | 502         |
| GET    | `/media/meta`  | 200     | 502         |

`GET /media/shows` returns `[{title, episode_count}]`, one entry per distinct show, sorted by title. `GET /media/meta` returns `{genres, ratings}`: the distinct, sorted values seen across every fetched program. Both return `502` under the same conditions as `GET /channels`. An empty library with Tunarr reachable is a normal `200` with empty arrays, not an error.

!!! note "Live Tunarr's episode shape"
    A live Tunarr `/api/programs/search` "episode" result never sends a flat `showTitle`/`rating`/`seasonNumber` key, and doesn't nest a `show` object either (live-verified against Tunarr 1.3.13). What it actually carries is a `showId` foreign key pointing at a separate, interleaved `Type == "show"` search-result entry — not nested, and not reliably on the same page as its own episodes. Schedularr's fetch path joins each episode's `showId` against those interleaved show entries after accumulating the *entire* paginated result set, and resolves each distinct `seasonId` individually via `GET /api/programming/seasons/{id}` (cached for the same 1h window). This is what makes `/media/shows`, `/media/meta`'s `ratings`, and series-block scheduling actually work against a real, unmodified Tunarr deployment.

## Live link

One Server-Sent Events stream carries every change the server makes to state a browser tab is already showing, so a tab learns about another tab's edit without polling for it.

| Method | Path      | Success | Error codes |
|--------|-----------|---------|-------------|
| GET    | `/events` | 200     | 500, 503    |

The response is `text/event-stream` and never completes while the client stays connected. It sets `Cache-Control: no-cache`, `Connection: keep-alive`, and `X-Accel-Buffering: no` — the last of which matters to anything fronting Schedularr (see [Deployment](deployment.md#reverse-proxies-and-the-event-stream)).

**Frames.** Each frame is `event:` plus a JSON `data:` line, and — for everything except heartbeats — an `id:`:

| Event              | Payload                                                                | Published when                                                      |
|--------------------|------------------------------------------------------------------------|---------------------------------------------------------------------|
| `heartbeat`        | `{server_time}`                                                        | Immediately on connect, then every 15s                              |
| `apply.completed`  | `{run_id, source, channel_ids, slot_count, warning_count, applied_at}` | An apply finishes — success **or** failure                          |
| `plan.invalidated` | `{reason, id}` where `reason` is `block` or `series`                   | A block is created, updated, patched, or deleted; a cursor is reset |
| `series.changed`   | `{show_title}`                                                         | `PATCH /state/series/{show_title}` succeeds                         |
| `status.changed`   | `{tunarr_reachable}`                                                   | Tunarr's reachability flips (see below)                             |

`heartbeat` does two jobs: `server_time` (RFC 3339, UTC, nanosecond precision) lets a client correct its own clock drift rather than trusting `Date.now()` for relative timestamps, and the traffic itself stops an intermediary from buffering the connection into uselessness. Heartbeats deliberately carry **no** `id` — they are not resumable state, and giving them ids would make a reconnect replay clock ticks.

`status.changed` fires only on a **transition**, never once per probe. `serve` probes Tunarr every 30s (5s timeout) and publishes only when the answer differs from the last one, plus once on the first probe so a tab that connects before any flip still learns the current reading. A per-probe publish would wake every connected tab twice a minute to tell it nothing had happened. This prober exists because nothing else server-side notices Tunarr going away: between applies, the only Tunarr calls are ones a browser triggers.

There is no `history.appended` event. It would fire at the same instant, from the same place, carrying the same `run_id` as `apply.completed`, and two events for one fact is a catalog that lies about its own granularity.

**Resume.** Send the `Last-Event-ID: <n>` header to receive everything published after id `n`. An unparseable or negative value starts a fresh subscription rather than failing the request — the client refetches its page data on connect anyway, and refusing the connection would strand a tab whose only problem was a corrupt header. The hub retains the 128 most recent events for this, which is sized for a reconnect measured in seconds, not for replaying a session — a client that has been away longer simply misses events and repairs itself by refetching the page's primary `GET`. Each connection also buffers 32 events; a tab that stops reading past that loses events rather than slowing a publisher down, and resumes the same way.

**Errors.** `503` (`title: "live link unavailable"`) means the server was started without an event hub — every `serve` instance wires one, so in practice this is the CLI's own handlers or an embedding that omits it. `500` with the same title means the response writer cannot stream, which is checked *before* any header is written so an unstreamable writer still gets a proper `problem+json` response instead of a committed, empty `200`.

The stream is consumed by a hand-rolled `fetch`/`ReadableStream` reader, never by `EventSource`, which cannot send an `Authorization` header. Passing the token in the query string instead is permanently out of scope: it would leak the token into access logs.

**Nothing depends on the stream.** Every page stays fully operable with a refresh when it is unavailable; the web UI treats a dead stream as a normal state and falls back to polling. Delivery is deliberately best-effort in both directions: publishing happens on an apply's critical path and never blocks, never errors, and never waits on a reader.

## Middleware

Every `/api/v1/*` route runs through:

- **Request ID** — a fresh identifier per request (an inbound `X-Request-Id` header is never trusted), returned as `X-Request-Id` and included in every `problem+json` body.
- **Logging** — one structured line per request: `method`, `path`, `status`, `duration_ms`, `request_id`.
- **Recovery** — turns a handler panic into a `500` `problem+json` response, logged with a stack trace, instead of crashing the process.
- **Bearer auth** — requires `Authorization: Bearer <token>` on protected routes. The token is compared as a SHA-256 digest via constant-time comparison; a token under 32 characters is rejected when the middleware is constructed. A missing or wrong token gets `401`.

`schedularr serve`'s own system endpoints — `/healthz`, `/readyz`, `/metrics`, `/openapi.json` — are not part of the OpenAPI contract and sit outside this middleware chain entirely. See the [CLI Reference's `serve` section](cli-reference.md#serve) for those.
