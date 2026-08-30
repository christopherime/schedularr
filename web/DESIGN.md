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
| `--color-warn`                      | `#7a5200` | `#e8a33d` | "Unarmed" token status dot; `.form-field__warning` (soft, non-blocking field warnings); the schedule picker's cron-lock note |
| `--color-danger`                    | `#a3271f` | `#ff6b5e` | Errors, "unarmed"/"down" status, destructive actions       |
| `--color-backdrop`                   | `rgb(22 33 28 / 45%)` | `rgb(2 6 4 / 65%)` | The token dialog's `::backdrop`         |

`::selection`, the focus ring (`:focus-visible`), and the scrollbar
(`scrollbar-color` plus a WebKit `::-webkit-scrollbar*` fallback) are all
themed from these same tokens -- no browser-default gray leaks through
either palette.

**Tinted state surfaces (v0.5.0).** Warn/danger panels sit on
`--surface-warn` / `--surface-danger` -- `color-mix(in srgb,
var(--color-warn|danger) 8%, var(--color-bg-raised))`, overridden to a
10% mix in the dark palette -- instead of plain `--color-bg-inset`. The
`.problem` panel uses the danger surface, `.schedule-warnings` the warn
surface. Contrast evidence for every text pairing on these surfaces is in
the WCAG section below.

## Token deltas (v0.5.0 bench rebuild)

Added in the v0.5.0 foundation slice (spec §4), all on `:root`:

| Token | Value | Role |
| ----- | ----- | ---- |
| `--z-bezel` / `--z-sticky` / `--z-popover` / `--z-dialog` | 10/20/30/40 | The z-scale; no ad hoc z-index values. Native `<dialog>` top layer sits above all of them for free. |
| `--glow-accent` | 3px accent ring (color-mix 25%) | Names the armed-dot ring (status dots `armed`/`ok`). |
| `--shadow-dialog` | the two-layer dialog shadow | The one deliberate shadow, now named. |
| `--icon-size` | `1.125rem` | One size for every drawn icon; collapses the three drifted values (1rem/1.1rem/1.125rem). |
| `--measure` | `68ch` | Prose line cap (page-head/section-head/empty-state text). |
| `--content-max` | `76rem` | The page column cap. The v0.5.1 guide is the one sanctioned full-bleed exception. |
| `--duration-slow` | `250ms` | Reserved for the largest disclosures. |
| `--surface-warn` / `--surface-danger` | color-mix tints | See Colors above. |

`--div` is load-bearing since v0.5.1: it IS the guide's 30-minute column
width, so graticule lines, ruler cells, and slot boundaries coincide.
The guide scopes `--div` up to `44px` (`.guide`) so an hour label fits
one division; the identity "one graticule square = 30 minutes" holds at
any `--div`, and every derived length (`--q-w` = `--div`/6 per 5-minute
quantum, `--px-per-min` = `--div`/30) follows the override
automatically. Any dynamic geometry keyed to it must go through CSSOM
(`el.style.setProperty`, Alpine `:style`) -- see Content-Security-Policy
below for why an inline `style` attribute silently fails.

## The guide grid (v0.5.1)

The Guide (`/`, `web/layouts/index.html` + `web/assets/ts/pages/guide.ts`
+ `web/assets/ts/runtime/grid.ts`) renders the EPG as home. Its geometry
contract comes from the measured pre-slice spike (spec §9) and is
binding:

- **Sticky topology from FLEX FLOW, never grid membership.** The sheet
  (`.guide-sheet`) is a block with `width: max-content`; the ruler
  (`.guide-ruler`) is a sticky-top flex row with a sticky-left corner;
  each channel row (`.guide-row`) is a flex of sticky-left plate cell +
  track. Chromium forgives sticky grid items; Safari historically does
  not. Both axes scroll inside ONE scrollport (`.guide-viewport`, a
  bounded-height `overflow: auto` box) -- that is what makes top and
  left stickiness hold simultaneously.
