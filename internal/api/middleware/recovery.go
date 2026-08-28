package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/christopherime/schedularr/internal/api"
)

// Recovery returns middleware that recovers from a panic anywhere in the
// wrapped handler chain, logs it (with a stack trace) to l, and writes a
// 500 problem+json response instead of letting the panic take down the
// server process.
func Recovery(l *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func(ctx context.Context) {
				if rec := recover(); rec != nil {
					l.Error("panic recovered",
						"panic", rec,
						"stack", string(debug.Stack()),
						"request_id", RequestIDFrom(ctx),
					)
					api.WriteProblem(w, r, http.StatusInternalServerError, "internal server error", "")
				}
			}(r.Context())
			next.ServeHTTP(w, r)
		})
	}
}
