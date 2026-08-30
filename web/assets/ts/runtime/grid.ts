// The guide's grid renderer (spec §3.1 + the resolved pre-slice spike).
// TS-built DOM: this module renders the EPG sheet and the mobile rundown
// directly from the typed plan; Alpine drives ONLY the toolbar and the
// inspector. Everything geometric goes through CSSOM
// (el.style.setProperty) -- the CSP's style-src 'self' silently drops
// inline style="..." attributes, and grid-column line numbers set via the
// CSSOM are not inline attributes (see DESIGN.md, Content-Security-Policy).
//
// Since the v0.5.3 full-week reframe (spec §3.1, second amendment) the
// sheet renders SEVEN consecutive days as one continuous horizontal
// timeline per channel -- constructed as seven 288-quantum day segments
// per track laid in a row, NOT one 2016-column grid, so the spike's
// measured per-day CSS grid stays the building block. A slot crossing
// midnight inside the rendered week renders as flush pieces joined
// across the segment boundary (one continuous block); dashed cut edges
// survive only at the week's outer edges, where the slot really does
// continue off the page.
//
// Spike findings this file honors (measured, binding -- spec §9):
//   - one CSS grid PER DAY SEGMENT, 288 five-minute fixed columns,
//     slots placed by grid-column line numbers via CSSOM;
//   - the sticky rail/ruler topology comes from FLEX FLOW, never grid
//     membership: sheet = block width:max-content; ruler = sticky-top
//     flex row with a sticky-left corner; each channel row = flex of
//     sticky-left plate + day-segment tracks (Chromium forgives sticky
//     grid items; Safari historically does not);
//   - the now-line is an absolute overlay child of the sheet at
//     calc(var(--rail-w) + var(--now-min) * var(--px-per-min)), with
//     --now-min set per minute via CSSOM (week-relative: dayIndex*1440 +
//     minutes into that day) -- no scroll handler;
//   - the graticule is a repeating-linear-gradient at --div, coinciding
//     exactly with ruler cells and slot boundaries (one division = 30
//     minutes of air time); day boundaries carry a slightly stronger
//     rule (--color-dayline) through ruler, tracks, and ground;
//   - z-order: nowline < plates < ruler < corner.
//
// The pure geometry below (quantum clamping, week windowing/chunking,
// segment-edge classification, ghost resolution, keyboard-nav picking)
// is exported for unit tests (web/tests/grid.test.ts) and never touches
// the DOM.

import type { PlateParts } from "./channels.ts";
import { formatClock, pad2, plural, sxxeyy } from "./format.ts";

// ---- pure geometry ---------------------------------------------------------

/** The placement quantum: slots land on 5-minute grid lines. */
export const QUANTUM_MIN = 5;
/** Columns per day segment: 24h / 5min. */
export const QUANTA_PER_DAY = 288;

/** Start of the local calendar day containing ms. */
export function localDayStart(ms: number): number {
  const d = new Date(ms);
  d.setHours(0, 0, 0, 0);
  return d.getTime();
}

/** Local day start n days after dayStartMs -- Date math, not n*24h, so a
 * DST-shifted day still lands on its real local midnight. */
export function addDays(dayStartMs: number, n: number): number {
  const d = new Date(dayStartMs);
  d.setDate(d.getDate() + n);
  d.setHours(0, 0, 0, 0);
  return d.getTime();
}

/**
 * How many calendar days the loaded plan window touches. The server
 * plans [fetch, fetch + days*24h) -- real hours, not calendar days --
 * so any fetch after local midnight spills into a trailing partial
 * calendar day: a 28-day plan fetched at 19:00 ends 19:00 four weeks
 * out, on a 29th calendar day that needs its own (one-day) week page
 * and rundown section. A fetch at exactly midnight stays at `days`
 * (unless an intervening spring-forward day shortens the span, in which
 * case the window still spills into one more calendar day and the count
 * reflects it). Date math via addDays, so DST-shifted windows still
 * count real local midnights.
 */
export function windowDayCount(fetchMs: number, days: number): number {
  const windowEndMs = fetchMs + days * 86_400_000;
  const start = localDayStart(fetchMs);
  for (let k = 1; k <= days; k++) {
    if (addDays(start, k) >= windowEndMs) return k;
  }
  return days + 1;
}

/** One slot piece's placement on a single day segment's 288-quantum
 * track. cutLeft/cutRight mark a spill clamped at the day edge (the
 * slot continues on the neighboring day); segmentEdges below decides
 * whether that edge is an intra-week JOIN or a week-edge dashed CUT. */
export interface SlotSpan {
  startQ: number;
  endQ: number;
  cutLeft: boolean;
  cutRight: boolean;
}

