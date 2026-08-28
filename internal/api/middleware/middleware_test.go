package middleware_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/christopherime/schedularr/internal/api"
	"github.com/christopherime/schedularr/internal/api/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testToken = "01234567890123456789012345678901" // 33 chars, > min

func okHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}
}

func requireProblemJSON(t *testing.T, rr *httptest.ResponseRecorder, wantStatus int) api.Problem {
	t.Helper()
	require.Equal(t, wantStatus, rr.Code)
	require.Equal(t, "application/problem+json", rr.Header().Get("Content-Type"))
	var p api.Problem
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&p))
	require.Equal(t, wantStatus, p.Status)
	return p
}

// --- BearerAuth ---

func TestBearerAuthRejectsMissingToken(t *testing.T) {
	mw, err := middleware.BearerAuth(strings.Repeat("x", 32))
	require.NoError(t, err)

	rr := httptest.NewRecorder()
	mw(okHandler()).ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/blocks", nil))

	requireProblemJSON(t, rr, http.StatusUnauthorized)
}

func TestBearerAuthRejectsWrongToken(t *testing.T) {
	mw, err := middleware.BearerAuth(testToken)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/blocks", nil)
	req.Header.Set("Authorization", "Bearer "+strings.Repeat("y", 33))
	rr := httptest.NewRecorder()
	mw(okHandler()).ServeHTTP(rr, req)

	requireProblemJSON(t, rr, http.StatusUnauthorized)
}

func TestBearerAuthAcceptsValidToken(t *testing.T) {
	mw, err := middleware.BearerAuth(testToken)
	require.NoError(t, err)

	var sawRequest bool
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		sawRequest = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/blocks", nil)
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	mw(handler).ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.True(t, sawRequest, "inner handler should have been invoked")
}

func TestBearerAuthConstructorRejectsShortToken(t *testing.T) {
	_, err := middleware.BearerAuth(strings.Repeat("x", 31))
	require.Error(t, err)
}

// --- RequestID ---

func TestRequestIDSetsResponseHeader(t *testing.T) {
	handler := middleware.RequestID(okHandler())

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/status", nil))

	id := rr.Header().Get("X-Request-Id")
	assert.NotEmpty(t, id)
}

func TestRequestIDIgnoresInboundHeader(t *testing.T) {
	const inboundID = "attacker-supplied-id"

	var sawID string
	inner := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		sawID = middleware.RequestIDFrom(r.Context())
	})
	handler := middleware.RequestID(inner)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	req.Header.Set("X-Request-Id", inboundID)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	responseID := rr.Header().Get("X-Request-Id")
	assert.NotEmpty(t, responseID)
	assert.NotEqual(t, inboundID, responseID, "inbound X-Request-Id must not be trusted")
	assert.Equal(t, responseID, sawID, "context id must match the generated response header id")
}

// TestRequestIDPropagatesToWriteProblem verifies the wiring described in
// Task 8 step 3: once RequestID has stamped the context, api.WriteProblem
// (which reads it back via api.RequestIDFromContext, the same storage
// middleware.RequestIDFrom exposes) includes that id in the problem body.
func TestRequestIDPropagatesToWriteProblem(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		api.WriteProblem(w, r, http.StatusTeapot, "teapot", "for testing")
	})
	handler := middleware.RequestID(inner)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/status", nil))

	headerID := rr.Header().Get("X-Request-Id")
	require.NotEmpty(t, headerID)

	var p api.Problem
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&p))
	assert.Equal(t, headerID, p.RequestID)
}

// --- Recovery ---

func TestRecoveryHandlesPanicAndSurvives(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	panicking := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		panic("boom")
	})
	handler := middleware.Recovery(logger)(panicking)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/blocks", nil))
	requireProblemJSON(t, rr, http.StatusInternalServerError)

	assert.Contains(t, buf.String(), "boom", "panic value should be logged")

	// The process (and this handler) must still be usable afterwards.
	rr2 := httptest.NewRecorder()
	handler2 := middleware.Recovery(logger)(okHandler())
	handler2.ServeHTTP(rr2, httptest.NewRequest(http.MethodGet, "/api/v1/blocks", nil))
	assert.Equal(t, http.StatusOK, rr2.Code)
}

func TestRecoveryPassesThroughNonPanickingHandler(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))
	handler := middleware.Recovery(logger)(okHandler())

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/blocks", nil))

	assert.Equal(t, http.StatusOK, rr.Code)
}

// --- Logging ---

func TestLoggingWritesStructuredLine(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})
	handler := middleware.RequestID(middleware.Logging(logger)(inner))

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/v1/blocks", nil))

	var line map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &line))

	assert.Equal(t, http.MethodPost, line["method"])
	assert.Equal(t, "/api/v1/blocks", line["path"])
	assert.InDelta(t, float64(http.StatusCreated), line["status"], 0)
	if _, ok := line["duration_ms"]; !ok {
		t.Error("log line missing duration_ms key")
	}
	assert.NotEmpty(t, line["request_id"])
}

func TestLoggingDefaultsStatusToOKWhenHandlerDoesNotCallWriteHeader(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})
	handler := middleware.Logging(logger)(inner)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/status", nil))

	var line map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &line))
	assert.InDelta(t, float64(http.StatusOK), line["status"], 0)
}
