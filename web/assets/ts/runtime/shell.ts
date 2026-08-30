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
//      readouts on every page, fed by a 60s GET /status poll (no SSE yet
//      -- the LIVE/POLL/LINK legend arrives with the event stream in
//      v0.5.6 and is deliberately absent rather than faked).
import { apiGet, apiPath, broadcastReauth, onUnauthorized } from "./api.ts";
import type { ApiResponse } from "./api.ts";
import { invalidateChannels } from "./channels.ts";
import { describeError } from "./errors.ts";
import { relativeTime, untilTime } from "./format.ts";
import { clearToken, getToken, setToken } from "./token.ts";

type Status = ApiResponse<"getStatus", 200>;

const POLL_INTERVAL_MS = 60_000;

function el<T extends HTMLElement>(id: string): T | null {
  return document.getElementById(id) as T | null;
}

let started = false;

/** Wires the token panel and starts the bezel telemetry poll. Idempotent
 * -- each page entry calls it exactly once, but a second call is a no-op
 * rather than a double-wire. */
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
    if (teleLastApply) teleLastApply.textContent = relativeTime(lastStatus.last_applied_at);
    // untilTime, not relativeTime: an overrunning tick's stored instant
    // sits in the past while the loop is still mid-run, and that must
    // read as "due", not "12 min ago" (which looks like a missed tick).
    if (teleNextTick) teleNextTick.textContent = untilTime(lastStatus.next_cron_tick);
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
    try {
      setToken(value);
    } catch (err) {
      setStatusMsg(`Could not save token: ${err instanceof Error ? err.message : String(err)}`, "error");
      return;
    }
    saveBtn?.setAttribute("aria-busy", "true");
    setStatusMsg("Probing /status with this token…");
    void apiGet<Status>(apiPath("/status"))
      .then((status) => {
        lastStatus = status;
        setArmedState("armed");
        promptedThisEpisode = false;
        renderTelemetry();
        closePanel();
        invalidateChannels();
        broadcastReauth();
      })
      .catch((err: unknown) => {
        setArmedState("unarmed");
        setStatusMsg(`Probe failed — ${describeError(err)}`, "error");
      })
      .finally(() => {
        saveBtn?.removeAttribute("aria-busy");
      });
  });

  clearBtn?.addEventListener("click", () => {
    try {
      clearToken();
      if (input) input.value = "";
      setArmedState("unarmed");
      setStatusMsg("Token cleared.");
    } catch (err) {
      setStatusMsg(`Could not clear token: ${err instanceof Error ? err.message : String(err)}`, "error");
    }
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

  if (getToken() === null) {
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
  window.setInterval(() => void poll(), POLL_INTERVAL_MS);
}
