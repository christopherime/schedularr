// Unit tests for the blocks page's power-tool predicates (v0.5.10): which
// row actions may arm right now, what instant each dark-window preset
// resolves to, what the editor's consequence rail is showing, and what a
// collapsed series row says about itself.
//
// Both are pinned here rather than in the page because they are the parts
// that are wrong SILENTLY. An action that arms when it should not writes
// through an open editor's If-Match; a preset that resolves a day off
// takes a channel dark for the wrong day, and neither shows up as an
// error anywhere. Same stub shape as blocks.test.ts -- the runner has no
// DOM, so only the module's pure exports are reachable.
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

const {
  ackedRewind,
  canArmRowAction,
  cronFootgunShows,
  darkPresetInstant,
  DARK_PRESETS,
  footgunCopy,
  occurrenceLabel,
  planInvalidatedReaction,
  railSiblings,
  railState,
  rewindReport,
  seriesRowSummary,
} = await import("../assets/ts/pages/blocks.ts");
const { formatLocal } = await import("../assets/ts/runtime/format.ts");

// ---- which actions may arm ------------------------------------------------

test("both power tools arm on an idle page", () => {
  assert.equal(canArmRowAction({ pending: false, editorOpen: false }), true);
});

test("an in-flight row action disarms them", () => {
  // pendingId is ONE global slot: a second write armed over the first
  // would be dropped by its own guard with nothing on screen saying so.
  assert.equal(canArmRowAction({ pending: true, editorOpen: false }), false);
});

test("an open editor disarms them", () => {
  // Duplicate would take the editor away from a half-typed block; the
  // dark window would splice a server record into the list the open
  // editor reads its If-Match out of. Different failures, one rule.
  assert.equal(canArmRowAction({ pending: false, editorOpen: true }), false);
  assert.equal(canArmRowAction({ pending: true, editorOpen: true }), false);
});

// ---- preset -> instant ----------------------------------------------------
//
// Every expectation below builds its expected instant with the local Date
// constructor, so the assertions hold in whatever zone the runner is in.

test("tomorrow is the next local midnight, not 24 hours out", () => {
  // 23:59 local: "dark until tomorrow" is a minute away, not tomorrow
  // night. Adding 86_400_000 ms would land a full day late.
  const now = new Date(2026, 2, 14, 23, 59).getTime();
  const got = new Date(darkPresetInstant("tomorrow", now)).getTime();
  assert.equal(got, new Date(2026, 2, 15, 0, 0, 0, 0).getTime());
});

test("a preset resolves on the wall clock, so a DST shift cannot move it", () => {
  // Whatever this zone does to the night of the 7th-8th of March, the
  // block wakes at midnight local -- which is 23 or 25 hours out, never
  // "midnight plus one hour".
  const now = new Date(2026, 2, 7, 12, 0).getTime();
  const midnight = new Date(darkPresetInstant("tomorrow", now));
  assert.equal(midnight.getHours(), 0);
  assert.equal(midnight.getMinutes(), 0);
  assert.equal(midnight.getSeconds(), 0);
  assert.equal(midnight.getMilliseconds(), 0);
  assert.equal(midnight.getDate(), 8);
});

test("next week and four weeks are whole local days out", () => {
  const now = new Date(2026, 2, 14, 9, 30).getTime();
  assert.equal(new Date(darkPresetInstant("week", now)).getTime(), new Date(2026, 2, 21).getTime());
  assert.equal(new Date(darkPresetInstant("four-weeks", now)).getTime(), new Date(2026, 3, 11).getTime());
});

test("every preset lands in the future and on the wire format", () => {
  // A preset resolving to a past instant would PATCH a dark window the
  // server already considers stale: the write succeeds and suppresses
  // nothing, which is the worst shape a failure can take here.
  const now = new Date(2026, 11, 31, 23, 59, 59).getTime();
  for (const preset of DARK_PRESETS) {
    const iso = darkPresetInstant(preset.id, now);
    assert.match(iso, /^\d{4}-\d{2}-\d{2}T[\d:.]+Z$/, `${preset.id} must be RFC 3339`);
    assert.ok(new Date(iso).getTime() > now, `${preset.id} must be ahead of now`);
  }
});

