package routing

import (
	"fmt"
	"math/rand"
	"sync/atomic"

	"github.com/raghna/fury-sms-gateway/internal/domain"
)

// Random abstracts random number generation for weighted selection.
// Production uses math/rand; tests use deterministic mock.
type Random interface {
	Intn(n int) int
}

// defaultRandom wraps math/rand for production.
type defaultRandom struct{}

func (r *defaultRandom) Intn(n int) int {
	return rand.Intn(n)
}

// Selector picks one route from a list of candidates.
// Different implementations implement different strategies.
type Selector interface {
	// Select picks a route from the candidates.
	Select(routes []domain.Route) (*domain.Route, error)

	// Name returns the strategy name for the RoutingDecision.
	Name() string
}

// --- StaticSelector ---

// StaticSelector always picks the first route (highest priority).
type StaticSelector struct{}

func NewStaticSelector() *StaticSelector {
	return &StaticSelector{}
}

func (s *StaticSelector) Name() string { return "static" }

func (s *StaticSelector) Select(routes []domain.Route) (*domain.Route, error) {
	if len(routes) == 0 {
		return nil, fmt.Errorf("static selector: no routes available")
	}
	return &routes[0], nil
}

// --- RoundRobinSelector ---

// RoundRobinSelector cycles through routes in order.
type RoundRobinSelector struct {
	counter atomic.Uint64
}

func NewRoundRobinSelector() *RoundRobinSelector {
	return &RoundRobinSelector{}
}

func (s *RoundRobinSelector) Name() string { return "round_robin" }

func (s *RoundRobinSelector) Select(routes []domain.Route) (*domain.Route, error) {
	if len(routes) == 0 {
		return nil, fmt.Errorf("round robin selector: no routes available")
	}
	idx := s.counter.Add(1) % uint64(len(routes))
	return &routes[idx], nil
}

// --- FailoverSelector ---

// FailoverSelector picks routes by priority order.
type FailoverSelector struct{}

func NewFailoverSelector() *FailoverSelector {
	return &FailoverSelector{}
}

func (s *FailoverSelector) Name() string { return "failover" }

func (s *FailoverSelector) Select(routes []domain.Route) (*domain.Route, error) {
	if len(routes) == 0 {
		return nil, fmt.Errorf("failover selector: no routes available")
	}
	return &routes[0], nil
}

// --- WeightedSelector ---

// WeightedSelector selects routes based on their Weight field.
// Higher weight = higher probability.
type WeightedSelector struct {
	random Random
}

func NewWeightedSelector(random Random) *WeightedSelector {
	if random == nil {
		random = &defaultRandom{}
	}
	return &WeightedSelector{random: random}
}

func (s *WeightedSelector) Name() string { return "weighted" }

func (s *WeightedSelector) Select(routes []domain.Route) (*domain.Route, error) {
	if len(routes) == 0 {
		return nil, fmt.Errorf("weighted selector: no routes available")
	}

	totalWeight := 0
	for _, r := range routes {
		if r.Weight <= 0 {
			continue
		}
		totalWeight += r.Weight
	}
	if totalWeight == 0 {
		return &routes[0], nil
	}

	// Pick based on weight distribution.
	pick := s.random.Intn(totalWeight)
	accumulated := 0
	for _, r := range routes {
		if r.Weight <= 0 {
			continue
		}
		accumulated += r.Weight
		if pick < accumulated {
			return &r, nil
		}
	}

	return &routes[0], nil
}

// SelectorForStrategy returns the appropriate Selector for a given strategy.
// WeightedSelector uses the default random source (math/rand).
func SelectorForStrategy(strategy domain.RouteStrategy) (Selector, error) {
	switch strategy {
	case domain.RouteStrategyStatic:
		return NewStaticSelector(), nil
	case domain.RouteStrategyRoundRobin:
		return NewRoundRobinSelector(), nil
	case domain.RouteStrategyFailover:
		return NewFailoverSelector(), nil
	case domain.RouteStrategyWeighted:
		return NewWeightedSelector(nil), nil // nil = default random
	default:
		return nil, fmt.Errorf("unknown route strategy: %q", strategy)
	}
}
