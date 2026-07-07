package pipeline

import (
	"context"
	"fmt"

	"github.com/raghna/fury-sms-gateway/internal/routing"
)

// RouteStage asks the Router for a routing decision and stores it
// in PipelineState.Decision. It does NOT modify the domain.Message.
//
// The Router implementation determines the strategy (static, round_robin,
// failover, weighted) — RouteStage is agnostic.
type RouteStage struct {
	router routing.Router
}

// NewRouteStage creates a new RouteStage with the given Router implementation.
func NewRouteStage(router routing.Router) *RouteStage {
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
