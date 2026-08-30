# Roadmap to v1.0.0

What still has to be true before Schedularr calls itself 1.0, grouped into
the minor releases that get there. Pre-1.0 semver applies: a minor may
change behavior or contracts freely (each change documented in the
CHANGELOG, per the no-legacy policy — superseded behavior is deleted, not
aliased); patches are fixes only. **v1.0.0 is a stability promise, not a
feature**: after it, `/api/v1`, the config schema, and the SQLite
migration chain only change additively.

Status: living document — reorder, drop, or promote items as reality
dictates. Last shaped: 2026-08-30 (at v0.2.x).

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
  that adopts existing rows. (TODO.md, parked at SP1 close-out.)
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

## v0.4.0 — Metadata engine (sub-project 3)

The long-planned third sub-project: enrich what the filters and the guide
can see.

- **TMDB/TVDB clients** (no IMDb — no official free API) with on-disk
  cache and rate-limit respect; API keys via env/secret, never in config
  files.
- **Genre/rating normalization.** Close the genre-filter gap: Tunarr's
  library genres are inconsistent across sources; normalize to one
  vocabulary so `filter.genres` matches what operators expect.
- **Tag engine.** Operator-defined tags (`filter.tags` exists but nothing
  populates tags today) assignable per show/movie, persisted in the
  store, editable in the UI.
- **Richer filters.** Decade shortcuts, watched/unwatched via history,
  exclude-lists.
- **EPG enrichment.** Feed artwork/descriptions through to the XMLTV
  guide where Tunarr allows it, so Jellyfin's guide looks like real TV.

## v0.5.0 — Operations and observability

Run it for months without shelling into the pod.

- **Warnings and apply history in the UI.** Conflict warnings are
  per-apply today; persist them with schedule history so "why didn't X
  air last Tuesday" is answerable after the fact.
- **Notifications.** Webhook (ntfy/Gotify-compatible) on apply failure,
  Tunarr unreachable, and conflict-dropped occurrences.
- **Grafana dashboard + alert rules** shipped in-repo for the existing
  Prometheus metrics; extend metrics to cover apply outcomes, snapshot
  counts, and per-block plan durations.
- **Backup endpoint.** `store.Backup` exists; expose an authenticated
  `GET /backup` (SQLite snapshot download) and document the restore path.
- **Config reload** on SIGHUP or a `/reload` admin call — serve currently
  requires a restart for any config change.

## v0.6.0 — UI maturity

- **Calendar/timeline view** of planned occurrences across channels — the
  Schedule page is a table; a week grid is the natural mental model for
  TV.
- **Block duplication and disable-until.** Clone a block as a starting
  point; snooze a block until a date instead of a bare enabled flag.
- **Bulk series operations.** Reset/advance several cursors at once;
  import/export series state from the UI (API exists).
- **Mobile pass.** The UI is desktop-first; make the guide-shaped pages
  usable on a phone.

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
