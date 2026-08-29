package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/christopherime/schedularr/internal/external/tunarr"
	"github.com/christopherime/schedularr/internal/scheduler"
	"github.com/christopherime/schedularr/internal/store"
)

// discardLogger is a slog.Logger that throws every record away, keeping
// test output clean while still exercising every r.logger.* call site.
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// fakeTunarr is an httptest-backed stand-in for the Tunarr API, covering
// the endpoints Runner.Run's fetch/apply path actually calls. It always
// reports zero media sources so fetchLibraryPrograms comes back empty and
// fetchPrograms falls through to the SearchPrograms-only fallback -- the
// simplest path to exercise deterministically, mirroring the fixtures in
// internal/external/tunarr/client_test.go (TestClient_SearchPrograms,
// TestClient_UpdateSchedule).
type fakeTunarr struct {
	programs []tunarr.Program

	mu      sync.Mutex
	updates map[string][]tunarr.Program // channelID -> programs pushed via UpdateSchedule
}

func newFakeTunarr(t *testing.T, programs []tunarr.Program) (*httptest.Server, *fakeTunarr) {
	t.Helper()
	f := &fakeTunarr{programs: programs, updates: make(map[string][]tunarr.Program)}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/media-sources", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]tunarr.MediaSource{})
	})
	mux.HandleFunc("/api/channels", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]tunarr.Channel{})
	})
	mux.HandleFunc("/api/programs/search", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Returns every program in one page regardless of the requested
		// page/limit -- so TotalPages is always 1, matching the live
		// envelope's actual semantics (no legacy "total"/"limit" keys; see
		// tunarr.ProgramSearchResponse) closely enough for tests that don't
		// care about pagination itself. See newPaginatedFakeTunarr below for
		// a fake that actually paginates, used to pin the fetch-truncation
		// fix.
		_ = json.NewEncoder(w).Encode(tunarr.ProgramSearchResponse{
			Results:    f.programs,
			Page:       1,
			TotalPages: 1,
			TotalHits:  len(f.programs),
		})
	})
	mux.HandleFunc("/api/channels/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		// path is /api/channels/{id}/programming
		id := r.URL.Path[len("/api/channels/") : len(r.URL.Path)-len("/programming")]

		var programs []tunarr.Program
		require.NoError(t, json.NewDecoder(r.Body).Decode(&programs))

		f.mu.Lock()
		f.updates[id] = programs
		f.mu.Unlock()

		w.WriteHeader(http.StatusNoContent)
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server, f
}

func (f *fakeTunarr) updatedChannels() map[string][]tunarr.Program {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make(map[string][]tunarr.Program, len(f.updates))
	for k, v := range f.updates {
		out[k] = v
	}
	return out
}

// newTestRunner builds a Runner against a fresh temp-dir store and the
// given fake Tunarr server, seeding one enabled filter block on channel-1.
// The block's cron ("0 * * * *", hourly) guarantees at least one occurrence
// in any 24h window regardless of the current wall-clock time, without the
// per-minute blowup a "* * * * *" cron would cause (each occurrence plans
// against the store, including a history lookup per candidate program).
func newTestRunner(t *testing.T, tunarrURL string) (*Runner, *store.Store) {
	t.Helper()

	st, err := store.New(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = st.Close() })

	ctx := context.Background()
	require.NoError(t, st.CreateBlock(ctx, &store.BlockRecord{
		ID:      "block-1",
		Name:    "Hourly Movies",
		Enabled: true,
		Spec: scheduler.Block{
			Name:      "Hourly Movies",
			Cron:      "0 * * * *",
			Duration:  60,
			ChannelID: "channel-1",
		},
	}))
	// A disabled block on a different channel must never surface in Run's
	// output -- ActiveBlocks (and, before it, loadActiveBlocks) excludes it.
	require.NoError(t, st.CreateBlock(ctx, &store.BlockRecord{
		ID:      "block-2",
		Name:    "Disabled Block",
		Enabled: false,
		Spec: scheduler.Block{
			Name:      "Disabled Block",
			Cron:      "0 * * * *",
			Duration:  30,
			ChannelID: "channel-2",
		},
	}))

	client := tunarr.NewClient(tunarr.Config{URL: tunarrURL})
	r := NewRunner(st, client, discardLogger(), time.UTC, 0)
	return r, st
}

