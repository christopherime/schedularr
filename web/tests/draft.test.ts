// Unit tests for the guide's draft model (runtime/draft.ts): the
// reading-vs-draft diff, the storage codec, the request body, the URL
// param, and every line of draft copy. Pure module -- no DOM stubs.
// Instants use local-time Date constructors so the assertions hold in
// any timezone (same convention as grid.test.ts).
import assert from "node:assert/strict";
import test from "node:test";

import type { GuideSlot } from "../assets/ts/runtime/grid.ts";

const {
  DRAFT_DAYS,
  DRAFT_HORIZON_MS,
  READING_MAX_AGE_MS,
  READING_MAX_CHARS,
  appliedTapeLine,
  applyConfirmBody,
  applyConfirmTitle,
  applyingLine,
  diffRows,
  draftBarLine,
  draftHref,
  draftRequestBody,
  draftScopeFromSearch,
  draftVerdictLine,
  draftingLine,
  lineupSignature,
  parseStoredReading,
  planChannelCount,
  planSlotCount,
  rowsFromStored,
  serializeReading,
  slotKey,
} = await import("../assets/ts/runtime/draft.ts");
const { draftAriaPrefix } = await import("../assets/ts/runtime/grid.ts");

const at = (h: number, m = 0) => new Date(2026, 8, 8, h, m, 0, 0).getTime();
const dayAt = (d: number, h: number) => new Date(2026, 8, 8 + d, h, 0, 0, 0).getTime();

function prog(title: string, minutes: number, season?: number, episode?: number) {
  return { title, type: season === undefined ? "movie" : "episode", season, episode, durationMs: minutes * 60_000, startMs: 0 };
}

function slot(channelId: string, blockName: string, startMs: number, minutes: number, programs = [prog("A", minutes)]): GuideSlot {
  return {
    kind: "slot" as const,
    channelId,
    blockName,
    blockType: "filter",
    cron: "0 21 * * *",
    priority: 50,
    startMs,
    endMs: startMs + minutes * 60_000,
    programs,
  };
}

function ghost(channelId: string, blockName: string, startMs: number): GuideSlot {
  return {
    kind: "ghost" as const,
    channelId,
    blockName,
    blockType: "",
    cron: "",
    priority: 0,
    startMs,
    endMs: startMs + 30 * 60_000,
    programs: [],
    lostTo: "Winner",
  };
}

const plate = { ch: "CH 01", name: "Horror" };
const row = (channelId: string, slots: (ReturnType<typeof slot> | ReturnType<typeof ghost>)[]) => ({ channelId, plate, slots });
const opts = (requestedAt: number, scopeChannelId = "") => ({ requestedAt, scopeChannelId });

// ---- keys ------------------------------------------------------------------

test("slotKey is channel|block|start and lineupSignature is end + the ordered program identity", () => {
  const s = slot("c1", "Night", at(21), 60, [prog("Pilot", 30, 1, 1), prog("Feature", 30)]);
  assert.equal(slotKey(s), `c1|Night|${at(21)}`);
  assert.equal(lineupSignature(s), `${at(22)}#Pilot|1800000|1|1;Feature|1800000|-|-`);
});

// ---- diff ------------------------------------------------------------------

test("diffRows marks identical rows same and counts them", () => {
  const reading = [row("c1", [slot("c1", "Night", at(21), 60)])];
  const draft = [row("c1", [slot("c1", "Night", at(21), 60)])];
  const { rows, counts } = diffRows(reading, draft, opts(at(12)));
  assert.deepEqual(counts, { new: 0, changed: 0, same: 1, removed: 0 });
  assert.equal(rows[0].slots[0].draft, "same");
});

test("diffRows marks a draft-only slot new and a lineup change changed", () => {
  const reading = [row("c1", [slot("c1", "Night", at(21), 60, [prog("E1", 60, 1, 1)])])];
  const draft = [row("c1", [slot("c1", "Night", at(21), 60, [prog("E2", 60, 1, 2)]), slot("c1", "Late", at(23), 60)])];
  const { rows, counts } = diffRows(reading, draft, opts(at(12)));
  assert.deepEqual(counts, { new: 1, changed: 1, same: 0, removed: 0 });
  assert.equal(rows[0].slots.find((s) => s.blockName === "Night")?.draft, "changed");
  assert.equal(rows[0].slots.find((s) => s.blockName === "Late")?.draft, "new");
});

