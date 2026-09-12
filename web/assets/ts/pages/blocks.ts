// Blocks page ("/blocks/"): list every stored block, toggle/edit/delete it,
// and create/edit blocks through an inline editor panel (not a modal --
// this form is long enough that interrupting the page for it isn't
// justified; see the "blocks: editor panel" comment in main.css). One
// bundle per page since the v0.5.0 runtime refactor -- this entry compiles
// the shared runtime in with itself (see partials/ui/page-js.html).
//
// Three contract rules drive most of the awkward-looking code below, all
// from the API's write semantics (api/openapi.yaml, BlockWrite):
//
//   1. PUT is a full replace, and so requires If-Match carrying the
//      updated_at this editor loaded. Never build a partial PUT body.
//   2. PATCH is the field-scoped write, and is what the list's toggle
//      uses: a toggle that resent the whole spec would overwrite a spec
//      edit made in another tab, which is also why PATCH needs no
//      If-Match -- it has no unrelated state to lose.
//   3. BlockWrite.enabled defaults to true when omitted. This UI never
//      relies on that default -- create and edit both send `enabled`
//      explicitly, even when it happens to already be what the default
//      would have produced.
//
// A further, less obvious constraint: the server decodes both create and
// update bodies with encoding/json's DisallowUnknownFields (see
// internal/api/blocks.go), so a request body may only contain fields the
// OpenAPI schema actually defines -- no client-side convenience fields can
// ride along in the JSON we send. buildSpec()/buildFilter()/etc. below stay
// close to gen/types.d.ts's BlockSpec shape for exactly that reason.
import { ApiError, apiGet, apiPath, apiSend, onReauth } from "../runtime/api.ts";
import type { ApiRequestJSON, ApiResponse } from "../runtime/api.ts";
import { serverNow, subscribe } from "../runtime/bus.ts";
import { channelHint as channelHintText, channelLabel, channelPlate, loadChannels } from "../runtime/channels.ts";
import type { Channel, PlateParts } from "../runtime/channels.ts";
import { cronReadback } from "../runtime/cron.ts";
import { draftHref } from "../runtime/draft.ts";
import { describeError, toProblemView } from "../runtime/errors.ts";
import type { ProblemView } from "../runtime/errors.ts";
import { durationLabel, formatClock, formatLocal, ordinal, pad2, plural, sxxeyy, untilTime } from "../runtime/format.ts";
import { isContending, priorityRank } from "../runtime/rank.ts";
import type { RankPeer } from "../runtime/rank.ts";
import { initShell, onResume } from "../runtime/shell.ts";
import { printTape } from "../runtime/tape.ts";
import type { components } from "../gen/types";

initShell();

type BlockRecord = ApiResponse<"listBlocks", 200>[number];
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
type CronOccurrences = ApiResponse<"nextCronOccurrences", 200>;
type SeriesState = components["schemas"]["SeriesState"];
type OnComplete = NonNullable<SeriesConfig["on_complete"]>;

declare const Alpine: {
  data<T extends object>(name: string, factory: () => T): void;
};

// The cron plain-language readback lives in runtime/cron.ts since v0.5.1
// (the guide inspector reads it too); cronstrue itself stays the vendored
// UMD global loaded before this bundle (ui/page-js's `cronstrue: true`).

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

/** Builds the 5-field cron string the picker's current state represents.
 * Always a plain integer minute/hour (never a list/range/step) and either
 * "*", a plain day-of-month integer, or a comma-joined sorted list of
 * plain weekday digits for day-of-week -- exactly the shapes
 * parseCronToSimple recognizes below, so build -> parse -> build
 * round-trips for every frequency. */
