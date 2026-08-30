// Dev-only /kit/ page: renders every ui/* partial in every state from
// fixture data. The gallery's own components never call the API or
// persist anything -- but the shell it initializes below is the real one,
// same as every page: the bezel polls /status every 60s and the token
// panel arms for real, so the shell chrome is reviewable here too rather
// than being a dead mock. This page is the review gate for the component
// floor (spec §4): a slice is not done until its new states appear here.
// Excluded from production builds (see web/config/production/hugo.toml);
// `hugo -s web -e development` builds it.
import { channelHint as channelHintText, channelLabel, channelPlate } from "../runtime/channels.ts";
import type { Channel, PlateParts } from "../runtime/channels.ts";
import type { ProblemView } from "../runtime/errors.ts";
import { relativeTime, untilTime } from "../runtime/format.ts";
import { localDayStart, renderGuideDay } from "../runtime/grid.ts";
import type { GuideRow, GuideSlot } from "../runtime/grid.ts";
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

// ---- guide grid fixtures ---------------------------------------------------
//
// The REAL renderer (runtime/grid.ts) on fixture data: an on-air slot, a
// past slot dimmed behind the sweep, a series-tinted slot, a NO SIGNAL
// ghost in its sub-lane, and an overnight spill cut at the day's right
// edge -- every slot state the guide can render, reviewable without a
// server. Times are derived from "now" so the sweep cursor always crosses
// the fixture.

function fixtureSlot(
  channelId: string,
  blockName: string,
  blockType: string,
  startMs: number,
  minutes: number,
  programCount: number,
): GuideSlot {
  const programs = [];
  for (let i = 0; i < programCount; i++) {
    programs.push({
      title: `${blockName} program ${i + 1}`,
      type: blockType === "series" ? "episode" : "movie",
      season: blockType === "series" ? 1 : undefined,
      episode: blockType === "series" ? i + 1 : undefined,
      durationMs: (minutes / programCount) * 60_000,
      startMs: startMs + (minutes / programCount) * 60_000 * i,
    });
  }
  return {
    kind: "slot",
    channelId,
    blockName,
    blockType,
    cron: "0 21 * * 6",
    priority: 50,
    startMs,
    endMs: startMs + minutes * 60_000,
    programs,
  };
}

function fixtureGuideRows(): GuideRow[] {
  const now = Date.now();
  const hour = 3_600_000;
  const ghost: GuideSlot = {
    kind: "ghost",
    channelId: "fixture-0001",
    blockName: "Late Sitcom Loop",
    blockType: "filter",
    cron: "",
    priority: 10,
    startMs: now + 2 * hour,
    endMs: now + 3 * hour,
    programs: [],
    lostTo: "Spooky Saturday Night",
  };
  return [
    {
      channelId: "fixture-0001",
      plate: channelPlate("fixture-0001", fixtureChannels),
      slots: [
        fixtureSlot("fixture-0001", "Morning Creatures", "filter", now - 4 * hour, 120, 4),
        fixtureSlot("fixture-0001", "Matinee Massacre", "filter", now - 0.5 * hour, 90, 2),
        fixtureSlot("fixture-0001", "Spooky Saturday Night", "series", now + 2 * hour, 120, 4),
        ghost,
        fixtureSlot("fixture-0001", "Graveyard Shift", "filter", localDayStart(now) + 23.5 * hour, 120, 3),
      ],
    },
    {
      channelId: "fixture-0002",
      plate: channelPlate("fixture-0002", fixtureChannels),
      slots: [
        fixtureSlot("fixture-0002", "Cereal Cartoons", "series", now - 2 * hour, 180, 6),
        fixtureSlot("fixture-0002", "After School", "filter", now + 4 * hour, 60, 2),
      ],
    },
    {
      channelId: "fixture-0003",
      plate: channelPlate("fixture-0003", fixtureChannels),
      slots: [fixtureSlot("fixture-0003", "Laugh Track", "filter", now + hour, 240, 8)],
    },
  ];
}

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
  confirmBusy: boolean;

  init(): void;
  plate(id: string): PlateParts;
  channelLabel(c: Channel): string;
  channelHint(): string;
  applyChannelMode(): void;
  relative(msOffset: number): string;
  until(msOffset: number): string;
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
      confirmBusy: false,

      init() {
        // Fixture-only component: nothing to fetch. The guide grid is the
        // one section that renders through real runtime code
        // (runtime/grid.ts) rather than static markup -- the gallery must
        // exercise what ships.
        const viewport = document.getElementById("kit-guide-viewport");
        if (viewport) {
          const handle = renderGuideDay(viewport, fixtureGuideRows(), localDayStart(Date.now()), {
            onOpen: (slot) => printTape(`Inspector would open — ${slot.blockName}`),
          });
          handle.updateNow(Date.now());
          const nowX = handle.nowOffsetPx(Date.now());
          if (nowX !== null) viewport.scrollLeft = Math.max(0, nowX - viewport.clientWidth / 3);
        }
      },

      plate(id) {
        return channelPlate(id, this.channels);
      },

      channelLabel,

      // The SAME shared helper the blocks page uses (runtime/channels.ts)
      // -- the gallery must exercise what ships, not a lookalike copy
      // that can drift.
      channelHint() {
        return channelHintText(this.channelsLoading, this.channelsError, this.channels);
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

      // The NEXT TICK variant: a past instant reads "due" (an overrunning
      // cron tick), never "N min ago".
      until(msOffset) {
        return untilTime(new Date(Date.now() + msOffset).toISOString());
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
        this.$refs.confirmDialog.showModal();
      },

      // Same state-level guard convention as the real pages (blocks
      // cancelDelete / schedule cancelApply): refuses to close while the
      // simulated action is in flight.
      cancelConfirm() {
        if (this.confirmBusy) return;
        this.$refs.confirmDialog.close();
      },

      // Simulates an in-flight action so the confirm partial's busy state
      // -- disabled buttons, aria-busy sweep, guarded Escape/backdrop --
      // actually renders in the gallery instead of being wired to a
      // literal "false".
      performConfirm() {
        if (this.confirmBusy) return;
        this.confirmBusy = true;
        window.setTimeout(() => {
          this.confirmBusy = false;
          this.cancelConfirm();
          printTape("Kit confirm — confirmed");
        }, 1500);
      },
    }),
  );
});
