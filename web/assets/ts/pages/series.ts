// Series page ("/series/"): every persisted series-state row (per-show
// season/episode cursor, run count, last-aired timestamp, completed/
// disabled flags), backed by GET/PATCH /api/v1/state/series[/{show_title}].
// Bundled separately via "page_js", same pattern as dashboard.ts/blocks.ts/
// schedule.ts.
//
// Contract notes that shape the code below (api/openapi.yaml, SeriesState/
// SeriesStatePatch; internal/api/state.go):
//
//   1. PATCH is a TRUE partial patch, not blocks.ts's full-replace PUT.
//      SeriesStatePatch's fields are all optional -- only a field actually
//      present in the request body changes server-side (internal/api/
//      state.go's PatchSeriesState applies each non-nil pointer in turn).
//      Every write path here (cursor save, completed toggle, disabled
//      toggle) sends ONLY the field(s) that changed, never the row's full
//      current state.
//   2. An empty patch is a 400 (state.go: "at least one field ... must be
//      set"). saveCursor() never calls applyPatch() with a body that has
//      no keys -- see buildCursorPatch()'s doc comment.
//   3. There is no create endpoint. A series_state row exists only once a
//      series block airs it for the first time (state.go's own comment:
//      "the store fabricates nothing for the API") -- an unknown show_title
//      404s. This page never offers to add a row; the empty state explains
//      why the table can be empty.
//   4. show_title is the path parameter, and titles routinely contain
//      spaces/punctuation -- every PATCH URL runs it through
//      encodeURIComponent (mirrors the server test suite's own
//      url.PathEscape, internal/api/state_test.go's patchPath helper).
//   5. A show_title vanishing between page-load and save (the row's last
//      persisted state got deleted/reset out from under the operator) is a
//      404 on PATCH -- rendered as an inline row error plus a "Refresh
//      list" action (re-running GET, which naturally drops the now-gone
//      row) rather than leaving a row on screen that can never save again.
import { ApiError, apiGet, apiPath, apiSend, onReauth } from "../runtime/api.ts";
import type { ApiRequestJSON, ApiResponse } from "../runtime/api.ts";
import { describeError, toProblemView } from "../runtime/errors.ts";
import type { ProblemView } from "../runtime/errors.ts";
import { formatLocal, pad2, plural } from "../runtime/format.ts";
import { initShell } from "../runtime/shell.ts";
import { printTape } from "../runtime/tape.ts";

initShell();

type SeriesRecord = ApiResponse<"listSeriesState", 200>[number];
type SeriesPatch = ApiRequestJSON<"patchSeriesState">;

declare const Alpine: {
  data<T extends object>(name: string, factory: () => T): void;
};

// Same double-init defense as dashboard.ts/blocks.ts/schedule.ts: Alpine.
// data()'s init() is auto-invoked, so nothing on this page also wires
// x-init="init()" to it.
let started = false;

// ---- cursor editing --------------------------------------------------------
//
// Season/episode are held as plain text per row (same convention as
// blocks.ts's numeric fields -- never coerced by Alpine's x-model.number
// modifier), keyed by show_title rather than array index so a row's draft
// survives the list re-rendering around it.

interface RowDraft {
  season: string;
  episode: string;
}

function draftFromState(state: SeriesRecord): RowDraft {
  return { season: String(state.current_season), episode: String(state.current_episode) };
}

/** SxxEyy, for the visually-hidden label read alongside the two separate
 * season/episode inputs -- the binding contract's "S/E cursor as SxxEyy"
 * display, kept distinct from the raw (unpadded) values the inputs edit. */
function cursorLabel(season: number, episode: number): string {
  return `S${pad2(season)}E${pad2(episode)}`;
}

function runsLabel(n: number | undefined): string {
  return plural(n ?? 0, "run");
}

/** Parses a whole number >= 1, or null when the input isn't one -- the
 * "number inputs enforce >=1 client-side" half of the contract. Anything
 * this doesn't catch (e.g. a season/episode that simply doesn't exist for
 * the show) is left to the API's own 400. */
function parsePositiveInt(raw: string): number | null {
  const trimmed = raw.trim();
  if (!/^\d+$/.test(trimmed)) return null;
  const n = Number(trimmed);
  return Number.isFinite(n) && n >= 1 ? n : null;
}

/** True partial patch: only current_season/current_episode entries whose
 * parsed value actually differs from the loaded row are included. Returns
 * null when there is nothing to send -- callers (saveCursor) must treat
 * that as a no-op and never call the API with an empty body (state.go
 * 400s an empty patch, and the binding contract calls for zero requests
 * on an unchanged save regardless). */
