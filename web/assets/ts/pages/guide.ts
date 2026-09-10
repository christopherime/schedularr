// The Guide ("/"): the EPG grid as home (spec §3.1 + §3.2, full-week
// grid since v0.5.3) and, since the draft slice, the one surface that
// plans and applies (spec §3.3). Two plans can be on the glass:
//
//   READING -- GET /schedule?days=28, ALL channels, auto-loaded on open
//              and re-fetched after every apply. Committed mode shows
//              it; SCOPE never narrows it.
//   DRAFT   -- POST /generate for a SCOPE over DRAFT_DAYS, armed for
//              apply and rendered as a diff overlay on the same grid
//              (runtime/draft.ts computes the verdicts and the copy).
//
// Armed-signature discipline (spec §3.3): APPLY sends exactly
// draftRequestBody(draft.signature) -- the body the preview on the glass
// was built from -- never one rebuilt from the live controls. A SCOPE
// change replaces the draft (re-preview), and nothing else can change
// the request, so the rule holds by construction.
//
// Honesty boundary: nothing on the client knows Tunarr's current
// lineup. Every verdict and every count is "vs the reading taken HH:MM"
// and the copy says so; answering "what does Tunarr hold right now"
// needs the enriched history of the Memory slice.
//
// Division of labor (spec non-negotiable): the grid + rundown DOM is
// built in TS by runtime/grid.ts from the typed plan; Alpine drives ONLY
// the toolbar (SCOPE / arm / week pager / mobile channel picker), the
// draft bar, and the slot inspector. The client fetches the FULL
// plannable window once (days=28, the API's practical max) per load and
// pages four weeks client-side -- the ‹/› week pager never re-plans and
// never touches the draft; only arming does.
//
// The sweep cursor advances on its OWN local 60s timer, reading
// runtime/bus.ts's serverNow() so the heartbeat's skew correction lands
// on it -- never on a stream frame. A minute that only turns when the
// server has something to say would stop turning the moment the link
// drops, which is exactly when an operator looks hardest at the cursor.
//
// The live link (v0.5.9) only ever tells this page "go look":
// apply.completed and plan.invalidated carry no plan. What the page may
// do about that is a GUARD, not a reflex -- see liveAction below. An
// auto-refetch that discarded an armed draft or re-rendered a half-read
// inspector out from under the operator would be worse than having no
// live link at all.
//
// A hidden tab is DEAF, not merely quiet: shell.ts drops the stream
// while the tab is hidden, so nothing arrives to defer in the first
// place. That is why this page registers its primary GET on shell.ts's
// onResume, behind the same guard -- the decision is simply re-run the
// moment the tab is looked at again. Without that hook a deferral is a
// silent drop, and Last-Event-ID replay cannot cover for it either: the
// hub's ring holds 128 events and a long-hidden tab outruns it.
import { ApiError, LONG_GET_TIMEOUT_MS, LONG_SEND_TIMEOUT_MS, apiGet, apiPath, apiSend, onReauth } from "../runtime/api.ts";
import type { ApiResponse } from "../runtime/api.ts";
import { serverNow, subscribe } from "../runtime/bus.ts";
import { channelHint as channelHintText, channelLabel, channelOrder, channelPlate, loadChannels } from "../runtime/channels.ts";
import type { Channel, PlateParts } from "../runtime/channels.ts";
import { cronReadback } from "../runtime/cron.ts";
import {
  DRAFT_DAYS,
  READING_STORAGE_KEY,
  appliedTapeLine,
  applyConfirmBody,
  applyConfirmTitle,
  applyingLine,
  diffRows,
  draftBarLine,
  draftRequestBody,
  draftScopeFromSearch,
  draftVerdictLine,
  draftingLine,
  parseStoredReading,
  planChannelCount,
  planSlotCount,
  rowsFromStored,
  serializeReading,
} from "../runtime/draft.ts";
import type { DiffCounts, DraftSignature } from "../runtime/draft.ts";
import { problemLine, toProblemView } from "../runtime/errors.ts";
import type { ProblemView } from "../runtime/errors.ts";
import { durationLabel, formatClock, ordinal, plural, sxxeyy } from "../runtime/format.ts";
import {
  addDays,
  localDayStart,
  renderGuideWeek,
  renderRundown,
  resolveGhost,
  weekChunk,
  weekPageCount,
  weekRangeLabel,
  windowDayCount,
} from "../runtime/grid.ts";
import type { GhostBlockInfo, GridHandle, GuideRow, GuideSlot, RundownHandle } from "../runtime/grid.ts";
import { initShell, onResume } from "../runtime/shell.ts";
import { printTape } from "../runtime/tape.ts";
import type { components } from "../gen/types";

initShell();

type PlanResult = ApiResponse<"getSchedule", 200>;
type Status = ApiResponse<"getStatus", 200>;
type BlockRecord = components["schemas"]["BlockRecord"];

/** The reading fetched by the guide (spec §3.1): the whole 28-day
 * window, ALL channels, paged client-side. Drafts plan DRAFT_DAYS of it. */
const FETCH_DAYS = 28;

// The honest first-load line: GET /schedule re-plans the full window
// against Tunarr, and the first call after a restart (cold pod, Tunarr
// itself waking) can genuinely take this long -- the request runs on
// the 90s LONG_GET_TIMEOUT_MS tier, so say so instead of looking hung.
const LOADING_LINE = "Loading programme guide — first load after a restart can take a minute";

// Stand-ins for a draft with no counts or no rows yet: the bar is
// hidden then, but Alpine still evaluates its bindings, so they read as
// "no changes" rather than throwing -- and sharing one empty array keeps
// the identity-keyed diff memo from thrashing.
const NO_COUNTS: DiffCounts = { new: 0, changed: 0, same: 0, removed: 0 };
const NO_ROWS: GuideRow[] = [];

declare const Alpine: {
  data<T extends object>(name: string, factory: () => T): void;
};

// Alpine magics injected at runtime (same ThisType erasure trick as
// blocks.ts uses for $nextTick).
interface WithMagics {
  $nextTick(cb: () => void): void;
  $refs: { confirmDialog: HTMLDialogElement };
  $root: HTMLElement;
}

// Same double-init defense as every page: Alpine auto-invokes init().
// Task 8 retired the page-level one-shots that existed to keep a second
// STREAM from opening -- shell.ts owns that guard now, and the live-link
// subscriptions below are made on init like any other wiring. This one
// stays for the rest of what init() does: a second invocation would add
// a second 60s sweep timer, a second onReauth handler, a second onResume
// handler and a duplicate pair of bus subscriptions, none of which is
// unsubscribed (the component lives as long as the page).
let started = false;

// DOM references the renderer hands back live OUTSIDE Alpine state --
// wrapping live elements in Alpine's reactive proxy buys nothing and
// risks proxy-vs-identity surprises on focus return.
let gridHandle: GridHandle | null = null;
let rundownHandle: RundownHandle | null = null;
let inspectorReturnEl: HTMLElement | null = null;

/** The projected renderable model plus the honesty counter: warnings
 * that could not be placed as ghosts (blocks enrichment failed, or a
 * block vanished between plan and render). */
interface RowsProjection {
  rows: GuideRow[];
  dropped: number;
}

// projectPlan() memo: ONE slot, holding the last (plan, blocksByName,
// channels) triple it projected. The GuideSlot graph is identical for a
// given triple, so what this saves is the SAME plan projected again --
// droppedWarnings() re-reads the reading's projection on every Alpine
// evaluation, and that projection is ~60-90k allocations at scale. It
// does not span two plans: reproject() projects the reading and then the
// draft, and in draft mode they evict each other. That is fine -- both
// results are kept on the component (readingRows, draft.rows), and
// reproject() only runs when late enrichment forces a redraw anyway.
// Keyed on reference identity; reload(), preview() and loadChannels()
// replace those objects wholesale, which is the only way they change.
let rowsMemo: {
  plan: object;
  blocks: object;
  channels: object;
  value: RowsProjection;
} | null = null;

