// History page ("/history/"): the one searchable record, absorbing what
// used to be /series/ (tracked cursors) and /dashboard/ (recent history),
// plus the apply runs the Memory slice made durable. Three panes behind
// one band selector, deep-linkable as /history/?view=<pane>.
//
// Contract notes that shape the code below:
//
//   1. PATCH /state/series/{show_title} is a TRUE partial patch -- only a
//      field present in the body changes server-side, and an empty patch
//      is a 400. Every write here sends only what actually changed.
//   2. There is no create endpoint for a tracked show: a row exists only
//      once a sequence block airs it. The empty state explains that
//      rather than offering an Add button that could not work.
//   3. show_title is a path parameter and titles routinely contain
//      spaces and punctuation -- apiPath encodeURIComponent-encodes it.
//   4. GET /applies and GET /history are both bounded by their own
//      retention knob, so a 90-day window can legitimately return less.
//      The captions say so instead of implying the store lost data.
//   5. The live link refetches these feeds when the server says they
//      changed -- except TRACKED while a cursor is being typed.
//      loadStates() rebuilds every row's draft wholesale (it is the
//      page's deliberate resync point), so refetching mid-edit would
//      discard the edit in progress. Dropping a refetch is recoverable:
//      the next event, the operator's own save, changing the window, or
//      the tab-resume refetch (onResume, wired in init) all catch up --
//      and the last of those is reachable without knowing the live link
//      exists at all. The row's own Refresh List button is NOT one of
//      those paths: list.html only renders it inside the row-404 branch.
//      Discarding typed input is not recoverable, which is why the guard
//      errs toward keeping it.
//   6. An event-driven refetch is a BACKGROUND read (loadX(true)): it
//      never raises the pane's spinner or its error surface, because
//      list.html hides the rows behind both. A refresh nobody asked for
//      that fails must leave the last good reading on screen rather than
//      trading it for an error panel -- the same honest-instrument rule
//      the bezel follows. Only a read the operator asked for (first load,
//      window change, retry) may blank a pane.
import { ApiError, apiGet, apiPath, apiSend, onReauth } from "../runtime/api.ts";
import type { ApiRequestJSON, ApiResponse } from "../runtime/api.ts";
import { subscribe } from "../runtime/bus.ts";
import { channelLabel, channelPlate, loadChannels } from "../runtime/channels.ts";
import type { Channel, PlateParts } from "../runtime/channels.ts";
import { describeError, toProblemView } from "../runtime/errors.ts";
import type { ProblemView } from "../runtime/errors.ts";
import { durationLabel, formatLocal, pad2, plural } from "../runtime/format.ts";
import { initShell, onResume } from "../runtime/shell.ts";
import { printTape } from "../runtime/tape.ts";

initShell();

type SeriesRecord = ApiResponse<"listSeriesState", 200>[number];
type SeriesPatch = ApiRequestJSON<"patchSeriesState">;
type HistoryEntry = ApiResponse<"getHistory", 200>[number];
type ApplyRun = ApiResponse<"listApplyRuns", 200>[number];
type ApplyRunWarning = NonNullable<ApplyRun["warnings"]>[number];

declare const Alpine: {
  data<T extends object>(name: string, factory: () => T): void;
};

// ---- panes -----------------------------------------------------------------

/** The three panes, in band order. The query value is the pane id. */
export const PANES = ["tracked", "asrun", "runs"] as const;
export type Pane = (typeof PANES)[number];

/**
 * Resolves the pane a URL asks for. An unknown or absent ?view= falls
 * back to "tracked": a mistyped deep link should land on something
 * useful rather than a blank page.
 */
export function paneFromSearch(search: string): Pane {
  const value = new URLSearchParams(search).get("view");
  return (PANES as readonly string[]).includes(value ?? "") ? (value as Pane) : "tracked";
}

// ---- tracked ---------------------------------------------------------------

export interface RowDraft {
  season: string;
  episode: string;
}

function draftFromState(state: SeriesRecord): RowDraft {
  return { season: String(state.current_season), episode: String(state.current_episode) };
}

/**
 * True when a row's draft holds a cursor the operator typed but has not
 * saved. Exported and pure because it is what the live link consults
 * before refetching TRACKED (contract note 5 above) as well as what
 * enables the row's own Save button -- one rule, so the button and the
 * refetch guard can never disagree about whether a row is being edited.
 *
 * A row with no draft counts as clean: drafts are keyed by show_title
 * and rebuilt on every load, so a missing one means the row arrived
 * after the map was built, never that it holds hidden input.
 */