/**
 * Clamps [startMs, endMs) onto the day [dayStartMs, dayEndMs) and
 * quantizes to 5-minute grid lines -- start rounds down, end rounds up,
 * and a degenerate span still occupies one quantum so it stays visible.
 * Returns null when the slot doesn't touch this day at all. On a
 * DST-shifted day the minutes past local midnight are clamped into the
 * 288-column track (the visual day is always 24 columns of --div).
 * Interim: placement counts real elapsed minutes while the ruler labels
 * the track as fixed wall-clock hours, so on the two DST days slots sit
 * one --div-hour off their labels (TODO.md, v0.5.1 guide deferrals).
 */
export function daySpan(startMs: number, endMs: number, dayStartMs: number, dayEndMs: number): SlotSpan | null {
  if (endMs <= dayStartMs || startMs >= dayEndMs) return null;
  const startMin = Math.max(0, (Math.max(startMs, dayStartMs) - dayStartMs) / 60_000);
  const endMin = Math.max(0, (Math.min(endMs, dayEndMs) - dayStartMs) / 60_000);
  let startQ = Math.floor(startMin / QUANTUM_MIN);
  let endQ = Math.ceil(endMin / QUANTUM_MIN);
  startQ = Math.min(QUANTA_PER_DAY - 1, Math.max(0, startQ));
  endQ = Math.min(QUANTA_PER_DAY, Math.max(0, endQ));
  if (endQ <= startQ) endQ = startQ + 1;
  return { startQ, endQ, cutLeft: startMs < dayStartMs, cutRight: endMs > dayEndMs };
}

/** CSS grid-column line numbers for a span (grid lines are 1-based). */
export function gridColumn(span: SlotSpan): string {
  return `${span.startQ + 1} / ${span.endQ + 1}`;
}

const DAY_NAMES = ["SUN", "MON", "TUE", "WED", "THU", "FRI", "SAT"];
const MONTH_NAMES = ["JAN", "FEB", "MAR", "APR", "MAY", "JUN", "JUL", "AUG", "SEP", "OCT", "NOV", "DEC"];

/** Day-header label for a segment: "SAT 30". */
export function dayLabel(dayStartMs: number): string {
  const d = new Date(dayStartMs);
  return `${DAY_NAMES[d.getDay()]} ${pad2(d.getDate())}`;
}

// ---- week paging (spec §3.1, second amendment 2026-08-30) ------------------
// The day-tab strip died with the full-week reframe: the grid renders a
// whole week per page, so navigation is ONLY the ‹/› week pager. Week
// pages are consecutive 7-day chunks of the loaded window, starting at
// the window's first calendar day (the fetch day) -- consistent with
// windowDayCount, so a 28-day plan fetched mid-day (29 calendar days)
// honestly pages as 7+7+7+7+1.

/** Days per guide week page. */
export const WEEK_DAYS = 7;

/** How many week pages a loaded window spans (a 29-calendar-day window
 * is 5 pages: 7+7+7+7+1). Never 0 -- an empty window still has page 0. */
export function weekPageCount(loadedDays: number): number {
  return Math.max(1, Math.ceil(loadedDays / WEEK_DAYS));
}

/** Page k's slice of the window: its first day index and how many days
 * it really holds (a trailing partial week keeps its real day count,
 * never seven padded ones). */
export interface WeekChunk {
  startDay: number;
  days: number;
}

export function weekChunk(page: number, loadedDays: number): WeekChunk {
  const startDay = page * WEEK_DAYS;
  return { startDay, days: Math.max(0, Math.min(WEEK_DAYS, loadedDays - startDay)) };
}

/** Pager label: "SUN 30 AUG – SAT 05 SEP" (a one-day trailing page is
 * just its day, "SAT 26 SEP"). */
export function weekRangeLabel(weekStartMs: number, dayCount: number): string {
  const first = `${dayLabel(weekStartMs)} ${MONTH_NAMES[new Date(weekStartMs).getMonth()]}`;
  if (dayCount <= 1) return first;
  const lastMs = addDays(weekStartMs, dayCount - 1);
  return `${first} – ${dayLabel(lastMs)} ${MONTH_NAMES[new Date(lastMs).getMonth()]}`;
}

/** The sticky corner's quiet month readout for a week page: "AUG 2026",
 * "AUG–SEP 2026" across a month edge, "DEC–JAN" across a year edge. */
export function weekCornerLabel(weekStartMs: number, dayCount: number): string {
  const start = new Date(weekStartMs);
  const end = new Date(addDays(weekStartMs, Math.max(0, dayCount - 1)));
  if (start.getFullYear() !== end.getFullYear()) {
    return `${MONTH_NAMES[start.getMonth()]}–${MONTH_NAMES[end.getMonth()]}`;
  }
  if (start.getMonth() !== end.getMonth()) {
    return `${MONTH_NAMES[start.getMonth()]}–${MONTH_NAMES[end.getMonth()]} ${start.getFullYear()}`;
  }
  return `${MONTH_NAMES[start.getMonth()]} ${start.getFullYear()}`;
}

