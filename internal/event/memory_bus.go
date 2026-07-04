package event

import (
	"fmt"
	"sync"
)

// MemoryBus is an in-memory implementation of the EventBus interface.
type MemoryBus struct {
	mu       sync.RWMutex
	handlers map[string][]Handler
	closed   bool
}

// NewMemoryBus creates a new in-memory event bus.
func NewMemoryBus() *MemoryBus {
	return &MemoryBus{
		handlers: make(map[string][]Handler),
	}
}

// Publish publishes an event to all subscribed handlers.
func (b *MemoryBus) Publish(event Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.closed {
		return
	}

	handlers := b.handlers[event.Type]
	for _, handler := range handlers {
		handler(event)
	}
}

// Subscribe subscribes a handler to an event type.
// Returns an unsubscribe function.
func (b *MemoryBus) Subscribe(eventType string, handler Handler) func() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.handlers[eventType] = append(b.handlers[eventType], handler)

	return func() {
		b.Unsubscribe(eventType, handler)
	}
}

// Unsubscribe removes a handler from an event type.
func (b *MemoryBus) Unsubscribe(eventType string, handler Handler) {
	b.mu.Lock()
	defer b.mu.Unlock()

	handlers := b.handlers[eventType]
	for i, h := range handlers {
		// Compare function values directly
		// Use string representation of pointer as a safe comparison
		if fmt.Sprintf("%p", h) == fmt.Sprintf("%p", handler) {
			b.handlers[eventType] = append(handlers[:i], handlers[i+1:]...)
			return
		}
	}
}

// Close closes the event bus and clears all handlers.
func (b *MemoryBus) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.closed = true
	b.handlers = nil
}
