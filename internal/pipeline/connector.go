package pipeline

import (
	"context"

	"github.com/raghna/fury-sms-gateway/internal/domain"
)

// ConnectorRegistry resolves a connector ID to a domain.Sender.
// The implementation handles caching, lifecycle, and initialization.
// The pipeline never knows how the registry works internally.
type ConnectorRegistry interface {
	Resolve(ctx context.Context, connectorID string) (domain.Sender, error)
}
