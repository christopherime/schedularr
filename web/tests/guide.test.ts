// Unit tests for the guide page, on Node's built-in test runner with
// native type stripping -- no test framework dependency. Run via
// `make web-test` (or `npm test` in web/); CI runs it in the web job.
//
// Two layers are pinned here:
//
//   * the pure logic the component delegates to -- the SCOPE readout
//     every line of draft copy names, and the channel order a diffed
//     sheet renders in;
//   * the draft state machine's own transitions, through a small
//     harness: the page registers its component on `alpine:init`, so a
//     stubbed document hands back that listener, a stubbed Alpine
//     captures the factory, and each test drives one fresh component
//     with the two Alpine magics it uses ($nextTick, $refs) and a
//     stubbed fetch. Rendering, focus and the network are the only
//     things the component touches that a browser owns, and each is
//     observable here (the render queue, the Arm button's focus count,
//     the fetch log).
import assert from "node:assert/strict";
import test from "node:test";

import type { Channel } from "../assets/ts/runtime/channels.ts";
import type { GuideRow } from "../assets/ts/runtime/grid.ts";

type GlobalStub = { document: unknown; window: unknown; Alpine?: unknown; fetch?: unknown };

// The two focus anchors: Arm when draft mode ends, Discard while a
// preview is in the air (Arm disables itself there). Count what lands on
// each, and let a test say which one the operator is standing on.
let armFocusCount = 0;
let discardFocusCount = 0;
let activeElement: { id: string; closest: (selector: string) => unknown } | null = null;
const armButton = {
  id: "guide-arm",
  closest: () => null,
  focus() {
    armFocusCount += 1;
  },
};
const discardButton = {
  id: "guide-discard",
  closest: () => null,
  focus() {
    discardFocusCount += 1;
  },
};
/** The Retry inside a draft problem block: no id, but it answers the
 * selector preview() tests for. */
const problemRetryButton = {
  id: "",
  closest: (selector: string) => (selector === ".guide-draftzone .problem" ? {} : null),
};

// The page registers its component from an alpine:init listener: keep
// the listeners the module adds so a stubbed Alpine can fire one.
const domListeners = new Map<string, () => void>();
(globalThis as unknown as GlobalStub).document = {
  addEventListener(name: string, cb: () => void) {
    domListeners.set(name, cb);
  },
  getElementById: (id: string) => {
    if (id === "guide-arm") return armButton;
    return id === "guide-discard" ? discardButton : null;
  },
  querySelector: () => null,
  get activeElement() {
    return activeElement;
  },
};
// The browser surfaces the component touches beyond fetch: the reading
// mirror it restores a ?draft arrival's diff baseline from, and the
// history entry that arrival consumes.
const sessionStore = new Map<string, string>();
(globalThis as unknown as GlobalStub).window = {
  setInterval: () => 0,
  location: { search: "" },
  history: { replaceState: () => {} },
  sessionStorage: {
    getItem: (key: string) => sessionStore.get(key) ?? null,
    setItem: (key: string, value: string) => {
      sessionStore.set(key, value);
    },
    removeItem: (key: string) => {
      sessionStore.delete(key);
    },
  },
};

// Dynamic import so the global stubs above are in place before the
// module's top-level registration code runs.
const { orderRows, scopeLabelText } = await import("../assets/ts/pages/guide.ts");
const { READING_STORAGE_KEY } = await import("../assets/ts/runtime/draft.ts");

// ---- pure helpers ---------------------------------------------------------

const channels: Channel[] = [
  { id: "ch-horror", name: "Horror", number: 4 },
  { id: "ch-toons", name: "Toons", number: 2 },
];

function row(channelId: string): GuideRow {
  return { channelId, plate: { ch: null, name: channelId }, slots: [] };
}

test("scopeLabelText names the whole station for the empty scope", () => {
  assert.equal(scopeLabelText("", channels), "ALL channels");
});

test("scopeLabelText reads a resolved channel as its plate text", () => {
  assert.equal(scopeLabelText("ch-horror", channels), "CH 04 · Horror");
});

