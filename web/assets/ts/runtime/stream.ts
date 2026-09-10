// The live link's client half: a hand-rolled SSE reader over fetch() +
// ReadableStream, and the pure frame parser that feeds it.
//
// `EventSource` does not appear in this file and must not be introduced
// into it. EventSource cannot send an Authorization header, and the only
// way around that -- the token in a query string -- writes the operator's
// bearer token into every access log, proxy trace, and browser history
// entry the request passes through. That is refused permanently, not
// deferred (see the plan's Global Constraints), so "simplifying" this
// module down to EventSource is a security regression wearing a cleanup's
// clothes.
import { apiPath, notifyUnauthorized } from "./api.ts";
import type { LinkState } from "./bus.ts";
import { loadToken } from "./token.ts";

/**
 * One dispatched SSE frame. `id` is present only when the server sent
 * one: heartbeats deliberately carry no id (internal/api/events.go), so
 * a resume never replays clock ticks as though they were changes.
 */
export interface ParsedFrame {
  id?: string;
  event: string;
  data: string;
}

/**
 * Pulls every COMPLETE frame out of a retained buffer and returns the
 * unterminated tail as `rest`, which the caller prepends to the next
 * chunk before calling again.
 *
 * Chunk boundaries are arbitrary and have nothing to do with frame
 * boundaries -- a frame splits mid-field, and even between the two
 * newlines that terminate it -- so parsing a chunk in isolation drops
 * data at random intervals that no test on a small payload would catch.
 *
 * CRLF is folded to LF first, but a trailing lone CR is left in `rest` on
 * purpose: it may be the first half of a CRLF whose LF is in the next
 * chunk, and folding it early would fabricate a frame terminator in the
 * middle of a frame.
 */
export function parseFrames(buffer: string): { frames: ParsedFrame[]; rest: string } {
  const blocks = buffer.replace(/\r\n/g, "\n").split("\n\n");
  // The tail after the last blank line is by definition unterminated:
  // a fully terminated buffer ends in "\n\n" and yields "" here.
  const rest = blocks.pop() ?? "";
  const frames: ParsedFrame[] = [];
  for (const block of blocks) {
    const frame = parseBlock(block);
    if (frame !== null) frames.push(frame);
  }
  return { frames, rest };
}

/** Field-by-field parse of one complete frame block, per the SSE spec's
 * dispatch rules. Returns null for a block that dispatches nothing. */
function parseBlock(block: string): ParsedFrame | null {
  let id: string | undefined;
  let event = "";
  const data: string[] = [];

  for (const line of block.split("\n")) {
    // A line starting with ":" is a comment -- proxies and keep-alives
    // use them. Fields we do not know (retry:, and whatever a future
    // server adds) are ignored rather than mistaken for payload.
    if (line === "" || line.startsWith(":")) continue;
    const colon = line.indexOf(":");
    const field = colon === -1 ? line : line.slice(0, colon);
    let value = colon === -1 ? "" : line.slice(colon + 1);
    if (value.startsWith(" ")) value = value.slice(1);
    if (field === "id") id = value;
    else if (field === "event") event = value;
    else if (field === "data") data.push(value);
  }

  // No data field means there is nothing to dispatch: a comment-only
  // keep-alive block, or a stray blank run. Emitting a frame here would
  // hand every subscriber an empty payload to misparse.
  if (data.length === 0) return null;

  // An absent or empty event name is "message", per the spec. The key is
  // omitted rather than set to undefined so a frame compares equal to the
  // literal a test (or a caller) writes out.
  const frame: ParsedFrame = { event: event === "" ? "message" : event, data: data.join("\n") };
  if (id !== undefined) frame.id = id;
  return frame;
}

/**
 * The rungs, in order. Nothing functional depends on the stream, so each
 * rung degrades what the operator SEES, never what they can do:
 *
 *   LIVE      the stream is connected, or reconnecting with backoff
 *             (1s doubling to 30s) resuming from Last-Event-ID.
 *   POLL      three consecutive connection failures. The stream keeps
 *             trying in the background while a 60s poll of /status and
 *             the page's primary GET takes over.
 *   LINK LOST the network is gone. Polling stops, and the bezel offers a
 *             manual reconnect -- an honest instrument says it has no
 *             reading rather than showing a stale one.
 */
const POLL_AFTER_FAILURES = 3;
const LOST_AFTER_FAILURES = 6;
const BACKOFF_MIN_MS = 1_000;
const BACKOFF_MAX_MS = 30_000;

/** Which rung `failures` consecutive failures puts us on. One reconnect
 * inside the backoff is still LIVE: a stream that drops and comes back in
 * a second never earned a downgrade in the bezel.
 *
 * That "still LIVE" is only honest once something HAS connected, which is
 * why the caller gates it on everDelivered below -- on a first attempt
 * against a server that is not there, rung(1) would otherwise light the
 * legend green while nothing has ever been connected. */
function rung(failures: number): LinkState {
  if (failures >= LOST_AFTER_FAILURES) return "lost";
  if (failures >= POLL_AFTER_FAILURES) return "poll";
  return "live";
}

/**
 * The last event id the server sent, at MODULE scope on purpose: it has to
 * OUTLIVE a connectStream teardown. The manual Reconnect action and the
 * visibilitychange resume are both "stop, then connectStream again" (there
 * is deliberately no reconnect()), so a per-connection id would be null on
 * exactly the two paths a resume exists for -- internal/api/events.go's
 * resumeFrom would read 0 and hand back a fresh subscription instead of
 * replaying the frames the tab missed while it was away.
 *
 * One id per module is one id per stream: shell.ts's `started` guard makes
 * "exactly one stream per page" true, so there is no second connection
 * whose id this could be confused with.
 */
