// Package service hosts the schedule-generation workflow shared by the CLI
// (cmd/generate.go) and the HTTP API (internal/api/schedule.go): loading the
// active scheduling blocks, fetching available Tunarr content, running the
// scheduling engine over a time window, and -- when asked -- pushing the
// result to Tunarr and committing engine state.
//
// This package is a straight extraction of what cmd/generate.go used to do
// inline (see git history for ProcessSchedule and its helpers): the
// scheduling logic itself (block loading, content fetching, engine
// invocation, apply) is unchanged. What did change, because this code now
// also runs inside a long-lived API server rather than a one-shot CLI
// process, is presentation and lifecycle: colored fmt.Printf progress
// output became slog calls, context.Background() became the caller's ctx,
// and the CLI's per-invocation (and therefore always-empty) content cache
// became a Runner-scoped one that actually gets reused across calls.
package service

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/christopherime/schedularr/internal/cache"
	"github.com/christopherime/schedularr/internal/external/tunarr"
	"github.com/christopherime/schedularr/internal/httpclient"
	"github.com/christopherime/schedularr/internal/scheduler"
	"github.com/christopherime/schedularr/internal/store"
)

// Options configures a single Run. Days is the number of 24h days ahead of
// now to generate a schedule for; callers (the API's handlers, cmd/generate.go)
// are responsible for validating it (the API restricts it to 1..30) --
// Run does not re-validate it. ChannelID, when non-empty, restricts which
// blocks are planned at all -- see Run's doc comment for why that matters
// beyond just narrowing the returned/pushed result. Apply gates whether Run
// mutates anything: false is a pure dry-run (no UpdateSchedule calls, no
// Engine.Commit()).
type Options struct {
	Days      int
	ChannelID string
	Apply     bool
}

// Result is what Run produces: the generated (and, if Applied, pushed)
// schedule, keyed by Tunarr channel ID. Warnings lists every occurrence
// GenerateForTimeRange planned a time slot for but then had to drop
// (conflict resolution -- see scheduler.Warning's doc comment); it can be
// non-empty on both a dry run and an apply, since conflict resolution
// happens during generation either way. Nil (not just empty) when there's
// nothing to report.
type Result struct {
	Applied  bool
	Channels map[string][]scheduler.ScheduledSlot
	Warnings []scheduler.Warning
}

// ScheduleRunner is the interface api.Deps.Sched depends on, so handler
// tests can substitute a fake instead of a real Runner (which needs a live
// store and Tunarr client to construct).
type ScheduleRunner interface {
	Run(ctx context.Context, o Options) (*Result, error)
}

// tunarrCacheKey is the cache.Cache key the fetched Tunarr program list is
// stored under.
const tunarrCacheKey = "tunarr_programs.json"

// contentCacheDuration is how long a Runner caches the fetched Tunarr
// program list before refetching. There used to be a config-driven
// cache.cache_duration CUE key mirroring this value, but nothing ever
// actually read it into a Runner (the CLI's old fetch path was
// per-invocation and therefore always cache-cold regardless), so it was
// removed as dead config; a Runner now lives for the lifetime of an API
// server process and serves many Run calls, so this fixed, Runner-scoped
// duration is what actually governs cache behavior.
const contentCacheDuration = time.Hour

// Runner executes the schedule-generation workflow against a store and a
// Tunarr client held for its lifetime.
type Runner struct {
	store         *store.Store
	tunarr        *tunarr.Client
	logger        *slog.Logger
	loc           *time.Location
	cache         *cache.Cache // nil if cache.New failed; fetch then always hits Tunarr
	historyWindow time.Duration
	// now returns the current time -- defaults to time.Now (set by
	// NewRunner) and is the sole clock Run reads. Tests in this package
	// override it directly (an unexported field, same package) to pin
	// "now" to a fixed instant, e.g. one that deterministically falls
	// inside a block's on-air window instead of depending on whatever the
	// real wall clock happens to be when the test runs.
	now func() time.Time
}

var _ ScheduleRunner = (*Runner)(nil)

// NewRunner builds a Runner backed by st and tc. l and loc default to
// slog.Default() and time.Local respectively when nil, matching
// scheduler.NewEngine's own nil-handling. historyWindow is forwarded
// unchanged to every scheduler.Engine Run builds
// (scheduler.EngineOptions.HistoryWindow) -- callers pass
// config.MaintenanceHistoryRetention(cfg) here (see cmd/generate.go,
// cmd/serve.go) so the engine's in-memory dedup window and its
// Commit-time schedule_history pruning both honor the configured
// maintenance.history_retention value instead of a hardcoded 7 days. Zero
// falls back to scheduler's own default (7 days) -- see
// scheduler.NewEngineWithOptions.
func NewRunner(st *store.Store, tc *tunarr.Client, l *slog.Logger, loc *time.Location, historyWindow time.Duration) *Runner {
	if l == nil {
		l = slog.Default()
	}
	if loc == nil {
		loc = time.Local
	}

	c, err := cache.New(contentCacheDuration)
	if err != nil {
		l.Warn("failed to initialize content cache, proceeding without cache", "error", err)
		c = nil
	}

	return &Runner{store: st, tunarr: tc, logger: l, loc: loc, cache: c, historyWindow: historyWindow, now: time.Now}
}