// ---- the consequence rail's end times --------------------------------------
//
// GET /cron/next returns STARTS. The end is the one piece of occurrence
// math the client is allowed to own, because adding a duration to an
// instant needs no calendar knowledge -- which is exactly what separates
// it from evaluating the expression. Pinned here because a wrong end time
// is a reading that looks perfectly plausible.

test("an occurrence inside one local day prints a bare end clock", () => {
  const start = new Date(2026, 8, 12, 21, 0).toISOString();
  assert.equal(occurrenceLabel(start, 120), `${formatLocal(start)} – 23:00`);
});

test("an occurrence running past midnight prints the end's own date", () => {
  // 23:30 + 90 min ends at 01:00 the NEXT day. A bare "01:00" there reads
  // as ending ninety minutes before it started, and "+1" is notation this
  // UI has never taught anyone.
  const start = new Date(2026, 8, 12, 23, 30);
  const end = new Date(2026, 8, 13, 1, 0);
  assert.equal(
    occurrenceLabel(start.toISOString(), 90),
    `${formatLocal(start.toISOString())} – ${formatLocal(end.toISOString())}`,
  );
});

test("a duration nobody has typed yet prints the start alone", () => {
  // parseRequiredInt coerces a blank duration field to 0. A range ending
  // exactly where it starts would be a fabricated reading; the start is
  // the part that is actually known.
  const start = new Date(2026, 8, 12, 21, 0).toISOString();
  assert.equal(occurrenceLabel(start, 0), formatLocal(start));
  assert.equal(occurrenceLabel(start, Number.NaN), formatLocal(start));
});

test("an unparseable start is echoed, never turned into a range", () => {
  assert.equal(occurrenceLabel("not-an-instant", 60), "not-an-instant");
});

// ---- what the rail is showing ---------------------------------------------

const idleRail = { loading: false, error: null, starts: [] as string[], expr: "" };
/** A settled rail: these instants answer for exactly this expression. */
const answered = (expr: string, starts: string[] = []) => ({ ...idleRail, expr, starts });

test("the rail's states are ordered so a failure is never hidden", () => {
  assert.equal(railState({ ...answered("0 21 * * 6"), loading: true }, "0 21 * * 6"), "loading");
  assert.equal(
    railState({ ...answered("0 21 * * 6"), error: "cron rejected: bad field" }, "0 21 * * 6"),
    "error",
  );
  assert.equal(railState(idleRail, "   "), "prompt");
  // A well-formed expression that never comes round returns an empty
  // array rather than a 400 (api/openapi.yaml) -- the answer is genuinely
  // "never", and an empty rail would report it as "nothing yet".
  assert.equal(railState(answered("0 0 30 2 *"), "0 0 30 2 *"), "never");
  assert.equal(
    railState(answered("0 21 * * 6", ["2026-09-12T19:00:00Z"]), "0 21 * * 6"),
    "occurrences",
  );
});

test("instants belonging to a superseded expression are never a settled reading", () => {
  // The debounce is still counting down: the picker already says the
  // 15th, the held instants are the 1st's. Presenting them as this
  // expression's occurrences is the one failure loadRailOccurrences
  // already refuses for out-of-order RESPONSES, arriving from the other
  // side. A read is on its way, so the rail is loading.
  const stale = answered("0 9 1 * *", ["2026-10-01T09:00:00Z"]);
  assert.equal(railState(stale, "0 9 15 * *"), "loading");
  // Same for a failure that belongs to the old expression: the error is
  // no longer an answer to what is on screen.
  assert.equal(railState({ ...stale, error: "cron rejected: bad field" }, "0 9 15 * *"), "loading");
  // And "never" is only ever said about the expression that earned it --
  // an empty list left over from the previous one is not an answer.
  assert.equal(railState(answered("0 9 1 * *"), "0 9 15 * *"), "loading");
});

test("whitespace around the expression is not a different question", () => {
  assert.equal(railState(answered("0 21 * * 6", ["2026-09-12T19:00:00Z"]), "  0 21 * * 6  "), "occurrences");
});

