// The guide's grid renderer (spec §3.1 + the resolved pre-slice spike).
// TS-built DOM: this module renders the EPG sheet and the mobile rundown
// directly from the typed plan; Alpine drives ONLY the toolbar and the
// inspector. Everything geometric goes through CSSOM
// (el.style.setProperty) -- the CSP's style-src 'self' silently drops
// inline style="..." attributes, and grid-column line numbers set via the
// CSSOM are not inline attributes (see DESIGN.md, Content-Security-Policy).
//
// Spike findings this file honors (measured, binding -- spec §9):
//   - one CSS grid PER CHANNEL TRACK, 288 five-minute fixed columns,
//     slots placed by grid-column line numbers via CSSOM;
//   - the sticky rail/ruler topology comes from FLEX FLOW, never grid
//     membership: sheet = block width:max-content; ruler = sticky-top
//     flex row with a sticky-left corner; each channel row = flex of
//     sticky-left plate + grid track (Chromium forgives sticky grid
//     items; Safari historically does not);
//   - the now-line is an absolute overlay child of the sheet at
//     calc(var(--rail-w) + var(--now-min) * var(--px-per-min)), with
//     --now-min set per minute via CSSOM -- no scroll handler;
//   - the graticule is a repeating-linear-gradient at --div, coinciding
//     exactly with ruler cells and slot boundaries (one division = 30
//     minutes of air time);
//   - z-order: nowline < plates < ruler < corner.
//
// The pure geometry below (quantum clamping, day windowing, ghost
// resolution, keyboard-nav picking) is exported for unit tests
// (web/tests/grid.test.ts) and never touches the DOM.

import type { PlateParts } from "./channels.ts";
import { formatClock, pad2, plural } from "./format.ts";

// ---- pure geometry ---------------------------------------------------------

/** The placement quantum: slots land on 5-minute grid lines. */
export const QUANTUM_MIN = 5;
/** Columns per day track: 24h / 5min. */
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

/** One slot's placement on a single day's 288-quantum track. cutLeft/
 * cutRight mark an overnight spill clamped at the day edge (the slot
 * continues on the neighboring day). */
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

/** Tab label for a day window's k-th day: "SAT 30". */
export function dayLabel(dayStartMs: number): string {
  const d = new Date(dayStartMs);
  const names = ["SUN", "MON", "TUE", "WED", "THU", "FRI", "SAT"];
  return `${names[d.getDay()]} ${pad2(d.getDate())}`;
}

/** Rundown group heading (spec §7): TONIGHT, TOMORROW, then "MON 02". */
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
 * carries only block names + occurrence_start until v0.5.3 adds
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
 * the contract in v0.5.3 -- so both are resolved client-side from the
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
}

export interface GridHandle {
  /** Repositions the now-line (CSSOM --now-min) and refreshes the
   * is-past / is-on-air classes. Driven by the guide page's local 60s
   * timer (heartbeat skew correction arrives with SSE in v0.5.4). */
  updateNow(nowMs: number): void;
  /** The x pixel offset of "now" inside the scroll viewport, or null
   * when now is outside the rendered day -- the auto-scroll target. */
  nowOffsetPx(nowMs: number): number | null;
}