func canonicalPrograms() []tunarr.Program {
	return []tunarr.Program{
		{ID: "prog-1", Title: "Movie One", Duration: 1_800_000, Type: "movie"},
		{ID: "prog-2", Title: "Movie Two", Duration: 1_800_000, Type: "movie"},
	}
}

// TestRunner_Run_DryRun_ReturnsSlotsWithoutMutating is the characterization
// test pinning Run's dry-run behavior as ported from cmd/generate.go's
// ProcessSchedule/generateSchedulePlan: an enabled block produces slots,
// Applied is false, and -- because dry-run never calls Engine.Commit() --
// the store's schedule history stays empty.
func TestRunner_Run_DryRun_ReturnsSlotsWithoutMutating(t *testing.T) {
	server, _ := newFakeTunarr(t, canonicalPrograms())
	r, st := newTestRunner(t, server.URL)

	result, err := r.Run(context.Background(), Options{Days: 1, Apply: false})
	require.NoError(t, err)

	require.NotNil(t, result)
	assert.False(t, result.Applied)

	slots, ok := result.Channels["channel-1"]
	require.True(t, ok, "expected slots for channel-1, got %+v", result.Channels)
	require.NotEmpty(t, slots, "expected at least one scheduled slot for the enabled hourly block")
	for _, slot := range slots {
		assert.NotEmpty(t, slot.Programs, "each slot should carry the programs FilterPrograms matched")
	}

	// The disabled block's channel must never appear.
	_, hasDisabledChannel := result.Channels["channel-2"]
	assert.False(t, hasDisabledChannel, "disabled block's channel must not be scheduled")

	// Dry-run must not touch the store: no schedule history recorded.
	history, err := st.ListScheduleHistory(context.Background(), time.Time{})
	require.NoError(t, err)
	assert.Empty(t, history, "dry-run must not call Engine.Commit(), so no history should be persisted")
}

// TestRunner_Run_Apply_PushesToTunarrAndCommits exercises the mutating
// path: Apply:true must call UpdateSchedule for the scheduled channel and
// then Engine.Commit(), which persists the schedule history the dry-run
// test above proved stays empty otherwise.
func TestRunner_Run_Apply_PushesToTunarrAndCommits(t *testing.T) {
	server, fake := newFakeTunarr(t, canonicalPrograms())
	r, st := newTestRunner(t, server.URL)

	result, err := r.Run(context.Background(), Options{Days: 1, Apply: true})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Applied)

	updates := fake.updatedChannels()
	require.Contains(t, updates, "channel-1", "UpdateSchedule must have been called for channel-1")
	assert.NotEmpty(t, updates["channel-1"])

	history, err := st.ListScheduleHistory(context.Background(), time.Time{})
	require.NoError(t, err)
	assert.NotEmpty(t, history, "Engine.Commit() should have persisted schedule history on apply")
}

