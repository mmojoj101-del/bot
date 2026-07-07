package pipeline

import (
	"context"
	"testing"
	"time"

	"github.com/raghna/fury-sms-gateway/internal/domain"
	"github.com/raghna/fury-sms-gateway/internal/domain/events"
)

func TestBuildEventsStage_Name(t *testing.T) {
	s := NewBuildEventsStage()
	if got := s.Name(); got != "build_events" {
		t.Errorf("Name() = %q, want %q", got, "build_events")
	}
}

func TestBuildEventsStage_NilDeliveryOutcome(t *testing.T) {
	s := NewBuildEventsStage()
	state := NewPipelineState(nil, "trace-1")
	_, err := s.Process(context.Background(), state)
	if err == nil || err != ErrBuildEventsNoDeliveryOutcome {
		t.Errorf("got %v, want %v", err, ErrBuildEventsNoDeliveryOutcome)
	}
}

func TestBuildEventsStage_SentEvent(t *testing.T) {
	msg := &domain.Message{BaseModel: domain.BaseModel{ID: "msg-1"}, TenantID: "tenant-1"}
	state := NewPipelineState(msg, "trace-abc")
	state.DeliveryOutcome = &DeliveryOutcome{
		Status:       domain.MessageStatusSent,
		ExternalID:   "ext-999",
		ConnectorID:  "smpp-1",
		Parts:        2,
	}

	s := NewBuildEventsStage()
	result, err := s.Process(context.Background(), state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.DomainEvents) != 1 {
		t.Fatalf("expected 1 event, got %d", len(result.DomainEvents))
	}
	evt := result.DomainEvents[0]
	if evt.EventType != events.EventTypeMessageSentV1 {
		t.Errorf("EventType = %q, want %q", evt.EventType, events.EventTypeMessageSentV1)
	}
	if evt.TraceID != "trace-abc" {
		t.Errorf("TraceID = %q, want trace-abc", evt.TraceID)
	}
	if evt.TenantID != "tenant-1" {
		t.Errorf("TenantID = %q, want tenant-1", evt.TenantID)
	}
}

func TestBuildEventsStage_DeliveredEvent(t *testing.T) {
	msg := &domain.Message{BaseModel: domain.BaseModel{ID: "msg-2"}, TenantID: "tenant-1"}
	state := NewPipelineState(msg, "trace-def")
	state.DeliveryOutcome = &DeliveryOutcome{
		Status:       domain.MessageStatusDelivered,
		ExternalID:   "ext-555",
	}

	s := NewBuildEventsStage()
	result, err := s.Process(context.Background(), state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.DomainEvents) != 1 {
		t.Fatalf("expected 1 event, got %d", len(result.DomainEvents))
	}
	if result.DomainEvents[0].EventType != events.EventTypeMessageDeliveredV1 {
		t.Errorf("EventType = %q, want %q", result.DomainEvents[0].EventType, events.EventTypeMessageDeliveredV1)
	}
}

func TestBuildEventsStage_FailedEvent(t *testing.T) {
	msg := &domain.Message{BaseModel: domain.BaseModel{ID: "msg-3"}, TenantID: "tenant-1", RetryCount: 0, MaxRetries: 3}
	state := NewPipelineState(msg, "trace-ghi")
	state.DeliveryOutcome = &DeliveryOutcome{
		Status:       domain.MessageStatusFailed,
		ExternalID:   "",
		ErrorCode:    "400",
		ErrorMessage: "invalid dest",
		Retryable:    false,
	}

	s := NewBuildEventsStage()
	result, err := s.Process(context.Background(), state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.DomainEvents) != 1 {
		t.Fatalf("expected 1 event, got %d", len(result.DomainEvents))
	}
	if result.DomainEvents[0].EventType != events.EventTypeMessageFailedV1 {
		t.Errorf("EventType = %q, want %q", result.DomainEvents[0].EventType, events.EventTypeMessageFailedV1)
	}
}

