package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
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

// capturingLogger returns a *slog.Logger writing to a buffer tests can
// inspect afterward, for the handful of tests that need to assert on an
// emitted log record (e.g. the "dropped invalid programs" WARN) rather
// than just discarding output like discardLogger.
func capturingLogger() (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	return slog.New(slog.NewTextHandler(&buf, nil)), &buf
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
	seasons  map[string]tunarr.Season // seasonID -> Season, served by GET /api/programming/seasons/{id}

	mu         sync.Mutex
	updates    map[string][]tunarr.Program    // channelID -> content programs extracted from the pushed lineup
	lineups    map[string][]tunarr.LineupItem // channelID -> the raw lineup (content + flex) pushed via UpdateSchedule
	startTimes map[string]int64               // channelID -> startTime (ms) set via PUT /api/channels/{id}
}

func newFakeTunarr(t *testing.T, programs []tunarr.Program) (*httptest.Server, *fakeTunarr) {
	t.Helper()
	return newFakeTunarrWithSeasons(t, programs, nil)
}

// newFakeTunarrWithSeasons is newFakeTunarr plus a GET
// /api/programming/seasons/{id} handler serving seasons -- the fake
// counterpart to tunarr.Client.GetSeason, letting a test exercise
// Runner.resolveSeasonNumber without a real Tunarr instance. A season ID
// with no entry in seasons yields a 404, matching a real deleted/unknown
// season.
func newFakeTunarrWithSeasons(t *testing.T, programs []tunarr.Program, seasons map[string]tunarr.Season) (*httptest.Server, *fakeTunarr) {
	t.Helper()
	f := &fakeTunarr{
		programs:   programs,
		seasons:    seasons,
		updates:    make(map[string][]tunarr.Program),
		lineups:    make(map[string][]tunarr.LineupItem),
		startTimes: make(map[string]int64),
	}

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
	mux.HandleFunc("/api/programming/seasons/", seasonsHandler(t, f.seasons))
	// /api/channels/{id} and /api/channels/{id}/programming both live under
	// this one prefix (net/http's ServeMux takes one handler per pattern),
	// so this dispatches on the "/programming" suffix. Both branches mirror
	// live Tunarr 1.3.13 semantics -- source-verified, see
	// tunarr.Client.UpdateSchedule's and setChannelStartTime's doc comments
	// in client.go:
	//
	//   - /programming: no PUT route exists at all (404); POST expects the
	//     manual-lineup contract ({"type": "manual", "lineup": [...],
	//     "append": ...}), not a bare []Program body.
	//   - the bare channel resource: GET then PUT
	//     (SaveableChannelSchema, which omits fallback/programCount/
	//     transcoding/sessions) is how UpdateSchedule anchors
	//     channel.startTime before pushing a lineup.
	//
	// Mirroring both -- not just the /programming half a prior round of
	// this fix covered -- is what makes a regression on either half (PUT
	// reappearing on the programming route, or a lineup getting pushed
	// without ever anchoring startTime) fail a test instead of silently
	// passing against a fake more lenient than reality.
	mux.HandleFunc("/api/channels/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/programming") {
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/channels/"), "/programming")

			var req tunarr.ManualLineupRequest
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			if req.Type != "manual" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			programs := make([]tunarr.Program, 0, len(req.Lineup))
			for _, item := range req.Lineup {
				if item.Type != "content" {
					continue
				}
				programs = append(programs, tunarr.Program{UUID: item.ID, Duration: item.Duration, Type: item.Type})
			}

			f.mu.Lock()
			f.updates[id] = programs
			f.lineups[id] = req.Lineup
			f.mu.Unlock()

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{})
			return
		}

		id := strings.TrimPrefix(r.URL.Path, "/api/channels/")
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			// Includes the four SaveableChannelSchema-omitted keys so the PUT
			// branch below can assert setChannelStartTime actually strips
			// them before writing back.
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": id, "name": "Fake Channel", "startTime": 0,
				"fallback": []any{}, "programCount": 0, "transcoding": map[string]any{}, "sessions": []any{},
			})
		case http.MethodPut:
			var body map[string]json.RawMessage
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			for _, k := range []string{"fallback", "programCount", "transcoding", "sessions"} {
				if _, ok := body[k]; ok {
					t.Errorf("fake Tunarr: PUT /api/channels/%s must not include %q (SaveableChannelSchema omits it)", id, k)
				}
			}
			var startTime int64
			require.NoError(t, json.Unmarshal(body["startTime"], &startTime), "PUT /api/channels/%s must set startTime", id)

			f.mu.Lock()
			f.startTimes[id] = startTime
			f.mu.Unlock()

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server, f
}

// seasonsHandler serves GET /api/programming/seasons/{id} from a fixed
// seasonID -> Season map -- the fake counterpart to
// tunarr.Client.GetSeason, shared by every fake Tunarr server in this
// file that needs to exercise Runner.resolveSeasonNumber.
func seasonsHandler(t *testing.T, seasons map[string]tunarr.Season) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/api/programming/seasons/")
		season, ok := seasons[id]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(season)
	}
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

// pushedLineup returns the raw lineup (content and flex entries, in the
// order UpdateSchedule pushed them) most recently posted for channelID --
// the anchoring invariant tests need the flex entries updatedChannels
// deliberately filters out.
func (f *fakeTunarr) pushedLineup(channelID string) []tunarr.LineupItem {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]tunarr.LineupItem, len(f.lineups[channelID]))
	copy(out, f.lineups[channelID])
	return out
}