/** How a piece's clamped edges render on day segment dayIndex of a
 * dayCount-day week: an edge at an interior midnight is a JOIN (the two
 * pieces render flush -- one continuous block); an edge at the week's
 * outer boundary stays a dashed CUT (the slot continues off the page). */
export interface SegmentEdges {
  cutLeft: boolean;
  cutRight: boolean;
  joinLeft: boolean;
  joinRight: boolean;
}

export function segmentEdges(span: SlotSpan, dayIndex: number, dayCount: number): SegmentEdges {
  return {
    joinLeft: span.cutLeft && dayIndex > 0,
    joinRight: span.cutRight && dayIndex < dayCount - 1,
    cutLeft: span.cutLeft && dayIndex === 0,
    cutRight: span.cutRight && dayIndex === dayCount - 1,
  };
}

/** Which piece of a multi-segment slot carries the visible face: the
 * widest one (ties to the earlier piece) -- a 23:30→06:00 slot labels
 * its six-hour morning piece, not the 30-minute sliver before midnight.
 * The other pieces keep the same content visibility-hidden so the join
 * stays step-free, and describe themselves via aria-label alone. */
export function primaryPieceIndex(spans: SlotSpan[]): number {
  let best = 0;
  let bestWidth = -1;
  for (let i = 0; i < spans.length; i++) {
    const width = spans[i].endQ - spans[i].startQ;
    if (width > bestWidth) {
      best = i;
      bestWidth = width;
    }
  }
  return best;
}

/** Rundown group heading (spec §7): TONIGHT, TOMORROW, then "MON 02" --
 * dayIndex is the day's absolute index in the loaded window (0 = the
 * fetch day), not its position on the current week page. */
export function rundownDayHeading(dayIndex: number, dayStartMs: number): string {
  if (dayIndex === 0) return "TONIGHT";
  if (dayIndex === 1) return "TOMORROW";
  return dayLabel(dayStartMs);
}

/** One program of a slot's lineup, projected from the typed wire shape. */
export interface GuideProgram {
  title: string;
  type?: string;
  season?: number;
  episode?: number;
  durationMs: number;
  startMs: number;
}

/** One renderable slot (a planned slot or a NO SIGNAL ghost). */
export interface GuideSlot {
  kind: "slot" | "ghost";
  channelId: string;
  blockName: string;
  blockType: string; // "filter" | "series"
  cron: string;
  priority: number;
  startMs: number;
  endMs: number;
  programs: GuideProgram[];
  /** ghost only: the winning block's name. */
  lostTo?: string;
}

/** One channel row of the sheet: plate + its slots (ghosts included),
 * sorted by start. */
export interface GuideRow {
  channelId: string;
  plate: PlateParts;
  slots: GuideSlot[];
}

/** What ghost resolution needs from a BlockRecord -- the Warning itself
 * carries only block names + occurrence_start until v0.5.5 (memory) adds
 * channel_id/duration_minutes to the wire shape. */
export interface GhostBlockInfo {
  channelId: string;
  durationMinutes: number;
}

export interface WarningShape {
  block_name?: string;
  occurrence_start?: string;
  blocking_block_name?: string;
}

/**
 * Places a current-plan warning as a NO SIGNAL ghost at its
 * would-have-aired time. INTERIM (v0.5.1): the Warning wire shape carries
 * only block names and occurrence_start -- duration and channel arrive on
 * the contract in v0.5.5 -- so both are resolved client-side from the
 * losing block's spec (GET /blocks). When the losing block can't be
 * resolved (blocks fetch failed, block deleted between plan and render),
 * the winner's channel places the ghost (a conflict is always
 * same-channel) and the duration falls back to one division (30 min);
 * with neither block resolvable the ghost is unplaceable and dropped.
 */
export function resolveGhost(
  w: WarningShape,
  blocksByName: Map<string, GhostBlockInfo>,
): GuideSlot | null {
  if (!w.occurrence_start) return null;
  const startMs = Date.parse(w.occurrence_start);
  if (Number.isNaN(startMs)) return null;
  const loser = w.block_name ? blocksByName.get(w.block_name) : undefined;
  const winner = w.blocking_block_name ? blocksByName.get(w.blocking_block_name) : undefined;
  const channelId = loser?.channelId ?? winner?.channelId;
  if (!channelId) return null;
  const durationMinutes = loser?.durationMinutes ?? 30;
  return {
    kind: "ghost",
    channelId,
    blockName: w.block_name ?? "Unknown block",
    blockType: "",
    cron: "",
    priority: 0,
    startMs,
    endMs: startMs + durationMinutes * 60_000,
    programs: [],
    lostTo: w.blocking_block_name ?? "a higher-priority block",
  };
}