- **One CSS grid per channel track**, `repeat(288, var(--q-w))` -- 288
  five-minute quanta per day. Slots are placed by grid-column line
  numbers set via CSSOM (`el.style.setProperty("grid-column", ...)`);
  the pure quantization (floor start, ceil end, clamp to the day,
  cut-left/right flags for overnight spills) lives in
  `runtime/grid.ts`'s geometry section and is unit-tested
  (`web/tests/grid.test.ts`).
- **The now-line** (`.guide-nowline`) is an absolute overlay child of
  the sheet at `calc(var(--rail-w) + var(--now-min) * var(--px-per-min))`;
  `--now-min` is set once per minute via CSSOM by a local 60s timer (a
  discrete step, not an animation loop; heartbeat skew correction
  arrives with SSE in v0.5.4). No scroll handler anywhere. The
  phosphor-persistence trail is a `::before` gradient riding the rule;
  reduced motion drops the trail and keeps the rule. z-order inside the
  sheet: nowline (1) < plates (2) < ruler (3) < corner (4).
- **Ghost slots** (`.guide-slot--ghost`): a current-plan conflict
  warning renders as an amber-hatched `NO SIGNAL — LOST TO <block>`
  block at its would-have-aired time, in an implicit second track lane
  under the slot that displaced it. Hatching never carries the fact
  alone -- the text label does (SC 1.4.1). INTERIM: the Warning wire
  shape carries only names + `occurrence_start` until v0.5.3, so the
  ghost's channel and duration resolve client-side from the losing
  block's spec (`resolveGhost`, `runtime/grid.ts`).
- **Slot states**: `.is-past` dims behind the sweep, `.is-on-air`
  carries the armed glow; both are refreshed by the same minute tick.
  The series tint (`data-type="series"`, a 7% accent mix) is a
  secondary scan aid -- the meta line names the type in text.
- **Inspector** (`.guide-inspector`): a desktop right rail sharing the
  flex row with the grid -- opening it compresses the grid, never
  overlays it; under the 640px breakpoint it becomes a fixed bottom
  sheet. Esc/X close with focus returned to the opening slot.
- **Mobile rundown** (`.rundown-*`): under 640px the grid yields to a
  vertical day-grouped rundown (TONIGHT / TOMORROW / `MON 02` headings)
  for one channel behind a picker, with the now-line as a horizontal
  rule re-slotted between past and future rows each minute.
- **Full-bleed**: the guide is the ONE sanctioned `--content-max`
  exception -- `content/_index.md` sets `full_bleed`, baseof adds
  `.content--bleed`, and the guide's chrome (head/toolbar) stays on the
  page column while the grid region escapes it.
- The grid and rundown DOM are built in TS from the typed plan
  (`renderGuideDay` / `renderRundown`); Alpine drives ONLY the toolbar
  and the inspector. Keyboard: roving tabindex across slots --
  Left/Right along a track, Up/Down across channels (nearest start
  time), Enter opens, Esc closes.

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

## Hugo partials (`web/layouts/partials/ui/`) and the `/kit/` gallery

Since v0.5.0 the shared component markup lives in Hugo partials instead
of being copy-pasted per page -- a page template instantiates a partial
with its Alpine expressions as dict args. The set:

- **`skeleton`** -- variants `bar` / `row` / `stack` / `table-row` (the
  table-shaped loading silhouette) / `grid-track` (the guide's
  silhouette: plate stubs + pulsing bars whose widths and offsets are
  multiples of `--div`, so the loading state pulses on the same
  30-minute graticule the real grid lands on); widths via modifier
  classes, never inline styles (CSP rule).
- **`problem`** -- the single inline error idiom: static context label,
  the API's own `title: detail` line, an optional retry action, and a
  muted `REF <request_id>` line for server-log correlation. Binds a
  runtime `ProblemView` (see `runtime/errors.ts`). The blocks page's two
  competing error surfaces (`listError`/`blocksError`) collapsed into it.
- **`empty`** -- teaching empty state: legend line, one sentence, one
  action (link or Alpine click).
