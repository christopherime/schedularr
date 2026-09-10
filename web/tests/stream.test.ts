// Unit tests for web/assets/ts/runtime/stream.ts, in two halves.
//
// First, parseFrames() against a retained buffer -- the pure half, and the
// part that actually breaks, since chunk boundaries have nothing to do
// with frame boundaries.
//
// Then connectStream() over a stubbed fetch, covering the two guarantees
// that were each broken once and are cheap to break again: the ladder
// must not report LIVE before a connection has delivered anything, and a
// reconnect must resume from the last id the previous connection saw.
// What stays on the Phase B gate's by-hand browser pass is the real
// backoff timing and what the operator SEES on each rung.
import assert from "node:assert/strict";
import test from "node:test";

const { parseFrames } = await import("../assets/ts/runtime/stream.ts");

test("parses a complete frame", () => {
  const { frames, rest } = parseFrames("id: 7\nevent: status.changed\ndata: {\"a\":1}\n\n");
  assert.equal(frames.length, 1);
  assert.deepEqual(frames[0], { id: "7", event: "status.changed", data: '{"a":1}' });
  assert.equal(rest, "");
});

test("holds an incomplete frame in the buffer until it terminates", () => {
  const first = parseFrames("event: heartbeat\ndata: {\"server_ti");
  assert.equal(first.frames.length, 0);
  const second = parseFrames(first.rest + 'me":"x"}\n\n');
  assert.equal(second.frames.length, 1);
  assert.equal(second.frames[0]?.data, '{"server_time":"x"}');
});

test("handles CRLF line endings", () => {
  const { frames } = parseFrames("event: heartbeat\r\ndata: {}\r\n\r\n");
  assert.equal(frames.length, 1);
  assert.equal(frames[0]?.event, "heartbeat");
});

test("joins repeated data lines with a newline, per the SSE spec", () => {
  const { frames } = parseFrames("event: x\ndata: one\ndata: two\n\n");
  assert.equal(frames[0]?.data, "one\ntwo");
});

test("parses several frames from one chunk", () => {
  const { frames } = parseFrames("event: a\ndata: 1\n\nevent: b\ndata: 2\n\n");
  assert.deepEqual(frames.map((f) => f.event), ["a", "b"]);
});

test("ignores comment lines and unknown fields", () => {
  const { frames } = parseFrames(": keep-alive\nevent: a\nretry: 5000\ndata: 1\n\n");
  assert.equal(frames.length, 1);
  assert.equal(frames[0]?.event, "a");
});

test("a frame with no event name defaults to message", () => {
  const { frames } = parseFrames("data: 1\n\n");
  assert.equal(frames[0]?.event, "message");
});

// ---- the reader half -----------------------------------------------------
//
// Two behaviours below the parser that no amount of frame-splitting
// exercises, and that both fail silently in production: the Last-Event-ID
// resume across a teardown, and the ladder's refusal to claim LIVE before
// anything has connected. fetch() is stubbed; nothing here reaches a
// server. Node has no IndexedDB and no localStorage, so token.ts's
// hydration degrades to "no token" on its own -- no stub needed for it.

const { connectStream } = await import("../assets/ts/runtime/stream.ts");

/** Spins the event loop until `pred` holds. connectStream drives itself
 * from an async loop, so there is no promise to await for "the request
 * went out" -- polling is the honest way to observe it. */
async function waitFor(pred: () => boolean, what: string): Promise<void> {
  for (let i = 0; i < 200; i += 1) {
    if (pred()) return;
    await new Promise((resolve) => setTimeout(resolve, 5));
  }
  throw new Error(`timed out waiting for ${what}`);
}

const encoder = new TextEncoder();

test("the ladder does not report live before a connection has delivered anything", async () => {
  // A server that is simply not there. The old ladder announced rung(1)
  // -- "live" -- after this first failure, painting the bezel green
  // against a link that has never once been up.
  let attempts = 0;
  globalThis.fetch = (() => {
    attempts += 1;
    return Promise.reject(new Error("connection refused"));
  }) as unknown as typeof fetch;
  const states: string[] = [];
  const stop = connectStream({ onFrame: () => {}, onState: (state) => states.push(state) });
  await waitFor(() => attempts > 0, "the first attempt");
  stop();
  assert.deepEqual(states, []);
});

test("a fresh connectStream resumes from the last id the previous one saw", async () => {
  // The whole point of the resume: a manual Reconnect and the
  // visibilitychange resume both tear the stream down and build a new
  // one, and the server replays only what Last-Event-ID asks it to.
  const sent: Record<string, string>[] = [];
  const bodies: ReadableStreamDefaultController<Uint8Array>[] = [];
  globalThis.fetch = ((_url: string, init?: RequestInit) => {
    sent.push({ ...(init?.headers as Record<string, string>) });
    return Promise.resolve(
      new Response(
        new ReadableStream<Uint8Array>({
          start(controller) {
            bodies.push(controller);
            // Mirror a real fetch: aborting the signal errors the body,
            // which is what ends the reader loop on stop().
            init?.signal?.addEventListener("abort", () => {
              try {
                controller.error(new Error("aborted"));
              } catch {
                // Already errored by a previous abort.
              }
            });
          },
        }),
      ),
    );
  }) as unknown as typeof fetch;

  const frames: string[] = [];
  const states: string[] = [];
  const stop = connectStream({ onFrame: (frame) => frames.push(frame.event), onState: (s) => states.push(s) });
  await waitFor(() => bodies.length === 1, "the first request");
  assert.deepEqual(states, [], "nothing is live until a frame lands");

  bodies[0]?.enqueue(encoder.encode('id: 42\nevent: status.changed\ndata: {}\n\n'));
  await waitFor(() => frames.length === 1, "the first frame");
  assert.deepEqual(states, ["live"], "a delivered frame is what makes it live");
  stop();

  const stop2 = connectStream({ onFrame: () => {}, onState: () => {} });
  await waitFor(() => sent.length === 2, "the reconnect request");
  assert.equal(sent[1]?.["Last-Event-ID"], "42");
  stop2();
});