export function cursorDirty(
  state: { current_season: number; current_episode: number },
  draft: RowDraft | undefined,
): boolean {
  if (!draft) return false;
  return (
    draft.season.trim() !== String(state.current_season) ||
    draft.episode.trim() !== String(state.current_episode)
  );
}

/** SxxEyy, for the visually-hidden label read alongside the two separate
 * season/episode inputs -- kept distinct from the raw (unpadded) values
 * the inputs edit. */
function cursorLabel(season: number, episode: number): string {
  return `S${pad2(season)}E${pad2(episode)}`;
}

function runsLabel(n: number | undefined): string {
  return plural(n ?? 0, "run");
}

/** Parses a whole number >= 1, or null when the input isn't one. Anything
 * this doesn't catch (e.g. a season that simply doesn't exist for the
 * show) is left to the API's own 400. */
function parsePositiveInt(raw: string): number | null {
  const trimmed = raw.trim();
  if (!/^\d+$/.test(trimmed)) return null;
  const n = Number(trimmed);
  return Number.isFinite(n) && n >= 1 ? n : null;
}

/** True partial patch: only entries whose parsed value actually differs
 * from the loaded row are included. Null when there is nothing to send --
 * callers must treat that as a no-op rather than sending an empty body,
 * which the API rejects with a 400. */
function buildCursorPatch(state: SeriesRecord, season: number, episode: number): SeriesPatch | null {
  const patch: SeriesPatch = {};
  if (season !== state.current_season) patch.current_season = season;
  if (episode !== state.current_episode) patch.current_episode = episode;
  return Object.keys(patch).length > 0 ? patch : null;
}

/**
 * Narrows tracked rows to those whose title contains query, ignoring
 * case. An empty query matches everything -- the toolbar's search is a
 * filter, not a required argument.
 */
export function filterTracked<T extends { show_title: string }>(rows: T[], query: string): T[] {
  const needle = query.trim().toLowerCase();
  if (needle === "") return rows;
  return rows.filter((r) => r.show_title.toLowerCase().includes(needle));
}

// ---- as-run ----------------------------------------------------------------

/** One local day's airings, in air order. */
export interface DayGroup<T> {
  /** Local YYYY-MM-DD -- the group's stable key and heading source. */
  day: string;
  entries: T[];
}

/**
 * Buckets airings by the LOCAL day they were scheduled on -- an operator
 * reads a station log in station time, not UTC -- newest day first, with
 * each day's entries in ascending air order.
 */
export function groupByDay<T extends { scheduled_at?: string }>(entries: T[]): DayGroup<T>[] {
  const buckets = new Map<string, T[]>();
  for (const entry of entries) {
    if (!entry.scheduled_at) continue;
    const d = new Date(entry.scheduled_at);
    if (Number.isNaN(d.getTime())) continue;
    const day = `${d.getFullYear()}-${pad2(d.getMonth() + 1)}-${pad2(d.getDate())}`;
    const bucket = buckets.get(day);
    if (bucket) bucket.push(entry);
    else buckets.set(day, [entry]);
  }
  return [...buckets.entries()]
    .sort((a, b) => b[0].localeCompare(a[0]))
    .map(([day, list]) => ({
      day,
      entries: [...list].sort((a, b) => (a.scheduled_at ?? "").localeCompare(b.scheduled_at ?? "")),
    }));
}

/** The title as the operator knows it. An untitled row is an em dash --
 * never a fabricated name, and never the raw program UUID the old
 * dashboard table showed. The wire carries no season/episode on a
 * history row, so this deliberately does not append SxxEyy it would
 * have to invent. */
function airingLabel(entry: HistoryEntry): string {
  return entry.title?.trim() || "—";
}

// ---- runs ------------------------------------------------------------------

/** The subset of a run runSummary reads. */
export interface RunSummaryInput {
  status: string;
  scope?: string;
  days?: number;
  channel_count?: number;
  slot_count?: number;
}

/**
 * The card's one-line readout, in the draft bar's register (counts
 * first, uppercase). A run that did not finish successfully reports only
 * what it attempted -- the window and the scope -- because its slot and
 * channel counts never landed, and printing zeros would read as "applied
 * nothing" rather than "never got that far". The outcome word beside the
 * timestamp is what says how it ended; repeating it here would put the
 * same word on the card twice.
 */
export function runSummary(run: RunSummaryInput): string {
  const window = plural(run.days ?? 0, "DAY").toUpperCase();
  const scope = run.scope ? "SCOPED" : "ALL CHANNELS";
  if (run.status !== "ok") return `${window} · ${scope}`;

  const slots = plural(run.slot_count ?? 0, "SLOT").toUpperCase();
  const channels = plural(run.channel_count ?? 0, "CHANNEL").toUpperCase();
  return `${slots} ACROSS ${channels} · ${window} · ${scope}`;
}

