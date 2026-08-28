// Blocks page ("/blocks/"): list every stored block, toggle/edit/delete it,
// and create/edit blocks through an inline editor panel (not a modal --
// this form is long enough that interrupting the page for it isn't
// justified; see the "blocks: editor panel" comment in main.css). Bundled
// separately via the "page_js" block, same pattern as dashboard.ts.
//
// Two contract rules drive most of the awkward-looking code below, both
// from the API's PUT semantics (api/openapi.yaml, BlockWrite):
//
//   1. PUT is a full replace. Toggling "enabled" from the list still sends
//      the block's complete stored spec back unchanged -- there is no
//      partial-update endpoint. Never build a toggle-only body.
//   2. BlockWrite.enabled defaults to true when omitted. This UI never
//      relies on that default -- every write (create, edit, and the list
//      toggle) sends `enabled` explicitly, even when it happens to already
//      be what the default would have produced.
//
// A third, less obvious constraint: the server decodes both create and
// update bodies with encoding/json's DisallowUnknownFields (see
// internal/api/blocks.go), so a request body may only contain fields the
// OpenAPI schema actually defines -- no client-side convenience fields can
// ride along in the JSON we send. buildSpec()/buildFilter()/etc. below stay
// close to gen/types.d.ts's BlockSpec shape for exactly that reason.
import type { ApiRequestJSON, ApiResponse } from "../api";
import type { components } from "../gen/types";

type BlockRecord = ApiResponse<"listBlocks", 200>[number];
type Channel = ApiResponse<"listChannels", 200>[number];
type BlockSpec = components["schemas"]["BlockSpec"];
type Filter = components["schemas"]["Filter"];
type FillerConfig = components["schemas"]["FillerConfig"];
type SeriesConfig = components["schemas"]["SeriesConfig"];
type SeriesFallback = components["schemas"]["SeriesFallback"];
// createBlock and updateBlock both take BlockWrite -- either operation name
// resolves to the identical request type.
type BlockWrite = ApiRequestJSON<"createBlock">;
type OnComplete = NonNullable<SeriesConfig["on_complete"]>;

declare const Alpine: {
  data<T extends object>(name: string, factory: () => T): void;
};

// Same double-init defense as dashboard.ts: Alpine.data()'s init() is
// auto-invoked, so nothing on this page also wires x-init="init()" to it
// (see that file's comment for the full story of why this guard exists).
let started = false;

/** Renders an ApiError as its problem title (+ detail, when present); any
 * other thrown value falls back to its message. Always read via
 * window.schedularr.ApiError, not a locally imported class -- main.ts and
 * this file are separate esbuild bundles, so only the window-hung instance
 * is guaranteed to be the one apiGet/apiSend actually throw. */
function describeError(err: unknown): string {
  if (err instanceof window.schedularr.ApiError) {
    return err.detail ? `${err.title}: ${err.detail}` : err.title;
  }
  return err instanceof Error ? err.message : String(err);
}

// ---- cron plain-language hint --------------------------------------------
//
// A small hand-rolled parser for the handful of 5-field cron shapes a
// scheduling UI's cron field realistically sees (fixed time, optionally
// restricted to specific weekdays) -- not a cron-parsing dependency
// (PRODUCT.md: "Static-first"). Anything outside that shape (a day-of-month
// or month restriction, a list/range/step on minute or hour) returns null;
// callers show the raw cron with no extra hint rather than guessing wrong.

const DAY_NAMES = ["Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"];

function pad2(n: number): string {
  return String(n).padStart(2, "0");
}

export function cronHint(raw: string): string | null {
  const fields = raw.trim().split(/\s+/);
  if (fields.length !== 5) return null;
  const [min, hour, dom, mon, dow] = fields;
  if (dom !== "*" || mon !== "*") return null;

  if (min === "*" && hour === "*") {
    return dow === "*" ? "Every minute" : null;
  }
  if (!/^\d{1,2}$/.test(min) || !/^\d{1,2}$/.test(hour)) return null;
  const m = Number(min);
  const h = Number(hour);
  if (m > 59 || h > 23) return null;
  const time = `${pad2(h)}:${pad2(m)}`;

  if (dow === "*") return `Daily at ${time}`;

  const days = dow.split(",").map((d) => d.trim());
  if (!days.every((d) => /^[0-7]$/.test(d))) return null;
  const nums = days.map((d) => Number(d) % 7); // cron allows 7 as an alt-Sunday
  const uniq = Array.from(new Set(nums)).sort((a, b) => a - b);

  if (uniq.length === 5 && [1, 2, 3, 4, 5].every((d) => uniq.includes(d))) {
    return `Weekdays at ${time}`;
  }
  if (uniq.length === 2 && uniq.includes(0) && uniq.includes(6)) {
    return `Weekends at ${time}`;
  }
  const names = uniq.map((d) => `${DAY_NAMES[d]}s`).join(", ");
  return `${names} at ${time}`;
}

