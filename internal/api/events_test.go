package api

import (
	"bufio"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/christopherime/schedularr/internal/api/gen"
	"github.com/christopherime/schedularr/internal/events"
	"github.com/christopherime/schedularr/internal/scheduler"
	"github.com/christopherime/schedularr/internal/store"
)

// newTestServerWithHub is newTestServerWithStore plus a live-link hub, so
// a test can publish an event and watch it arrive on the stream.
func newTestServerWithHub(t *testing.T) (http.Handler, *events.Hub, *store.Store) {
	t.Helper()
	s, err := store.New(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err, "failed to create test store")
	t.Cleanup(func() { _ = s.Close() })

	hub := events.NewHub(subscriberBuffer)
	h := NewHandlers(Deps{Store: s, Logger: slog.Default(), Version: "test", Events: hub})
	return gen.HandlerFromMux(h, chi.NewRouter()), hub, s
}

// frameReader turns one streaming response body into a pull API: read n
// frames, wait up to a deadline. Exactly ONE goroutine reads the body for
// the lifetime of the connection -- spawning a reader per call would have
// the first goroutine steal frames the second is waiting for, which is a
// bug in the test, not in the handler.
type frameReader struct {
	lines <-chan string
	cur   strings.Builder
}

func newFrameReader(body io.Reader) *frameReader {
	lines := make(chan string)
	go func() {
		defer close(lines)
		r := bufio.NewReader(body)
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			lines <- line
		}
	}()
	return &frameReader{lines: lines}
}

// next pulls up to want SSE frames, returning early if the deadline
// passes. A frame is the text between blank lines.
func (fr *frameReader) next(t *testing.T, want int, deadline time.Duration) []string {
	t.Helper()
	frames := make([]string, 0, want)
	done := time.After(deadline)
	for len(frames) < want {
		select {
		case line, ok := <-fr.lines:
			if !ok {
				return frames
			}
			if strings.TrimRight(line, "\r\n") == "" {
				if fr.cur.Len() > 0 {
					frames = append(frames, fr.cur.String())
					fr.cur.Reset()
				}
				continue
			}
			fr.cur.WriteString(line)
		case <-done:
			return frames
		}
	}
	return frames
}

func TestStreamEvents_SetsStreamingHeadersAndOpensWithAHeartbeat(t *testing.T) {
	h, _, _ := newTestServerWithHub(t)
	srv := httptest.NewServer(h)
	defer srv.Close()

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/events", nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))
	assert.Equal(t, "no-cache", resp.Header.Get("Cache-Control"))
	// Defeats nginx/oauth2-proxy response buffering, which would otherwise
	// hold every event until the connection closed.
	assert.Equal(t, "no", resp.Header.Get("X-Accel-Buffering"))

	// A client learns it is connected -- and gets its clock offset --
	// without waiting a full heartbeat interval.
	frames := newFrameReader(resp.Body).next(t, 1, 3*time.Second)
	require.NotEmpty(t, frames, "no opening frame arrived")
	assert.Contains(t, frames[0], "event: heartbeat")
	assert.Contains(t, frames[0], "server_time")
	assert.NotContains(t, frames[0], "id: ", "a heartbeat is not resumable state")
}

func TestStreamEvents_DeliversAPublishedEvent(t *testing.T) {
	h, hub, _ := newTestServerWithHub(t)
	srv := httptest.NewServer(h)
	defer srv.Close()

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/events", nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	reader := newFrameReader(resp.Body)
	require.NotEmpty(t, reader.next(t, 1, 3*time.Second), "opening heartbeat")

	hub.Publish(events.StatusChanged, map[string]any{"tunarr_reachable": false})

	frames := reader.next(t, 1, 3*time.Second)
	require.NotEmpty(t, frames, "published event never arrived")
	assert.Contains(t, frames[0], "event: status.changed")
	assert.Contains(t, frames[0], `"tunarr_reachable":false`)
	assert.Contains(t, frames[0], "id: 1")
}

func TestStreamEvents_ResumesFromLastEventID(t *testing.T) {
	h, hub, _ := newTestServerWithHub(t)
	hub.Publish(events.SeriesChanged, map[string]any{"show_title": "A"})
	hub.Publish(events.SeriesChanged, map[string]any{"show_title": "B"})

	srv := httptest.NewServer(h)
	defer srv.Close()

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/events", nil)
	require.NoError(t, err)
	req.Header.Set("Last-Event-ID", "1")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// The opening heartbeat, then only what the client has not seen.
	frames := newFrameReader(resp.Body).next(t, 2, 3*time.Second)
	require.Len(t, frames, 2)
	joined := strings.Join(frames, "\n")
	assert.Contains(t, joined, `"show_title":"B"`)
	assert.NotContains(t, joined, `"show_title":"A"`, "an already-seen event must not replay")
}

