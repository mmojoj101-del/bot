package connector

import (
	"fmt"
	"sync"
)

// MemoryRegistry is a thread-safe in-memory implementation of ConnectorRegistry.
// It holds only ready-to-use, initialized connectors.
//
// Populate via:
//
//	reg := NewMemoryRegistry()
//	reg.Add(myHTTPConnector)
//	reg.Add(mySMPPConnector)
//
// Connector construction and lifecycle are separate concerns (ConnectorFactory).
type MemoryRegistry struct {
	mu     sync.RWMutex
	items  map[string]Connector
}

// NewMemoryRegistry creates an empty MemoryRegistry.
func NewMemoryRegistry() *MemoryRegistry {
	return &MemoryRegistry{
		items: make(map[string]Connector),
	}
}

// Add registers a connector. Returns an error if the ID already exists.
func (r *MemoryRegistry) Add(c Connector) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	id := c.ID()
	if _, exists := r.items[id]; exists {
		return fmt.Errorf("memory registry: connector %q already registered", id)
	}
	r.items[id] = c
	return nil
}

// MustAdd registers a connector and panics on conflict (convenience for tests).
func (r *MemoryRegistry) MustAdd(c Connector) {
	if err := r.Add(c); err != nil {
		panic(err)
	}
}

// Remove unregisters a connector by ID.
func (r *MemoryRegistry) Remove(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.items, id)
}

// Get returns the connector with the given ID.
// Returns an error if not found.
func (r *MemoryRegistry) Get(id string) (Connector, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	c, ok := r.items[id]
	if !ok {
		return nil, fmt.Errorf("memory registry: connector %q not found", id)
	}
	return c, nil
}

// List returns all registered connectors in insertion order.
func (r *MemoryRegistry) List() []Connector {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]Connector, 0, len(r.items))
	for _, c := range r.items {
		result = append(result, c)
	}
	return result
}

// Len returns the number of registered connectors.
func (r *MemoryRegistry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.items)
}