// startTimeFor returns the startTime (ms) most recently PUT to
// /api/channels/{channelID}, and whether one was ever set.
func (f *fakeTunarr) startTimeFor(channelID string) (int64, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.startTimes[channelID]
	return v, ok
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

// TestBuildAnchoredLineup_CumulativeOffsetsMatchWallClock is the
// conversion unit test for buildAnchoredLineup: given slots at known
// wall-clock times, every "content" entry's cumulative offset from anchor
// (the sum of every preceding lineup entry's Duration) must equal that
// program's actual offset from anchor, and the lineup's total duration
// must span the entire [anchor, windowEnd) apply window -- the exact
// invariant this function exists to guarantee, since Tunarr computes
// playback position purely from cumulative lineup-item durations past
// channel.startTime (see UpdateSchedule's doc comment, client.go).
//
// The fixture below exercises every gap buildAnchoredLineup has to pad:
// a leading gap before the first slot, an in-slot remainder once a
// slot's own programs run out short of its nominal duration, a
// between-slot gap, and a trailing gap out to windowEnd. Slot 1 is also
// listed AFTER slot 2 in the input to pin the sort-by-StartTime step
// (GenerateForTimeRange doesn't guarantee chronological order across
// blocks sharing a channel -- see this function's doc comment in
// schedule.go). The in-slot remainder and the following between-slot gap
// are two SEPARATE calls that both append flex, back to back -- appendFlex
// merges them into one entry (see its doc comment), so the expected
// lineup below has one flex item spanning both, not two.
func TestBuildAnchoredLineup_CumulativeOffsetsMatchWallClock(t *testing.T) {
	anchor := time.Date(2026, 1, 12, 0, 0, 0, 0, time.UTC)
	windowEnd := anchor.Add(24 * time.Hour)

	slot1 := scheduler.ScheduledSlot{
		StartTime: anchor.Add(2 * time.Hour),                // 02:00
		EndTime:   anchor.Add(2*time.Hour + 30*time.Minute), // 02:30 -- 30-min block
		Block:     scheduler.Block{Name: "Slot 1"},
		Programs: []tunarr.Program{
			{ID: "p1", Title: "Program One", Duration: float64(20 * time.Minute.Milliseconds())}, // 20 min -- 10-min in-slot remainder
		},
	}
	slot2 := scheduler.ScheduledSlot{
		StartTime: anchor.Add(5 * time.Hour),                // 05:00
		EndTime:   anchor.Add(5*time.Hour + 60*time.Minute), // 06:00
		Block:     scheduler.Block{Name: "Slot 2"},
		Programs: []tunarr.Program{
			{ID: "p2", Title: "Program Two", Duration: float64(30 * time.Minute.Milliseconds())},
			{ID: "p3", Title: "Program Three", Duration: float64(30 * time.Minute.Milliseconds())},
		},
	}

	// Deliberately out of chronological order.
	lineup, err := buildAnchoredLineup(anchor, windowEnd, []scheduler.ScheduledSlot{slot2, slot1})
	require.NoError(t, err)

	type wantItem struct {
		typ      string
		id       string
		duration time.Duration
	}
	want := []wantItem{
		{typ: "flex", duration: 2 * time.Hour},                 // 00:00 -> 02:00, leading gap
		{typ: "content", id: "p1", duration: 20 * time.Minute}, // 02:00 -> 02:20
		{typ: "flex", duration: 2*time.Hour + 40*time.Minute},  // 02:20 -> 05:00, in-slot remainder + between-slot gap merged into one entry
		{typ: "content", id: "p2", duration: 30 * time.Minute}, // 05:00 -> 05:30
		{typ: "content", id: "p3", duration: 30 * time.Minute}, // 05:30 -> 06:00
		{typ: "flex", duration: 18 * time.Hour},                // 06:00 -> 24:00, trailing pad to windowEnd
	}
	require.Len(t, lineup, len(want), "lineup: %+v", lineup)

	var cursor time.Duration
	var contentOffsets []time.Duration
	for i, item := range lineup {
		w := want[i]
		assert.Equal(t, w.typ, item.Type, "item %d type", i)
		if w.typ == "content" {
			assert.Equal(t, w.id, item.ID, "item %d id", i)
			contentOffsets = append(contentOffsets, cursor)
		} else {
			assert.Positive(t, item.Duration, "item %d (flex) must have a strictly positive duration", i)
		}
		assert.Equal(t, float64(w.duration.Milliseconds()), item.Duration, "item %d duration", i)
		cursor += time.Duration(item.Duration) * time.Millisecond
	}

	// The invariant in its own terms, independent of the exact-sequence
	// assertions above: each content entry's cumulative offset from anchor
	// equals that slot's actual wall-clock offset from anchor.
	assert.Equal(t, []time.Duration{2 * time.Hour, 5 * time.Hour, 5*time.Hour + 30*time.Minute}, contentOffsets)

	assert.Equal(t, windowEnd.Sub(anchor), cursor, "lineup must cover the entire apply window")
}

// TestAnchorForChannel_SkipsSlotWithZeroPrograms pins round-2 finding 7's
// fix: an on-air occurrence whose committed history has since been pruned
// (or predates the occurrence_start column -- migration 000003's
// legacy-epoch sentinel rows) resolves to zero Programs. Anchoring the
// channel to such a slot's StartTime anyway shifts playback to line up
// with nothing -- dead air, no real content whose mid-episode position
// needs to keep making sense. anchorForChannel must skip it, even though
// its StartTime is otherwise the earliest.
func TestAnchorForChannel_SkipsSlotWithZeroPrograms(t *testing.T) {
	anchor := time.Date(2026, 1, 12, 14, 30, 0, 0, time.UTC)

	onAirZeroContent := scheduler.ScheduledSlot{
		StartTime: anchor.Add(-20 * time.Minute), // earliest, but nothing to show
		EndTime:   anchor.Add(40 * time.Minute),
		Block:     scheduler.Block{Name: "Pruned On-Air"},
		Programs:  nil,
	}
	normal := scheduler.ScheduledSlot{
		StartTime: anchor.Add(time.Hour),
		EndTime:   anchor.Add(2 * time.Hour),
		Block:     scheduler.Block{Name: "Later Block"},
		Programs:  []tunarr.Program{{ID: "p1", Duration: 1_800_000}},
	}

	got := anchorForChannel(anchor, []scheduler.ScheduledSlot{onAirZeroContent, normal})
	assert.True(t, got.Equal(anchor), "a zero-program on-air occurrence must not shift the anchor, even though its StartTime is earliest")

	// Sanity check the opposite: an on-air occurrence WITH content still
	// shifts the anchor exactly like before this fix.
	onAirWithContent := scheduler.ScheduledSlot{
		StartTime: anchor.Add(-20 * time.Minute),
		EndTime:   anchor.Add(40 * time.Minute),
		Block:     scheduler.Block{Name: "Real On-Air"},
		Programs:  []tunarr.Program{{ID: "p2", Duration: 1_800_000}},
	}
	got2 := anchorForChannel(anchor, []scheduler.ScheduledSlot{onAirWithContent, normal})
	assert.True(t, got2.Equal(onAirWithContent.StartTime), "an on-air occurrence WITH content must still shift the anchor to its own StartTime")
}

// TestBuildAnchoredLineup_ZeroProgramSlot_MergesAdjacentFlexEntries is
// round-2 finding 7's other half: even with anchorForChannel no longer
// anchoring to it, a zero-program slot sitting between anchor and
// windowEnd still needs padding -- and, without merging, that padding
// comes out as TWO separate, directly adjacent flex entries (the leading
// gap up to the slot's own StartTime, immediately followed by the
// trailing gap covering the slot's own content-less span, with nothing
// -- no content item -- between them). appendFlex must merge them into
// one.
func TestBuildAnchoredLineup_ZeroProgramSlot_MergesAdjacentFlexEntries(t *testing.T) {
	anchor := time.Date(2026, 1, 12, 0, 0, 0, 0, time.UTC)
	windowEnd := anchor.Add(2 * time.Hour)

	zeroContent := scheduler.ScheduledSlot{
		StartTime: anchor.Add(20 * time.Minute),
		EndTime:   anchor.Add(50 * time.Minute),
		Block:     scheduler.Block{Name: "Pruned On-Air"},
		Programs:  nil,
	}

	lineup, err := buildAnchoredLineup(anchor, windowEnd, []scheduler.ScheduledSlot{zeroContent})
	require.NoError(t, err)

	// Without merging this would be THREE separate flex entries back to
	// back (leading gap to the slot's start, the slot's own
	// content-less span, and the trailing pad to windowEnd) -- all dead
	// air, nothing separating them. Merged, it must be exactly one.
	require.Len(t, lineup, 1, "lineup: %+v", lineup)
	assert.Equal(t, "flex", lineup[0].Type)
	assert.Equal(t, float64(windowEnd.Sub(anchor).Milliseconds()), lineup[0].Duration)
}

// TestBuildAnchoredLineup_NoSlots_ReturnsWholeWindowAsFlex covers the
// degenerate case (a channel with no occurrences in this apply window):
// buildAnchoredLineup must still produce a lineup, spanning the entire
// window as a single flex entry, rather than an empty one -- an empty
// lineup would leave Tunarr's channel.duration at 0, which
// calculateStreamDuration (StreamProgramCalculator.ts) treats as a
// guaranteed one-day flex fallback of its own, not "nothing scheduled,
// leave the channel alone".
func TestBuildAnchoredLineup_NoSlots_ReturnsWholeWindowAsFlex(t *testing.T) {
	anchor := time.Date(2026, 1, 12, 0, 0, 0, 0, time.UTC)
	windowEnd := anchor.Add(24 * time.Hour)

	lineup, err := buildAnchoredLineup(anchor, windowEnd, nil)
	require.NoError(t, err)

	require.Len(t, lineup, 1)
	assert.Equal(t, "flex", lineup[0].Type)
	assert.Equal(t, float64((24 * time.Hour).Milliseconds()), lineup[0].Duration)
}

// TestBuildAnchoredLineup_RejectsInvalidProgram is buildAnchoredLineup's
// half of the validation TestClient_UpdateSchedule_ValidationError used
// to cover directly against the client (moved here because UpdateSchedule
// no longer sees Program values at all -- see the note left in its place
// in internal/external/tunarr/client_test.go).
func TestBuildAnchoredLineup_RejectsInvalidProgram(t *testing.T) {
	anchor := time.Date(2026, 1, 12, 0, 0, 0, 0, time.UTC)
	windowEnd := anchor.Add(24 * time.Hour)

	slots := []scheduler.ScheduledSlot{
		{
			StartTime: anchor,
			EndTime:   anchor.Add(30 * time.Minute),
			Block:     scheduler.Block{Name: "Bad Block"},
			Programs: []tunarr.Program{
				{ID: "p1", Title: "", Duration: 1_800_000, Type: "movie"}, // missing required title
			},
		},
	}

	_, err := buildAnchoredLineup(anchor, windowEnd, slots)
	assert.Error(t, err, "expected a validation error for the program missing a required title")
}

// TestRunner_Run_Apply_AnchorsChannelStartTimeAndPadsFullWindow is the
// end-to-end regression test proving Runner.Run's real Apply path -- not
// just buildAnchoredLineup in isolation -- anchors and pads the lineup it
// pushes: the fake Tunarr server's recorded startTime (PUT
// /api/channels/{id}) must equal the anchor buildAnchoredLineup actually
// used, and the recorded lineup (GET .../programming) must cover at
// least the entire apply window (never less; see buildAnchoredLineup's
// doc comment for why it can legitimately run a little over -- the
// engine lets a block starting just inside the window complete rather
// than cutting it off).
//
// This Runner's single block ("Hourly Movies", cron "0 * * * *", 60min
// duration) has no gap between consecutive occurrences at all, and --
// with exactly two 30-minute candidate movies and nothing else in the
// catalog -- each occurrence fills its own full 60 minutes too. Which
// means this fixture always has some occurrence on air at apply time
// (some occurrence's [start, start+60min) window always contains "now"):
// see GenerateForTimeRange's on-air-shell-injection doc comment
// (engine.go) and anchorForChannel (schedule.go). So the expected anchor
// here is that on-air occurrence's own StartTime -- an exact hour
// boundary, always <= `before` -- not Run's own `start`; and because that
// on-air occurrence becomes the lineup's first (lowest StartTime) slot,
// sitting exactly at the anchor, there is no leading flex gap before it
// either (unlike before finding 7's on-air handling existed, back when
// the anchor was always Run's own `start`, essentially never exactly on
// the hour). A regression back to flattenSlots-style concatenation (the
// original Bug 1 this test was written for: content only, no flex, no
// channel.startTime call) would still be caught here: the anchor
// wouldn't land on an exact hour boundary <= before, and
// UpdateSchedule/setChannelStartTime would never be called at all.
func TestRunner_Run_Apply_AnchorsChannelStartTimeAndPadsFullWindow(t *testing.T) {
	server, fake := newFakeTunarr(t, canonicalPrograms())
	r, _ := newTestRunner(t, server.URL)

	before := time.Now().Truncate(time.Minute)
	result, err := r.Run(context.Background(), Options{Days: 1, Apply: true})
	require.NoError(t, err)
	require.True(t, result.Applied)

	startTimeMs, ok := fake.startTimeFor("channel-1")
	require.True(t, ok, "expected channel.startTime to have been set via PUT /api/channels/channel-1")
	gotAnchor := time.UnixMilli(startTimeMs).UTC()

	// .UTC() before comparing: gotAnchor is already UTC (round-tripped
	// through a UnixMilli timestamp), and testify's Equal on time.Time
	// compares struct fields directly (not the .Equal() absolute-instant
	// semantic) -- two Times for the same instant in different
	// Locations would otherwise spuriously fail this assertion.
	topOfHour := before.Truncate(time.Hour).UTC()
	assert.Equal(t, topOfHour, gotAnchor, "anchor must be the on-air occurrence's own StartTime (the current hour boundary), not Run's start")
	assert.False(t, gotAnchor.After(time.Now()), "anchor must not be after Run finished")
	assert.Equal(t, gotAnchor, gotAnchor.Truncate(time.Minute), "anchor must already be minute-truncated")

	lineup := fake.pushedLineup("channel-1")
	require.NotEmpty(t, lineup)

	var total time.Duration
	var haveContent bool
	for _, item := range lineup {
		switch item.Type {
		case "content":
			haveContent = true
			assert.NotEmpty(t, item.ID, "a content entry must carry a program ID")
		case "flex":
			assert.Positive(t, item.Duration, "a flex entry must have a strictly positive duration")
		default:
			t.Errorf("unexpected lineup item type %q", item.Type)
		}
		total += time.Duration(item.Duration) * time.Millisecond
	}
	assert.True(t, haveContent, "expected at least one content entry (the hourly block's matched program)")
	// No haveFlex assertion: this fixture's occurrences are exactly
	// back-to-back and each exactly fills its own 60 minutes (two 30min
	// movies), and its anchor now sits exactly on the on-air occurrence's
	// own start (see the doc comment above) -- so, unlike before finding
	// 7, there is no longer a guaranteed flex source for this specific
	// fixture. Whether a flex entry happens to appear (e.g. a trailing
	// pad if the window doesn't end on an hour boundary) isn't this
	// test's concern; TestBuildAnchoredLineup_* covers flex-padding
	// behavior directly.
	assert.GreaterOrEqual(t, total, 24*time.Hour, "pushed lineup must cover at least the entire apply window (it may legitimately run a little over -- see buildAnchoredLineup's doc comment)")
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

// TestRunner_Run_SeriesBlock_MatchesEpisodeFromNestedShowObject pins
// hydrateEpisodeShowFields's SECONDARY, defensive hydration path (see its
// doc comment in internal/external/tunarr/client.go): a series block
// matches an episode whose show identity arrives nested under
// Program.Show. This does NOT reflect a live Tunarr 1.3.13 response --
// live-verified this session (transcript in this task's report) that a
// real episode result never nests a "show" object; a prior round of this
// fix claimed otherwise from a spec read alone, and that claim was wrong.
// See TestRunner_Run_SeriesBlock_MatchesEpisodeViaLiveShowAndSeasonJoin
// below for the actual live-shaped end-to-end test (episode carries only
// ShowID/SeasonID foreign keys, joined against a separate interleaved
// Type == "show" entry and a resolved season) -- THAT is the test that
// proves a Saturday-night block works against real Tunarr. This test
// stays because the nested-Show path itself is still real code
// (client.go keeps it as a harmless secondary hydration, correct for a
// hypothetical richer future response shape) and deserves its own
// regression coverage, just not the "this is what live Tunarr sends"
// claim it used to carry.
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

// liveShapeOfficeShowID and liveShapeOfficeSeasonID are the join keys
// shared by the live-shaped fixtures below (liveShapeOfficeShow,
// liveShapeOfficePilot, liveShapeOfficeSeasonOne): a real Tunarr episode
// result's ShowID/SeasonID foreign keys, exactly as live-verified this
// session (transcript in this task's report).
const (
	liveShapeOfficeShowID   = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	liveShapeOfficeSeasonID = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
)

// liveShapeOfficeShow is a separate Type == "show" search-result entry --
// live-verified this session: a real Tunarr search interleaves entries
// like this one in the SAME paginated result stream as episodes, not
// nested inside them.
func liveShapeOfficeShow() tunarr.Program {
	return tunarr.Program{
		UUID: liveShapeOfficeShowID, Type: "show", Title: "The Office", Rating: "TV-14",
	}
}

// liveShapeOfficePilot is what a live Tunarr episode result actually
// looks like -- live-verified this session: only ShowID/SeasonID foreign
// keys, no flat ShowTitle/Rating/SeasonNumber, no nested Show object at
// all.
func liveShapeOfficePilot() tunarr.Program {
	return tunarr.Program{
		ID: "ep-1", Title: "Pilot", Type: "episode", Duration: 1_320_000,
		ShowID: liveShapeOfficeShowID, SeasonID: liveShapeOfficeSeasonID, EpisodeNumber: 1,
	}
}

// liveShapeOfficeSeasonOne is the tunarr.Season GET
// /api/programming/seasons/{id} resolves liveShapeOfficeSeasonID to --
// live-verified this session: the wire key is "index" (SeasonNumber's
// json tag; see models.go), not "seasonNumber".
func liveShapeOfficeSeasonOne() tunarr.Season {
	return tunarr.Season{UUID: liveShapeOfficeSeasonID, Title: "Season 1", SeasonNumber: 1}
}

// TestRunner_Run_SeriesBlock_MatchesEpisodeViaLiveShowAndSeasonJoin is the
// DEFINITIVE end-to-end regression test for the show/season-hydration bug
// -- the one that actually proves a Saturday-night series block works
// against a real Tunarr 1.3.13 instance, replacing the now-corrected
// nested-Show version above (which never reflected live Tunarr's actual
// wire shape). It uses exactly the shape live-verified this session
// (transcript in this task's report): the episode carries only
// ShowID/SeasonID foreign keys (liveShapeOfficePilot); its show is a
// separate, interleaved Type == "show" search result entry
// (liveShapeOfficeShow); its season number comes from a real HTTP
// round trip to a fake GET /api/programming/seasons/{id}
// (liveShapeOfficeSeasonOne, via newFakeTunarrWithSeasons). A series block
// with SeriesConfig.ShowTitle == "The Office" must still match this
// episode, proving service.Runner.hydrateShowsAndSeasons' join (ShowID ->
// interleaved show entry) and season resolution (SeasonID -> GetSeason)
// both actually feed scheduler.Engine's findEpisode, which requires
// ShowTitle AND SeasonNumber AND EpisodeNumber to all match.
func TestRunner_Run_SeriesBlock_MatchesEpisodeViaLiveShowAndSeasonJoin(t *testing.T) {
	programs := []tunarr.Program{liveShapeOfficeShow(), liveShapeOfficePilot()}
	seasons := map[string]tunarr.Season{liveShapeOfficeSeasonID: liveShapeOfficeSeasonOne()}
	server, _ := newFakeTunarrWithSeasons(t, programs, seasons)

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
			if p.Title == "Pilot" && p.ShowTitle == "The Office" && p.SeasonNumber == 1 && p.EpisodeNumber == 1 {
				matchedPilot = true
			}
		}
	}
	assert.True(t, matchedPilot,
		`series block with SeriesConfig.ShowTitle == "The Office" must match the live-shaped episode (ShowID/SeasonID FKs only) via the show join + season resolution`)
}

