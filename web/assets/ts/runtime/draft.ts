// The guide's draft model (spec §3.3): pure functions behind draft mode.
// A DRAFT is a 7-day POST /generate result for a SCOPE, armed for apply;
// the READING is the committed-mode plan the guide already shows (always
// ALL channels, 28 days). This module diffs the two, keeps every line of
// draft copy in one place, and codes the reading to/from sessionStorage
// for the Blocks-page round trip. No DOM, no fetch -- guide.ts wires it;
// web/tests/draft.test.ts pins it.
//
// Honesty boundary: nothing here knows what Tunarr currently holds. A
// verdict is always "vs the reading taken at HH:MM" -- the bar, the
// confirm, and the inspector say so in text -- never a claim about
// Tunarr's lineup. That question belongs to the Memory slice's enriched
// history.
//
// Why 7 days: the engine prunes occurrence snapshots and history by
// write time on every commit (history_retention, 7 days by default), so
// an apply wider than the retention window commits state the store
// forgets before it airs. Seven days is the CLI default, the old
// Schedule page's default, and the spec's confirmed default (§11 Q6).
import type { ApiRequestJSON } from "./api.ts";
import type { PlateParts } from "./channels.ts";
import { formatClock, plural } from "./format.ts";
import type { DraftVerdict, GuideProgram, GuideRow, GuideSlot } from "./grid.ts";
import type { components } from "../gen/types";

type PlanResult = components["schemas"]["PlanResult"];
type GenerateBody = ApiRequestJSON<"generateSchedule">;

/** The draft/apply window. */
export const DRAFT_DAYS = 7;
export const DRAFT_HORIZON_MS = DRAFT_DAYS * 86_400_000;

/** Exactly what a draft was generated from; apply() sends this, verbatim. */
export interface DraftSignature {
  days: number;
  channelId: string;
}

/** The GenerateRequest body for a signature -- channel_id omitted (never
 * sent as "") for ALL channels. Shared by preview and apply so the
 * armed-signature rule holds by construction. */
export function draftRequestBody(sig: DraftSignature): GenerateBody {
  const body: GenerateBody = { days: sig.days };
  if (sig.channelId !== "") body.channel_id = sig.channelId;
  return body;
}

/** `?draft=all` -> "" (ALL channels), `?draft=<id>` -> the id, absent or
 * empty -> null (no draft requested). */
export function draftScopeFromSearch(search: string): string | null {
  const value = new URLSearchParams(search).get("draft");
  if (value === null || value === "") return null;
  return value === "all" ? "" : value;
}

/** The guide URL a block save's PREVIEW ON GUIDE action navigates to. */
export function draftHref(channelId: string | undefined): string {
  return `/?draft=${encodeURIComponent(channelId ? channelId : "all")}`;
}

// ---- diff -------------------------------------------------------------------

export interface DiffCounts {
  new: number;
  changed: number;
  same: number;
  removed: number;
}

export interface DiffOptions {
  /** When the draft request was SENT (browser clock): the diff cutoff
   * and the start of the 7-day horizon. */
  requestedAt: number;
  /** "" for ALL channels, else the one channel the draft planned. */
  scopeChannelId: string;
}

/** Occurrence identity: the engine keys an occurrence by (block, start)
 * on one channel. Block names are unique in the store. */
export function slotKey(s: GuideSlot): string {
  return `${s.channelId}|${s.blockName}|${s.startMs}`;
}

/** Content identity: the end time plus the ordered lineup. Season and
 * episode print as "-" when absent so a movie and an episode never
 * collide by accident. */
export function lineupSignature(s: GuideSlot): string {
  const programs = s.programs
    .map((p) => `${p.title}|${p.durationMs}|${p.season ?? "-"}|${p.episode ?? "-"}`)
    .join(";");
  return `${s.endMs}#${programs}`;
}

function withVerdict(s: GuideSlot, draft: DraftVerdict): GuideSlot {
  return { ...s, draft };
}

function rank(s: GuideSlot): number {
  return s.draft === "removed" ? 0 : 1;
}