// ActiveBlocks returns the Spec of every enabled block in the store.
// scheduler.yaml is import-only (see blockio.Bootstrap): the store is the
// engine's live source of scheduling truth, and disabled blocks stay
// defined but out of schedule generation. Moved here from
// cmd/generate.go's loadActiveBlocks so both the CLI and the API server
// share one implementation.
func ActiveBlocks(ctx context.Context, s *store.Store) ([]scheduler.Block, error) {
	records, err := s.ListBlocks(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list blocks from store: %w", err)
	}

	blocks := make([]scheduler.Block, 0, len(records))
	for _, rec := range records {
		if !rec.Enabled {
			continue
		}
		spec := rec.Spec
		// rec.Spec (the stored JSON blob) never carries the record's own
		// store ID -- copy it in here so the engine can key series
		// occurrence snapshots by it (stable across a rename, unlike
		// Name) -- see scheduler.Block.ID's doc comment.
		spec.ID = rec.ID
		blocks = append(blocks, spec)
	}
	return blocks, nil
}

// Run executes one generate-(and-maybe-apply) cycle: load the active
// blocks (restricted to o.ChannelID's blocks when set -- see below), fetch
// available Tunarr content, run the scheduling engine over [now,
// now+o.Days days), and -- only when o.Apply is set -- push the result to
// Tunarr per channel and commit the engine's pending state. A dry run
// (o.Apply == false) never calls UpdateSchedule or Commit, so it cannot
// mutate the store or Tunarr.
//
// o.ChannelID filtering happens to the *blocks*, before scheduler.Engine
// ever sees them -- not just to the result map after planning. This
// matters because Engine plans every block it's given: for each one it
// mutates in-memory pending state (series-cursor advances in
// pendingStates, "aired" rows in pendingHistory -- see
// planSeriesForConfig/recordHistory in engine.go) regardless of whether
// that block's channel ends up in the caller's requested scope. If
// filtering were applied only to Run's *return value* (as an earlier
// version of this function did), a channel-scoped Apply would still plan
// -- and Engine.Commit() would still persist -- every other channel's
// series cursors and history, even though nothing was pushed to Tunarr
// for them: series scheduling on untouched channels would silently skip
// episodes or wrongly dedup against history it never actually aired.
// Pre-filtering the blocks slice keeps planning itself scoped, so
// pendingStates/pendingHistory can only ever contain entries for blocks
// on the requested channel.
//
// A ChannelID that matches no enabled block (typo, disabled, unknown
// channel) is not an error: blocks ends up empty, Engine plans nothing,
// and Run returns Result{Applied: o.Apply, Channels: <empty map>} with
// nothing planned or committed -- the same "well-formed request, nothing
// to schedule" treatment an empty result gets for any other reason. An
// error was considered (an unrecognized channel_id could reasonably be a
// caller mistake worth surfacing loudly), but Run has no channel registry
// to validate against here -- only Tunarr does, via GetChannels, which
// Run never calls -- so rejecting it would mean guessing at "known
// channels" from a source Run doesn't otherwise consult.
func (r *Runner) Run(ctx context.Context, o Options) (*Result, error) {
	blocks, err := ActiveBlocks(ctx, r.store)
	if err != nil {
		return nil, fmt.Errorf("failed to load scheduling blocks: %w", err)
	}

	if o.ChannelID != "" {
		blocks = blocksForChannel(blocks, o.ChannelID)
	}

	programs, err := r.fetchPrograms(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch programs: %w", err)
	}

	engine := scheduler.NewEngineWithOptions(ctx, r.tunarr, blocks, r.store, scheduler.EngineOptions{
		Logger:        r.logger,
		Location:      r.loc,
		HistoryWindow: r.historyWindow,
	})

	// Truncated to the whole minute because it does double duty as
	// applyChannels' lineup anchor (offset 0 of every pushed lineup, and
	// the value written to channel.startTime): Tunarr's own channel-update
	// write path truncates startTime to the whole minute server-side
	// regardless of what's sent (tunarr.Client.setChannelStartTime's doc
	// comment), so truncating here keeps this Run's own timing math
	// (below, and in applyChannels/buildAnchoredLineup) consistent with
	// what actually gets stored.
	start := r.now().Truncate(time.Minute)
	end := start.Add(time.Duration(o.Days) * 24 * time.Hour)

	// scheduler.Engine's exported methods predate this ctx-threaded
	// service and don't take a context.Context (they use
	// context.Background() internally, e.g. in getSeriesState/Commit) --
	// out of scope for this task's extraction (see the package doc
	// comment: engine logic itself is unchanged), so ctx can't actually be
	// propagated any further than here.
	//
	// channels is already scoped to o.ChannelID by construction here: every
	// block fed to NewEngine above has ChannelID == o.ChannelID (when set),
	// and GenerateForTimeRange keys its result by each block's ChannelID,
	// so no separate "narrow the result map" step is needed anymore.
	channels, warnings, err := engine.GenerateForTimeRange(start, end, programs) //nolint:contextcheck
	if err != nil {
		return nil, fmt.Errorf("failed to generate schedule: %w", err)
	}

	if o.Apply {
		if err := r.applyChannels(ctx, start, end, channels, o.ChannelID); err != nil {
			return nil, err
		}
		if err := engine.Commit(); err != nil { //nolint:contextcheck // see GenerateForTimeRange above
			return nil, fmt.Errorf("failed to commit schedule state: %w", err)
		}
	}

	return &Result{Applied: o.Apply, Channels: channels, Warnings: warnings}, nil
}

