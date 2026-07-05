package worker

import (
	"sync"
)

// healthDetailer is the interface each worker implements for health.
type healthDetailer interface {
	IsHealthy() error
	HealthDetail() map[string]interface{}
}

// HealthChecker aggregates worker health into a single WorkerHealthChecker.
// Implements handler.WorkerHealthChecker.
type HealthChecker struct {
	mu      sync.RWMutex
	workers []healthDetailer
}

// NewHealthChecker creates a health checker with the given workers.
func NewHealthChecker(workers ...healthDetailer) *HealthChecker {
	return &HealthChecker{
		mu:      sync.RWMutex{},
		workers: workers,
	}
}

// AddWorker registers an additional worker post-construction.
func (hc *HealthChecker) AddWorker(w healthDetailer) {
	hc.mu.Lock()
	defer hc.mu.Unlock()
	hc.workers = append(hc.workers, w)
}

// AllHealthy returns true only if every registered worker is healthy.
func (hc *HealthChecker) AllHealthy() bool {
	hc.mu.RLock()
	defer hc.mu.RUnlock()
	for _, w := range hc.workers {
		if err := w.IsHealthy(); err != nil {
			return false
		}
	}
	return true
}

// Details returns a per-type map of worker health details.
// The map key is the worker type (e.g. "queue_worker", "retry_engine").
func (hc *HealthChecker) Details() map[string]map[string]interface{} {
	hc.mu.RLock()
	defer hc.mu.RUnlock()
	details := make(map[string]map[string]interface{}, len(hc.workers))
	for _, w := range hc.workers {
		d := w.HealthDetail()
		if wType, ok := d["type"].(string); ok {
			details[wType] = d
		}
	}
	return details
}
