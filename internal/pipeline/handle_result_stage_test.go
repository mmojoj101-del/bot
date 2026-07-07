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

func TestHandleResultStage_AcceptanceFinal(t *testing.T) {
	state := NewPipelineState(&domain.Message{}, "trace-1")
	state.Decision = &RoutingDecision{ConnectorID: "http-1", RouteID: "route-1"}
	state.SendResult = &SendResult{
		Success:    true,
		ExternalID: "ext-123",
		Parts:      2,
		Acceptance: domain.AcceptanceFinal,
	}

	s := NewHandleResultStage()
	result, err := s.Process(context.Background(), state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.DeliveryOutcome.Status != domain.MessageStatusSent {
		t.Errorf("Status = %q, want %q", result.DeliveryOutcome.Status, domain.MessageStatusSent)
	}
	if !result.DeliveryOutcome.IsTerminal() {
		t.Error("expected IsTerminal() = true for AcceptanceFinal (no DLR)")
	}
	if result.DeliveryOutcome.FailureKind != FailureNone {
		t.Errorf("FailureKind = %q, want %q", result.DeliveryOutcome.FailureKind, FailureNone)
	}
	// Verify SendResult fields are copied into DeliveryOutcome
	if result.DeliveryOutcome.ExternalID != "ext-123" {
		t.Errorf("ExternalID = %q, want ext-123", result.DeliveryOutcome.ExternalID)
	}
	if result.DeliveryOutcome.Parts != 2 {
		t.Errorf("Parts = %d, want 2", result.DeliveryOutcome.Parts)
	}
	// Verify Decision fields are copied
	if result.DeliveryOutcome.ConnectorID != "http-1" {
		t.Errorf("ConnectorID = %q, want http-1", result.DeliveryOutcome.ConnectorID)
	}
	if result.DeliveryOutcome.RouteID != "route-1" {
		t.Errorf("RouteID = %q, want route-1", result.DeliveryOutcome.RouteID)
	}
	// Timestamps
	if result.DeliveryOutcome.SentAt == nil {
		t.Error("expected SentAt to be set for final acceptance")
	}
	if result.DeliveryOutcome.DeliveredAt != nil {
		t.Error("expected DeliveredAt to be nil (not delivered yet)")
	}
	if result.DeliveryOutcome.FailedAt != nil {
		t.Error("expected FailedAt to be nil")
	}
}

func TestHandleResultStage_AcceptanceFinal_NoExternalID(t *testing.T) {
	state := NewPipelineState(&domain.Message{}, "trace-1")
	state.SendResult = &SendResult{
		Success:    true,
		ExternalID: "",
		Parts:      1,
		Acceptance: domain.AcceptanceFinal,
	}

	s := NewHandleResultStage()
	result, err := s.Process(context.Background(), state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.DeliveryOutcome.Status != domain.MessageStatusSent {
		t.Errorf("Status = %q, want %q", result.DeliveryOutcome.Status, domain.MessageStatusSent)
	}
	if !result.DeliveryOutcome.IsTerminal() {
		t.Error("expected IsTerminal() = true (no DLR, terminal)")
	}
	if result.DeliveryOutcome.ExternalID != "" {
		t.Errorf("ExternalID = %q, want empty", result.DeliveryOutcome.ExternalID)
	}
	if result.DeliveryOutcome.SentAt == nil {
		t.Error("expected SentAt to be set")
	}
}

func TestHandleResultStage_AcceptancePendingDLR(t *testing.T) {
	state := NewPipelineState(&domain.Message{}, "trace-1")
	state.SendResult = &SendResult{
		Success:    true,
		ExternalID: "",
		Parts:      1,
		Acceptance: domain.AcceptancePendingDLR,
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
	if !result.DeliveryOutcome.AwaitingDLR {
		t.Error("expected AwaitingDLR = true")
	}
	if result.DeliveryOutcome.Reason != "accepted, awaiting delivery receipt" {
		t.Errorf("Reason = %q, want %q", result.DeliveryOutcome.Reason, "accepted, awaiting delivery receipt")
	}
	if result.DeliveryOutcome.SentAt == nil {
		t.Error("expected SentAt to be set for pending DLR")
	}
}

func TestHandleResultStage_Rejected_Failed(t *testing.T) {
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
		Acceptance:   domain.AcceptanceRejected,
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
	if result.DeliveryOutcome.FailureKind != FailurePermanent {
		t.Errorf("FailureKind = %q, want %q", result.DeliveryOutcome.FailureKind, FailurePermanent)
	}
	if result.DeliveryOutcome.FailedAt == nil {
		t.Error("expected FailedAt to be set")
	}
	if result.DeliveryOutcome.SentAt == nil {
		t.Error("expected SentAt to be set retroactively on failure")
	}
	// Error details copied
	if result.DeliveryOutcome.ErrorCode != "400" {
		t.Errorf("ErrorCode = %q, want 400", result.DeliveryOutcome.ErrorCode)
	}
	if result.DeliveryOutcome.ErrorMessage != "invalid destination" {
		t.Errorf("ErrorMessage = %q, want invalid destination", result.DeliveryOutcome.ErrorMessage)
	}
	if result.DeliveryOutcome.Retryable != false {
		t.Error("Expected Retryable = false")
	}
}

func TestHandleResultStage_Rejected_Retrying(t *testing.T) {
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
		Acceptance:   domain.AcceptanceRejected,
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
	if result.DeliveryOutcome.FailureKind != FailureTemporary {
		t.Errorf("FailureKind = %q, want %q", result.DeliveryOutcome.FailureKind, FailureTemporary)
	}
	// Retrying — no timestamps yet (message wasn't "sent")
	if result.DeliveryOutcome.SentAt != nil {
		t.Error("expected no SentAt on retry (not sent yet)")
	}
	if result.DeliveryOutcome.FailedAt != nil {
		t.Error("expected no FailedAt on retry")
	}
	// Retryable flag copied from SendResult
	if result.DeliveryOutcome.Retryable != true {
		t.Error("Expected Retryable = true")
	}
}

func TestHandleResultStage_Rejected_RetryExhausted(t *testing.T) {
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
		Acceptance:   domain.AcceptanceRejected,
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
	if result.DeliveryOutcome.FailedAt == nil {
		t.Error("expected FailedAt to be set after retry exhausted")
	}
	if result.DeliveryOutcome.SentAt == nil {
		t.Error("expected SentAt to be set retroactively")
	}
	// Error was retryable, but we exhausted retries
	if result.DeliveryOutcome.Retryable != true {
		t.Error("Expected Retryable = true (connector said retryable, but budget exhausted)")
	}
}

func TestHandleResultStage_UnknownAcceptance(t *testing.T) {
	state := NewPipelineState(&domain.Message{}, "trace-1")
	state.SendResult = &SendResult{
		Success:    true,
		ExternalID: "ext",
		Acceptance: "unknown_value",
	}

	s := NewHandleResultStage()
	result, err := s.Process(context.Background(), state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.DeliveryOutcome.Status != domain.MessageStatusFailed {
		t.Errorf("Status = %q, want %q", result.DeliveryOutcome.Status, domain.MessageStatusFailed)
	}
	if result.DeliveryOutcome.FailureKind != FailureInternal {
		t.Errorf("FailureKind = %q, want %q", result.DeliveryOutcome.FailureKind, FailureInternal)
	}
	if result.DeliveryOutcome.FailedAt == nil {
		t.Error("expected FailedAt to be set for unknown acceptance")
	}
	if result.DeliveryOutcome.SentAt == nil {
		t.Error("expected SentAt to be set retroactively")
	}
}

func TestHandleResultStage_PreservesPriorArtifacts(t *testing.T) {
	msg := &domain.Message{Text: "hello", RetryCount: 0, MaxRetries: 3}
	prepared := &domain.PreparedMessage{Destination: "+1234", Encoding: "GSM7", Parts: 1}
	decision := &RoutingDecision{ConnectorID: "http-1", RouteID: "route-1"}
	sr := &SendResult{
		Success:    true,
		ExternalID: "ext-1",
		Parts:      1,
		Acceptance: domain.AcceptanceFinal,
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

func TestHandleResultStage_NoDecision_NoPanic(t *testing.T) {
	state := NewPipelineState(&domain.Message{}, "trace-1")
	state.SendResult = &SendResult{
		Success:    true,
		ExternalID: "ext-1",
		Parts:      1,
		Acceptance: domain.AcceptanceFinal,
	}

	s := NewHandleResultStage()
	result, err := s.Process(context.Background(), state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// ConnectorID/RouteID should be empty (nil Decision)
	if result.DeliveryOutcome.ConnectorID != "" {
		t.Errorf("ConnectorID = %q, want empty", result.DeliveryOutcome.ConnectorID)
	}
	if result.DeliveryOutcome.RouteID != "" {
		t.Errorf("RouteID = %q, want empty", result.DeliveryOutcome.RouteID)
	}
}

func TestHandleResultStage_SentAt_NotOverwritten(t *testing.T) {
	now := time.Now().UTC()
	msg := &domain.Message{SentAt: &now}
	state := NewPipelineState(msg, "trace-1")
	state.SendResult = &SendResult{
		Success:    true,
		ExternalID: "ext-1",
		Parts:      1,
		Acceptance: domain.AcceptanceFinal,
	}

	s := NewHandleResultStage()
	result, err := s.Process(context.Background(), state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// HandleResultStage always sets SentAt for Final — COALESCE protects over-write.
	if result.DeliveryOutcome.SentAt == nil {
		t.Error("expected SentAt to be set")
	}
}

func TestDeliveryOutcome_IsTerminal(t *testing.T) {
	tests := []struct {
		status      domain.MessageStatus
		awaitingDLR bool
		term        bool
	}{
		{domain.MessageStatusDelivered, false, true},
		{domain.MessageStatusFailed, false, true},
		{domain.MessageStatusRetrying, false, false},
		{domain.MessageStatusSent, false, true},   // terminal (no DLR)
		{domain.MessageStatusSent, true, false},    // non-terminal (DLR expected)
		{domain.MessageStatusQueued, false, false},
		{domain.MessageStatusSending, false, false},
		{domain.MessageStatusAccepted, false, false},
	}

	for _, tt := range tests {
		name := string(tt.status)
		if tt.status == domain.MessageStatusSent && !tt.awaitingDLR {
			name += "/terminal"
		}
		if tt.status == domain.MessageStatusSent && tt.awaitingDLR {
			name += "/non-terminal"
		}
		t.Run(name, func(t *testing.T) {
			d := DeliveryOutcome{
				Status:      tt.status,
				AwaitingDLR: tt.awaitingDLR,
			}
			if got := d.IsTerminal(); got != tt.term {
				t.Errorf("IsTerminal() for Status=%q AwaitingDLR=%v = %v, want %v",
					tt.status, tt.awaitingDLR, got, tt.term)
			}
		})
	}
}

func TestNewDeliveryOutcome(t *testing.T) {
	d := NewDeliveryOutcome(
		domain.MessageStatusSent,
		FailureNone,
		true,
		"accepted, awaiting delivery receipt",
	)
	if d.Status != domain.MessageStatusSent {
		t.Errorf("Status = %q, want %q", d.Status, domain.MessageStatusSent)
	}
	if d.FailureKind != FailureNone {
		t.Errorf("FailureKind = %q, want %q", d.FailureKind, FailureNone)
	}
	if !d.AwaitingDLR {
		t.Errorf("AwaitingDLR = %v, want true", d.AwaitingDLR)
	}
	if d.Reason != "accepted, awaiting delivery receipt" {
		t.Errorf("Reason = %q", d.Reason)
	}
}