// TestRunner_Run_ChannelIDNarrowsResultAndApply verifies the extraction's
// resolution of an ambiguity the task brief left open ("ChannelID filters
// the result map when set" without specifying whether that happens before
// or after the apply push): scoping happens to the *blocks*, before
// planning (blocksForChannel, called from Run), so a channel-scoped apply
// request only ever pushes to that channel, never to others. See
// TestRunner_Run_ChannelScopedApply_LeavesOtherChannelStateUntouched below
// for the deeper regression test proving planning itself -- not just the
// returned/pushed result -- stays scoped.
func TestRunner_Run_ChannelIDNarrowsResultAndApply(t *testing.T) {
	server, fake := newFakeTunarr(t, canonicalPrograms())
	st, err := store.New(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = st.Close() })

	ctx := context.Background()
	require.NoError(t, st.CreateBlock(ctx, &store.BlockRecord{
		ID: "block-1", Name: "Channel One Block", Enabled: true,
		Spec: scheduler.Block{Name: "Channel One Block", Cron: "0 * * * *", Duration: 60, ChannelID: "channel-1"},
	}))
	require.NoError(t, st.CreateBlock(ctx, &store.BlockRecord{
		ID: "block-2", Name: "Channel Two Block", Enabled: true,
		Spec: scheduler.Block{Name: "Channel Two Block", Cron: "0 * * * *", Duration: 60, ChannelID: "channel-2"},
	}))

	client := tunarr.NewClient(tunarr.Config{URL: server.URL})
	r := NewRunner(st, client, discardLogger(), time.UTC, 0)

	result, err := r.Run(ctx, Options{Days: 1, Apply: true, ChannelID: "channel-1"})
	require.NoError(t, err)

	assert.Len(t, result.Channels, 1, "result should be narrowed to the requested channel only")
	assert.Contains(t, result.Channels, "channel-1")
	assert.NotContains(t, result.Channels, "channel-2")

	updates := fake.updatedChannels()
	assert.Contains(t, updates, "channel-1")
	assert.NotContains(t, updates, "channel-2", "apply must not push to a channel outside the requested scope")
}

// TestBlocksForChannel proves the mechanism Run relies on to keep planning
// itself scoped to o.ChannelID (see Run's doc comment for why narrowing
// only the *result* isn't enough: scheduler.Engine mutates pending state
// for every block it's given to plan, regardless of channel). Exercised
// directly at the Go-value level -- not through the HTTP-mediated
// fetch/plan pipeline the other tests in this file use -- so a series
// block's ShowTitle can be asserted on freely; see
// TestRunner_Run_ChannelScopedApply_LeavesOtherChannelStateUntouched below
// for why that isn't possible end-to-end through Run.
func TestBlocksForChannel(t *testing.T) {
	blocks := []scheduler.Block{
		{Name: "Channel One Filter", ChannelID: "channel-1"},
		{
			Name: "Channel Two Series", ChannelID: "channel-2",
			Type:   scheduler.BlockTypeSeries,
			Series: []scheduler.SeriesConfig{{ShowTitle: "Some Show", EpisodesPerBlock: 1}},
		},
		{Name: "Channel One Second Block", ChannelID: "channel-1"},
	}

	got := blocksForChannel(blocks, "channel-1")

	require.Len(t, got, 2)
	assert.Equal(t, "Channel One Filter", got[0].Name)
	assert.Equal(t, "Channel One Second Block", got[1].Name)
	for _, b := range got {
		assert.Equal(t, "channel-1", b.ChannelID)
	}

	assert.Empty(t, blocksForChannel(blocks, "channel-3"), "a channel with no matching block returns empty, not nil-panicking or all blocks")
}

// programsForChannelScopeTest returns canonicalPrograms() plus one
// "episode" program that
// TestRunner_Run_ChannelScopedApply_LeavesOtherChannelStateUntouched uses
// to make channel-2's series block actually match and advance state when
// planned -- so that test can prove state advancement is *suppressed* by
// channel scoping, rather than merely observing an always-empty result
// that would pass whether or not the scoping fix actually works. See that
// test's doc comment for why the series block's ShowTitle is deliberately
// "".
func programsForChannelScopeTest() []tunarr.Program {
	return append(canonicalPrograms(), tunarr.Program{
		ID: "ep-1", Title: "Pilot", Type: "episode",
		SeasonNumber: 1, EpisodeNumber: 1, Duration: 1_800_000,
	})
}

