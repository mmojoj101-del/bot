package connector

import (
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// CircuitBreakerStore manages circuit breakers keyed by connector ID.
type CircuitBreakerStore interface {
	// Allow returns true if the call should proceed for the given connector.
	Allow(connectorID string) bool
	// Success records a successful call.
	Success(connectorID string)
	// Failure records a failed call.
	Failure(connectorID string)
	// Stats returns circuit breaker stats for all connectors.
	Stats() map[string]map[string]interface{}
}

// CircuitBreakerState represents the state of a circuit breaker.
type CircuitBreakerState int

const (
	StateClosed   CircuitBreakerState = iota // normal operation
	StateOpen                                // provider is considered unavailable
	StateHalfOpen                            // testing if provider recovered
)

func (s CircuitBreakerState) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half_open"
	default:
		return "unknown"
	}
}

// CircuitBreaker protects a connector from repeated failures.
// After failureThreshold consecutive failures, it trips to Open state.
// After resetTimeout, it transitions to HalfOpen and allows one probe.
type CircuitBreaker struct {
	mu               sync.RWMutex
	connectorID      string
	state            CircuitBreakerState
	failureCount     int
	failureThreshold int
	resetTimeout     time.Duration
	lastFailureTime  time.Time
	probeAllowed     bool

	totalCalls      atomic.Int64
	totalSuccesses  atomic.Int64
	totalFailures   atomic.Int64
	totalRejections atomic.Int64

	onStateChange func(connectorID string, old, new CircuitBreakerState)
}

// CircuitBreakerOption configures a CircuitBreaker.
type CircuitBreakerOption func(*CircuitBreaker)

func WithFailureThreshold(n int) CircuitBreakerOption {
	return func(cb *CircuitBreaker) { cb.failureThreshold = n }
}

func WithResetTimeout(d time.Duration) CircuitBreakerOption {
	return func(cb *CircuitBreaker) { cb.resetTimeout = d }
}

func WithOnStateChange(fn func(connectorID string, old, new CircuitBreakerState)) CircuitBreakerOption {
	return func(cb *CircuitBreaker) { cb.onStateChange = fn }
}

// NewCircuitBreaker creates a circuit breaker with sane defaults.
func NewCircuitBreaker(connectorID string, opts ...CircuitBreakerOption) *CircuitBreaker {
	cb := &CircuitBreaker{
		connectorID:      connectorID,
		state:            StateClosed,
		failureThreshold: 5,
		resetTimeout:     30 * time.Second,
		onStateChange:    func(string, CircuitBreakerState, CircuitBreakerState) {},
	}
	for _, opt := range opts {
		opt(cb)
	}
	return cb
}

// State returns the current circuit breaker state.
func (cb *CircuitBreaker) State() CircuitBreakerState {
	cb.mu.RLock()
	if cb.state == StateOpen && time.Since(cb.lastFailureTime) > cb.resetTimeout {
		cb.mu.RUnlock()
		cb.mu.Lock()
		if cb.state == StateOpen && time.Since(cb.lastFailureTime) > cb.resetTimeout {
			oldState := cb.state
			cb.state = StateHalfOpen
			cb.probeAllowed = true
			cb.onStateChange(cb.connectorID, oldState, StateHalfOpen)
		}
		cb.mu.Unlock()
		cb.mu.RLock()
	}
	defer cb.mu.RUnlock()
	return cb.state
}

// Allow returns true if the call should be allowed through.
func (cb *CircuitBreaker) Allow() bool {
	state := cb.State()
	switch state {
	case StateClosed:
		return true
	case StateHalfOpen:
		cb.mu.Lock()
		allowed := cb.probeAllowed
		if allowed {
			cb.probeAllowed = false
		}
		cb.mu.Unlock()
		return allowed
	case StateOpen:
		cb.totalRejections.Add(1)
		return false
	default:
		return false
	}
}

