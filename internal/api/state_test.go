package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/christopherime/schedularr/internal/api/gen"
	"github.com/christopherime/schedularr/internal/scheduler"
	"github.com/christopherime/schedularr/internal/store"
)

// newTestServerWithStore is newTestServer (blocks_test.go) plus direct
// access to the backing store, needed here to seed series_state rows: the
// state API has no create endpoint, only list and patch, and PatchSeriesState
// 404s on an unknown show_title by design (see state.go), so seeding for
// list/patch-persistence tests must go through the store directly.
func newTestServerWithStore(t *testing.T) (http.Handler, *store.Store) {
	t.Helper()
	s, err := store.New(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err, "failed to create test store")
	t.Cleanup(func() { _ = s.Close() })

	h := NewHandlers(Deps{Store: s, Logger: slog.Default(), Version: "test"})
	return gen.HandlerFromMux(h, chi.NewRouter()), s
}

func decodeSeriesStateList(t *testing.T, w *httptest.ResponseRecorder) []gen.SeriesState {
	t.Helper()
	var list []gen.SeriesState
	require.NoError(t, json.NewDecoder(w.Body).Decode(&list), "body: %s", w.Body.String())
	return list
}

func decodeSeriesState(t *testing.T, w *httptest.ResponseRecorder) gen.SeriesState {
	t.Helper()
	var st gen.SeriesState
	require.NoError(t, json.NewDecoder(w.Body).Decode(&st), "body: %s", w.Body.String())
	return st
}

func patchPath(showTitle string) string {
	return "/state/series/" + url.PathEscape(showTitle)
}

func TestListSeriesState_ReturnsSeeded(t *testing.T) {
	h, s := newTestServerWithStore(t)
	ctx := t.Context()

	require.NoError(t, s.UpdateSeriesState(ctx, &scheduler.SeriesState{
		ShowTitle: "Zeta Show", CurrentSeason: 2, CurrentEpisode: 5, RunCount: 1,
	}))
	require.NoError(t, s.UpdateSeriesState(ctx, &scheduler.SeriesState{
		ShowTitle: "Alpha Show", CurrentSeason: 1, CurrentEpisode: 3, Disabled: true,
	}))

	w := doRequest(t, h, http.MethodGet, "/state/series", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Equal(t, "application/json", w.Header().Get("Content-Type"))

	list := decodeSeriesStateList(t, w)
	require.Len(t, list, 2)

	// ExportAllSeriesStates orders by show_title, so Alpha precedes Zeta.
	assert.Equal(t, "Alpha Show", list[0].ShowTitle)
	assert.Equal(t, 1, list[0].CurrentSeason)
	assert.Equal(t, 3, list[0].CurrentEpisode)
	require.NotNil(t, list[0].Disabled)
	assert.True(t, *list[0].Disabled)

	assert.Equal(t, "Zeta Show", list[1].ShowTitle)
	assert.Equal(t, 2, list[1].CurrentSeason)
	assert.Equal(t, 5, list[1].CurrentEpisode)
	require.NotNil(t, list[1].RunCount)
	assert.Equal(t, 1, *list[1].RunCount)
}

func TestListSeriesState_Empty(t *testing.T) {
	h, _ := newTestServerWithStore(t)

	w := doRequest(t, h, http.MethodGet, "/state/series", nil)
	require.Equal(t, http.StatusOK, w.Code)
	list := decodeSeriesStateList(t, w)
	assert.Empty(t, list)
}

func TestPatchSeriesState_SeasonEpisodePersists(t *testing.T) {
	h, s := newTestServerWithStore(t)
	ctx := t.Context()

	require.NoError(t, s.UpdateSeriesState(ctx, &scheduler.SeriesState{
		ShowTitle: "Show A", CurrentSeason: 1, CurrentEpisode: 1,
	}))

	season, episode := 3, 7
	body := gen.SeriesStatePatch{CurrentSeason: &season, CurrentEpisode: &episode}

	w := doRequest(t, h, http.MethodPatch, patchPath("Show A"), body)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	got := decodeSeriesState(t, w)
	assert.Equal(t, "Show A", got.ShowTitle)
	assert.Equal(t, 3, got.CurrentSeason)
	assert.Equal(t, 7, got.CurrentEpisode)

	// Persisted, not just returned: a fresh lookup reflects the same values.
	persisted, err := s.GetPersistedSeriesState(ctx, "Show A")
	require.NoError(t, err)
	assert.Equal(t, 3, persisted.CurrentSeason)
	assert.Equal(t, 7, persisted.CurrentEpisode)
}

func TestPatchSeriesState_DisabledTruePersists(t *testing.T) {
	h, s := newTestServerWithStore(t)
	ctx := t.Context()

	require.NoError(t, s.UpdateSeriesState(ctx, &scheduler.SeriesState{
		ShowTitle: "Show B", CurrentSeason: 1, CurrentEpisode: 1, Disabled: false,
	}))

	disabled := true
	body := gen.SeriesStatePatch{Disabled: &disabled}

	w := doRequest(t, h, http.MethodPatch, patchPath("Show B"), body)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	got := decodeSeriesState(t, w)
	require.NotNil(t, got.Disabled)
	assert.True(t, *got.Disabled)
	// Untouched fields must survive the patch unchanged.
	assert.Equal(t, 1, got.CurrentSeason)
	assert.Equal(t, 1, got.CurrentEpisode)

	persisted, err := s.GetPersistedSeriesState(ctx, "Show B")
	require.NoError(t, err)
	assert.True(t, persisted.Disabled)
}

func TestPatchSeriesState_UnknownTitle_NotFound(t *testing.T) {
	h, _ := newTestServerWithStore(t)

	season := 2
	body := gen.SeriesStatePatch{CurrentSeason: &season}

	w := doRequest(t, h, http.MethodPatch, patchPath("Does Not Exist"), body)
	require.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
	require.Equal(t, "application/problem+json", w.Header().Get("Content-Type"))

	p := decodeProblem(t, w)
	assert.Equal(t, http.StatusNotFound, p.Status)
}

func TestPatchSeriesState_EmptyPatch_BadRequest(t *testing.T) {
	h, s := newTestServerWithStore(t)
	ctx := t.Context()

	require.NoError(t, s.UpdateSeriesState(ctx, &scheduler.SeriesState{
		ShowTitle: "Show C", CurrentSeason: 1, CurrentEpisode: 1,
	}))

	w := doRequest(t, h, http.MethodPatch, patchPath("Show C"), gen.SeriesStatePatch{})
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())

	p := decodeProblem(t, w)
	assert.Equal(t, http.StatusBadRequest, p.Status)
}

