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
	"github.com/christopherime/schedularr/internal/external/tunarr"
	"github.com/christopherime/schedularr/internal/store"
)

// fakeTunarr is a test double for TunarrAPI. A nil err means GetChannels
// returns channels, nil; a non-nil err means it returns nil, err.
type fakeTunarr struct {
	channels []tunarr.Channel
	err      error
}

func (f *fakeTunarr) GetChannels(_ context.Context) ([]tunarr.Channel, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.channels, nil
}

// newTestServerWithTunarr builds the same kind of router newTestServer
// does (a fresh temp-dir sqlite store, no auth middleware), but with Deps.
// Tunarr set to tun -- which may be nil, exercising the "not configured"
// path both ListChannels and GetStatus must handle without panicking.
func newTestServerWithTunarr(t *testing.T, tun TunarrAPI) http.Handler {
	t.Helper()
	s, err := store.New(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err, "failed to create test store")
	t.Cleanup(func() { _ = s.Close() })

	h := NewHandlers(Deps{Store: s, Logger: slog.Default(), Version: "test", Tunarr: tun})
	return gen.HandlerFromMux(h, chi.NewRouter())
}

func TestListChannels_MapsFields(t *testing.T) {
	fake := &fakeTunarr{channels: []tunarr.Channel{
		{ID: "chan-1", Name: "News", Number: 1},
		{ID: "chan-2", Name: "Movies", Number: 2},
	}}
	h := newTestServerWithTunarr(t, fake)

	w := doRequest(t, h, http.MethodGet, "/channels", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var got []gen.Channel
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	require.Len(t, got, 2)

	assert.Equal(t, "chan-1", *got[0].Id)
	assert.Equal(t, "News", *got[0].Name)
	assert.Equal(t, 1, *got[0].Number)
	assert.Equal(t, "chan-2", *got[1].Id)
	assert.Equal(t, "Movies", *got[1].Name)
	assert.Equal(t, 2, *got[1].Number)
}

func TestListChannels_TunarrError_BadGateway(t *testing.T) {
	fake := &fakeTunarr{err: errors.New("dial tcp: connection refused")}
	h := newTestServerWithTunarr(t, fake)

	w := doRequest(t, h, http.MethodGet, "/channels", nil)
	require.Equal(t, http.StatusBadGateway, w.Code, w.Body.String())
	require.Equal(t, "application/problem+json", w.Header().Get("Content-Type"))

	p := decodeProblem(t, w)
	assert.Equal(t, "tunarr unreachable", p.Title)
	assert.Equal(t, http.StatusBadGateway, p.Status)
	// Detail is a fixed, non-leaking string (matches writeMediaAPIError's
	// convention, media.go) -- the wrapped connectivity error goes to the
	// server-side log only, never the response body.
	assert.Equal(t, "unable to reach tunarr", p.Detail)
	assert.NotContains(t, p.Detail, "connection refused")
}

func TestListChannels_NilTunarr_BadGateway(t *testing.T) {
	h := newTestServerWithTunarr(t, nil)

	w := doRequest(t, h, http.MethodGet, "/channels", nil)
	require.Equal(t, http.StatusBadGateway, w.Code, w.Body.String())

	p := decodeProblem(t, w)
	assert.Equal(t, "tunarr unreachable", p.Title)
	assert.Equal(t, "tunarr not configured", p.Detail)
}

func TestGetStatus_HealthyFake_ReachableTrueBlocksCountVersion(t *testing.T) {
	fake := &fakeTunarr{channels: []tunarr.Channel{{ID: "chan-1", Name: "News", Number: 1}}}
	h := newTestServerWithTunarr(t, fake)

	// Seed two blocks via the normal create path so CountBlocks has
	// something real to report.
	doRequest(t, h, http.MethodPost, "/blocks", filterBlockWrite("Block A", "0 20 * * *"))
	doRequest(t, h, http.MethodPost, "/blocks", filterBlockWrite("Block B", "0 21 * * *"))

	w := doRequest(t, h, http.MethodGet, "/status", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var got gen.Status
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))

	assert.Equal(t, "test", got.Version)
	assert.True(t, got.TunarrReachable)
	assert.Nil(t, got.TunarrError)
	require.NotNil(t, got.Blocks)
	assert.Equal(t, 2, *got.Blocks)
}

func TestGetStatus_FailingFake_ReachableFalseWithError(t *testing.T) {
	fake := &fakeTunarr{err: errors.New("dial tcp: connection refused")}
	h := newTestServerWithTunarr(t, fake)

	w := doRequest(t, h, http.MethodGet, "/status", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var got gen.Status
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))

	assert.False(t, got.TunarrReachable)
	require.NotNil(t, got.TunarrError)
	assert.Contains(t, *got.TunarrError, "connection refused")
}

func TestGetStatus_NilTunarr_ReachableFalseNotConfigured(t *testing.T) {
	h := newTestServerWithTunarr(t, nil)

	w := doRequest(t, h, http.MethodGet, "/status", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var got gen.Status
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))

	assert.Equal(t, "test", got.Version)
	assert.False(t, got.TunarrReachable)
	require.NotNil(t, got.TunarrError)
	assert.Equal(t, "not configured", *got.TunarrError)
}

func TestGetStatus_StoreCountError_StillReturns200(t *testing.T) {
	s, err := store.New(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err, "failed to create test store")
	require.NoError(t, s.Close(), "failed to close test store")

	fake := &fakeTunarr{channels: []tunarr.Channel{{ID: "chan-1", Name: "News", Number: 1}}}
	h := NewHandlers(Deps{Store: s, Logger: slog.Default(), Version: "test", Tunarr: fake})
	router := gen.HandlerFromMux(h, chi.NewRouter())

	w := doRequest(t, router, http.MethodGet, "/status", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var got gen.Status
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.True(t, got.TunarrReachable)
	assert.Nil(t, got.Blocks, "blocks should be omitted, not zero-valued, on a store error")
}
