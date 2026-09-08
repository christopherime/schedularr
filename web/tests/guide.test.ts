// Unit tests for the guide page's exported logic, on Node's built-in
// test runner with native type stripping -- no test framework dependency.
// Run via `make web-test` (or `npm test` in web/); CI runs it in the web
// job.
//
// The page module's top-level side effects are registering an
// alpine:init listener and initShell() (which no-ops against the stubs
// below), so a small document/window stub is all the DOM it needs. The
// draft state machine itself lives inside the Alpine component and is
// exercised in the browser; what is pinned here is the pure logic the
// component delegates to -- the SCOPE readout every line of draft copy
// names, and the channel order a diffed sheet renders in.
import assert from "node:assert/strict";
import test from "node:test";

import type { Channel } from "../assets/ts/runtime/channels.ts";
import type { GuideRow } from "../assets/ts/runtime/grid.ts";

type GlobalStub = { document: unknown; window: unknown };
(globalThis as unknown as GlobalStub).document = {
  addEventListener() {},
  getElementById: () => null,
  querySelector: () => null,
};
(globalThis as unknown as GlobalStub).window = { setInterval: () => 0 };

// Dynamic import so the global stubs above are in place before the
// module's top-level registration code runs.
const { orderRows, scopeLabelText } = await import("../assets/ts/pages/guide.ts");

const channels: Channel[] = [
  { id: "ch-horror", name: "Horror", number: 4 },
  { id: "ch-toons", name: "Toons", number: 2 },
];

function row(channelId: string): GuideRow {
  return { channelId, plate: { ch: null, name: channelId }, slots: [] };
}

test("scopeLabelText names the whole station for the empty scope", () => {
  assert.equal(scopeLabelText("", channels), "ALL channels");
});

test("scopeLabelText reads a resolved channel as its plate text", () => {
  assert.equal(scopeLabelText("ch-horror", channels), "CH 04 · Horror");
});

test("scopeLabelText falls back to the plate's name alone when the channel is unresolved", () => {
  // No number to print, and no fabricated name: the shortened raw id.
  assert.equal(scopeLabelText("ch-unknown", channels), "ch-unknown");
  assert.equal(scopeLabelText("01234567-89ab-cdef-0123-456789abcdef", channels), "01234567…");
});

test("orderRows puts a reading-only row back in channel order", () => {
  // diffRows returns the draft's channels first and the reading-only
  // ones after: an all-removed channel arrives last however it sorts.
  const diffed = [row("ch-horror"), row("ch-toons")];
  assert.deepEqual(
    orderRows(diffed, channels).map((r) => r.channelId),
    ["ch-toons", "ch-horror"],
  );
});

test("orderRows leaves its input untouched", () => {
  const diffed = [row("ch-horror"), row("ch-toons")];
  orderRows(diffed, channels);
  assert.deepEqual(
    diffed.map((r) => r.channelId),
    ["ch-horror", "ch-toons"],
  );
});
