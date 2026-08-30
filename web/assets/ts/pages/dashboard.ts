// Dashboard page ("/"): status readout (version, Tunarr reachability, block
// count) plus recent scheduling history. One esbuild bundle per page since
// the v0.5.0 runtime refactor: this entry compiles the shared runtime
// (runtime/*) in with itself, so ApiError identity, formatting, and
// channel labeling are the same modules every other page uses.
import { apiGet, apiPath, onReauth } from "../runtime/api.ts";
import type { ApiResponse } from "../runtime/api.ts";
import { channelPlate, loadChannels } from "../runtime/channels.ts";
import type { Channel, PlateParts } from "../runtime/channels.ts";
import { toProblemView } from "../runtime/errors.ts";
import type { ProblemView } from "../runtime/errors.ts";
import { formatLocal, plural } from "../runtime/format.ts";
import { initShell } from "../runtime/shell.ts";

initShell();

type Status = ApiResponse<"getStatus", 200>;
type HistoryEntry = ApiResponse<"getHistory", 200>[number];

// Alpine's global is supplied at runtime by vendor/alpine.min.js; there is
// no @types package for the vendored build, so this is the minimal surface
// this file actually calls.
declare const Alpine: {
  data<T extends object>(name: string, factory: () => T): void;
};

// Alpine auto-invokes a data object's own init() method (documented
// behavior -- https://alpinejs.dev/globals/alpine-data), so nothing may
// also wire x-init="init()" to it. This guard is defense-in-depth against
// a future accidental double-wire, not a workaround for a live bug.
let started = false;

interface DashboardState {
  statusLoading: boolean;
  statusError: ProblemView | null;
  status: Status | null;

  historyLoading: boolean;
  historyError: ProblemView | null;
  history: HistoryEntry[];

  channels: Channel[];

  init(): void;
  loadStatus(): Promise<void>;
  loadHistory(): Promise<void>;
  loadPlateChannels(): void;
  formatLocal(iso: string | undefined): string;
  blocksLabel(n: number | undefined): string;
  plate(id: string | undefined): PlateParts;
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

      channels: [],

      init() {
        if (started) return;
        started = true;
        void this.loadStatus();
        void this.loadHistory();
        // Arming a new token re-fires whichever loads failed -- the
        // re-auth broadcast from the token panel's successful probe.
        onReauth(() => {
          if (this.statusError) void this.loadStatus();
          if (this.historyError) void this.loadHistory();
          // The plate channels are a best-effort side fetch, so a 401 on
          // /channels alone leaves historyError null and nothing above
          // re-fires -- refetch directly whenever the plates are still on
          // their shortened-id fallback (the re-auth broadcast has
          // already invalidated the channel cache), so they recover
          // without a reload.
          if (this.channels.length === 0) this.loadPlateChannels();
        });
      },

      async loadStatus() {
        this.statusLoading = true;
        this.statusError = null;
        try {
          this.status = await apiGet<Status>(apiPath("/status"));
        } catch (err) {
          this.statusError = toProblemView(err);
        } finally {
          this.statusLoading = false;
        }
      },

      async loadHistory() {
        this.historyLoading = true;
        this.historyError = null;
        try {
          this.loadPlateChannels();
          this.history = await apiGet<HistoryEntry[]>(apiPath("/history", undefined, { days: 7 }));
        } catch (err) {
          this.historyError = toProblemView(err);
        } finally {
          this.historyLoading = false;
        }
      },

      // The channel cache feeds the history table's legend plates;
      // best-effort -- a failed channel fetch leaves plates on their
      // shortened-id fallback rather than failing the section. Called
      // alongside loadHistory and again from the re-auth handler (see
      // init) so plates recover once a working token is armed.
      loadPlateChannels() {
        void loadChannels().then(
          (channels) => {
            this.channels = channels;
          },
          () => undefined,
        );
      },

      formatLocal,

      // Status.blocks is optional (a store error omits it, see
      // internal/api/tunarr.go's GetStatus) -- undefined renders as an em
      // dash rather than a fabricated "0 blocks".
      blocksLabel(n) {
        if (n === undefined) return "—";
        return plural(n, "block");
      },

      plate(id) {
        return channelPlate(id, this.channels);
      },
    }),
  );
});
