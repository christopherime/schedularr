// Package api implements the Schedularr HTTP API defined by api/openapi.yaml.
package api

import (
	"context"
	"net/http"

	"github.com/christopherime/schedularr/internal/problem"
)

// Problem is an RFC 7807 problem+json error body. It is a type alias for
// problem.Problem: see internal/problem's package doc for why the type
// (and the helpers below) moved to their own leaf package -- in short,
// internal/api/router.go needs to import internal/api/middleware, and
// internal/api/middleware needs these, so they can no longer live in
// internal/api itself without an import cycle. The alias means every
// existing caller in this package (and its tests) is unaffected.
type Problem = problem.Problem

// ContextWithRequestID returns a copy of ctx carrying the given request ID.
// The request-ID middleware (internal/api/middleware.RequestID) calls this
// once per request.
func ContextWithRequestID(ctx context.Context, id string) context.Context {
	return problem.ContextWithRequestID(ctx, id)
}

// RequestIDFromContext returns the request ID stored by ContextWithRequestID,
// or "" if none was set.
func RequestIDFromContext(ctx context.Context) string {
	return problem.RequestIDFromContext(ctx)
}

// WriteProblem writes an RFC 7807 problem+json response. Type is set to
// "about:blank" since Schedularr does not publish per-error type URIs.
// RequestID is populated from the request context when the request-ID
// middleware has set one.
func WriteProblem(w http.ResponseWriter, r *http.Request, status int, title, detail string) {
	problem.WriteProblem(w, r, status, title, detail)
}
