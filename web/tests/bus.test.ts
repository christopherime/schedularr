// Unit tests for the live link's event bus (web/assets/ts/runtime/bus.ts):
// subscribe/unsubscribe delivery, the link-state ladder's change
// notification, and the heartbeat skew smoothing.
//
// The bus holds module-level state by design (one switchboard per page
// bundle), so each test unsubscribes what it registers -- a leaked
// handler here would surface as a mystery failure three tests later.
// Nothing below touches the DOM: the frames' source (stream.ts) and
// their destinations (the page modules) are the halves that do.
import assert from "node:assert/strict";
import test from "node:test";

const { linkState, noteHeartbeat, onLinkChange, publishLocal, serverNow, setLinkState, smoothOffset, subscribe } =
  await import("../assets/ts/runtime/bus.ts");

// ---- subscribe / publishLocal --------------------------------------------

test("a subscriber receives its event's payload, and nothing else's", () => {
  const seen: unknown[] = [];
  const off = subscribe("apply.completed", (data) => seen.push(data));

  publishLocal("apply.completed", { run_id: "r1" });
  publishLocal("plan.invalidated", { reason: "block", id: "b1" });

  assert.deepEqual(seen, [{ run_id: "r1" }]);
  off();
});

test("unsubscribing stops delivery", () => {
  let calls = 0;
  const off = subscribe("series.changed", () => {
    calls += 1;
  });

  publishLocal("series.changed", null);
  off();
  publishLocal("series.changed", null);

  assert.equal(calls, 1);
});

test("several subscribers of one event all fire, and unsubscribe independently", () => {
  const hits: string[] = [];
  const offA = subscribe("status.changed", () => hits.push("a"));
  const offB = subscribe("status.changed", () => hits.push("b"));

  publishLocal("status.changed", null);
  offA();
  publishLocal("status.changed", null);
  offB();

  assert.deepEqual(hits, ["a", "b", "b"]);
});

test("publishing an event nobody subscribes to is a no-op", () => {
  assert.doesNotThrow(() => publishLocal("nobody.listening", { any: "thing" }));
});

test("a handler that unsubscribes itself mid-dispatch does not rob its siblings", () => {
  // The guide's one-shot refetch guards do exactly this. Dispatching over
  // the live Set instead of a snapshot would skip the sibling here.
  const hits: string[] = [];
  const offFirst = subscribe("plan.invalidated", () => {
    hits.push("first");
    offFirst();
  });
  const offSecond = subscribe("plan.invalidated", () => hits.push("second"));

  publishLocal("plan.invalidated", null);
  publishLocal("plan.invalidated", null);
  offSecond();

  assert.deepEqual(hits, ["first", "second", "second"]);
});

// ---- link state ----------------------------------------------------------

test("the ladder starts on POLL and notifies watchers only on a change", () => {
  // POLL, not LIVE: at page load the stream has not connected yet.
  assert.equal(linkState(), "poll");

  const seen: string[] = [];
  const off = onLinkChange((state) => seen.push(state));

  setLinkState("live");
  // Re-announcing the same rung must stay silent: the bezel pulses on
  // entering LINK LOST, and a pulse per 30s retry is a blinking light.
  setLinkState("live");
  setLinkState("lost");
  off();
  setLinkState("poll");

  assert.deepEqual(seen, ["live", "lost"]);
  assert.equal(linkState(), "poll");
});

// ---- skew smoothing ------------------------------------------------------

test("the first sample is taken whole -- there is nothing to smooth against", () => {
  assert.equal(smoothOffset(null, 4_200), 4_200);
});

test("a later sample eases a quarter of the way, so latency noise cannot twitch the clock", () => {
  assert.equal(smoothOffset(1_000, 2_000), 1_250);
  assert.equal(smoothOffset(1_000, 0), 750);
  assert.equal(smoothOffset(-2_000, -2_000), -2_000);
});

test("a step larger than the resync threshold snaps instead of easing", () => {
  // Waking from sleep or an NTP correction is not latency: easing through
  // it would leave every timestamp on the page minutes wrong for minutes.
  assert.equal(smoothOffset(0, 60_000), 60_000);
  assert.equal(smoothOffset(0, -60_000), -60_000);
  // Just inside the threshold still eases.
  assert.equal(smoothOffset(0, 4_000), 1_000);
});

// ---- serverNow -----------------------------------------------------------

test("an unparseable server_time is dropped rather than folded in as NaN", () => {
  noteHeartbeat("not a timestamp");
  // Still the plain local clock: exactly as right as the page was before
  // the live link existed, and finite.
  assert.ok(Math.abs(serverNow() - Date.now()) <= 5);
});

test("serverNow tracks the heartbeat offset, smoothing the second sample", () => {
  noteHeartbeat(new Date(Date.now() + 60_000).toISOString());
  assert.ok(Math.abs(serverNow() - Date.now() - 60_000) <= 50);

  // 1s further out, inside the resync threshold, so the offset moves a
  // quarter of the way (~60_250) rather than jumping to the sample.
  noteHeartbeat(new Date(Date.now() + 61_000).toISOString());
  const offset = serverNow() - Date.now();
  assert.ok(offset > 60_100 && offset < 60_500, `offset eased too far or not at all: ${offset}`);
});
