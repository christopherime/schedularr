# API Reference

Schedularr exposes an HTTP API defined by an OpenAPI 3.0.3 contract at [`api/openapi.yaml`](https://github.com/christopherime/schedularr/blob/main/api/openapi.yaml) in the repository. The contract covers blocks CRUD, block import/export, schedule generation and application, history, series state, channels, and status.

!!! note "The live contract"
    A running `schedularr serve` instance serves the same contract as JSON at `GET /openapi.json` (unauthenticated) — that endpoint is the source of truth for whatever version you're actually running. The tables below describe the contract as of this page's writing; `/openapi.json` never drifts from your binary.

Server code is generated from the contract with [oapi-codegen](https://github.com/oapi-codegen/oapi-codegen) v2 (`make generate`), writing `internal/api/gen/server.gen.go` — committed, and not hand-edited. Handlers live in `internal/api/` and implement the generated `gen.ServerInterface`. Errors use an RFC 7807 `application/problem+json` body.

The API is served by `schedularr serve` — see the [CLI Reference](cli-reference.md#serve) for flags and startup behavior.

## Blocks

Every write path (`POST`/`PUT`) validates the block spec against the CUE scheduler schema before touching the store; every response body is `application/json` (or `application/problem+json` for errors).

| Method | Path | Success | Error codes |
| --- | --- | --- | --- |
| GET | `/blocks` | 200 | — |
| POST | `/blocks` | 201 | 400, 409 |
| GET | `/blocks/{id}` | 200 | 404 |
| PUT | `/blocks/{id}` | 200 | 400, 404, 409 |
| DELETE | `/blocks/{id}` | 204 | 404 |

- `POST`/`PUT` return `400` for a spec that fails CUE validation (e.g. a missing `cron` or a non-positive `duration`) or a malformed JSON body.
- `POST` returns `409` for a duplicate block name; `PUT` returns `409` if the request body's `spec.name` differs from the existing block's name and collides with another block. A `PUT` whose `spec.name` differs from the current name without colliding renames the block.
- `POST`/`PUT` also return `400` for a series block (`spec.type: series`) whose `series[].show_title` is empty. The CUE scheduler schema types `show_title` as a bare `string` with no non-empty constraint, so this case would otherwise pass CUE validation — the check is applied in Go on both block-ingestion paths (here, and `blocks/import` below).
- `PUT`/`DELETE` on a series block also invalidate every not-yet-*finished* occurrence's cursor snapshot for that block — including one currently on air, not just occurrences that haven't started yet (see [Scheduling Concepts' idempotent-apply section](scheduling-concepts.md#idempotent-apply-and-editing-a-block-before-it-airs)) — so the next apply re-derives those occurrences against the spec you just changed instead of a snapshot captured under the old one.

## Import / export

Round-trips the same sqlite-backed blocks through the same YAML parse/render used for the on-disk `scheduler.yaml` bootstrap path. Both endpoints exchange raw YAML text, not JSON.

| Method | Path | Success | Error codes |
| --- | --- | --- | --- |
| POST | `/blocks/import` | 200 | 400, 409, 413 |
| GET | `/blocks/export` | 200 | — |

- `POST /blocks/import` takes an `application/yaml` body (capped at 1MiB; an oversized body gets `413`) and an optional `?dry_run=true` query parameter (default `false`). The body is strictly decoded and CUE-validated, including duplicate block-name and empty-`show_title` rejection — any failure is `400` with the CUE detail included. Every parsed block's name is checked against every block already in the store; any collision is `409` (listing the colliding name(s)) with **zero writes**, even for the non-colliding blocks in the same batch. `dry_run=true` stops after that check and reports what would have been imported (`{imported, dry_run, names}`) without writing anything; otherwise every block is created with a fresh UUID and `enabled: true`.
- `GET /blocks/export` renders every stored block's spec as YAML — **including disabled blocks**, since export doubles as a backup mechanism. The response has no `enabled` state of its own (that lives on the store record); re-importing an exported file creates every block as enabled.

## Schedule

Delegates to the same runner the CLI's `generate`/`generate --apply` uses, so the CLI and the API share one implementation: load the enabled blocks, fetch available Tunarr content, run the scheduling engine over a `days`-wide window starting now, and — only when applying — push the result to Tunarr per channel and commit pending state.

| Method | Path | Success | Error codes |
| --- | --- | --- | --- |
| POST | `/generate` | 200 | 400, 502 |
| POST | `/apply` | 200 | 400, 502 |
| GET | `/schedule?days=N` | 200 | 400, 502 |

- `POST /generate` and `POST /apply` share the same optional `GenerateRequest` body (`days`, `channel_id`); `GET /schedule` takes the same `days` as a query parameter. `days` defaults to `7` and is range-checked by the handler itself against `[1, 30]`, returning `400` outside that range (oapi-codegen's generated bindings don't enforce the OpenAPI schema's own default/minimum/maximum).
- `POST /generate` always runs a dry run (`applied: false` in the response) regardless of the request body — it never mutates the store or Tunarr. Only `POST /apply` (`applied: true` on success) pushes anything.
- `channel_id`, when set, restricts *which blocks get planned at all* — not just which channels appear in the response or get pushed. A channel-scoped `POST /apply` never touches Tunarr, schedule history, or series-cursor state for any other channel.
- A failure (loading blocks, fetching Tunarr content, generating the schedule, or — on apply — pushing/committing) returns `502` (`title: "schedule generation failed"`) with a short, fixed detail; the underlying error is logged server-side only, never echoed in the response body.
- The response's `warnings` array (present, non-empty only when at least one occurrence was dropped) lists every occurrence that was planned a time slot but then lost conflict resolution to an overlapping, higher- (or equal-, first-come-) priority occurrence on the same channel — `block_name`, `occurrence_start`, and `blocking_block_name` for each. Both `POST /generate` and `POST /apply` populate it identically (conflict resolution happens during generation either way, not only on apply); the [Web UI's Schedule page](web-ui-guide.md#schedule-schedule) surfaces it after every preview or apply.

## History

Lists `schedule_history` rows, ordered by `scheduled_at` descending, scheduled within the last `days` days.

| Method | Path | Success | Error codes |
| --- | --- | --- | --- |
| GET | `/history?days=N` | 200 | 400 |

`days` defaults to `7`; the handler applies the default and range-checks against `[1, 90]` itself, returning `400` outside that range. `days` only has data to return as far back as `maintenance.history_retention` allows — see [Scheduling Concepts' history retention section](scheduling-concepts.md#schedule-history-and-retention).

## Series state

Lists and patches the per-show `series_state` tracking rows (current season/episode, completion, and the disabled flag the scheduler sets once a non-restarting series runs out of episodes).

| Method | Path | Success | Error codes |
| --- | --- | --- | --- |
| GET | `/state/series` | 200 | — |
| PATCH | `/state/series/{show_title}` | 200 | 400, 404 |

- `PATCH` applies a partial update: only fields present in the request body (`current_season`, `current_episode`, `completed`, `disabled`) change, and a body with none of them set returns `400`, as does a malformed JSON body.
- `PATCH` returns `404` for a `show_title` with no persisted `series_state` row — the store fabricates nothing for the API.
- A successful `PATCH` also invalidates every not-yet-*finished* occurrence snapshot for every block that schedules `show_title` — including a currently on-air occurrence — so a manual cursor reset takes effect on the very next apply and stays in effect, instead of being shadowed by (or overwritten back to) an already-captured snapshot for up to the schedule-generation window. `schedularr state reset`/`state set` (see the [CLI Reference](cli-reference.md)) do the same invalidation directly against the store. See [Scheduling Concepts' idempotent-apply section](scheduling-concepts.md#idempotent-apply-and-editing-a-block-before-it-airs).

## Channels and status

The Tunarr boundary: `ListChannels` proxies `GET /api/channels` on the configured Tunarr instance; `GetStatus` reports overall service health, probing Tunarr reachability the same way.

| Method | Path | Success | Error codes |
| --- | --- | --- | --- |
| GET | `/channels` | 200 | 502 |
| GET | `/status` | 200 | — |

- `GET /channels` returns `502` (`title: "tunarr unreachable"`) both when Tunarr isn't configured (`detail: "tunarr not configured"`) and when the configured client's call fails (`detail` carries the wrapped connectivity error).
- `GET /status` never returns a `5xx`. It always responds `200` with `version`, `tunarr_reachable` (a live probe), `tunarr_error` (set whenever `tunarr_reachable` is `false`), `blocks` (the current block count, omitted rather than failing the request if the count itself errors), `last_applied_at` (when the most recent apply pushed at least one lineup to Tunarr — planned pushes and stale-channel clears alike, sampled at push time; omitted when no apply has been recorded), and `next_cron_tick` (when `serve`'s cron loop will next generate and apply; omitted when no cron loop is running). The two timestamp fields feed the web UI's bezel telemetry strip.

## Media discovery

Exposes what Tunarr's synced library actually contains — shows and the distinct genre/rating values observed across it. Both endpoints share a 1h cache with schedule generation: a call that finds the cache already warm issues no Tunarr HTTP requests at all.

| Method | Path | Success | Error codes |
| --- | --- | --- | --- |
| GET | `/media/shows` | 200 | 502 |
| GET | `/media/meta` | 200 | 502 |

`GET /media/shows` returns `[{title, episode_count}]`, one entry per distinct show, sorted by title. `GET /media/meta` returns `{genres, ratings}`: the distinct, sorted values seen across every fetched program. Both return `502` under the same conditions as `GET /channels`. An empty library with Tunarr reachable is a normal `200` with empty arrays, not an error.

!!! note "Live Tunarr's episode shape"
    A live Tunarr `/api/programs/search` "episode" result never sends a flat `showTitle`/`rating`/`seasonNumber` key, and doesn't nest a `show` object either (live-verified against Tunarr 1.3.13). What it actually carries is a `showId` foreign key pointing at a separate, interleaved `Type == "show"` search-result entry — not nested, and not reliably on the same page as its own episodes. Schedularr's fetch path joins each episode's `showId` against those interleaved show entries after accumulating the *entire* paginated result set, and resolves each distinct `seasonId` individually via `GET /api/programming/seasons/{id}` (cached for the same 1h window). This is what makes `/media/shows`, `/media/meta`'s `ratings`, and series-block scheduling actually work against a real, unmodified Tunarr deployment.

## Middleware

Every `/api/v1/*` route runs through:

- **Request ID** — a fresh identifier per request (an inbound `X-Request-Id` header is never trusted), returned as `X-Request-Id` and included in every `problem+json` body.
- **Logging** — one structured line per request: `method`, `path`, `status`, `duration_ms`, `request_id`.
- **Recovery** — turns a handler panic into a `500` `problem+json` response, logged with a stack trace, instead of crashing the process.
- **Bearer auth** — requires `Authorization: Bearer <token>` on protected routes. The token is compared as a SHA-256 digest via constant-time comparison; a token under 32 characters is rejected when the middleware is constructed. A missing or wrong token gets `401`.

`schedularr serve`'s own system endpoints — `/healthz`, `/readyz`, `/metrics`, `/openapi.json` — are not part of the OpenAPI contract and sit outside this middleware chain entirely. See the [CLI Reference's `serve` section](cli-reference.md#serve) for those.