interface PlacedSlot {
  slot: GuideSlot;
  el: HTMLButtonElement;
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

function buildSlotButton(slot: GuideSlot, span: SlotSpan, cb: GridCallbacks): HTMLButtonElement {
  const btn = el("button", slot.kind === "ghost" ? "guide-slot guide-slot--ghost" : "guide-slot");
  btn.type = "button";
  btn.tabIndex = -1;
  btn.dataset.type = slot.blockType;
  if (span.cutLeft) btn.dataset.cut = span.cutRight ? "both" : "left";
  else if (span.cutRight) btn.dataset.cut = "right";
  btn.style.setProperty("grid-column", gridColumn(span));
  btn.setAttribute("aria-label", slotAriaLabel(slot));
  if (slot.kind === "ghost") {
    btn.appendChild(el("span", "guide-slot__name", "NO SIGNAL"));
    btn.appendChild(el("span", "guide-slot__meta", `LOST TO ${(slot.lostTo ?? "").toUpperCase()}`));
  } else {
    btn.appendChild(el("span", "guide-slot__name", slot.blockName));
    btn.appendChild(el("span", "guide-slot__meta", slotMeta(slot)));
  }
  btn.addEventListener("click", () => cb.onOpen(slot, btn));
  return btn;
}

/** 48 ruler cells, one per --div (30 min): hour cells carry the label,
 * half-hour cells are a bare tick -- the ruler IS the graticule. */
function buildRuler(dayText: string): HTMLElement {
  const ruler = el("div", "guide-ruler");
  const corner = el("div", "guide-ruler__corner", dayText);
  ruler.appendChild(corner);
  const cells = el("div", "guide-ruler__cells");
  for (let half = 0; half < 48; half++) {
    const cell = el("div", "guide-ruler__cell");
    if (half % 2 === 0) cell.textContent = `${pad2(half / 2)}:00`;
    cells.appendChild(cell);
  }
  ruler.appendChild(cells);
  return ruler;
}

/**
 * Renders one day window of the guide sheet into `container` (replacing
 * whatever it held) and wires roving-tabindex keyboard navigation:
 * Left/Right along a track, Up/Down across channels (nearest start),
 * Enter/Space opens, Esc is left to the page (inspector close + focus
 * return). Returns the handle the 60s timer drives.
 */
export function renderGuideDay(
  container: HTMLElement,
  rows: GuideRow[],
  dayStartMs: number,
  cb: GridCallbacks,
): GridHandle {
  const dayEndMs = addDays(dayStartMs, 1);
  const sheet = el("div", "guide-sheet");
  sheet.appendChild(buildRuler(dayLabel(dayStartMs)));

  // tracks[t] mirrors rows[t]; each entry holds the day's placed slots in
  // start order -- the keyboard-nav model and the minute-tick refresh list.
  const tracks: PlacedSlot[][] = [];

  for (const row of rows) {
    const rowEl = el("div", "guide-row");
    const plateCell = el("div", "guide-plate-cell");
    plateCell.appendChild(plateEl(row.plate));
    rowEl.appendChild(plateCell);

    const track = el("div", "guide-track");
    const placed: PlacedSlot[] = [];
    for (const slot of row.slots) {
      const span = daySpan(slot.startMs, slot.endMs, dayStartMs, dayEndMs);
      if (!span) continue;
      const btn = buildSlotButton(slot, span, cb);
      btn.dataset.track = String(tracks.length);
      btn.dataset.idx = String(placed.length);
      track.appendChild(btn);
      placed.push({ slot, el: btn });
    }
    tracks.push(placed);
    rowEl.appendChild(track);
    sheet.appendChild(rowEl);
  }

  const nowline = el("div", "guide-nowline");
  nowline.hidden = true;
  sheet.appendChild(nowline);

  // Roving tabindex: exactly one slot is tabbable. Start on the slot
  // nearest now on the first non-empty track (the sweep is the focal
  // moment; keyboard entry lands beside it).
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
    setActive(firstTrack[Math.max(0, idx)].el, false);
  }

