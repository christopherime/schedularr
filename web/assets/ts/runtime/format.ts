// Shared formatting helpers -- previously copy-pasted per page bundle.

/**
 * Local-timezone date+time for an RFC 3339 wire value. Renders an em dash
 * for a missing value and echoes an unparseable one verbatim rather than
 * guessing -- the binding contract calls for local time, not the UTC wire
 * value.
 */
export function formatLocal(iso: string | null | undefined): string {
  if (!iso) return "—";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleString(undefined, {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  });
}

/** A real plural ("1 slot", "2 slots"), never a mechanical "(s)" suffix. */
export function plural(n: number, noun: string): string {
  return `${n} ${noun}${n === 1 ? "" : "s"}`;
}

/**
 * Clamps free-text days input to the plan window's documented [1, 30]
 * range (api/openapi.yaml: GenerateRequest.days and GET /schedule's days
 * share the same bounds), defaulting to 7 (the schema's own default) for
 * blank/non-finite input. Shared by the guide's DAYS control and the
 * schedule page. This is the client-side clamp half of the contract; a
 * value that somehow still slips out of range is caught by the API's own
 * 400.
 */
export function clampDays(raw: string): number {
  const trimmed = raw.trim();
  // Blank input must take the schema default (7), not fall through
  // Number("") === 0 into the 1-day clamp.
  if (trimmed === "") return 7;
  const n = Number(trimmed);
  if (!Number.isFinite(n)) return 7;
  return Math.min(30, Math.max(1, Math.round(n)));
}

/** "21:05" -- local wall-clock label for a millisecond timestamp. */
export function formatClock(ms: number): string {
  const d = new Date(ms);
  return `${pad2(d.getHours())}:${pad2(d.getMinutes())}`;
}

/** "2 h" / "1 h 30 min" / "45 min" -- compact duration from minutes. */
export function durationLabel(minutes: number): string {
  const m = Math.max(0, Math.round(minutes));
  const h = Math.floor(m / 60);
  const rest = m % 60;
  if (h === 0) return `${rest} min`;
  if (rest === 0) return `${h} h`;
  return `${h} h ${rest} min`;
}

/** "S02E05" marker from the typed program shape, or null when either
 * half is absent -- a movie or flex placeholder gets no fabricated
 * S00E00. */
export function sxxeyy(season: number | undefined, episode: number | undefined): string | null {
  if (season === undefined || episode === undefined) return null;
  return `S${pad2(season)}E${pad2(episode)}`;
}

export function pad2(n: number): string {
  return String(n).padStart(2, "0");
}

/**
 * Compact relative readout for the bezel telemetry strip, both directions:
 * "just now" / "5 min ago" / "3 h ago" / "2 d ago", and "in 5 min" /
 * "in 3 h" for future instants (next cron tick). Uses the browser clock
 * uncorrected -- heartbeat skew correction arrives with SSE (v0.5.4).
 * `now` is injectable for tests.
 */
export function relativeTime(iso: string | null | undefined, now: number = Date.now()): string {
  if (!iso) return "—";
  const t = new Date(iso).getTime();
  if (Number.isNaN(t)) return "—";
  const diff = t - now;
  const abs = Math.abs(diff);
  const minute = 60_000;
  const hour = 60 * minute;
  const day = 24 * hour;

  let value: string;
  if (abs < minute) {
    return diff <= 0 ? "just now" : "in <1 min";
  } else if (abs < hour) {
    value = `${Math.round(abs / minute)} min`;
  } else if (abs < day) {
    value = `${Math.round(abs / hour)} h`;
  } else {
    value = `${Math.round(abs / day)} d`;
  }
  return diff < 0 ? `${value} ago` : `in ${value}`;
}

/**
 * NEXT TICK readout: relativeTime for future instants, but a past instant
 * renders "due" instead of "N min ago". The cron loop records the next
 * tick's time BEFORE running the current one (cmd/serve.go's
 * runCronLoop), so an overrunning tick leaves the stored instant in the
 * past while the loop is still mid-run -- "12 min ago" there reads as a
 * missed tick rather than one in progress.
 */
export function untilTime(iso: string | null | undefined, now: number = Date.now()): string {
  if (!iso) return "—";
  const t = new Date(iso).getTime();
  if (Number.isNaN(t)) return "—";
  return t <= now ? "due" : relativeTime(iso, now);
}
