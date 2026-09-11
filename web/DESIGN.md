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
- Authored motion is a short, named, closed inventory -- the token
  panel's open/close, the guide's trace draw-in and settle, and the live
  link's two flares. Everything else is a plain, short state change.
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

| Token                        | Light                 | Dark               | Role                                                                                                                         |
| ---------------------------- | --------------------- | ------------------ | ---------------------------------------------------------------------------------------------------------------------------- |
| `--color-bg`                 | `#eef2f0`             | `#0a0f0d`          | Page background                                                                                                              |
| `--color-bg-raised`          | `#ffffff`             | `#101715`          | Cards, panels, the header bar, table rows                                                                                    |
| `--color-bg-inset`           | `#e3e9e6`             | `#060a08`          | Recessed surfaces: inputs, hover rows, series-row blocks                                                                     |
| `--color-graticule`          | `#c7d2cd`             | `#1c2a24`          | The `.graticule` grid lines                                                                                                  |
| `--color-ink`                | `#16211c`             | `#dfeee6`          | Primary text                                                                                                                 |
| `--color-ink-muted`          | `#3f5148`             | `#8fac9f`          | Secondary text: hints, descriptions, muted labels                                                                            |
| `--color-border`             | `#b9c4bf`             | `#24352d`          | Static dividers, card borders                                                                                                |
| `--color-border-interactive` | `#5b6b63`             | `#5a7469`          | Borders on inputs, buttons, toggles -- anything clickable                                                                    |
| `--color-accent`             | `#0f6b3c`             | `#3ddc84`          | Primary action, "armed"/"ok" status, active nav underline                                                                    |
| `--color-accent-contrast`    | `#ffffff`             | `#06150c`          | Text/icons on an accent-filled surface                                                                                       |
| `--color-warn`               | `#7a5200`             | `#e8a33d`          | "Unarmed" token status dot; `.form-field__warning` (soft, non-blocking field warnings); the schedule picker's cron-lock note |
| `--color-danger`             | `#a3271f`             | `#ff6b5e`          | Errors, "unarmed"/"down" status, destructive actions                                                                         |
| `--color-backdrop`           | `rgb(22 33 28 / 45%)` | `rgb(2 6 4 / 65%)` | The token dialog's `::backdrop`                                                                                              |

`::selection`, the focus ring (`:focus-visible`), and the scrollbar
(`scrollbar-color` plus a WebKit `::-webkit-scrollbar*` fallback) are all
themed from these same tokens -- no browser-default gray leaks through
either palette.

**Tinted state surfaces (v0.5.0).** Warn/danger panels sit on
`--surface-warn` / `--surface-danger` -- `color-mix(in srgb,
var(--color-warn|danger) 8%, var(--color-bg-raised))`, overridden to a
10% mix in the dark palette -- instead of plain `--color-bg-inset`. The
`.problem` panel uses the danger surface; the warn surface carries the
guide's conflict vocabulary (ghost slots, the drop legend, and the
inspector's dropped-occurrence verdict). Contrast evidence for every
text pairing on these surfaces is in the WCAG section below.

## Token deltas (v0.5.0 bench rebuild)

Added in the v0.5.0 foundation slice (spec §4), all on `:root`:

| Token                                                     | Value                                  | Role                                                                                                                                                                                                       |
| --------------------------------------------------------- | -------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `--z-bezel` / `--z-sticky` / `--z-popover` / `--z-dialog` | 10/20/30/40                            | The z-scale; no ad hoc z-index values. Native `<dialog>` top layer sits above all of them for free.                                                                                                        |
| `--glow-accent`                                           | 3px accent ring (color-mix 25%)        | Names the armed-dot ring (status dots `armed`/`ok`).                                                                                                                                                       |
| `--shadow-dialog`                                         | the two-layer dialog shadow            | The one deliberate shadow, now named.                                                                                                                                                                      |
| `--icon-size`                                             | `1.125rem`                             | One size for every drawn icon; collapses the three drifted values (1rem/1.1rem/1.125rem).                                                                                                                  |
| `--measure`                                               | `68ch`                                 | Prose line cap (page-head/section-head/empty-state text).                                                                                                                                                  |
| `--content-max`                                           | `76rem`                                | The page column cap. The v0.5.1 guide is the one sanctioned full-bleed exception.                                                                                                                          |
| `--duration-slow`                                         | `250ms`                                | Reserved for the largest disclosures.                                                                                                                                                                      |
| `--surface-warn` / `--surface-danger`                     | color-mix tints                        | See Colors above.                                                                                                                                                                                          |
| `--sweep-trail` (v0.5.2)                                  | accent color-mix: 38% light / 20% dark | The sweep cursor's phosphor-persistence peak. Light prints denser: the 20% wash that reads on dark glass disappears on paper. Decorative (the 2px rule carries the reading), so no contrast floor applies. |

`--div` is load-bearing since v0.5.1: it IS the guide's 30-minute column
width, so graticule lines, ruler cells, and slot boundaries coincide.
The guide scopes `--div` up to `44px` (`.guide`) so an hour label fits
one division; the identity "one graticule square = 30 minutes" holds at
any `--div`, and every derived length (`--q-w` = `--div`/6 per 5-minute
quantum, `--px-per-min` = `--div`/30) follows the override
automatically. Any dynamic geometry keyed to it must go through CSSOM
(`el.style.setProperty`, Alpine `:style`) -- see Content-Security-Policy
below for why an inline `style` attribute silently fails.

## The guide grid (v0.5.1, full-week since v0.5.3)

The Guide (`/`, `web/layouts/index.html` + `web/assets/ts/pages/guide.ts`

- `web/assets/ts/runtime/grid.ts`) renders the EPG as home -- a FULL
WEEK per page since v0.5.3 (spec §3.1, second amendment). Its geometry
contract comes from the measured pre-slice spike (spec §9) and is
binding:

- **Sticky topology from FLEX FLOW, never grid membership.** The sheet
  (`.guide-sheet`) is a block with `width: max-content`; the ruler
  (`.guide-ruler`) is a sticky-top flex row with a sticky-left corner;
  each channel row (`.guide-row`) is a flex of sticky-left plate cell +
  day-segment tracks. Chromium forgives sticky grid items; Safari
  historically does not. Both axes scroll inside ONE scrollport
  (`.guide-viewport`, a bounded-height `overflow: auto` box) -- that is
  what makes top and left stickiness hold simultaneously.
- **One CSS grid per DAY SEGMENT**, `repeat(288, var(--q-w))` -- 288
  five-minute quanta per day, seven segments laid in a row per channel
  by the row's flex (`flex: none` each). The week is NEVER one
  2016-column grid: the spike measured the 288-column grid and that
  stays the building block. Slots are placed by grid-column line numbers
  set via CSSOM (`el.style.setProperty("grid-column", ...)`); the pure
  quantization (floor start, ceil end, clamp to the day, cut-left/right
  flags for spills) lives in `runtime/grid.ts`'s geometry section and is
  unit-tested (`web/tests/grid.test.ts`).
- **Two-tier week ruler (v0.5.3)**: a day-header tier
  (`.guide-ruler__days`, one `.guide-ruler__day` cell of exactly
  48 × `--div` per day: `SUN 30 · MON 31 · …`) over the hour cells, both
  tiers inside the ONE sticky flex row; the sticky-left corner spans
  both and reads the page's month(s) (`weekCornerLabel`: "AUG 2026",
  "AUG–SEP 2026"). Each day label (`.guide-ruler__day-label`) is
  `position: sticky; left: calc(var(--rail-w) + var(--space-5))` INSIDE
  its cell, so the day name stays readable while its day pans and slides
  away at the day's right edge. Today's header keeps the accent dot +
  `aria-current="date"` (the old day-tab idiom carried over).
- **The midnight rule (`--color-dayline`)**: day boundaries carry a
  slightly stronger graticule rule -- `color-mix` of
  `--color-border-interactive` at 60% -- through the ruler tiers, the
  tracks, and the quiet ground (a 48 × `--div` repeating gradient
  there). Always an inset `box-shadow`, never a border: segment widths
  stay exactly 288 × `--q-w`, and inset shadows paint UNDER grid items,
  so a slot joined across midnight covers the rule.
- **Joins vs cuts (v0.5.3)**: a slot crossing midnight INSIDE the
  rendered week renders one piece per touched segment, flush at the
  boundary -- `data-join="left|right|both"` zeroes the joined side's
  margin, border-width, and radius, so the pieces read as ONE continuous
  block (`segmentEdges` classifies; unit-tested). Only the widest piece
  carries the visible face (`primaryPieceIndex`, ties to the earlier
  piece -- a 23:30→06:00 slot labels its six-hour morning piece); the
  other pieces mirror the same content `visibility: hidden`
  (`.guide-slot--silent`) so the join stays step-free, and describe
  themselves via aria-label ("…, continues across midnight"). Dashed
  `data-cut` edges survive only at the WEEK's outer boundaries, where
  the slot really continues onto a neighboring page. Border removal on
  joins uses `border-*-width: 0`, not `border: none` -- the ghost hover
  flips `border-style` back to solid and a zeroed width must stay zero.
