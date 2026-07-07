package pipeline

import (
	"context"
	"testing"
	"time"

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
		name     string
		sr       *SendResult
		want     domain.MessageStatus
		wantTerm bool
	}{
		{
			name: "success with external ID",
			sr: &SendResult{
				Success:    true,
				ExternalID: "provider-msg-123",
				Parts:      1,
			},
			want:     domain.MessageStatusDelivered,
			wantTerm: true,
		},
		{
			name: "success without ID, no DLR",
			sr: &SendResult{
				Success:    true,
				ExternalID: "",
				Parts:      1,
			},
			want:     domain.MessageStatusDelivered,
			wantTerm: true,
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
			if result.DeliveryOutcome.Terminal != tt.wantTerm {
				t.Errorf("Terminal = %v, want %v", result.DeliveryOutcome.Terminal, tt.wantTerm)
			}
			if result.DeliveryOutcome.Retry {
				t.Error("expected Retry = false")
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
	if result.DeliveryOutcome.Terminal {
		t.Error("expected Terminal = false when DLR pending")
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
	if !result.DeliveryOutcome.Terminal {
		t.Error("expected Terminal = true for failed message")
	}
	if result.DeliveryOutcome.Retry {
		t.Error("expected Retry = false for non-retryable error")
	}
}

func TestHandleResultStage_ProviderError_Retry(t *testing.T) {
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
	if !result.DeliveryOutcome.Retry {
		t.Error("expected Retry = true for retryable error")
	}
	if result.DeliveryOutcome.Terminal {
		t.Error("expected Terminal = false when retrying")
	}
	if result.DeliveryOutcome.RetryAfter <= 0 {
		t.Error("expected RetryAfter > 0")
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
	if !result.DeliveryOutcome.Terminal {
		t.Error("expected Terminal = true after retries exhausted")
	}
	if result.DeliveryOutcome.Retry {
		t.Error("expected Retry = false after retries exhausted")
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
	// DeliveryOutcome should be the only new artifact.
	if state.DeliveryOutcome == nil {
		t.Error("DeliveryOutcome was not set")
	}
}

func TestBackoffForAttempt(t *testing.T) {
	tests := []struct {
		attempt int
		want    time.Duration
	}{
		{0, 1 * time.Second},
		{1, 2 * time.Second},
		{2, 4 * time.Second},
		{3, 8 * time.Second},
		{4, 16 * time.Second},
		{5, 32 * time.Second},
		{6, 64 * time.Second},
		{7, 128 * time.Second},
		{8, 256 * time.Second},
		{9, 300 * time.Second}, // capped at 300s
		{10, 300 * time.Second},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			got := backoffForAttempt(tt.attempt)
			if got != tt.want {
				t.Errorf("backoffForAttempt(%d) = %v, want %v", tt.attempt, got, tt.want)
			}
		})
	}
}
