package pipeline

import (
	"context"
	"errors"
	"testing"
)

// routedState creates a PipelineState with Decision and SendRequest populated.
// This simulates the output of Validate→Prepare→Route stages.
func routedState() *PipelineState {
	state := validState()
	state.SendRequest = &SendRequest{
		MessageID:   "msg-1",
		Source:      "app-1",
		Destination: "+1234567890",
		Text:        "Hello, World!",
		Encoding:    "GSM7",
		Parts:       1,
		ConnectorID: "connector-http-1",
		TraceID:     "trace-1",
	}
	state.Decision = &RoutingDecision{
		RouteID:      "route-1",
		ConnectorID:  "connector-http-1",
		StrategyUsed: "static",
		Priority:     10,
	}
	return state
}

// mockConnector implements Connector for testing.
type mockConnector struct {
	result *SendResult
	err    error
}

func (m *mockConnector) Send(_ context.Context, _ *SendRequest) (*SendResult, error) {
	return m.result, m.err
}

// mockRegistry implements ConnectorRegistry for testing.
type mockRegistry struct {
	connectors map[string]Connector
	err        error
}

func (m *mockRegistry) Get(_ context.Context, id string) (Connector, error) {
	if m.err != nil {
		return nil, m.err
	}
	c, ok := m.connectors[id]
	if !ok {
		return nil, errors.New("connector not found")
	}
	return c, nil
}

func TestSendStage_Success(t *testing.T) {
	reg := &mockRegistry{
		connectors: map[string]Connector{
			"connector-http-1": &mockConnector{
				result: &SendResult{Success: true, ExternalID: "ext-123", Parts: 1},
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
	if result.SendResult.ExternalID != "ext-123" {
		t.Fatalf("expected ext-123, got %q", result.SendResult.ExternalID)
	}
}

func TestSendStage_ConnectorError(t *testing.T) {
	reg := &mockRegistry{
		connectors: map[string]Connector{
			"connector-http-1": &mockConnector{
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
	reg := &mockRegistry{
		connectors: map[string]Connector{}, // empty
	}
	stage := NewSendStage(reg)
	state := routedState()

	_, err := stage.Process(context.Background(), state)
	if err == nil {
		t.Fatal("expected error for missing connector")
	}
}

func TestSendStage_NoDecision(t *testing.T) {
	reg := &mockRegistry{}
	stage := NewSendStage(reg)
	state := validState() // no Decision set

	_, err := stage.Process(context.Background(), state)
	if err == nil {
		t.Fatal("expected error when Decision is nil")
	}
}

func TestSendStage_NoSendRequest(t *testing.T) {
	reg := &mockRegistry{
		connectors: map[string]Connector{
			"connector-http-1": &mockConnector{result: &SendResult{Success: true}},
		},
	}
	stage := NewSendStage(reg)
	state := routedState()
	state.SendRequest = nil

	_, err := stage.Process(context.Background(), state)
	if err == nil {
		t.Fatal("expected error when SendRequest is nil")
	}
}

func TestSendStage_Name(t *testing.T) {
	stage := NewSendStage(&mockRegistry{})
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
		NewSendStage(&mockRegistry{
			connectors: map[string]Connector{
				"connector-http-1": &mockConnector{
					result: &SendResult{Success: true, ExternalID: "ext-456", Parts: 1},
				},
			},
		}),
	)

	state := validState()
	err := p.Execute(context.Background(), state)
	if err != nil {
		t.Fatalf("expected pipeline to succeed, got: %v", err)
	}

	// Verify all stages populated their results
	if state.SendRequest == nil {
		t.Fatal("expected SendRequest")
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

	// Verify domain.Message was NOT mutated
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
		NewSendStage(&mockRegistry{
			connectors: map[string]Connector{
				"connector-http-1": &mockConnector{
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

	// Verify earlier stages populated their results
	if state.SendRequest == nil {
		t.Fatal("expected SendRequest even on failure")
	}
	if state.Decision == nil {
		t.Fatal("expected Decision even on failure")
	}
	// SendResult should be nil since send failed
	if state.SendResult != nil {
		t.Fatal("expected no SendResult when send fails")
	}
}
