# Roadmap to v1.0.0

What still has to be true before Schedularr calls itself 1.0, grouped into
the minor releases that get there. Pre-1.0 semver applies: a minor may
change behavior or contracts freely (each change documented in the
CHANGELOG, per the no-legacy policy — superseded behavior is deleted, not
aliased); patches are fixes only. **v1.0.0 is a stability promise, not a
feature**: after it, `/api/v1`, the config schema, and the SQLite
migration chain only change additively.

Status: living document — reorder, drop, or promote items as reality
dictates. Last shaped: 2026-08-30 (at v0.5.4), when three operator-directed
product streams were slotted into the ladder — station-lingo terminology,
any-media scheduling, and `/series` becoming `/history`. Their design lives
in the v1.0 product-intake spec
(`docs/superpowers/specs/2026-08-30-v1-station-terminology-media-history-design.md`),
which also carries the Open Questions those streams still owe an answer to.

## Where things stand (v0.2.x)

Shipped and live-verified: cron-driven blocks (filter + series) in SQLite;
idempotent apply with per-occurrence snapshots and provenance-ordered
replay; conflict resolution with visible warnings; seed-preserving block
edits (reorder = same episodes, new order); stale-channel clearing;
anchored flex-padded lineups matching Tunarr v1.3.13's wall-clock model;
Hugo/Alpine web UI embedded in the binary; token auth; OpenAPI-first API;
CI with codegen-drift, race tests, and a committed web unit harness;
Docker/Helm/ArgoCD deployment.

## v0.3.0 — Scheduling correctness at full width

The engine is right for one channel and cooperative blocks; make it right
for every configuration the API already accepts.

- **Channel-scoped series state.** `series_state` is keyed by `show_title`
  alone; two blocks on different channels tracking the same show collide
  on one cursor. Re-key to `(channel_id, show_title)` with a migration
  that adopts existing rows. (TODO.md, parked at SP1 close-out.) If this
  is still open when v0.6.1 lands, do it there instead — that slice
  re-keys the same table anyway, and migrating it twice buys nothing.
- **Seed-preserving cron edits.** Changing a block's `cron` today moves
  its occurrences off their snapshot keys, so the moved occurrence
  replans from the live cursor (documented operator caveat). Re-key
  not-yet-aired occurrence snapshots to the new occurrence starts instead,
  killing the "never change cron while an occurrence is committed" rule.
- ~~**Contradictory `on_complete` validation.**~~ Done (first v0.3.0
  slice): every write path rejects disagreeing shared-show policies with
  a 400 naming the conflict.
- **Residue hardening from the v0.2.2 gate:** backward-CAS coincidence
  (a wrap-lap landing exactly on a frozen baseline) — remaining; ~~pin
  the `max()` provenance guard with a test~~ and ~~bounded `cronDone`
  shutdown wait~~ done (first v0.3.0 slice).
- ~~**Added-series caveat.**~~ Done (first v0.3.0 slice): a series added
  to a block with a pending seeded occurrence now plans from its live
  cursor instead of re-airing S01E01.

## v0.4.0 — Metadata engine, and any-media criteria (sub-project 3)

The long-planned third sub-project: enrich what the filters and the guide
can see. The operator's "schedule any Tunarr media kind" directive
(2026-08-30) converges here rather than standing alone — "every Tuesday
20:00, a movie tagged Funny" is a metadata question, a tag question, and a
media-kind question, and all three land in this theme. What it takes,
verified against the code and detailed in the v1.0 product-intake spec:
movies already reach a filter block's candidate pool today
(`Runner.fetchPrograms` pulls every library, `matchesFilter` never looks at
`Program.Type`), so the gaps are the criteria, not the content — no
media-kind criterion, `filter.tags` accepted everywhere and evaluated
nowhere, raw rather than normalized genres, and no `/media/movies` for a
picker to call. Ordered slices: enrichment wiring → normalized genres and
ratings in the engine → tags (including the `matchesFilter` branch that is
missing today) → media-kind criterion + `/media/movies`. The ordered movie
*sequence* is a separate concept and lands at v0.6.1, after the rename.