test("scopeLabelText falls back to the plate's name alone when the channel is unresolved", () => {
  // No number to print, and no fabricated name: the shortened raw id.
  assert.equal(scopeLabelText("ch-unknown", channels), "ch-unknown");
  assert.equal(scopeLabelText("01234567-89ab-cdef-0123-456789abcdef", channels), "01234567…");
});

test("orderRows puts a reading-only row back in channel order", () => {
  // diffRows returns the draft's channels first and the reading-only
  // ones after: an all-removed channel arrives last however it sorts.
  const diffed = [row("ch-horror"), row("ch-toons")];
  assert.deepEqual(
    orderRows(diffed, channels).map((r) => r.channelId),
    ["ch-toons", "ch-horror"],
  );
});

test("orderRows leaves its input untouched", () => {
  const diffed = [row("ch-horror"), row("ch-toons")];
  orderRows(diffed, channels);
  assert.deepEqual(
    diffed.map((r) => r.channelId),
    ["ch-horror", "ch-toons"],
  );
});

// ---- the component harness ------------------------------------------------

/** The slice of the component the state-machine tests read and write. */
interface GuideComponent {
  controls: { channelId: string };
  loading: boolean;
  problem: unknown;
  readingRequestedAt: number;
  readingRestored: boolean;
  readingRows: unknown[];
  blocksByName: Record<string, unknown>;
  draft: {
    mode: string;
    pendingScope: boolean;
    signature: { days: number; channelId: string } | null;
    plan: unknown;
    rows: unknown;
    applyFailed: boolean;
    applyError: { title: string } | null;
  };
  $nextTick: (cb: () => void) => void;
  $refs: { confirmDialog: { open: boolean; close(): void; showModal(): void } };
  $root: unknown;
  reload: () => Promise<void>;
  preview: () => Promise<void>;
  openWithDraft: (scope: string) => Promise<void>;
  requestDraft: () => void;
  requestApply: () => void;
  confirmApply: () => Promise<void>;
  discardDraft: () => void;
  draftBarLine: () => string;
  droppedWarnings: () => number;
}

let factory: (() => unknown) | null = null;
(globalThis as unknown as GlobalStub).Alpine = {
  data(_name: string, f: () => unknown) {
    factory = f;
  },
};
domListeners.get("alpine:init")?.();

// Alpine's DOM flush: the component queues work on $nextTick, so the
// tests hold that queue and run it when a browser would have.
let renderQueue: (() => void)[] = [];

function flush(): void {
  const queued = renderQueue;
  renderQueue = [];
  for (const cb of queued) cb();
}

/** One request as the component sent it. The body matters as much as
 * the path: the armed-signature rule is that APPLY re-sends exactly what
 * the preview was generated from, which is unobservable from the URL. */
interface FetchCall {
  path: string;
  method: string;
  body: string | null;
}
type FetchInit = { method?: string; body?: unknown };
type FetchStub = (path: string, init?: FetchInit) => Promise<unknown>;
let fetchStub: FetchStub = () => Promise.reject(new Error("no fetch stub installed"));
let fetchLog: FetchCall[] = [];
(globalThis as unknown as GlobalStub).fetch = (path: string, init?: FetchInit) => {
  fetchLog.push({
    path,
    method: init?.method ?? "GET",
    body: typeof init?.body === "string" ? init.body : null,
  });
  return fetchStub(path, init);
};

/** Every logged request whose path contains `fragment`. */
function calls(fragment: string): FetchCall[] {
  return fetchLog.filter((c) => c.path.includes(fragment));
}

/** A JSON response as fetch would hand it back. */
function jsonResponse(body: unknown, status = 200): unknown {
  return {
    ok: status >= 200 && status < 300,
    status,
    statusText: "",
    headers: { get: () => "application/json" },
    json: () => Promise.resolve(body),
  };
}

