package domain

import (
	"math"
	"time"
)

// ExponentialBackoff implements RetryPolicy with exponential backoff.
// It is safe for concurrent use (no mutable state).
//
// Delay formula: min(initial * multiplier^attempt, maxDelay)
// Defaults: initial=1s, multiplier=2, maxDelay=300s
type ExponentialBackoff struct {
	InitialDelay time.Duration
	Multiplier   float64
	MaxDelay     time.Duration
}

// NewExponentialBackoff creates an ExponentialBackoff with sensible defaults.
func NewExponentialBackoff() *ExponentialBackoff {
	return &ExponentialBackoff{
		InitialDelay: 1 * time.Second,
		Multiplier:   2.0,
		MaxDelay:     300 * time.Second,
	}
}

// NextDelay returns the backoff duration for the given retry context.
// Formula: min(initial * multiplier^attempt, maxDelay)
func (e *ExponentialBackoff) NextDelay(ctx RetryContext) time.Duration {
	attempt := ctx.Attempt
	if attempt < 0 {
		attempt = 0
	}

	delay := float64(e.InitialDelay) * math.Pow(e.Multiplier, float64(attempt))
	if delay > float64(e.MaxDelay) {
		delay = float64(e.MaxDelay)
	}
	return time.Duration(delay)
}

// NoRetry is a RetryPolicy that never retries — always returns 0.
type NoRetry struct{}

// NextDelay returns 0.
func (NoRetry) NextDelay(_ RetryContext) time.Duration { return 0 }

// compile-time interface checks
var _ RetryPolicy = (*ExponentialBackoff)(nil)
var _ RetryPolicy = NoRetry{}
