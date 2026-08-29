// Shell entry point: bundled once by Hugo's js.Build and loaded (deferred)
// on every page via web/layouts/_default/baseof.html. Two jobs:
//
//   1. Publish window.schedularr so page modules (Tasks 4-7) share one
//      compiled copy of api.ts/token.ts instead of each bundling their own.
//   2. Wire the token panel: open automatically when no token is stored or
//      on the first 401, save/clear, and the header trigger button.
import { apiGet, apiSend, ApiError, onUnauthorized } from "./api";
import { getToken, setToken, clearToken } from "./token";

declare global {
  interface Window {
    schedularr: {
      apiGet: typeof apiGet;
      apiSend: typeof apiSend;
      ApiError: typeof ApiError;
      getToken: typeof getToken;
      setToken: typeof setToken;
      clearToken: typeof clearToken;
    };
  }
}

window.schedularr = { apiGet, apiSend, ApiError, getToken, setToken, clearToken };

// ---- token panel ---------------------------------------------------------

function el<T extends HTMLElement>(id: string): T | null {
  return document.getElementById(id) as T | null;
}

const dialog = el<HTMLDialogElement>("token-panel");
const form = el<HTMLFormElement>("token-form");
const trigger = el<HTMLButtonElement>("token-trigger");
const closeBtn = el<HTMLButtonElement>("token-panel-close");
const cancelBtn = el<HTMLButtonElement>("token-cancel");
const clearBtn = el<HTMLButtonElement>("token-clear");
const input = el<HTMLInputElement>("token-input");
const statusEl = el<HTMLParagraphElement>("token-panel-status");
const statusDot = el<HTMLSpanElement>("token-status");
const statusLabel = el<HTMLSpanElement>("token-status-label");

function setStatus(text: string, tone: "info" | "error" = "info"): void {
  if (!statusEl) return;
  statusEl.textContent = text;
  statusEl.dataset.tone = tone;
}

function refreshIndicator(): void {
  const armed = getToken() !== null;
  if (statusDot) statusDot.dataset.state = armed ? "armed" : "unarmed";
  if (statusLabel) statusLabel.textContent = armed ? "Armed" : "Unarmed";
  if (trigger) trigger.setAttribute("aria-label", armed ? "API token stored — open token panel" : "No API token stored — open token panel");
}

function openPanel(): void {
  if (!dialog || dialog.open) return;
  if (input) input.value = getToken() ?? "";
  setStatus("");
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
// itself, not one of its children) closes it too, matching standard
// modal conventions.
dialog?.addEventListener("click", (event) => {
  if (event.target === dialog) closePanel();
});

dialog?.addEventListener("close", refreshIndicator);

form?.addEventListener("submit", (event) => {
  event.preventDefault();
  const value = input?.value.trim() ?? "";
  if (!value) {
    setStatus("Enter a token, or use Clear to remove it.", "error");
    input?.focus();
    return;
  }
  try {
    setToken(value);
    refreshIndicator();
    closePanel();
  } catch (err) {
    setStatus(`Could not save token: ${err instanceof Error ? err.message : String(err)}`, "error");
  }
});

clearBtn?.addEventListener("click", () => {
  try {
    clearToken();
    if (input) input.value = "";
    refreshIndicator();
    setStatus("Token cleared.");
  } catch (err) {
    setStatus(`Could not clear token: ${err instanceof Error ? err.message : String(err)}`, "error");
  }
});

// Binding contract: opens automatically when no token is stored, or on
// the first 401 any page's API call receives. No auto-retry -- the user
// re-triggers whatever action failed after saving a new token.
onUnauthorized(() => {
  openPanel();
  setStatus("Request rejected (401 Unauthorized). Enter a valid token.", "error");
});

refreshIndicator();
if (getToken() === null) {
  openPanel();
}