- **TMDB/TVDB clients** (no IMDb — no official free API) with on-disk
  cache and rate-limit respect; API keys via env/secret, never in config
  files. ~~Provider interface, TMDB v3 client, TheTVDB v4 client~~ done
  (first v0.4.0 slice, `internal/metadata` — see
  [Metadata Engine](metadata.md)): keys arrive through the constructors
  so a caller can source them from env/secret, and retry with
  429-backoff rides on the house HTTP client. Remaining: the on-disk
  cache (only TMDB's genre table is cached today, in memory), the
  wiring that calls a provider at all, and a **movie lookup** —
  `Provider` is show-only (`LookupShow`), and both clients implement TV
  routes only, so nothing can enrich a movie yet.
- **Genre/rating normalization.** Close the genre-filter gap: Tunarr's
  library genres are inconsistent across sources; normalize to one
  vocabulary so `filter.genres` matches what operators expect. ~~The
  genre half~~ done (first v0.4.0 slice): a 23-name canonical vocabulary
  plus a mapping table covering the TMDB and TheTVDB spellings
  (`metadata.NormalizeGenres`). Remaining: rating normalization, and
  feeding normalized genres into the engine's filter — the gap an
  operator feels stays open until that lands.
- **Tag engine.** Operator-defined tags assignable per show/movie,
  persisted in the store, editable in the UI. Sharper than the previous
  wording: `filter.tags` is not merely unpopulated — it is never *read*.
  It round-trips through the OpenAPI `Filter` schema, the CUE schema, and
  `internal/api/blocks.go`, and `matchesFilter` has no branch for it, so
  a block with `tags:` set behaves exactly as if the field were absent.
  `docs/scheduling-concepts.md` currently documents it as working
  AND-logic; that line is a factual error until this ships.
- **Media-kind criterion.** `Program.Type` (`movie`, `episode`, …) is
  never consulted, so "movies only" can only be approximated with a
  duration floor. Add `kinds` to the criteria object, and a
  `GET /media/movies` counterpart to `/media/shows` (which groups
  episodes only) so the UI can offer a movie picker.
- **Richer filters.** Decade shortcuts, watched/unwatched via history,
  exclude-lists.
- **EPG enrichment.** Feed artwork/descriptions through to the XMLTV
  guide where Tunarr allows it, so Jellyfin's guide looks like real TV.

## v0.5.0 — The web interface, rebuilt

The operator's directive (2026-08-30): this release is an overall,
extreme, major improvement of the web interface. The committed visual
world (CRT signal-bench, web/DESIGN.md) stays — the experience around it
is rebuilt. Detailed scope lives in the v0.5.0 UI spec
(docs/superpowers/specs/2026-08-30-v0.5-web-overhaul-design.md), cut
into independently releasable slices — v0.5.0–v0.5.11 after four
insertions (three operator-directed, one security patch), of which the
spec's own renumber note records only the first two (see v0.5.4 and
v0.5.5 below).

**Progress:**

- **v0.5.0 — Bench rebuild: SHIPPED (2026-08-30).** The foundation
  slice: `Status.last_applied_at` + `next_cron_tick` on the contract;
  the shared TS runtime (one bundle per page, single `ApiError`
  identity, request timeouts, mutation entry guards); the `ui/*` Hugo
  partials (skeleton/problem/empty/toggle/channel-select/plate/icon/
  confirm/tape/page-js); the CSS component-state floor (z-scale and
  surface tokens, input invalid/focus treatment, button busy shimmer,
  hover guards, forced-colors fallbacks, one field system); channel
  legend plates replacing raw UUIDs; token validate-on-save with
  re-auth re-fires; the polled bezel telemetry strip; the dev-only
  `/kit/` review gallery; and the grown web test harness.
