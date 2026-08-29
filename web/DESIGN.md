---
name: Schedularr Web UI
description: CRT oscilloscope / signal bench -- an operate-mode instrument panel for a self-hosted Tunarr scheduler
colors:
  bg: "#eef2f0"
  bg-raised: "#ffffff"
  bg-inset: "#e3e9e6"
  graticule: "#c7d2cd"
  ink: "#16211c"
  ink-muted: "#3f5148"
  border: "#b9c4bf"
  border-interactive: "#5b6b63"
  accent: "#0f6b3c"
  accent-contrast: "#ffffff"
  warn: "#7a5200"
  danger: "#a3271f"
typography:
  display:
    fontFamily: "ui-monospace, 'SF Mono', 'Cascadia Code', 'JetBrains Mono', Menlo, Consolas, 'Liberation Mono', monospace"
    fontSize: "2.25rem"
    fontWeight: 700
    lineHeight: 1.2
    letterSpacing: "0.06em"
  headline:
    fontFamily: "ui-monospace, 'SF Mono', 'Cascadia Code', 'JetBrains Mono', Menlo, Consolas, 'Liberation Mono', monospace"
    fontSize: "1.75rem"
    fontWeight: 700
    lineHeight: 1.2
    letterSpacing: "0.06em"
  title:
    fontFamily: "ui-monospace, 'SF Mono', 'Cascadia Code', 'JetBrains Mono', Menlo, Consolas, 'Liberation Mono', monospace"
    fontSize: "1.125rem"
    fontWeight: 700
    lineHeight: 1.2
    letterSpacing: "0.06em"
  body:
    fontFamily: "ui-monospace, 'SF Mono', 'Cascadia Code', 'JetBrains Mono', Menlo, Consolas, 'Liberation Mono', monospace"
    fontSize: "1rem"
    fontWeight: 400
    lineHeight: 1.55
    letterSpacing: "normal"
  label:
    fontFamily: "ui-monospace, 'SF Mono', 'Cascadia Code', 'JetBrains Mono', Menlo, Consolas, 'Liberation Mono', monospace"
    fontSize: "0.75rem"
    fontWeight: 600
    lineHeight: 1.2
    letterSpacing: "0.06em"
rounded:
  sm: "2px"
  md: "4px"
  lg: "6px"
spacing:
  1: "0.25rem"
  2: "0.5rem"
  3: "0.75rem"
  4: "1rem"
  5: "1.5rem"
  6: "2rem"
  7: "3rem"
  8: "4rem"
components:
  button-primary:
    backgroundColor: "{colors.accent}"
    textColor: "{colors.accent-contrast}"
    rounded: "{rounded.sm}"
    padding: "0.5rem 1rem"
  button-ghost:
    backgroundColor: "transparent"
    textColor: "{colors.ink}"
    rounded: "{rounded.sm}"
    padding: "0.5rem 1rem"
  button-danger-ghost:
    backgroundColor: "transparent"
    textColor: "{colors.danger}"
    rounded: "{rounded.sm}"
    padding: "0.5rem 1rem"
  panel:
    backgroundColor: "{colors.bg-raised}"
    textColor: "{colors.ink}"
    rounded: "{rounded.lg}"
    width: "min(28rem, calc(100vw - 2rem))"
  badge:
    backgroundColor: "transparent"
    textColor: "{colors.ink-muted}"
    typography: "{typography.label}"
    rounded: "{rounded.sm}"
    padding: "0.25rem 0.5rem"
---

# Design System: Schedularr Web UI

This file documents the system as it shipped across Tasks 1-8 of the web
UI sub-project (2026-08-28), written retrospectively from the built
code -- `web/assets/css/main.css`, `web/layouts/`, `web/assets/ts/`. It
is not a pre-build spec: where a decision happened once and stayed that
way through Task 7, this records what happened, not a recommendation for
what should happen next.
The original design-review record -- the THESIS/OWN-WORLD/STORY/FIRST
VIEWPORT/FORM/FINISH direction contract -- lives as an HTML comment in
`web/layouts/_default/baseof.html` (first child of `<body>`, emitted
through `safeHTML` so it survives Hugo's minifier into the shipped
`web/public/index.html`); this document expands on it rather than
repeating it verbatim. Product context (audience, scope, non-goals)
lives in `PRODUCT.md` at the repo root.

