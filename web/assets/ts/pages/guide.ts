// The Guide ("/"): the EPG grid as home (spec §3.1 + §3.2, v0.5.1
// read-only slice). Auto-loads GET /schedule on open -- the manual
// Generate click is dead for reading; the old Schedule page still owns
// preview/apply until v0.5.2 absorbs it as the guide's draft mode.
//
// Division of labor (spec non-negotiable): the grid + rundown DOM is
// built in TS by runtime/grid.ts from the typed plan; Alpine drives ONLY
// the toolbar (DAYS / SCOPE / day tabs / mobile channel picker) and the
// slot inspector. In THIS read-only slice a DAYS/SCOPE change re-fetches
// the plan (draft mode arrives in v0.5.2); day tabs navigate the loaded
// window and never re-plan.
//
// The now-line advances on a local 60s timer (browser clock); heartbeat
// skew correction arrives with SSE in v0.5.4.
import { apiGet, apiPath, onReauth } from "../runtime/api.ts";
import type { ApiResponse } from "../runtime/api.ts";
import { channelHint as channelHintText, channelLabel, channelOrder, channelPlate, loadChannels } from "../runtime/channels.ts";
import type { Channel } from "../runtime/channels.ts";
import { cronReadback } from "../runtime/cron.ts";
import { toProblemView } from "../runtime/errors.ts";
import type { ProblemView } from "../runtime/errors.ts";
import { clampDays, durationLabel, formatClock, plural, sxxeyy } from "../runtime/format.ts";
import {
  addDays,
  dayLabel,
  localDayStart,
  renderGuideDay,
  renderRundown,
  resolveGhost,
} from "../runtime/grid.ts";
import type { GhostBlockInfo, GridHandle, GuideRow, GuideSlot, RundownHandle } from "../runtime/grid.ts";
import { initShell } from "../runtime/shell.ts";
import type { components } from "../gen/types";

initShell();

type PlanResult = ApiResponse<"getSchedule", 200>;
type BlockRecord = components["schemas"]["BlockRecord"];

declare const Alpine: {
  data<T extends object>(name: string, factory: () => T): void;
};

// Alpine magics injected at runtime (same ThisType erasure trick as
// blocks.ts uses for $nextTick).
interface WithMagics {
  $nextTick(cb: () => void): void;
}

// Same double-init defense as every page: Alpine auto-invokes init().
let started = false;

// DOM references the renderer hands back live OUTSIDE Alpine state --
// wrapping live elements in Alpine's reactive proxy buys nothing and
// risks proxy-vs-identity surprises on focus return.
let gridHandle: GridHandle | null = null;
let rundownHandle: RundownHandle | null = null;
let inspectorReturnEl: HTMLElement | null = null;

interface DayTab {
  index: number;
  label: string;
  isToday: boolean;
}

interface InspectorState {
  open: boolean;
  slot: GuideSlot | null;
}

interface GuideState {
  controls: { days: string; channelId: string };

  channelsLoading: boolean;
  channelsError: string | null;
  channels: Channel[];

  loading: boolean;
  problem: ProblemView | null;
  plan: PlanResult | null;
  /** BlockRecords by name -- inspector enrichment (id for the editor
   * deep-link, enabled state) and interim ghost placement (see
   * runtime/grid.ts's resolveGhost). Degrades silently when the fetch
   * fails: links fall back to /blocks/, ghosts to the winner's channel. */
  blocksByName: Record<string, BlockRecord>;

  /** The loaded window: local midnight of day 0 + how many days. */
  windowStartMs: number;
  loadedDays: number;
  dayIndex: number;

  rundownChannelId: string;
  inspector: InspectorState;

  init(): void;
  loadChannels(): Promise<void>;
  reload(): Promise<void>;
  onDaysChange(): void;
  channelLabel(c: Channel): string;
  channelHint(): string;

  dayTabs(): DayTab[];
  selectDay(index: number): void;

  rows(): GuideRow[];
  rundownRow(): GuideRow | null;
  rundownChannels(): { id: string; label: string }[];
  hasAnySlots(): boolean;
  hasBlocks(): boolean;
  renderAll(): void;
  renderRundownOnly(): void;
  tick(): void;

