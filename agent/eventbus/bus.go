package eventbus

import "sync"

// Event represents a pipeline event (iteration start, stream delta, etc).
type Event struct {
	Kind string
	Data interface{}
}

// EventBus provides pub/sub for pipeline observability.
// Display, logging, and metrics subscribe to events.
// The pipeline never calls print() or log() directly.
type EventBus struct {
	mu          sync.RWMutex
	subscribers map[string][]chan Event
	closed      bool
}

// NewEventBus creates a new EventBus.
func NewEventBus() *EventBus {
	return &EventBus{
		subscribers: make(map[string][]chan Event),
	}
}

// Subscribe returns a channel that receives events of the given kind.
func (b *EventBus) Subscribe(kind string) <-chan Event {
	ch := make(chan Event, 64) // Buffered to avoid blocking emitters
	b.mu.Lock()
	defer b.mu.Unlock()
	b.subscribers[kind] = append(b.subscribers[kind], ch)
	return ch
}

// SubscribeAll returns a channel that receives all events.
func (b *EventBus) SubscribeAll() <-chan Event {
	return b.Subscribe("*")
}

// Unsubscribe removes a subscriber channel. Call this when done to prevent leaks.
func (b *EventBus) Unsubscribe(kind string, ch <-chan Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	subs := b.subscribers[kind]
	for i, sub := range subs {
		if sub == ch {
			// Remove and close the channel
			b.subscribers[kind] = append(subs[:i], subs[i+1:]...)
			close(sub)
			return
		}
	}
}

// Emit sends an event to all subscribers of that kind and "*" subscribers.
func (b *EventBus) Emit(event Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.closed {
		return
	}
	// Send to kind-specific subscribers
	for _, ch := range b.subscribers[event.Kind] {
		select {
		case ch <- event:
		default:
			// Drop if subscriber is too slow (non-blocking)
		}
	}
	// Send to wildcard subscribers
	if event.Kind != "*" {
		for _, ch := range b.subscribers["*"] {
			select {
			case ch <- event:
			default:
			}
		}
	}
}

// Close closes all subscriber channels.
func (b *EventBus) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed = true
	for _, subs := range b.subscribers {
		for _, ch := range subs {
			close(ch)
		}
	}
	b.subscribers = make(map[string][]chan Event)
}