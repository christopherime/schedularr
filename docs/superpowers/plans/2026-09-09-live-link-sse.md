# Live Link (SSE) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
>
> **UI tasks (9–13) additionally REQUIRE the `impeccable` skill** — CLAUDE.md rule 4. Prose written for docs and the CHANGELOG gets cleaned with `stop-slop`.

**Goal:** Give every page a live connection to the server — one SSE stream carrying a drift-correcting heartbeat and five change events — with an honest degradation ladder, and close the lost-update hole that multi-writer visibility opens.

**Architecture:** A broadcast hub in a new `internal/events` package fans one event out to every subscriber over a buffered channel, with a small ring buffer backing `Last-Event-ID` resume. `GET /api/v1/events` streams it as `text/event-stream`. Producers are the seams that already exist: `service.Runner` publishes `apply.completed`, the block and cursor handlers publish `plan.invalidated` and `series.changed`, and a new reachability prober in `serve` publishes `status.changed`. On the client, a hand-rolled `fetch()` + `ReadableStream` reader lives in the shared runtime (`EventSource` cannot send a bearer header), feeding an event bus that each page subscribes to. Nothing functional depends on the stream: when it fails, the ladder falls back to polling and finally to a manual reconnect, and every page stays operable with a refresh.

**Tech Stack:** Go 1.2x (stdlib `net/http` flushing, `context`, `sync`, stdlib `testing` with `-race`), chi router, oapi-codegen v2, CUE (config), Hugo + hand-written CSS + Alpine.js (vendored) + TypeScript (`node --test`).

**Spec:**
- `docs/superpowers/specs/2026-08-30-v0.5-web-overhaul-design.md` — §6 (Live Data, the whole slice), §3.4 (bezel telemetry and the LINK legend), §5 (motion budget: the two bezel pulses and the trace draw-in), §7 (`GET /api/v1/events`, `PATCH /blocks/{id}`, `If-Match` on `PUT /blocks/{id}`), §9.2 item 3 (the slice's own engine-first ordering and gate).
- `docs/roadmap.md` — "Then — Live link (SSE): pending. Unchanged in scope."

## Global Constraints

- **Nothing functional requires the stream.** Every page must stay fully operable with a manual refresh at every rung of the degradation ladder. A reviewer should be able to block `/api/v1/events` at the proxy and find the app still usable.
- **No `EventSource`, ever, and no token in a query string.** The client is a hand-rolled `fetch()` + `ReadableStream` reader so the `Authorization: Bearer` header can be sent. A token-in-query fallback would leak the token into access logs; it is out of scope permanently, not deferred.
- **CSP:** `connect-src 'self'`. Same origin, no new dependency, no new infrastructure. Geometry still goes through `element.style.setProperty` (inline `style` attributes are dropped under `style-src 'self'`).
- **Motion budget (spec §5):** 150–250ms, state-change only, never on initial load, full `prefers-reduced-motion` parity — data still lands, transitions and pulses are suppressed. This slice adds exactly two motion moments: one amber bezel pulse on LINK LOST, one green on reacquire. Plus the trace draw-in on newly arrived rows, which already exists.
- **Lean and clean (CLAUDE.md rule 1):** the 60s `/status` poll in `runtime/shell.ts` is not deleted — it becomes the ladder's POLL rung. It must not run concurrently with a healthy stream.
- **Docs in the same commit (CLAUDE.md rule 3).** README, CLAUDE.md, AGENTS.md, GEMINI.md, `docs/`, `mkdocs.yml` nav, `web/DESIGN.md`, CHANGELOG, `docs/roadmap.md`.
- **`make test` green before every commit** (rule 2); `golangci-lint run` clean before the release commit. `gosec` is currently broken against Go 1.27 in this toolchain (`internal error: package "strings" without types`) and fails on a clean tree — see TODO's v0.5.7 deferred list. Do not treat its failure as this slice's regression, and do not try to fix it here.
- **Lint limits:** cyclomatic ≤ 15, cognitive ≤ 20, nesting ≤ 5, results ≤ 3, **arguments ≤ 5**.
- **Error wrapping:** `fmt.Errorf("failed to X: %w", err)`. **Logging:** `slog`, snake_case keys.
- **Blocked packages:** `github.com/pkg/errors`, `logrus`, `crypto/md5`, `crypto/sha1`, `io/ioutil`, `gopkg.in/yaml.v1`, `gopkg.in/yaml.v2`. **No new Go or npm dependency in this slice** — the hub is stdlib, the client is ~60 lines.
- **Contract-first:** `api/openapi.yaml` before any handler; `make generate` and `make web-types` regenerate. Generated files are never hand-edited.
- **Terminology:** the spec's routing section still says "Log ← …" and "Series ← …". Those pages became `/history/` in v0.5.7. Read every such reference as the History page's AS-RUN/RUNS panes and TRACKED pane respectively; the plan uses the current names throughout.

## Scope decisions taken here

Three points where the spec's event catalog meets a codebase that has changed under it. Each is decided rather than left open, and each is written into the code as a comment so the next reader inherits the reasoning:

1. **`history.appended` is folded into `apply.completed`.** The spec lists both, and routes the Log page at both. They fire at the same instant from the same place — an apply committing — and carry the same `run_id`. Two events for one fact is a catalog that lies about its own granularity. The History page subscribes to `apply.completed` for all three panes. If a future slice appends history outside an apply, `history.appended` earns its place then.
2. **`status.changed` needs a producer that does not exist**, so this slice builds one: a reachability prober in `serve` that probes Tunarr on an interval and publishes only on a *transition*. Without it the event could never fire, because nothing server-side notices Tunarr going away between applies. The prober is also what lets the bezel stop polling `/status` while the stream is healthy.
3. **The concurrency hardening ships with the stream, not after it.** `If-Match` on `PUT /blocks/{id}` and the new `PATCH /blocks/{id}` are listed in the spec as "mandatory once multi-writer visibility exists" — this is the slice that creates that visibility. Shipping the stream without them would knowingly open a lost-update window.

## Explicitly out of scope

`BlockSpec.disabled_until` and `GET /cron/next` (block power tools slice); the consequence rail; `apply.started` or any two-phase apply event (rejected in spec §10); a station clock in the bezel (rejected, §10); SSE for anything outside `/api/v1`; multi-user presence.

---

## File Structure

**Created**

| File                              | Responsibility                                                          |
|-----------------------------------|-------------------------------------------------------------------------|
| `internal/events/hub.go`          | `Hub`, `Event`, `Subscribe`/`Publish`, ring buffer, monotonic ids       |
| `internal/events/hub_test.go`     | fan-out, slow-subscriber drop, resume, close semantics                  |
| `internal/api/events.go`          | `GET /events`: SSE framing, flushing, `Last-Event-ID`, heartbeat ticker |
| `internal/api/events_test.go`     | handler tests over `httptest`                                           |
| `cmd/probe.go`                    | Tunarr reachability prober publishing `status.changed` on transition    |
| `web/assets/ts/runtime/stream.ts` | fetch-stream SSE reader + parser + degradation ladder                   |
| `web/assets/ts/runtime/bus.ts`    | typed event bus pages subscribe to                                      |
| `web/tests/stream.test.ts`        | parser tests: chunk boundaries, CRLF, multi-line data, resume           |

**Modified**

| File                                            | Change                                                                     |
|-------------------------------------------------|----------------------------------------------------------------------------|
| `api/openapi.yaml`                              | `/events`; `PATCH /blocks/{id}`; `If-Match` on `PUT /blocks/{id}` + 412    |
| `internal/api/router.go`                        | `Config`/`Deps` gain the hub; `/events` mounted outside the generated tree |
| `internal/api/blocks.go`                        | `If-Match` enforcement, `PatchBlock`, `plan.invalidated` publish           |
| `internal/api/state.go`                         | `series.changed` publish                                                   |
| `internal/service/schedule.go`                  | `apply.completed` publish from `finishApplyRun`                            |
| `cmd/serve.go`                                  | build the hub, wire the prober, pass both into the API                     |
| `web/assets/ts/runtime/shell.ts`                | LINK legend, skew offset, poll becomes the ladder's POLL rung              |
| `web/assets/ts/pages/{guide,blocks,history}.ts` | subscribe-on-init; dirty guards                                            |
| `web/layouts/_default/baseof.html`              | the LINK telemetry item (its slot is already reserved)                     |
| `web/assets/css/main.css`                       | LINK states, the two pulses, `LINEUP CHANGED` line                         |
| `web/DESIGN.md`, docs, CHANGELOG, roadmap, TODO | see Task 14                                                                |

---

# Phase A — The stream and its producers

## Task 1: The broadcast hub

**Files:**
- Create: `internal/events/hub.go`, `internal/events/hub_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `events.Event{ID int64; Name string; Data any}`
  - names `events.ApplyCompleted`, `events.PlanInvalidated`, `events.StatusChanged`, `events.SeriesChanged`, `events.Heartbeat`
  - `events.NewHub(buffer int) *Hub`
  - `(*Hub).Publish(name string, data any)`
  - `(*Hub).Subscribe(ctx context.Context, lastID int64) (<-chan Event, func())`
  - `(*Hub).LastID() int64`

**Design constraints an implementer must respect:**

- **A slow subscriber must never block a publisher.** `Publish` is called from an apply's critical path; a browser tab that stopped reading must not wedge scheduling. Each subscriber gets a buffered channel and a full buffer means the event is *dropped for that subscriber*, not queued forever — the client's own resume-from-`Last-Event-ID` is what repairs a gap.
- **Ids are monotonic across the hub**, assigned under the same lock that appends to the ring, so a subscriber that resumes from an id can be told exactly what it missed.
- **The ring is small and lossy on purpose.** It backs a reconnect that happens within seconds; it is not an event store. A resume request older than the ring's oldest entry is answered by replaying nothing and letting the client refetch — losing an event is recoverable, pretending to have it is not.

- [ ] **Step 1: Write the failing test**

```go
package events

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHub_FansOutToEverySubscriber(t *testing.T) {
	h := NewHub(8)
	ctx := context.Background()

	a, closeA := h.Subscribe(ctx, 0)
	defer closeA()
	b, closeB := h.Subscribe(ctx, 0)
	defer closeB()

	h.Publish(StatusChanged, map[string]any{"tunarr_reachable": true})

	for name, ch := range map[string]<-chan Event{"a": a, "b": b} {
		select {
		case ev := <-ch:
			assert.Equal(t, StatusChanged, ev.Name, "subscriber %s", name)
			assert.Equal(t, int64(1), ev.ID, "ids start at 1")
		case <-time.After(time.Second):
			t.Fatalf("subscriber %s received nothing", name)
		}
	}
}

func TestHub_SlowSubscriberNeverBlocksPublish(t *testing.T) {
	// Buffer of 1, and nobody reads: publishing must still return.
	h := NewHub(1)
	_, cancel := h.Subscribe(context.Background(), 0)
	defer cancel()

	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			h.Publish(Heartbeat, map[string]any{"n": i})
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Publish blocked on a subscriber that stopped reading")
	}
	assert.Equal(t, int64(100), h.LastID(), "every publish still took an id")
}