- **v0.5.1 — The Guide, read-only: SHIPPED (2026-08-30).** The typed
  `ScheduledProgram` hard swap + `channel_id` on `GET /schedule`; the
  EPG grid as home (auto-loading, day tabs, graticule-coupled ruler,
  sweep cursor on a local minute timer, NO SIGNAL ghost slots, slot
  inspector rail/bottom-sheet with the typed program rundown, keyboard
  grid navigation, grid skeleton / NO SIGNAL / teaching empty states,
  mobile vertical rundown); nav became GUIDE · BLOCKS · SCHEDULE ·
  SERIES, with the old dashboard surviving unlinked at `/dashboard/`
  until the Log absorbs it. The old Schedule page still owns
  preview/apply.
- **v0.5.2 — Live-UI polish + Guide week-pager reframe: SHIPPED
  (2026-08-30).** Inserted by operator directive after the live v0.5.1
  audit: the Dockerfile ships `web/static` (broken brand image/favicon
  fixed), one left rail for the guide, the `‹`/`›` week pager replacing
  the flat day-tab strip (spec §3.1 amended), series slot faces listing
  their programs, the quiet ground under short grids, and the
  mobile/series/schedule/blocks polish items. Draft & apply and every
  later slice shift one number down the ladder.
- **v0.5.3 — The Guide becomes a full-week grid: SHIPPED
  (2026-08-30).** Second operator-directed insertion (spec §3.1 amended
  again): each channel renders seven consecutive days as one continuous
  timeline (seven per-day 288-quantum segments per track — the spike's
  grid stays the building block); a two-tier sticky ruler (day headers
  over hour cells, month corner, stronger midnight rules); slots
  crossing midnight join flush inside the week (dashed cuts only at the
  week's outer edges, the face on the wider piece); navigation reduced
  to the `‹`/`›` week pager with a calendar-range label (day tabs and
  the DAYS control deleted — the client fetches `days=28` once and
  pages four weeks client-side); the `/schedule` call moved to a 90s
  timeout tier with an honest first-load note; week-relative sweep +
  auto-scroll; the mobile rundown paged by the same pager; keyboard nav
  across the segmented week. Draft & apply and every later slice shift
  one more number down the ladder.
- **v0.5.4 — Theme-fit brand mark: SHIPPED (2026-08-30).** Third
  operator-directed insertion, and the one this roadmap missed at the
  time: the header logo, favicon, and touch icon rebuilt as transparent
  duotone renders that sit in the signal-bench palette. It took the
  v0.5.4 slot the spec's second renumber note had assigned to draft &
  apply, so **every pending slice below shifts one more number down**
  (draft & apply → v0.5.5, memory → v0.5.6, live link → v0.5.7, block
  power tools → v0.5.8, the desk → v0.5.9, polish → v0.5.10). The spec's
  §9 renumber note and `web/layouts/partials/nav.html`'s comments still
  carry the pre-shift numbers and are corrected by the slice that next
  touches them.
- **v0.5.5 — Token encrypted at rest: SHIPPED (2026-08-30).** Fourth
  insertion, this one a security patch rather than an operator-directed
  feature: GitHub code-scanning alert #2 (CodeQL
  `js/clear-text-storage-of-sensitive-data`) — the UI stored the API
  bearer token as plaintext in `localStorage`. Now AES-GCM-256 under a
  non-extractable WebCrypto key held in IndexedDB; `localStorage` keeps
  only `{iv, ciphertext}` (`schedularr_api_token_v2`), the plaintext
  entry migrated and deleted on first read, memory-only degradation when
  crypto is unavailable. A security patch is its own theme, so every
  pending slice below shifts one more number down (draft & apply →
  v0.5.6, memory/`/history/` → v0.5.7, live link → v0.5.8, block power
  tools → v0.5.9, the desk → v0.5.10, polish → v0.5.11).