// ---- the collapsed series row's summary ------------------------------------

function row(over: Partial<Record<string, string>> = {}) {
  return { show_title: "", episodes_per_block: "", start_season: "", start_episode: "", ...over };
}

test("a filled row summarises to what it airs", () => {
  assert.deepEqual(seriesRowSummary(row({ show_title: "Breaking Bad", episodes_per_block: "2" })), {
    kind: "named",
    text: "Breaking Bad · 2 episodes",
  });
});

test("a start cursor joins the summary only when both halves are set", () => {
  const both = row({ show_title: "The Wire", episodes_per_block: "1", start_season: "2", start_episode: "5" });
  assert.equal(seriesRowSummary(both).text, "The Wire · 1 episode · from S02E05");
  // A season with no episode cannot make an S/E marker, and half a cursor
  // printed as "S02E00" would be a position nothing airs at.
  const half = row({ show_title: "The Wire", episodes_per_block: "1", start_season: "2" });
  assert.equal(seriesRowSummary(half).text, "The Wire · 1 episode");
});

test("a row with no show title says what is missing, not nothing", () => {
  // The failure this exists to prevent: a collapsed row rendering a blank
  // line the operator cannot find again in a list of twelve.
  const summary = seriesRowSummary(row({ episodes_per_block: "3" }));
  assert.equal(summary.kind, "incomplete");
  assert.match(summary.text, /show title/);
  assert.notEqual(summary.text.trim(), "");
});

test("a titled row with no episode count leads with the title it does have", () => {
  // Findable beats complete: "Breaking Bad — add an episode count" points
  // at a row; "row 3 is incomplete" makes the operator open all twelve.
  const summary = seriesRowSummary(row({ show_title: "Breaking Bad" }));
  assert.equal(summary.kind, "incomplete");
  assert.match(summary.text, /^Breaking Bad/);
  assert.match(summary.text, /episode count/);
});

test("an episode count of zero or less is missing, not a count", () => {
  // The half of the guard that has no blank field behind it: "0" and
  // "-2" both PARSE, so only `episodes <= 0` catches them. The wire
  // requires `int & >0` (cmd/schema/scheduler.cue), so a row summarising
  // as "Breaking Bad · 0 episodes" reads as complete and 400s on save --
  // which is the exact reading this summary exists to prevent.
  for (const count of ["0", "-2"]) {
    const summary = seriesRowSummary(row({ show_title: "Breaking Bad", episodes_per_block: count }));
    assert.equal(summary.kind, "incomplete", `${count} episodes is not a schedule`);
    assert.match(summary.text, /episode count/);
  }
});

// ---- the rail's priority siblings ------------------------------------------

const peer = (id: string, name: string, priority: number, enabled = true, channel = "ch-1") => ({
  id,
  name,
  enabled,
  spec: { channel_id: channel, priority },
});

const editing = {
  name: "Morning Cartoons",
  channelId: "ch-1",
  priority: 50,
  enabled: true,
  editingId: "b-2",
};

test("the field is ordered by priority and marks the block being edited", () => {
  const list = railSiblings(editing, [peer("b-1", "Late Night", 80), peer("b-2", "stale copy", 10)]);
  assert.deepEqual(list, [
    { text: "PRI 80 · Late Night", self: false },
    { text: "PRI 50 · Morning Cartoons (this block)", self: true },
  ]);
});

test("the form's values stand in for the stored record, not beside it", () => {
  // The stored b-2 says priority 10. The operator just typed 50, and the
  // whole point of the rail is showing what THAT does -- so the record it
  // supersedes must not also appear in the list.
  const list = railSiblings(editing, [peer("b-2", "Morning Cartoons", 10)]);
  assert.equal(list.length, 1);
  assert.equal(list[0].text, "PRI 50 · Morning Cartoons (this block)");
});

test("a disabled peer is out of the field; a dark one is still in it", () => {
  // Matches runtime/rank.ts and the guide's inspector exactly: a dark
  // block is defined and it comes back, so a consequence that ignored it
  // would be wrong the morning it wakes. Darkness is not a field here at
  // all -- the caller passes the record, and only `enabled` removes it.
  const list = railSiblings(editing, [peer("b-1", "Off For Now", 90, false), peer("b-3", "Dark Block", 70)]);
  assert.deepEqual(
    list.map((s) => s.text),
    ["PRI 70 · Dark Block", "PRI 50 · Morning Cartoons (this block)"],
  );
});