let lastEventID: string | null = null;

export interface StreamOptions {
  /** Every frame the server sends, heartbeats included, in arrival
   * order. Routing frames onto the bus is the caller's job (shell.ts). */
  onFrame: (frame: ParsedFrame) => void;
  /** Fired only when the ladder actually changes rung -- the bezel pulses
   * on entering LINK LOST, and re-announcing the same rung on every
   * retry would pulse it every 30 seconds forever.
   *
   * "live" is never reported speculatively: it arrives only once a frame
   * has actually been delivered, so a caller can paint the legend green
   * straight off this callback. */
  onState: (state: LinkState) => void;
}

/**
 * Opens the live link and keeps it open, returning a disconnect function.
 *
 * There is no reconnect() in the return: a manual reconnect (the LINK
 * LOST action) and the visibilitychange resume are both "disconnect, then
 * connectStream again", so one entry point covers all three and there is
 * no second lifecycle to keep in sync with this one.
 */
export function connectStream(opts: StreamOptions): () => void {
  let stopped = false;
  let failures = 0;
  let backoff = BACKOFF_MIN_MS;
  let announced: LinkState | null = null;
  // Whether this stream has ever had a frame delivered. Until it has, the
  // ladder's low rungs must not be reported as "live": a frame arriving is
  // the only PROOF the link is up, and a 200 whose body then delivers
  // nothing (a proxy that accepts and drops, a server mid-shutdown) is not
  // that proof.
  let everDelivered = false;
  let controller: AbortController | null = null;
  // Set while the retry backoff is sleeping, so disconnect() cuts the
  // wait short instead of leaving a 30s timer holding the loop open.
  let wake: (() => void) | null = null;

  function announce(state: LinkState): void {
    if (announced === state) return;
    announced = state;
    opts.onState(state);
  }

  function sleep(ms: number): Promise<void> {
    return new Promise((resolve) => {
      const timer = setTimeout(() => {
        wake = null;
        resolve();
      }, ms);
      wake = () => {
        clearTimeout(timer);
        wake = null;
        resolve();
      };
    });
  }

  /** Reads the response body until the server or the network ends it. */
  async function pump(body: ReadableStream<Uint8Array>): Promise<void> {
    const reader = body.getReader();
    const decoder = new TextDecoder();
    let buffer = "";
    for (;;) {
      const chunk = await reader.read();
      if (chunk.done) return;
      // stream: true so a multi-byte UTF-8 sequence split across chunks
      // (a show title's accented character, routinely) decodes as one
      // character instead of two replacement glyphs.
      buffer += decoder.decode(chunk.value, { stream: true });
      const parsed = parseFrames(buffer);
      buffer = parsed.rest;
      for (const frame of parsed.frames) {
        if (frame.id !== undefined) lastEventID = frame.id;
        // A delivered frame -- not a 200 -- is what clears the failure
        // count, resets the backoff, and unlocks "live". Resetting on the
        // response instead would pin a server that accepts and instantly
        // closes at LIVE forever, retrying it every second.
        everDelivered = true;
        failures = 0;
        backoff = BACKOFF_MIN_MS;
        announce("live");
        opts.onFrame(frame);
      }
    }
  }

  /** One connection attempt, from request to the stream ending. Throws
   * for every failure mode; the loop below decides what that means. */
  async function open(): Promise<void> {
    const headers: Record<string, string> = { Accept: "text/event-stream" };
    // Hydration is memoized single-flight (token.ts), so this costs one
    // decrypt per page load no matter how often we reconnect.
    const token = await loadToken();
    if (token) headers["Authorization"] = `Bearer ${token}`;
    // Resume where this tab left off: the hub replays whatever its ring
    // still holds past this id. Past the ring nothing replays, and the
    // page refetches instead -- events are hints, never the truth.
    if (lastEventID !== null) headers["Last-Event-ID"] = lastEventID;

    controller = new AbortController();
    // No AbortController timeout here, unlike every call in api.ts: this
    // response is SUPPOSED to never complete, so a read deadline would
    // guarantee a disconnect every N seconds rather than prevent a hang.
    // A dead connection surfaces as the read ending, or as the heartbeat
    // stopping, both of which land in the retry loop below.
    const res = await fetch(apiPath("/events"), { headers, signal: controller.signal });

    if (res.status === 401) {
      // Same contract as api.ts: announce it and let the shell open the
      // token panel at most once per unarmed episode. Never retry harder
      // -- a bad token 401s forever, and the backoff is what keeps that
      // from becoming a request loop.
      notifyUnauthorized();
      throw new Error("live link unauthorized");
    }
    if (!res.ok || res.body === null) {
      throw new Error(`live link failed with ${res.status}`);
    }

    await pump(res.body);
  }

  void (async () => {
    while (!stopped) {
      try {
        await open();
      } catch {
        // Refused, dropped mid-stream, 401, aborted, DNS gone: from here
        // they are the same event. The rung is decided by HOW MANY
        // failures in a row, not by which one -- a client that tried to
        // tell a dead server from a dead network would be guessing.
      }
      if (stopped) return;
      failures += 1;
      // Downgrades always pass; "live" only once something has connected.
      // Without that gate the very first failed attempt of a page load
      // announces rung(1) == "live", and the bezel reads LIVE against a
      // server it has never once reached.
      const next = rung(failures);
      if (next !== "live" || everDelivered) announce(next);
      await sleep(backoff);
      backoff = Math.min(backoff * 2, BACKOFF_MAX_MS);
    }
  })();

  return () => {
    stopped = true;
    controller?.abort();
    wake?.();
  };
}
