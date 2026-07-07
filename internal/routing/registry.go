package routing

import (
	"context"

	"github.com/raghna/fury-sms-gateway/internal/connector"
)

// ConnectorHealthCheck is the minimal interface the routing engine needs
// from a connector — just health checking.
// It uses a type assertion in filterHealthy to detect connectors that
// implement the connector.HealthChecker interface.
type ConnectorHealthCheck interface {
	CheckHealth(ctx context.Context) error
}

// ConnectorResolver is how the routing engine looks up connectors
// for health checking. It follows Interface Segregation: only Get()
// is needed — List() lives only in connector.ConnectorRegistry.
type ConnectorResolver interface {
	Get(id string) (connector.Connector, error)
}

// BackgroundHealthMonitor periodically checks connector health and caches
// results atomically. The routing engine reads IsHealthy() instead of calling
// CheckHealth() per Route(), eliminating network calls during routing.
//
// TODO(v0.3): Implement and plug into Engine.filterHealthy.
//   - Engine checks monitor.IsHealthy(id) — zero network I/O during route()
//   - Monitor runs in its own goroutine with configurable interval
//   - On health change, monitor publishes connector.healthy/connector.unhealthy events
type BackgroundHealthMonitor interface {
	// Start begins periodic health checks. Should be called during app startup.
	Start(ctx context.Context)

	// Stop terminates the health check loop. Should be called during shutdown.
	Stop()

	// IsHealthy returns the last known health state for a connector.
	IsHealthy(connectorID string) bool

	// Subscribe registers a callback for health state changes.
	Subscribe(connectorID string, callback func(healthy bool))
}