  sheet.addEventListener("keydown", (event) => {
    const target = event.target as HTMLElement;
    if (!(target instanceof HTMLButtonElement) || target.dataset.track === undefined) return;
    const t = Number(target.dataset.track);
    const i = Number(target.dataset.idx);
    if (event.key === "ArrowLeft" || event.key === "ArrowRight") {
      const next = tracks[t][i + (event.key === "ArrowRight" ? 1 : -1)];
      if (next) setActive(next.el, true);
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
        setActive(tracks[nt][idx].el, true);
        break;
      }
      event.preventDefault();
    }
    // Enter/Space activate the button natively (click -> onOpen).
  });

  container.replaceChildren(sheet);

  function updateNow(nowMs: number): void {
    const inDay = nowMs >= dayStartMs && nowMs < dayEndMs;
    nowline.hidden = !inDay;
    if (inDay) {
      const min = Math.floor((nowMs - dayStartMs) / 60_000);
      sheet.style.setProperty("--now-min", String(min));
    }
    for (const track of tracks) {
      for (const p of track) {
        p.el.classList.toggle("is-past", p.slot.endMs <= nowMs);
        p.el.classList.toggle("is-on-air", p.slot.startMs <= nowMs && nowMs < p.slot.endMs);
      }
    }
  }

  function nowOffsetPx(nowMs: number): number | null {
    if (nowMs < dayStartMs || nowMs >= dayEndMs) return null;
    // --px-per-min derives from --div; read the computed division width
    // off the ruler cell so the offset matches whatever --div resolves to.
    const cell = sheet.querySelector(".guide-ruler__cell");
    if (!cell) return null;
    const pxPerMin = cell.getBoundingClientRect().width / 30;
    const railW = sheet.querySelector(".guide-ruler__corner")?.getBoundingClientRect().width ?? 0;
    return railW + ((nowMs - dayStartMs) / 60_000) * pxPerMin;
  }

  return { updateNow, nowOffsetPx };
}

// ---- mobile rundown --------------------------------------------------------

export interface RundownHandle {
  updateNow(nowMs: number): void;
}

/**
 * The §7 mobile treatment: a vertical day-grouped rundown for ONE channel
 * (the mobile channel picker chooses which) -- TONIGHT / TOMORROW / "MON
 * 02" headings, chronological slot rows, the now-line as a horizontal
 * rule re-slotted between past and future rows each minute.
 */
export function renderRundown(
  container: HTMLElement,
  row: GuideRow | null,
  windowStartMs: number,
  days: number,
  cb: GridCallbacks,
): RundownHandle {
  const root = el("div", "rundown");
  const entries: PlacedSlot[] = [];
  const nowRule = el("li", "rundown-nowrule");
  nowRule.setAttribute("role", "presentation");
  const nowRuleLabel = el("span", "rundown-nowrule__label", "NOW");
  nowRule.appendChild(nowRuleLabel);

  const lists: { dayStartMs: number; dayEndMs: number; listEl: HTMLOListElement; items: PlacedSlot[] }[] = [];

  for (let k = 0; k < days; k++) {
    const dayStartMs = addDays(windowStartMs, k);
    const dayEndMs = addDays(windowStartMs, k + 1);
    const slots = (row?.slots ?? []).filter((s) => s.startMs >= dayStartMs && s.startMs < dayEndMs);
    if (slots.length === 0) continue;

    const section = el("section", "rundown-day");
    section.appendChild(el("h3", "rundown-day__head", rundownDayHeading(k, dayStartMs)));
    const list = el("ol", "rundown-list");
    const items: PlacedSlot[] = [];
    for (const slot of slots) {
      const item = el("li", "rundown-item");
      const btn = el("button", slot.kind === "ghost" ? "rundown-slot rundown-slot--ghost" : "rundown-slot");
      btn.type = "button";
      btn.dataset.type = slot.blockType;
      btn.setAttribute("aria-label", slotAriaLabel(slot));
      btn.appendChild(el("span", "rundown-slot__time", slotTimeRange(slot)));
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
      const placed = { slot, el: btn };
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
      p.el.classList.toggle("is-past", p.slot.endMs <= nowMs);
      p.el.classList.toggle("is-on-air", p.slot.startMs <= nowMs && nowMs < p.slot.endMs);
    }
    nowRule.remove();
    const today = lists.find((l) => nowMs >= l.dayStartMs && nowMs < l.dayEndMs);
    if (!today) return;
    nowRuleLabel.textContent = `NOW · ${formatClock(nowMs)}`;
    const firstFuture = today.items.find((p) => p.slot.startMs > nowMs);
    if (firstFuture) {
      today.listEl.insertBefore(nowRule, firstFuture.el.parentElement);
    } else {
      today.listEl.appendChild(nowRule);
    }
  }

  return { updateNow };
}
