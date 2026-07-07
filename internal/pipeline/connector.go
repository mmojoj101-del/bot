package pipeline

import (
	"github.com/raghna/fury-sms-gateway/internal/connector"
)

// ConnectorRegistry is an alias for connector.ConnectorRegistry.
// The pipeline never knows how the registry works internally.
// It only calls Get(connectorID) to retrieve a ready-to-use Connector.
type ConnectorRegistry = connector.ConnectorRegistry

// Connector is an alias for connector.Connector.
// This avoids the pipeline package needing to reference the connector
// package for type assertions while keeping the interface definition
// in the connector package (where it belongs architecturally).
type Connector = connector.Connector