// TestRunner_fetchLibraryPrograms_JoinsShowAcrossPaginationBoundary is the
// pagination+join interaction regression test: a show's Type == "show"
// entry lands on page 1 of a paginated library fetch, and its episode
// lands on page 3 -- live-verified this session that a show entry is NOT
// reliably co-located on the same search-results page as its own
// episodes (see hydrateShowsAndSeasons' doc comment in schedule.go for the
// live evidence). This proves the join happens over the FULLY
// accumulated, post-pagination []Program (as hydrateShowsAndSeasons'
// callers guarantee), not per-page -- a per-page join would miss this
// episode's show entirely, since they never share a page.
func TestRunner_fetchLibraryPrograms_JoinsShowAcrossPaginationBoundary(t *testing.T) {
	const limit = 100
	all := make([]tunarr.Program, 0, 251)
	all = append(all, liveShapeOfficeShow()) // index 0 -> page 1
	for i := 1; i < 250; i++ {
		all = append(all, tunarr.Program{
			ID: fmt.Sprintf("filler-%03d", i), Title: fmt.Sprintf("Filler %03d", i),
			Duration: 1_800_000, Type: "movie",
		})
	}
	all = append(all, liveShapeOfficePilot()) // index 250 -> page 3 (250/100 = page 3)

	server := newPaginatedFakeLibraryTunarr(t, all)

	st, err := store.New(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = st.Close() })

	client := tunarr.NewClient(tunarr.Config{URL: server.URL})
	r := NewRunner(st, client, discardLogger(), time.UTC, 0)

	got := r.fetchTunarrContent(context.Background())
	require.Len(t, got, 251)

	var pilot *tunarr.Program
	for i := range got {
		if got[i].ID == "ep-1" {
			pilot = &got[i]
		}
	}
	require.NotNil(t, pilot, "the pilot episode must still be present in the fully accumulated fetch")
	assert.Equal(t, "The Office", pilot.ShowTitle,
		"ShowTitle must be joined from the show entry on page 1 even though the episode itself was fetched from page 3")
	assert.Equal(t, "TV-14", pilot.Rating)
}