interface DiffResult {
  rows: GuideRow[];
  counts: DiffCounts;
}

// draftDiff() memo: rows() is the hot getter (renderAll, the rundown,
// the Alpine x-for over rundownChannels, every empty-state guard), and
// in draft mode each read would otherwise re-diff two whole plans.
// Keyed on the identity of the two row sets and the channel cache (the
// row order depends on it) -- all three are replaced wholesale.
let diffMemo: {
  reading: object;
  draft: object;
  channels: object;
  value: DiffResult;
} | null = null;

export type DraftMode = "off" | "previewing" | "armed" | "applying";

interface DraftState {
  mode: DraftMode;
  signature: DraftSignature | null;
  plan: PlanResult | null;
  rows: GuideRow[] | null;
  dropped: number;
  /** When the draft request was sent: the diff cutoff and horizon start. */
  requestedAt: number;
  counts: DiffCounts | null;
  previewError: ProblemView | null;
  applyError: ProblemView | null;
  /** A client-side abort (status 0): the apply may have partially landed. */
  applyTimedOut: boolean;
  /** Any apply failure: the reading may no longer match Tunarr, so
   * DISCARD re-fetches instead of restoring it. */
  applyFailed: boolean;
  /** In-flight guard: a preview that lands after a newer one started is
   * dropped on the floor. */
  seq: number;
  /** A SCOPE change or Arm press that landed during a reload, preview,
   * or apply -- re-fired once the flight lands. */
  pendingScope: boolean;
  /** The live link announced a change AFTER this draft was generated:
   * the plan on the glass was built from a source that has since moved,
   * so it stays readable but may no longer be applied. Only a fresh
   * preview clears it -- see onLiveChange. */
  sourceChanged: boolean;
}

function emptyDraft(seq = 0): DraftState {
  return {
    mode: "off",
    signature: null,
    plan: null,
    rows: null,
    dropped: 0,
    requestedAt: 0,
    counts: null,
    previewError: null,
    applyError: null,
    applyTimedOut: false,
    applyFailed: false,
    seq,
    pendingScope: false,
    sourceChanged: false,
  };
}

// ---- the live link ---------------------------------------------------------

/**
 * How long a burst of announcements is allowed to settle before the
 * guide re-reads. One apply publishes apply.completed once, but a
 * blocks-page session publishes plan.invalidated per write: saving three
 * blocks in another tab would otherwise cost three full 28-day re-plans
 * against Tunarr, each one redrawing the sheet under the operator.
 * Trailing edge, so the re-read is issued after the LAST write -- a
 * leading-edge refetch would race the very change it was announcing.
 */
export const LIVE_REFETCH_MS = 2_000;

/** The pinned notice, and its button's label. The reading on the glass
 * no longer matches the server, and while the operator is mid-task it is
 * THEY who decide when it is replaced -- the page only says so. */
export const LINEUP_CHANGED_LINE = "Lineup changed — refresh";

/** The draft bar's line once an announced change has disarmed a draft.
 * A draft is a promise about a source; when the source moves the promise
 * is void, and only a fresh preview can make it good again. */
export const DRAFT_STALE_LINE = "Source changed — re-preview";

/** The two timer calls debounce() needs, injected rather than called
 * through window: it is what lets the coalescing be pinned by a test on
 * a fake clock instead of by waiting two real seconds. */
export interface Timers {
  set(cb: () => void, ms: number): number;
  clear(handle: number): void;
}

export interface Debounced {
  /** Restarts the wait: a burst runs `run` exactly once, `ms` after the
   * last call. */
  trigger(): void;
  /** Drops a pending run. reload() calls it -- a read already in flight
   * covers every change announced before it started. */
  cancel(): void;
}

export function debounce(ms: number, timers: Timers, run: () => void): Debounced {
  let handle: number | null = null;
  const cancel = (): void => {
    if (handle !== null) timers.clear(handle);
    handle = null;
  };
  return {
    trigger() {
      cancel();
      handle = timers.set(() => {
        // Cleared BEFORE run(): run() re-reads, reload() cancels, and a
        // cancel of a handle that has already fired would clear a timer
        // some later trigger owns.
        handle = null;
        run();
      }, ms);
    },
    cancel,
  };
}

/** Everything the two guards read, in one shape with no DOM in it. */
export interface GuideLiveness {
  /** "off" is the only idle mode: previewing and applying are flights,
   * and armed is a decision the operator has staged. */
  draftMode: DraftMode;
  inspectorOpen: boolean;
  /** A GET /schedule already in the air. */
  loading: boolean;
  /** document.hidden -- the tab is not being looked at. */
  hidden: boolean;
}

/**
 * True when the operator's own work is on the glass: a draft in any
 * state, or an open inspector. Refetching here would throw away a
 * preview they staged, or re-render the slot they are reading out of
 * existence, so the change is PINNED instead -- announced, and left for
 * them to act on. An event is a hint; a hint may not outrank a decision.
 */
export function liveRefetchBlocked(s: GuideLiveness): boolean {
  return s.draftMode !== "off" || s.inspectorOpen;
}

/**
 * True when the guide is idle in committed mode, visible, and has no
 * read of its own in flight -- the only state in which replacing the
 * reading costs the operator nothing.
 *
 * Neither of the two remaining refusals pins a notice, and neither one
 * drops the announcement: liveStale outlives both, and something re-runs
 * the decision. A hidden tab would pin a line nobody is reading, and
 * shell.ts's onResume re-fires the decision the moment it is looked at
 * again. A load in flight is already about to replace the reading, and
 * reload()'s tail re-runs the decision in case that response was built
 * before the write the announcement described.
 */
export function liveRefetchReady(s: GuideLiveness): boolean {
  return !liveRefetchBlocked(s) && !s.loading && !s.hidden;
}

/** What the page does about a change it has been told about, or about a
 * window in which it could not be told anything at all. */
export type LiveAction = "refetch" | "pin" | "defer";

/**
 * The whole decision, in one pure place: the debounce's fire and the tab
 * resume both route through it, so a resume can no more discard an armed
 * draft or a half-read inspector than a live event can.
 *
 * `announced` is whether a change is actually KNOWN to have happened
 * (liveStale). A resume arrives with it false: the tab was deaf, which is
 * a reason to re-read but not a reason to claim the lineup moved --
 * pinning LINEUP CHANGED on a hunch would make the one notice this page
 * has cheap. So an unannounced resume onto blocked work defers silently
 * and leaves the operator's staged work exactly as they left it.
 */
export function liveAction(s: GuideLiveness, announced: boolean): LiveAction {
  if (liveRefetchReady(s)) return "refetch";
  if (announced && liveRefetchBlocked(s)) return "pin";
  return "defer";
}

// Built in init(), so it closes over the one component on the page.
let liveRefetch: Debounced | null = null;
// A change has been announced that the reading on the glass does not
// include. Cleared only by a read that STARTS after the announcement.
let liveStale = false;
let staleNoticeEl: HTMLElement | null = null;

const browserTimers: Timers = {
  set: (cb, ms) => window.setTimeout(cb, ms),
  clear: (handle) => window.clearTimeout(handle),
};

/**
 * Pins the stale notice, reusing the drop legend's pinned-amber
 * vocabulary (.guide-droplegend) -- the shape this page already uses for
 * "the reading you are looking at is not the whole truth". Built here
 * rather than bound in layouts/index.html because it belongs to the live
 * link, not to the plan: the guide renders identically with no stream at
 * all, and the template should not imply otherwise.
 *
 * It mounts in the FIRST .guide-chrome -- the one outside .guide-body --
 * so it survives every state that hides the grid, and it is a button
 * because "refresh" has to be an affordance: the operator it is pinned
 * for is exactly the one the page refuses to re-read behind.
 */
