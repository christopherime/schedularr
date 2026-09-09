package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/christopherime/schedularr/internal/api/gen"
	"github.com/christopherime/schedularr/internal/events"
)

// heartbeatInterval is how often the stream emits a heartbeat. It does
// two jobs: it carries server_time so a client can correct its own clock
// drift -- the permanent fix for the whole class of skew bug the retired
// schedule.ts had -- and it keeps bytes flowing so an intermediary proxy
// cannot buffer the connection into uselessness.
const heartbeatInterval = 15 * time.Second

// subscriberBuffer is the per-connection channel depth. A tab that stops
// reading loses events past this and resumes via Last-Event-ID; it never
// slows a publisher down (see events.Hub.Publish).
const subscriberBuffer = 32

// StreamEvents implements gen.ServerInterface.
//
// The response never completes while the client stays connected, which
// is why this handler writes and flushes frames itself rather than
// returning a body the way every other handler does.
//
// A nil Deps.Events means the live link was never wired -- the CLI's own
// handlers, and any embedding that omits it. Answered with 503 rather
// than a nil dereference, because the client treats an unavailable
// stream as a normal state and falls back to polling.
func (h *Handlers) StreamEvents(w http.ResponseWriter, r *http.Request, params gen.StreamEventsParams) {
	if h.d.Events == nil {
		WriteProblem(w, r, http.StatusServiceUnavailable, "live link unavailable",
			"this server was started without an event hub")
		return
	}

	// Checked BEFORE any header is written, so an unstreamable writer
	// still gets a proper problem response -- flushing to find out would
	// commit an empty 200 first.
	if !supportsFlush(w) {
		WriteProblem(w, r, http.StatusInternalServerError, "live link unavailable",
			"the response writer does not support streaming")
		return
	}

	// Flushes go through ResponseController rather than a direct
	// http.Flusher assertion: the writer reaching this handler is wrapped
	// by the logging middleware, and a plain assertion sees the wrapper
	// rather than the socket. The controller unwraps through anything
	// implementing Unwrap, so this keeps working as middleware is added.
	rc := http.NewResponseController(w)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	// nginx and oauth2-proxy buffer a response by default, which would
	// hold every event until the connection closed. The v0.9 SSO fronting
	// must preserve this -- see docs/deployment.md's proxy requirement.
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	_ = rc.Flush()

	ctx := r.Context()
	ch, unsubscribe := h.d.Events.Subscribe(ctx, resumeFrom(params))
	defer unsubscribe()

	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()

	// One heartbeat immediately, so a client knows it is connected -- and
	// has its clock offset -- without waiting a full interval.
	if !writeEvent(w, rc, events.Event{Name: events.Heartbeat, Data: heartbeatData()}) {
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case ev, open := <-ch:
			if !open {
				return
			}
			if !writeEvent(w, rc, ev) {
				return
			}
		case <-ticker.C:
			if !writeEvent(w, rc, events.Event{Name: events.Heartbeat, Data: heartbeatData()}) {
				return
			}
		}
	}
}

// resumeFrom reads the client's Last-Event-ID, or 0 for a fresh
// subscription. An unparseable or non-positive value is treated as "start
// fresh" rather than as an error: the client refetches its page data on
// connect anyway, and refusing the connection would strand a tab whose
// only problem was a corrupt header.
func resumeFrom(params gen.StreamEventsParams) int64 {
	if params.LastEventID == nil {
		return 0
	}
	parsed, err := strconv.ParseInt(*params.LastEventID, 10, 64)
	if err != nil || parsed < 0 {
		return 0
	}
	return parsed
}

// supportsFlush reports whether w can flush, unwrapping through
// middleware the way http.ResponseController does internally. A type
// assertion alone answers for the outermost wrapper, which is how the
// stream shipped broken once already.
func supportsFlush(w http.ResponseWriter) bool {
	for {
		if _, ok := w.(http.Flusher); ok {
			return true
		}
		unwrapper, ok := w.(interface{ Unwrap() http.ResponseWriter })
		if !ok {
			return false
		}
		w = unwrapper.Unwrap()
	}
}

func heartbeatData() map[string]any {
	return map[string]any{"server_time": time.Now().UTC().Format(time.RFC3339Nano)}
}

// writeEvent frames one event and flushes it. Returns false when the
// connection can no longer be written to, which is the signal to return
// from the handler and let the deferred unsubscribe run.
//
// A payload that will not marshal is skipped rather than killing the
// connection: one malformed event must not cost a tab its live link.
func writeEvent(w http.ResponseWriter, rc *http.ResponseController, ev events.Event) bool {
	payload, err := json.Marshal(ev.Data)
	if err != nil {
		return true
	}

	// Heartbeats carry no id: they are not resumable state, and giving
	// them ids would make a reconnect replay clock ticks.
	frame := fmt.Sprintf("event: %s\ndata: %s\n\n", ev.Name, payload)
	if ev.ID > 0 {
		frame = fmt.Sprintf("id: %d\n%s", ev.ID, frame)
	}

	if _, err := w.Write([]byte(frame)); err != nil {
		return false
	}
	return rc.Flush() == nil
}

// publish announces an event on the live-link hub, if one is wired. A
// Handlers built without a hub (the CLI's, and every test that doesn't
// need one) simply doesn't announce.
//
// Always called AFTER the write it announces has committed: a tab that
// refetches on an event must never be able to outrun the data the event
// describes.
func (h *Handlers) publish(name string, data any) {
	if h.d.Events == nil {
		return
	}
	h.d.Events.Publish(name, data)
}

// publishPlanInvalidated announces that something the schedule is
// derived from changed, so the guide should re-read it. reason is
// "block" or "series"; id is the block's store id or the show title.
func (h *Handlers) publishPlanInvalidated(reason, id string) {
	h.publish(events.PlanInvalidated, map[string]any{"reason": reason, "id": id})
}
