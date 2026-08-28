package middleware

import (
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"net/http"
	"strings"

	"github.com/christopherime/schedularr/internal/problem"
)

// minBearerTokenLength is the shortest token BearerAuth accepts. Anything
// shorter is rejected at construction time so a weak token cannot be
// deployed accidentally.
const minBearerTokenLength = 32

// bearerPrefix is the required Authorization header scheme.
const bearerPrefix = "Bearer "

// BearerAuth returns middleware that requires every request to carry
// "Authorization: Bearer <token>" matching token, and errors if token is
// shorter than 32 characters.
//
// The presented token is never compared to the configured one directly:
// both are hashed with SHA-256 first and the digests are compared with
// crypto/subtle.ConstantTimeCompare, so a wrong guess cannot be timed to
// learn how many leading characters it got right. A missing or
// non-matching token gets a 401 application/problem+json response.
func BearerAuth(token string) (func(http.Handler) http.Handler, error) {
	if len(token) < minBearerTokenLength {
		return nil, fmt.Errorf("bearer token must be at least %d characters, got %d", minBearerTokenLength, len(token))
	}
	want := sha256.Sum256([]byte(token))

	mw := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			if !strings.HasPrefix(header, bearerPrefix) {
				problem.WriteProblem(w, r, http.StatusUnauthorized, "unauthorized", "missing bearer token")
				return
			}

			presented := strings.TrimPrefix(header, bearerPrefix)
			got := sha256.Sum256([]byte(presented))
			if subtle.ConstantTimeCompare(want[:], got[:]) != 1 {
				problem.WriteProblem(w, r, http.StatusUnauthorized, "unauthorized", "invalid bearer token")
				return
			}

			next.ServeHTTP(w, r)
		})
	}

	return mw, nil
}