function showStaleNotice(onRefresh: () => void): void {
  if (staleNoticeEl) return;
  const chrome = document.querySelector(".guide-chrome");
  if (!chrome) return;
  const btn = document.createElement("button");
  btn.type = "button";
  btn.id = "guide-stale";
  btn.className = "guide-droplegend";
  btn.textContent = LINEUP_CHANGED_LINE;
  btn.addEventListener("click", onRefresh);
  chrome.append(btn);
  staleNoticeEl = btn;
}

function hideStaleNotice(): void {
  staleNoticeEl?.remove();
  staleNoticeEl = null;
}

/** Mirrors the reading for the Blocks round trip (save -> PREVIEW ON
 * GUIDE): the diff baseline the operator last saw, per tab. */
function storeReading(requestedAt: number, rows: GuideRow[]): void {
  try {
    const raw = serializeReading(requestedAt, rows);
    if (raw !== null) window.sessionStorage.setItem(READING_STORAGE_KEY, raw);
  } catch {
    // Quota or a privacy lockdown: the mirror is a convenience for the
    // Blocks round trip, never load-bearing.
  }
}

function readStoredReading(nowMs: number, lastAppliedAt: string | null) {
  try {
    return parseStoredReading(window.sessionStorage.getItem(READING_STORAGE_KEY), nowMs, lastAppliedAt);
  } catch {
    return null;
  }
}

/** Drops the mirror when the reading behind it is gone: a Blocks round
 * trip must not restore a baseline this tab has already discarded. */
function dropStoredReading(): void {
  try {
    window.sessionStorage.removeItem(READING_STORAGE_KEY);
  } catch {
    // Same privacy lockdown storeReading() tolerates.
  }
}

/** Marks a container as carrying a draft: the dimmed `same` slots, the
 * removed lane, and the viewport's wider chrome budget all key off it. */
function setDraftFlag(el: HTMLElement | null, on: boolean): void {
  if (!el) return;
  if (on) el.dataset.draft = "";
  else delete el.dataset.draft;
}

/** A channel plate as one line of text: "CH 04 · HORROR", or the name
 * alone when the channel carries no number (an unresolved id shortens
 * to its own name). */
function plateText(plate: PlateParts): string {
  return plate.ch ? `${plate.ch} · ${plate.name}` : plate.name;
}

/** The SCOPE readout every line of draft copy names: "" is the whole
 * station, anything else is that channel's plate text. */
export function scopeLabelText(channelId: string, channels: Channel[]): string {
  return channelId === "" ? "ALL channels" : plateText(channelPlate(channelId, channels));
}

/** Diffed rows in channel order. diffRows returns the draft's channels
 * first and the reading-only ones after, so a channel the draft dropped
 * entirely (an all-removed row) would otherwise sink to the bottom of
 * the sheet instead of holding its place -- the committed projection
 * sorts by exactly this comparator. */
export function orderRows(rows: GuideRow[], channels: Channel[]): GuideRow[] {
  return [...rows].sort((a, b) => channelOrder(a.channelId, b.channelId, channels));
}

/** BlockRecords by name -- inspector enrichment (id for the editor
 * deep-link, enabled state) and ghost placement (see runtime/grid.ts's
 * resolveGhost). The catch is attached up front so a failure that lands
 * while the plan is still in flight degrades silently instead of
 * surfacing as an unhandled rejection: links fall back to /blocks/,
 * ghosts to the winner's channel. */
function loadBlockIndex(): Promise<Record<string, BlockRecord>> {
  return apiGet<BlockRecord[]>(apiPath("/blocks"))
    .catch((): BlockRecord[] | null => null)
    .then((blocks) => {
      const byName: Record<string, BlockRecord> = {};
      for (const b of blocks ?? []) byName[b.name] = b;
      return byName;
    });
}

interface InspectorState {
  open: boolean;
  slot: GuideSlot | null;
}

interface GuideState {
  controls: { channelId: string };

  channelsLoading: boolean;
  channelsError: string | null;
  channels: Channel[];

  loading: boolean;
  /** The visually-hidden role="status" line: announces the guide's
   * async states (loading / loaded / drafting / applied / unreachable)
   * to screen readers. */
  statusLine: string;
  problem: ProblemView | null;
  /** The reading's wire plan -- null while a restored reading is the
   * baseline, so nothing may read it as "a reading is on the glass". */
  plan: PlanResult | null;
  /** The reading's projected rows: what committed mode renders and what
   * every draft is diffed against. */
  readingRows: GuideRow[];
  /** When the reading's request was sent -- the clock every verdict is
   * measured against ("vs reading 21:02"). */
  readingRequestedAt: number;
  /** The reading came from the sessionStorage mirror, not the server:
   * it seeds the diff only, and leaving draft mode re-fetches. */
  readingRestored: boolean;
  blocksByName: Record<string, BlockRecord>;

  draft: DraftState;

  /** The loaded window: local midnight of day 0 + how many calendar
   * days it touches (a trailing partial day included -- see
   * windowDayCount). */
  windowStartMs: number;
  loadedDays: number;
  /** The week page on the glass: page k = window days k*7..k*7+6
   * (weekChunk). The grid, the pager label, and the mobile rundown all
   * follow it; chevrons disable at the window edges. */
  weekPage: number;

  rundownChannelId: string;
  inspector: InspectorState;

  init(): void;
  loadChannels(): Promise<void>;
  reload(): Promise<void>;
  openWithDraft(scope: string): Promise<void>;
  reproject(): void;
  channelLabel(c: Channel): string;
  channelHint(): string;

  requestDraft(): void;
  preview(): Promise<void>;
  draftOnGlass(): boolean;
  draftDiff(): DiffResult;
  draftBarLine(): string;
  scopeLabelFor(channelId: string): string;
  scopeLabel(): string;
  canApply(): boolean;
  requestApply(): void;
  confirmApply(): Promise<void>;
  cancelApply(force?: boolean): void;
  discardDraft(): void;
  focusArmSoon(): void;
  previewOrphansFocus(): boolean;
  focusDiscardSoon(): void;
  applyConfirmTitle(): string;
  applyConfirmBody(): string;

  weekPages(): number;
  weekLabel(): string;
  pageWeek(delta: number, navEl?: HTMLButtonElement): void;

  projectPlan(plan: PlanResult): RowsProjection;
  rows(): GuideRow[];
  droppedWarnings(): number;
  droppedLegendLine(): string;
  rundownRow(): GuideRow | null;
  rundownChannels(): { id: string; label: string }[];
  hasAnySlots(): boolean;
  hasBlocks(): boolean;
  renderAll(opts?: { drawIn?: boolean; settle?: boolean }): void;
  renderRundownOnly(): void;
  tick(): void;

  liveness(): GuideLiveness;
  onLiveChange(): void;
  fireLive(): void;
  pinStale(): void;

  openInspector(slot: GuideSlot, el: HTMLElement): void;
  closeInspector(returnFocus?: boolean): void;
  inspectorBlock(): BlockRecord | null;
  inspectorEditHref(): string;
  winnerEditHref(): string;
  inspectorVerdictLine(): string;
  inspectorTimeRange(): string;
  inspectorDuration(): string;
  inspectorPlate(): PlateParts;
  inspectorCron(): string;
  inspectorCronReadback(): string | null;
  inspectorPriority(): string;
  inspectorEnabled(): string | null;
  inspectorPrograms(): { start: string; title: string; marker: string | null; duration: string }[];
  problemLine(): string;
  programCountLabel(): string;
}

