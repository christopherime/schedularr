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