- **v0.5.6 — Draft & apply on the Guide: pending.** Unchanged in scope.
- **v0.5.7 — Memory, landing as `/history/`: pending.** The spec's Memory
  slice, amended by the operator's `/series` → `/history` directive: the
  apply-run persistence migration, `GET /applies`, the enriched
  `HistoryEntry` and extended `Warning` — but the page ships as
  `/history/` rather than `/log/`, with three panes behind one filter
  toolbar (TRACKED sequences, AS-RUN airings, apply RUNS). `/series/` and
  `/dashboard/` are both deleted here; nav becomes `GUIDE · BLOCKS ·
  HISTORY`. The industry models this as one record in two states — a
  traffic log of what is scheduled, an as-run log of what aired — which
  is the operator's "one searchable place" verbatim.
- **v0.5.8 — Live link (SSE): pending.** Unchanged in scope.
- **v0.5.9 — Block power tools: pending.** Unchanged in scope.
- **v0.5.10 — History desk power tools: pending.** Was "the series desk";
  now the desk lives on `/history/`. Bulk cursor operations, YAML
  import/export, and the two new operator asks: **remove a show or movie
  from history entirely** (its sequence state, its snapshots, and its
  airings) and **range cleanup** (delete entries before a date or within
  a range), behind a STORAGE strip that shows what is actually stored.
  Engine-first prerequisites, both from the intake spec's invariant
  audit: the plan-sequence allocator floor must move from a derived
  `MAX(...)` into `app_meta` so a deletion cannot lower it, and the
  deletion path must be one transaction across state, snapshots, and
  history. A removal that intersects a currently-on-air occurrence is the
  one case that changes what is playing right now — the spec proposes
  refusing it.
- **v0.5.11 — Polish pass: pending.** Unchanged in scope.

Headline surfaces across the train:

- **EPG week-grid timeline** — the TV mental model: planned occurrences
  across channels on a real programme guide, not a table.
- **Warnings and apply history in the UI.** Conflict warnings persisted
  with schedule history so "why didn't X air last Tuesday" is answerable
  after the fact, surfaced where the operator looks — on `/history/`,
  alongside what is scheduled and what aired.
- **Live data** — the no-WebSocket/SSE v1 non-goal is retired; pages
  reflect applies, cron ticks, and Tunarr signal without manual reload.
- **Complete component states** — skeleton loading, teaching empty
  states, full hover/focus/active/disabled/error vocabulary everywhere.
- **Block duplication and disable-until**; **bulk series operations**
  (reset/advance several cursors, import/export from the UI).
- **Mobile pass** — structural responsiveness for the guide-shaped pages.
- **Motion with purpose** — 150–250ms state-conveying transitions, and
  the few earned moments of signal-bench delight.

## v0.6.0 — Station terminology (breaking rename)

The operator's terminology directive (2026-08-30), landed as its own
minor because it is a pure breaking change across the wire, the config
schema, the `scheduler.yaml` import format, and the DB — it deserves its
own CHANGELOG headline rather than hiding inside a feature slice. It must
precede v0.9.0's freeze, and it precedes v0.6.1 so the generalized
sequence concept is authored once under its final name.

- **"Block" stays.** Researched against broadcast sources and validated:
  block programming is standard US and UK vocabulary for a themed,
  multi-programme, multi-hour grouping, which is exactly what a block
  occurrence produces. The industry's block is the grouping on air and
  Schedularr's `Block` is the recurring rule that generates it; the code
  already calls the dated instance an *occurrence*, which keeps those
  apart.
- **`type: "series"` → `type: "sequence"`** (operator-chosen, fixed).
  Aligns with Tunarr's own "sequential" slot mode and its shared
  iterator, which is the same mechanic as Schedularr's cursor.
- **`type: "filter"` → a criteria-programming word.** "Filter" names a
  mechanism, not programming. Candidates are ranked in the intake spec
  (`rotation` recommended, from radio automation's category/rotation
  vocabulary); the operator picks before this slice starts.
- **Blast radius, all in one change** (no aliases, no dual acceptance,
  per the no-legacy policy): the OpenAPI schemas and both generated
  artifacts, the CUE schemas, ~400 Go identifiers, the `series_state` and
  `series_occurrence_snapshots` tables and their indexes, a **data**
  migration rewriting every `blocks.spec_json` row's keys, the
  `scheduler.yaml` import format with its fixtures and the `scheduler
  init` template, the web UI copy, and the docs set.