- **`toggle`** -- the `role="switch"` rocker, shared instead of pasted.
- **`channel-select`** -- the channel picker with an explicit disabled
  "Loading channels…" select while the list is in flight (ends the
  raw-text-input ambiguity); falls back to free text on error/empty.
- **`plate`** -- the channel legend plate (`CH 04 · HORROR`), the
  UUID-replacement idiom; resolves via the runtime's `/channels` cache
  and falls back to the shortened raw id when unresolvable.
- **`icon`** + **`icon-defs`** -- one inline SVG sprite (one stroke
  voice: 1.75/round), sized by `--icon-size`; ends path duplication.
- **`confirm`** -- the one native `<dialog>` confirm idiom (apply,
  delete, future bulk), with max-height/overflow for small viewports;
  replaced the blocks page's inline row-swap confirm.
- **`tape`** -- the event tape region: timestamped uppercase lines,
  newest first, max 3 retained, at most one action per line, no
  auto-dismiss; driven by `runtime/tape.ts`'s `printTape`.
- **`page-js`** -- the bundling boilerplate, including the
  cronstrue-before-page-bundle ordering constraint.

**`/kit/` (dev builds only)** renders every partial in every state on
fixture data and is the review gate: a slice is not done until its new
states appear there. It is excluded from production builds via
`web/config/production/hugo.toml`'s cascade (`build.render/list =
"never"` for `/kit/**`); build it with `hugo -s web -e development`.

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
- **`.telemetry`** (v0.5.0) -- the bezel telemetry strip: TUNARR
  (coded-legend dot+text) plus LAST APPLY / NEXT TICK relative readouts
  on every page, fed by `runtime/shell.ts`'s 60s `GET /status` poll. The
  LIVE/POLL/LINK legend is deliberately absent until the SSE slice
  (v0.5.4) can make it honest.
- **`.plate`** (v0.5.0) -- the channel legend plate; see the partials
  section above.
- **`.tape`** (v0.5.0) -- the event tape; success is printed, not
  toasted. The newest line takes a 150ms print draw-in (suppressed under
  reduced motion); older lines recede to muted.

### Component-state floor (v0.5.0)

- Inputs: `[aria-invalid="true"]` gets the danger border;
  `.form-field:focus-within > label` sharpens to full ink; the focus
  ring stays the global `:focus-visible` outline.
- Buttons: `[aria-busy="true"]` runs a subdued sweep shimmer
  (`currentcolor` at 18% -- adapts to any variant; frozen to a faint
  static wash under reduced motion). Callers pair it with `:disabled`.
- Every hover rule carries `:not(:disabled)`.
- `@media (forced-colors: active)` fallbacks cover every box-shadow- or
  background-borne state: dots get a `CanvasText` border (+ `Highlight`
  fill for armed/ok), the active nav link a `Highlight` bottom border,
  the toggle thumb a border and its checked track `Highlight`, and the
  busy shimmer is dropped (text still changes).
- The token panel's parallel `.field`/`.field__control` vocabulary is
  gone: one field system (`.form-field`, plus `.form-field__control` /
  `.form-field__reveal` for the trailing-button case).

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

**v0.5.0 bench rebuild** introduced the tinted surfaces, the danger-fill
button, and the tape/plate/telemetry text placements. Checked
computationally (same throwaway-script convention; 8% mix light, 10%
dark):

| Pairing | Light | Dark |
| ------- | ----- | ---- |
| `--color-warn` on `--surface-warn` (warnings title) | 6.14:1 | 7.15:1 |
| `--color-ink-muted` on `--surface-warn` (warnings list) | 7.51:1 | 6.30:1 |
| `--color-danger` on `--surface-danger` (`.problem__title`) | 6.43:1 | 5.77:1 |
| `--color-ink-muted` on `--surface-danger` (detail + REF lines) | 7.43:1 | 6.58:1 |
| `--color-accent-contrast` on `--color-danger` (`.btn--danger`) | 7.32:1 | 6.71:1 |
| `--color-ink` on `--color-bg-inset` (plate name) | 13.45:1 | 16.60:1 |
| `--color-ink` / `--color-ink-muted` on `--color-bg` (tape lines) | 14.66 / 7.49:1 | 16.11 / 7.88:1 |

Every pairing clears WCAG AA's 4.5:1 text floor (worst case 5.77:1). The
telemetry strip's label/value inks on `--color-bg-raised` are the
already-verified Task 3/4 pairings.

**v0.5.1 guide** introduced the slot faces, the series tint, and the
ghost hatching. Checked computationally (same throwaway-script
convention):

| Pairing | Light | Dark |
| ------- | ----- | ---- |
| `--color-ink-muted` on `--color-bg-inset` (slot meta line) | 6.88:1 | 8.13:1 |
| `--color-ink` on the series tint (7% accent mix over bg-inset) | 12.20:1 | 15.09:1 |
| `--color-ink-muted` on the series tint | 6.23:1 | 7.39:1 |
| `--color-warn` on `--surface-warn` (ghost text, base surface) | 6.14:1 | 7.15:1 |
| `--color-warn` on the 18% hatch stripe (ghost text, worst case) | 4.71:1 | 5.00:1 |

Every pairing clears the 4.5:1 AA floor; the ghost's worst case (text
over a hatch stripe) is the tightest at 4.71:1 light. Ruler labels,
rundown rows, and the inspector reuse already-verified token pairings on
`--color-bg-raised`.

## TypeScript runtime and Alpine.js conventions

**One bundle per page (v0.5.0).** The shared runtime
(`web/assets/ts/runtime/`: `api.ts`, `token.ts`, `errors.ts`,
`format.ts`, `channels.ts`, `tape.ts`, `shell.ts`) is imported by thin
page entries (`web/assets/ts/pages/*.ts`) and compiled INTO each page's
single esbuild bundle by the `ui/page-js` partial -- there is no
separate shell bundle and no `window.schedularr` global anymore. Within
a page every module (the `ApiError` class identity included) is the same
compiled copy, which is what makes `instanceof` checks safe. The
runtime's `api.ts` is the only module allowed to call `fetch`, adds
AbortController timeouts (15s reads / 60s writes), and entry-guards
mutations (an identical in-flight mutation shares the first request's
promise instead of double-firing). `shell.ts` wires the token panel
(Save probes `GET /status`, arms only on success, then broadcasts the
re-auth event that re-fires failed loads) and the bezel telemetry poll.

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

## Vendored dependencies

Every third-party script this UI loads is vendored into
`web/assets/vendor/` -- pinned to an exact version, loaded via a plain
`<script defer>` from this origin, no CDN, no npm runtime dependency (the
files aren't `require`/`import`-ed by any bundled TS; each attaches
itself to the global scope the way a plain `<script>` tag would). This
table records what's pinned and its sha256, the same verification the
Colors section above already applies to contrast pairings -- checked
computationally, not eyeballed.

| File | Version | Loaded on | sha256 |
| ---- | ------- | --------- | ------ |
| `alpine.min.js` | 3.16.3 | every page (`baseof.html`) | `e31d6d92aefd41979d3c66f994d3a6b77fafa5062aec67d13f3ec5099d70d5d6` |
| `cronstrue.min.js` | 3.24.0 | blocks (`blocks/list.html`) | `f47fa32a8c38a0fd996ef386ffc8c97694e483742a3efc3e3d70d147112b8bd5` |

`cronstrue.min.js` is the npm package's standalone UMD build
(`dist/cronstrue.min.js` from the `cronstrue` tarball), English locale
only -- not `dist/cronstrue-i18n.js`, which bundles every locale this UI
never offers a way to select. It replaces the blocks editor's earlier
hand-rolled `cronHint()` (a narrow parser recognizing only fixed-time/
weekday-restricted patterns) with a universal plain-language readback for
any valid 5-field expression, backing the Simple/Cron schedule picker's
live readback in both modes (see `web/assets/ts/pages/blocks.ts`'s
`cronReadback()`). MIT-licensed, same as Alpine.

To re-vendor either file: download the exact pinned version's tarball
(`npm pack <package>@<version>`), copy the standalone build from `dist/`
into `web/assets/vendor/`, and update this table's version + sha256
together -- never bump one without the other.

## Static assets

`web/static/favicon.png` (64x64) and `web/static/apple-touch-icon.png`
(180x180) are committed, hand-derived rasters -- `sips`/`qlmanage`
rendering the repo's `assets/logo.svg` down from its native 668x702, not
build-time generated -- because that source SVG is 321KB of genuine but
auto-traced vector (526 single-color `<path>` fills, no embedded raster)
unfit to ship or inline as-is; `favicon.png` doubles as the header
wordmark's `.wordmark__mark` brand icon (`baseof.html`).

## Content-Security-Policy

Every UI response (`internal/api/ui.go`'s `newUIHandler`, spec Decision 6
in `docs/superpowers/specs/2026-08-28-web-ui-design.md`) carries:

```
Content-Security-Policy: default-src 'self'; script-src 'self' 'unsafe-eval'; style-src 'self'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'
```

Every directive is `'self'` (or `'none'` for `frame-ancestors`) because the
shipped site never loads a third-party origin: Alpine is vendored (no
CDN), `web/assets/ts/runtime/api.ts`/`runtime/token.ts` are the only modules that touch
`fetch` and only ever call this same origin's `/api/v1`, and the only
`<img>` reference (the header's `/favicon.png` brand mark, see Static
assets above) and the only stylesheet are both same-origin, served from
this same origin's `web/assets/css/main.css` and `web/static/`.

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

## Security: CodeQL accepted risk

CodeQL alert #1 (`js/clear-text-storage-of-sensitive-data`,
`setToken`'s `localStorage.setItem`, now in
`web/assets/ts/runtime/token.ts` after the v0.5.0 refactor) is
**dismissed as won't-fix**, not unaddressed. `PRODUCT.md`'s "Token-once,
same-origin" principle is the deliberate design this alert is flagging:
there is no server session, no cookie, no CSRF surface, and a single
pasted bearer token is the entire auth model for a single self-hosting
operator. Storing that token anywhere client-side trips this rule by
construction; the question this repo answered is whether the storage
location is an acceptable risk for the actual threat model, not whether
to avoid storing it at all.

Accepted because three things are all true at once:

1. **CSP is `self`-only.** `script-src 'self' 'unsafe-eval'`,
   `connect-src 'self'` (see Content-Security-Policy above) -- there is no
   third-party origin anywhere in the shipped site that could exfiltrate
   `localStorage` via an injected script, since nothing but this origin's
   own vendored/bundled JS ever runs.
2. **No `innerHTML`/`x-html` anywhere.** Every dynamic string renders via
   `x-text` (Alpine.js conventions above) -- there is no code path in this
   UI that turns untrusted text into markup, which is the mechanism an XSS
   payload would need to reach `localStorage` in the first place.
3. **LAN-only exposure.** `PRODUCT.md`'s Operating Context: a self-hosted
   instance on the operator's own network, not a public multi-tenant
   service -- the realistic attacker model is not "arbitrary internet
   script gets same-origin access," it's "someone already has a foothold
   on this LAN," at which point the token is one of many things already
   at risk.

Revisit if either premise changes: the dismissal comment itself notes SSO
fronting as a planned future direction, which would change the auth model
enough to reopen this question, as would this UI ever loading a
third-party script or gaining an `innerHTML`/`x-html` path.

## Provenance

Written for Task 8 of the web UI sub-project, from the shipped code and
the four prior task reports that recorded design decisions
(`.superpowers/sdd/2026-08-28-web-ui/task-{3,4,5,6,7}-report.md`). The
`impeccable` skill's `context.mjs` reported `WORLD_DISCOVERY_REQUIRED`
for this and every UI-touching task through Task 7 because this file
didn't exist yet; it now serves as the visual-world reference future
`impeccable` invocations against `web/` should find.