// ---- live link -------------------------------------------------------------

/**
 * The tape line an apply.completed refresh earns, printed once both reads
 * have settled. A partial or total failure says so and names what is on
 * screen instead: the panes still hold their last good reading (contract
 * note 6), and claiming "refreshed" over data that is not refreshed is
 * the one thing an instrument may never do.
 */
export function applyRefreshLine(runsOk: boolean, historyOk: boolean): string {
  if (runsOk && historyOk) return "Apply landed — runs and as-run refreshed";
  if (runsOk) return "Apply landed — runs refreshed, as-run still showing the last reading";
  if (historyOk) return "Apply landed — as-run refreshed, runs still showing the last reading";
  return "Apply landed — refresh failed, both panes still showing the last reading";
}

// ---- component -------------------------------------------------------------

interface AiringRow extends HistoryEntry {
  key: string;
  clock: string;
  label: string;
  duration: string;
}

// ---- deletion ---------------------------------------------------------------

type StorageReport = ApiResponse<"getStorage", 200>;
type RemovalReport = ApiResponse<"removeSeriesState", 200>;
type BlockRecord = ApiResponse<"listBlocks", 200>[number];

/** What the confirm is armed to do once the operator presses through. */
type PendingRemoval =
  | { kind: "none" }
  | { kind: "show"; showTitle: string }
  | { kind: "range"; from: string; to: string };

/** The two fields blocksListingShow reads. Narrower than BlockRecord on
 * purpose: a predicate that demanded the whole wire shape would make
 * every caller and every fixture carry fields it never looks at. */
interface BlockLike {
  name: string;
  spec: { series?: { show_title: string }[] };
}

/**
 * Names every block whose spec lists showTitle, so the desk can show a
 * removal as blocked BEFORE the operator clicks rather than after the
 * server refuses it. The server still refuses -- this is the first step
 * of a two-step flow, not a replacement for the guard.
 *
 * Only sequence blocks list shows by title; a selection block matches on
 * criteria and names none, so it cannot block a removal here.
 */
export function blocksListingShow(blocks: BlockLike[], showTitle: string): string[] {
  const names: string[] = [];
  for (const block of blocks) {
    const listed = block.spec.series?.some((row) => row.show_title === showTitle);
    if (listed) names.push(block.name);
  }
  return names;
}

/**
 * Reads a datetime-local value as an instant. The control has no zone, so
 * the browser's own is the only honest reading: the operator picked the
 * time they see on the page, and the page renders in local time.
 * Undefined for a blank or unparseable field, which the caller treats as
 * an open end rather than as now.
 */
export function instantFromLocalInput(value: string): string | undefined {
  if (value.trim() === "") return undefined;
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) return undefined;
  return parsed.toISOString();
}

/** A datetime-local value for an ISO instant, for prefilling the form. */
export function localInputFromInstant(iso: string | undefined): string {
  if (!iso) return "";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  return `${d.getFullYear()}-${pad2(d.getMonth() + 1)}-${pad2(d.getDate())}T${pad2(d.getHours())}:${pad2(d.getMinutes())}`;
}

/**
 * The sentence the confirm shows. It names the real counts from the dry
 * run and states plainly that nothing brings the data back -- the same
 * honesty the empty states carry, on the one action that cannot be
 * walked back.
 */
export function removalConfirmBody(report: RemovalReport, subject: string): string {
  const parts = [`${report.airings.toLocaleString()} ${report.airings === 1 ? "airing" : "airings"}`];
  if (report.series_states) {
    parts.push(`${report.series_states} tracked ${report.series_states === 1 ? "cursor" : "cursors"}`);
  }
  if (report.snapshots) {
    parts.push(`${report.snapshots} ${report.snapshots === 1 ? "snapshot" : "snapshots"}`);
  }

  const emptied =
    report.emptied_slots > 0
      ? ` ${report.emptied_slots} ${report.emptied_slots === 1 ? "occurrence" : "occurrences"} will be left marked as having aired nothing, so a later apply does not re-plan them.`
      : "";

  return `Deleting ${subject} removes ${parts.join(", ")}.${emptied} This cannot be undone and nothing backfills it.`;
}

// Alpine's magics, reached through ThisType rather than declared as
// fields -- the object literal never supplies them. Same shape guide.ts
// uses for its own confirm dialog.
interface WithMagics {
  $refs: { confirmDialog: HTMLDialogElement };
}