test("diffRows treats an end-time change as changed", () => {
  const reading = [row("c1", [slot("c1", "Night", at(21), 60)])];
  const draft = [row("c1", [slot("c1", "Night", at(21), 90)])];
  assert.equal(diffRows(reading, draft, opts(at(12))).counts.changed, 1);
});

test("diffRows marks reading-only slots removed when they overlap the draft window, drops the past, keeps beyond-horizon slots plain", () => {
  const reading = [
    row("c1", [
      slot("c1", "Morning", at(6), 60), // ended before the draft: past, dropped
      slot("c1", "OnAir", at(11, 30), 60), // airing at 12:00: removed (cut)
      slot("c1", "Night", at(21), 60), // still planned: same
      slot("c1", "Late", at(23), 60), // future, absent: removed
      slot("c1", "NextWeek", dayAt(8, 21), 60), // past the 7-day horizon: beyond
    ]),
  ];
  const draft = [row("c1", [slot("c1", "Night", at(21), 60)])];
  const { rows, counts } = diffRows(reading, draft, opts(at(12)));
  assert.deepEqual(counts, { new: 0, changed: 0, same: 1, removed: 2 });
  assert.deepEqual(
    rows[0].slots.map((s) => [s.blockName, s.draft]),
    [
      ["OnAir", "removed"],
      ["Night", "same"],
      ["Late", "removed"],
      ["NextWeek", "beyond"],
    ],
  );
  assert.equal(rows[0].slots[2].endMs, at(24));
});

test("diffRows bounds the horizon at exactly requestedAt + DRAFT_HORIZON_MS", () => {
  const start = at(12);
  const reading = [row("c1", [slot("c1", "Edge", start + DRAFT_HORIZON_MS, 60), slot("c1", "Inside", start + DRAFT_HORIZON_MS - 60_000, 60)])];
  const { rows, counts } = diffRows(reading, [row("c1", [])], opts(start));
  assert.deepEqual(rows[0].slots.map((s) => [s.blockName, s.draft]), [
    ["Inside", "removed"],
    ["Edge", "beyond"],
  ]);
  assert.equal(counts.removed, 1);
  assert.equal(DRAFT_DAYS, 7);
});

test("diffRows orders a removed slot before a new slot at the same start", () => {
  const reading = [row("c1", [slot("c1", "Old", at(21), 60)])];
  const draft = [row("c1", [slot("c1", "New", at(21), 60)])];
  const { rows } = diffRows(reading, draft, opts(at(12)));
  assert.deepEqual(rows[0].slots.map((s) => s.draft), ["removed", "new"]);
});

test("diffRows lets a draft ghost stand for a displaced reading slot (dropped, not removed)", () => {
  const reading = [row("c1", [slot("c1", "Loser", at(21), 60)])];
  const draft = [row("c1", [slot("c1", "Winner", at(21), 60), ghost("c1", "Loser", at(21))])];
  const { rows, counts } = diffRows(reading, draft, opts(at(12)));
  assert.deepEqual(counts, { new: 1, changed: 0, same: 0, removed: 0 });
  assert.deepEqual(rows[0].slots.map((s) => [s.kind, s.draft]), [
    ["slot", "new"],
    ["ghost", undefined],
  ]);
});

test("diffRows with ALL scope keeps a reading-only channel as an all-removed row", () => {
  const reading = [row("c1", [slot("c1", "Night", at(21), 60)]), row("c2", [slot("c2", "Kids", at(7), 60), slot("c2", "Prime", at(20), 60)])];
  const draft = [row("c1", [slot("c1", "Night", at(21), 60)])];
  const { rows, counts } = diffRows(reading, draft, opts(at(12)));
  assert.equal(rows.length, 2);
  assert.deepEqual(rows[1].slots.map((s) => s.draft), ["removed"], "07:00 ended before the draft: dropped");
  assert.deepEqual(counts, { new: 0, changed: 0, same: 1, removed: 1 });
});

