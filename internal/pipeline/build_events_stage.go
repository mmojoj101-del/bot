package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/raghna/fury-sms-gateway/internal/domain"
	"github.com/raghna/fury-sms-gateway/internal/domain/events"
)

// BuildEventsStage reads DeliveryOutcome and Message from PipelineState
// and produces domain events.
//
// It is deliberately "dumb" — it only maps DeliveryOutcome to event payloads.
// No event publishing, no side effects, no business logic.
//
// Reads:   DeliveryOutcome (Status, ExternalID, ConnectorID, Parts,
//                            ErrorCode, ErrorMessage, Retryable)
//          Message (ID, TenantID, RetryCount for event metadata)
// Produces: DomainEvents
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

// Process builds domain events from DeliveryOutcome + Message.
func (s *BuildEventsStage) Process(ctx context.Context, state *PipelineState) (*PipelineState, error) {
	if state.DeliveryOutcome == nil {
		return nil, ErrBuildEventsNoDeliveryOutcome
	}

	delivery := state.DeliveryOutcome
	msg := state.Message
	t := s.now()

	var evt events.EventEnvelope

	switch delivery.Status {
	case domain.MessageStatusSent:
		payload := events.MessageSentV1Payload{
			MessageID:   msg.ID,
			ExternalID:  delivery.ExternalID,
			ConnectorID: delivery.ConnectorID,
			Parts:       delivery.Parts,
		}
		evt = makeEvent(events.EventTypeMessageSentV1, msg.TenantID, state.TraceID, t, payload)

	case domain.MessageStatusDelivered:
		payload := events.MessageDeliveredV1Payload{
			MessageID:  msg.ID,
			ExternalID: delivery.ExternalID,
		}
		evt = makeEvent(events.EventTypeMessageDeliveredV1, msg.TenantID, state.TraceID, t, payload)

	case domain.MessageStatusFailed:
		payload := events.MessageFailedV1Payload{
			MessageID:    msg.ID,
			ExternalID:   delivery.ExternalID,
			ErrorCode:    delivery.ErrorCode,
			ErrorMessage: delivery.ErrorMessage,
			Attempt:      msg.RetryCount + 1,
			Retryable:    delivery.Retryable,
		}
		evt = makeEvent(events.EventTypeMessageFailedV1, msg.TenantID, state.TraceID, t, payload)

	case domain.MessageStatusRetrying:
		payload := events.MessageRetryingV1Payload{
			MessageID: msg.ID,
			Attempt:   msg.RetryCount + 1,
		}
		evt = makeEvent(events.EventTypeMessageRetryingV1, msg.TenantID, state.TraceID, t, payload)

	default:
		return nil, fmt.Errorf("build events stage: no event mapping for status %q", delivery.Status)
	}

	state.DomainEvents = []events.EventEnvelope{evt}
	return state, nil
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