// TestRunner_hydrateShowTitleAndRating is a direct, HTTP-free unit test of
// the join itself -- found necessary during this fix's revert-verify pass:
// the higher-level MediaMeta live-shape test (media_test.go) turned out
// NOT to discriminate this function (Type == "show" entries contribute
// their own Rating to MediaMeta's aggregate regardless of whether the
// join ran), so this is the test that actually pins hydrateShowTitleAndRating's
// behavior in isolation. Covers every branch: an episode's ShowID
// resolves against a Type == "show" entry elsewhere in the same slice and
// gets hydrated; an already-flat episode is left untouched; a ShowID with
// no matching show entry stays empty (not an error); non-episode types
// are never touched.
func TestRunner_hydrateShowTitleAndRating(t *testing.T) {
	r := &Runner{} // pure function: touches only its `programs` argument, no store/tunarr/cache/logger needed

	programs := []tunarr.Program{
		{UUID: "show-1", Type: "show", Title: "The Office", Rating: "TV-14"},
		{ID: "ep-1", Type: "episode", Title: "Pilot", ShowID: "show-1"},
		{
			ID: "ep-2", Type: "episode", Title: "Already Flat", ShowID: "show-1",
			ShowTitle: "Flat Title", Rating: "Flat Rating",
		},
		{ID: "ep-3", Type: "episode", Title: "Unknown Show", ShowID: "does-not-exist"},
		{ID: "m1", Type: "movie", Title: "A Movie", Rating: "R"},
	}

	r.hydrateShowTitleAndRating(programs)

	assert.Equal(t, "The Office", programs[1].ShowTitle, "ep-1 must be hydrated from the show entry its ShowID points at")
	assert.Equal(t, "TV-14", programs[1].Rating)

	assert.Equal(t, "Flat Title", programs[2].ShowTitle, "ep-2's already-flat ShowTitle must not be overridden")
	assert.Equal(t, "Flat Rating", programs[2].Rating, "ep-2's already-flat Rating must not be overridden")

	assert.Empty(t, programs[3].ShowTitle, "ep-3's ShowID matches no show entry -- stays empty, not an error")
	assert.Empty(t, programs[3].Rating)

	assert.Equal(t, "R", programs[4].Rating, "a movie is never touched by episode-only hydration")
}