// blocksForChannel returns only the blocks whose ChannelID matches
// channelID -- the mechanism behind Run's channel-scoping guarantee (see
// Run's doc comment). Called only when channelID is non-empty; an empty
// result (no block targets that channel) is valid and handled by Run, not
// here.
func blocksForChannel(blocks []scheduler.Block, channelID string) []scheduler.Block {
	filtered := make([]scheduler.Block, 0, len(blocks))
	for _, b := range blocks {
		if b.ChannelID == channelID {
			filtered = append(filtered, b)
		}
	}
	return filtered
}

// applyChannels pushes each channel's anchored lineup to Tunarr via
// UpdateSchedule, best-effort across channels (mirroring cmd/generate.go's
// former applySchedule): every channel is attempted even if an earlier one
// failed. If any channel failed, it returns an aggregate error and Run
// skips Engine.Commit() entirely -- pre-existing partial-failure behavior,
// unchanged by this package's extraction. Two consequences worth being
// explicit about: (1) Commit is all-or-nothing across the *whole* result,
// not per-channel -- one failing channel means series-cursor advances and
// history for every other (successfully pushed) channel in this same Run
// are discarded too, not just the failed one; and (2) a channel that *did*
// successfully receive UpdateSchedule before a later channel failed is not
// rolled back on Tunarr's side -- Run has no compensating action for that,
// so Tunarr and Schedularr's own commit state can end up disagreeing about
// that channel until the next successful Run. Neither is new: the old
// cmd/generate.go applyScheduleAndSync/applySchedule pair had the same
// shape (loop every channel, then Commit() once only if none failed).
//
// anchor and windowEnd are Run's own start/end (already truncated to the
// whole minute there): every applied channel shares the same apply
// window, and buildAnchoredLineup needs both ends to flex-pad the pushed
// lineup across the entire window, not just up to the last scheduled
// slot -- see its doc comment for why a partial-window lineup would drift
// out of phase with wall-clock once Tunarr loops it. anchor is only the
// STARTING POINT for each channel's own anchor, though -- see
// anchorForChannel for why a channel with an on-air occurrence shifts its
// own anchor earlier.
//
// scope is Run's o.ChannelID, threaded through to clearStaleChannels so a
// channel-scoped apply only ever touches (including clears) that one
// channel.
//
// Whenever at least one push landed -- a planned channel's lineup OR a
// stale channel's flex-only clear (clearing IS an apply: it pushes a
// lineup to Tunarr) -- the apply instant is recorded via SetLastApplyAt,
// sampled at write time rather than reusing the plan anchor (the anchor
// is truncated to the minute and predates the pushes by however long
// generation took). That persisted instant, not the applied_channels
// tracking set (whose rows clearStaleChannels removes again), is what GET
// /status reports as last_applied_at.
func (r *Runner) applyChannels(ctx context.Context, anchor, windowEnd time.Time, channels map[string][]scheduler.ScheduledSlot, scope string) error {
	failCount := 0
	pushed := 0
	for channelID, slots := range channels {
		channelAnchor := anchorForChannel(anchor, slots)
		lineup, err := buildAnchoredLineup(channelAnchor, windowEnd, slots)
		if err != nil {
			r.logger.Error("failed to build anchored lineup for channel",
				"channel_id", channelID,
				"error", err,
			)
			failCount++
			continue
		}
		if err := r.tunarr.UpdateSchedule(ctx, channelID, channelAnchor, lineup); err != nil {
			r.logger.Error("failed to apply schedule to channel",
				"channel_id", channelID,
				"error", err,
			)
			failCount++
			continue
		}
		// Track the push so a later apply can clear this channel if its
		// blocks all disappear (see clearStaleChannels). A tracking failure
		// is logged but never fails the apply: the push itself succeeded,
		// and the worst case of a missing row is pre-tracking behavior (the
		// channel just never gets auto-cleared).
		if err := r.store.MarkChannelApplied(ctx, channelID, anchor); err != nil {
			r.logger.Warn("failed to track applied channel", "channel_id", channelID, "error", err)
		}
		pushed++
		r.logger.Info("applied schedule to channel", "channel_id", channelID)
	}

	cleared := r.clearStaleChannels(ctx, anchor, windowEnd, channels, scope)

	// Best-effort, same contract as MarkChannelApplied above: the pushes
	// themselves succeeded, so a failure to record the instant is logged,
	// never turned into an apply failure.
	if pushed+cleared > 0 {
		if err := r.store.SetLastApplyAt(ctx, r.now().UTC()); err != nil {
			r.logger.Warn("failed to record last apply time", "error", err)
		}
	}

	if failCount > 0 {
		return fmt.Errorf("failed to apply schedule to %d channel(s)", failCount)
	}
	return nil
}