function makeGuide(): GuideComponent {
  assert.ok(factory, "the page registered no Alpine component");
  const state = factory() as GuideComponent;
  renderQueue = [];
  armFocusCount = 0;
  discardFocusCount = 0;
  activeElement = null;
  fetchLog = [];
  sessionStore.clear();
  state.$nextTick = (cb) => {
    renderQueue.push(cb);
  };
  state.$refs = {
    confirmDialog: {
      open: true,
      close() {
        this.open = false;
      },
      showModal() {
        this.open = true;
      },
    },
  };
  state.$root = null;
  return state;
}

/** A one-slot plan, enough for planSlotCount/planChannelCount and the
 * projection to have something real to chew on. */
function plan(): unknown {
  const start = new Date(Date.now() + 3_600_000).toISOString();
  const end = new Date(Date.now() + 7_200_000).toISOString();
  return {
    channels: {
      "ch-horror": [
        { start_time: start, end_time: end, block: { name: "Late Night", type: "filter" }, programs: [] },
      ],
    },
    warnings: [],
  };
}

/** The same plan with two occurrences lost to conflicts, both of them
 * placeable as ghosts once the block index below is on the component. */
function planWithWarnings(): unknown {
  const base = plan() as { warnings: unknown[] };
  base.warnings = [
    {
      block_name: "Late Night",
      blocking_block_name: "Prime Time",
      occurrence_start: new Date(Date.now() + 86_400_000).toISOString(),
    },
    {
      block_name: "Late Night",
      blocking_block_name: "Prime Time",
      occurrence_start: new Date(Date.now() + 172_800_000).toISOString(),
    },
  ];
  return base;
}

/** The enrichment `GET /blocks` lands, keyed by name as loadBlockIndex
 * keys it: without it no ghost can be placed. */
function blockIndex(): Record<string, unknown> {
  return {
    "Late Night": {
      id: "blk-1",
      name: "Late Night",
      enabled: true,
      spec: { channel_id: "ch-horror", duration: 60 },
    },
  };
}

/** A mirrored reading as the Blocks round trip left it in
 * sessionStorage: one channel, one slot, taken a minute ago. */
function storedReading(): string {
  const startMs = Date.now() + 3_600_000;
  return JSON.stringify({
    requestedAt: Date.now() - 60_000,
    rows: [
      {
        channelId: "ch-horror",
        slots: [
          {
            blockName: "Late Night",
            blockType: "filter",
            cron: "0 21 * * *",
            priority: 50,
            startMs,
            endMs: startMs + 3_600_000,
            programs: [{ title: "A Movie", durationMs: 3_600_000, startMs }],
          },
        ],
      },
    ],
  });
}

/** Yields to the timer queue until `done` holds: the states these tests
 * observe sit a few awaits deep inside apiGet. */
async function until(done: () => boolean, what: string): Promise<void> {
  for (let i = 0; i < 50 && !done(); i++) await new Promise((resolve) => setTimeout(resolve, 0));
  assert.ok(done(), `${what} never arrived`);
}

// ---- draft state machine --------------------------------------------------

test("leaving draft mode defers the Arm focus past Alpine's DOM flush", () => {
  // focus() on a button whose :disabled binding Alpine has not flushed
  // yet is a silent no-op -- it would strand the keyboard on <body>.
  const guide = makeGuide();
  guide.readingRequestedAt = Date.now();
  guide.draft.mode = "armed";
  guide.draft.rows = [];
  guide.discardDraft();
  assert.equal(armFocusCount, 0, "focus must not be taken before the mode change paints");
  flush();
  assert.equal(armFocusCount, 1);
});

test("DISCARD during DRAFTING drops the preview still in the air", () => {
  const guide = makeGuide();
  guide.readingRequestedAt = Date.now();
  // The draft is held in the air until the discard has happened.
  let land: (value: unknown) => void = () => {};
  const inFlight = new Promise((resolve) => {
    land = resolve;
  });
  fetchStub = () => inFlight;
  const flight = guide.preview();
  assert.equal(guide.draft.mode, "previewing");
  guide.discardDraft();
  assert.equal(guide.draft.mode, "off");
  land(jsonResponse(plan()));
  return flight.then(() => {
    // The landing carries a stale sequence token: it may not re-arm a
    // draft the operator abandoned, for a scope SCOPE no longer shows.
    assert.equal(guide.draft.mode, "off");
    assert.equal(guide.draft.plan, null);
    assert.equal(guide.draft.rows, null);
    assert.equal(guide.controls.channelId, "");
  });
});