- **Ghost lanes stay uniform**: a channel with any ghost applies the
  two-lane `grid-template-rows` (`.guide-track--lanes`) to EVERY segment
  of its row, and the ghost lane is a FIXED 3.9rem row -- the row's flex
  stretch hands each segment's surplus height to its auto rows, so an
  empty auto lane would absorb a share the ghost-bearing segment's lane
  doesn't and step the airing lane's floor at midnight; with the lane
  fixed, every segment's airing lane lands at the same height and joins
  stay flush.
- **The now-line** (`.guide-nowline`) is an absolute overlay child of
  the sheet at `calc(var(--rail-w) + var(--now-min) * var(--px-per-min))`;
  `--now-min` is WEEK-relative since v0.5.3 (whole day segments before
  now × 1440, plus the minutes into now's own day clamped to its 288
  columns -- DST days keep their clamped geometry) and is set once per
  minute via CSSOM by a local 60s timer (a discrete step, not an
  animation loop). Since v0.5.9 that timer reads `bus.ts`'s
  `serverNow()`, not `Date.now()`, so the stream heartbeat's skew
  correction lands on the sweep -- the one line whose whole job is being
  where the SERVER thinks now is. It renders only on the week page
  containing now; other pages hide it.
  No scroll handler anywhere. The phosphor-persistence trail is a
  `::before` gradient riding the rule; reduced motion drops the trail
  and keeps the rule. Opening auto-scrolls to the sweep on that page (a
  third of the viewport in); every other page opens at its start.
  z-order inside the sheet: nowline (1) < plates (2) < ruler (3) <
  corner (4).
