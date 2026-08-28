package service

import (
	"context"
	"encoding/json"
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
		_ = json.NewEncoder(w).Encode(tunarr.ProgramSearchResponse{
			Results: f.programs,
			Total:   len(f.programs),
			Page:    1,
			Limit:   100,
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
	r := NewRunner(st, client, discardLogger(), time.UTC)
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
	r := NewRunner(st, client, discardLogger(), time.UTC)

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
// whose ShowTitle is also, unavoidably, "" after the HTTP round trip:
// tunarr.Program.ShowTitle is tagged `json:"-"`
// (internal/external/tunarr/models.go) and so is never actually populated
// by SearchPrograms over HTTP in this codebase today, for any show title
// -- a separate, pre-existing gap outside this fix's scope. Using "" on
// both sides is what makes findEpisode's match succeed despite that gap,
// which is what makes this a *discriminating* regression test: without
// the blocksForChannel pre-filter in Run, channel-2's series state would
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
	r := NewRunner(st, client, discardLogger(), time.UTC)

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
