// Package middleware provides the HTTP middleware shared by every route on
// the Schedularr API server: request-id propagation, structured request
// logging, panic recovery, and bearer-token authentication.
// internal/api/router.go wires these into the chi router built from
// internal/api/gen.
package middleware

import (
	"context"
	"net/http"

	"github.com/christopherime/schedularr/internal/problem"
	"github.com/google/uuid"
)

// requestIDHeader is the response header carrying the per-request
// identifier.
const requestIDHeader = "X-Request-Id"

// RequestID returns middleware that generates a fresh request identifier
// for every request, sets it on the response as X-Request-Id, and stores it
// in the request context so downstream handlers — and api.WriteProblem, via
// RequestIDFrom — can read it back.
//
// An inbound X-Request-Id header is never trusted: a caller could use one
// to spoof or correlate log entries across requests it does not own, so a
// new id is always generated regardless of what the client sent.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := uuid.NewString()
		w.Header().Set(requestIDHeader, id)
		ctx := problem.ContextWithRequestID(r.Context(), id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequestIDFrom returns the request id stored in ctx by RequestID, or "" if
// none is present. It shares storage with problem.ContextWithRequestID /
// problem.RequestIDFromContext, so this is what problem.WriteProblem (and
// api.WriteProblem, which forwards to it) effectively reads from when it
// populates Problem.RequestID.
func RequestIDFrom(ctx context.Context) string {
	return problem.RequestIDFromContext(ctx)
}