/** One rundown listing of a slot: `continuation` marks the second
 * appearance of an overnight slot, under the day it spills into. */
export interface RundownEntry {
  slot: GuideSlot;
  continuation: boolean;
}

/**
 * The slots a rundown day section lists: everything OVERLAPPING
 * [dayStartMs, dayEndMs), mirroring the desktop grid's join/cut behavior
 * (daySpan) -- an overnight slot appears under BOTH its start day and
 * the day it spills into, the second appearance marked as a
 * continuation. A slot ending exactly at midnight belongs only to its
 * start day, matching daySpan's half-open window.
 */
export function rundownDaySlots(slots: GuideSlot[], dayStartMs: number, dayEndMs: number): RundownEntry[] {
  return slots
    .filter((s) => s.startMs < dayEndMs && s.endMs > dayStartMs)
    .map((s) => ({ slot: s, continuation: s.startMs < dayStartMs }));
}

// ---- slot faces (spec §3.1, amended 2026-08-30) ----------------------------

/** A slot narrower than this many divisions (3 × 30 min) keeps the
 * summary face -- no room to list programs legibly. */
export const FACE_MIN_DIVS = 3;
/** Most program lines a face lists; a longer lineup folds its tail into
 * one "+N more" line, so a face never exceeds FACE_MAX_LINES lines. */
export const FACE_MAX_LINES = 3;

/** What a slot's face renders under the name + meta lines. */
export interface SlotFace {
  /** One line per program ("SHOW · SxxEyy"; a movie is just its title). */
  lines: string[];
  /** Programs folded behind the trailing "+N more" line (0 = none). */
  more: number;
}

/**
 * Face-content selection: series slots list their programs on the face,
 * one line each, folding anything past FACE_MAX_LINES into a "+N more"
 * line; filter blocks and ghosts keep the name + count face; a narrow
 * slot (< FACE_MIN_DIVS divisions) degrades to name + count regardless
 * of type. For a cross-midnight slot the span passed here is its
 * PRIMARY (widest) piece's span -- the face is decided once per slot,
 * then mirrored hidden into the other pieces. Pure -- unit-tested in
 * web/tests/grid.test.ts.
 */
export function slotFace(slot: GuideSlot, span: SlotSpan): SlotFace {
  if (slot.kind !== "slot" || slot.blockType !== "series") return { lines: [], more: 0 };
  if (span.endQ - span.startQ < FACE_MIN_DIVS * 6) return { lines: [], more: 0 };
  if (slot.programs.length === 0) return { lines: [], more: 0 };
  const all = slot.programs.map((p) => {
    const title = p.title.trim() === "" ? "—" : p.title;
    const marker = sxxeyy(p.season, p.episode);
    return marker ? `${title} · ${marker}` : title;
  });
  if (all.length <= FACE_MAX_LINES) return { lines: all, more: 0 };
  const kept = FACE_MAX_LINES - 1;
  return { lines: all.slice(0, kept), more: all.length - kept };
}

/**
 * Keyboard nav (Up/Down across channels): the index of the slot whose
 * start is nearest the target instant -- minimal |start − target|, ties
 * to the earlier slot. -1 for an empty track.
 */
export function nearestSlotIndex(startsMs: number[], targetMs: number): number {
  let best = -1;
  let bestDist = Number.POSITIVE_INFINITY;
  for (let i = 0; i < startsMs.length; i++) {
    const dist = Math.abs(startsMs[i] - targetMs);
    if (dist < bestDist) {
      best = i;
      bestDist = dist;
    }
  }
  return best;
}

// ---- DOM rendering ---------------------------------------------------------

export interface GridCallbacks {
  /** A slot (or ghost) was activated -- open the inspector on it. */
  onOpen(slot: GuideSlot, el: HTMLElement): void;
  /** Element id of the inspector panel the slots disclose. When set,
   * every slot button carries aria-controls + aria-expanded="false";
   * the page flips the opener's aria-expanded while its inspector is
   * open. Absent on surfaces with no inspector (the /kit/ fixtures). */
  inspectorId?: string;
}

export interface GridHandle {
  /** Repositions the now-line (CSSOM --now-min, week-relative) and
   * refreshes the is-past / is-on-air classes on every piece. Driven by
   * the guide page's local 60s timer (heartbeat skew correction arrives
   * with SSE in v0.5.6). */
  updateNow(nowMs: number): void;
  /** The x pixel offset of "now" inside the scroll viewport, or null
   * when now is outside the rendered week -- the auto-scroll target on
   * the week page containing now (other pages open at their start). */
  nowOffsetPx(nowMs: number): number | null;
}