test("diffRows with a channel scope diffs only that channel", () => {
  const reading = [row("c1", [slot("c1", "Night", at(21), 60)]), row("c2", [slot("c2", "Prime", at(20), 60)])];
  const draft = [row("c1", [slot("c1", "Night", at(21), 60)])];
  const { rows, counts } = diffRows(reading, draft, opts(at(12), "c1"));
  assert.equal(rows.length, 1);
  assert.equal(rows[0].channelId, "c1");
  assert.deepEqual(counts, { new: 0, changed: 0, same: 1, removed: 0 });
});

test("diffRows treats a channel absent from the reading as all new, and an empty reading as all new", () => {
  const draft = [row("c9", [slot("c9", "Fresh", at(21), 60)])];
  assert.equal(diffRows([], draft, opts(at(12))).rows[0].slots[0].draft, "new");
  assert.deepEqual(diffRows([row("c1", [])], draft, opts(at(12))).counts, { new: 1, changed: 0, same: 0, removed: 0 });
});

test("diffRows does not mutate its inputs", () => {
  const reading = [row("c1", [slot("c1", "Night", at(21), 60)])];
  const draft = [row("c1", [slot("c1", "Night", at(21), 60)])];
  diffRows(reading, draft, opts(at(12)));
  assert.equal(reading[0].slots[0].draft, undefined);
  assert.equal(draft[0].slots[0].draft, undefined);
});

// ---- request body / URL ----------------------------------------------------

test("draftRequestBody omits channel_id for ALL and sends it otherwise", () => {
  assert.deepEqual(draftRequestBody({ days: DRAFT_DAYS, channelId: "" }), { days: 7 });
  assert.deepEqual(draftRequestBody({ days: DRAFT_DAYS, channelId: "abc" }), { days: 7, channel_id: "abc" });
});

test("draftScopeFromSearch reads ?draft=all|<id> and nothing else", () => {
  assert.equal(draftScopeFromSearch(""), null);
  assert.equal(draftScopeFromSearch("?edit=x"), null);
  assert.equal(draftScopeFromSearch("?draft=all"), "");
  assert.equal(draftScopeFromSearch("?draft=ch%20one"), "ch one");
  assert.equal(draftScopeFromSearch("?draft="), null);
});

test("draftHref builds the guide arrival URL", () => {
  assert.equal(draftHref(undefined), "/?draft=all");
  assert.equal(draftHref(""), "/?draft=all");
  assert.equal(draftHref("ch one"), "/?draft=ch%20one");
});

// ---- storage codec ---------------------------------------------------------

test("stored reading round-trips trimmed rows, re-plates on restore, and expires", () => {
  const rows = [row("c1", [slot("c1", "Night", at(21), 60, [prog("E1", 60, 1, 1)]), ghost("c1", "Loser", at(22))])];
  const raw = serializeReading(at(12), rows);
  assert.ok(raw);
  const stored = parseStoredReading(raw, at(13));
  assert.ok(stored);
  assert.equal(stored.requestedAt, at(12));
  assert.deepEqual(stored.rows, [
    {
      channelId: "c1",
      slots: [{ blockName: "Night", blockType: "filter", cron: "0 21 * * *", priority: 50, startMs: at(21), endMs: at(22), programs: [prog("E1", 60, 1, 1)] }],
    },
  ]);
  const restored = rowsFromStored(stored, () => ({ ch: "CH 09", name: "Restored" }));
  assert.deepEqual(restored[0].plate, { ch: "CH 09", name: "Restored" });
  assert.equal(restored[0].slots[0].kind, "slot");
  assert.equal(restored[0].slots[0].channelId, "c1");
  assert.equal(parseStoredReading(raw, at(12) + READING_MAX_AGE_MS + 1), null);
  assert.equal(parseStoredReading(null, at(13)), null);
  assert.equal(parseStoredReading("{not json", at(13)), null);
  assert.equal(parseStoredReading(JSON.stringify({ requestedAt: "x", rows: [] }), at(13)), null);
  assert.equal(parseStoredReading(JSON.stringify({ requestedAt: at(12) }), at(13)), null);
});

test("a stored reading older than the last apply is not a baseline", () => {
  const raw = serializeReading(at(12), [row("c1", [])]);
  assert.ok(raw);
  assert.ok(parseStoredReading(raw, at(13), new Date(at(11)).toISOString()));
  assert.equal(parseStoredReading(raw, at(13), new Date(at(12, 30)).toISOString()), null);
  assert.ok(parseStoredReading(raw, at(13), "not a date"), "an unparseable stamp does not invalidate");
  assert.ok(parseStoredReading(raw, at(13), null));
});

