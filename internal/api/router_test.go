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
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/christopherime/schedularr/internal/store"
)

// routerTestToken is a 32-char bearer token, exactly at
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

// --- Embedded UI serving (Config.UI) ---

// testUIFS is a small fstest.MapFS standing in for web.Site()'s real
// embedded output: a root index.html, a 404.html, and a "blocks" section
// with its own index.html -- enough to exercise directory-index
// resolution without depending on `make web` having run.
func testUIFS() fstest.MapFS {
	return fstest.MapFS{
		"index.html":        &fstest.MapFile{Data: []byte("<html><body>schedularr ui</body></html>")},
		"404.html":          &fstest.MapFile{Data: []byte("<html><body>not found</body></html>")},
		"blocks/index.html": &fstest.MapFile{Data: []byte("<html><body>blocks</body></html>")},
	}
}

func newUIRouterTest(t *testing.T) http.Handler {
	t.Helper()
	r, err := NewRouter(Config{InsecureNoAuth: true, UI: testUIFS()}, Deps{Store: newRouterTestStore(t), Logger: slog.Default()})
	require.NoError(t, err)
	return r
}

func TestRouter_UIServesIndexAtRoot(t *testing.T) {
	r := newUIRouterTest(t)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "text/html; charset=utf-8", w.Header().Get("Content-Type"))
	assert.Contains(t, w.Body.String(), "schedularr ui")
	assert.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, "same-origin", w.Header().Get("Referrer-Policy"))
}

func TestRouter_UIServesDirectoryIndex(t *testing.T) {
	r := newUIRouterTest(t)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/blocks/", nil))
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "text/html; charset=utf-8", w.Header().Get("Content-Type"))
	assert.Contains(t, w.Body.String(), "blocks")
}

// TestRouter_UIRedirectsDirectoryWithoutTrailingSlash documents this
// package's chosen resolution for "/blocks" (no trailing slash): a 301
// redirect to "/blocks/", matching net/http.FileServer's own behavior for
// a directory request missing its slash.
func TestRouter_UIRedirectsDirectoryWithoutTrailingSlash(t *testing.T) {
	r := newUIRouterTest(t)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/blocks", nil))
	assert.Equal(t, http.StatusMovedPermanently, w.Code)
	assert.Equal(t, "/blocks/", w.Header().Get("Location"))
}

func TestRouter_UIUnknownPathServes404Page(t *testing.T) {
	r := newUIRouterTest(t)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/nope", nil))
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "not found")
	assert.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
}

// TestRouter_UIContentSecurityPolicyHeader confirms the CSP header (spec
// Decision 6) is present on both a 200 (index) and a 404 (unknown path)
// UI response -- newUIHandler sets it unconditionally at the top of the
// handler, before any status-code branching, so both paths must carry it.
func TestRouter_UIContentSecurityPolicyHeader(t *testing.T) {
	r := newUIRouterTest(t)

	wOK := httptest.NewRecorder()
	r.ServeHTTP(wOK, httptest.NewRequest(http.MethodGet, "/", nil))
	assert.Equal(t, http.StatusOK, wOK.Code)
	assert.Equal(t, uiCSP, wOK.Header().Get("Content-Security-Policy"))

	wNotFound := httptest.NewRecorder()
	r.ServeHTTP(wNotFound, httptest.NewRequest(http.MethodGet, "/nope", nil))
	assert.Equal(t, http.StatusNotFound, wNotFound.Code)
	assert.Equal(t, uiCSP, wNotFound.Header().Get("Content-Security-Policy"))
}

func TestRouter_UIPostToUnknownPathIsMethodNotAllowed(t *testing.T) {
	r := newUIRouterTest(t)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/anything", nil))
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

// TestRouter_UITraversalAttemptRejected confirms a ".." path segment in
// the request never escapes the embedded UI filesystem -- it's treated
// identically to an unknown path (the 404 page), not served as some other
// file (e.g. the module's real embed.go) and not a 500.
func TestRouter_UITraversalAttemptRejected(t *testing.T) {
	r := newUIRouterTest(t)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/../embed.go", nil))
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "not found")
}

