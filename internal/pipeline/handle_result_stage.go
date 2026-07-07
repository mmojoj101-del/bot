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
// IMPORTANT: HandleResultStage does NOT decide retry timing or policy.
// It classifies the outcome (Status + FailureKind) and signals whether
// retry is appropriate via Status=Retrying. A separate RetryPolicy or
// RetryDecorator handles scheduling (backoff, jitter, cap).
//
// Input:  PipelineState.SendResult + PipelineState.Message (read-only)
// Output: PipelineState.DeliveryOutcome (new artifact, appended after SendResult)
//
// NEVER reads or modifies: Prepared, Decision, or any prior artifact.
// NEVER inspects: DB state, event bus, retry counters from external sources.
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

// Process produces a DeliveryOutcome by interpreting SendResult contextually.
//
// Decision matrix:
//
//	SendResult.Success │ ErrorCode  │ ExternalID     │ Outcome
//	───────────────────┼────────────┼────────────────┼─────────────────────────────
//	true               │ ""         │ non-empty      │ Delivered, FailureNone
//	true               │ ""         │ "" + DLR       │ Sent, FailureNone
//	true               │ ""         │ "" − DLR       │ Delivered, FailureNone
//	false              │ non-empty  │ any            │ Retrying† or Failed
//
//	† Retrying only when Retryable=true AND RetryCount < MaxRetries.
//	  Otherwise Failed.
func (s *HandleResultStage) Process(ctx context.Context, state *PipelineState) (*PipelineState, error) {
	if state.SendResult == nil {
		return nil, ErrNoSendResult
	}

	sr := state.SendResult
	var outcome DeliveryOutcome

	// ── Provider-level failure (transport succeeded, provider returned error) ──
	if !sr.Success && sr.ErrorCode != "" {
		if sr.Retryable && state.Message.RetryCount < state.Message.MaxRetries {
			outcome = DeliveryOutcome{
				Status:      domain.MessageStatusRetrying,
				FailureKind: FailureProvider,
				Reason:      fmt.Sprintf("retryable provider error: %s", sr.ErrorCode),
			}
		} else if !sr.Retryable {
			outcome = DeliveryOutcome{
				Status:      domain.MessageStatusFailed,
				FailureKind: FailureRejected,
				Reason:      fmt.Sprintf("non-retryable provider error: %s", sr.ErrorCode),
			}
		} else {
			outcome = DeliveryOutcome{
				Status:      domain.MessageStatusFailed,
				FailureKind: FailureProvider,
				Reason:      fmt.Sprintf("retries exhausted: %s", sr.ErrorCode),
			}
		}
		state.DeliveryOutcome = &outcome
		return state, nil
	}

	// ── Transport + provider success ──
	switch {
	case sr.ExternalID != "":
		outcome = DeliveryOutcome{
			Status: domain.MessageStatusDelivered,
			Reason: fmt.Sprintf("delivered, id=%s", sr.ExternalID),
		}
	case sr.RequestsDLR:
		outcome = DeliveryOutcome{
			Status:      domain.MessageStatusSent,
			FailureKind: FailureNone,
			Reason:      "sent, awaiting delivery receipt",
		}
	default:
		outcome = DeliveryOutcome{
			Status: domain.MessageStatusDelivered,
			Reason: "accepted by provider",
		}
	}

	state.DeliveryOutcome = &outcome
	return state, nil
}
