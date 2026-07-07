package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/raghna/fury-sms-gateway/internal/domain"
	"github.com/raghna/fury-sms-gateway/internal/domain/events"
)

// HandleResultStage interprets SendResult and produces two artifacts:
//   - DeliveryOutcome (business decision)
//   - PendingEvents (domain events for subscribers)
//
// Both are pure data — no DB, no event bus, no external calls.
// EmitStage later publishes the PendingEvents.
type HandleResultStage struct{}

func NewHandleResultStage() *HandleResultStage {
	return &HandleResultStage{}
}

func (s *HandleResultStage) Name() string {
	return "handle_result"
}

var ErrNoSendResult = fmt.Errorf("handle result stage: no send result to interpret")

// Process produces DeliveryOutcome and PendingEvents from SendResult.
//
// AcceptanceFinal       → Sent, terminal, event: message.sent.v1
// AcceptancePendingDLR  → Sent, non-terminal, event: message.sent.v1
// AcceptanceRejected+   → Retrying, event: message.retrying.v1
// AcceptanceRejected−   → Failed, event: message.failed.v1
func (s *HandleResultStage) Process(ctx context.Context, state *PipelineState) (*PipelineState, error) {
	if state.SendResult == nil {
		return nil, ErrNoSendResult
	}

	sr := state.SendResult
	msg := state.Message
	traceID := state.TraceID
	now := time.Now().UTC()

	var outcome DeliveryOutcome
	var pendingEvents []events.EventEnvelope

	switch sr.Acceptance {
	case domain.AcceptanceFinal:
		outcome = NewDeliveryOutcome(
			domain.MessageStatusSent, FailureNone, false,
			fmt.Sprintf("accepted by provider, id=%s", sr.ExternalID),
		)
		pendingEvents = append(pendingEvents, sentEvent(msg, sr, traceID, now))

	case domain.AcceptancePendingDLR:
		outcome = NewDeliveryOutcome(
			domain.MessageStatusSent, FailureNone, true,
			"accepted, awaiting delivery receipt",
		)
		pendingEvents = append(pendingEvents, sentEvent(msg, sr, traceID, now))

	case domain.AcceptanceRejected:
		if sr.Retryable && msg.RetryCount < msg.MaxRetries {
			outcome = NewDeliveryOutcome(
				domain.MessageStatusRetrying, FailureTemporary, false,
				fmt.Sprintf("retryable rejection: %s", sr.ErrorCode),
			)
			pendingEvents = append(pendingEvents, retryingEvent(msg, sr, traceID, now))
		} else {
			outcome = NewDeliveryOutcome(
				domain.MessageStatusFailed, FailurePermanent, false,
				fmt.Sprintf("rejected: %s", sr.ErrorCode),
			)
			pendingEvents = append(pendingEvents, failedEvent(msg, sr, traceID, now))
		}

	default:
		outcome = NewDeliveryOutcome(
			domain.MessageStatusFailed, FailureInternal, false,
			fmt.Sprintf("unknown acceptance kind: %s", sr.Acceptance),
		)
		pendingEvents = append(pendingEvents, failedEvent(msg, sr, traceID, now))
	}

	state.DeliveryOutcome = &outcome
	state.PendingEvents = pendingEvents
	return state, nil
}

// --- event builders (pure, no side effects) ---

func sentEvent(msg *domain.Message, sr *SendResult, traceID string, t time.Time) events.EventEnvelope {
	payload := events.MessageSentV1Payload{
		MessageID:   msg.ID,
		ExternalID:  sr.ExternalID,
		ConnectorID: "", // set by SendStage from state.Decision — not available here
		Parts:       sr.Parts,
	}
	return makeEvent(events.EventTypeMessageSentV1, msg, traceID, t, payload)
}

func retryingEvent(msg *domain.Message, sr *SendResult, traceID string, t time.Time) events.EventEnvelope {
	payload := events.MessageRetryingV1Payload{
		MessageID: msg.ID,
		Attempt:   msg.RetryCount + 1,
	}
	return makeEvent(events.EventTypeMessageRetryingV1, msg, traceID, t, payload)
}

func failedEvent(msg *domain.Message, sr *SendResult, traceID string, t time.Time) events.EventEnvelope {
	payload := events.MessageFailedV1Payload{
		MessageID:    msg.ID,
		ExternalID:   sr.ExternalID,
		ErrorCode:    sr.ErrorCode,
		ErrorMessage: sr.ErrorMessage,
		Attempt:      msg.RetryCount + 1,
		Retryable:    sr.Retryable,
	}
	return makeEvent(events.EventTypeMessageFailedV1, msg, traceID, t, payload)
}

func makeEvent(eventType string, msg *domain.Message, traceID string, t time.Time, payload interface{}) events.EventEnvelope {
	raw, _ := json.Marshal(payload)
	return events.EventEnvelope{
		EventID:    "", // set by publisher if needed
		EventType:  eventType,
		OccurredAt: t,
		TraceID:    traceID,
		TenantID:   msg.TenantID,
		Payload:    raw,
	}
}
