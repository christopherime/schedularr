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
};
(globalThis as unknown as GlobalStub).window = { setInterval: () => 0 };

const require = createRequire(import.meta.url);
const cronstrueUMD: { default?: unknown } = require("../assets/vendor/cronstrue.min.js");
(globalThis as unknown as GlobalStub).cronstrue = cronstrueUMD.default ?? cronstrueUMD;

// Dynamic import so the global stubs above are in place before the
// module's top-level registration code runs. cronReadback lives in the
// shared runtime since v0.5.1 (the guide inspector reads it too).
const { swapAdjacent, buildCronFromSimple, parseCronToSimple, buildSpec, formFromSpec } =
  await import("../assets/ts/pages/blocks.ts");
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
