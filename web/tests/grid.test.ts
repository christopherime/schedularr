// Unit tests for the guide grid's pure geometry (runtime/grid.ts):
// time->quantum clamping (overnight spill included), slot->grid-column
// mapping, day windowing, ghost placement, and the keyboard-nav picking
// logic. The DAYS clamp itself (clampDays) is covered in runtime.test.ts
// with the other format helpers. The geometry section of grid.ts never
// touches the DOM, so no stubs are needed; all instants are built with
// local-time Date constructors so the assertions hold in any timezone.
import assert from "node:assert/strict";
import test from "node:test";

const {
  QUANTA_PER_DAY,
  addDays,
  dayLabel,
  daySpan,
  gridColumn,
  localDayStart,
  nearestSlotIndex,
  resolveGhost,
  rundownDayHeading,
} = await import("../assets/ts/runtime/grid.ts");

// A fixed local day: Saturday 2026-08-29 (month is 0-based).
const day = new Date(2026, 7, 29, 0, 0, 0, 0).getTime();
const dayEnd = new Date(2026, 7, 30, 0, 0, 0, 0).getTime();
const at = (h: number, m = 0) => new Date(2026, 7, 29, h, m, 0, 0).getTime();
const nextDayAt = (h: number, m = 0) => new Date(2026, 7, 30, h, m, 0, 0).getTime();

// ---- time -> quantum clamping ----------------------------------------------

test("daySpan maps a same-day slot onto 5-minute quanta", () => {
  // 21:00-23:00 -> quanta 252..276 (21h*12 .. 23h*12), no cuts.
  const span = daySpan(at(21), at(23), day, dayEnd);
  assert.ok(span);
  assert.deepEqual(span, { startQ: 252, endQ: 276, cutLeft: false, cutRight: false });
});

test("daySpan floors the start and ceils the end to the quantum", () => {
  // 06:02-06:12 -> start floors to 06:00 (q72), end ceils to 06:15 (q75).
  const span = daySpan(at(6, 2), at(6, 12), day, dayEnd);
  assert.ok(span);
  assert.equal(span.startQ, 72);
  assert.equal(span.endQ, 75);
});

test("daySpan keeps a degenerate sliver visible at one quantum", () => {
  const span = daySpan(at(12), at(12, 1), day, dayEnd);
  assert.ok(span);
  assert.equal(span.endQ - span.startQ, 1);
});

test("daySpan returns null for a slot that never touches the day", () => {
  assert.equal(daySpan(nextDayAt(6), nextDayAt(8), day, dayEnd), null);
  assert.equal(daySpan(at(6) - 86_400_000, at(8) - 86_400_000, day, dayEnd), null);
  // Touching exactly at the boundary is not on this day.
  assert.equal(daySpan(dayEnd, nextDayAt(2), day, dayEnd), null);
});

test("daySpan cuts an overnight spill at the right edge of its start day", () => {
  // 23:30 -> 01:30 next day: on day 0 it is clamped to 23:30-24:00.
  const span = daySpan(at(23, 30), nextDayAt(1, 30), day, dayEnd);
  assert.ok(span);
  assert.deepEqual(span, { startQ: 282, endQ: QUANTA_PER_DAY, cutLeft: false, cutRight: true });
});

test("daySpan cuts the same spill at the left edge of the next day", () => {
  const nextEnd = new Date(2026, 7, 31, 0, 0, 0, 0).getTime();
  const span = daySpan(at(23, 30), nextDayAt(1, 30), dayEnd, nextEnd);
  assert.ok(span);
  assert.deepEqual(span, { startQ: 0, endQ: 18, cutLeft: true, cutRight: false });
});

// ---- slot -> grid-column mapping -------------------------------------------

test("gridColumn emits 1-based grid line numbers", () => {
  const span = daySpan(at(21), at(23), day, dayEnd);
  assert.ok(span);
  assert.equal(gridColumn(span), "253 / 277");
  const full = daySpan(day, dayEnd, day, dayEnd);
  assert.ok(full);
  assert.equal(gridColumn(full), `1 / ${QUANTA_PER_DAY + 1}`);
});

