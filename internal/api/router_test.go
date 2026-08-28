package api

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/christopherime/schedularr/internal/store"
)

// routerTestToken is a 33-char bearer token, comfortably over
// middleware.BearerAuth's 32-char minimum.
const routerTestToken = "01234567890123456789012345678901"

// newRouterTestStore returns a fresh temp-dir sqlite store for router
// tests -- NewRouter's /readyz and /api/v1/* both need a real *store.Store,
// not a fake, since Deps.Store is a concrete type.
func newRouterTestStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.New(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err, "failed to create test store")
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestNewRouter_RejectsEmptyToken(t *testing.T) {
	_, err := NewRouter(Config{Token: ""}, Deps{Store: newRouterTestStore(t), Logger: slog.Default()})
	require.Error(t, err)
}

func TestNewRouter_RejectsShortToken(t *testing.T) {
	_, err := NewRouter(Config{Token: strings.Repeat("x", 31)}, Deps{Store: newRouterTestStore(t), Logger: slog.Default()})
	require.Error(t, err)
}

func TestNewRouter_InsecureNoAuthNeedsNoToken(t *testing.T) {
	r, err := NewRouter(Config{Token: "", InsecureNoAuth: true}, Deps{Store: newRouterTestStore(t), Logger: slog.Default()})
	require.NoError(t, err)
	require.NotNil(t, r)
}

// --- Unauthenticated system endpoints ---

func TestRouter_HealthzUnauthenticated(t *testing.T) {
	r, err := NewRouter(Config{InsecureNoAuth: true}, Deps{Store: newRouterTestStore(t), Logger: slog.Default()})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRouter_ReadyzPingsStore(t *testing.T) {
	r, err := NewRouter(Config{InsecureNoAuth: true}, Deps{Store: newRouterTestStore(t), Logger: slog.Default()})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRouter_ReadyzReportsClosedStore(t *testing.T) {
	s := newRouterTestStore(t)
	require.NoError(t, s.Close())

	r, err := NewRouter(Config{InsecureNoAuth: true}, Deps{Store: s, Logger: slog.Default()})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	assert.Equal(t, "application/problem+json", w.Header().Get("Content-Type"))
}

func TestRouter_MetricsUnauthenticated(t *testing.T) {
	r, err := NewRouter(Config{InsecureNoAuth: true}, Deps{Store: newRouterTestStore(t), Logger: slog.Default()})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	assert.Equal(t, http.StatusOK, w.Code)
	assert.NotEmpty(t, w.Body.String())
}

func TestRouter_OpenAPIJSONUnauthenticated(t *testing.T) {
	r, err := NewRouter(Config{InsecureNoAuth: true}, Deps{Store: newRouterTestStore(t), Logger: slog.Default()})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/openapi.json", nil))
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
	assert.Contains(t, w.Body.String(), `"openapi"`)
}

// --- /api/v1/* auth gating ---

func TestRouter_APIv1RequiresBearerToken(t *testing.T) {
	r, err := NewRouter(Config{Token: routerTestToken}, Deps{Store: newRouterTestStore(t), Logger: slog.Default()})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/blocks", nil))
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Equal(t, "application/problem+json", w.Header().Get("Content-Type"))
}

func TestRouter_APIv1AcceptsValidToken(t *testing.T) {
	r, err := NewRouter(Config{Token: routerTestToken}, Deps{Store: newRouterTestStore(t), Logger: slog.Default()})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/blocks", nil)
	req.Header.Set("Authorization", "Bearer "+routerTestToken)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRouter_APIv1InsecureNoAuthServesWithoutHeader(t *testing.T) {
	r, err := NewRouter(Config{InsecureNoAuth: true}, Deps{Store: newRouterTestStore(t), Logger: slog.Default()})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/blocks", nil))
	assert.Equal(t, http.StatusOK, w.Code)
}

// TestRouter_APIv1RequestIDHeaderPresent confirms /api/v1/* actually runs
// through the RequestID middleware (not just BearerAuth), since NewRouter
// wires the whole RequestID->Logging->Recovery->BearerAuth chain onto that
// sub-router, not just the auth check.
func TestRouter_APIv1RequestIDHeaderPresent(t *testing.T) {
	r, err := NewRouter(Config{InsecureNoAuth: true}, Deps{Store: newRouterTestStore(t), Logger: slog.Default()})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/blocks", nil))
	assert.NotEmpty(t, w.Header().Get("X-Request-Id"))
}

// TestRouter_HealthzHasNoRequestIDHeader confirms the four system endpoints
// are NOT wrapped in RequestID/Logging/BearerAuth: only /api/v1/* gets a
// request id. They DO still run through Recovery -- see
// TestRouter_ReadyzPanicIsRecoveredNotConnectionDrop below -- Recovery
// alone has no observable header of its own, so this only asserts the
// absence of RequestID's header, not of the whole chain.
func TestRouter_HealthzHasNoRequestIDHeader(t *testing.T) {
	r, err := NewRouter(Config{InsecureNoAuth: true}, Deps{Store: newRouterTestStore(t), Logger: slog.Default()})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	assert.Empty(t, w.Header().Get("X-Request-Id"))
}

// TestRouter_ReadyzPanicIsRecoveredNotConnectionDrop proves the four
// system routes really do run through Recovery (not just RequestID/
// Logging/BearerAuth minus Recovery too). A nil Deps.Store makes
// readyzHandler's d.Store.Ping(ctx) call panic with a nil-pointer
// dereference -- a real, naturally occurring misconfiguration, not a
// contrived test-only hook. This has to go over a real network
// round-trip (httptest.NewServer + http.Get), not a direct
// r.ServeHTTP(httptest.NewRecorder(), ...) call: an *unrecovered* panic
// would otherwise just propagate straight up and crash the test process
// itself rather than being observable as a response. If Recovery is
// wired up, the client gets back a clean 500 problem+json body; if it
// isn't, net/http's own bare-minimum panic handling closes the
// connection instead, which surfaces here as an http.Get network error,
// not a 500 status.
func TestRouter_ReadyzPanicIsRecoveredNotConnectionDrop(t *testing.T) {
	r, err := NewRouter(Config{InsecureNoAuth: true}, Deps{Store: nil, Logger: slog.Default()})
	require.NoError(t, err)

	srv := httptest.NewServer(r)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/readyz") //nolint:noctx // test-only, fixed local address
	require.NoError(t, err, "a recovered panic must still produce a normal HTTP response, not a dropped connection")
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	assert.Equal(t, "application/problem+json", resp.Header.Get("Content-Type"))
}
