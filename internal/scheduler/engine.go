// Package scheduler provides the core scheduling engine for Schedularr.
package scheduler

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/fnv"
	"log/slog"
	"math/rand"
	"sort"
	"time"

	"github.com/christopherime/schedularr/internal/external/tunarr"
	"github.com/christopherime/schedularr/internal/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/robfig/cron/v3"
)

// Engine is the scheduling engine that generates programming schedules.
type Engine struct {
	client         *tunarr.Client
	blocks         []Block
	parser         cron.Parser
	location       *time.Location // Location for cron parsing
	history        *ScheduleHistory
	store          StateStore
	pendingStates  map[string]*SeriesState
	pendingHistory []ScheduleHistoryEntry
	// pendingSnapshots and pendingReplacements are the idempotent-apply
	// bookkeeping for series blocks -- see PlanBlock's doc comment.
	// pendingSnapshots holds a series occurrence's cursor snapshot the
	// first (and only) time it's ever planned; pendingReplacements holds a
	// not-yet-aired occurrence's re-derived assignment, which must replace
	// (not append to) whatever schedule_history rows it already has.
	// Both are committed by Commit() alongside pendingStates/pendingHistory
	// and reset there too.
	pendingSnapshots    []occurrenceSnapshotRecord
	pendingReplacements []occurrenceReplacement
	// lastPlanSeq backs nextPlanSeq -- see its doc comment.
	lastPlanSeq int64
	logger      *slog.Logger
}

// nextPlanSeq allocates the next plan-provenance sequence
// (OccurrenceSnapshot.PlanSeq / SeriesState.CursorPlanSeq): the current
// wall clock in nanoseconds, bumped past the previous allocation if it
// isn't already ahead of it, so sequences are strictly increasing in
// plan order within an engine. Across processes (the serve loop vs. a
// one-shot CLI apply) plain wall-clock ordering is what relates them,
// which is ample at real apply cadences. Wall-clock nanos rather than a
// persisted counter so the sequence can never move backward when rows
// that carried the highest values are deleted (invalidation, retention
// GC). lastPlanSeq is seeded at engine construction from the store's own
// high-water mark (StateStore.MaxPlanSeq, across snapshot plan_seq and
// live cursor_plan_seq), so new allocations outrank every stored
// sequence even after a backward wall-clock step or an import that
// carried a clock-ahead cursor_plan_seq -- without the seed, either
// would wedge the provenance guard (every replay <= live provenance,
// silently dropped) until the clock caught up. Engines are single-run,
// not concurrent, so no locking.
func (e *Engine) nextPlanSeq() int64 {
	seq := time.Now().UnixNano()
	if seq <= e.lastPlanSeq {
		seq = e.lastPlanSeq + 1
	}
	e.lastPlanSeq = seq
	return seq
}

// occurrenceSnapshotRecord is one pending SaveOccurrenceSnapshot call,
// queued during planning and flushed by Commit(). Keyed by blockID, not
// blockName -- see StateStore.GetOccurrenceSnapshot's doc comment. The
// snapshot carries both the occurrence's pre-plan seed and its plan-time
// post-state -- see OccurrenceSnapshot's doc comment.
type occurrenceSnapshotRecord struct {
	blockID         string
	occurrenceStart time.Time
	snapshot        OccurrenceSnapshot
}

// occurrenceReplacement is one pending ReplaceOccurrenceHistory call,
// queued during planning and flushed by Commit().
type occurrenceReplacement struct {
	blockName       string
	occurrenceStart time.Time
	entries         []ScheduleHistoryEntry
}

// ScheduledSlot represents a scheduled time slot with its block and priority
type ScheduledSlot struct {
	StartTime time.Time
	EndTime   time.Time
	Block     Block
	Programs  []tunarr.Program
}

// Warning describes one occurrence GenerateForTimeRange planned a time
// slot for but then had to drop, because a higher- (or equal-, first-come)
// priority occurrence on the same channel overlapped it (see
// resolveConflicts). Previously this was only visible in a server-side
// INFO log line ("scheduling conflict - block blocked by higher
// priority") -- API callers (POST /generate, POST /apply) had no way to
// know an occurrence they might have expected never actually got
// scheduled.
type Warning struct {
	BlockName         string    // the block whose occurrence was dropped
	OccurrenceStart   time.Time // that occurrence's cron-computed start time
	BlockingBlockName string    // the block whose occurrence it lost to
}

// occurrenceKey identifies one specific occurrence of a block -- the unit
// GenerateForTimeRange's conflict resolution (phase 2) and content
// planning (phase 3) both need to agree on, and the same identity
// Engine.PlanBlock's idempotence check persists against
// (scheduler_history's block_name/occurrence_start columns). StartUnixNano
// rather than a raw time.Time so map-key equality can't be tripped up by
// monotonic-reading differences between two time.Time values that
// represent the same instant.
type occurrenceKey struct {
	blockName     string
	startUnixNano int64
}

func keyFor(slot ScheduledSlot) occurrenceKey {
	return occurrenceKey{blockName: slot.Block.Name, startUnixNano: slot.StartTime.UnixNano()}
}

// occurrenceRand returns a *rand.Rand deterministically seeded from
// (blockID, occurrenceStart), for filler/fallback content shuffling
// within a series occurrence's planning. Re-deriving the same
// not-yet-aired occurrence on a later apply (see
// Engine.planSeriesOccurrences) must pick the SAME filler/fallback
// content again given the same inputs, not reshuffle randomly each time
// -- otherwise a filler-enabled series block would never be
// apply-idempotent even though its episode selection is. blockID (not
// Name) so a rename doesn't change the seed for an occurrence that's
// otherwise identical -- consistent with keying
// series_occurrence_snapshots by block ID (see StateStore.
// GetOccurrenceSnapshot's doc comment).
func occurrenceRand(blockID string, occurrenceStart time.Time) *rand.Rand {
	h := fnv.New64a()
	_, _ = h.Write([]byte(blockID))
	_, _ = h.Write([]byte{0}) // separator, so e.g. "ab"+"" and "a"+"b" can't collide
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], uint64(occurrenceStart.UnixNano()))
	_, _ = h.Write(buf[:])
	// #nosec G404 G115 -- deterministic-by-design PRNG for reproducible
	// content shuffling across re-applies of the same occurrence, not a
	// security-sensitive value; the uint64->int64 conversion (G115) only
	// feeds rand.NewSource's seed, where any bit pattern -- wrapped
	// negative included -- is an equally valid seed.
	return rand.New(rand.NewSource(int64(h.Sum64())))
}

// shuffleWith shuffles using rng if non-nil, else the global math/rand
// source -- see occurrenceRand's doc comment for when each applies.
func shuffleWith(rng *rand.Rand, n int, swap func(i, j int)) {
	if rng != nil {
		rng.Shuffle(n, swap)
		return
	}
	// #nosec G404 - content shuffle for programming variety, not a security-sensitive value
	rand.Shuffle(n, swap)
}

// EngineOptions contains optional configuration for the scheduling engine.
type EngineOptions struct {
	// HistoryWindow bounds both the in-memory dedup check (filterByHistory)
	// and, via Commit's CleanupScheduleHistory call, how much of the
	// persisted schedule_history table survives each apply. service.Runner
	// sets this from the maintenance.history_retention config key so the
	// two stay coherent: GET /history?days=N (api/openapi.yaml, 1..90) can
	// only ever return data as far back as this window allows -- a wider
	// history_retention is required to actually query a wider days range.
	// Zero uses defaultHistoryWindow (7 days).
	HistoryWindow time.Duration
	Logger        *slog.Logger
	Location      *time.Location
}

// defaultHistoryWindow is the schedule-history retention/dedup window used
// when neither NewEngine's caller nor EngineOptions.HistoryWindow specify
// one. It matches cmd/schema/config.cue's maintenance.history_retention
// CUE default (168h / 7 days) -- see EngineOptions's doc comment.
const defaultHistoryWindow = 7 * 24 * time.Hour

// NewEngine creates a new scheduling engine with the given Tunarr client and
// scheduling blocks, using the default 7-day history window. Callers that
// need a configured window (e.g. service.NewRunner, which threads through
// maintenance.history_retention) should use NewEngineWithOptions instead,
// which also lets them thread a real context through the construction-time
// plan-sequence floor read; this convenience constructor uses
// context.Background() for it.
func NewEngine(client *tunarr.Client, blocks []Block, store StateStore, logger *slog.Logger, loc *time.Location) *Engine {
	return NewEngineWithOptions(context.Background(), client, blocks, store, EngineOptions{Logger: logger, Location: loc})
}

// NewEngineWithOptions creates a new scheduling engine with optional
// configuration. Any zero-valued field in opts falls back to the same
// default NewEngine uses: opts.Logger -> slog.Default(), opts.Location ->
// time.Local, opts.HistoryWindow -> defaultHistoryWindow (7 days). ctx
// bounds the construction-time plan-sequence floor read (MaxPlanSeq --
// see nextPlanSeq's doc comment); it is not retained.
func NewEngineWithOptions(ctx context.Context, client *tunarr.Client, blocks []Block, store StateStore, opts EngineOptions) *Engine {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	loc := opts.Location
	if loc == nil {
		loc = time.Local
	}
	historyWindow := opts.HistoryWindow
	if historyWindow == 0 {
		historyWindow = defaultHistoryWindow
	}
	e := &Engine{
		client:         client,
		blocks:         blocks,
		parser:         cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor),
		location:       loc,
		history:        NewScheduleHistory(historyWindow),
		store:          store,
		pendingStates:  make(map[string]*SeriesState),
		pendingHistory: nil,
		logger:         logger,
	}
	if store != nil {
		// Seed the plan-sequence floor from the store's high-water mark
		// (see nextPlanSeq's doc comment). Failure is survivable -- the
		// wall clock alone is still correct on any host whose clock never
		// stepped backward -- so log and continue rather than fail
		// engine construction.
		if maxSeq, err := store.MaxPlanSeq(ctx); err != nil {
			logger.Warn("failed to seed plan-sequence floor from store; falling back to wall clock alone", "error", err)
		} else {
			e.lastPlanSeq = maxSeq
		}
	}
	return e
}

