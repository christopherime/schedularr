# Schedularr Web UI — Design

**Date:** 2026-08-28
**Status:** Approved (design review with operator)
**Sub-project:** 2 of 4 (API core ✓ → **Web UI** → Tag/metadata engine → Container/CI/GitOps deploy)

## Context

Sub-project 1 shipped the API server core: `schedularr serve` hosts a
contract-first REST API (`api/openapi.yaml`, bearer-token auth) plus the
cron loop. The TUI was deleted. This sub-project delivers its
replacement: a web UI embedded in the same binary, built with Hugo
(operator preference) and served by the existing router.

## Goals

- Full v1 coverage (operator-selected): blocks editor, schedule
  preview + apply, series-state panel, status dashboard + history.
- Hugo-built static frontend embedded via `go:embed`, served at `/` by
  the existing `serve` listener — one binary, one port, same origin.
- Contract-first on the client too: TypeScript types generated from
  `api/openapi.yaml`; UI cannot drift from the API without a build error.
- Token-once auth UX: assets are public (no secrets in them); the user
  pastes the API token into a settings panel; the JS client stores it
  (localStorage) and sends `Authorization: Bearer` on every call; 401
  responses route back to the settings panel.
- All UI implementation work goes through the `impeccable` skill; prose
  through `stop-slop` (operator rules).

## Non-goals

- Live updates (WebSockets/SSE) — pages fetch on load and on user action.
- User management, sessions, CSRF — single operator, token model,
  oauth2-proxy fronts the UI in-cluster (sub-project 4).
- Tag/metadata UI (sub-project 3).
- CI/Docker integration of the Hugo build (sub-project 4; the Dockerfile
  gains a Hugo stage there).

## Decisions (settled with operator)

1. **Approach A**: Hugo (layouts, pages, built-in esbuild `js.Build`
   asset pipeline) + Alpine.js (declarative interactivity, no framework
   toolchain) + generated TS types (`openapi-typescript`) with a thin
   typed fetch wrapper. htmx rejected (wants HTML fragments; our API is
   JSON). SPA island rejected (toolchain weight; Hugo reduced to a shell).
2. **Auth UX**: token pasted once, stored in localStorage, attached by
   the client wrapper; UI assets served WITHOUT auth; `/api/v1`
   enforcement unchanged.
3. **v1 pages**: `/` dashboard (Tunarr reachability, block count,
   version, recent history), `/blocks/` (list + create/edit/delete,
   enable/disable, filter and series block forms, cron helper text),
   `/schedule/` (dry-run preview rendered per channel + apply with
   explicit confirmation), `/series/` (per-series cursor
   season/episode, completed/disabled toggles).
4. **Build order**: `make web` = generate TS types → `tsc --noEmit`
   check → `hugo` build into `web/public/` → `go build` embeds it.
   A committed placeholder `web/public/index.html` ("UI not built — run
   make web") keeps plain `go build` working; `make build` depends on
   the web build. `web/public/` (except the placeholder) and
   `web/node_modules/` are gitignored.
5. **Toolchain**: Hugo installed locally via brew (extended edition not
   required — no SCSS), version floor asserted by a Makefile check;
   `web/package.json` pins `typescript` + `openapi-typescript` as the
   only node devDependencies. Node ≥ 20 assumed (v26 present).
6. **Serving**: embedded FS mounted on the router's NotFound path chain:
   exact system routes and `/api/v1` keep precedence; everything else
   serves from the embedded site (Hugo's own `404.html` for misses).
   Standard security headers on UI responses (`X-Content-Type-Options`,
   `Referrer-Policy`; CSP kept permissive enough for Alpine and
   documented — no third-party origins at all).

## Architecture

```txt
web/
├── hugo.toml                # site config, js.Build pipeline
├── package.json             # typescript + openapi-typescript (pinned)
├── layouts/                 # base shell, partials (nav, token panel)
├── content/                 # one page bundle per view
├── assets/ts/               # api.ts (client wrapper), page modules
│   └── gen/types.d.ts       # generated from api/openapi.yaml
├── public/                  # hugo output (gitignored except placeholder)
└── embed.go                 # //go:embed all:public, exported FS
```

- `internal/api/router.go` gains a `UI fs.FS` field on `Config` (or a
  mount function) — the router stays testable with a fake FS.
- The client wrapper (`assets/ts/api.ts`) is the single fetch path:
  token injection, problem+json parsing, 401 → settings routing.
- Alpine components per page consume the wrapper; no direct `fetch`
  calls in page modules.

## Error handling

- API errors render the problem `title`/`detail` inline near the action
  that caused them; 401 opens the token panel; network failure shows a
  retry banner. No silent failures.

## Testing

- Go: router serves embedded index at `/`, correct content-type,
  `/api/v1` and system routes keep precedence, unknown path → Hugo 404
  page with status 404.
- TS: `tsc --noEmit` gate in `make web-check` (wired into `make lint`).
- Feature-first; no browser-automation e2e in v1 (manual smoke per page
  documented in the plan's final task).

## Documentation (same-commit rule)

README gains a Web UI section (build prereqs, `make web`, token setup,
page tour); CLAUDE.md/AGENTS.md architecture trees gain `web/`;
CHANGELOG entry per feature landing.
