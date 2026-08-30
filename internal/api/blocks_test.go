package api

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/christopherime/schedularr/internal/api/gen"
	"github.com/christopherime/schedularr/internal/external/tunarr"
	"github.com/christopherime/schedularr/internal/scheduler"
	"github.com/christopherime/schedularr/internal/store"
)

// newTestServer builds the full gen.HandlerFromMux router (no auth
// middleware -- that's Task 8's concern, exercised elsewhere) backed by a
// real, fresh temp-dir sqlite store, matching how Task 9's brief specifies
// handler tests: against the router, with a real store, no mocks.
func newTestServer(t *testing.T) http.Handler {
	t.Helper()
	s, err := store.New(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err, "failed to create test store")
	t.Cleanup(func() { _ = s.Close() })

	h := NewHandlers(Deps{Store: s, Logger: slog.Default(), Version: "test"})
	return gen.HandlerFromMux(h, chi.NewRouter())
}

// doRequest sends method/path with body JSON-encoded (or no body if nil)
// through h and returns the recorded response.
func doRequest(t *testing.T, h http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()

	var req *http.Request
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req = httptest.NewRequest(method, path, bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}

	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

// filterBlockWrite builds a minimal, valid filter-block BlockWrite body.
// It deliberately leaves Spec.Type nil (as a real client omitting the
// optional "type" field would) to exercise fromGen's normalization.
func filterBlockWrite(name, cron string) gen.BlockWrite {
	return gen.BlockWrite{
		Spec: gen.BlockSpec{
			Name:      name,
			Cron:      cron,
			Duration:  60,
			ChannelId: "channel-1",
		},
	}
}

// seriesBlockWrite builds a minimal, valid series-block BlockWrite body,
// deliberately omitting Type and every optional SeriesConfig field
// (OnComplete, StartSeason, StartEpisode) to exercise fromGen's
// CUE-default normalization end to end through the handler.
func seriesBlockWrite(name string) gen.BlockWrite {
	seriesType := gen.BlockSpecTypeSeries
	return gen.BlockWrite{
		Spec: gen.BlockSpec{
			Name:      name,
			Cron:      "0 20 * * 6",
			Duration:  90,
			ChannelId: "channel-1",
			Type:      &seriesType,
			Series: &[]gen.SeriesConfig{
				{ShowTitle: "Show A", EpisodesPerBlock: 2},
			},
		},
	}
}

func decodeBlockRecord(t *testing.T, w *httptest.ResponseRecorder) gen.BlockRecord {
	t.Helper()
	var rec gen.BlockRecord
	require.NoError(t, json.NewDecoder(w.Body).Decode(&rec), "body: %s", w.Body.String())
	return rec
}

func decodeProblem(t *testing.T, w *httptest.ResponseRecorder) Problem {
	t.Helper()
	var p Problem
	require.NoError(t, json.NewDecoder(w.Body).Decode(&p), "body: %s", w.Body.String())
	return p
}

func TestCreateBlock_Success(t *testing.T) {
	h := newTestServer(t)

	w := doRequest(t, h, http.MethodPost, "/blocks", filterBlockWrite("morning-cartoons", "0 6 * * *"))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	require.Equal(t, "application/json", w.Header().Get("Content-Type"))

	rec := decodeBlockRecord(t, w)
	assert.NotEmpty(t, rec.Id)
	assert.Equal(t, "morning-cartoons", rec.Name)
	assert.True(t, rec.Enabled, "enabled should default true when omitted")
	assert.Equal(t, "morning-cartoons", rec.Spec.Name)
	assert.Equal(t, "0 6 * * *", rec.Spec.Cron)
	assert.Equal(t, 60, rec.Spec.Duration)
	assert.Equal(t, "channel-1", rec.Spec.ChannelId)
	require.NotNil(t, rec.Spec.Type, "type should be normalized, not omitted")
	assert.Equal(t, gen.BlockSpecTypeFilter, *rec.Spec.Type, "empty type should normalize to filter")
	assert.False(t, rec.CreatedAt.IsZero())
	assert.False(t, rec.UpdatedAt.IsZero())

	// Round trip: GET returns the same record that CreateBlock returned.
	wg := doRequest(t, h, http.MethodGet, "/blocks/"+rec.Id, nil)
	require.Equal(t, http.StatusOK, wg.Code)
	got := decodeBlockRecord(t, wg)
	assert.Equal(t, rec, got)
}

func TestCreateBlock_EnabledExplicitFalse(t *testing.T) {
	h := newTestServer(t)

	body := filterBlockWrite("disabled-block", "0 6 * * *")
	f := false
	body.Enabled = &f

	w := doRequest(t, h, http.MethodPost, "/blocks", body)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	rec := decodeBlockRecord(t, w)
	assert.False(t, rec.Enabled)
}

func TestCreateBlock_SeriesDefaultsNormalized(t *testing.T) {
	h := newTestServer(t)

	w := doRequest(t, h, http.MethodPost, "/blocks", seriesBlockWrite("weekend-marathon"))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	rec := decodeBlockRecord(t, w)
	require.NotNil(t, rec.Spec.Series)
	series := *rec.Spec.Series
	require.Len(t, series, 1)

	require.NotNil(t, series[0].OnComplete, "on_complete should be normalized to continue")
	assert.Equal(t, gen.Continue, *series[0].OnComplete)
	require.NotNil(t, series[0].StartSeason, "start_season should be normalized to 1")
	assert.Equal(t, 1, *series[0].StartSeason)
	require.NotNil(t, series[0].StartEpisode, "start_episode should be normalized to 1")
	assert.Equal(t, 1, *series[0].StartEpisode)
}

func TestCreateBlock_InvalidSpec_MissingCron(t *testing.T) {
	h := newTestServer(t)

	w := doRequest(t, h, http.MethodPost, "/blocks", filterBlockWrite("bad-block", ""))
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	require.Equal(t, "application/problem+json", w.Header().Get("Content-Type"))

	p := decodeProblem(t, w)
	assert.Equal(t, http.StatusBadRequest, p.Status)
	assert.Contains(t, strings.ToLower(p.Title), "validation", "title should cite validation")
	assert.Contains(t, strings.ToLower(p.Detail), "cron", "detail should name the missing field")
}

func TestCreateBlock_InvalidSpec_ZeroDuration(t *testing.T) {
	h := newTestServer(t)

	body := filterBlockWrite("bad-duration", "0 6 * * *")
	body.Spec.Duration = 0

	w := doRequest(t, h, http.MethodPost, "/blocks", body)
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())

	p := decodeProblem(t, w)
	assert.Contains(t, strings.ToLower(p.Title), "validation")
	assert.Contains(t, strings.ToLower(p.Detail), "duration")
}

