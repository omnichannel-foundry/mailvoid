package events

import (
	"sync"
	"time"
)

// Sink persists or forwards an event. Sink implementations must not block for
// long; the bus calls them synchronously.
type Sink interface {
	Write(Event) error
}

// SinkFunc adapts a function to Sink.
type SinkFunc func(Event) error

// Write implements Sink.
func (f SinkFunc) Write(e Event) error { return f(e) }

// Bus fans an event out to sinks and live subscribers while keeping a small
// in-memory history for dashboards.
type Bus struct {
	mu       sync.RWMutex
	sinks    []Sink
	subs     map[int]chan Event
	nextSub  int
	ring     []Event
	ringSize int
}

// NewBus creates a bus with the provided sinks.
func NewBus(sinks ...Sink) *Bus {
	return &Bus{
		sinks:    sinks,
		subs:     make(map[int]chan Event),
		ringSize: 256,
		ring:     make([]Event, 0, 256),
	}
}

// AddSink appends a sink after construction.
func (b *Bus) AddSink(s Sink) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.sinks = append(b.sinks, s)
}

// Emit timestamps and normalizes the event, persists it to sinks, records it in
// the history ring, and broadcasts it to subscribers.
func (b *Bus) Emit(ev Event) Event {
	if ev.ID == "" {
		ev.ID = newID()
	}
	if ev.Timestamp.IsZero() {
		ev.Timestamp = time.Now().UTC()
	}
	if ev.Severity == "" {
		ev.Severity = SeverityInfo
	}

	b.mu.Lock()
	sinks := append([]Sink(nil), b.sinks...)
	b.ring = append(b.ring, ev)
	if len(b.ring) > b.ringSize {
		b.ring = b.ring[len(b.ring)-b.ringSize:]
	}
	subs := make([]chan Event, 0, len(b.subs))
	for _, ch := range b.subs {
		subs = append(subs, ch)
	}
	b.mu.Unlock()

	for _, s := range sinks {
		_ = s.Write(ev)
	}
	for _, ch := range subs {
		select {
		case ch <- ev:
		default:
		}
	}
	return ev
}

// Subscribe returns a channel of live events and an unsubscribe function.
func (b *Bus) Subscribe(buffer int) (<-chan Event, func()) {
	if buffer <= 0 {
		buffer = 32
	}
	ch := make(chan Event, buffer)

	b.mu.Lock()
	id := b.nextSub
	b.nextSub++
	b.subs[id] = ch
	b.mu.Unlock()

	unsubscribe := func() {
		b.mu.Lock()
		if existing, ok := b.subs[id]; ok {
			delete(b.subs, id)
			close(existing)
		}
		b.mu.Unlock()
	}
	return ch, unsubscribe
}

// Recent returns up to limit of the most recent events, oldest first.
func (b *Bus) Recent(limit int) []Event {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if limit <= 0 || limit > len(b.ring) {
		limit = len(b.ring)
	}
	out := make([]Event, limit)
	copy(out, b.ring[len(b.ring)-limit:])
	return out
}
