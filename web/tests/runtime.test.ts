// Unit tests for the shared runtime (web/assets/ts/runtime/): error
// describing, typed path building, channel labeling/plates, and the
// formatting helpers. These modules are import-side-effect-free, so no
// DOM stubs are needed here.
import assert from "node:assert/strict";
import test from "node:test";

const { ApiError, apiPath } = await import("../assets/ts/runtime/api.ts");
const { describeError, toProblemView } = await import("../assets/ts/runtime/errors.ts");
const { channelLabel, channelPlate } = await import("../assets/ts/runtime/channels.ts");
const { formatLocal, pad2, plural, relativeTime } = await import("../assets/ts/runtime/format.ts");

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