// GenerateForTimeRange generates a schedule for the given window with
// priority-based conflict resolution. It returns a map of ChannelID ->
// []ScheduledSlot, plus a Warning for every occurrence that got planned a
// time slot but then lost conflict resolution (see resolveConflicts and
// Warning's doc comment).
//
// This runs in three phases, deliberately in this order:
//
//  1. Build occurrence "shells" for every block -- StartTime/EndTime/Block
//     only, no Programs -- by walking each block's cron schedule across
//     [start, end). Grouped both per channel (channelShells, for phase 2)
//     and per block in each block's own chronological order
//     (perBlockShells, for phase 3).
//  2. Resolve conflicts per channel on the shells (resolveConflicts never
//     reads .Programs, so this works identically on content-less shells),
//     recording which occurrences survive in the survived set and
//     collecting a Warning for every one that doesn't.
//  3. Plan content -- Engine.PlanBlock, the only step that can advance a
//     series cursor or write schedule_history -- one block at a time, in
//     that block's own perBlockShells order, skipping any shell conflict
//     resolution already dropped.
//
// Phases 2 and 3 used to be one step: the original single-pass version
// called PlanBlock for every occurrence *before* resolveConflicts ever
// ran, so a losing occurrence's cursor advance and history write already
// happened by the time it got thrown away -- a plan that never aired
// still permanently consumed a series' next episode. Splitting content
// planning out until after conflict resolution is what fixes that: only
// occurrences that actually make it into the returned schedule ever reach
// PlanBlock.
//
// Phase 3 iterates block-by-block (not channel-by-group, the way phase
// 1's channelShells or the final resolvedSchedule are organized) because
// that's what preserves each block's own chronological occurrence order:
// channelShells (and therefore resolvedSchedule) can interleave more than
// one block's occurrences on the same channel in whatever order the
// phase-1 outer loop happened to visit those blocks, not time order (see
// buildAnchoredLineup in internal/service/schedule.go, which has the same
// problem on the output side and the same fix -- sort by time itself
// rather than trust the map's order). A series block's cursor is only
// correct if its own occurrences are planned in the order they'll
// actually air, so phase 3 has to walk perBlockShells, not resolvedSchedule.
func (e *Engine) GenerateForTimeRange(start, end time.Time, availablePrograms []tunarr.Program) (map[string][]ScheduledSlot, []Warning, error) {
	timer := prometheus.NewTimer(metrics.ScheduleGenerationDurationSeconds)
	defer timer.ObserveDuration()

	// Anchor cron occurrence math to e.location. robfig/cron's
	// SpecSchedule.Next only converts its argument into a *schedule's own*
	// location -- set from a "CRON_TZ=..." prefix on the cron string itself
	// (see spec.go's Next) -- which block.Cron strings here never carry, so
	// every parsed SpecSchedule.Location is the zero value, time.Local.
	// Next's own doc comment for that case: "schedules without a time zone
	// specified (time.Local) are treated as local to the time provided" --
	// i.e. it matches calendar fields against whatever Location its input
	// already carries, not e.location. Without this .In(), start (bare
	// time.Now() from service.Runner.Run/cmd/generate.go) carries the
	// process's own zone -- UTC in a container without TZ set -- and every
	// occurrence below is computed against UTC wall-clock fields instead of
	// the deployment's configured log.timezone, e.g. planning "30 20 * * 6"
	// for tonight at 20:30 UTC even when 20:30 in the real configured zone
	// already passed. Next() converts its result back to the same location
	// it was given (see spec.go), so this alone is sufficient -- nextTime
	// below stays in e.location for the rest of the loop.
	start = start.In(e.location)

	perBlockShells, channelShells, err := e.generateShells(start, end)
	if err != nil {
		return nil, nil, err
	}

	// Phase 2: resolve conflicts on the shells, per channel.
	survived := make(map[occurrenceKey]bool)
	var warnings []Warning
	for _, shells := range channelShells {
		kept, dropped := e.resolveConflicts(shells)
		for _, s := range kept {
			survived[keyFor(s)] = true
		}
		warnings = append(warnings, dropped...)
	}

	// Phase 3: plan content, one block at a time in that block's own
	// chronological order, skipping anything phase 2 dropped. See
	// planSurvivingShells for how series vs. filter blocks differ here.
	resolvedSchedule := make(map[string][]ScheduledSlot)
	for i, block := range e.blocks {
		var survivingShells []ScheduledSlot
		for _, shell := range perBlockShells[i] {
			if survived[keyFor(shell)] {
				survivingShells = append(survivingShells, shell)
			}
		}
		if len(survivingShells) == 0 {
			continue
		}

		planned, err := e.planSurvivingShells(block, availablePrograms, survivingShells, start)
		if err != nil {
			metrics.ScheduleErrorsTotal.WithLabelValues("plan_block_error").Inc()
			return nil, nil, fmt.Errorf("failed to plan block %s: %w", block.Name, err)
		}
		resolvedSchedule[block.ChannelID] = append(resolvedSchedule[block.ChannelID], planned...)
	}

	return resolvedSchedule, warnings, nil
}

// generateShells is GenerateForTimeRange's phase 1: occurrence shells, no
// content yet, for every block, split both per-block (perBlockShells,
// index-aligned with e.blocks -- phase 3's iteration order) and per-channel
// (channelShells -- phase 2's conflict-resolution grouping).
//
// Each block also gets an on-air predecessor shell injected first, if one
// exists -- an occurrence whose [start, start+duration) window contains
// `start` itself, i.e. something that started airing before this apply but
// hasn't finished yet. Without it, a channel's pushed lineup (see
// service.buildAnchoredLineup/applyChannels, which anchor at the earliest
// surviving slot's StartTime) would never represent whatever Tunarr is
// currently playing at all -- every apply would anchor at `start` and cut
// the on-air occurrence off mid-episode. This occurrence is never
// re-planned once it has a snapshot (see planSeriesOccurrences/PlanBlock's
// "aired" handling): its content comes verbatim from committed history.
func (e *Engine) generateShells(start, end time.Time) (perBlockShells [][]ScheduledSlot, channelShells map[string][]ScheduledSlot, err error) {
	perBlockShells = make([][]ScheduledSlot, len(e.blocks))
	channelShells = make(map[string][]ScheduledSlot)

	for i, block := range e.blocks {
		metrics.SchedulesGeneratedTotal.WithLabelValues(block.ChannelID, block.Name).Inc()
		scheduleObj, parseErr := e.parser.Parse(block.Cron)
		if parseErr != nil {
			metrics.ScheduleErrorsTotal.WithLabelValues("cron_parse_error").Inc()
			return nil, nil, fmt.Errorf("invalid cron '%s' for block %s: %w", block.Cron, block.Name, parseErr)
		}

		duration := time.Duration(block.Duration) * time.Minute
		if onAirStart, ok := onAirOccurrenceStart(scheduleObj, duration, start); ok {
			shell := ScheduledSlot{StartTime: onAirStart, EndTime: onAirStart.Add(duration), Block: block}
			perBlockShells[i] = append(perBlockShells[i], shell)
			channelShells[block.ChannelID] = append(channelShells[block.ChannelID], shell)
		}

		nextTime := scheduleObj.Next(start.Add(-1 * time.Second))
		for !nextTime.After(end) {
			shell := ScheduledSlot{
				StartTime: nextTime,
				EndTime:   nextTime.Add(duration),
				Block:     block,
			}
			perBlockShells[i] = append(perBlockShells[i], shell)
			channelShells[block.ChannelID] = append(channelShells[block.ChannelID], shell)

			nextTime = scheduleObj.Next(nextTime)
		}
	}

	return perBlockShells, channelShells, nil
}

// planSurvivingShells plans content for one block's surviving (post-
// conflict-resolution) shells, returned in the same order as
// survivingShells. Series blocks are planned as a whole chain
// (planSeriesOccurrences) so a not-yet-aired occurrence's baseline always
// reflects an earlier occurrence's ACTUAL (possibly re-derived) end state,
// not a stale snapshot of its own -- see planSeriesOccurrences' doc
// comment. Filter blocks have no such chain to maintain, so they keep the
// simpler per-occurrence PlanBlock call. Errors are returned unwrapped;
// GenerateForTimeRange (the only caller) adds the "failed to plan block"
// context uniformly for both branches.
func (e *Engine) planSurvivingShells(block Block, availablePrograms []tunarr.Program, survivingShells []ScheduledSlot, start time.Time) ([]ScheduledSlot, error) {
	if block.Type == BlockTypeSeries && e.store != nil {
		return e.planSeriesOccurrences(block, availablePrograms, survivingShells, start)
	}

	planned := make([]ScheduledSlot, 0, len(survivingShells))
	for _, shell := range survivingShells {
		programs, err := e.PlanBlock(block, availablePrograms, shell.StartTime, start)
		if err != nil {
			return nil, err
		}
		shell.Programs = programs
		planned = append(planned, shell)
	}
	return planned, nil
}

// onAirOccurrenceStart returns the start time of the occurrence of a
// schedule (with the given duration) that is on air at now -- its [start,
// start+duration) window contains now -- if any. Used by
// GenerateForTimeRange's phase 1 to inject that occurrence's shell even
// though its start is before the window: see the call site's doc comment
// for why.
//
// robfig/cron only offers Next (no "previous occurrence" query), so this
// finds the on-air candidate by asking for the first occurrence strictly
// after (now - duration - 1s): if one exists, starts before now, and its
// own [start, start+duration) window still contains now, it's on air.
// candidate.Before(now) is strict (not !After) so an occurrence starting
// exactly at now -- which the caller's own normal sweep already covers --
// is never double-counted here.
func onAirOccurrenceStart(scheduleObj cron.Schedule, duration time.Duration, now time.Time) (time.Time, bool) {
	if duration <= 0 {
		return time.Time{}, false
	}
	candidate := scheduleObj.Next(now.Add(-duration - time.Second))
	if !candidate.Before(now) {
		return time.Time{}, false
	}
	if !candidate.Add(duration).After(now) {
		return time.Time{}, false
	}
	return candidate, true
}

