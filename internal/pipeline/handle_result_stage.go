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
// BuildEventsStage later builds domain events from DeliveryOutcome.
// PersistStage later copies DeliveryOutcome (including timestamps) to the DB.
//
// Timestamp policy (pure — no DB read needed):
//
//	AcceptanceFinal / AcceptancePendingDLR → SentAt = now
//	AcceptanceRejected (retryable)         → no timestamps (retrying)
//	AcceptanceRejected (exhausted)         → FailedAt = now, SentAt = now
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
func (s *HandleResultStage) Process(ctx context.Context, state *PipelineState) (*PipelineState, error) {
	if state.SendResult == nil {
		return nil, ErrNoSendResult
	}

	sr := state.SendResult
	now := time.Now().UTC()

	var outcome DeliveryOutcome

	switch sr.Acceptance {
	case domain.AcceptanceFinal:
		outcome = NewDeliveryOutcome(
			domain.MessageStatusSent, FailureNone, false,
			fmt.Sprintf("accepted by provider, id=%s", sr.ExternalID),
		)
		outcome.SentAt = &now

	case domain.AcceptancePendingDLR:
		outcome = NewDeliveryOutcome(
			domain.MessageStatusSent, FailureNone, true,
			"accepted, awaiting delivery receipt",
		)
		outcome.SentAt = &now

	case domain.AcceptanceRejected:
		if sr.Retryable && state.Message.RetryCount < state.Message.MaxRetries {
			outcome = NewDeliveryOutcome(
				domain.MessageStatusRetrying, FailureTemporary, false,
				fmt.Sprintf("retryable rejection: %s", sr.ErrorCode),
			)
			// No timestamps — message wasn't sent.
		} else {
			outcome = NewDeliveryOutcome(
				domain.MessageStatusFailed, FailurePermanent, false,
				fmt.Sprintf("rejected: %s", sr.ErrorCode),
			)
			outcome.FailedAt = &now
			outcome.SentAt = &now // retroactive if never sent
		}

	default:
		outcome = NewDeliveryOutcome(
			domain.MessageStatusFailed, FailureInternal, false,
			fmt.Sprintf("unknown acceptance kind: %s", sr.Acceptance),
		)
		outcome.FailedAt = &now
		outcome.SentAt = &now
	}

	state.DeliveryOutcome = &outcome
	return state, nil
}
