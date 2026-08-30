package api

import (
	"context"
	"net/http"
	"time"

	"github.com/christopherime/schedularr/internal/api/gen"
	"github.com/christopherime/schedularr/internal/external/tunarr"
)

// TunarrAPI is the subset of *tunarr.Client that Handlers needs to serve
// GET /channels and GET /status. Production wiring passes a *tunarr.Client
// (which satisfies this interface as-is, no adapter needed); tests pass a
// fake.
type TunarrAPI interface {
	GetChannels(ctx context.Context) ([]tunarr.Channel, error)
}

var _ TunarrAPI = (*tunarr.Client)(nil)

// ListChannels implements gen.ServerInterface.
//
// A nil Deps.Tunarr (Tunarr integration not configured for this deployment)
// and a live Tunarr call failing are both reported the same way: 502 with
// title "tunarr unreachable". From the client's perspective both mean "the
// channel list is unavailable right now" -- the detail string distinguishes
// them ("tunarr not configured" vs. the wrapped connectivity error).
func (h *Handlers) ListChannels(w http.ResponseWriter, r *http.Request) {
	if h.d.Tunarr == nil {
		WriteProblem(w, r, http.StatusBadGateway, "tunarr unreachable", "tunarr not configured")
		return
	}

	channels, err := h.d.Tunarr.GetChannels(r.Context())
	if err != nil {
		// Matches writeMediaAPIError's convention (media.go): err (an
		// upstream connectivity message, e.g. "dial tcp: connection
		// refused") is logged server-side only, never echoed into the
		// response Detail. This used to treat a connectivity message as
		// safe to surface verbatim; media.go's later, more careful
		// handling of the same 502 class established that a wrapped
		// error should never reach the response body, so this now
		// follows that rule too.
		h.d.Logger.Error("tunarr channel list failed",
			"op", "list_channels",
			"request_id", RequestIDFromContext(r.Context()),
			"error", err,
		)
		WriteProblem(w, r, http.StatusBadGateway, "tunarr unreachable", "unable to reach tunarr")
		return
	}

	list := make([]gen.Channel, 0, len(channels))
	for _, c := range channels {
		list = append(list, channelToGen(c))
	}
	writeJSON(w, http.StatusOK, list)
}

// channelToGen converts a tunarr.Channel (the upstream Tunarr API shape)
// into a gen.Channel (this API's wire shape). All three gen.Channel fields
// are pointers (OpenAPI marks them optional) but always populated here
// since this only ever converts a channel Tunarr actually returned.
func channelToGen(c tunarr.Channel) gen.Channel {
	id := c.ID
	name := c.Name
	number := c.Number
	return gen.Channel{
		Id:     &id,
		Name:   &name,
		Number: &number,
	}
}

// statusProbeTimeout bounds GetStatus's Tunarr liveness probe. Without it,
// a slow-but-not-dead Tunarr instance makes /status itself slow: the probe
// otherwise inherits only r.Context() plus whatever retry/backoff budget
// internal/httpclient imposes, neither of which caps a single slow
// response. 5s is generous for a same-cluster GetChannels round-trip while
// keeping /status's worst-case latency bounded and independent of
// Tunarr's.
const statusProbeTimeout = 5 * time.Second

// GetStatus implements gen.ServerInterface. Unlike every other handler in
// this package, it never returns a problem+json error response: Tunarr
// reachability is a probe result reported in the body (tunarr_reachable /
// tunarr_error), not an HTTP-level failure, and a block-count store error
// degrades to omitting Blocks (logged server-side via the same convention
// as logAndWriteInternalError, minus the response write) rather than
// failing the whole request.
func (h *Handlers) GetStatus(w http.ResponseWriter, r *http.Request) {
	status := gen.Status{Version: h.d.Version}

	probeCtx, cancel := context.WithTimeout(r.Context(), statusProbeTimeout)
	defer cancel()

	if h.d.Tunarr == nil {
		notConfigured := "not configured"
		status.TunarrReachable = false
		status.TunarrError = &notConfigured
	} else if _, err := h.d.Tunarr.GetChannels(probeCtx); err != nil {
		errMsg := err.Error()
		status.TunarrReachable = false
		status.TunarrError = &errMsg
	} else {
		status.TunarrReachable = true
	}

	count, err := h.d.Store.CountBlocks(r.Context())
	if err != nil {
		h.d.Logger.Error("internal error",
			"op", "get_status_count_blocks",
			"request_id", RequestIDFromContext(r.Context()),
			"error", err,
		)
	} else {
		blocks := int(count)
		status.Blocks = &blocks
	}

	// Same degradation contract as Blocks above: a store error omits the
	// field (logged server-side) rather than failing the whole request.
	lastApplied, err := h.d.Store.LastAppliedAt(r.Context())
	if err != nil {
		h.d.Logger.Error("internal error",
			"op", "get_status_last_applied_at",
			"request_id", RequestIDFromContext(r.Context()),
			"error", err,
		)
	} else {
		status.LastAppliedAt = lastApplied
	}

	if h.d.NextCronTick != nil {
		status.NextCronTick = h.d.NextCronTick()
	}

	writeJSON(w, http.StatusOK, status)
}
