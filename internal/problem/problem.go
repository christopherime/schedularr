// Package problem defines the RFC 7807 problem+json error body shared by
// the Schedularr API's handlers (internal/api) and its HTTP middleware
// (internal/api/middleware).
//
// It exists as its own leaf package -- depending on neither of those --
// specifically to avoid an import cycle. internal/api/middleware needs to
// write a problem+json response and read the per-request id
// middleware.RequestID stashes in context; internal/api/router.go (which
// lives in package api, alongside every handler file) needs to import
// internal/api/middleware to assemble RequestID/Logging/Recovery/BearerAuth
// onto the router. Before router.go existed, middleware imported
// internal/api directly for exactly the two things this package now
// provides -- that was fine when nothing in internal/api imported
// internal/api/middleware back. Once router.go does, api -> middleware ->
// api would cycle. internal/api/problem.go re-exports everything here
// (Problem as a type alias, the rest as thin wrapper functions), so every
// existing caller in internal/api keeps compiling unchanged.
package problem

import (
	"context"
	"encoding/json"
	"net/http"
)

// Problem is an RFC 7807 problem+json error body.
type Problem struct {
	Type      string `json:"type"`
	Title     string `json:"title"`
	Status    int    `json:"status"`
	Detail    string `json:"detail,omitempty"`
	RequestID string `json:"request_id,omitempty"`
}

// contextKey is an unexported type for context keys defined by this
// package, avoiding collisions with keys defined in other packages.
type contextKey int

const requestIDContextKey contextKey = iota

// ContextWithRequestID returns a copy of ctx carrying the given request ID.
// The request-ID middleware (internal/api/middleware.RequestID) calls this
// once per request.
func ContextWithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDContextKey, id)
}

// RequestIDFromContext returns the request ID stored by ContextWithRequestID,
// or "" if none was set.
func RequestIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(requestIDContextKey).(string)
	return id
}

// WriteProblem writes an RFC 7807 problem+json response. Type is set to
// "about:blank" since Schedularr does not publish per-error type URIs.
// RequestID is populated from the request context when the request-ID
// middleware has set one.
func WriteProblem(w http.ResponseWriter, r *http.Request, status int, title, detail string) {
	p := Problem{
		Type:      "about:blank",
		Title:     title,
		Status:    status,
		Detail:    detail,
		RequestID: RequestIDFromContext(r.Context()),
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(p)
}