// ---- form-side shapes -----------------------------------------------------
//
// Every numeric/array field in the editor is held as plain text, never
// coerced by Alpine's x-model.number modifier (its empty-string/NaN
// behavior differs across Alpine builds and this project vendors one
// specific minified copy with no changelog trail). parseOptionalInt/
// parseRequiredInt/parseCommaList below do the coercion once, at submit
// time, uniformly.

interface FilterForm {
  title_pattern: string;
  genres: string;
  ratings: string;
  year_from: string;
  year_to: string;
  min_duration: string;
  max_duration: string;
  tags: string;
}

interface FillerForm {
  enabled: boolean;
  filler_list_id: string;
  max_filler_time: string;
  min_gap_time: string;
}

interface SeriesRowForm {
  show_title: string;
  episodes_per_block: string;
  start_season: string;
  start_episode: string;
  on_complete: OnComplete;
  skip_episodes: string;
  max_runs: string;
}

interface FallbackForm {
  mode: "" | NonNullable<SeriesFallback["mode"]>;
  fillerFilter: FilterForm;
}

interface EditorForm {
  type: "filter" | "series";
  name: string;
  cron: string;
  duration: string;
  channel_id: string;
  priority: string;
  enabled: boolean;
  max_duration_overflow_minutes: string;
  filter: FilterForm;
  filler: FillerForm;
  series: SeriesRowForm[];
  fallback: FallbackForm;
}

function emptyFilterForm(): FilterForm {
  return {
    title_pattern: "",
    genres: "",
    ratings: "",
    year_from: "",
    year_to: "",
    min_duration: "",
    max_duration: "",
    tags: "",
  };
}

function emptyFillerForm(): FillerForm {
  return { enabled: false, filler_list_id: "", max_filler_time: "", min_gap_time: "" };
}

function emptySeriesRow(): SeriesRowForm {
  return {
    show_title: "",
    episodes_per_block: "",
    start_season: "",
    start_episode: "",
    on_complete: "continue",
    skip_episodes: "",
    max_runs: "",
  };
}

function emptyFallbackForm(): FallbackForm {
  return { mode: "", fillerFilter: emptyFilterForm() };
}

function emptyEditorForm(): EditorForm {
  return {
    type: "filter",
    name: "",
    cron: "",
    duration: "",
    channel_id: "",
    priority: "",
    enabled: true,
    max_duration_overflow_minutes: "",
    filter: emptyFilterForm(),
    filler: emptyFillerForm(),
    series: [],
    fallback: emptyFallbackForm(),
  };
}

// ---- form -> wire (submit) --------------------------------------------

function parseOptionalInt(raw: string): number | undefined {
  const trimmed = raw.trim();
  if (trimmed === "") return undefined;
  const n = Number(trimmed);
  return Number.isFinite(n) ? n : undefined;
}

/** duration and episodes_per_block are required, non-nullable ints on the
 * wire (gen.BlockSpec.Duration, gen.SeriesConfig.EpisodesPerBlock are plain
 * Go ints, not pointers) -- JSON.stringify(NaN) serializes as `null`, which
 * would round-trip through Go's json.Unmarshal as a silent no-op (leaving
 * the zero value) rather than the 400 a blank/invalid required field should
 * produce. Coercing blank/invalid input to 0 here keeps the wire body
 * always a real number, so a blank duration reliably fails CUE's `int &
 * >0` check server-side (400) instead of an ambiguous null. */
function parseRequiredInt(raw: string): number {
  const trimmed = raw.trim();
  const n = Number(trimmed);
  return trimmed !== "" && Number.isFinite(n) ? n : 0;
}

function parseCommaList(raw: string): string[] | undefined {
  const items = raw
    .split(",")
    .map((s) => s.trim())
    .filter((s) => s.length > 0);
  return items.length > 0 ? items : undefined;
}

const SKIP_EPISODE_RE = /^S\d{2}E\d{2}$/i;

