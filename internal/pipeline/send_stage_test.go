package pipeline

import (
	"context"
	"errors"
	"testing"

	"github.com/raghna/fury-sms-gateway/internal/domain"
)

// routedState creates a PipelineState with Decision and SendRequest populated.
// This simulates the output of Validate→Prepare→Route stages.
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

// mockSender implements domain.Sender for testing.
type mockSender struct {
	result *domain.SendResult
	err    error
}

func (m *mockSender) Type() domain.ConnectorType {
	return domain.ConnectorTypeHTTPClient
}

func (m *mockSender) Send(_ context.Context, _ domain.SendRequest) (*domain.SendResult, error) {
	return m.result, m.err
}

// mockRegistry implements ConnectorRegistry for testing.
type mockRegistry struct {
	senders map[string]domain.Sender
	err     error
}

func (m *mockRegistry) Resolve(_ context.Context, id string) (domain.Sender, error) {
	if m.err != nil {
		return nil, m.err
	}
	s, ok := m.senders[id]
	if !ok {
		return nil, errors.New("connector not found")
	}
	return s, nil
}

func TestSendStage_Success(t *testing.T) {
	reg := &mockRegistry{
		senders: map[string]domain.Sender{
			"connector-http-1": &mockSender{
				result: &domain.SendResult{ExternalID: "ext-123", Parts: 1},
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
	reg := &mockRegistry{
		senders: map[string]domain.Sender{
			"connector-http-1": &mockSender{
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
		senders: map[string]domain.Sender{}, // empty
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

func TestSendStage_NoPreparedMessage(t *testing.T) {
	reg := &mockRegistry{
		senders: map[string]domain.Sender{
			"connector-http-1": &mockSender{result: &domain.SendResult{ExternalID: "ext"}},
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
			senders: map[string]domain.Sender{
				"connector-http-1": &mockSender{
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

	// Verify all stages populated their results
	if state.Prepared == nil {
		t.Fatal("expected Prepared")
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
			senders: map[string]domain.Sender{
				"connector-http-1": &mockSender{
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