function buildCursorPatch(state: SeriesRecord, season: number, episode: number): SeriesPatch | null {
  const patch: SeriesPatch = {};
  if (season !== state.current_season) patch.current_season = season;
  if (episode !== state.current_episode) patch.current_episode = episode;
  return Object.keys(patch).length > 0 ? patch : null;
}

interface SeriesPageState {
  statesLoading: boolean;
  statesError: ProblemView | null;
  states: SeriesRecord[];

  // Per-row UI state, keyed by show_title (not array index -- a full list
  // reload re-sorts/re-fetches by show_title anyway, and a single row's
  // own PATCH success only ever touches its own key).
  drafts: Record<string, RowDraft>;
  pending: Record<string, boolean>;
  rowErrors: Record<string, string | null>;
  rowNotFound: Record<string, boolean>;
  fieldErrors: Record<string, string | null>;

  init(): void;
  loadStates(): Promise<void>;

  cursorLabel(season: number, episode: number): string;
  runsLabel(n: number | undefined): string;
  formatLocal(iso: string | null | undefined): string;

  rowDirty(state: SeriesRecord): boolean;
  saveCursor(state: SeriesRecord): Promise<void>;
  toggleCompleted(state: SeriesRecord): Promise<void>;
  toggleDisabled(state: SeriesRecord): Promise<void>;
  applyPatch(state: SeriesRecord, patch: SeriesPatch): Promise<void>;
}

document.addEventListener("alpine:init", () => {
  Alpine.data(
    "series",
    (): SeriesPageState => ({
      statesLoading: true,
      statesError: null,
      states: [],

      drafts: {},
      pending: {},
      rowErrors: {},
      rowNotFound: {},
      fieldErrors: {},

      init() {
        if (started) return;
        started = true;
        void this.loadStates();
        // Arming a new token re-fires a failed list load (the token
        // panel's probe broadcast, runtime/api.ts's onReauth).
        onReauth(() => {
          if (this.statesError) void this.loadStates();
        });
      },

      async loadStates() {
        this.statesLoading = true;
        this.statesError = null;
        try {
          const list = await apiGet<SeriesRecord[]>(apiPath("/state/series"));
          this.states = list;
          // Rebuilt wholesale on every load -- a full list reload
          // (including the "Refresh list" 404 recovery action) is a
          // deliberate resync point; any row's in-flight edit is
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

      cursorLabel,
      runsLabel,
      formatLocal,

      // Text-level diff against the loaded row, not a numeric one: "01" vs
      // "1" still counts as dirty (arms the Save button) even though they'd
      // parse to the same value, since the operator visibly changed the
      // field. saveCursor()'s own numeric diff (buildCursorPatch) is what
      // actually decides whether a request goes out.
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
        // Nothing actually changed (e.g. "01" edited back to "1", or Save
        // reached with no real edit) -- no request, per the binding
        // contract. The Save button's own :disabled="!rowDirty(state)"
        // covers the common case; this is the belt-and-suspenders guard
        // for anyone/anything invoking saveCursor() directly.
        if (!patch) return;

        await this.applyPatch(state, patch);
      },

      // Single-field PATCH -- state.completed defaults to false when the
      // wire omits it (only possible for a row this page never actually
      // renders, since seriesStateToGen always populates it for a
      // persisted row, but the OpenAPI type is optional so this still
      // guards defensively, same as blocks.ts's fillerToForm). Success
      // prints a tape line -- the settings-snap idiom; UNDO on that line
      // arrives with the series-desk slice.
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

      // Shared PATCH path for the cursor save and both toggles. Always
      // sends exactly the patch object it's given -- callers are
      // responsible for that object containing only the field(s) that
      // actually changed (buildCursorPatch for the cursor; a single-key
      // literal for each toggle).
      async applyPatch(state, patch) {
        const title = state.show_title;
        if (this.pending[title]) return;
        this.pending[title] = true;
        this.rowErrors[title] = null;
        this.rowNotFound[title] = false;
        try {
          // apiPath encodeURIComponent-encodes the path parameter --
          // show titles routinely contain spaces/punctuation (mirrors the
          // server test suite's own url.PathEscape).
          const updated = await apiSend<SeriesRecord>(
            "PATCH",
            apiPath("/state/series/{show_title}", { show_title: title }),
            patch,
          );
          this.states = this.states.map((s) => (s.show_title === title ? updated : s));
          // Resync the draft to the server's authoritative values -- a
          // toggle-only patch still refreshes season/episode display (a
          // no-op in practice, since they didn't change), and a cursor
          // save clears any stale text the operator had typed.
          this.drafts[title] = draftFromState(updated);
        } catch (err) {
          if (err instanceof ApiError && err.status === 404) {
            // The row vanished server-side between load and save (its
            // persisted series_state was deleted/reset). Left on screen
            // with an inline error and a "Refresh list" action rather than
            // silently dropped -- a further save attempt against the same
            // stale row would only 404 again.
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
