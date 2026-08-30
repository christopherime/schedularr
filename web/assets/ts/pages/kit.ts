// Dev-only /kit/ page: renders every ui/* partial in every state from
// fixture data -- no API calls, nothing persisted. This page is the review
// gate for the component floor (spec §4): a slice is not done until its
// new states appear here. Excluded from production builds (see
// web/config/production/hugo.toml); `hugo -s web -e development` builds it.
import { channelLabel, channelPlate } from "../runtime/channels.ts";
import type { Channel, PlateParts } from "../runtime/channels.ts";
import type { ProblemView } from "../runtime/errors.ts";
import { relativeTime } from "../runtime/format.ts";
import { initShell } from "../runtime/shell.ts";
import { printTape } from "../runtime/tape.ts";

initShell();

declare const Alpine: {
  data<T extends object>(name: string, factory: () => T): void;
};

const fixtureChannels: Channel[] = [
  { id: "fixture-0001", name: "Horror", number: 4 },
  { id: "fixture-0002", name: "Cartoons", number: 7 },
  { id: "fixture-0003", name: "Sitcoms", number: 12 },
];

const fixtureProblem: ProblemView = {
  title: "tunarr unreachable",
  detail: "unable to reach tunarr",
  requestId: "3f9c2a7e51d84b0c",
};

const fixtureProblemBare: ProblemView = {
  title: "request timed out",
  detail: null,
  requestId: null,
};

interface KitState {
  channels: Channel[];
  problem: ProblemView;
  problemBare: ProblemView;
  toggleOn: boolean;
  toggleOff: boolean;
  channelsLoading: boolean;
  channelsError: string | null;
  channelMode: "loaded" | "loading" | "error" | "empty";
  selectedChannel: string;
  busyDemo: boolean;
  invalidValue: string;
  confirmArmed: boolean;

  init(): void;
  plate(id: string): PlateParts;
  channelLabel(c: Channel): string;
  channelHint(): string;
  applyChannelMode(): void;
  relative(msOffset: number): string;
  tapeDemo(): void;
  tapeActionDemo(): void;
  busyPulse(): void;
  openConfirm(): void;
  cancelConfirm(): void;
  performConfirm(): void;
}

interface WithRefs {
  $refs: { confirmDialog: HTMLDialogElement };
}

document.addEventListener("alpine:init", () => {
  Alpine.data(
    "kit",
    (): KitState & ThisType<KitState & WithRefs> => ({
      channels: fixtureChannels,
      problem: fixtureProblem,
      problemBare: fixtureProblemBare,
      toggleOn: true,
      toggleOff: false,
      channelsLoading: false,
      channelsError: null,
      channelMode: "loaded",
      selectedChannel: "",
      busyDemo: false,
      invalidValue: "not a number",
      confirmArmed: false,

      init() {
        // Fixture-only page: nothing to fetch.
      },

      plate(id) {
        return channelPlate(id, this.channels);
      },

      channelLabel,

      channelHint() {
        if (this.channelsLoading) return "Loading channels from Tunarr…";
        if (this.channelsError) {
          return `Tunarr channel list unavailable (${this.channelsError}) — enter the channel ID manually.`;
        }
        if (this.channels.length === 0) {
          return "Tunarr returned no channels — enter the channel ID manually.";
        }
        return "";
      },

      applyChannelMode() {
        this.channelsLoading = this.channelMode === "loading";
        this.channelsError =
          this.channelMode === "error" ? "dial tcp 10.0.0.7:8000: connection refused" : null;
        this.channels = this.channelMode === "loaded" ? fixtureChannels : [];
      },

      relative(msOffset) {
        return relativeTime(new Date(Date.now() + msOffset).toISOString());
      },

      tapeDemo() {
        printTape("Block saved — Spooky Saturday Night");
      },

      tapeActionDemo() {
        printTape("Supernatural disabled", {
          label: "Undo",
          run: () => printTape("Supernatural re-enabled"),
        });
      },

      busyPulse() {
        this.busyDemo = true;
        window.setTimeout(() => {
          this.busyDemo = false;
        }, 2500);
      },

      openConfirm() {
        this.confirmArmed = true;
        this.$refs.confirmDialog.showModal();
      },

      cancelConfirm() {
        this.$refs.confirmDialog.close();
        this.confirmArmed = false;
      },

      performConfirm() {
        this.cancelConfirm();
        printTape("Kit confirm — confirmed");
      },
    }),
  );
});