test("a tie breaks by name, so the list never reshuffles between renders", () => {
  const list = railSiblings({ ...editing, priority: 80 }, [peer("b-1", "Zed", 80), peer("b-3", "Alpha", 80)]);
  assert.deepEqual(
    list.map((s) => s.text),
    ["PRI 80 · Alpha", "PRI 80 · Morning Cartoons (this block)", "PRI 80 · Zed"],
  );
});

test("another channel's blocks are not siblings", () => {
  const list = railSiblings(editing, [peer("b-1", "Elsewhere", 90, true, "ch-9")]);
  assert.deepEqual(list.map((s) => s.text), ["PRI 50 · Morning Cartoons (this block)"]);
});

test("no channel chosen means no field to show", () => {
  // A lone "this block" entry under no channel reads as "nothing else
  // contends", which is a different claim from "you have not said where".
  assert.deepEqual(railSiblings({ ...editing, channelId: "" }, [peer("b-1", "Late Night", 80)]), []);
});

test("a block the operator has switched off is not contending either", () => {
  const list = railSiblings({ ...editing, enabled: false }, [peer("b-1", "Late Night", 80)]);
  assert.deepEqual(list.map((s) => s.text), ["PRI 80 · Late Night"]);
});

test("an unnamed new block still has a line in the field", () => {
  const list = railSiblings({ ...editing, name: "", editingId: null }, []);
  assert.deepEqual(list, [{ text: "PRI 50 · This block", self: true }]);
});

// ---- the cron footgun -----------------------------------------------------
//
// The one predicate that decides whether an operator is warned before a
// cron edit re-aligns a series mid-run. It is wrong silently in both
// directions: firing on a block that never aired trains the operator to
// dismiss it, and staying quiet on one that did lets the episode order
// shift with nothing on screen saying so.

type FootgunForm = Parameters<typeof cronFootgunShows>[0];
type CursorRow = Parameters<typeof cronFootgunShows>[2][number];

function seriesForm(overrides: Partial<FootgunForm> = {}): FootgunForm {
  return {
    type: "series",
    cron: "0 21 * * *",
    series: [{ show_title: "Breaking Bad", start_season: "1", start_episode: "1" }],
    ...overrides,
  };
}

function cursor(title: string, season: number, episode: number, lastAired: string | null): CursorRow {
  return { show_title: title, current_season: season, current_episode: episode, last_aired: lastAired };
}

const AIRED = cursor("Breaking Bad", 2, 5, "2026-09-09T21:00:00Z");
const NEVER_AIRED = cursor("Breaking Bad", 1, 1, null);

test("a committed cursor under a changed cron fires the confirm", () => {
  const shows = cronFootgunShows(seriesForm({ cron: "0 18 * * *" }), "0 21 * * *", [AIRED]);
  assert.deepEqual(shows, [{ title: "Breaking Bad", cursor: "S02E05", season: 1, episode: 1 }]);
});

test("a show that has never aired does not fire it", () => {
  // last_aired nil is exactly what initializeSeriesState checks: the
  // block's own start position still applies, so nothing is re-aligned.
  assert.deepEqual(cronFootgunShows(seriesForm({ cron: "0 18 * * *" }), "0 21 * * *", [NEVER_AIRED]), []);
});

test("a filter block never fires it", () => {
  assert.deepEqual(cronFootgunShows(seriesForm({ type: "filter", cron: "0 18 * * *" }), "0 21 * * *", [AIRED]), []);
});

test("an unchanged cron never fires it", () => {
  // Whitespace only is not an edit -- the server trims nothing here, but
  // neither does re-saving " 0 21 * * * " move a single episode.
  assert.deepEqual(cronFootgunShows(seriesForm({ cron: " 0 21 * * * " }), "0 21 * * *", [AIRED]), []);
});

