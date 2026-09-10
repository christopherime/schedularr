// Unit tests for the blocks page's exported logic, on Node's built-in
// test runner with native type stripping -- no test framework dependency.
// Run via `make web-test` (or `npm test` in web/); CI runs it in the web
// job.
//
// Page modules' top-level side effects are registering an alpine:init
// listener and initShell() (which no-ops against the stubs below), so a
// small document/window stub is all the DOM they need. cronstrue is the
// same vendored UMD bundle the page loads via a script tag
// (web/assets/vendor/cronstrue.min.js), exposed here as the global the
// module's `declare const cronstrue` expects.
import assert from "node:assert/strict";
import { createRequire } from "node:module";
import test from "node:test";

type GlobalStub = { document: unknown; window: unknown; cronstrue: unknown };
(globalThis as unknown as GlobalStub).document = {
  addEventListener() {},
  getElementById: () => null,
  querySelector: () => null,
};
(globalThis as unknown as GlobalStub).window = { setInterval: () => 0 };

const require = createRequire(import.meta.url);
const cronstrueUMD: { default?: unknown } = require("../assets/vendor/cronstrue.min.js");
(globalThis as unknown as GlobalStub).cronstrue = cronstrueUMD.default ?? cronstrueUMD;

// Dynamic import so the global stubs above are in place before the
// module's top-level registration code runs. cronReadback lives in the
// shared runtime since v0.5.1 (the guide inspector reads it too).
const {
  swapAdjacent,
  buildCronFromSimple,
  parseCronToSimple,
  buildSpec,
  formFromSpec,
  nextReading,
  darkUntilLabel,
  priorityLabel,
} = await import("../assets/ts/pages/blocks.ts");
const { cronReadback } = await import("../assets/ts/runtime/cron.ts");

test("cronReadback describes a valid cron in plain language", () => {
  const text = cronReadback("30 20 * * 6");
  assert.ok(text, "expected a non-null readback for a valid cron");
  assert.match(text, /Saturday/);
});

test("cronReadback returns null for blank input", () => {
  assert.equal(cronReadback(""), null);
  assert.equal(cronReadback("   "), null);
});

test("cronReadback returns null for unparseable input", () => {
  assert.equal(cronReadback("not a cron"), null);
  assert.equal(cronReadback("99 99 * * *"), null);
});

test("swapAdjacent swaps a row with its neighbor in place", () => {
  const up = ["a", "b", "c"];
  swapAdjacent(up, 1, -1);
  assert.deepEqual(up, ["b", "a", "c"]);

  const down = ["a", "b", "c"];
  swapAdjacent(down, 1, 1);
  assert.deepEqual(down, ["a", "c", "b"]);
});

test("swapAdjacent is a no-op past either end", () => {
  const arr = ["a", "b", "c"];
  swapAdjacent(arr, 0, -1); // first row up
  swapAdjacent(arr, 2, 1); // last row down
  swapAdjacent(arr, -1, 1); // out of range
  swapAdjacent(arr, 3, -1); // out of range
  assert.deepEqual(arr, ["a", "b", "c"]);
});

// ---- schedule picker: cron round-trip ------------------------------------
//
// buildCronFromSimple and parseCronToSimple are documented exact inverses
// for every shape the picker itself can produce -- pin that contract.

test("cron round-trips through the picker for every frequency", () => {
  const cases = [
    { frequency: "daily" as const, daysOfWeek: [], dayOfMonth: "1", time: "06:30" },
    { frequency: "weekdays" as const, daysOfWeek: [], dayOfMonth: "1", time: "20:00" },
    { frequency: "weekly" as const, daysOfWeek: [6], dayOfMonth: "1", time: "21:00" },
    { frequency: "custom" as const, daysOfWeek: [1, 3, 5], dayOfMonth: "1", time: "09:15" },
    { frequency: "monthly" as const, daysOfWeek: [], dayOfMonth: "15", time: "00:00" },
  ];
  for (const simple of cases) {
    const cron = buildCronFromSimple(simple);
    const parsed = parseCronToSimple(cron);
    assert.ok(parsed, `expected ${cron} to parse back into Simple mode`);
    assert.equal(buildCronFromSimple(parsed), cron, `round-trip changed ${simple.frequency}`);
    assert.equal(parsed.frequency, simple.frequency);
  }
});

test("parseCronToSimple refuses unrepresentable expressions", () => {
  assert.equal(parseCronToSimple("0 12 1 * 1"), null, "day-of-month + weekday together");
  assert.equal(parseCronToSimple("0 12 * 2 *"), null, "month restriction");
  assert.equal(parseCronToSimple("*/5 * * * *"), null, "step on minute");
  assert.equal(parseCronToSimple("0 8-10 * * *"), null, "range on hour");
  assert.equal(parseCronToSimple("not a cron"), null);
});

// ---- buildSpec / formFromSpec round-trip ---------------------------------

test("a filter spec survives formFromSpec -> buildSpec unchanged", () => {
  const spec = {
    type: "filter" as const,
    name: "Spooky Saturday Night",
    cron: "0 21 * * 6",
    duration: 180,
    channel_id: "channel-1",
    priority: 50,
    filter: { genres: ["Horror"], year_from: 1978 },
  };
  assert.deepEqual(buildSpec(formFromSpec(spec, true)), spec);
});

