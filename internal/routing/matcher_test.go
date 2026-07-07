package routing

import (
	"testing"

	"github.com/raghna/fury-sms-gateway/internal/domain"
)

func route(id, prefix, connectorID string, priority int, enabled bool) domain.Route {
	return domain.Route{
		BaseModel:   domain.BaseModel{ID: id},
		Type:        domain.RouteTypeSMS,
		Prefix:      prefix,
		ConnectorID: connectorID,
		Strategy:    domain.RouteStrategyStatic,
		Priority:    priority,
		Enabled:     enabled,
	}
}

func TestMatch_EmptyRoutes(t *testing.T) {
	m := &RouteMatcher{}
	msg := &domain.Message{Destination: "+1234567890"}
	result := m.Match(msg, nil)
	if result != nil {
		t.Errorf("expected nil, got %d routes", len(result))
	}
}

func TestMatch_ExactPrefix(t *testing.T) {
	m := &RouteMatcher{}
	msg := &domain.Message{Destination: "+1234567890"}
	routes := []domain.Route{
		route("r1", "1", "conn-http", 10, true),
	}

	result := m.Match(msg, routes)
	if len(result) != 1 {
		t.Fatalf("expected 1 match, got %d", len(result))
	}
	if result[0].ID != "r1" {
		t.Errorf("expected r1, got %s", result[0].ID)
	}
}

func TestMatch_CatchAllPrefix(t *testing.T) {
	m := &RouteMatcher{}
	msg := &domain.Message{Destination: "+1234567890"}
	routes := []domain.Route{
		route("catch-all", "", "conn-http", 1, true),
	}

	result := m.Match(msg, routes)
	if len(result) != 1 {
		t.Fatalf("expected 1 match, got %d", len(result))
	}
	if result[0].ID != "catch-all" {
		t.Errorf("expected catch-all, got %s", result[0].ID)
	}
}

func TestMatch_DisabledRouteSkipped(t *testing.T) {
	m := &RouteMatcher{}
	msg := &domain.Message{Destination: "+1234567890"}
	routes := []domain.Route{
		route("disabled", "1", "conn-http", 10, false),
	}

	result := m.Match(msg, routes)
	if len(result) != 0 {
		t.Errorf("expected 0 matches for disabled route, got %d", len(result))
	}
}

func TestMatch_LongestPrefixWins(t *testing.T) {
	m := &RouteMatcher{}
	msg := &domain.Message{Destination: "+1234567890"}
	routes := []domain.Route{
		route("short", "1", "conn-1", 10, true),
		route("long", "123", "conn-2", 10, true),
	}

	result := m.Match(msg, routes)
	if len(result) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(result))
	}
	// Longest prefix first (same priority, longer prefix wins)
	if result[0].ID != "long" {
		t.Errorf("expected 'long' first (longer prefix), got %s", result[0].ID)
	}
}

func TestMatch_PriorityOrder(t *testing.T) {
	m := &RouteMatcher{}
	msg := &domain.Message{Destination: "+1234567890"}
	routes := []domain.Route{
		route("low", "1", "conn-1", 1, true),
		route("high", "1", "conn-2", 100, true),
	}

	result := m.Match(msg, routes)
	if len(result) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(result))
	}
	// Higher priority first
	if result[0].ID != "high" {
		t.Errorf("expected 'high' first (higher priority), got %s", result[0].ID)
	}
}

func TestMatch_NoMatch(t *testing.T) {
	m := &RouteMatcher{}
	msg := &domain.Message{Destination: "+4498765432"} // UK number
	routes := []domain.Route{
		route("us", "1", "conn-http", 10, true), // US prefix only
	}

	result := m.Match(msg, routes)
	if len(result) != 0 {
		t.Errorf("expected 0 matches, got %d", len(result))
	}
}

func TestMatch_DestinationNormalization(t *testing.T) {
	m := &RouteMatcher{}
	// Both should match prefix "1" after normalization
	tests := []string{"+1234567890", "1234567890", "001234567890"}
	prefix := "1"

	for _, dest := range tests {
		t.Run(dest, func(t *testing.T) {
			msg := &domain.Message{Destination: dest}
			routes := []domain.Route{
				route("match", prefix, "conn-http", 10, true),
			}
			result := m.Match(msg, routes)
			if len(result) != 1 {
				t.Errorf("expected 1 match for %q, got %d", dest, len(result))
			}
		})
	}
}

func TestMatch_WrongTypeSkipped(t *testing.T) {
	m := &RouteMatcher{}
	msg := &domain.Message{Destination: "+1234567890"}
	routes := []domain.Route{
		{
			BaseModel:   domain.BaseModel{ID: "voice"},
			Type:        "voice", // not SMS
			Prefix:      "1",
			ConnectorID: "conn-sip",
			Enabled:     true,
		},
	}

	result := m.Match(msg, routes)
	if len(result) != 0 {
		t.Errorf("expected 0 matches for non-SMS route, got %d", len(result))
	}
}
