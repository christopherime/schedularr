// Unit tests for the guide's live-link routing (v0.5.9): the 2s refetch
// debounce and the two guards that decide whether an announced change
// may replace the reading on the glass at all.
//
// The guards are the point. An auto-refetch that discarded an armed
// draft or re-rendered a half-read inspector out from under the operator
// is worse than having no live link, so the decision is written as pure
// predicates over a plain state shape and pinned here -- no DOM, no
// component, no stream. The frame-to-bus half is web/tests/stream.test.ts
// and web/tests/bus.test.ts; the draft state machine those guards protect
// is web/tests/guide.test.ts.
import assert from "node:assert/strict";
import test from "node:test";

import type { GuideLiveness, Timers } from "../assets/ts/pages/guide.ts";

// guide.ts registers an Alpine component and starts the shell on import:
// the stubs the module touches at load time, and nothing more (the pure
// functions below reach for none of them).
type GlobalStub = { document: unknown; window: unknown };
(globalThis as unknown as GlobalStub).document = {
  addEventListener: () => {},
  getElementById: () => null,
  querySelector: () => null,
};
(globalThis as unknown as GlobalStub).window = {
  setInterval: () => 0,
  location: { search: "" },
  history: { replaceState: () => {} },
  sessionStorage: { getItem: () => null, setItem: () => {}, removeItem: () => {} },
};

const { LIVE_REFETCH_MS, debounce, liveAction, liveRefetchBlocked, liveRefetchReady } = await import(
  "../assets/ts/pages/guide.ts"
);

// ---- the guards -----------------------------------------------------------

/** Idle in committed mode, visible, nothing in flight: the one state in
 * which replacing the reading costs the operator nothing. */
function idle(): GuideLiveness {
  return { draftMode: "off", inspectorOpen: false, loading: false, hidden: false };
}

test("an idle, committed, visible guide refetches", () => {
  assert.equal(liveRefetchBlocked(idle()), false);
  assert.equal(liveRefetchReady(idle()), true);
});

test("an armed draft blocks the refetch", () => {
  // The one the guard exists for: the operator staged a plan against a
  // reading, and a refetch would silently throw both away.
  const state: GuideLiveness = { ...idle(), draftMode: "armed" };
  assert.equal(liveRefetchBlocked(state), true);
  assert.equal(liveRefetchReady(state), false);
});

test("a draft in flight blocks the refetch, and so does an apply", () => {
  for (const draftMode of ["previewing", "applying"] as const) {
    const state: GuideLiveness = { ...idle(), draftMode };
    assert.equal(liveRefetchBlocked(state), true, draftMode);
    assert.equal(liveRefetchReady(state), false, draftMode);
  }
});

test("an open inspector blocks the refetch", () => {
  // Half-read: re-rendering the sheet discards the slot node the panel
  // is describing, and the focus return with it.
  const state: GuideLiveness = { ...idle(), inspectorOpen: true };
  assert.equal(liveRefetchBlocked(state), true);
  assert.equal(liveRefetchReady(state), false);
});

test("a hidden tab defers the refetch without pinning a notice", () => {
  // Not blocked, so nothing is pinned to a tab nobody is looking at.
  // The deferral is honest only because the page also registers on
  // shell.ts's onResume, which re-runs the decision when the tab
  // returns -- see the liveAction cases below.
  const state: GuideLiveness = { ...idle(), hidden: true };
  assert.equal(liveRefetchBlocked(state), false);
  assert.equal(liveRefetchReady(state), false);
});

test("a load already in flight defers the refetch without pinning a notice", () => {
  // Same shape as hidden: the load is about to replace the reading
  // anyway, and reload()'s tail re-runs the guard once it lands.
  const state: GuideLiveness = { ...idle(), loading: true };
  assert.equal(liveRefetchBlocked(state), false);
  assert.equal(liveRefetchReady(state), false);
});

test("a blocked guide stays blocked however the tab is doing", () => {
  // Blockedness is about the operator's work, not the tab: a hidden tab
  // with an armed draft must still pin when it comes back.
  const state: GuideLiveness = { ...idle(), draftMode: "armed", hidden: true, loading: true };
  assert.equal(liveRefetchBlocked(state), true);
});

// ---- the decision -----------------------------------------------------------

// liveAction is what the debounce's fire AND the tab resume both call.
// `announced` is the difference between them: an event KNOWS the lineup
// moved, a resume only knows it was deaf for a while.

test("an announced change on an idle, visible guide refetches", () => {
  assert.equal(liveAction(idle(), true), "refetch");
});