// TestCreateBlock_SeriesEmptyShowTitle_Returns400 covers the deferred
// CUE-validation gap validateSeriesShowTitles closes (see its doc comment
// in blocks.go): cmd/schema/config.cue types show_title as a bare `string`
// with no non-empty constraint, so a series block with an empty
// show_title would otherwise pass blockio.ValidateBlocks cleanly. This is
// the blocks-CRUD half of the two required ingestion-path tests; see
// TestImportBlocks_SeriesEmptyShowTitle_Returns400 (importexport_test.go)
// for the import half.
func TestCreateBlock_SeriesEmptyShowTitle_Returns400(t *testing.T) {
	h := newTestServer(t)

	body := seriesBlockWrite("empty-show-title-block")
	series := *body.Spec.Series
	series[0].ShowTitle = ""
	body.Spec.Series = &series

	w := doRequest(t, h, http.MethodPost, "/blocks", body)
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())

	p := decodeProblem(t, w)
	assert.Equal(t, http.StatusBadRequest, p.Status)
	assert.Contains(t, strings.ToLower(p.Detail), "show_title")
}

func TestCreateBlock_InvalidBody(t *testing.T) {
	h := newTestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/blocks", strings.NewReader("{not json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
}

// TestCreateBlock_UnknownField_Returns400 pins the JSON-decode strictness
// added across the API's request bodies (CreateBlock/UpdateBlock,
// decodeGenerateRequest, PatchSeriesState all now call
// json.Decoder.DisallowUnknownFields): a body carrying a field the wire
// schema doesn't recognize is a client error, not something to silently
// ignore -- the same posture blockio's YAML import already takes via
// yaml.Decoder.KnownFields(true) (internal/blockio/blockio.go).
func TestCreateBlock_UnknownField_Returns400(t *testing.T) {
	h := newTestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/blocks", strings.NewReader(
		`{"spec":{"name":"x","cron":"0 6 * * *","duration":60,"channel_id":"channel-1"},"unknown_field":true}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	p := decodeProblem(t, w)
	assert.Equal(t, http.StatusBadRequest, p.Status)
}

func TestCreateBlock_DuplicateName(t *testing.T) {
	h := newTestServer(t)

	body := filterBlockWrite("dup-block", "0 6 * * *")
	w1 := doRequest(t, h, http.MethodPost, "/blocks", body)
	require.Equal(t, http.StatusCreated, w1.Code, w1.Body.String())

	w2 := doRequest(t, h, http.MethodPost, "/blocks", body)
	require.Equal(t, http.StatusConflict, w2.Code, w2.Body.String())

	p := decodeProblem(t, w2)
	assert.Equal(t, http.StatusConflict, p.Status)
}

func TestGetBlock_NotFound(t *testing.T) {
	h := newTestServer(t)

	w := doRequest(t, h, http.MethodGet, "/blocks/does-not-exist", nil)
	require.Equal(t, http.StatusNotFound, w.Code, w.Body.String())

	p := decodeProblem(t, w)
	assert.Equal(t, http.StatusNotFound, p.Status)
}

func TestUpdateBlock_ChangesCronAndPersists(t *testing.T) {
	h := newTestServer(t)

	w := doRequest(t, h, http.MethodPost, "/blocks", filterBlockWrite("evening-news", "0 18 * * *"))
	require.Equal(t, http.StatusCreated, w.Code)
	created := decodeBlockRecord(t, w)

	wu := doRequest(t, h, http.MethodPut, "/blocks/"+created.Id, filterBlockWrite("evening-news", "0 19 * * *"))
	require.Equal(t, http.StatusOK, wu.Code, wu.Body.String())
	updated := decodeBlockRecord(t, wu)
	assert.Equal(t, "0 19 * * *", updated.Spec.Cron)
	assert.Equal(t, created.Id, updated.Id)
	assert.Equal(t, created.CreatedAt, updated.CreatedAt, "CreatedAt should not change on update")

	wg := doRequest(t, h, http.MethodGet, "/blocks/"+created.Id, nil)
	require.Equal(t, http.StatusOK, wg.Code)
	fetched := decodeBlockRecord(t, wg)
	assert.Equal(t, "0 19 * * *", fetched.Spec.Cron, "store should reflect the update")
}

func TestUpdateBlock_RenameSucceeds(t *testing.T) {
	h := newTestServer(t)

	w := doRequest(t, h, http.MethodPost, "/blocks", filterBlockWrite("old-name", "0 6 * * *"))
	require.Equal(t, http.StatusCreated, w.Code)
	created := decodeBlockRecord(t, w)

	wu := doRequest(t, h, http.MethodPut, "/blocks/"+created.Id, filterBlockWrite("new-name", "0 6 * * *"))
	require.Equal(t, http.StatusOK, wu.Code, wu.Body.String())
	updated := decodeBlockRecord(t, wu)
	assert.Equal(t, "new-name", updated.Name)
	assert.Equal(t, created.Id, updated.Id, "id should be stable across a rename")

	wl := doRequest(t, h, http.MethodGet, "/blocks", nil)
	require.Equal(t, http.StatusOK, wl.Code)
	var list gen.BlockList
	require.NoError(t, json.NewDecoder(wl.Body).Decode(&list))
	require.Len(t, list, 1, "rename should not create a second block")
	assert.Equal(t, "new-name", list[0].Name)
}

func TestUpdateBlock_RenameCollision(t *testing.T) {
	h := newTestServer(t)

	w1 := doRequest(t, h, http.MethodPost, "/blocks", filterBlockWrite("block-one", "0 6 * * *"))
	require.Equal(t, http.StatusCreated, w1.Code)

	w2 := doRequest(t, h, http.MethodPost, "/blocks", filterBlockWrite("block-two", "0 7 * * *"))
	require.Equal(t, http.StatusCreated, w2.Code)
	second := decodeBlockRecord(t, w2)

	wu := doRequest(t, h, http.MethodPut, "/blocks/"+second.Id, filterBlockWrite("block-one", "0 7 * * *"))
	require.Equal(t, http.StatusConflict, wu.Code, wu.Body.String())

	p := decodeProblem(t, wu)
	assert.Equal(t, http.StatusConflict, p.Status)
}

// TestUpdateBlock_OmittedEnabledDefaultsTrue pins UpdateBlock's documented
// full-replace contract (see its doc comment in blocks.go): PUT replaces
// the block's enabled flag along with its spec, using the same
// nil-means-true default CreateBlock applies (blockEnabled) -- there is no
// partial-update path that would let an update silently preserve a
// previously-disabled block's current enabled state. A block created
// disabled, then PUT without an "enabled" field, must come back enabled.
func TestUpdateBlock_OmittedEnabledDefaultsTrue(t *testing.T) {
	h := newTestServer(t)

	body := filterBlockWrite("toggle-me", "0 6 * * *")
	disabled := false
	body.Enabled = &disabled

	w := doRequest(t, h, http.MethodPost, "/blocks", body)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	created := decodeBlockRecord(t, w)
	require.False(t, created.Enabled, "block should be created disabled")

	wu := doRequest(t, h, http.MethodPut, "/blocks/"+created.Id, filterBlockWrite("toggle-me", "0 6 * * *"))
	require.Equal(t, http.StatusOK, wu.Code, wu.Body.String())
	updated := decodeBlockRecord(t, wu)
	assert.True(t, updated.Enabled, "PUT without enabled should default to true, per the full-replace contract")

	// Persisted state must reflect it too, not just the handler's response.
	wg := doRequest(t, h, http.MethodGet, "/blocks/"+created.Id, nil)
	require.Equal(t, http.StatusOK, wg.Code)
	fetched := decodeBlockRecord(t, wg)
	assert.True(t, fetched.Enabled, "store should reflect the re-enabled block")
}

func TestUpdateBlock_NotFound(t *testing.T) {
	h := newTestServer(t)

	w := doRequest(t, h, http.MethodPut, "/blocks/does-not-exist", filterBlockWrite("x", "0 6 * * *"))
	require.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
}

func TestUpdateBlock_InvalidSpec(t *testing.T) {
	h := newTestServer(t)

	w := doRequest(t, h, http.MethodPost, "/blocks", filterBlockWrite("to-update", "0 6 * * *"))
	require.Equal(t, http.StatusCreated, w.Code)
	created := decodeBlockRecord(t, w)

	body := filterBlockWrite("to-update", "0 6 * * *")
	body.Spec.Duration = 0
	wu := doRequest(t, h, http.MethodPut, "/blocks/"+created.Id, body)
	require.Equal(t, http.StatusBadRequest, wu.Code, wu.Body.String())
}

func TestDeleteBlock_ThenNotFound(t *testing.T) {
	h := newTestServer(t)

	w := doRequest(t, h, http.MethodPost, "/blocks", filterBlockWrite("to-delete", "0 6 * * *"))
	require.Equal(t, http.StatusCreated, w.Code)
	created := decodeBlockRecord(t, w)

	wd := doRequest(t, h, http.MethodDelete, "/blocks/"+created.Id, nil)
	require.Equal(t, http.StatusNoContent, wd.Code, wd.Body.String())
	assert.Empty(t, wd.Body.String())

	wg := doRequest(t, h, http.MethodGet, "/blocks/"+created.Id, nil)
	require.Equal(t, http.StatusNotFound, wg.Code)

	wd2 := doRequest(t, h, http.MethodDelete, "/blocks/"+created.Id, nil)
	require.Equal(t, http.StatusNotFound, wd2.Code, "deleting an already-deleted block should 404")
}

// TestUpdateBlock_PreservesOccurrenceSnapshots is the v0.2.3 live-bug
// regression at the wiring level: PUT /blocks/{id} must NOT invalidate
// the block's occurrence snapshots -- not the future ones, not the
// on-air one. Spec edits are seed-preserving (a pending occurrence
// re-derives from its stored seed + the CURRENT spec); the invalidation
// an earlier version performed here deleted the seed, and the seedless
// re-derive fell back to the live cursor -- observed in production as a
// reorder making tonight's occurrence SKIP its already-committed
// episodes. Only DeleteBlock (orphan cleanup) and the cursor-edit paths
// (PATCH /state/series, CLI state set/reset/import) invalidate. See
// TestUpdateBlock_Reorder_PendingOccurrenceKeepsSameEpisodes for the
// end-to-end episode-level consequence.
func TestUpdateBlock_PreservesOccurrenceSnapshots(t *testing.T) {
	h, s := newTestServerWithStore(t)
	ctx := t.Context()

	w := doRequest(t, h, http.MethodPost, "/blocks", seriesBlockWrite("weekend-marathon"))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	created := decodeBlockRecord(t, w)

	future := time.Now().Add(24 * time.Hour)
	onAirStart := time.Now().Add(-10 * time.Minute) // 10min into the 90min block: on air
	snapshot := scheduler.OccurrenceSnapshot{PreStates: map[string]scheduler.SeriesStateSnapshot{"Show A": {CurrentSeason: 1, CurrentEpisode: 1}}}
	require.NoError(t, s.SaveOccurrenceSnapshot(ctx, created.Id, future, snapshot))
	require.NoError(t, s.SaveOccurrenceSnapshot(ctx, created.Id, onAirStart, snapshot))

	wu := doRequest(t, h, http.MethodPut, "/blocks/"+created.Id, seriesBlockWrite("weekend-marathon"))
	require.Equal(t, http.StatusOK, wu.Code, wu.Body.String())

	_, ok, err := s.GetOccurrenceSnapshot(ctx, created.Id, future)
	require.NoError(t, err)
	assert.True(t, ok, "PUT must preserve a pending occurrence's seed -- spec edits re-derive from seed + current spec, they never reset the seed")

	_, ok, err = s.GetOccurrenceSnapshot(ctx, created.Id, onAirStart)
	require.NoError(t, err)
	assert.True(t, ok, "PUT must preserve the on-air occurrence's snapshot too -- its post-state is what advances the cursor when it airs")
}

// TestUpdateBlock_Reorder_PendingOccurrenceKeepsSameEpisodes is the
// v0.2.3 live bug end to end, at the layer that was untested: the REAL
// PUT handler against the REAL store, with the REAL engine planning and
// re-planning the pending occurrence around it. Observed on the cluster:
// tonight's occurrence was committed with each show's E2 (cursors
// legitimately advanced to E3 at plan time); a PUT reorder then deleted
// the occurrence's seed, so the re-derive fell back to the LIVE cursor
// (E3) and re-planned tonight with the E3s -- the committed E2s would
// never have aired. Engine-level reorder tests kept passing because they
// mutate the spec directly with the snapshots intact; it was the handler
// wiring that defeated the seed-preserving semantics. After a reorder,
// the SAME occurrence must re-plan with the SAME episodes in the NEW
// order, and the persisted cursors must not move.
func TestUpdateBlock_Reorder_PendingOccurrenceKeepsSameEpisodes(t *testing.T) {
	h, s := newTestServerWithStore(t)
	ctx := t.Context()

	// Cursors already mid-series: E1s aired some previous night.
	seeded := time.Now().Add(-24 * time.Hour)
	require.NoError(t, s.UpdateSeriesState(ctx, &scheduler.SeriesState{
		ShowTitle: "Alpha", CurrentSeason: 1, CurrentEpisode: 2, LastAired: &seeded,
	}))
	require.NoError(t, s.UpdateSeriesState(ctx, &scheduler.SeriesState{
		ShowTitle: "Beta", CurrentSeason: 1, CurrentEpisode: 2, LastAired: &seeded,
	}))

	catalog := make([]tunarr.Program, 0, 6)
	for _, show := range []string{"Alpha", "Beta"} {
		for ep := 1; ep <= 3; ep++ {
			catalog = append(catalog, tunarr.Program{
				ID: fmt.Sprintf("%s-e%d", strings.ToLower(show), ep), Type: "episode",
				ShowTitle: show, SeasonNumber: 1, EpisodeNumber: ep, Duration: 1_800_000,
			})
		}
	}

	blockWrite := func(series []gen.SeriesConfig) gen.BlockWrite {
		seriesType := gen.BlockSpecTypeSeries
		return gen.BlockWrite{Spec: gen.BlockSpec{
			Name: "tonight", Cron: "0 20 * * *", Duration: 60, ChannelId: "channel-1",
			Type: &seriesType, Series: &series,
		}}
	}
	w := doRequest(t, h, http.MethodPost, "/blocks", blockWrite([]gen.SeriesConfig{
		{ShowTitle: "Alpha", EpisodesPerBlock: 1}, {ShowTitle: "Beta", EpisodesPerBlock: 1},
	}))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	created := decodeBlockRecord(t, w)

	// activeBlock mirrors service.ActiveBlocks: the stored spec plus the
	// record's stable ID (what keys the occurrence snapshots).
	activeBlock := func() scheduler.Block {
		rec, err := s.GetBlock(ctx, created.Id)
		require.NoError(t, err)
		blk := rec.Spec
		blk.ID = rec.ID
		return blk
	}

	// Apply 1: tonight's occurrence (still hours away) commits [alpha-e2,
	// beta-e2] and the plan advances both cursors to E3 -- exactly the
	// production state before the reorder.
	now := time.Now()
	occurrenceStart := now.Add(4 * time.Hour)
	engine := scheduler.NewEngine(&tunarr.Client{}, nil, s, slog.Default(), time.UTC)
	first, err := engine.PlanBlock(activeBlock(), catalog, occurrenceStart, now)
	require.NoError(t, err)
	require.NoError(t, engine.Commit())
	require.Equal(t, []string{"alpha-e2", "beta-e2"}, planProgramIDs(first))

	// The reorder, through the real handler.
	wu := doRequest(t, h, http.MethodPut, "/blocks/"+created.Id, blockWrite([]gen.SeriesConfig{
		{ShowTitle: "Beta", EpisodesPerBlock: 1}, {ShowTitle: "Alpha", EpisodesPerBlock: 1},
	}))
	require.Equal(t, http.StatusOK, wu.Code, wu.Body.String())

	// Re-apply: the SAME occurrence re-derives from its preserved seed
	// (E2s) + the NEW spec order -- same episodes, new arrangement.
	second, err := engine.PlanBlock(activeBlock(), catalog, occurrenceStart, now.Add(time.Minute))
	require.NoError(t, err)
	require.NoError(t, engine.Commit())
	assert.Equal(t, []string{"beta-e2", "alpha-e2"}, planProgramIDs(second),
		"a reorder must re-plan the pending occurrence with the SAME episodes in the NEW order -- never skip to the live cursor's E3s")

	committed, ok, err := s.GetCommittedOccurrence(ctx, "tonight", occurrenceStart)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, []string{"beta-e2", "alpha-e2"}, planProgramIDs(committed), "the committed assignment must reflect the re-derive")

	for _, show := range []string{"Alpha", "Beta"} {
		state, err := s.GetPersistedSeriesState(ctx, show)
		require.NoError(t, err)
		assert.Equal(t, 3, state.CurrentEpisode, "%s: the persisted cursor must be unchanged by the reorder + re-apply", show)
	}
}

// planProgramIDs extracts program IDs (GetID) from a planned assignment.
func planProgramIDs(programs []tunarr.Program) []string {
	ids := make([]string, 0, len(programs))
	for _, p := range programs {
		ids = append(ids, p.GetID())
	}
	return ids
}

// TestDeleteBlock_InvalidatesFutureOccurrenceSnapshots pins DeleteBlock's
// orphan cleanup (a deleted block ID must not leave snapshot rows
// behind); TestDeleteBlock_InvalidatesOnAirOccurrenceSnapshot below pins
// its widened, on-air-inclusive cutoff (store.InvalidationCutoff).
func TestDeleteBlock_InvalidatesFutureOccurrenceSnapshots(t *testing.T) {
	h, s := newTestServerWithStore(t)
	ctx := t.Context()

	w := doRequest(t, h, http.MethodPost, "/blocks", seriesBlockWrite("weekend-marathon"))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	created := decodeBlockRecord(t, w)

	future := time.Now().Add(24 * time.Hour)
	snapshot := scheduler.OccurrenceSnapshot{PreStates: map[string]scheduler.SeriesStateSnapshot{"Show A": {CurrentSeason: 1, CurrentEpisode: 1}}}
	require.NoError(t, s.SaveOccurrenceSnapshot(ctx, created.Id, future, snapshot))

	wd := doRequest(t, h, http.MethodDelete, "/blocks/"+created.Id, nil)
	require.Equal(t, http.StatusNoContent, wd.Code)

	_, ok, err := s.GetOccurrenceSnapshot(ctx, created.Id, future)
	require.NoError(t, err)
	assert.False(t, ok, "DELETE must invalidate every not-yet-aired occurrence snapshot for the deleted block")
}

func TestDeleteBlock_InvalidatesOnAirOccurrenceSnapshot(t *testing.T) {
	h, s := newTestServerWithStore(t)
	ctx := t.Context()

	w := doRequest(t, h, http.MethodPost, "/blocks", seriesBlockWrite("weekend-marathon"))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	created := decodeBlockRecord(t, w)

	onAirStart := time.Now().Add(-10 * time.Minute)
	snapshot := scheduler.OccurrenceSnapshot{PreStates: map[string]scheduler.SeriesStateSnapshot{"Show A": {CurrentSeason: 1, CurrentEpisode: 1}}}
	require.NoError(t, s.SaveOccurrenceSnapshot(ctx, created.Id, onAirStart, snapshot))

	wd := doRequest(t, h, http.MethodDelete, "/blocks/"+created.Id, nil)
	require.Equal(t, http.StatusNoContent, wd.Code)

	_, ok, err := s.GetOccurrenceSnapshot(ctx, created.Id, onAirStart)
	require.NoError(t, err)
	assert.False(t, ok, "DELETE must invalidate the on-air occurrence's own snapshot too, not just strictly future ones")
}

// corruptOccurrenceSnapshotsTable drops the series_occurrence_snapshots
// table via a raw connection to the SAME sqlite file dsn points at, so a
// later DeleteFutureOccurrenceSnapshots call against a *store.Store
// opened on that same file fails -- without touching any OTHER table
// (UpdateBlock/DeleteBlock/CreateBlock/UpdateSeriesState never reference
// series_occurrence_snapshots), letting a test isolate "the primary
// mutation succeeds but the post-mutation invalidation step fails"
// (round-3 finding 7) without making the WHOLE store unusable the way
// newClosedStoreServer's closed connection would (that would fail the
// primary mutation too, not just the invalidation step).
func corruptOccurrenceSnapshotsTable(t *testing.T, dsn string) {
	t.Helper()
	db, err := sql.Open("sqlite3", dsn)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	_, err = db.Exec(`DROP TABLE series_occurrence_snapshots`)
	require.NoError(t, err, "test setup: failed to drop series_occurrence_snapshots")
}

// TestDeleteBlock_SucceedsEvenWhenSnapshotInvalidationFails is round-3
// finding 7's regression (UpdateBlock's variant was retired in v0.2.3
// along with its invalidation -- spec edits no longer touch snapshots at
// all): DeleteBlock logs (not 500s) a failed post-mutation snapshot
// cleanup, since the primary mutation has already committed by then.
// corruptOccurrenceSnapshotsTable makes DeleteFutureOccurrenceSnapshots
// specifically fail while every other store call keeps working.
func TestDeleteBlock_SucceedsEvenWhenSnapshotInvalidationFails(t *testing.T) {
	dsn := filepath.Join(t.TempDir(), "test.db")
	s, err := store.New(dsn)
	require.NoError(t, err, "failed to create test store")
	t.Cleanup(func() { _ = s.Close() })
	h := gen.HandlerFromMux(NewHandlers(Deps{Store: s, Logger: slog.Default(), Version: "test"}), chi.NewRouter())

	w := doRequest(t, h, http.MethodPost, "/blocks", seriesBlockWrite("weekend-marathon"))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	created := decodeBlockRecord(t, w)

	corruptOccurrenceSnapshotsTable(t, dsn)

	wd := doRequest(t, h, http.MethodDelete, "/blocks/"+created.Id, nil)
	assert.Equal(t, http.StatusNoContent, wd.Code, wd.Body.String())

	wg := doRequest(t, h, http.MethodGet, "/blocks/"+created.Id, nil)
	assert.Equal(t, http.StatusNotFound, wg.Code, "the delete itself must still have succeeded")
}

func TestListBlocks_SortedByNameAndEmpty(t *testing.T) {
	h := newTestServer(t)

	wEmpty := doRequest(t, h, http.MethodGet, "/blocks", nil)
	require.Equal(t, http.StatusOK, wEmpty.Code)
	var empty gen.BlockList
	require.NoError(t, json.NewDecoder(wEmpty.Body).Decode(&empty))
	assert.Empty(t, empty)

	w1 := doRequest(t, h, http.MethodPost, "/blocks", filterBlockWrite("zeta-block", "0 6 * * *"))
	require.Equal(t, http.StatusCreated, w1.Code)
	w2 := doRequest(t, h, http.MethodPost, "/blocks", filterBlockWrite("alpha-block", "0 7 * * *"))
	require.Equal(t, http.StatusCreated, w2.Code)

	w := doRequest(t, h, http.MethodGet, "/blocks", nil)
	require.Equal(t, http.StatusOK, w.Code)
	var list gen.BlockList
	require.NoError(t, json.NewDecoder(w.Body).Decode(&list))
	require.Len(t, list, 2)
	assert.Equal(t, []string{"alpha-block", "zeta-block"}, []string{list[0].Name, list[1].Name})
}

// --- fromGen/toGen unit-level coverage -------------------------------------
//
// The HTTP-level tests above exercise fromGen/toGen indirectly through
// every handler; these two add direct, field-level assertions on the
// CUE-default normalization documented on fromGen, since that behavior is
// the carry-forward risk this task was explicitly warned about.

func TestFromGen_NormalizesOmittedEnumsAndPositiveInts(t *testing.T) {
	seriesType := gen.BlockSpecType(gen.BlockSpecTypeSeries)
	spec := gen.BlockSpec{
		Name:      "s1",
		Cron:      "0 6 * * *",
		Duration:  30,
		ChannelId: "chan-1",
		Type:      &seriesType,
		Series: &[]gen.SeriesConfig{
			{ShowTitle: "Show", EpisodesPerBlock: 2}, // OnComplete/StartSeason/StartEpisode all omitted
		},
	}

	b := fromGen(spec)
	require.Len(t, b.Series, 1)
	assert.Equal(t, scheduler.CompletionActionContinue, b.Series[0].OnComplete)
	assert.Equal(t, 1, b.Series[0].StartSeason)
	assert.Equal(t, 1, b.Series[0].StartEpisode)
}

func TestFromGen_PreservesExplicitValues(t *testing.T) {
	filterType := gen.BlockSpecTypeFilter
	spec := gen.BlockSpec{
		Name:      "f1",
		Cron:      "0 6 * * *",
		Duration:  30,
		ChannelId: "chan-1",
		Type:      &filterType,
	}

	b := fromGen(spec)
	assert.Equal(t, scheduler.BlockTypeFilter, b.Type)
	assert.Equal(t, "f1", b.Name)
	assert.Equal(t, 30, b.Duration)
}

func TestFromGen_DefaultsMissingType(t *testing.T) {
	spec := gen.BlockSpec{
		Name:      "f1",
		Cron:      "0 6 * * *",
		Duration:  30,
		ChannelId: "chan-1",
		// Type omitted entirely
	}

	b := fromGen(spec)
	assert.Equal(t, scheduler.BlockTypeFilter, b.Type)
}

// --- 500 responses must not leak internal error strings --------------------
//
// Both tests force a genuine, unmocked internal error by closing the
// store's underlying sqlite handle before issuing the request: every
// subsequent store call then fails with a real driver-level error (roughly
// "sql: database is closed"), the same shape of error a production
// sqlite/sqlx failure would produce. That exercises the two 500 call sites
// blocks.go actually has -- ListBlocks' direct logAndWriteInternalError
// call, and writeBlockStoreError's default branch (hit here via GetBlock)
// -- with a real error, rather than asserting against a hand-rolled fake.

func newClosedStoreServer(t *testing.T) http.Handler {
	t.Helper()
	s, err := store.New(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err, "failed to create test store")
	require.NoError(t, s.Close(), "failed to close test store")

	h := NewHandlers(Deps{Store: s, Logger: slog.Default(), Version: "test"})
	return gen.HandlerFromMux(h, chi.NewRouter())
}

func assertGenericInternalErrorProblem(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	require.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())
	require.Equal(t, "application/problem+json", w.Header().Get("Content-Type"))

	p := decodeProblem(t, w)
	assert.Equal(t, http.StatusInternalServerError, p.Status)
	assert.Equal(t, "internal server error", p.Title)
	assert.Empty(t, p.Detail, "500 detail must not leak the underlying store/driver error")
	lower := strings.ToLower(p.Detail)
	assert.NotContains(t, lower, "sql")
	assert.NotContains(t, lower, "database")
}

func TestListBlocks_InternalErrorDoesNotLeakDetail(t *testing.T) {
	h := newClosedStoreServer(t)

	w := doRequest(t, h, http.MethodGet, "/blocks", nil)
	assertGenericInternalErrorProblem(t, w)
}

func TestGetBlock_InternalErrorDoesNotLeakDetail(t *testing.T) {
	h := newClosedStoreServer(t)

	w := doRequest(t, h, http.MethodGet, "/blocks/some-id", nil)
	assertGenericInternalErrorProblem(t, w)
}

// seriesBlockWriteWithPolicy is seriesBlockWrite with an explicit show
// title and on_complete policy, for the shared-show agreement tests.
func seriesBlockWriteWithPolicy(name, show string, policy gen.SeriesConfigOnComplete) gen.BlockWrite {
	seriesType := gen.BlockSpecTypeSeries
	return gen.BlockWrite{
		Spec: gen.BlockSpec{
			Name:      name,
			Cron:      "0 20 * * 6",
			Duration:  90,
			ChannelId: "channel-1",
			Type:      &seriesType,
			Series: &[]gen.SeriesConfig{
				{ShowTitle: show, EpisodesPerBlock: 1, OnComplete: &policy},
			},
		},
	}
}

// TestCreateBlock_ContradictoryOnCompleteRejected pins the shared-show
// policy agreement rule (blockio.ValidateOnCompleteAgreement): two enabled
// blocks giving the same show different on_complete policies fight over
// the show's shared series state, so the second write is rejected with a
// 400 naming the conflict.
func TestCreateBlock_ContradictoryOnCompleteRejected(t *testing.T) {
	h := newTestServer(t)

	w := doRequest(t, h, http.MethodPost, "/blocks", seriesBlockWriteWithPolicy("first", "Shared Show", gen.Restart))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	w = doRequest(t, h, http.MethodPost, "/blocks", seriesBlockWriteWithPolicy("second", "Shared Show", gen.Disable))
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "contradictory on_complete")
	assert.Contains(t, w.Body.String(), "Shared Show")

	// The same policy (or the equivalent default) is fine.
	w = doRequest(t, h, http.MethodPost, "/blocks", seriesBlockWriteWithPolicy("third", "Shared Show", gen.Restart))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
}

// TestCreateBlock_ContradictoryOnCompleteAllowedWhenOtherDisabled: a
// disabled block plans nothing and fights nobody, so only ENABLED blocks
// participate in the agreement check.
func TestCreateBlock_ContradictoryOnCompleteAllowedWhenOtherDisabled(t *testing.T) {
	h := newTestServer(t)

	disabled := false
	first := seriesBlockWriteWithPolicy("first", "Shared Show", gen.Restart)
	first.Enabled = &disabled
	w := doRequest(t, h, http.MethodPost, "/blocks", first)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	created := decodeBlockRecord(t, w)

	w = doRequest(t, h, http.MethodPost, "/blocks", seriesBlockWriteWithPolicy("second", "Shared Show", gen.Disable))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	// But re-ENABLING the first block now must be rejected: it would
	// bring the contradiction live.
	enable := seriesBlockWriteWithPolicy("first", "Shared Show", gen.Restart)
	w = doRequest(t, h, http.MethodPut, "/blocks/"+created.Id, enable)
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "contradictory on_complete")
}
