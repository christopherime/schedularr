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
import { clampDays, durationLabel, formatClock, ordinal, plural, sxxeyy } from "../runtime/format.ts";
import {
  addDays,
  dayLabel,
  localDayStart,
  renderGuideDay,
  renderRundown,
  resolveGhost,
  weekPageCount,
  weekPageDays,
  weekPageOf,
  windowDayCount,
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

/** The projected renderable model plus the honesty counter: warnings
 * that could not be placed as ghosts (blocks enrichment failed, or a
 * block vanished between plan and render). */
interface RowsProjection {
  rows: GuideRow[];
  dropped: number;
}

// projection() memo: the GuideSlot graph is identical for a given
// (plan, blocksByName, channels) triple, but one render pass asks for
// it several times (renderAll, the rundown, the Alpine x-for over
// rundownChannels) -- ~60-90k throwaway allocations per pass at scale.
// Keyed on reference identity; reload()/loadChannels() replace all
// three objects wholesale, which is the only way they change.
let rowsMemo: {
  plan: object;
  blocks: object;
  channels: object;
  value: RowsProjection;
} | null = null;

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
  /** Latches a DAYS/SCOPE change that lands while a reload is already
   * in flight -- the flight finishes, then re-fires with the latest
   * control values (the controls stay enabled and keep focus). */
  reloadPending: boolean;
  /** The visually-hidden role="status" line: announces the guide's
   * async states (loading / loaded / unreachable) to screen readers. */
  statusLine: string;
  problem: ProblemView | null;
  plan: PlanResult | null;
  /** BlockRecords by name -- inspector enrichment (id for the editor
   * deep-link, enabled state) and interim ghost placement (see
   * runtime/grid.ts's resolveGhost). Degrades silently when the fetch
   * fails: links fall back to /blocks/, ghosts to the winner's channel. */
  blocksByName: Record<string, BlockRecord>;

  /** The loaded window: local midnight of day 0 + how many calendar
   * days it touches (a trailing partial day included -- see
   * windowDayCount). */
  windowStartMs: number;
  loadedDays: number;
  dayIndex: number;
  /** The week page the tab strip shows (spec §3.1 pager): page k = days
   * k*7..k*7+6 of the loaded window. Paging only moves the strip -- the
   * rendered day changes when a tab is pressed. */
  weekPage: number;

  rundownChannelId: string;
  inspector: InspectorState;

  init(): void;
  loadChannels(): Promise<void>;
  reload(): Promise<void>;
  requestReload(): void;
  channelLabel(c: Channel): string;
  channelHint(): string;

  dayTabs(): DayTab[];
  weekPages(): number;
  pageWeek(delta: number): void;
  selectDay(index: number, tabEl?: HTMLElement): void;

  projection(): RowsProjection;
  rows(): GuideRow[];
  droppedWarnings(): number;
  droppedLegendLine(): string;
  rundownRow(): GuideRow | null;
  rundownChannels(): { id: string; label: string }[];
  hasAnySlots(): boolean;
  hasBlocks(): boolean;
  renderAll(): void;
  renderRundownOnly(): void;
  tick(): void;

  openInspector(slot: GuideSlot, el: HTMLElement): void;
  closeInspector(returnFocus?: boolean): void;
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
      reloadPending: false,
      statusLine: "Loading programme guide",
      problem: null,
      plan: null,
      blocksByName: {},

      windowStartMs: localDayStart(Date.now()),
      loadedDays: 7,
      dayIndex: 0,
      weekPage: 0,

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
        // A re-render is the closer here, not Esc/X: never return focus
        // to a slot node the reload is about to hide or discard.
        this.closeInspector(false);
        this.loading = true;
        this.problem = null;
        this.statusLine = "Loading programme guide";
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
          const landedAt = Date.now();
          this.windowStartMs = localDayStart(landedAt);
          // The server plans [now, now + days*24h): any fetch after
          // midnight spills into a trailing partial calendar day that
          // needs its own tab and rundown section.
          this.loadedDays = windowDayCount(landedAt, days);
          this.dayIndex = 0;
          this.weekPage = 0;
        } catch (err) {
          this.problem = toProblemView(err);
          this.plan = null;
        }
        const blocks = await blocksPromise;
        const byName: Record<string, BlockRecord> = {};
        for (const b of blocks ?? []) byName[b.name] = b;
        this.blocksByName = byName;
        this.loading = false;
        if (this.plan) {
          const { rows, dropped } = this.projection();
          const programCount = rows.reduce(
            (n, r) => n + r.slots.reduce((m, s) => (s.kind === "slot" ? m + s.programs.length : m), 0),
            0,
          );
          let line = `Programme guide loaded, ${plural(programCount, "program")} across ${plural(rows.length, "channel")}`;
          if (dropped > 0) line += `; ${plural(dropped, "occurrence")} dropped by conflicts, placement unavailable`;
          this.statusLine = line;
          this.renderAll();
        } else {
          this.statusLine = "Guide unavailable — Tunarr unreachable";
        }
        // A DAYS/SCOPE change that landed mid-flight re-fires now with
        // the latest control values (x-model already holds them).
        if (this.reloadPending) {
          this.reloadPending = false;
          void this.reload();
        }
      },

      // DAYS/SCOPE change entry point. The controls stay ENABLED during
      // a reload (disabling the focused control would blur it and dump
      // keyboard focus on document.body); a change landing mid-flight
      // is latched and re-fired once the flight lands.
      requestReload() {
        if (this.loading) {
          this.reloadPending = true;
          return;
        }
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

      // The current week page's tabs -- at most seven, every one fully
      // labeled weekday + date (the flat strip's clipped-tab bug cannot
      // recur by construction).
      dayTabs() {
        const todayStart = localDayStart(Date.now());
        return weekPageDays(this.weekPage, this.loadedDays).map((k): DayTab => {
          const startMs = addDays(this.windowStartMs, k);
          return { index: k, label: dayLabel(startMs), isToday: startMs === todayStart };
        });
      },

      weekPages() {
        return weekPageCount(this.loadedDays);
      },

      // ‹/› pager: moves the strip one week, clamped to the window --
      // the rendered day only changes when a tab is pressed.
      pageWeek(delta) {
        this.weekPage = Math.min(this.weekPages() - 1, Math.max(0, this.weekPage + delta));
      },

      // Day tabs are navigation only -- they never re-plan (spec §3.1).
      selectDay(index, tabEl) {
        if (index === this.dayIndex) return;
        this.dayIndex = index;
        this.weekPage = weekPageOf(index);
        // The re-render is about to discard the inspector's return
        // slot: keep focus on the tab the user pressed instead of
        // handing it to a dying node (which strands it on body).
        this.closeInspector(false);
        tabEl?.focus();
        this.renderAll();
      },

      // The full renderable model: one row per planned channel, plate
      // resolved from the channel cache, slots + ghosts sorted by
      // start; plus the count of warnings that could not be placed as
      // ghosts. Memoized on (plan, blocksByName, channels) identity.
      projection() {
        if (!this.plan) return { rows: [], dropped: 0 };
        if (
          rowsMemo &&
          rowsMemo.plan === this.plan &&
          rowsMemo.blocks === this.blocksByName &&
          rowsMemo.channels === this.channels
        ) {
          return rowsMemo.value;
        }
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
        // always already has a row in the plan. A warning that cannot be
        // placed (blocks enrichment failed, block deleted, no matching
        // row) is COUNTED, never silently dropped -- the pinned legend
        // line above the grid keeps the reading honest.
        const ghostLookup = new Map<string, GhostBlockInfo>();
        for (const [name, b] of Object.entries(this.blocksByName)) {
          ghostLookup.set(name, { channelId: b.spec.channel_id, durationMinutes: b.spec.duration });
        }
        let dropped = 0;
        for (const w of this.plan.warnings ?? []) {
          const ghost = resolveGhost(w, ghostLookup);
          const list = ghost ? rowsByChannel.get(ghost.channelId) : undefined;
          if (ghost && list) list.push(ghost);
          else dropped++;
        }
        const rows = [...rowsByChannel.entries()]
          .map(([channelId, slots]) => ({
            channelId,
            plate: channelPlate(channelId, this.channels),
            slots: slots.sort((a, b) => a.startMs - b.startMs),
          }))
          .sort((a, b) => channelOrder(a.channelId, b.channelId, this.channels));
        const value = { rows, dropped };
        rowsMemo = { plan: this.plan, blocks: this.blocksByName, channels: this.channels, value };
        return value;
      },

      rows() {
        return this.projection().rows;
      },

      droppedWarnings() {
        return this.projection().dropped;
      },

      droppedLegendLine() {
        return `${plural(this.droppedWarnings(), "occurrence")} dropped by conflicts — placement unavailable`;
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
            inspectorId: "guide-inspector",
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
          inspectorId: "guide-inspector",
        });
        rundownHandle.updateNow(Date.now());
      },

      tick() {
        gridHandle?.updateNow(Date.now());
        rundownHandle?.updateNow(Date.now());
      },

      // Opening is made perceptible: the opener's aria-expanded flips
      // (every slot button carries aria-controls="guide-inspector"),
      // and focus moves to the inspector heading (tabindex="-1") once
      // Alpine has shown the panel -- which also puts the mobile bottom
      // sheet directly in the tab order instead of after the whole
      // remaining rundown.
      openInspector(slot, el) {
        if (inspectorReturnEl && inspectorReturnEl !== el) {
          inspectorReturnEl.setAttribute("aria-expanded", "false");
        }
        inspectorReturnEl = el;
        el.setAttribute("aria-expanded", "true");
        this.inspector.slot = slot;
        this.inspector.open = true;
        this.$nextTick(() => {
          document.getElementById("guide-inspector-title")?.focus();
        });
      },

      // Esc/X close with focus returned to the slot that opened it.
      // A re-render closer (selectDay/reload) passes returnFocus=false:
      // the return slot is about to be discarded, and focusing a dying
      // node strands keyboard focus on document.body.
      closeInspector(returnFocus = true) {
        if (!this.inspector.open) return;
        this.inspector.open = false;
        this.inspector.slot = null;
        inspectorReturnEl?.setAttribute("aria-expanded", "false");
        if (returnFocus) inspectorReturnEl?.focus();
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

      // §3.2 rank context: "50 · 2nd of 5" among ENABLED same-channel
      // blocks, computed from the already-loaded blocksByName.
      // Competition ranking, so ties share a rank (two blocks at 80 are
      // both 1st of n). Falls back to the bare number when the blocks
      // fetch failed (no peers resolvable).
      inspectorPriority() {
        const slot = this.inspector.slot;
        if (!slot) return "";
        const record = this.inspectorBlock();
        const priority = slot.kind === "ghost" ? (record?.spec.priority ?? slot.priority) : slot.priority;
        const channelId = record?.spec.channel_id ?? slot.channelId;
        const peers = Object.values(this.blocksByName).filter(
          (b) => b.enabled && b.spec.channel_id === channelId,
        );
        if (peers.length === 0) return String(priority);
        const higher = peers.filter((b) => (b.spec.priority ?? 0) > priority).length;
        return `${priority} · ${ordinal(higher + 1)} of ${peers.length}`;
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