test("a resume onto an idle, visible guide refetches too", () => {
  // The half that was missing: without this the tab would come back
  // showing a reading from before every event it slept through, and the
  // hub's 128-event ring is no guarantee the replay covers them.
  assert.equal(liveAction(idle(), false), "refetch");
});

test("an announced change on blocked work pins instead of refetching", () => {
  const armed: GuideLiveness = { ...idle(), draftMode: "armed" };
  assert.equal(liveAction(armed, true), "pin");
  assert.equal(liveAction({ ...idle(), inspectorOpen: true }, true), "pin");
});

test("a resume onto blocked work defers silently -- it may not claim a change", () => {
  // Being deaf is a reason to re-read, never a reason to assert the
  // lineup moved: LINEUP CHANGED on a hunch is how the one notice this
  // page has stops meaning anything.
  assert.equal(liveAction({ ...idle(), draftMode: "armed" }, false), "defer");
  assert.equal(liveAction({ ...idle(), inspectorOpen: true }, false), "defer");
});

test("a hidden tab defers rather than falling through to nothing", () => {
  // The finding this test exists for: hidden is neither ready nor
  // blocked, so the old two-branch fire path did nothing at all -- no
  // reload, no pinned notice, the announcement simply gone. It is a
  // deferral now, and onResume is what collects it.
  assert.equal(liveAction({ ...idle(), hidden: true }, true), "defer");
  assert.equal(liveAction({ ...idle(), hidden: true, draftMode: "armed" }, true), "pin");
});

test("a load in flight defers, announced or not", () => {
  assert.equal(liveAction({ ...idle(), loading: true }, true), "defer");
  assert.equal(liveAction({ ...idle(), loading: true }, false), "defer");
});

// ---- the debounce ---------------------------------------------------------

/** A fake clock for the injected Timers: real 2s waits in a unit test
 * are how a suite becomes something nobody runs. */
function fakeClock(): { timers: Timers; advance(ms: number): void } {
  let now = 0;
  let next = 1;
  const pending = new Map<number, { at: number; cb: () => void }>();
  return {
    timers: {
      set(cb, ms) {
        const handle = next++;
        pending.set(handle, { at: now + ms, cb });
        return handle;
      },
      clear(handle) {
        pending.delete(handle);
      },
    },
    advance(ms) {
      now += ms;
      for (const [handle, entry] of [...pending]) {
        if (entry.at <= now) {
          pending.delete(handle);
          entry.cb();
        }
      }
    },
  };
}

test("a burst of announcements coalesces into one refetch", () => {
  // Saving three blocks in another tab publishes plan.invalidated three
  // times; that must cost ONE 28-day re-plan, not three.
  const clock = fakeClock();
  let runs = 0;
  const d = debounce(LIVE_REFETCH_MS, clock.timers, () => {
    runs += 1;
  });

  d.trigger();
  clock.advance(500);
  d.trigger();
  clock.advance(500);
  d.trigger();
  clock.advance(LIVE_REFETCH_MS);

  assert.equal(runs, 1);
});

test("the wait restarts from the LAST announcement, not the first", () => {
  const clock = fakeClock();
  let runs = 0;
  const d = debounce(LIVE_REFETCH_MS, clock.timers, () => {
    runs += 1;
  });

  d.trigger();
  clock.advance(LIVE_REFETCH_MS - 1);
  d.trigger();
  clock.advance(LIVE_REFETCH_MS - 1);
  assert.equal(runs, 0, "the second announcement must have pushed the run back");

  clock.advance(1);
  assert.equal(runs, 1);
});

test("cancel drops a pending refetch", () => {
  // reload() cancels: a read already in flight covers every change
  // announced before it started.
  const clock = fakeClock();
  let runs = 0;
  const d = debounce(LIVE_REFETCH_MS, clock.timers, () => {
    runs += 1;
  });

  d.trigger();
  d.cancel();
  clock.advance(LIVE_REFETCH_MS * 10);

  assert.equal(runs, 0);
});

test("a trigger after a run schedules a fresh one", () => {
  // The pending handle is cleared before run(), so a cancel from inside
  // the run (reload() does exactly that) cannot eat the next trigger.
  const clock = fakeClock();
  let runs = 0;
  const d = debounce(LIVE_REFETCH_MS, clock.timers, () => {
    runs += 1;
  });

  d.trigger();
  clock.advance(LIVE_REFETCH_MS);
  d.trigger();
  clock.advance(LIVE_REFETCH_MS);

  assert.equal(runs, 2);
});

test("the debounce window is the spec's two seconds", () => {
  assert.equal(LIVE_REFETCH_MS, 2_000);
});
