package api

import (
	"context"
	"net/http"

	"github.com/christopherime/schedularr/internal/api/gen"
	"github.com/christopherime/schedularr/internal/httpclient"
	"github.com/christopherime/schedularr/internal/service"
)

// MediaAPI is the subset of *service.Runner that Handlers needs to serve
// GET /media/shows and GET /media/meta. It is deliberately a separate
// interface from service.ScheduleRunner (Deps.Sched) even though
// production wiring (cmd/serve.go) passes the same *service.Runner value
// to both fields: schedule generation and media discovery are different
// concerns, and TunarrAPI (tunarr.go) already established the precedent
// of one narrow interface per concern in this Deps struct, so a handler
// test double for one never has to also implement the other. Both
// boundaries resolving to the same concrete Runner in production is what
// guarantees MediaShows/MediaMeta reuse Run's exact fetch+cache path
// rather than standing up a second one.
type MediaAPI interface {
	MediaShows(ctx context.Context) ([]service.MediaShow, error)
	MediaMeta(ctx context.Context) (*service.MediaMeta, error)
}

var _ MediaAPI = (*service.Runner)(nil)

// ListMediaShows implements gen.ServerInterface.
//
// A nil Deps.Media always maps to 502 "tunarr unreachable" -- the same
// convention ListChannels uses (tunarr.go): Tunarr integration isn't
// configured at all, so "unreachable" is accurate. A live MediaShows call
// failing maps to one of two distinct 502s -- see writeMediaAPIError's doc
// comment for why a single "tunarr unreachable" wording stopped being
// accurate for every failure this can have: Runner.fetchPrograms
// (MediaShows/MediaMeta's shared fetch path) can fail because Tunarr
// genuinely wasn't reachable, OR because Tunarr responded but sent a
// response body Client.SearchPrograms/GetSeason couldn't decode --
// live-verified this session as a real, distinguishable failure mode
// (httpclient.IsDecodeError), not a hypothetical.
func (h *Handlers) ListMediaShows(w http.ResponseWriter, r *http.Request) {
	if h.d.Media == nil {
		WriteProblem(w, r, http.StatusBadGateway, "tunarr unreachable", "tunarr not configured")
		return
	}

	shows, err := h.d.Media.MediaShows(r.Context())
	if err != nil {
		h.writeMediaAPIError(w, r, "list_media_shows", err)
		return
	}

	list := make([]gen.MediaShow, 0, len(shows))
	for _, s := range shows {
		list = append(list, mediaShowToGen(s))
	}
	writeJSON(w, http.StatusOK, list)
}

// GetMediaMeta implements gen.ServerInterface. See ListMediaShows's doc
// comment for the nil-Deps.Media / error-mapping convention this shares.
func (h *Handlers) GetMediaMeta(w http.ResponseWriter, r *http.Request) {
	if h.d.Media == nil {
		WriteProblem(w, r, http.StatusBadGateway, "tunarr unreachable", "tunarr not configured")
		return
	}

	meta, err := h.d.Media.MediaMeta(r.Context())
	if err != nil {
		h.writeMediaAPIError(w, r, "get_media_meta", err)
		return
	}

	writeJSON(w, http.StatusOK, gen.MediaMeta{
		Genres:  meta.Genres,
		Ratings: meta.Ratings,
	})
}

// mediaShowToGen converts a service.MediaShow (the domain representation)
// into a gen.MediaShow (this API's wire shape).
func mediaShowToGen(s service.MediaShow) gen.MediaShow {
	return gen.MediaShow{Title: s.Title, EpisodeCount: s.EpisodeCount}
}

// writeMediaAPIError logs err server-side and writes a 502 problem+json
// response with a short, fixed detail -- err here is whatever
// Runner.fetchPrograms surfaced, which can wrap a raw Tunarr/HTTP error, so
// (matching writeScheduleRunnerError's convention, schedule.go) it never
// reaches the response body (no internals leaked either way -- both
// details below stay short and fixed; only the server-side log line, not
// the response, carries err itself). op identifies which handler failed
// (e.g. "list_media_shows") for the server-side log line.
//
// Both branches are still 502 (the caller can't fix either from their
// side) but the detail now distinguishes what actually went wrong:
// httpclient.IsDecodeError(err) is true when Tunarr was reached and
// responded, but its response body didn't decode into the shape
// Client.SearchPrograms/GetSeason expected (malformed JSON or a field
// type mismatch) -- a data-shape problem, not connectivity. Saying
// "tunarr unreachable" for that case used to be actively misleading: an
// operator chasing a network/connectivity problem that doesn't exist
// wastes time no differently than one who'd be helped by knowing it's a
// response-shape mismatch instead. Every other failure (couldn't connect
// at all, or Tunarr itself returned a non-2xx status) keeps the original
// "tunarr unreachable" wording, which is accurate for both.
func (h *Handlers) writeMediaAPIError(w http.ResponseWriter, r *http.Request, op string, err error) {
	h.d.Logger.Error("media discovery failed",
		"op", op,
		"request_id", RequestIDFromContext(r.Context()),
		"error", err,
	)
	if httpclient.IsDecodeError(err) {
		WriteProblem(w, r, http.StatusBadGateway, "tunarr response invalid", "tunarr returned unexpected data")
		return
	}
	WriteProblem(w, r, http.StatusBadGateway, "tunarr unreachable", "unable to reach tunarr")
}