// TestRunner_hydrateSeasonNumbers is a direct, HTTP-free-except-for-the-
// fake-seasons-endpoint unit test of season resolution in isolation,
// mirroring TestRunner_hydrateShowTitleAndRating above for the same
// revert-verify reason: this needs its own discriminating test, separate
// from any higher-level end-to-end one. Covers: an episode's SeasonID
// resolves via the fake seasons endpoint; a second episode sharing the
// same SeasonID reuses the one resolution; an unknown SeasonID leaves
// SeasonNumber at 0 (not an error); an episode that already has a
// SeasonNumber is never re-resolved; movies are untouched.
func TestRunner_hydrateSeasonNumbers(t *testing.T) {
	const seasonID = "season-1"
	seasons := map[string]tunarr.Season{
		seasonID: {UUID: seasonID, Title: "Season 1", SeasonNumber: 3},
	}
	server, _ := newFakeTunarrWithSeasons(t, nil, seasons)

	st, err := store.New(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = st.Close() })

	client := tunarr.NewClient(tunarr.Config{URL: server.URL})
	r := NewRunner(st, client, discardLogger(), time.UTC, 0)

	programs := []tunarr.Program{
		{ID: "ep-1", Type: "episode", Title: "Ep 1", SeasonID: seasonID},
		{ID: "ep-2", Type: "episode", Title: "Ep 2", SeasonID: seasonID},
		{ID: "ep-3", Type: "episode", Title: "Ep 3", SeasonID: "unknown-season"},
		{ID: "ep-4", Type: "episode", Title: "Ep 4", SeasonID: seasonID, SeasonNumber: 7},
		{ID: "m1", Type: "movie", Title: "A Movie"},
	}

	r.hydrateSeasonNumbers(context.Background(), programs)

	assert.Equal(t, 3, programs[0].SeasonNumber, "ep-1 resolved from the fake seasons endpoint")
	assert.Equal(t, 3, programs[1].SeasonNumber, "ep-2 shares the same season, resolved once and applied to both")
	assert.Equal(t, 0, programs[2].SeasonNumber, "ep-3's season doesn't exist -- stays unresolved (0), not an error")
	assert.Equal(t, 7, programs[3].SeasonNumber, "ep-4 already had a SeasonNumber -- must not be re-resolved/overridden")
	assert.Equal(t, 0, programs[4].SeasonNumber, "movies are never touched")
}