// TestRunner_Run_ChannelScopedApply_LeavesOtherChannelStateUntouched is
// the end-to-end regression test for the bug this package's Run fixes:
// before the fix, Run planned every enabled block regardless of
// o.ChannelID and only narrowed the *returned* map afterward, so
// Engine.Commit() on a channel-scoped apply still persisted series-cursor
// advances and "aired" history for every other channel's blocks too, even
// though nothing was pushed to Tunarr for them.
//
// The fixture: channel-1 has a filter block (applied); channel-2 has a
// series block that is never applied. The series block's SeriesConfig
// ShowTitle is deliberately "" -- matching a canned "episode" program
// whose ShowTitle is also "" after the HTTP round trip, since that
// program (programsForChannelScopeTest) sets neither a flat ShowTitle nor
// a nested Show, so tunarr.Client's hydrateEpisodeShowFields has nothing
// to hydrate from (see tunarr.Program.ShowTitle's doc comment in
// models.go for the flat/nested/hydration contract this now follows).
// Using "" on both sides is what makes findEpisode's match succeed, which
// is what makes this a *discriminating* regression test: without the
// blocksForChannel pre-filter in Run, channel-2's series state would
// visibly start advancing (and get committed) here; with it, this test
// fails loudly if that guarantee ever regresses, rather than passing
// vacuously regardless of whether scoping works.
func TestRunner_Run_ChannelScopedApply_LeavesOtherChannelStateUntouched(t *testing.T) {
	server, fake := newFakeTunarr(t, programsForChannelScopeTest())
	st, err := store.New(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = st.Close() })

	ctx := context.Background()
	require.NoError(t, st.CreateBlock(ctx, &store.BlockRecord{
		ID: "block-1", Name: "Channel One Filter", Enabled: true,
		Spec: scheduler.Block{
			Name: "Channel One Filter", Cron: "0 * * * *", Duration: 60, ChannelID: "channel-1",
		},
	}))
	require.NoError(t, st.CreateBlock(ctx, &store.BlockRecord{
		ID: "block-2", Name: "Channel Two Series", Enabled: true,
		Spec: scheduler.Block{
			Name: "Channel Two Series", Cron: "0 * * * *", Duration: 30, ChannelID: "channel-2",
			Type:   scheduler.BlockTypeSeries,
			Series: []scheduler.SeriesConfig{{ShowTitle: "", EpisodesPerBlock: 1}},
		},
	}))

	client := tunarr.NewClient(tunarr.Config{URL: server.URL})
	r := NewRunner(st, client, discardLogger(), time.UTC, 0)

	result, err := r.Run(ctx, Options{Days: 1, Apply: true, ChannelID: "channel-1"})
	require.NoError(t, err)
	assert.True(t, result.Applied)
	assert.Len(t, result.Channels, 1, "result must only ever contain the requested channel")
	assert.Contains(t, result.Channels, "channel-1")

	updates := fake.updatedChannels()
	assert.Contains(t, updates, "channel-1")
	assert.NotContains(t, updates, "channel-2", "apply must never push to a channel outside the requested scope")

	// Store-side purity, part 1: schedule history was recorded only for
	// the applied channel.
	history, err := st.ListScheduleHistory(ctx, time.Time{})
	require.NoError(t, err)
	require.NotEmpty(t, history, "channel-1's filter block should have recorded history on apply")
	for _, entry := range history {
		assert.Equal(t, "channel-1", entry.ChannelID,
			"no history entry should exist for the untouched channel-2 block")
	}

	// Store-side purity, part 2: channel-2's series block was never
	// planned at all, so no series_state row was ever created for it --
	// not "created but left unchanged", genuinely never written.
	_, err = st.GetPersistedSeriesState(ctx, "")
	assert.ErrorIs(t, err, store.ErrNotFound,
		"channel-2's series block must never be planned, let alone committed, by a channel-1-scoped apply")
}

// TestActiveBlocks_ExcludesDisabled verifies that ActiveBlocks (moved from
// cmd/generate.go's loadActiveBlocks) returns only the Specs of enabled
// block records -- disabled blocks are the mechanism for keeping a block
// defined but out of schedule generation, so the engine must never see one.
func TestActiveBlocks_ExcludesDisabled(t *testing.T) {
	st, err := store.New(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = st.Close() })

	ctx := context.Background()

	enabled := &store.BlockRecord{
		ID:      "enabled-1",
		Name:    "Enabled Block",
		Enabled: true,
		Spec: scheduler.Block{
			Name:      "Enabled Block",
			Cron:      "0 6 * * *",
			Duration:  60,
			ChannelID: "channel-1",
		},
	}
	disabled := &store.BlockRecord{
		ID:      "disabled-1",
		Name:    "Disabled Block",
		Enabled: false,
		Spec: scheduler.Block{
			Name:      "Disabled Block",
			Cron:      "0 7 * * *",
			Duration:  30,
			ChannelID: "channel-2",
		},
	}

	require.NoError(t, st.CreateBlock(ctx, enabled))
	require.NoError(t, st.CreateBlock(ctx, disabled))

	blocks, err := ActiveBlocks(ctx, st)
	require.NoError(t, err)

	require.Len(t, blocks, 1)
	assert.Equal(t, "Enabled Block", blocks[0].Name)
}

