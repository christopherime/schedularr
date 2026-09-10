// Shell wiring, run once per page by every page entry (initShell):
//
//   1. The token panel: opens automatically when no token is stored or on
//      a 401 -- but at most ONCE per unarmed episode (see
//      promptedThisEpisode below): the 60s telemetry poll also 401s while
//      the token is bad, and re-opening the modal on every poll would
//      steal focus from whatever the operator dismissed it to finish.
//      Save PROBES GET /api/v1/status and flips the dot to ARMED only on
//      success, then broadcasts the re-auth event that re-fires the
//      page's failed loads (api.ts's onReauth).
//   2. The bezel telemetry strip: TUNARR signal, LAST APPLY, and NEXT TICK
//      readouts on every page, plus the LINK legend naming which rung of
//      the live link's degradation ladder is currently feeding them.
//   3. The live link: exactly ONE stream per page (runtime/stream.ts),
//      every frame routed onto the bus (runtime/bus.ts), every heartbeat
//      folded into the clock offset every relative timestamp reads.
//
// The 60s GET /status poll is the ladder's POLL rung, not a second source
// of truth running alongside the stream. It is suspended while the link
// is LIVE (the stream's own status.changed / apply.completed frames
// trigger the refetch instead) and again on LINK LOST (nothing to poll,
// and a stale reading is worse than none) -- two pollers feeding one
// bezel is the race this whole design exists to avoid.
import { apiGet, apiPath, broadcastReauth, onUnauthorized } from "./api.ts";
import type { ApiResponse } from "./api.ts";
import { linkState, noteHeartbeat, onLinkChange, publishLocal, serverNow, setLinkState, subscribe } from "./bus.ts";
import type { LinkState } from "./bus.ts";
import { invalidateChannels } from "./channels.ts";
import { describeError } from "./errors.ts";
import { relativeTime, untilTime } from "./format.ts";
import { connectStream } from "./stream.ts";
import { clearToken, getToken, loadToken, setToken } from "./token.ts";

type Status = ApiResponse<"getStatus", 200>;

const POLL_INTERVAL_MS = 60_000;

/** LINK legend text. Coded-legend discipline, same as the TUNARR pair:
 * the dot's colour never carries the state on its own. */
const LINK_LABELS: Record<LinkState, string> = { live: "Live", poll: "Poll", lost: "Link lost" };

function el<T extends HTMLElement>(id: string): T | null {
  return document.getElementById(id) as T | null;
}

/**
 * A frame's `data` field, or null when it is not JSON. Parsing is guarded
 * because onFrame runs inside the reader loop: one malformed frame
 * throwing would tear down a connection that is otherwise healthy, and
 * every later frame with it.
 */
export function frameData(raw: string): unknown {
  try {
    return JSON.parse(raw) as unknown;
  } catch {
    return null;
  }
}

/**
 * The `server_time` of a heartbeat payload, or null when the frame does
 * not carry one. Narrowed rather than asserted: a heartbeat whose shape
 * changed must degrade to "no offset correction" (the plain local clock,
 * exactly as right as the page was before the live link existed), never
 * feed NaN into the offset -- averaging cannot recover from NaN.
 */
export function heartbeatTime(data: unknown): string | null {
  if (typeof data !== "object" || data === null) return null;
  const value = (data as { server_time?: unknown }).server_time;
  return typeof value === "string" ? value : null;
}

// ---- tab-resume hook -----------------------------------------------------

type ResumeHandler = () => void;
const resumeHandlers = new Set<ResumeHandler>();

/**
 * Registers a callback fired when the tab becomes visible again, after
 * the stream has been reconnected. Page modules register their primary
 * GET here: a hidden tab stops streaming, so it has missed every event
 * since it was hidden and the resume replay only reaches back as far as
 * the hub's ring -- refetching is the only way it can be sure of what it
 * shows. Returns its unsubscriber.
 */
export function onResume(cb: ResumeHandler): () => void {
  resumeHandlers.add(cb);
  return () => {
    resumeHandlers.delete(cb);
  };
}

// Idempotence guard. This is what makes "exactly one stream per page"
// true: every page bundle calls initShell() once at module scope, and a
// second call must not open a second connection to /events.
let started = false;

/** Wires the token panel, the bezel telemetry, and the page's one live
 * link. Idempotent -- each page entry calls it exactly once, but a second
 * call is a no-op rather than a double-wire (and a second stream). */
