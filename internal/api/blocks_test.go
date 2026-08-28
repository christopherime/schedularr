package api

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/christopherime/schedularr/internal/api/gen"
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

func TestCreateBlock_InvalidBody(t *testing.T) {
	h := newTestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/blocks", strings.NewReader("{not json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
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