test("preview refuses to arm without a reading and fetches one instead", async () => {
  // Every count and the bar's clock are "vs the reading taken HH:MM":
  // with no reading, the clock would print the epoch.
  const guide = makeGuide();
  // The NO SIGNAL state: the first load landed nothing and is over.
  guide.loading = false;
  let reloads = 0;
  guide.reload = () => {
    reloads += 1;
    return Promise.resolve();
  };
  let sends = 0;
  fetchStub = () => {
    sends += 1;
    return Promise.resolve(jsonResponse(plan()));
  };
  await guide.preview();
  assert.equal(sends, 0, "no draft may be planned without a reading to diff it against");
  assert.equal(reloads, 1);
  assert.equal(guide.draft.mode, "off");
  assert.equal(guide.draft.pendingScope, true, "the arm press is latched for the reading's landing");
});

test("a reading that never lands leaves the latch unfired", async () => {
  const guide = makeGuide();
  guide.draft.pendingScope = true;
  let previews = 0;
  guide.preview = () => {
    previews += 1;
    return Promise.resolve();
  };
  fetchStub = () => Promise.resolve(jsonResponse({ title: "tunarr unreachable", status: 502 }, 502));
  await guide.reload();
  assert.equal(guide.readingRequestedAt, 0);
  assert.equal(previews, 0, "NO SIGNAL's Retry is the recovery, not a draft with no baseline");
  assert.equal(guide.draft.pendingScope, false);
});

test("the latch fires against the reading a load just landed", async () => {
  const guide = makeGuide();
  guide.draft.pendingScope = true;
  let previews = 0;
  guide.preview = () => {
    previews += 1;
    return Promise.resolve();
  };
  fetchStub = (path) => Promise.resolve(jsonResponse(path.includes("/blocks") ? [] : plan()));
  await guide.reload();
  assert.notEqual(guide.readingRequestedAt, 0);
  assert.equal(previews, 1);
  assert.equal(guide.draft.pendingScope, false);
});

test("a SCOPE change latched during an apply survives the post-apply reset", async () => {
  const guide = makeGuide();
  guide.readingRequestedAt = Date.now();
  guide.draft.mode = "armed";
  guide.draft.signature = { days: 7, channelId: "ch-horror" };
  guide.draft.plan = plan();
  guide.draft.rows = [];
  // Latched while the push was in the air (both draft entry points do
  // this while mode is "applying").
  guide.draft.pendingScope = true;
  let latchAtReload: boolean | null = null;
  guide.reload = () => {
    latchAtReload = guide.draft.pendingScope;
    return Promise.resolve();
  };
  fetchStub = () => Promise.resolve(jsonResponse(plan()));
  await guide.confirmApply();
  assert.equal(guide.draft.mode, "off");
  assert.equal(guide.controls.channelId, "");
  assert.equal(latchAtReload, true, "the reload must find the latch and re-fire it against the fresh reading");
  assert.equal(armFocusCount, 0);
  flush();
  assert.equal(armFocusCount, 1);
});

test("a failed apply drops the latch and leaves the draft armed", async () => {
  const guide = makeGuide();
  guide.readingRequestedAt = Date.now();
  guide.draft.mode = "armed";
  guide.draft.signature = { days: 7, channelId: "" };
  guide.draft.plan = plan();
  guide.draft.rows = [];
  guide.draft.pendingScope = true;
  guide.reload = () => Promise.reject(new Error("the failure path must not re-read"));
  fetchStub = () => Promise.resolve(jsonResponse({ title: "tunarr unreachable", status: 502 }, 502));
  await guide.confirmApply();
  // The armed draft and its error stay on the glass: a fresh preview
  // would clear the error the operator still has to read.
  assert.equal(guide.draft.mode, "armed");
  assert.equal(guide.draft.applyFailed, true);
  assert.equal(guide.draft.applyError?.title, "tunarr unreachable");
  assert.equal(guide.draft.pendingScope, false);
  assert.equal(guide.$refs.confirmDialog.open, false);
  flush();
  assert.equal(armFocusCount, 1);
});

