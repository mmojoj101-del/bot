package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/raghna/fury-sms-gateway/internal/domain"
	"github.com/raghna/fury-sms-gateway/internal/domain/events"
)

// BuildEventsStage reads DeliveryOutcome, Message, SendResult, and Decision
// from PipelineState and produces domain events (EventEnvelope slice).
//
// It is deliberately "dumb" — it only maps state artifacts to event payloads.
// No event publishing, no side effects, no business logic (HandleResultStage's job).
//
// Reads:   DeliveryOutcome (Status determines which events to build)
//          Message (ID, TenantID for event metadata)
//          SendResult (ExternalID, Parts, ErrorCode, ErrorMessage)
//          Decision (ConnectorID for sent events)
//          TraceID
// Produces: Events
type BuildEventsStage struct {
	now func() time.Time
}

func NewBuildEventsStage() *BuildEventsStage {
	return &BuildEventsStage{now: time.Now}
}

// Name returns the stage name for logging and metrics.
func (s *BuildEventsStage) Name() string {
	return "build_events"
}

var ErrBuildEventsNoDeliveryOutcome = fmt.Errorf("build events stage: no delivery outcome")

// Process builds domain events from the current pipeline state.
func (s *BuildEventsStage) Process(ctx context.Context, state *PipelineState) (*PipelineState, error) {
	if state.DeliveryOutcome == nil {
		return nil, ErrBuildEventsNoDeliveryOutcome
	}

	msg := state.Message
	sr := state.SendResult
	delivery := state.DeliveryOutcome
	t := s.now()

	var evt events.EventEnvelope

	switch delivery.Status {
	case domain.MessageStatusSent:
		evt = buildSentEvent(msg, sr, state.Decision, state.TraceID, t)

	case domain.MessageStatusDelivered:
		evt = buildDeliveredEvent(msg, sr, state.TraceID, t)

	case domain.MessageStatusFailed:
		evt = buildFailedEvent(msg, sr, state.TraceID, t)

	case domain.MessageStatusRetrying:
		evt = buildRetryingEvent(msg, state.TraceID, t)

	default:
		return nil, fmt.Errorf("build events stage: no event mapping for status %q", delivery.Status)
	}

	state.Events = []events.EventEnvelope{evt}
	return state, nil
}

// --- pure event builder helpers ---

func buildSentEvent(msg *domain.Message, sr *SendResult, decision *RoutingDecision, traceID string, t time.Time) events.EventEnvelope {
	connectorID := ""
	if decision != nil {
		connectorID = decision.ConnectorID
	}
	payload := events.MessageSentV1Payload{
		MessageID:   msg.ID,
		ExternalID:  sr.ExternalID,
		ConnectorID: connectorID,
		Parts:       sr.Parts,
	}
	return makeEvent(events.EventTypeMessageSentV1, msg.TenantID, traceID, t, payload)
}

func buildDeliveredEvent(msg *domain.Message, sr *SendResult, traceID string, t time.Time) events.EventEnvelope {
	payload := events.MessageDeliveredV1Payload{
		MessageID:  msg.ID,
		ExternalID: sr.ExternalID,
	}
	return makeEvent(events.EventTypeMessageDeliveredV1, msg.TenantID, traceID, t, payload)
}

func buildFailedEvent(msg *domain.Message, sr *SendResult, traceID string, t time.Time) events.EventEnvelope {
	payload := events.MessageFailedV1Payload{
		MessageID:    msg.ID,
		ExternalID:   sr.ExternalID,
		ErrorCode:    sr.ErrorCode,
		ErrorMessage: sr.ErrorMessage,
		Attempt:      msg.RetryCount + 1,
		Retryable:    sr.Retryable,
	}
	return makeEvent(events.EventTypeMessageFailedV1, msg.TenantID, traceID, t, payload)
}

func buildRetryingEvent(msg *domain.Message, traceID string, t time.Time) events.EventEnvelope {
	payload := events.MessageRetryingV1Payload{
		MessageID: msg.ID,
		Attempt:   msg.RetryCount + 1,
	}
	return makeEvent(events.EventTypeMessageRetryingV1, msg.TenantID, traceID, t, payload)
}

func makeEvent(eventType, tenantID, traceID string, t time.Time, payload interface{}) events.EventEnvelope {
	raw, _ := json.Marshal(payload)
	return events.EventEnvelope{
		EventType:  eventType,
		OccurredAt: t,
		TraceID:    traceID,
		TenantID:   tenantID,
		Payload:    raw,
	}
}
