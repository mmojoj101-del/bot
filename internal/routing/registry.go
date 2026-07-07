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

// ConnectorHealthChecker is how the routing engine accesses connectors
// for health checking. It mirrors connector.ConnectorRegistry's Get method
// but the routing engine only uses it for health queries.
type ConnectorHealthChecker interface {
	Get(id string) (connector.Connector, error)
	List() []connector.Connector
}
