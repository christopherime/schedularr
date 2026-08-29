package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/christopherime/schedularr/internal/api/gen"
	"github.com/christopherime/schedularr/internal/service"
	"github.com/christopherime/schedularr/internal/store"
)

// fakeMedia is a test double for MediaAPI. A nil err means MediaShows and
// GetMediaMeta return their canned values, nil; a non-nil err means both
// return their zero value, err -- mirroring fakeTunarr/fakeScheduleRunner's
// shape in this package's other handler test files.
type fakeMedia struct {
	shows []service.MediaShow
	meta  *service.MediaMeta
	err   error
}

func (f *fakeMedia) MediaShows(_ context.Context) ([]service.MediaShow, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.shows, nil
}

func (f *fakeMedia) MediaMeta(_ context.Context) (*service.MediaMeta, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.meta, nil
}

// newTestServerWithMedia builds the same kind of router newTestServer does
// (a fresh temp-dir sqlite store, no auth middleware), but with Deps.Media
// set to media -- which may be nil, exercising the "not configured" path
// both ListMediaShows and GetMediaMeta must handle without panicking, the
// same convention newTestServerWithTunarr uses for Deps.Tunarr.
func newTestServerWithMedia(t *testing.T, media MediaAPI) http.Handler {
	t.Helper()
	s, err := store.New(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err, "failed to create test store")
	t.Cleanup(func() { _ = s.Close() })

	h := NewHandlers(Deps{Store: s, Logger: slog.Default(), Version: "test", Media: media})
	return gen.HandlerFromMux(h, chi.NewRouter())
}

func TestListMediaShows_MapsFieldsSortedAsReturned(t *testing.T) {
	fake := &fakeMedia{shows: []service.MediaShow{
		{Title: "Parks and Recreation", EpisodeCount: 1},
		{Title: "The Office", EpisodeCount: 3},
	}}
	h := newTestServerWithMedia(t, fake)

	w := doRequest(t, h, http.MethodGet, "/media/shows", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var got []gen.MediaShow
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	require.Len(t, got, 2)
	assert.Equal(t, "Parks and Recreation", got[0].Title)
	assert.Equal(t, 1, got[0].EpisodeCount)
	assert.Equal(t, "The Office", got[1].Title)
	assert.Equal(t, 3, got[1].EpisodeCount)
}

func TestListMediaShows_EmptyLibrary_ReturnsEmptyArray(t *testing.T) {
	fake := &fakeMedia{shows: []service.MediaShow{}}
	h := newTestServerWithMedia(t, fake)

	w := doRequest(t, h, http.MethodGet, "/media/shows", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.JSONEq(t, "[]", w.Body.String())
}

func TestListMediaShows_MediaError_BadGatewayDoesNotLeakDetail(t *testing.T) {
	fake := &fakeMedia{err: errors.New("dial tcp: connection refused")}
	h := newTestServerWithMedia(t, fake)

	w := doRequest(t, h, http.MethodGet, "/media/shows", nil)
	require.Equal(t, http.StatusBadGateway, w.Code, w.Body.String())
	require.Equal(t, "application/problem+json", w.Header().Get("Content-Type"))

	p := decodeProblem(t, w)
	assert.Equal(t, "tunarr unreachable", p.Title)
	assert.Equal(t, http.StatusBadGateway, p.Status)
	assert.NotContains(t, p.Detail, "connection refused", "the raw Tunarr error must never reach the response body")
}

func TestListMediaShows_NilMedia_BadGateway(t *testing.T) {
	h := newTestServerWithMedia(t, nil)

	w := doRequest(t, h, http.MethodGet, "/media/shows", nil)
	require.Equal(t, http.StatusBadGateway, w.Code, w.Body.String())

	p := decodeProblem(t, w)
	assert.Equal(t, "tunarr unreachable", p.Title)
	assert.Equal(t, "tunarr not configured", p.Detail)
}

func TestGetMediaMeta_MapsFields(t *testing.T) {
	fake := &fakeMedia{meta: &service.MediaMeta{
		Genres:  []string{"Action", "Comedy"},
		Ratings: []string{"PG-13", "TV-14"},
	}}
	h := newTestServerWithMedia(t, fake)

	w := doRequest(t, h, http.MethodGet, "/media/meta", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var got gen.MediaMeta
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, []string{"Action", "Comedy"}, got.Genres)
	assert.Equal(t, []string{"PG-13", "TV-14"}, got.Ratings)
}

func TestGetMediaMeta_EmptyLibrary_ReturnsEmptyArrays(t *testing.T) {
	fake := &fakeMedia{meta: &service.MediaMeta{Genres: []string{}, Ratings: []string{}}}
	h := newTestServerWithMedia(t, fake)

	w := doRequest(t, h, http.MethodGet, "/media/meta", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.JSONEq(t, `{"genres":[],"ratings":[]}`, w.Body.String())
}

func TestGetMediaMeta_MediaError_BadGatewayDoesNotLeakDetail(t *testing.T) {
	fake := &fakeMedia{err: errors.New("sql: database is closed (driver internals)")}
	h := newTestServerWithMedia(t, fake)

	w := doRequest(t, h, http.MethodGet, "/media/meta", nil)
	require.Equal(t, http.StatusBadGateway, w.Code, w.Body.String())

	p := decodeProblem(t, w)
	assert.Equal(t, "tunarr unreachable", p.Title)
	lower := p.Detail
	assert.NotContains(t, lower, "sql")
	assert.NotContains(t, lower, "driver")
}

func TestGetMediaMeta_NilMedia_BadGateway(t *testing.T) {
	h := newTestServerWithMedia(t, nil)

	w := doRequest(t, h, http.MethodGet, "/media/meta", nil)
	require.Equal(t, http.StatusBadGateway, w.Code, w.Body.String())

	p := decodeProblem(t, w)
	assert.Equal(t, "tunarr unreachable", p.Title)
	assert.Equal(t, "tunarr not configured", p.Detail)
}
