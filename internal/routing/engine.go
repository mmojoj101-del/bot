package routing

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/raghna/fury-sms-gateway/internal/domain"
)

// Engine is the main routing engine implementing Router.
//
// Lifecycle:
//  1. Match routes by message destination (longest prefix)
//  2. Filter unhealthy connectors via HealthStatusProvider
//  3. Determine strategy from the best matching route
//  4. Create (or reuse) Selector for the route group
//  5. Apply selection strategy → RoutingDecision
//
// Selectors are cached by a deterministic hash of the matched route group
// (sorted route IDs + prefix + priority + strategy). This ensures stateful
// selectors (RoundRobin, Weighted) are independent per group.
// See: groupKey() for details.
type Engine struct {
	routes    []domain.Route
	health    HealthStatusProvider
	factory   SelectorFactory
	selectors map[string]Selector
	mu        sync.Mutex
}

// NewEngine creates a routing Engine.
//
// routes: all enabled routes for this tenant
// health:  optional health status provider (nil = skip health filtering)
// factory: optional selector factory (nil = use DefaultSelectorRegistry)
func NewEngine(routes []domain.Route, health HealthStatusProvider, factory SelectorFactory) *Engine {
	if factory == nil {
		factory = DefaultSelectorRegistry
	}
	return &Engine{
		routes:    routes,
		health:    health,
		factory:   factory,
		selectors: make(map[string]Selector),
	}
}

// Route selects a connector for the given message.
func (e *Engine) Route(ctx context.Context, msg *domain.Message) (*domain.RoutingDecision, error) {
	matcher := &RouteMatcher{}
	candidates := matcher.Match(msg, e.routes)
	if len(candidates) == 0 {
		return nil, fmt.Errorf("routing: no matching route for destination %q", msg.Destination)
	}

	// Filter unhealthy connectors
	healthy := e.filterHealthy(candidates)
	if len(healthy) == 0 {
		return nil, fmt.Errorf("routing: all matching routes have unhealthy connectors")
	}

	// Determine strategy from the best-matching route (first in sorted order).
	primary := healthy[0]

	// Get or create selector for the route group (not just one route).
	// The group key is derived from ALL routes in the healthy set.
	selector, err := e.getOrCreateSelector(healthy, primary.Strategy)
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

// groupKey produces a deterministic key for a set of routes forming a group.
//
// The key is derived from ALL routes in the set (sorted by ID), not from a
// single route. This ensures:
//   - Two different route groups never collide, even if they share prefix+priority+strategy
//   - Adding/removing a route from the group produces a new key (new selector)
//   - The same set of routes always produces the same key (selector state preserved)
//
// Format: "id:prefix:priority:strategy|id:prefix:priority:strategy|..."
func groupKey(routes []domain.Route) string {
	sorted := make([]domain.Route, len(routes))
	copy(sorted, routes)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].ID < sorted[j].ID
	})

	var b strings.Builder
	for _, r := range sorted {
		// id:prefix:priority:strategy|
		b.WriteString(r.ID)
		b.WriteByte(':')
		b.WriteString(r.Prefix)
		b.WriteByte(':')
		b.WriteString(fmt.Sprintf("%d", r.Priority))
		b.WriteByte(':')
		b.WriteString(string(r.Strategy))
		b.WriteByte('|')
	}
	return b.String()
}

// getOrCreateSelector returns a cached selector for the route group.
// The cache key is derived from ALL routes in the group (sorted by ID),
// ensuring independent state for each unique group.
func (e *Engine) getOrCreateSelector(routes []domain.Route, strategy domain.RouteStrategy) (Selector, error) {
	key := groupKey(routes)

	e.mu.Lock()
	defer e.mu.Unlock()

	if s, ok := e.selectors[key]; ok {
		return s, nil
	}

	s, err := e.factory.Create(strategy)
	if err != nil {
		return nil, err
	}

	e.selectors[key] = s
	return s, nil
}

// filterHealthy returns routes whose connectors are healthy.
// The Engine only calls HealthStatusProvider.IsHealthy() — it has no
// knowledge of CheckHealth(), BackgroundHealthMonitor, or any other
// health checking mechanism.
func (e *Engine) filterHealthy(routes []domain.Route) []domain.Route {
	if e.health == nil {
		return routes // no health checking — assume all healthy
	}

	var healthy []domain.Route
	for _, r := range routes {
		if e.health.IsHealthy(r.ConnectorID) {
			healthy = append(healthy, r)
		}
	}
	return healthy
}
