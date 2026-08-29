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

	return &Runner{store: st, tunarr: tc, logger: l, loc: loc, cache: c, historyWindow: historyWindow}
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

	engine := scheduler.NewEngineWithOptions(r.tunarr, blocks, r.store, scheduler.EngineOptions{
		Logger:        r.logger,
		Location:      r.loc,
		HistoryWindow: r.historyWindow,
	})

	start := time.Now()
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
	channels, err := engine.GenerateForTimeRange(start, end, programs) //nolint:contextcheck
	if err != nil {
		return nil, fmt.Errorf("failed to generate schedule: %w", err)
	}

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

// applyChannels pushes each channel's flattened program list to Tunarr via
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

		// Stop once every page has been fetched. resp.TotalPages is the
		// live envelope's authoritative page count (POST
		// /api/programs/search returns {results, page, totalPages,
		// totalHits, facetDistribution} -- there is no "total"/"limit"
		// key; see tunarr.ProgramSearchResponse). page == resp.TotalPages
		// means the page just fetched was the last one. The
		// len(resp.Results) == 0 fallback guards against an unexpected
		// TotalPages of 0 (or a server that never reports it) causing an
		// infinite loop.
		if len(resp.Results) == 0 || page >= resp.TotalPages {
			break
		}
		page++
	}

	r.logger.Debug("fetched library programs", "library", lib.Name, "count", len(allPrograms))
	return r.hydrateShowsAndSeasons(ctx, allPrograms)
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

		// See the matching comment in fetchSingleLibrary above: resp.Total
		// never existed on a live response, so this used to stop after the
		// first page for any library/search result over `limit` programs.
		if len(resp.Results) == 0 || page >= resp.TotalPages {
			break
		}
		page++
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
//   - No equivalent "season"-type entry is ever interleaved the way "show"
//     entries are, so season numbers can't be joined locally at all --
//     hydrateSeasonNumbers resolves each distinct SeasonID individually via
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
// SeasonNumber by resolving its SeasonID through GET
// /api/programming/seasons/{id} (see hydrateShowsAndSeasons's doc comment
// for why this can't be a local join like show data can). Each distinct
// SeasonID is resolved at most once per cache window -- resolveSeasonNumber
// caches the result in Runner's existing content cache, so repeat fetches
// (and repeat episodes of the same season within one fetch) cost at most
// one Tunarr request per season, not one per episode. A season that fails
// to resolve (network error, deleted season, unexpected zero index) is
// logged and skipped: that episode's SeasonNumber simply stays 0 (its
// existing "unknown" value, matching the field's omitempty contract)
// rather than failing the whole fetch over one bad season lookup.
func (r *Runner) hydrateSeasonNumbers(ctx context.Context, programs []tunarr.Program) {
	needed := make(map[string][]int) // SeasonID -> indexes into programs needing it resolved
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
