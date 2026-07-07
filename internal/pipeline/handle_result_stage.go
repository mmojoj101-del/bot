package pipeline

import (
	"context"
	"fmt"

	"github.com/raghna/fury-sms-gateway/internal/domain"
)

// HandleResultStage interprets the connector's SendResult and produces a
// DeliveryOutcome — the single business interpretation artifact.
//
// It is pure logic: no DB access, no event publishing, no external calls.
// Its only job is to answer: "What does this SendResult mean for the message?"
//
// IMPORTANT:
//   - HandleResultStage does NOT decide retry timing (that is a RetryPolicy).
//   - HandleResultStage does NOT guess DLR semantics (Acceptance comes from connector).
//
// Input:  PipelineState.SendResult (with Acceptance from connector)
// Output: PipelineState.DeliveryOutcome
//
// NEVER reads or modifies: Prepared, Decision, or any prior artifact.
type HandleResultStage struct{}

// NewHandleResultStage creates a HandleResultStage.
func NewHandleResultStage() *HandleResultStage {
	return &HandleResultStage{}
}

// Name returns the stage name for logging and metrics.
func (s *HandleResultStage) Name() string {
	return "handle_result"
}

var ErrNoSendResult = fmt.Errorf("handle result stage: no send result to interpret")

// Process produces a DeliveryOutcome by interpreting SendResult.
//
// Decision matrix (AcceptanceKind drives business interpretation):
//
//	Acceptance    │ Success │ Retryable+Budget │ Outcome
//	──────────────┼─────────┼─────────────────┼────────────────────────────
//	final         │ true    │ —               │ Delivered, FailureNone
//	pending_dlr   │ true    │ —               │ Sent, FailureNone
//	rejected      │ false   │ yes             │ Retrying, FailureTemporary
//	rejected      │ false   │ no              │ Failed, FailurePermanent
func (s *HandleResultStage) Process(ctx context.Context, state *PipelineState) (*PipelineState, error) {
	if state.SendResult == nil {
		return nil, ErrNoSendResult
	}

	sr := state.SendResult
	var outcome DeliveryOutcome

	switch sr.Acceptance {
	case domain.AcceptanceFinal:
		outcome = DeliveryOutcome{
			Status: domain.MessageStatusDelivered,
			Reason: fmt.Sprintf("delivered, id=%s", sr.ExternalID),
		}

	case domain.AcceptancePendingDLR:
		outcome = DeliveryOutcome{
			Status:      domain.MessageStatusSent,
			FailureKind: FailureNone,
			Reason:      "accepted, awaiting delivery receipt",
		}

	case domain.AcceptanceRejected:
		// Provider rejected the message. Retry if budget permits and error is retryable.
		if sr.Retryable && state.Message.RetryCount < state.Message.MaxRetries {
			outcome = DeliveryOutcome{
				Status:      domain.MessageStatusRetrying,
				FailureKind: FailureTemporary,
				Reason:      fmt.Sprintf("retryable rejection: %s", sr.ErrorCode),
			}
		} else {
			fk := FailurePermanent
			if sr.Retryable {
				fk = FailurePermanent // retries exhausted
			} else if !sr.Retryable {
				fk = FailurePermanent // non-retryable from provider
			}
			outcome = DeliveryOutcome{
				Status:      domain.MessageStatusFailed,
				FailureKind: fk,
				Reason:      fmt.Sprintf("rejected: %s", sr.ErrorCode),
			}
		}

	default:
		// Fallback: unknown AcceptanceKind — treat as failed.
		outcome = DeliveryOutcome{
			Status:      domain.MessageStatusFailed,
			FailureKind: FailureInternal,
			Reason:      fmt.Sprintf("unknown acceptance kind: %s", sr.Acceptance),
		}
	}

	state.DeliveryOutcome = &outcome
	return state, nil
}