func TestPatchSeriesState_InvalidBody(t *testing.T) {
	h, s := newTestServerWithStore(t)
	ctx := t.Context()
	require.NoError(t, s.UpdateSeriesState(ctx, &scheduler.SeriesState{ShowTitle: "Show D"}))

	req := httptest.NewRequest(http.MethodPatch, patchPath("Show D"), strings.NewReader("{not json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestListSeriesState_InternalErrorDoesNotLeakDetail(t *testing.T) {
	h := newClosedStoreServer(t)

	w := doRequest(t, h, http.MethodGet, "/state/series", nil)
	assertGenericInternalErrorProblem(t, w)
}

// TestPatchSeriesState_InvalidatesFutureOccurrenceSnapshotsForReferencingBlocks
// is the handler-level regression for round-2 finding 5: only the SQL
// primitive (StateStore.DeleteFutureOccurrenceSnapshots) had a dedicated
// test before this -- nothing exercised the actual PATCH
// /state/series/{show_title} handler end to end to confirm it really
// calls it, for the right blocks, and leaves an unrelated block's
// snapshot alone.
func TestPatchSeriesState_InvalidatesFutureOccurrenceSnapshotsForReferencingBlocks(t *testing.T) {
	h, s := newTestServerWithStore(t)
	ctx := t.Context()

	require.NoError(t, s.UpdateSeriesState(ctx, &scheduler.SeriesState{
		ShowTitle: "Show A", CurrentSeason: 1, CurrentEpisode: 1,
	}))

	require.NoError(t, s.CreateBlock(ctx, &store.BlockRecord{
		ID: "block-1", Name: "Block One", Enabled: true,
		Spec: scheduler.Block{
			Name: "Block One", Type: scheduler.BlockTypeSeries, Cron: "0 20 * * *", Duration: 30, ChannelID: "channel-1",
			Series: []scheduler.SeriesConfig{{ShowTitle: "Show A", EpisodesPerBlock: 1}},
		},
	}))
	// A block that does NOT reference Show A must be untouched.
	require.NoError(t, s.CreateBlock(ctx, &store.BlockRecord{
		ID: "block-2", Name: "Block Two", Enabled: true,
		Spec: scheduler.Block{
			Name: "Block Two", Type: scheduler.BlockTypeSeries, Cron: "0 21 * * *", Duration: 30, ChannelID: "channel-1",
			Series: []scheduler.SeriesConfig{{ShowTitle: "Show B", EpisodesPerBlock: 1}},
		},
	}))

	future := time.Now().Add(24 * time.Hour)
	snapshot := map[string]scheduler.SeriesStateSnapshot{"Show A": {CurrentSeason: 1, CurrentEpisode: 1}}
	require.NoError(t, s.SaveOccurrenceSnapshot(ctx, "block-1", future, snapshot))
	require.NoError(t, s.SaveOccurrenceSnapshot(ctx, "block-2", future, snapshot))

	season := 3
	body := gen.SeriesStatePatch{CurrentSeason: &season}
	w := doRequest(t, h, http.MethodPatch, patchPath("Show A"), body)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	_, ok, err := s.GetOccurrenceSnapshot(ctx, "block-1", future)
	require.NoError(t, err)
	assert.False(t, ok, "block-1 references Show A and must have its future occurrence snapshot invalidated by the PATCH")

	_, ok, err = s.GetOccurrenceSnapshot(ctx, "block-2", future)
	require.NoError(t, err)
	assert.True(t, ok, "block-2 does not reference Show A and must be untouched")
}
