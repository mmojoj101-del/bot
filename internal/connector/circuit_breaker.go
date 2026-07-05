package connector

import (
	"sync"
	"sync/atomic"
	"time"
)

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

	onStateChange func(old, new CircuitBreakerState)
}

// CircuitBreakerOption configures a CircuitBreaker.
type CircuitBreakerOption func(*CircuitBreaker)

func WithFailureThreshold(n int) CircuitBreakerOption {
	return func(cb *CircuitBreaker) { cb.failureThreshold = n }
}

func WithResetTimeout(d time.Duration) CircuitBreakerOption {
	return func(cb *CircuitBreaker) { cb.resetTimeout = d }
}

func WithOnStateChange(fn func(old, new CircuitBreakerState)) CircuitBreakerOption {
	return func(cb *CircuitBreaker) { cb.onStateChange = fn }
}

// NewCircuitBreaker creates a circuit breaker with sane defaults.
func NewCircuitBreaker(opts ...CircuitBreakerOption) *CircuitBreaker {
	cb := &CircuitBreaker{
		state:            StateClosed,
		failureThreshold: 5,
		resetTimeout:     30 * time.Second,
		onStateChange:    func(CircuitBreakerState, CircuitBreakerState) {},
	}
	for _, opt := range opts {
		opt(cb)
	}
	return cb
}

// State returns the current circuit breaker state.
func (cb *CircuitBreaker) State() CircuitBreakerState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	// If open, check if reset timeout has elapsed → transition to half-open
	if cb.state == StateOpen && time.Since(cb.lastFailureTime) > cb.resetTimeout {
		cb.mu.RUnlock()
		cb.mu.Lock()
		if cb.state == StateOpen && time.Since(cb.lastFailureTime) > cb.resetTimeout {
			oldState := cb.state
			cb.state = StateHalfOpen
			cb.probeAllowed = true
			cb.onStateChange(oldState, StateHalfOpen)
		}
		cb.mu.Unlock()
		cb.mu.RLock()
	}
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
		cb.onStateChange(oldState, StateClosed)
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
		cb.onStateChange(oldState, StateOpen)
	}
}

// Stats returns current circuit breaker statistics.
func (cb *CircuitBreaker) Stats() map[string]interface{} {
	cb.mu.RLock()
	state := cb.state
	lastFail := cb.lastFailureTime
	cb.mu.RUnlock()

	return map[string]interface{}{
		"state":            state.String(),
		"failure_threshold": cb.failureThreshold,
		"reset_timeout_ms": cb.resetTimeout.Milliseconds(),
		"last_failure_at":  nullTimePtr(lastFail),
		"total_calls":      cb.totalCalls.Load(),
		"total_successes":  cb.totalSuccesses.Load(),
		"total_failures":   cb.totalFailures.Load(),
		"total_rejections": cb.totalRejections.Load(),
	}
}

func nullTimePtr(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}