// Commit persists all pending state changes to the store: series cursor
// advances and schedule history from occurrences planned for the first
// time this Run (pendingStates/pendingHistory -- the only path that ever
// really advances a series cursor, see PlanBlock's doc comment), plus
// this Run's series-occurrence idempotence bookkeeping
// (pendingSnapshots/pendingReplacements).
func (e *Engine) Commit() error {
	ctx := context.Background()
	for _, state := range e.pendingStates {
		if err := e.store.UpdateSeriesState(ctx, state); err != nil {
			return fmt.Errorf("failed to update state for %s: %w", state.ShowTitle, err)
		}
	}
	if len(e.pendingHistory) > 0 {
		if err := e.store.RecordScheduleHistory(ctx, e.pendingHistory); err != nil {
			return fmt.Errorf("failed to record schedule history: %w", err)
		}
	}
	for _, snap := range e.pendingSnapshots {
		if err := e.store.SaveOccurrenceSnapshot(ctx, snap.blockID, snap.occurrenceStart, snap.snapshot); err != nil {
			return fmt.Errorf("failed to save occurrence snapshot for block %q: %w", snap.blockID, err)
		}
	}
	for _, rep := range e.pendingReplacements {
		if err := e.store.ReplaceOccurrenceHistory(ctx, rep.blockName, rep.occurrenceStart, rep.entries); err != nil {
			return fmt.Errorf("failed to replace occurrence history for block %q: %w", rep.blockName, err)
		}
	}
	if _, err := e.store.CleanupScheduleHistory(ctx, e.history.Window()); err != nil {
		return fmt.Errorf("failed to cleanup schedule history: %w", err)
	}
	if _, err := e.store.CleanupOccurrenceSnapshots(ctx, e.history.Window()); err != nil {
		return fmt.Errorf("failed to cleanup occurrence snapshots: %w", err)
	}
	// Clear pending state after commit
	e.pendingStates = make(map[string]*SeriesState)
	e.pendingHistory = nil
	e.pendingSnapshots = nil
	e.pendingReplacements = nil
	return nil
}

// resolveConflicts resolves overlapping slots by priority, returning both
// the surviving slots and a Warning for every one dropped -- API callers
// (POST /generate, POST /apply) get this back in the response, not just a
// server-side log line, so a block's occurrence never making it into the
// schedule is visible without log access.
//
// Processes slots highest-priority-first (a stable sort, so equal
// priorities keep their original "first submitted" relative order --
// matching the old strict `>` comparison's tie-break) and greedily keeps
// a slot only if it overlaps nothing already kept. Once a slot is kept it
// can never be dropped later: everything considered afterward has equal
// or lower priority, so nothing left to process could ever outrank it.
//
// This -- not an evict-as-you-go scan over slots in their original order
// -- is deliberate: an earlier version let an incoming slot evict a
// lower-priority kept one immediately, only to itself lose to a still
// -higher-priority slot considered later in the SAME original-order pass,
// without ever undoing that eviction. A slot that never even overlapped
// the eventual survivor could end up dropped anyway, its Warning blaming
// the slot that evicted it -- which, having itself lost, was never even
// scheduled. Concretely: A and C don't overlap, but both overlap B; B
// (priority 2) evicts A (priority 1), then loses to C (priority 3). The
// old scan (processing order A, C, B) evicted A on B's behalf and never
// reinstated it once B itself lost, and blamed B -- not C, the actual
// final winner -- in A's Warning. Sorting by priority first sidesteps the
// whole "evict then lose" case: C is considered before B, so B never gets
// the chance to evict anything C would have beaten anyway.
func (e *Engine) resolveConflicts(slots []ScheduledSlot) (kept []ScheduledSlot, dropped []Warning) {
	if len(slots) == 0 {
		return slots, nil
	}

	order := make([]int, len(slots))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool {
		return slots[order[a]].Block.Priority > slots[order[b]].Block.Priority
	})

	var resolved []ScheduledSlot
	for _, i := range order {
		slot := slots[i]

		blocker, blocked := firstOverlapping(slot, resolved)
		if !blocked {
			resolved = append(resolved, slot)
			continue
		}

		metrics.ConflictsResolvedTotal.Inc()
		e.logger.Info("scheduling conflict - block blocked by higher priority",
			"blocked_block", slot.Block.Name,
			"blocked_priority", slot.Block.Priority,
			"blocking_block", blocker.Block.Name,
			"blocking_priority", blocker.Block.Priority,
			"start_time", slot.StartTime.Format("2006-01-02 15:04"))
		dropped = append(dropped, Warning{
			BlockName:         slot.Block.Name,
			OccurrenceStart:   slot.StartTime,
			BlockingBlockName: blocker.Block.Name,
		})
	}

	if len(dropped) > 0 {
		e.logger.Info("resolved scheduling conflicts using priority", "conflict_count", len(dropped))
	}

	return resolved, dropped
}

// firstOverlapping returns the first slot in resolved that overlaps slot,
// if any -- resolveConflicts' only use of overlap once slots are
// processed in priority order: a candidate is dropped the moment it
// overlaps ANYTHING already kept (which, by construction, is always
// equal-or-higher priority), so which specific kept slot is named as the
// blocker in that case is arbitrary among ties but always a real,
// surviving one -- never a slot that itself later lost.
func firstOverlapping(slot ScheduledSlot, resolved []ScheduledSlot) (ScheduledSlot, bool) {
	for _, r := range resolved {
		if slotsOverlap(slot, r) {
			return r, true
		}
	}
	return ScheduledSlot{}, false
}

// slotsOverlap returns true if two slots overlap in time
func slotsOverlap(a, b ScheduledSlot) bool {
	return a.StartTime.Before(b.EndTime) && a.EndTime.After(b.StartTime)
}

// PlanBlock generates the list of programs for one occurrence of block --
// the one whose cron-computed start time is occurrenceStart -- filling
// its duration. now is the current apply's own time reference
// (service.Runner.Run's start, i.e. effectively "now" for this whole Run)
// -- not necessarily literally time.Now() at the moment this executes,
// but a single, stable value shared by every occurrence this Run plans,
// which is what "has this occurrence aired yet" needs to mean consistently
// across a whole GenerateForTimeRange call.
//
// This is idempotent per (block.Name, occurrenceStart) for filter blocks,
// and per (block.ID, occurrenceStart) for series blocks (see why the key
// differs in StateStore.GetOccurrenceSnapshot's doc comment): the
// schedule-generation window is 24h and re-applies happen more often than
// that (the serve cron loop's default 6h interval), so the same future
// occurrence falls inside several consecutive applies' windows -- naive
// re-planning on every one of them used to advance a series block's
// episode cursor and write a new schedule_history row each time, for
// content that was never actually going to air differently, since the
// *next* apply would just overwrite it again. A Saturday-8pm block
// observed E01 on its first apply and E04 after two more, and a show's
// cursor drifted multiple episodes ahead of anything that had actually
// aired.
//
// Filter blocks have no spec-derived cursor to worry about re-deriving
// (a filter's random pick has nothing consistent to recompute toward), so
// any already-committed occurrence -- aired or not -- is simply replayed
// verbatim via GetCommittedOccurrence/resolveCommittedPrograms: freezing
// the choice once made is the only idempotent option, and is also just
// good behavior (a live Tunarr lineup that flip-flopped its random pick
// every 6h would be its own bug).
//
// Series blocks DO have a cursor (SeriesState, keyed by show title) that
// should visibly respond to a block spec edited before an occurrence
// airs -- reordering block.Series, adding/removing a series, or changing
// episodes_per_block/duration -- so freezing the final program list
// verbatim (like filter blocks) isn't good enough: it would silently
// ignore such an edit until the stale occurrence ages out of the window
// entirely. This single-occurrence entry point delegates to
// planSeriesOccurrences (a single-element shell slice) for that --
// GenerateForTimeRange's phase 3 calls planSeriesOccurrences directly
// with a whole block's surviving occurrences at once instead, which is
// what actually matters for correctness: see its own doc comment for why
// a single-occurrence view (this method, or a naive per-occurrence loop)
// isn't sufficient on its own once more than one not-yet-aired occurrence
// of the same block is in play.
//
// GenerateForTimeRange's phase 3 (the only production caller of the
// underlying planning, whether through this method for filter blocks or
// directly through planSeriesOccurrences for series ones) only plans
// occurrences that survived conflict resolution, so a conflict-dropped
// occurrence never reaches either path at all -- it can't advance a
// cursor, capture a snapshot, or get recorded either. See
// GenerateForTimeRange's own doc comment for why that ordering (resolve
// conflicts, *then* plan content) matters.
func (e *Engine) PlanBlock(block Block, availablePrograms []tunarr.Program, occurrenceStart, now time.Time) ([]tunarr.Program, error) {
	if e.store == nil {
		if block.Type == BlockTypeSeries {
			return e.planSeriesBlock(block, availablePrograms, occurrenceStart)
		}
		return e.planFilterBlock(block, availablePrograms, occurrenceStart)
	}

	if block.Type == BlockTypeSeries {
		shell := ScheduledSlot{StartTime: occurrenceStart, Block: block}
		planned, err := e.planSeriesOccurrences(block, availablePrograms, []ScheduledSlot{shell}, now)
		if err != nil {
			return nil, err
		}
		if len(planned) == 0 {
			return nil, nil
		}
		return planned[0].Programs, nil
	}

	ctx := context.Background()
	committed, ok, err := e.store.GetCommittedOccurrence(ctx, block.Name, occurrenceStart)
	if err != nil {
		return nil, fmt.Errorf("failed to check committed occurrence for block %q at %s: %w", block.Name, occurrenceStart, err)
	}
	if ok {
		return resolveCommittedPrograms(committed, availablePrograms), nil
	}
	return e.planFilterBlock(block, availablePrograms, occurrenceStart)
}

