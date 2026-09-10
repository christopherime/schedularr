// Unit tests for the shared priority-rank helper
// (web/assets/ts/runtime/rank.ts). The guide inspector and the blocks
// list both read this, so a disagreement here is two surfaces telling
// the operator different things about the same block.
import assert from "node:assert/strict";
import test from "node:test";

const { isContending, priorityRank } = await import("../assets/ts/runtime/rank.ts");

/** Minimal peer literal -- structurally what a BlockRecord already is. */
function peer(priority: number | undefined, enabled = true) {
  return { enabled, spec: { priority } };
}

test("ranks a block among its enabled same-channel peers", () => {
  const peers = [peer(90), peer(50), peer(10)];
  assert.deepEqual(priorityRank(50, peers), { rank: 2, of: 3 });
  assert.deepEqual(priorityRank(90, peers), { rank: 1, of: 3 });
  assert.deepEqual(priorityRank(10, peers), { rank: 3, of: 3 });
});

test("ties share a rank -- competition ranking, not dense", () => {
  const peers = [peer(80), peer(80), peer(20)];
  // Both 80s are 1st of 3; the 20 is 3rd, not 2nd, because two blocks
  // outrank it. Dense ranking here would claim the loser is runner-up.
  assert.deepEqual(priorityRank(80, peers), { rank: 1, of: 3 });
  assert.deepEqual(priorityRank(20, peers), { rank: 3, of: 3 });
});

test("a single peer is 1st of 1", () => {
  assert.deepEqual(priorityRank(50, [peer(50)]), { rank: 1, of: 1 });
});

test("no enabled peers reports of: 0 so callers can drop the rank", () => {
  assert.deepEqual(priorityRank(50, []), { rank: 1, of: 0 });
  assert.deepEqual(priorityRank(50, [peer(90, false)]), { rank: 1, of: 0 });
});

test("disabled peers are excluded from both the rank and the total", () => {
  const peers = [peer(90, false), peer(50), peer(10)];
  // The disabled 90 does not contend, so 50 leads the field of two.
  assert.deepEqual(priorityRank(50, peers), { rank: 1, of: 2 });
});

test("a block absent from its own peer list still ranks against the field", () => {
  // Ghost slots and disabled blocks are not in the enabled peer list;
  // the readout stays honest by counting only who outranks them.
  assert.deepEqual(priorityRank(60, [peer(90), peer(50)]), { rank: 2, of: 2 });
  assert.deepEqual(priorityRank(99, [peer(90), peer(50)]), { rank: 1, of: 2 });
});

test("a missing priority counts as 0, matching the block defaulting", () => {
  // Negative priorities are the only case that can observe the default:
  // `undefined > x` is false for every x, so a peer left undefined would
  // silently stop outranking anything below zero. Config allows them --
  // `priority: int | *10` in cmd/schema/config.cue is not bounded at 0.
  assert.deepEqual(priorityRank(-5, [peer(undefined), peer(-10)]), { rank: 2, of: 2 });
});

// ---- isContending: may this block be ranked at all? ----------------------

test("an enabled block with no dark window is contending", () => {
  assert.equal(isContending({ enabled: true }, 1_000), true);
});

test("a disabled block is not contending", () => {
  assert.equal(isContending({ enabled: false }, 1_000), false);
});

test("a dark window still ahead suppresses the rank", () => {
  assert.equal(isContending({ enabled: true, disabled_until: "2026-01-01T00:00:00Z" }, Date.parse("2025-12-31T23:59:00Z")), false);
});

test("a dark window already past suppresses nothing", () => {
  // Same reading as the blocks list's DARK UNTIL chip: a wake-up in the
  // past is stale, not a state the server still holds. Boundary too --
  // at the wake-up instant the block is back.
  const wake = Date.parse("2026-01-01T00:00:00Z");
  assert.equal(isContending({ enabled: true, disabled_until: "2026-01-01T00:00:00Z" }, wake), true);
  assert.equal(isContending({ enabled: true, disabled_until: "2026-01-01T00:00:00Z" }, wake + 1), true);
});

test("an unparseable dark window suppresses nothing", () => {
  assert.equal(isContending({ enabled: true, disabled_until: "not a date" }, 1_000), true);
});