interface HistoryPageState {
  view: Pane;
  days: number;
  channelId: string;
  blockQuery: string;
  search: string;
  source: string;

  channels: Channel[];

  statesLoading: boolean;
  statesError: ProblemView | null;
  states: SeriesRecord[];
  drafts: Record<string, RowDraft>;
  pending: Record<string, boolean>;
  rowErrors: Record<string, string | null>;
  rowNotFound: Record<string, boolean>;
  fieldErrors: Record<string, string | null>;

  historyLoading: boolean;
  historyError: ProblemView | null;
  history: HistoryEntry[];

  runsLoading: boolean;
  runsError: ProblemView | null;
  runs: ApplyRun[];

  storage: StorageReport | null;
  storageError: ProblemView | null;
  blocks: BlockRecord[];

  removal: {
    pending: PendingRemoval;
    title: string;
    body: string;
    busy: boolean;
  };
  cleanup: {
    open: boolean;
    from: string;
    to: string;
    busy: boolean;
    problem: ProblemView | null;
  };

  readonly visibleStates: SeriesRecord[];
  readonly dayGroups: (DayGroup<AiringRow> & { heading: string })[];
  readonly visibleRuns: ApplyRun[];
  readonly asRunCaption: string;
  readonly storageSpan: string;
  readonly cleanupWindowValid: boolean;
  readonly cleanupHint: string;

  init(): void;
  select(pane: Pane): void;
  onBandKey(event: KeyboardEvent): void;
  onWindowChange(): void;
  clearFilters(): void;

  // Each returns whether the read landed, so an event-driven refresh can
  // say what actually happened instead of announcing a success it never
  // waited for. `background` is contract note 6 above.
  loadStates(background?: boolean): Promise<boolean>;
  loadHistory(background?: boolean): Promise<boolean>;
  loadRuns(background?: boolean): Promise<boolean>;
  loadPlateChannels(): void;
  loadStorage(): Promise<void>;
  loadBlocks(): Promise<void>;

  blockingBlocks(showTitle: string): string[];
  requestRemoval(showTitle: string): Promise<void>;
  requestRangeCleanup(): Promise<void>;
  confirmRemoval(): Promise<void>;
  cancelRemoval(): void;

  cursorLabel(season: number, episode: number): string;
  runsLabel(n: number | undefined): string;
  formatLocal(iso: string | null | undefined): string;
  durationLabel(minutes: number): string;
  channelLabel(c: Channel): string;
  plate(id: string | undefined): PlateParts;
  runSummary(run: ApplyRun): string;
  outcomeLabel(run: ApplyRun): string;
  warningsOf(run: ApplyRun): ApplyRunWarning[];
  warningsLabel(run: ApplyRun): string;

  rowDirty(state: SeriesRecord): boolean;
  anyRowEditing(): boolean;
  saveCursor(state: SeriesRecord): Promise<void>;
  toggleCompleted(state: SeriesRecord): Promise<void>;
  toggleDisabled(state: SeriesRecord): Promise<void>;
  applyPatch(state: SeriesRecord, patch: SeriesPatch): Promise<void>;
}

