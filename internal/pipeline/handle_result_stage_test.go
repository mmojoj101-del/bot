package pipeline

import (
	"context"
	"testing"

	"github.com/raghna/fury-sms-gateway/internal/domain"
)

func TestHandleResultStage_Name(t *testing.T) {
	s := NewHandleResultStage()
	if got := s.Name(); got != "handle_result" {
		t.Errorf("Name() = %q, want %q", got, "handle_result")
	}
}

func TestHandleResultStage_NilSendResult(t *testing.T) {
	s := NewHandleResultStage()
	state := NewPipelineState(&domain.Message{}, "trace-1")
	_, err := s.Process(context.Background(), state)
	if err == nil {
		t.Fatal("expected error for nil SendResult")
	}
	if err != ErrNoSendResult {
		t.Errorf("got %v, want %v", err, ErrNoSendResult)
	}
}

func TestHandleResultStage_Delivered(t *testing.T) {
	tests := []struct {
		name string
		sr   *SendResult
		want domain.MessageStatus
	}{
		{
			name: "success with external ID",
			sr: &SendResult{
				Success:    true,
				ExternalID: "provider-msg-123",
				Parts:      1,
			},
			want: domain.MessageStatusDelivered,
		},
		{
			name: "success without ID, no DLR",
			sr: &SendResult{
				Success:    true,
				ExternalID: "",
				Parts:      1,
			},
			want: domain.MessageStatusDelivered,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := NewPipelineState(&domain.Message{}, "trace-1")
			state.SendResult = tt.sr

			s := NewHandleResultStage()
			result, err := s.Process(context.Background(), state)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.DeliveryOutcome == nil {
				t.Fatal("DeliveryOutcome is nil")
			}
			if result.DeliveryOutcome.Status != tt.want {
				t.Errorf("Status = %q, want %q", result.DeliveryOutcome.Status, tt.want)
			}
			if !result.DeliveryOutcome.IsTerminal() {
				t.Error("expected IsTerminal() = true for Delivered")
			}
			if result.DeliveryOutcome.FailureKind != FailureNone {
				t.Errorf("FailureKind = %q, want %q", result.DeliveryOutcome.FailureKind, FailureNone)
			}
		})
	}
}

