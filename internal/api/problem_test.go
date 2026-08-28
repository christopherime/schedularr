package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWriteProblem(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/blocks", nil)

	WriteProblem(w, r, http.StatusNotImplemented, "not implemented", "listBlocks pending")

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusNotImplemented, resp.StatusCode)
	require.Equal(t, "application/problem+json", resp.Header.Get("Content-Type"))

	var p Problem
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&p))
	require.Equal(t, "about:blank", p.Type)
	require.Equal(t, "not implemented", p.Title)
	require.Equal(t, http.StatusNotImplemented, p.Status)
	require.Equal(t, "listBlocks pending", p.Detail)
	require.Empty(t, p.RequestID)
}

func TestWriteProblem_RequestIDFromContext(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/blocks", nil)
	r = r.WithContext(ContextWithRequestID(r.Context(), "req-123"))

	WriteProblem(w, r, http.StatusBadRequest, "bad request", "invalid filter")

	var p Problem
	require.NoError(t, json.NewDecoder(w.Result().Body).Decode(&p))
	require.Equal(t, "req-123", p.RequestID)
}