// planSeriesOccurrences plans every one of shells -- one series block's
// surviving occurrences, which the caller must already have sorted
// chronologically (GenerateForTimeRange's perBlockShells is, by
// construction; PlanBlock's single-occurrence delegation trivially
// satisfies it too) -- chaining cursor state across them within this one
// call.
//
// This exists because per-occurrence planning (each occurrence
// independently loading only its OWN stored snapshot) breaks once an
// earlier occurrence gets re-derived: a later occurrence's snapshot was
// captured assuming the pre-edit result, and nothing ever updates it, so
// it keeps re-deriving against a now-stale baseline. Concretely:
// block.Series == [A, B, C], duration fits 2 of 3 episodes per
// occurrence. Occurrence 1 originally aired [A, B] (C didn't fit);
// occurrence 2's stored snapshot was captured assuming that, i.e. "C
// hasn't aired yet." Now reorder block.Series to [C, A, B] and re-apply:
// occurrence 1 re-derives to [C, A] (C now fits first) -- but occurrence
// 2, re-planned from its OWN unchanged stored snapshot, still thinks C
// hasn't aired and airs C a SECOND time, while B -- which occurrence 1 no
// longer includes -- never gets picked up at all.
//
// The fix: maintain one running `chain` (nil until first used) across
// this whole call, in occurrence order. The first occurrence reached
// while chain is still nil seeds it via establishSeriesChain (its own
// stored snapshot; or, lacking one, its already-committed content if it
// has any; or, lacking both, a real plan against the true global cursor
// -- see establishSeriesChain's doc comment for why the distinction
// matters). Every subsequent occurrence in this call then always uses
// `chain` directly -- never its own, possibly-stale, stored snapshot --
// and (for a not-yet-aired occurrence) has its stored snapshot rewritten
// to match: "rewrite later not-yet-aired occurrences' snapshots when an
// earlier one is re-derived."
//
// An aired occurrence (its start is at or before now -- see PlanBlock's
// doc comment for what "aired" means here, notably that it includes an
// on-air occurrence GenerateForTimeRange's phase 1 injects so a mid-apply
// doesn't cut off whatever Tunarr is currently playing) is never
// re-derived, regardless of chain state: its content is replayed verbatim
// from committed history via airedSeriesOccurrenceContent (empty if that
// history has since been pruned -- what actually aired is historical
// fact a later apply cannot invent). Its effect on cursor state is
// replayed too, not recomputed: the occurrence's own stored snapshot
// already carries the per-show POST-state captured when it was planned
// (OccurrenceSnapshot.PostStates), so the aired branch applies that to
// `chain` -- the baseline any not-yet-aired occurrence after it
// re-derives from -- and syncs it into e.pendingStates (persisted for
// real by Commit()) through syncPostStates' guards. Deriving the advance
// from anything else was every prior round's bug in turn: a re-plan
// against the current spec drifted with edits, and reconstructing from
// committed content metadata broke on mid-occurrence on_complete
// restarts (content [E1..E3], correct post-state S01E01), on programs
// that vanished from the Tunarr catalog (history rows carry no
// season/episode to reconstruct from), and on stale replays dragging a
// cursor another block had already advanced. An aired occurrence whose
// snapshot is gone (operator invalidation) or predates post-state
// capture (a pre-migration-000006 legacy row) contributes no advance at
// all; the chain re-seeds from live series_state, which the
// invalidation's own trigger (an operator cursor write) has just
// made the freshest truth.

// seriesChainResult is establishSeriesChain's result, bundled into one
// struct to stay within revive's 3-result-limit (chain, content, and
// planned are all independently meaningful to the caller; see
// establishSeriesChain's doc comment).
type seriesChainResult struct {
	chain   map[string]*SeriesState
	content []tunarr.Program
	planned bool
}

// establishSeriesChain is planSeriesOccurrences' "chain is still nil"
// step, factored out on its own: given the FIRST shell of a batch, it
// seeds the chain one of three ways, cheapest/safest first:
//
//  1. This occurrence already has a stored snapshot: seed chain from it
//     (planned=false -- the caller runs the shared aired/not-aired
//     handling for this same shell).
//  2. No snapshot, but StateStore.GetCommittedOccurrence already has rows
//     for it: this occurrence WAS planned by some earlier apply -- only
//     its snapshot is gone, via DeleteFutureOccurrenceSnapshots (an
//     operator's cursor write -- PATCH /state/series, CLI
//     set/reset/import -- or a block delete's orphan cleanup) or
//     CleanupOccurrenceSnapshots' retention GC -- so it must NOT be
//     real-planned again (planned=false, same as case 1): for an
//     aired/on-air occurrence that would silently replace already-aired,
//     historical content with something freshly (and differently)
//     picked, cutting Tunarr over mid-episode into DIFFERENT content on
//     top of it; for either aired or not-yet-aired it would call
//     recordHistory (append-only, RecordScheduleHistory) on rows that
//     already exist, producing a doubled/duplicate assignment for the
//     same occurrence once GetCommittedOccurrence reads it back. Both
//     were reviewer-probed directly. Chain seeds from the current live
//     series_state instead of a snapshot that no longer exists --
//     planSeriesOccurrences' own aired branch keeps that live state
//     accurate as of the most recently aired/on-air occurrence (see its
//     doc comment), so it's a safe substitute, not a stale one.
//  3. Neither exists: genuinely never planned by any apply at all -- the
//     one case that really advances the persisted cursor and appends
//     (rather than replaces) schedule_history.
func (e *Engine) establishSeriesChain(block Block, availablePrograms []tunarr.Program, occurrenceStart time.Time) (seriesChainResult, error) {
	ctx := context.Background()

	snapshot, hasSnapshot, err := e.store.GetOccurrenceSnapshot(ctx, block.ID, occurrenceStart)
	if err != nil {
		return seriesChainResult{}, fmt.Errorf("failed to check occurrence snapshot for block %q at %s: %w", block.Name, occurrenceStart, err)
	}
	if hasSnapshot {
		return seriesChainResult{chain: statesFromSnapshot(snapshot.PreStates)}, nil
	}

	_, hasCommitted, err := e.store.GetCommittedOccurrence(ctx, block.Name, occurrenceStart)
	if err != nil {
		return seriesChainResult{}, fmt.Errorf("failed to check committed occurrence for block %q at %s: %w", block.Name, occurrenceStart, err)
	}
	if hasCommitted {
		chain, err := e.cloneLiveSeriesStates(block)
		if err != nil {
			return seriesChainResult{}, err
		}
		return seriesChainResult{chain: chain}, nil
	}

	before, err := e.captureSeriesSnapshot(block)
	if err != nil {
		return seriesChainResult{}, err
	}

	seq := e.nextPlanSeq()
	rng := occurrenceRand(block.ID, occurrenceStart)
	content := e.planSeriesBlockWithContext(engineSeriesContext{e}, block, availablePrograms, rng)
	e.recordHistory(content, block.ChannelID, block.Name, time.Now(), occurrenceStart)
	// Capture the post-plan cursor too (planning just advanced it through
	// engineSeriesContext into e.pendingStates, which captureSeriesSnapshot
	// reads first): once this occurrence airs, its stored post-state --
	// not a reconstruction from its content -- is what advances
	// series_state. See OccurrenceSnapshot's doc comment.
	after, err := e.captureSeriesSnapshot(block)
	if err != nil {
		return seriesChainResult{}, err
	}
	// This real plan wrote live state directly (engineSeriesContext), so
	// stamp those writes with the same provenance the snapshot row gets:
	// the eventual aired replay of this occurrence then ties against the
	// live cursor (seq == CursorPlanSeq) and is correctly a no-op, while
	// a STALE replay from an older plan can't outrank what this plan
	// just wrote. Only shows this plan ACTUALLY changed (before !=
	// after) are stamped: a block.Series show can carry a pendingStates
	// entry written by an EARLIER block in this same apply, and a plan
	// that never reached the show (disabled, completed, duration
	// exhausted) must not bump its provenance -- doing so would outrank,
	// and silently drop, that other block's legitimate later replay
	// whose sequence falls in between. max() rather than assignment as a
	// last-ditch floor: provenance must never move backward even if a
	// higher value got in some other way (e.g. an imported
	// cursor_plan_seq from a machine with a fast clock).
	for _, sc := range block.Series {
		if before[sc.ShowTitle] == after[sc.ShowTitle] {
			continue
		}
		if st, ok := e.pendingStates[sc.ShowTitle]; ok && seq > st.CursorPlanSeq {
			st.CursorPlanSeq = seq
		}
	}
	e.pendingSnapshots = append(e.pendingSnapshots, occurrenceSnapshotRecord{
		blockID: block.ID, occurrenceStart: occurrenceStart,
		snapshot: OccurrenceSnapshot{PreStates: before, PostStates: after, PlanSeq: seq},
	})

	// Seed the chain for subsequent occurrences from CLONES of the
	// just-updated real state, never the live *SeriesState pointers
	// themselves -- chain-mode planning must never be able to mutate
	// e.pendingStates through an aliased pointer.
	chain, err := e.cloneLiveSeriesStates(block)
	if err != nil {
		return seriesChainResult{}, err
	}

	return seriesChainResult{chain: chain, content: content, planned: true}, nil
}

