package worker

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/raghna/fury-sms-gateway/internal/domain"
	"github.com/raghna/fury-sms-gateway/internal/domain/events"
	"github.com/raghna/fury-sms-gateway/internal/pipeline"
)

// WorkerExecutor orchestrates the message lifecycle:
//   1. Claim a message from the queue
//   2. Create PipelineState
//   3. Execute the pipeline
//   4. Publish domain events
//
// It has no knowledge of HTTP, SMPP, SIP, Routing, or Retry logic.
// Those are handled by pipeline stages and event subscribers.
type WorkerExecutor struct {
	queueRepo domain.QueueRepository
	pipeline  *pipeline.Pipeline
	publisher events.DomainEventPublisher
}

// NewWorkerExecutor creates a new WorkerExecutor.
func NewWorkerExecutor(
	queueRepo domain.QueueRepository,
	pipeline *pipeline.Pipeline,
	publisher events.DomainEventPublisher,
) *WorkerExecutor {
	return &WorkerExecutor{
		queueRepo: queueRepo,
		pipeline:  pipeline,
		publisher: publisher,
	}
}

// ExecuteOne claims and processes a single message from the queue.
// Returns nil if no message was available (empty queue).
func (e *WorkerExecutor) ExecuteOne(ctx context.Context) error {
	// 1. Claim message from queue (limit=1 for single execution)
	messages, err := e.queueRepo.ClaimQueued(ctx, 1)
	if err != nil {
		return fmt.Errorf("claim message: %w", err)
	}
	if len(messages) == 0 {
		return nil // no messages in queue
	}
	msg := messages[0]

	// 2. Publish MessageClaimed event
	e.publishEvent(ctx, &msg, events.EventTypeMessageClaimedV1, events.MessageClaimedV1Payload{
		MessageID: msg.ID,
		TenantID:  msg.TenantID,
		WorkerID:  "executor",
	})

	// 3. Create PipelineState
	state := pipeline.NewPipelineState(&msg, msg.ID)

	// 4. Execute the pipeline
	if err := e.pipeline.Execute(ctx, state); err != nil {
		failedPayload := events.MessageFailedV1Payload{
			MessageID:    msg.ID,
			ErrorCode:    "pipeline_error",
			ErrorMessage: err.Error(),
			Attempt:      1,
			Retryable:    true,
		}
		e.publishEvent(ctx, &msg, events.EventTypeMessageFailedV1, failedPayload)

		// Ack as failed so it's not stuck in 'claimed' status
		if ackErr := e.queueRepo.AckFailed(ctx, msg.ID, msg.Version, "pipeline_error", err.Error()); ackErr != nil {
			return fmt.Errorf("ack failed after pipeline error: %w (original: %v)", ackErr, err)
		}
		return fmt.Errorf("pipeline: %w", err)
	}

	// 5. Pipeline succeeded — ack as sent (or delivered if no DLR expected)
	if state.SendResult != nil && state.SendResult.Success {
		return e.queueRepo.AckSent(ctx, msg.ID, msg.Version, state.SendResult.ExternalID, state.SendResult.Parts, 0, 0)
	}

	// No send result — ack without external ID
	return e.queueRepo.AckSent(ctx, msg.ID, msg.Version, "", 1, 0, 0)
}

// publishEvent is a helper to publish domain events.
func (e *WorkerExecutor) publishEvent(ctx context.Context, msg *domain.Message, eventType string, payload interface{}) {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return
	}

	envelope := events.EventEnvelope{
		EventType:     eventType,
		TraceID:       msg.ID,
		TenantID:      msg.TenantID,
		CorrelationID: msg.ID,
		Payload:       payloadBytes,
	}
	_ = e.publisher.Publish(ctx, envelope)
}
