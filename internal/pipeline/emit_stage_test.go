package pipeline

import (
	"context"
	"errors"
	"testing"

	"github.com/raghna/fury-sms-gateway/internal/domain/events"
)

type mockPublisher struct {
	published []events.EventEnvelope
	publishErr error
}

func (m *mockPublisher) Publish(_ context.Context, e events.EventEnvelope) error {
	m.published = append(m.published, e)
	return m.publishErr
}

func TestEmitStage_Name(t *testing.T) {
	s := NewEmitStage(&mockPublisher{})
	if got := s.Name(); got != "emit" {
		t.Errorf("Name() = %q, want %q", got, "emit")
	}
}

func TestEmitStage_NilPublisher(t *testing.T) {
	s := NewEmitStage(nil)
	state := NewPipelineState(nil, "trace-1")
	_, err := s.Process(context.Background(), state)
	if err == nil || err != ErrEmitEmptyPublisher {
		t.Errorf("got %v, want %v", err, ErrEmitEmptyPublisher)
	}
}

func TestEmitStage_PublishesAllEvents(t *testing.T) {
	pub := &mockPublisher{}
	s := NewEmitStage(pub)

	state := NewPipelineState(nil, "trace-1")
	state.PendingEvents = []events.EventEnvelope{
		{EventType: events.EventTypeMessageSentV1, TraceID: "trace-1"},
		{EventType: events.EventTypeMessageDeliveredV1, TraceID: "trace-2"},
	}

	result, err := s.Process(context.Background(), state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(pub.published) != 2 {
		t.Fatalf("expected 2 published events, got %d", len(pub.published))
	}
	if pub.published[0].EventType != events.EventTypeMessageSentV1 {
		t.Errorf("event[0].EventType = %q, want %q", pub.published[0].EventType, events.EventTypeMessageSentV1)
	}
	if result.PendingEvents != nil {
		t.Error("expected PendingEvents to be cleared after publish")
	}
}

func TestEmitStage_EmptyEventsNoOp(t *testing.T) {
	pub := &mockPublisher{}
	s := NewEmitStage(pub)

	state := NewPipelineState(nil, "trace-1")
	state.PendingEvents = nil

	_, err := s.Process(context.Background(), state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pub.published) != 0 {
		t.Errorf("expected 0 published events, got %d", len(pub.published))
	}
}

func TestEmitStage_PublishError(t *testing.T) {
	pub := &mockPublisher{publishErr: errors.New("bus full")}
	s := NewEmitStage(pub)

	state := NewPipelineState(nil, "trace-1")
	state.PendingEvents = []events.EventEnvelope{
		{EventType: events.EventTypeMessageSentV1},
	}

	_, err := s.Process(context.Background(), state)
	if err == nil {
		t.Fatal("expected error")
	}
}