test("parseStoredReading rejects rows, slots and programs of the wrong shape", () => {
  // A payload written by another build must never reach the geometry as
  // NaN, so every row, slot and program is checked elementwise.
  const storedSlot = {
    blockName: "Night",
    blockType: "filter",
    cron: "0 21 * * *",
    priority: 50,
    startMs: at(21),
    endMs: at(22),
    programs: [prog("E1", 60, 1, 1)],
  };
  const payload = (slots: unknown[]) => JSON.stringify({ requestedAt: at(12), rows: [{ channelId: "c1", slots }] });

  assert.ok(parseStoredReading(payload([storedSlot]), at(13)), "a well-formed payload still parses");
  assert.equal(parseStoredReading(payload([{ ...storedSlot, startMs: "21:00" }]), at(13)), null, "a non-numeric startMs");
  assert.equal(parseStoredReading(payload([{ ...storedSlot, programs: undefined }]), at(13)), null, "a slot with no programs array");
  assert.equal(parseStoredReading(payload([{ ...storedSlot, blockName: 7 }]), at(13)), null, "a non-string blockName");
  assert.equal(
    parseStoredReading(payload([{ ...storedSlot, startMs: at(22), endMs: at(21) }]), at(13)),
    null,
    "a slot that ends before it starts",
  );
  assert.equal(
    parseStoredReading(payload([{ ...storedSlot, programs: [{ title: "E1" }] }]), at(13)),
    null,
    "a program with no durationMs",
  );
  // The inspector clocks a program's own start: an absent startMs
  // reaches formatClock as undefined and prints NaN:NaN.
  assert.equal(
    parseStoredReading(payload([{ ...storedSlot, programs: [{ title: "E1", durationMs: 60_000 }] }]), at(13)),
    null,
    "a program with no startMs",
  );
  // typeof NaN === "number", so the loose check let it through.
  assert.equal(parseStoredReading(payload([{ ...storedSlot, priority: NaN }]), at(13)), null, "a NaN priority");
  assert.equal(parseStoredReading(JSON.stringify({ requestedAt: at(12), rows: ["c1"] }), at(13)), null, "a row that is not an object");
  assert.equal(
    parseStoredReading(JSON.stringify({ requestedAt: at(12), rows: [{ channelId: "c1" }] }), at(13)),
    null,
    "a row with no slots array",
  );
});

test("serializeReading refuses a payload over READING_MAX_CHARS", () => {
  const programs = [];
  for (let i = 0; i < 30_000; i++) programs.push(prog(`Program ${i} with a fairly long title to inflate the payload`, 1));
  const rows = [row("c1", [slot("c1", "Huge", at(21), 60, programs)])];
  assert.equal(serializeReading(at(12), rows), null);
  assert.ok(READING_MAX_CHARS >= 1_000_000);
});

// ---- counts and copy -------------------------------------------------------

const wire = (start: number, name: string) => ({
  start_time: new Date(start).toISOString(),
  end_time: new Date(start + 3_600_000).toISOString(),
  block: { name, cron: "0 21 * * *", duration: 60, channel_id: "c1", type: "filter" as const },
  programs: [],
});

test("planSlotCount and planChannelCount read the wire shape", () => {
  const plan = { applied: false, channels: { c1: [wire(at(21), "A"), wire(at(23), "B")], c2: [wire(at(20), "C")] } };
  assert.equal(planSlotCount(plan), 3);
  assert.equal(planChannelCount(plan), 2);
});

test("draftBarLine prints counts, omitting zero verdicts, with the reading clock", () => {
  assert.equal(
    draftBarLine({ slots: 14, channels: 3, counts: { new: 2, changed: 1, same: 11, removed: 1 }, dropped: 1, readingRequestedAt: at(21, 2) }),
    "7-day draft — 14 slots across 3 channels · 2 new · 1 changed · 1 removed · 1 dropped · vs reading 21:02",
  );
  assert.equal(
    draftBarLine({ slots: 1, channels: 1, counts: { new: 0, changed: 0, same: 1, removed: 0 }, dropped: 0, readingRequestedAt: at(9, 5) }),
    "7-day draft — 1 slot across 1 channel · no changes · vs reading 09:05",
  );
  assert.equal(
    draftBarLine({ slots: 0, channels: 0, counts: { new: 0, changed: 0, same: 0, removed: 2 }, dropped: 0, readingRequestedAt: at(9, 5) }),
    "7-day draft — 0 slots across 0 channels · 2 removed · vs reading 09:05",
  );
});

