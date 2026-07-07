package routing

import (
	"context"
	"fmt"
	"sync"

	"github.com/raghna/fury-sms-gateway/internal/domain"
)

// Engine is the main routing engine implementing Router.
// It combines route matching, health filtering, and strategy-based selection.
//
// Lifecycle:
//  1. Match routes by message destination (longest prefix)
//  2. Filter unhealthy connectors (if HealthChecker available)
//  3. Determine strategy from the best matching route
//  4. Create (or reuse) Selector for that strategy
//  5. Apply selection strategy → RoutingDecision
//
// Selectors are cached per strategy so stateful selectors
// (RoundRobin, Weighted) persist across Route() calls.
type Engine struct {
	routes    []domain.Route
	registry  ConnectorResolver
	selectors map[string]Selector
	mu        sync.Mutex
}

// NewEngine creates a routing Engine.
//
// routes: all enabled routes (matching/filtering happens internally)
// registry: optional connector resolver for health checking (may be nil)
func NewEngine(routes []domain.Route, registry ConnectorResolver) *Engine {
	return &Engine{
		routes:    routes,
		registry:  registry,
		selectors: make(map[string]Selector),
	}
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

	// Determine strategy from the best-matching route (first in sorted order).
	primaryStrategy := healthy[0].Strategy

	// Get or create selector for this strategy.
	selector, err := e.getOrCreateSelector(primaryStrategy)
	if err != nil {
		return nil, fmt.Errorf("routing: %w", err)
	}

	// Select one route using the strategy.
	selected, err := selector.Select(healthy)
	if err != nil {
		return nil, fmt.Errorf("routing: selection failed: %w", err)
	}

	return &domain.RoutingDecision{
		RouteID:      selected.ID,
		ConnectorID:  selected.ConnectorID,
		StrategyUsed: selector.Name(),
		Priority:     selected.Priority,
		Reason:       fmt.Sprintf("prefix=%q priority=%d strategy=%s", selected.Prefix, selected.Priority, selector.Name()),
	}, nil
}

// getOrCreateSelector returns a cached selector for the strategy, creating one if needed.
// Stateful selectors (RoundRobin, Weighted) persist across Route() calls this way.
func (e *Engine) getOrCreateSelector(strategy domain.RouteStrategy) (Selector, error) {
	key := string(strategy)

	e.mu.Lock()
	defer e.mu.Unlock()

	if s, ok := e.selectors[key]; ok {
		return s, nil
	}

	s, err := SelectorForStrategy(strategy)
	if err != nil {
		return nil, err
	}

	e.selectors[key] = s
	return s, nil
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