/** One logical slot on a track: every rendered piece (one per touched
 * day segment) plus the primary piece keyboard focus lands on. */
interface PlacedSlot {
  slot: GuideSlot;
  pieces: HTMLButtonElement[];
  primary: HTMLButtonElement;
}

function el<K extends keyof HTMLElementTagNameMap>(
  tag: K,
  className: string,
  text?: string,
): HTMLElementTagNameMap[K] {
  const node = document.createElement(tag);
  node.className = className;
  if (text !== undefined) node.textContent = text;
  return node;
}

/** The channel legend plate, DOM-built (same shape as ui/plate.html). */
function plateEl(plate: PlateParts): HTMLElement {
  const wrap = el("span", "plate");
  if (plate.ch) wrap.appendChild(el("span", "plate__ch", plate.ch));
  wrap.appendChild(el("span", "plate__name", plate.name));
  return wrap;
}

function slotTimeRange(slot: GuideSlot): string {
  return `${formatClock(slot.startMs)}–${formatClock(slot.endMs)}`;
}

/** The slot face's second line: time range, program count, type-in-text
 * (the tint is a secondary scan aid; text carries the fact). */
function slotMeta(slot: GuideSlot): string {
  const parts = [slotTimeRange(slot)];
  parts.push(`${slot.programs.length} PROG`);
  if (slot.blockType !== "") parts.push(slot.blockType.toUpperCase());
  return parts.join(" · ");
}

function slotAriaLabel(slot: GuideSlot): string {
  if (slot.kind === "ghost") {
    return `No signal — ${slot.blockName} lost to ${slot.lostTo} at ${formatClock(slot.startMs)}`;
  }
  return `${slot.blockName}, ${slotTimeRange(slot)}, ${plural(slot.programs.length, "program")}`;
}

interface PieceOptions {
  edges: SegmentEdges;
  /** The widest piece carries the visible face; the others mirror the
   * same content visibility-hidden (equal natural height -> a step-free
   * join) and say "continues across midnight" via aria-label. */
  labeled: boolean;
  /** The face decided once per slot from its primary piece's span. */
  face: SlotFace;
}

function buildSlotPiece(slot: GuideSlot, span: SlotSpan, opts: PieceOptions, cb: GridCallbacks): HTMLButtonElement {
  let cls = slot.kind === "ghost" ? "guide-slot guide-slot--ghost" : "guide-slot";
  if (!opts.labeled) cls += " guide-slot--silent";
  const btn = el("button", cls);
  btn.type = "button";
  btn.tabIndex = -1;
  btn.dataset.type = slot.blockType;
  const { cutLeft, cutRight, joinLeft, joinRight } = opts.edges;
  if (cutLeft) btn.dataset.cut = cutRight ? "both" : "left";
  else if (cutRight) btn.dataset.cut = "right";
  if (joinLeft) btn.dataset.join = joinRight ? "both" : "left";
  else if (joinRight) btn.dataset.join = "right";
  btn.style.setProperty("grid-column", gridColumn(span));
  const base = slotAriaLabel(slot);
  btn.setAttribute("aria-label", opts.labeled ? base : `${base}, continues across midnight`);
  if (cb.inspectorId) {
    btn.setAttribute("aria-controls", cb.inspectorId);
    btn.setAttribute("aria-expanded", "false");
  }
  if (slot.kind === "ghost") {
    btn.appendChild(el("span", "guide-slot__name", "NO SIGNAL"));
    btn.appendChild(el("span", "guide-slot__meta", `LOST TO ${(slot.lostTo ?? "").toUpperCase()}`));
  } else {
    btn.appendChild(el("span", "guide-slot__name", slot.blockName));
    btn.appendChild(el("span", "guide-slot__meta", slotMeta(slot)));
    // Series faces list their content (spec §3.1): the block name stays
    // primary, program lines are secondary at label scale.
    for (const line of opts.face.lines) btn.appendChild(el("span", "guide-slot__prog", line));
    if (opts.face.more > 0) btn.appendChild(el("span", "guide-slot__more", `+${opts.face.more} MORE`));
  }
  btn.addEventListener("click", () => cb.onOpen(slot, btn));
  return btn;
}

/** The two-tier ruler: a sticky day-header tier (SUN 30 · MON 31 · …,
 * each spanning its day's 48 cells, labels sticking left within their
 * day while it pans) over the hour cells -- both tiers ride one sticky
 * flex row with the sticky-left month corner, so the topology stays
 * flex-flow (the spike's binding constraint). */
