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
	"time"

	"github.com/christopherime/schedularr/internal/cache"
	"github.com/christopherime/schedularr/internal/external/tunarr"
	"github.com/christopherime/schedularr/internal/scheduler"
	"github.com/christopherime/schedularr/internal/store"
)

// Options configures a single Run. Days is the number of 24h days ahead of
// now to generate a schedule for; callers (the API's handlers, cmd/generate.go)
// are responsible for validating it (the API restricts it to 1..30) --
// Run does not re-validate it. ChannelID, when non-empty, narrows both the
// returned result and (when Apply is set) which channels get pushed to
// Tunarr, to that one channel. Apply gates whether Run mutates anything:
// false is a pure dry-run (no UpdateSchedule calls, no Engine.Commit()).
type Options struct {
	Days      int
	ChannelID string
	Apply     bool
}

// Result is what Run produces: the generated (and, if Applied, pushed)
// schedule, keyed by Tunarr channel ID.
type Result struct {
	Applied  bool
	Channels map[string][]scheduler.ScheduledSlot
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

// contentCacheDuration mirrors cmd/schema/config.cue's cache.cache_duration
// default ("1h"). The CLI sourced this from *config.Config, but NewRunner's
// signature has no config parameter -- a Runner now lives for the lifetime
// of an API server process and serves many Run calls, rather than the
// CLI's one-shot-per-process use, so a fixed Runner-scoped duration
// replaces the config-driven one.
const contentCacheDuration = time.Hour

// Runner executes the schedule-generation workflow against a store and a
// Tunarr client held for its lifetime.
type Runner struct {
	store  *store.Store
	tunarr *tunarr.Client
	logger *slog.Logger
	loc    *time.Location
	cache  *cache.Cache // nil if cache.New failed; fetch then always hits Tunarr
}

var _ ScheduleRunner = (*Runner)(nil)

// NewRunner builds a Runner backed by st and tc. l and loc default to
// slog.Default() and time.Local respectively when nil, matching
// scheduler.NewEngine's own nil-handling.
func NewRunner(st *store.Store, tc *tunarr.Client, l *slog.Logger, loc *time.Location) *Runner {
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

	return &Runner{store: st, tunarr: tc, logger: l, loc: loc, cache: c}
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
		blocks = append(blocks, rec.Spec)
	}
	return blocks, nil
}

// Run executes one generate-(and-maybe-apply) cycle: load the active
// blocks, fetch available Tunarr content, run the scheduling engine over
// [now, now+o.Days days), narrow the result to o.ChannelID when set, and --
// only when o.Apply is set -- push the (possibly narrowed) result to
// Tunarr per channel and commit the engine's pending state. A dry run
// (o.Apply == false) never calls UpdateSchedule or Commit, so it cannot
// mutate the store or Tunarr.
func (r *Runner) Run(ctx context.Context, o Options) (*Result, error) {
	blocks, err := ActiveBlocks(ctx, r.store)
	if err != nil {
		return nil, fmt.Errorf("failed to load scheduling blocks: %w", err)
	}

	programs, err := r.fetchPrograms(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch programs: %w", err)
	}

	engine := scheduler.NewEngine(r.tunarr, blocks, r.store, r.logger, r.loc)

	start := time.Now()
	end := start.Add(time.Duration(o.Days) * 24 * time.Hour)

	// scheduler.Engine's exported methods predate this ctx-threaded
	// service and don't take a context.Context (they use
	// context.Background() internally, e.g. in getSeriesState/Commit) --
	// out of scope for this task's extraction (see the package doc
	// comment: engine logic itself is unchanged), so ctx can't actually be
	// propagated any further than here.
	plan, err := engine.GenerateForTimeRange(start, end, programs) //nolint:contextcheck
	if err != nil {
		return nil, fmt.Errorf("failed to generate schedule: %w", err)
	}

	channels := narrowToChannel(plan, o.ChannelID)

	if o.Apply {
		if err := r.applyChannels(ctx, channels); err != nil {
			return nil, err
		}
		if err := engine.Commit(); err != nil { //nolint:contextcheck // see GenerateForTimeRange above
			return nil, fmt.Errorf("failed to commit schedule state: %w", err)
		}
	}

	return &Result{Applied: o.Apply, Channels: channels}, nil
}

// narrowToChannel returns plan unchanged when channelID is empty, or a map
// containing only that channel's slots (empty if the channel has none)
// otherwise. Run applies this before the optional apply step, so a
// channel-scoped request (POST /apply with channel_id set) only ever
// touches that one channel's Tunarr schedule -- it never pushes to
// channels the caller didn't ask about, and the returned Result.Channels
// always matches exactly what Apply (if set) pushed.
func narrowToChannel(plan map[string][]scheduler.ScheduledSlot, channelID string) map[string][]scheduler.ScheduledSlot {
	if channelID == "" {
		return plan
	}
	narrowed := make(map[string][]scheduler.ScheduledSlot)
	if slots, ok := plan[channelID]; ok {
		narrowed[channelID] = slots
	}
	return narrowed
}

// applyChannels pushes each channel's flattened program list to Tunarr via
// UpdateSchedule, best-effort across channels (mirroring cmd/generate.go's
// former applySchedule): every channel is attempted even if an earlier one
// failed. If any channel failed, it returns an aggregate error and Run
// skips Engine.Commit() -- state is only committed once every channel in
// the (possibly narrowed) result was successfully pushed.
func (r *Runner) applyChannels(ctx context.Context, channels map[string][]scheduler.ScheduledSlot) error {
	failCount := 0
	for channelID, slots := range channels {
		if err := r.tunarr.UpdateSchedule(ctx, channelID, flattenSlots(slots)); err != nil {
			r.logger.Error("failed to apply schedule to channel",
				"channel_id", channelID,
				"error", err,
			)
			failCount++
			continue
		}
		r.logger.Info("applied schedule to channel", "channel_id", channelID)
	}
	if failCount > 0 {
		return fmt.Errorf("failed to apply schedule to %d channel(s)", failCount)
	}
	return nil
}

// flattenSlots concatenates every slot's programs into a single playlist,
// in slot (chronological) order -- the shape client.UpdateSchedule expects.
func flattenSlots(slots []scheduler.ScheduledSlot) []tunarr.Program {
	var programs []tunarr.Program
	for _, slot := range slots {
		programs = append(programs, slot.Programs...)
	}
	return programs
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

		// Check if we've fetched all programs
		if len(resp.Results) < limit || len(allPrograms) >= resp.Total {
			break
		}
		page++
	}

	r.logger.Debug("fetched library programs", "library", lib.Name, "count", len(allPrograms))
	return allPrograms
}

func (r *Runner) fetchAllProgramsViaSearch(ctx context.Context) ([]tunarr.Program, error) {
	var allPrograms []tunarr.Program
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

		// Check if we've fetched all programs
		if len(resp.Results) < limit || len(allPrograms) >= resp.Total {
			break
		}
		page++
	}

	return allPrograms, nil
}

func (r *Runner) saveToCache(programs []tunarr.Program) {
	if r.cache == nil {
		return
	}
	if err := r.cache.Set(tunarrCacheKey, programs); err != nil {
		r.logger.Warn("failed to write tunarr programs to cache", "error", err)
	}
}
