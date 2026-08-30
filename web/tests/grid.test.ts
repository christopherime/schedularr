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
  primaryPieceIndex,
  resolveGhost,
  rundownDayHeading,
  rundownDaySlots,
  segmentEdges,
  slotFace,
  weekChunk,
  weekCornerLabel,
  weekPageCount,
  weekRangeLabel,
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

// ---- week paging (7-day chunks of the loaded window) -----------------------

test("weekPageCount pages the loaded window in sevens", () => {
  assert.equal(weekPageCount(7), 1);
  assert.equal(weekPageCount(8), 2, "the trailing partial day opens a second page");
  assert.equal(weekPageCount(14), 2);
  assert.equal(weekPageCount(28), 4);
  assert.equal(weekPageCount(29), 5);
  assert.equal(weekPageCount(1), 1);
  assert.equal(weekPageCount(0), 1, "an empty window still has page 0");
});

test("weekChunk slices page k as 7 consecutive window days, the tail kept real", () => {
  assert.deepEqual(weekChunk(0, 28), { startDay: 0, days: 7 });
  assert.deepEqual(weekChunk(1, 28), { startDay: 7, days: 7 });
  assert.deepEqual(weekChunk(3, 28), { startDay: 21, days: 7 });
  assert.deepEqual(weekChunk(4, 29), { startDay: 28, days: 1 }, "the spill day is a one-day page");
  assert.deepEqual(weekChunk(0, 3), { startDay: 0, days: 3 });
  assert.deepEqual(weekChunk(1, 7), { startDay: 7, days: 0 }, "no days past the window");
});

test("week chunking composes with windowDayCount for the guide's days=28 fetch", () => {
  // Fetched at 19:00 the 28-day window touches 29 calendar days: four
  // full week pages plus a one-day fifth -- the chevrons' edge math.
  const loaded = windowDayCount(at(19), 28);
  assert.equal(loaded, 29);
  assert.equal(weekPageCount(loaded), 5);
  assert.deepEqual(weekChunk(4, loaded), { startDay: 28, days: 1 });
  // Fetched at exactly midnight it stays four clean weeks.
  const clean = windowDayCount(day, 28);
  assert.equal(clean, 28);
  assert.equal(weekPageCount(clean), 4);
  assert.deepEqual(weekChunk(3, clean), { startDay: 21, days: 7 });
});

test("weekRangeLabel names the visible page's calendar range", () => {
  // day = SAT 2026-08-29; a full week ends FRI 2026-09-04 (month edge).
  assert.equal(weekRangeLabel(day, 7), "SAT 29 AUG – FRI 04 SEP");
  const sameMonth = new Date(2026, 7, 2, 0, 0, 0, 0).getTime();
  assert.equal(weekRangeLabel(sameMonth, 7), "SUN 02 AUG – SAT 08 AUG");
  assert.equal(weekRangeLabel(day, 1), "SAT 29 AUG", "a one-day trailing page is just its day");
});

test("weekCornerLabel reads the page's month(s)", () => {
  const aug = new Date(2026, 7, 2, 0, 0, 0, 0).getTime();
  assert.equal(weekCornerLabel(aug, 7), "AUG 2026");
  assert.equal(weekCornerLabel(day, 7), "AUG–SEP 2026", "a month edge names both");
  const dec = new Date(2026, 11, 28, 0, 0, 0, 0).getTime();
  assert.equal(weekCornerLabel(dec, 7), "DEC–JAN", "a year edge drops the ambiguous year");
});

test("dayLabel and rundownDayHeading name the windows", () => {
  assert.equal(dayLabel(day), "SAT 29");
  assert.equal(dayLabel(dayEnd), "SUN 30");
  assert.equal(rundownDayHeading(0, day), "TONIGHT");
  assert.equal(rundownDayHeading(1, dayEnd), "TOMORROW");
  assert.equal(rundownDayHeading(2, addDays(day, 2)), "MON 31");
});

// ---- segment edges: intra-week joins vs week-edge cuts ---------------------

test("segmentEdges joins a midnight crossing inside the week", () => {
  // 23:30 -> 01:30 across the day1/day2 boundary of a 7-day week:
  // the head piece joins right, the tail piece joins left -- no dashed
  // cut on either.
  const head = daySpan(at(23, 30), nextDayAt(1, 30), day, dayEnd);
  const tail = daySpan(at(23, 30), nextDayAt(1, 30), dayEnd, addDays(day, 2));
  assert.ok(head && tail);
  assert.deepEqual(segmentEdges(head, 0, 7), { joinLeft: false, joinRight: true, cutLeft: false, cutRight: false });
  assert.deepEqual(segmentEdges(tail, 1, 7), { joinLeft: true, joinRight: false, cutLeft: false, cutRight: false });
});

test("segmentEdges keeps dashed cuts only at the week's outer edges", () => {
  // The same crossing at the week's LAST midnight: the head piece sits
  // on day 6 and its spill leaves the page -> a dashed right cut.
  const head = daySpan(at(23, 30), nextDayAt(1, 30), day, dayEnd);
  assert.ok(head);
  assert.deepEqual(segmentEdges(head, 6, 7), { joinLeft: false, joinRight: false, cutLeft: false, cutRight: true });
  // And a slot spilling IN from the previous week cuts dashed at day 0.
  const tail = daySpan(at(23, 30) - 86_400_000, at(1, 30), day, dayEnd);
  assert.ok(tail);
  assert.deepEqual(segmentEdges(tail, 0, 7), { joinLeft: false, joinRight: false, cutLeft: true, cutRight: false });
});

test("primaryPieceIndex labels the widest piece, ties to the earlier one", () => {
  // 23:30 -> 06:00: the 30-minute sliver loses the label to the
  // six-hour morning piece.
  const head = daySpan(at(23, 30), nextDayAt(6), day, dayEnd);
  const tail = daySpan(at(23, 30), nextDayAt(6), dayEnd, addDays(day, 2));
  assert.ok(head && tail);
  assert.equal(primaryPieceIndex([head, tail]), 1);
  // 22:00 -> 02:00: equal two-hour halves -- a tie goes to the earlier
  // piece.
  const evenHead = daySpan(at(22), nextDayAt(2), day, dayEnd);
  const evenTail = daySpan(at(22), nextDayAt(2), dayEnd, addDays(day, 2));
  assert.ok(evenHead && evenTail);
  assert.equal(primaryPieceIndex([evenHead, evenTail]), 0);
  assert.equal(primaryPieceIndex([evenHead]), 0, "a single piece is its own primary");
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