test("a series spec survives formFromSpec -> buildSpec unchanged", () => {
  const spec = {
    type: "series" as const,
    name: "Morning Cartoons",
    cron: "0 6 * * *",
    duration: 120,
    channel_id: "channel-2",
    series: [
      {
        show_title: "Batman: The Animated Series",
        episodes_per_block: 4,
        on_complete: "restart" as const,
        skip_episodes: ["S01E02"],
      },
    ],
    fallback: { mode: "redistribute" as const },
  };
  assert.deepEqual(buildSpec(formFromSpec(spec, true)), spec);
});

// ---- the list row's readings (v0.5.10 block power tools) -----------------
//
// NEXT, the DARK UNTIL chip and the priority line are pure functions taking
// `now`, which is the only reason they can be tested here at all: the
// runner has no DOM, and the page passes serverNow() (the heartbeat-
// corrected clock) at every call site.

const T0 = Date.parse("2026-09-10T12:00:00Z");
const iso = (msOffset: number): string => new Date(T0 + msOffset).toISOString();

const row = (over: Partial<{ enabled: boolean; next_occurrence: string; cron: string }> = {}) => ({
  enabled: over.enabled ?? true,
  next_occurrence: over.next_occurrence,
  spec: { cron: over.cron ?? "0 21 * * 6" },
});

test("NEXT reads an instant as a countdown plus its local date", () => {
  const reading = nextReading(row({ next_occurrence: iso(3 * 3_600_000) }), T0);
  assert.equal(reading.kind, "instant");
  assert.equal(reading.primary, "in 3 h");
  assert.ok(reading.detail.length > 0, "expected an absolute instant under the countdown");
});

test("NEXT reads an occurrence already under way as due, never as elapsed", () => {
  // The server computes next_occurrence before the block starts airing, so
  // a fresh row can carry a past instant. "12 min ago" would read as a
  // missed airing rather than one in progress.
  const reading = nextReading(row({ next_occurrence: iso(-12 * 60_000) }), T0);
  assert.equal(reading.primary, "due");
});

test("NEXT distinguishes all three ways an instant can be absent", () => {
  const disabled = nextReading(row({ enabled: false }), T0);
  assert.equal(disabled.kind, "disabled");

  const unreadable = nextReading(row({ cron: "not a cron" }), T0);
  assert.equal(unreadable.kind, "unreadable");

  const never = nextReading(row({ cron: "0 21 * * 6" }), T0);
  assert.equal(never.kind, "never");

  const primaries = new Set([disabled.primary, unreadable.primary, never.primary]);
  assert.equal(primaries.size, 3, "each absence must read as its own state, not a shared em dash");
});

test("NEXT reads a disabled block as disabled even when its cron is also broken", () => {
  // Turning it on is the step that comes first, and the Status column
  // beside it already says the same thing.
  assert.equal(nextReading(row({ enabled: false, cron: "not a cron" }), T0).kind, "disabled");
});

test("DARK UNTIL paints only while the wake-up is still ahead", () => {
  assert.ok(darkUntilLabel(iso(2 * 86_400_000), T0), "a future window is a live dark state");
  assert.equal(darkUntilLabel(iso(-1), T0), null, "a passed window suppresses nothing");
  assert.equal(darkUntilLabel(undefined, T0), null);
  assert.equal(darkUntilLabel("not a date", T0), null);
});

// priorityRank itself is pinned by rank.test.ts; these cover the call
// site's own rule -- who gets a rank printed at all.

const peer = (priority: number, enabled = true) => ({ enabled, spec: { priority } });

test("priority prints a rank only for a block that is actually contending", () => {
  const peers = [peer(80), peer(50), peer(20)];
  const contender = { enabled: true, spec: { priority: 50 } };
  assert.equal(priorityLabel(contender, peers, T0), "PRI 50 · 2nd of 3");

  const off = { enabled: false, spec: { priority: 50 } };
  assert.equal(priorityLabel(off, peers, T0), "PRI 50");

  const dark = { enabled: true, disabled_until: iso(86_400_000), spec: { priority: 50 } };
  assert.equal(priorityLabel(dark, peers, T0), "PRI 50");

  // A dark window that already lapsed is not dark: the block is back in
  // the field and ranks again.
  const woken = { enabled: true, disabled_until: iso(-86_400_000), spec: { priority: 50 } };
  assert.equal(priorityLabel(woken, peers, T0), "PRI 50 · 2nd of 3");
});

test("priority prints the bare number when there is no field to rank against", () => {
  const contender = { enabled: true, spec: { priority: 50 } };
  assert.equal(priorityLabel(contender, [], T0), "PRI 50", "blocks fetch failed");
  assert.equal(priorityLabel(contender, [peer(80, false)], T0), "PRI 50", "every peer disabled");
});

test("priority defaults an unset priority to zero, as the engine does", () => {
  assert.equal(priorityLabel({ enabled: true, spec: {} }, [], T0), "PRI 0");
});