  openInspector(slot: GuideSlot, el: HTMLElement): void;
  closeInspector(): void;
  inspectorBlock(): BlockRecord | null;
  inspectorEditHref(): string;
  winnerEditHref(): string;
  inspectorTimeRange(): string;
  inspectorDuration(): string;
  inspectorPlate(): { ch: string | null; name: string };
  inspectorCron(): string;
  inspectorCronReadback(): string | null;
  inspectorPriority(): string;
  inspectorEnabled(): string | null;
  inspectorPrograms(): { start: string; title: string; marker: string | null; duration: string }[];
  problemLine(): string;
  programCountLabel(): string;
}

document.addEventListener("alpine:init", () => {
  Alpine.data(
    "guide",
    (): GuideState & ThisType<GuideState & WithMagics> => ({
      controls: { days: "7", channelId: "" },

      channelsLoading: true,
      channelsError: null,
      channels: [],

      loading: true,
      problem: null,
      plan: null,
      blocksByName: {},

      windowStartMs: localDayStart(Date.now()),
      loadedDays: 7,
      dayIndex: 0,

      rundownChannelId: "",
      inspector: { open: false, slot: null },

      init() {
        if (started) return;
        started = true;
        void this.loadChannels();
        void this.reload();
        onReauth(() => {
          if (this.channelsError) void this.loadChannels();
          if (this.problem) void this.reload();
        });
        // The sweep's minute advance: a discrete step on a local 60s
        // timer, not an animation loop (motion inventory item 1).
        window.setInterval(() => this.tick(), 60_000);
      },

      async loadChannels() {
        this.channelsLoading = true;
        this.channelsError = null;
        try {
          this.channels = await loadChannels();
        } catch (err) {
          const p = toProblemView(err);
          this.channelsError = p.detail ?? p.title;
          this.channels = [];
        } finally {
          this.channelsLoading = false;
        }
        // Plates resolve lazily: if the plan landed before the channel
        // list, re-render so UUID fallbacks become names.
        if (this.plan) this.renderAll();
      },

      // One load = plan + blocks, in parallel. The plan is load-bearing
      // (its failure IS the NO SIGNAL state); the blocks fetch only
      // enriches (inspector deep-links, ghost placement) and degrades
      // silently, matching the blocks editor's media-fetch convention.
      async reload() {
        this.closeInspector();
        this.loading = true;
        this.problem = null;
        const days = clampDays(this.controls.days);
        this.controls.days = String(days);
        const channelId = this.controls.channelId.trim();
        const planPromise = apiGet<PlanResult>(
          apiPath("/schedule", undefined, { days, channel_id: channelId === "" ? undefined : channelId }),
        );
        // The catch is attached up front: a blocks failure that lands
        // while the plan is still in flight must degrade silently, not
        // surface as an unhandled rejection.
        const blocksPromise = apiGet<BlockRecord[]>(apiPath("/blocks")).catch((): BlockRecord[] | null => null);
        try {
          this.plan = await planPromise;
          this.windowStartMs = localDayStart(Date.now());
          this.loadedDays = days;
          this.dayIndex = 0;
        } catch (err) {
          this.problem = toProblemView(err);
          this.plan = null;
        }
        const blocks = await blocksPromise;
        const byName: Record<string, BlockRecord> = {};
        for (const b of blocks ?? []) byName[b.name] = b;
        this.blocksByName = byName;
        this.loading = false;
        if (this.plan) this.renderAll();
      },

      onDaysChange() {
        void this.reload();
      },

      channelLabel,

      channelHint() {
        return channelHintText(
          this.channelsLoading,
          this.channelsError,
          this.channels,
          "enter a channel ID manually, or leave blank for all channels",
        );
      },

      dayTabs() {
        const tabs: DayTab[] = [];
        const todayStart = localDayStart(Date.now());
        for (let k = 0; k < this.loadedDays; k++) {
          const startMs = addDays(this.windowStartMs, k);
          tabs.push({ index: k, label: dayLabel(startMs), isToday: startMs === todayStart });
        }
        return tabs;
      },

      // Day tabs are navigation only -- they never re-plan (spec §3.1).
      selectDay(index) {
        if (index === this.dayIndex) return;
        this.dayIndex = index;
        this.closeInspector();
        this.renderAll();
      },

      // The full renderable model: one row per planned channel, plate
      // resolved from the channel cache, slots + ghosts sorted by start.
      rows() {
        if (!this.plan) return [];
        const rowsByChannel = new Map<string, GuideSlot[]>();
        for (const [channelId, slots] of Object.entries(this.plan.channels)) {
          const list: GuideSlot[] = [];
          for (const s of slots) {
            const startMs = s.start_time ? Date.parse(s.start_time) : NaN;
            const endMs = s.end_time ? Date.parse(s.end_time) : NaN;
            if (Number.isNaN(startMs) || Number.isNaN(endMs)) continue;
            list.push({
              kind: "slot",
              channelId,
              blockName: s.block?.name ?? "—",
              blockType: s.block?.type ?? "filter",
              cron: s.block?.cron ?? "",
              priority: s.block?.priority ?? 0,
              startMs,
              endMs,
              programs: (s.programs ?? []).map((p) => ({
                title: p.title,
                type: p.type,
                season: p.season,
                episode: p.episode,
                durationMs: p.duration_ms,
                startMs: Date.parse(p.start_time),
              })),
            });
          }
          rowsByChannel.set(channelId, list);
        }
        // Ghost slots for current-plan warnings, at their would-have-aired
        // time. A conflict is always same-channel, so the ghost's channel
        // always already has a row in the plan.
        const ghostLookup = new Map<string, GhostBlockInfo>();
        for (const [name, b] of Object.entries(this.blocksByName)) {
          ghostLookup.set(name, { channelId: b.spec.channel_id, durationMinutes: b.spec.duration });
        }
        for (const w of this.plan.warnings ?? []) {
          const ghost = resolveGhost(w, ghostLookup);
          if (!ghost) continue;
          const list = rowsByChannel.get(ghost.channelId);
          if (list) list.push(ghost);
        }
        return [...rowsByChannel.entries()]
          .map(([channelId, slots]) => ({
            channelId,
            plate: channelPlate(channelId, this.channels),
            slots: slots.sort((a, b) => a.startMs - b.startMs),
          }))
          .sort((a, b) => channelOrder(a.channelId, b.channelId, this.channels));
      },

      rundownRow() {
        const rows = this.rows();
        if (rows.length === 0) return null;
        return rows.find((r) => r.channelId === this.rundownChannelId) ?? rows[0];
      },

      rundownChannels() {
        return this.rows().map((r) => ({
          id: r.channelId,
          label: r.plate.ch ? `${r.plate.ch} · ${r.plate.name}` : r.plate.name,
        }));
      },

      hasAnySlots() {
        if (!this.plan) return false;
        if (Object.values(this.plan.channels).some((slots) => slots.length > 0)) return true;
        return (this.plan.warnings ?? []).length > 0;
      },

      hasBlocks() {
        return Object.keys(this.blocksByName).length > 0;
      },

      // Deferred one tick: renderAll is called right after reactive state
      // flips (loading -> false), and the viewport sits behind x-show --
      // rendering before Alpine applies the display change would measure
      // a display:none element (clientWidth 0) and the auto-scroll would
      // silently no-op.
      renderAll() {
        this.$nextTick(() => {
          const viewport = document.getElementById("guide-viewport");
          if (!viewport) return;
          const rows = this.rows();
          const dayStartMs = addDays(this.windowStartMs, this.dayIndex);
          gridHandle = renderGuideDay(viewport, rows, dayStartMs, {
            onOpen: (slot, el) => this.openInspector(slot, el),
          });
          gridHandle.updateNow(Date.now());
          // Auto-scroll target on open: the sweep cursor, parked a third
          // of the viewport in so the next hours are visible (initial
          // positioning, not motion -- no smooth scrolling here). The
          // nextTick above is not a guarantee the x-show display change
          // has painted, and a display:none viewport measures 0 -- wait
          // it out over a few frames instead of silently landing on 0.
          const scrollToNow = (attempts: number): void => {
            if (attempts <= 0) return;
            if (viewport.clientWidth === 0) {
              requestAnimationFrame(() => scrollToNow(attempts - 1));
              return;
            }
            const nowX = gridHandle?.nowOffsetPx(Date.now());
            if (nowX != null) {
              viewport.scrollLeft = Math.max(0, nowX - viewport.clientWidth / 3);
            }
          };
          scrollToNow(20);
          this.renderRundownOnly();
        });
      },

      renderRundownOnly() {
        const rundownEl = document.getElementById("guide-rundown");
        if (!rundownEl) return;
        const row = this.rundownRow();
        if (row) this.rundownChannelId = row.channelId;
        rundownHandle = renderRundown(rundownEl, row, this.windowStartMs, this.loadedDays, {
          onOpen: (slot, el) => this.openInspector(slot, el),
        });
        rundownHandle.updateNow(Date.now());
      },

      tick() {
        gridHandle?.updateNow(Date.now());
        rundownHandle?.updateNow(Date.now());
      },

      openInspector(slot, el) {
        inspectorReturnEl = el;
        this.inspector.slot = slot;
        this.inspector.open = true;
      },

      // Esc/X close with focus returned to the slot that opened it.
      closeInspector() {
        if (!this.inspector.open) return;
        this.inspector.open = false;
        this.inspector.slot = null;
        inspectorReturnEl?.focus();
        inspectorReturnEl = null;
      },

      inspectorBlock() {
        const slot = this.inspector.slot;
        if (!slot) return null;
        return this.blocksByName[slot.blockName] ?? null;
      },

      // Block-name deep link: /blocks/?edit=<id> opens the editor on that
      // block (blocks.ts reads the param after its list loads). Falls back
      // to the plain blocks page when the record didn't resolve.
      inspectorEditHref() {
        const record = this.inspectorBlock();
        return record ? `/blocks/?edit=${encodeURIComponent(record.id)}` : "/blocks/";
      },

      winnerEditHref() {
        const name = this.inspector.slot?.lostTo;
        const record = name ? this.blocksByName[name] : undefined;
        return record ? `/blocks/?edit=${encodeURIComponent(record.id)}` : "/blocks/";
      },

      inspectorTimeRange() {
        const slot = this.inspector.slot;
        if (!slot) return "";
        return `${formatClock(slot.startMs)}–${formatClock(slot.endMs)}`;
      },

      inspectorDuration() {
        const slot = this.inspector.slot;
        if (!slot) return "";
        return durationLabel((slot.endMs - slot.startMs) / 60_000);
      },

      inspectorPlate() {
        return channelPlate(this.inspector.slot?.channelId, this.channels);
      },

      inspectorCron() {
        const slot = this.inspector.slot;
        if (!slot) return "";
        return slot.cron !== "" ? slot.cron : (this.inspectorBlock()?.spec.cron ?? "");
      },

      inspectorCronReadback() {
        const cron = this.inspectorCron();
        return cron === "" ? null : cronReadback(cron);
      },

      inspectorPriority() {
        const slot = this.inspector.slot;
        if (!slot) return "";
        const record = this.inspectorBlock();
        const priority = slot.kind === "ghost" ? (record?.spec.priority ?? slot.priority) : slot.priority;
        return String(priority);
      },

      // Enabled state comes from the BlockRecord (BlockSpec doesn't carry
      // it); null when the blocks fetch failed -- the row is omitted
      // rather than guessed.
      inspectorEnabled() {
        const record = this.inspectorBlock();
        if (!record) return null;
        return record.enabled ? "Enabled" : "Disabled";
      },

      inspectorPrograms() {
        const slot = this.inspector.slot;
        if (!slot) return [];
        return slot.programs.map((p) => ({
          start: Number.isNaN(p.startMs) ? "—" : formatClock(p.startMs),
          title: p.title.trim() === "" ? "—" : p.title,
          marker: sxxeyy(p.season, p.episode),
          duration: durationLabel(p.durationMs / 60_000),
        }));
      },

      problemLine() {
        if (!this.problem) return "";
        return this.problem.detail ? `${this.problem.title}: ${this.problem.detail}` : this.problem.title;
      },

      programCountLabel() {
        return plural(this.inspector.slot?.programs.length ?? 0, "program");
      },
    }),
  );
});