function buildRuler(weekStartMs: number, dayCount: number): HTMLElement {
  const ruler = el("div", "guide-ruler");
  ruler.appendChild(el("div", "guide-ruler__corner", weekCornerLabel(weekStartMs, dayCount)));
  const strip = el("div", "guide-ruler__strip");
  const days = el("div", "guide-ruler__days");
  const todayStart = localDayStart(Date.now());
  for (let k = 0; k < dayCount; k++) {
    const dayStartMs = addDays(weekStartMs, k);
    const cell = el("div", "guide-ruler__day");
    if (dayStartMs === todayStart) {
      cell.dataset.today = "";
      cell.setAttribute("aria-current", "date");
    }
    cell.appendChild(el("span", "guide-ruler__day-label", dayLabel(dayStartMs)));
    days.appendChild(cell);
  }
  strip.appendChild(days);
  const cells = el("div", "guide-ruler__cells");
  for (let k = 0; k < dayCount; k++) {
    for (let half = 0; half < 48; half++) {
      const cell = el(
        "div",
        half === 0 && k > 0 ? "guide-ruler__cell guide-ruler__cell--newday" : "guide-ruler__cell",
      );
      if (half % 2 === 0) cell.textContent = `${pad2(half / 2)}:00`;
      cells.appendChild(cell);
    }
  }
  strip.appendChild(cells);
  ruler.appendChild(strip);
  return ruler;
}

/**
 * Renders one week page of the guide sheet into `container` (replacing
 * whatever it held): dayCount consecutive day segments per channel laid
 * in a row as one continuous timeline, and roving-tabindex keyboard
 * navigation -- Left/Right along a track across the whole week (a
 * cross-midnight slot is ONE stop: focus rides its primary piece),
 * Up/Down across channels (nearest start), Enter/Space opens, Esc is
 * left to the page (inspector close + focus return). Returns the handle
 * the 60s timer drives.
 */
