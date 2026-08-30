// Unit tests for the schedule page's exported logic. Same stub story as
// blocks.test.ts: the page's top-level side effects are an alpine:init
// registration and initShell(), both inert against these stubs.
import assert from "node:assert/strict";
import test from "node:test";

type GlobalStub = { document: unknown; window: unknown };
(globalThis as unknown as GlobalStub).document = {
  addEventListener() {},
  getElementById: () => null,
};
(globalThis as unknown as GlobalStub).window = { setInterval: () => 0 };

const { clampDays } = await import("../assets/ts/pages/schedule.ts");

test("clampDays clamps to the API's [1, 30] range", () => {
  assert.equal(clampDays("7"), 7);
  assert.equal(clampDays("1"), 1);
  assert.equal(clampDays("30"), 30);
  assert.equal(clampDays("0"), 1);
  assert.equal(clampDays("-4"), 1);
  assert.equal(clampDays("45"), 30);
});

test("clampDays defaults blank/non-finite input to 7 and rounds", () => {
  assert.equal(clampDays(""), 7);
  assert.equal(clampDays("  "), 7);
  assert.equal(clampDays("abc"), 7);
  assert.equal(clampDays("6.6"), 7);
  assert.equal(clampDays("2.2"), 2);
});
