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
// HandleResultStage maps both AcceptanceFinal and AcceptancePendingDLR
// to Sent. DLR callbacks (external) are the only mechanism that
// transitions Sent → Delivered or Sent → Failed.
//
// It is pure logic: no DB, no event bus, no external calls.
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
//	AcceptanceKind  │ AwaitingDLR │ Outcome
//	────────────────┼─────────────┼─────────────────────────────
//	final           │ false       │ Sent (terminal, no DLR)
//	pending_dlr     │ true        │ Sent (non-terminal, DLR)
//	rejected+retry  │ false       │ Retrying (non-terminal)
//	rejected+done   │ false       │ Failed (terminal)
func (s *HandleResultStage) Process(ctx context.Context, state *PipelineState) (*PipelineState, error) {
	if state.SendResult == nil {
		return nil, ErrNoSendResult
	}

	sr := state.SendResult

	switch sr.Acceptance {
	case domain.AcceptanceFinal:
		// Provider accepted the message. No DLR expected.
		outcome := NewDeliveryOutcome(
			domain.MessageStatusSent,
			FailureNone,
			false, // AwaitingDLR: no DLR expected
			fmt.Sprintf("accepted by provider, id=%s", sr.ExternalID),
		)
		state.DeliveryOutcome = &outcome

	case domain.AcceptancePendingDLR:
		// Provider accepted and will send a delivery receipt.
		outcome := NewDeliveryOutcome(
			domain.MessageStatusSent,
			FailureNone,
			true, // AwaitingDLR: DLR expected
			"accepted, awaiting delivery receipt",
		)
		state.DeliveryOutcome = &outcome

	case domain.AcceptanceRejected:
		if sr.Retryable && state.Message.RetryCount < state.Message.MaxRetries {
			outcome := NewDeliveryOutcome(
				domain.MessageStatusRetrying,
				FailureTemporary,
				false,
				fmt.Sprintf("retryable rejection: %s", sr.ErrorCode),
			)
			state.DeliveryOutcome = &outcome
		} else {
			outcome := NewDeliveryOutcome(
				domain.MessageStatusFailed,
				FailurePermanent,
				false,
				fmt.Sprintf("rejected: %s", sr.ErrorCode),
			)
			state.DeliveryOutcome = &outcome
		}

	default:
		outcome := NewDeliveryOutcome(
			domain.MessageStatusFailed,
			FailureInternal,
			false,
			fmt.Sprintf("unknown acceptance kind: %s", sr.Acceptance),
		)
		state.DeliveryOutcome = &outcome
	}

	return state, nil
}