test("creating a block never fires it", () => {
  // No stored cron means nothing has aired under this block at all.
  assert.deepEqual(cronFootgunShows(seriesForm({ cron: "0 18 * * *" }), null, [AIRED]), []);
});

test("only the block's own shows are reported, and only once each", () => {
  const form = seriesForm({
    cron: "0 18 * * *",
    series: [
      { show_title: "Breaking Bad", start_season: "2", start_episode: "3" },
      { show_title: "Breaking Bad", start_season: "9", start_episode: "9" },
      { show_title: " ", start_season: "", start_episode: "" },
    ],
  });
  const shows = cronFootgunShows(form, "0 21 * * *", [AIRED, cursor("The Wire", 1, 3, "2026-09-01T00:00:00Z")]);
  assert.deepEqual(shows, [{ title: "Breaking Bad", cursor: "S02E05", season: 2, episode: 3 }]);
});

test("a row with no start position rewinds to S01E01, exactly as the engine seeds it", () => {
  const form = seriesForm({
    cron: "0 18 * * *",
    series: [{ show_title: "Breaking Bad", start_season: "", start_episode: "" }],
  });
  assert.deepEqual(cronFootgunShows(form, "0 21 * * *", [AIRED]), [
    { title: "Breaking Bad", cursor: "S02E05", season: 1, episode: 1 },
  ]);
});

test("the confirm names the block, the count, and where each show stands", () => {
  const copy = footgunCopy("Anime Night", [
    { title: "Breaking Bad", cursor: "S02E05", season: 1, episode: 1 },
    { title: "The Wire", cursor: "S01E03", season: 2, episode: 1 },
  ]);
  assert.match(copy.consequence, /^Anime Night has already aired\./);
  assert.match(copy.consequence, /2 shows/);
  assert.equal(
    copy.rewind,
    "Breaking Bad is at S02E05 — rewind sets it to S01E01. The Wire is at S01E03 — rewind sets it to S02E01.",
  );
});

test("one show reads as one show", () => {
  const copy = footgunCopy("Anime Night", [{ title: "Breaking Bad", cursor: "S02E05", season: 1, episode: 1 }]);
  assert.match(copy.consequence, /1 show\b/);
});

// ---- the rewind's per-title outcomes --------------------------------------
//
// The rewind is per SHOW and the confirm is per BLOCK, so a block seeding
// three shows is three independent writes. "Cursors rewound" over a
// partial failure is the silent half-success this page exists to avoid.

test("every cursor moved reads as one line and no failure line", () => {
  const report = rewindReport([{ title: "Breaking Bad", error: null }, { title: "The Wire", error: null }]);
  assert.equal(report.moved, "Cursors rewound — Breaking Bad, The Wire");
  assert.equal(report.failed, null);
});

test("a partial failure says which moved and which did not", () => {
  const report = rewindReport([
    { title: "Breaking Bad", error: null },
    { title: "Firefly", error: "not found: no state for Firefly" },
  ]);
  assert.equal(report.moved, "Cursor rewound — Breaking Bad");
  assert.equal(report.failed, "Cursor unchanged — Firefly: not found: no state for Firefly");
});

test("every cursor failing reads as no success at all", () => {
  const report = rewindReport([{ title: "Breaking Bad", error: "conflict" }, { title: "The Wire", error: "conflict" }]);
  assert.equal(report.moved, null);
  assert.equal(report.failed, "Cursors unchanged — Breaking Bad: conflict; The Wire: conflict");
});

// ---- the answered confirm, across a save that failed -----------------------
//
// The editor stays OPEN on a 409 (name taken) or a 412 (record moved), so
// whatever the operator answered is still in hand when they try again --
// and a queued rewind is a write to SERIES STATE, not to the block, so a
// stale one moves cursors for a change that was abandoned with nothing on
// screen saying so. Both of the page's call sites read the answer through
// ackedRewind: the confirm's gate (armCronConfirm) and the rewind the
// write performs afterwards (submit). This walks the same sequence they
// do, with the ack as the only state there is.

type CronAck = NonNullable<Parameters<typeof ackedRewind>[0]>;

const BREAKING_BAD = { title: "Breaking Bad", cursor: "S02E05", season: 1, episode: 1 };

