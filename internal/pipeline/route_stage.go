package pipeline

import (
	"context"
	"fmt"

	"github.com/raghna/fury-sms-gateway/internal/domain"
)

// Router decides which connector should handle a message.
// The Pipeline only calls Route() — it never knows how routing works
// (static, round-robin, failover, weighted, or future strategies).
type Router interface {
	Route(ctx context.Context, msg *domain.Message) (*RoutingDecision, error)
}

// RouteStage asks the Router for a routing decision and stores it
// in PipelineState.Decision. It does NOT modify the domain.Message.
type RouteStage struct {
	router Router
}

// NewRouteStage creates a new RouteStage with the given Router implementation.
func NewRouteStage(router Router) *RouteStage {
	return &RouteStage{router: router}
}

// Name returns the stage name for logging and metrics.
func (s *RouteStage) Name() string {
	return "route"
}

// Process asks the Router for a decision and stores it in PipelineState.
// The domain.Message is NOT modified — RouteID/ConnectorID are not set here.
func (s *RouteStage) Process(ctx context.Context, state *PipelineState) (*PipelineState, error) {
	decision, err := s.router.Route(ctx, state.Message)
	if err != nil {
		return nil, fmt.Errorf("routing failed: %w", err)
	}
	if decision == nil {
		return nil, fmt.Errorf("routing returned nil decision")
	}

	// Store the immutable RoutingDecision — never modified after this point.
	state.Decision = decision

	return state, nil
}
