package routing

import (
	"context"
	"testing"

	"github.com/raghna/fury-sms-gateway/internal/domain"
)

// mockHealth implements HealthStatusProvider for testing.
type mockHealth struct {
	healthy map[string]bool
}

func newMockHealth() *mockHealth {
	return &mockHealth{healthy: make(map[string]bool)}
}

func (m *mockHealth) Set(id string, healthy bool) {
	m.healthy[id] = healthy
}

func (m *mockHealth) IsHealthy(id string) bool {
	v, ok := m.healthy[id]
	return !ok || v // default: healthy if not set
}

func TestEngine_Route_Static(t *testing.T) {
	routes := []domain.Route{
		route("r1", "1", "conn-http", 10, true),
	}
	health := newMockHealth()
	health.Set("conn-http", true)

	engine := NewEngine(routes, health, nil)
	msg := &domain.Message{Destination: "+1234567890"}

	decision, err := engine.Route(context.Background(), msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision.RouteID != "r1" {
		t.Errorf("RouteID = %q, want r1", decision.RouteID)
	}
	if decision.ConnectorID != "conn-http" {
		t.Errorf("ConnectorID = %q, want conn-http", decision.ConnectorID)
	}
	if decision.StrategyUsed != "static" {
		t.Errorf("StrategyUsed = %q, want static", decision.StrategyUsed)
	}
}

func TestEngine_Route_NoMatch(t *testing.T) {
	routes := []domain.Route{
		route("uk", "44", "conn-uk", 10, true),
	}
	engine := NewEngine(routes, nil, nil)
	msg := &domain.Message{Destination: "+1234567890"}

	_, err := engine.Route(context.Background(), msg)
	if err == nil {
		t.Fatal("expected error for no matching route")
	}
}

func TestEngine_Route_AllUnhealthy(t *testing.T) {
	routes := []domain.Route{
		route("r1", "1", "conn-bad", 10, true),
	}
	health := newMockHealth()
	health.Set("conn-bad", false) // explicitly unhealthy

	engine := NewEngine(routes, health, nil)
	msg := &domain.Message{Destination: "+1234567890"}

	_, err := engine.Route(context.Background(), msg)
	if err == nil {
		t.Fatal("expected error when all connectors unhealthy")
	}
}

func TestEngine_Route_HealthyConnector(t *testing.T) {
	routes := []domain.Route{
		route("r1", "1", "conn-ok", 10, true),
	}
	health := newMockHealth()
	health.Set("conn-ok", true)

	engine := NewEngine(routes, health, nil)
	msg := &domain.Message{Destination: "+1234567890"}

	decision, err := engine.Route(context.Background(), msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision.ConnectorID != "conn-ok" {
		t.Errorf("expected conn-ok, got %s", decision.ConnectorID)
	}
}

func TestEngine_Route_RoundRobin(t *testing.T) {
	routes := []domain.Route{
		{BaseModel: domain.BaseModel{ID: "r1"}, Type: domain.RouteTypeSMS, Prefix: "1", ConnectorID: "conn-a", Priority: 10, Enabled: true, Strategy: domain.RouteStrategyRoundRobin},
		{BaseModel: domain.BaseModel{ID: "r2"}, Type: domain.RouteTypeSMS, Prefix: "1", ConnectorID: "conn-b", Priority: 10, Enabled: true, Strategy: domain.RouteStrategyRoundRobin},
	}
	health := newMockHealth()
	health.Set("conn-a", true)
	health.Set("conn-b", true)

	engine := NewEngine(routes, health, nil)
	msg := &domain.Message{Destination: "+1234567890"}

	d1, _ := engine.Route(context.Background(), msg)
	d2, _ := engine.Route(context.Background(), msg)

	if d1.ConnectorID == d2.ConnectorID {
		t.Errorf("expected different connectors in round-robin, got %s twice", d1.ConnectorID)
	}
}

func TestEngine_Route_PremiumRouteWins(t *testing.T) {
	routes := []domain.Route{
		route("catch-all", "", "conn-default", 1, true),
		route("premium", "123", "conn-premium", 10, true),
	}
	health := newMockHealth()
	health.Set("conn-default", true)
	health.Set("conn-premium", true)

	engine := NewEngine(routes, health, nil)
	msg := &domain.Message{Destination: "+1234567890"}

	decision, err := engine.Route(context.Background(), msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision.ConnectorID != "conn-premium" {
		t.Errorf("expected premium connector, got %s", decision.ConnectorID)
	}
}

func TestEngine_Route_NilHealth(t *testing.T) {
	routes := []domain.Route{
		route("r1", "1", "conn-http", 10, true),
	}

	// nil health provider = skip health filtering (assume all healthy)
	engine := NewEngine(routes, nil, nil)
	msg := &domain.Message{Destination: "+1234567890"}

	decision, err := engine.Route(context.Background(), msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision.ConnectorID != "conn-http" {
		t.Errorf("expected conn-http, got %s", decision.ConnectorID)
	}
}

func TestEngine_Route_UnhealthySkippedFallsToBackup(t *testing.T) {
	routes := []domain.Route{
		route("primary", "1", "conn-bad", 10, true),
		route("backup", "1", "conn-ok", 5, true),
	}
	health := newMockHealth()
	health.Set("conn-bad", false)  // unhealthy
	health.Set("conn-ok", true)     // healthy

	engine := NewEngine(routes, health, nil)
	msg := &domain.Message{Destination: "+1234567890"}

	decision, err := engine.Route(context.Background(), msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision.ConnectorID != "conn-ok" {
		t.Errorf("expected backup conn-ok (primary unhealthy), got %s", decision.ConnectorID)
	}
}

func TestEngine_Route_NilHealthSkipsHealthCheck(t *testing.T) {
	routes := []domain.Route{
		route("r1", "1", "conn-unknown", 10, true),
	}

	// nil health provider: unknown connector treated as healthy
	engine := NewEngine(routes, nil, nil)
	msg := &domain.Message{Destination: "+1234567890"}

	decision, err := engine.Route(context.Background(), msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision.ConnectorID != "conn-unknown" {
		t.Errorf("expected conn-unknown, got %s", decision.ConnectorID)
	}
}

func TestEngine_Route_GroupKeyIsolation(t *testing.T) {
	// Two groups with same prefix+priority+strategy but different route sets
	// MUST have independent selectors (no shared RoundRobin counter).
	routeA := domain.Route{BaseModel: domain.BaseModel{ID: "a1"}, Type: domain.RouteTypeSMS, Prefix: "1", ConnectorID: "conn-a", Priority: 10, Enabled: true, Strategy: domain.RouteStrategyRoundRobin}
	routeB := domain.Route{BaseModel: domain.BaseModel{ID: "a2"}, Type: domain.RouteTypeSMS, Prefix: "1", ConnectorID: "conn-b", Priority: 10, Enabled: true, Strategy: domain.RouteStrategyRoundRobin}

	// Group 2: different routes, same prefix+priority+strategy
	routeC := domain.Route{BaseModel: domain.BaseModel{ID: "b1"}, Type: domain.RouteTypeSMS, Prefix: "1", ConnectorID: "conn-c", Priority: 10, Enabled: true, Strategy: domain.RouteStrategyRoundRobin}
	routeD := domain.Route{BaseModel: domain.BaseModel{ID: "b2"}, Type: domain.RouteTypeSMS, Prefix: "1", ConnectorID: "conn-d", Priority: 10, Enabled: true, Strategy: domain.RouteStrategyRoundRobin}

	health := newMockHealth()
	health.Set("conn-a", true)
	health.Set("conn-b", true)
	health.Set("conn-c", true)
	health.Set("conn-d", true)

	// First engine with group 1
	engine1 := NewEngine([]domain.Route{routeA, routeB}, health, nil)
	msg1 := &domain.Message{Destination: "+1234567890"}
	d1_a, _ := engine1.Route(context.Background(), msg1)
	d1_b, _ := engine1.Route(context.Background(), msg1)

	// Second engine with group 2 (different routes, same prefix+priority+strategy)
	engine2 := NewEngine([]domain.Route{routeC, routeD}, health, nil)
	msg2 := &domain.Message{Destination: "+1234567890"}
	d2_a, _ := engine2.Route(context.Background(), msg2)
	d2_b, _ := engine2.Route(context.Background(), msg2)

	// Group 1 routes should only distribute among conn-a and conn-b
	if d1_a.ConnectorID != "conn-a" && d1_a.ConnectorID != "conn-b" {
		t.Errorf("group 1 returned connector outside group: %s", d1_a.ConnectorID)
	}
	if d1_b.ConnectorID != "conn-a" && d1_b.ConnectorID != "conn-b" {
		t.Errorf("group 1 returned connector outside group: %s", d1_b.ConnectorID)
	}

	// Group 2 routes should only distribute among conn-c and conn-d
	if d2_a.ConnectorID != "conn-c" && d2_a.ConnectorID != "conn-d" {
		t.Errorf("group 2 returned connector outside group: %s", d2_a.ConnectorID)
	}
	if d2_b.ConnectorID != "conn-c" && d2_b.ConnectorID != "conn-d" {
		t.Errorf("group 2 returned connector outside group: %s", d2_b.ConnectorID)
	}
}

func TestGroupKey(t *testing.T) {
	tests := []struct {
		name   string
		routes []domain.Route
		expect string
	}{
		{
			name:   "single route",
			routes: []domain.Route{{BaseModel: domain.BaseModel{ID: "r1"}, Prefix: "1", Priority: 10, Strategy: domain.RouteStrategyStatic}},
			expect: "r1:1:10:static|",
		},
		{
			name: "two routes sorted by ID",
			routes: []domain.Route{
				{BaseModel: domain.BaseModel{ID: "b"}, Prefix: "44", Priority: 5, Strategy: domain.RouteStrategyRoundRobin},
				{BaseModel: domain.BaseModel{ID: "a"}, Prefix: "44", Priority: 5, Strategy: domain.RouteStrategyRoundRobin},
			},
			expect: "a:44:5:round_robin|b:44:5:round_robin|",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := groupKey(tt.routes); got != tt.expect {
				t.Errorf("groupKey() = %q, want %q", got, tt.expect)
			}
		})
	}
}
