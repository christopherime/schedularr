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
type MediaShow = components["schemas"]["MediaShow"];
type MediaMeta = components["schemas"]["MediaMeta"];
// createBlock and updateBlock both take BlockWrite -- either operation name
// resolves to the identical request type.
type BlockWrite = ApiRequestJSON<"createBlock">;
type OnComplete = NonNullable<SeriesConfig["on_complete"]>;

declare const Alpine: {
  data<T extends object>(name: string, factory: () => T): void;
};

// cronstrue is vendored as a plain UMD script (web/assets/vendor/
// cronstrue.min.js, loaded before this bundle -- see blocks/list.html's
// "page_js" block), the same no-CDN/no-runtime-dep convention as Alpine
// (DESIGN.md's "Vendored dependencies" note). It attaches itself to the
// global scope when loaded as a plain <script> (no module system present),
// so it's declared here the same way `Alpine` above is.
declare const cronstrue: {
  toString(expression: string, options?: { throwExceptionOnParseError?: boolean; verbose?: boolean }): string;
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

// ---- cron plain-language readback -----------------------------------------
//
// cronstrue (vendored, see the `declare const cronstrue` comment above)
// replaces this file's earlier hand-rolled cronHint(): that parser only
// recognized fixed-time/weekday-restricted patterns and returned null (no
// hint at all) for anything else -- a day-of-month/month restriction, a
// list/step on minute or hour. cronstrue reads any valid 5-field
// expression, so the readback is now universal rather than a narrow
// subset, and is shown next to the schedule field in both Simple and Cron
// mode (see EditorForm.scheduleMode below).

const CRONSTRUE_ERROR_PREFIX = "An error occurred";

/** Plain-language readback for a cron string, or null for blank/
 * unparseable input (never thrown out of a template expression -- Alpine
 * evaluates x-text/x-show inline, so a thrown error here would break the
 * whole editor panel's render, not just this one field). */
export function cronReadback(raw: string): string | null {
  const trimmed = raw.trim();
  if (trimmed === "") return null;
  let text: string;
  try {
    text = cronstrue.toString(trimmed, { throwExceptionOnParseError: false });
  } catch {
    return null;
  }
  return text.startsWith(CRONSTRUE_ERROR_PREFIX) ? null : text;
}

// ---- schedule picker: simple <-> cron ---------------------------------
//
// Feature spec (2026-08-29 UI wave): a Simple/Cron mode toggle on the
// schedule field. Simple mode drives a frequency select, day-of-week
// checkboxes (weekly/custom only), and a native <input type="time">,
// which together generate the 5-field cron string live
// (updateCronFromSimple(), called from list.html on every picker input).
// Cron mode is the pre-existing raw text field, unchanged. Storage/API
// are unaffected either way -- editor.form.cron is still the one value
// buildSpec() reads at submit time; scheduleMode/simpleSchedule/
// scheduleLockNote are UI-only state, never sent to the server (buildSpec
// only reads editor.form.cron, never these fields -- see buildSpec below).
//
// buildCronFromSimple/parseCronToSimple are exact inverses for every shape
// the picker itself can produce (see each function's own comment): a cron
// this file generates always round-trips back through Simple mode
// unchanged. Anything parseCronToSimple doesn't recognize -- a
// day-of-month combined with a weekday restriction, a month restriction,
// any list/range/step on minute or hour -- makes setScheduleMode("simple")
// refuse the switch and show scheduleLockNote instead of guessing at a
// lossy approximation.

type Frequency = "daily" | "weekdays" | "weekly" | "monthly" | "custom";

interface SimpleSchedule {
  frequency: Frequency;
  daysOfWeek: number[]; // 0 (Sunday) .. 6 (Saturday); "weekly"/"custom" only
  dayOfMonth: string; // "1".."31" as typed; "monthly" only
  time: string; // "HH:MM", 24h -- the native <input type="time"> wire format
}

interface ScheduleDayOption {
  value: number;
  label: string;
}

// Sunday-first, matching cron's own day-of-week numbering (0 = Sunday).
const SCHEDULE_DAY_OPTIONS: ScheduleDayOption[] = [
  { value: 0, label: "Sun" },
  { value: 1, label: "Mon" },
  { value: 2, label: "Tue" },
  { value: 3, label: "Wed" },
  { value: 4, label: "Thu" },
  { value: 5, label: "Fri" },
  { value: 6, label: "Sat" },
];

function emptySimpleSchedule(): SimpleSchedule {
  return { frequency: "daily", daysOfWeek: [], dayOfMonth: "1", time: "00:00" };
}

function clampDayOfMonth(raw: string): number {
  const n = Number(raw.trim());
  if (!Number.isFinite(n)) return 1;
  return Math.min(31, Math.max(1, Math.round(n)));
}

/** Parses an <input type="time"> value ("HH:MM"), defaulting to midnight
 * for blank/malformed input -- the picker always has *some* time to build
 * a cron from, even before the operator touches the field. */
function parseTimeInput(raw: string): [hour: number, minute: number] {
  const m = /^(\d{1,2}):(\d{2})$/.exec(raw.trim());
  if (!m) return [0, 0];
  const hour = Math.min(23, Math.max(0, Number(m[1])));
  const minute = Math.min(59, Math.max(0, Number(m[2])));
  return [hour, minute];
}

function pad2(n: number): string {
  return String(n).padStart(2, "0");
}

/** Builds the 5-field cron string the picker's current state represents.
 * Always a plain integer minute/hour (never a list/range/step) and either
 * "*", a plain day-of-month integer, or a comma-joined sorted list of
 * plain weekday digits for day-of-week -- exactly the shapes
 * parseCronToSimple recognizes below, so build -> parse -> build
 * round-trips for every frequency. */
function buildCronFromSimple(s: SimpleSchedule): string {
  const [hour, minute] = parseTimeInput(s.time);
  const m = String(minute);
  const h = String(hour);
  switch (s.frequency) {
    case "daily":
      return `${m} ${h} * * *`;
    case "weekdays":
      return `${m} ${h} * * 1,2,3,4,5`;
    case "monthly":
      return `${m} ${h} ${clampDayOfMonth(s.dayOfMonth)} * *`;
    case "weekly":
    case "custom": {
      const days = Array.from(new Set(s.daysOfWeek)).sort((a, b) => a - b);
      // An empty daysOfWeek here must never reach submit() -- "*" would
      // silently build a DAILY cron for a picker showing "Weekly"/"Custom
      // days", and that cron re-parses as "daily" on the next edit, with
      // no trace anything was ever wrong. validateSchedule() (below)
      // blocks submit() whenever daysOfWeek is empty for these two
      // frequencies, and onFrequencyChange() pre-selects today's weekday
      // the moment the operator switches into either one, so in practice
      // this branch is live-preview-only: it can render into
      // editor.form.cron while the operator is still mid-edit (e.g. the
      // instant after unchecking the last box), never into a saved spec.
      const dow = days.length > 0 ? days.join(",") : "*";
      return `${m} ${h} * * ${dow}`;
    }
  }
}

/** Inverse of buildCronFromSimple: recognizes exactly the shapes the
 * picker can produce and returns null for everything else (a
 * day-of-month/weekday combination, a month restriction, a list/range/
 * step on minute or hour, a non-plain day-of-month) -- callers treat null
 * as "this expression can't be represented in Simple mode" and lock to
 * Cron mode with a note rather than render a lossy guess. A single
 * weekday collapses to "weekly" (the picker's framing for "once a week,
 * on this day"); more than one, to "custom" -- exactly matching which
 * label building either one back through buildCronFromSimple reproduces,
 * since both write the same daysOfWeek-driven day-of-week field. */
function parseCronToSimple(raw: string): SimpleSchedule | null {
  const fields = raw.trim().split(/\s+/);
  if (fields.length !== 5) return null;
  const [min, hour, dom, mon, dow] = fields;
  if (mon !== "*") return null;
  if (!/^\d{1,2}$/.test(min) || !/^\d{1,2}$/.test(hour)) return null;
  const m = Number(min);
  const h = Number(hour);
  if (m > 59 || h > 23) return null;
  const time = `${pad2(h)}:${pad2(m)}`;

  if (dom !== "*") {
    if (dow !== "*") return null; // day-of-month + weekday together: not representable
    if (!/^([1-9]|[12]\d|3[01])$/.test(dom)) return null;
    return { frequency: "monthly", daysOfWeek: [], dayOfMonth: dom, time };
  }

  if (dow === "*") {
    return { frequency: "daily", daysOfWeek: [], dayOfMonth: "1", time };
  }

  const days = dow.split(",").map((d) => d.trim());
  if (!days.every((d) => /^[0-6]$/.test(d))) return null;
  const nums = Array.from(new Set(days.map(Number))).sort((a, b) => a - b);

  if (nums.length === 5 && [1, 2, 3, 4, 5].every((d) => nums.includes(d))) {
    return { frequency: "weekdays", daysOfWeek: [], dayOfMonth: "1", time };
  }

  return { frequency: nums.length === 1 ? "weekly" : "custom", daysOfWeek: nums, dayOfMonth: "1", time };
}

// ---- library-aware autocomplete (media discovery) --------------------
//
// GET /api/v1/media/shows and GET /api/v1/media/meta back a show-title
// <datalist> (series rows) and genre/rating <datalist>s (filter block +
// fallback filler filter) -- fetched once per editor open and reused
// across every row, never per-row (loadMedia(), called from openCreate/
// openEdit). Free text is always accepted regardless of fetch outcome; a
// failed fetch degrades silently to no datalist and no warnings (no
// console logging either -- see loadMedia's own comment), never a
// .problem panel, since this is a convenience layer on top of an editor
// that already works entirely with free text.

const SHOW_TITLE_NOT_FOUND = "Not found in Tunarr's library.";

/** True when `title` (trimmed) doesn't case-insensitively match any
 * loaded show title. Case-insensitive on purpose: the warning is a soft,
 * non-blocking nudge, not a strict validator, and a pure case mismatch
 * ("the wire" vs "The Wire") isn't a meaningfully different problem from
 * a match. */
function titleKnown(shows: MediaShow[], title: string): boolean {
  const needle = title.trim().toLowerCase();
  return shows.some((s) => s.title.toLowerCase() === needle);
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
  // scheduleMode/simpleSchedule/scheduleLockNote are UI-only picker state
  // (see the "schedule picker" section above) -- buildSpec() below only
  // ever reads `cron`, never these three, so they're never sent to the
  // server regardless of which mode produced the cron string.
  scheduleMode: "simple" | "cron";
  simpleSchedule: SimpleSchedule;
  scheduleLockNote: string | null;
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

// ---- series row reordering ---------------------------------------------
//
// A block's `series` array order IS the airing order (see
// docs/scheduling-concepts.md's "Idempotent apply and editing a block
// before it airs"): a not-yet-aired occurrence re-derives from the
// block's current spec on every apply, so reordering here changes what
// airs next without touching anything already aired. Exported for direct
// testing, the same convention as cronReadback above.

/** Swaps `arr[index]` with its neighbor in `direction` (-1 up, +1 down),
 * in place. A no-op past either end of the array -- list.html already
 * disables the first row's up button and the last row's down button, so
 * this is defense-in-depth against a stray call, not the primary guard. */
export function swapAdjacent<T>(arr: T[], index: number, direction: -1 | 1): void {
  const target = index + direction;
  if (index < 0 || index >= arr.length || target < 0 || target >= arr.length) return;
  [arr[index], arr[target]] = [arr[target], arr[index]];
}

function emptyEditorForm(): EditorForm {
  const simpleSchedule = emptySimpleSchedule();
  return {
    type: "filter",
    name: "",
    // A new block starts in Simple mode with the picker's own defaults
    // (daily, midnight) already reflected into `cron`, rather than an
    // empty cron field the operator has to notice and fill in before the
    // readback/picker show anything live.
    cron: buildCronFromSimple(simpleSchedule),
    scheduleMode: "simple",
    simpleSchedule,
    scheduleLockNote: null,
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

// scheduleMode/simpleSchedule/scheduleLockNote are placeholders here --
// this function only converts the wire spec's flat `cron` string into
// form shape. openEdit() (the only caller) immediately overwrites all
// three by running the loaded cron through parseCronToSimple(), the same
// "derive UI-only state right after loading" pattern seriesRowErrors
// already uses one line below its own call site.
function formFromSpec(spec: BlockSpec, enabled: boolean): EditorForm {
  return {
    type: spec.type === "series" ? "series" : "filter",
    name: spec.name,
    cron: spec.cron,
    scheduleMode: "cron",
    simpleSchedule: emptySimpleSchedule(),
    scheduleLockNote: null,
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
  // Set by validateSchedule() (called from submit()) when
  // form.simpleSchedule.frequency is "weekly"/"custom" and daysOfWeek is
  // empty -- the hard guard against silently saving a DAILY cron for a
  // picker showing "Weekly"/"Custom days". Cleared the moment the
  // operator checks a day (toggleScheduleDay) or on the next submit()
  // attempt, whichever comes first.
  scheduleDaysError: string | null;
  form: EditorForm;
  // id of the element focus returns to once the panel closes -- the
  // toolbar's "+ New Block" button for create, or the specific row's Edit
  // button for edit (see openCreate/openEdit/closeEditor below). Falls
  // back to "new-block-btn" if the row's own button is somehow gone by
  // the time focus returns (e.g. the row was deleted while its own
  // editor was open -- performDelete() already calls closeEditor() in
  // that case).
  returnFocusId: string;
}

interface BlocksState {
  blocksLoading: boolean;
  blocksError: string | null;
  blocks: BlockRecord[];
  listError: string | null;

  channelsLoading: boolean;
  channelsError: string | null;
  channels: Channel[];

  // Library-aware autocomplete (media discovery): fetched once per editor
  // open (loadMedia(), called from openCreate/openEdit), reused across
  // every series/filter/fallback row for that open session. mediaOk
  // gates both datalist rendering and the show-title warning -- a failed
  // fetch degrades silently to free text with neither (see loadMedia's
  // own comment).
  mediaShows: MediaShow[];
  mediaGenres: string[];
  mediaRatings: string[];
  mediaOk: boolean;

  pendingId: string | null;
  confirmDeleteId: string | null;

  editor: EditorState;
  scheduleDayOptions: ScheduleDayOption[];

  init(): void;
  loadBlocks(): Promise<void>;
  loadChannels(): Promise<void>;
  loadMedia(): Promise<void>;

  cronReadback(raw: string): string | null;
  channelLabel(c: Channel): string;
  channelSelectOptions(): Channel[];
  channelHint(): string;
  seriesTitleWarning(title: string): string | null;

  setScheduleMode(mode: "simple" | "cron"): void;
  onFrequencyChange(): void;
  updateCronFromSimple(): void;
  toggleScheduleDay(day: number): void;
  validateSchedule(): boolean;

  openCreate(): void;
  openEdit(block: BlockRecord): void;
  closeEditor(): void;
  focusEditorSoon(): void;
  focusReturnSoon(): void;
  ensureSeriesRow(): void;
  addSeriesRow(): void;
  removeSeriesRow(index: number): void;
  moveSeriesRowUp(index: number): void;
  moveSeriesRowDown(index: number): void;
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

      mediaShows: [],
      mediaGenres: [],
      mediaRatings: [],
      mediaOk: false,

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
        scheduleDaysError: null,
        form: emptyEditorForm(),
        returnFocusId: "new-block-btn",
      },
      scheduleDayOptions: SCHEDULE_DAY_OPTIONS,

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

      // Fetched once per editor open (called from openCreate/openEdit,
      // never per-row), reused across every series/filter/fallback row
      // for that open session. Both calls run in parallel; either or both
      // can fail independently (Promise.allSettled, not Promise.all) --
      // a genre/rating fetch failure shouldn't also blank out the show
      // list a moment earlier fetched successfully, or vice versa.
      // Failure degrades silently: mediaOk stays false, no datalist
      // renders (list.html gates every <datalist> on mediaOk), the
      // show-title warning never fires, and there is no console logging
      // -- this is a convenience layer over an editor that already works
      // entirely with free text, not a required data source, so it never
      // raises a .problem panel either.
      async loadMedia() {
        const [showsResult, metaResult] = await Promise.allSettled([
          window.schedularr.apiGet<MediaShow[]>("/api/v1/media/shows"),
          window.schedularr.apiGet<MediaMeta>("/api/v1/media/meta"),
        ]);
        const showsOk = showsResult.status === "fulfilled";
        const metaOk = metaResult.status === "fulfilled";
        this.mediaShows = showsOk ? showsResult.value : [];
        this.mediaGenres = metaOk ? metaResult.value.genres : [];
        this.mediaRatings = metaOk ? metaResult.value.ratings : [];
        // Both must succeed for mediaOk: the show-title warning
        // specifically needs mediaShows, and a genre/rating datalist
        // needs mediaMeta -- "mediaOk" gates both uniformly rather than
        // tracking two independent flags the templates would otherwise
        // have to check separately.
        this.mediaOk = showsOk && metaOk;
      },

      cronReadback,
      channelLabel,

      // Factual, non-blocking nudge -- never blocks submit (buildSpec/
      // submit() never consult this). Case-insensitive on purpose (see
      // titleKnown's own comment); returns null (no warning) for a blank
      // title or whenever the fetch didn't succeed, per the feature
      // spec's "media fetch failure -> no warnings" rule.
      seriesTitleWarning(title) {
        if (!this.mediaOk) return null;
        if (title.trim() === "") return null;
        return titleKnown(this.mediaShows, title) ? null : SHOW_TITLE_NOT_FOUND;
      },

      // Attempts to switch into Simple mode by parsing the current cron
      // text; a non-representable expression refuses the switch and sets
      // scheduleLockNote instead (see parseCronToSimple's own comment).
      // Switching to Cron mode always succeeds -- the raw field can show
      // any string, representable or not.
      setScheduleMode(mode) {
        if (mode === this.editor.form.scheduleMode) return;
        if (mode === "cron") {
          this.editor.form.scheduleMode = "cron";
          this.editor.form.scheduleLockNote = null;
          return;
        }
        const parsed = parseCronToSimple(this.editor.form.cron);
        if (!parsed) {
          this.editor.form.scheduleLockNote = "This cron expression can't be represented in Simple mode.";
          return;
        }
        this.editor.form.simpleSchedule = parsed;
        this.editor.form.scheduleMode = "simple";
        this.editor.form.scheduleLockNote = null;
        // A parsed weekly/custom pattern always has >=1 day (dow !== "*"
        // guarantees a non-empty split in parseCronToSimple), so this is
        // always clearing stale state from an earlier attempt, never
        // masking a real problem with the just-parsed result.
        this.editor.scheduleDaysError = null;
      },

      // Wired to the frequency <select>'s own @change (not
      // updateCronFromSimple() directly): switching INTO "weekly"/"custom"
      // with no day yet checked pre-selects today's weekday, so the
      // picker never silently represents "every day" the instant the
      // operator picks "Weekly" -- see buildCronFromSimple's own comment
      // on why an empty daysOfWeek must never reach submit() un-caught.
      // Only fires when daysOfWeek is actually empty -- switching away
      // and back preserves whatever the operator already chose.
      onFrequencyChange() {
        const s = this.editor.form.simpleSchedule;
        if ((s.frequency === "weekly" || s.frequency === "custom") && s.daysOfWeek.length === 0) {
          s.daysOfWeek = [new Date().getDay()];
        }
        this.updateCronFromSimple();
      },

      // Called from every Simple-mode picker control (list.html) on
      // change -- keeps editor.form.cron (the value submit() actually
      // reads) live in sync with the picker's current state.
      updateCronFromSimple() {
        this.editor.form.cron = buildCronFromSimple(this.editor.form.simpleSchedule);
      },

      toggleScheduleDay(day) {
        const days = this.editor.form.simpleSchedule.daysOfWeek;
        const idx = days.indexOf(day);
        if (idx === -1) days.push(day);
        else days.splice(idx, 1);
        // Immediate feedback: the moment a day is checked, the "pick at
        // least one day" error (if showing) is no longer true -- don't
        // make the operator re-click Save just to see it clear.
        if (days.length > 0) this.editor.scheduleDaysError = null;
        this.updateCronFromSimple();
      },

      // Hard guard, run from submit() before anything else: a Simple-mode
      // schedule set to "weekly"/"custom" with no day checked is not a
      // valid state to save (see buildCronFromSimple's comment) -- this
      // is what actually keeps that silent-daily-cron branch unreachable
      // in practice, rather than relying on the UX default
      // (onFrequencyChange) alone, since the operator can still uncheck
      // every box after it pre-selects one.
      validateSchedule() {
        const form = this.editor.form;
        if (
          form.scheduleMode === "simple" &&
          (form.simpleSchedule.frequency === "weekly" || form.simpleSchedule.frequency === "custom") &&
          form.simpleSchedule.daysOfWeek.length === 0
        ) {
          this.editor.scheduleDaysError = "Pick at least one day.";
          return false;
        }
        this.editor.scheduleDaysError = null;
        return true;
      },

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
          return `Tunarr channel list unavailable (${this.channelsError}) — enter the channel ID manually.`;
        }
        if (this.channels.length === 0) {
          return "Tunarr returned no channels — enter the channel ID manually.";
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
        this.editor.scheduleDaysError = null;
        this.editor.returnFocusId = "new-block-btn";
        this.editor.open = true;
        this.focusEditorSoon();
        void this.loadMedia();
      },

      openEdit(block) {
        this.editor.mode = "edit";
        this.editor.editingId = block.id;
        this.editor.error = null;
        this.editor.nameConflict = null;
        this.editor.form = formFromSpec(block.spec, block.enabled);
        this.editor.seriesRowErrors = this.editor.form.series.map(() => null);
        this.editor.scheduleDaysError = null;
        // Derive the picker's starting mode from the loaded cron: Simple
        // mode (pre-parsed) when representable, Cron mode with an
        // explanatory note otherwise -- same rule setScheduleMode("simple")
        // applies on an explicit mode switch, applied once up front here
        // so the operator isn't shown a picker that doesn't match what's
        // actually stored.
        const parsed = parseCronToSimple(this.editor.form.cron);
        if (parsed) {
          this.editor.form.simpleSchedule = parsed;
          this.editor.form.scheduleMode = "simple";
          this.editor.form.scheduleLockNote = null;
        } else {
          this.editor.form.scheduleMode = "cron";
          this.editor.form.scheduleLockNote = "This cron expression can't be represented in Simple mode.";
        }
        this.editor.returnFocusId = `block-edit-${block.id}`;
        this.editor.open = true;
        this.focusEditorSoon();
        void this.loadMedia();
      },

      closeEditor() {
        this.editor.open = false;
        this.editor.mode = "create";
        this.editor.editingId = null;
        this.editor.error = null;
        this.editor.nameConflict = null;
        this.editor.seriesRowErrors = [];
        this.editor.scheduleDaysError = null;
        this.editor.form = emptyEditorForm();
        this.focusReturnSoon();
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

      // Moves focus back to whatever opened the editor once the x-show
      // swap has hidden the panel -- otherwise focus is left stranded on
      // the now-removed Cancel/close-X button (the browser drops it to
      // <body>), silent to a keyboard or screen-reader user. Same
      // $nextTick idiom as focusEditorSoon() above and requestDelete()
      // below; falls back to the toolbar's "+ New Block" button if the
      // row-specific id (edit mode) is gone by the time this runs, e.g.
      // performDelete() closing an editor whose own row it just removed.
      focusReturnSoon() {
        const id = this.editor.returnFocusId;
        this.$nextTick(() => {
          (document.getElementById(id) ?? document.getElementById("new-block-btn"))?.focus();
        });
      },

      // Deliberately non-destructive: switching the type selector away
      // from "series" and back does NOT clear editor.form.series or
      // seriesRowErrors, only adds a first empty row the first time the
      // operator switches into series. This preserves already-entered
      // series data across an accidental (or exploratory) type toggle.
      // submit() is responsible for making sure that leftover series-row
      // state can never block a filter-type submit -- see submit()'s own
      // comment.
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

      // Keeps editor.seriesRowErrors aligned to the same rows as
      // editor.form.series -- addSeriesRow/removeSeriesRow already keep
      // the two in lockstep, a reorder must too, so an inline skip-episode
      // error stays attached to the row it was actually raised for.
      moveSeriesRowUp(index) {
        swapAdjacent(this.editor.form.series, index, -1);
        swapAdjacent(this.editor.seriesRowErrors, index, -1);
      },

      moveSeriesRowDown(index) {
        swapAdjacent(this.editor.form.series, index, 1);
        swapAdjacent(this.editor.seriesRowErrors, index, 1);
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
        // Applies to both block types (the schedule field isn't gated by
        // type) -- must run before buildSpec() ever reads editor.form.cron,
        // since a Simple-mode weekly/custom schedule with no day checked
        // would otherwise silently save as a daily cron (see
        // buildCronFromSimple's comment).
        if (!this.validateSchedule()) return;
        // Scoped to type === "series": switching the type selector away
        // from series deliberately leaves editor.form.series/
        // seriesRowErrors alone (non-destructive toggling -- see
        // ensureSeriesRow's comment), so a filter-type submit must never
        // be blocked by a leftover invalid series row from an earlier
        // series-type edit in the same session. A series-type submit
        // still validates every row, with inline errors, as before.
        if (this.editor.form.type === "series" && !this.validateSeriesRows()) return;

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

      // Moves focus onto the Confirm button once the x-if swap has
      // replaced the row's Edit/Delete pair with the Confirm/Cancel pair --
      // otherwise focus is left stranded on the now-removed Delete button
      // (the browser drops it to <body>), which is silent to a keyboard or
      // screen-reader user. $nextTick + getElementById (not the
      // `autofocus` HTML attribute) because autofocus's WHATWG processing
      // model is one-shot per Document: the very first autofocus-bearing
      // element ever inserted claims the browsing context's single
      // "autofocus processed" flag, and every later insertion -- e.g. this
      // same button reappearing after a Cancel, or a different row's
      // Confirm button -- is silently ignored. Same $nextTick idiom as
      // focusEditorSoon() above; getElementById (not x-ref) since only one
      // row is ever in the confirm state at a time (confirmDeleteId is a
      // single value), so the static id can't collide.
      requestDelete(id) {
        this.confirmDeleteId = id;
        this.$nextTick(() => {
          document.getElementById("block-delete-confirm-btn")?.focus();
        });
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
