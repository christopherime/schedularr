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
import { ApiError, apiGet, apiPath, apiSend, onReauth } from "../runtime/api.ts";
import type { ApiRequestJSON, ApiResponse } from "../runtime/api.ts";
import { channelLabel, channelPlate, loadChannels } from "../runtime/channels.ts";
import type { Channel, PlateParts } from "../runtime/channels.ts";
import { describeError, toProblemView } from "../runtime/errors.ts";
import type { ProblemView } from "../runtime/errors.ts";
import { durationLabel, formatLocal, pad2, plural } from "../runtime/format.ts";
import { initShell } from "../runtime/shell.ts";
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

// Same double-init defense as every other page bundle: Alpine.data()'s
// init() is auto-invoked, so nothing here also wires x-init="init()".
let started = false;

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

interface RowDraft {
  season: string;
  episode: string;
}

function draftFromState(state: SeriesRecord): RowDraft {
  return { season: String(state.current_season), episode: String(state.current_episode) };
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

// ---- component -------------------------------------------------------------

interface AiringRow extends HistoryEntry {
  key: string;
  clock: string;
  label: string;
  duration: string;
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

  readonly visibleStates: SeriesRecord[];
  readonly dayGroups: (DayGroup<AiringRow> & { heading: string })[];
  readonly visibleRuns: ApplyRun[];
  readonly asRunCaption: string;

  init(): void;
  select(pane: Pane): void;
  onBandKey(event: KeyboardEvent): void;
  onWindowChange(): void;
  clearFilters(): void;

  loadStates(): Promise<void>;
  loadHistory(): Promise<void>;
  loadRuns(): Promise<void>;
  loadPlateChannels(): void;

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
  saveCursor(state: SeriesRecord): Promise<void>;
  toggleCompleted(state: SeriesRecord): Promise<void>;
  toggleDisabled(state: SeriesRecord): Promise<void>;
  applyPatch(state: SeriesRecord, patch: SeriesPatch): Promise<void>;
}

document.addEventListener("alpine:init", () => {
  Alpine.data(
    "history",
    (): HistoryPageState => ({
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

      init() {
        if (started) return;
        started = true;
        void this.loadStates();
        void this.loadHistory();
        void this.loadRuns();
        this.loadPlateChannels();
        // Arming a new token re-fires whichever loads failed.
        onReauth(() => {
          if (this.statesError) void this.loadStates();
          if (this.historyError) void this.loadHistory();
          if (this.runsError) void this.loadRuns();
          if (this.channels.length === 0) this.loadPlateChannels();
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

      async loadStates() {
        this.statesLoading = true;
        this.statesError = null;
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
        } catch (err) {
          this.statesError = toProblemView(err);
          this.states = [];
        } finally {
          this.statesLoading = false;
        }
      },

      async loadHistory() {
        this.historyLoading = true;
        this.historyError = null;
        try {
          this.history = await apiGet<HistoryEntry[]>(apiPath("/history", undefined, { days: this.days }));
        } catch (err) {
          this.historyError = toProblemView(err);
          this.history = [];
        } finally {
          this.historyLoading = false;
        }
      },

      async loadRuns() {
        this.runsLoading = true;
        this.runsError = null;
        try {
          this.runs = await apiGet<ApplyRun[]>(apiPath("/applies", undefined, { days: this.days }));
        } catch (err) {
          this.runsError = toProblemView(err);
          this.runs = [];
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
        const draft = this.drafts[state.show_title];
        if (!draft) return false;
        return (
          draft.season.trim() !== String(state.current_season) ||
          draft.episode.trim() !== String(state.current_episode)
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