func TestBuildEventsStage_RetryingEvent(t *testing.T) {
	msg := &domain.Message{BaseModel: domain.BaseModel{ID: "msg-4"}, TenantID: "tenant-1", RetryCount: 2, MaxRetries: 5}
	state := NewPipelineState(msg, "trace-jkl")
	state.DeliveryOutcome = &DeliveryOutcome{
		Status: domain.MessageStatusRetrying,
	}

	s := NewBuildEventsStage()
	result, err := s.Process(context.Background(), state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.DomainEvents) != 1 {
		t.Fatalf("expected 1 event, got %d", len(result.DomainEvents))
	}
	if result.DomainEvents[0].EventType != events.EventTypeMessageRetryingV1 {
		t.Errorf("EventType = %q, want %q", result.DomainEvents[0].EventType, events.EventTypeMessageRetryingV1)
	}
}

func TestBuildEventsStage_UnknownStatus(t *testing.T) {
	msg := &domain.Message{BaseModel: domain.BaseModel{ID: "msg-6"}, TenantID: "tenant-1"}
	state := NewPipelineState(msg, "trace-pqr")
	state.DeliveryOutcome = &DeliveryOutcome{Status: domain.MessageStatusQueued}

	s := NewBuildEventsStage()
	_, err := s.Process(context.Background(), state)
	if err == nil {
		t.Fatal("expected error for unmapped status")
	}
}

func TestBuildEventsStage_TimeInjection(t *testing.T) {
	fixedTime := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	stage := &BuildEventsStage{now: func() time.Time { return fixedTime }}

	msg := &domain.Message{BaseModel: domain.BaseModel{ID: "msg-8"}, TenantID: "tenant-1"}
	state := NewPipelineState(msg, "trace-vwx")
	state.DeliveryOutcome = &DeliveryOutcome{
		Status:     domain.MessageStatusSent,
		ExternalID: "ext-321",
		Parts:      1,
	}

	result, err := stage.Process(context.Background(), state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.DomainEvents[0].OccurredAt != fixedTime {
		t.Errorf("OccurredAt = %v, want %v", result.DomainEvents[0].OccurredAt, fixedTime)
	}
}

func TestBuildEventsStage_ConnectorIDInSentEvent(t *testing.T) {
	msg := &domain.Message{BaseModel: domain.BaseModel{ID: "msg-5"}, TenantID: "tenant-1"}
	state := NewPipelineState(msg, "trace-mno")
	state.DeliveryOutcome = &DeliveryOutcome{
		Status:      domain.MessageStatusSent,
		ExternalID:  "ext-777",
		ConnectorID: "http-gateway-1",
		Parts:       1,
	}

	s := NewBuildEventsStage()
	result, err := s.Process(context.Background(), state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.DomainEvents[0].EventType != events.EventTypeMessageSentV1 {
		t.Error("expected sent event")
	}
}

func TestBuildEventsStage_RetryableFlagInFailedEvent(t *testing.T) {
	msg := &domain.Message{BaseModel: domain.BaseModel{ID: "msg-9"}, TenantID: "tenant-1", RetryCount: 3, MaxRetries: 3}
	state := NewPipelineState(msg, "trace-xyz")
	state.DeliveryOutcome = &DeliveryOutcome{
		Status:       domain.MessageStatusFailed,
		ExternalID:   "",
		ErrorCode:    "500",
		ErrorMessage: "overloaded",
		Retryable:    true, // was retryable but exhausted
	}

	s := NewBuildEventsStage()
	result, err := s.Process(context.Background(), state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.DomainEvents) != 1 {
		t.Fatalf("expected 1 event, got %d", len(result.DomainEvents))
	}
	if result.DomainEvents[0].EventType != events.EventTypeMessageFailedV1 {
		t.Errorf("EventType = %q, want %q", result.DomainEvents[0].EventType, events.EventTypeMessageFailedV1)
	}
}