// TestRunner_Run_Apply_UsesConfiguredHistoryWindowForCleanup pins the
// history-retention plumbing fix: NewRunner's historyWindow parameter
// reaches scheduler.Engine.Commit()'s CleanupScheduleHistory call (via
// scheduler.NewEngineWithOptions/EngineOptions.HistoryWindow), rather than
// the engine's own hardcoded 7-day default that Commit used before this
// fix regardless of what config.MaintenanceHistoryRetention said.
//
// The fixture seeds two schedule_history rows directly (bypassing the
// scheduling engine): one 10 days old and one 40 days old. A Runner built
// with a 30-day (720h) historyWindow -- wider than the engine's old
// hardcoded 168h/7-day default -- must, on Apply's Engine.Commit(), keep
// the 10-day-old row (it falls inside the configured 30-day window; the
// old hardcoded default would have pruned it, since 10 days > 7 days) and
// still prune the 40-day-old row (it falls outside even the configured
// window, proving cleanup itself still works and this isn't just "nothing
// ever gets pruned now").
func TestRunner_Run_Apply_UsesConfiguredHistoryWindowForCleanup(t *testing.T) {
	server, _ := newFakeTunarr(t, canonicalPrograms())
	st, err := store.New(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = st.Close() })

	ctx := context.Background()
	require.NoError(t, st.CreateBlock(ctx, &store.BlockRecord{
		ID: "block-1", Name: "Hourly Movies", Enabled: true,
		Spec: scheduler.Block{Name: "Hourly Movies", Cron: "0 * * * *", Duration: 60, ChannelID: "channel-1"},
	}))

	now := time.Now()
	require.NoError(t, st.RecordScheduleHistory(ctx, []scheduler.ScheduleHistoryEntry{
		{ProgramID: "kept-10d", ChannelID: "channel-1", BlockName: "seed", ScheduledAt: now.Add(-10 * 24 * time.Hour)},
		{ProgramID: "pruned-40d", ChannelID: "channel-1", BlockName: "seed", ScheduledAt: now.Add(-40 * 24 * time.Hour)},
	}))

	client := tunarr.NewClient(tunarr.Config{URL: server.URL})
	const thirtyDays = 30 * 24 * time.Hour
	r := NewRunner(st, client, discardLogger(), time.UTC, thirtyDays)

	_, err = r.Run(ctx, Options{Days: 1, Apply: true})
	require.NoError(t, err)

	history, err := st.ListScheduleHistory(ctx, time.Time{})
	require.NoError(t, err)

	ids := make([]string, 0, len(history))
	for _, entry := range history {
		ids = append(ids, entry.ProgramID)
	}
	assert.Contains(t, ids, "kept-10d",
		"a 10-day-old entry must survive Commit's cleanup when history_retention is configured to 30 days -- the engine's old hardcoded 7-day default would have pruned it")
	assert.NotContains(t, ids, "pruned-40d",
		"a 40-day-old entry must still be pruned even under the wider configured 30-day window")
}

// makeMovies returns n distinct "movie" programs, used by the pagination
// regression tests below where the content doesn't matter, only the
// count.
func makeMovies(n int) []tunarr.Program {
	programs := make([]tunarr.Program, n)
	for i := range programs {
		programs[i] = tunarr.Program{
			ID:       fmt.Sprintf("prog-%03d", i),
			Title:    fmt.Sprintf("Movie %03d", i),
			Duration: 1_800_000,
			Type:     "movie",
		}
	}
	return programs
}

