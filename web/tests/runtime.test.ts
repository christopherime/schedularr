// Unit tests for the shared runtime (web/assets/ts/runtime/): error
// describing, typed path building, channel labeling/plates, and the
// formatting helpers. These modules are import-side-effect-free, so no
// DOM stubs are needed here.
import assert from "node:assert/strict";
import test from "node:test";

const { ApiError, apiPath } = await import("../assets/ts/runtime/api.ts");
const { describeError, toProblemView } = await import("../assets/ts/runtime/errors.ts");
const { channelHint, channelLabel, channelOrder, channelPlate } = await import("../assets/ts/runtime/channels.ts");
const { clampDays, durationLabel, formatClock, formatLocal, ordinal, pad2, plural, relativeTime, sxxeyy, untilTime } =
  await import("../assets/ts/runtime/format.ts");

// ---- errors --------------------------------------------------------------

test("toProblemView structures an ApiError, request_id included", () => {
  const err = new ApiError({
    type: "about:blank",
    title: "tunarr unreachable",
    status: 502,
    detail: "unable to reach tunarr",
    request_id: "3f9c2a7e51d84b0c",
  });
  assert.deepEqual(toProblemView(err), {
    title: "tunarr unreachable",
    detail: "unable to reach tunarr",
    requestId: "3f9c2a7e51d84b0c",
  });
});

test("toProblemView falls back for non-ApiError values", () => {
  assert.deepEqual(toProblemView(new Error("boom")), {
    title: "Request failed",
    detail: "boom",
    requestId: null,
  });
  assert.equal(toProblemView("plain string").detail, "plain string");
});

test("describeError folds title and detail into one line", () => {
  const withDetail = new ApiError({ type: "about:blank", title: "conflict", status: 409, detail: "name taken" });
  assert.equal(describeError(withDetail), "conflict: name taken");
  const bare = new ApiError({ type: "about:blank", title: "request timed out", status: 0 });
  assert.equal(describeError(bare), "request timed out");
});

// ---- typed path building -------------------------------------------------

test("apiPath prefixes /api/v1 and substitutes encoded params", () => {
  assert.equal(apiPath("/status"), "/api/v1/status");
  assert.equal(apiPath("/blocks/{id}", { id: "abc-123" }), "/api/v1/blocks/abc-123");
  assert.equal(
    apiPath("/state/series/{show_title}", { show_title: "Batman: The Animated Series" }),
    "/api/v1/state/series/Batman%3A%20The%20Animated%20Series",
  );
});

test("apiPath appends only defined query values", () => {
  assert.equal(apiPath("/history", undefined, { days: 7 }), "/api/v1/history?days=7");
  assert.equal(apiPath("/history", undefined, { days: undefined }), "/api/v1/history");
});

test("apiPath throws on a missing path parameter", () => {
  assert.throws(() => apiPath("/blocks/{id}"), /missing path parameter/);
});

// ---- channel labeling & plates -------------------------------------------

const channels = [
  { id: "chan-1", name: "Horror", number: 4 },
  { id: "chan-2", name: "Cartoons", number: 7 },
  { id: "chan-3", name: "No Number" },
];

test("channelLabel joins number and name", () => {
  assert.equal(channelLabel(channels[0]), "4 · Horror");
  assert.equal(channelLabel(channels[2]), "No Number");
  assert.equal(channelLabel({ id: "x" }), "x");
});

test("channelPlate resolves CH number and name from the cache", () => {
  assert.deepEqual(channelPlate("chan-1", channels), { ch: "CH 04", name: "Horror" });
  assert.deepEqual(channelPlate("chan-3", channels), { ch: null, name: "No Number" });
});

test("channelPlate falls back to the shortened id when unresolvable", () => {
  assert.deepEqual(channelPlate("9a1b2c3d-0001-4000-8000-aaaaaaaaaaaa", channels), {
    ch: null,
    name: "9a1b2c3d…",
  });
  assert.deepEqual(channelPlate("short-id", channels), { ch: null, name: "short-id" });
  assert.deepEqual(channelPlate(undefined, channels), { ch: null, name: "—" });
});

// ---- formatting ----------------------------------------------------------

test("formatLocal renders an em dash for missing and echoes unparseable", () => {
  assert.equal(formatLocal(null), "—");
  assert.equal(formatLocal(undefined), "—");
  assert.equal(formatLocal("not a date"), "not a date");
});

test("plural and pad2", () => {
  assert.equal(plural(1, "slot"), "1 slot");
  assert.equal(plural(3, "channel"), "3 channels");
  assert.equal(pad2(4), "04");
  assert.equal(pad2(12), "12");
});

