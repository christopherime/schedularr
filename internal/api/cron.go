package api

import (
	"fmt"
	"net/http"
	"time"

	"github.com/christopherime/schedularr/internal/api/gen"
	"github.com/christopherime/schedularr/internal/scheduler"
)

// Occurrence-count bounds for GET /cron/next. The OpenAPI schema declares
// the same default/minimum/maximum, but oapi-codegen's generated bindings
// do not enforce them -- the identical gap GET /schedule's days parameter
// works around in schedule.go -- so they are applied here.
const (
	defaultCronCount = 3
	minCronCount     = 1
	maxCronCount     = 10
)

// NextCronOccurrences implements gen.ServerInterface.
//
// Stateless and store-free: the only endpoint in the contract that reads
// nothing. It exists so the client never evaluates cron itself -- the UI
// vendors cronstrue for PROSE, and asks here for instants.
func (h *Handlers) NextCronOccurrences(w http.ResponseWriter, r *http.Request, params gen.NextCronOccurrencesParams) {
	count := defaultCronCount
	if params.Count != nil {
		count = *params.Count
	}
	if count < minCronCount || count > maxCronCount {
		WriteProblem(w, r, http.StatusBadRequest, "count out of range",
			fmt.Sprintf("count must be between %d and %d, got %d", minCronCount, maxCronCount, count))
		return
	}

	// nil falls back to time.Local, matching the engine's own nil-handling.
	loc := h.location()

	// The location is what makes the answer readable: cron fields are wall
	// clock, so evaluating "0 6 * * *" anywhere but the configured zone
	// hands the operator a different hour than their channel will air.
	from := time.Now().In(loc)
	if params.From != nil {
		from = params.From.In(loc)
	}

	occurrences, err := scheduler.NextOccurrences(params.Expr, from, count)
	if err != nil {
		// The only failure NextOccurrences can produce here is a parse
		// error -- count is already range-checked -- and that is the
		// operator's expression, so it is a 400 and the detail is safe to
		// echo.
		WriteProblem(w, r, http.StatusBadRequest, "invalid cron expression", err.Error())
		return
	}

	// A valid expression that never fires (February 30th) yields an empty
	// array, not an error: the question was answerable and the answer is
	// "never". Built with make so it serializes as [] rather than null.
	out := make([]string, 0, len(occurrences))
	for _, o := range occurrences {
		out = append(out, o.Format(time.RFC3339))
	}
	writeJSON(w, http.StatusOK, map[string]any{"occurrences": out})
}
