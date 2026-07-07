package routing

import (
	"context"
	"errors"
	"testing"

	"github.com/raghna/fury-sms-gateway/internal/connector"
	"github.com/raghna/fury-sms-gateway/internal/domain"
)

// mockReg implements ConnectorHealthChecker for testing.
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

func (r *mockReg) List() []connector.Connector {
	var result []connector.Connector
	for _, c := range r.connectors {
		result = append(result, c)
	}
	return result
}

func TestEngine_Route_Static(t *testing.T) {
	routes := []domain.Route{
		route("r1", "1", "conn-http", 10, true),
	}
	reg := &mockReg{connectors: map[string]connector.Connector{
		"conn-http": connector.NewMockConnector("conn-http", domain.ConnectorTypeHTTPClient),
	}}

	engine, err := NewEngine(routes, reg, domain.RouteStrategyStatic)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

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
	reg := &mockReg{connectors: map[string]connector.Connector{}}

	engine, err := NewEngine(routes, reg, domain.RouteStrategyStatic)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	msg := &domain.Message{Destination: "+1234567890"} // US number, no UK match
	_, err = engine.Route(context.Background(), msg)
	if err == nil {
		t.Fatal("expected error for no matching route")
	}
}

// unhealthyConnector implements both Connector and HealthChecker.
type unhealthyConnector struct {
	connector.MockConnector
}

func (u *unhealthyConnector) CheckHealth(_ context.Context) error {
	return errors.New("provider down")
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

	engine, err := NewEngine(routes, reg, domain.RouteStrategyStatic)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	msg := &domain.Message{Destination: "+1234567890"}
	_, err = engine.Route(context.Background(), msg)
	if err == nil {
		t.Fatal("expected error when all connectors unhealthy")
	}
	if err.Error() != "routing: all matching routes have unhealthy connectors" {
		t.Errorf("unexpected error message: %v", err)
	}
}

// healthyConnector implements both Connector and HealthChecker.
type healthyConnector struct {
	connector.MockConnector
}

func (h *healthyConnector) CheckHealth(_ context.Context) error {
	return nil
}

func TestEngine_Route_RoundRobin(t *testing.T) {
	routes := []domain.Route{
		route("r1", "1", "conn-a", 10, true),
		route("r2", "1", "conn-b", 10, true),
	}

	reg := &mockReg{connectors: map[string]connector.Connector{
		"conn-a": connector.NewMockConnector("conn-a", domain.ConnectorTypeHTTPClient),
		"conn-b": connector.NewMockConnector("conn-b", domain.ConnectorTypeHTTPClient),
	}}

	engine, err := NewEngine(routes, reg, domain.RouteStrategyRoundRobin)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	msg := &domain.Message{Destination: "+1234567890"}

	// First two calls should return different routes
	d1, _ := engine.Route(context.Background(), msg)
	d2, _ := engine.Route(context.Background(), msg)

	if d1.ConnectorID == d2.ConnectorID {
		t.Errorf("expected different connectors in round-robin, got %s twice", d1.ConnectorID)
	}
}

func TestEngine_Route_MultipleMatches(t *testing.T) {
	routes := []domain.Route{
		route("catch-all", "", "conn-default", 1, true),
		route("premium", "123", "conn-premium", 10, true),
	}

	reg := &mockReg{connectors: map[string]connector.Connector{
		"conn-default": connector.NewMockConnector("conn-default", domain.ConnectorTypeHTTPClient),
		"conn-premium": connector.NewMockConnector("conn-premium", domain.ConnectorTypeHTTPClient),
	}}

	engine, err := NewEngine(routes, reg, domain.RouteStrategyStatic)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Number matching premium prefix
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
	// Engine with nil registry should skip health filtering.
	routes := []domain.Route{
		route("r1", "1", "conn-http", 10, true),
	}

	engine, err := NewEngine(routes, nil, domain.RouteStrategyStatic)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	msg := &domain.Message{Destination: "+1234567890"}
	decision, err := engine.Route(context.Background(), msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision.ConnectorID != "conn-http" {
		t.Errorf("expected conn-http, got %s", decision.ConnectorID)
	}
}