// paginatedSearchHandler returns an /api/programs/search handler that
// actually respects the requested page/limit against all, reporting
// TotalHits/TotalPages against the live envelope shape (see
// tunarr.ProgramSearchResponse's doc comment) -- unlike
// fakeTunarr/fakeLibraryTunarr's handlers, which always return every
// program in a single TotalPages: 1 response regardless of what was
// requested. Shared by the two pagination-truncation regression tests
// below: one exercising fetchAllProgramsViaSearch (the
// zero-media-source fallback fetchPrograms takes), one exercising
// fetchSingleLibrary/fetchLibraryPrograms (the primary, library-scoped
// path Run/MediaShows/MediaMeta actually use day to day).
func paginatedSearchHandler(t *testing.T, all []tunarr.Program) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		var req tunarr.ProgramSearchRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))

		page := req.Page
		if page < 1 {
			page = 1
		}
		limit := req.Limit
		if limit <= 0 {
			limit = len(all)
		}

		start := (page - 1) * limit
		if start > len(all) {
			start = len(all)
		}
		end := start + limit
		if end > len(all) {
			end = len(all)
		}

		totalPages := (len(all) + limit - 1) / limit
		if totalPages == 0 {
			totalPages = 1
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(tunarr.ProgramSearchResponse{
			Results:    all[start:end],
			Page:       page,
			TotalPages: totalPages,
			TotalHits:  len(all),
		})
	}
}

// newPaginatedFakeTunarr reports zero media sources (like fakeTunarr) so
// fetchPrograms falls through to fetchAllProgramsViaSearch, but its
// /api/programs/search handler actually paginates -- see
// paginatedSearchHandler.
func newPaginatedFakeTunarr(t *testing.T, all []tunarr.Program) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/media-sources", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]tunarr.MediaSource{})
	})
	mux.HandleFunc("/api/programs/search", paginatedSearchHandler(t, all))

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

// newPaginatedFakeLibraryTunarr reports one media source with one library
// (like fakeLibraryTunarr in media_test.go) so fetchLibraryPrograms's
// fetchSingleLibrary is what actually pages through all of `all`, rather
// than the SearchPrograms-only fallback newPaginatedFakeTunarr exercises.
func newPaginatedFakeLibraryTunarr(t *testing.T, all []tunarr.Program) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/media-sources", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]tunarr.MediaSource{{ID: "src-1", Name: "Plex", Type: "plex"}})
	})
	mux.HandleFunc("/api/media-sources/src-1/libraries", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]tunarr.Library{{ID: "lib-1", Name: "Movies", MediaType: "movies"}})
	})
	mux.HandleFunc("/api/programs/search", paginatedSearchHandler(t, all))

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

// TestRunner_fetchAllProgramsViaSearch_FetchesEveryPage is the regression
// test for the pagination-truncation bug: a fake Tunarr library of 250
// programs, paginated 100 per page (3 pages: 100+100+50, totalHits: 250),
// must yield all 250 programs -- not just the first page's 100.
//
// Before the fix, tunarr.ProgramSearchResponse modeled a "total"/"limit"
// pair no live response actually sends, so resp.Total silently
// deserialized to its zero value every time. fetchAllProgramsViaSearch's
// old loop condition, `len(resp.Results) < limit || len(allPrograms) >=
// resp.Total`, breaks on that second clause the moment len(allPrograms)
// (positive, after page 1) is compared >= 0 -- which is always true -- so
// the loop stopped after exactly one page regardless of how many programs
// actually matched. This test's 250-program, 3-page fixture would have
// returned only the first 100 programs under that bug.
func TestRunner_fetchAllProgramsViaSearch_FetchesEveryPage(t *testing.T) {
	all := makeMovies(250)
	server := newPaginatedFakeTunarr(t, all)

	st, err := store.New(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = st.Close() })

	client := tunarr.NewClient(tunarr.Config{URL: server.URL})
	r := NewRunner(st, client, discardLogger(), time.UTC, 0)

	got, err := r.fetchAllProgramsViaSearch(context.Background())
	require.NoError(t, err)
	assert.Len(t, got, 250,
		"must fetch every page (100+100+50, totalHits: 250), not stop after the first page's 100")
}