func TestHub_ResumeReplaysWhatWasMissed(t *testing.T) {
	h := NewHub(16)
	h.Publish(StatusChanged, map[string]any{"n": 1})
	h.Publish(StatusChanged, map[string]any{"n": 2})
	h.Publish(StatusChanged, map[string]any{"n": 3})

	ch, cancel := h.Subscribe(context.Background(), 1) // saw 1, wants 2 and 3
	defer cancel()

	var got []int64
	for range 2 {
		select {
		case ev := <-ch:
			got = append(got, ev.ID)
		case <-time.After(time.Second):
			t.Fatal("resume replayed nothing")
		}
	}
	assert.Equal(t, []int64{2, 3}, got)
}

func TestHub_ResumeBeyondTheRingReplaysNothing(t *testing.T) {
	// A gap the ring cannot fill is not silently papered over: the
	// subscriber gets no replay and refetches instead.
	h := NewHub(2)
	for i := range 10 {
		h.Publish(StatusChanged, map[string]any{"n": i})
	}

	ch, cancel := h.Subscribe(context.Background(), 1)
	defer cancel()

	select {
	case ev := <-ch:
		assert.GreaterOrEqual(t, ev.ID, int64(9), "only what the ring still holds")
	case <-time.After(200 * time.Millisecond):
		// Also acceptable: nothing replayed at all.
	}
}

func TestHub_UnsubscribeIsIdempotentAndRaceFree(t *testing.T) {
	h := NewHub(4)
	ch, cancel := h.Subscribe(context.Background(), 0)

	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() { defer wg.Done(); cancel() }()
	}
	wg.Wait()

	h.Publish(StatusChanged, nil) // must not panic on a closed subscriber
	_, open := <-ch
	require.False(t, open, "channel closes on unsubscribe")
}

