package event

import (
	"sync"
	"sync/atomic"
)

// subscription represents a registered handler with a unique ID.
type subscription struct {
	id      uint64
	handler Handler
}

// MemoryBus is an in-memory implementation of the EventBus interface.
type MemoryBus struct {
	mu       sync.RWMutex
	handlers map[string][]*subscription
	nextID   uint64
	closed   bool
}

// NewMemoryBus creates a new in-memory event bus.
func NewMemoryBus() *MemoryBus {
	return &MemoryBus{
		handlers: make(map[string][]*subscription),
	}
}

// Publish publishes an event to all subscribed handlers.
func (b *MemoryBus) Publish(event Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.closed {
		return
	}

	subs := b.handlers[event.Type]
	for _, sub := range subs {
		sub.handler(event)
	}
}

// Subscribe subscribes a handler to an event type.
// Returns an unsubscribe function that removes the subscription by ID.
func (b *MemoryBus) Subscribe(eventType string, handler Handler) func() {
	b.mu.Lock()
	defer b.mu.Unlock()

	id := atomic.AddUint64(&b.nextID, 1)
	sub := &subscription{id: id, handler: handler}
	b.handlers[eventType] = append(b.handlers[eventType], sub)

	return func() {
		b.removeByID(eventType, id)
	}
}

// removeByID removes a subscription by its unique ID.
func (b *MemoryBus) removeByID(eventType string, id uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()

	subs := b.handlers[eventType]
	for i, sub := range subs {
		if sub.id == id {
			b.handlers[eventType] = append(subs[:i], subs[i+1:]...)
			return
		}
	}
}

// Unsubscribe is kept for interface compatibility.
// Prefer using the closure returned by Subscribe() instead.
func (b *MemoryBus) Unsubscribe(eventType string, _ Handler) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Clear all handlers for this event type
	delete(b.handlers, eventType)
}

// Close closes the event bus and clears all handlers.
func (b *MemoryBus) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.closed = true
	b.handlers = nil
}
