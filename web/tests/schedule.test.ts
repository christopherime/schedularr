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

const { channelOrder } = await import("../assets/ts/pages/schedule.ts");

test("channelOrder sorts sections by resolved channel number, not raw id", () => {
  const channels = [
    { id: "zzz-uuid", name: "Horror", number: 4 },
    { id: "aaa-uuid", name: "Sitcoms", number: 12 },
  ];
  // Raw-id order would put aaa-uuid first; the plates read CH 04 before CH 12.
  assert.ok(channelOrder("zzz-uuid", "aaa-uuid", channels) < 0);
  assert.ok(channelOrder("aaa-uuid", "zzz-uuid", channels) > 0);
});

test("channelOrder falls back to name, then id, and sorts unresolved last", () => {
  const channels = [
    { id: "num-1", name: "Horror", number: 4 },
    { id: "name-b", name: "Beta" },
    { id: "name-a", name: "Alpha" },
  ];
  // Numbered before unnumbered.
  assert.ok(channelOrder("num-1", "name-a", channels) < 0);
  // Both unnumbered: by name.
  assert.ok(channelOrder("name-a", "name-b", channels) < 0);
  // Unresolvable ids (not in the cache): by raw id as the last resort.
  assert.ok(channelOrder("unknown-a", "unknown-b", channels) < 0);
  // Unresolved sorts after a numbered channel.
  assert.ok(channelOrder("unknown-a", "num-1", channels) > 0);
});
