# API Reference

Schedularr exposes an HTTP API defined by an OpenAPI 3.0.3 contract at [`api/openapi.yaml`](https://github.com/christopherime/schedularr/blob/main/api/openapi.yaml) in the repository. The contract covers blocks CRUD, block import/export, schedule generation and application, history, series state, channels, and status.

!!! note "The live contract"
    A running `schedularr serve` instance serves the same contract as JSON at `GET /openapi.json` (unauthenticated) — that endpoint is the source of truth for whatever version you're actually running. The tables below describe the contract as of this page's writing; `/openapi.json` never drifts from your binary.

Server code is generated from the contract with [oapi-codegen](https://github.com/oapi-codegen/oapi-codegen) v2 (`make generate`), writing `internal/api/gen/server.gen.go` — committed, and not hand-edited. Handlers live in `internal/api/` and implement the generated `gen.ServerInterface`. Errors use an RFC 7807 `application/problem+json` body.

The API is served by `schedularr serve` — see the [CLI Reference](cli-reference.md#serve) for flags and startup behavior.

## Blocks

Every write path (`POST`/`PUT`) validates the block spec against the CUE scheduler schema before touching the store; every response body is `application/json` (or `application/problem+json` for errors).

| Method | Path           | Success | Error codes        |
|--------|----------------|---------|--------------------|
| GET    | `/blocks`      | 200     | —                  |
| POST   | `/blocks`      | 201     | 400, 409           |
| GET    | `/blocks/{id}` | 200     | 404                |
| PUT    | `/blocks/{id}` | 200     | 400, 404, 409, 412 |
| PATCH  | `/blocks/{id}` | 200     | 400, 404, 409      |
| DELETE | `/blocks/{id}` | 204     | 404                |

- `POST`/`PUT` return `400` for a spec that fails CUE validation (e.g. a missing `cron` or a non-positive `duration`) or a malformed JSON body.
- `POST` returns `409` for a duplicate block name; `PUT` returns `409` if the request body's `spec.name` differs from the existing block's name and collides with another block. A `PUT` whose `spec.name` differs from the current name without colliding renames the block.
- `POST`/`PUT` also return `400` for a series block (`spec.type: series`) whose `series[].show_title` is empty. The CUE scheduler schema types `show_title` as a bare `string` with no non-empty constraint, so this case would otherwise pass CUE validation — the check is applied in Go on both block-ingestion paths (here, and `blocks/import` below).
- `PUT` requires an `If-Match` header carrying the block's current `updated_at`, which you read from a prior `GET`. A missing or unparseable header is `400`; a value that no longer matches the stored `updated_at` is `412` (`title: "block changed since you loaded it"`). This is lost-update protection: `PUT` replaces a block's entire spec, so without it two operators editing one block silently discard the slower one's work. The value is compared as an instant rather than as a string — a client that round-trips `updated_at` through JSON may re-render it equivalently but not identically — and surrounding quotes are accepted, since callers reasonably treat it as an entity tag. The correct response to a `412` is to reload the block and re-apply the edit, never a silent retry.
- `PATCH /blocks/{id}` is the field-scoped complement to `PUT`: only fields present in the body change (`enabled` is the sole field today), so an enable/disable toggle cannot clobber a spec edit made elsewhere. It takes **no** `If-Match` for that reason — there is no unrelated state for it to overwrite. A body with no fields set is `400` (`title: "empty patch"`), matching `PATCH /state/series/{show_title}`. Re-enabling a block re-enters the same shared-show policy check a create or full update runs, so a `409` is still possible; disabling never triggers one.
- `PUT`/`DELETE` on a series block also invalidate every not-yet-*finished* occurrence's cursor snapshot for that block — including one currently on air, not just occurrences that haven't started yet (see [Scheduling Concepts' idempotent-apply section](scheduling-concepts.md#idempotent-apply-and-editing-a-block-before-it-airs)) — so the next apply re-derives those occurrences against the spec you just changed instead of a snapshot captured under the old one.

## Import / export

Round-trips the same sqlite-backed blocks through the same YAML parse/render used for the on-disk `scheduler.yaml` bootstrap path. Both endpoints exchange raw YAML text, not JSON.

| Method | Path             | Success | Error codes   |
|--------|------------------|---------|---------------|
| POST   | `/blocks/import` | 200     | 400, 409, 413 |
| GET    | `/blocks/export` | 200     | —             |

- `POST /blocks/import` takes an `application/yaml` body (capped at 1MiB; an oversized body gets `413`) and an optional `?dry_run=true` query parameter (default `false`). The body is strictly decoded and CUE-validated, including duplicate block-name and empty-`show_title` rejection — any failure is `400` with the CUE detail included. Every parsed block's name is checked against every block already in the store; any collision is `409` (listing the colliding name(s)) with **zero writes**, even for the non-colliding blocks in the same batch. `dry_run=true` stops after that check and reports what would have been imported (`{imported, dry_run, names}`) without writing anything; otherwise every block is created with a fresh UUID and `enabled: true`.
- `GET /blocks/export` renders every stored block's spec as YAML — **including disabled blocks**, since export doubles as a backup mechanism. The response has no `enabled` state of its own (that lives on the store record); re-importing an exported file creates every block as enabled.

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
