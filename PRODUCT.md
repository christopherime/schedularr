# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

## Stack

Hugo (Go static site generator, layouts/partials, built-in esbuild
`js.Build` asset pipeline) + Alpine.js (vendored single file, no CDN, no
JS framework) + hand-written CSS with custom properties (no CSS
framework/utility library) + TypeScript compiled by Hugo's pipeline,
typed against a contract (OpenAPI) generated `types.d.ts`. This was
decided by the operator in an approved design review, not chosen by this
task — see `docs/superpowers/specs/2026-08-28-web-ui-design.md` decision
1 and `docs/superpowers/plans/2026-08-28-web-ui.md`.

## Users

A single operator: the person who self-hosts the `schedularr` binary
alongside Tunarr (an IPTV/live-channel server) and runs `schedularr
serve`. They administer their own instance — there is no multi-user
model, no signup, no public audience. They interact with the UI to avoid
hand-writing YAML block specs or calling the API directly with curl.

## Product Purpose

Schedularr generates and manages scheduled TV "blocks" — filter-based
(pull matching programs from a library by genre/rating/year/duration/etc.)
or series-based (play episodes of a named show in order, tracking a
season/episode cursor) — and schedules them onto Tunarr channels via cron
expressions. It previously shipped a TUI; that was deleted, and this web
UI is its replacement, reachable at the same origin and port as the API
(`schedularr serve`, default `:8484`).

## Positioning

Tunarr provides the channel/streaming engine but no scheduling logic;
Schedularr is the scheduling brain in front of it — contract-first (an
OpenAPI spec at `api/openapi.yaml` is the single source of truth for both
server and generated client types), so the UI cannot silently drift from
what the API actually supports.

## Operating Context

Self-hosted, typically on a home server or small cluster, accessed by the
operator over their own network via a browser. `schedularr serve` mounts
both the JSON API (`/api/v1/*`, bearer-token authenticated) and the
static UI (everything else) on the same listener. UI assets themselves
are public/unauthenticated (no secrets baked into them); only `/api/v1`
calls require the token, pasted once into the UI and kept in
`localStorage`. This is an inferred normal usage pattern (evenings,
after-work adjustments, occasional daytime checks) rather than a
confirmed fact — no usage-time constraint was given, so the UI should
read comfortably in both light and dark ambient conditions rather than
assume one.

## Capabilities and Constraints

v1 scope (full coverage, operator-selected in the design review):
dashboard (Tunarr reachability, block count, version, recent history),
blocks editor (list + create/edit/delete + enable/disable, filter-block
and series-block forms), schedule preview + apply (dry-run per channel,
then explicit-confirmation apply), series-state panel (per-show
season/episode cursor, completed/disabled toggles).

Explicit non-goals for v1: no live updates (WebSocket/SSE) — pages fetch
on load and on user action; no user accounts, sessions, or CSRF (single
operator, token model); no tag/metadata UI (separate sub-project); no
charts/graphics for the schedule timeline (list-based per channel).

Terminology: a **block** is a scheduled content rule (filter or series
type) that produces programs for a Tunarr channel on a cron trigger. A
block's **series state** is the per-show cursor (current season/episode,
run count, completed/disabled) that a series block advances as it airs.
A **plan** is the dry-run or applied output of `/generate` or `/apply` —
a set of scheduled slots per channel.

## Brand Commitments

Name is "Schedularr" — follows the `*arr` naming convention common across
the self-hosted media-server ecosystem (Sonarr, Radarr, Prowlarr, etc.).
No logo, color mark, or other visual asset exists yet; no brand voice was
specified beyond that ecosystem's typical plain, technical tone. This UI
is free to establish its own visual world within that constraint.

## Evidence on Hand

No screenshots, mockups, existing UI, or brand assets exist prior to this
sub-project (the previous TUI was deleted, not carried forward as visual
reference). Nothing here should be treated as inherited visual truth.

## Product Principles

1. **Contract-first**: the UI represents exactly what `api/openapi.yaml`
   defines — no invented fields, no speculative capabilities.
2. **Operate, not persuade**: every screen exists to get a task done
   efficiently for one trusted operator; clarity and speed outrank
   marketing polish or ornamentation.
3. **No silent failures**: API errors surface inline, near the action
   that caused them, using the problem+json `detail` text — never a
   swallowed rejection.
4. **Token-once, same-origin**: no session/cookie machinery; a single
   pasted bearer token unlocks the UI until the operator clears it.
5. **Static-first**: Hugo renders static HTML; Alpine adds only the
   interactivity each page specifically needs — no SPA routing, no
   client bundle bloat.

## Accessibility & Inclusion

No standard was mandated by the operator. Default to solid semantic
HTML, a keyboard-operable token modal (focus trap while open, Escape to
close, returns focus on dismiss), and sufficient contrast in both the
light and dark palettes (WCAG AA as a floor, not a formally required
target).
