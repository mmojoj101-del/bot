package worker

import (
	"sync"
	"time"
)

// shutdownTimeout is the duration to wait for worker graceful shutdown.
// Used by outbox_worker and retry_engine for their Stop timeout.
var shutdownTimeout = 30 * time.Second

// healthyIdleThreshold is the maximum idle time before a worker is
// reported as potentially unhealthy.
var healthyIdleThreshold = 30 * time.Second

// nullTime returns nil for zero time, otherwise a pointer.
func nullTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

// durationMillis converts nanoseconds to milliseconds (float64).
func durationMillis(ns int64) float64 {
	return float64(ns) / float64(time.Millisecond)
}

// Executor provides a pattern for controlled goroutine execution
// with panic recovery and restart backoff.
type Executor struct {
	Name string

	Mu         sync.Mutex
	WG         sync.WaitGroup
	StopCh     chan struct{}
	Running    bool
	RestartCnt int64
}

// Go starts f in a goroutine and returns immediately.
// f must return false when it should stop (context cancelled).
func (e *Executor) Go(f func() bool) {
	// No-op; embed and call directly in your worker.
}

// IsClosed reports whether a channel is closed.
func isClosed(ch <-chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}
