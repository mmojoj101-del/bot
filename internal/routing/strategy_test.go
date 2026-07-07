package routing

import (
	"testing"

	"github.com/raghna/fury-sms-gateway/internal/domain"
)

// mockRandom returns deterministic values for weighted selection.
type mockRandom struct {
	values []int
	index  int
}

func (r *mockRandom) Intn(n int) int {
	if r.index >= len(r.values) {
		return 0
	}
	v := r.values[r.index] % n
	r.index++
	return v
}

func TestStaticSelector_Select(t *testing.T) {
	s := NewStaticSelector()
	routes := []domain.Route{
		route("first", "1", "conn-1", 10, true),
		route("second", "1", "conn-2", 5, true),
	}

	selected, err := s.Select(routes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if selected.ID != "first" {
		t.Errorf("expected first route, got %s", selected.ID)
	}
}

func TestStaticSelector_Select_Empty(t *testing.T) {
	s := NewStaticSelector()
	_, err := s.Select(nil)
	if err == nil {
		t.Fatal("expected error for empty routes")
	}
}

func TestStaticSelector_Name(t *testing.T) {
	s := NewStaticSelector()
	if s.Name() != "static" {
		t.Errorf("expected 'static', got %q", s.Name())
	}
}

func TestRoundRobinSelector_Select(t *testing.T) {
	s := NewRoundRobinSelector()
	routes := []domain.Route{
		route("a", "1", "conn-1", 10, true),
		route("b", "1", "conn-2", 10, true),
	}

	first, _ := s.Select(routes)
	second, _ := s.Select(routes)
	third, _ := s.Select(routes)

	if first.ID == second.ID && second.ID == third.ID {
		t.Errorf("expected round-robin distribution, got %s three times", first.ID)
	}
	if first.ID != third.ID {
		t.Errorf("expected wrap-around (first == third), got first=%s third=%s", first.ID, third.ID)
	}
}

func TestRoundRobinSelector_Select_Empty(t *testing.T) {
	s := NewRoundRobinSelector()
	_, err := s.Select(nil)
	if err == nil {
		t.Fatal("expected error for empty routes")
	}
}

func TestRoundRobinSelector_Name(t *testing.T) {
	s := NewRoundRobinSelector()
	if s.Name() != "round_robin" {
		t.Errorf("expected 'round_robin', got %q", s.Name())
	}
}

func TestFailoverSelector_Select(t *testing.T) {
	s := NewFailoverSelector()
	routes := []domain.Route{
		route("primary", "1", "conn-1", 100, true),
		route("backup", "1", "conn-2", 50, true),
	}

	selected, err := s.Select(routes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if selected.ID != "primary" {
		t.Errorf("expected primary route, got %s", selected.ID)
	}
}

func TestFailoverSelector_Select_Empty(t *testing.T) {
	s := NewFailoverSelector()
	_, err := s.Select(nil)
	if err == nil {
		t.Fatal("expected error for empty routes")
	}
}

func TestFailoverSelector_Name(t *testing.T) {
	s := NewFailoverSelector()
	if s.Name() != "failover" {
		t.Errorf("expected 'failover', got %q", s.Name())
	}
}

func TestWeightedSelector_Deterministic(t *testing.T) {
	// Use mockRandom with fixed sequence for deterministic tests.
	mock := &mockRandom{values: []int{0, 50, 99, 0}}
	s := NewWeightedSelector(mock)

	routes := []domain.Route{
		{BaseModel: domain.BaseModel{ID: "heavy"}, Prefix: "1", ConnectorID: "conn-1", Weight: 90, Enabled: true, Type: domain.RouteTypeSMS},
		{BaseModel: domain.BaseModel{ID: "light"}, Prefix: "1", ConnectorID: "conn-2", Weight: 10, Enabled: true, Type: domain.RouteTypeSMS},
	}

	// First pick: random=0 → heavy (0 < 90)
	selected, err := s.Select(routes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if selected.ID != "heavy" {
		t.Errorf("expected heavy (random=0 < 90), got %s", selected.ID)
	}

	// Second pick: random=50 → heavy (50 < 90)
	selected, _ = s.Select(routes)
	if selected.ID != "heavy" {
		t.Errorf("expected heavy (random=50 < 90), got %s", selected.ID)
	}

	// Third pick: random=99 → light (99 >= 90, 99-90=9 < 10)
	selected, _ = s.Select(routes)
	if selected.ID != "light" {
		t.Errorf("expected light (random=99 >= 90), got %s", selected.ID)
	}
}

func TestWeightedSelector_Select_Empty(t *testing.T) {
	s := NewWeightedSelector(nil)
	_, err := s.Select(nil)
	if err == nil {
		t.Fatal("expected error for empty routes")
	}
}

func TestWeightedSelector_ZeroWeights(t *testing.T) {
	s := NewWeightedSelector(nil)
	routes := []domain.Route{
		route("first", "1", "conn-1", 10, true),
		route("second", "1", "conn-2", 10, true),
	}

	selected, err := s.Select(routes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if selected.ID != "first" {
		t.Errorf("expected first route (zero-weight fallback), got %s", selected.ID)
	}
}

func TestWeightedSelector_Name(t *testing.T) {
	s := NewWeightedSelector(nil)
	if s.Name() != "weighted" {
		t.Errorf("expected 'weighted', got %q", s.Name())
	}
}

func TestSelectorForStrategy(t *testing.T) {
	tests := []struct {
		strategy domain.RouteStrategy
		expected string
	}{
		{domain.RouteStrategyStatic, "static"},
		{domain.RouteStrategyRoundRobin, "round_robin"},
		{domain.RouteStrategyFailover, "failover"},
		{domain.RouteStrategyWeighted, "weighted"},
	}

	for _, tt := range tests {
		t.Run(string(tt.strategy), func(t *testing.T) {
			s, err := SelectorForStrategy(tt.strategy)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if s.Name() != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, s.Name())
			}
		})
	}
}

func TestSelectorForStrategy_Unknown(t *testing.T) {
	_, err := SelectorForStrategy("unknown")
	if err == nil {
		t.Fatal("expected error for unknown strategy")
	}
}

func TestDefaultSelectorRegistry(t *testing.T) {
	tests := []struct {
		strategy domain.RouteStrategy
		expected string
	}{
		{domain.RouteStrategyStatic, "static"},
		{domain.RouteStrategyRoundRobin, "round_robin"},
		{domain.RouteStrategyFailover, "failover"},
		{domain.RouteStrategyWeighted, "weighted"},
	}

	for _, tt := range tests {
		t.Run(string(tt.strategy), func(t *testing.T) {
			s, err := DefaultSelectorRegistry.Create(tt.strategy)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if s.Name() != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, s.Name())
			}
		})
	}

	// Unknown strategy should error
	_, err := DefaultSelectorRegistry.Create("unknown")
	if err == nil {
		t.Fatal("expected error for unknown strategy in DefaultSelectorRegistry")
	}
}

func TestSelectorFactoryFunc(t *testing.T) {
	// SelectorFactoryFunc adapts a function to SelectorFactory.
	var factory SelectorFactory = SelectorFactoryFunc(func(s domain.RouteStrategy) (Selector, error) {
		return NewStaticSelector(), nil
	})

	s, err := factory.Create(domain.RouteStrategyFailover) // any strategy returns static
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Name() != "static" {
		t.Errorf("expected static, got %q", s.Name())
	}
}