- **Ghost slots** (`.guide-slot--ghost`): a current-plan conflict
  warning renders as an amber-hatched `NO SIGNAL — LOST TO <block>`
  block at its would-have-aired time, in an implicit second track lane
  under the slot that displaced it. Hatching never carries the fact
  alone -- the text label does (SC 1.4.1). INTERIM: the Warning wire
  shape carries only names + `occurrence_start` until the Memory slice,
  so the ghost's channel and duration resolve client-side from the
  losing block's spec (`resolveGhost`, `runtime/grid.ts`).
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
  rule re-slotted between past and future rows each minute. Since
  v0.5.3 it pages with the SAME ‹/› week pager as the grid -- its day
  sections span the visible week page only, and headings stay
  window-absolute (TONIGHT is only ever the fetch day). Since v0.5.2
  the rundown's CHANNEL picker is the ONE channel control on mobile --
  the desktop SCOPE select hides (it would ask the same question twice;
  the plan stays all-channels), and the shell narrows with it: the nav
  becomes a single scrollable tab row with mask-image edge fades (spec
  §3.4 -- no wrap that orphans the last item), and the telemetry
  cluster drops its multimeter dividers for a quiet two-column readout
  grid (a wrapped row's leading divider dangles).
- **Full-bleed with ONE left rail (v0.5.2 alignment thesis)**: the
  guide is the ONE sanctioned `--content-max` exception --
  `content/_index.md` sets `full_bleed`, baseof adds `.content--bleed`,
  and the guide's chrome (head/toolbar) stays on the page column while
  the grid region escapes it. Since v0.5.2 the escape is one-sided:
  `.guide-body`'s left padding reproduces the centered column's own
  offset (`max(--space-5, (100% − --content-max)/2 + --space-5)`), so
  the pager and the grid's left edge sit exactly on the line the
  heading, intro, and toolbar start on, and the grid bleeds RIGHT
  toward the viewport edge only -- the trace runs off the glass, the
  reading spine stays put. Two unrelated left edges was the defect this
  replaces.
- **Week pager (v0.5.3, spec §3.1 second amendment)**: navigation is
  ONLY the ‹/› pager (`ui/icon` chevrons on `.btn--icon`) paging whole
  weeks -- the day tabs are gone (the grid shows the whole week, so a
  day selector had nothing left to select). The `.guide-pager__label`
  between the chevrons reads the visible page's calendar range
  (`weekRangeLabel`: "SUN 30 AUG – SAT 05 SEP"; a trailing one-day page
  is just its day) and is `aria-live="polite"` so paging announces.
  The client fetches the whole plannable window once (`days=28`) and
  pages it client-side: page k = window days k·7..k·7+6
  (`weekPageCount` / `weekChunk` in `runtime/grid.ts`, unit-tested,
  consistent with `windowDayCount` -- a mid-day fetch's 29th calendar
  day becomes an honest one-day fifth page). Chevrons carry aria-labels
  ("Previous week"/"Next week") and disable at the window edges; a
  chevron that disables itself under the pointer hands keyboard focus
  to its opposite. The pager stays on mobile, where it pages the
  rundown. That one `/schedule?days=28` call runs on the 90-second
  `LONG_GET_TIMEOUT_MS` read tier (`runtime/api.ts`) -- a cold-pod
  first plan can exceed a minute -- and the loading state says so
  (`.guide-loading-note`); every other read keeps the 15s default.
- **Keyboard over the segmented DOM (v0.5.3)**: the roving tabindex
  walks LOGICAL slots -- Left/Right cross the whole week on a track,
  and a cross-midnight slot is ONE stop whose focus rides its primary
  piece; Up/Down land on the nearest start across channels, unchanged.
- **Slot faces show content (v0.5.2, spec §3.1 amended)**: a series
  slot lists its programs on the face, one `.guide-slot__prog` line per
  program (`SHOW · SxxEyy`; a movie is just its title), folding
  anything past `FACE_MAX_LINES` (3) into one `+N MORE` line
  (`slotFace`, `runtime/grid.ts`, unit-tested). Filter blocks and
  ghosts keep the name + count face; a slot narrower than
  `FACE_MIN_DIVS` (3 divisions = 90 min) degrades to name + count
  regardless of type. Hierarchy holds: the block name stays the primary
  line, program lines are secondary at label scale, sentence-case --
  data, not legend. Program lines reuse the already-verified
  ink-muted-on-inset and ink-muted-on-series-tint pairings.
- **Quiet ground (v0.5.2)**: `.guide-viewport` carries
  `min-height: min(24rem, 100dvh − 16rem)`; the sheet flex-grows into
  it and the renderer appends a `.guide-ground` row (sticky empty rail +
  a flex track) whose background continues the graticule -- vertical
  divisions plus horizontal lines at the track pitch -- so a
  one-channel install reads as an instrument with unused traces, not a
  sliver over a void. The sweep line spans the ground too.
- **Ruler mask edge (v0.5.2)**: `.guide-ruler__corner::after` casts a
  short bg-raised fade over the cell strip, so an hour label scrolling
  under the sticky corner dissolves instead of slicing mid-glyph.
- **Draft mode (v0.5.6, spec §3.3)**: the Guide is also the surface
  that plans and applies, so two plans can be on the glass -- the
  READING (`GET /schedule?days=28`, always every channel, re-fetched
  after every apply) and a DRAFT (`POST /generate` for the SCOPE over
  `DRAFT_DAYS` = 7), rendered as a diff overlay on the same grid.
  `runtime/draft.ts` computes the verdicts and holds every line of
  draft copy; `pages/guide.ts` wires it.
  - **The draft zone**: the bar and its problem blocks live in
    `.guide-draftzone`, a direct child of the `.guide` root rather than
    part of the chrome above -- sticky is bounded by the containing
    block, and the chrome's box ends right under the bar, so APPLY and
    DISCARD scrolled off with it (worst on mobile, where the long
    rundown scrolls the page). The ZONE carries both the column
    geometry (`--content-max`, `margin-inline: auto`, the page's inline
    padding) and the pin, `top: var(--bezel-h, 4.5rem)`;
    `.guide-draftbar` itself is unpositioned. `--bezel-h` is published
    on the root element by `runtime/shell.ts` from a `ResizeObserver`
    on the bezel, so the bar sits under the real header at any wrap.
    The zone is `x-show`n away outright when no draft is armed and no
    problem is up: an empty sticky box must not reserve a strip under
    the bezel.
  - **Verdict vocabulary**: `data-draft` on the slot carries
    `new | changed | same | removed | beyond`. The text chip (`NEW` /
    `CHANGED` / `REMOVED`) carries the fact (SC 1.4.1) and is spoken as
    a `Draft <verdict> — ` aria-label prefix; the accent left edge is
    the scan aid. `same` slots dim to `0.5` while a draft is on the
    glass so the diff is the content; `beyond` (a reading slot starting
    past the 7-day horizon) renders plain -- never a chip, never a
    count, because the draft did not plan that far.
  - **Removed slots take the second lane** (`grid-row: 2`, the ghost
    lane) with a dashed `--color-danger` edge, the ghost hatch reversed
    (-45°), the block name struck through, and NO on-air glow: a slot
    the draft removes must not read as one that is airing. The
    rundown's twin is scoped
    `.guide-rundown[data-draft] .rundown-slot[data-draft="removed"]` so
    specificity beats the later `.is-past` / `.is-on-air` rules: a
    removed row that already ended must not dim, and must never carry
    the on-air stroke. Its edge is a danger `box-shadow: inset` rather
    than the grid slot's dashed border -- `.rundown-slot` sets
    `border: none`, so a border-color/-style pair alone would resolve to
    the initial `medium` width and frame the row in 3px dashes under the
    list's own 1px separator. Replacing `.is-on-air`'s inset stroke is
    also what drops it.
  - **Motion**: the trace draw-in (`clip-path` inset over
    `--duration-slow`) plays only when a draft replaces a sheet ALREADY
    drawn -- a SCOPE change or an Arm press -- never as an entrance on a
    first load or a `?draft` arrival; the settle is the committed sheet
    fading back after a discard. Both fill `backwards`, never `both`: a
    held `clip-path` clips the outline and the box-shadow with the box,
    which would cost a drawn-in slot its focus ring and an on-air one
    its glow for as long as the sheet lived. Reduced motion drops both
    animations and the data still lands. Under `forced-colors` the bar's
    tinted ground and the removed hatch drop out, so the armed edge
    becomes a `Highlight` stroke, the verdict edges keep their width,
    the removed grid slot keeps its dashed border, and the removed
    rundown row restates its stroke as a dashed `border-left` (shadows
    do not survive forced colors) -- chip and strikethrough already
    carry the fact.
  - **The reading mirror**: every landed reading is mirrored to
    `sessionStorage` (`schedularr_guide_reading`, per tab, skipped above
    2M characters) so a Blocks round trip (save → `PREVIEW ON GUIDE`
    → `/?draft=…`) can diff against what the operator last saw instead
    of paying for a fresh 28-day plan. It is a DIFF BASELINE ONLY: it is
    never painted as the committed grid (the skeleton holds the frame
    until the first draft lands), it is rejected when older than 24h or
    older than `Status.last_applied_at`, and both DISCARD and apply
    re-fetch.
  - **The honesty boundary**: nothing on the client knows Tunarr's
    current lineup, so the bar, the confirm dialog, and the inspector
    each word their verdicts and counts "vs the reading taken at
    HH:MM". No copy anywhere says a slot is in, or removed from,
    Tunarr.
  - **CSP**: `data-draft` is an ATTRIBUTE, not a style. The whole
    overlay is painted by attribute selectors in `main.css`, so draft
    mode adds no inline style anywhere; the one dynamic value
    (`--bezel-h`) goes through `el.style.setProperty` like every other
    geometry value in this system.
- The grid and rundown DOM are built in TS from the typed plan
  (`renderGuideWeek` / `renderRundown`); Alpine drives ONLY the toolbar
  (SCOPE + the `Arm draft` button + week pager + mobile channel picker
  -- the DAYS control died with the full-week reframe), the draft bar,
  and the inspector.

## The history page (v0.5.7)

`/history/` is the one searchable record. It replaced `/series/` and
`/dashboard/`, which were deleted outright -- no redirect stubs; the
404 page carries the nav. Nav became `GUIDE · BLOCKS · HISTORY`.

**Why one page and not two.** The industry models this as one record in
two states: a traffic log of what is scheduled and an as-run log of what
aired. Two routes would force a guess about which one holds the answer,
and both are backed by the same rows.

- **The band selector** (`.band` / `.band__tab`) is an instrument's band
  switch, not a pill group: flush tabs inside the same machined bezel as
  every other bordered surface, the selected band lit by an accent rule
  at its foot and a raised ground. The tab TEXT carries which pane is
  open; the rule and the ground are the scan aid on top of it, the same
  coded-legend rule the status dots follow. `role="tablist"` with roving
  `tabindex`, arrow/Home/End keys, and `:focus-visible` pulled inside the
  tab (`outline-offset: -3px`) because the tab itself is flush.
- **Deep links.** The open pane is the `?view=` query value
  (`tracked` / `asrun` / `runs`), rewritten with `replaceState` on every
  switch -- the address bar always names what is on screen, and one
  record's three panes are not three history entries to back through. An
  unknown or absent value resolves to `tracked` rather than a blank page.
- **The filter bar** (`.filter-bar`) reuses the `.form-field`
  vocabulary; only the layout is its own. Controls the visible pane does
  not honor are HIDDEN, not disabled: a dead control on an instrument
  reads as a broken one. The window is a server-side query on both
  feeds, so changing it refetches; every other control filters what is
  already in hand.
- **TRACKED** carries the whole of the old `/series/` page unchanged --
  the armed cursor edit behind Save, the instant completed/disabled
  toggles that print a tape line, the per-row 404 recovery.
- **AS-RUN** renders inside the guide's own `.rundown-day` /
  `.rundown-list` day grouping rather than a parallel list system, so
  the two surfaces read as one instrument. Rows show local time, the
  channel plate, the title, the block, and the duration -- programme
  names, never the program UUID the old dashboard table showed.
  Bucketing is by LOCAL day, newest day first, air order within: an
  operator reads a station log in station time.
- **RUNS** are cards where airings are rows, on purpose: a run is a
  discrete event with an outcome to weigh, an airing is one line of a
  log to scan. Status is a 1px border colour plus a tinted ground, never
  a thick colored rail, and the outcome word beside the timestamp
  carries the fact. Warnings expand in a native `<details>` -- keyboard
  operable with no Alpine involved. Each warning line is a flex row of
  discrete elements, because the minifier collapses the whitespace
  between inline siblings and a sentence built from `x-text` spans would
  otherwise read "Late Moviedropped for Evening News".
- **Honesty in the copy.** Every empty state names the real limit: runs
  are never backfilled, and each window is bounded by its own table's
  retention knob, so a 90-day view can legitimately come back short. No
  copy implies the store lost anything.
- **A run that never finished** reports only what it attempted (window
  and scope). Its slot and channel counts never landed, and printing
  zeros would read as "applied nothing" rather than "never got that
  far".

## The blocks row (v0.5.10)

The blocks list is the page's primary scanning surface, and the block
power tools slice handed it four more facts to carry: how long a block
runs, when it next actually airs, where it ranks against the other blocks
on its channel, and whether it is sitting dark.

**Four facts, one new column.** A column per fact would have taken the
table to ten. Measured against the real longest values -- a 21-character
block name, cronstrue's `At 09:00 PM, only on Saturday`, a
`Dark until 09/12/2026, 06:00` chip -- that row overflows `--content-max`
on any laptop, which turns the one surface an operator scans into a
horizontal scroller. Three of the four facts went INSIDE a cell that was
already answering their question instead:

| Column               | Carries                                                                     |
| -------------------- | --------------------------------------------------------------------------- |
| `Name`               | the block name                                                              |
| `Type`               | the `.badge` (`FILTER` / `SERIES`)                                          |
| `Schedule`           | the cron expression, its **duration** beside it, cronstrue's readback under |
| `Next`               | the countdown, with the absolute local instant under it                     |
| `Channel · Priority` | the `.plate`, with the **priority rank** under it                           |
| `Status`             | the enabled `.toggle`, with the **DARK UNTIL** chip under it                |
| `Actions`            | Edit / Duplicate / Go dark (`Bring back` while dark) / Delete               |

Each pairing is a claim about where the fact belongs, not a space-saving
dodge. Duration annotates the expression it qualifies -- `0 21 * * 6` and
`3 h` are one sentence an operator says out loud. Priority rank is
channel-scoped, so `2nd of 5` only means anything beside the channel
those five blocks contend on. The dark window qualifies the switch, and
sits under it rather than replacing it because `enabled` and
`disabled_until` are independent axes: hiding the toggle would hide the
control that ends a dark window for good.

Only `NEXT` took a column outright, because no cell was answering "when
does this actually air next".

Every stack is an inner div. `display: flex` on a `<td>` strips the
cell's table display and floats its bottom border at content height
instead of the row edge -- the rule the cron cell has followed since
v0.5.2, now applied four more times.

**An absent instant is three different facts.** `next_occurrence` is
absent when the block is disabled, when its cron will not parse, and when
the cron parses but never comes round. An em dash covering all three
hides the only thing worth knowing: which one applies, and whether it is
the operator's to fix. Each gets its own legend plus the step that
recovers it, and the cell drops out of value voice into the uppercase
legend voice the badges and empty-state legends already use, so
`DISABLED` cannot be misread as a time:

| `data-kind`  | Legend            | Line under it                               | Ink   |
| ------------ | ----------------- | ------------------------------------------- | ----- |
| `instant`    | `in 3 d` / `due`  | `09/12/2026, 21:00`                         | ink   |
| `disabled`   | `DISABLED`        | Enable the block to schedule it.            | muted |
| `unreadable` | `CRON UNREADABLE` | Fix the expression to schedule this block.  | warn  |
| `never`      | `NEVER FIRES`     | No date matches this cron.                  | warn  |

Amber marks the two the operator has to fix; a disabled block is a choice
they already made, so it stays muted. The legend text carries all three
regardless (SC 1.4.1).

`untilTime`, not `relativeTime`, for the present case, and for the same
reason the bezel's NEXT TICK uses it: the server computes
`next_occurrence` before the block starts airing, so an occurrence
already under way leaves a past instant on a perfectly fresh row.
`12 min ago` there reads as a missed airing; `due` reads as one in
progress.

**The `unreadable` branch reads cronstrue, and that is still not cron
evaluation.** cronstrue renders prose and computes no instant, which is
the one thing the client is allowed to do with a cron expression (spec
§10). It is also the same signal the Schedule cell two columns over
already uses to decide whether to print a readback, so the two cells
cannot disagree about whether the expression can be read at all. Should
the two parsers ever disagree the other way -- cronstrue reading an
expression the server refused -- the row falls through to `NEVER FIRES`,
whose recovery line points at the same cron.

**Every relative reading is measured against `serverNow()`**
(`runtime/bus.ts`), never `Date.now()`, and against a `now` held in
component state that a local 60s timer advances. The field is what makes
the column reactive at all: Alpine only redraws a row when reactive state
it read has changed, so a `NEXT` computed from a bare `serverNow()` would
freeze at whatever the clock said when the list last loaded. The timer is
local for the same reason the guide's sweep is: it has to keep turning on
POLL and on LINK LOST, so it can never ride a stream frame.

**The DARK UNTIL chip** is `.badge[data-state="dark"]` -- the type
badge's shape, in warn. Warn and not danger: a dark block is degraded,
not broken, and comes back by itself, the same distinction `POLL` takes
from `LINK LOST` on the link ladder. It paints only while
`disabled_until` is in the FUTURE. A passed wake-up suppresses nothing,
and a chip there would report a state the server does not hold; the same
predicate suppresses the priority rank, so the chip and the rank cannot
end up disagreeing about one instant.

**Rank suppression is the call site's rule, not the helper's.**
`runtime/rank.ts`'s `priorityRank` counts every ENABLED peer, dark ones
included, and the guide's inspector reads the identical function -- a
second opinion here is exactly the drift the extraction ended. What this
page decides on its own is whether to PRINT a rank: a disabled or
currently-dark block shows `PRI 50` bare, because it is not contending
for airtime right now and announcing a placing in a contest it is not in
would be a reading that lies. `of === 0` (the blocks fetch failed, or
every peer is disabled) prints the bare number too, never `1st of 0`.

**The row's four actions.** `Edit` / `Duplicate` / `Go dark` / `Delete`,
all in the one `.row-actions` div, because the row has no spare column and
a second switch parked in the `Status` cell would have made that cell a
three-deck stack. The dark button renames itself to `Bring back` on a
block that is currently dark: one control, each state naming the action it
actually performs from there -- and each state performing it. `Go dark`
opens the wake-up picker, because a window has an instant to choose;
`Bring back` is the write itself, straight to `PATCH` with a null, because
ending one has nothing to choose. Routing the second through a panel of
re-schedule presets made the label a claim the button did not honour.

Both power tools go `:disabled` whenever a row action is in flight or the
editor panel is open (`canArmRowAction`, `pages/blocks.ts`). Two different
failures, one rule. `pendingId` is a single global slot, so a second write
armed over the first is dropped silently by its own re-entrancy guard,
after the click. And an open panel has to stay undisturbed: Duplicate
takes the editor away from whatever is half-typed in it, while the dark
write splices a server record into `this.blocks` -- which is where
`submit()` reads its `If-Match`. If that block moved elsewhere since the
list loaded, the returned record carries the *other* edit's `updated_at`,
the open editor's next save is re-armed against a record nobody on this
screen has seen, and it succeeds. That is the lost update
`planInvalidatedReaction` freezes the list to prevent, arriving through a
row action instead of through a frame.

**The dark window is a `<dialog>`, not a popover** -- and it opens on one
path only, the `Go dark` half of that button. `--z-popover` carries
exactly one thing in this system -- the guide's mobile bottom sheet -- and
a choice of three presets does not earn a third overlay vocabulary. Both
power tools use `dialog.panel` verbatim, the same element the confirm
idiom uses, which is also where their focus trap, Escape handling and
focus return come from.

Four dialogs ship on this page, and each of the three the page authors
itself carries its **own `x-ref`** -- `duplicateDialog`, `darkDialog`,
and `cronDialog` (the mid-run change below) -- alongside the
`ui/confirm.html` instance Delete arms, whose `x-ref="confirmDialog"` the
partial hard-codes. That hard-coding is the whole constraint: a second
element claiming `confirmDialog` wins it and quietly leaves Delete arming
a dialog it no longer points at, so a page needing a second modal writes
its own element rather than instantiating the partial twice.

The presets ARE that dialog's commit controls: each writes the wake-up
printed on it, so `Cancel` is the only button in the action row. There is
no `Bring back now` beside it, because a block that is already dark never
opens this dialog.

Every preset prints the instant it commits to (`Tomorrow` ·
`09/11/2026, 00:00`). "Next week" is not a choice until the operator can
see which day it lands on, and that instant is exactly what the server
stores. The label keeps the button's uppercase legend voice; the instant
beside it reads as a value, the same split the row's own cells make.
Presets resolve on the **wall clock**, from `serverNow()`: local midnight
N days on, stepped through the local date fields, so "tomorrow" at 23:59
is a minute away rather than a day, and a DST night cannot move a wake-up
by an hour. The three are whole days (1 / 7 / 28) -- an "in a month"
preset would have to answer what the 31st of January plus one month is,
and JavaScript answers "the 3rd of March".

**Duplicate is one click; the dialog is the collision path.** The name is
pre-filled `Copy of <source>` and sent immediately. The copy arrives
disabled, so its row reads `DISABLED` the moment it appears, and the
editor opens on the record the *server* returned -- which is what arms
`If-Match` against the block that actually exists rather than a
client-side guess at it. A `409` is a normal outcome, not a failure: the
naming dialog comes up carrying the server's own reason with the name
selected, and the operator retypes and sends again. Nothing appends a
counter, for the same reason the server refuses to invent a name: only the
operator can see what the other block is.

**The mid-run change is the fourth dialog, and the one with two ways to
say yes.** A series block's cron is not only when it airs -- it is how
often the cursor advances, so once a show has aired under one expression,
saving a different one changes which episode lands on which date.
`initializeSeriesState` (`internal/scheduler/engine.go`) applies a row's
`start_season`/`start_episode` ONLY while `last_aired` is nil; after that
the stored cursor decides, and the block's own start position is ignored
for good. So the confirm says that out loud before the write and offers
the one repair: putting each cursor back where the block says it starts.

It is a fourth `<dialog class="panel">` (`x-ref="cronDialog"`) rather than
a second `ui/confirm.html` for two reasons that compound. The partial
hard-codes `confirmDialog`, so a second instance takes the ref away from
Delete; and it offers exactly ONE action, while this choice has two that
both save -- with the cursors rewound or without. Multiplexing Delete's
dialog behind a kind discriminator would grow the shared partial a second
confirm button for the guide and the kit as well, to buy a body its single
`<p>` could not carry either.

The copy is composed in `footgunCopy` (`pages/blocks.ts`), not in the
template: both the count and each cursor are facts the operator decides
on, and a template expression is the one place they cannot be pinned by a
test. Each show gets BOTH readings -- where it is and where a rewind sends
it -- as plain sentences rather than `S02E05 → S01E01`: an arrow is
notation this UI never taught anywhere else, and a modal read once is the
worst place to introduce one. The plain save is the primary action,
because carrying on from where each show stands is what the operator asked
for by editing the cron; the rewind is the offer beside it. Neither save
path carries a busy binding -- both close this dialog before the write
starts, and the editor's own Save button already shows the in-flight
state. A rewind is one `PATCH /state/series/{show_title}` per show, so
`rewindReport` prints what moved and what did not as two separate tape
lines: one "Cursors rewound" over a partial failure is exactly the silent
half-success this page refuses everywhere else.

**Responsive.** The table keeps `.table-wrap`'s own horizontal scroll --
the system's answer for every table since Task 4 -- rather than
restructuring into cards at a breakpoint, which would be a second row
vocabulary competing with this one. Two caps keep the columns honest
before it comes to that: the readback line is capped at `24ch` and the
`NEXT` detail line at `26ch`, so cronstrue's longest sentence wraps
instead of setting the column width for every row in the table.

## The block editor (v0.5.10)

The same slice that gave the row four facts gave the editor two shapes it
did not have: a consequence rail under the form, and a disclosure on every
series row. Both exist because the panel had grown past what one screen
answers.

**The consequence rail (`.editor-rail*`) answers "what will this do"
before Save.** It is the LAST section of `.panel__body`, directly above
the commit controls, because that is where such a reading is actually
read -- on the way to Save, not at the top of a form nobody has filled in
yet. It is a `.form-section` like every other block in the panel
(border-top, title, spacing all reused); only the readout cluster inside
it is new.

Three labelled readouts, in the bezel telemetry strip's register rather
than three cards: no per-group border, no icon, no fixed shape -- one
legend over one reading, three times, each a different length. The groups
sit on `repeat(auto-fit, minmax(15rem, 1fr))` so a narrow panel drops them
to one column instead of squeezing an instant into six characters.

| Group             | Reads                                                                                |
| ----------------- | ------------------------------------------------------------------------------------ |
| `Next three`      | the next three occurrences, each closed by the block's own duration                  |
| `On this channel` | the channel plate, then every enabled peer contending on it, priority order          |
| `Lineup`          | (series blocks only) the series rows in airing order, one line each                  |

Every instant in the rail is the SERVER's, from `GET /cron/next`. The
client still never evaluates cron (spec §10); the readback two fields up
is cronstrue rendering prose about the expression, which is a different
thing from computing a date. The one piece of occurrence math that stays
on this side is adding the block's duration to a start the server already
gave -- no calendar knowledge involved -- and the end prints as a bare
clock while it lands on the start's local day, as a full instant when it
does not: a block running 23:30 to 01:00 would otherwise read as ending
ninety minutes before it starts, and `+1` is notation this UI has never
taught anyone.

The occurrence group has five states and `railState()` decides between
them once, in TS, so the precedence is pinned by a test rather than by
four `x-show` expressions racing in a template:

| `railState()` | Shows                                                    |
| ------------- | -------------------------------------------------------- |
| `loading`     | `ui/skeleton` (`stack`, 3) -- never a spinner            |
| `error`       | the server's own reason, in `.form-field__error`         |
| `prompt`      | `Set a schedule to see when this block airs.`            |
| `never`       | `No date matches this cron.`                             |
| `occurrences` | the three instants                                       |

`never` is a state and not an empty list because a well-formed expression
that never comes round (February 30th) returns an EMPTY array rather than
a 400: the expression is fine and the answer is genuinely "never". An
empty rail there would report it as "nothing yet", the one reading an
instrument must not give. Its sentence is deliberately the same one the
list's `NEXT` column gives a never-firing block -- one cron, one answer,
whichever surface the operator is looking at. `error` outranks everything
for the same reason: an empty rail under a cron the server rejected is a
silent failure.

The readout is `role="status"`, polite and never assertive: the rail
re-reads on a pause in typing (Alpine's own `.debounce`, one listener on
the schedule field, because `input` bubbles from all six controls that can
change the cron), and an assertive region would interrupt the field the
operator is still in.

The channel group's list is ordered by priority, so the POSITION in it is
the rank -- there is no second computation of a rank here to disagree with
the row's own `PRI` cell. The block being edited stands in the field with
the FORM's values, not the stored record's: a priority typed a second ago
would otherwise be missing from the very list that shows what it does,
while the superseded one sat there looking current. It marks itself in
TEXT (`Typo Block (this block)`, or `This block` before it has a name);
`data-self="false"` receding to muted ink is the scan aid on top of that,
never the fact (SC 1.4.1). A blank channel returns nothing at all, because
a lone "this block" entry reads as "nothing else contends", which is a
different claim from "you have not said where this goes yet".

`.editor-rail__list` caps at `9rem` and scrolls. A channel carrying a
dozen blocks would otherwise push Save off the bottom of a laptop screen
with a list nobody asked to read in full.

**The series row is its own disclosure (`.series-row__toggle`).** A dozen
expanded shows is what made this editor unreadable, and the summary line
is what makes a closed row findable again. The row's head is the control:
chevron, `Series N` index, and one summary line.

**The disclosure is a `<button aria-expanded>`, not
`<details>`/`<summary>`** -- recorded here because it was a real fork.
Reorder and Remove have to stay reachable while the row is closed, and
they sit on the same head line. Put that head in a `<summary>` and those
three controls nest inside the one control that opens the row: HTML's
content model forbids interactive descendants there, and a click on Remove
would toggle the row on its way through. Splitting the head so the summary
holds only the label costs the summary its whole width, which is the part
that makes a closed row findable. The button carries
`aria-controls` pointing at the field grid, and the grid is hidden with
`x-show`, never a `<template>`: the fields keep their `x-model` bindings
while the row is closed, so collapsing a row never touches what is in it.

The toggle is chrome-free on purpose -- no border, no background, `font:
inherit` -- because it is the row's own heading, not a button parked next
to one; it leans on the global `:focus-visible` ring. One chevron in two
orientations carries the open state (`rotate(-90deg)` when closed, a
`--duration-fast` transition), which is why it can share a glyph with the
move buttons at the row's other end without reading as a third of them.

The summary comes from `seriesRowSummary`, the SAME function the rail's
lineup prints, so a row and the projection of it can never describe one
show differently. Both required fields count as identity: a row with no
show title has nothing to be called, a row with no episode count is not
yet a schedule, and either summarises to what is missing in warn voice
(`data-state="incomplete"`, on the closed line and on the rail's lineup
entry alike). Collapsing a dozen rows must not be able to hide the one
that will 400 on save. Findable beats complete -- the title still leads
when there is one, because `Breaking Bad — add an episode count` points at
a row and "row 3 is incomplete" makes the operator open all twelve.

### The deletion desk (v0.5.12)

Three additions to `/history/`, all of them destructive or about what
destruction would touch. The rule governing every one: **a dry run runs
first, and the confirm names the number it returned.** A dialog that
guessed would be a dialog the operator confirms against a number that
was never true.

- **The storage strip** sits above the band selector, because it
  describes the whole record rather than one pane. It reports ROWS, never
  policy: retention says what should age out, and a database whose
  retention was widened holds whatever it holds. Sentinels — occurrences
  that were committed and aired nothing — are counted apart from airings
  and carry their explanation inline, because the strip is where an
  operator first meets the word. A failed read degrades to one muted line
  rather than blanking the page; the panes below are independent.
- **A blocked row says so before the click.** A removal refuses while any
  block still lists the show, so the row names those blocks and links to
  each one instead of offering a button that would 409. That is the first
  half of a two-step flow, not a replacement for the server's guard: the
  page reads `GET /blocks` itself, and a failed read leaves the button
  offered so the refusal can still land. Hiding the action on a failed
  read would strand the operator.
- **Range cleanup lives inside the pane whose rows it deletes**,
  collapsed behind a disclosure. A destructive control does not belong
  permanently open above a log. Opened, it sits on `--surface-danger` so
  the ground itself carries the warning, inherits the pane's own channel
  and block filters so the preview counts the list being looked at, and
  prefills its window from the strip's stored bounds so it opens on
  something pickable rather than two empty fields.
- **The copy names the surprise.** An emptied occurrence keeps a marker
  saying it aired nothing, which is unexpected until it is explained, so
  both the panel and the confirm explain it rather than leaving the
  operator to discover a row they cannot delete.

## Typography

One family everywhere: `var(--font-mono)`, a `ui-monospace` stack with
named fallbacks (SF Mono, Cascadia Code, JetBrains Mono, Menlo, Consolas,
Liberation Mono) ending in generic `monospace`. Base line-height is
`--leading-normal` (1.55) for body copy; tight headings and status text
use `--leading-tight` (1.2). There is no separate serif/display face --
size, weight, and letter-spacing carry hierarchy instead.

| Token         | Size       | Used for                                                                                                    |
| ------------- | ---------- | ----------------------------------------------------------------------------------------------------------- |
| `--text-xs`   | `0.75rem`  | Labels, legends, table headers, badges, hints -- almost always paired with `--tracking-label` and uppercase |
| `--text-sm`   | `0.875rem` | Secondary body text, table cells, form hints/errors                                                         |
| `--text-base` | `1rem`     | Default body text                                                                                           |
| `--text-md`   | `1.125rem` | Panel/dialog headings (`.panel__head h2`)                                                                   |
| `--text-lg`   | `1.375rem` | Section headings (`.section-head h2`)                                                                       |
| `--text-xl`   | `1.75rem`  | Page headings (`.page-head h1`)                                                                             |
| `--text-2xl`  | `2.25rem`  | The hero-panel's own `h1` (dashboard status card container)                                                 |

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
and wide content (the history/blocks/series tables) scrolls on its own
axis via `.table-wrap { overflow-x: auto }` rather than letting the page
scroll horizontally.

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
accent, the history/blocks/series table header's separator line, the
focus ring.

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
states appear there. It is no longer only the `ui/*` partials -- a
page-level cluster with states of its own is a fixture too, which is what
the blocks row, the editor's consequence rail -- in both its answered and
its capped-and-scrolling states, since the 9rem cap is the reason those
lists carry `tabindex` -- the series-row disclosure, and all three
own-`x-ref` dialogs are doing there: Dark window (idle and sending), Name
the copy (clean and name-taken, because the 409 is the whole reason that
dialog exists), and Mid-run change. It is excluded from production builds via
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
  appears without adjacent text naming the state. Three vocabularies
  share the same `data-state` attribute and the same three colors: the
  token panel's `armed`/`unarmed`, every live reading's `ok`/`down`
  (Tunarr signal, blocks/series channel fallbacks, the 404 page's "No
  Signal"), and the live link's `live`/`poll`/`lost` (v0.5.9). `unknown`
  -- the pre-JS initial value on the token trigger and on both bezel dots
  -- falls through to the base muted color.
- **`.problem`** -- the inline API-error panel (`.problem__title` +
  `.problem__detail`), used identically on every page that fetches on
  load: dashboard status/history, blocks list/editor, the guide's
  reading / draft / apply, series list. Always rendered next to the
  section that failed, with a `Retry`-equivalent action, per
  `PRODUCT.md`'s "No silent failures."
- **`.skeleton-bar` / `.skeleton-row` / `.skeleton-stack`** -- a muted
  pulsing block (1.2s ease-in-out, frozen to static under
  `prefers-reduced-motion` via the global reset) for every page's
  initial-load state.
- **`.table-wrap` + `.history-table`** -- the one table shape in the
  system, introduced on the dashboard (Task 4) and reused verbatim,
  unmodified, by blocks (`blocks-table__*` only adds cell-content
  wrappers) and series (`series-table__show`/`.series-cursor`). Bordered
  wrapper with its own horizontal scroll, `--border-width-thick` header
  rule, tabular-nums on numeric/timestamp columns.
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
- **`.editor-rail` / `__group` / `__legend` / `__readout` / `__list` /
  `__item` / `__note`** (v0.5.10) -- the editor's consequence rail: an
  auto-fit grid of labelled readouts in the telemetry strip's register,
  last in `.panel__body`. `__item` takes `data-self` (the edited block
  stays in full ink, the rest recede) and `data-state="incomplete"` (warn,
  the same voice the collapsed series row uses for the same row);
  `__list` caps at `9rem` and scrolls so a busy channel cannot push Save
  off the screen. Every other shape in it is reused verbatim --
  `.form-section`, `ui/skeleton`, `.form-field__error`, `ui/plate`. See
  "The block editor" above.
- **`.series-row__toggle` / `__chevron` / `__index` / `__summary`**
  (v0.5.10) -- the series row's disclosure. A chrome-free
  `<button aria-expanded>` that IS the row's heading (no border, no
  background, `font: inherit`, the global focus ring), not `<details>` --
  Reorder and Remove share the head line and cannot nest inside a
  `<summary>`. One chevron rotates; `__summary[data-state="incomplete"]`
  goes warn so a closed row cannot hide a field that will 400 on save.
- **`.dark-presets` / `.dark-preset` / `.dark-preset__when`** (v0.5.10) --
  the dark-window presets. Each is a full-width `.btn` with the instant it
  commits to pushed to the far edge: `__when` steps out of the button's
  uppercase legend voice into a value's (normal tracking, sentence
  weight, muted ink), the same split the blocks row's own cells make
  between a legend and a reading. No new colours -- the muted-on-raised
  and muted-on-inset (hover) pairings are in the WCAG evidence below.
