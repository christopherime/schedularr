// Dashboard page ("/"): status readout (version, Tunarr reachability, block
// count) plus recent scheduling history. Bundled separately from main.ts via
// the "page_js" block (see web/layouts/_default/baseof.html) and loaded
// before vendor/alpine.min.js, so the Alpine.data() registration below is
// already in place when Alpine dispatches "alpine:init" on startup.
//
// This bundle never calls fetch() itself -- every request goes through
// window.schedularr.apiGet, published once by main.ts (see api.ts). Errors
// are read via window.schedularr.ApiError specifically (not a locally
// imported copy of the class) because main.ts and this file are compiled as
// two separate esbuild bundles; only the instance living on `window` is
// guaranteed to be the same class identity that apiGet() actually throws,
// so that's the one `instanceof` has to check against.
import type { ApiResponse } from "../api";

type Status = ApiResponse<"getStatus", 200>;
type HistoryEntry = ApiResponse<"getHistory", 200>[number];

// Alpine's global is supplied at runtime by vendor/alpine.min.js; there is
// no @types package for the vendored build, so this is the minimal surface
// this file actually calls.
declare const Alpine: {
  data<T extends object>(name: string, factory: () => T): void;
};

// Guards against a double x-init call firing two redundant sets of fetches
// on the same page load. Module-level (not per-component-instance state)
// because it needs to hold even if something re-runs the Alpine.data
// factory itself, not just re-invokes init() on the same instance -- found
// empirically via a jsdom-based verification harness (two full
// status+history fetch pairs landed within 1ms of each other on load); a
// real browser may or may not hit the same path, but the guard is correct
// and cheap either way: one dashboard load should mean one status fetch and
// one history fetch, never two.
let started = false;

interface DashboardState {
  statusLoading: boolean;
  statusError: string | null;
  status: Status | null;

  historyLoading: boolean;
  historyError: string | null;
  history: HistoryEntry[];

  init(): void;
  loadStatus(): Promise<void>;
  loadHistory(): Promise<void>;
  formatLocal(iso: string | undefined): string;
}

/** Renders an ApiError as its problem title (+ detail, when present); any
 * other thrown value falls back to its message. Callers assign the result
 * via x-text, which sets textContent -- never innerHTML. */
function describeError(err: unknown): string {
  if (err instanceof window.schedularr.ApiError) {
    return err.detail ? `${err.title}: ${err.detail}` : err.title;
  }
  return err instanceof Error ? err.message : String(err);
}

document.addEventListener("alpine:init", () => {
  Alpine.data(
    "dashboard",
    (): DashboardState => ({
      statusLoading: true,
      statusError: null,
      status: null,

      historyLoading: true,
      historyError: null,
      history: [],

      init() {
        if (started) return;
        started = true;
        void this.loadStatus();
        void this.loadHistory();
      },

      async loadStatus() {
        this.statusLoading = true;
        this.statusError = null;
        try {
          this.status = await window.schedularr.apiGet<Status>("/api/v1/status");
        } catch (err) {
          this.statusError = describeError(err);
        } finally {
          this.statusLoading = false;
        }
      },

      async loadHistory() {
        this.historyLoading = true;
        this.historyError = null;
        try {
          this.history = await window.schedularr.apiGet<HistoryEntry[]>("/api/v1/history?days=7");
        } catch (err) {
          this.historyError = describeError(err);
        } finally {
          this.historyLoading = false;
        }
      },

      // scheduled_at is optional on the wire (HistoryEntry has no required
      // fields); render an em dash rather than guessing. Formatting uses the
      // browser's local timezone and locale -- the binding contract calls
      // for local time, not the UTC wire value.
      formatLocal(iso) {
        if (!iso) return "—";
        const d = new Date(iso);
        if (Number.isNaN(d.getTime())) return iso;
        return d.toLocaleString(undefined, {
          year: "numeric",
          month: "2-digit",
          day: "2-digit",
          hour: "2-digit",
          minute: "2-digit",
        });
      },
    }),
  );
});