// TestRunner_resolveSeasonNumber_CachesAcrossCalls pins the caching
// contract: a second resolveSeasonNumber call for a SeasonID already
// resolved must be served from Runner's cache, issuing no new Tunarr
// request -- the "cache resolutions in the same 1h content cache keyed
// by seasonId" requirement.
func TestRunner_resolveSeasonNumber_CachesAcrossCalls(t *testing.T) {
	const seasonID = "season-1"
	var mu sync.Mutex
	requestCount := 0

	mux := http.NewServeMux()
	mux.HandleFunc("/api/programming/seasons/", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requestCount++
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(tunarr.Season{UUID: seasonID, SeasonNumber: 2})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	st, err := store.New(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = st.Close() })

	client := tunarr.NewClient(tunarr.Config{URL: server.URL})
	r := NewRunner(st, client, discardLogger(), time.UTC, 0)

	number1, ok1 := r.resolveSeasonNumber(context.Background(), seasonID)
	require.True(t, ok1)
	assert.Equal(t, 2, number1)

	number2, ok2 := r.resolveSeasonNumber(context.Background(), seasonID)
	require.True(t, ok2)
	assert.Equal(t, 2, number2)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, 1, requestCount,
		"a second resolveSeasonNumber call for the same seasonID must be served from Runner's cache, issuing no new Tunarr request")
}