func TestStreamEvents_MalformedResumeHeaderStartsFresh(t *testing.T) {
	// A corrupt header must not strand a tab: it starts fresh and
	// refetches, which is what it does on any connect.
	h, hub, _ := newTestServerWithHub(t)
	hub.Publish(events.SeriesChanged, map[string]any{"show_title": "A"})

	srv := httptest.NewServer(h)
	defer srv.Close()

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/events", nil)
	require.NoError(t, err)
	req.Header.Set("Last-Event-ID", "not-a-number")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)
	frames := newFrameReader(resp.Body).next(t, 2, 3*time.Second)
	assert.Contains(t, strings.Join(frames, "\n"), `"show_title":"A"`, "replays from the start")
}

func TestStreamEvents_WithoutAHubAnswers503(t *testing.T) {
	// The CLI's handlers have no live link. That is a normal state the
	// client's ladder handles, not a programming error.
	s, err := store.New(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	h := gen.HandlerFromMux(NewHandlers(Deps{Store: s, Logger: slog.Default(), Version: "test"}), chi.NewRouter())

	w := doRequest(t, h, http.MethodGet, "/events", nil)
	assert.Equal(t, http.StatusServiceUnavailable, w.Code, w.Body.String())
}

// drain pulls one event off a subscription, failing if none arrives.
func drain(t *testing.T, sub <-chan events.Event, within time.Duration) events.Event {
	t.Helper()
	select {
	case ev := <-sub:
		return ev
	case <-time.After(within):
		t.Fatal("no event arrived")
		return events.Event{}
	}
}

// TestBlockWrites_PublishPlanInvalidated pins that every write to a
// block tells the guide its plan is stale. Create, update and delete all
// change what the schedule derives from.
func TestBlockWrites_PublishPlanInvalidated(t *testing.T) {
	h, hub, _ := newTestServerWithHub(t)
	sub, cancel := hub.Subscribe(t.Context(), 0)
	defer cancel()

	w := doRequest(t, h, http.MethodPost, "/blocks", filterBlockWrite("live-link-block", "0 6 * * *"))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	created := decodeBlockRecord(t, w)

	ev := drain(t, sub, 2*time.Second)
	assert.Equal(t, events.PlanInvalidated, ev.Name)
	data, ok := ev.Data.(map[string]any)
	require.True(t, ok, "payload shape: %T", ev.Data)
	assert.Equal(t, "block", data["reason"])
	assert.Equal(t, created.Id, data["id"])

	w = doRequest(t, h, http.MethodDelete, "/blocks/"+created.Id, nil)
	require.Equal(t, http.StatusNoContent, w.Code, w.Body.String())

	ev = drain(t, sub, 2*time.Second)
	assert.Equal(t, events.PlanInvalidated, ev.Name)
	deleted, ok := ev.Data.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, created.Id, deleted["id"])
}

// TestPatchSeriesState_PublishesBothEvents: a cursor write changes the
// tracked row the History page shows AND invalidates every pending
// occurrence the guide drew from it, so it announces both.
func TestPatchSeriesState_PublishesBothEvents(t *testing.T) {
	h, hub, st := newTestServerWithHub(t)
	require.NoError(t, st.UpdateSeriesState(t.Context(), &scheduler.SeriesState{
		ShowTitle: "Live Show", CurrentSeason: 1, CurrentEpisode: 3,
	}))

	sub, cancel := hub.Subscribe(t.Context(), 0)
	defer cancel()

	w := doRequest(t, h, http.MethodPatch, patchPath("Live Show"),
		map[string]any{"current_episode": 4})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	names := []string{drain(t, sub, 2*time.Second).Name, drain(t, sub, 2*time.Second).Name}
	assert.ElementsMatch(t, []string{events.SeriesChanged, events.PlanInvalidated}, names)
}

// TestStreamEvents_FlushesThroughTheRealMiddlewareStack is the
// regression test for the bug the unit tests above could not see: they
// drive the generated mux directly, where the ResponseWriter is
// httptest's own and implements http.Flusher. In the real server the
// logging middleware wraps it, and a plain `w.(http.Flusher)` assertion
// sees the wrapper -- so the stream answered 500 in production while
// every unit test passed. This builds the router the binary builds.
func TestStreamEvents_FlushesThroughTheRealMiddlewareStack(t *testing.T) {
	s, err := store.New(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })

	hub := events.NewHub(subscriberBuffer)
	router, err := NewRouter(
		Config{InsecureNoAuth: true},
		Deps{Store: s, Logger: slog.Default(), Version: "test", Events: hub},
	)
	require.NoError(t, err)

	srv := httptest.NewServer(router)
	defer srv.Close()

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/events", nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode, "the stream must survive the middleware stack")
	assert.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))

	reader := newFrameReader(resp.Body)
	require.NotEmpty(t, reader.next(t, 1, 3*time.Second), "opening heartbeat never flushed")

	hub.Publish(events.StatusChanged, map[string]any{"tunarr_reachable": true})
	frames := reader.next(t, 1, 3*time.Second)
	require.NotEmpty(t, frames, "published event never flushed")
	assert.Contains(t, frames[0], "event: status.changed")
}