document.addEventListener("alpine:init", () => {
  Alpine.data(
    "guide",
    (): GuideState & ThisType<GuideState & WithMagics> => ({
      controls: { channelId: "" },

      channelsLoading: true,
      channelsError: null,
      channels: [],

      loading: true,
      statusLine: LOADING_LINE,
      problem: null,
      plan: null,
      readingRows: [],
      readingRequestedAt: 0,
      readingRestored: false,
      blocksByName: {},

      draft: emptyDraft(),

      windowStartMs: localDayStart(Date.now()),
      loadedDays: FETCH_DAYS,
      weekPage: 0,

      rundownChannelId: "",
      inspector: { open: false, slot: null },

      init() {
        if (started) return;
        started = true;
        void this.loadChannels();
        // ?draft=<id|all> is an arrival from a block save's PREVIEW ON
        // GUIDE: draft that scope at once instead of opening committed.
        const scope = draftScopeFromSearch(window.location.search);
        if (scope === null) void this.reload();
        else void this.openWithDraft(scope);
        onReauth(() => {
          if (this.channelsError) void this.loadChannels();
          if (this.problem) void this.reload();
          if (this.draft.previewError) void this.preview();
        });
        // The sweep's minute advance: a discrete step on a local 60s
        // timer, not an animation loop (motion inventory item 1). Local
        // on purpose -- the cursor must keep turning on POLL and on LINK
        // LOST, so it never rides a stream frame; tick()'s serverNow()
        // is what makes the step land on the SERVER's minute.
        window.setInterval(() => this.tick(), 60_000);
        // The live link. Both events say one thing to this page -- the
        // reading is out of date -- and both route through the one
        // guarded, debounced re-read. Never unsubscribed: the component
        // lives exactly as long as the page does, same as onReauth above.
        // Two seconds is long enough for the operator to have opened the
        // inspector or armed a draft since the announcement, so the
        // decision is taken here at fire time, not only on arrival.
        liveRefetch = debounce(LIVE_REFETCH_MS, browserTimers, () => this.fireLive());
        subscribe("apply.completed", () => this.onLiveChange());
        subscribe("plan.invalidated", () => this.onLiveChange());
        // The tab-resume half of the live link. A hidden tab has no
        // stream at all (shell.ts stops it), so on return this page must
        // re-read on its own account rather than wait to be told: the
        // resume replay only reaches back as far as the hub's 128-event
        // ring. Same decision, same guards -- coming back to a tab may
        // not cost the operator an armed draft either.
        //
        // Routed through onLiveChange(), NOT fireLive(), so it lands in
        // the same debounce window as the replay. shell.ts reconnects
        // before it fires the resume handlers, and that reconnect resumes
        // from Last-Event-ID -- so the frames this tab missed arrive at
        // almost the same moment. Calling fireLive() directly bought two
        // full 28-day re-plans for one resume, and let the replay pin
        // LINEUP CHANGED on a reading the resume had already refreshed.
        onResume(() => this.onLiveChange());
      },

      async loadChannels() {
        this.channelsLoading = true;
        this.channelsError = null;
        try {
          this.channels = await loadChannels();
        } catch (err) {
          const p = toProblemView(err);
          this.channelsError = p.detail ?? p.title;
          this.channels = [];
        } finally {
          this.channelsLoading = false;
        }
        // Plates resolve lazily: if a plan landed before the channel
        // list, re-project so UUID fallbacks become names.
        this.reproject();
      },

      // One load = plan + blocks, in parallel. The plan is load-bearing
      // (its failure IS the NO SIGNAL state); the blocks fetch only
      // enriches (inspector deep-links, ghost placement) and degrades
      // silently, matching the blocks editor's media-fetch convention.
      //
      // The reading is ALWAYS every channel and the whole window: SCOPE
      // is a draft control, so narrowing the reading would leave the
      // rest of the station unreadable while a draft is armed.
      async reload() {
        // A re-render is the closer here, not Esc/X: never return focus
        // to a slot node the reload is about to hide or discard.
        this.closeInspector(false);
        // This read supersedes every change announced before it: the
        // server publishes only after the write it names has committed,
        // so anything already announced is in the response.
        liveStale = false;
        liveRefetch?.cancel();
        hideStaleNotice();
        this.loading = true;
        this.problem = null;
        this.statusLine = LOADING_LINE;
        // Stamped BEFORE the send: every verdict is "vs the reading
        // taken at HH:MM", and that clock is when it was asked for.
        const requestedAt = Date.now();
        // The whole plannable window in one request, on the long read
        // tier (a cold-pod first plan can exceed a minute); the week
        // pager then works entirely client-side.
        const planPromise = apiGet<PlanResult>(
          apiPath("/schedule", undefined, { days: FETCH_DAYS }),
          LONG_GET_TIMEOUT_MS,
        );
        const blocksPromise = loadBlockIndex();
        try {
          this.plan = await planPromise;
          const landedAt = Date.now();
          this.windowStartMs = localDayStart(landedAt);
          // The server plans [now, now + 28*24h): any fetch after
          // midnight spills into a trailing partial calendar day that
          // becomes its own one-day week page.
          this.loadedDays = windowDayCount(landedAt, FETCH_DAYS);
          this.weekPage = 0;
        } catch (err) {
          this.problem = toProblemView(err);
          this.plan = null;
          // The reading this load was replacing goes with it. After an
          // apply the old one is already stale -- the push it describes
          // has landed -- so leaving it as the diff baseline would let
          // the latch below arm a draft that dates its verdicts to a
          // reading nobody can still see. Clearing the stamp is what
          // holds preview()'s reading gate; NO SIGNAL's Retry is the
          // recovery, here as on a failed first load.
          this.readingRows = NO_ROWS;
          this.readingRequestedAt = 0;
          this.readingRestored = false;
          dropStoredReading();
        }
        this.blocksByName = await blocksPromise;
        this.loading = false;
        if (this.plan) {
          const { rows, dropped } = this.projectPlan(this.plan);
          this.readingRows = rows;
          this.readingRequestedAt = requestedAt;
          this.readingRestored = false;
          // Mirrored on EVERY landing, an empty plan included: the
          // Blocks round trip diffs against what the operator last saw,
          // and "nothing scheduled" is a reading like any other.
          storeReading(requestedAt, rows);
          const programCount = rows.reduce(
            (n, r) => n + r.slots.reduce((m, s) => (s.kind === "slot" ? m + s.programs.length : m), 0),
            0,
          );
          let line = `Programme guide loaded, ${plural(programCount, "program")} across ${plural(rows.length, "channel")}`;
          if (dropped > 0) line += `; ${plural(dropped, "occurrence")} dropped by conflicts, placement unavailable`;
          this.statusLine = line;
          // No drawIn, and none is reachable: grid.ts decorates only
          // slots carrying a diff verdict (draft === "new" | "changed"),
          // and draft.ts's withVerdict is the sole thing that sets one --
          // a committed reading has none, so the trace draw-in has
          // nothing to draw. It is a draft-mode motion by construction
          // (preview() passes it); the committed sheet's counterpart is
          // discard()'s settle fade.
          this.renderAll();
        } else {
          this.statusLine = "Guide unavailable — Tunarr unreachable";
        }
        // A SCOPE change or Arm press that landed mid-flight fires now,
        // against the reading this load just put on the glass. A load
        // that failed leaves none there (the catch above clears the
        // stamp): drop the latch rather than loop on preview()'s guard
        // -- NO SIGNAL's Retry is the visible recovery.
        if (this.draft.pendingScope) {
          this.draft.pendingScope = false;
          if (this.readingRequestedAt !== 0) void this.preview();
        }
        // A change announced WHILE this load was in the air is NOT
        // covered by it -- the response may have been built before that
        // write committed. Re-run the guard against the reading that
        // just landed; it may now be blocked (the latch above can have
        // armed a draft) and pin instead.
        if (liveStale) liveRefetch?.trigger();
      },

      // What the two guards read, sampled at the moment of the decision:
      // the 2s debounce means arrival state and fire state genuinely
      // differ. document.hidden is absent under the test stubs, which
      // reads as visible -- the safe default, since the other three
      // conditions still hold the guard.
      liveness() {
        return {
          draftMode: this.draft.mode,
          inspectorOpen: this.inspector.open,
          loading: this.loading,
          hidden: document.hidden,
        };
      },

      // One announced change (apply.completed or plan.invalidated),
      // arriving. It is a hint with no plan in it: the repair is always
      // a fresh GET /schedule, and the only question is whether now is
      // the moment to take it.
      onLiveChange() {
        liveStale = true;
        if (liveRefetchBlocked(this.liveness())) {
          // A draft planned against a source that has since moved stays
          // readable but may not be applied: canApply() refuses, and the
          // bar says why. Cleared by the next preview -- the only thing
          // that can make the promise good again. Deliberately set for
          // an in-flight preview too: the server built that plan before
          // the change landed.
          if (this.draft.mode !== "off") this.draft.sourceChanged = true;
          this.pinStale();
          return;
        }
        liveRefetch?.trigger();
      },

      // The debounce firing, or the tab coming back. Both ask the same
      // question -- may the reading on the glass be replaced right now?
      fireLive() {
        const action = liveAction(this.liveness(), liveStale);
        if (action === "refetch") {
          void this.reload();
          return;
        }
        if (action === "pin") {
          this.pinStale();
          return;
        }
        // "defer", and explicitly so -- this is the branch that used to
        // fall through and lose the announcement. Nothing is dropped:
        // liveStale stays set, and whichever of the two deferrals this
        // is has something behind it. Hidden: onResume above re-fires
        // this the moment the tab is visible. Loading: reload()'s tail
        // re-triggers the debounce, because a response built before the
        // announced write does not cover it.
      },

      // The branch where re-reading would take something away from the
      // operator: say so instead, in the pinned line and in the
      // role="status" announcement, and leave the reading alone.
      pinStale() {
        this.statusLine = this.draft.mode === "off" ? LINEUP_CHANGED_LINE : DRAFT_STALE_LINE;
        showStaleNotice(() => void this.reload());
      },

      // Arrival from a block save's PREVIEW ON GUIDE (/?draft=<id|all>).
      // The param is consumed immediately so a refresh is a plain guide
      // load. The reading mirrored before the round trip seeds the diff
      // -- but it is NEVER rendered as the committed reading: a stale
      // baseline may not pass as current, so every path out of draft
      // mode from here re-fetches (see discardDraft / preview's catch).
      async openWithDraft(scope) {
        window.history.replaceState(null, "", "/");
        this.controls.channelId = scope;
        // A reading older than the server's last apply is not what the
        // operator would be diffing against -- /status dates it.
        const status = await apiGet<Status>(apiPath("/status")).catch((): Status | null => null);
        const stored = readStoredReading(Date.now(), status?.last_applied_at ?? null);
        if (!stored) {
          // The draft rides reload()'s latch: it fires the moment the
          // reading lands, and stays unfired when the fetch fails --
          // there is no honest diff without a reading.
          this.draft.pendingScope = true;
          await this.reload();
          return;
        }
        this.readingRows = rowsFromStored(stored, (id) => channelPlate(id, this.channels));
        this.readingRequestedAt = stored.requestedAt;
        this.plan = null;
        this.readingRestored = true;
        // `loading` deliberately STAYS true: a restored reading is never
        // painted as the committed grid, so until the draft lands there
        // is nothing to put on the glass. The skeleton and its honest
        // first-load note hold the frame instead of an empty stage --
        // the draft bar sits above both and already reads DRAFTING….
        // preview() clears the flag when the draft lands; its failure
        // path reloads, and reload() owns the flag from there.
        // The blocks index is enrichment on this path too (inspector
        // deep-links, and the ghosts of the draft that follows).
        void loadBlockIndex().then((byName) => {
          this.blocksByName = byName;
          this.reproject();
        });
        await this.preview();
      },

      // Late-landing enrichment (channels, blocks) changes projected
      // rows: re-project whatever plans are held, then redraw.
      reproject() {
        if (this.plan) {
          const projected = this.projectPlan(this.plan);
          this.readingRows = projected.rows;
        } else if (this.readingRestored) {
          // A restored reading has no plan to re-project: re-plate its
          // rows so late channel names replace the raw ids.
          this.readingRows = this.readingRows.map((r) => ({ ...r, plate: channelPlate(r.channelId, this.channels) }));
        }
        if (this.draft.plan) {
          const projected = this.projectPlan(this.draft.plan);
          this.draft.rows = projected.rows;
          this.draft.dropped = projected.dropped;
        }
        // Nothing is on the glass while the reading is still in flight,
        // and a RESTORED reading is never painted as the committed grid
        // -- it seeds the diff; the draft that follows is what gets
        // drawn.
        if (this.loading || (this.readingRestored && !this.draftOnGlass())) return;
        this.renderAll();
      },

      channelLabel,

      channelHint() {
        return channelHintText(
          this.channelsLoading,
          this.channelsError,
          this.channels,
          "enter a channel ID manually, or leave blank for all channels",
        );
      },

      // The two draft entry points (SCOPE change, Arm press) come here.
      // SCOPE is never disabled -- that would blur the focused control
      // and strand keyboard focus on body; the Arm button is disabled
      // only while previewing or applying, never by a load. Either way
      // a change or press landing mid-flight is latched here and
      // re-fired when the flight lands, with whatever SCOPE holds then.
      requestDraft() {
        if (this.loading || this.draft.mode === "previewing" || this.draft.mode === "applying") {
          this.draft.pendingScope = true;
          return;
        }
        void this.preview();
      },

      // POST /generate for the current SCOPE over the draft window, on
      // the long send tier (a cold re-plan against Tunarr outruns the
      // default). The signature captured here IS what APPLY sends.
      async preview() {
        // The honesty boundary makes the reading load-bearing: every
        // count and the bar's clock read "vs the reading taken HH:MM".
        // With no reading behind it a draft could only name a fabricated
        // one (the epoch), so ask for a reading first and let its
        // landing fire the latch below. An EMPTY reading is still a
        // reading -- this is the never-landed case alone (a failed first
        // load, a ?draft arrival whose fetch failed).
        if (this.readingRequestedAt === 0) {
          this.draft.pendingScope = true;
          if (!this.loading) void this.reload();
          return;
        }
        const signature: DraftSignature = { days: DRAFT_DAYS, channelId: this.controls.channelId.trim() };
        const requestedAt = Date.now();
        const seq = ++this.draft.seq;
        // The draw-in is a state change, never an entrance: it plays
        // only when the draft replaces a sheet ALREADY DRAWN (a SCOPE
        // change or an Arm press). gridHandle is the honest test --
        // a ?draft arrival off a restored reading has drawn nothing
        // yet, and animating there would be an entrance.
        const wasGridOnGlass =
          gridHandle !== null && !this.loading && !this.problem && this.rows().length > 0;
        // Read BEFORE the mode flip: the two controls that start a
        // preview are both about to leave the page under the operator's
        // finger (see previewOrphansFocus).
        const orphaned = this.previewOrphansFocus();
        this.draft.mode = "previewing";
        this.draft.previewError = null;
        // The re-preview the disarm asked for: this request is being
        // built from the source as it stands now.
        this.draft.sourceChanged = false;
        this.draft.applyError = null;
        this.draft.applyTimedOut = false;
        this.statusLine = draftingLine(this.scopeLabelFor(signature.channelId));
        if (orphaned) this.focusDiscardSoon();
        try {
          const result = await apiSend<PlanResult>(
            "POST",
            apiPath("/generate"),
            draftRequestBody(signature),
            LONG_SEND_TIMEOUT_MS,
          );
          if (seq !== this.draft.seq) return;
          const projected = this.projectPlan(result);
          this.draft.plan = result;
          this.draft.rows = projected.rows;
          this.draft.dropped = projected.dropped;
          this.draft.signature = signature;
          this.draft.requestedAt = requestedAt;
          // The draft's own window becomes the grid's: the reading's
          // later weeks stay pageable and render as `beyond`.
          this.windowStartMs = localDayStart(requestedAt);
          this.loadedDays = windowDayCount(requestedAt, FETCH_DAYS);
          this.weekPage = 0;
          this.draft.mode = "armed";
          this.draft.counts = this.draftDiff().counts;
          this.statusLine = this.draftBarLine();
          this.closeInspector(false);
          // A ?draft arrival left the skeleton up for exactly this
          // moment (see openWithDraft): the draft that just landed is
          // what replaces it.
          this.loading = false;
          this.renderAll({ drawIn: wasGridOnGlass });
        } catch (err) {
          if (seq !== this.draft.seq) return;
          this.draft.previewError = toProblemView(err);
          this.draft.mode = "off";
          this.draft.plan = null;
          this.draft.rows = null;
          this.draft.counts = null;
          this.statusLine = `Draft failed — ${problemLine(this.draft.previewError)}`;
          // Committed mode needs a reading to fall back to, and a
          // restored one may not pass as current.
          if (this.readingRestored) void this.reload();
          else this.renderAll();
        } finally {
          if (seq === this.draft.seq && this.draft.pendingScope && this.draft.mode !== "previewing") {
            this.draft.pendingScope = false;
            void this.preview();
          }
        }
      },

      draftOnGlass() {
        return this.draft.rows !== null && this.draft.mode !== "off";
      },

      draftDiff() {
        const readingRows = this.readingRows;
        const draftRows = this.draft.rows ?? NO_ROWS;
        if (
          diffMemo &&
          diffMemo.reading === readingRows &&
          diffMemo.draft === draftRows &&
          diffMemo.channels === this.channels
        ) {
          return diffMemo.value;
        }
        const { rows, counts } = diffRows(readingRows, draftRows, {
          requestedAt: this.draft.requestedAt,
          scopeChannelId: this.draft.signature?.channelId ?? "",
        });
        const value = { rows: orderRows(rows, this.channels), counts };
        diffMemo = { reading: readingRows, draft: draftRows, channels: this.channels, value };
        return value;
      },

      // The draft bar's one line, per state. (draftBarLine, draftingLine
      // and applyingLine below are runtime/draft.ts's copy functions,
      // not these methods -- every line of draft copy lives there.)
      draftBarLine() {
        const plan = this.draft.plan;
        if (this.draft.mode === "previewing" || !plan) return draftingLine(this.scopeLabel());
        const slots = planSlotCount(plan);
        const channels = planChannelCount(plan);
        if (this.draft.mode === "applying") return applyingLine(slots, channels);
        // The live link disarmed this draft: the counts below are still
        // true of the reading they were measured against, but that
        // reading is no longer the server's, so the bar reports the one
        // fact that now governs -- APPLY is off until a re-preview.
        if (this.draft.sourceChanged) return DRAFT_STALE_LINE;
        return draftBarLine({
          slots,
          channels,
          counts: this.draft.counts ?? NO_COUNTS,
          // Every occurrence this run lost to a conflict, whether or not
          // the grid could place a ghost for it: the bar reports the
          // run, and an operator reading "1 DROPPED" wants the count of
          // what the draft will not air. The narrower "could not be
          // placed" count is the amber legend line's job (see
          // droppedWarnings) -- that one is about the grid, not the run.
          dropped: plan.warnings?.length ?? 0,
          readingRequestedAt: this.readingRequestedAt,
        });
      },

      scopeLabelFor(channelId) {
        return scopeLabelText(channelId, this.channels);
      },

      // Armed copy names the scope the draft was PLANNED for, not
      // whatever SCOPE holds now -- the same armed-signature rule the
      // request body follows.
      scopeLabel() {
        const signature = this.draft.signature;
        const armed = this.draft.mode === "armed" || this.draft.mode === "applying";
        return this.scopeLabelFor(armed && signature ? signature.channelId : this.controls.channelId.trim());
      },

      // sourceChanged is the live link's veto: everything else here is
      // about whether a draft EXISTS, and that one is about whether it
      // still describes the server it would be pushed to.
      canApply() {
        return (
          this.draft.mode === "armed" &&
          !this.draft.sourceChanged &&
          this.draft.plan !== null &&
          this.draft.signature !== null
        );
      },

      requestApply() {
        if (!this.canApply()) return;
        this.$refs.confirmDialog.showModal();
      },

      // Sends the SAME body the preview sent (draftRequestBody over the
      // armed signature), never a body built from the live controls: the
      // applied scope must match what the dialog just named.
      async confirmApply() {
        const sig = this.draft.signature;
        const plan = this.draft.plan;
        if (!sig || !plan || this.draft.mode !== "armed") {
          this.cancelApply();
          return;
        }
        this.draft.mode = "applying";
        this.draft.applyError = null;
        this.draft.applyTimedOut = false;
        this.statusLine = applyingLine(planSlotCount(plan), planChannelCount(plan));
        try {
          const result = await apiSend<PlanResult>("POST", apiPath("/apply"), draftRequestBody(sig), LONG_SEND_TIMEOUT_MS);
          this.cancelApply(true);
          const slots = planSlotCount(result);
          const channels = planChannelCount(result);
          printTape(appliedTapeLine(slots, channels));
          this.statusLine = `Applied — ${plural(slots, "slot")} across ${plural(channels, "channel")}; re-reading the guide`;
          // A SCOPE change or Arm press latched during the push outlives
          // the reset: the reload below fires it against the reading it
          // is about to land, never against the one this apply just
          // aged out. SCOPE itself has snapped back to All channels by
          // then (the post-apply rule two lines down), so the re-fired
          // draft plans the scope the control now shows.
          const latched = this.draft.pendingScope;
          this.draft = emptyDraft(this.draft.seq);
          this.draft.pendingScope = latched;
          this.controls.channelId = "";
          this.closeInspector(false);
          this.focusArmSoon();
          // The reading is always re-fetched after an apply: the server
          // now replays what it just committed, and a merge of the old
          // reading with the applied scope would carry two ages.
          void this.reload();
        } catch (err) {
          this.cancelApply(true);
          this.draft.mode = "armed";
          this.draft.applyError = toProblemView(err);
          this.draft.applyTimedOut = err instanceof ApiError && err.status === 0;
          this.draft.applyFailed = true;
          this.statusLine = `Apply failed — ${problemLine(this.draft.applyError)}`;
          // A latched SCOPE change is dropped here rather than fired: a
          // fresh preview would clear the apply error the operator still
          // has to read (an aborted push may have partially landed), and
          // the bar keeps naming the scope the armed draft was planned
          // for. Arming again drafts the new scope.
          this.draft.pendingScope = false;
          this.focusArmSoon();
        }
      },

      // State-level guard, not just the dialog's :disabled/backdrop
      // checks: refuses to close while an apply is in flight. The two
      // closes confirmApply() itself performs pass force -- both happen
      // deliberately while the mode is still "applying".
      cancelApply(force = false) {
        if (this.draft.mode === "applying" && !force) return;
        this.$refs.confirmDialog.close();
      },

      // Back to committed mode. A restored baseline (or a reading an
      // apply may have invalidated) is re-fetched rather than restored;
      // anything else is already on the client, so the committed sheet
      // just fades back in.
      discardDraft() {
        const refetch = this.readingRestored || this.draft.applyFailed;
        // The sequence bump IS the discard for a preview still in the
        // air (Discard stays live while DRAFTING -- it is the only way
        // out of a 120s wait): the flight lands on a stale token and is
        // dropped, instead of arming a draft the operator abandoned for
        // a scope the SCOPE control no longer shows.
        this.draft = emptyDraft(this.draft.seq + 1);
        this.controls.channelId = "";
        this.closeInspector(false);
        this.focusArmSoon();
        if (refetch) {
          void this.reload();
          return;
        }
        this.renderAll({ settle: true });
        this.statusLine = "Draft discarded — reading restored";
      },

      // The one focus anchor when draft mode ends (apply, apply failure,
      // discard): the Arm button is present in every state and never
      // disabled by loading. Deferred one tick -- Alpine flushes the
      // :disabled binding the mode change just released on a queued
      // microtask, and focus() on a still-disabled button is a silent
      // no-op that strands the keyboard on <body>. Same idiom as
      // blocks.ts's focusReturnSoon().
      focusArmSoon() {
        this.$nextTick(() => {
          document.getElementById("guide-arm")?.focus();
        });
      },

      // True when the element the operator is on is about to leave the
      // page because a preview is starting. Arm's :disabled flips with
      // the mode, and a browser blurs an element it disables, so the
      // keypress that armed the draft would drop focus on <body> for the
      // whole flight -- up to 120 seconds, with nothing to hand it back
      // when the draft lands. A Retry or Re-draft button inside a draft
      // problem block goes the same way: preview() clears the error, and
      // x-if removes the block the button lives in. SCOPE is not in this
      // list -- it stays enabled and keeps focus by itself, which is
      // exactly why the template never disables it.
      previewOrphansFocus() {
        const active = document.activeElement;
        if (!active) return false;
        return active.id === "guide-arm" || active.closest(".guide-draftzone .problem") !== null;
      },

      // Discard is the handoff: it is on the glass for every moment of
      // DRAFTING, never disabled there, and it is the honest exit from
      // the wait. Deferred a tick for the same reason focusArmSoon() is
      // -- Alpine has not flushed the bar's x-show yet.
      focusDiscardSoon() {
        this.$nextTick(() => {
          document.getElementById("guide-discard")?.focus();
        });
      },

      applyConfirmTitle() {
        return applyConfirmTitle(this.scopeLabel());
      },

      applyConfirmBody() {
        const plan = this.draft.plan;
        if (!plan) return "";
        return applyConfirmBody({
          slots: planSlotCount(plan),
          channels: planChannelCount(plan),
          counts: this.draft.counts ?? NO_COUNTS,
          scopeLabel: this.scopeLabel(),
          readingRequestedAt: this.readingRequestedAt,
        });
      },

      weekPages() {
        return weekPageCount(this.loadedDays);
      },

      // The pager's readout: the visible week page's calendar range
      // ("SUN 30 AUG – SAT 05 SEP"; a trailing one-day page is just its
      // day). aria-live on the label announces paging.
      weekLabel() {
        const { startDay, days } = weekChunk(this.weekPage, this.loadedDays);
        return weekRangeLabel(addDays(this.windowStartMs, startDay), Math.max(1, days));
      },

      // ‹/›: page whole weeks, clamped to the window -- entirely
      // client-side, never a re-plan and never a change to the draft.
      // The grid and the mobile rundown re-render to the new page
      // together.
      pageWeek(delta, navEl) {
        const next = Math.min(this.weekPages() - 1, Math.max(0, this.weekPage + delta));
        if (next === this.weekPage) return;
        this.weekPage = next;
        // The re-render is about to discard the inspector's return
        // slot: never hand focus to a dying node.
        this.closeInspector(false);
        this.renderAll();
        // A chevron that just disabled itself (window edge reached)
        // would strand keyboard focus on body -- hand it to the
        // opposite chevron instead.
        this.$nextTick(() => {
          if (navEl?.disabled) {
            navEl.closest(".guide-pager")
              ?.querySelector<HTMLButtonElement>(".guide-pager__nav:not(:disabled)")
              ?.focus();
          }
        });
      },

      // The full renderable model for ONE plan (the reading or a draft):
      // one row per planned channel, plate resolved from the channel
      // cache, slots + ghosts sorted by start; plus the count of
      // warnings that could not be placed as ghosts. Memoized on (plan,
      // blocksByName, channels) identity.
      projectPlan(plan) {
        if (rowsMemo && rowsMemo.plan === plan && rowsMemo.blocks === this.blocksByName && rowsMemo.channels === this.channels) {
          return rowsMemo.value;
        }
        const rowsByChannel = new Map<string, GuideSlot[]>();
        for (const [channelId, slots] of Object.entries(plan.channels)) {
          const list: GuideSlot[] = [];
          for (const s of slots) {
            const startMs = s.start_time ? Date.parse(s.start_time) : NaN;
            const endMs = s.end_time ? Date.parse(s.end_time) : NaN;
            if (Number.isNaN(startMs) || Number.isNaN(endMs)) continue;
            list.push({
              kind: "slot",
              channelId,
              blockName: s.block?.name ?? "—",
              blockType: s.block?.type ?? "filter",
              cron: s.block?.cron ?? "",
              priority: s.block?.priority ?? 0,
              startMs,
              endMs,
              programs: (s.programs ?? []).map((p) => ({
                title: p.title,
                type: p.type,
                season: p.season,
                episode: p.episode,
                durationMs: p.duration_ms,
                startMs: Date.parse(p.start_time),
              })),
            });
          }
          rowsByChannel.set(channelId, list);
        }
        // Ghost slots for current-plan warnings, at their would-have-aired
        // time. A conflict is always same-channel, so the ghost's channel
        // always already has a row in the plan. A warning that cannot be
        // placed (blocks enrichment failed, block deleted, no matching
        // row) is COUNTED, never silently dropped -- the pinned legend
        // line above the grid keeps the reading honest.
        const ghostLookup = new Map<string, GhostBlockInfo>();
        for (const [name, b] of Object.entries(this.blocksByName)) {
          ghostLookup.set(name, { channelId: b.spec.channel_id, durationMinutes: b.spec.duration });
        }
        let dropped = 0;
        for (const w of plan.warnings ?? []) {
          const ghost = resolveGhost(w, ghostLookup);
          const list = ghost ? rowsByChannel.get(ghost.channelId) : undefined;
          if (ghost && list) list.push(ghost);
          else dropped++;
        }
        const rows = [...rowsByChannel.entries()]
          .map(([channelId, slots]) => ({
            channelId,
            plate: channelPlate(channelId, this.channels),
            slots: slots.sort((a, b) => a.startMs - b.startMs),
          }))
          .sort((a, b) => channelOrder(a.channelId, b.channelId, this.channels));
        const value = { rows, dropped };
        rowsMemo = { plan, blocks: this.blocksByName, channels: this.channels, value };
        return value;
      },

      // What the grid renders: the committed reading, or -- while a
      // draft is on the glass -- the reading diffed against it.
      rows() {
        return this.draftOnGlass() ? this.draftDiff().rows : this.readingRows;
      },

      droppedWarnings() {
        if (this.draftOnGlass()) return this.draft.dropped;
        return this.plan ? this.projectPlan(this.plan).dropped : 0;
      },

      droppedLegendLine() {
        return `${plural(this.droppedWarnings(), "occurrence")} dropped by conflicts — placement unavailable`;
      },

      rundownRow() {
        const rows = this.rows();
        if (rows.length === 0) return null;
        return rows.find((r) => r.channelId === this.rundownChannelId) ?? rows[0];
      },

      rundownChannels() {
        return this.rows().map((r) => ({ id: r.channelId, label: plateText(r.plate) }));
      },

      hasAnySlots() {
        return this.rows().some((r) => r.slots.length > 0);
      },

      hasBlocks() {
        return Object.keys(this.blocksByName).length > 0;
      },

      // Deferred one tick: renderAll is called right after reactive state
      // flips (loading -> false), and the viewport sits behind x-show --
      // rendering before Alpine applies the display change would measure
      // a display:none element (clientWidth 0) and the auto-scroll would
      // silently no-op.
      renderAll(opts = {}) {
        this.$nextTick(() => {
          const viewport = document.getElementById("guide-viewport");
          if (!viewport) return;
          // The whole page reads as drafting: the root widens the
          // viewport's chrome budget for the bar, the scrollers dim
          // their unchanged slots.
          setDraftFlag(this.$root, this.draftOnGlass());
          setDraftFlag(viewport, this.draftOnGlass());
          const rows = this.rows();
          const { startDay, days } = weekChunk(this.weekPage, this.loadedDays);
          const weekStartMs = addDays(this.windowStartMs, startDay);
          gridHandle = renderGuideWeek(viewport, rows, weekStartMs, Math.max(1, days), {
            onOpen: (slot, el) => this.openInspector(slot, el),
            inspectorId: "guide-inspector",
            drawIn: opts.drawIn,
          });
          gridHandle.updateNow(serverNow());
          // The committed sheet fading back after a discard that had
          // nothing to re-fetch (motion inventory item 3's other half).
          if (opts.settle) viewport.querySelector(".guide-sheet")?.classList.add("guide-sheet--settle");
          // Auto-scroll target: the sweep cursor when this week page
          // contains now (parked a third of the viewport in so the next
          // hours are visible); any other page opens at its start.
          // Initial positioning, not motion -- no smooth scrolling. The
          // nextTick above is not a guarantee the x-show display change
          // has painted, and a display:none viewport measures 0 -- wait
          // it out over a few frames instead of silently landing on 0.
          const scrollIntoWeek = (attempts: number): void => {
            if (attempts <= 0) return;
            if (viewport.clientWidth === 0) {
              requestAnimationFrame(() => scrollIntoWeek(attempts - 1));
              return;
            }
            const nowX = gridHandle?.nowOffsetPx(serverNow());
            viewport.scrollLeft = nowX != null ? Math.max(0, nowX - viewport.clientWidth / 3) : 0;
          };
          scrollIntoWeek(20);
          this.renderRundownOnly();
        });
      },

      renderRundownOnly() {
        const rundownEl = document.getElementById("guide-rundown");
        if (!rundownEl) return;
        setDraftFlag(rundownEl, this.draftOnGlass());
        const row = this.rundownRow();
        if (row) this.rundownChannelId = row.channelId;
        // The rundown pages with the SAME week pager as the grid: its
        // day sections span the visible week only, headings staying
        // window-absolute (TONIGHT is only ever the fetch day).
        const { startDay, days } = weekChunk(this.weekPage, this.loadedDays);
        rundownHandle = renderRundown(
          rundownEl,
          row,
          addDays(this.windowStartMs, startDay),
          startDay,
          Math.max(1, days),
          {
            onOpen: (slot, el) => this.openInspector(slot, el),
            inspectorId: "guide-inspector",
          },
        );
        rundownHandle.updateNow(serverNow());
      },

      // serverNow(), never Date.now(): the sweep is the one line an
      // operator reads the schedule AGAINST, so a laptop clock minutes
      // off the server's parks it in the wrong programme -- and both
      // handles must be told the same instant, or the grid and the
      // rundown disagree about what is on air.
      tick() {
        const now = serverNow();
        gridHandle?.updateNow(now);
        rundownHandle?.updateNow(now);
      },

      // Opening is made perceptible: the opener's aria-expanded flips
      // (every slot button carries aria-controls="guide-inspector"),
      // and focus moves to the inspector heading (tabindex="-1") once
      // Alpine has shown the panel -- which also puts the mobile bottom
      // sheet directly in the tab order instead of after the whole
      // remaining rundown.
      openInspector(slot, el) {
        if (inspectorReturnEl && inspectorReturnEl !== el) {
          inspectorReturnEl.setAttribute("aria-expanded", "false");
        }
        inspectorReturnEl = el;
        el.setAttribute("aria-expanded", "true");
        this.inspector.slot = slot;
        this.inspector.open = true;
        this.$nextTick(() => {
          document.getElementById("guide-inspector-title")?.focus();
        });
      },

      // Esc/X close with focus returned to the slot that opened it.
      // A re-render closer (pageWeek/reload/preview) passes
      // returnFocus=false: the return slot is about to be discarded, and
      // focusing a dying node strands keyboard focus on document.body.
      closeInspector(returnFocus = true) {
        if (!this.inspector.open) return;
        this.inspector.open = false;
        this.inspector.slot = null;
        inspectorReturnEl?.setAttribute("aria-expanded", "false");
        if (returnFocus) inspectorReturnEl?.focus();
        inspectorReturnEl = null;
      },

      inspectorBlock() {
        const slot = this.inspector.slot;
        if (!slot) return null;
        return this.blocksByName[slot.blockName] ?? null;
      },

      // Block-name deep link: /blocks/?edit=<id> opens the editor on that
      // block (blocks.ts reads the param after its list loads). Falls back
      // to the plain blocks page when the record didn't resolve.
      inspectorEditHref() {
        const record = this.inspectorBlock();
        return record ? `/blocks/?edit=${encodeURIComponent(record.id)}` : "/blocks/";
      },

      winnerEditHref() {
        const name = this.inspector.slot?.lostTo;
        const record = name ? this.blocksByName[name] : undefined;
        return record ? `/blocks/?edit=${encodeURIComponent(record.id)}` : "/blocks/";
      },

      // The draft verdict in one sentence, always naming the reading it
      // is measured against -- the chip's spoken long form.
      inspectorVerdictLine() {
        const slot = this.inspector.slot;
        if (!slot?.draft) return "";
        // Same clock as the sweep: "airing now" in this line and the
        // on-air piece on the grid must never disagree.
        const now = serverNow();
        return draftVerdictLine(slot.draft, this.readingRequestedAt, slot.startMs <= now && now < slot.endMs);
      },

      inspectorTimeRange() {
        const slot = this.inspector.slot;
        if (!slot) return "";
        return `${formatClock(slot.startMs)}–${formatClock(slot.endMs)}`;
      },

      inspectorDuration() {
        const slot = this.inspector.slot;
        if (!slot) return "";
        return durationLabel((slot.endMs - slot.startMs) / 60_000);
      },

      inspectorPlate() {
        return channelPlate(this.inspector.slot?.channelId, this.channels);
      },

      inspectorCron() {
        const slot = this.inspector.slot;
        if (!slot) return "";
        return slot.cron !== "" ? slot.cron : (this.inspectorBlock()?.spec.cron ?? "");
      },

      inspectorCronReadback() {
        const cron = this.inspectorCron();
        return cron === "" ? null : cronReadback(cron);
      },

      // §3.2 rank context: "50 · 2nd of 5" among ENABLED same-channel
      // blocks, computed from the already-loaded blocksByName.
      // Competition ranking, so ties share a rank (two blocks at 80 are
      // both 1st of n). Falls back to the bare number when the blocks
      // fetch failed (no peers resolvable).
      inspectorPriority() {
        const slot = this.inspector.slot;
        if (!slot) return "";
        const record = this.inspectorBlock();
        const priority = slot.kind === "ghost" ? (record?.spec.priority ?? slot.priority) : slot.priority;
        const channelId = record?.spec.channel_id ?? slot.channelId;
        const peers = Object.values(this.blocksByName).filter(
          (b) => b.enabled && b.spec.channel_id === channelId,
        );
        if (peers.length === 0) return String(priority);
        const higher = peers.filter((b) => (b.spec.priority ?? 0) > priority).length;
        return `${priority} · ${ordinal(higher + 1)} of ${peers.length}`;
      },

      // Enabled state comes from the BlockRecord (BlockSpec doesn't carry
      // it); null when the blocks fetch failed -- the row is omitted
      // rather than guessed.
      inspectorEnabled() {
        const record = this.inspectorBlock();
        if (!record) return null;
        return record.enabled ? "Enabled" : "Disabled";
      },

      inspectorPrograms() {
        const slot = this.inspector.slot;
        if (!slot) return [];
        return slot.programs.map((p) => ({
          start: Number.isNaN(p.startMs) ? "—" : formatClock(p.startMs),
          title: p.title.trim() === "" ? "—" : p.title,
          marker: sxxeyy(p.season, p.episode),
          duration: durationLabel(p.durationMs / 60_000),
        }));
      },

      // (problemLine below is runtime/errors.ts's, not this method.)
      problemLine() {
        return this.problem ? problemLine(this.problem) : "";
      },

      programCountLabel() {
        return plural(this.inspector.slot?.programs.length ?? 0, "program");
      },
    }),
  );
});
