package routing

import (
	"context"

	"github.com/raghna/fury-sms-gateway/internal/connector"
)

// ConnectorHealthCheck is the minimal interface the routing engine needs
// from a connector — just health checking.
// Used via type assertion in filterHealthy when HealthStatusProvider is not available.
type ConnectorHealthCheck interface {
	CheckHealth(ctx context.Context) error
}

// ConnectorResolver is how the routing engine looks up connectors
// for health checking. It follows Interface Segregation: only Get()
// is needed — List() lives only in connector.ConnectorRegistry.
type ConnectorResolver interface {
	Get(id string) (connector.Connector, error)
}

// HealthStatusProvider is the single interface the Engine uses to query
// connector health. The Engine never calls CheckHealth() directly — it
// only reads cached health status through IsHealthy().
//
// Implementations:
//   - Per-RequestChecker wraps CheckHealth() calls for initial simplicity
//   - BackgroundHealthMonitor maintains atomic cached states updated by
//     periodic checks in a background goroutine (zero I/O during routing)
//   - mockHealthProvider for tests
type HealthStatusProvider interface {
	// IsHealthy returns the current cached health state for a connector.
	IsHealthy(connectorID string) bool
}

// PerRequestChecker is a HealthStatusProvider that calls CheckHealth()
// synchronously on every Route(). Acceptable for low throughput; replace
// with BackgroundHealthMonitor for production at scale.
type PerRequestChecker struct {
	resolver ConnectorResolver
}

func NewPerRequestChecker(resolver ConnectorResolver) *PerRequestChecker {
	return &PerRequestChecker{resolver: resolver}
}

func (c *PerRequestChecker) IsHealthy(connectorID string) bool {
	conn, err := c.resolver.Get(connectorID)
	if err != nil {
		return false
	}
	hc, ok := conn.(ConnectorHealthCheck)
	if !ok {
		return true // no HealthChecker = assume healthy
	}
	// Note: using context.Background() because we're in the per-request path.
	// BackgroundHealthMonitor cancels properly.
	return hc.CheckHealth(context.Background()) == nil
}

// BackgroundHealthMonitor periodically checks connector health and caches
// results atomically. The routing engine reads IsHealthy() instead of calling
// CheckHealth() per Route(), eliminating network calls during routing.
//
// TODO(v0.3): Implement.
//
//	type BackgroundHealthMonitor struct {
//	    status   sync.Map // connectorID → atomic bool
//	    interval time.Duration
//	    resolver ConnectorResolver
//	}
//
//	func (m *BackgroundHealthMonitor) Start(ctx context.Context) // goroutine with ticker
//	func (m *BackgroundHealthMonitor) Stop()                     // stop ticker
//	func (m *BackgroundHealthMonitor) IsHealthy(id string) bool  // atomic load
//	func (m *BackgroundHealthMonitor) Subscribe(id string, fn func(healthy bool))
type BackgroundHealthMonitor interface {
	HealthStatusProvider
	Start(ctx context.Context)
	Stop()
	Subscribe(connectorID string, callback func(healthy bool))
}