export function buildCronFromSimple(s: SimpleSchedule): string {
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
export function parseCronToSimple(raw: string): SimpleSchedule | null {
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
export function buildSpec(form: EditorForm): BlockSpec {
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
    // 0 is "unset" throughout the model -- the engine treats a zero
    // start as 1, and specs bootstrapped before the import funnel
    // mirrored the CUE defaults still store 0s. Reading them as 1 here
    // keeps the summary from claiming "from S00E00" and the rewind path
    // from PATCHing a position nothing airs at.
    episodes_per_block: String(sc.episodes_per_block || 1),
    start_season: numToStr(sc.start_season || 1),
    start_episode: numToStr(sc.start_episode || 1),
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
export function formFromSpec(spec: BlockSpec, enabled: boolean): EditorForm {
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

// ---- the list row's readings (v0.5.10) ----------------------------------
//
// NEXT, the duration annotation, the priority rank and the DARK UNTIL chip
// are the four facts this slice added to a row. Three of them fold into
// cells that already answer their question and only NEXT took a column of
// its own -- see web/DESIGN.md, "The blocks row", for why a ten-column
// table was the wrong answer to five new facts.
//
// All four are pure functions taking `now` rather than reading a clock.
// Two reasons: the test runner has no DOM, so a pure function is the only
// part of a row that can be pinned at all; and the caller passes
// serverNow() (runtime/bus.ts), never Date.now() -- the heartbeat's skew
// correction is what keeps "in 3 h" honest on a laptop whose clock
// drifted, and a helper reaching for Date.now() itself would quietly opt
// every row on this page out of it.

/** The NEXT cell. `kind` drives the cell's own treatment -- an absent
 * instant renders as a state legend rather than as a value -- and is what
 * makes the three absences tellable apart at a glance. */
export interface NextReading {
  kind: "instant" | "disabled" | "unreadable" | "never";
  /** The cell's headline: the countdown, or the state's legend. */
  primary: string;
  /** The line under it: the absolute local instant, or what to do next. */
  detail: string;
}

/**
 * What the NEXT column says for one row.
 *
 * `next_occurrence` is absent for THREE different reasons (see the field's
 * own description in api/openapi.yaml): the block is disabled, its cron
 * will not parse, or it parses but never comes round. One em dash covering
 * all three would hide the only thing worth knowing -- which of them
 * applies, and whether it is the operator's to fix -- so each absence gets
 * its own legend and its own recovery line.
 *
 * The unreadable branch reads cronstrue, which renders PROSE and computes
 * no instant: the client still never evaluates cron semantics (spec §10).
 * It is the same signal the Schedule cell two columns over already uses to
 * decide whether to print a readback, so the two cells cannot disagree
 * about whether the expression can be read at all. If the two parsers ever
 * disagree the other way -- cronstrue reads an expression the server
 * refused -- the row falls through to "never fires", whose recovery line
 * points at the same cron either way.
 *
 * Order matters: a disabled block whose cron is also broken reads
 * DISABLED, because turning it on is the step that comes first and the
 * Status column beside it already says exactly that.
 */
export function nextReading(
  block: { enabled: boolean; next_occurrence?: string; spec: { cron: string } },
  now: number,
): NextReading {
  if (!block.enabled) {
    return { kind: "disabled", primary: "Disabled", detail: "Enable the block to schedule it." };
  }
  if (block.next_occurrence) {
    // untilTime, not relativeTime, for the same reason the bezel's NEXT
    // TICK uses it: the server computes this instant before the block
    // starts airing, so an occurrence already under way leaves a past
    // value on a perfectly fresh row. "12 min ago" there reads as a
    // missed airing; "due" reads as one in progress.
    //
    // A dark block reports the first occurrence at or after it wakes
    // rather than the next raw cron tick, so this instant is real airtime
    // whichever switch the block is sitting behind.
    return {
      kind: "instant",
      primary: untilTime(block.next_occurrence, now),
      detail: formatLocal(block.next_occurrence),
    };
  }
  if (cronReadback(block.spec.cron) === null) {
    return { kind: "unreadable", primary: "Cron unreadable", detail: "Fix the expression to schedule this block." };
  }
  return { kind: "never", primary: "Never fires", detail: "No date matches this cron." };
}

/**
 * The DARK UNTIL chip's text, or null when there is nothing to paint.
 *
 * Null for an absent value, for an unparseable one, and -- the case that
 * actually bites -- for one in the PAST: a dark window whose wake-up has
 * already passed suppresses nothing, and a chip there would report a state
 * the server does not hold. Doubles as the "is this block dark right now"
 * predicate, so the chip and the rank suppression below cannot end up
 * disagreeing about the same instant.
 */
export function darkUntilLabel(iso: string | undefined, now: number): string | null {
  if (!iso) return null;
  const t = new Date(iso).getTime();
  if (Number.isNaN(t) || t <= now) return null;
  return `Dark until ${formatLocal(iso)}`;
}

/**
 * The priority line under the channel plate: `PRI 50`, or `PRI 50 · 2nd of
 * 5` once this block is actually contending for airtime.
 *
 * A disabled or currently-dark block prints the bare number. It is not in
 * the field right now, and printing a rank would announce a contest it is
 * not in. The field itself (`of`) still counts every enabled peer,
 * including one sitting dark: that rule belongs to runtime/rank.ts, which
 * the guide's inspector reads too, and a second opinion about it here is
 * exactly the drift the shared helper was extracted to end.
 *
 * `peers` is the same-channel list, whole -- priorityRank drops the
 * disabled ones itself. Filtering by channel is the caller's job; filtering
 * by enabled is not.
 */
export function priorityLabel(
  block: { enabled: boolean; disabled_until?: string; spec: { priority?: number } },
  peers: readonly RankPeer[],
  now: number,
): string {
  const priority = block.spec.priority ?? 0;
  // isContending, not a local re-derivation of it: the guide's inspector
  // asks the same function, and the whole point of runtime/rank.ts is that
  // there is one answer to "may this be ranked" rather than two that agree
  // until someone edits one of them.
  if (!isContending(block, now)) return `PRI ${priority}`;
  const { rank, of } = priorityRank(priority, peers);
  if (of === 0) return `PRI ${priority}`;
  return `PRI ${priority} · ${ordinal(rank)} of ${of}`;
}

// ---- the row's power tools (v0.5.10) ------------------------------------

/** What the page knows when a row action is about to arm. */
export interface RowActionGate {
  /** A row action is already in flight (the single global pendingId). */
  pending: boolean;
  /** The editor panel is open, on any block or on none. */
  editorOpen: boolean;
}

/**
 * Whether Duplicate or the dark window may arm right now.
 *
 * One predicate for both, because both are refused in exactly the same
 * two states -- for two different reasons worth stating, since neither
 * failure would announce itself:
 *
 *   pending: pendingId is ONE slot, not one per row (the toggle and the
 *   delete share it). A second write armed over the first is dropped by
 *   its own re-entrancy guard, silently, after the operator has already
 *   clicked.
 *
 *   editorOpen: Duplicate opens the editor on the copy, which would take
 *   the panel away from a half-typed block. The dark window splices the
 *   PATCH's returned record into this.blocks -- and this.blocks is where
 *   submit() reads its If-Match. If that block moved elsewhere since the
 *   list loaded, the returned record carries the OTHER edit's updated_at,
 *   the open editor's next save is re-armed against a record nobody on
 *   this screen has seen, and it succeeds -- full-replacing the other
 *   tab's spec. Exactly the lost update planInvalidatedReaction freezes
 *   the list to prevent, arriving through a row action instead of a
 *   frame.
 */
export function canArmRowAction(gate: RowActionGate): boolean {
  return !gate.pending && !gate.editorOpen;
}

/** The dark-window presets, in the order the disclosure lists them. */
export type DarkPreset = "tomorrow" | "week" | "four-weeks";

export interface DarkPresetOption {
  id: DarkPreset;
  label: string;
}

// Whole days, all of them: an "in a month" preset would have to answer
// what the 31st of January plus one month is, and JS answers "the 3rd of
// March". Four weeks is the same span with no case to explain.
const DARK_PRESET_DAYS: Record<DarkPreset, number> = {
  tomorrow: 1,
  week: 7,
  "four-weeks": 28,
};

export const DARK_PRESETS: DarkPresetOption[] = [
  { id: "tomorrow", label: "Tomorrow" },
  { id: "week", label: "Next week" },
  { id: "four-weeks", label: "In four weeks" },
];

/**
 * The instant a preset wakes the block: local midnight, N days on.
 *
 * "Until tomorrow" is a WALL-CLOCK question, not an arithmetic one. Adding
 * 86_400_000 ms at 23:59 takes the block dark until tomorrow NIGHT, a full
 * day past what the operator asked for, and across a DST boundary every
 * preset lands an hour off. Stepping the local date fields instead is what
 * makes "tomorrow" mean the start of tomorrow in the operator's own zone,
 * whatever the calendar does that night.
 *
 * `now` is passed in and is always serverNow() (runtime/bus.ts) at the
 * moment of the write, never Date.now(): the heartbeat's skew correction
 * is the difference between a wake-up the server agrees with and one that
 * lands in its past, where it suppresses nothing and reports nothing.
 */
export function darkPresetInstant(preset: DarkPreset, now: number): string {
  const wake = new Date(now);
  wake.setHours(0, 0, 0, 0);
  wake.setDate(wake.getDate() + DARK_PRESET_DAYS[preset]);
  return wake.toISOString();
}

// ---- the editor's consequence rail (v0.5.10) ----------------------------
//
// The rail answers "what will this do" while the block is still an edit:
// when the next three occurrences actually run, what else is contending
// for that channel, and -- for a series block -- what airs in what order.
// Every instant in it is the SERVER's, from GET /cron/next: the readback
// two fields up is cronstrue rendering prose about the expression, which
// is the one thing the client may do with a cron (spec §10).

// Occurrences the rail asks for. Three is a consequence; ten would be a
// forecast, and the contract caps `count` at ten anyway. Module-local, the
// same convention as DARK_PRESET_DAYS above: nothing outside this file has
// a reason to know it.
const RAIL_OCCURRENCES = 3;

/**
 * One occurrence as the rail prints it: the start the server computed,
 * and the end the block's own duration puts on it.
 *
 * `GET /cron/next` returns STARTS only, deliberately, and the end is the
 * one piece of occurrence math that belongs on this side: adding a
 * duration to an instant needs no calendar knowledge, which is exactly
 * what separates it from evaluating the expression.
 *
 * The end prints as a bare clock while it lands on the start's own local
 * day and as a full instant when it does not -- a block running 23:30 to
 * 01:00 would otherwise read as ending ninety minutes BEFORE it starts,
 * and "+1" is notation this UI has never taught anyone.
 *
 * A duration the operator has not typed yet (parseRequiredInt coerces the
 * blank field to 0) or a start that will not parse prints the start alone.
 * A range from an unknown length would be a fabricated reading, and the
 * start is the half that is actually known.
 */
export function occurrenceLabel(startIso: string, durationMinutes: number): string {
  const start = new Date(startIso);
  const startMs = start.getTime();
  if (Number.isNaN(startMs)) return formatLocal(startIso);
  if (!Number.isFinite(durationMinutes) || durationMinutes <= 0) return formatLocal(startIso);
  const end = new Date(startMs + durationMinutes * 60_000);
  // toDateString() compares LOCAL calendar days, which is the day the
  // operator is reading -- a UTC comparison would call a 20:00-22:00
  // evening block a midnight crossing in half the world's timezones.
  const sameDay = end.toDateString() === start.toDateString();
  return `${formatLocal(startIso)} – ${sameDay ? formatClock(end.getTime()) : formatLocal(end.toISOString())}`;
}

/** What the rail's occurrence group is currently showing. One value
 * rather than four booleans in the template, so the precedence -- a
 * failure outranks everything, and "never" is a real answer rather than
 * an empty list -- is decided once, here, and can be pinned by a test. */
export type RailState = "loading" | "error" | "prompt" | "never" | "occurrences";

/**
 * Which of the five the rail is in.
 *
 * Read against `rail.expr` -- the expression the held instants actually
 * answer for -- and not against the form's cron alone. The two differ
 * for as long as the debounce is still counting down, and printing the
 * PREVIOUS expression's occurrences under the current one is a settled
 * reading of a question nobody asked: the picker's preview says "the
 * 15th" while the rail lists the 1st, and nothing on screen says which
 * one it is answering. That is the same out-of-order failure
 * loadRailOccurrences refuses on the way in, seen on the way out. An
 * expression nothing has answered yet reads as `loading`, because a
 * read of it is on its way.
 *
 * `never` exists because a well-formed expression that never comes round
 * (February 30th) returns an EMPTY array rather than a 400
 * (api/openapi.yaml): the expression is fine and the answer is genuinely
 * "never". Rendering that as an empty rail would report it as "nothing
 * yet", which is the one reading an instrument must not give.
 */
export function railState(
  rail: { loading: boolean; error: string | null; starts: readonly string[]; expr: string },
  cron: string,
): RailState {
  const expr = cron.trim();
  if (expr === "") return "prompt";
  if (rail.loading || rail.expr !== expr) return "loading";
  if (rail.error) return "error";
  return rail.starts.length === 0 ? "never" : "occurrences";
}

/** What a collapsed series row says about itself. `kind` is what keeps a
 * row findable: an incomplete row has no title to print, and a blank line
 * in a list of twelve is a row the operator cannot get back to. */
export interface SeriesRowSummary {
  kind: "named" | "incomplete";
  text: string;
}

/**
 * The one line a collapsed series row shows -- and the same line the
 * rail's lineup prints, so the row and the projection of it can never
 * describe the same show differently.
 *
 * Both REQUIRED fields count as identity here. A row with no show title
 * has nothing to be called; a row with no episode count is not yet a
 * schedule. Either summarises to what is missing, in warn voice, so
 * collapsing a dozen rows cannot hide the one that will 400 on save.
 * Findable beats complete: the title still leads when there is one,
 * because "Breaking Bad — add an episode count" points at a row and
 * "row 3 is incomplete" makes the operator open all twelve.
 */
export function seriesRowSummary(row: {
  show_title: string;
  episodes_per_block: string;
  start_season: string;
  start_episode: string;
}): SeriesRowSummary {
  const title = row.show_title.trim();
  if (title === "") return { kind: "incomplete", text: "Untitled — add a show title" };
  const episodes = parseOptionalInt(row.episodes_per_block);
  if (episodes === undefined || episodes <= 0) {
    return { kind: "incomplete", text: `${title} — add an episode count` };
  }
  const parts = [title, plural(episodes, "episode")];
  // Half a cursor cannot make an S/E marker: sxxeyy returns null unless
  // both halves are set, and an invented "S02E00" would be a position
  // nothing airs at.
  const cursor = sxxeyy(parseOptionalInt(row.start_season), parseOptionalInt(row.start_episode));
  if (cursor) parts.push(`from ${cursor}`);
  return { kind: "named", text: parts.join(" · ") };
}

/** One line in the rail's channel group. `self` marks the block being
 * edited so it can be picked out of the field at a glance; the text says
 * so as well, because weight and ink are not facts (SC 1.4.1). */
export interface RailSibling {
  text: string;
  self: boolean;
}

/** The edited block as the rail sees it: the FORM's values, never the
 * stored record's. Changing the channel or the priority is exactly the
 * edit whose consequence the rail exists to show, so reading the saved
 * record here would answer the question the operator just stopped
 * asking. */
export interface RailSelf {
  name: string;
  channelId: string;
  priority: number;
  enabled: boolean;
  /** The stored record this form is editing; null while creating. */
  editingId: string | null;
}

/** The shape a BlockRecord already satisfies, same convention as
 * runtime/rank.ts's RankPeer -- the call site passes its list untouched. */
interface RailPeer {
  id: string;
  name: string;
  enabled: boolean;
  spec: { channel_id: string; priority?: number };
}

/**
 * Who this block contends with on its channel, highest priority first.
 *
 * The stored record for the block being edited is dropped and the form's
 * own values stand in its place. Otherwise a priority the operator typed
 * a second ago would be missing from the very list that shows what it
 * does, while the superseded one sat there looking current.
 *
 * A dark peer stays in the field, matching runtime/rank.ts and the guide's
 * inspector exactly: a block sitting dark is defined and it comes back, so
 * a consequence that quietly dropped it would be wrong the morning it
 * wakes. Only `enabled` removes a block from the field -- including the
 * one being edited.
 *
 * A blank channel returns nothing at all. A lone "this block" entry with
 * no channel chosen reads as "nothing else contends", which is a
 * different claim from "you have not said where this goes yet".
 */
export function railSiblings(self: RailSelf, blocks: readonly RailPeer[]): RailSibling[] {
  const channel = self.channelId.trim();
  if (channel === "") return [];
  const field: (RailSibling & { priority: number; name: string })[] = blocks
    .filter((b) => b.id !== self.editingId && b.enabled && b.spec.channel_id === channel)
    .map((b) => ({ name: b.name, priority: b.spec.priority ?? 0, self: false, text: "" }));
  if (self.enabled) {
    // An unnamed block in create mode is still in the field; it just has
    // nothing to be called yet, so it names its role instead of carrying
    // a "(this block)" suffix on the words "This block".
    const label = self.name.trim() === "" ? "This block" : `${self.name.trim()} (this block)`;
    field.push({ name: label, priority: self.priority, self: true, text: "" });
  }
  // Name is the tiebreak so the list cannot reshuffle between two renders
  // of the same data -- a field that reorders under the operator's eye
  // reads as a change they made.
  field.sort((a, b) => b.priority - a.priority || a.name.localeCompare(b.name));
  return field.map((e) => ({ text: `PRI ${e.priority} · ${e.name}`, self: e.self }));
}

// ---- the cron footgun ----------------------------------------------------
//
// A series block's cron is not just when it airs -- it is how often the
// cursor advances, so once a show has aired under one expression, saving a
// different one changes which episode lands on which date. The engine says
// so directly: initializeSeriesState (internal/scheduler/engine.go) applies
// a row's start_season/start_episode ONLY while last_aired is nil. After
// that the stored cursor is the only thing deciding what plays, and the
// block's own start position is ignored for good.
//
// So the confirm has exactly one job: say that out loud before the write,
// and offer the one repair -- putting each cursor back where the block says
// it starts.

/** A show this block seeds whose cursor the engine has already committed. */
export interface FootgunShow {
  title: string;
  /** Where the cursor stands right now, "S02E05". */
  cursor: string;
  /** Where a rewind would put it back: this block's own start position,
   * defaulted to S01E01 exactly as initializeSeriesState seeds it. */
  season: number;
  episode: number;
}

/** The series-state rows the predicate reads. Structural, same convention
 * as runtime/rank.ts's RankPeer -- the contract's SeriesState[] passes
 * through untouched. */
export interface CursorRow {
  show_title: string;
  current_season: number;
  current_episode: number;
  last_aired?: string | null;
}

/** The editor form as the predicate sees it -- the values about to be
 * saved, never the stored record's. */
interface FootgunForm {
  type: "filter" | "series";
  cron: string;
  series: readonly { show_title: string; start_season: string; start_episode: string }[];
}

/**
 * Which of this block's shows a cron edit would re-align, or an empty list
 * when the confirm has nothing to say.
 *
 * Four ways to answer "nothing", and each is a real case rather than a
 * defensive guard:
 *
 *   - A filter block has no cursor to move. It never fires this.
 *   - A create (`storedCron === null`) has nothing that aired under it.
 *   - An unchanged cron changes nothing, whitespace included: re-saving
 *     the same expression with a new duration must not raise an episode
 *     warning that has no episode consequence.
 *   - A show whose `last_aired` is unset is still seeded from the block's
 *     own start position, so its next airing is the one this form already
 *     describes.
 *
 * A show is reported once however many rows name it: the rewind writes
 * per SHOW, and two writes to one title would report two outcomes for one
 * cursor. The first row wins, matching the engine's own order of play.
 */
export function cronFootgunShows(
  form: FootgunForm,
  storedCron: string | null,
  states: readonly CursorRow[],
): FootgunShow[] {
  if (form.type !== "series") return [];
  if (storedCron === null) return [];
  if (form.cron.trim() === storedCron.trim()) return [];
  const committed = new Map<string, CursorRow>();
  for (const s of states) {
    // The one signal, and the same one the engine reads. run_count is not
    // it: a show can complete a run and be restarted back to S01E01, at
    // which point its cursor is exactly where the block would put it.
    if (typeof s.last_aired === "string" && s.last_aired !== "") committed.set(s.show_title, s);
  }
  const shows: FootgunShow[] = [];
  const seen = new Set<string>();
  for (const row of form.series) {
    const title = row.show_title.trim();
    if (title === "" || seen.has(title)) continue;
    const state = committed.get(title);
    if (!state) continue;
    seen.add(title);
    shows.push({
      title,
      // Both halves are required on the wire (components.SeriesState), so
      // sxxeyy always answers here; the fallback exists only because its
      // signature admits undefined.
      cursor: sxxeyy(state.current_season, state.current_episode) ?? "",
      season: parseOptionalInt(row.start_season) ?? 1,
      episode: parseOptionalInt(row.start_episode) ?? 1,
    });
  }
  return shows;
}

/** The two sentences the confirm shows. Composed here rather than in the
 * template because both count and cursor are facts the operator decides
 * on, and a template expression is the one place they cannot be pinned. */
export interface FootgunCopy {
  consequence: string;
  rewind: string;
}

/**
 * What the confirm says.
 *
 * The consequence sentence names the block and how many shows move,
 * because "this changes episode order" without a number is a warning the
 * operator cannot size. The rewind sentence gives each show BOTH readings
 * -- where it is and where rewind sends it -- since that difference is the
 * entire decision, and it prints them as plain sentences rather than
 * `S02E05 → S01E01`: an arrow is notation this UI never taught anywhere
 * else, and a modal read once is the worst place to introduce one.
 */
export function footgunCopy(blockName: string, shows: readonly FootgunShow[]): FootgunCopy {
  const name = blockName.trim() === "" ? "This block" : blockName.trim();
  const lines = shows.map((s) => {
    const target = sxxeyy(s.season, s.episode) ?? "";
    return `${s.title} is at ${s.cursor} — rewind sets it to ${target}.`;
  });
  return {
    consequence:
      `${name} has already aired. Which episode lands on which date follows from the cron, ` +
      `so saving a new one re-aligns ${plural(shows.length, "show")} from here on.`,
    rewind: lines.join(" "),
  };
}

/** The operator's answer to one cron confirm, and the expression it was
 * given about. */
export interface CronAck {
  /** The trimmed cron the confirm was raised on. */
  cron: string;
  /** What "Save and rewind" queued -- the shows exactly as the dialog
   * read them out; empty for a plain save. */
  rewind: FootgunShow[];
}

/**
 * The cursor rewind an answered confirm still authorises for the cron
 * about to be saved, or null when no answer covers it.
 *
 * Keyed to the expression because the editor OUTLIVES a failed save: a
 * 409 on the name or a 412 on the record leaves the panel open with the
 * answer still in hand. If the operator then puts the old cron back and
 * fixes the name, there is no mid-run change left to warn about -- and a
 * rewind still queued from the abandoned one would move every cursor for
 * a change that never happened, writing series state nobody confirmed.
 * An answer that names its own expression cannot outlive the edit that
 * justified it; a boolean could not tell the two saves apart.
 */
export function ackedRewind(ack: CronAck | null, cron: string): FootgunShow[] | null {
  if (!ack || ack.cron !== cron.trim()) return null;
  return ack.rewind;
}

/** One PATCH /state/series/{show_title} outcome. `error` is the server's
 * own words, or null when the cursor moved. */
export interface RewindOutcome {
  title: string;
  error: string | null;
}

/** The two tape lines a rewind earns: what moved, and what did not. Either
 * is null when its half is empty. */
export interface RewindReport {
  moved: string | null;
  failed: string | null;
}

/**
 * Reports a rewind per TITLE, because that is how it was written.
 *
 * The confirm is per block and the rewind is per show, so a block seeding
 * three shows is three independent writes over three round trips. One of
 * them failing says nothing about the other two, and a single "Cursors
 * rewound" over a partial failure is exactly the silent half-success this
 * page refuses elsewhere -- the operator would go on believing a cursor
 * moved that is still sitting where it was.
 */
export function rewindReport(outcomes: readonly RewindOutcome[]): RewindReport {
  const moved: string[] = [];
  const failed: string[] = [];
  for (const o of outcomes) {
    if (o.error === null) moved.push(o.title);
    else failed.push(`${o.title}: ${o.error}`);
  }
  return {
    // "Cursor"/"Cursors", the noun alone, where runtime/format.ts's
    // plural() would put the count in front of it -- the count is
    // already in the list of titles that follows.
    moved: moved.length === 0 ? null : `Cursor${moved.length === 1 ? "" : "s"} rewound — ${moved.join(", ")}`,
    failed: failed.length === 0 ? null : `Cursor${failed.length === 1 ? "" : "s"} unchanged — ${failed.join("; ")}`,
  };
}

// ---- live link: what an external change does to an open editor -----------
//
// plan.invalidated means "something the schedule is derived from changed,
// go look" -- on this page, that the list is stale. Reading it back is
// held until the editor closes, because the list is not just what the
// operator sees: it is where the next save's If-Match comes from (submit()
// reads updated_at off this.blocks). A list that is a few seconds behind
// is recoverable; a rebased If-Match is a silent lost update.

/** The editor state the live-link decision reads. */
export interface EditorSnapshot {
  open: boolean;
  /** The block being edited; null in create mode or with the panel shut. */
  editingId: string | null;
  /** The block THIS page changed and then opened the editor on -- the
   * duplicate's own copy. Its plan.invalidated frame is this page's own
   * echo coming back, not somebody else's edit. Optional because absent
   * says exactly what null says: no frame in flight is this page's. */
  selfChangedId?: string | null;
}

export interface LiveReaction {
  /** Read the list now. */
  refetchNow: boolean;
  /** A read still owed, to be drained when the editor closes. */
  queued: boolean;
  /** Raise the "changed elsewhere" note on the open editor. */
  note: boolean;
}

/**
 * Reads the block id out of a plan.invalidated frame, or null when the
 * frame names something else. The payload is {reason, id} where reason is
 * "block" or "series" (internal/api/events.go) and a series id is a show
 * TITLE -- matching that against editingId would raise the note on an
 * unrelated block whose store id happened to collide with a show name.
 *
 * A malformed frame returns null rather than throwing: the bus hands
 * handlers `unknown` precisely so a payload change fails in the page's
 * own narrowing instead of taking the stream's pump loop down with it.
 */
export function changedBlockId(data: unknown): string | null {
  if (typeof data !== "object" || data === null) return null;
  const frame = data as { reason?: unknown; id?: unknown };
  if (frame.reason !== "block" || typeof frame.id !== "string") return null;
  return frame.id;
}

/**
 * What one plan.invalidated frame does to the list.
 *
 * An OPEN editor freezes the list -- dirty or clean, edit or create. A
 * clean editor looks harmless, and is the whole reason this fix exists:
 * submit() sends `If-Match: <updated_at>` read out of this.blocks, so a
 * refetch under an open panel quietly re-arms that header with the
 * updated_at of a record the operator has never seen. The next save then
 * SUCCEEDS and full-replaces the other tab's spec with the pre-change one
 * still on this screen -- exactly the lost update If-Match was added to
 * stop. Nothing is refetched under an open panel; what the operator holds
 * and the header they will send stay the same record.
 *
 * `queued` is a flag and deliberately not a count: however many changes
 * land while the editor is open, they are all answered by ONE read of the
 * list when it closes, so draining is a single fetch rather than a burst
 * of identical ones.
 *
 * The note fires whether or not the editor is dirty. Even a clean editor
 * is holding the updated_at its next save will send as If-Match, and the
 * freeze above keeps it holding it -- so once the record moves, that save
 * is genuinely doomed to a 412, and saying so now is cheaper than saying
 * so after the operator finishes typing.
 *
 * It does NOT fire on this page's own echo. Duplicate leaves the editor
 * open on the copy it just made, and the write that made it raises a
 * frame naming that very id: the note would tell the operator their
 * block "changed elsewhere" about a change they made here, one click
 * ago, holding the freshest record there is. The note is for an EXTERNAL
 * change; the freeze and the queued read still apply to both, since the
 * REST of the list is stale either way.
 */
export function planInvalidatedReaction(editor: EditorSnapshot, changedId: string | null): LiveReaction {
  const hold = editor.open;
  const mine = changedId !== null && changedId === editor.selfChangedId;
  return {
    refetchNow: !hold,
    queued: hold,
    note: editor.open && changedId !== null && changedId === editor.editingId && !mine,
  };
}

/** Raised on the open editor when its own block moved somewhere else.
 * Worded as an instruction, not an alarm: nothing typed is lost, the edit
 * just needs the current record under it before it can be saved. */
const STALE_BLOCK_NOTE = "This block changed elsewhere — reload the list, then save to re-apply your edit.";

interface RailData {
  starts: string[];
  expr: string;
  loading: boolean;
  /** The server's own reason for refusing the expression. Rendered inline
   * rather than left as an empty rail: an empty readout on a rejected
   * cron is a silent failure (PRODUCT.md, principle 3). */
  error: string | null;
}

function emptyRail(): RailData {
  return { starts: [], expr: "", loading: false, error: null };
}

interface EditorState {
  open: boolean;
  mode: "create" | "edit";
  editingId: string | null;
  submitting: boolean;
  error: string | null;
  nameConflict: string | null;
  seriesRowErrors: (string | null)[];
  // Which series rows are expanded. A parallel per-row array kept in
  // lockstep with form.series, exactly as seriesRowErrors already is --
  // the alternative, native <details> state living in the DOM, follows
  // the row's POSITION through a reorder instead of the row itself, so
  // moving a show up would hand its open panel to whatever took its
  // place.
  seriesOpen: boolean[];
  // What the consequence rail is holding. `starts` are the SERVER's
  // instants, kept raw: the end times are composed at render time from
  // form.duration, so editing the duration re-reads the rail without
  // asking the server anything. `expr` is the expression `starts`
  // answers for -- a debounced field can still land two requests, and
  // the older answer must not overwrite the newer one.
  rail: RailData;
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
  // Set only by Duplicate, which opens this editor on a record this page
  // itself just wrote: the plan.invalidated frame that write raises is
  // this page's own echo, and the "changed elsewhere" note must not fire
  // on it (planInvalidatedReaction). Cleared with the editor, and
  // consumed by the first frame that names it -- a LATER change to the
  // same block, by somebody else, is external and does raise the note.
  selfChangedId: string | null;
}

interface BlocksState {
  blocksLoading: boolean;
  // The one error surface for the list section (spec §4: the page's two
  // competing error surfaces collapse into a single .problem) -- both a
  // failed load AND a failed row action (toggle/delete) land here, with
  // "reload the list" as the shared recovery.
  blocksProblem: ProblemView | null;
  blocks: BlockRecord[];

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

  // serverNow() as of the last minute tick -- what every relative reading
  // in the list is measured against. A field rather than a call inside
  // each reading because Alpine only redraws a row when reactive state it
  // read has changed: a NEXT column computed from a bare serverNow() would
  // be frozen at whatever the clock said when the list last loaded.
  now: number;

  pendingId: string | null;
  // The block the shared confirm dialog (partials/ui/confirm.html) is
  // armed on -- name included so the dialog can say what it deletes.
  confirmDelete: { id: string; name: string } | null;

  // The copy being named. Set on the first click and kept while the
  // name dialog is up, so a 409 is answered by editing the name and
  // sending again rather than by starting over.
  duplicating: { id: string; name: string; error: string | null } | null;
  // The block a dark-window write is armed on -- the dialog's target
  // while it is choosing a wake-up, and bringBack()'s target for the one
  // PATCH that clears one. Nothing here records whether the block is
  // dark: darkUntilLabel answers that, at the row.
  darkTarget: { id: string; name: string } | null;

  // Every tracked show's cursor, read once per editor open (openEdit;
  // create can never raise the cron confirm, so it never asks). The one
  // input to cronFootgunShows besides the form itself. A failed read
  // leaves it empty, which degrades to the pre-v0.5.10 save -- see
  // loadSeriesStates.
  seriesStates: SeriesState[];
  // The shows a cron edit is about to re-align, while the confirm is up.
  cronConfirm: { shows: FootgunShow[] } | null;
  // The operator's answer, and the expression they answered about.
  // Survives the dialog closing (which clears cronConfirm) so the second
  // pass through submit() writes instead of re-asking, and survives a
  // save that FAILS -- which is exactly why it carries its cron rather
  // than being a boolean (see ackedRewind). Cleared with the editor.
  cronAck: CronAck | null;

  // A read of the list the live link owes but is holding back, because
  // plan.invalidated (or a tab resume) landed while the editor panel was
  // open. A flag, not a count (see planInvalidatedReaction); drained by
  // closeEditor.
  refetchQueued: boolean;

  editor: EditorState;
  scheduleDayOptions: ScheduleDayOption[];

  init(): void;
  openLinkedEditor(): void;
  loadBlocks(): Promise<void>;
  loadChannels(): Promise<void>;
  loadMedia(): Promise<void>;
  noteStale(message: string): void;

  cronReadback(raw: string): string | null;
  durationLabel(minutes: number): string;
  railState(): RailState;
  railNext(): string[];
  railChannel(): RailSibling[];
  seriesSummary(row: SeriesRowForm): SeriesRowSummary;
  loadRailOccurrences(): Promise<void>;
  toggleSeriesRow(index: number): void;
  next(block: BlockRecord): NextReading;
  darkUntil(block: BlockRecord): string | null;
  priorityCell(block: BlockRecord): string;
  channelLabel(c: Channel): string;
  plate(id: string): PlateParts;
  channelSelectOptions(): Channel[];
  channelHint(): string;
  seriesTitleWarning(title: string): string | null;

  setScheduleMode(mode: "simple" | "cron"): void;
  onFrequencyChange(): void;
  updateCronFromSimple(): void;
  toggleScheduleDay(day: number): void;
  validateSchedule(): boolean;

  openCreate(): void;
  openEdit(block: BlockRecord, self?: boolean): void;
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
  requestDelete(block: BlockRecord): void;
  cancelDelete(force?: boolean): void;
  performDelete(): Promise<void>;

  canArm(): boolean;
  requestDuplicate(block: BlockRecord): void;
  cancelDuplicate(force?: boolean): void;
  performDuplicate(): Promise<void>;

  loadSeriesStates(): Promise<void>;
  armCronConfirm(): boolean;
  cronCopy(): FootgunCopy;
  proceedCron(rewind: boolean): void;
  cancelCron(): void;
  rewindCursors(shows: readonly FootgunShow[]): Promise<void>;

  darkPresets: DarkPresetOption[];
  darkPresetWhen(preset: DarkPreset): string;
  darkBody(): string;
  requestDark(block: BlockRecord): void;
  bringBack(block: BlockRecord): void;
  cancelDark(force?: boolean): void;
  applyDark(preset: DarkPreset | null): Promise<void>;
}

// Alpine binds a handful of "magic" helpers (https://alpinejs.dev/magics)
// onto the component instance at runtime, on top of whatever Alpine.data()'s
// factory returns -- $nextTick (focusEditorSoon() et al.) and $refs (the
// shared confirm <dialog>). ThisType<...> (erased at compile time) tells
// TypeScript to type `this` inside the object literal's methods as
// BlocksState-plus-magics, without requiring the literal itself to supply
// them -- they don't exist until Alpine injects them.
interface WithMagics {
  $nextTick(callback: () => void): void;
  // One ref per dialog, four in all: ui/confirm.html hard-codes
  // x-ref="confirmDialog", so the three dialogs this page adds (the
  // duplicate's name, the dark window, the cron confirm) each carry a ref
  // of their own. A second element claiming "confirmDialog" would win the
  // ref and leave Delete arming a dialog it no longer points at.
  $refs: {
    confirmDialog: HTMLDialogElement;
    duplicateDialog: HTMLDialogElement;
    darkDialog: HTMLDialogElement;
    cronDialog: HTMLDialogElement;
  };
}

document.addEventListener("alpine:init", () => {
  Alpine.data(
    "blocks",
    (): BlocksState & ThisType<BlocksState & WithMagics> => ({
      blocksLoading: true,
      blocksProblem: null,
      blocks: [],

      channelsLoading: true,
      channelsError: null,
      channels: [],

      mediaShows: [],
      mediaGenres: [],
      mediaRatings: [],
      mediaOk: false,

      now: serverNow(),
      pendingId: null,
      confirmDelete: null,
      duplicating: null,
      darkTarget: null,
      darkPresets: DARK_PRESETS,
      seriesStates: [],
      cronConfirm: null,
      cronAck: null,
      refetchQueued: false,

      editor: {
        open: false,
        mode: "create",
        editingId: null,
        submitting: false,
        error: null,
        nameConflict: null,
        seriesRowErrors: [],
        seriesOpen: [],
        rail: emptyRail(),
        scheduleDaysError: null,
        form: emptyEditorForm(),
        returnFocusId: "new-block-btn",
        selfChangedId: null,
      },
      scheduleDayOptions: SCHEDULE_DAY_OPTIONS,

      // Alpine calls this once per component instance and the page has
      // exactly one x-data="blocks", so there is no module-level one-shot
      // guard here: the subscriptions below are wired from init and live
      // with the component, and initShell() keeps its own idempotence
      // guard so no second stream can be opened either way.
      init() {
        void this.loadBlocks().then(() => this.openLinkedEditor());
        void this.loadChannels();
        // Arming a new token re-fires whichever loads failed (the token
        // panel's probe broadcast, runtime/api.ts's onReauth).
        //
        // Through the same freeze as the stream and the resume: this is
        // the third read the operator did not ask for, and it rebases
        // If-Match exactly as silently as the other two would. Reachable
        // whenever a load fails while a panel is already open -- the row
        // actions are not disabled behind it -- so the guard is not
        // theoretical. The channel list carries no If-Match and is not
        // held.
        onReauth(() => {
          if (this.blocksProblem) {
            if (this.editor.open) this.refetchQueued = true;
            else void this.loadBlocks();
          }
          if (this.channelsError) void this.loadChannels();
        });

        // The live link. Never unsubscribed: an Alpine page component
        // lives exactly as long as the document does.
        subscribe("plan.invalidated", (data) => {
          const changedId = changedBlockId(data);
          const reaction = planInvalidatedReaction(
            {
              open: this.editor.open,
              editingId: this.editor.editingId,
              selfChangedId: this.editor.selfChangedId,
            },
            changedId,
          );
          // Once, not forever: this page's own write echoes back exactly
          // one frame, and the NEXT change to the same block is somebody
          // else's -- which is a note the operator does need.
          if (changedId !== null && changedId === this.editor.selfChangedId) {
            this.editor.selfChangedId = null;
          }
          // Set before the fetch, which clears it again on the way in:
          // the reverse order would leave a phantom read owed forever.
          this.refetchQueued = reaction.queued;
          if (reaction.note) this.noteStale(STALE_BLOCK_NOTE);
          if (reaction.refetchNow) void this.loadBlocks();
        });

        // A hidden tab streams nothing, so it has missed every frame
        // since it was hidden and the resume replay only reaches back as
        // far as the hub's ring -- refetching is the only way it can
        // trust what it shows. Routed through the same freeze as the
        // stream: a resume that refetched under an open panel would
        // rebase If-Match just as silently as a frame would.
        onResume(() => {
          if (this.editor.open) {
            this.refetchQueued = true;
            return;
          }
          void this.loadBlocks();
        });

        // The minute tick behind the NEXT column. A relative reading ages
        // between refetches, and the list is otherwise only redrawn when
        // the live link says something moved -- an "in 5 min" left sitting
        // for an hour is precisely the reading an instrument must never
        // show. A local timer on purpose, same as the guide's sweep: it
        // has to keep turning on POLL and on LINK LOST, so it can never
        // ride a stream frame. serverNow(), so the step lands on the
        // SERVER's minute rather than this laptop's.
        window.setInterval(() => {
          this.now = serverNow();
        }, 60_000);
      },

      // /blocks/?edit=<id> deep-link (the guide inspector's EDIT BLOCK
      // path): once the list has loaded, open the editor on the linked
      // block. A stale link (block deleted since the guide rendered)
      // prints an honest tape line instead of failing silently.
      openLinkedEditor() {
        const editId = new URLSearchParams(window.location.search).get("edit");
        if (!editId) return;
        const target = this.blocks.find((b) => b.id === editId);
        if (target) this.openEdit(target);
        else if (!this.blocksProblem) printTape("Linked block not found — it may have been deleted");
      },

      async loadBlocks() {
        this.blocksLoading = true;
        this.blocksProblem = null;
        // Any read of the list settles every read owed, wherever it came
        // from -- the live link's queue, onReauth, or the first load.
        this.refetchQueued = false;
        try {
          this.blocks = await apiGet<BlockRecord[]>(apiPath("/blocks"));
        } catch (err) {
          this.blocksProblem = toProblemView(err);
        } finally {
          this.blocksLoading = false;
        }
      },

      async loadChannels() {
        this.channelsLoading = true;
        this.channelsError = null;
        try {
          this.channels = await loadChannels();
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
          apiGet<MediaShow[]>(apiPath("/media/shows")),
          apiGet<MediaMeta>(apiPath("/media/meta")),
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

      // Raises the "this block moved under you" note on the open editor
      // and offers the one action that resolves it. Deliberately does NOT
      // touch editor.form: reloading the list refreshes the record the
      // next save's If-Match reads, while what the operator typed stays
      // exactly where they left it. The note itself rides editor.error,
      // the panel's one inline status line; the action rides the tape,
      // which is where this page's actionable lines already live.
      noteStale(message) {
        // Once per note, not once per frame: a burst of changes to the
        // same block would otherwise fill all three tape slots with the
        // same sentence and push every other line off the tape.
        if (this.editor.error === message) return;
        this.editor.error = message;
        printTape(message, {
          label: "Reload blocks",
          run: () => {
            void this.loadBlocks();
          },
        });
      },

      cronReadback,
      durationLabel,

      // The three row readings, each measured against `now` (the minute
      // tick above), never Date.now().
      next(block) {
        return nextReading(block, this.now);
      },

      darkUntil(block) {
        return darkUntilLabel(block.disabled_until, this.now);
      },

      // The same-channel list, passed WHOLE: runtime/rank.ts drops the
      // disabled peers itself, and the guide's inspector hands it its list
      // the same way. Filtering enabled here as well would be harmless
      // today and is exactly how the two surfaces start disagreeing about
      // what rank a block holds.
      priorityCell(block) {
        const peers = this.blocks.filter((b) => b.spec.channel_id === block.spec.channel_id);
        return priorityLabel(block, peers, this.now);
      },

      // ---- the consequence rail ----------------------------------------

      railState() {
        return railState(this.editor.rail, this.editor.form.cron);
      },

      // Composed at render time rather than stored, so editing the
      // duration field re-reads the ends without asking the server for
      // starts it already has.
      railNext() {
        const minutes = parseRequiredInt(this.editor.form.duration);
        return this.editor.rail.starts.map((start) => occurrenceLabel(start, minutes));
      },

      // The whole list, unfiltered: railSiblings drops the record this
      // form supersedes and the peers that are not contending, and it is
      // the only place on this page that decides either.
      railChannel() {
        return railSiblings(
          {
            name: this.editor.form.name,
            channelId: this.editor.form.channel_id,
            priority: parseOptionalInt(this.editor.form.priority) ?? 0,
            enabled: this.editor.form.enabled,
            editingId: this.editor.editingId,
          },
          this.blocks,
        );
      },

      seriesSummary: seriesRowSummary,

      /**
       * Asks the server when this expression next comes round.
       *
       * Debounced by Alpine's own `.debounce` modifier on the schedule
       * field's bubbled `input` (list.html), not by a timer here: every
       * control that can change the cron lives inside that one field, so
       * one listener covers the raw text box and all five picker
       * controls, and the vendored dependency already owns the timer.
       *
       * The debounce reads form.cron when it FIRES, so every picker
       * control has to have written by then. They do: each writes on the
       * event Alpine's own x-model commits on (`input` for the typed
       * fields, `change` for the <select> and the checkboxes), which is
       * always the same interaction that armed the timer. A number field
       * left on `change` was the exception and the bug -- it committed on
       * blur, long after the debounce had already asked about the
       * superseded expression, and raised no input event to re-arm with.
       *
       * A blank expression asks nothing -- there is no question yet, and
       * a 400 for an empty field tells the operator only that they have
       * not finished typing.
       */
      async loadRailOccurrences() {
        const expr = this.editor.form.cron.trim();
        this.editor.rail.expr = expr;
        this.editor.rail.error = null;
        if (expr === "") {
          this.editor.rail.starts = [];
          this.editor.rail.loading = false;
          return;
        }
        this.editor.rail.loading = true;
        try {
          const res = await apiGet<CronOccurrences>(
            apiPath("/cron/next", undefined, { expr, count: RAIL_OCCURRENCES }),
          );
          // A newer edit already asked a different question; its answer
          // is the one the operator is waiting for. Debouncing makes the
          // overlap rare, not impossible, and an out-of-order answer
          // here would put the previous expression's instants under the
          // current one -- wrong in the one way nothing on screen would
          // show.
          if (this.editor.rail.expr !== expr) return;
          this.editor.rail.starts = res.occurrences;
        } catch (err) {
          if (this.editor.rail.expr !== expr) return;
          // The server's own words: a 400 here carries which field of
          // the expression it choked on, which is more than this client
          // could say and exactly what the operator needs.
          this.editor.rail.starts = [];
          this.editor.rail.error = describeError(err);
        } finally {
          if (this.editor.rail.expr === expr) this.editor.rail.loading = false;
        }
      },

      toggleSeriesRow(index) {
        this.editor.seriesOpen[index] = !this.editor.seriesOpen[index];
      },

      channelLabel,

      plate(id) {
        return channelPlate(id, this.channels);
      },

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

      // Called from every Simple-mode picker control (list.html) as it
      // commits -- `input` for the typed fields, `change` for the
      // <select> and the checkboxes, matching what x-model binds for each
      // -- so editor.form.cron (the value submit() actually reads, and
      // the one railState measures the rail's instants against) is in
      // step with the picker before the rail's debounce fires, never one
      // blur behind it.
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

      // Shared hint logic (runtime/channels.ts's channelHint) -- see its
      // doc comment for the select-vs-free-text gating rationale.
      channelHint() {
        return channelHintText(this.channelsLoading, this.channelsError, this.channels);
      },

      openCreate() {
        this.editor.mode = "create";
        this.editor.editingId = null;
        this.editor.error = null;
        this.editor.nameConflict = null;
        this.editor.form = emptyEditorForm();
        this.editor.seriesRowErrors = [];
        this.editor.seriesOpen = [];
        this.editor.scheduleDaysError = null;
        this.editor.rail = emptyRail();
        this.editor.selfChangedId = null;
        this.cronAck = null;
        this.editor.returnFocusId = "new-block-btn";
        this.editor.open = true;
        this.focusEditorSoon();
        void this.loadMedia();
        // A new block starts on the picker's default cron, so the rail
        // has a real question to ask before the operator types anything.
        void this.loadRailOccurrences();
      },

      // `self` marks an edit opened on a record this page itself just
      // wrote (Duplicate, and only Duplicate): the frame that write
      // raises is this page's own echo, not an external change. Every
      // other caller is opening the editor on a record somebody else may
      // move at any moment, which is precisely when the note is right.
      openEdit(block, self = false) {
        this.editor.mode = "edit";
        this.editor.editingId = block.id;
        this.editor.error = null;
        this.editor.nameConflict = null;
        this.editor.form = formFromSpec(block.spec, block.enabled);
        this.editor.seriesRowErrors = this.editor.form.series.map(() => null);
        // Every row starts closed: a dozen expanded shows is the state
        // this disclosure exists to end, and each row's summary line is
        // what makes a closed one findable again.
        this.editor.seriesOpen = this.editor.form.series.map(() => false);
        this.editor.rail = emptyRail();
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
        this.editor.selfChangedId = self ? block.id : null;
        this.cronAck = null;
        this.editor.returnFocusId = `block-edit-${block.id}`;
        this.editor.open = true;
        this.focusEditorSoon();
        void this.loadMedia();
        void this.loadRailOccurrences();
        // Read once per edit, not per save: by the time an operator has
        // typed a new cron and reached Save, this has long landed, and
        // asking at submit time would put an await in front of the write
        // before editor.submitting is set -- a second click's way in.
        void this.loadSeriesStates();
      },

      closeEditor() {
        this.editor.open = false;
        this.editor.mode = "create";
        this.editor.editingId = null;
        this.editor.error = null;
        this.editor.nameConflict = null;
        this.editor.seriesRowErrors = [];
        this.editor.seriesOpen = [];
        this.editor.rail = emptyRail();
        this.editor.scheduleDaysError = null;
        this.editor.form = emptyEditorForm();
        this.editor.selfChangedId = null;
        // The cron confirm is answered per save, not per session: the next
        // edit of this block asks again, because it is a different change.
        this.cronAck = null;
        this.focusReturnSoon();
        // Drain the live link's held read, if one is owed. However many
        // changes landed while the panel was open, they cost exactly one
        // fetch now that no If-Match is armed against the old record.
        if (this.refetchQueued) void this.loadBlocks();
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
          this.editor.seriesOpen.push(true);
        }
      },

      // A row the operator just asked for opens: it is empty, and the
      // summary of an empty row is only the instruction to fill it in.
      addSeriesRow() {
        this.editor.form.series.push(emptySeriesRow());
        this.editor.seriesRowErrors.push(null);
        this.editor.seriesOpen.push(true);
      },

      removeSeriesRow(index) {
        this.editor.form.series.splice(index, 1);
        this.editor.seriesRowErrors.splice(index, 1);
        this.editor.seriesOpen.splice(index, 1);
      },

      // Keeps editor.seriesRowErrors and editor.seriesOpen aligned to the
      // same rows as editor.form.series -- addSeriesRow/removeSeriesRow
      // already keep all three in lockstep, a reorder must too, so an
      // inline skip-episode error and an expanded panel both stay attached
      // to the row they belong to rather than to the position.
      moveSeriesRowUp(index) {
        swapAdjacent(this.editor.form.series, index, -1);
        swapAdjacent(this.editor.seriesRowErrors, index, -1);
        swapAdjacent(this.editor.seriesOpen, index, -1);
      },

      moveSeriesRowDown(index) {
        swapAdjacent(this.editor.form.series, index, 1);
        swapAdjacent(this.editor.seriesRowErrors, index, 1);
        swapAdjacent(this.editor.seriesOpen, index, 1);
      },

      // Opens every row it rejects. An inline error under a field inside
      // a collapsed row is an error nobody can read, which is the one way
      // this disclosure could make the editor worse than it was.
      validateSeriesRows() {
        let ok = true;
        this.editor.seriesRowErrors = this.editor.form.series.map((row, index) => {
          const { error } = parseSkipEpisodes(row.skip_episodes);
          if (error) {
            ok = false;
            this.editor.seriesOpen[index] = true;
          }
          return error ?? null;
        });
        return ok;
      },

      async submit() {
        // State-level re-entrancy guard, same convention as toggleEnabled/
        // performDelete's pendingId checks: the view layer's
        // :disabled="editor.submitting" is a browser-enforced UI
        // convention, not a guarantee. Two concurrent identical creates
        // would be collapsed to one POST by apiSend's in-flight guard, but
        // BOTH callers would then append the same record to this.blocks --
        // a duplicate x-for key -- so the second entry must stop here.
        if (this.editor.submitting) return;
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

        // The cron footgun (v0.5.10). After validation, so a form that
        // would be refused anyway never raises it; before the write, so
        // the operator's answer is what decides whether the cursors move.
        // Returns true once it has taken the save over -- proceedCron()
        // re-enters submit() with the answer recorded against this cron.
        if (this.armCronConfirm()) return;

        this.editor.submitting = true;
        try {
          if (this.editor.mode === "create") {
            const rec = await apiSend<BlockRecord>("POST", apiPath("/blocks"), body);
            this.blocks = [...this.blocks, rec];
          } else {
            const id = this.editor.editingId;
            if (!id) throw new Error("editor is in edit mode with no editingId set");
            // If-Match carries the updated_at this editor loaded, so a
            // save is refused with 412 rather than discarding an edit
            // another tab made in the meantime -- routine now that both
            // tabs watch each other's changes land over the live link.
            const loaded = this.blocks.find((b) => b.id === id);
            const rec = await apiSend<BlockRecord>(
              "PUT",
              apiPath("/blocks/{id}", { id }),
              body,
              undefined,
              loaded ? { "If-Match": loaded.updated_at } : undefined,
            );
            this.blocks = this.blocks.map((b) => (b.id === id ? rec : b));
          }
          // Read before closeEditor, which clears it with the rest of
          // the editor's state -- and read against the cron actually
          // saved, so an answer given about an expression this save no
          // longer carries authorises nothing (ackedRewind).
          const rewind = ackedRewind(this.cronAck, spec.cron) ?? [];
          this.closeEditor();
          // Success is printed, not toasted. The one action is the
          // create→apply bridge (spec §2 step 4): PREVIEW ON GUIDE opens
          // the guide in draft mode scoped to this block's channel, diffed
          // against the reading the operator last saw. The "reaches
          // Tunarr at <next_cron_tick>" readout arrives with the block
          // power tools slice.
          printTape(`Block saved — ${spec.name}`, {
            label: "Preview on guide",
            run: () => {
              window.location.href = draftHref(spec.channel_id);
            },
          });
          // Only once the block itself is saved: a cursor rewound for a
          // spec that then 409'd would be a repair to a change nobody
          // made. Awaited rather than fired off, so a failure lands on
          // the tape under the save it belongs to rather than minutes
          // later next to something else.
          if (rewind.length > 0) await this.rewindCursors(rewind);
        } catch (err) {
          if (err instanceof ApiError && err.status === 409) {
            // describeError, not err.detail alone: the store's own
            // ErrConflict text is a terse "conflict" (internal/store/
            // blocks.go), so err.title ("block name already exists")
            // carries the actual meaning -- dropping it would show the
            // operator a fairly unhelpful single word.
            this.editor.nameConflict = describeError(err);
          } else if (err instanceof ApiError && err.status === 412) {
            // The If-Match guard fired: another tab saved this block
            // first. The server's own detail already reads "Reload and
            // re-apply your edit" (internal/api/blocks.go), so it is
            // repeated verbatim rather than paraphrased into something
            // that could drift out of step with it. What the server
            // cannot do from its side is offer the reload -- noteStale
            // attaches that, and nothing here retries silently, which
            // would be precisely the lost update the header prevents.
            this.noteStale(describeError(err));
          } else {
            this.editor.error = describeError(err);
          }
        } finally {
          this.editor.submitting = false;
        }
      },

      // Field-scoped PATCH (contract rule 2): the body carries `enabled`
      // and nothing else, so the toggle cannot overwrite a spec edit made
      // elsewhere since this list loaded.
      async toggleEnabled(block) {
        if (this.pendingId) return;
        this.pendingId = block.id;
        this.blocksProblem = null;
        try {
          // PATCH, not PUT: a toggle that resent the whole spec would
          // overwrite a spec edit made elsewhere since this list loaded.
          // A field-scoped write has nothing to clobber, which is also
          // why it needs no If-Match.
          const updated = await apiSend<BlockRecord>(
            "PATCH",
            apiPath("/blocks/{id}", { id: block.id }),
            { enabled: !block.enabled },
          );
          this.blocks = this.blocks.map((b) => (b.id === block.id ? updated : b));
          printTape(`Block ${updated.enabled ? "enabled" : "disabled"} — ${updated.name}`);
        } catch (err) {
          this.blocksProblem = toProblemView(err);
        } finally {
          this.pendingId = null;
        }
      },

      // Arms the shared confirm dialog (partials/ui/confirm.html) on this
      // block. Native <dialog> handles focus: showModal() traps it inside,
      // close() returns it to the row's Delete button -- the old inline
      // row-swap confirm (and its hand-rolled focus management) is gone,
      // one confirm idiom for the whole app.
      requestDelete(block) {
        // pendingId is global (one in-flight row action at a time), so a
        // confirm opened while another row's toggle/delete is in flight
        // would render an enabled Confirm button whose click
        // performDelete() then silently drops -- a "no silent failures"
        // violation. Refuse to arm the dialog at all until the in-flight
        // action settles.
        if (this.pendingId) return;
        this.confirmDelete = { id: block.id, name: block.name };
        this.$refs.confirmDialog.showModal();
      },

      // State-level re-entrancy guard, same convention as the guide's
      // cancelApply: refuses to close while the delete is still in
      // flight unless performDelete itself forces it.
      cancelDelete(force = false) {
        if (this.pendingId && !force) return;
        this.$refs.confirmDialog.close();
        this.confirmDelete = null;
      },

      async performDelete() {
        const target = this.confirmDelete;
        if (!target || this.pendingId) return;
        this.pendingId = target.id;
        this.blocksProblem = null;
        try {
          await apiSend<void>("DELETE", apiPath("/blocks/{id}", { id: target.id }));
          this.blocks = this.blocks.filter((b) => b.id !== target.id);
          if (this.editor.open && this.editor.editingId === target.id) this.closeEditor();
          printTape(`Block deleted — ${target.name}`);
        } catch (err) {
          this.blocksProblem = toProblemView(err);
        } finally {
          this.pendingId = null;
          this.cancelDelete(true);
        }
      },

      // ---- power tools: duplicate, and the dark window ------------------

      // One gate for both new row actions -- canArmRowAction's own comment
      // carries what each of them would break. Bound to :disabled on both
      // buttons AND re-checked at each entry point: the attribute is a
      // browser convention, the check is the guarantee.
      canArm() {
        return canArmRowAction({ pending: this.pendingId !== null, editorOpen: this.editor.open });
      },

      // One click duplicates. The name is pre-filled rather than asked
      // for, because a collision is the only case where the operator has
      // anything to decide -- and that case raises the dialog with the
      // server's own reason on it (performDuplicate below).
      requestDuplicate(block) {
        if (!this.canArm()) return;
        this.duplicating = { id: block.id, name: `Copy of ${block.name}`, error: null };
        void this.performDuplicate();
      },

      async performDuplicate() {
        const target = this.duplicating;
        if (!target || this.pendingId) return;
        const name = target.name.trim();
        if (name === "") {
          // Caught here rather than sent: the contract requires a
          // non-empty name, and a 400 for a field the operator is
          // looking at is a round trip that tells them nothing new.
          target.error = "Name the copy.";
          return;
        }
        this.pendingId = target.id;
        this.blocksProblem = null;
        target.error = null;
        try {
          const rec = await apiSend<BlockRecord>("POST", apiPath("/blocks/{id}/duplicate", { id: target.id }), { name });
          // Into the list before the editor opens, because openEdit arms
          // If-Match off this.blocks -- and against the record the SERVER
          // minted, never a client-side guess at what it created. The copy
          // arrives disabled (decision 3), so the row reads that way the
          // moment it appears.
          this.blocks = [...this.blocks, rec];
          this.cancelDuplicate(true);
          printTape(`Copy created, disabled — ${rec.name}`);
          // Opened as this page's own change: the copy's plan.invalidated
          // frame names this very id, and "changed elsewhere" about a
          // record created here one click ago is a note with nothing
          // behind it.
          this.openEdit(rec, true);
        } catch (err) {
          if (err instanceof ApiError && err.status === 409) {
            // A taken name is a normal outcome, not a failure. The dialog
            // comes up (or stays up) carrying the server's own reason,
            // with the name selected so the next attempt is one retype.
            // Nothing here appends a counter: the server refuses to invent
            // a name for the same reason this does, which is that the
            // operator is the one who can see what the other block is.
            target.error = describeError(err);
            if (!this.$refs.duplicateDialog.open) this.$refs.duplicateDialog.showModal();
            this.$nextTick(() => document.querySelector<HTMLInputElement>("#duplicate-name")?.select());
          } else {
            this.blocksProblem = toProblemView(err);
            this.cancelDuplicate(true);
          }
        } finally {
          this.pendingId = null;
        }
      },

      // Same state-level guard as cancelDelete: the dialog cannot be
      // dismissed out from under a write still in flight unless
      // performDuplicate itself forces it.
      cancelDuplicate(force = false) {
        if (this.pendingId && !force) return;
        if (this.$refs.duplicateDialog.open) this.$refs.duplicateDialog.close();
        this.duplicating = null;
      },

      // The instant a preset commits to, spelled out beside its label.
      // Read from this.now (the minute tick) rather than serverNow()
      // directly: a bare call establishes no reactive dependency, so the
      // previews would freeze at whatever the clock said when the page
      // loaded and quietly go a day stale overnight.
      darkPresetWhen(preset) {
        return formatLocal(darkPresetInstant(preset, this.now));
      },

      // One reading, because the dialog now has one job: choosing a
      // wake-up. Ending a window never reaches it -- the row's own
      // control does that directly (bringBack).
      darkBody() {
        const t = this.darkTarget;
        if (!t) return "";
        return `${t.name} leaves the schedule until the instant you pick, then comes back on its own. Its enabled switch is untouched.`;
      },

      requestDark(block) {
        if (!this.canArm()) return;
        // Snap the clock as the dialog arms. The previews render from
        // this.now, which the minute tick leaves up to 60 s behind --
        // close enough for a countdown sitting in a row, not for a
        // midnight boundary the operator is about to commit to.
        this.now = serverNow();
        this.darkTarget = { id: block.id, name: block.name };
        this.$refs.darkDialog.showModal();
      },

      // The dark face of the row's one dark control, and it does what its
      // label says: a single PATCH clearing the window, no dialog on the
      // way. A dialog is where a wake-up is CHOSEN; there is nothing to
      // choose about ending one, and a "Bring back" button that opened a
      // panel of re-schedule presets named an action it did not perform.
      // A block that wants a different wake-up comes back and goes dark
      // again -- two named clicks, neither of them a lie.
      bringBack(block) {
        if (!this.canArm()) return;
        this.darkTarget = { id: block.id, name: block.name };
        void this.applyDark(null);
      },

      cancelDark(force = false) {
        if (this.pendingId && !force) return;
        if (this.$refs.darkDialog.open) this.$refs.darkDialog.close();
        this.darkTarget = null;
      },

      // Field-scoped PATCH carrying disabled_until and nothing else.
      // `enabled` is the OTHER axis -- a block can be enabled and dark,
      // and sending both would answer a question the operator did not ask
      // (decision 2). A null preset clears the window: an explicit null is
      // what the contract reads as "bring it back", where an absent key
      // would leave the window exactly where it is.
      async applyDark(preset) {
        const target = this.darkTarget;
        if (!target || this.pendingId) return;
        this.pendingId = target.id;
        this.blocksProblem = null;
        try {
          // serverNow() at the moment of the write, not this.now: a preset
          // resolved from a stale clock can land in the server's past,
          // where the write succeeds and suppresses nothing -- a dark
          // window that reports itself set and does nothing.
          const until = preset === null ? null : darkPresetInstant(preset, serverNow());
          const updated = await apiSend<BlockRecord>("PATCH", apiPath("/blocks/{id}", { id: target.id }), {
            disabled_until: until,
          });
          this.blocks = this.blocks.map((b) => (b.id === target.id ? updated : b));
          printTape(
            updated.disabled_until
              ? `Dark until ${formatLocal(updated.disabled_until)} — ${updated.name}`
              : `Back on schedule — ${updated.name}`,
          );
        } catch (err) {
          this.blocksProblem = toProblemView(err);
        } finally {
          this.pendingId = null;
          this.cancelDark(true);
        }
      },

      // ---- the cron footgun --------------------------------------------

      // Degrades exactly like loadMedia: no cursors means no warning, and
      // the save behaves as it did before this confirm existed. Not a
      // .problem panel -- nothing on screen depends on this read until the
      // operator changes a cron, and there is no action they could take
      // from inside the editor if it failed.
      async loadSeriesStates() {
        try {
          this.seriesStates = await apiGet<SeriesState[]>(apiPath("/state/series"));
        } catch {
          this.seriesStates = [];
        }
      },

      // True once the confirm has taken the save over. The stored record
      // supplies the cron this edit is measured against -- the same record
      // submit() reads If-Match from, and the same one the open-editor
      // freeze keeps still, so the two can never disagree about what was
      // loaded.
      armCronConfirm() {
        // Answered for THIS expression, or not answered at all. An
        // earlier answer to an earlier cron re-asks, which is what keeps
        // a queued rewind from outliving the edit it was given for.
        if (ackedRewind(this.cronAck, this.editor.form.cron) !== null) return false;
        const stored = this.blocks.find((b) => b.id === this.editor.editingId);
        const shows = cronFootgunShows(this.editor.form, stored ? stored.spec.cron : null, this.seriesStates);
        if (shows.length === 0) return false;
        this.cronConfirm = { shows };
        this.$refs.cronDialog.showModal();
        return true;
      },

      cronCopy() {
        // The dialog stays in the DOM once closed and Alpine keeps
        // evaluating its bindings, so an empty answer here is the closed
        // state rather than a guard against a case that cannot happen.
        if (!this.cronConfirm) return { consequence: "", rewind: "" };
        return footgunCopy(this.editor.form.name, this.cronConfirm.shows);
      },

      // Both answers save. The only difference is whether the cursors come
      // with it, which is why neither button is a Cancel -- Cancel is the
      // third button, and it is the one that returns to the editor.
      proceedCron(rewind) {
        this.cronAck = {
          cron: this.editor.form.cron.trim(),
          rewind: rewind ? (this.cronConfirm?.shows ?? []) : [],
        };
        // close() fires @close, which clears cronConfirm.
        this.$refs.cronDialog.close();
        void this.submit();
      },

      cancelCron() {
        if (this.$refs.cronDialog.open) this.$refs.cronDialog.close();
        this.cronConfirm = null;
      },

      // One PATCH per SHOW: PATCH /state/series/{show_title} is per-title
      // and this confirm is per-block, so a block seeding three shows is
      // three round trips. allSettled, never all: one 404 for a title that
      // was retired since the list loaded must not hide the two cursors
      // that did move. The failure line points at the history page's
      // TRACKED pane, which is where a cursor is fixed by hand.
      async rewindCursors(shows) {
        const results = await Promise.allSettled(
          shows.map((s) =>
            apiSend<SeriesState>("PATCH", apiPath("/state/series/{show_title}", { show_title: s.title }), {
              current_season: s.season,
              current_episode: s.episode,
            }),
          ),
        );
        const report = rewindReport(
          shows.map((s, i) => {
            const result = results[i];
            return {
              title: s.title,
              error: result && result.status === "rejected" ? describeError(result.reason) : null,
            };
          }),
        );
        if (report.moved) printTape(report.moved);
        if (report.failed) {
          printTape(report.failed, {
            label: "Open cursors",
            run: () => {
              window.location.href = "/history/?view=tracked";
            },
          });
        }
      },
    }),
  );
});
