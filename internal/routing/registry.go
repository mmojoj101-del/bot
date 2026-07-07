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