/**
 * Diffs the draft's rows against the reading's rows and returns NEW rows
 * (inputs untouched):
 *
 *   - every draft slot carries new / changed / same;
 *   - a reading slot the draft no longer plans is `removed` when it
 *     overlaps the draft window (ends after requestedAt, starts before
 *     the horizon) -- the on-air slot included, since applying cuts it;
 *     `beyond` when it starts at or past the horizon (shown plain, not
 *     counted -- the draft never planned that far); dropped silently when
 *     it ended before the draft was requested (the past, not a removal);
 *   - a reading slot a draft ghost stands for (same channel, block, and
 *     start) is left to the ghost: it is DROPPED, not removed, so one
 *     displaced occurrence is never counted twice;
 *   - ghosts pass through unverdicted and uncounted.
 *
 * With scopeChannelId "" every reading channel is in scope (a channel the
 * draft dropped entirely becomes an all-removed row); with a channel
 * scope only that channel is compared and returned. Slots sort by start,
 * `removed` first at a tie so a replacement never paints over what it
 * replaces (removed slots render in the second lane anyway).
 */
export function diffRows(
  readingRows: GuideRow[],
  draftRows: GuideRow[],
  opts: DiffOptions,
): { rows: GuideRow[]; counts: DiffCounts } {
  const horizonMs = opts.requestedAt + DRAFT_HORIZON_MS;
  const counts: DiffCounts = { new: 0, changed: 0, same: 0, removed: 0 };
  const readingByChannel = new Map(readingRows.map((r) => [r.channelId, r]));
  const draftByChannel = new Map(draftRows.map((r) => [r.channelId, r]));
  const channelIds = new Set<string>();
  for (const r of draftRows) channelIds.add(r.channelId);
  if (opts.scopeChannelId === "") for (const r of readingRows) channelIds.add(r.channelId);
  else if (readingByChannel.has(opts.scopeChannelId)) channelIds.add(opts.scopeChannelId);

  const rows: GuideRow[] = [];
  for (const channelId of channelIds) {
    const reading = readingByChannel.get(channelId);
    const draft = draftByChannel.get(channelId);
    const readingSlots = new Map<string, GuideSlot>();
    for (const s of reading?.slots ?? []) if (s.kind === "slot") readingSlots.set(slotKey(s), s);

    const slots: GuideSlot[] = [];
    const seen = new Set<string>();
    for (const s of draft?.slots ?? []) {
      const key = slotKey(s);
      if (s.kind !== "slot") {
        // A ghost stands for the reading slot it displaced.
        seen.add(key);
        slots.push(s);
        continue;
      }
      seen.add(key);
      const before = readingSlots.get(key);
      let verdict: DraftVerdict;
      if (!before) verdict = "new";
      else if (lineupSignature(before) !== lineupSignature(s)) verdict = "changed";
      else verdict = "same";
      counts[verdict]++;
      slots.push(withVerdict(s, verdict));
    }
    for (const [key, before] of readingSlots) {
      if (seen.has(key)) continue;
      if (before.startMs >= horizonMs) {
        slots.push(withVerdict(before, "beyond"));
        continue;
      }
      if (before.endMs <= opts.requestedAt) continue;
      counts.removed++;
      slots.push(withVerdict(before, "removed"));
    }
    slots.sort((a, b) => a.startMs - b.startMs || rank(a) - rank(b));
    rows.push({ channelId, plate: (draft ?? reading)!.plate, slots });
  }
  return { rows, counts };
}

// ---- reading persistence ----------------------------------------------------

/** One reading slot as stored: the fields the diff and the inspector
 * need, nothing from the wire (no ISO strings, no block spec, no plate
 * -- plates are re-resolved on restore). Ghosts are not stored. */
export interface StoredSlot {
  blockName: string;
  blockType: string;
  cron: string;
  priority: number;
  startMs: number;
  endMs: number;
  programs: GuideProgram[];
}

/** The committed reading, mirrored to sessionStorage so a Blocks-page
 * round trip (save -> PREVIEW ON GUIDE) can diff against what the
 * operator last saw. It seeds the diff baseline ONLY -- a restored
 * reading is never shown as current: DISCARD and apply re-fetch. */
export interface StoredReading {
  requestedAt: number;
  rows: { channelId: string; slots: StoredSlot[] }[];
}