// TestRunner_fetchSingleLibrary_SkipsInvalidProgramsAndLogsOnce is the
// end-to-end regression test for the pre-existing bug a growing library
// scan exposed: live-verified this session that a real library search
// interleaves entries of several Type values, including ones this
// client's Program.Type oneof didn't recognize at the time (Type ==
// "season" specifically -- now fixed, see TestValidateProgram in
// internal/external/tunarr/client_test.go) -- and nothing guarantees
// every value Tunarr might ever send is covered. Before this fix, ANY
// single invalid entry made the whole page's SearchPrograms call return
// an error, which fetchSingleLibrary treated as fatal: `return nil`,
// discarding every already-accumulated page too, not just the one bad
// entry. This fixture mixes a valid movie, a valid (now that the oneof is
// fixed) season entry, and a truly-unknown-type entry: the fetch must
// still succeed, keep the two valid entries, drop only the invalid one,
// and log exactly one WARN summarizing the counts for the whole fetch.
func TestRunner_fetchSingleLibrary_SkipsInvalidProgramsAndLogsOnce(t *testing.T) {
	all := []tunarr.Program{
		{ID: "m1", Title: "A Movie", Type: "movie", Duration: 1_800_000},
		{UUID: "season-1", Title: "Season 1", Type: "season", Index: 1},
		{ID: "mystery-1", Title: "Mystery Entry", Type: "definitely-not-a-real-type", Duration: 100},
	}
	server := newPaginatedFakeLibraryTunarr(t, all)

	st, err := store.New(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = st.Close() })

	logger, logBuf := capturingLogger()
	client := tunarr.NewClient(tunarr.Config{URL: server.URL})
	r := NewRunner(st, client, logger, time.UTC, 0)

	got := r.fetchTunarrContent(context.Background())
	require.Len(t, got, 2, "the fetch must succeed and keep the 2 valid entries, dropping only the unknown-type one")

	var haveMovie, haveSeason bool
	for _, p := range got {
		switch {
		case p.ID == "m1":
			haveMovie = true
		case p.UUID == "season-1":
			haveSeason = true
		}
	}
	assert.True(t, haveMovie, "the valid movie must survive")
	assert.True(t, haveSeason, `the season entry must survive now that "season" is a valid Type`)

	logOutput := logBuf.String()
	assert.Contains(t, logOutput, "dropped invalid programs", "exactly one summary WARN must be logged for the whole fetch")
	assert.Contains(t, logOutput, "dropped_count=1")
	assert.Contains(t, logOutput, "valid_count=2")
}

// TestRunner_hydrateSeasonNumbers_LocalJoinAvoidsNetworkFallback pins the
// optimization half of this round's season fix: when a Type == "season"
// entry for an episode's SeasonID is already present in the accumulated
// slice (live-verified this session that this happens -- a 100-item page
// was observed as 100% season entries during a library scan), resolving
// it must cost zero Tunarr requests, not fall through to
// resolveSeasonNumber/Client.GetSeason. The fake season endpoint here
// would fail the test if hit (t.Error inside the handler), so a passing
// test proves the local join path was actually taken.
func TestRunner_hydrateSeasonNumbers_LocalJoinAvoidsNetworkFallback(t *testing.T) {
	const seasonID = "season-1"

	mux := http.NewServeMux()
	mux.HandleFunc("/api/programming/seasons/", func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("resolveSeasonNumber's network fallback must not be reached when a local season entry already resolves the SeasonID (got request for %s)", r.URL.Path)
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	st, err := store.New(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = st.Close() })

	client := tunarr.NewClient(tunarr.Config{URL: server.URL})
	r := NewRunner(st, client, discardLogger(), time.UTC, 0)

	programs := []tunarr.Program{
		{UUID: seasonID, Type: "season", Title: "Season 1", Index: 4},
		{ID: "ep-1", Type: "episode", Title: "Ep 1", SeasonID: seasonID},
		{ID: "ep-2", Type: "episode", Title: "Ep 2", SeasonID: seasonID},
	}

	r.hydrateSeasonNumbers(context.Background(), programs)

	assert.Equal(t, 4, programs[1].SeasonNumber, "ep-1 resolved locally from the interleaved season entry's Index")
	assert.Equal(t, 4, programs[2].SeasonNumber, "ep-2 resolved locally too, from the same local join")
}

// TestRunner_hydrateSeasonNumbers_LocalJoinThenNetworkFallback proves the
// two paths compose correctly in one call: a SeasonID covered by a local
// season entry resolves for free, while a different SeasonID with no
// local entry still falls back to the network resolver -- "resolver stays
// as fallback," not replaced.
func TestRunner_hydrateSeasonNumbers_LocalJoinThenNetworkFallback(t *testing.T) {
	const localSeasonID = "season-local"
	const remoteSeasonID = "season-remote"

	seasons := map[string]tunarr.Season{
		remoteSeasonID: {UUID: remoteSeasonID, Title: "Season 9", SeasonNumber: 9},
	}
	server, _ := newFakeTunarrWithSeasons(t, nil, seasons)

	st, err := store.New(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = st.Close() })

	client := tunarr.NewClient(tunarr.Config{URL: server.URL})
	r := NewRunner(st, client, discardLogger(), time.UTC, 0)

	programs := []tunarr.Program{
		{UUID: localSeasonID, Type: "season", Title: "Season 1", Index: 1},
		{ID: "ep-local", Type: "episode", Title: "Local Ep", SeasonID: localSeasonID},
		{ID: "ep-remote", Type: "episode", Title: "Remote Ep", SeasonID: remoteSeasonID},
	}

	r.hydrateSeasonNumbers(context.Background(), programs)

	assert.Equal(t, 1, programs[1].SeasonNumber, "resolved via the local join")
	assert.Equal(t, 9, programs[2].SeasonNumber, "resolved via the network fallback, since no local season entry covered it")
}