func TestHub_ContextCancellationUnsubscribes(t *testing.T) {
	h := NewHub(4)
	ctx, cancel := context.WithCancel(context.Background())
	ch, _ := h.Subscribe(ctx, 0)

	cancel()
	assert.Eventually(t, func() bool {
		_, open := <-ch
		return !open
	}, time.Second, 10*time.Millisecond, "a cancelled context drops the subscriber")
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test -race ./internal/events -v`
Expected: FAIL — the package does not exist.

- [ ] **Step 3: Write the hub**

Create `internal/events/hub.go`. The shape:

```go
// Package events provides the in-process broadcast hub behind
// GET /api/v1/events. One publisher fans an event out to every connected
// browser tab; nothing outside this process ever sees it, and nothing in
// this process depends on delivery succeeding.
package events

import (
	"context"
	"sync"
)

// Event names. These are the wire values in the SSE `event:` field and
// the strings the client's bus switches on, so they are part of the
// contract -- see api/openapi.yaml's /events description.
const (
	Heartbeat        = "heartbeat"
	ApplyCompleted   = "apply.completed"
	PlanInvalidated  = "plan.invalidated"
	StatusChanged    = "status.changed"
	SeriesChanged    = "series.changed"
)

// Event is one broadcast. Data is marshalled to JSON by the HTTP handler,
// not here: the hub stays transport-agnostic and a marshalling failure
// belongs to the connection that failed, not to every subscriber.
type Event struct {
	ID   int64
	Name string
	Data any
}

// Hub fans events out to subscribers. The zero value is not usable; call
// NewHub.
type Hub struct {
	mu     sync.Mutex
	nextID int64
	subs   map[int]*subscriber
	nextSub int
	buffer int

	// ring holds the most recent events for Last-Event-ID resume. It is
	// deliberately small and lossy: it backs a reconnect that happens
	// within seconds, not an event store. A client asking to resume from
	// before the ring's oldest entry is replayed nothing and refetches --
	// losing an event is recoverable, pretending to have delivered one is
	// not.
	ring    []Event
	ringCap int
}

type subscriber struct {
	ch     chan Event
	once   sync.Once
	closed bool
}
```

`NewHub(buffer int) *Hub` sets `buffer` (per-subscriber channel depth; use 32 at the call site) and `ringCap` (use 128).

`Publish(name string, data any)`:
1. lock, `h.nextID++`, build the `Event`, append to the ring (trimming to `ringCap`),
2. for each subscriber, **non-blocking send**:

```go
		select {
		case sub.ch <- ev:
		default:
			// Dropped for this subscriber only. A tab that stopped
			// reading must never wedge an apply, and the client's own
			// Last-Event-ID resume is what repairs the gap.
		}
```

3. unlock. `Publish` must not hold the lock across a blocking send, and must not call user code.

`Subscribe(ctx context.Context, lastID int64) (<-chan Event, func())`:
1. lock, allocate the subscriber and its buffered channel, register it,
2. replay from the ring: every held event with `ID > lastID`, using the same non-blocking send (a replay that overflows the buffer is a client that will resume again),
3. unlock, start a goroutine that waits on `ctx.Done()` and unsubscribes,
4. return the channel and an idempotent `func()` that removes the subscriber and closes its channel exactly once (`sub.once.Do`).

`LastID() int64` returns `h.nextID` under the lock.

- [ ] **Step 4: Run the tests**

Run: `go test -race ./internal/events -v`
Expected: PASS, including under `-race` with the concurrent-cancel test.

- [ ] **Step 5: Commit**

```bash
git add internal/events/
git commit -m "feat(events): add the in-process broadcast hub behind the live link"
```

---

## Task 2: `GET /api/v1/events`

**Files:**
- Modify: `api/openapi.yaml`
- Create: `internal/api/events.go`, `internal/api/events_test.go`
- Modify: `internal/api/router.go` (mount), `internal/api/server.go` (`Deps.Events`)

**Interfaces:**
- Consumes: `events.Hub` (Task 1).
- Produces: `GET /api/v1/events` streaming `text/event-stream`; `api.Deps.Events *events.Hub`.

**Why this route is mounted by hand rather than generated:** oapi-codegen's chi-server generator models a response body, and this one never ends. The route is registered directly on the `/api/v1` sub-router alongside the generated tree, inside the same auth middleware, and `api/openapi.yaml` documents it for clients without generating a handler for it.

- [ ] **Step 1: Document it in the contract**

In `api/openapi.yaml`, alongside the other paths:

```yaml
  /events:
    get:
      operationId: streamEvents
      summary: Server-sent event stream of live changes
      description: >-
        A `text/event-stream` of change events, one connection per browser
        tab. Carries a `heartbeat` every 15 seconds whose `server_time`
        lets a client correct its own clock drift; the heartbeat also
        defeats proxy buffering. Supports `Last-Event-ID` resume from a
        small in-memory ring buffer — a resume point older than the ring
        replays nothing, and the client refetches instead.

        Consumed by a hand-rolled fetch/ReadableStream reader, never by
        `EventSource`, which cannot send the Authorization header.

        Nothing in the UI requires this stream: every page stays operable
        with a manual refresh when it is unavailable.
      parameters:
        - name: Last-Event-ID
          in: header
          required: false
          schema:
            type: string
          description: Resume point — the id of the last event the client processed.
      responses:
        "200":
          description: The event stream (never completes while connected).
          content:
            text/event-stream:
              schema:
                type: string
```

Run `make generate && make web-types`. The generated `ServerInterface` gains `StreamEvents` only if oapi-codegen emits one for this path; if it does, implement it as a thin wrapper that calls the handler below, and if it does not, mount the handler directly. Read the regenerated file rather than assuming.

- [ ] **Step 2: Write the failing test**

Create `internal/api/events_test.go`:

```go
package api

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/christopherime/schedularr/internal/events"
)

// readFrames pulls SSE frames off the stream until want frames arrive or
// the deadline passes. A frame is the text between blank lines.
func readFrames(t *testing.T, body *bufio.Reader, want int, deadline time.Duration) []string {
	t.Helper()
	frames := make([]string, 0, want)
	var cur strings.Builder
	done := time.After(deadline)
	lines := make(chan string)
	go func() {
		for {
			line, err := body.ReadString('\n')
			if err != nil {
				close(lines)
				return
			}
			lines <- line
		}
	}()
	for len(frames) < want {
		select {
		case line, ok := <-lines:
			if !ok {
				return frames
			}
			if strings.TrimRight(line, "\r\n") == "" {
				if cur.Len() > 0 {
					frames = append(frames, cur.String())
					cur.Reset()
				}
				continue
			}
			cur.WriteString(line)
		case <-done:
			return frames
		}
	}
	return frames
}

func TestStreamEvents_SetsStreamingHeaders(t *testing.T) {
	h, _ := newTestServerWithStore(t)
	hub := events.NewHub(8)
	h.d.Events = hub

	req := httptest.NewRequest(http.MethodGet, "/api/v1/events", nil)
	ctx, cancel := context.WithCancel(req.Context())
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	go func() { time.Sleep(150 * time.Millisecond); cancel() }()
	h.StreamEvents(rec, req)

	assert.Equal(t, "text/event-stream", rec.Header().Get("Content-Type"))
	assert.Equal(t, "no-cache", rec.Header().Get("Cache-Control"))
	// Defeats nginx/oauth2-proxy response buffering, which would otherwise
	// hold every event until the connection closed.
	assert.Equal(t, "no", rec.Header().Get("X-Accel-Buffering"))
}

func TestStreamEvents_DeliversAPublishedEvent(t *testing.T) {
	h, _ := newTestServerWithStore(t)
	hub := events.NewHub(8)
	h.d.Events = hub

	srv := httptest.NewServer(http.HandlerFunc(h.StreamEvents))
	defer srv.Close()

	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Give the handler time to subscribe before publishing, otherwise the
	// event predates the subscription and only the ring would carry it.
	time.Sleep(100 * time.Millisecond)
	hub.Publish(events.StatusChanged, map[string]any{"tunarr_reachable": false})

	frames := readFrames(t, bufio.NewReader(resp.Body), 1, 3*time.Second)
	require.NotEmpty(t, frames, "no frame arrived")

	joined := strings.Join(frames, "\n")
	assert.Contains(t, joined, "event: status.changed")
	assert.Contains(t, joined, `"tunarr_reachable":false`)
	assert.Contains(t, joined, "id: ")
}

func TestStreamEvents_ResumesFromLastEventID(t *testing.T) {
	h, _ := newTestServerWithStore(t)
	hub := events.NewHub(16)
	h.d.Events = hub
	hub.Publish(events.SeriesChanged, map[string]any{"show_title": "A"})
	hub.Publish(events.SeriesChanged, map[string]any{"show_title": "B"})

	srv := httptest.NewServer(http.HandlerFunc(h.StreamEvents))
	defer srv.Close()

	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	require.NoError(t, err)
	req.Header.Set("Last-Event-ID", "1")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	frames := readFrames(t, bufio.NewReader(resp.Body), 1, 3*time.Second)
	require.NotEmpty(t, frames)
	joined := strings.Join(frames, "\n")
	assert.Contains(t, joined, `"show_title":"B"`)
	assert.NotContains(t, joined, `"show_title":"A"`, "already-seen event must not replay")
}
```

Reuse `newTestServerWithStore` from the package's existing tests; add an `Events` field to whatever `Deps` it builds.

- [ ] **Step 3: Run to verify it fails**

Run: `go test -race ./internal/api -run StreamEvents -v`
Expected: FAIL — `h.StreamEvents undefined`.

- [ ] **Step 4: Write the handler**

Create `internal/api/events.go`:

```go
package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/christopherime/schedularr/internal/events"
)