export const READING_STORAGE_KEY = "schedularr_guide_reading";
export const READING_MAX_AGE_MS = 24 * 60 * 60 * 1000;
/** Above this many characters the mirror is skipped (sessionStorage
 * quotas sit around 5 MB; a 20-channel four-week plan can exceed it). */
export const READING_MAX_CHARS = 2_000_000;

/** null when the payload would be too large to store. */
export function serializeReading(requestedAt: number, rows: GuideRow[]): string | null {
  const stored: StoredReading = {
    requestedAt,
    rows: rows.map((r) => ({
      channelId: r.channelId,
      slots: r.slots
        .filter((s) => s.kind === "slot")
        .map((s) => ({
          blockName: s.blockName,
          blockType: s.blockType,
          cron: s.cron,
          priority: s.priority,
          startMs: s.startMs,
          endMs: s.endMs,
          programs: s.programs,
        })),
    })),
  };
  const raw = JSON.stringify(stored);
  return raw.length > READING_MAX_CHARS ? null : raw;
}

function isFiniteNumber(value: unknown): value is number {
  return typeof value === "number" && Number.isFinite(value);
}

/** A program as stored: title and durationMs are required (the rundown
 * prints them and lineupSignature keys on them); season and episode are
 * optional everywhere they are read. */
function isStoredProgram(value: unknown): boolean {
  if (typeof value !== "object" || value === null) return false;
  const { title, durationMs } = value as { title?: unknown; durationMs?: unknown };
  return typeof title === "string" && isFiniteNumber(durationMs);
}

/** A slot as stored. startMs/endMs are the geometry: a non-finite or
 * inverted pair would place a box of NaN width. */
function isStoredSlot(value: unknown): boolean {
  if (typeof value !== "object" || value === null) return false;
  const { blockName, blockType, cron, priority, startMs, endMs, programs } = value as Record<string, unknown>;
  if (typeof blockName !== "string" || typeof blockType !== "string" || typeof cron !== "string") return false;
  if (typeof priority !== "number") return false;
  if (!isFiniteNumber(startMs) || !isFiniteNumber(endMs) || startMs >= endMs) return false;
  return Array.isArray(programs) && programs.every(isStoredProgram);
}

function isStoredRow(value: unknown): boolean {
  if (typeof value !== "object" || value === null) return false;
  const { channelId, slots } = value as { channelId?: unknown; slots?: unknown };
  return typeof channelId === "string" && Array.isArray(slots) && slots.every(isStoredSlot);
}

/**
 * null for anything missing, malformed, older than READING_MAX_AGE_MS,
 * or taken before the server's last apply (lastAppliedAt =
 * Status.last_applied_at) -- a reading from before an apply is not what
 * the operator would be diffing against. An unparseable stamp does not
 * invalidate (the server simply has not recorded one).
 *
 * Malformed is checked ELEMENTWISE, not just at the envelope: this
 * payload survives a deploy in the operator's tab, so a reading an
 * older build wrote must be rejected outright rather than reaching the
 * grid's geometry as NaN.
 */
export function parseStoredReading(raw: string | null, nowMs: number, lastAppliedAt?: string | null): StoredReading | null {
  if (raw === null) return null;
  let parsed: unknown;
  try {
    parsed = JSON.parse(raw);
  } catch {
    return null;
  }
  if (typeof parsed !== "object" || parsed === null) return null;
  const { requestedAt, rows } = parsed as { requestedAt?: unknown; rows?: unknown };
  if (!isFiniteNumber(requestedAt)) return null;
  if (!Array.isArray(rows) || !rows.every(isStoredRow)) return null;
  if (nowMs - requestedAt > READING_MAX_AGE_MS) return null;
  if (lastAppliedAt) {
    const applied = Date.parse(lastAppliedAt);
    if (!Number.isNaN(applied) && applied > requestedAt) return null;
  }
  return { requestedAt, rows: rows as StoredReading["rows"] };
}

/** Rebuilds renderable rows from a stored reading, re-resolving plates
 * through the live channel cache. */