// ---- day windowing ---------------------------------------------------------

test("localDayStart and addDays walk local midnights", () => {
  assert.equal(localDayStart(at(14, 37)), day);
  assert.equal(addDays(day, 1), dayEnd);
  assert.equal(addDays(day, 0), day);
  // Windowing: day k of a 7-day window owns exactly [start, nextStart).
  const k2 = addDays(day, 2);
  const k3 = addDays(day, 3);
  assert.ok(daySpan(k2 + 3_600_000, k2 + 7_200_000, k2, k3), "a day-2 slot lands in day 2's window");
  assert.equal(daySpan(k2 + 3_600_000, k2 + 7_200_000, day, dayEnd), null, "and not in day 0's");
});

test("dayLabel and rundownDayHeading name the windows", () => {
  assert.equal(dayLabel(day), "SAT 29");
  assert.equal(dayLabel(dayEnd), "SUN 30");
  assert.equal(rundownDayHeading(0, day), "TONIGHT");
  assert.equal(rundownDayHeading(1, dayEnd), "TOMORROW");
  assert.equal(rundownDayHeading(2, addDays(day, 2)), "MON 31");
});

// ---- ghost placement -------------------------------------------------------

const warning = {
  block_name: "Late Sitcom Loop",
  occurrence_start: new Date(at(21)).toISOString(),
  blocking_block_name: "Spooky Saturday Night",
};

test("resolveGhost places a warning with the losing block's channel and duration", () => {
  const blocks = new Map([
    ["Late Sitcom Loop", { channelId: "chan-1", durationMinutes: 90 }],
    ["Spooky Saturday Night", { channelId: "chan-1", durationMinutes: 120 }],
  ]);
  const ghost = resolveGhost(warning, blocks);
  assert.ok(ghost);
  assert.equal(ghost.kind, "ghost");
  assert.equal(ghost.channelId, "chan-1");
  assert.equal(ghost.startMs, at(21));
  assert.equal(ghost.endMs, at(21) + 90 * 60_000);
  assert.equal(ghost.lostTo, "Spooky Saturday Night");
});

test("resolveGhost falls back to the winner's channel and a 30-minute width", () => {
  // The losing block is unresolvable (deleted, or the blocks fetch
  // failed) -- a conflict is always same-channel, so the winner places it.
  const blocks = new Map([["Spooky Saturday Night", { channelId: "chan-1", durationMinutes: 120 }]]);
  const ghost = resolveGhost(warning, blocks);
  assert.ok(ghost);
  assert.equal(ghost.channelId, "chan-1");
  assert.equal(ghost.endMs - ghost.startMs, 30 * 60_000);
});

test("resolveGhost drops an unplaceable or unparseable warning", () => {
  assert.equal(resolveGhost(warning, new Map()), null, "neither block resolvable");
  assert.equal(
    resolveGhost({ ...warning, occurrence_start: "not a date" }, new Map([["Late Sitcom Loop", { channelId: "c", durationMinutes: 30 }]])),
    null,
  );
  assert.equal(resolveGhost({ block_name: "X" }, new Map()), null, "no occurrence_start at all");
});

// ---- keyboard nav: nearest-slot picking ------------------------------------

test("nearestSlotIndex picks the slot whose start is nearest the target", () => {
  const starts = [at(6), at(12), at(18)];
  assert.equal(nearestSlotIndex(starts, at(11)), 1);
  assert.equal(nearestSlotIndex(starts, at(5)), 0);
  assert.equal(nearestSlotIndex(starts, at(23)), 2);
});

test("nearestSlotIndex ties go to the earlier slot and empty tracks return -1", () => {
  const starts = [at(6), at(12)];
  assert.equal(nearestSlotIndex(starts, at(9)), 0, "equidistant -> earlier slot");
  assert.equal(nearestSlotIndex([], at(9)), -1);
});
