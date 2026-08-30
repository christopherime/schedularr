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
  FACE_MAX_LINES,
  QUANTA_PER_DAY,
  addDays,
  dayLabel,
  daySpan,
  gridColumn,
  localDayStart,
  nearestSlotIndex,
  resolveGhost,
  rundownDayHeading,
  rundownDaySlots,
  slotFace,
  weekPageCount,
  weekPageDays,
  weekPageOf,
  windowDayCount,
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

test("windowDayCount includes the trailing partial calendar day", () => {
  // The server plans [fetch, fetch + days*24h): a 19:00 fetch spills
  // into an extra calendar day that needs its own tab/section.
  assert.equal(windowDayCount(at(19), 7), 8);
  assert.equal(windowDayCount(at(19), 1), 2);
  // A fetch at exactly local midnight stays at `days`.
  assert.equal(windowDayCount(day, 7), 7);
  assert.equal(windowDayCount(day, 1), 1);
  // One millisecond past midnight already touches the next day.
  assert.equal(windowDayCount(day + 1, 1), 2);
});

// ---- week paging -----------------------------------------------------------

test("weekPageCount pages the loaded window in sevens", () => {
  assert.equal(weekPageCount(7), 1);
  assert.equal(weekPageCount(8), 2, "the trailing partial day opens a second page");
  assert.equal(weekPageCount(14), 2);
  assert.equal(weekPageCount(30), 5);
  assert.equal(weekPageCount(31), 5);
  assert.equal(weekPageCount(1), 1);
  assert.equal(weekPageCount(0), 1, "an empty window still has page 0");
});

test("weekPageCount composes with windowDayCount", () => {
  // DAYS=30 fetched at 19:00 touches 31 calendar days -> 5 pages,
  // the last holding 3 tabs (days 28..30).
  const loaded = windowDayCount(at(19), 30);
  assert.equal(loaded, 31);
  assert.equal(weekPageCount(loaded), 5);
  assert.deepEqual(weekPageDays(4, loaded), [28, 29, 30]);
});

test("weekPageDays returns page k's day indices, partial last week clamped", () => {
  assert.deepEqual(weekPageDays(0, 8), [0, 1, 2, 3, 4, 5, 6]);
  assert.deepEqual(weekPageDays(1, 8), [7], "a partial week holds its real tabs, not padded ones");
  assert.deepEqual(weekPageDays(0, 3), [0, 1, 2]);
  assert.deepEqual(weekPageDays(1, 7), [], "no page past the window");
});

