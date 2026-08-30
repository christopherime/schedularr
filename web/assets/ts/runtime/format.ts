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