/** One trip through submit(), reduced to the two questions it asks about
 * an answer already given: does the confirm come up again, and what may
 * this save rewind once the block is stored? */
function save(ack: CronAck | null, cron: string) {
  const authorised = ackedRewind(ack, cron);
  return { asksAgain: authorised === null, rewinds: authorised ?? [] };
}

test("an answer covers the save it was given for", () => {
  const ack: CronAck = { cron: "0 18 * * *", rewind: [BREAKING_BAD] };
  assert.deepEqual(save(ack, "0 18 * * *"), { asksAgain: false, rewinds: [BREAKING_BAD] });
  // Trimmed the same way buildSpec trims it, so a trailing space in the
  // field is not a different question.
  assert.equal(save(ack, " 0 18 * * * ").asksAgain, false);
});

test("a rewind queued for an abandoned cron edit does not survive the retry", () => {
  // The whole defect, in order. Stored cron is "0 21 * * *". The operator
  // types "0 18 * * *", confirms "Save and rewind", and the PUT 409s on a
  // name collision -- the panel stays open, the answer stays in hand.
  const ack: CronAck = { cron: "0 18 * * *", rewind: [BREAKING_BAD] };
  // They put the schedule back and fix the name instead. There is no
  // mid-run change left, so nothing may be rewound: a boolean ack would
  // have skipped the confirm here AND fired the queued PATCH, moving
  // every cursor for a change that never happened.
  const retry = save(ack, "0 21 * * *");
  assert.deepEqual(retry.rewinds, []);
  // And the confirm is armed again, because this save has not been
  // answered for at all.
  assert.equal(retry.asksAgain, true);
});

test("an answer does not carry to a third expression either", () => {
  // Not just the revert: any further cron edit after the answer is a
  // different change, and it gets its own confirm.
  const ack: CronAck = { cron: "0 18 * * *", rewind: [BREAKING_BAD] };
  assert.deepEqual(save(ack, "30 6 * * 1"), { asksAgain: true, rewinds: [] });
});

test("a plain save is an answer too, and it rewinds nothing", () => {
  // "Save schedule" is answered, not unanswered: the confirm must not
  // come back on the second pass through submit().
  const ack: CronAck = { cron: "0 18 * * *", rewind: [] };
  assert.deepEqual(save(ack, "0 18 * * *"), { asksAgain: false, rewinds: [] });
});

test("an editor that has answered nothing asks", () => {
  assert.deepEqual(save(null, "0 18 * * *"), { asksAgain: true, rewinds: [] });
});

// ---- whose change was it? --------------------------------------------------

const editorOn = (editingId: string, selfChangedId: string | null = null) => ({
  open: true,
  editingId,
  selfChangedId,
});

test("an external change to the open block raises the note", () => {
  const reaction = planInvalidatedReaction(editorOn("b-1"), "b-1");
  assert.equal(reaction.note, true);
  assert.equal(reaction.refetchNow, false);
  assert.equal(reaction.queued, true);
});

test("the duplicate's own echo does not raise it", () => {
  // Duplicate leaves the editor open on the copy, and the write that
  // made the copy raises a frame naming that very id. "This block
  // changed elsewhere" about a record created here, one click ago, from
  // the server's own response, is a note with nothing behind it.
  const reaction = planInvalidatedReaction(editorOn("copy-1", "copy-1"), "copy-1");
  assert.equal(reaction.note, false);
  // The rest of the list is still stale, so the read is still owed.
  assert.equal(reaction.queued, true);
});

test("someone else's change to a block this page made still raises it", () => {
  // The echo is consumed by the frame it belongs to (the page clears
  // selfChangedId), so the next frame for the same block is external --
  // and that one the operator does need, because their next save is
  // armed against a record that moved.
  assert.equal(planInvalidatedReaction(editorOn("copy-1"), "copy-1").note, true);
});

test("a frame about another block never notes, self or not", () => {
  assert.equal(planInvalidatedReaction(editorOn("b-1", "copy-1"), "b-2").note, false);
  assert.equal(planInvalidatedReaction(editorOn("b-1"), null).note, false);
});
