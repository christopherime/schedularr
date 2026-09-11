// Unit tests for the History page's pure helpers: pane routing off the
// query string, the tracked-row filter, local-day bucketing of airings,
// and the run card's summary line. These are the parts a rendering bug
// would silently get wrong, so they are pinned away from the DOM.
//
// The page module's top-level side effects are registering an
// alpine:init listener and initShell() (which no-ops against the stubs
// below), so a small document/window stub is all the DOM it needs --
// the same shape blocks.test.ts and guide.test.ts use.
import assert from "node:assert/strict";
import test from "node:test";

type GlobalStub = { document: unknown; window: unknown };
(globalThis as unknown as GlobalStub).document = {
  addEventListener() {},
  getElementById: () => null,
  querySelector: () => null,
};
(globalThis as unknown as GlobalStub).window = {
  setInterval: () => 0,
  location: { search: "" },
  history: { replaceState() {} },
};

// Dynamic import so the stubs above are in place before the module's
// top-level registration code runs.
const { PANES, filterTracked, groupByDay, paneFromSearch, runSummary } =
  await import("../assets/ts/pages/history.ts");

// ---- pane routing ---------------------------------------------------------

test("paneFromSearch reads a deep link", () => {
  assert.equal(paneFromSearch("?view=runs"), "runs");
  assert.equal(paneFromSearch("?view=asrun&block=News"), "asrun");
  assert.equal(paneFromSearch("?view=tracked"), "tracked");
});

test("paneFromSearch falls back to tracked rather than a blank page", () => {
  assert.equal(paneFromSearch(""), "tracked");
  assert.equal(paneFromSearch("?view="), "tracked");
  assert.equal(paneFromSearch("?view=nonsense"), "tracked");
  assert.equal(paneFromSearch("?days=7"), "tracked");
  assert.equal(PANES[0], "tracked");
});

// ---- tracked filter -------------------------------------------------------

test("filterTracked matches title case-insensitively", () => {
  const rows = [{ show_title: "Supernatural" }, { show_title: "The Wire" }];
  assert.deepEqual(filterTracked(rows, "wire").map((r) => r.show_title), ["The Wire"]);
  assert.deepEqual(filterTracked(rows, "SUPER").map((r) => r.show_title), ["Supernatural"]);
});

test("an empty search is a filter that matches everything", () => {
  const rows = [{ show_title: "Supernatural" }, { show_title: "The Wire" }];
  assert.equal(filterTracked(rows, "").length, 2);
  assert.equal(filterTracked(rows, "   ").length, 2);
  assert.equal(filterTracked(rows, "zzz").length, 0);
});

// ---- as-run day grouping --------------------------------------------------

test("groupByDay buckets by local day, newest day first, air order within", () => {
  // Local noon-ish stamps, so the assertion can't be flipped by the
  // runner's timezone the way a midnight-adjacent UTC stamp would be.
  const entries = [
    { scheduled_at: "2026-09-08T12:00:00", title: "A" },
    { scheduled_at: "2026-09-08T14:30:00", title: "B" },
    { scheduled_at: "2026-09-07T13:00:00", title: "C" },
  ];
  const groups = groupByDay(entries);
  assert.equal(groups.length, 2);
  assert.equal(groups[0]?.day, "2026-09-08");
  assert.deepEqual(groups[0]?.entries.map((e) => e.title), ["A", "B"]);
  assert.deepEqual(groups[1]?.entries.map((e) => e.title), ["C"]);
});

test("groupByDay drops rows it cannot place rather than inventing a day", () => {
  assert.deepEqual(groupByDay([]), []);
  assert.deepEqual(groupByDay([{ scheduled_at: undefined, title: "no stamp" }]), []);
  assert.deepEqual(groupByDay([{ scheduled_at: "not-a-date", title: "bad" }]), []);
});

// ---- run summary ----------------------------------------------------------

