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
//  4. Create (or reuse) Selector for that strategy's RouteGroup
//  5. Apply selection strategy → RoutingDecision
//
// Selectors are cached by RouteGroupID (prefix:priority:strategy), NOT by strategy
// alone. This ensures RoundRobin counters for prefix "1" and prefix "44" are
// independent even if both use the same strategy.
type Engine struct {
	routes    []domain.Route
	registry  ConnectorResolver
	factory   SelectorFactory
	selectors map[string]Selector
	mu        sync.Mutex
}

// NewEngine creates a routing Engine.
//
// routes: all enabled routes (matching/filtering happens internally)
// registry: optional connector resolver for health checking (may be nil)
// factory: optional selector factory (nil = use DefaultSelectorRegistry)
func NewEngine(routes []domain.Route, registry ConnectorResolver, factory SelectorFactory) *Engine {
	if factory == nil {
		factory = DefaultSelectorRegistry
	}
	return &Engine{
		routes:    routes,
		registry:  registry,
		factory:   factory,
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
	primary := healthy[0]

	// Get or create selector for this route's strategy group.
	selector, err := e.getOrCreateSelector(primary)
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

// RouteGroupID uniquely identifies a group of routes that share a selector.
// Routes with the same prefix, priority, and strategy form a group — they share
// one stateful selector (e.g., one RoundRobin counter per group).
func RouteGroupID(r domain.Route) string {
	return fmt.Sprintf("%s:%d:%s", r.Prefix, r.Priority, r.Strategy)
}

// getOrCreateSelector returns a cached selector for the route's group,
// creating one if needed. Keyed by RouteGroupID so different route groups
// (e.g., Egypt round_robin vs Saudi round_robin) have independent state.
func (e *Engine) getOrCreateSelector(route domain.Route) (Selector, error) {
	key := RouteGroupID(route)

	e.mu.Lock()
	defer e.mu.Unlock()

	if s, ok := e.selectors[key]; ok {
		return s, nil
	}

	s, err := e.factory.Create(route.Strategy)
	if err != nil {
		return nil, err
	}

	e.selectors[key] = s
	return s, nil
}

// filterHealthy returns routes whose connectors are healthy.
//
// TODO(v0.3): Replace per-Route() health checks with Background Health Monitor.
//   Current approach calls CheckHealth() for every Route() call, which becomes
//   prohibitively expensive at high throughput (20k+ msg/sec).
//
//   Future design:
//
//	type HealthMonitor interface {
//	    Start(ctx context.Context)
//	    IsHealthy(connectorID string) bool
//	    Subscribe(connectorID string, callback func(healthy bool))
//	}
//
//   Background Health Monitor updates atomic health states periodically.
//   Engine reads IsHealthy() from the monitor — zero network calls during routing.
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