- **`.hero-panel` + `.graticule`** -- the bordered "instrument surface"
  primitive (Task 3's landing placeholder), reused as-is by the
  dashboard's status card, the guide's NO SIGNAL blackout, and the 404
  page's own "No Signal" readout. `.graticule` is a repeating-gradient
  grid background, applied wherever a surface needs the literal
  measurement-grid texture.
- **`.badge`** -- the blocks list's type indicator (`Filter`/`Series`);
  text carries the fact, `data-type="series"` adds an accent tint as a
  secondary scan aid only. `data-state="dark"` (v0.5.10) is the same
  shape in warn, carrying the blocks row's `DARK UNTIL <instant>` -- see
  "The blocks row" above.
- **`.toggle`** -- a `role="switch"` button styled as a small rocker
  (not an iOS-style pill, matching the small-radii shape language), used
  for both blocks' Enabled/Disabled and series' Completed/Disabled
  toggles.
- **`.telemetry`** (v0.5.0) -- the bezel telemetry strip: LINK, TUNARR
  (both coded-legend dot+text) plus LAST APPLY / NEXT TICK relative
  readouts on every page, fed by `runtime/shell.ts`. LINK sits FIRST
  because it qualifies the other three: off the LIVE rung those numbers
  are last-known, not live, and that has to be read before them rather
  than after.
- **The LINK legend** (v0.5.9) -- the live link's degradation ladder as a
  bezel readout, `LIVE` / `POLL` / `LINK LOST`. The `data-state` values
  are `bus.ts`'s `LinkState` written through verbatim (`live`, `poll`,
  `lost`), so no name-mapping table sits between the runtime and the
  stylesheet to drift. `POLL` takes the warn slot rather than danger --
  the page still shows current data, it has just gone back to refetching
  instead of listening; degraded, not broken. `LINK LOST` is the one
  readout in the system that grows a control: a real `Reconnect`
  `<button>` (`.telemetry__reconnect`, styled `.btn btn--sm` rather than
  ghost, whose border clears the 3:1 non-text floor in both palettes).
  Its visibility is CSS off the dot's own `data-state` via the general
  sibling combinator, never a second attribute from JS -- one writer per
  transition, and no way to end up with a Reconnect button beside a LIVE
  legend. `display: none` keeps it out of the tab order while the link is
  up. Both halves of the gate are scoped under `.telemetry` for weight,
  and that scope is not cosmetic: the button also carries `.btn`, whose
  own `display: inline-flex` is a bare class selector declared LATER in
  the stylesheet, so an unscoped `.telemetry__reconnect { display: none }`
  ties on specificity and loses on source order -- which is exactly how
  the button shipped visible in every link state during the first cut of
  this slice. The value carries `role="status"` (TUNARR's does not): a
  screen reader that never heard the state change would never go looking
  for the button that appeared next to it.
- **The two link flares** (v0.5.9) -- this slice's entire motion budget:
  one 200ms amber ring when the link drops, one green when it is
  reacquired, both `box-shadow`-borne off `@keyframes link-flare`. They
  live on a separate `data-pulse` attribute, not on `data-state`, for two
  reasons: an animation keyed to a state selector fires the first time
  that selector matches, which for `live` is the ordinary happy-path
  connect a second after every page load (spec §5 bans motion on initial
  load); and the attribute has to be cleared -- `shell.ts` clears it on
  `animationend` -- for a second flare of the same kind to retrigger at
  all. No `fill-mode`, so the ring reverts to the state's own box-shadow
  (the accent glow, or nothing) the instant the 200ms is up. Suppressed
  by an explicit `animation: none` under `prefers-reduced-motion` (the
  global 0.01ms collapse still RUNS the keyframes, and their final frame
  would blink the live dot's glow off and on) and dropped under
  `forced-colors`, which strips shadows outright -- there the legend text
  and the appearing Reconnect button carry the transition.
- **`.plate`** (v0.5.0) -- the channel legend plate; see the partials
  section above.
- **`.tape`** (v0.5.0) -- the event tape; success is printed, not
  toasted. The newest line takes a 150ms print draw-in (suppressed under
  reduced motion); older lines recede to muted.
- **`.section-head--row`** (v0.5.2) -- the section-head variant that
  seats a section's one primary action on the header line (heading +
  description left, button right; stacks under 640px). Replaced the
  blocks page's `.blocks-toolbar`, whose lone right-aligned button over
  an empty band read as a dead row.
- **`.empty-state` width** (v0.5.2) -- the container spans the full
  content rail, matching the tables and panels it stands in for (the
  old `--measure` cap left it arbitrarily narrower than the hero above
  it); only `.empty-state__text` keeps the measure.
- **`.series-cursor`** (v0.5.2) -- baseline-aligned at a `--space-2`
  gap: the S/E prefixes share the text baseline of the input values and
  the Save label instead of floating centered against taller boxes.

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
  (`--duration-fast`/`--duration-base`) unless the moment is already in
  the authored inventory named in the Overview. Respect
  `prefers-reduced-motion` -- the global reset collapses durations, but a
  box-shadow or `clip-path` animation still needs its own explicit
  `animation: none`, because at 0.01ms the keyframes run and their final
  frame lands.
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
- **Don't** set `display: flex` (or `grid`) on a `<td>`. It strips the
  cell's table display, so its bottom border floats at content height
  instead of the row edge -- put the flex stack on an inner div
  (`.blocks-table__cron`, `.series-table__show` post-v0.5.2).

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

| Pairing                                                                           | Light   | Dark    |
| --------------------------------------------------------------------------------- | ------- | ------- |
| `--color-danger` text on `--color-bg-inset` (`.problem__title`)                   | 5.95:1  | 7.13:1  |
| `--color-ink-muted` on `--color-bg-raised` (detail text, empty-state, table `th`) | 8.46:1  | 7.42:1  |
| `--color-ink` on `--color-bg-raised` (table body text)                            | 16.56:1 | 15.15:1 |
| `--color-border-interactive` on `--color-bg-raised` (table header rule, non-text) | 5.63:1  | 3.58:1  |

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

| Pairing                                                          | Light          | Dark           |
| ---------------------------------------------------------------- | -------------- | -------------- |
| `--color-warn` on `--surface-warn` (warnings title)              | 6.14:1         | 7.15:1         |
| `--color-ink-muted` on `--surface-warn` (warnings list)          | 7.51:1         | 6.30:1         |
| `--color-danger` on `--surface-danger` (`.problem__title`)       | 6.43:1         | 5.77:1         |
| `--color-ink-muted` on `--surface-danger` (detail + REF lines)   | 7.43:1         | 6.58:1         |
| `--color-accent-contrast` on `--color-danger` (`.btn--danger`)   | 7.32:1         | 6.71:1         |
| `--color-ink` on `--color-bg-inset` (plate name)                 | 13.45:1        | 16.60:1        |
| `--color-ink` / `--color-ink-muted` on `--color-bg` (tape lines) | 14.66 / 7.49:1 | 16.11 / 7.88:1 |

Every pairing clears WCAG AA's 4.5:1 text floor (worst case 5.77:1). The
telemetry strip's label/value inks on `--color-bg-raised` are the
already-verified Task 3/4 pairings.

**v0.5.1 guide** introduced the slot faces, the series tint, and the
ghost hatching. Checked computationally (same throwaway-script
convention):

| Pairing                                                         | Light   | Dark    |
| --------------------------------------------------------------- | ------- | ------- |
| `--color-ink-muted` on `--color-bg-inset` (slot meta line)      | 6.88:1  | 8.13:1  |
| `--color-ink` on the series tint (7% accent mix over bg-inset)  | 12.20:1 | 15.09:1 |
| `--color-ink-muted` on the series tint                          | 6.23:1  | 7.39:1  |
| `--color-warn` on `--surface-warn` (ghost text, base surface)   | 6.14:1  | 7.15:1  |
| `--color-warn` on the 18% hatch stripe (ghost text, worst case) | 4.71:1  | 5.00:1  |

Every pairing clears the 4.5:1 AA floor; the ghost's worst case (text
over a hatch stripe) is the tightest at 4.71:1 light. Ruler labels,
rundown rows, and the inspector reuse already-verified token pairings on
`--color-bg-raised`.