// clearStaleChannels pushes a flex-only lineup to -- and then untracks --
// every channel a previous apply pushed a lineup to (applied_channels, see
// MarkChannelApplied above) that this apply's plan no longer covers.
// Without this, deleting or disabling a channel's last block leaves the
// previous apply's lineup airing in Tunarr indefinitely: an absent channel
// simply stops being pushed to, which is not the same as being cleared
// (found live 2026-08-30: a deleted block's episodes stayed scheduled in
// Tunarr through every subsequent apply).
//
// Untracking after a successful clear is what bounds this to exactly one
// clear per channel per "all its blocks went away" transition: a channel
// the operator takes over manually in Tunarr afterwards is never clobbered
// by later applies, and a channel whose blocks come back is simply pushed
// (and re-tracked) as part of the normal plan again.
//
// When scope (Run's o.ChannelID) is set, only that channel is considered:
// a channel-scoped apply must not touch other channels' lineups in any
// direction, clearing included.
//
// Failures here are logged but never fail the apply: the planned channels'
// pushes already succeeded, and failing the Run would discard their
// Engine.Commit (series-cursor advances, history) over a cleanup that the
// still-present tracking row retries on the next apply anyway.
//
// Returns how many channels were successfully cleared -- each clear is a
// lineup pushed to Tunarr, so applyChannels counts them toward "did this
// apply push anything" when recording the last apply instant.
func (r *Runner) clearStaleChannels(ctx context.Context, anchor, windowEnd time.Time, planned map[string][]scheduler.ScheduledSlot, scope string) int {
	tracked, err := r.store.ListAppliedChannels(ctx)
	if err != nil {
		r.logger.Error("failed to list applied channels for stale-lineup clearing", "error", err)
		return 0
	}

	cleared := 0
	for _, channelID := range tracked {
		if _, ok := planned[channelID]; ok {
			continue
		}
		if scope != "" && channelID != scope {
			continue
		}
		lineup := []tunarr.LineupItem{flexItem(windowEnd.Sub(anchor))}
		if err := r.tunarr.UpdateSchedule(ctx, channelID, anchor, lineup); err != nil {
			r.logger.Error("failed to clear stale channel lineup", "channel_id", channelID, "error", err)
			continue
		}
		cleared++
		if err := r.store.UnmarkChannelApplied(ctx, channelID); err != nil {
			// The clear itself landed; the next apply will just push the
			// same flex-only lineup again until the row goes away.
			r.logger.Error("failed to untrack cleared channel", "channel_id", channelID, "error", err)
			continue
		}
		r.logger.Info("cleared stale channel lineup (no blocks schedule it anymore)", "channel_id", channelID)
	}
	return cleared
}

// anchorForChannel returns the anchor (lineup offset 0, and the value
// written to channel.startTime) to use for one channel's own lineup: the
// earliest of Run's own global anchor and every one of this channel's
// slots' StartTime. Slots normally all start at or after the global
// anchor (Run's own start) -- except for an on-air occurrence's shell,
// which scheduler.Engine.GenerateForTimeRange's phase 1 injects whenever
// something is already airing at apply time, and whose StartTime is
// therefore *before* start (see its own doc comment for why).
//
// Anchoring at that occurrence's own StartTime rather than the global
// anchor is what makes Tunarr's wall-clock playback formula (elapsed =
// (now - channel.startTime) % duration -- see buildAnchoredLineup's doc
// comment) resolve to a position partway through that occurrence's
// content instead of restarting it from the beginning the moment this
// apply's lineup takes effect: the on-air occurrence's own content is
// always a frozen, verbatim replay (see PlanBlock/planSeriesOccurrences'
// "aired" handling and resolveCommittedPrograms), specifically so its
// position within the episode keeps making sense relative to whichever
// anchor this function picks.
//
// A channel with no on-air occurrence is unaffected: every slot's
// StartTime is already >= anchor, so the loop never lowers it, and this
// returns anchor unchanged -- exactly the pre-finding-7 behavior.
//
// A slot with zero resolved Programs is skipped entirely, even if its
// StartTime is the earliest: an on-air occurrence whose committed history
// has since been pruned (CleanupScheduleHistory's retention window) or
// predates the occurrence_start column (migration 000003's legacy-epoch
// sentinel rows) resolves to no content at all -- see
// airedSeriesOccurrenceContent's doc comment (internal/scheduler/
// engine.go). Anchoring at such an occurrence's StartTime anyway would
// shift the whole channel's playback position to line up with nothing --
// dead air the operator gets no benefit from being "anchored" to, since
// there's no real content whose mid-episode position needs to keep making
// sense. Falling back toward the global anchor (or a later on-air
// occurrence's StartTime, if this channel has more than one block) still
// produces a correct lineup either way: buildAnchoredLineup pads any gap
// -- including one before a zero-content slot's own StartTime -- with
// flex, it just isn't specially anchored to it.
func anchorForChannel(anchor time.Time, slots []scheduler.ScheduledSlot) time.Time {
	for _, slot := range slots {
		if len(slot.Programs) == 0 {
			continue
		}
		if slot.StartTime.Before(anchor) {
			anchor = slot.StartTime
		}
	}
	return anchor
}