// TestRunner_fetchLibraryPrograms_FetchesEveryPage mirrors
// TestRunner_fetchAllProgramsViaSearch_FetchesEveryPage above, but against
// fetchSingleLibrary/fetchLibraryPrograms -- the primary, library-scoped
// fetch path Run/MediaShows/MediaMeta take when Tunarr reports at least
// one media source and library, which fetchAllProgramsViaSearch's
// zero-media-source fallback never exercises. Same bug, same fix, same
// 250-programs-across-3-pages fixture.
func TestRunner_fetchLibraryPrograms_FetchesEveryPage(t *testing.T) {
	all := makeMovies(250)
	server := newPaginatedFakeLibraryTunarr(t, all)

	st, err := store.New(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = st.Close() })

	client := tunarr.NewClient(tunarr.Config{URL: server.URL})
	r := NewRunner(st, client, discardLogger(), time.UTC, 0)

	got := r.fetchTunarrContent(context.Background())
	assert.Len(t, got, 250,
		"the primary library-scoped fetch path must also paginate through every page, not just SearchPrograms' fallback")
}

// TestRunner_Run_SeriesBlock_MatchesEpisodeFromNestedShowObject is the
// end-to-end regression test for the show-hydration bug: a series block
// whose SeriesConfig.ShowTitle is "The Office" must match an episode whose
// show identity only ever arrives nested under Program.Show -- exactly
// what a live Tunarr /api/programs/search response looks like (see
// tunarr.Program.ShowTitle's doc comment in models.go) -- with no flat
// "showTitle" key anywhere in the wire response. Before
// hydrateEpisodeShowFields (internal/external/tunarr/client.go) existed,
// this episode would decode with an empty ShowTitle and
// scheduler.Engine's findEpisode would never match it against the block's
// SeriesConfig, silently starving the series block every run. This is the
// test that proves series scheduling (e.g. a Saturday-night block) works
// against a live-shaped Tunarr response, not just this package's own
// flat-shaped fixtures.
func TestRunner_Run_SeriesBlock_MatchesEpisodeFromNestedShowObject(t *testing.T) {
	episode := tunarr.Program{
		ID: "ep-1", Title: "Pilot", Type: "episode",
		SeasonNumber: 1, EpisodeNumber: 1, Duration: 1_320_000,
		// No flat ShowTitle/Rating -- exactly what a live Tunarr response
		// sends: show identity only ever arrives nested.
		Show: &tunarr.Show{
			UUID:   "44444444-4444-4444-4444-444444444444",
			Title:  "The Office",
			Rating: "TV-14",
		},
	}
	server, _ := newFakeTunarr(t, []tunarr.Program{episode})

	st, err := store.New(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = st.Close() })

	ctx := context.Background()
	require.NoError(t, st.CreateBlock(ctx, &store.BlockRecord{
		ID: "block-1", Name: "Evening Sitcoms", Enabled: true,
		Spec: scheduler.Block{
			Name: "Evening Sitcoms", Cron: "0 * * * *", Duration: 30, ChannelID: "channel-1",
			Type:   scheduler.BlockTypeSeries,
			Series: []scheduler.SeriesConfig{{ShowTitle: "The Office", EpisodesPerBlock: 1}},
		},
	}))

	client := tunarr.NewClient(tunarr.Config{URL: server.URL})
	r := NewRunner(st, client, discardLogger(), time.UTC, 0)

	result, err := r.Run(ctx, Options{Days: 1, Apply: false})
	require.NoError(t, err)

	slots, ok := result.Channels["channel-1"]
	require.True(t, ok, "expected a scheduled slot for channel-1's series block")
	require.NotEmpty(t, slots)

	var matchedPilot bool
	for _, slot := range slots {
		for _, p := range slot.Programs {
			if p.Title == "Pilot" && p.ShowTitle == "The Office" {
				matchedPilot = true
			}
		}
	}
	assert.True(t, matchedPilot,
		`series block with SeriesConfig.ShowTitle == "The Office" must match the episode whose show identity only ever arrived nested under Program.Show`)
}