// Success records a successful call.
func (cb *CircuitBreaker) Success() {
	cb.totalCalls.Add(1)
	cb.totalSuccesses.Add(1)

	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.failureCount = 0
	if cb.state == StateHalfOpen {
		oldState := cb.state
		cb.state = StateClosed
		cb.onStateChange(cb.connectorID, oldState, StateClosed)
	}
}

// Failure records a failed call.
func (cb *CircuitBreaker) Failure() {
	cb.totalCalls.Add(1)
	cb.totalFailures.Add(1)

	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.failureCount++
	cb.lastFailureTime = time.Now()

	if cb.failureCount >= cb.failureThreshold && cb.state == StateClosed {
		oldState := cb.state
		cb.state = StateOpen
		cb.onStateChange(cb.connectorID, oldState, StateOpen)
	}
}

// Stats returns current circuit breaker statistics.
func (cb *CircuitBreaker) Stats() map[string]interface{} {
	cb.mu.RLock()
	state := cb.state
	lastFail := cb.lastFailureTime
	cb.mu.RUnlock()

	return map[string]interface{}{
		"state":              state.String(),
		"failure_threshold":  cb.failureThreshold,
		"reset_timeout_ms":   cb.resetTimeout.Milliseconds(),
		"last_failure_at":    nullTimeCB(lastFail),
		"total_calls":        cb.totalCalls.Load(),
		"total_successes":    cb.totalSuccesses.Load(),
		"total_failures":     cb.totalFailures.Load(),
		"total_rejections":   cb.totalRejections.Load(),
	}
}

func nullTimeCB(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

// ============================================================
// InMemoryCircuitBreakerStore — shared circuit breaker registry
// ============================================================

// InMemoryCircuitBreakerStore implements CircuitBreakerStore.
// Breakers are created lazily on first access with default options.
// Use NewCircuitBreakerStore to create one.
type InMemoryCircuitBreakerStore struct {
	mu       sync.RWMutex
	breakers map[string]*CircuitBreaker

	defaultOpts []CircuitBreakerOption
	globalOpts  []CircuitBreakerOption
}

// NewCircuitBreakerStore creates a store. Global opts apply to all breakers.
func NewCircuitBreakerStore(opts ...CircuitBreakerOption) *InMemoryCircuitBreakerStore {
	return &InMemoryCircuitBreakerStore{
		breakers:    make(map[string]*CircuitBreaker),
		globalOpts:  opts,
	}
}

// getOrCreate returns the breaker for connectorID, creating it lazily.
func (s *InMemoryCircuitBreakerStore) getOrCreate(connectorID string) *CircuitBreaker {
	s.mu.RLock()
	cb, ok := s.breakers[connectorID]
	s.mu.RUnlock()
	if ok {
		return cb
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Double-check after acquiring write lock
	if cb, ok := s.breakers[connectorID]; ok {
		return cb
	}

	cb = NewCircuitBreaker(connectorID, s.globalOpts...)
	s.breakers[connectorID] = cb
	return cb
}

func (s *InMemoryCircuitBreakerStore) Allow(connectorID string) bool {
	return s.getOrCreate(connectorID).Allow()
}

func (s *InMemoryCircuitBreakerStore) Success(connectorID string) {
	s.getOrCreate(connectorID).Success()
}

func (s *InMemoryCircuitBreakerStore) Failure(connectorID string) {
	s.getOrCreate(connectorID).Failure()
}

func (s *InMemoryCircuitBreakerStore) Stats() map[string]map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()
	stats := make(map[string]map[string]interface{}, len(s.breakers))
	for id, cb := range s.breakers {
		stats[strings.ReplaceAll(id, ".", "_")] = cb.Stats()
	}
	return stats
}

// ============================================================
// noopCircuitBreakerStore — safe default, no-op implementation
// ============================================================

type noopCircuitBreakerStore struct{}

func (noopCircuitBreakerStore) Allow(connectorID string) bool        { return true }
func (noopCircuitBreakerStore) Success(connectorID string)           {}
func (noopCircuitBreakerStore) Failure(connectorID string)           {}
func (noopCircuitBreakerStore) Stats() map[string]map[string]interface{} { return nil }
