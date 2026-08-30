// Minimal committed unit harness for web UI logic, on Node's built-in
// test runner with native type stripping -- no test framework dependency.
// Run via `make web-test` (or `npm test` in web/); CI runs it in the web
// job. This replaces the throwaway verification scripts the picker and
// reorder logic were originally checked with, so UI logic tests have a
// committed place to accumulate.
//
// Page modules' only top-level side effect is registering an alpine:init
// listener, so a one-line document stub is all the DOM they need.
// cronstrue is the same vendored UMD bundle the page loads via a script
// tag (web/assets/vendor/cronstrue.min.js), exposed here as the global
// the module's `declare const cronstrue` expects.
import assert from "node:assert/strict";
import { createRequire } from "node:module";
import test from "node:test";

type GlobalStub = { document: unknown; cronstrue: unknown };
(globalThis as unknown as GlobalStub).document = { addEventListener() {} };

const require = createRequire(import.meta.url);
const cronstrueUMD: { default?: unknown } = require("../assets/vendor/cronstrue.min.js");
(globalThis as unknown as GlobalStub).cronstrue = cronstrueUMD.default ?? cronstrueUMD;

// Dynamic import so the global stubs above are in place before the
// module's top-level registration code runs.
const { cronReadback, swapAdjacent } = await import("../assets/ts/pages/blocks.ts");

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