test("relativeTime reads both directions off an injected clock", () => {
  const now = Date.parse("2026-08-30T12:00:00Z");
  const at = (iso: string | null) => relativeTime(iso, now);
  assert.equal(at("2026-08-30T11:59:50Z"), "just now");
  assert.equal(at("2026-08-30T11:55:00Z"), "5 min ago");
  assert.equal(at("2026-08-30T09:00:00Z"), "3 h ago");
  assert.equal(at("2026-08-28T12:00:00Z"), "2 d ago");
  assert.equal(at("2026-08-30T12:00:30Z"), "in <1 min");
  assert.equal(at("2026-08-30T15:00:00Z"), "in 3 h");
  assert.equal(at(null), "—");
  assert.equal(at("garbage"), "—");
});

test("untilTime renders a past instant as due, never 'N min ago'", () => {
  const now = Date.parse("2026-08-30T12:00:00Z");
  assert.equal(untilTime("2026-08-30T11:48:00Z", now), "due");
  assert.equal(untilTime("2026-08-30T12:00:00Z", now), "due");
  assert.equal(untilTime("2026-08-30T12:30:00Z", now), "in 30 min");
  assert.equal(untilTime(null, now), "—");
  assert.equal(untilTime("garbage", now), "—");
});

test("channelHint walks loading, error, empty, and usable states", () => {
  assert.equal(channelHint(true, null, []), "Loading channels from Tunarr…");
  assert.equal(
    channelHint(false, "connection refused", channels),
    "Tunarr channel list unavailable (connection refused) — enter the channel ID manually.",
  );
  assert.equal(channelHint(false, null, []), "Tunarr returned no channels — enter the channel ID manually.");
  assert.equal(channelHint(false, null, channels), "");
});

test("channelHint honors a caller-supplied manual-entry wording", () => {
  const blankMeansAll = "enter a channel ID manually, or leave blank for all channels";
  assert.equal(
    channelHint(false, null, [], blankMeansAll),
    "Tunarr returned no channels — enter a channel ID manually, or leave blank for all channels.",
  );
});

// ---- plan-days clamp (shared by guide + schedule) --------------------------

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

test("ordinal handles the teens exception", () => {
  assert.equal(ordinal(1), "1st");
  assert.equal(ordinal(2), "2nd");
  assert.equal(ordinal(3), "3rd");
  assert.equal(ordinal(4), "4th");
  assert.equal(ordinal(11), "11th");
  assert.equal(ordinal(12), "12th");
  assert.equal(ordinal(13), "13th");
  assert.equal(ordinal(21), "21st");
});

// ---- channel ordering ------------------------------------------------------

test("channelOrder sorts by resolved channel number, not raw id", () => {
  const ordered = [
    { id: "zzz-uuid", name: "Horror", number: 4 },
    { id: "aaa-uuid", name: "Sitcoms", number: 12 },
  ];
  // Raw-id order would put aaa-uuid first; the plates read CH 04 before CH 12.
  assert.ok(channelOrder("zzz-uuid", "aaa-uuid", ordered) < 0);
  assert.ok(channelOrder("aaa-uuid", "zzz-uuid", ordered) > 0);
});

test("channelOrder falls back to name, then id, and sorts unresolved last", () => {
  const mixed = [
    { id: "num-1", name: "Horror", number: 4 },
    { id: "name-b", name: "Beta" },
    { id: "name-a", name: "Alpha" },
  ];
  assert.ok(channelOrder("num-1", "name-a", mixed) < 0, "numbered before unnumbered");
  assert.ok(channelOrder("name-a", "name-b", mixed) < 0, "both unnumbered: by name");
  assert.ok(channelOrder("unknown-a", "unknown-b", mixed) < 0, "unresolvable: by raw id");
  assert.ok(channelOrder("unknown-a", "num-1", mixed) > 0, "unresolved after numbered");
});

// ---- guide-adjacent formatting ---------------------------------------------

test("formatClock renders a padded local HH:MM", () => {
  const d = new Date(2026, 7, 30, 9, 5, 0, 0); // local time, no TZ math
  assert.equal(formatClock(d.getTime()), "09:05");
});

test("durationLabel folds minutes into h/min", () => {
  assert.equal(durationLabel(45), "45 min");
  assert.equal(durationLabel(60), "1 h");
  assert.equal(durationLabel(90), "1 h 30 min");
  assert.equal(durationLabel(480), "8 h");
});

test("sxxeyy marks episodes and refuses to fabricate S00E00", () => {
  assert.equal(sxxeyy(2, 5), "S02E05");
  assert.equal(sxxeyy(1, 12), "S01E12");
  assert.equal(sxxeyy(undefined, 5), null);
  assert.equal(sxxeyy(2, undefined), null);
  assert.equal(sxxeyy(undefined, undefined), null);
});