// buildAnchoredLineup converts one channel's scheduled slots into the
// flex-padded manual lineup UpdateSchedule pushes, anchored so that
// lineup offset 0 corresponds exactly to anchor (the same instant
// UpdateSchedule writes to channel.startTime) and the lineup's total
// duration spans at least [anchor, windowEnd) -- "at least" because
// GenerateForTimeRange includes any occurrence whose START falls at or
// before windowEnd even if its nominal EndTime runs past it (a block
// starting just inside the window is allowed to complete, not be cut
// off), so the last slot's own content can push the actual total a
// little past windowEnd - anchor. That's fine for this function's job:
// see the looping note below, which only requires "at least," never
// "exactly."
//
// This exists because Tunarr computes channel playback position purely
// from elapsed wall-clock time since channel.startTime, modulo the
// pushed lineup's own total duration (source-verified against tag
// v1.3.13, server/src/stream/StreamProgramCalculator.ts's
// calculateStreamDuration: `elapsed = (now - channel.startTime) %
// channel.duration`, then a binary search over the lineup's own
// cumulative offsets) -- a lineup that's just scheduled content
// concatenated back-to-back, with no representation of the gaps between
// occurrences, plays every block immediately after whatever aired before
// it, not at the block's actual cron time. There is no way to attach an
// absolute timestamp to an individual lineup item; the only lever is
// cumulative duration from offset 0, hence the "flex" (dead-air/offline)
// padding entries this function inserts for every gap: before the first
// slot, between slots, and for whatever's left of a slot's own nominal
// duration once its scheduled programs run out (the engine already logs
// -- but doesn't fill -- that remainder; see engine.go's "block has
// remaining gap after filling").
//
// Covering at least the *entire* [anchor, windowEnd) window, not just up
// to the last scheduled slot, matters independently of getting any single
// occurrence's time right: Tunarr loops a lineup once elapsed wraps past
// its total duration, so a lineup shorter than the window would start
// repeating from anchor again before the next cron re-apply (typically
// windowEnd - anchor later) replaces it -- silently drifting the whole
// channel out of phase with wall-clock between applies. The trailing
// flex entry this function appends (whenever the scheduled slots don't
// already reach windowEnd on their own) is what prevents that.
//
// slots is sorted by StartTime first: scheduler.Engine.GenerateForTimeRange
// accumulates a channel's slots in block-iteration order, not time
// order, when more than one block targets the same channel (and
// resolveConflicts, which runs after, preserves whatever order it's
// given rather than sorting) -- this function is the first point in the
// pipeline that actually needs chronological order, so it establishes it
// itself rather than relying on an upstream guarantee that doesn't
// exist. resolveConflicts does guarantee the sorted slots don't overlap,
// which is what keeps every gap computed below non-negative.
func buildAnchoredLineup(anchor, windowEnd time.Time, slots []scheduler.ScheduledSlot) ([]tunarr.LineupItem, error) {
	sorted := make([]scheduler.ScheduledSlot, len(slots))
	copy(sorted, slots)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].StartTime.Before(sorted[j].StartTime)
	})

	var lineup []tunarr.LineupItem
	cursor := anchor
	for _, slot := range sorted {
		if gap := slot.StartTime.Sub(cursor); gap > 0 {
			lineup = appendFlex(lineup, gap)
			cursor = slot.StartTime
		}
		for i := range slot.Programs {
			p := &slot.Programs[i]
			if err := httpclient.Validate(p); err != nil {
				return nil, fmt.Errorf("invalid program %q in block %q: %w", p.GetID(), slot.Block.Name, err)
			}
			lineup = append(lineup, tunarr.LineupItem{
				Type:     "content",
				ID:       p.GetID(),
				Duration: p.Duration,
			})
			cursor = cursor.Add(time.Duration(p.Duration) * time.Millisecond)
		}
		// Pad whatever's left of this slot's own nominal duration once its
		// scheduled programs run out, so the NEXT slot's gap (above) is
		// computed against this slot's intended end time, not wherever its
		// actual content happened to stop.
		if gap := slot.EndTime.Sub(cursor); gap > 0 {
			lineup = appendFlex(lineup, gap)
			cursor = slot.EndTime
		}
	}
	if gap := windowEnd.Sub(cursor); gap > 0 {
		lineup = appendFlex(lineup, gap)
	}

	return lineup, nil
}