test("runSummary reads counts, window and scope", () => {
  assert.equal(
    runSummary({ status: "ok", scope: "", days: 7, channel_count: 3, slot_count: 14 }),
    "14 SLOTS ACROSS 3 CHANNELS · 7 DAYS · ALL CHANNELS",
  );
  assert.equal(
    runSummary({ status: "ok", scope: "ch-1", days: 1, channel_count: 1, slot_count: 1 }),
    "1 SLOT ACROSS 1 CHANNEL · 1 DAY · SCOPED",
  );
});

test("a run that never finished reports what it attempted, not zero counts", () => {
  // Zeros here would read as "applied nothing" rather than "never got
  // that far", and the outcome word beside the timestamp already says
  // how the run ended.
  assert.equal(runSummary({ status: "error", days: 1, channel_count: 0, slot_count: 0 }), "1 DAY · ALL CHANNELS");
  assert.equal(runSummary({ status: "running", days: 7 }), "7 DAYS · ALL CHANNELS");
  assert.equal(runSummary({ status: "error", days: 7, scope: "ch-1" }), "7 DAYS · SCOPED");
});

test("runSummary tolerates a run whose optional counts are absent", () => {
  assert.equal(runSummary({ status: "ok" }), "0 SLOTS ACROSS 0 CHANNELS · 0 DAYS · ALL CHANNELS");
});

// ---- the deletion desk ----------------------------------------------------

const { blocksListingShow, instantFromLocalInput, localInputFromInstant, removalConfirmBody } =
  await import("../assets/ts/pages/history.ts");

test("blocksListingShow names only the blocks that list the show", () => {
  const blocks = [
    { name: "Anime Night", spec: { series: [{ show_title: "Bloody Mary" }] } },
    { name: "Late Movie", spec: { series: [{ show_title: "Other" }] } },
    { name: "Horror Hour", spec: {} }, // a selection block lists nothing by title
  ];
  assert.deepEqual(blocksListingShow(blocks, "Bloody Mary"), ["Anime Night"]);
  assert.deepEqual(blocksListingShow(blocks, "Nobody"), []);
});

test("blocksListingShow tolerates a failed block read", () => {
  // A failed /blocks read leaves the list empty, which offers the removal
  // and lets the server's own refusal be the guard -- never the reverse.
  assert.deepEqual(blocksListingShow([], "Bloody Mary"), []);
});

test("instantFromLocalInput reads a picker value, or nothing", () => {
  const iso = instantFromLocalInput("2026-05-01T20:30");
  assert.ok(iso, "a filled field parses");
  assert.equal(new Date(iso).getMinutes(), 30);

  assert.equal(instantFromLocalInput(""), undefined, "a blank field is an open end");
  assert.equal(instantFromLocalInput("   "), undefined);
  assert.equal(instantFromLocalInput("not a date"), undefined);
});

test("localInputFromInstant round-trips through the picker format", () => {
  const iso = instantFromLocalInput("2026-05-01T20:30");
  assert.equal(localInputFromInstant(iso), "2026-05-01T20:30");
  assert.equal(localInputFromInstant(undefined), "");
  assert.equal(localInputFromInstant("nonsense"), "");
});

test("removalConfirmBody names the real counts and says it cannot be undone", () => {
  const body = removalConfirmBody(
    { airings: 12, series_states: 1, snapshots: 3, emptied_slots: 0, dry_run: true },
    "Bloody Mary",
  );
  assert.match(body, /12 airings/);
  assert.match(body, /1 tracked cursor/);
  assert.match(body, /3 snapshots/);
  assert.match(body, /cannot be undone/);
});

test("removalConfirmBody explains an emptied occurrence rather than hiding it", () => {
  const body = removalConfirmBody(
    { airings: 4, series_states: 0, snapshots: 0, emptied_slots: 2, dry_run: true },
    "this range",
  );
  assert.match(body, /2 occurrences will be left marked as having aired nothing/);
  assert.doesNotMatch(body, /tracked cursor/, "a range touches no cursors, so it claims none");
});

test("removalConfirmBody singularises one of everything", () => {
  const body = removalConfirmBody(
    { airings: 1, series_states: 1, snapshots: 1, emptied_slots: 1, dry_run: true },
    "X",
  );
  assert.match(body, /1 airing\b/);
  assert.match(body, /1 occurrence will be/);
});
