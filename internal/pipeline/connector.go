package pipeline

import "context"

// Connector sends a message through a specific provider.
// It receives SendRequest (not PipelineState, not domain.Message).
// Implementations wrap domain.Sender, protocol adapters, or mock connectors.
type Connector interface {
	Send(ctx context.Context, req *SendRequest) (*SendResult, error)
}

// ConnectorRegistry resolves a connector ID to a Connector instance.
// The implementation may return a cached instance or create one on demand.
// Returns an error if the connector is not found or not ready.
type ConnectorRegistry interface {
	Get(ctx context.Context, connectorID string) (Connector, error)
}