## Overview

**Creative North Star: "CRT oscilloscope / signal bench."**

The UI reads as calibrated measurement instrumentation the operator arms
before trusting a reading, not a dashboard-card admin panel. Dark mode is
the live phosphor trace on the glass; light mode is the instrument's own
calibration printout on graph paper -- both are first-class, since no
usage-time constraint was given (see `PRODUCT.md`'s "Operating Context").
The graticule (an oscilloscope's measuring grid) is the literal CSS
layout background on bordered surfaces. Every status indicator carries
an adjacent text label; color alone never carries a fact (`.status-dot`
next to "Signal"/"No Signal", a toggle's `.toggle__label` next to its
track, a badge's own text next to its accent tint). Typography is
monospace throughout: the product's actual data is cron strings,
tabular durations, and season/episode cursors, so a fixed-width face
reads as instrument readout.

This direction won a fused-challenger round against the design process's
own assigned grounded candidate (plain terminal/crontab); Task 3 kept
that candidate's "visible construction grid" and "coded legend"
disciplines and folded them into the graticule and status-dot rules
above, instead of discarding them along with the rest of the losing
candidate.

**Key characteristics:**

- One font family (monospace), a fixed rem type scale, no `clamp()`.
- Small radii (2-6px) -- a machined instrument bezel, not a soft
  consumer card; this is a deliberate override of a softer default.
- Restrained color: neutrals plus one accent (phosphor green), with
  amber/red reserved for functional signal states, never decoration.
- One authored motion (the token panel's open/close); every other
  transition is a plain, short state change.
- No manual light/dark toggle -- the OS/browser setting decides (see
  Colors below).

## Colors

Every color is a CSS custom property on `:root`, light values first,
overridden as a block under `@media (prefers-color-scheme: dark)`. There
is no third, app-controlled theme state and no toggle UI; `<meta
name="color-scheme" content="light dark">` (in `baseof.html`'s `<head>`)
and `color-scheme: light dark` (in `main.css`'s `:root`) tell the browser
both palettes are genuinely supported so native form controls and
scrollbars theme correctly too.

| Token                        | Light     | Dark      | Role                                                    |
| ----------------------------- | --------- | --------- | -------------------------------------------------------- |
| `--color-bg`                   | `#eef2f0` | `#0a0f0d` | Page background                                          |
| `--color-bg-raised`            | `#ffffff` | `#101715` | Cards, panels, the header bar, table rows                |
| `--color-bg-inset`              | `#e3e9e6` | `#060a08` | Recessed surfaces: inputs, hover rows, series-row blocks |
| `--color-graticule`             | `#c7d2cd` | `#1c2a24` | The `.graticule` grid lines                              |
| `--color-ink`                    | `#16211c` | `#dfeee6` | Primary text                                              |
| `--color-ink-muted`              | `#3f5148` | `#8fac9f` | Secondary text: hints, descriptions, muted labels        |
| `--color-border`                  | `#b9c4bf` | `#24352d` | Static dividers, card borders                             |
| `--color-border-interactive`      | `#5b6b63` | `#5a7469` | Borders on inputs, buttons, toggles -- anything clickable |
| `--color-accent`                   | `#0f6b3c` | `#3ddc84` | Primary action, "armed"/"ok" status, active nav underline |
| `--color-accent-contrast`          | `#ffffff` | `#06150c` | Text/icons on an accent-filled surface                    |
| `--color-warn`                      | `#7a5200` | `#e8a33d` | Reserved; not yet consumed by a shipped component          |
| `--color-danger`                    | `#a3271f` | `#ff6b5e` | Errors, "unarmed"/"down" status, destructive actions       |
| `--color-backdrop`                   | `rgb(22 33 28 / 45%)` | `rgb(2 6 4 / 65%)` | The token dialog's `::backdrop`         |

`::selection`, the focus ring (`:focus-visible`), and the scrollbar
(`scrollbar-color` plus a WebKit `::-webkit-scrollbar*` fallback) are all
themed from these same tokens -- no browser-default gray leaks through
either palette.

## Typography

One family everywhere: `var(--font-mono)`, a `ui-monospace` stack with
named fallbacks (SF Mono, Cascadia Code, JetBrains Mono, Menlo, Consolas,
Liberation Mono) ending in generic `monospace`. Base line-height is
`--leading-normal` (1.55) for body copy; tight headings and status text
use `--leading-tight` (1.2). There is no separate serif/display face --
size, weight, and letter-spacing carry hierarchy instead.

| Token          | Size      | Used for                                                        |
| -------------- | --------- | ----------------------------------------------------------------- |
| `--text-xs`     | `0.75rem`  | Labels, legends, table headers, badges, hints -- almost always paired with `--tracking-label` and uppercase |
| `--text-sm`      | `0.875rem` | Secondary body text, table cells, form hints/errors               |
| `--text-base`     | `1rem`     | Default body text                                                  |
| `--text-md`         | `1.125rem` | Panel/dialog headings (`.panel__head h2`)                          |
| `--text-lg`           | `1.375rem` | Section headings (`.section-head h2`)                              |
| `--text-xl`             | `1.75rem`  | Page headings (`.page-head h1`)                                    |
| `--text-2xl`              | `2.25rem`  | The hero-panel's own `h1` (dashboard status card container)         |

`--tracking-label` (`0.06em`) plus `text-transform: uppercase` is the
recurring "instrument legend" treatment: wordmark, nav links, button
text, badges, form-field labels, section/page headings all use it. Body
prose (`<p>` inside `.panel__hint`, `.section-head p`, etc.) is the one
context that stays sentence-case with normal tracking.

## Layout

`.shell` is a column flexbox pinning a sticky `.bezel` header above a
flexed `.content` region and a `.footer`. `.content` caps at `76rem`,
centered (`margin-inline: auto`), with `--space-6 --space-5 --space-8`
padding that steps down to `--space-5 --space-4 --space-7` under a single
`640px` breakpoint -- the only breakpoint value used anywhere in the
file. Responsiveness is structural, not fluid: no `clamp()` typography,
and wide content (the history/blocks/schedule/series tables) scrolls on
its own axis via `.table-wrap { overflow-x: auto }` rather than letting
the page scroll horizontally.

`.form-grid` (`display: grid; grid-template-columns: repeat(auto-fit,
minmax(12rem, 1fr))`) is the one reusable multi-column layout primitive,
used by the blocks editor's field groups; `.form-field--wide` spans all
columns via `grid-column: 1 / -1` for a field that shouldn't share a row
(title-pattern regex, tag lists).

## Elevation & Depth

The system is almost flat -- bordered surfaces, not shadowed cards. The
one deliberate shadow lives on the token `<dialog>` (`0 20px 60px -20px
rgb(0 0 0 / 45%), 0 4px 16px rgb(0 0 0 / 20%)`), justified because it is
the one surface that visually detaches from the page (a native
`<dialog>` with its own backdrop); every other panel/card
(`.hero-panel`, `.editor-panel`, `.empty-state`, `.problem`) is
differentiated by a `1px` border plus a background-color step
(`--color-bg-raised` or `--color-bg-inset`), never a shadow. Depth
ordering elsewhere is z-index only: `.bezel` is `position: sticky; z-index:
10` above the content flow.

## Shapes

Radii are small on purpose -- `--radius-sm` (`2px`) for the tightest
elements (buttons, badges, toggles, inputs), `--radius-md` (`4px`) for
mid-size blocks (series rows, the problem panel), `--radius-lg` (`6px`)
for the largest bordered surfaces (hero-panel, table-wrap, the token
dialog, the empty-state panel). This is a disclosed override of a softer
consumer default: the thesis is a machined instrument bezel, and a
12-16px radius would read as a SaaS dashboard card instead. Border width
is `--border-width` (`1px`) almost everywhere; `--border-width-thick`
(`2px`) marks a few load-bearing rules -- the active nav link's bottom
accent, the history/blocks/schedule/series table header's separator
line, the focus ring.

## Components

Class names below are the actual selectors in `web/assets/css/main.css`;
"reused verbatim" means the later page introduced zero new CSS for that
shape.

- **`.btn`** -- base button: `--text-sm`, uppercase, `--tracking-label`,
  `--radius-sm`, `1px` interactive-border, `--color-bg-raised`
  background. Variants: `.btn--primary` (accent fill, white/near-black
  text per palette), `.btn--ghost` (transparent, border-only),
  `.btn--danger-ghost` (transparent, danger-colored text/hover-border),
  `.btn--icon` (no border, icon-only), `.btn--sm` (tighter padding,
  `--text-xs`). Task 8 added `text-decoration: none` to the base rule:
  the 404 page's "Return to Dashboard" is the first `.btn` on an `<a>`
  rather than a `<button>`, and anchors need it reset explicitly.
- **`.status-dot`** -- an 8px circle, coded-legend discipline: it never
  appears without adjacent text naming the state. Two vocabularies share
  the same `data-state` attribute and the same two colors: the token
  panel's `armed`/`unarmed`, and every live reading's `ok`/`down`
  (dashboard Tunarr signal, blocks/schedule/series channel fallbacks,
  the 404 page's "No Signal"). `unknown` (the token trigger's initial
  state before JS runs) falls through to the base muted color.
- **`.problem`** -- the inline API-error panel (`.problem__title` +
  `.problem__detail`), used identically on every page that fetches on
  load: dashboard status/history, blocks list/editor, schedule
  generate/apply, series list. Always rendered next to the section that
  failed, with a `Retry`-equivalent action, per `PRODUCT.md`'s "No
  silent failures."
- **`.skeleton-bar` / `.skeleton-row` / `.skeleton-stack`** -- a muted
  pulsing block (1.2s ease-in-out, frozen to static under
  `prefers-reduced-motion` via the global reset) for every page's
  initial-load state.
- **`.table-wrap` + `.history-table`** -- the one table shape in the
  system, introduced on the dashboard (Task 4) and reused verbatim,
  unmodified, by blocks (`blocks-table__*` only adds cell-content
  wrappers), schedule (`schedule-*`), and series
  (`series-table__show`/`.series-cursor`). Bordered wrapper with its own
  horizontal scroll, `--border-width-thick` header rule, tabular-nums on
  numeric/timestamp columns.
- **`.form-grid` / `.form-field` / `.checkbox-field`** -- the blocks
  editor's field system: auto-fit grid columns, uppercase `--text-xs`
  labels, `--color-bg-inset` inputs with an interactive border.
  `.form-field__error` (danger-colored) and `.form-field__hint` (muted)
  sit directly under a field. Deliberately parallel to, not shared with,
  the token panel's `.field`/`.field__control` (that one reserves
  padding for a password show/hide icon a plain field never has).
- **`dialog.panel` / `.editor-panel`** -- two panel chromes sharing one
  internal-spacing vocabulary (`.panel__head`, `.panel__body`,
  `.panel__actions`). The token panel is a native `<dialog>` (focus trap
  and Escape-to-close for free, per `operate.md`'s ban on hand-rolled
  modals) with one authored open/close transition
  (`@starting-style`/`allow-discrete`, `--duration-base`, degrades to an
  instant show/hide without support). The blocks editor
  (`.editor-panel`) reuses the same head/body/actions spacing but is an
  inline `<section>`, not a `<dialog>` -- creating/editing a block is a
  multi-minute task, long enough that interrupting the page under a
  modal isn't worth it.
- **`.hero-panel` + `.graticule`** -- the bordered "instrument surface"
  primitive (Task 3's landing placeholder), reused as-is by the
  dashboard's status card, the schedule controls panel, and the 404
  page's "No Signal" readout. `.graticule` is a repeating-gradient grid
  background, applied wherever a surface needs the literal
  measurement-grid texture.
- **`.badge`** -- the blocks list's type indicator (`Filter`/`Series`);
  text carries the fact, `data-type="series"` adds an accent tint as a
  secondary scan aid only.
- **`.toggle`** -- a `role="switch"` button styled as a small rocker
  (not an iOS-style pill, matching the small-radii shape language), used
  for both blocks' Enabled/Disabled and series' Completed/Disabled
  toggles.

## Do's and Don'ts

- **Do** reuse an existing token or class verbatim before adding a new
  one. Every page after Task 4 shipped its page-specific markup on top
  of `.hero-panel`/`.graticule`/`.table-wrap`/`.history-table`/`.problem`/
  `.skeleton-*`/`.form-field` without modifying them; new CSS was added
  only for genuinely new shapes (the series cursor's paired inputs, the
  schedule status readout).
- **Do** pair every `.status-dot` (and any other color-only signal) with
  adjacent text naming the state. This is load-bearing, not stylistic:
  it is this system's WCAG SC 1.4.1 (Use of Color) answer.
- **Do** keep motion to plain, short state transitions
  (`--duration-fast`/`--duration-base`) except the token panel's one
  authored open/close. Respect `prefers-reduced-motion` (already
  enforced globally in the reset).
- **Don't** introduce a card-grid, kicker/eyebrow, or same-size
  icon+heading+text pattern -- this is an operate-mode instrument panel
  (a readout row and a table), not a marketing surface.
- **Don't** add a manual light/dark toggle. The system commits to
  `prefers-color-scheme` answering for both palettes; see Colors above.
- **Don't** use a spinner for a loading state; use `.skeleton-*`.
- **Don't** raise a toast for an API failure; render `.problem` inline
  next to the section that failed.
- **Don't** soften the radius scale past `--radius-lg` (`6px`) for a new
  bordered surface -- the small-radii choice is a deliberate, disclosed
  break from a softer default, not an oversight to "fix."

## Accessibility: WCAG contrast evidence

Every color pairing introduced while building this system was checked
computationally (the WCAG relative-luminance/contrast-ratio formula),
not eyeballed, using a throwaway Node script (not committed, per the
project's own convention for verification-only tooling). Two tasks
introduced new color pairings; every page after that reused the same
tokens on the same component shapes, so no further pairings needed
independent verification.

**Task 3 (shell: header, nav, token panel, buttons):** every text
pairing in both palettes cleared WCAG AA's 4.5:1 floor, several by a
wide margin (6:1-16:1). The one interactive non-text pairing checked
(an input/button border against its background) cleared the 3:1
non-text floor in both palettes -- light `4.99:1`, dark `3.81:1` (raised
from an initial `3.02:1` during that task after the check flagged it as
too close to the floor).

**Task 4 (dashboard: status card, history table, problem/skeleton/empty
states):**

| Pairing                                                    | Light    | Dark     |
| ------------------------------------------------------------ | -------- | -------- |
| `--color-danger` text on `--color-bg-inset` (`.problem__title`) | 5.95:1  | 7.13:1  |
| `--color-ink-muted` on `--color-bg-raised` (detail text, empty-state, table `th`) | 8.46:1 | 7.42:1 |
| `--color-ink` on `--color-bg-raised` (table body text)          | 16.56:1 | 15.15:1 |
| `--color-border-interactive` on `--color-bg-raised` (table header rule, non-text) | 5.63:1 | 3.58:1 |

All text pairings clear the 4.5:1 AA floor by a wide margin in both
palettes. The `.hero-panel` border itself (a decorative ~1px, ~1.5:1
pairing) is not held to the 3:1 non-text floor -- it is not a
state-bearing UI element, the same judgment Task 3 made for the same
border class.

Tasks 5-7 (blocks, schedule, series) each state explicitly in their own
task reports that no new colors were introduced -- every component
reused an existing token on an existing shape (the series cursor's
paired inputs use the same `--color-bg-inset`/`--color-border-interactive`
pairing already verified above; the schedule status readout reuses the
`ok`/muted `.status-dot` vocabulary). Task 8's 404 page is the same:
`data-state="down"` and `.hero-panel`/`.graticule` are the exact classes
Task 4 already verified, and its one new CSS rule
(`.btn { text-decoration: none }`) touches no color.

## Alpine.js conventions

Alpine is vendored (`web/assets/vendor/alpine.min.js`, pinned, loaded
`defer`, no CDN) and used narrowly: one `Alpine.data()` component per
page, registered inside a `document.addEventListener("alpine:init",
...)` block in that page's own TS bundle, plus one small inline
`x-data` for the token panel's show/hide toggle in `baseof.html`. Three
rules hold across every page (`web/assets/ts/pages/*.ts`,
`web/layouts/**/*.html`):

1. **Never `x-init` a method `Alpine.data()` already names `init()`.**
   Alpine auto-invokes a data object's own `init()` method as part of
   component initialization (documented behavior, not an assumption --
   see https://alpinejs.dev/globals/alpine-data). An early build of the
   dashboard page had `x-init="init()"` on the root element *in addition
   to* the component's own `init()` method, which ran `loadStatus()`/
   `loadHistory()` twice on every real page load (fixed in commit
   `0ad914e`). The fix was deleting the `x-init` attribute, not renaming
   the method -- there is no `x-init` attribute anywhere in
   `web/layouts/` today.
2. **Keep the `started`-guard as defense-in-depth, not as the fix.**
   Every page component (`dashboard.ts`, `blocks.ts`, `schedule.ts`,
   `series.ts`) still declares a module-level `let started = false;`
   and checks/sets it as the first two lines of `init()`. This is cheap
   insurance against a *future* accidental double-wire (someone
   re-adding `x-init`, a second component instance on the same page),
   not a workaround for a live bug -- rule 1 is the actual fix, and the
   guard is documented as such inline in each file.
3. **`x-text` only; never `x-html`.** Every dynamic string in every page
   template (API error `detail`/`title` text, status readouts, table
   cell values, form errors) is bound via `x-text`, which sets
   `textContent`. There is no `x-html` anywhere in `web/layouts/`. This
   is a deliberate choice: `problem+json` `detail` strings come from the
   API and are never trusted as markup (`dashboard.ts`'s `describeError`
   doc comment states this explicitly), so a `detail` containing
   HTML-looking text renders as visible text on screen instead of
   getting parsed.

## Content-Security-Policy

Every UI response (`internal/api/ui.go`'s `newUIHandler`, spec Decision 6
in `docs/superpowers/specs/2026-08-28-web-ui-design.md`) carries:

```
Content-Security-Policy: default-src 'self'; script-src 'self' 'unsafe-eval'; style-src 'self'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'
```

Every directive is `'self'` (or `'none'` for `frame-ancestors`) because the
shipped site never loads a third-party origin: Alpine is vendored (no
CDN), `web/assets/ts/api.ts`/`token.ts` are the only modules that touch
`fetch` and only ever call this same origin's `/api/v1`, and there are no
`<img>`/font/stylesheet references to anywhere but `web/assets/css/main.css`
itself.

`script-src` carries `'unsafe-eval'` because Alpine.js 3's directive
expressions (`x-data`, `x-show`, `x-text`, `x-if`, ...) are evaluated via
`new Function(...)`, which CSP classifies as `eval`-family and blocks by
default; there is no CSP-compliant build of Alpine 3 that keeps this UI's
templates working (`@alpinejs/csp`, Alpine's own strict-CSP build, trades
expression evaluation for a much narrower directive subset this codebase's
templates don't target). `img-src` allows `data:` defensively even though
nothing in the shipped templates uses a `data:` URI today.

**Consequence for every future page/component: no inline `style="..."`
attributes and no inline `<style>` blocks.** `style-src 'self'` (no
`'unsafe-inline'`) means a real browser silently drops any inline style's
declarations -- unlike a CSP-blocked script, this fails quiet, not loud,
so it's easy to miss in review. This bit the dashboard's status-card
skeleton (`web/layouts/index.html`) during the CSP fix wave: its three
loading bars set per-bar widths via `style="width: 6rem"` etc. (needed
because `.skeleton-row` is `flex-direction: row`, so unlike
`.skeleton-stack`'s column layout there's no implicit cross-axis stretch
to size an empty `<span>`). Fixed by adding three width-modifier classes
instead (`.skeleton-bar--w-sm/--w-md/--w-lg`, `web/assets/css/main.css`)
-- same visual result, zero inline styles. Same discipline as the `x-text`
vs. `x-html` rule above: prefer a CSS class or a data-attribute selector
over anything the CSP would have to special-case.

Verified live (`schedularr serve`, `curl -sI`) on every route
(`/`, `/blocks/`, `/schedule/`, `/series/`, and an unknown path's 404) --
see `internal/api/router_test.go`'s `TestRouter_UIContentSecurityPolicyHeader`
for the automated 200-and-404 assertion.

## Provenance

Written for Task 8 of the web UI sub-project, from the shipped code and
the four prior task reports that recorded design decisions
(`.superpowers/sdd/2026-08-28-web-ui/task-{3,4,5,6,7}-report.md`). The
`impeccable` skill's `context.mjs` reported `WORLD_DISCOVERY_REQUIRED`
for this and every UI-touching task through Task 7 because this file
didn't exist yet; it now serves as the visual-world reference future
`impeccable` invocations against `web/` should find.
