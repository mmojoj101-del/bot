package pipeline

import (
	"context"
	"fmt"

	"github.com/raghna/fury-sms-gateway/internal/domain"
)

// HandleResultStage interprets the connector's SendResult and produces a
// DeliveryOutcome — the single business interpretation artifact.
//
// IMPORTANT: "Accepted by provider" ≠ "Delivered to device".
// HandleResultStage maps AcceptanceFinal to Sent, not Delivered.
// DLR callbacks (external) are the only mechanism that transitions
// Sent → Delivered or Sent → Failed.
//
// It is pure logic: no DB access, no event publishing, no external calls.
type HandleResultStage struct{}

func NewHandleResultStage() *HandleResultStage {
	return &HandleResultStage{}
}

func (s *HandleResultStage) Name() string {
	return "handle_result"
}

var ErrNoSendResult = fmt.Errorf("handle result stage: no send result to interpret")

// Process produces a DeliveryOutcome by interpreting SendResult.
//
// Decision matrix:
//
//	AcceptanceKind  │ Terminal │ Outcome
//	────────────────┼──────────┼────────────────────────────
//	final           │ true     │ Sent (no DLR expected)
//	pending_dlr     │ false    │ Sent (DLR expected)
//	rejected + retry│ false    │ Retrying (Temporary)
//	rejected + done │ true     │ Failed (Permanent)
func (s *HandleResultStage) Process(ctx context.Context, state *PipelineState) (*PipelineState, error) {
	if state.SendResult == nil {
		return nil, ErrNoSendResult
	}

	sr := state.SendResult

	switch sr.Acceptance {
	case domain.AcceptanceFinal:
		// Provider accepted the message. No DLR expected.
		// This is "Sent" (not "Delivered") — delivery confirmation
		// comes only via DLR callbacks.
		state.DeliveryOutcome = &DeliveryOutcome{
			Status:   domain.MessageStatusSent,
			Terminal: true, // no DLR expected
			Reason:   fmt.Sprintf("accepted by provider, id=%s", sr.ExternalID),
		}

	case domain.AcceptancePendingDLR:
		// Provider accepted and will send a delivery receipt.
		state.DeliveryOutcome = &DeliveryOutcome{
			Status:   domain.MessageStatusSent,
			Terminal: false, // DLR expected
			Reason:   "accepted, awaiting delivery receipt",
		}

	case domain.AcceptanceRejected:
		// Provider rejected the message. Retry if budget allows.
		if sr.Retryable && state.Message.RetryCount < state.Message.MaxRetries {
			state.DeliveryOutcome = &DeliveryOutcome{
				Status:      domain.MessageStatusRetrying,
				FailureKind: FailureTemporary,
				Terminal:    false,
				Reason:      fmt.Sprintf("retryable rejection: %s", sr.ErrorCode),
			}
		} else {
			fk := FailurePermanent
			if !sr.Retryable {
				fk = FailurePermanent
			}
			state.DeliveryOutcome = &DeliveryOutcome{
				Status:      domain.MessageStatusFailed,
				FailureKind: fk,
				Terminal:    true,
				Reason:      fmt.Sprintf("rejected: %s", sr.ErrorCode),
			}
		}

	default:
		state.DeliveryOutcome = &DeliveryOutcome{
			Status:      domain.MessageStatusFailed,
			FailureKind: FailureInternal,
			Terminal:    true,
			Reason:      fmt.Sprintf("unknown acceptance kind: %s", sr.Acceptance),
		}
	}

	return state, nil
}
