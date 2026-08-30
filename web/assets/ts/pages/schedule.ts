// Schedule page ("/schedule/"): preview a dry-run plan (POST /api/v1/generate)
// per channel, then push it to Tunarr (POST /api/v1/apply) behind an explicit
// confirmation dialog -- the web equivalent of the CLI's `--yes` gate
// (cmd/generate.go's checkApplyGate). Bundled separately via "page_js", same
// pattern as dashboard.ts/blocks.ts.
//
// Contract notes that shape the code below:
//
//   1. `/generate` always dry-runs (PlanResult.applied is always false) and
//      `/apply` always mutates (applied: true on success) -- both take the
//      identical GenerateRequest body (days, optional channel_id). This page
//      never invents a third request shape: buildRequestBody() is shared by
//      both preview() and confirmApply().
//   2. `/apply` does NOT replay the client's previewed plan -- it independently
//      re-runs Runner.Run server-side (internal/service/schedule.go) with the
//      same parameters. "Apply with the same body as the preview" (the
//      binding contract's wording) means the same *request*, not a cached
//      plan payload; this file follows that literally.
//   3. `channel_id`, when set, restricts which blocks get planned at all
//      (README's "Schedule Endpoints" notes) -- an empty/omitted value means
//      every channel. This page always omits `channel_id` from the wire body
//      when the control resolves to "" ("All channels"), never sending an
//      empty string, matching blocks.ts's "omit rather than send a blank
//      placeholder" convention.
//   4. ScheduledSlot.programs is the typed ScheduledProgram shape
//      (api/openapi.yaml, v0.5.1 hard swap): title/duration_ms/start_time
//      required, type/season/episode optional. `title` is contractually a
//      string; the render helper below still normalizes a blank one to an
//      em dash so the expandable list never shows an empty row.
import { apiGet, apiPath, apiSend, onReauth } from "../runtime/api.ts";
import type { ApiRequestJSON, ApiResponse } from "../runtime/api.ts";
import { channelHint as channelHintText, channelLabel, channelPlate, loadChannels } from "../runtime/channels.ts";
import type { Channel, PlateParts } from "../runtime/channels.ts";
import { toProblemView } from "../runtime/errors.ts";
import type { ProblemView } from "../runtime/errors.ts";
import { formatLocal, plural } from "../runtime/format.ts";
import { initShell } from "../runtime/shell.ts";
import type { components } from "../gen/types";

initShell();

type PlanResult = ApiResponse<"generateSchedule", 200>;
type ScheduledSlot = components["schemas"]["ScheduledSlot"];
type Warning = components["schemas"]["Warning"];
type GenerateBody = ApiRequestJSON<"generateSchedule">;

declare const Alpine: {
  data<T extends object>(name: string, factory: () => T): void;
};

// Same double-init defense as dashboard.ts/blocks.ts: Alpine.data()'s init()
// is auto-invoked, so nothing on this page also wires x-init="init()" to it.
let started = false;

// Problem rendering note: this page's binding contract calls for the
// API's own title *and* detail rendered as two distinct pieces -- the 502
// case specifically names "schedule generation failed" (the server's
// literal Problem.title, internal/api/schedule.go:writeScheduleRunnerError)
// as something the UI must show verbatim. The shared ProblemView/ui-problem
// idiom does exactly that (title line + detail line + REF), so this page
// now uses the runtime's toProblemView like every other page.

// ---- controls -> wire request -------------------------------------------

interface Controls {
  days: string; // free text, like blocks.ts's numeric fields -- parsed once at request time, never via x-model.number
  channelId: string; // "" = All channels; select value or free-text fallback value
}

/** The exact {days, channelId} a preview or apply request was built from.
 * canApply() compares this against the *live* controls (via requestSignature())
 * to decide whether the current plan still matches what's on screen -- the
 * binding contract's "changing days/channel invalidates" rule. */
interface RequestSignature {
  days: number;
  channelId: string;
}

/** Clamps free-text days input to the API's documented [1, 30] range
 * (api/openapi.yaml GenerateRequest.days minimum/maximum), defaulting to 7
 * (the schema's own default) for blank/non-finite input. This is the
 * "client-side clamp" half of the binding contract; a value that somehow
 * still slips out of range is caught by the API's own 400 (the "rely on API
 * 400" half) -- toProblem()/previewError render that like any other
 * problem+json response. */