// flexItem builds a "flex" (dead-air/offline) lineup entry of duration d.
// d must be strictly positive -- Tunarr's FlexProgramSchema requires it
// (source-verified, see tunarr.LineupItem's doc comment); every call site
// (via appendFlex) already guards for gap > 0 before calling this.
func flexItem(d time.Duration) tunarr.LineupItem {
	return tunarr.LineupItem{Type: "flex", Duration: float64(d.Milliseconds())}
}

// appendFlex appends a flex entry of duration d to lineup, merging it
// into the trailing entry instead of adding a new one whenever that
// trailing entry is ALSO flex, rather than leaving two separate,
// adjacent flex items. This can happen even under normal operation --
// e.g. a slot with zero resolved Programs (see anchorForChannel's doc
// comment for when: a pruned or legacy on-air occurrence) sits right
// before a genuine schedule gap, and each independently contributes its
// own flex entry via the two separate call sites above. Two adjacent
// flex entries aren't wrong (Tunarr plays them back to back, functionally
// identical to one longer one), but merging keeps the pushed lineup
// minimal and its item count predictable.
func appendFlex(lineup []tunarr.LineupItem, d time.Duration) []tunarr.LineupItem {
	if n := len(lineup); n > 0 && lineup[n-1].Type == "flex" {
		lineup[n-1].Duration += float64(d.Milliseconds())
		return lineup
	}
	return append(lineup, flexItem(d))
}

// fetchPrograms returns the available Tunarr content to schedule against:
// content pulled from Tunarr's libraries (cached for contentCacheDuration),
// falling back to an unscoped SearchPrograms() when no library content is
// available. Mirrors cmd/generate.go's former fetchAllContent /
// fetchTunarrContent pipeline exactly; see the package doc comment for what
// changed in the port (logging, context, cache lifetime).
func (r *Runner) fetchPrograms(ctx context.Context) ([]tunarr.Program, error) {
	programs := r.fetchTunarrContent(ctx)

	if len(programs) == 0 {
		r.logger.Warn("no content available from libraries, falling back to SearchPrograms")
		var err error
		programs, err = r.fetchAllProgramsViaSearch(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch programs: %w", err)
		}
	}

	return programs, nil
}

func (r *Runner) fetchTunarrContent(ctx context.Context) []tunarr.Program {
	if programs := r.tryLoadFromCache(); programs != nil {
		return programs
	}

	allPrograms := r.fetchLibraryPrograms(ctx)
	r.saveToCache(allPrograms)

	return allPrograms
}

func (r *Runner) tryLoadFromCache() []tunarr.Program {
	if r.cache == nil {
		return nil
	}
	data, found := r.cache.Get(tunarrCacheKey)
	if !found {
		return nil
	}
	programs, ok := data.([]tunarr.Program)
	if !ok {
		return nil
	}
	r.logger.Debug("loaded tunarr programs from cache")
	return programs
}

func (r *Runner) fetchLibraryPrograms(ctx context.Context) []tunarr.Program {
	sources, err := r.tunarr.GetMediaSources(ctx)
	if err != nil {
		r.logger.Debug("could not fetch media sources", "error", err)
		return nil
	}
	r.logger.Debug("found media source(s)", "count", len(sources))

	var allLibraries []tunarr.Library
	for _, source := range sources {
		libraries, err := r.tunarr.GetLibraries(ctx, source.ID)
		if err != nil {
			r.logger.Debug("could not fetch libraries", "media_source", source.Name, "error", err)
			continue
		}
		allLibraries = append(allLibraries, libraries...)
	}
	r.logger.Debug("found librar(y/ies)", "count", len(allLibraries))

	var allPrograms []tunarr.Program
	for _, lib := range allLibraries {
		allPrograms = append(allPrograms, r.fetchSingleLibrary(ctx, lib)...)
	}
	return allPrograms
}

func (r *Runner) fetchSingleLibrary(ctx context.Context, lib tunarr.Library) []tunarr.Program {
	var allPrograms []tunarr.Program
	droppedTotal := 0
	page := 1
	const limit = 100

	for {
		req := tunarr.ProgramSearchRequest{
			Query:     &tunarr.ProgramSearchQuery{}, // API requires query object
			LibraryID: lib.ID,
			Page:      page,
			Limit:     limit,
		}

		resp, err := r.tunarr.SearchPrograms(ctx, req)
		if err != nil {
			r.logger.Debug("could not fetch programs from library", "library", lib.Name, "error", err)
			return nil
		}

		allPrograms = append(allPrograms, resp.Results...)
		droppedTotal += resp.DroppedCount

		// Stop once every page has been fetched. resp.TotalPages is the
		// live envelope's authoritative page count (POST
		// /api/programs/search returns {results, page, totalPages,
		// totalHits, facetDistribution} -- there is no "total"/"limit"
		// key; see tunarr.ProgramSearchResponse). page == resp.TotalPages
		// means the page just fetched was the last one. This used to also
		// break on len(resp.Results) == 0 as a defensive fallback -- but
		// resp.Results is now the *validated* count (see
		// tunarr.Client.filterValidPrograms), and a page consisting
		// entirely of entries that fail validation (rare, but a whole
		// page can share one type -- live-verified: one page was 100%
		// season-type entries) would zero that out and truncate the fetch
		// early even though more valid pages remained, reintroducing a
		// shape of the truncation bug the TotalPages fix itself closed.
		// TotalPages alone is authoritative and already handles the
		// legitimate empty-results case (page 1 >= TotalPages 0), so the
		// fallback was both redundant there and actively wrong here.
		if page >= resp.TotalPages {
			break
		}
		page++
	}

	r.logger.Debug("fetched library programs", "library", lib.Name, "count", len(allPrograms))
	if droppedTotal > 0 {
		r.logger.Warn("dropped invalid programs while fetching library",
			"library", lib.Name, "valid_count", len(allPrograms), "dropped_count", droppedTotal)
	}
	return r.hydrateShowsAndSeasons(ctx, allPrograms)
}