func TestHandleResultStage_SentPendingDLR(t *testing.T) {
	state := NewPipelineState(&domain.Message{}, "trace-1")
	state.SendResult = &SendResult{
		Success:     true,
		ExternalID:  "",
		Parts:       1,
		RequestsDLR: true,
	}

	s := NewHandleResultStage()
	result, err := s.Process(context.Background(), state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.DeliveryOutcome.Status != domain.MessageStatusSent {
		t.Errorf("Status = %q, want %q", result.DeliveryOutcome.Status, domain.MessageStatusSent)
	}
	if result.DeliveryOutcome.IsTerminal() {
		t.Error("expected IsTerminal() = false when DLR pending")
	}
	if result.DeliveryOutcome.Reason != "sent, awaiting delivery receipt" {
		t.Errorf("Reason = %q, want %q", result.DeliveryOutcome.Reason, "sent, awaiting delivery receipt")
	}
}

func TestHandleResultStage_ProviderError_Failed(t *testing.T) {
	// Non-retryable provider error.
	state := NewPipelineState(&domain.Message{
		RetryCount: 0,
		MaxRetries: 3,
	}, "trace-1")
	state.SendResult = &SendResult{
		Success:      false,
		ExternalID:   "",
		ErrorCode:    "400",
		ErrorMessage: "invalid destination",
		Retryable:    false,
	}

	s := NewHandleResultStage()
	result, err := s.Process(context.Background(), state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.DeliveryOutcome.Status != domain.MessageStatusFailed {
		t.Errorf("Status = %q, want %q", result.DeliveryOutcome.Status, domain.MessageStatusFailed)
	}
	if !result.DeliveryOutcome.IsTerminal() {
		t.Error("expected IsTerminal() = true for Failed")
	}
	if result.DeliveryOutcome.FailureKind != FailureRejected {
		t.Errorf("FailureKind = %q, want %q", result.DeliveryOutcome.FailureKind, FailureRejected)
	}
}

func TestHandleResultStage_ProviderError_Retrying(t *testing.T) {
	// Retryable provider error, still within retry limit.
	state := NewPipelineState(&domain.Message{
		RetryCount: 1,
		MaxRetries: 3,
	}, "trace-1")
	state.SendResult = &SendResult{
		Success:      false,
		ExternalID:   "",
		ErrorCode:    "500",
		ErrorMessage: "provider overloaded",
		Retryable:    true,
	}

	s := NewHandleResultStage()
	result, err := s.Process(context.Background(), state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.DeliveryOutcome.Status != domain.MessageStatusRetrying {
		t.Errorf("Status = %q, want %q", result.DeliveryOutcome.Status, domain.MessageStatusRetrying)
	}
	if result.DeliveryOutcome.IsTerminal() {
		t.Error("expected IsTerminal() = false when retrying")
	}
	if result.DeliveryOutcome.FailureKind != FailureProvider {
		t.Errorf("FailureKind = %q, want %q", result.DeliveryOutcome.FailureKind, FailureProvider)
	}
}

func TestHandleResultStage_ProviderError_RetryExhausted(t *testing.T) {
	// Retryable error but retries exhausted.
	state := NewPipelineState(&domain.Message{
		RetryCount: 3,
		MaxRetries: 3,
	}, "trace-1")
	state.SendResult = &SendResult{
		Success:      false,
		ExternalID:   "",
		ErrorCode:    "500",
		ErrorMessage: "provider overloaded",
		Retryable:    true,
	}

	s := NewHandleResultStage()
	result, err := s.Process(context.Background(), state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.DeliveryOutcome.Status != domain.MessageStatusFailed {
		t.Errorf("Status = %q, want %q", result.DeliveryOutcome.Status, domain.MessageStatusFailed)
	}
	if !result.DeliveryOutcome.IsTerminal() {
		t.Error("expected IsTerminal() = true after retries exhausted")
	}
	if result.DeliveryOutcome.FailureKind != FailureProvider {
		t.Errorf("FailureKind = %q, want %q", result.DeliveryOutcome.FailureKind, FailureProvider)
	}
}

func TestHandleResultStage_PreservesPriorArtifacts(t *testing.T) {
	// Ensure HandleResultStage does not modify Message, Prepared, Decision, or SendResult.
	msg := &domain.Message{Text: "hello", RetryCount: 0, MaxRetries: 3}
	prepared := &domain.PreparedMessage{Destination: "+1234", Encoding: "GSM7", Parts: 1}
	decision := &RoutingDecision{ConnectorID: "http-1", RouteID: "route-1"}
	sr := &SendResult{
		Success:    true,
		ExternalID: "ext-1",
		Parts:      1,
	}

	state := NewPipelineState(msg, "trace-1")
	state.Prepared = prepared
	state.Decision = decision
	state.SendResult = sr

	s := NewHandleResultStage()
	_, err := s.Process(context.Background(), state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify none of the prior artifacts were modified.
	if state.Message.Text != "hello" {
		t.Error("Message was modified")
	}
	if state.Prepared != prepared {
		t.Error("Prepared was replaced")
	}
	if state.Decision != decision {
		t.Error("Decision was replaced")
	}
	if state.SendResult != sr {
		t.Error("SendResult was replaced")
	}
	if state.DeliveryOutcome == nil {
		t.Error("DeliveryOutcome was not set")
	}
}

func TestDeliveryOutcome_IsTerminal(t *testing.T) {
	tests := []struct {
		status domain.MessageStatus
		term   bool
	}{
		{domain.MessageStatusDelivered, true},
		{domain.MessageStatusFailed, true},
		{domain.MessageStatusRetrying, false},
		{domain.MessageStatusSent, false},
		{domain.MessageStatusQueued, false},
		{domain.MessageStatusSending, false},
		{domain.MessageStatusAccepted, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			d := DeliveryOutcome{Status: tt.status}
			if got := d.IsTerminal(); got != tt.term {
				t.Errorf("IsTerminal() for Status=%q = %v, want %v", tt.status, got, tt.term)
			}
		})
	}
}