export function clampDays(raw: string): number {
  const trimmed = raw.trim();
  // Blank input must take the schema default (7), not fall through
  // Number("") === 0 into the 1-day clamp -- the latter contradicted both
  // this function's contract and the field's own "defaults to 7" hint
  // (caught by web/tests/schedule.test.ts during the v0.5.0 refactor).
  if (trimmed === "") return 7;
  const n = Number(trimmed);
  if (!Number.isFinite(n)) return 7;
  return Math.min(30, Math.max(1, Math.round(n)));
}

/** Builds the GenerateRequest body preview() and confirmApply() both send --
 * one shape, shared, per the contract note above. channel_id is omitted
 * (never sent as "") when the scope is "All channels". */
function buildRequestBody(sig: RequestSignature): GenerateBody {
  const body: GenerateBody = { days: sig.days };
  if (sig.channelId !== "") body.channel_id = sig.channelId;
  return body;
}

/** Orders channel sections the way their plates read: by resolved channel
 * number first (a plan section headed `CH 04 · HORROR` sorting on raw
 * UUID looks arbitrary), then name, then raw id as the final tiebreak.
 * Channels the cache can't resolve (or that carry no number) sort after
 * numbered ones, by name/id -- exported for direct testing, same
 * convention as clampDays above. */
export function channelOrder(aId: string, bId: string, channels: Channel[]): number {
  const a = channels.find((c) => c.id === aId);
  const b = channels.find((c) => c.id === bId);
  const aNum = a?.number ?? Number.POSITIVE_INFINITY;
  const bNum = b?.number ?? Number.POSITIVE_INFINITY;
  if (aNum !== bNum) return aNum - bNum;
  const byName = (a?.name ?? "").localeCompare(b?.name ?? "");
  if (byName !== 0) return byName;
  return aId.localeCompare(bId);
}

// ---- program rendering ---------------------------------------------------

type ScheduledProgram = components["schemas"]["ScheduledProgram"];

/** Renders a program's title (see contract note 4 above): a blank title
 * renders as an em dash rather than an empty row, so the expandable
 * list's item count always matches programCount(). */
function programTitle(p: ScheduledProgram): string {
  return p.title.trim() !== "" ? p.title : "—";
}

interface ChannelEntry {
  id: string;
  slots: ScheduledSlot[];
}

interface ScheduleState {
  controls: Controls;

  channelsLoading: boolean;
  channelsError: string | null;
  channels: Channel[];

  previewing: boolean;
  previewError: ProblemView | null;
  plan: PlanResult | null;
  previewedRequest: RequestSignature | null;

  applying: boolean;
  applyError: ProblemView | null;
  appliedAt: string | null;

  init(): void;
  loadChannels(): Promise<void>;
  channelLabel(c: Channel): string;
  channelHint(): string;
  plate(id: string): PlateParts;

  requestSignature(): RequestSignature;
  canApply(): boolean;

  preview(): Promise<void>;
  requestApply(): void;
  cancelApply(force?: boolean): void;
  confirmApply(): Promise<void>;

  channelEntries(): ChannelEntry[];
  programCount(slot: ScheduledSlot): number;
  programTitles(slot: ScheduledSlot): string[];
  slotsLabel(entry: ChannelEntry): string;
  programsLabel(slot: ScheduledSlot): string;
  formatLocal(iso: string | undefined): string;

  warnings(): Warning[];
  warningsHeading(): string;
  warningLabel(w: Warning): string;

  applyScopeLabel(): string;
  applyConfirmBody(): string;
  appliedSummary(): string;
}

// $refs is an Alpine magic (https://alpinejs.dev/magics/refs), injected at
// runtime, not part of the object literal Alpine.data()'s factory returns --
// same ThisType erasure trick blocks.ts uses for $nextTick.
interface WithRefs {
  $refs: { confirmDialog: HTMLDialogElement };
}

