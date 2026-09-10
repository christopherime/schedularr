// The live link's client-side switchboard: one place where stream frames
// become page reactions, and one place that owns the clock offset the
// heartbeat carries.
//
// It is deliberately dumb -- a name-keyed handler map with no schema and
// no queueing. The frames it carries are HINTS ("something changed, go
// look"), never data the UI renders directly, so a lost or duplicated
// publish costs a refetch and nothing else. That is what lets the whole
// live link fail without taking a page down with it.

/** The degradation ladder's three rungs, and the only three. See
 * runtime/stream.ts for what puts us on each one. */
export type LinkState = "live" | "poll" | "lost";

// ---- the event bus -------------------------------------------------------

type Handler = (data: unknown) => void;

const handlers = new Map<string, Set<Handler>>();

/**
 * Registers `handler` for `event` (the wire names in internal/events:
 * "apply.completed", "plan.invalidated", "status.changed",
 * "series.changed", "heartbeat"), returning its unsubscriber.
 *
 * Handlers get `unknown`, not a parsed shape: the bus does not own the
 * payload schemas, and a page that reads a field the server stopped
 * sending should fail in that page's own narrowing, not silently here.
 */
export function subscribe(event: string, handler: Handler): () => void {
  let set = handlers.get(event);
  if (set === undefined) {
    set = new Set<Handler>();
    handlers.set(event, set);
  }
  set.add(handler);
  const registered = set;
  return () => {
    registered.delete(handler);
  };
}

/**
 * Delivers `data` to every handler of `event`, in this tab only. Named
 * publishLocal, not publish, because nothing here reaches the server:
 * the server-to-client direction is the SSE stream, and there is no
 * client-to-server direction on this bus at all.
 */
export function publishLocal(event: string, data: unknown): void {
  const set = handlers.get(event);
  if (set === undefined) return;
  // Iterate a snapshot: a handler that unsubscribes itself (or a
  // sibling) mid-dispatch is normal -- the guide's one-shot refetch
  // guards do exactly that -- and a handler subscribed during dispatch
  // must wait for the NEXT event rather than see this one late.
  for (const handler of [...set]) handler(data);
}

// ---- link state ----------------------------------------------------------

type LinkWatcher = (state: LinkState) => void;

// Starts on POLL, not LIVE: at page load the stream has not connected
// yet, and the 60s /status poll is genuinely what is feeding the bezel.
// Claiming LIVE for the few hundred milliseconds before the stream
// answers would be the one thing the bezel must never do -- show a
// reading it does not have.
let link: LinkState = "poll";
const linkWatchers = new Set<LinkWatcher>();

export function linkState(): LinkState {
  return link;
}

/** Registers a link-state watcher, returning its unsubscriber. Fires only
 * on an actual change, never on the current value -- read linkState() for
 * that. */
export function onLinkChange(cb: LinkWatcher): () => void {
  linkWatchers.add(cb);
  return () => {
    linkWatchers.delete(cb);
  };
}

/**
 * Moves the ladder. shell.ts drives this from connectStream's onState,
 * and it is what suspends or resumes the /status poll.
 *
 * The no-change early return matters: entering LINK LOST pulses the
 * bezel amber, and a re-announcement on every 30s retry would turn one
 * state change into a permanent blinking light.
 */
export function setLinkState(state: LinkState): void {
  if (state === link) return;
  link = state;
  for (const cb of [...linkWatchers]) cb(state);
}

// ---- clock skew ----------------------------------------------------------

/**
 * How much of a fresh heartbeat sample is folded into the running offset.
 * Low on purpose: every sample carries that request's one-way latency as
 * error, so trusting one outright makes the guide's sweep cursor twitch
 * backwards and forwards every 15 seconds. A quarter converges within a
 * handful of heartbeats and stays still afterwards.
 */
const SMOOTHING = 0.25;

/**
 * A disagreement this large is not latency -- it is a real step: the
 * laptop woke from sleep, NTP corrected the machine, or the container's
 * clock was wrong until now. Smoothing through it would leave every
 * timestamp on the page minutes wrong for minutes more, so a step snaps
 * instead of easing.
 */
const RESYNC_THRESHOLD_MS = 5_000;

/**
 * Folds one offset sample (server_time − local now, in ms) into the
 * running offset. Pure, so the smoothing behaviour is pinned by tests:
 * this is the arithmetic every relative timestamp on every page depends
 * on, and it is invisible when it is subtly wrong.
 */
export function smoothOffset(prev: number | null, sample: number): number {
  if (prev === null || Math.abs(sample - prev) > RESYNC_THRESHOLD_MS) return sample;
  return prev + (sample - prev) * SMOOTHING;
}

// null until the first heartbeat lands: serverNow() then reads as the
// plain local clock, which is exactly as right as the page was before
// the live link existed.
let offsetMs: number | null = null;

/**
 * Folds a heartbeat's `server_time` (RFC 3339, as internal/api/events.go
 * writes it) into the offset.
 */
export function noteHeartbeat(serverTime: string): void {
  const server = Date.parse(serverTime);
  // A server_time we cannot parse is dropped rather than folded in: NaN
  // would poison the offset permanently, and every timestamp and the
  // guide's sweep cursor with it -- there is no recovering from NaN by
  // averaging more samples into it.
  if (Number.isNaN(server)) return;
  offsetMs = smoothOffset(offsetMs, server - Date.now());
}

/**
 * Date.now() corrected by the heartbeat offset. Every relative timestamp
 * and the guide's sweep cursor read this instead of Date.now(): an
 * operator whose laptop clock is minutes off the server's was the whole
 * class of bug the retired schedule.ts kept producing, and correcting at
 * the source is the only fix that does not have to be repeated per
 * call site.
 */
export function serverNow(): number {
  return Date.now() + (offsetMs ?? 0);
}
