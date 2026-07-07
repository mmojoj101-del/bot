package routing

import (
	"context"
	"fmt"

	"github.com/raghna/fury-sms-gateway/internal/domain"
)

// Engine is the main routing engine implementing Router.
// It combines route matching, health filtering, and strategy-based selection.
//
// Lifecycle:
//  1. Match routes by message destination (longest prefix)
//  2. Filter unhealthy connectors (if HealthChecker available)
//  3. Apply selection strategy (static, round_robin, failover, weighted)
//  4. Return RoutingDecision
type Engine struct {
	routes   []domain.Route
	registry ConnectorHealthChecker
	selector Selector
}

// NewEngine creates a routing Engine with the given routes, registry, and strategy.
//
// routes: all available routes (matching/filtering happens internally)
// registry: connector registry (used for health checking, not route storage)
// strategy: selection strategy (static, round_robin, failover, weighted)
func NewEngine(routes []domain.Route, registry ConnectorHealthChecker, strategy domain.RouteStrategy) (*Engine, error) {
	selector, err := SelectorForStrategy(strategy)
	if err != nil {
		return nil, err
	}

	return &Engine{
		routes:   routes,
		registry: registry,
		selector: selector,
	}, nil
}

// Route selects a route for the given message.
func (e *Engine) Route(ctx context.Context, msg *domain.Message) (*domain.RoutingDecision, error) {
	matcher := &RouteMatcher{}
	candidates := matcher.Match(msg, e.routes)
	if len(candidates) == 0 {
		return nil, fmt.Errorf("routing: no matching route for destination %q", msg.Destination)
	}

	// Filter unhealthy connectors
	healthy := e.filterHealthy(ctx, candidates)
	if len(healthy) == 0 {
		return nil, fmt.Errorf("routing: all matching routes have unhealthy connectors")
	}

	// Select one route using the strategy
	selected, err := e.selector.Select(healthy)
	if err != nil {
		return nil, fmt.Errorf("routing: selection failed: %w", err)
	}

	return &domain.RoutingDecision{
		RouteID:      selected.ID,
		ConnectorID:  selected.ConnectorID,
		StrategyUsed: e.selector.Name(),
		Priority:     selected.Priority,
		Reason:       fmt.Sprintf("prefix=%q priority=%d", selected.Prefix, selected.Priority),
	}, nil
}

// filterHealthy returns routes whose connectors are healthy.
func (e *Engine) filterHealthy(ctx context.Context, routes []domain.Route) []domain.Route {
	if e.registry == nil {
		return routes // no health checking
	}

	var healthy []domain.Route
	for _, r := range routes {
		conn, err := e.registry.Get(r.ConnectorID)
		if err != nil {
			continue // connector not found — skip route
		}

		if hc, ok := conn.(ConnectorHealthCheck); ok {
			if err := hc.CheckHealth(ctx); err != nil {
				continue // unhealthy — skip
			}
		}

		healthy = append(healthy, r)
	}
	return healthy
}