func (e *Engine) planSeriesOccurrences(block Block, availablePrograms []tunarr.Program, shells []ScheduledSlot, now time.Time) ([]ScheduledSlot, error) {
	var chain map[string]*SeriesState

	out := make([]ScheduledSlot, 0, len(shells))
	for _, shell := range shells {
		occurrenceStart := shell.StartTime
		aired := !occurrenceStart.After(now)

		if chain == nil {
			result, err := e.establishSeriesChain(block, availablePrograms, occurrenceStart)
			if err != nil {
				return nil, err
			}
			chain = result.chain
			if result.planned {
				// The establish step already fully planned (and recorded
				// history/a snapshot for) this occurrence -- a genuinely
				// never-before-seen one, planned for real. Nothing left to
				// do for it.
				shell.Programs = result.content
				out = append(out, shell)
				continue
			}
		}

		if aired {
			content, newChain, err := e.replayAiredOccurrence(block, availablePrograms, chain, occurrenceStart)
			if err != nil {
				return nil, err
			}
			chain = newChain
			shell.Programs = content
			out = append(out, shell)
			continue
		}

		seq := e.nextPlanSeq()
		rng := occurrenceRand(block.ID, occurrenceStart)
		before := snapshotFromStates(chain) // value-only copies: planning's in-place chain mutation can't touch them
		content := e.planSeriesBlockWithContext(&snapshotSeriesContext{states: chain}, block, availablePrograms, rng)
		e.pendingSnapshots = append(e.pendingSnapshots, occurrenceSnapshotRecord{
			blockID: block.ID, occurrenceStart: occurrenceStart,
			snapshot: OccurrenceSnapshot{PreStates: before, PostStates: snapshotFromStates(chain), PlanSeq: seq},
		})
		entries := makeHistoryEntries(content, block.ChannelID, block.Name, time.Now(), occurrenceStart)
		e.pendingReplacements = append(e.pendingReplacements, occurrenceReplacement{
			blockName: block.Name, occurrenceStart: occurrenceStart, entries: entries,
		})

		shell.Programs = content
		out = append(out, shell)
	}

	return out, nil
}

// replayAiredOccurrence handles one aired (or on-air) shell of
// planSeriesOccurrences: replays the occurrence's committed content
// verbatim, and replays its plan-time post-state -- never a
// reconstruction from content or a re-plan against the current spec.
// Each aired shell reads its OWN snapshot row (only the batch's first
// shell went through establishSeriesChain): the post-state becomes the
// chain baseline for whatever follows (the returned chain), and
// (guarded, see syncPostStates) the persisted cursor advance.
//
// When there is no post-state to replay -- the snapshot was invalidated
// (an operator cursor write -- live series_state IS the
// fresher truth now) or predates post-state capture (a
// pre-migration-000006 legacy row, whose old-code plan already advanced
// live state itself) -- the chain is re-seeded from live state instead,
// so any not-yet-aired occurrence after this one derives from that
// truth, and nothing is persisted: there is nothing trustworthy of this
// occurrence's own to persist.
func (e *Engine) replayAiredOccurrence(block Block, availablePrograms []tunarr.Program, chain map[string]*SeriesState, occurrenceStart time.Time) ([]tunarr.Program, map[string]*SeriesState, error) {
	ctx := context.Background()
	content, err := e.airedSeriesOccurrenceContent(ctx, block, availablePrograms, occurrenceStart)
	if err != nil {
		return nil, nil, err
	}

	snap, hasSnap, err := e.store.GetOccurrenceSnapshot(ctx, block.ID, occurrenceStart)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to check occurrence snapshot for block %q at %s: %w", block.Name, occurrenceStart, err)
	}
	if !hasSnap || snap.PostStates == nil {
		reseeded, err := e.cloneLiveSeriesStates(block)
		if err != nil {
			return nil, nil, err
		}
		return content, reseeded, nil
	}

	applyPostStates(chain, snap.PostStates)
	if err := e.syncPostStates(snap, occurrenceStart); err != nil {
		return nil, nil, err
	}
	return content, chain, nil
}

// airedSeriesOccurrenceContent returns an aired (or on-air) series
// occurrence's frozen assignment, replayed verbatim from committed
// history -- never re-derived, regardless of whether a cursor snapshot
// or the block spec has since changed: what actually aired is historical
// fact. If that history has since been pruned (CleanupScheduleHistory),
// this returns (nil, nil) -- an honest "nothing to replay" -- rather than
// falling back to fabricating fresh content from the current spec, which
// would silently violate "aired occurrences never re-plan" the moment
// retention pruned their record.
func (e *Engine) airedSeriesOccurrenceContent(ctx context.Context, block Block, availablePrograms []tunarr.Program, occurrenceStart time.Time) ([]tunarr.Program, error) {
	committed, ok, err := e.store.GetCommittedOccurrence(ctx, block.Name, occurrenceStart)
	if err != nil {
		return nil, fmt.Errorf("failed to check committed occurrence for block %q at %s: %w", block.Name, occurrenceStart, err)
	}
	if !ok {
		return nil, nil
	}
	return resolveCommittedPrograms(committed, availablePrograms), nil
}

// captureSeriesSnapshot reads the current cursor for every show
// block.Series references, via the same chain e.getSeriesState always
// uses, and clones each one into a value-only SeriesStateSnapshot so this
// occurrence's captured baseline is a genuinely independent copy,
// unaffected by whatever mutation the planning pass this feeds is about
// to perform on the live *SeriesState.
func (e *Engine) captureSeriesSnapshot(block Block) (map[string]SeriesStateSnapshot, error) {
	states, err := e.cloneLiveSeriesStates(block)
	if err != nil {
		return nil, err
	}
	return snapshotFromStates(states), nil
}

// cloneLiveSeriesStates returns cloned (never aliased) *SeriesState
// pointers for every show block.Series references, read from the live
// cursor via e.getSeriesState. Used to seed a scratch chain (or a
// SeriesStateSnapshot, via captureSeriesSnapshot) from the current real
// state without letting scratch-mode planning mutate the real state
// through an aliased pointer -- see planSeriesOccurrences' doc comment.
func (e *Engine) cloneLiveSeriesStates(block Block) (map[string]*SeriesState, error) {
	states := make(map[string]*SeriesState, len(block.Series))
	for _, sc := range block.Series {
		if _, ok := states[sc.ShowTitle]; ok {
			continue
		}
		state, err := e.getSeriesState(sc.ShowTitle)
		if err != nil {
			return nil, fmt.Errorf("failed to read live series state for %q: %w", sc.ShowTitle, err)
		}
		cloned := *state
		states[sc.ShowTitle] = &cloned
	}
	return states, nil
}

// applyPostStates overwrites chain's entry for every show in post with
// that show's stored plan-time post-state -- an aired occurrence's
// actual end state, which is the baseline any not-yet-aired occurrence
// after it in the same batch must re-derive from. Shows absent from post
// (this occurrence's plan never touched them) keep whatever the chain
// already holds.
func applyPostStates(chain map[string]*SeriesState, post map[string]SeriesStateSnapshot) {
	for title, s := range post {
		chain[title] = stateFromSnapshot(title, s)
	}
}

// syncPostStates persists an aired occurrence's stored plan-time
// post-state into e.pendingStates, so Commit() writes it for real via
// UpdateSeriesState: CurrentSeason/CurrentEpisode, Completed, Disabled,
// RunCount (via max, never backward -- run counts only accumulate), and
// LastAired stamped from occurrenceStart (the occurrence's own fixed
// airtime, so replaying the same occurrence on every apply is
// idempotent, and so a Seeded pre-state's sentinel marker can never leak
// into persistence). Completion bookkeeping is deliberately included: an
// on_complete:disable or a max_runs trip decided at plan time is real
// state the persisted row must reflect once the occurrence airs --
// operator writes to those same fields are protected by the stamp guard
// below, not by excluding them. OperatorUpdatedAt always rides through
// from the live row untouched.
//
// Two guards, checked per show and in this order:
//
//   - operator wins: a show whose live series_state carries an operator
//     write (SeriesState.OperatorUpdatedAt -- PATCH /state/series, CLI
//     `state set`/`state reset`/`state import`) NEWER than this
//     occurrence's own commit stamp (the snapshot row's RecordedAt) is
//     skipped entirely. The operator paths normally delete the
//     occurrence's snapshot outright
//     (store.InvalidateSeriesOccurrenceSnapshots), so this is the
//     backstop for when they couldn't: a logged-and-continued
//     invalidation failure, or an operator write racing an in-flight
//     apply. It's what makes an operator's BACKWARD jump stick -- every
//     occurrence planned before the jump is, by definition, older than
//     it.
//   - provenance: the post-state is written only if its plan sequence
//     (snap.PlanSeq) is strictly newer than the plan that last wrote the
//     live cursor (SeriesState.CursorPlanSeq) -- a stale replay from an
//     OLDER plan (or a re-replay of an occurrence already applied) is
//     rejected. A value-scoped "forward-only" comparison here would
//     silently drop a legitimate on_complete:restart wrap (post-state
//     S01E01 after S01E05), freezing the persisted cursor at the
//     pre-wrap high-water mark, where the next snapshot invalidation
//     would re-derive onto already-aired episodes.
//   - baseline agreement, for BACKWARD moves only: provenance alone
//     over-corrects the other way when two blocks share a show. A
//     not-yet-aired occurrence is re-derived every apply, so it always
//     carries the freshest PlanSeq -- while its PreStates baseline
//     (seeded from its own frozen snapshot) can be arbitrarily stale. A
//     slower block's occurrence airing with "newest plan + ancient
//     baseline" must not rewind a cursor a faster block advanced. So a
//     backward move additionally requires the plan's OWN starting
//     baseline (snap.PreStates[title]) to equal the live cursor -- a
//     compare-and-swap. A restart wrap passes (its pre-state IS the
//     live high-water mark it planned from); a stale-baseline slow
//     block fails (it planned from a cursor that is no longer the live
//     one, so its wrap-shaped result is about a different past).
//     Forward moves keep pure provenance ordering: blocks sharing a
//     show cooperatively advance one cursor, newest plan wins.
func (e *Engine) syncPostStates(snap OccurrenceSnapshot, occurrenceStart time.Time) error {
	for title, s := range snap.PostStates {
		existing, err := e.getSeriesState(title)
		if err != nil {
			return fmt.Errorf("failed to read existing series state for %q: %w", title, err)
		}
		if existing.OperatorUpdatedAt != nil && existing.OperatorUpdatedAt.After(snap.RecordedAt) {
			continue
		}
		if snap.PlanSeq <= existing.CursorPlanSeq {
			continue
		}
		if cursorBehind(s.CurrentSeason, s.CurrentEpisode, existing.CurrentSeason, existing.CurrentEpisode) {
			pre, ok := snap.PreStates[title]
			if !ok || pre.CurrentSeason != existing.CurrentSeason || pre.CurrentEpisode != existing.CurrentEpisode {
				continue
			}
		}

		cloned := *existing
		cloned.CurrentSeason = s.CurrentSeason
		cloned.CurrentEpisode = s.CurrentEpisode
		cloned.Completed = s.Completed
		cloned.Disabled = s.Disabled
		if s.RunCount > cloned.RunCount {
			cloned.RunCount = s.RunCount
		}
		aired := occurrenceStart
		cloned.LastAired = &aired
		cloned.CursorPlanSeq = snap.PlanSeq
		e.pendingStates[title] = &cloned
	}
	return nil
}

