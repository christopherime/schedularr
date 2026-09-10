// Unit tests for the two guards the shell puts between the live link's
// wire format and everything downstream of it (runtime/shell.ts).
//
// Both run inside the stream reader's loop, where a throw does not stay
// local: it aborts a connection that is otherwise healthy and takes every
// later frame with it. So the contract they are pinned to here is "return
// null", never "throw" -- for a body that is not JSON, and for a
// heartbeat whose payload no longer carries the field it used to.
//
// The second half of the file DOES run initShell(), against the smallest
// document stub that gets past its guards, to pin the tab-resume path --
// see the section comment there.
import assert from "node:assert/strict";
import test from "node:test";

// Installed before the import so initShell() below sees them. shell.ts
// reads the document only from inside initShell(), so the pure-function
// tests are unaffected either way.
interface StubEl {
  dataset: Record<string, string>;
  textContent: string;
  addEventListener: (type: string, cb: () => void) => void;
  removeAttribute: (name: string) => void;
  setAttribute: (name: string, value: string) => void;
}

// The LINK dot is the one element initShell() insists on: it returns early
// when that is missing (the guard that keeps the other test files from
// opening a real stream), so it is the one element worth faking.
const linkDot: StubEl = {
  dataset: {},
  textContent: "",
  addEventListener: () => {},
  removeAttribute: () => {},
  setAttribute: () => {},
};

const domListeners = new Map<string, (() => void)[]>();
const documentStub = {
  hidden: false,
  addEventListener(type: string, cb: () => void): void {
    const list = domListeners.get(type) ?? [];
    list.push(cb);
    domListeners.set(type, list);
  },
  getElementById: (id: string): StubEl | null => (id === "tele-link-dot" ? linkDot : null),
  querySelector: (): null => null,
};

interface GlobalStub {
  document: unknown;
  window: unknown;
}
(globalThis as unknown as GlobalStub).document = documentStub;
// setInterval stubbed to a no-op: the shell's 60s telemetry timer would
// otherwise hold the runner open for a minute doing nothing.
(globalThis as unknown as GlobalStub).window = { setInterval: (): number => 0 };

// GET /status is counted; GET /events hands back a body that stays open
// until its signal aborts, exactly as a real stream does.
let statusCalls = 0;
globalThis.fetch = ((url: string, init?: RequestInit) => {
  if (url.includes("/events")) {
    return Promise.resolve(
      new Response(
        new ReadableStream<Uint8Array>({
          start(controller) {
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
  }
  statusCalls += 1;
  return Promise.resolve(
    new Response(JSON.stringify({ tunarr_reachable: true }), { headers: { "Content-Type": "application/json" } }),
  );
}) as unknown as typeof fetch;

const { frameData, heartbeatTime, initShell, onResume } = await import("../assets/ts/runtime/shell.ts");

test("frameData decodes a JSON frame body", () => {
  assert.deepEqual(frameData('{"run_id":7,"source":"cron"}'), { run_id: 7, source: "cron" });
});

test("a frame body that is not JSON degrades to null rather than throwing", () => {
  // A truncated frame is the realistic case: the reader hands over
  // whatever terminated, and a proxy that cut a connection mid-write can
  // terminate a half-written payload.
  assert.equal(frameData('{"server_time":"2026-09-10T12'), null);
  assert.equal(frameData(""), null);
  assert.equal(frameData("undefined"), null);
});

test("heartbeatTime reads server_time out of a heartbeat payload", () => {
  assert.equal(heartbeatTime({ server_time: "2026-09-10T12:00:00Z" }), "2026-09-10T12:00:00Z");
});

test("heartbeatTime refuses anything that is not a server_time string", () => {
  // Each of these would otherwise reach Date.parse and fold NaN into the
  // clock offset, which no amount of later samples recovers from.
  assert.equal(heartbeatTime(null), null);
  assert.equal(heartbeatTime({}), null);
  assert.equal(heartbeatTime({ server_time: 1_757_500_000_000 }), null);
  assert.equal(heartbeatTime("2026-09-10T12:00:00Z"), null);
  assert.equal(heartbeatTime(undefined), null);
});

// ---- the tab-resume path -------------------------------------------------
//
// A hidden tab drops its stream, so it misses every event until it is
// shown again -- and the hub's resume ring only reaches back 128 events.
// Refetching on the way back is therefore not an optimisation, it is the
// only thing that makes what the tab shows true. Both halves of that
// refetch are pinned here, because both were wired and neither ran: the
// page handlers (onResume had no callers at all) and the shell's own
// /status, whose absence left the bezel holding a pre-hidden reading
// beside a LIVE legend.

initShell();
const onVisibility = domListeners.get("visibilitychange")?.[0];

/** Hides the tab and shows it again -- the transition under test. */
function hideAndShow(): void {
  assert.ok(onVisibility, "initShell must register a visibilitychange listener");
  documentStub.hidden = true;
  onVisibility();
  documentStub.hidden = false;
  onVisibility();
}

async function settle(): Promise<void> {
  for (let i = 0; i < 20; i += 1) await new Promise((resolve) => setTimeout(resolve, 5));
}

test("becoming visible again fires the registered resume handlers", async () => {
  let fired = 0;
  const off = onResume(() => {
    fired += 1;
  });
  hideAndShow();
  await settle();
  off();
  assert.equal(fired, 1);
});

test("becoming visible again refetches the shell's own /status", async () => {
  // Without this the LINK legend goes back to LIVE while TUNARR, LAST
  // APPLY and NEXT TICK still read whatever they read before the tab was
  // hidden -- a stale reading the operator cannot tell from a fresh one.
  const before = statusCalls;
  hideAndShow();
  await settle();
  assert.ok(statusCalls > before, `expected a /status refetch, got ${statusCalls - before}`);
});

test("an unsubscribed resume handler stops firing", async () => {
  let fired = 0;
  const off = onResume(() => {
    fired += 1;
  });
  off();
  hideAndShow();
  await settle();
  assert.equal(fired, 0);
});
