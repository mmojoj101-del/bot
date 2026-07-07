package pipeline

import (
	"context"
	"fmt"
	"time"

	"github.com/raghna/fury-sms-gateway/internal/domain"
)

// HandleResultStage interprets SendResult and produces DeliveryOutcome.
// Pure logic — no DB, no event bus, no event building.
//
// HandleResultStage is the LAST consumer of SendResult and Decision.
// It copies all fields needed downstream into DeliveryOutcome so that
// PersistStage, BuildEventsStage, and RetryDecorator only need
// DeliveryOutcome + Message (no SendResult, no Decision).
//
// Timestamp policy (pure — no DB read needed):
//
//	AcceptanceFinal / AcceptancePendingDLR → SentAt = now
//	AcceptanceRejected (retryable+budget) → no timestamps (retrying)
//	AcceptanceRejected (retryable+exhausted) → FailedAt = now, SentAt = now
//	AcceptanceRejected (non-retryable)     → FailedAt = now, SentAt = now
type HandleResultStage struct{}

func NewHandleResultStage() *HandleResultStage {
	return &HandleResultStage{}
}

func (s *HandleResultStage) Name() string {
	return "handle_result"
}

var ErrNoSendResult = fmt.Errorf("handle result stage: no send result to interpret")

// Process interprets SendResult via AcceptanceKind and produces DeliveryOutcome.
// All SendResult and Decision fields are copied into DeliveryOutcome for downstream.
func (s *HandleResultStage) Process(ctx context.Context, state *PipelineState) (*PipelineState, error) {
	if state.SendResult == nil {
		return nil, ErrNoSendResult
	}

	sr := state.SendResult
	now := time.Now().UTC()

	outcome := NewDeliveryOutcome(
		domain.MessageStatusQueued, // will be overridden below
		FailureNone,
		false,
		"",
	)

	// Copy SendResult fields into DeliveryOutcome (downstream needs these).
	outcome.ExternalID = sr.ExternalID
	outcome.Parts = sr.Parts
	outcome.ErrorCode = sr.ErrorCode
	outcome.ErrorMessage = sr.ErrorMessage
	outcome.Retryable = sr.Retryable

	// Copy Decision fields.
	if state.Decision != nil {
		outcome.ConnectorID = state.Decision.ConnectorID
		outcome.RouteID = state.Decision.RouteID
	}

	// Interpret AcceptanceKind.
	switch sr.Acceptance {
	case domain.AcceptanceFinal:
		outcome.Status = domain.MessageStatusSent
		outcome.FailureKind = FailureNone
		outcome.AwaitingDLR = false
		outcome.Reason = fmt.Sprintf("accepted by provider, id=%s", sr.ExternalID)
		outcome.SentAt = &now

	case domain.AcceptancePendingDLR:
		outcome.Status = domain.MessageStatusSent
		outcome.FailureKind = FailureNone
		outcome.AwaitingDLR = true
		outcome.Reason = "accepted, awaiting delivery receipt"
		outcome.SentAt = &now

	case domain.AcceptanceRejected:
		if sr.Retryable && state.Message.RetryCount < state.Message.MaxRetries {
			outcome.Status = domain.MessageStatusRetrying
			outcome.FailureKind = FailureTemporary
			outcome.Reason = fmt.Sprintf("retryable rejection: %s", sr.ErrorCode)
			// No timestamps — message wasn't sent.
		} else {
			outcome.Status = domain.MessageStatusFailed
			outcome.FailureKind = FailurePermanent
			outcome.Reason = fmt.Sprintf("rejected: %s", sr.ErrorCode)
			outcome.FailedAt = &now
			outcome.SentAt = &now // retroactive if never sent
		}

	default:
		outcome.Status = domain.MessageStatusFailed
		outcome.FailureKind = FailureInternal
		outcome.Reason = fmt.Sprintf("unknown acceptance kind: %s", sr.Acceptance)
		outcome.FailedAt = &now
		outcome.SentAt = &now
	}

	state.DeliveryOutcome = &outcome
	return state, nil
}