/** Validates the brief's required client-side check: skip_episodes entries
 * must look like "S01E05". Returns the parsed (uppercased) list, or an
 * error message naming exactly which entries didn't match -- never both. */
function parseSkipEpisodes(raw: string): { value?: string[]; error?: string } {
  const items = raw
    .split(",")
    .map((s) => s.trim())
    .filter((s) => s.length > 0);
  if (items.length === 0) return {};
  const bad = items.filter((s) => !SKIP_EPISODE_RE.test(s));
  if (bad.length > 0) {
    return { error: `Invalid episode code(s): ${bad.join(", ")} (expected SxxExx, e.g. S01E05)` };
  }
  return { value: items.map((s) => s.toUpperCase()) };
}

function buildFilter(f: FilterForm): Filter | undefined {
  const out: Filter = {};
  const titlePattern = f.title_pattern.trim();
  if (titlePattern !== "") out.title_pattern = titlePattern;
  const genres = parseCommaList(f.genres);
  if (genres) out.genres = genres;
  const ratings = parseCommaList(f.ratings);
  if (ratings) out.ratings = ratings;
  const yearFrom = parseOptionalInt(f.year_from);
  if (yearFrom !== undefined) out.year_from = yearFrom;
  const yearTo = parseOptionalInt(f.year_to);
  if (yearTo !== undefined) out.year_to = yearTo;
  const minDuration = parseOptionalInt(f.min_duration);
  if (minDuration !== undefined) out.min_duration = minDuration;
  const maxDuration = parseOptionalInt(f.max_duration);
  if (maxDuration !== undefined) out.max_duration = maxDuration;
  const tags = parseCommaList(f.tags);
  if (tags) out.tags = tags;
  return Object.keys(out).length > 0 ? out : undefined;
}

function buildFiller(f: FillerForm): FillerConfig | undefined {
  const out: FillerConfig = {};
  let touched = false;
  if (f.enabled) {
    out.enabled = true;
    touched = true;
  }
  const fillerListID = f.filler_list_id.trim();
  if (fillerListID !== "") {
    out.filler_list_id = fillerListID;
    touched = true;
  }
  const maxFillerTime = parseOptionalInt(f.max_filler_time);
  if (maxFillerTime !== undefined) {
    out.max_filler_time = maxFillerTime;
    touched = true;
  }
  const minGapTime = parseOptionalInt(f.min_gap_time);
  if (minGapTime !== undefined) {
    out.min_gap_time = minGapTime;
    touched = true;
  }
  return touched ? out : undefined;
}

function buildSeriesConfig(row: SeriesRowForm): SeriesConfig {
  const out: SeriesConfig = {
    show_title: row.show_title.trim(),
    episodes_per_block: parseRequiredInt(row.episodes_per_block),
    // on_complete is a <select>, always at a definite value -- unlike a
    // text input it can't be "left blank", so it's always sent rather than
    // conditionally omitted like the rest of this function's fields.
    on_complete: row.on_complete,
  };
  const startSeason = parseOptionalInt(row.start_season);
  if (startSeason !== undefined) out.start_season = startSeason;
  const startEpisode = parseOptionalInt(row.start_episode);
  if (startEpisode !== undefined) out.start_episode = startEpisode;
  const { value: skipEpisodes } = parseSkipEpisodes(row.skip_episodes);
  if (skipEpisodes) out.skip_episodes = skipEpisodes;
  const maxRuns = parseOptionalInt(row.max_runs);
  if (maxRuns !== undefined) out.max_runs = maxRuns;
  return out;
}

function buildFallback(f: FallbackForm): SeriesFallback | undefined {
  if (f.mode === "") return undefined;
  const out: SeriesFallback = { mode: f.mode };
  if (f.mode === "filler") {
    const fillerFilter = buildFilter(f.fillerFilter);
    if (fillerFilter) out.filler_filter = fillerFilter;
  }
  return out;
}

/** Builds the BlockSpec to send. `type` is always explicit (the server
 * normalizes an omitted type to "filter" anyway, but this UI never leans on
 * that -- see the binding contract). filter/series/fallback sections are
 * only populated for the type they belong to; filler is independent of
 * type in the schema (BlockSpec.filler isn't gated by BlockSpec.type) and
 * is always considered regardless of which type is selected. */
