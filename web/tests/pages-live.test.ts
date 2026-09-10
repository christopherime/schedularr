// Unit tests for the live link's page-side routing decisions: what the
// Blocks and History pages do to THEMSELVES when a frame lands. The
// handlers that consult these predicates are a few lines of Alpine each;
// the predicates are where a live link stops being a feature and starts
// being the thing that ate somebody's half-typed block, so they are the
// part pinned away from the DOM.
//
// There is no DOM and no fetch in this runner, so nothing here touches
// Alpine, the bus, or the stream -- only the pure functions the two page
// bundles export. Same stub shape as blocks.test.ts and history.test.ts,
// merged because this file imports both page modules.
import assert from "node:assert/strict";
import { createRequire } from "node:module";
import test from "node:test";

type GlobalStub = { document: unknown; window: unknown; cronstrue: unknown };
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

const require = createRequire(import.meta.url);
const cronstrueUMD: { default?: unknown } = require("../assets/vendor/cronstrue.min.js");
(globalThis as unknown as GlobalStub).cronstrue = cronstrueUMD.default ?? cronstrueUMD;

// Dynamic imports so the stubs above are in place before either module's
// top-level registration code runs.
const { changedBlockId, planInvalidatedReaction } = await import("../assets/ts/pages/blocks.ts");
const { applyRefreshLine, cursorDirty } = await import("../assets/ts/pages/history.ts");

// ---- blocks: reading the frame -------------------------------------------

test("changedBlockId reads the id out of a block invalidation", () => {
  assert.equal(changedBlockId({ reason: "block", id: "blk-1" }), "blk-1");
});

test("changedBlockId ignores a frame that names something other than a block", () => {
  // A series invalidation's id is a SHOW TITLE, not a store id -- treating
  // it as one would raise the note on whichever block's id collided.
  assert.equal(changedBlockId({ reason: "series", id: "The Wire" }), null);
});

test("changedBlockId returns null for a malformed frame rather than throwing", () => {
  // The bus hands handlers `unknown` so a payload change fails here and
  // not in the stream's pump loop.
  assert.equal(changedBlockId(null), null);
  assert.equal(changedBlockId("block"), null);
  assert.equal(changedBlockId({}), null);
  assert.equal(changedBlockId({ reason: "block" }), null);
  assert.equal(changedBlockId({ reason: "block", id: 7 }), null);
});

// ---- blocks: the freeze-until-close predicate ----------------------------

const EDITING = { open: true, editingId: "blk-1" };
const CREATING = { open: true, editingId: null };
const SHUT = { open: false, editingId: null };

test("a shut editor refetches the list immediately", () => {
  const r = planInvalidatedReaction(SHUT, "blk-9");
  assert.equal(r.refetchNow, true);
  assert.equal(r.queued, false);
  assert.equal(r.note, false);
});

test("an open editor never refetches, even holding no typed work", () => {
  // The regression this predicate exists for. submit() reads If-Match off
  // the list, so refetching under an open panel re-arms it with an
  // updated_at the operator has never seen: the save then succeeds and
  // full-replaces the other tab's spec with the stale one on screen.
  const r = planInvalidatedReaction(EDITING, "blk-9");
  assert.equal(r.refetchNow, false, "a clean open editor must not have its If-Match rebased");
  assert.equal(r.queued, true);
});

test("a create panel is frozen on the same rule", () => {
  // One rule for "the panel is open", rather than a second predicate that
  // has to agree with the first about what counts as work in progress.
  const r = planInvalidatedReaction(CREATING, "blk-9");
  assert.equal(r.refetchNow, false);
  assert.equal(r.queued, true);
});

test("an open editor owes exactly one refetch however many changes land", () => {
  // The queue is a flag, not a counter: draining it on close must cost
  // one read of the list, not one per frame that arrived meanwhile.
  let queued = false;
  let fetches = 0;
  for (const id of ["blk-2", "blk-3", "blk-4"]) {
    const r = planInvalidatedReaction(EDITING, id);
    if (r.refetchNow) fetches += 1;
    queued = r.queued;
  }
  assert.equal(fetches, 0, "nothing may refetch under an open editor");
  assert.equal(queued, true);
  // What closeEditor does with it.
  if (queued) fetches += 1;
  assert.equal(fetches, 1, "three frames, one drain");
});

test("a change to the OPEN block raises the note", () => {
  // The list is frozen, so the editor really is still holding the
  // updated_at its next save sends as If-Match: that save will 412.
  assert.equal(planInvalidatedReaction(EDITING, "blk-1").note, true);
});

test("a change to some other block never raises the note", () => {
  assert.equal(planInvalidatedReaction(EDITING, "blk-9").note, false);
  assert.equal(planInvalidatedReaction(EDITING, null).note, false, "an unreadable frame names no block");
  assert.equal(planInvalidatedReaction(SHUT, "blk-1").note, false, "no editor, no note");
  // Create mode has no editingId; a null id must not match a null id.
  assert.equal(planInvalidatedReaction(CREATING, null).note, false);
});

// ---- history: the TRACKED refetch guard ----------------------------------

const ROW = { current_season: 2, current_episode: 5 };

test("a draft matching the stored cursor is clean", () => {
  assert.equal(cursorDirty(ROW, { season: "2", episode: "5" }), false);
  assert.equal(cursorDirty(ROW, { season: " 2 ", episode: " 5 " }), false, "whitespace is not an edit");
});

test("a typed cursor is dirty, so the live link leaves TRACKED alone", () => {
  assert.equal(cursorDirty(ROW, { season: "3", episode: "5" }), true);
  assert.equal(cursorDirty(ROW, { season: "2", episode: "" }), true);
  // "02" reads as an edit here even though buildCursorPatch would send
  // nothing for it -- the guard errs toward keeping what was typed.
  assert.equal(cursorDirty(ROW, { season: "02", episode: "5" }), true);
});

test("a row with no draft counts as clean rather than blocking every refetch", () => {
  assert.equal(cursorDirty(ROW, undefined), false);
});

// ---- history: what the tape says after an apply ---------------------------

test("the apply line is only allowed to claim what actually refreshed", () => {
  assert.equal(applyRefreshLine(true, true), "Apply landed — runs and as-run refreshed");
  // A background refetch that failed leaves the pane's last good reading
  // on screen (history.ts contract note 6), so the line names that rather
  // than announcing a refresh that never landed.
  assert.match(applyRefreshLine(true, false), /as-run still showing the last reading/);
  assert.match(applyRefreshLine(false, true), /runs still showing the last reading/);
  assert.match(applyRefreshLine(false, false), /refresh failed/);
});