document.addEventListener("alpine:init", () => {
  Alpine.data(
    "history",
    (): HistoryPageState & ThisType<HistoryPageState & WithMagics> => ({
      view: paneFromSearch(window.location.search),
      days: 7,
      channelId: "",
      blockQuery: "",
      search: "",
      source: "",

      channels: [],

      statesLoading: true,
      statesError: null,
      states: [],
      drafts: {},
      pending: {},
      rowErrors: {},
      rowNotFound: {},
      fieldErrors: {},

      historyLoading: true,
      historyError: null,
      history: [],

      runsLoading: true,
      runsError: null,
      runs: [],

      storage: null,
      storageError: null,
      blocks: [],

      removal: { pending: { kind: "none" }, title: "", body: "", busy: false },
      cleanup: { open: false, from: "", to: "", busy: false, problem: null },

      // Alpine calls this once per component instance and the page has
      // exactly one x-data="history", so there is no module-level one-shot
      // guard: the subscriptions below are wired from init and live with
      // the component, and initShell() keeps its own idempotence guard so
      // no second stream can be opened either way.
      init() {
        void this.loadStates();
        void this.loadHistory();
        void this.loadRuns();
        void this.loadStorage();
        void this.loadBlocks();
        this.loadPlateChannels();
        // Arming a new token re-fires whichever loads failed.
        onReauth(() => {
          if (this.statesError) void this.loadStates();
          if (this.historyError) void this.loadHistory();
          if (this.runsError) void this.loadRuns();
          if (this.channels.length === 0) this.loadPlateChannels();
        });

        // The live link. A frame is a hint -- "go look" -- never data to
        // render (runtime/bus.ts), so each one refetches the feed it
        // names and trusts the server's own ordering: /applies comes
        // back newest-first, so an arriving run lands at the top of RUNS
        // without this page having to splice a card together out of a
        // payload that carries no status, no start time and no window.
        //
        // None of the registrations below is ever cancelled: an Alpine
        // page component lives exactly as long as the document does.
        subscribe("apply.completed", () => {
          // The line is printed after both reads settle, and says which
          // way they went: printing "refreshed" up front would assert a
          // refresh that had not happened yet and might not.
          void Promise.all([this.loadRuns(true), this.loadHistory(true)]).then(([runsOk, historyOk]) => {
            // The tape's 150ms print draw-in (runtime/tape.ts) is this
            // page's arrival motion. The panes themselves come back
            // through a plain refetch, keyed per row, so rows that did
            // not change do not flash.
            printTape(applyRefreshLine(runsOk, historyOk));
          });
        });
        subscribe("series.changed", () => {
          if (this.anyRowEditing()) return;
          void this.loadStates(true);
        });

        // A hidden tab streams nothing, so it has missed every frame
        // since it was hidden and the resume replay only reaches back as
        // far as the hub's ring. This is the page's catch-up, and the
        // one an operator gets without knowing the live link exists --
        // all three panes, since all three were loaded at init and any
        // of them can be the one on screen. TRACKED keeps its guard: a
        // cursor half-typed before the tab was hidden is still typed.
        onResume(() => {
          if (!this.anyRowEditing()) void this.loadStates(true);
          void this.loadHistory(true);
          void this.loadRuns(true);
        });
      },

      // Pane switching rewrites ?view= without a navigation, so the
      // address bar always names what is on screen and the link is
      // shareable. replaceState rather than pushState: three panes of one
      // record are not three history entries to back through.
      select(pane) {
        this.view = pane;
        const url = new URL(window.location.href);
        url.searchParams.set("view", pane);
        window.history.replaceState({}, "", url);
        document.getElementById(`pane-${pane}`)?.focus();
      },

      // WAI-ARIA tabs keyboard pattern: arrows move between bands, Home
      // and End jump to the ends. Roving tabindex is bound in the markup.
      onBandKey(event) {
        const index = PANES.indexOf(this.view);
        let next = index;
        switch (event.key) {
          case "ArrowRight":
            next = (index + 1) % PANES.length;
            break;
          case "ArrowLeft":
            next = (index - 1 + PANES.length) % PANES.length;
            break;
          case "Home":
            next = 0;
            break;
          case "End":
            next = PANES.length - 1;
            break;
          default:
            return;
        }
        event.preventDefault();
        const pane = PANES[next];
        if (pane === undefined) return;
        this.view = pane;
        const url = new URL(window.location.href);
        url.searchParams.set("view", pane);
        window.history.replaceState({}, "", url);
        document.getElementById(`band-${pane}`)?.focus();
      },

      // The window is a server-side query on both feeds, so changing it
      // refetches rather than filtering what is already in hand.
      onWindowChange() {
        void this.loadHistory();
        void this.loadRuns();
      },

      clearFilters() {
        this.channelId = "";
        this.blockQuery = "";
        this.search = "";
      },

      get visibleStates() {
        return filterTracked(this.states, this.search);
      },

      get dayGroups() {
        const block = this.blockQuery.trim().toLowerCase();
        const title = this.search.trim().toLowerCase();
        const rows: AiringRow[] = this.history
          .filter((e) => (this.channelId === "" ? true : e.channel_id === this.channelId))
          .filter((e) => (block === "" ? true : (e.block_name ?? "").toLowerCase().includes(block)))
          .filter((e) => (title === "" ? true : (e.title ?? "").toLowerCase().includes(title)))
          .map((e, i) => ({
            ...e,
            // program_id repeats across occurrences, so the key pairs it
            // with the air time and the index -- a stable identity Alpine
            // can track without re-rendering the whole day on a refetch.
            key: `${e.program_id ?? "?"}|${e.scheduled_at ?? ""}|${String(i)}`,
            clock: e.scheduled_at === undefined ? "—" : localClock(e.scheduled_at),
            label: airingLabel(e),
            // durationLabel, not formatClock: duration_ms is a LENGTH,
            // and formatClock reads its argument as an epoch timestamp
            // (a 44-minute programme rendered as "01:44" that way).
            duration: e.duration_ms === undefined ? "" : durationLabel(e.duration_ms / 60_000),
          }));

        return groupByDay(rows).map((g) => ({ ...g, heading: dayHeading(g.day) }));
      },

      get visibleRuns() {
        if (this.source === "") return this.runs;
        return this.runs.filter((r) => r.source === this.source);
      },

      // Says plainly that the window is bounded by retention, so an
      // empty 90-day view reads as policy rather than data loss.
      get asRunCaption() {
        return `What actually aired in the last ${plural(this.days, "day")}, bounded by history retention.`;
      },

      async loadStates(background = false) {
        if (!background) {
          this.statesLoading = true;
          this.statesError = null;
        }
        try {
          const list = await apiGet<SeriesRecord[]>(apiPath("/state/series"));
          this.states = list;
          // Rebuilt wholesale on every load: a full reload is a
          // deliberate resync point, so any row's in-flight edit is
          // superseded by whatever the server holds now.
          const drafts: Record<string, RowDraft> = {};
          const pending: Record<string, boolean> = {};
          const rowErrors: Record<string, string | null> = {};
          const rowNotFound: Record<string, boolean> = {};
          const fieldErrors: Record<string, string | null> = {};
          for (const state of list) {
            drafts[state.show_title] = draftFromState(state);
            pending[state.show_title] = false;
            rowErrors[state.show_title] = null;
            rowNotFound[state.show_title] = false;
            fieldErrors[state.show_title] = null;
          }
          this.drafts = drafts;
          this.pending = pending;
          this.rowErrors = rowErrors;
          this.rowNotFound = rowNotFound;
          this.fieldErrors = fieldErrors;
          this.statesError = null;
          return true;
        } catch (err) {
          // Contract note 6: a background read that fails leaves the rows
          // that are already on screen alone. Raising statesError would
          // hide them (list.html gates every row on !statesError), which
          // is a worse reading than a few-seconds-old one.
          if (!background) {
            this.statesError = toProblemView(err);
            this.states = [];
          }
          return false;
        } finally {
          this.statesLoading = false;
        }
      },

      async loadHistory(background = false) {
        if (!background) {
          this.historyLoading = true;
          this.historyError = null;
        }
        try {
          this.history = await apiGet<HistoryEntry[]>(apiPath("/history", undefined, { days: this.days }));
          this.historyError = null;
          return true;
        } catch (err) {
          // Contract note 6 -- see loadStates.
          if (!background) {
            this.historyError = toProblemView(err);
            this.history = [];
          }
          return false;
        } finally {
          this.historyLoading = false;
        }
      },

      async loadRuns(background = false) {
        if (!background) {
          this.runsLoading = true;
          this.runsError = null;
        }
        try {
          this.runs = await apiGet<ApplyRun[]>(apiPath("/applies", undefined, { days: this.days }));
          this.runsError = null;
          return true;
        } catch (err) {
          // Contract note 6 -- see loadStates.
          if (!background) {
            this.runsError = toProblemView(err);
            this.runs = [];
          }
          return false;
        } finally {
          this.runsLoading = false;
        }
      },

      // Best-effort side fetch: a failed channel load leaves plates on
      // their shortened-id fallback rather than failing a whole pane.
      loadPlateChannels() {
        void loadChannels().then(
          (channels) => {
            this.channels = channels;
          },
          () => undefined,
        );
      },

      // The strip reports rows, so a failed read degrades to a one-line
      // notice rather than blanking the page: the panes below it are
      // independent and still work.
      async loadStorage() {
        this.storageError = null;
        try {
          this.storage = await apiGet<StorageReport>(apiPath("/storage"));
          if (this.cleanup.from === "" && this.cleanup.to === "") {
            // Prefilled from what is actually stored, so the form opens on
            // a pickable window instead of two empty fields.
            this.cleanup.from = localInputFromInstant(this.storage.oldest_airing ?? undefined);
            this.cleanup.to = localInputFromInstant(this.storage.newest_airing ?? undefined);
          }
        } catch (err) {
          this.storageError = toProblemView(err);
          this.storage = null;
        }
      },

      // Best-effort, like the channel plates: a failed read leaves
      // blockingBlocks empty, so the desk offers the removal and the
      // server's own refusal becomes the guard. Never the reverse --
      // hiding the action on a failed read would strand the operator.
      async loadBlocks() {
        try {
          this.blocks = await apiGet<BlockRecord[]>(apiPath("/blocks"));
        } catch {
          this.blocks = [];
        }
      },

      blockingBlocks(showTitle) {
        return blocksListingShow(this.blocks, showTitle);
      },

      get storageSpan() {
        const oldest = this.storage?.oldest_airing;
        const newest = this.storage?.newest_airing;
        if (!oldest || !newest) return "Nothing stored yet";
        return `${formatLocal(oldest)} — ${formatLocal(newest)}`;
      },

      get cleanupWindowValid() {
        const from = instantFromLocalInput(this.cleanup.from);
        const to = instantFromLocalInput(this.cleanup.to);
        if (from === undefined && to === undefined) return false;
        if (from !== undefined && to !== undefined) return from <= to;
        return true;
      },

      get cleanupHint() {
        const from = instantFromLocalInput(this.cleanup.from);
        const to = instantFromLocalInput(this.cleanup.to);
        if (from === undefined && to === undefined) return "Set at least one end of the window.";
        if (from !== undefined && to !== undefined && from > to) return "From must not be after To.";

        const scope: string[] = [];
        if (this.channelId !== "") scope.push("this channel");
        if (this.blockQuery.trim() !== "") scope.push(`blocks matching "${this.blockQuery.trim()}"`);
        const narrowed = scope.length > 0 ? ` Narrowed to ${scope.join(" and ")}.` : "";

        if (from === undefined) return `Everything before To.${narrowed}`;
        if (to === undefined) return `Everything from From onward.${narrowed}`;
        return `Airings inside the window.${narrowed}`;
      },

      // Both flows run their dry run FIRST and arm the confirm with its
      // real counts. A dialog that guessed would be a dialog the operator
      // confirms against a number that was never true.
      async requestRemoval(showTitle) {
        if (this.removal.busy) return;
        this.removal.busy = true;
        try {
          const report = await apiSend<RemovalReport>(
            "DELETE",
            apiPath("/state/series/{show_title}", { show_title: showTitle }, { dry_run: "true" }),
          );
          this.removal.pending = { kind: "show", showTitle };
          this.removal.title = `Remove ${showTitle}?`;
          this.removal.body = removalConfirmBody(report, showTitle);
          this.$refs.confirmDialog.showModal();
        } catch (err) {
          this.rowErrors[showTitle] = describeError(err);
        } finally {
          this.removal.busy = false;
        }
      },

      async requestRangeCleanup() {
        if (this.cleanup.busy) return;
        const from = instantFromLocalInput(this.cleanup.from);
        const to = instantFromLocalInput(this.cleanup.to);
        if (from === undefined && to === undefined) return;

        this.cleanup.busy = true;
        this.cleanup.problem = null;
        try {
          const report = await apiSend<RemovalReport>(
            "DELETE",
            apiPath("/history", undefined, {
              from,
              to,
              channel_id: this.channelId === "" ? undefined : this.channelId,
              block_name: this.blockQuery.trim() === "" ? undefined : this.blockQuery.trim(),
              dry_run: "true",
            }),
          );
          if (report.airings === 0) {
            this.cleanup.problem = {
              title: "Nothing to delete",
              detail: "No airings fall inside that window.",
            } as ProblemView;
            return;
          }
          this.removal.pending = { kind: "range", from: this.cleanup.from, to: this.cleanup.to };
          this.removal.title = "Delete this range?";
          this.removal.body = removalConfirmBody(report, "this range");
          this.$refs.confirmDialog.showModal();
        } catch (err) {
          this.cleanup.problem = toProblemView(err);
        } finally {
          this.cleanup.busy = false;
        }
      },

      async confirmRemoval() {
        const pending = this.removal.pending;
        if (pending.kind === "none" || this.removal.busy) return;

        this.removal.busy = true;
        try {
          let report: RemovalReport;
          let line: string;

          if (pending.kind === "show") {
            report = await apiSend<RemovalReport>(
              "DELETE",
              apiPath("/state/series/{show_title}", { show_title: pending.showTitle }),
            );
            line = `${pending.showTitle} removed — ${report.airings} ${report.airings === 1 ? "airing" : "airings"}`;
          } else {
            report = await apiSend<RemovalReport>(
              "DELETE",
              apiPath("/history", undefined, {
                from: instantFromLocalInput(pending.from),
                to: instantFromLocalInput(pending.to),
                channel_id: this.channelId === "" ? undefined : this.channelId,
                block_name: this.blockQuery.trim() === "" ? undefined : this.blockQuery.trim(),
              }),
            );
            line = `Range deleted — ${report.airings} ${report.airings === 1 ? "airing" : "airings"}`;
          }

          this.$refs.confirmDialog.close();
          this.removal.pending = { kind: "none" };
          printTape(line);

          // Every surface the deletion touched is now stale.
          await Promise.all([this.loadStates(true), this.loadHistory(true), this.loadStorage()]);
        } catch (err) {
          const problem = toProblemView(err);
          if (pending.kind === "show") {
            this.rowErrors[pending.showTitle] = describeError(err);
          } else {
            this.cleanup.problem = problem;
          }
          this.$refs.confirmDialog.close();
          this.removal.pending = { kind: "none" };
        } finally {
          this.removal.busy = false;
        }
      },

      cancelRemoval() {
        if (this.removal.busy) return;
        this.$refs.confirmDialog.close();
        this.removal.pending = { kind: "none" };
      },

      cursorLabel,
      runsLabel,
      formatLocal,
      durationLabel,
      channelLabel,
      runSummary,

      plate(id) {
        return channelPlate(id, this.channels);
      },

      outcomeLabel(run) {
        switch (run.status) {
          case "ok":
            return "Applied";
          case "error":
            return "Failed";
          case "running":
            return "In flight";
          default:
            return run.status;
        }
      },

      // The wire always sends an array (internal/api/applies.go), but the
      // generated type marks it optional -- coalesce rather than branch
      // on null at every call site.
      warningsOf(run) {
        return run.warnings ?? [];
      },

      warningsLabel(run) {
        return `${plural(this.warningsOf(run).length, "occurrence")} dropped`;
      },

      rowDirty(state) {
        return cursorDirty(state, this.drafts[state.show_title]);
      },

      // The live link's guard for TRACKED: true while any row holds a
      // typed-but-unsaved cursor, or a save still in flight. pending
      // counts because applyPatch rewrites that row's draft when it
      // lands -- a refetch racing it would resolve to whichever finished
      // last, which is exactly the kind of outcome an operator cannot
      // reason about.
      anyRowEditing() {
        return this.states.some(
          (s) => this.pending[s.show_title] === true || cursorDirty(s, this.drafts[s.show_title]),
        );
      },

      async saveCursor(state) {
        const title = state.show_title;
        if (this.pending[title] || this.rowNotFound[title]) return;
        const draft = this.drafts[title];
        if (!draft) return;

        const season = parsePositiveInt(draft.season);
        const episode = parsePositiveInt(draft.episode);
        if (season === null || episode === null) {
          this.fieldErrors[title] = "Season and episode must be whole numbers, 1 or greater.";
          return;
        }
        this.fieldErrors[title] = null;

        const patch = buildCursorPatch(state, season, episode);
        // Nothing actually changed (e.g. "01" edited back to "1") -- no
        // request. The Save button's own :disabled covers the common
        // case; this guards a direct call.
        if (!patch) return;

        await this.applyPatch(state, patch);
      },

      async toggleCompleted(state) {
        const next = !(state.completed ?? false);
        await this.applyPatch(state, { completed: next });
        if (!this.rowErrors[state.show_title]) {
          printTape(`${state.show_title} — ${next ? "marked completed" : "back in progress"}`);
        }
      },

      async toggleDisabled(state) {
        const next = !(state.disabled ?? false);
        await this.applyPatch(state, { disabled: next });
        if (!this.rowErrors[state.show_title]) {
          printTape(`${state.show_title} — ${next ? "disabled" : "active"}`);
        }
      },

      async applyPatch(state, patch) {
        const title = state.show_title;
        if (this.pending[title]) return;
        this.pending[title] = true;
        this.rowErrors[title] = null;
        this.rowNotFound[title] = false;
        try {
          const updated = await apiSend<SeriesRecord>(
            "PATCH",
            apiPath("/state/series/{show_title}", { show_title: title }),
            patch,
          );
          this.states = this.states.map((s) => (s.show_title === title ? updated : s));
          this.drafts[title] = draftFromState(updated);
        } catch (err) {
          if (err instanceof ApiError && err.status === 404) {
            // The row vanished server-side between load and save. Left on
            // screen with an inline error and a refresh action rather
            // than silently dropped -- another save would only 404 again.
            this.rowNotFound[title] = true;
          }
          this.rowErrors[title] = describeError(err);
        } finally {
          this.pending[title] = false;
        }
      },
    }),
  );
});

// ---- local-time helpers ----------------------------------------------------

/** HH:MM in the reader's own zone -- the station clock an as-run row is
 * read against. */
function localClock(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "—";
  return `${pad2(d.getHours())}:${pad2(d.getMinutes())}`;
}

/** A day group's heading: the local date spelled out, so two adjacent
 * groups can't be confused by their numerals alone. */
function dayHeading(day: string): string {
  const d = new Date(`${day}T00:00:00`);
  if (Number.isNaN(d.getTime())) return day;
  return d.toLocaleDateString(undefined, { weekday: "long", year: "numeric", month: "long", day: "numeric" });
}