function buildSpec(form: EditorForm): BlockSpec {
  const spec: BlockSpec = {
    type: form.type,
    name: form.name.trim(),
    cron: form.cron.trim(),
    duration: parseRequiredInt(form.duration),
    channel_id: form.channel_id.trim(),
  };

  const priority = parseOptionalInt(form.priority);
  if (priority !== undefined) spec.priority = priority;
  const overflow = parseOptionalInt(form.max_duration_overflow_minutes);
  if (overflow !== undefined) spec.max_duration_overflow_minutes = overflow;

  if (form.type === "filter") {
    const filter = buildFilter(form.filter);
    if (filter) spec.filter = filter;
  }

  const filler = buildFiller(form.filler);
  if (filler) spec.filler = filler;

  if (form.type === "series") {
    if (form.series.length > 0) spec.series = form.series.map(buildSeriesConfig);
    const fallback = buildFallback(form.fallback);
    if (fallback) spec.fallback = fallback;
  }

  return spec;
}

// ---- wire -> form (edit prefill) -----------------------------------------

function numToStr(n: number | undefined): string {
  return n === undefined ? "" : String(n);
}

function joinList(arr: string[] | undefined): string {
  return arr && arr.length > 0 ? arr.join(", ") : "";
}

function filterToForm(f: Filter | undefined): FilterForm {
  if (!f) return emptyFilterForm();
  return {
    title_pattern: f.title_pattern ?? "",
    genres: joinList(f.genres),
    ratings: joinList(f.ratings),
    year_from: numToStr(f.year_from),
    year_to: numToStr(f.year_to),
    min_duration: numToStr(f.min_duration),
    max_duration: numToStr(f.max_duration),
    tags: joinList(f.tags),
  };
}

function fillerToForm(f: FillerConfig | undefined): FillerForm {
  if (!f) return emptyFillerForm();
  return {
    enabled: f.enabled ?? false,
    filler_list_id: f.filler_list_id ?? "",
    max_filler_time: numToStr(f.max_filler_time),
    min_gap_time: numToStr(f.min_gap_time),
  };
}

function seriesConfigToForm(sc: SeriesConfig): SeriesRowForm {
  return {
    show_title: sc.show_title,
    episodes_per_block: String(sc.episodes_per_block),
    start_season: numToStr(sc.start_season),
    start_episode: numToStr(sc.start_episode),
    on_complete: sc.on_complete ?? "continue",
    skip_episodes: joinList(sc.skip_episodes),
    max_runs: numToStr(sc.max_runs),
  };
}

function fallbackToForm(f: SeriesFallback | undefined): FallbackForm {
  if (!f) return emptyFallbackForm();
  return { mode: f.mode ?? "redistribute", fillerFilter: filterToForm(f.filler_filter) };
}

function formFromSpec(spec: BlockSpec, enabled: boolean): EditorForm {
  return {
    type: spec.type === "series" ? "series" : "filter",
    name: spec.name,
    cron: spec.cron,
    duration: String(spec.duration),
    channel_id: spec.channel_id,
    priority: numToStr(spec.priority),
    enabled,
    max_duration_overflow_minutes: numToStr(spec.max_duration_overflow_minutes),
    filter: filterToForm(spec.filter),
    filler: fillerToForm(spec.filler),
    series: (spec.series ?? []).map(seriesConfigToForm),
    fallback: fallbackToForm(spec.fallback),
  };
}

function channelLabel(c: Channel): string {
  const parts: string[] = [];
  if (c.number !== undefined) parts.push(String(c.number));
  parts.push(c.name ?? c.id ?? "?");
  return parts.join(" · ");
}

interface EditorState {
  open: boolean;
  mode: "create" | "edit";
  editingId: string | null;
  submitting: boolean;
  error: string | null;
  nameConflict: string | null;
  seriesRowErrors: (string | null)[];
  form: EditorForm;
}

interface BlocksState {
  blocksLoading: boolean;
  blocksError: string | null;
  blocks: BlockRecord[];
  listError: string | null;

  channelsLoading: boolean;
  channelsError: string | null;
  channels: Channel[];

  pendingId: string | null;
  confirmDeleteId: string | null;

  editor: EditorState;

  init(): void;
  loadBlocks(): Promise<void>;
  loadChannels(): Promise<void>;

  cronHint(raw: string): string | null;
  channelLabel(c: Channel): string;
  channelSelectOptions(): Channel[];
  channelHint(): string;

  openCreate(): void;
  openEdit(block: BlockRecord): void;
  closeEditor(): void;
  focusEditorSoon(): void;
  ensureSeriesRow(): void;
  addSeriesRow(): void;
  removeSeriesRow(index: number): void;
  validateSeriesRows(): boolean;
  submit(): Promise<void>;

