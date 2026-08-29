// Package scheduler provides the core scheduling engine for Schedularr.
package scheduler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
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
	logger              *slog.Logger
}

// occurrenceSnapshotRecord is one pending SaveOccurrenceSnapshot call,
// queued during planning and flushed by Commit().
type occurrenceSnapshotRecord struct {
	blockName       string
	occurrenceStart time.Time
	snapshot        map[string]SeriesStateSnapshot
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
// maintenance.history_retention) should use NewEngineWithOptions instead.
func NewEngine(client *tunarr.Client, blocks []Block, store StateStore, logger *slog.Logger, loc *time.Location) *Engine {
	return NewEngineWithOptions(client, blocks, store, EngineOptions{Logger: logger, Location: loc})
}

// NewEngineWithOptions creates a new scheduling engine with optional
// configuration. Any zero-valued field in opts falls back to the same
// default NewEngine uses: opts.Logger -> slog.Default(), opts.Location ->
// time.Local, opts.HistoryWindow -> defaultHistoryWindow (7 days).
func NewEngineWithOptions(client *tunarr.Client, blocks []Block, store StateStore, opts EngineOptions) *Engine {
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
	return &Engine{
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

	// Phase 1: occurrence shells, no content yet.
	perBlockShells := make([][]ScheduledSlot, len(e.blocks))
	channelShells := make(map[string][]ScheduledSlot)

	for i, block := range e.blocks {
		metrics.SchedulesGeneratedTotal.WithLabelValues(block.ChannelID, block.Name).Inc()
		scheduleObj, err := e.parser.Parse(block.Cron)
		if err != nil {
			metrics.ScheduleErrorsTotal.WithLabelValues("cron_parse_error").Inc()
			return nil, nil, fmt.Errorf("invalid cron '%s' for block %s: %w", block.Cron, block.Name, err)
		}

		nextTime := scheduleObj.Next(start.Add(-1 * time.Second))
		for !nextTime.After(end) {
			shell := ScheduledSlot{
				StartTime: nextTime,
				EndTime:   nextTime.Add(time.Duration(block.Duration) * time.Minute),
				Block:     block,
			}
			perBlockShells[i] = append(perBlockShells[i], shell)
			channelShells[block.ChannelID] = append(channelShells[block.ChannelID], shell)

			nextTime = scheduleObj.Next(nextTime)
		}
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
	// chronological order, skipping anything phase 2 dropped.
	resolvedSchedule := make(map[string][]ScheduledSlot)
	for i, block := range e.blocks {
		for _, shell := range perBlockShells[i] {
			if !survived[keyFor(shell)] {
				continue
			}

			planned, err := e.PlanBlock(block, availablePrograms, shell.StartTime, start)
			if err != nil {
				metrics.ScheduleErrorsTotal.WithLabelValues("plan_block_error").Inc()
				return nil, nil, fmt.Errorf("failed to plan block %s: %w", block.Name, err)
			}

			shell.Programs = planned
			resolvedSchedule[block.ChannelID] = append(resolvedSchedule[block.ChannelID], shell)
		}
	}

	return resolvedSchedule, warnings, nil
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
		if err := e.store.SaveOccurrenceSnapshot(ctx, snap.blockName, snap.occurrenceStart, snap.snapshot); err != nil {
			return fmt.Errorf("failed to save occurrence snapshot for block %q: %w", snap.blockName, err)
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
	// Clear pending state after commit
	e.pendingStates = make(map[string]*SeriesState)
	e.pendingHistory = nil
	e.pendingSnapshots = nil
	e.pendingReplacements = nil
	return nil
}

// resolveConflicts resolves overlapping slots by priority, returning both
// the surviving slots and a Warning for every one dropped -- the INFO log
// lines below were, before Warning existed, the only record of a drop at
// all; API callers (POST /generate, POST /apply) now get the same
// information back in the response instead of needing server-side log
// access to notice a block's occurrence never made it into the schedule.
func (e *Engine) resolveConflicts(slots []ScheduledSlot) (kept []ScheduledSlot, dropped []Warning) {
	if len(slots) == 0 {
		return slots, nil
	}

	var resolved []ScheduledSlot
	conflicts := 0

	for i := range slots {
		shouldInclude := true

		// Check against already resolved slots
		for j := range resolved {
			if slotsOverlap(slots[i], resolved[j]) {
				conflicts++
				// Higher priority wins (higher number = higher priority)
				if slots[i].Block.Priority > resolved[j].Block.Priority {
					metrics.ConflictsResolvedTotal.Inc()
					// Remove the lower priority slot and add the higher one
					e.logger.Info("scheduling conflict resolved by priority",
						"winner_block", slots[i].Block.Name,
						"winner_priority", slots[i].Block.Priority,
						"loser_block", resolved[j].Block.Name,
						"loser_priority", resolved[j].Block.Priority,
						"start_time", slots[i].StartTime.Format("2006-01-02 15:04"))
					dropped = append(dropped, Warning{
						BlockName:         resolved[j].Block.Name,
						OccurrenceStart:   resolved[j].StartTime,
						BlockingBlockName: slots[i].Block.Name,
					})
					resolved = append(resolved[:j], resolved[j+1:]...)
					break
				} else {
					e.logger.Info("scheduling conflict - block blocked by higher priority",
						"blocked_block", slots[i].Block.Name,
						"blocked_priority", slots[i].Block.Priority,
						"blocking_block", resolved[j].Block.Name,
						"blocking_priority", resolved[j].Block.Priority,
						"start_time", slots[i].StartTime.Format("2006-01-02 15:04"))
					dropped = append(dropped, Warning{
						BlockName:         slots[i].Block.Name,
						OccurrenceStart:   slots[i].StartTime,
						BlockingBlockName: resolved[j].Block.Name,
					})
					shouldInclude = false
					break
				}
			}
		}

		if shouldInclude {
			resolved = append(resolved, slots[i])
		}
	}

	if conflicts > 0 {
		e.logger.Info("resolved scheduling conflicts using priority", "conflict_count", conflicts)
	}

	return resolved, dropped
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
// This is idempotent per (block.Name, occurrenceStart): block.Name is
// unique (enforced by the blocks table) and occurrenceStart doesn't
// change no matter how many times or how much later this same occurrence
// gets planned again, so the pair identifies one specific real-world
// occurrence across every apply that ever sees it. That matters because
// the schedule-generation window is 24h and re-applies happen more often
// than that (the serve cron loop's default 6h interval), so the same
// future occurrence falls inside several consecutive applies' windows --
// naive re-planning on every one of them used to advance a series block's
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
// entirely. planSeriesOccurrence implements the three-way split that
// supports that: never-seen (plan for real, advancing the cursor and
// capturing a snapshot), already-seen-but-still-future (re-derive from
// the *stored, immutable* snapshot plus the *current* spec, never
// touching the global cursor), and already-seen-and-now-aired (frozen,
// replayed verbatim like a filter block, since an aired occurrence is
// historical fact a later spec edit cannot retroactively change).
//
// GenerateForTimeRange's phase 3 (the only production caller) only calls
// this for occurrences that survived conflict resolution, so a
// conflict-dropped occurrence never reaches here at all -- it can't
// advance a cursor, capture a snapshot, or get recorded either. See
// GenerateForTimeRange's own doc comment for why that ordering (resolve
// conflicts, *then* plan content) matters.
func (e *Engine) PlanBlock(block Block, availablePrograms []tunarr.Program, occurrenceStart, now time.Time) ([]tunarr.Program, error) {
	if e.store == nil {
		if block.Type == BlockTypeSeries {
			return e.planSeriesBlock(block, availablePrograms, occurrenceStart)
		}
		return e.planFilterBlock(block, availablePrograms, occurrenceStart)
	}

	ctx := context.Background()

	if block.Type == BlockTypeSeries {
		return e.planSeriesOccurrence(ctx, block, availablePrograms, occurrenceStart, now)
	}

	committed, ok, err := e.store.GetCommittedOccurrence(ctx, block.Name, occurrenceStart)
	if err != nil {
		return nil, fmt.Errorf("failed to check committed occurrence for block %q at %s: %w", block.Name, occurrenceStart, err)
	}
	if ok {
		return resolveCommittedPrograms(committed, availablePrograms), nil
	}
	return e.planFilterBlock(block, availablePrograms, occurrenceStart)
}

// planSeriesOccurrence implements the three-way idempotence split
// PlanBlock's doc comment describes for series blocks.
func (e *Engine) planSeriesOccurrence(ctx context.Context, block Block, availablePrograms []tunarr.Program, occurrenceStart, now time.Time) ([]tunarr.Program, error) {
	snapshot, hasSnapshot, err := e.store.GetOccurrenceSnapshot(ctx, block.Name, occurrenceStart)
	if err != nil {
		return nil, fmt.Errorf("failed to check occurrence snapshot for block %q at %s: %w", block.Name, occurrenceStart, err)
	}

	if !hasSnapshot {
		// planSeriesBlockFirstTime (like planSeriesBlock/planFilterBlock
		// below it, and every content-planning method on Engine) predates
		// ctx-threading and uses context.Background() internally -- see
		// service/schedule.go's own //nolint:contextcheck on
		// GenerateForTimeRange/Commit for the same, repo-wide, "out of
		// scope for this task" note.
		return e.planSeriesBlockFirstTime(block, availablePrograms, occurrenceStart) //nolint:contextcheck
	}

	if occurrenceStart.After(now) {
		// Still in the future: re-derive from the fixed stored snapshot
		// plus whatever the block spec currently says, never touching the
		// global cursor chain.
		return e.planSeriesBlockFromSnapshot(block, availablePrograms, occurrenceStart, snapshot) //nolint:contextcheck // see above
	}

	// Aired: replay the frozen assignment verbatim, exactly like a filter
	// block's committed occurrence.
	committed, ok, err := e.store.GetCommittedOccurrence(ctx, block.Name, occurrenceStart)
	if err != nil {
		return nil, fmt.Errorf("failed to check committed occurrence for block %q at %s: %w", block.Name, occurrenceStart, err)
	}
	if !ok {
		// Has a snapshot but somehow no recorded assignment -- shouldn't
		// happen, since planSeriesBlockFirstTime/planSeriesBlockFromSnapshot
		// always write both together, but re-derive from the snapshot
		// rather than erroring outright if it ever does.
		return e.planSeriesBlockFromSnapshot(block, availablePrograms, occurrenceStart, snapshot) //nolint:contextcheck // see above
	}
	return resolveCommittedPrograms(committed, availablePrograms), nil
}

// planSeriesBlockFirstTime is the only path that ever really advances a
// series cursor: it captures every show block.Series references' current
// state as this occurrence's permanent snapshot (via the same chain
// e.getSeriesState always uses -- pendingStates first, so it correctly
// builds on an earlier occurrence of this same block already planned in
// this same Run, then falling back to the authoritative persisted store),
// then plans normally, which is what actually mutates pendingStates
// (committed for real by Engine.Commit) and pendingHistory.
func (e *Engine) planSeriesBlockFirstTime(block Block, availablePrograms []tunarr.Program, occurrenceStart time.Time) ([]tunarr.Program, error) {
	snapshotBefore, err := e.captureSeriesSnapshot(block)
	if err != nil {
		return nil, err
	}

	playlist := e.planSeriesBlockWithContext(engineSeriesContext{e}, block, availablePrograms)
	e.recordHistory(playlist, block.ChannelID, block.Name, time.Now(), occurrenceStart)
	e.pendingSnapshots = append(e.pendingSnapshots, occurrenceSnapshotRecord{
		blockName:       block.Name,
		occurrenceStart: occurrenceStart,
		snapshot:        snapshotBefore,
	})
	return playlist, nil
}

// planSeriesBlockFromSnapshot re-derives a not-yet-aired occurrence's
// content from its fixed, previously-captured snapshot and the CURRENT
// block spec -- via a scratch snapshotSeriesContext, so this never reads
// or mutates the global cursor chain (e.pendingStates/the authoritative
// store) at all. The result replaces this occurrence's stored assignment
// (pendingReplacements, flushed by Commit via
// StateStore.ReplaceOccurrenceHistory) rather than appending to it.
func (e *Engine) planSeriesBlockFromSnapshot(block Block, availablePrograms []tunarr.Program, occurrenceStart time.Time, snapshot map[string]SeriesStateSnapshot) ([]tunarr.Program, error) {
	scratch := newSnapshotSeriesContext(snapshot)
	playlist := e.planSeriesBlockWithContext(scratch, block, availablePrograms)

	entries := makeHistoryEntries(playlist, block.ChannelID, block.Name, time.Now(), occurrenceStart)
	e.pendingReplacements = append(e.pendingReplacements, occurrenceReplacement{
		blockName:       block.Name,
		occurrenceStart: occurrenceStart,
		entries:         entries,
	})
	return playlist, nil
}

// captureSeriesSnapshot reads the current cursor for every show
// block.Series references, via the same chain e.getSeriesState always
// uses, and clones each one into a value-only SeriesStateSnapshot so this
// occurrence's captured baseline is a genuinely independent copy,
// unaffected by whatever mutation the planning pass this feeds is about
// to perform on the live *SeriesState.
func (e *Engine) captureSeriesSnapshot(block Block) (map[string]SeriesStateSnapshot, error) {
	snapshot := make(map[string]SeriesStateSnapshot, len(block.Series))
	for _, sc := range block.Series {
		if _, ok := snapshot[sc.ShowTitle]; ok {
			continue
		}
		state, err := e.getSeriesState(sc.ShowTitle)
		if err != nil {
			return nil, fmt.Errorf("failed to snapshot series state for %q: %w", sc.ShowTitle, err)
		}
		snapshot[sc.ShowTitle] = SeriesStateSnapshot{
			CurrentSeason:  state.CurrentSeason,
			CurrentEpisode: state.CurrentEpisode,
			Completed:      state.Completed,
			Disabled:       state.Disabled,
			RunCount:       state.RunCount,
		}
	}
	return snapshot, nil
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
		fillerPrograms, err := e.getFiller(block, gapDuration)
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

func newSnapshotSeriesContext(snapshot map[string]SeriesStateSnapshot) *snapshotSeriesContext {
	states := make(map[string]*SeriesState, len(snapshot))
	for showTitle, s := range snapshot {
		states[showTitle] = &SeriesState{
			ShowTitle:      showTitle,
			CurrentSeason:  s.CurrentSeason,
			CurrentEpisode: s.CurrentEpisode,
			Completed:      s.Completed,
			Disabled:       s.Disabled,
			RunCount:       s.RunCount,
			// LastAired is deliberately left nil: it exists to make
			// initializeSeriesState a one-time-only start-position
			// override, and every snapshotSeriesContext's states already
			// reflect wherever the show's cursor legitimately stood, so
			// treating this as "first time seeing this show" (nil
			// LastAired) is correct here too.
		}
	}
	return &snapshotSeriesContext{states: states}
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
// planSeriesBlockFirstTime/planSeriesBlockFromSnapshot instead (see
// PlanBlock).
func (e *Engine) planSeriesBlock(block Block, availablePrograms []tunarr.Program, occurrenceStart time.Time) ([]tunarr.Program, error) {
	playlist := e.planSeriesBlockWithContext(engineSeriesContext{e}, block, availablePrograms)
	e.recordHistory(playlist, block.ChannelID, block.Name, time.Now(), occurrenceStart)
	return playlist, nil
}

// planSeriesBlockWithContext is the actual series-block planning
// algorithm, parameterized over where its per-show cursor state comes
// from and goes -- see seriesPlanContext's doc comment.
func (e *Engine) planSeriesBlockWithContext(ctx seriesPlanContext, block Block, availablePrograms []tunarr.Program) []tunarr.Program {
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

	playlist, currentDuration = e.applySeriesFallback(block, availablePrograms, playlist, currentDuration, targetDuration)
	playlist, _ = e.applyBlockFiller(block, playlist, currentDuration, targetDuration)

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

func (e *Engine) applySeriesFallback(block Block, availablePrograms []tunarr.Program, playlist []tunarr.Program, currentDuration, targetDuration int64) ([]tunarr.Program, int64) {
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

	// #nosec G404 - content shuffle for programming variety, not a security-sensitive value
	rand.Shuffle(len(candidates), func(i, j int) {
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

func (e *Engine) applyBlockFiller(block Block, playlist []tunarr.Program, currentDuration, targetDuration int64) ([]tunarr.Program, int64) {
	gapDuration := targetDuration - currentDuration
	gapMinutes := int(gapDuration / 60000)
	if !block.Filler.Enabled || gapMinutes < block.Filler.MinGapTime {
		return playlist, currentDuration
	}

	fillerPrograms, err := e.getFiller(block, gapDuration)
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

// getFiller retrieves filler content to fill the remaining time
func (e *Engine) getFiller(block Block, remainingDuration int64) ([]tunarr.Program, error) {
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
	// #nosec G404 - content shuffle for programming variety, not a security-sensitive value
	rand.Shuffle(len(fillerContent), func(i, j int) {
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