test("a ?draft arrival keeps the skeleton up until its first draft lands", async () => {
  // The restored reading seeds the diff but is never painted as the
  // committed grid, so until the draft lands there is nothing to show:
  // the skeleton (with its honest first-load note) has to stay on the
  // glass rather than an empty frame.
  const guide = makeGuide();
  sessionStore.set(READING_STORAGE_KEY, storedReading());
  let land: (value: unknown) => void = () => {};
  const inFlight = new Promise((resolve) => {
    land = resolve;
  });
  fetchStub = (path) => {
    if (path.includes("/status")) return Promise.resolve(jsonResponse({}));
    if (path.includes("/blocks")) return Promise.resolve(jsonResponse([]));
    return inFlight;
  };

  const arrival = guide.openWithDraft("ch-horror");
  await until(() => guide.draft.mode === "previewing", "the first preview");
  assert.equal(guide.readingRestored, true, "the mirrored reading seeds the diff");
  assert.equal(guide.loading, true, "the skeleton must hold while the first draft is in the air");

  land(jsonResponse(plan()));
  await arrival;
  assert.equal(guide.loading, false, "the draft landing is what replaces the skeleton");
  assert.equal(guide.draft.mode, "armed");
});

test("an Arm press while the ?draft skeleton holds re-drafts exactly once", async () => {
  // Both draft entry points latch while a flight is in the air, and the
  // skeleton window is one: the latch must fire once when the first
  // draft lands, not loop on it.
  const guide = makeGuide();
  sessionStore.set(READING_STORAGE_KEY, storedReading());
  let land: (value: unknown) => void = () => {};
  const inFlight = new Promise((resolve) => {
    land = resolve;
  });
  let generates = 0;
  fetchStub = (path) => {
    if (path.includes("/status")) return Promise.resolve(jsonResponse({}));
    if (path.includes("/blocks")) return Promise.resolve(jsonResponse([]));
    generates += 1;
    return inFlight;
  };

  const arrival = guide.openWithDraft("ch-horror");
  await until(() => guide.draft.mode === "previewing", "the first preview");
  guide.requestDraft();
  assert.equal(guide.draft.pendingScope, true, "a press against the skeleton latches");

  land(jsonResponse(plan()));
  await arrival;
  await until(() => generates === 2 && guide.draft.mode === "armed", "the latched re-draft");
  await until(() => guide.draft.pendingScope === false, "the latch to clear");
  assert.equal(generates, 2, "the latch must fire exactly once, never loop");
});

test("APPLY re-sends the body the preview was generated from", () => {
  // The armed-signature rule (spec §3.3): the scope the confirm dialog
  // named is the scope that lands, whatever SCOPE holds by the time the
  // operator presses through. Only the request body can show it.
  const guide = makeGuide();
  guide.readingRequestedAt = Date.now();
  guide.controls.channelId = "ch-horror";
  fetchStub = (path) => Promise.resolve(jsonResponse(path.includes("/blocks") ? [] : plan()));
  return guide
    .preview()
    .then(() => {
      assert.equal(guide.draft.mode, "armed");
      // SCOPE moves under the open dialog.
      guide.controls.channelId = "ch-toons";
      guide.requestApply();
      return guide.confirmApply();
    })
    .then(() => {
      const generate = calls("/generate");
      const apply = calls("/apply");
      assert.equal(generate.length, 1);
      assert.equal(apply.length, 1);
      assert.equal(apply[0].method, "POST");
      assert.equal(apply[0].body, JSON.stringify({ days: 7, channel_id: "ch-horror" }));
      assert.equal(apply[0].body, generate[0].body, "apply rebuilt the body from the live controls");
      assert.equal(guide.controls.channelId, "", "SCOPE snaps back to all channels after an apply");
    });
});