**v0.5.6 draft mode** introduced the verdict chip on both slot grounds
and the removed slot's danger-on-hatch pairings. Checked computationally
(same throwaway-script convention; the hatch stripe is a 14%
`--color-danger` mix over `--surface-danger`, the worst case a chip can
land on):

| Pairing                                                         | Light  | Dark    |
| --------------------------------------------------------------- | ------ | ------- |
| `--color-accent` chip on `--color-bg-inset` (`NEW` / `CHANGED`) | 5.35:1 | 11.16:1 |
| `--color-accent` chip on the series tint (7% accent mix)        | 4.85:1 | 10.15:1 |
| `--color-danger` on `--surface-danger` (`REMOVED` chip + name)  | 6.43:1 | 5.77:1  |
| `--color-danger` on the 14% removed hatch stripe (worst case)   | 5.13:1 | 4.64:1  |

Every pairing clears the 4.5:1 AA text floor; the tightest is the danger
chip over a hatch stripe in dark at 4.64:1. The draft bar's own line is
`--color-ink` on an 8% accent mix over `--color-bg-raised` -- the same
ink-on-raised pairing Task 4 verified, and the accent tint raises that
ratio rather than lowering it. Dimmed `same` slots are not held to the
floor: they carry no verdict, and their unmodified twins in committed
mode are the verified pairing.

