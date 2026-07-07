package routing

import (
	"context"
	"errors"
	"testing"

	"github.com/raghna/fury-sms-gateway/internal/connector"
	"github.com/raghna/fury-sms-gateway/internal/domain"
)

// mockReg implements ConnectorResolver for testing.
type mockReg struct {
	connectors map[string]connector.Connector
}

func (r *mockReg) Get(id string) (connector.Connector, error) {
	c, ok := r.connectors[id]
	if !ok {
		return nil, errors.New("not found")
	}
	return c, nil
}

// unhealthyConnector implements both Connector and HealthChecker (returns error).
type unhealthyConnector struct {
	connector.MockConnector
}

func (u *unhealthyConnector) CheckHealth(_ context.Context) error {
	return errors.New("provider down")
}

// healthyConnector implements both Connector and HealthChecker (returns nil).
type healthyConnector struct {
	connector.MockConnector
}

func (h *healthyConnector) CheckHealth(_ context.Context) error {
	return nil
}

func TestEngine_Route_Static(t *testing.T) {
	routes := []domain.Route{
		route("r1", "1", "conn-http", 10, true),
	}
	reg := &mockReg{connectors: map[string]connector.Connector{
		"conn-http": connector.NewMockConnector("conn-http", domain.ConnectorTypeHTTPClient),
	}}

	engine := NewEngine(routes, reg, nil)
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
	base := connector.NewMockConnector("conn-bad", domain.ConnectorTypeHTTPClient)
	unhealthy := &unhealthyConnector{MockConnector: *base}

	routes := []domain.Route{
		route("r1", "1", "conn-bad", 10, true),
	}
	reg := &mockReg{connectors: map[string]connector.Connector{
		"conn-bad": unhealthy,
	}}

	engine := NewEngine(routes, reg, nil)
	msg := &domain.Message{Destination: "+1234567890"}

	_, err := engine.Route(context.Background(), msg)
	if err == nil {
		t.Fatal("expected error when all connectors unhealthy")
	}
}

func TestEngine_Route_HealthyConnector(t *testing.T) {
	base := connector.NewMockConnector("conn-ok", domain.ConnectorTypeHTTPClient)
	healthy := &healthyConnector{MockConnector: *base}

	routes := []domain.Route{
		route("r1", "1", "conn-ok", 10, true),
	}
	reg := &mockReg{connectors: map[string]connector.Connector{
		"conn-ok": healthy,
	}}

	engine := NewEngine(routes, reg, nil)
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
	reg := &mockReg{connectors: map[string]connector.Connector{
		"conn-a": connector.NewMockConnector("conn-a", domain.ConnectorTypeHTTPClient),
		"conn-b": connector.NewMockConnector("conn-b", domain.ConnectorTypeHTTPClient),
	}}

	engine := NewEngine(routes, reg, nil)
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
	reg := &mockReg{connectors: map[string]connector.Connector{
		"conn-default": connector.NewMockConnector("conn-default", domain.ConnectorTypeHTTPClient),
		"conn-premium": connector.NewMockConnector("conn-premium", domain.ConnectorTypeHTTPClient),
	}}

	engine := NewEngine(routes, reg, nil)
	msg := &domain.Message{Destination: "+1234567890"}

	decision, err := engine.Route(context.Background(), msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision.ConnectorID != "conn-premium" {
		t.Errorf("expected premium connector, got %s", decision.ConnectorID)
	}
}

func TestEngine_Route_NilRegistry(t *testing.T) {
	routes := []domain.Route{
		route("r1", "1", "conn-http", 10, true),
	}

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
	baseBad := connector.NewMockConnector("conn-bad", domain.ConnectorTypeHTTPClient)
	bad := &unhealthyConnector{MockConnector: *baseBad}

	routes := []domain.Route{
		route("primary", "1", "conn-bad", 10, true),
		route("backup", "1", "conn-ok", 5, true),
	}
	reg := &mockReg{connectors: map[string]connector.Connector{
		"conn-bad": bad,
		"conn-ok":  connector.NewMockConnector("conn-ok", domain.ConnectorTypeHTTPClient),
	}}

	engine := NewEngine(routes, reg, nil)
	msg := &domain.Message{Destination: "+1234567890"}

	decision, err := engine.Route(context.Background(), msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision.ConnectorID != "conn-ok" {
		t.Errorf("expected backup conn-ok (primary unhealthy), got %s", decision.ConnectorID)
	}
}

func TestEngine_Route_NilRegistrySkipsHealthCheck(t *testing.T) {
	routes := []domain.Route{
		route("r1", "1", "conn-unknown", 10, true),
	}

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

func TestRouteGroupID(t *testing.T) {
	tests := []struct {
		name     string
		route    domain.Route
		expected string
	}{
		{"static", route("r1", "1", "c1", 10, true), "1:10:static"},
		{"round-robin", domain.Route{Prefix: "44", Priority: 5, Strategy: domain.RouteStrategyRoundRobin}, "44:5:round_robin"},
		{"empty prefix", domain.Route{Prefix: "", Priority: 1, Strategy: domain.RouteStrategyFailover}, ":1:failover"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RouteGroupID(tt.route); got != tt.expected {
				t.Errorf("RouteGroupID() = %q, want %q", got, tt.expected)
			}
		})
	}
}
