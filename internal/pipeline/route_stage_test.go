package pipeline

import (
	"context"
	"errors"
	"testing"

	"github.com/raghna/fury-sms-gateway/internal/domain"
)

// mockRouter simulates a Router for testing.
type mockRouter struct {
	decision *RoutingDecision
	err      error
}

func (m *mockRouter) Route(_ context.Context, _ *domain.Message) (*RoutingDecision, error) {
	return m.decision, m.err
}

func TestRouteStage_ValidDecision(t *testing.T) {
	expected := &RoutingDecision{
		RouteID:      "route-1",
		ConnectorID:  "connector-http-1",
		StrategyUsed: "static",
		Priority:     10,
	}
	stage := NewRouteStage(&mockRouter{decision: expected})
	state := validState()

	result, err := stage.Process(context.Background(), state)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if result.Decision == nil {
		t.Fatal("expected Decision to be set")
	}
	if result.Decision.ConnectorID != "connector-http-1" {
		t.Fatalf("expected connector-http-1, got %q", result.Decision.ConnectorID)
	}
}

func TestRouteStage_Error(t *testing.T) {
	stage := NewRouteStage(&mockRouter{err: errors.New("no route found")})
	state := validState()

	_, err := stage.Process(context.Background(), state)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestRouteStage_NilDecision(t *testing.T) {
	stage := NewRouteStage(&mockRouter{decision: nil})
	state := validState()

	_, err := stage.Process(context.Background(), state)
	if err == nil {
		t.Fatal("expected error for nil decision, got nil")
	}
}

func TestRouteStage_Name(t *testing.T) {
	stage := NewRouteStage(&mockRouter{decision: &RoutingDecision{}})
	if stage.Name() != "route" {
		t.Fatalf("expected 'route', got %q", stage.Name())
	}
}

func TestPipeline_ValidatePrepareRoute(t *testing.T) {
	// Full pipeline integration test: Validate → Prepare → Route.
	// This validates that state flows correctly through all three stages.
	p := New(
		NewValidateStage(),
		NewPrepareStage(),
		NewRouteStage(&mockRouter{
			decision: &RoutingDecision{
				RouteID:      "route-1",
				ConnectorID:  "connector-http-1",
				StrategyUsed: "static",
				Priority:     10,
			},
		}),
	)

	state := validState()
	err := p.Execute(context.Background(), state)
	if err != nil {
		t.Fatalf("expected pipeline to succeed, got: %v", err)
	}

	// Validate stage passed
	if state.Error != nil {
		t.Fatalf("unexpected error: %v", state.Error)
	}

	// Prepare stage: PreparedMessage populated
	if state.Prepared == nil {
		t.Fatal("expected PreparedMessage from PrepareStage")
	}
	if state.Prepared.Encoding != "GSM7" {
		t.Fatalf("expected GSM7 encoding, got %q", state.Prepared.Encoding)
	}
	if state.Prepared.Parts != 1 {
		t.Fatalf("expected 1 part, got %d", state.Prepared.Parts)
	}

	// Route stage: Decision populated
	if state.Decision == nil {
		t.Fatal("expected Decision from RouteStage")
	}
	if state.Decision.ConnectorID != "connector-http-1" {
		t.Fatalf("expected connector-http-1, got %q", state.Decision.ConnectorID)
	}

	// Verify domain.Message was NOT mutated by any stage
	if state.Message.Destination != "+1234567890" {
		t.Fatalf("msg.Destination should remain original, got %q", state.Message.Destination)
	}
}

func TestPipeline_ValidatePrepareRoute_RouteFails(t *testing.T) {
	p := New(
		NewValidateStage(),
		NewPrepareStage(),
		NewRouteStage(&mockRouter{err: errors.New("no connector available for this message")}),
	)

	state := validState()
	err := p.Execute(context.Background(), state)
	if err == nil {
		t.Fatal("expected pipeline to fail when route fails")
	}

	// PreparedMessage should still be set (PrepareStage ran before RouteStage)
	if state.Prepared == nil {
		t.Fatal("expected PreparedMessage even when route fails")
	}

	// Decision should NOT be set (RouteStage failed)
	if state.Decision != nil {
		t.Fatal("expected no Decision when route fails")
	}
}