  toggleEnabled(block: BlockRecord): Promise<void>;
  requestDelete(id: string): void;
  cancelDelete(): void;
  performDelete(id: string): Promise<void>;
}

// Alpine binds a handful of "magic" helpers (https://alpinejs.dev/magics)
// onto the component instance at runtime, on top of whatever Alpine.data()'s
// factory returns -- $nextTick (used by focusEditorSoon(), below) is the
// only one this page needs. ThisType<...> (erased at compile time) tells
// TypeScript to type `this` inside the object literal's methods as
// BlocksState-plus-$nextTick, without requiring the literal itself to
// supply $nextTick -- it doesn't exist until Alpine injects it.
interface WithNextTick {
  $nextTick(callback: () => void): void;
}

document.addEventListener("alpine:init", () => {
  Alpine.data(
    "blocks",
    (): BlocksState & ThisType<BlocksState & WithNextTick> => ({
      blocksLoading: true,
      blocksError: null,
      blocks: [],
      listError: null,

      channelsLoading: true,
      channelsError: null,
      channels: [],

      pendingId: null,
      confirmDeleteId: null,

      editor: {
        open: false,
        mode: "create",
        editingId: null,
        submitting: false,
        error: null,
        nameConflict: null,
        seriesRowErrors: [],
        form: emptyEditorForm(),
      },

      init() {
        if (started) return;
        started = true;
        void this.loadBlocks();
        void this.loadChannels();
      },

      async loadBlocks() {
        this.blocksLoading = true;
        this.blocksError = null;
        try {
          this.blocks = await window.schedularr.apiGet<BlockRecord[]>("/api/v1/blocks");
        } catch (err) {
          this.blocksError = describeError(err);
        } finally {
          this.blocksLoading = false;
        }
      },

      async loadChannels() {
        this.channelsLoading = true;
        this.channelsError = null;
        try {
          this.channels = await window.schedularr.apiGet<Channel[]>("/api/v1/channels");
        } catch (err) {
          this.channelsError = describeError(err);
          this.channels = [];
        } finally {
          this.channelsLoading = false;
        }
      },

      cronHint,
      channelLabel,

      // Guards against silently reassigning a block's channel: if the
      // block being edited points at a channel_id the live Tunarr channel
      // list doesn't contain (deleted channel, stale data, Tunarr
      // reconfigured), the <select> still needs an option for it -- an
      // ordinary <select> with no matching <option> silently falls back to
      // whichever option happens to be first, which would rewrite the
      // block's channel on save without the operator ever choosing that.
      channelSelectOptions() {
        const current = this.editor.form.channel_id.trim();
        if (current === "" || this.channels.some((c) => c.id === current)) {
          return this.channels;
        }
        return [...this.channels, { id: current, name: `${current} (not in Tunarr's channel list)` }];
      },

      // Select-vs-free-text is gated on "usable options exist", not just
      // "the call didn't error": a reachable Tunarr with zero channels
      // configured would otherwise render an unusable empty <select>. Both
      // cases fall back to the same free-text input.
      channelHint() {
        if (this.channelsLoading) return "Loading channels from Tunarr…";
        if (this.channelsError) {
          return `Tunarr channel list unavailable (${this.channelsError}) -- enter the channel ID manually.`;
        }
        if (this.channels.length === 0) {
          return "Tunarr returned no channels -- enter the channel ID manually.";
        }
        return "";
      },

      openCreate() {
        this.editor.mode = "create";
        this.editor.editingId = null;
        this.editor.error = null;
        this.editor.nameConflict = null;
        this.editor.form = emptyEditorForm();
        this.editor.seriesRowErrors = [];
        this.editor.open = true;
        this.focusEditorSoon();
      },

      openEdit(block) {
        this.editor.mode = "edit";
        this.editor.editingId = block.id;
        this.editor.error = null;
        this.editor.nameConflict = null;
        this.editor.form = formFromSpec(block.spec, block.enabled);
        this.editor.seriesRowErrors = this.editor.form.series.map(() => null);
        this.editor.open = true;
        this.focusEditorSoon();
      },

      closeEditor() {
        this.editor.open = false;
        this.editor.mode = "create";
        this.editor.editingId = null;
        this.editor.error = null;
        this.editor.nameConflict = null;
        this.editor.seriesRowErrors = [];
        this.editor.form = emptyEditorForm();
      },

      // The editor panel sits above the list in document order, so opening
      // it from either the top toolbar or a mid-list "Edit" click can land
      // off-screen; this brings it into view and moves focus to the first
      // field, same "arm before you read/write" language as the token
      // panel. Honors prefers-reduced-motion for the scroll itself (the
      // global CSS override already neutralizes transitions/animations,
      // but doesn't touch scrollIntoView's own behavior option).
      focusEditorSoon() {
        this.$nextTick(() => {
          const panel = document.getElementById("block-editor");
          const reduceMotion = window.matchMedia?.("(prefers-reduced-motion: reduce)").matches ?? false;
          panel?.scrollIntoView?.({ behavior: reduceMotion ? "auto" : "smooth", block: "start" });
          document.getElementById("editor-name")?.focus();
        });
      },

      ensureSeriesRow() {
        if (this.editor.form.type === "series" && this.editor.form.series.length === 0) {
          this.editor.form.series.push(emptySeriesRow());
          this.editor.seriesRowErrors.push(null);
        }
      },

      addSeriesRow() {
        this.editor.form.series.push(emptySeriesRow());
        this.editor.seriesRowErrors.push(null);
      },

      removeSeriesRow(index) {
        this.editor.form.series.splice(index, 1);
        this.editor.seriesRowErrors.splice(index, 1);
      },

      validateSeriesRows() {
        let ok = true;
        this.editor.seriesRowErrors = this.editor.form.series.map((row) => {
          const { error } = parseSkipEpisodes(row.skip_episodes);
          if (error) ok = false;
          return error ?? null;
        });
        return ok;
      },

      async submit() {
        this.editor.error = null;
        this.editor.nameConflict = null;
        if (!this.validateSeriesRows()) return;

        const spec = buildSpec(this.editor.form);
        const body: BlockWrite = { enabled: this.editor.form.enabled, spec };

        this.editor.submitting = true;
        try {
          if (this.editor.mode === "create") {
            const rec = await window.schedularr.apiSend<BlockRecord>("POST", "/api/v1/blocks", body);
            this.blocks = [...this.blocks, rec];
          } else {
            const id = this.editor.editingId;
            if (!id) throw new Error("editor is in edit mode with no editingId set");
            const rec = await window.schedularr.apiSend<BlockRecord>("PUT", `/api/v1/blocks/${id}`, body);
            this.blocks = this.blocks.map((b) => (b.id === id ? rec : b));
          }
          this.closeEditor();
        } catch (err) {
          if (err instanceof window.schedularr.ApiError && err.status === 409) {
            // describeError, not err.detail alone: the store's own
            // ErrConflict text is a terse "conflict" (internal/store/
            // blocks.go), so err.title ("block name already exists")
            // carries the actual meaning -- dropping it would show the
            // operator a fairly unhelpful single word.
            this.editor.nameConflict = describeError(err);
          } else {
            this.editor.error = describeError(err);
          }
        } finally {
          this.editor.submitting = false;
        }
      },

      // Full-replace PUT: the body carries the block's own currently
      // stored spec back unchanged, with only `enabled` flipped -- there is
      // no partial-update endpoint, and `enabled` is always explicit (see
      // this file's header comment).
      async toggleEnabled(block) {
        if (this.pendingId) return;
        this.pendingId = block.id;
        this.listError = null;
        try {
          const body: BlockWrite = { enabled: !block.enabled, spec: block.spec };
          const updated = await window.schedularr.apiSend<BlockRecord>(
            "PUT",
            `/api/v1/blocks/${block.id}`,
            body,
          );
          this.blocks = this.blocks.map((b) => (b.id === block.id ? updated : b));
        } catch (err) {
          this.listError = describeError(err);
        } finally {
          this.pendingId = null;
        }
      },

      requestDelete(id) {
        this.confirmDeleteId = id;
      },

      cancelDelete() {
        this.confirmDeleteId = null;
      },

      async performDelete(id) {
        if (this.pendingId) return;
        this.pendingId = id;
        this.listError = null;
        try {
          await window.schedularr.apiSend<void>("DELETE", `/api/v1/blocks/${id}`);
          this.blocks = this.blocks.filter((b) => b.id !== id);
          this.confirmDeleteId = null;
          if (this.editor.open && this.editor.editingId === id) this.closeEditor();
        } catch (err) {
          this.listError = describeError(err);
        } finally {
          this.pendingId = null;
        }
      },
    }),
  );
});