// cursorBehind reports whether cursor (season, episode) is strictly
// earlier than (otherSeason, otherEpisode) -- a lower season, or the
// same season with a lower episode number.
func cursorBehind(season, episode, otherSeason, otherEpisode int) bool {
	if season != otherSeason {
		return season < otherSeason
	}
	return episode < otherEpisode
}

// resolveCommittedPrograms upgrades a committed occurrence's stored
// assignment (StateStore.GetCommittedOccurrence's minimal reconstruction:
// only ID/UUID, Title, Duration, and Type survive the round trip through
// schedule_history -- see ScheduleHistoryEntry's doc comment) back to the
// full tunarr.Program from availablePrograms when the same program (by
// GetID()) is still in the live catalog, so a replayed occurrence's
// result is indistinguishable from the original plan's (ShowTitle,
// SeasonNumber, EpisodeNumber, Genres, and everything else the minimal
// reconstruction can't carry) whenever possible. Falls back to the
// stored/reconstructed value itself when a program has since disappeared
// from the catalog -- committed content isn't discarded just because
// Tunarr's library changed since the original apply.
func resolveCommittedPrograms(committed, availablePrograms []tunarr.Program) []tunarr.Program {
	if len(committed) == 0 {
		// nil, not an allocated empty slice: a genuinely-empty freshly
		// planned result is also nil (playlist is never appended to), and
		// keeping this branch's output identical matters for byte-for-byte
		// plan comparisons (e.g. reflect.DeepEqual across two consecutive
		// applies).
		return nil
	}

	live := make(map[string]tunarr.Program, len(availablePrograms))
	for _, p := range availablePrograms {
		live[p.GetID()] = p
	}

	resolved := make([]tunarr.Program, len(committed))
	for i, c := range committed {
		if p, ok := live[c.GetID()]; ok {
			resolved[i] = p
		} else {
			resolved[i] = c
		}
	}
	return resolved
}

func (e *Engine) planFilterBlock(block Block, availablePrograms []tunarr.Program, occurrenceStart time.Time) ([]tunarr.Program, error) {
	candidates, err := FilterPrograms(availablePrograms, block.Filter)
	if err != nil {
		metrics.ScheduleErrorsTotal.WithLabelValues("filter_programs_error").Inc()
		return nil, err
	}

	if len(candidates) == 0 {
		return nil, fmt.Errorf("no content matches filter for block %s", block.Name)
	}

	// Filter out recently scheduled programs to prevent repetition
	originalCount := len(candidates)
	candidates = e.filterByHistory(candidates, block.ChannelID)

	if len(candidates) < originalCount {
		e.logger.Debug("filtered out recently scheduled programs",
			"filtered_count", originalCount-len(candidates),
			"block_name", block.Name)
	}

	// If we filtered everything out, fall back to all candidates
	// (better to repeat than have no content)
	if len(candidates) == 0 {
		metrics.ScheduleFallbacksTotal.WithLabelValues(block.ChannelID, block.Name, "history_exhausted").Inc()
		e.logger.Warn("all candidates recently scheduled, allowing repeats",
			"block_name", block.Name,
			"original_count", originalCount)
		var filterErr error
		candidates, filterErr = FilterPrograms(availablePrograms, block.Filter)
		if filterErr != nil {
			e.logger.Error("failed to re-filter programs during fallback",
				"block_name", block.Name,
				"error", filterErr)
			return nil, fmt.Errorf("history fallback failed for block %s: %w", block.Name, filterErr)
		}
	}

	var playlist []tunarr.Program
	var currentDuration int64 = 0
	targetDuration := int64(block.Duration) * 60000 // ms

	maxOverflowMs := int64(block.MaxDurationOverflowMinutes) * time.Minute.Milliseconds()
	allowedDurationWithOverflow := targetDuration + maxOverflowMs

	// Simple random shuffle and fill
	// #nosec G404 - content shuffle for programming variety, not a security-sensitive value
	rand.Shuffle(len(candidates), func(i, j int) {
		candidates[i], candidates[j] = candidates[j], candidates[i]
	})

	for _, p := range candidates {
		if currentDuration+p.GetDurationMs() <= allowedDurationWithOverflow {
			playlist = append(playlist, p)
			currentDuration += p.GetDurationMs()
			metrics.ProgramsScheduledTotal.WithLabelValues(block.ChannelID, block.Name, p.Type).Inc()
		} else {
			break
		}
	}

	// Gap filling with filler content
	gapDuration := targetDuration - currentDuration
	gapMinutes := int(gapDuration / 60000)

	// Check if we should add filler content
	if block.Filler.Enabled && gapMinutes >= block.Filler.MinGapTime {
		// nil rng: a filter block's content (this included) is frozen
		// verbatim after its first commit, never re-derived (see
		// PlanBlock's doc comment), so filler non-determinism here can't
		// break apply-idempotence the way it can for a re-derived series
		// occurrence -- global math/rand is fine.
		fillerPrograms, err := e.getFiller(block, gapDuration, nil)
		if err != nil {
			e.logger.Warn("failed to get filler for block",
				"block_name", block.Name,
				"error", err)
		} else if len(fillerPrograms) > 0 {
			e.logger.Info("adding filler programs to fill gap",
				"filler_count", len(fillerPrograms),
				"gap_minutes", gapMinutes,
				"block_name", block.Name)
			playlist = append(playlist, fillerPrograms...)
			for _, f := range fillerPrograms {
				currentDuration += f.GetDurationMs()
			}
		}
	}

	// Log if there's still a significant gap
	finalGap := targetDuration - currentDuration
	finalGapMinutes := int(finalGap / 60000)
	if finalGapMinutes > 5 {
		e.logger.Info("block has remaining gap after filling",
			"block_name", block.Name,
			"gap_minutes", finalGapMinutes)
	}

	// Record scheduled programs in history
	e.recordHistory(playlist, block.ChannelID, block.Name, time.Now(), occurrenceStart)

	return playlist, nil
}

// seriesPlanContext abstracts where a series block's per-show cursor
// state comes from and where its advances go, so the same planning logic
// (planSeriesForConfig, findNextSeriesEpisode, markSeriesCompleted) can
// run against two different backends -- see PlanBlock's doc comment for
// which callers use which:
//
//   - engineSeriesContext: the engine's real, global, cross-occurrence
//     chain (e.pendingStates, falling back to the authoritative
//     persisted store). Advances here are real and get committed.
//   - snapshotSeriesContext: a local, throwaway copy seeded from one
//     occurrence's stored snapshot. Advances here never leave the
//     snapshotSeriesContext -- they exist only to let a single planning
//     pass run to completion.
type seriesPlanContext interface {
	get(showTitle string) (*SeriesState, error)
	set(showTitle string, state *SeriesState)
}

// engineSeriesContext is seriesPlanContext backed by the engine's real
// state chain -- see seriesPlanContext's doc comment.
type engineSeriesContext struct{ e *Engine }

func (c engineSeriesContext) get(showTitle string) (*SeriesState, error) {
	return c.e.getSeriesState(showTitle)
}

func (c engineSeriesContext) set(showTitle string, state *SeriesState) {
	c.e.pendingStates[showTitle] = state
}

// snapshotSeriesContext is seriesPlanContext backed by a fixed,
// in-memory-only snapshot -- see seriesPlanContext's doc comment. A show
// title with no entry in the snapshot starts from the same S01E01
// default a genuinely untracked show gets from
// Store.GetSeriesState/MockStateStore.GetSeriesState.
type snapshotSeriesContext struct {
	states map[string]*SeriesState
}

// seededMarker is the non-nil, non-zero LastAired value statesFromSnapshot
// assigns to a reconstructed *SeriesState whose snapshot says the
// underlying cursor was already initialized -- see
// SeriesStateSnapshot.Seeded's doc comment. Its exact value is
// meaningless (initializeSeriesState only checks LastAired != nil &&
// !LastAired.IsZero()); it exists only to be a fixed, non-zero instant.
var seededMarker = time.Unix(0, 0)

// statesFromSnapshot reconstructs live *SeriesState values from a
// persisted SeriesStateSnapshot map -- see stateFromSnapshot.
func statesFromSnapshot(snapshot map[string]SeriesStateSnapshot) map[string]*SeriesState {
	states := make(map[string]*SeriesState, len(snapshot))
	for showTitle, s := range snapshot {
		states[showTitle] = stateFromSnapshot(showTitle, s)
	}
	return states
}

