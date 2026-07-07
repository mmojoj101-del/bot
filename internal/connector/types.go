package connector

import (
	"context"

	"github.com/raghna/fury-sms-gateway/internal/domain"
)

// --- Core interfaces ---

// Connector is the unified interface every protocol adapter implements.
// Protocol-specific details (HTTP templates, SMPP sessions, SIP dialogs)
// are encapsulated inside the implementation — the pipeline never sees them.
//
// Implementations:
//   - HTTPSender  (existing, in http_sender.go)
//   - SMPPConnector
//   - SIPConnector
//   - MockConnector (for tests)
//
// Optional interfaces that a Connector may also implement:
//   - HealthChecker (CheckHealth)
//   - CapabilityProvider (Capabilities)
type Connector interface {
	// ID returns the unique identifier for this connector instance.
	ID() string

	// Protocol declares what protocol this connector uses.
	Protocol() domain.ConnectorType

	// Send transmits a prepared message through this connector
	// and returns the provider-level result.
	Send(ctx context.Context, req *domain.SendRequest) (*domain.SendResult, error)
}

// HealthChecker is an optional interface that connectors may implement
// to signal their operational status. Use type assertion to detect:
//
//	if hc, ok := connector.(HealthChecker); ok {
//	    err := hc.CheckHealth(ctx)
//	}
type HealthChecker interface {
	CheckHealth(ctx context.Context) error
}

// CapabilityProvider is an optional interface that connectors may implement
// to advertise their capabilities (e.g., DLR support, encoding, segments).
// The Capabilities type will be defined when Phase 2.7 introduces capability
// negotiation (see ADR-0009). For now, use interface{}.
type CapabilityProvider interface {
	Capabilities() interface{}
}

// --- Registry ---

// ConnectorRegistry resolves connector IDs to ready-to-use Connector instances.
// It is a pure lookup interface — no I/O, no context.Context needed.
//
// The registry holds only initialized, ready connectors.
// Connector construction and lifecycle belong to ConnectorFactory (separate concern).
type ConnectorRegistry interface {
	// Get returns the connector with the given ID.
	Get(id string) (Connector, error)

	// List returns all registered connectors.
	List() []Connector
}