export function rowsFromStored(stored: StoredReading, plateFor: (channelId: string) => PlateParts): GuideRow[] {
  return stored.rows.map((r) => ({
    channelId: r.channelId,
    plate: plateFor(r.channelId),
    slots: r.slots.map((s) => ({ kind: "slot" as const, channelId: r.channelId, ...s })),
  }));
}

// ---- counts and copy --------------------------------------------------------

export function planSlotCount(plan: PlanResult): number {
  return Object.values(plan.channels).reduce((n, slots) => n + slots.length, 0);
}

export function planChannelCount(plan: PlanResult): number {
  return Object.keys(plan.channels).length;
}

function verdictParts(counts: DiffCounts): string[] {
  const parts: string[] = [];
  if (counts.new > 0) parts.push(`${counts.new} new`);
  if (counts.changed > 0) parts.push(`${counts.changed} changed`);
  if (counts.removed > 0) parts.push(`${counts.removed} removed`);
  return parts;
}

/** The draft bar's readout. Zero verdict counts are omitted; a draft
 * with no verdict at all prints "no changes". The reading's clock keeps
 * the comparison honest: it is a diff against THAT reading. */
export function draftBarLine(p: {
  slots: number;
  channels: number;
  counts: DiffCounts;
  dropped: number;
  readingRequestedAt: number;
}): string {
  const parts = [`${DRAFT_DAYS}-day draft — ${plural(p.slots, "slot")} across ${plural(p.channels, "channel")}`];
  const verdicts = verdictParts(p.counts);
  parts.push(...(verdicts.length > 0 ? verdicts : ["no changes"]));
  if (p.dropped > 0) parts.push(`${p.dropped} dropped`);
  parts.push(`vs reading ${formatClock(p.readingRequestedAt)}`);
  return parts.join(" · ");
}

export function draftingLine(scopeLabel: string): string {
  return `Drafting — planning ${scopeLabel} against Tunarr…`;
}

export function applyingLine(slots: number, channels: number): string {
  return `Applying — ${plural(slots, "slot")} across ${plural(channels, "channel")}…`;
}

export function applyConfirmTitle(scopeLabel: string): string {
  return `Apply ${scopeLabel}`;
}

/** Real counts from the draft on the glass, never a guess. An empty
 * draft is not "nothing": the server pushes an empty lineup to every
 * channel it last applied inside the scope, so the body says so. */
export function applyConfirmBody(p: {
  slots: number;
  channels: number;
  counts: DiffCounts;
  scopeLabel: string;
  readingRequestedAt: number;
}): string {
  const when = formatClock(p.readingRequestedAt);
  if (p.channels === 0) {
    const head = `This draft schedules nothing for ${p.scopeLabel}. Applying pushes an empty lineup to any channel Schedularr last applied in this scope`;
    return p.counts.removed > 0 ? `${head} — ${plural(p.counts.removed, "slot")} removed vs the ${when} reading.` : `${head}.`;
  }
  const verdicts = verdictParts(p.counts);
  const tail = verdicts.length > 0 ? verdicts.join(", ") : "no changes";
  return `This applies ${plural(p.slots, "slot")} across ${plural(p.channels, "channel")} to Tunarr — ${tail} vs the ${when} reading.`;
}

export function appliedTapeLine(slots: number, channels: number): string {
  return `Applied — ${plural(slots, "slot")} / ${plural(channels, "channel")}`;
}

/** The inspector's verdict line for a draft slot. */
export function draftVerdictLine(v: DraftVerdict, readingRequestedAt: number, onAir: boolean): string {
  const when = formatClock(readingRequestedAt);
  switch (v) {
    case "new":
      return `Draft: new — not in the ${when} reading; the draft adds it.`;
    case "changed":
      return `Draft: changed — the lineup differs from the ${when} reading.`;
    case "removed":
      return onAir
        ? `Draft: removed — airing now in the ${when} reading, not in this draft; applying cuts it.`
        : `Draft: removed — in the ${when} reading, not in this draft; the draft does not schedule it.`;
    case "beyond":
      return `Beyond the ${DRAFT_DAYS}-day draft — from the ${when} reading, not part of this apply.`;
    default:
      return `Draft: unchanged from the ${when} reading.`;
  }
}