**v0.5.7 history** introduced the band selector, the run cards' status
grounds, and the source badges. Checked computationally (same
throwaway-script convention):

| Pairing                                                            | Light   | Dark    |
| ------------------------------------------------------------------ | ------- | ------- |
| `--color-ink-muted` on `--color-bg-inset` (unselected band tab)    | 6.88:1  | 8.13:1  |
| `--color-ink` on `--color-bg-raised` (selected band tab)           | 16.56:1 | 15.15:1 |
| `--color-accent` on `--color-bg-raised` (selected band's rule, UI) | 6.59:1  | 10.19:1 |
| `--color-ink-muted` on `--surface-danger` (failed run's summary)   | 7.43:1  | 6.58:1  |
| `--color-ink-muted` on `--surface-warn` (in-flight run's summary)  | 7.51:1  | 6.30:1  |
| `--color-danger` on `--surface-danger` (outcome word + error line) | 6.43:1  | 5.77:1  |
| `--color-warn` on `--color-bg-raised` (warnings disclosure)        | 6.92:1  | 8.43:1  |
| `--color-accent` on `--color-bg-raised` (`CRON` source badge)      | 6.59:1  | 10.19:1 |
| `--color-warn` on `--color-bg-raised` (`CLI` source badge)         | 6.92:1  | 8.43:1  |

Every pairing clears the 4.5:1 AA text floor (worst case 5.77:1); the
band's accent rule is a non-text UI boundary and clears the 3:1 floor
with room to spare. The as-run rows reuse the guide's already-verified
`--color-ink-muted`-on-raised and plate pairings, since they render
inside the same `.rundown-day` / `.rundown-list` idiom.

**v0.5.9 live link** introduced the bezel LINK legend -- three dot
states, a legend, and a Reconnect button, all on the bezel's
`--color-bg-raised` ground. Checked computationally (same
throwaway-script convention):

| Pairing                                                             | Light   | Dark    |
| ------------------------------------------------------------------- | ------- | ------- |
| `--color-ink-muted` on `--color-bg-raised` (`LINK` label)           | 8.46:1  | 7.42:1  |
| `--color-ink` on `--color-bg-raised` (`LIVE`/`POLL`/`LINK LOST`)    | 16.56:1 | 15.15:1 |
| `--color-ink` on `--color-bg-raised` (`RECONNECT` button label)     | 16.56:1 | 15.15:1 |
| `--color-accent` on `--color-bg-raised` (`live` dot, non-text)      | 6.59:1  | 10.19:1 |
| `--color-warn` on `--color-bg-raised` (`poll` dot, non-text)        | 6.92:1  | 8.43:1  |
| `--color-danger` on `--color-bg-raised` (`lost` dot, non-text)      | 7.32:1  | 6.51:1  |
| `--color-border-interactive` on raised (Reconnect border, non-text) | 5.63:1  | 3.58:1  |

Both text pairings clear the 4.5:1 AA floor by a wide margin, and all
four non-text pairings clear the 3:1 floor -- the tightest is the
Reconnect button's border in dark at 3.58:1 -- `--color-border-interactive`,
which every `.btn` in the system already uses precisely because it clears
that floor. The button is deliberately *not*
`.btn--ghost`: that variant's `--color-border` edge is a 1.79:1 (light) /
1.40:1 (dark) pairing, acceptable for a button sitting inside a form the
operator is already looking at, too quiet for a control that has to be
findable the moment it appears in dense chrome.

The two flare rings are decorative and are not held to a floor -- the
same judgment `--sweep-trail` and `--glow-accent` already carry, and for
the same reason: the dot's own fill and the legend text carry the
reading, the ring only marks the moment it changed. For the record they
compute to 2.54:1 light / 3.36:1 dark (amber, 55% over raised) and
2.52:1 / 3.86:1 (green), against `--glow-accent`'s 1.47:1 / 1.75:1.

**v0.5.10 blocks row** introduced the `DARK UNTIL` chip and the two
amber `NEXT` legends. Both land on `--color-bg-raised` normally and on
`--color-bg-inset` under `.history-table tbody tr:hover td`, so each
pairing is checked on BOTH grounds -- a hovered row is the one the
operator is reading. Checked computationally (same throwaway-script
convention; the script reproduces every ratio already recorded above
before it was trusted for the new ones):

| Pairing                                                                    | Light   | Dark    |
| -------------------------------------------------------------------------- | ------- | ------- |
| `--color-warn` on `--color-bg-raised` (`DARK UNTIL` chip, amber legends)   | 6.92:1  | 8.43:1  |
| `--color-warn` on `--color-bg-inset` (same, hovered row)                   | 5.62:1  | 9.23:1  |
| `--color-warn` chip border on `--color-bg-raised` (non-text)               | 6.92:1  | 8.43:1  |
| `--color-warn` chip border on `--color-bg-inset` (non-text, hovered row)   | 5.62:1  | 9.23:1  |
| `--color-ink-muted` on `--color-bg-raised` (duration, detail, `PRI` line)  | 8.46:1  | 7.42:1  |
| `--color-ink-muted` on `--color-bg-inset` (same, hovered row)              | 6.88:1  | 8.13:1  |
| `--color-ink` on `--color-bg-raised` (the `NEXT` countdown)                | 16.56:1 | 15.15:1 |
| `--color-ink` on `--color-bg-inset` (same, hovered row)                    | 13.45:1 | 16.60:1 |

Every text pairing clears the 4.5:1 AA floor; the tightest is amber on
the hovered row's inset in light at 5.62:1. The chip's border clears the
3:1 non-text floor on both grounds by the same margin, since it is the
same colour on the same ground. The muted and ink rows are the pairings
Tasks 4 and v0.5.0 already verified, repeated here because this slice is
the first to put text on them inside a hovered table row. Nothing else in
the row introduced a colour: the plate, the toggle, the type badge and
the row actions are the classes those tasks already cleared.

None of the three dialogs this page authors -- Duplicate, the dark window,
the mid-run change -- introduces a pairing. `dialog.panel`,
`.panel__hint`, `.form-field` and `.btn` are Task 3's and Task 4's own
components on Task 3's own `--color-bg-raised` ground; the one shape
inside them that is new, the dark preset, is in the table below.

**v0.5.10 block editor** introduced the consequence rail, the series-row
disclosure, and the dark-window presets. Every one of them lands on a
ground this document already holds -- the editor panel's
`--color-bg-raised`, the series row's `--color-bg-inset`, and a `.btn`'s
hover step from the first to the second -- so the slice's second half
introduced no colour of its own. Recomputed regardless (same
throwaway-script convention; the script reproduced every ratio already
recorded above, this slice's first half included, before it was trusted
for these), because a ratio is a property of the ground a pairing lands
on, not of the class carrying it:

| Pairing                                                                            | Light   | Dark    |
| ---------------------------------------------------------------------------------- | ------- | ------- |
| `--color-ink-muted` on `--color-bg-raised` (rail legends + notes, preset instant)  | 8.46:1  | 7.42:1  |
| `--color-ink` on `--color-bg-raised` (rail item -- the edited block; preset label) | 16.56:1 | 15.15:1 |
| `--color-warn` on `--color-bg-raised` (rail lineup, incomplete row)                | 6.92:1  | 8.43:1  |
| `--color-danger` on `--color-bg-raised` (the rail's cron error line)               | 7.32:1  | 6.51:1  |
| `--color-ink` on `--color-bg-inset` (series-row toggle + summary)                  | 13.45:1 | 16.60:1 |
| `--color-ink-muted` on `--color-bg-inset` (row index + chevron; hovered preset)    | 6.88:1  | 8.13:1  |
| `--color-warn` on `--color-bg-inset` (collapsed series row, incomplete)            | 5.62:1  | 9.23:1  |
| `--color-border-interactive` on `--color-bg-raised` (preset border, non-text)      | 5.63:1  | 3.58:1  |
| `--color-border-interactive` on `--color-bg-inset` (hovered preset, non-text)      | 4.58:1  | 3.92:1  |

Every text pairing clears the 4.5:1 AA floor; the tightest is amber on the
series row's inset ground in light at 5.62:1. Both non-text border
pairings clear 3:1 -- the tightest, 3.58:1 for a resting preset in dark,
is `--color-border-interactive`, the ratio every `.btn` in this system
already carries, and the button's hover step raises it rather than
lowering it. The chevron is drawn ink, not a colour-only signal: at
6.88:1 / 8.13:1 it clears the non-text floor on its own, and the row's
state is carried by `aria-expanded` and the summary line regardless.

Two things in these clusters are deliberately not held to a floor.
`.series-row`'s own 1px `--color-border` container edge is a 1.46:1 /
1.54:1 pairing -- a container boundary, not a state-bearing UI component,
the same judgment Task 3 and Task 4 already made for `.hero-panel`'s
border. And a disabled preset (`.btn:disabled`, `opacity: 0.5`) falls
under WCAG 1.4.3's inactive-component exception; the whole preset row goes
disabled together while a write is on the wire, so nothing in it is a
choice the operator can still make.

**v0.5.12 history desk** introduced the storage strip, the blocked-row
notice, and the range-cleanup panel. Checked computationally (same
throwaway-script convention):

| Pairing                                                            | Light   | Dark    |
| ------------------------------------------------------------------ | ------- | ------- |
| `--color-ink-muted` on `--color-bg-inset` (strip labels)           | 6.88:1  | 8.13:1  |
| `--color-ink` on `--color-bg-inset` (strip values)                 | 13.45:1 | 16.60:1 |
| `--color-ink-muted` at 85% over `--color-bg-inset` (the note line) | 4.79:1  | 6.09:1  |
| `--color-danger` on `--color-bg-raised` (cleanup disclosure)       | 7.32:1  | 6.51:1  |
| `--color-ink` on `--surface-danger` (cleanup body copy)            | 14.54:1 | 13.44:1 |
| `--color-ink-muted` on `--surface-danger` (cleanup field labels)   | 7.43:1  | 6.58:1  |

Every pairing clears the 4.5:1 AA text floor; the tightest is the
strip's note line in light at 4.79:1, and it is the one pairing here
carrying an opacity, so the ratio is computed against the composited
colour rather than the token. The blocked row reuses the already-verified
muted-on-raised and accent-on-raised pairings.

## TypeScript runtime and Alpine.js conventions

**One bundle per page (v0.5.0).** The shared runtime
(`web/assets/ts/runtime/`: `api.ts`, `token.ts`, `errors.ts`,
`format.ts`, `channels.ts`, `tape.ts`, `shell.ts`, and since v0.5.9
`bus.ts` + `stream.ts`) is imported by thin page entries (`web/assets/ts/pages/*.ts`) and compiled INTO each page's
single esbuild bundle by the `ui/page-js` partial -- there is no
separate shell bundle and no `window.schedularr` global anymore. Within
a page every module (the `ApiError` class identity included) is the same
compiled copy, which is what makes `instanceof` checks safe. The
runtime's `api.ts` is the only module allowed to call `fetch`, adds
AbortController timeouts (15s reads / 60s writes), and entry-guards
mutations (an identical in-flight mutation shares the first request's
promise instead of double-firing). `shell.ts` wires the token panel
(Save probes `GET /status`, arms only on success, then broadcasts the
re-auth event that re-fires failed loads), owns the page's single live
link -- one `stream.ts` connection, every frame onto `bus.ts` -- and runs
the `GET /status` poll that is both the POLL rung's fallback and the
source of the three non-LINK bezel readouts. It fires only on the POLL
rung: on LIVE the stream's own frames drive the refetch, and on LINK LOST
there is nothing on the other end to poll -- either way a second reader
on the same bezel is how a stale number overwrites a fresh one.

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
   see <https://alpinejs.dev/globals/alpine-data>). An early build of the
   dashboard page had `x-init="init()"` on the root element *in addition
   to* the component's own `init()` method, which ran `loadStatus()`/
   `loadHistory()` twice on every real page load (fixed in commit
   `0ad914e`). The fix was deleting the `x-init` attribute, not renaming
   the method -- there is no `x-init` attribute anywhere in
   `web/layouts/` today.
2. **Keep the `started`-guard as defense-in-depth, not as the fix.**
   Every page component (`guide.ts`, `dashboard.ts`, `blocks.ts`,
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

| File               | Version | Loaded on                   | sha256                                                             |
| ------------------ | ------- | --------------------------- | ------------------------------------------------------------------ |
| `alpine.min.js`    | 3.16.3  | every page (`baseof.html`)  | `e31d6d92aefd41979d3c66f994d3a6b77fafa5062aec67d13f3ec5099d70d5d6` |
| `cronstrue.min.js` | 3.24.0  | blocks (`blocks/list.html`) | `f47fa32a8c38a0fd996ef386ffc8c97694e483742a3efc3e3d70d147112b8bd5` |

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

```txt
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
(`/`, `/blocks/`, `/series/`, and an unknown path's 404) --
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