export function renderGuideWeek(
  container: HTMLElement,
  rows: GuideRow[],
  weekStartMs: number,
  dayCount: number,
  cb: GridCallbacks,
): GridHandle {
  const dayStarts: number[] = [];
  for (let k = 0; k <= dayCount; k++) dayStarts.push(addDays(weekStartMs, k));
  const weekEndMs = dayStarts[dayCount];

  const sheet = el("div", "guide-sheet");
  sheet.appendChild(buildRuler(weekStartMs, dayCount));

  // tracks[t] mirrors rows[t]; each entry holds the week's logical slots
  // in start order -- the keyboard-nav model and the minute-tick refresh
  // list (a cross-midnight slot is one entry with several pieces).
  const tracks: PlacedSlot[][] = [];

  for (const row of rows) {
    const rowEl = el("div", "guide-row");
    const plateCell = el("div", "guide-plate-cell");
    plateCell.appendChild(plateEl(row.plate));
    rowEl.appendChild(plateCell);

    // A channel with any ghost gets the two-lane template on EVERY
    // segment, so the lane pitch stays uniform across the week band.
    const hasGhost = row.slots.some((s) => s.kind === "ghost");
    const segEls: HTMLElement[] = [];
    for (let k = 0; k < dayCount; k++) {
      let cls = "guide-track";
      if (k > 0) cls += " guide-track--newday";
      if (hasGhost) cls += " guide-track--lanes";
      const seg = el("div", cls);
      segEls.push(seg);
      rowEl.appendChild(seg);
    }

    const placed: PlacedSlot[] = [];
    for (const slot of row.slots) {
      const touched: { k: number; span: SlotSpan }[] = [];
      for (let k = 0; k < dayCount; k++) {
        const span = daySpan(slot.startMs, slot.endMs, dayStarts[k], dayStarts[k + 1]);
        if (span) touched.push({ k, span });
      }
      if (touched.length === 0) continue;
      const primaryIdx = primaryPieceIndex(touched.map((p) => p.span));
      const face = slotFace(slot, touched[primaryIdx].span);
      const pieces: HTMLButtonElement[] = [];
      for (let i = 0; i < touched.length; i++) {
        const { k, span } = touched[i];
        const piece = buildSlotPiece(
          slot,
          span,
          { edges: segmentEdges(span, k, dayCount), labeled: i === primaryIdx, face },
          cb,
        );
        piece.dataset.track = String(tracks.length);
        piece.dataset.idx = String(placed.length);
        segEls[k].appendChild(piece);
        pieces.push(piece);
      }
      placed.push({ slot, pieces, primary: pieces[primaryIdx] });
    }
    tracks.push(placed);
    sheet.appendChild(rowEl);
  }

  // Quiet ground under short grids: the graticule continues beneath the
  // last track (flex-grown to the viewport's min-height), so a
  // one-channel install still reads as an instrument -- empty traces,
  // not a sliver over a void -- and the sweep line runs the full glass.
  const ground = el("div", "guide-ground");
  ground.appendChild(el("div", "guide-ground__rail"));
  ground.appendChild(el("div", "guide-ground__track"));
  sheet.appendChild(ground);

  const nowline = el("div", "guide-nowline");
  nowline.hidden = true;
  sheet.appendChild(nowline);

  // Roving tabindex: exactly one slot is tabbable, always via its
  // primary piece. Start on the slot nearest now on the first non-empty
  // track (the sweep is the focal moment; keyboard entry lands beside it).
  let active: HTMLButtonElement | null = null;
  function setActive(btn: HTMLButtonElement, focus: boolean): void {
    if (active) active.tabIndex = -1;
    active = btn;
    btn.tabIndex = 0;
    if (focus) btn.focus();
  }
  const firstTrack = tracks.find((t) => t.length > 0);
  if (firstTrack) {
    const idx = nearestSlotIndex(
      firstTrack.map((p) => p.slot.startMs),
      Date.now(),
    );
    setActive(firstTrack[Math.max(0, idx)].primary, false);
  }

  sheet.addEventListener("keydown", (event) => {
    const target = event.target as HTMLElement;
    if (!(target instanceof HTMLButtonElement) || target.dataset.track === undefined) return;
    const t = Number(target.dataset.track);
    const i = Number(target.dataset.idx);
    if (event.key === "ArrowLeft" || event.key === "ArrowRight") {
      // One logical slot = one stop, even when its DOM is segmented
      // across midnight -- ±1 walks the whole week's slots on the track.
      const next = tracks[t][i + (event.key === "ArrowRight" ? 1 : -1)];
      if (next) setActive(next.primary, true);
      event.preventDefault();
    } else if (event.key === "ArrowUp" || event.key === "ArrowDown") {
      const dir = event.key === "ArrowDown" ? 1 : -1;
      // Skip empty tracks; land on the slot nearest this one's start.
      for (let nt = t + dir; nt >= 0 && nt < tracks.length; nt += dir) {
        if (tracks[nt].length === 0) continue;
        const idx = nearestSlotIndex(
          tracks[nt].map((p) => p.slot.startMs),
          tracks[t][i].slot.startMs,
        );
        setActive(tracks[nt][idx].primary, true);
        break;
      }
      event.preventDefault();
    }
    // Enter/Space activate the button natively (click -> onOpen).
  });

  container.replaceChildren(sheet);

  /** The day segment containing nowMs -- indexed against real local
   * midnights (dayStarts), so DST-shifted days keep their clamped
   * 288-column geometry. */
  function dayIndexOf(nowMs: number): number {
    let k = 0;
    while (k + 1 < dayCount && nowMs >= dayStarts[k + 1]) k++;
    return k;
  }

  function updateNow(nowMs: number): void {
    const inWeek = nowMs >= weekStartMs && nowMs < weekEndMs;
    nowline.hidden = !inWeek;
    if (inWeek) {
      // Week-relative: whole day segments before now (24h of --div each)
      // plus the minutes into now's own day, clamped to its 288 columns
      // (the DST fall-back day is 25h; raw elapsed minutes would draw
      // the line past its segment).
      const k = dayIndexOf(nowMs);
      const min = Math.max(0, Math.min(24 * 60, Math.floor((nowMs - dayStarts[k]) / 60_000)));
      sheet.style.setProperty("--now-min", String(k * 24 * 60 + min));
    }
    for (const track of tracks) {
      for (const p of track) {
        const past = p.slot.endMs <= nowMs;
        const onAir = p.slot.startMs <= nowMs && nowMs < p.slot.endMs;
        for (const piece of p.pieces) {
          piece.classList.toggle("is-past", past);
          piece.classList.toggle("is-on-air", onAir);
        }
      }
    }
  }

  function nowOffsetPx(nowMs: number): number | null {
    if (nowMs < weekStartMs || nowMs >= weekEndMs) return null;
    // --px-per-min derives from --div; read the computed division width
    // off the ruler cell so the offset matches whatever --div resolves to.
    const cell = sheet.querySelector(".guide-ruler__cell");
    if (!cell) return null;
    const pxPerMin = cell.getBoundingClientRect().width / 30;
    const railW = sheet.querySelector(".guide-ruler__corner")?.getBoundingClientRect().width ?? 0;
    const k = dayIndexOf(nowMs);
    const min = Math.max(0, Math.min(24 * 60, (nowMs - dayStarts[k]) / 60_000));
    return railW + (k * 24 * 60 + min) * pxPerMin;
  }

  return { updateNow, nowOffsetPx };
}