## v0.6.1 — Media sequences

The second half of the operator's any-media directive: an ordered list of
titles that advances one step per occurrence ("Movie A this week, Movie B
next", monthly runs included). Designed as a **variant of the Sequence
concept**, not a third block type — same `on_complete` vocabulary
(continue / restart / disable), same `max_runs`, same fallback machinery,
same per-occurrence snapshot and provenance path. The state row's key
widens from `show_title` to `(channel_id, sequence_key)` — folding in
v0.3.0's channel-scoping item — and gains a cursor index; only
`cursorBehind` and the next-item lookup fork on kind. `GET /media/movies`
feeds the picker. Details and the interface-level deltas are in the
intake spec.

## v0.7.0 — Operations and observability

Run it for months without shelling into the pod.

- **Notifications.** Webhook (ntfy/Gotify-compatible) on apply failure,
  Tunarr unreachable, and conflict-dropped occurrences.
- **Grafana dashboard + alert rules** shipped in-repo for the existing
  Prometheus metrics; extend metrics to cover apply outcomes, snapshot
  counts, and per-block plan durations.
- **Backup endpoint.** `store.Backup` exists; expose an authenticated
  `GET /backup` (SQLite snapshot download) and document the restore path.
- **Config reload** on SIGHUP or a `/reload` admin call — serve currently
  requires a restart for any config change.

## v0.9.0 — Freeze candidate

No new features; everything is about being able to promise stability.

- **API contract review** of `api/openapi.yaml`: naming, required-ness,
  error shapes, pagination — the last chance to break anything.
- **Config schema review** of `cmd/schema/*.cue` with the same license.
- **Upgrade/rollback guide**: migration chain tested from every released
  0.x DB; documented downgrade stance.
- **E2E suite** (`make e2e-*`) grown to cover the full apply loop against
  a fake Tunarr, run in CI; web harness covering every page's logic
  module.
- **Security pass**: token handling, authz on every route, gosec/
  govulncheck clean, SSO-at-gateway (oauth2-proxy) deployment documented
  as the recommended fronting (in-app stays token-only by design).
- **Docs completeness audit** against the shipped behavior; re-captured
  demo assets.

## v1.0.0 — The promise

- `/api/v1` is frozen: additive changes only until a `/api/v2`.
- Config schema and `scheduler.yaml` import format are frozen the same
  way.
- SQLite migrations are forward-only and tested from every 0.x release.
- Supported deployments: the Docker image and the Helm chart, with a
  documented Tunarr version floor (currently v1.3.13 wire contracts).
- A release is: green CI, CHANGELOG cut, tagged image, chart bump — the
  same flow as today, promised rather than habitual.

## Versioning rules on the way

- **Patch (0.x.y)**: fixes and doc corrections, no behavior changes an
  operator must react to.
- **Minor (0.x.0)**: the milestone releases above; may break API/config/
  DB with a CHANGELOG entry and, where a DB change is involved, an
  automatic migration. No deprecation aliases — old behavior is removed
  in the same release its replacement lands (house no-legacy policy).
- Milestones may ship in slices (e.g. v0.3.1 carrying an item that missed
  v0.3.0) — the numbers mark themes, not gates; v0.9.0's freeze is the
  only hard gate.
- Because the numbers mark themes rather than order, an inserted slice
  pushes the pending ones down instead of renaming the theme. The v0.5
  train has absorbed four such insertions (v0.5.2, v0.5.3, v0.5.4, and
  the v0.5.5 security patch), and the shifted numbers above are the
  current truth — the v0.5 spec's §9 renumber note records only the
  first two. Themes also do not have to
  land in numeric order: the v0.4 metadata slices ship after the v0.5
  train, and if they land before v0.6.0 they are written in the current
  vocabulary and swept by the rename like everything else.