test("drafting and applying lines", () => {
  assert.equal(draftingLine("ALL channels"), "Drafting — planning ALL channels against Tunarr…");
  assert.equal(draftingLine("CH 04 · HORROR"), "Drafting — planning CH 04 · HORROR against Tunarr…");
  assert.equal(applyingLine(14, 3), "Applying — 14 slots across 3 channels…");
});

test("apply confirm copy names scope, real counts, and the reading it is measured against", () => {
  assert.equal(applyConfirmTitle("ALL channels"), "Apply ALL channels");
  assert.equal(applyConfirmTitle("CH 04 · HORROR"), "Apply CH 04 · HORROR");
  assert.equal(
    applyConfirmBody({ slots: 14, channels: 3, counts: { new: 2, changed: 1, same: 11, removed: 1 }, scopeLabel: "ALL channels", readingRequestedAt: at(21, 2) }),
    "This applies 14 slots across 3 channels to Tunarr — 2 new, 1 changed, 1 removed vs the 21:02 reading.",
  );
  assert.equal(
    applyConfirmBody({ slots: 14, channels: 3, counts: { new: 0, changed: 0, same: 14, removed: 0 }, scopeLabel: "ALL channels", readingRequestedAt: at(21, 2) }),
    "This applies 14 slots across 3 channels to Tunarr — no changes vs the 21:02 reading.",
  );
  assert.equal(
    applyConfirmBody({ slots: 0, channels: 0, counts: { new: 0, changed: 0, same: 0, removed: 3 }, scopeLabel: "CH 04 · HORROR", readingRequestedAt: at(21, 2) }),
    "This draft schedules nothing for CH 04 · HORROR. Applying pushes an empty lineup to any channel Schedularr last applied in this scope — 3 slots removed vs the 21:02 reading.",
  );
  assert.equal(
    applyConfirmBody({ slots: 0, channels: 0, counts: { new: 0, changed: 0, same: 0, removed: 0 }, scopeLabel: "ALL channels", readingRequestedAt: at(21, 2) }),
    "This draft schedules nothing for ALL channels. Applying pushes an empty lineup to any channel Schedularr last applied in this scope.",
  );
});

test("appliedTapeLine is the spec's printout", () => {
  assert.equal(appliedTapeLine(14, 3), "Applied — 14 slots / 3 channels");
  assert.equal(appliedTapeLine(1, 1), "Applied — 1 slot / 1 channel");
});

test("draftVerdictLine explains each verdict against the timestamped reading", () => {
  assert.equal(draftVerdictLine("new", at(21, 2), false), "Draft: new — not in the 21:02 reading; the draft adds it.");
  assert.equal(draftVerdictLine("changed", at(21, 2), false), "Draft: changed — the lineup differs from the 21:02 reading.");
  assert.equal(draftVerdictLine("removed", at(21, 2), false), "Draft: removed — in the 21:02 reading, not in this draft; the draft does not schedule it.");
  assert.equal(draftVerdictLine("removed", at(21, 2), true), "Draft: removed — airing now in the 21:02 reading, not in this draft; applying cuts it.");
  assert.equal(draftVerdictLine("same", at(21, 2), false), "Draft: unchanged from the 21:02 reading.");
  assert.equal(draftVerdictLine("beyond", at(21, 2), false), "Beyond the 7-day draft — from the 21:02 reading, not part of this apply.");
});

test("draftAriaPrefix prefixes only the verdicts that change something", () => {
  assert.equal(draftAriaPrefix("new"), "Draft new — ");
  assert.equal(draftAriaPrefix("changed"), "Draft changed — ");
  assert.equal(draftAriaPrefix("removed"), "Draft removed — ");
  assert.equal(draftAriaPrefix("same"), "");
  assert.equal(draftAriaPrefix("beyond"), "");
  assert.equal(draftAriaPrefix(undefined), "");
});
