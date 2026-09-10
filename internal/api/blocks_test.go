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

// doRequestWithHeaders is doRequest plus caller-supplied headers -- the
// If-Match a PUT now requires, above all.
func doRequestWithHeaders(t *testing.T, h http.Handler, method, path string, body any, headers map[string]string) *httptest.ResponseRecorder {
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
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

// putBlock does what a real client does before a full-spec save: read
// the block for its current updated_at, then send that back as If-Match.
// A 404 on the read is passed through, so "update a block that isn't
// there" still reaches the handler and still 404s.
func putBlock(t *testing.T, h http.Handler, id string, body any) *httptest.ResponseRecorder {
	t.Helper()

	read := doRequest(t, h, http.MethodGet, "/blocks/"+id, nil)
	if read.Code != http.StatusOK {
		return doRequestWithHeaders(t, h, http.MethodPut, "/blocks/"+id, body,
			map[string]string{"If-Match": time.Now().UTC().Format(time.RFC3339Nano)})
	}

	current := decodeBlockRecord(t, read)
	return doRequestWithHeaders(t, h, http.MethodPut, "/blocks/"+id, body,
		map[string]string{"If-Match": current.UpdatedAt.UTC().Format(time.RFC3339Nano)})
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

	wu := putBlock(t, h, created.Id, filterBlockWrite("evening-news", "0 19 * * *"))
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

	wu := putBlock(t, h, created.Id, filterBlockWrite("new-name", "0 6 * * *"))
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

	wu := putBlock(t, h, second.Id, filterBlockWrite("block-one", "0 7 * * *"))
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

	wu := putBlock(t, h, created.Id, filterBlockWrite("toggle-me", "0 6 * * *"))
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

	w := putBlock(t, h, "does-not-exist", filterBlockWrite("x", "0 6 * * *"))
	require.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
}

func TestUpdateBlock_InvalidSpec(t *testing.T) {
	h := newTestServer(t)

	w := doRequest(t, h, http.MethodPost, "/blocks", filterBlockWrite("to-update", "0 6 * * *"))
	require.Equal(t, http.StatusCreated, w.Code)
	created := decodeBlockRecord(t, w)

	body := filterBlockWrite("to-update", "0 6 * * *")
	body.Spec.Duration = 0
	wu := putBlock(t, h, created.Id, body)
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

	wu := putBlock(t, h, created.Id, seriesBlockWrite("weekend-marathon"))
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
	wu := putBlock(t, h, created.Id, blockWrite([]gen.SeriesConfig{
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
	assert.Contains(t, w.Body.String(), "contradictory completion policy")
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
	w = putBlock(t, h, created.Id, enable)
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "contradictory completion policy")
}

// TestUpdateBlock_ChangingOwnSharedShowPolicySucceeds pins
// checkSharedShowPolicies' excludeID: a PUT that changes a shared show's
// policy must not 400 against the block's OWN stored spec (which the
// update is replacing).
func TestUpdateBlock_ChangingOwnSharedShowPolicySucceeds(t *testing.T) {
	h := newTestServer(t)

	w := doRequest(t, h, http.MethodPost, "/blocks", seriesBlockWriteWithPolicy("solo", "Solo Show", gen.Restart))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	created := decodeBlockRecord(t, w)

	w = putBlock(t, h, created.Id, seriesBlockWriteWithPolicy("solo", "Solo Show", gen.Disable))
	require.Equal(t, http.StatusOK, w.Code,
		"changing a block's own policy must not conflict with the stored spec it replaces: %s", w.Body.String())
}

// TestUpdateBlock_DisablingSkipsSharedShowPolicyCheck: a PUT that leaves
// the block disabled is never policy-checked -- a disabled block plans
// nothing and fights nobody, even when its spec contradicts a live one.
func TestUpdateBlock_DisablingSkipsSharedShowPolicyCheck(t *testing.T) {
	h := newTestServer(t)

	w := doRequest(t, h, http.MethodPost, "/blocks", seriesBlockWriteWithPolicy("live", "Shared Show", gen.Restart))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	w = doRequest(t, h, http.MethodPost, "/blocks", seriesBlockWriteWithPolicy("other", "Other Show", gen.Restart))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	other := decodeBlockRecord(t, w)

	// Repoint "other" at the shared show with a CONTRADICTORY policy --
	// but disabled, which must succeed.
	disabled := false
	contradicting := seriesBlockWriteWithPolicy("other", "Shared Show", gen.Disable)
	contradicting.Enabled = &disabled
	w = putBlock(t, h, other.Id, contradicting)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

// ---- lost-update protection ------------------------------------------------

func seedOneBlock(t *testing.T, h http.Handler) gen.BlockRecord {
	t.Helper()
	w := doRequest(t, h, http.MethodPost, "/blocks", filterBlockWrite("guarded", "0 6 * * *"))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	return decodeBlockRecord(t, w)
}

func TestUpdateBlock_RefusesAPutWithNoIfMatch(t *testing.T) {
	h := newTestServer(t)
	rec := seedOneBlock(t, h)

	w := doRequest(t, h, http.MethodPut, "/blocks/"+rec.Id, filterBlockWrite("guarded", "0 7 * * *"))
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
}

func TestUpdateBlock_412WhenSomeoneElseSavedFirst(t *testing.T) {
	h := newTestServer(t)
	rec := seedOneBlock(t, h)

	// Two tabs read the same block; the first saves.
	first := putBlock(t, h, rec.Id, filterBlockWrite("guarded", "0 7 * * *"))
	require.Equal(t, http.StatusOK, first.Code, first.Body.String())

	// The second still holds the stale updated_at it read at load.
	stale := rec.UpdatedAt.UTC().Format(time.RFC3339Nano)
	second := doRequestWithHeaders(t, h, http.MethodPut, "/blocks/"+rec.Id,
		filterBlockWrite("guarded", "0 8 * * *"), map[string]string{"If-Match": stale})

	require.Equal(t, http.StatusPreconditionFailed, second.Code, second.Body.String())
	assert.Contains(t, second.Body.String(), "Reload", "the problem says how to recover")

	// The first tab's edit survived; the second's was refused, not merged.
	read := doRequest(t, h, http.MethodGet, "/blocks/"+rec.Id, nil)
	assert.Equal(t, "0 7 * * *", decodeBlockRecord(t, read).Spec.Cron)
}

func TestUpdateBlock_AcceptsAQuotedIfMatch(t *testing.T) {
	// Callers reasonably treat this as an entity-tag and quote it.
	h := newTestServer(t)
	rec := seedOneBlock(t, h)

	w := doRequestWithHeaders(t, h, http.MethodPut, "/blocks/"+rec.Id,
		filterBlockWrite("guarded", "0 9 * * *"),
		map[string]string{"If-Match": `"` + rec.UpdatedAt.UTC().Format(time.RFC3339Nano) + `"`})
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

func TestUpdateBlock_MalformedIfMatchIs400(t *testing.T) {
	h := newTestServer(t)
	rec := seedOneBlock(t, h)

	w := doRequestWithHeaders(t, h, http.MethodPut, "/blocks/"+rec.Id,
		filterBlockWrite("guarded", "0 9 * * *"),
		map[string]string{"If-Match": "last tuesday"})
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
}

// ---- field-scoped writes ---------------------------------------------------

func TestPatchBlock_TogglesEnabledWithoutTouchingTheSpec(t *testing.T) {
	h := newTestServer(t)
	rec := seedOneBlock(t, h)
	require.True(t, rec.Enabled)

	w := doRequest(t, h, http.MethodPatch, "/blocks/"+rec.Id, map[string]any{"enabled": false})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	got := decodeBlockRecord(t, w)
	assert.False(t, got.Enabled)
	assert.Equal(t, rec.Spec.Cron, got.Spec.Cron, "a toggle must not rewrite the spec")
	assert.Equal(t, rec.Spec.Name, got.Spec.Name)
}

func TestPatchBlock_NeedsNoIfMatch(t *testing.T) {
	// The point of the field-scoped write: it cannot clobber an unrelated
	// edit, so it does not need the guard that protects one.
	h := newTestServer(t)
	rec := seedOneBlock(t, h)

	require.Equal(t, http.StatusOK,
		putBlock(t, h, rec.Id, filterBlockWrite("guarded", "0 7 * * *")).Code)

	w := doRequest(t, h, http.MethodPatch, "/blocks/"+rec.Id, map[string]any{"enabled": false})
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

func TestPatchBlock_EmptyPatchIs400(t *testing.T) {
	h := newTestServer(t)
	rec := seedOneBlock(t, h)

	w := doRequest(t, h, http.MethodPatch, "/blocks/"+rec.Id, map[string]any{})
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
}

func TestPatchBlock_UnknownIdIs404(t *testing.T) {
	h := newTestServer(t)
	w := doRequest(t, h, http.MethodPatch, "/blocks/does-not-exist", map[string]any{"enabled": false})
	assert.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
}

// ---- next_occurrence -------------------------------------------------------
//
// next_occurrence answers "when does this block actually air next", which
// is a different question from "what does its cron say next": both dark
// switches are in the answer, and an expression that never fires has no
// answer at all rather than an error.

func TestBlockRecord_CarriesItsNextOccurrence(t *testing.T) {
	h := newTestServer(t)

	w := doRequest(t, h, http.MethodPost, "/blocks", filterBlockWrite("morning", "0 6 * * *"))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	rec := decodeBlockRecord(t, w)

	require.NotNil(t, rec.NextOccurrence, "next_occurrence absent for an enabled block")
	assert.True(t, rec.NextOccurrence.After(time.Now()), "next_occurrence %v is in the past", *rec.NextOccurrence)
	assert.Equal(t, 6, rec.NextOccurrence.In(time.Local).Hour(), "the occurrence should land on the cron's hour")
}

func TestBlockRecord_DisabledBlockReportsNoNextOccurrence(t *testing.T) {
	h := newTestServer(t)

	w := doRequest(t, h, http.MethodPost, "/blocks", filterBlockWrite("off", "0 6 * * *"))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	created := decodeBlockRecord(t, w)

	wp := doRequest(t, h, http.MethodPatch, "/blocks/"+created.Id, map[string]any{"enabled": false})
	require.Equal(t, http.StatusOK, wp.Code, wp.Body.String())

	rec := decodeBlockRecord(t, doRequest(t, h, http.MethodGet, "/blocks/"+created.Id, nil))
	assert.Nil(t, rec.NextOccurrence, "a block an operator switched off has no next airing to report")
}

func TestBlockRecord_DarkBlockReportsTheFirstOccurrenceAfterItWakes(t *testing.T) {
	h := newTestServer(t)

	w := doRequest(t, h, http.MethodPost, "/blocks", filterBlockWrite("dark", "0 6 * * *"))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	created := decodeBlockRecord(t, w)

	wake := time.Now().Add(10 * 24 * time.Hour).UTC().Truncate(time.Second)
	wp := doRequest(t, h, http.MethodPatch, "/blocks/"+created.Id,
		map[string]any{"disabled_until": wake.Format(time.RFC3339)})
	require.Equal(t, http.StatusOK, wp.Code, wp.Body.String())

	rec := decodeBlockRecord(t, wp)
	require.NotNil(t, rec.NextOccurrence, "a dark block comes back on its own, so it has a next airing")
	assert.False(t, rec.NextOccurrence.Before(wake),
		"next_occurrence %v is before the block wakes at %v -- the planning gate would skip that tick",
		*rec.NextOccurrence, wake)
}

// TestBlockRecord_DarkBlockIncludesAnOccurrenceExactlyAtItsWakeInstant pins
// the one-second search anchor in nextOccurrenceFor: NextOccurrences is
// strictly-after, so a block waking at 06:00 with a 06:00 cron would
// otherwise report tomorrow's tick and skip the one it actually airs.
func TestBlockRecord_DarkBlockIncludesAnOccurrenceExactlyAtItsWakeInstant(t *testing.T) {
	rec := store.BlockRecord{Enabled: true, Spec: scheduler.Block{Cron: "0 6 * * *"}}
	wake := time.Date(2026, 9, 20, 6, 0, 0, 0, time.UTC)
	rec.DisabledUntil = &wake

	got := nextOccurrenceFor(rec, time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC), time.UTC)
	require.NotNil(t, got)
	assert.True(t, got.Equal(wake), "got %v, want the tick at the wake instant itself (%v)", *got, wake)
}

func TestBlockRecord_UnfireableCronReportsNoNextOccurrence(t *testing.T) {
	h := newTestServer(t)

	// February 30th is a well-formed expression that never fires.
	w := doRequest(t, h, http.MethodPost, "/blocks", filterBlockWrite("never", "0 0 30 2 *"))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	rec := decodeBlockRecord(t, w)
	assert.Nil(t, rec.NextOccurrence, "February 30th never comes, so there is nothing to report")
}

// ---- the timed dark switch -------------------------------------------------

func TestUpdateBlock_PreservesAnExistingDarkWindow(t *testing.T) {
	h := newTestServer(t)

	post := doRequest(t, h, http.MethodPost, "/blocks", filterBlockWrite("dark-put", "0 6 * * *"))
	require.Equal(t, http.StatusCreated, post.Code, post.Body.String())
	created := decodeBlockRecord(t, post)

	until := time.Now().Add(48 * time.Hour).UTC().Truncate(time.Second)
	patched := decodeBlockRecord(t, doRequest(t, h, http.MethodPatch, "/blocks/"+created.Id,
		map[string]any{"disabled_until": until.Format(time.RFC3339)}))
	require.NotNil(t, patched.DisabledUntil)

	// A full spec replacement must not silently clear the dark window.
	// store.UpdateBlock writes EVERY column from the record, so this holds
	// only because the handler mutates the record it loaded rather than
	// rebuilding one. Rebuilding it would nil the column with no test to
	// notice -- which is exactly why this test exists.
	w := putBlock(t, h, created.Id, filterBlockWrite("dark-put", "0 7 * * *"))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	after := decodeBlockRecord(t, doRequest(t, h, http.MethodGet, "/blocks/"+created.Id, nil))
	require.NotNil(t, after.DisabledUntil, "PUT cleared the dark window")
	assert.True(t, after.DisabledUntil.Equal(until), "dark window moved: got %v want %v", after.DisabledUntil, until)
	assert.Equal(t, "0 7 * * *", after.Spec.Cron, "the spec edit itself did not land")
}

func TestPatchBlock_SetsAndClearsDisabledUntil(t *testing.T) {
	h := newTestServer(t)
	rec := seedOneBlock(t, h)

	until := time.Now().Add(48 * time.Hour).UTC().Truncate(time.Second)
	w := doRequest(t, h, http.MethodPatch, "/blocks/"+rec.Id,
		map[string]any{"disabled_until": until.Format(time.RFC3339)})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	set := decodeBlockRecord(t, w)
	require.NotNil(t, set.DisabledUntil, "disabled_until not set")
	assert.True(t, set.DisabledUntil.Equal(until))

	// An explicit null is the only way an operator brings a block back
	// early, so it must clear rather than read as "leave it alone".
	w = doRequest(t, h, http.MethodPatch, "/blocks/"+rec.Id, map[string]any{"disabled_until": nil})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Nil(t, decodeBlockRecord(t, w).DisabledUntil, "an explicit null must clear the dark window")

	// And it must be persisted, not just reflected in the response.
	assert.Nil(t, decodeBlockRecord(t, doRequest(t, h, http.MethodGet, "/blocks/"+rec.Id, nil)).DisabledUntil)
}

func TestPatchBlock_OnlyEnabledLeavesTheDarkWindowAlone(t *testing.T) {
	h := newTestServer(t)
	rec := seedOneBlock(t, h)

	until := time.Now().Add(48 * time.Hour).UTC().Truncate(time.Second)
	require.Equal(t, http.StatusOK, doRequest(t, h, http.MethodPatch, "/blocks/"+rec.Id,
		map[string]any{"disabled_until": until.Format(time.RFC3339)}).Code)

	w := doRequest(t, h, http.MethodPatch, "/blocks/"+rec.Id, map[string]any{"enabled": false})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	got := decodeBlockRecord(t, w)
	require.NotNil(t, got.DisabledUntil, "an absent disabled_until key cleared the dark window -- the axes are independent")
	assert.True(t, got.DisabledUntil.Equal(until))
	assert.False(t, got.Enabled, "enabled not applied")
}

func TestPatchBlock_OnlyDisabledUntilLeavesEnabledAlone(t *testing.T) {
	h := newTestServer(t)
	rec := seedOneBlock(t, h)

	require.Equal(t, http.StatusOK,
		doRequest(t, h, http.MethodPatch, "/blocks/"+rec.Id, map[string]any{"enabled": false}).Code)

	until := time.Now().Add(48 * time.Hour).UTC().Truncate(time.Second)
	w := doRequest(t, h, http.MethodPatch, "/blocks/"+rec.Id,
		map[string]any{"disabled_until": until.Format(time.RFC3339)})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	got := decodeBlockRecord(t, w)
	assert.False(t, got.Enabled, "an absent enabled key re-enabled the block -- the axes are independent")
	require.NotNil(t, got.DisabledUntil)
}

func TestPatchBlock_MalformedDisabledUntilIs400(t *testing.T) {
	// Silently ignoring it would leave the block awake while the operator
	// believes they just took it dark.
	h := newTestServer(t)
	rec := seedOneBlock(t, h)

	w := doRequest(t, h, http.MethodPatch, "/blocks/"+rec.Id, map[string]any{"disabled_until": "last tuesday"})
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
}

// ---- a bad cron is caught at write time ------------------------------------

func TestCreateBlock_UnparseableCronIs400(t *testing.T) {
	h := newTestServer(t)

	w := doRequest(t, h, http.MethodPost, "/blocks", filterBlockWrite("bad-cron", "not a cron"))
	require.Equal(t, http.StatusBadRequest, w.Code,
		"a bad cron must be caught at write time, not as a 502 from the engine at apply time: %s", w.Body.String())

	p := decodeProblem(t, w)
	assert.Contains(t, strings.ToLower(p.Detail), "cron", "the problem body must name the field: %s", w.Body)
}

func TestUpdateBlock_UnparseableCronIs400(t *testing.T) {
	h := newTestServer(t)
	rec := seedOneBlock(t, h)

	w := putBlock(t, h, rec.Id, filterBlockWrite("guarded", "75 99 * * *"))
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())

	// The stored block is untouched: a rejected write changes nothing.
	assert.Equal(t, "0 6 * * *", decodeBlockRecord(t, doRequest(t, h, http.MethodGet, "/blocks/"+rec.Id, nil)).Spec.Cron)
}

// ---- duplicate -------------------------------------------------------------

func duplicateBlock(t *testing.T, h http.Handler, id, name string) *httptest.ResponseRecorder {
	t.Helper()
	return doRequest(t, h, http.MethodPost, "/blocks/"+id+"/duplicate", map[string]any{"name": name})
}

func TestDuplicateBlock_CopiesTheSpecAndArrivesDisabled(t *testing.T) {
	h := newTestServer(t)

	w := doRequest(t, h, http.MethodPost, "/blocks", filterBlockWrite("morning", "0 6 * * *"))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	source := decodeBlockRecord(t, w)

	wd := duplicateBlock(t, h, source.Id, "copy of morning")
	require.Equal(t, http.StatusCreated, wd.Code, wd.Body.String())

	copied := decodeBlockRecord(t, wd)
	assert.NotEqual(t, source.Id, copied.Id, "the copy reused the source's id")
	assert.Equal(t, "copy of morning", copied.Name)
	assert.Equal(t, "copy of morning", copied.Spec.Name, "the spec's own name must follow the record's")
	assert.False(t, copied.Enabled,
		"the copy arrived enabled -- it would contend with its source at the same cron on the same channel")
	assert.Nil(t, copied.DisabledUntil, "a copy arrives with no dark window of its own")
	assert.Equal(t, source.Spec.Cron, copied.Spec.Cron)
	assert.Equal(t, source.Spec.ChannelId, copied.Spec.ChannelId)
	assert.Equal(t, source.Spec.Duration, copied.Spec.Duration)
	assert.False(t, copied.CreatedAt.IsZero())

	// The source is left exactly as it was.
	stillThere := decodeBlockRecord(t, doRequest(t, h, http.MethodGet, "/blocks/"+source.Id, nil))
	assert.True(t, stillThere.Enabled, "duplicating must not disturb the source")
}

func TestDuplicateBlock_CarriesSeriesSeeds(t *testing.T) {
	h := newTestServer(t)

	w := doRequest(t, h, http.MethodPost, "/blocks", seriesBlockWrite("anime-night"))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	source := decodeBlockRecord(t, w)

	wd := duplicateBlock(t, h, source.Id, "anime-night-b")
	require.Equal(t, http.StatusCreated, wd.Code, wd.Body.String())

	copied := decodeBlockRecord(t, wd)
	require.NotNil(t, copied.Spec.Series, "series seeds not copied -- that is what makes it a duplicate")
	series := *copied.Spec.Series
	require.Len(t, series, 1)
	assert.Equal(t, "Show A", series[0].ShowTitle)
	assert.Equal(t, 2, series[0].EpisodesPerBlock)
	require.NotNil(t, copied.Spec.Type)
	assert.Equal(t, gen.BlockSpecTypeSeries, *copied.Spec.Type)
}

func TestDuplicateBlock_NameCollisionIs409(t *testing.T) {
	h := newTestServer(t)

	w := doRequest(t, h, http.MethodPost, "/blocks", filterBlockWrite("morning", "0 6 * * *"))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	source := decodeBlockRecord(t, w)

	require.Equal(t, http.StatusCreated,
		doRequest(t, h, http.MethodPost, "/blocks", filterBlockWrite("taken", "0 7 * * *")).Code)

	// The server never invents a name, so a collision is the caller's to
	// resolve -- the same 409 a colliding create gets.
	wd := duplicateBlock(t, h, source.Id, "taken")
	assert.Equal(t, http.StatusConflict, wd.Code, wd.Body.String())
}

func TestDuplicateBlock_EmptyNameIs400(t *testing.T) {
	h := newTestServer(t)

	w := doRequest(t, h, http.MethodPost, "/blocks", filterBlockWrite("morning", "0 6 * * *"))
	require.Equal(t, http.StatusCreated, w.Code)
	source := decodeBlockRecord(t, w)

	assert.Equal(t, http.StatusBadRequest, duplicateBlock(t, h, source.Id, "").Code)
	assert.Equal(t, http.StatusBadRequest, duplicateBlock(t, h, source.Id, "   ").Code,
		"a whitespace-only name is an empty name")
}

func TestDuplicateBlock_MissingSourceIs404(t *testing.T) {
	h := newTestServer(t)
	assert.Equal(t, http.StatusNotFound, duplicateBlock(t, h, "does-not-exist", "x").Code)
}

// TestDuplicateBlock_ContradictorySharedShowPolicyRejected pins that
// arriving disabled does not exempt the copy from the shared-show
// agreement check: it is defined and it will come back, so the collision
// would otherwise surface the moment somebody enabled it.
func TestDuplicateBlock_ContradictorySharedShowPolicyRejected(t *testing.T) {
	h := newTestServer(t)

	w := doRequest(t, h, http.MethodPost, "/blocks", seriesBlockWriteWithPolicy("live", "Shared Show", gen.Restart))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	disabled := false
	contradicting := seriesBlockWriteWithPolicy("draft", "Shared Show", gen.Disable)
	contradicting.Enabled = &disabled
	w = doRequest(t, h, http.MethodPost, "/blocks", contradicting)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	source := decodeBlockRecord(t, w)

	wd := duplicateBlock(t, h, source.Id, "draft-copy")
	require.Equal(t, http.StatusBadRequest, wd.Code, wd.Body.String())
	assert.Contains(t, wd.Body.String(), "contradictory completion policy")
}

func TestDuplicateBlock_AnnouncesOnTheLiveLink(t *testing.T) {
	h, hub, _ := newTestServerWithHub(t)

	w := doRequest(t, h, http.MethodPost, "/blocks", filterBlockWrite("morning", "0 6 * * *"))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	source := decodeBlockRecord(t, w)
	before := hub.LastID()

	wd := duplicateBlock(t, h, source.Id, "copy")
	require.Equal(t, http.StatusCreated, wd.Code, wd.Body.String())
	assert.NotEqual(t, before, hub.LastID(),
		"duplicate did not announce plan.invalidated -- other tabs would not see the new block")
}