// stateFromSnapshot reconstructs one live *SeriesState from its
// persisted, value-only SeriesStateSnapshot form -- see
// SeriesStateSnapshot.Seeded's doc comment for why LastAired is set (to
// seededMarker, an arbitrary fixed non-zero instant) exactly when Seeded
// is true, rather than always left nil.
func stateFromSnapshot(showTitle string, s SeriesStateSnapshot) *SeriesState {
	state := &SeriesState{
		ShowTitle:      showTitle,
		CurrentSeason:  s.CurrentSeason,
		CurrentEpisode: s.CurrentEpisode,
		Completed:      s.Completed,
		Disabled:       s.Disabled,
		RunCount:       s.RunCount,
	}
	if s.Seeded {
		state.LastAired = &seededMarker
	}
	return state
}

// snapshotFromStates is statesFromSnapshot's inverse: converts live
// *SeriesState values into the persisted, value-only SeriesStateSnapshot
// shape (Seeded standing in for LastAired -- see its doc comment).
func snapshotFromStates(states map[string]*SeriesState) map[string]SeriesStateSnapshot {
	snapshot := make(map[string]SeriesStateSnapshot, len(states))
	for title, s := range states {
		snapshot[title] = SeriesStateSnapshot{
			CurrentSeason:  s.CurrentSeason,
			CurrentEpisode: s.CurrentEpisode,
			Completed:      s.Completed,
			Disabled:       s.Disabled,
			RunCount:       s.RunCount,
			Seeded:         s.LastAired != nil,
		}
	}
	return snapshot
}

func (c *snapshotSeriesContext) get(showTitle string) (*SeriesState, error) {
	if state, ok := c.states[showTitle]; ok {
		return state, nil
	}
	return &SeriesState{ShowTitle: showTitle, CurrentSeason: 1, CurrentEpisode: 1}, nil
}

func (c *snapshotSeriesContext) set(showTitle string, state *SeriesState) {
	c.states[showTitle] = state
}

// planSeriesBlock is the store-less fallback path (e.store == nil): plans
// fresh against the engine's real state chain every time, exactly like
// before idempotent apply existed. Store-backed callers use
// planSeriesOccurrences instead (see PlanBlock).
func (e *Engine) planSeriesBlock(block Block, availablePrograms []tunarr.Program, occurrenceStart time.Time) ([]tunarr.Program, error) {
	// nil rng: no store means no idempotent-apply guarantee to uphold here
	// anyway (see planSeriesOccurrences' doc comment on why a re-derive
	// needs a deterministic seed) -- global math/rand is fine.
	playlist := e.planSeriesBlockWithContext(engineSeriesContext{e}, block, availablePrograms, nil)
	e.recordHistory(playlist, block.ChannelID, block.Name, time.Now(), occurrenceStart)
	return playlist, nil
}

// planSeriesBlockWithContext is the actual series-block planning
// algorithm, parameterized over where its per-show cursor state comes
// from and goes (see seriesPlanContext's doc comment) and over the source
// of randomness fallback/filler content shuffling draws from: rng nil
// means "use the global math/rand source" (fine for a block that's only
// ever planned once, like a first-time occurrence, or the store-less
// path); a non-nil, occurrence-seeded rng (see occurrenceRand) is what
// makes a re-derived occurrence's filler/fallback pick the SAME content
// again given the same inputs, instead of reshuffling differently on
// every apply.
func (e *Engine) planSeriesBlockWithContext(ctx seriesPlanContext, block Block, availablePrograms []tunarr.Program, rng *rand.Rand) []tunarr.Program {
	var playlist []tunarr.Program
	var currentDuration int64
	targetDuration := int64(block.Duration) * 60000 // ms

	for _, seriesConf := range block.Series {
		seriesPlaylist, durationAdded := e.planSeriesForConfig(ctx, block, seriesConf, availablePrograms, targetDuration-currentDuration)
		playlist = append(playlist, seriesPlaylist...)
		currentDuration += durationAdded

		if currentDuration >= targetDuration {
			break
		}
	}

	playlist, currentDuration = e.applySeriesFallback(block, availablePrograms, playlist, durationBudget{current: currentDuration, target: targetDuration}, rng)
	playlist, _ = e.applyBlockFiller(block, playlist, currentDuration, targetDuration, rng)

	return playlist
}

func (e *Engine) planSeriesForConfig(ctx seriesPlanContext, block Block, seriesConf SeriesConfig, availablePrograms []tunarr.Program, remainingDuration int64) ([]tunarr.Program, int64) {
	if remainingDuration <= 0 {
		return nil, 0
	}

	state, err := ctx.get(seriesConf.ShowTitle)
	if err != nil {
		metrics.ScheduleErrorsTotal.WithLabelValues("get_series_state_error").Inc()
		e.logger.Error("failed to get series state",
			"show_title", seriesConf.ShowTitle,
			"error", err)
		return nil, 0
	}

	// Skip disabled series
	if state.Disabled {
		e.logger.Debug("skipping disabled series",
			"show_title", seriesConf.ShowTitle)
		return nil, 0
	}

	e.initializeSeriesState(state, seriesConf)

	var playlist []tunarr.Program
	var currentDuration int64
	episodesAdded := 0

	maxOverflowMs := int64(block.MaxDurationOverflowMinutes) * time.Minute.Milliseconds()
	allowedDurationWithOverflow := remainingDuration + maxOverflowMs

	for episodesAdded < seriesConf.EpisodesPerBlock {
		ep := e.findNextSeriesEpisode(ctx, seriesConf, state, availablePrograms)
		if ep == nil {
			break
		}

		if currentDuration+ep.GetDurationMs() <= allowedDurationWithOverflow {
			playlist = append(playlist, *ep)
			currentDuration += ep.GetDurationMs()
			state.CurrentEpisode++
			now := time.Now()
			state.LastAired = &now
			episodesAdded++
			ctx.set(seriesConf.ShowTitle, state)
			metrics.ProgramsScheduledTotal.WithLabelValues(block.ChannelID, block.Name, ep.Type).Inc()
			metrics.SeriesStateUpdatesTotal.WithLabelValues(seriesConf.ShowTitle).Inc()
		} else {
			break
		}
	}

	return playlist, currentDuration
}

func (e *Engine) initializeSeriesState(state *SeriesState, seriesConf SeriesConfig) {
	if state.LastAired != nil && !state.LastAired.IsZero() {
		return
	}

	if seriesConf.StartSeason > 0 {
		state.CurrentSeason = seriesConf.StartSeason
	}
	if seriesConf.StartEpisode > 0 {
		state.CurrentEpisode = seriesConf.StartEpisode
	}
}

func (e *Engine) findNextSeriesEpisode(ctx seriesPlanContext, seriesConf SeriesConfig, state *SeriesState, availablePrograms []tunarr.Program) *tunarr.Program {
	// Skip episodes if configured
	for e.shouldSkipEpisode(seriesConf, state) {
		e.logger.Debug("skipping episode",
			"show_title", seriesConf.ShowTitle,
			"season", state.CurrentSeason,
			"episode", state.CurrentEpisode)
		state.CurrentEpisode++
	}

	ep := findEpisode(availablePrograms, seriesConf.ShowTitle, state.CurrentSeason, state.CurrentEpisode)
	if ep != nil {
		return ep
	}

	// Try next season
	ep = findEpisode(availablePrograms, seriesConf.ShowTitle, state.CurrentSeason+1, 1)
	if ep != nil {
		state.CurrentSeason++
		state.CurrentEpisode = 1
		// Check if first episode of new season should be skipped
		if e.shouldSkipEpisode(seriesConf, state) {
			return e.findNextSeriesEpisode(ctx, seriesConf, state, availablePrograms)
		}
		return ep
	}

	e.markSeriesCompleted(ctx, seriesConf, state)
	return nil
}

func (e *Engine) shouldSkipEpisode(seriesConf SeriesConfig, state *SeriesState) bool {
	if len(seriesConf.SkipEpisodes) == 0 {
		return false
	}

	episodeID := fmt.Sprintf("S%02dE%02d", state.CurrentSeason, state.CurrentEpisode)
	for _, skip := range seriesConf.SkipEpisodes {
		if skip == episodeID {
			return true
		}
	}
	return false
}

// markSeriesCompleted records that seriesConf's show has run out of
// episodes to schedule. A no-op if state is already Completed: without
// this guard, every later occurrence in a wide apply window that also
// finds no episode left (e.g. once a show completes partway through an
// hourly block's 24 occurrences) re-ran the RunCount++ bookkeeping below,
// inflating RunCount once per occurrence instead of once per actual
// completion. The default CompletionActionContinue never resets
// Completed back to false, so without the guard this compounded without
// bound. CompletionActionRestart legitimately does reset Completed to
// false on a genuine restart, so a later completion after a real restart
// cycle is unaffected by this guard -- it isn't "still completed" at that
// point.
func (e *Engine) markSeriesCompleted(ctx seriesPlanContext, seriesConf SeriesConfig, state *SeriesState) {
	if state.Completed {
		return
	}

	e.logger.Info("series completed all episodes",
		"show_title", seriesConf.ShowTitle,
		"season", state.CurrentSeason,
		"episode", state.CurrentEpisode,
		"on_complete", seriesConf.OnComplete)

	state.Completed = true
	state.RunCount++
	metrics.SeriesStateUpdatesTotal.WithLabelValues(seriesConf.ShowTitle).Inc()

	// Handle completion action
	action := seriesConf.OnComplete
	if action == "" {
		action = CompletionActionContinue // Default
	}

	switch action {
	case CompletionActionRestart:
		// Check if max runs is set and exceeded
		if seriesConf.MaxRuns > 0 && state.RunCount >= seriesConf.MaxRuns {
			e.logger.Info("series reached max runs, disabling",
				"show_title", seriesConf.ShowTitle,
				"run_count", state.RunCount,
				"max_runs", seriesConf.MaxRuns)
			state.Disabled = true
		} else {
			e.logger.Info("restarting series from beginning",
				"show_title", seriesConf.ShowTitle,
				"run_count", state.RunCount)
			state.CurrentSeason = 1
			state.CurrentEpisode = 1
			state.Completed = false
		}

	case CompletionActionDisable:
		e.logger.Info("disabling completed series",
			"show_title", seriesConf.ShowTitle)
		state.Disabled = true

	case CompletionActionContinue:
		// Do nothing, just mark as completed
		e.logger.Debug("series marked as completed, will continue in block",
			"show_title", seriesConf.ShowTitle)
	}

	ctx.set(seriesConf.ShowTitle, state)
}

