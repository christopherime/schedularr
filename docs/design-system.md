# Design System

The web UI's design system is documented in full at [`web/DESIGN.md`](https://github.com/christopherime/schedularr/blob/main/web/DESIGN.md) in the repository — token inventory, component-class catalog, the light/dark mechanism, WCAG contrast evidence, and Alpine.js conventions. It lives with the code it documents rather than in this site, since tooling (the `impeccable` design-review skill) reads it directly from that path. This page summarizes it; treat `web/DESIGN.md` as the source of truth for anyone adding a page or component to `web/`.

## Creative direction

**"CRT oscilloscope / signal bench."** The UI reads as calibrated measurement instrumentation the operator arms before trusting a reading, not a dashboard-card admin panel. Dark mode is the live phosphor trace on the glass; light mode is the instrument's own calibration printout on graph paper — both are first-class, decided by `prefers-color-scheme` with no manual toggle. The graticule (an oscilloscope's measuring grid) is a literal CSS layout background on bordered surfaces. Every status indicator carries an adjacent text label; color alone never carries a fact. Typography is monospace throughout, since the product's actual data — cron strings, tabular durations, season/episode cursors — reads as instrument output in a fixed-width face.

Key characteristics: one font family, a fixed rem type scale (no `clamp()`), small radii (2-6px, a machined bezel rather than a soft consumer card), restrained color (neutrals plus one accent, with amber/red reserved for functional signal states), one authored motion (the token panel's open/close), no manual light/dark toggle.

## Colors and typography

Every color is a CSS custom property on `:root`, light values first, overridden under `@media (prefers-color-scheme: dark)` — no third app-controlled theme state. The palette: a background/raised/inset triad, a graticule grid color, ink/ink-muted text, border/border-interactive, one accent (phosphor green), and warn/danger. `::selection`, the focus ring, and the scrollbar are themed from the same tokens.

One typeface everywhere: a `ui-monospace` stack (SF Mono, Cascadia Code, JetBrains Mono, Menlo, Consolas, Liberation Mono, generic `monospace`). A `--tracking-label` (`0.06em`) plus uppercase treatment marks the recurring "instrument legend" look — wordmark, nav, buttons, badges, form labels, headings. Body prose is the one context that stays sentence-case.

## Components

Shared shapes reused verbatim across every page after the dashboard shipped: `.btn` (with `--primary`/`--ghost`/`--danger-ghost`/`--icon`/`--sm` variants), `.status-dot` (a coded-legend indicator, never without adjacent text), `.problem` (the inline API-error panel), `.skeleton-*` (loading states — never a spinner), `.table-wrap`/`.history-table` (the one table shape in the system), `.form-grid`/`.form-field`/`.checkbox-field` (the blocks editor's field system), `dialog.panel`/`.editor-panel` (two panel chromes sharing one spacing vocabulary), `.hero-panel`/`.graticule` (the bordered instrument-surface primitive), `.badge`, `.toggle` (a `role="switch"` rocker).

## Accessibility

Every color pairing was checked computationally against the WCAG relative-luminance/contrast-ratio formula, not eyeballed. Text pairings clear the 4.5:1 AA floor by a wide margin in both palettes (6:1-16:1 typical); the one interactive non-text pairing checked (input/button border against background) clears the 3:1 non-text floor. `web/DESIGN.md`'s "Accessibility: WCAG contrast evidence" section has the full pairing-by-pairing record.

## Vendored dependencies

Every third-party script is vendored into `web/assets/vendor/`, pinned to an exact version, loaded via a plain `<script defer>` from the UI's own origin — no CDN:

| File               | Version | Purpose                                                                                                |
| ------------------ | ------- | ------------------------------------------------------------------------------------------------------ |
| `alpine.min.js`    | 3.16.3  | Interactivity, loaded on every page                                                                    |
| `cronstrue.min.js` | 3.24.0  | Plain-language cron readback in the blocks editor's [schedule picker](web-ui-guide.md#schedule-picker) |

## Content-Security-Policy

Every UI response carries `default-src 'self'; script-src 'self' 'unsafe-eval'; style-src 'self'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'`. Every directive is `'self'`/`'none'` because the shipped site never loads a third-party origin. `'unsafe-eval'` is required by Alpine.js 3, which evaluates directive expressions via `new Function(...)`.

Consequence for any future page: no inline `style="..."` attributes and no inline `<style>` blocks — `style-src 'self'` silently drops their declarations rather than raising a visible CSP error. Use a CSS class instead.

## Accepted risk: token storage

CodeQL's `js/clear-text-storage-of-sensitive-data` alert on the UI's `localStorage.setItem` call for the bearer token is dismissed as won't-fix, not unaddressed. `PRODUCT.md`'s "Token-once, same-origin" principle is the deliberate design this flags: no server session, no cookie, no CSRF surface, a single pasted bearer token as the entire auth model for a single self-hosting operator on a LAN-only exposure. `web/DESIGN.md`'s "Security: CodeQL accepted risk" section has the full reasoning and what would reopen the question.