// ---- mobile rundown --------------------------------------------------------

export interface RundownHandle {
  updateNow(nowMs: number): void;
}

/**
 * The §7 mobile treatment: a vertical day-grouped rundown for ONE channel
 * (the mobile channel picker chooses which), paged by the SAME week
 * pager as the grid -- its day sections span the visible week page only.
 * TONIGHT / TOMORROW / "MON 02" headings (firstDayIndex keeps them
 * window-absolute), chronological slot rows, the now-line as a
 * horizontal rule re-slotted between past and future rows each minute.
 */
export function renderRundown(
  container: HTMLElement,
  row: GuideRow | null,
  weekStartMs: number,
  firstDayIndex: number,
  dayCount: number,
  cb: GridCallbacks,
): RundownHandle {
  const root = el("div", "rundown");
  const entries: PlacedSlot[] = [];
  const nowRule = el("li", "rundown-nowrule");
  nowRule.setAttribute("role", "presentation");
  const nowRuleLabel = el("span", "rundown-nowrule__label", "NOW");
  nowRule.appendChild(nowRuleLabel);

  const lists: { dayStartMs: number; dayEndMs: number; listEl: HTMLOListElement; items: PlacedSlot[] }[] = [];

  for (let k = 0; k < dayCount; k++) {
    const dayStartMs = addDays(weekStartMs, k);
    const dayEndMs = addDays(weekStartMs, k + 1);
    // Overlap grouping (not start-time): an overnight slot lists under
    // BOTH days, its second appearance marked as a continuation --
    // rundown parity with the grid's flush joins.
    const daySlots = rundownDaySlots(row?.slots ?? [], dayStartMs, dayEndMs);
    if (daySlots.length === 0) continue;

    const section = el("section", "rundown-day");
    section.appendChild(el("h3", "rundown-day__head", rundownDayHeading(firstDayIndex + k, dayStartMs)));
    const list = el("ol", "rundown-list");
    const items: PlacedSlot[] = [];
    for (const { slot, continuation } of daySlots) {
      const item = el("li", "rundown-item");
      const btn = el("button", slot.kind === "ghost" ? "rundown-slot rundown-slot--ghost" : "rundown-slot");
      btn.type = "button";
      btn.dataset.type = slot.blockType;
      btn.setAttribute(
        "aria-label",
        continuation
          ? `${slotAriaLabel(slot)} (continued from the previous day, until ${formatClock(slot.endMs)})`
          : slotAriaLabel(slot),
      );
      if (cb.inspectorId) {
        btn.setAttribute("aria-controls", cb.inspectorId);
        btn.setAttribute("aria-expanded", "false");
      }
      btn.appendChild(
        el("span", "rundown-slot__time", continuation ? `cont'd · until ${formatClock(slot.endMs)}` : slotTimeRange(slot)),
      );
      const body = el("span", "rundown-slot__body");
      if (slot.kind === "ghost") {
        body.appendChild(el("span", "guide-slot__name", "NO SIGNAL"));
        body.appendChild(el("span", "guide-slot__meta", `LOST TO ${(slot.lostTo ?? "").toUpperCase()}`));
      } else {
        body.appendChild(el("span", "guide-slot__name", slot.blockName));
        const meta = [`${slot.programs.length} PROG`];
        if (slot.blockType !== "") meta.push(slot.blockType.toUpperCase());
        body.appendChild(el("span", "guide-slot__meta", meta.join(" · ")));
      }
      btn.appendChild(body);
      btn.addEventListener("click", () => cb.onOpen(slot, btn));
      item.appendChild(btn);
      list.appendChild(item);
      const placed = { slot, pieces: [btn], primary: btn };
      items.push(placed);
      entries.push(placed);
    }
    lists.push({ dayStartMs, dayEndMs, listEl: list, items });
    section.appendChild(list);
    root.appendChild(section);
  }

  container.replaceChildren(root);

  function updateNow(nowMs: number): void {
    for (const p of entries) {
      p.primary.classList.toggle("is-past", p.slot.endMs <= nowMs);
      p.primary.classList.toggle("is-on-air", p.slot.startMs <= nowMs && nowMs < p.slot.endMs);
    }
    nowRule.remove();
    const today = lists.find((l) => nowMs >= l.dayStartMs && nowMs < l.dayEndMs);
    if (!today) return;
    nowRuleLabel.textContent = `NOW · ${formatClock(nowMs)}`;
    const firstFuture = today.items.find((p) => p.slot.startMs > nowMs);
    if (firstFuture) {
      today.listEl.insertBefore(nowRule, firstFuture.primary.parentElement);
    } else {
      today.listEl.appendChild(nowRule);
    }
  }

  return { updateNow };
}