// TestRunner_Run_Apply_IsIdempotentPerOccurrence is the regression test for
// the non-idempotent-apply bug found during live multi-block testing: a
// Saturday-8pm block observed E01 on its first apply, then E04 after two
// more, because the 24h apply window slid forward every cron tick
// (default 6h) while still covering the same future occurrence, and every
// apply that saw it replanned it from scratch -- advancing the series
// cursor again for content that was never going to change, since the next
// apply would just overwrite it anyway. Two consecutive Runner.Run(Apply:
// true) calls over the same window must instead produce byte-identical
// plans, and the second must leave series state exactly as the first left
// it.
func TestRunner_Run_Apply_IsIdempotentPerOccurrence(t *testing.T) {
	episodes := []tunarr.Program{
		{ID: "ep-1", Title: "Pilot", Type: "episode", ShowTitle: "The Office", SeasonNumber: 1, EpisodeNumber: 1, Duration: 1_320_000},
		{ID: "ep-2", Title: "Diversity Day", Type: "episode", ShowTitle: "The Office", SeasonNumber: 1, EpisodeNumber: 2, Duration: 1_320_000},
	}
	server, _ := newFakeTunarr(t, episodes)

	st, err := store.New(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = st.Close() })

	ctx := context.Background()
	require.NoError(t, st.CreateBlock(ctx, &store.BlockRecord{
		ID: "block-1", Name: "Evening Sitcoms", Enabled: true,
		Spec: scheduler.Block{
			Name: "Evening Sitcoms", Cron: "0 * * * *", Duration: 30, ChannelID: "channel-1", Priority: 10,
			Type:   scheduler.BlockTypeSeries,
			Series: []scheduler.SeriesConfig{{ShowTitle: "The Office", EpisodesPerBlock: 1}},
		},
	}))

	client := tunarr.NewClient(tunarr.Config{URL: server.URL})
	r := NewRunner(st, client, discardLogger(), time.UTC, 0)

	result1, err := r.Run(ctx, Options{Days: 1, Apply: true})
	require.NoError(t, err)
	require.True(t, result1.Applied)
	require.NotEmpty(t, result1.Channels["channel-1"], "expected at least one occurrence planned in the window")

	state1, err := st.GetPersistedSeriesState(ctx, "The Office")
	require.NoError(t, err)
	// The hourly cron plans every occurrence in the 24h window within this
	// one Run call, so the cursor may already have advanced past both
	// available episodes (and the series marked completed) by the time
	// this first apply returns -- the exact count isn't what this test is
	// about. What matters, and is asserted below, is that a *second* apply
	// over the same window leaves this exact state untouched.
	require.NotEqual(t, 1, state1.CurrentEpisode,
		"first apply should have advanced the cursor at least once past the S01E01 default")

	// A second apply over the same window -- exactly what a real
	// deployment's cron loop does every tick: the same future occurrences
	// already committed by the previous apply are still inside the new
	// window and get planned again.
	result2, err := r.Run(ctx, Options{Days: 1, Apply: true})
	require.NoError(t, err)
	require.True(t, result2.Applied)

	assert.Equal(t, result1.Channels, result2.Channels,
		"re-applying over the same window must reuse every already-committed occurrence's assignment byte-for-byte, not re-plan it")

	state2, err := st.GetPersistedSeriesState(ctx, "The Office")
	require.NoError(t, err)
	assert.Equal(t, state1, state2,
		"series state must not advance again for an occurrence that was already committed")
}

// TestRunner_Run_Apply_ConflictDroppedOccurrence_DoesNotAdvanceCursor is
// the second half of the idempotent-apply fix's required coverage: an
// occurrence that conflict resolution drops must never reach
// Engine.PlanBlock at all, so it can't advance a series cursor (or create
// series state in the first place) for content that will never actually
// air. "Loser Series" and "Winner Filter" share the exact same cron and
// duration, so every one of "Loser Series"'s occurrences across the whole
// apply window overlaps -- and, being lower priority, loses to -- the
// corresponding "Winner Filter" occurrence.
func TestRunner_Run_Apply_ConflictDroppedOccurrence_DoesNotAdvanceCursor(t *testing.T) {
	episode := tunarr.Program{ID: "ep-1", Title: "Pilot", Type: "episode", ShowTitle: "The Office", SeasonNumber: 1, EpisodeNumber: 1, Duration: 1_320_000}
	movie := tunarr.Program{ID: "mov-1", Title: "A Movie", Type: "movie", Duration: 3_600_000}
	server, _ := newFakeTunarr(t, []tunarr.Program{episode, movie})

	st, err := store.New(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = st.Close() })

	ctx := context.Background()
	require.NoError(t, st.CreateBlock(ctx, &store.BlockRecord{
		ID: "block-loser", Name: "Loser Series", Enabled: true,
		Spec: scheduler.Block{
			Name: "Loser Series", Cron: "0 * * * *", Duration: 60, ChannelID: "channel-1", Priority: 5,
			Type:   scheduler.BlockTypeSeries,
			Series: []scheduler.SeriesConfig{{ShowTitle: "The Office", EpisodesPerBlock: 1}},
		},
	}))
	require.NoError(t, st.CreateBlock(ctx, &store.BlockRecord{
		ID: "block-winner", Name: "Winner Filter", Enabled: true,
		Spec: scheduler.Block{
			Name: "Winner Filter", Cron: "0 * * * *", Duration: 60, ChannelID: "channel-1", Priority: 10,
		},
	}))

	client := tunarr.NewClient(tunarr.Config{URL: server.URL})
	r := NewRunner(st, client, discardLogger(), time.UTC, 0)

	result, err := r.Run(ctx, Options{Days: 1, Apply: true})
	require.NoError(t, err)
	require.True(t, result.Applied)

	require.NotEmpty(t, result.Warnings, "expected every one of the loser series block's occurrences to be reported as dropped")
	for _, w := range result.Warnings {
		assert.Equal(t, "Loser Series", w.BlockName)
		assert.Equal(t, "Winner Filter", w.BlockingBlockName)
	}

	// The decisive assertion: a conflict-dropped occurrence must never
	// reach PlanBlock, so no series_state row should exist for "The
	// Office" at all -- not "exists but still at S01E01", genuinely never
	// written, mirroring the existing channel-scoping regression test's
	// pattern (TestRunner_Run_ChannelScopedApply_LeavesOtherChannelStateUntouched).
	_, err = st.GetPersistedSeriesState(ctx, "The Office")
	assert.ErrorIs(t, err, store.ErrNotFound,
		"a conflict-dropped series occurrence must never advance, or even create, series state")
}
