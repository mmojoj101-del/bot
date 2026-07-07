package routing

import (
	"context"

	"github.com/raghna/fury-sms-gateway/internal/domain"
)

// Router decides which connector should handle a message.
// The pipeline only calls Route() — it never knows how routing works.
//
// Implementations:
//   - Engine (static, round_robin, failover, weighted)
//   - mockRouter (for pipeline tests)
type Router interface {
	// Route returns a RoutingDecision for the given message.
	Route(ctx context.Context, msg *domain.Message) (*domain.RoutingDecision, error)
}
