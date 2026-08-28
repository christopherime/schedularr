// Package api implements the Schedularr HTTP API defined by api/openapi.yaml.
package api

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

// contextKey is an unexported type for context keys defined by this package,
// avoiding collisions with keys defined in other packages.
type contextKey int

const requestIDContextKey contextKey = iota

// ContextWithRequestID returns a copy of ctx carrying the given request ID.
// The request-ID middleware (Task 8) calls this once per request; until
// that middleware is wired in, RequestIDFromContext returns "".
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
// middleware (Task 8) has set one.
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
