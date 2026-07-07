package pipeline

import (
	"context"
	"testing"

	"github.com/raghna/fury-sms-gateway/internal/domain"
	"github.com/raghna/fury-sms-gateway/internal/domain/events"
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
	// AcceptanceFinal → Sent, Terminal=true (no DLR expected)
	state := NewPipelineState(&domain.Message{}, "trace-1")
	state.SendResult = &SendResult{
		Success:    true,
		ExternalID: "ext-123",
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
		t.Error("expected IsTerminal() = true for AcceptanceFinal (no DLR)")
	}
	if result.DeliveryOutcome.FailureKind != FailureNone {
		t.Errorf("FailureKind = %q, want %q", result.DeliveryOutcome.FailureKind, FailureNone)
	}
}

func TestHandleResultStage_AcceptanceFinal_NoExternalID(t *testing.T) {
	// Final acceptance without ID — still Sent, terminal.
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
}

func TestHandleResultStage_AcceptancePendingDLR(t *testing.T) {
	// AcceptancePendingDLR → Sent, Terminal=false (DLR expected)
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
	if result.DeliveryOutcome.Reason != "accepted, awaiting delivery receipt" {
		t.Errorf("Reason = %q, want %q", result.DeliveryOutcome.Reason, "accepted, awaiting delivery receipt")
	}
}

func TestHandleResultStage_Rejected_Failed(t *testing.T) {
	// Non-retryable rejection → Failed, Terminal=true
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
}

func TestHandleResultStage_Rejected_Retrying(t *testing.T) {
	// Retryable rejection, budget available → Retrying, Terminal=false
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
}

func TestHandleResultStage_Rejected_RetryExhausted(t *testing.T) {
	// Retryable error but retries exhausted → Failed, Terminal=true
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

func TestHandleResultStage_ProducesPendingEvents(t *testing.T) {
	msg := &domain.Message{BaseModel: domain.BaseModel{ID: "msg-1"}, Text: "hello", TenantID: "tenant-1", RetryCount: 0, MaxRetries: 3}
	state := NewPipelineState(msg, "trace-abc")
	state.SendResult = &SendResult{
		Success:    true,
		ExternalID: "ext-123",
		Parts:      1,
		Acceptance: domain.AcceptanceFinal,
	}

	s := NewHandleResultStage()
	result, err := s.Process(context.Background(), state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.PendingEvents) == 0 {
		t.Fatal("expected PendingEvents to be non-empty")
	}
	if result.PendingEvents[0].EventType != events.EventTypeMessageSentV1 {
		t.Errorf("EventType = %q, want %q", result.PendingEvents[0].EventType, events.EventTypeMessageSentV1)
	}
	if result.PendingEvents[0].TraceID != "trace-abc" {
		t.Errorf("TraceID = %q, want trace-abc", result.PendingEvents[0].TraceID)
	}
	if result.PendingEvents[0].TenantID != "tenant-1" {
		t.Errorf("TenantID = %q, want tenant-1", result.PendingEvents[0].TenantID)
	}
	if result.DeliveryOutcome == nil {
		t.Error("expected DeliveryOutcome to also be set")
	}
}

func TestHandleResultStage_FailedEvent(t *testing.T) {
	msg := &domain.Message{BaseModel: domain.BaseModel{ID: "msg-2"}, TenantID: "tenant-1", RetryCount: 0, MaxRetries: 3}
	state := NewPipelineState(msg, "trace-xyz")
	state.SendResult = &SendResult{
		Success:      false,
		ExternalID:   "",
		ErrorCode:    "400",
		ErrorMessage: "invalid dest",
		Retryable:    false,
		Acceptance:   domain.AcceptanceRejected,
	}

	s := NewHandleResultStage()
	result, err := s.Process(context.Background(), state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.PendingEvents) != 1 {
		t.Fatalf("expected 1 event, got %d", len(result.PendingEvents))
	}
	if result.PendingEvents[0].EventType != events.EventTypeMessageFailedV1 {
		t.Errorf("EventType = %q, want %q", result.PendingEvents[0].EventType, events.EventTypeMessageFailedV1)
	}
}