func (r *Runner) fetchAllProgramsViaSearch(ctx context.Context) ([]tunarr.Program, error) {
	var allPrograms []tunarr.Program
	droppedTotal := 0
	page := 1
	const limit = 100

	for {
		req := tunarr.ProgramSearchRequest{
			Query: &tunarr.ProgramSearchQuery{}, // API requires query object
			Page:  page,
			Limit: limit,
		}

		resp, err := r.tunarr.SearchPrograms(ctx, req)
		if err != nil {
			return nil, err
		}

		allPrograms = append(allPrograms, resp.Results...)
		droppedTotal += resp.DroppedCount

		// See the matching comment in fetchSingleLibrary above -- both for
		// why resp.Total never existed on a live response (the original
		// truncation bug) and for why the termination check is
		// TotalPages-only now, not also len(resp.Results) == 0.
		if page >= resp.TotalPages {
			break
		}
		page++
	}

	if droppedTotal > 0 {
		r.logger.Warn("dropped invalid programs while fetching via search",
			"valid_count", len(allPrograms), "dropped_count", droppedTotal)
	}
	return r.hydrateShowsAndSeasons(ctx, allPrograms), nil
}

// seasonCacheKeyPrefix namespaces cached season-index resolutions
// (tunarr.Program.SeasonID -> its 1-based season number) inside Runner's
// existing 1h content cache (r.cache), distinct from tunarrCacheKey's
// program-list entry. See hydrateSeasonNumbers.
const seasonCacheKeyPrefix = "tunarr_season_index:"

// hydrateShowsAndSeasons is the production join step for a live Tunarr
// instance's search results, called once by each of fetchSingleLibrary and
// fetchAllProgramsViaSearch on their own fully accumulated []Program --
// after every page for that call has already been fetched.
//
// Live-verified against a real Tunarr 1.3.13 instance this session
// (transcript in this task's report): a real /api/programs/search result
// does NOT nest an episode's show data (tunarr.Program.Show, hydrated
// defensively by tunarr.Client's hydrateEpisodeShowFields, is always nil
// against live data). Instead:
//   - An episode carries only ShowID/SeasonID foreign keys (flat "showId"/
//     "seasonId" UUIDs) -- no ShowTitle, no Rating, no SeasonNumber of its
//     own at all.
//   - Separate Type == "show" Program entries are interleaved in the SAME
//     paginated result stream as Type == "episode" entries (live-verified:
//     one 100-item page held 88 episodes and 12 shows together), related
//     only by ShowID -- and a show's entry is NOT reliably co-located on
//     the same page as its own episodes (live-verified: of 88 episodes on
//     one page, only 8 had their show's entry on that same page). That is
//     exactly why this can only run once per caller's fully accumulated
//     result set, not per-page inside the client: a per-page choke point
//     would miss the show entry for most episodes.
//   - Type == "season" entries ARE also interleaved in the same stream --
//     live-verified this round (a correction to an earlier version of
//     this comment, which claimed otherwise): a season entry carries its
//     own "index" (1-based season number) directly, and a 100-item page
//     was observed as 100% season entries during a library scan. Unlike
//     shows, though, a season is not guaranteed to be interleaved in
//     every fetch -- hydrateSeasonNumbers tries this local join first
//     (free, no HTTP call) and falls back to resolving whichever
//     SeasonIDs it didn't cover individually via
//     GET /api/programming/seasons/{id}.
func (r *Runner) hydrateShowsAndSeasons(ctx context.Context, programs []tunarr.Program) []tunarr.Program {
	r.hydrateShowTitleAndRating(programs)
	r.hydrateSeasonNumbers(ctx, programs)
	return programs
}