// heartbeatInterval is how often the stream emits a heartbeat. It does
// two jobs: it carries server_time so a client can correct its own clock
// drift (the permanent fix for the whole class of skew bug), and it keeps
// bytes flowing so an intermediary proxy cannot buffer the connection
// into uselessness.
const heartbeatInterval = 15 * time.Second

// subscriberBuffer is the per-connection channel depth. A tab that stops
// reading loses events past this and resumes via Last-Event-ID; it never
// slows a publisher down (see events.Hub.Publish).
const subscriberBuffer = 32

// StreamEvents serves GET /api/v1/events.
//
// Mounted by hand rather than through the generated tree: oapi-codegen
// models a response body, and this response never ends.
//
// A nil Deps.Events means the live link is not wired (some tests, and any
// future embedding that omits it) -- answered with 503 rather than a nil
// dereference, since the client's own degradation ladder treats a failed
// stream as a normal state and falls back to polling.
func (h *Handlers) StreamEvents(w http.ResponseWriter, r *http.Request) {
	if h.d.Events == nil {
		WriteProblem(w, r, http.StatusServiceUnavailable, "live link unavailable",
			"this server was started without an event hub")
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		WriteProblem(w, r, http.StatusInternalServerError, "live link unavailable",
			"the response writer does not support streaming")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	// nginx and oauth2-proxy buffer a response by default, which would
	// hold every event until the connection closed. The v0.9 SSO fronting
	// must preserve this -- recorded as a standing integration
	// requirement in the spec's §6.
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	lastID := int64(0)
	if raw := r.Header.Get("Last-Event-ID"); raw != "" {
		if parsed, err := strconv.ParseInt(raw, 10, 64); err == nil && parsed > 0 {
			lastID = parsed
		}
		// An unparseable resume point is treated as "start fresh" rather
		// than an error: the client refetches anyway, and refusing the
		// connection would strand a tab that only had a corrupt header.
	}

	ctx := r.Context()
	ch, unsubscribe := h.d.Events.Subscribe(ctx, lastID)
	defer unsubscribe()

	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()

	// One heartbeat immediately, so a client knows it is connected (and
	// gets its clock offset) without waiting a full interval.
	if !writeEvent(w, flusher, events.Event{Name: events.Heartbeat, Data: heartbeatData()}) {
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
			if !writeEvent(w, flusher, ev) {
				return
			}
		case <-ticker.C:
			if !writeEvent(w, flusher, events.Event{Name: events.Heartbeat, Data: heartbeatData()}) {
				return
			}
		}
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
func writeEvent(w http.ResponseWriter, flusher http.Flusher, ev events.Event) bool {
	payload, err := json.Marshal(ev.Data)
	if err != nil {
		return true
	}
	var frame string
	if ev.ID > 0 {
		frame = fmt.Sprintf("id: %d\nevent: %s\ndata: %s\n\n", ev.ID, ev.Name, payload)
	} else {
		// Heartbeats carry no id: they are not resumable state, and
		// giving them ids would make a reconnect replay clock ticks.
		frame = fmt.Sprintf("event: %s\ndata: %s\n\n", ev.Name, payload)
	}
	if _, err := w.Write([]byte(frame)); err != nil {
		return false
	}
	flusher.Flush()
	return true
}
```

- [ ] **Step 5: Wire it into `Deps` and the router**

`internal/api/server.go` — add to `Deps`:

```go
	// Events is the live-link broadcast hub behind GET /events, and the
	// publisher every mutating handler notifies. A nil Hub means the live
	// link is not wired: publishes are skipped and /events answers 503,
	// which the client treats as a normal degraded state.
	Events *events.Hub
```

`internal/api/router.go` — inside the `r.Route("/api/v1", ...)` closure, next to where the generated tree is mounted:

```go
		// Registered by hand, inside the same auth middleware as the
		// generated tree: oapi-codegen models a response body, and this
		// response never ends. See Handlers.StreamEvents.
		sr.Get("/events", h.StreamEvents)
```

Confirm registration order against the generated mount so the explicit route is not shadowed; read `router.go`'s existing comments about `notFoundHandler` ordering before placing it.

- [ ] **Step 6: Run the tests**

Run: `go test -race ./internal/api -run StreamEvents -v && go test -race ./internal/api`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add api/openapi.yaml internal/api/ web/assets/ts/gen/types.d.ts
git commit -m "feat(api): stream live changes over GET /events"
```

---

## Task 3: Publishers on the seams that already exist

**Files:**
- Modify: `internal/service/schedule.go` (`apply.completed`)
- Modify: `internal/api/blocks.go` (`plan.invalidated` on create/update/patch/delete)
- Modify: `internal/api/state.go` (`series.changed` on cursor patch)
- Test: the respective `_test.go` files

**Interfaces:**
- Consumes: `events.Hub` (Task 1).
- Produces: no new exported symbols. `service.RunnerOptions` gains `Events *events.Hub`; `Runner` publishes `apply.completed` from `finishApplyRun` **after** the run record is written.

**Payloads (these are the contract the client switches on):**

```text
apply.completed  {run_id, source, channel_ids[], slot_count, warning_count, applied_at}
plan.invalidated {reason: "block"|"series", id}
series.changed   {show_title}
```

**Ordering rule:** publish *after* the write it announces has committed. A tab that refetches on an event must not race ahead of the data the event describes.

**Nil-hub rule:** every publish site must tolerate a nil hub, because the CLI builds a `Runner` with no live link at all. Put the nil check in one place — a small `publish` helper on the Runner and on `Handlers` — rather than at each call site.

- [ ] **Step 1: Write the failing tests**

In `internal/service/schedule_test.go`:

```go
func TestRunner_Run_PublishesApplyCompleted(t *testing.T) {
	server, _ := newFakeTunarr(t, canonicalPrograms())
	st, _ := newTestRunnerDepsStore(t, server.URL) // or reuse newTestRunner's store
	hub := events.NewHub(8)
	sub, cancel := hub.Subscribe(context.Background(), 0)
	defer cancel()

	r := NewRunner(st, tunarr.NewClient(tunarr.Config{URL: server.URL}), RunnerOptions{
		Logger: discardLogger(), Location: time.UTC, Events: hub,
	})

	res, err := r.Run(context.Background(), Options{Days: 1, Apply: true, Source: SourceCron})
	require.NoError(t, err)

	select {
	case ev := <-sub:
		assert.Equal(t, events.ApplyCompleted, ev.Name)
		data, ok := ev.Data.(map[string]any)
		require.True(t, ok, "payload shape")
		assert.Equal(t, res.RunID, data["run_id"])
		assert.Equal(t, string(SourceCron), data["source"])
	case <-time.After(2 * time.Second):
		t.Fatal("no apply.completed event")
	}
}

func TestRunner_Run_DryRunPublishesNothing(t *testing.T) {
	// A dry run applies nothing, so there is nothing to announce.
	server, _ := newFakeTunarr(t, canonicalPrograms())
	st, _ := newTestRunnerDepsStore(t, server.URL)
	hub := events.NewHub(8)
	sub, cancel := hub.Subscribe(context.Background(), 0)
	defer cancel()

	r := NewRunner(st, tunarr.NewClient(tunarr.Config{URL: server.URL}), RunnerOptions{
		Logger: discardLogger(), Location: time.UTC, Events: hub,
	})
	_, err := r.Run(context.Background(), Options{Days: 1, Apply: false, Source: SourceUI})
	require.NoError(t, err)

	select {
	case ev := <-sub:
		t.Fatalf("dry run published %s", ev.Name)
	case <-time.After(300 * time.Millisecond):
	}
}

func TestRunner_Run_NilHubIsFine(t *testing.T) {
	// The CLI builds a Runner with no live link at all.
	server, _ := newFakeTunarr(t, canonicalPrograms())
	r, _ := newTestRunner(t, server.URL) // RunnerOptions{} -- Events nil
	_, err := r.Run(context.Background(), Options{Days: 1, Apply: true, Source: SourceCLI})
	require.NoError(t, err)
}
```

Adapt the helper names to what `schedule_test.go` actually provides — `newTestRunner` already returns `(*Runner, *store.Store)`; add a variant that accepts `RunnerOptions` rather than duplicating its block seeding.

In `internal/api/blocks_test.go` and `state_test.go`, add the mirror cases: creating, updating, patching, and deleting a block each publish exactly one `plan.invalidated` naming that block's id; a cursor `PATCH` publishes one `series.changed` naming the show.

- [ ] **Step 2: Run to verify they fail**

Run: `go test -race ./internal/service ./internal/api -run 'Publishes|NilHub' -v`
Expected: FAIL — `unknown field Events`.

- [ ] **Step 3: Publish from the Runner**

`RunnerOptions` gains:

```go
	// Events is the live-link hub this Runner announces completed applies
	// on. Nil disables publishing entirely -- the CLI has no live link,
	// and an apply must never depend on one.
	Events *events.Hub
```

Store it on `Runner`, and at the end of `finishApplyRun` — after `FinishApplyRun` has succeeded, so a subscriber that refetches cannot outrun the row:

```go
	r.publish(events.ApplyCompleted, map[string]any{
		"run_id":        rec.ID,
		"source":        rec.Source,
		"channel_ids":   channelIDs(res),
		"slot_count":    rec.SlotCount,
		"warning_count": len(rec.Warnings),
		"applied_at":    finished.UTC().Format(time.RFC3339Nano),
	})
```

with:

```go
// publish announces ev on the live-link hub, if this Runner has one. A
// Runner built without a hub (the CLI) simply doesn't announce -- an
// apply must never depend on the live link.
func (r *Runner) publish(name string, data any) {
	if r.events == nil {
		return
	}
	r.events.Publish(name, data)
}

// channelIDs lists the channels a run touched, for the guide's refetch
// decision. Nil result on a failed run: nothing was pushed.
func channelIDs(res *Result) []string {
	if res == nil {
		return nil
	}
	ids := make([]string, 0, len(res.Channels))
	for id := range res.Channels {
		ids = append(ids, id)
	}
	sort.Strings(ids) // stable payload; a set iterated raw would reorder per apply
	return ids
}
```

Publish on both outcomes — a failed apply is still news the guide should act on — but keep `slot_count` at whatever the record holds (zero for a failure), matching what the RUNS pane already shows.

- [ ] **Step 4: Publish from the handlers**

Add the same helper to `Handlers`:

```go
// publish announces ev on the live-link hub, if one is wired. Called
// AFTER the write it announces has committed: a tab that refetches on an
// event must never outrun the data the event describes.
func (h *Handlers) publish(name string, data any) {
	if h.d.Events == nil {
		return
	}
	h.d.Events.Publish(name, data)
}
```

Call it at the end of the success path in `CreateBlock`, `UpdateBlock`, `PatchBlock` (Task 4), and `DeleteBlock`:

```go
	h.publish(events.PlanInvalidated, map[string]any{"reason": "block", "id": rec.ID})
```

and in `PatchSeriesState`, after the state write and its snapshot invalidation:

```go
	h.publish(events.SeriesChanged, map[string]any{"show_title": showTitle})
	h.publish(events.PlanInvalidated, map[string]any{"reason": "series", "id": showTitle})
```

Both, because a cursor write changes the tracked row *and* invalidates pending occurrences — the History page cares about the first, the guide about the second.

- [ ] **Step 5: Run the tests**

Run: `make test`
Expected: all packages `ok`.

- [ ] **Step 6: Commit**

```bash
git add internal/service/ internal/api/
git commit -m "feat(service,api): announce applies, block edits and cursor writes on the live link"
```

---

## Task 4: `PATCH /blocks/{id}` and `If-Match` on `PUT`

**Files:**
- Modify: `api/openapi.yaml`, `internal/api/blocks.go`, `internal/api/blocks_test.go`
- Regenerated: `internal/api/gen/server.gen.go`, `web/assets/ts/gen/types.d.ts`

**Interfaces:**
- Consumes: `store.BlockRecord.UpdatedAt` (already persisted and already on the wire).
- Produces: `PATCH /api/v1/blocks/{id}` with body `{enabled?: bool}` → `200 BlockRecord`; `PUT /api/v1/blocks/{id}` requires `If-Match: <updated_at>` and answers `412` on mismatch.

**Why now:** this slice is what makes concurrent edits visible, and therefore what makes a lost update likely. `PUT` replaces a whole spec, so two tabs saving the same block silently discard one operator's work. `PATCH` is the field-scoped complement: a toggle cannot lose an unrelated edit because it never sends one.

**`If-Match` value:** `updated_at` in RFC3339Nano, quoted as an entity-tag (`If-Match: "2026-09-09T07:18:04.123456789Z"`). Compare by parsing both sides to `time.Time` and testing `.Equal`, not by string equality — a client that round-trips the timestamp through JSON may re-render it with a different but equivalent representation.

- [ ] **Step 1: Extend the contract**

Add to the `/blocks/{id}` path item:

```yaml
    patch:
      operationId: patchBlock
      description: >-
        Field-scoped write for toggles. Only fields present in the body
        change, so a toggle cannot discard an unrelated edit the way a
        full-spec PUT can. No If-Match required, for the same reason.
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: "#/components/schemas/BlockPatch"
      responses:
        "200":
          description: updated
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/BlockRecord"
        "400":
          $ref: "#/components/responses/Problem"
        "404":
          $ref: "#/components/responses/Problem"
```

and on the existing `put:`, a required header parameter plus the new response:

```yaml
      parameters:
        - name: If-Match
          in: header
          required: true
          schema:
            type: string
          description: >-
            The block's current `updated_at`, as returned by GET. A
            mismatch means someone else saved since this client last
            read, and the write is refused with 412 rather than
            discarding their edit.
      responses:
        "412":
          $ref: "#/components/responses/Problem"
```

and the schema:

```yaml
    BlockPatch:
      type: object
      description: Field-scoped block update; absent fields are left unchanged.
      properties:
        enabled:
          type: boolean
```

Run `make generate && make web-types`.

- [ ] **Step 2: Write the failing tests**

```go
func TestUpdateBlock_RequiresIfMatch(t *testing.T) {
	h, s := newTestServerWithStore(t)
	rec := seedBlock(t, s) // existing helper, or create one via the API

	w := doRequestWithHeaders(t, h, http.MethodPut, "/blocks/"+rec.ID, validBody(rec), nil)
	assert.Equal(t, http.StatusBadRequest, w.Code, "a PUT with no If-Match is refused")
}

func TestUpdateBlock_412OnStaleIfMatch(t *testing.T) {
	h, s := newTestServerWithStore(t)
	rec := seedBlock(t, s)

	stale := rec.UpdatedAt.Add(-time.Hour).UTC().Format(time.RFC3339Nano)
	w := doRequestWithHeaders(t, h, http.MethodPut, "/blocks/"+rec.ID, validBody(rec),
		map[string]string{"If-Match": `"` + stale + `"`})
	require.Equal(t, http.StatusPreconditionFailed, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "reload")
}

func TestUpdateBlock_SucceedsOnCurrentIfMatch(t *testing.T) {
	h, s := newTestServerWithStore(t)
	rec := seedBlock(t, s)

	current := rec.UpdatedAt.UTC().Format(time.RFC3339Nano)
	w := doRequestWithHeaders(t, h, http.MethodPut, "/blocks/"+rec.ID, validBody(rec),
		map[string]string{"If-Match": `"` + current + `"`})
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

func TestPatchBlock_TogglesEnabledWithoutTouchingSpec(t *testing.T) {
	h, s := newTestServerWithStore(t)
	rec := seedBlock(t, s)
	before := rec.Spec

	w := doRequest(t, h, http.MethodPatch, "/blocks/"+rec.ID, strings.NewReader(`{"enabled":false}`))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	after, err := s.GetBlock(t.Context(), rec.ID)
	require.NoError(t, err)
	assert.False(t, after.Enabled)
	assert.Equal(t, before, after.Spec, "a toggle must not rewrite the spec")
}

func TestPatchBlock_EmptyBodyIs400(t *testing.T) {
	h, s := newTestServerWithStore(t)
	rec := seedBlock(t, s)
	w := doRequest(t, h, http.MethodPatch, "/blocks/"+rec.ID, strings.NewReader(`{}`))
	assert.Equal(t, http.StatusBadRequest, w.Code)
}
```

`doRequestWithHeaders` may not exist — add it next to `doRequest`, sharing its body, rather than duplicating the request plumbing.

- [ ] **Step 3: Run to verify they fail**

Run: `go test -race ./internal/api -run 'IfMatch|PatchBlock' -v`
Expected: FAIL — the PUT accepts a missing header, and `PatchBlock` is undefined.

- [ ] **Step 4: Implement**

In `UpdateBlock`, before applying the write:

```go
	// Lost-update protection. PUT replaces the whole spec, so without
	// this two tabs editing one block silently discard the slower
	// operator's work -- which the live link makes likely rather than
	// theoretical, since both tabs now see each other's changes land.
	raw := strings.Trim(r.Header.Get("If-Match"), `"`)
	if raw == "" {
		WriteProblem(w, r, http.StatusBadRequest, "missing If-Match",
			"send the block's current updated_at as If-Match; GET the block to read it")
		return
	}
	want, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		WriteProblem(w, r, http.StatusBadRequest, "malformed If-Match",
			"If-Match must be the block's updated_at in RFC3339 format")
		return
	}
	// Compared as instants, not strings: a client that round-trips the
	// timestamp through JSON may re-render it equivalently but not
	// identically.
	if !existing.UpdatedAt.Equal(want) {
		WriteProblem(w, r, http.StatusPreconditionFailed, "block changed since you loaded it",
			"someone else saved this block. Reload and re-apply your edit.")
		return
	}
```

`PatchBlock` mirrors `PatchSeriesState`'s shape: load the record (404 when absent), reject a body with no fields set (400), apply only the present fields, persist, publish `plan.invalidated`, and return the updated `BlockRecord`.

- [ ] **Step 5: Run the tests**

Run: `go test -race ./internal/api && make test`
Expected: PASS. Existing tests that `PUT` without `If-Match` will now fail — update them to send the header; that is the point of the change, not collateral damage.

- [ ] **Step 6: Commit**

```bash
git add api/openapi.yaml internal/api/ web/assets/ts/gen/types.d.ts
git commit -m "feat(api): add PATCH /blocks/{id} and require If-Match on PUT"
```

---

## Task 5: The reachability prober

**Files:**
- Create: `cmd/probe.go`, `cmd/probe_test.go`
- Modify: `cmd/serve.go`

**Interfaces:**
- Consumes: `events.Hub`, the Tunarr client.
- Produces: `startTunarrProbe(ctx context.Context, p probeDeps)` publishing `status.changed {tunarr_reachable}` **only on a transition**.

**Why this exists:** `status.changed` is in the spec's catalog with no producer in the codebase. Nothing server-side notices Tunarr going away between applies, so without this the event could never fire and the bezel's TUNARR readout would have nothing to be live about.

**Publish-on-transition, not on every probe.** A per-interval publish would wake every tab every 30 seconds to tell it nothing changed. The prober holds the last known state and publishes only when it flips, which also makes the event's arrival meaningful: it *is* the news.

- [ ] **Step 1: Write the failing test**

```go
func TestTunarrProbe_PublishesOnlyOnTransition(t *testing.T) {
	hub := events.NewHub(16)
	sub, cancel := hub.Subscribe(context.Background(), 0)
	defer cancel()

	reachable := atomic.Bool{}
	reachable.Store(true)

	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	startTunarrProbe(ctx, probeDeps{
		Hub:      hub,
		Interval: 10 * time.Millisecond,
		Logger:   discardLogger(),
		Check:    func(context.Context) bool { return reachable.Load() },
	})

	// First probe establishes the baseline and announces it once.
	first := waitForEvent(t, sub, time.Second)
	require.Equal(t, events.StatusChanged, first.Name)
	assert.Equal(t, true, first.Data.(map[string]any)["tunarr_reachable"])

	// Steady state: several intervals, no further events.
	select {
	case ev := <-sub:
		t.Fatalf("published %v with no state change", ev.Data)
	case <-time.After(80 * time.Millisecond):
	}

	// A flip is news.
	reachable.Store(false)
	second := waitForEvent(t, sub, time.Second)
	assert.Equal(t, false, second.Data.(map[string]any)["tunarr_reachable"])
}
```

with a small `waitForEvent` helper in the test file.

- [ ] **Step 2: Run to verify it fails**

Run: `go test -race ./cmd -run TunarrProbe -v`
Expected: FAIL — `startTunarrProbe` undefined.

- [ ] **Step 3: Implement**

```go
// probeDeps is what the reachability prober needs. Check is injected so
// the test can flip reachability without a live Tunarr; production passes
// a closure over the Tunarr client's own health call.
type probeDeps struct {
	Hub      *events.Hub
	Interval time.Duration
	Logger   *slog.Logger
	Check    func(context.Context) bool
}

// defaultProbeInterval is how often serve asks Tunarr whether it is
// there. Frequent enough that the bezel's reading is worth trusting,
// infrequent enough to be invisible in Tunarr's access log.
const defaultProbeInterval = 30 * time.Second

// startTunarrProbe watches Tunarr's reachability and publishes
// status.changed when it FLIPS -- never on every probe. A per-interval
// publish would wake every connected tab twice a minute to tell it
// nothing happened; publishing only on a transition is what makes the
// event's arrival meaningful.
//
// The very first probe always publishes, so a tab that connects before
// any flip still learns the current state without waiting for one.
func startTunarrProbe(ctx context.Context, p probeDeps) {
	if p.Hub == nil || p.Check == nil {
		return
	}
	if p.Interval <= 0 {
		p.Interval = defaultProbeInterval
	}

	go func() {
		ticker := time.NewTicker(p.Interval)
		defer ticker.Stop()

		var last bool
		seeded := false
		for {
			now := p.Check(ctx)
			if !seeded || now != last {
				p.Hub.Publish(events.StatusChanged, map[string]any{"tunarr_reachable": now})
				if seeded {
					p.Logger.Info("tunarr reachability changed", "reachable", now)
				}
				last, seeded = now, true
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}
```

In `cmd/serve.go`, build the hub once, pass it to `service.NewRunner` (as `RunnerOptions.Events`) and into `api.Deps.Events`, and start the prober with a `Check` closure that calls whatever `GetStatus` already uses to test reachability — reuse that path rather than inventing a second definition of "reachable".

- [ ] **Step 4: Run the tests**

Run: `go test -race ./cmd && make test`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/
git commit -m "feat(serve): probe Tunarr reachability and announce changes on the live link"
```

---

## Task 6: Phase A gate

- [ ] **Step 1: Lint and test**

Run: `golangci-lint run && make test`
Expected: 0 issues, all packages `ok`. (`make lint` will still fail at `gosec` — pre-existing, see Global Constraints.)

- [ ] **Step 2: Watch the stream by hand**

```bash
make build
./bin/schedularr --config config.yaml serve --listen :8484 --insecure-no-auth &
curl -N -H 'Accept: text/event-stream' http://127.0.0.1:8484/api/v1/events
```

Expected: an immediate `event: heartbeat` frame carrying `server_time`, another every 15s. In a second terminal, toggle a block (`PATCH /api/v1/blocks/{id}`) and watch a `plan.invalidated` frame arrive on the first.

- [ ] **Step 3: Prove resume**

Reconnect with `-H 'Last-Event-ID: <id>'` after publishing a couple of events while disconnected, and confirm only the unseen ones replay.

- [ ] **Step 4: Prove the apply path**

Trigger an apply and confirm one `apply.completed` frame with a `run_id` that matches a row in `GET /api/v1/applies`.

- [ ] **Step 5: Commit any fixes**

---

# Phase B — The client

> Every task here invokes the `impeccable` skill first. Reuse the shared runtime and existing partials; do not re-implement them.

## Task 7: The fetch-stream reader and its parser

**Files:**
- Create: `web/assets/ts/runtime/stream.ts`, `web/tests/stream.test.ts`

**Interfaces:**
- Produces:
  - `parseFrames(buffer: string): {frames: ParsedFrame[]; rest: string}` — the pure, testable half
  - `ParsedFrame{id?: string; event: string; data: string}`
  - `connectStream(opts): () => void` — the impure half, returning a disconnect function

**The parser is the part that gets tested, and it is the part that breaks.** An SSE stream arrives in arbitrary chunks: a frame can be split mid-line, mid-field, or between the two newlines that terminate it. Line endings may be `\n` or `\r\n`. `data:` may repeat across lines and must be joined with `\n`. Parse against a retained buffer, never against a single chunk.

- [ ] **Step 1: Write the failing tests**

```ts
import assert from "node:assert/strict";
import test from "node:test";

const { parseFrames } = await import("../assets/ts/runtime/stream.ts");

test("parses a complete frame", () => {
  const { frames, rest } = parseFrames("id: 7\nevent: status.changed\ndata: {\"a\":1}\n\n");
  assert.equal(frames.length, 1);
  assert.deepEqual(frames[0], { id: "7", event: "status.changed", data: '{"a":1}' });
  assert.equal(rest, "");
});

test("holds an incomplete frame in the buffer until it terminates", () => {
  const first = parseFrames("event: heartbeat\ndata: {\"server_ti");
  assert.equal(first.frames.length, 0);
  const second = parseFrames(first.rest + 'me":"x"}\n\n');
  assert.equal(second.frames.length, 1);
  assert.equal(second.frames[0]?.data, '{"server_time":"x"}');
});

test("handles CRLF line endings", () => {
  const { frames } = parseFrames("event: heartbeat\r\ndata: {}\r\n\r\n");
  assert.equal(frames.length, 1);
  assert.equal(frames[0]?.event, "heartbeat");
});

test("joins repeated data lines with a newline, per the SSE spec", () => {
  const { frames } = parseFrames("event: x\ndata: one\ndata: two\n\n");
  assert.equal(frames[0]?.data, "one\ntwo");
});

test("parses several frames from one chunk", () => {
  const { frames } = parseFrames("event: a\ndata: 1\n\nevent: b\ndata: 2\n\n");
  assert.deepEqual(frames.map((f) => f.event), ["a", "b"]);
});

test("ignores comment lines and unknown fields", () => {
  const { frames } = parseFrames(": keep-alive\nevent: a\nretry: 5000\ndata: 1\n\n");
  assert.equal(frames.length, 1);
  assert.equal(frames[0]?.event, "a");
});

test("a frame with no event name defaults to message", () => {
  const { frames } = parseFrames("data: 1\n\n");
  assert.equal(frames[0]?.event, "message");
});
```

- [ ] **Step 2: Run to verify they fail**

Run: `npm test --prefix web`
Expected: FAIL — module not found.

- [ ] **Step 3: Implement**

`parseFrames` splits the retained buffer on a blank line (`\n\n` after normalising `\r\n` to `\n`), parses each complete block field-by-field, and returns the trailing partial as `rest`.

`connectStream` wraps `fetch(apiPath("/events"), {headers: {Authorization, Accept, Last-Event-ID?}, signal})`, reads `response.body.getReader()` in a loop, decodes with a `TextDecoder({stream: true})`, feeds `parseFrames`, and hands each frame to a callback. It owns the **degradation ladder**:

```ts
/**
 * The rungs, in order. Nothing functional depends on the stream, so each
 * rung degrades what the operator SEES, never what they can do:
 *
 *   LIVE      the stream is connected, or reconnecting with backoff
 *             (1s doubling to 30s) resuming from Last-Event-ID.
 *   POLL      three consecutive connection failures. The stream keeps
 *             trying in the background while a 60s poll of /status and
 *             the page's primary GET takes over.
 *   LINK LOST the network is gone. Polling stops, and the bezel offers a
 *             manual reconnect -- an honest instrument says it has no
 *             reading rather than showing a stale one.
 */
```

`EventSource` must not appear anywhere in this file. Add a comment saying why, so a future contributor does not "simplify" it into existence.

- [ ] **Step 4: Run the tests**

Run: `make web-check && npm test --prefix web`
Expected: PASS.

- [ ] **Step 5: Commit**

---

## Task 8: The event bus and subscribe-on-init

**Files:**

- Create: `web/assets/ts/runtime/bus.ts`
- Modify: `web/assets/ts/runtime/shell.ts`

**Interfaces:**

- Produces: `subscribe(event: string, handler: (data: unknown) => void): () => void`, `publishLocal(event, data)`, `linkState(): LinkState`, `onLinkChange(cb)`.

`shell.ts` starts exactly one stream per page and routes frames onto the bus. The existing 60s `/status` poll is not deleted — it becomes the POLL rung, and must be **suspended while the stream is healthy** so the two never run at once.

**Skew correction.** Each `heartbeat` updates a smoothed offset (`server_time − Date.now()`); the runtime exports `serverNow()` and every relative timestamp and the guide's sweep cursor read it instead of `Date.now()`. This is the permanent fix for the skew class of bug the retired `schedule.ts` had.

**Lifecycle.** `visibilitychange` disconnects the stream when the tab is hidden and, on becoming visible, reconnects and refetches the current page's primary GET. The module-level one-shot `started` guards in each page bundle are retired in favour of subscribe-on-init.

- [ ] Steps mirror Task 7: failing test for the bus's subscribe/unsubscribe and the skew smoothing, implement, verify, commit.

---

## Task 9: The bezel LINK legend

**Files:** `web/layouts/_default/baseof.html`, `web/assets/ts/runtime/shell.ts`, `web/assets/css/main.css`, `web/layouts/kit/list.html`

The slot is already reserved — `baseof.html` carries a comment saying the LINK legend "deliberately does not exist yet — it needs the SSE live-link slice's event stream to be honest." Replace that comment when adding the item.

A coded-legend dot plus text (`LIVE` / `POLL` / `LINK LOST`), matching TUNARR's existing pattern: colour never carries the state alone. Two motion moments and no more — one 200ms amber pulse on entering LINK LOST, one green on reacquire — both suppressed under `prefers-reduced-motion`, with `forced-colors` fallbacks since the pulse is box-shadow-borne. `LINK LOST` carries a manual reconnect action.

- [ ] Compute contrast for every new pairing in both palettes and record it in `web/DESIGN.md`'s evidence section, in the format the v0.5.7 table uses. Add `/kit/` fixtures for all three LINK states.

---

## Task 10: Guide routing and the dirty guards

**Files:** `web/assets/ts/pages/guide.ts`, `web/assets/css/main.css`

- `apply.completed` / `plan.invalidated` → refetch `GET /schedule`, **debounced 2s**, with the trace draw-in — but only when the guide is idle in committed mode and the tab is visible.
- While the inspector is open **or** a draft is armed: do not refetch. Show a pinned `LINEUP CHANGED — REFRESH` line instead, and a draft additionally disarms with `SOURCE CHANGED — RE-PREVIEW`.
- The sweep cursor advances on its own 60s timer corrected by the heartbeat offset, never on the stream.

The guard is the point of this task: an auto-refetch that discards an armed draft or a half-read inspector is worse than no live link at all.

- [ ] Failing tests for the debounce and both guard predicates (pure functions, testable without a DOM), implement, verify, commit.

---

## Task 11: History and Blocks routing

**Files:** `web/assets/ts/pages/history.ts`, `web/assets/ts/pages/blocks.ts`

- History ← `apply.completed` prepends to RUNS and refetches AS-RUN; `series.changed` refetches TRACKED. Newly arrived rows take the trace draw-in.
- Blocks ← `plan.invalidated` refetches the list, **unless an editor is dirty** — then queue until it closes. An external change to the *open* block raises the inline reload note; it never clobbers.

- [ ] Same shape: failing tests for the queue-until-close predicate, implement, verify, commit.

---

## Task 12: `If-Match` and `PATCH` on the client

**Files:** `web/assets/ts/pages/blocks.ts`, `web/assets/ts/runtime/api.ts`

Block save sends `If-Match: "<updated_at>"` from the record it loaded. A `412` renders the inline problem "reload and re-apply your edit" with a reload action — not a silent retry, which would be the lost update the header exists to prevent. The enable/disable toggle moves from `PUT` to `PATCH`.

- [ ] Failing test, implement, verify, commit.

---

## Task 13: Phase B gate

- [ ] `make web` clean; `npm test --prefix web` green.
- [ ] Run the real binary and verify by hand, in a browser: two tabs open, a block toggled in one appears in the other; the bezel reads LIVE; killing the server flips it to POLL then LINK LOST with a working reconnect; an armed draft is *not* discarded by an incoming event; a hidden tab stops streaming and catches up when shown.
- [ ] Batched screenshot round, desktop and mobile, light and dark, covering the three LINK states.
- [ ] Run the mechanical detector once over the changed UI files.

---

# Phase C — Documentation and release

## Task 14: Docs, changelog, roadmap

- [ ] `docs/api-reference.md`: `GET /events` (frames, heartbeat, resume, the 503), `PATCH /blocks/{id}`, `If-Match` and the 412 on `PUT`.
- [ ] `docs/web-ui-guide.md`: the LINK legend and its three states, what refetches and what does not, both dirty guards, and the plain statement that no page needs the stream.
- [ ] `docs/architecture.md`: the hub between producers and connected tabs.
- [ ] `docs/deployment.md`: **the standing proxy requirement** — any reverse proxy or SSO fronting must not buffer `/api/v1/events`; name `X-Accel-Buffering: no` and `proxy_buffering off`.
- [ ] `CLAUDE.md`, `AGENTS.md`, `GEMINI.md`: the new `internal/events` package in the architecture tree.
- [ ] `web/DESIGN.md`: the LINK legend, the two pulses, and the WCAG evidence table.
- [ ] `docs/roadmap.md`: convert "Then — Live link (SSE): pending" into a shipped entry in the v0.5.7 entry's voice, naming what was deliberately left out.
- [ ] `TODO.md`: a "Deferred (live link)" section.
- [ ] `CHANGELOG.md`: a new version heading with Added / Changed / Fixed, plus the compare link at the foot of the file.
- [ ] Run `stop-slop` over every paragraph written here, and `markdownlint-cli2` to confirm no new findings against the baseline.
- [ ] Hand the operator the paste text for the cluster wiki page and the Obsidian note — including the proxy-buffering requirement, which is an operational change for the cluster.

---

## Self-Review

**Spec coverage (§6 unless noted).**

| Requirement                                                  | Task                                                |
|--------------------------------------------------------------|-----------------------------------------------------|
| `GET /api/v1/events`, `text/event-stream`                    | 2                                                   |
| Hand-rolled fetch/ReadableStream client, never `EventSource` | 7                                                   |
| Goroutine-per-subscriber broadcast hub                       | 1                                                   |
| `heartbeat {server_time}` every 15s                          | 2                                                   |
| Monotonic ids, `Last-Event-ID` resume from a ring buffer     | 1, 2                                                |
| `apply.completed`                                            | 3                                                   |
| `plan.invalidated`                                           | 3                                                   |
| `status.changed`                                             | 5                                                   |
| `series.changed`                                             | 3                                                   |
| `history.appended`                                           | folded into `apply.completed` — see Scope decisions |
| Bezel ← `status.changed`; heartbeat drives the skew offset   | 8, 9                                                |
| Guide refetch, debounced, with inspector/draft guards        | 10                                                  |
| History prepend; Blocks refetch unless dirty                 | 11                                                  |
| Sweep on a local timer, corrected — not on the stream        | 8, 10                                               |
| Subscribe-on-init replaces the one-shot guards               | 8                                                   |
| `visibilitychange` pause/resume                              | 8                                                   |
| Degradation ladder: backoff → POLL → LINK LOST               | 7, 9                                                |
| No-buffering headers; proxy requirement recorded             | 2, 14                                               |
| `If-Match` on `PUT /blocks/{id}`, 412                        | 4, 12                                               |
| `PATCH /blocks/{id}`                                         | 4, 12                                               |
| Bezel LINK legend + two pulses (§3.4, §5)                    | 9                                                   |
| Gate: parser unit tests + e2e against the real server (§9.2) | 7, 13                                               |

**Placeholders:** Tasks 8–12 carry their design constraints and interfaces but compress the TDD step list to a single line, because each follows the identical failing-test → implement → verify → commit shape spelled out in full in Tasks 1, 2 and 7. An implementer working Task 10 has the predicates, the debounce, and the guard rules stated; what they do not have is boilerplate repeated a fifth time. If that proves too thin in execution, expand the task in place rather than improvising.

**Type consistency:** `events.Hub` is the same type in Tasks 1, 2, 3 and 5. `RunnerOptions.Events` (Task 3) matches `api.Deps.Events` (Task 2) in type and nil-tolerance. `parseFrames`/`ParsedFrame` (Task 7) are what `shell.ts` consumes in Task 8. The event name constants are defined once, in `internal/events`, and appear on the wire and in the client's bus switch as the same strings.

**Risk to watch during execution:** Task 4 changes an existing endpoint's contract — every current `PUT /blocks/{id}` caller must start sending `If-Match`, including the web UI and any test that exercises it. Expect that task's diff to be wider than its description.