// TestRouter_UIAPIv1StillRequiresBearerToken confirms mounting the UI
// doesn't change /api/v1/*'s existing auth gating.
func TestRouter_UIAPIv1StillRequiresBearerToken(t *testing.T) {
	r, err := NewRouter(Config{Token: routerTestToken, UI: testUIFS()}, Deps{Store: newRouterTestStore(t), Logger: slog.Default()})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/blocks", nil))
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Equal(t, "application/problem+json", w.Header().Get("Content-Type"))
}

// TestRouter_UIAPIv1UnknownPathStaysJSON is the key route-precedence
// check: with the UI mounted, an unmatched /api/v1/* path must still get
// the API's own JSON 404 (apiNotFoundHandler), not the UI's HTML 404 page.
// Without router.go explicitly setting sr.NotFound(apiNotFoundHandler)
// before mounting /api/v1, chi.Mux.NotFound would push the HTML handler
// down onto the /api/v1 sub-router too, since it propagates onto any
// mounted sub-router whose own NotFound handler is still nil at the time
// r.NotFound(...) is called on the parent.
func TestRouter_UIAPIv1UnknownPathStaysJSON(t *testing.T) {
	r := newUIRouterTest(t)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/nonexistent", nil))
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, "application/problem+json", w.Header().Get("Content-Type"))
	assert.NotContains(t, w.Body.String(), "<html")
}

// TestRouter_UIHealthzStillWorks confirms mounting the UI doesn't shadow
// the four system routes (chi matches exact/registered routes before ever
// falling through to NotFound).
func TestRouter_UIHealthzStillWorks(t *testing.T) {
	r := newUIRouterTest(t)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	assert.Equal(t, http.StatusOK, w.Code)
}

// TestRouter_APIPostWithUIStillWorks confirms mounting the UI doesn't just
// leave /api/v1/* reads working (TestRouter_UIAPIv1StillRequiresBearerToken
// only exercises a GET) but a real write too: a valid POST /api/v1/blocks
// with a correct bearer token still reaches gen.HandlerFromMux's own
// registered route and returns 201, proving newUIHandler -- installed as
// the router's catch-all NotFound -- never shadows a matched /api/v1/*
// route regardless of method.
func TestRouter_APIPostWithUIStillWorks(t *testing.T) {
	r, err := NewRouter(Config{Token: routerTestToken, UI: testUIFS()}, Deps{Store: newRouterTestStore(t), Logger: slog.Default()})
	require.NoError(t, err)

	body, err := json.Marshal(filterBlockWrite("ui-mounted-post", "0 6 * * *"))
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/blocks", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+routerTestToken)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusCreated, w.Code, "body: %s", w.Body.String())
}

// TestRouter_UIHeadIndex confirms a HEAD / against the mounted UI behaves
// like the GET / case in TestRouter_UIServesIndexAtRoot minus the body:
// same status and Content-Type, but http.ServeContent (which newUIHandler
// delegates to) never writes a body for HEAD.
func TestRouter_UIHeadIndex(t *testing.T) {
	r := newUIRouterTest(t)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodHead, "/", nil))
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "text/html; charset=utf-8", w.Header().Get("Content-Type"))
	assert.Empty(t, w.Body.String())
}

// TestRouter_NilUIKeepsPreviousNotFoundBehavior confirms Config.UI == nil
// (the zero value) leaves chi's own default 404 in place, exactly as
// before this package could serve a UI at all -- no behavior change for
// any caller that doesn't set UI.
func TestRouter_NilUIKeepsPreviousNotFoundBehavior(t *testing.T) {
	r, err := NewRouter(Config{InsecureNoAuth: true}, Deps{Store: newRouterTestStore(t), Logger: slog.Default()})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/nope", nil))
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, "404 page not found\n", w.Body.String())
	// Referrer-Policy, unlike X-Content-Type-Options, isn't something
	// net/http.Error sets on its own -- so its absence here is the useful
	// discriminator that nil UI really did skip newUIHandler entirely
	// rather than merely producing a similar-looking 404.
	assert.Empty(t, w.Header().Get("Referrer-Policy"), "nil UI must not add the UI-only Referrer-Policy header")
}