// hydrateShowTitleAndRating fills each Type == "episode" program's
// ShowTitle/Rating -- only when currently empty; never overrides a flat
// value a fixture/test double already set directly -- by joining its
// ShowID against the Type == "show" Program entries also present in
// programs (see hydrateShowsAndSeasons's doc comment for why both must
// come from the same, fully accumulated slice). No HTTP calls: the show
// data needed is already present in the result set this was fetched from.
func (r *Runner) hydrateShowTitleAndRating(programs []tunarr.Program) {
	shows := make(map[string]tunarr.Program)
	for _, p := range programs {
		if p.Type != "show" {
			continue
		}
		if id := p.GetID(); id != "" {
			shows[id] = p
		}
	}
	if len(shows) == 0 {
		return
	}

	for i := range programs {
		p := &programs[i]
		if p.Type != "episode" || p.ShowID == "" {
			continue
		}
		show, ok := shows[p.ShowID]
		if !ok {
			continue
		}
		if p.ShowTitle == "" {
			p.ShowTitle = show.Title
		}
		if p.Rating == "" {
			p.Rating = show.Rating
		}
	}
}

// hydrateSeasonNumbers fills each Type == "episode" program's
// SeasonNumber. Unlike the show join (hydrateShowTitleAndRating), a
// season's own entry is NOT guaranteed to be interleaved in every fetch's
// result stream the way a show's is -- but when it IS present (live
// -verified this session: a Type == "season" entry carries its own
// "index" directly, same key GET /api/programming/seasons/{id} uses; a
// 100-item page was observed as 100% season entries during a library
// scan), resolving it from the already-accumulated slice costs nothing
// and needs no HTTP call at all. So this tries that local join FIRST --
// building a SeasonID -> index map from every Type == "season" entry
// already in programs -- and only falls back to resolveSeasonNumber
// (GET /api/programming/seasons/{id} per distinct SeasonID, cached) for
// whichever SeasonIDs the local join didn't cover. Each distinct SeasonID
// that does need the network fallback is still resolved at most once per
// cache window either way (see resolveSeasonNumber). A season that fails
// to resolve by either path (network error, deleted season, unexpected
// zero index) is logged and skipped: that episode's SeasonNumber simply
// stays 0 (its existing "unknown" value, matching the field's omitempty
// contract) rather than failing the whole fetch over one bad season
// lookup.
func (r *Runner) hydrateSeasonNumbers(ctx context.Context, programs []tunarr.Program) {
	needed := make(map[string][]int) // SeasonID -> indexes into programs still needing it resolved
	for i := range programs {
		p := &programs[i]
		if p.Type != "episode" || p.SeasonID == "" || p.SeasonNumber != 0 {
			continue
		}
		needed[p.SeasonID] = append(needed[p.SeasonID], i)
	}
	if len(needed) == 0 {
		return
	}

	// Local join first: any Type == "season" entry already in this same
	// accumulated slice resolves its SeasonID for free, no HTTP call.
	for i := range programs {
		p := &programs[i]
		if p.Type != "season" || p.Index <= 0 {
			continue
		}
		id := p.GetID()
		indexes, ok := needed[id]
		if !ok {
			continue
		}
		for _, idx := range indexes {
			programs[idx].SeasonNumber = p.Index
		}
		delete(needed, id)
	}
	if len(needed) == 0 {
		return
	}

	// Fallback: whichever SeasonIDs the local join didn't cover.
	for seasonID, indexes := range needed {
		number, ok := r.resolveSeasonNumber(ctx, seasonID)
		if !ok {
			continue
		}
		for _, i := range indexes {
			programs[i].SeasonNumber = number
		}
	}
}

// resolveSeasonNumber returns seasonID's 1-based season number (Tunarr's
// "index" field -- see tunarr.Season.SeasonNumber's doc comment), serving
// from Runner's cache when possible and falling back to
// tunarr.Client.GetSeason on a miss. The second return is false if
// resolution failed outright (the request errored) or came back with a
// non-positive index; callers must treat that as "leave unresolved," not
// as season 0.
func (r *Runner) resolveSeasonNumber(ctx context.Context, seasonID string) (int, bool) {
	cacheKey := seasonCacheKeyPrefix + seasonID

	if r.cache != nil {
		if cached, found := r.cache.Get(cacheKey); found {
			if number, ok := cached.(int); ok {
				return number, true
			}
		}
	}

	season, err := r.tunarr.GetSeason(ctx, seasonID)
	if err != nil {
		r.logger.Debug("could not resolve season number", "season_id", seasonID, "error", err)
		return 0, false
	}
	if season.SeasonNumber <= 0 {
		r.logger.Debug("season resolved with no positive index", "season_id", seasonID, "index", season.SeasonNumber)
		return 0, false
	}

	if r.cache != nil {
		if err := r.cache.Set(cacheKey, season.SeasonNumber); err != nil {
			r.logger.Warn("failed to cache resolved season number", "season_id", seasonID, "error", err)
		}
	}

	return season.SeasonNumber, true
}

func (r *Runner) saveToCache(programs []tunarr.Program) {
	if r.cache == nil {
		return
	}
	if err := r.cache.Set(tunarrCacheKey, programs); err != nil {
		r.logger.Warn("failed to write tunarr programs to cache", "error", err)
	}
}
