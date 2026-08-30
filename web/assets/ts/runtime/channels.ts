// The runtime's /channels cache: one shared, promise-cached fetch per page
// load, consumed by every surface that names a channel -- the channel-select
// partial's options, and the legend plates that replace raw UUIDs
// (dashboard history, blocks list, schedule channel heads). A failed fetch
// clears the cache so a retry actually refetches.
import { apiGet, apiPath } from "./api.ts";
import type { ApiResponse } from "./api.ts";
import { pad2 } from "./format.ts";

export type Channel = ApiResponse<"listChannels", 200>[number];

let cached: Promise<Channel[]> | null = null;

export function loadChannels(): Promise<Channel[]> {
  if (!cached) {
    cached = apiGet<Channel[]>(apiPath("/channels")).catch((err: unknown) => {
      cached = null;
      throw err;
    });
  }
  return cached;
}

/** Drops the cache -- called on re-auth so the next load refetches with
 * the new token instead of replaying a 401's rejection. */
export function invalidateChannels(): void {
  cached = null;
}

/**
 * The channel field's hint line (ui/channel-select's fallback messaging),
 * shared by every page with a channel picker -- previously written
 * near-verbatim three times (blocks, schedule, kit), which let the kit's
 * copy drift from what ships. Select-vs-free-text is gated on "usable
 * options exist", not just "the call didn't error": a reachable Tunarr
 * with zero channels configured would otherwise render an unusable empty
 * <select>. Both cases fall back to the same free-text input, described
 * by `manualHint` (the schedule page adds "or leave blank for all
 * channels" to it; sentence punctuation is appended here).
 */
export function channelHint(
  loading: boolean,
  error: string | null,
  channels: Channel[],
  manualHint = "enter the channel ID manually",
): string {
  if (loading) return "Loading channels from Tunarr…";
  if (error) return `Tunarr channel list unavailable (${error}) — ${manualHint}.`;
  if (channels.length === 0) return `Tunarr returned no channels — ${manualHint}.`;
  return "";
}

/** "4 · Horror" -- the <option> label for channel selects. */
export function channelLabel(c: Channel): string {
  const parts: string[] = [];
  if (c.number !== undefined) parts.push(String(c.number));
  parts.push(c.name ?? c.id ?? "?");
  return parts.join(" · ");
}

/**
 * The two text pieces of a channel legend plate (`CH 04 · HORROR` -- the
 * ui/plate partial uppercases visually). `ch` is null when the channel is
 * unresolvable (or has no number), in which case `name` falls back to the
 * shortened raw id rather than a fabricated name -- the plate never lies
 * about what it knows.
 */
export interface PlateParts {
  ch: string | null;
  name: string;
}

function shortId(id: string): string {
  return id.length > 11 ? `${id.slice(0, 8)}…` : id;
}

export function channelPlate(id: string | null | undefined, channels: Channel[]): PlateParts {
  if (!id) return { ch: null, name: "—" };
  const c = channels.find((ch) => ch.id === id);
  if (!c) return { ch: null, name: shortId(id) };
  return {
    ch: c.number === undefined ? null : `CH ${pad2(c.number)}`,
    name: c.name ?? shortId(id),
  };
}

/** Orders channels the way their plates read: by resolved channel number
 * first (a section headed `CH 04 · HORROR` sorting on raw UUID looks
 * arbitrary), then name, then raw id as the final tiebreak. Channels the
 * cache can't resolve (or that carry no number) sort after numbered ones,
 * by name/id. Shared by the guide's row order and the schedule page's
 * section order. */
export function channelOrder(aId: string, bId: string, channels: Channel[]): number {
  const a = channels.find((c) => c.id === aId);
  const b = channels.find((c) => c.id === bId);
  const aNum = a?.number ?? Number.POSITIVE_INFINITY;
  const bNum = b?.number ?? Number.POSITIVE_INFINITY;
  if (aNum !== bNum) return aNum - bNum;
  const byName = (a?.name ?? "").localeCompare(b?.name ?? "");
  if (byName !== 0) return byName;
  return aId.localeCompare(bId);
}
