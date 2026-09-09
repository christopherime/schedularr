// Package events provides the in-process broadcast hub behind
// GET /api/v1/events. One publisher fans an event out to every connected
// browser tab; nothing outside this process ever sees it, and nothing in
// this process depends on delivery succeeding.
//
// The asymmetry is deliberate: a publish happens on an apply's critical
// path, while a subscriber is a browser tab that may have stopped
// reading at any moment. So publishing never blocks, never returns an
// error, and never waits on a reader -- a tab that falls behind loses
// events and repairs itself by resuming from Last-Event-ID.
package events

import (
	"context"
	"sync"
)

// Event names. These are the wire values in the SSE `event:` field and
// the strings the client's bus switches on, so they are part of the
// contract -- see api/openapi.yaml's /events description.
//
// history.appended from the design spec's catalog is deliberately absent:
// it would fire at the same instant, from the same place, carrying the
// same run_id as ApplyCompleted, and two events for one fact is a
// catalog that lies about its own granularity. If history ever grows a
// path that appends outside an apply, it earns its own event then.
const (
	Heartbeat       = "heartbeat"
	ApplyCompleted  = "apply.completed"
	PlanInvalidated = "plan.invalidated"
	StatusChanged   = "status.changed"
	SeriesChanged   = "series.changed"
)

// defaultRingCap is how many recent events the hub retains for
// Last-Event-ID resume. Sized for a reconnect measured in seconds, not
// for replaying a session.
const defaultRingCap = 128

// Event is one broadcast. Data is marshaled to JSON by the HTTP handler
// rather than here: the hub stays transport-agnostic, and a marshaling
// failure belongs to the one connection that hit it instead of to every
// subscriber.
type Event struct {
	ID   int64
	Name string
	Data any
}

// Hub fans events out to subscribers. The zero value is not usable; call
// NewHub.
type Hub struct {
	mu      sync.Mutex
	nextID  int64
	subs    map[int]*subscriber
	nextSub int
	buffer  int

	// ring holds the most recent events for Last-Event-ID resume. It is
	// deliberately small and lossy: it backs a reconnect that happens
	// within seconds, not an event store. A client asking to resume from
	// before the ring's oldest entry gets only what survives and
	// refetches for the rest -- losing an event is recoverable, while
	// pretending to have delivered one is not.
	ring    []Event
	ringCap int
}

type subscriber struct {
	ch   chan Event
	once sync.Once
}

// NewHub returns a hub whose subscribers each get a channel buffered to
// buffer events. A non-positive buffer becomes 1, since an unbuffered
// channel would make every Publish drop for every subscriber that isn't
// blocked in a receive at that exact instant.
func NewHub(buffer int) *Hub {
	if buffer < 1 {
		buffer = 1
	}
	return &Hub{
		subs:    make(map[int]*subscriber),
		buffer:  buffer,
		ringCap: defaultRingCap,
	}
}

// Publish broadcasts an event to every current subscriber and records it
// in the resume ring. It never blocks and never fails: a subscriber whose
// buffer is full simply misses this event.
//
// Callers publish AFTER the write they are announcing has committed. A
// tab that refetches on an event must not be able to outrun the data the
// event describes.
func (h *Hub) Publish(name string, data any) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.nextID++
	ev := Event{ID: h.nextID, Name: name, Data: data}

	h.ring = append(h.ring, ev)
	if len(h.ring) > h.ringCap {
		h.ring = h.ring[len(h.ring)-h.ringCap:]
	}

	for _, sub := range h.subs {
		send(sub, ev)
	}
}

// LastID returns the id of the most recent event, or 0 when none has been
// published.
func (h *Hub) LastID() int64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.nextID
}

// Subscribe registers a new subscriber and replays whatever the ring
// still holds past lastID, so a reconnecting client picks up where it
// left off. Pass lastID 0 for a fresh subscription.
//
// The returned function unsubscribes and closes the channel; it is
// idempotent and safe to call from several goroutines. Canceling ctx
// does the same thing, so a dropped HTTP connection cleans itself up
// without the handler having to notice.
func (h *Hub) Subscribe(ctx context.Context, lastID int64) (<-chan Event, func()) {
	h.mu.Lock()
	id := h.nextSub
	h.nextSub++
	sub := &subscriber{ch: make(chan Event, h.buffer)}
	h.subs[id] = sub

	for _, ev := range h.ring {
		if ev.ID > lastID {
			send(sub, ev)
		}
	}
	h.mu.Unlock()

	unsubscribe := func() {
		h.mu.Lock()
		delete(h.subs, id)
		h.mu.Unlock()
		// Closed exactly once even under concurrent callers, and only
		// after the hub can no longer reach it -- so Publish can never
		// send on a closed channel.
		sub.once.Do(func() { close(sub.ch) })
	}

	go func() {
		<-ctx.Done()
		unsubscribe()
	}()

	return sub.ch, unsubscribe
}

// send delivers one event to one subscriber without blocking. A full
// buffer means this subscriber loses the event; its client resumes from
// Last-Event-ID and the ring fills the gap. Callers hold h.mu.
func send(sub *subscriber, ev Event) {
	select {
	case sub.ch <- ev:
	default:
	}
}
