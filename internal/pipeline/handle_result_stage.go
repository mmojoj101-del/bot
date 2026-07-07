package pipeline

import (
	"context"
	"fmt"
	"time"

	"github.com/raghna/fury-sms-gateway/internal/domain"
)

// HandleResultStage interprets the connector's SendResult and produces a
// DeliveryOutcome — the single business decision artifact.
//
// It is pure logic: no DB access, no event publishing, no external calls.
// Its only job is to answer: "What does this SendResult mean for the message?"
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
//	───────────────────┼────────────┼────────────────┼──────────────────────────────
//	false              │ any        │ any            │ Failed (non-retryable)
//	true               │ ""         │ non-empty      │ Delivered (terminal)
//	true               │ ""         │ "" + DLR       │ Sent (pending DLR)
//	true               │ ""         │ "" + no DLR    │ Delivered (accepted w/o ID)
//	true               │ non-empty  │ any            │ Depends on Retryable + RetryCount
func (s *HandleResultStage) Process(ctx context.Context, state *PipelineState) (*PipelineState, error) {
	if state.SendResult == nil {
		return nil, ErrNoSendResult
	}

	sr := state.SendResult
	outcome := DeliveryOutcome{}

	// ── Transport-level failure (sender returned error, SendResult not set) ──
	// This case is handled by SendStage returning an error — HandleResultStage
	// never sees it. If we reach here, sender.Send() succeeded.

	// ── Provider-level rejection (HTTP 200, but provider returned error code) ──
	if sr.ErrorCode != "" && !sr.Success {
		if sr.Retryable && state.Message.RetryCount < state.Message.MaxRetries {
			outcome = DeliveryOutcome{
				Status:     domain.MessageStatusRetrying,
				Retry:      true,
				RetryAfter: backoffForAttempt(state.Message.RetryCount),
				Reason:     fmt.Sprintf("retryable provider error: %s", sr.ErrorCode),
				Terminal:   false,
			}
		} else {
			outcome = DeliveryOutcome{
				Status:   domain.MessageStatusFailed,
				Retry:    false,
				Reason:   fmt.Sprintf("provider error: %s", sr.ErrorCode),
				Terminal: true,
			}
		}
		state.DeliveryOutcome = &outcome
		return state, nil
	}

	// ── Transport + provider success ──
	// sr.Success == true (set by SendStage when sender.Send() returned nil)
	if sr.ExternalID != "" {
		// Provider returned a message ID — message was accepted.
		outcome = DeliveryOutcome{
			Status:   domain.MessageStatusDelivered,
			Retry:    false,
			Reason:   fmt.Sprintf("delivered to provider, id=%s", sr.ExternalID),
			Terminal: true,
		}
	} else if sr.RequestsDLR {
		// Provider accepted without immediate ID, DLR expected.
		outcome = DeliveryOutcome{
			Status:   domain.MessageStatusSent,
			Retry:    false,
			Reason:   "sent, awaiting delivery receipt",
			Terminal: false, // DLR may update status
		}
	} else {
		// Provider accepted without ID and no DLR — consider delivered.
		outcome = DeliveryOutcome{
			Status:   domain.MessageStatusDelivered,
			Retry:    false,
			Reason:   "accepted by provider",
			Terminal: true,
		}
	}

	state.DeliveryOutcome = &outcome
	return state, nil
}

// backoffForAttempt returns the delay before the next retry.
// Uses exponential backoff: 2^attempt seconds, capped at 300s (5 min).
func backoffForAttempt(attempt int) time.Duration {
	base := 1 << uint(attempt) // 1, 2, 4, 8, 16, 32, 64, 128, 256
	seconds := base
	if seconds > 300 {
		seconds = 300
	}
	return time.Duration(seconds) * time.Second
}
