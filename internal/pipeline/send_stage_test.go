package pipeline

import (
	"context"
	"errors"
	"testing"

	"github.com/raghna/fury-sms-gateway/internal/domain"
)

// routedState creates a PipelineState with Decision and SendRequest populated.
func routedState() *PipelineState {
	state := validState()
	state.Prepared = &PreparedMessage{
		Destination: "+1234567890",
		Encoding:    "GSM7",
		Parts:       1,
	}
	state.Decision = &RoutingDecision{
		RouteID:      "route-1",
		ConnectorID:  "connector-http-1",
		StrategyUsed: "static",
		Priority:     10,
	}
	return state
}

// testConnector implements Connector (alias for connector.Connector) for testing.
type testConnector struct {
	id       string
	protocol domain.ConnectorType
	result   *domain.SendResult
	err      error
}

func (c *testConnector) ID() string                              { return c.id }
func (c *testConnector) Protocol() domain.ConnectorType          { return c.protocol }
func (c *testConnector) Send(_ context.Context, _ *domain.SendRequest) (*domain.SendResult, error) {
	return c.result, c.err
}

// testRegistry implements ConnectorRegistry for testing.
type testRegistry struct {
	connectors map[string]Connector
	getErr     error
}

func (r *testRegistry) Get(id string) (Connector, error) {
	if r.getErr != nil {
		return nil, r.getErr
	}
	c, ok := r.connectors[id]
	if !ok {
		return nil, errors.New("connector not found")
	}
	return c, nil
}

func (r *testRegistry) List() []Connector {
	result := make([]Connector, 0, len(r.connectors))
	for _, c := range r.connectors {
		result = append(result, c)
	}
	return result
}

func TestSendStage_Success(t *testing.T) {
	reg := &testRegistry{
		connectors: map[string]Connector{
			"connector-http-1": &testConnector{
				id:       "connector-http-1",
				protocol: domain.ConnectorTypeHTTPClient,
				result:   &domain.SendResult{ExternalID: "ext-123", Parts: 1},
			},
		},
	}
	stage := NewSendStage(reg)
	state := routedState()

	result, err := stage.Process(context.Background(), state)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if result.SendResult == nil {
		t.Fatal("expected SendResult to be set")
	}
	if !result.SendResult.Success {
		t.Fatal("expected successful send")
	}
	if result.SendResult.ErrorCode != "" {
		t.Fatalf("expected no error code, got %q", result.SendResult.ErrorCode)
	}
	if result.SendResult.ExternalID != "ext-123" {
		t.Fatalf("expected ext-123, got %q", result.SendResult.ExternalID)
	}
}

func TestSendStage_ConnectorError(t *testing.T) {
	reg := &testRegistry{
		connectors: map[string]Connector{
			"connector-http-1": &testConnector{
				id:  "connector-http-1",
				err: errors.New("provider timeout"),
			},
		},
	}
	stage := NewSendStage(reg)
	state := routedState()

	_, err := stage.Process(context.Background(), state)
	if err == nil {
		t.Fatal("expected error when connector fails")
	}
	if state.SendResult != nil {
		t.Fatal("expected SendResult to be nil when send fails")
	}
}

func TestSendStage_ConnectorNotFound(t *testing.T) {
	reg := &testRegistry{
		connectors: map[string]Connector{},
	}
	stage := NewSendStage(reg)
	state := routedState()

	_, err := stage.Process(context.Background(), state)
	if err == nil {
		t.Fatal("expected error for missing connector")
	}
}

func TestSendStage_NoDecision(t *testing.T) {
	reg := &testRegistry{}
	stage := NewSendStage(reg)
	state := validState() // no Decision set

	_, err := stage.Process(context.Background(), state)
	if err == nil {
		t.Fatal("expected error when Decision is nil")
	}
}

func TestSendStage_NoPreparedMessage(t *testing.T) {
	reg := &testRegistry{
		connectors: map[string]Connector{
			"connector-http-1": &testConnector{
				id:     "connector-http-1",
				result: &domain.SendResult{ExternalID: "ext"},
			},
		},
	}
	stage := NewSendStage(reg)
	state := routedState()
	state.Prepared = nil

	_, err := stage.Process(context.Background(), state)
	if err == nil {
		t.Fatal("expected error when Prepared is nil")
	}
}

func TestSendStage_Name(t *testing.T) {
	stage := NewSendStage(&testRegistry{})
	if stage.Name() != "send" {
		t.Fatalf("expected 'send', got %q", stage.Name())
	}
}

func TestPipeline_FullSequence(t *testing.T) {
	// Full integration: Validate → Prepare → Route → Send
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
		NewSendStage(&testRegistry{
			connectors: map[string]Connector{
				"connector-http-1": &testConnector{
					id:     "connector-http-1",
					result: &domain.SendResult{ExternalID: "ext-456", Parts: 1},
				},
			},
		}),
	)

	state := validState()
	err := p.Execute(context.Background(), state)
	if err != nil {
		t.Fatalf("expected pipeline to succeed, got: %v", err)
	}

	if state.Prepared == nil {
		t.Fatal("expected Prepared to be populated")
	}
	if state.Decision == nil {
		t.Fatal("expected Decision")
	}
	if state.SendResult == nil {
		t.Fatal("expected SendResult")
	}
	if !state.SendResult.Success {
		t.Fatal("expected successful send")
	}
	if state.SendResult.ExternalID != "ext-456" {
		t.Fatalf("expected ext-456, got %q", state.SendResult.ExternalID)
	}

	if state.Message.Destination != "+1234567890" {
		t.Fatalf("msg.Destination should remain original, got %q", state.Message.Destination)
	}
}

func TestPipeline_FullSequence_SendFails(t *testing.T) {
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
		NewSendStage(&testRegistry{
			connectors: map[string]Connector{
				"connector-http-1": &testConnector{
					id:  "connector-http-1",
					err: errors.New("provider returned 500"),
				},
			},
		}),
	)

	state := validState()
	err := p.Execute(context.Background(), state)
	if err == nil {
		t.Fatal("expected pipeline to fail when send fails")
	}

	if state.Prepared == nil {
		t.Fatal("expected Prepared even on failure")
	}
	if state.Decision == nil {
		t.Fatal("expected Decision even on failure")
	}
	if state.SendResult != nil {
		t.Fatal("expected no SendResult when send fails")
	}
}