export function initShell(): void {
  if (started) return;
  started = true;

  const dialog = el<HTMLDialogElement>("token-panel");
  const form = el<HTMLFormElement>("token-form");
  const trigger = el<HTMLButtonElement>("token-trigger");
  const closeBtn = el<HTMLButtonElement>("token-panel-close");
  const cancelBtn = el<HTMLButtonElement>("token-cancel");
  const clearBtn = el<HTMLButtonElement>("token-clear");
  const saveBtn = el<HTMLButtonElement>("token-save");
  const input = el<HTMLInputElement>("token-input");
  const statusEl = el<HTMLParagraphElement>("token-panel-status");
  const statusDot = el<HTMLSpanElement>("token-status");
  const statusLabel = el<HTMLSpanElement>("token-status-label");

  const teleTunarrDot = el<HTMLSpanElement>("tele-tunarr-dot");
  const teleTunarrText = el<HTMLSpanElement>("tele-tunarr-text");
  const teleLastApply = el<HTMLSpanElement>("tele-last-apply");
  const teleNextTick = el<HTMLSpanElement>("tele-next-tick");

  const teleLinkDot = el<HTMLSpanElement>("tele-link-dot");
  const teleLinkText = el<HTMLSpanElement>("tele-link-text");
  const teleLinkReconnect = el<HTMLButtonElement>("tele-link-reconnect");

  // The last successful poll's payload, or null before one has succeeded
  // (or after a failed poll) -- renderTelemetry() reads it so a failed
  // poll degrades to an honest NO DATA reading instead of a stale one.
  let lastStatus: Status | null = null;

  // True once the token panel has auto-opened for the current unarmed
  // episode -- the no-token first-load open and a 401's open both count.
  // Reset when a /status probe succeeds (the poll, or Save's own probe),
  // i.e. on every successful arm, so the NEXT episode gets exactly one
  // prompt again. Without this the 60s poll's 401 would reopen the modal
  // -- and steal focus -- every minute for as long as the token stays bad.
  let promptedThisEpisode = false;

  function setStatusMsg(text: string, tone: "info" | "error" = "info"): void {
    if (!statusEl) return;
    statusEl.textContent = text;
    statusEl.dataset.tone = tone;
  }

  // Three dot states, coded-legend discipline (adjacent text always names
  // the state): "armed" = a probe with the stored token has succeeded,
  // "unarmed" = no token, or the token failed its probe / got a 401,
  // "unknown" = token stored but not yet verified (page just loaded).
  function setArmedState(state: "armed" | "unarmed" | "unknown"): void {
    if (statusDot) statusDot.dataset.state = state;
    if (statusLabel) {
      statusLabel.textContent = state === "armed" ? "Armed" : state === "unarmed" ? "Unarmed" : "Token";
    }
    trigger?.setAttribute(
      "aria-label",
      state === "armed"
        ? "API token armed — open token panel"
        : "No armed API token — open token panel",
    );
  }

  function renderTelemetry(): void {
    if (lastStatus === null) {
      // No successful poll yet (or the poll failed): an honest NO DATA
      // reading, never a stale or fabricated one.
      if (teleTunarrDot) teleTunarrDot.dataset.state = "unknown";
      if (teleTunarrText) teleTunarrText.textContent = "No data";
      if (teleLastApply) teleLastApply.textContent = "—";
      if (teleNextTick) teleNextTick.textContent = "—";
      return;
    }
    if (teleTunarrDot) teleTunarrDot.dataset.state = lastStatus.tunarr_reachable ? "ok" : "down";
    if (teleTunarrText) teleTunarrText.textContent = lastStatus.tunarr_reachable ? "Signal" : "No signal";
    // serverNow(), not Date.now(): both readouts are differences against
    // instants the SERVER stamped, so an operator whose laptop clock runs
    // three minutes fast would otherwise read "3 min ago" as "just now"
    // -- the heartbeat offset is the one place that is corrected.
    const now = serverNow();
    if (teleLastApply) teleLastApply.textContent = relativeTime(lastStatus.last_applied_at, now);
    // untilTime, not relativeTime: an overrunning tick's stored instant
    // sits in the past while the loop is still mid-run, and that must
    // read as "due", not "12 min ago" (which looks like a missed tick).
    if (teleNextTick) teleNextTick.textContent = untilTime(lastStatus.next_cron_tick, now);
  }

  /**
   * Paints the LINK legend. The rung is written verbatim onto the dot's
   * data-state and nowhere else -- baseof.html's contract: the Reconnect
   * button's visibility is CSS off that same attribute, so there is no
   * second thing to write and no half-applied update that could leave a
   * Reconnect button sitting beside a LIVE legend.
   */
  function renderLink(state: LinkState): void {
    if (teleLinkDot) teleLinkDot.dataset.state = state;
    if (teleLinkText) teleLinkText.textContent = LINK_LABELS[state];
  }

  /** Fires one 200ms flare on the dot. Cleared on animationend (below),
   * because re-setting an attribute to the value it already holds
   * restarts no animation -- without the clear, a second drop would be
   * silent. */
  function flare(kind: "live" | "lost"): void {
    if (teleLinkDot) teleLinkDot.dataset.pulse = kind;
  }

  async function poll(): Promise<void> {
    try {
      lastStatus = await apiGet<Status>(apiPath("/status"));
      setArmedState("armed");
      // A successful probe ends the unarmed episode: the next 401 is a
      // NEW episode and earns one fresh auto-open.
      promptedThisEpisode = false;
    } catch {
      lastStatus = null;
      // A 401 already flipped the dot to unarmed via onUnauthorized
      // below; any other failure (server down, timeout) leaves arming
      // alone -- the token isn't wrong, the link is.
    }
    renderTelemetry();
  }

  function openPanel(): void {
    if (!dialog || dialog.open) return;
    if (input) input.value = getToken() ?? "";
    setStatusMsg("");
    dialog.showModal();
    input?.focus();
  }

  function closePanel(): void {
    dialog?.close();
  }

  trigger?.addEventListener("click", openPanel);
  closeBtn?.addEventListener("click", closePanel);
  cancelBtn?.addEventListener("click", closePanel);

  // Native <dialog> gives focus trapping and Escape-to-close for free.
  // Clicking the ::backdrop (a click whose target is the dialog element
  // itself, not one of its children) closes it too.
  dialog?.addEventListener("click", (event) => {
    if (event.target === dialog) closePanel();
  });

  // Save = store + probe. The dot flips ARMED only when GET /status
  // answers with the new token; success closes the panel and broadcasts
  // the re-auth event so the page's failed loads re-fire themselves.
  form?.addEventListener("submit", (event) => {
    event.preventDefault();
    const value = input?.value.trim() ?? "";
    if (!value) {
      setStatusMsg("Enter a token, or use Clear to remove it.", "error");
      input?.focus();
      return;
    }
    void (async () => {
      try {
        // Async since v0.5.5: setToken encrypts before it persists.
        await setToken(value);
      } catch (err) {
        setStatusMsg(`Could not save token: ${err instanceof Error ? err.message : String(err)}`, "error");
        return;
      }
      saveBtn?.setAttribute("aria-busy", "true");
      setStatusMsg("Probing /status with this token…");
      try {
        lastStatus = await apiGet<Status>(apiPath("/status"));
        setArmedState("armed");
        promptedThisEpisode = false;
        renderTelemetry();
        closePanel();
        invalidateChannels();
        broadcastReauth();
      } catch (err) {
        setArmedState("unarmed");
        setStatusMsg(`Probe failed — ${describeError(err)}`, "error");
      } finally {
        saveBtn?.removeAttribute("aria-busy");
      }
    })();
  });

  clearBtn?.addEventListener("click", () => {
    void clearToken()
      .then(() => {
        if (input) input.value = "";
        setArmedState("unarmed");
        setStatusMsg("Token cleared.");
      })
      .catch((err: unknown) => {
        setStatusMsg(`Could not clear token: ${err instanceof Error ? err.message : String(err)}`, "error");
      });
  });

  // A 401 always flips the dot and sets the status line, but auto-opens
  // the panel at most once per unarmed episode -- the 60s telemetry poll
  // keeps 401ing while the token is bad, and reopening a dismissed modal
  // (stealing focus mid-edit) every minute is worse than a dot the
  // operator can act on. No auto-retry of the failed action -- arming (a
  // successful save-probe) is what re-fires loads, via the re-auth
  // broadcast.
  onUnauthorized(() => {
    setArmedState("unarmed");
    if (!promptedThisEpisode) {
      promptedThisEpisode = true;
      openPanel();
    }
    // After openPanel(), which resets the status line to blank on open.
    setStatusMsg("Request rejected (401 Unauthorized). Enter a valid token.", "error");
  });

  // The stored token is encrypted at rest (token.ts): await hydration
  // before the auto-open decision, so a stored-but-not-yet-decrypted token
  // can't misread as "no token" and steal focus with a needless panel.
  void loadToken().then((token) => {
    if (token === null) {
      setArmedState("unarmed");
      renderTelemetry();
      // The first-load auto-open IS this episode's one prompt -- the poll's
      // ensuing 401s must not reopen a panel the operator dismissed.
      promptedThisEpisode = true;
      openPanel();
    } else {
      setArmedState("unknown");
      void poll();
    }
    window.setInterval(() => {
      // The poll IS the POLL rung. On LIVE the stream's frames drive the
      // refetch below, and on LINK LOST there is nothing on the other end
      // to poll -- firing here anyway would put two readers on one bezel,
      // which is how a stale number ends up overwriting a fresh one.
      if (linkState() === "poll") {
        void poll();
        return;
      }
      // Still repaint on the tick: NEXT TICK counts down and LAST APPLY
      // ages without any new payload arriving.
      renderTelemetry();
    }, POLL_INTERVAL_MS);
  });

  // The draft bar (guide) sticks under the bezel, whose height varies as
  // its rows wrap: publish it as a root custom property via CSSOM (the
  // CSP forbids inline styles, not CSSOM writes). ResizeObserver is
  // universal in the supported browsers; guard anyway for the test stubs.
  const bezel = document.querySelector<HTMLElement>(".bezel");
  if (bezel && typeof ResizeObserver !== "undefined") {
    const publish = (): void => {
      document.documentElement.style.setProperty("--bezel-h", `${bezel.offsetHeight}px`);
    };
    publish();
    new ResizeObserver(publish).observe(bezel);
  }

  // ---- the live link ------------------------------------------------------

  // Everything below is gated on the LINK legend really being in the
  // document: the node test stubs answer getElementById with null, and
  // node HAS fetch, so an ungated connect would loop forever on a
  // relative URL with no server behind it and hold the runner open.
  if (teleLinkDot === null) return;
  const dot = teleLinkDot;

  // The flare attribute clears itself the moment the animation ends, so
  // the next transition can set it again (see flare()). Registered once.
  dot.addEventListener("animationend", () => dot.removeAttribute("data-pulse"));

  // The opening paint, deliberately unflared: the bus starts on POLL and
  // the first frame promotes it to LIVE a moment later, which is the
  // ordinary happy path of every page load -- spec §5 bans motion there.
  renderLink(linkState());

  // Frames are hints, never data: an event says a field MAY have moved,
  // and the bezel re-reads the same /status the poll would have read
  // rather than rendering the payload, which only carries what changed.
  subscribe("status.changed", () => void poll());
  subscribe("apply.completed", () => void poll());

  // Gates the green flare on there having been a red one to recover
  // from. Without it the LIVE arrival at every page load would flare.
  let dropped = false;

  onLinkChange((state) => {
    renderLink(state);
    if (state === "lost") {
      dropped = true;
      flare("lost");
      // An honest instrument reports that it has no reading rather than
      // holding up the last one it had, which an operator cannot tell
      // apart from a current one.
      lastStatus = null;
      renderTelemetry();
      return;
    }
    if (state === "live" && dropped) {
      dropped = false;
      flare("live");
    }
    // Entering POLL takes the reading over immediately instead of leaving
    // the bezel a minute stale; entering LIVE catches up on whatever
    // changed while the link was down. At page load the first frame's
    // promotion repeats the opening poll a second later -- one duplicate
    // GET, cheaper than a branch that has to know which transitions
    // belong to a load and which to a recovery.
    void poll();
  });

  let stop: (() => void) | null = null;

  function connect(): void {
    // Always disconnect first: a manual reconnect and the visibility
    // resume are both "stop, then start again" (stream.ts deliberately
    // has no reconnect()), and skipping the stop would leave the old
    // connection pumping into the same bus.
    stop?.();
    stop = connectStream({
      onFrame: (frame) => {
        const data = frameData(frame.data);
        if (frame.event === "heartbeat") {
          const stamp = heartbeatTime(data);
          if (stamp !== null) noteHeartbeat(stamp);
        }
        publishLocal(frame.event, data);
      },
      // Straight through, promotions included: stream.ts only reports
      // "live" once a frame has actually been delivered, so there is
      // nothing left here to second-guess. The shell used to promote to
      // LIVE off the first frame itself, because the ladder announced
      // rung(1) == "live" before anything had ever connected -- that is
      // fixed at the source now (runtime/stream.ts, everDelivered).
      onState: setLinkState,
    });
  }

  connect();

  // The LINK LOST recovery action. CSS keeps it out of the tab order on
  // every other rung, so this can never race the client's own retry.
  teleLinkReconnect?.addEventListener("click", () => connect());

  // A hidden tab holds a connection open for nobody, and the server holds
  // a subscriber slot for it. Drop the stream while hidden, then
  // reconnect AND refetch on the way back: the hub's resume ring is small
  // by design (128 events), so a tab that was away for a while cannot
  // trust Last-Event-ID replay to tell it everything it missed.
  document.addEventListener("visibilitychange", () => {
    if (document.hidden) {
      stop?.();
      stop = null;
      return;
    }
    connect();
    // The bezel is as stale as the page: while hidden it received no
    // frames, and the 60s poll was suspended because the rung still read
    // LIVE. Refetch here rather than leaning on onLinkChange -- the link
    // usually comes back on the same rung it left on, and a watcher that
    // fires only on a CHANGE never runs. Without this the LINK legend
    // reads LIVE beside a pre-hidden TUNARR/LAST APPLY reading, which is
    // the one thing this bezel must never show.
    void poll();
    // Then the pages' own primary GETs, for the same reason.
    for (const handler of [...resumeHandlers]) handler();
  });
}