// durationBudget bundles applySeriesFallback's accumulated/target
// duration pair into one argument -- pulled out of the parameter list
// specifically to stay within revive's 5-argument-limit: this is the one
// content-fill helper that also needs availablePrograms (for
// FilterPrograms), which would otherwise push it to 6 loose parameters.
type durationBudget struct {
	current int64
	target  int64
}

func (e *Engine) applySeriesFallback(block Block, availablePrograms []tunarr.Program, playlist []tunarr.Program, budget durationBudget, rng *rand.Rand) ([]tunarr.Program, int64) {
	currentDuration, targetDuration := budget.current, budget.target
	if currentDuration >= targetDuration {
		return playlist, currentDuration
	}

	if block.Fallback.Mode != FallbackModeFiller {
		return playlist, currentDuration
	}

	candidates, err := FilterPrograms(availablePrograms, block.Fallback.FillerFilter)
	if err != nil {
		metrics.ScheduleFallbacksTotal.WithLabelValues(block.ChannelID, block.Name, "fallback_filter_error").Inc()
		e.logger.Warn("series fallback filter failed",
			"block_name", block.Name,
			"error", err)
		return playlist, currentDuration
	}
	if len(candidates) == 0 {
		metrics.ScheduleFallbacksTotal.WithLabelValues(block.ChannelID, block.Name, "fallback_no_content").Inc()
		e.logger.Debug("no content matches series fallback filter",
			"block_name", block.Name,
			"gap_minutes", int((targetDuration-currentDuration)/60000))
		return playlist, currentDuration
	}

	shuffleWith(rng, len(candidates), func(i, j int) {
		candidates[i], candidates[j] = candidates[j], candidates[i]
	})

	for _, p := range candidates {
		if currentDuration+p.GetDurationMs() <= targetDuration {
			playlist = append(playlist, p)
			currentDuration += p.GetDurationMs()
		}
		if currentDuration >= targetDuration {
			break
		}
	}

	return playlist, currentDuration
}

func (e *Engine) applyBlockFiller(block Block, playlist []tunarr.Program, currentDuration, targetDuration int64, rng *rand.Rand) ([]tunarr.Program, int64) {
	gapDuration := targetDuration - currentDuration
	gapMinutes := int(gapDuration / 60000)
	if !block.Filler.Enabled || gapMinutes < block.Filler.MinGapTime {
		return playlist, currentDuration
	}

	fillerPrograms, err := e.getFiller(block, gapDuration, rng)
	if err != nil {
		metrics.ScheduleFallbacksTotal.WithLabelValues(block.ChannelID, block.Name, "filler_fetch_error").Inc()
		e.logger.Warn("failed to fetch filler content for series block",
			"block_name", block.Name,
			"filler_list_id", block.Filler.FillerListID,
			"gap_minutes", gapMinutes,
			"error", err)
		return playlist, currentDuration
	}
	if len(fillerPrograms) == 0 {
		metrics.ScheduleFallbacksTotal.WithLabelValues(block.ChannelID, block.Name, "filler_empty").Inc()
		e.logger.Debug("no filler programs fit remaining gap",
			"block_name", block.Name,
			"gap_minutes", gapMinutes)
		return playlist, currentDuration
	}

	playlist = append(playlist, fillerPrograms...)
	for _, filler := range fillerPrograms {
		currentDuration += filler.GetDurationMs()
	}

	return playlist, currentDuration
}

func (e *Engine) getSeriesState(title string) (*SeriesState, error) {
	if state, ok := e.pendingStates[title]; ok {
		return state, nil
	}
	return e.store.GetSeriesState(context.Background(), title)
}

func findEpisode(programs []tunarr.Program, title string, season, episode int) *tunarr.Program {
	for i := range programs {
		p := &programs[i]
		if p.Type == "episode" && p.ShowTitle == title && p.SeasonNumber == season && p.EpisodeNumber == episode {
			return p
		}
	}
	return nil
}

// recordHistory records that programs were just planned for one
// occurrence of blockName: scheduledAt is the recency-dedup bookkeeping
// timestamp (see ScheduleHistoryEntry's doc comment -- wall-clock
// planning time, e.history's WasRecentlyScheduled window is measured
// against it), occurrenceStart is that occurrence's own cron-computed
// start time -- the identity PlanBlock's idempotence check looks
// (block.Name, occurrenceStart) up by.
func (e *Engine) recordHistory(programs []tunarr.Program, channelID, blockName string, scheduledAt, occurrenceStart time.Time) {
	e.history.RecordPrograms(programs, channelID, blockName, scheduledAt)
	e.pendingHistory = append(e.pendingHistory, makeHistoryEntries(programs, channelID, blockName, scheduledAt, occurrenceStart)...)
}

// makeHistoryEntries converts programs into the rows that represent one
// occurrence's assignment. When programs is empty, it still returns
// exactly one row -- a sentinel with an empty ProgramID -- so
// StateStore.GetCommittedOccurrence can tell "this occurrence was planned
// and genuinely produced nothing" apart from "this occurrence was never
// planned at all"; without it, a filter block occurrence with no matching
// content (or an aired series occurrence that legitimately scheduled
// zero programs) could never be recognized as already committed, and
// would be replanned on every subsequent apply that still covered it.
func makeHistoryEntries(programs []tunarr.Program, channelID, blockName string, scheduledAt, occurrenceStart time.Time) []ScheduleHistoryEntry {
	if len(programs) == 0 {
		return []ScheduleHistoryEntry{{
			ChannelID:       channelID,
			BlockName:       blockName,
			ScheduledAt:     scheduledAt,
			OccurrenceStart: occurrenceStart,
		}}
	}

	entries := make([]ScheduleHistoryEntry, 0, len(programs))
	for i, program := range programs {
		entries = append(entries, ScheduleHistoryEntry{
			ProgramID:       program.GetID(),
			ChannelID:       channelID,
			BlockName:       blockName,
			ScheduledAt:     scheduledAt,
			OccurrenceStart: occurrenceStart,
			Sequence:        i,
			DurationMs:      program.Duration,
			Title:           program.Title,
			Type:            program.Type,
		})
	}
	return entries
}

func (e *Engine) filterByHistory(programs []tunarr.Program, channelID string) []tunarr.Program {
	filtered := make([]tunarr.Program, 0, len(programs))
	window := e.history.Window()
	ctx := context.Background()

	for _, program := range programs {
		programID := program.GetID()
		if e.history.WasRecentlyScheduled(programID, channelID) {
			continue
		}
		if e.store != nil {
			recent, err := e.store.WasRecentlyScheduled(ctx, programID, channelID, window)
			if err != nil {
				e.logger.Warn("failed to check schedule history",
					"program_id", programID,
					"channel_id", channelID,
					"error", err)
			} else if recent {
				continue
			}
		}
		filtered = append(filtered, program)
	}

	return filtered
}

// getFiller retrieves filler content to fill the remaining time. rng nil
// uses the global math/rand source; see planSeriesBlockWithContext's doc
// comment for when a non-nil, occurrence-seeded one matters.
func (e *Engine) getFiller(block Block, remainingDuration int64, rng *rand.Rand) ([]tunarr.Program, error) {
	if block.Filler.FillerListID == "" {
		return nil, errors.New("no filler list ID specified")
	}

	// Fetch filler programs from the specified list. dropped counts
	// entries GetFillerPrograms discarded for failing validation (see its
	// doc comment in internal/external/tunarr/client.go) -- not fatal,
	// but worth one WARN so an operator can see it happened; a single,
	// non-paginated call trivially satisfies "one log per fetch."
	fillerContent, dropped, err := e.client.GetFillerPrograms(context.Background(), block.Filler.FillerListID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch filler programs: %w", err)
	}
	if dropped > 0 {
		e.logger.Warn("dropped invalid filler programs",
			"filler_list_id", block.Filler.FillerListID,
			"valid_count", len(fillerContent),
			"dropped_count", dropped)
	}

	if len(fillerContent) == 0 {
		return nil, fmt.Errorf("filler list %s is empty", block.Filler.FillerListID)
	}

	var fillerPlaylist []tunarr.Program
	var fillerDuration int64 = 0
	maxFillerDuration := remainingDuration

	// If max filler time is set, respect it
	if block.Filler.MaxFillerTime > 0 {
		maxFillerMs := int64(block.Filler.MaxFillerTime) * 60000
		if maxFillerMs < remainingDuration {
			maxFillerDuration = maxFillerMs
		}
	}

	// Shuffle filler content for variety
	shuffleWith(rng, len(fillerContent), func(i, j int) {
		fillerContent[i], fillerContent[j] = fillerContent[j], fillerContent[i]
	})

	// Fill remaining time with filler content
	for _, f := range fillerContent {
		if fillerDuration+f.GetDurationMs() <= maxFillerDuration {
			fillerPlaylist = append(fillerPlaylist, f)
			fillerDuration += f.GetDurationMs()
		}
		if fillerDuration >= maxFillerDuration {
			break
		}
	}

	return fillerPlaylist, nil
}