test("a failed post-apply re-read leaves no baseline behind", async () => {
  // The applied-away reading describes a push that has landed: keeping
  // it as the diff baseline would arm a draft dating its verdicts to a
  // reading nobody can see, under a tape line that says APPLIED.
  const guide = makeGuide();
  guide.readingRequestedAt = Date.now() - 60_000;
  guide.readingRows = [];
  guide.draft.mode = "armed";
  guide.draft.signature = { days: 7, channelId: "" };
  guide.draft.plan = plan();
  guide.draft.rows = [];
  // A SCOPE change latched while the push was in the air.
  guide.draft.pendingScope = true;
  fetchStub = (path) => {
    if (path.includes("/apply")) return Promise.resolve(jsonResponse(plan()));
    if (path.includes("/blocks")) return Promise.resolve(jsonResponse([]));
    return Promise.resolve(jsonResponse({ title: "tunarr unreachable", status: 502 }, 502));
  };

  await guide.confirmApply();
  await until(() => guide.loading === false, "the post-apply re-read");

  assert.notEqual(guide.problem, null, "the failed re-read is NO SIGNAL");
  assert.equal(guide.readingRequestedAt, 0, "the applied-away reading may not stay as the baseline");
  assert.deepEqual(guide.readingRows, []);
  assert.equal(guide.readingRestored, false);
  assert.equal(calls("/generate").length, 0, "no draft may be armed over a reading that is gone");
  assert.equal(guide.draft.mode, "off");
  assert.equal(guide.draft.pendingScope, false);
  assert.equal(sessionStore.get(READING_STORAGE_KEY), undefined, "the mirror goes with the reading");
});

test("the bar's DROPPED counts every occurrence the run lost, placed or not", async () => {
  // DROPPED reports the RUN: what the draft will not air. The amber
  // legend line above the grid is the narrower count -- warnings the
  // grid could not place as ghosts -- and these two must not be the
  // same number.
  const guide = makeGuide();
  guide.readingRequestedAt = Date.now();
  guide.blocksByName = blockIndex();
  fetchStub = (path) => Promise.resolve(jsonResponse(path.includes("/blocks") ? [] : planWithWarnings()));

  await guide.preview();

  assert.equal(guide.draft.mode, "armed");
  assert.match(guide.draftBarLine(), /2 dropped/, "the bar must count both lost occurrences");
  assert.equal(guide.droppedWarnings(), 0, "both ghosts were placeable, so the legend line has nothing to say");
});

test("a preview started from a control it disables hands focus to Discard", async () => {
  // Arm's :disabled flips with the mode and the browser blurs it, so the
  // keypress that armed the draft would sit on <body> for the flight.
  // Discard is on the glass and enabled for all of DRAFTING.
  const guide = makeGuide();
  guide.readingRequestedAt = Date.now();
  activeElement = armButton;
  let land: (value: unknown) => void = () => {};
  const inFlight = new Promise((resolve) => {
    land = resolve;
  });
  fetchStub = () => inFlight;

  const flight = guide.preview();
  assert.equal(guide.draft.mode, "previewing");
  assert.equal(discardFocusCount, 0, "focus must not move before the bar paints");
  flush();
  assert.equal(discardFocusCount, 1);

  land(jsonResponse(plan()));
  await flight;
});

test("a preview started from a draft problem's Retry hands focus to Discard", async () => {
  // preview() nulls the error, so x-if removes the block the button
  // lives in -- the same stranding by a different route.
  const guide = makeGuide();
  guide.readingRequestedAt = Date.now();
  activeElement = problemRetryButton;
  fetchStub = () => Promise.resolve(jsonResponse(plan()));

  await guide.preview();
  flush();
  assert.equal(discardFocusCount, 1);
});

test("a preview started from SCOPE leaves focus where it is", async () => {
  // SCOPE is never disabled: it keeps its own focus, and moving it would
  // take the operator off the control they are still using.
  const guide = makeGuide();
  guide.readingRequestedAt = Date.now();
  activeElement = { id: "guide-scope", closest: () => null };
  fetchStub = () => Promise.resolve(jsonResponse(plan()));

  await guide.preview();
  flush();
  assert.equal(discardFocusCount, 0);
  assert.equal(armFocusCount, 0);
});