test("chevron disable edges: page 0 has no previous, the last page no next", () => {
  const loaded = 15; // 2 full weeks + 1 day -> 3 pages
  const pages = weekPageCount(loaded);
  assert.equal(pages, 3);
  // The guide disables ‹ at page 0 and › at pages-1; the math those
  // bindings rest on:
  assert.equal(weekPageOf(0), 0);
  assert.equal(weekPageOf(6), 0);
  assert.equal(weekPageOf(7), 1);
  assert.equal(weekPageOf(14), 2);
  assert.ok(weekPageOf(loaded - 1) === pages - 1, "the window's last day lives on the last page");
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

// ---- rundown day grouping --------------------------------------------------

function mkSlot(startMs: number, endMs: number, name = "Slot") {
  return {
    kind: "slot" as const,
    channelId: "chan-1",
    blockName: name,
    blockType: "filter",
    cron: "",
    priority: 0,
    startMs,
    endMs,
    programs: [],
  };
}

test("rundownDaySlots lists an overnight slot under both days, second as continuation", () => {
  // 23:00 -> 01:00 next day, plus a plain evening slot.
  const overnight = mkSlot(at(23), nextDayAt(1), "Graveyard Shift");
  const evening = mkSlot(at(19), at(21), "Prime Time");
  const slots = [evening, overnight];
  const day0 = rundownDaySlots(slots, day, dayEnd);
  assert.deepEqual(
    day0.map((e) => [e.slot.blockName, e.continuation]),
    [["Prime Time", false], ["Graveyard Shift", false]],
  );
  const nextEnd = new Date(2026, 7, 31, 0, 0, 0, 0).getTime();
  const day1 = rundownDaySlots(slots, dayEnd, nextEnd);
  assert.deepEqual(
    day1.map((e) => [e.slot.blockName, e.continuation]),
    [["Graveyard Shift", true]],
  );
});

test("rundownDaySlots keeps the half-open day window", () => {
  // Ending exactly at midnight belongs only to the start day; starting
  // exactly at midnight belongs only to the new day (matches daySpan).
  const toMidnight = mkSlot(at(22), dayEnd);
  const fromMidnight = mkSlot(dayEnd, nextDayAt(2));
  const nextEnd = new Date(2026, 7, 31, 0, 0, 0, 0).getTime();
  assert.equal(rundownDaySlots([toMidnight], day, dayEnd).length, 1);
  assert.equal(rundownDaySlots([toMidnight], dayEnd, nextEnd).length, 0);
  assert.equal(rundownDaySlots([fromMidnight], day, dayEnd).length, 0);
  const entries = rundownDaySlots([fromMidnight], dayEnd, nextEnd);
  assert.equal(entries.length, 1);
  assert.equal(entries[0].continuation, false);
});

// ---- slot faces ------------------------------------------------------------

function mkProgram(title: string, season?: number, episode?: number) {
  return { title, season, episode, durationMs: 30 * 60_000, startMs: at(21) };
}

function seriesSlot(programs: ReturnType<typeof mkProgram>[]) {
  return { ...mkSlot(at(21), at(23), "Spooky Saturday Night"), blockType: "series", programs };
}

const wideSpan = { startQ: 0, endQ: 24, cutLeft: false, cutRight: false }; // 2 h = 4 divisions
const narrowSpan = { startQ: 0, endQ: 12, cutLeft: false, cutRight: false }; // 1 h = 2 divisions

test("slotFace lists a series slot's programs, SHOW · SxxEyy per line", () => {
  const face = slotFace(
    seriesSlot([mkProgram("Bloody Mary", 1, 5), mkProgram("Skin", 1, 6)]),
    wideSpan,
  );
  assert.deepEqual(face, { lines: ["Bloody Mary · S01E05", "Skin · S01E06"], more: 0 });
});

test("slotFace folds a long lineup into +N more past FACE_MAX_LINES", () => {
  const face = slotFace(
    seriesSlot([
      mkProgram("Bloody Mary", 1, 5),
      mkProgram("Skin", 1, 6),
      mkProgram("Hook Man", 1, 7),
      mkProgram("Bugs", 1, 8),
    ]),
    wideSpan,
  );
  // 4 programs > 3 lines: keep FACE_MAX_LINES - 1, fold the rest.
  assert.equal(face.lines.length, FACE_MAX_LINES - 1);
  assert.deepEqual(face.lines, ["Bloody Mary · S01E05", "Skin · S01E06"]);
  assert.equal(face.more, 2);
});

test("slotFace keeps exactly FACE_MAX_LINES programs without a fold", () => {
  const face = slotFace(
    seriesSlot([mkProgram("A", 1, 1), mkProgram("B", 1, 2), mkProgram("C", 1, 3)]),
    wideSpan,
  );
  assert.equal(face.lines.length, 3);
  assert.equal(face.more, 0);
});

test("slotFace degrades narrow slots, filters, and ghosts to the summary face", () => {
  const programs = [mkProgram("Bloody Mary", 1, 5)];
  assert.deepEqual(slotFace(seriesSlot(programs), narrowSpan), { lines: [], more: 0 });
  assert.deepEqual(slotFace({ ...mkSlot(at(21), at(23)), programs }, wideSpan), { lines: [], more: 0 });
  assert.deepEqual(
    slotFace({ ...seriesSlot(programs), kind: "ghost" as const }, wideSpan),
    { lines: [], more: 0 },
  );
  assert.deepEqual(slotFace(seriesSlot([]), wideSpan), { lines: [], more: 0 });
});

test("slotFace never fabricates a marker and never prints a blank title", () => {
  const face = slotFace(seriesSlot([mkProgram("The Fog Rolls In"), mkProgram("  ")]), wideSpan);
  assert.deepEqual(face.lines, ["The Fog Rolls In", "—"]);
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

test("slotFace keeps the full face at exactly the 90-minute narrow threshold", () => {
  // FACE_MIN_DIVS*6 = 18 quanta = 90 minutes; the comparison is strict `<`,
  // so a span of exactly 18 quanta keeps its program lines and 17 degrades
  // to the name + count face.
  const slot = seriesSlot([mkProgram("Bloody Mary", 1, 5)]);
  const spanOf = (endQ: number) => ({ startQ: 0, endQ, cutLeft: false, cutRight: false });
  assert.ok(slotFace(slot, spanOf(18)).lines.length > 0, "exactly 18 quanta keeps program lines");
  assert.equal(slotFace(slot, spanOf(17)).lines.length, 0, "17 quanta degrades");
});