document.addEventListener("alpine:init", () => {
  Alpine.data(
    "schedule",
    (): ScheduleState & ThisType<ScheduleState & WithRefs> => ({
      controls: { days: "7", channelId: "" },

      channelsLoading: true,
      channelsError: null,
      channels: [],

      previewing: false,
      previewError: null,
      plan: null,
      previewedRequest: null,

      applying: false,
      applyError: null,
      appliedAt: null,

      init() {
        if (started) return;
        started = true;
        void this.loadChannels();
        // Arming a new token re-fires the channel load; a failed preview
        // is an action, not a load, so it stays behind its own button.
        onReauth(() => {
          if (this.channelsError) void this.loadChannels();
        });
      },

      async loadChannels() {
        this.channelsLoading = true;
        this.channelsError = null;
        try {
          this.channels = await loadChannels();
        } catch (err) {
          const problem = toProblemView(err);
          this.channelsError = problem.detail ?? problem.title;
          this.channels = [];
        } finally {
          this.channelsLoading = false;
        }
      },

      channelLabel,

      plate(id) {
        return channelPlate(id, this.channels);
      },

      // Shared hint logic (runtime/channels.ts's channelHint); this page's
      // manual-entry wording additionally notes that blank still means
      // "All channels" -- the one real difference from the blocks editor's
      // channel field.
      channelHint() {
        return channelHintText(
          this.channelsLoading,
          this.channelsError,
          this.channels,
          "enter a channel ID manually, or leave blank for all channels",
        );
      },

      requestSignature() {
        return { days: clampDays(this.controls.days), channelId: this.controls.channelId.trim() };
      },

      // Apply is armed only when: nothing is in flight, a plan exists, and
      // that plan was generated for exactly the controls currently on
      // screen. Editing days/channel after a preview changes
      // requestSignature()'s result without touching previewedRequest, so
      // this flips false immediately -- Alpine re-evaluates it reactively
      // since both are read here.
      canApply() {
        if (this.previewing || this.applying) return false;
        if (!this.plan || !this.previewedRequest) return false;
        const current = this.requestSignature();
        return this.previewedRequest.days === current.days && this.previewedRequest.channelId === current.channelId;
      },

      async preview() {
        const sig = this.requestSignature();
        this.previewing = true;
        this.previewError = null;
        this.applyError = null;
        try {
          const result = await apiSend<PlanResult>(
            "POST",
            apiPath("/generate"),
            buildRequestBody(sig),
          );
          this.plan = result;
          this.previewedRequest = sig;
          this.appliedAt = null;
        } catch (err) {
          // Clearing the plan (rather than leaving a stale one on screen
          // under the error) matters here specifically: a plan generated
          // for a *different* scope sitting next to a fresh error could
          // read as still valid, even though canApply() would already
          // block it.
          this.previewError = toProblemView(err);
          this.plan = null;
          this.previewedRequest = null;
        } finally {
          this.previewing = false;
        }
      },

      requestApply() {
        if (!this.canApply()) return;
        this.applyError = null;
        this.$refs.confirmDialog.showModal();
      },

      // State-level guard, not just the view layer's :disabled/backdrop
      // checks in list.html: refuses to close the dialog while an apply is
      // in flight, same "explicit re-entrancy guard" convention blocks.ts
      // uses for pendingId (toggleEnabled/performDelete). This is the last
      // line of defense against a dismissed-but-still-mutating dialog --
      // :disabled on Cancel/close-X and the backdrop handler's `!applying`
      // check exist too, but neither is trustworthy on its own (a disabled
      // attribute is a browser-enforced UI convention, not a guarantee this
      // method itself can rely on). confirmApply() passes force:true for
      // its own two closes, both of which happen deliberately while
      // `applying` is still true (it flips false in the `finally` clause
      // that runs after).
      cancelApply(force = false) {
        if (this.applying && !force) return;
        this.$refs.confirmDialog.close();
      },

      // Sends the SAME body preview() sent (buildRequestBody over the
      // signature the shown plan was generated from), never a body built
      // from live controls -- if the operator could somehow reach this
      // method with stale controls, the applied scope must still match
      // what the confirmation dialog told them, not whatever is in the
      // fields right now.
      async confirmApply() {
        const sig = this.previewedRequest;
        if (!sig) {
          this.cancelApply();
          return;
        }
        this.applying = true;
        try {
          const result = await apiSend<PlanResult>(
            "POST",
            apiPath("/apply"),
            buildRequestBody(sig),
          );
          this.plan = result;
          this.appliedAt = new Date().toISOString();
          // Force a fresh preview before the next apply, even if the
          // operator immediately re-opens the dialog against unchanged
          // controls -- an applied plan is a past action, not a standing
          // offer to repeat it with one more click.
          this.previewedRequest = null;
          this.cancelApply(true);
        } catch (err) {
          this.applyError = toProblemView(err);
          this.cancelApply(true);
        } finally {
          this.applying = false;
        }
      },

      // Server order is per-block, not per-channel: the engine concatenates
      // each block's own occurrences (internal/scheduler/engine.go's
      // GenerateForTimeRange iterates blocks, appending each one's slots to
      // its channel's list), so a channel fed by two blocks (e.g. a morning
      // block and an evening block) arrives grouped by block, not sorted by
      // time. The binding contract calls for "slots chronological", so each
      // channel's slots are sorted here -- start_time is an RFC 3339
      // date-time (api/openapi.yaml's ScheduledSlot.start_time), and RFC
      // 3339's fixed field widths make its string form sort identically to
      // its chronological order, so a plain lexicographic compare is exact,
      // not an approximation.
      channelEntries() {
        if (!this.plan) return [];
        return Object.entries(this.plan.channels)
          .map(([id, slots]) => ({
            id,
            slots: [...slots].sort((a, b) => (a.start_time ?? "").localeCompare(b.start_time ?? "")),
          }))
          .sort((a, b) => channelOrder(a.id, b.id, this.channels));
      },

      programCount(slot) {
        return (slot.programs ?? []).length;
      },

      programTitles(slot) {
        return (slot.programs ?? []).map(programTitle);
      },

      slotsLabel(entry) {
        return plural(entry.slots.length, "slot");
      },

      programsLabel(slot) {
        return plural(this.programCount(slot), "program");
      },

      formatLocal,

      // Exact wording the binding contract requires: "Apply ALL channels"
      // vs "Apply channel <id>" -- callers concatenate "Apply " + this.
      // Falls back to the live signature if somehow called before
      // previewedRequest is set (defensive; requestApply() never opens the
      // dialog without it).
      applyScopeLabel() {
        const id = (this.previewedRequest ?? this.requestSignature()).channelId;
        return id === "" ? "ALL channels" : `channel ${id}`;
      },

      // The confirmation dialog's body sentence. Exact required substring
      // for the empty-plan case ("nothing to apply for this scope") per the
      // binding contract; otherwise a real count drawn from the shown plan,
      // never a guess.
      applyConfirmBody() {
        if (!this.plan) return "";
        const channelCount = Object.keys(this.plan.channels).length;
        if (channelCount === 0) return "There is nothing to apply for this scope.";
        const slotCount = Object.values(this.plan.channels).reduce((sum, slots) => sum + slots.length, 0);
        // "Applies", matching the dialog's own title ("Apply " +
        // applyScopeLabel()), the Confirm Apply button, and
        // appliedSummary()'s "Applied ..." result below -- see the copy
        // audit's Apply/Applied terminology decision.
        return `This applies ${plural(slotCount, "slot")} across ${plural(channelCount, "channel")} to Tunarr.`;
      },

      // Every dropped occurrence from the last preview/apply (both
      // /generate and /apply return PlanResult.warnings the same way) --
      // conflict resolution on the server (internal/scheduler/engine.go's
      // resolveConflicts) picked a higher- (or equal-, first-come-)
      // priority occurrence on the same channel instead. Previously this
      // was only visible in a server-side INFO log line.
      warnings() {
        return this.plan?.warnings ?? [];
      },

      warningsHeading() {
        return `${plural(this.warnings().length, "occurrence")} not scheduled — conflict with a higher-priority block`;
      },

      warningLabel(w) {
        const block = w.block_name ?? "A block";
        const blocker = w.blocking_block_name ?? "a higher-priority block";
        const when = this.formatLocal(w.occurrence_start);
        return `${block} lost to ${blocker} at ${when} and was not scheduled.`;
      },

      appliedSummary() {
        if (!this.plan || !this.plan.applied) return "";
        const channelCount = Object.keys(this.plan.channels).length;
        const when = this.appliedAt ? this.formatLocal(this.appliedAt) : "";
        if (channelCount === 0) {
          return when ? `Applied — nothing to apply for this scope (${when}).` : "Applied — nothing to apply for this scope.";
        }
        const slotCount = Object.values(this.plan.channels).reduce((sum, slots) => sum + slots.length, 0);
        const summary = `Applied ${plural(slotCount, "slot")} across ${plural(channelCount, "channel")}`;
        return when ? `${summary} at ${when}.` : `${summary}.`;
      },
    }),
  );
});
