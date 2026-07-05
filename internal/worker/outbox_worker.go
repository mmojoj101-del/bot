package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/raghna/fury-sms-gateway/internal/domain"
	"github.com/raghna/fury-sms-gateway/internal/event"
)

// OutboxWorker reads outbox_events in batches and publishes them to the event bus.
type OutboxWorker struct {
	outboxRepo   domain.OutboxRepository
	eventBus     event.Bus
	batchSize    int
	pollInterval time.Duration
	stopCh       chan struct{}
}

func NewOutboxWorker(
	outboxRepo domain.OutboxRepository,
	eventBus event.Bus,
	opts ...OutboxWorkerOption,
) *OutboxWorker {
	w := &OutboxWorker{
		outboxRepo:   outboxRepo,
		eventBus:     eventBus,
		batchSize:    100,
		pollInterval: 500 * time.Millisecond,
		stopCh:       make(chan struct{}),
	}
	for _, opt := range opts {
		opt(w)
	}
	return w
}

type OutboxWorkerOption func(*OutboxWorker)

func OutboxWorkerWithBatchSize(size int) OutboxWorkerOption {
	return func(w *OutboxWorker) { w.batchSize = size }
}

func OutboxWorkerWithPollInterval(d time.Duration) OutboxWorkerOption {
	return func(w *OutboxWorker) { w.pollInterval = d }
}

func (w *OutboxWorker) Start() {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("outbox worker panic recovered", "panic", r)
			}
		}()
		w.loop()
	}()
	slog.Info("outbox worker started", "batch_size", w.batchSize, "poll_interval", w.pollInterval)
}

func (w *OutboxWorker) Stop() {
	close(w.stopCh)
}

func (w *OutboxWorker) IsHealthy() error {
	select {
	case <-w.stopCh:
		return fmt.Errorf("outbox worker stopped")
	default:
		return nil
	}
}

func (w *OutboxWorker) loop() {
	for {
		select {
		case <-w.stopCh:
			slog.Info("outbox worker stopped")
			return
		default:
			w.processBatch()
			time.Sleep(w.pollInterval)
		}
	}
}

func (w *OutboxWorker) processBatch() {
	ctx := context.Background()

	events, err := w.outboxRepo.GetPending(ctx, w.batchSize)
	if err != nil {
		slog.Error("get pending outbox events", "error", err)
		return
	}

	if len(events) == 0 {
		return
	}

		// Publish then mark each event individually.
	// If marking fails after a crash, the event will be re-published
	// on restart (at-least-once delivery). Subscribers must be idempotent.
	published := 0
	for _, evt := range events {
		payload, err := unmarshalPayload(evt.Payload)
		if err != nil {
			slog.Error("unmarshal outbox payload", "event_id", evt.ID, "error", err)
			if err := w.outboxRepo.MarkFailed(ctx, evt.ID, err.Error()); err != nil {
				slog.Error("mark outbox event failed", "event_id", evt.ID, "error", err)
			}
			continue
		}

		w.eventBus.Publish(event.Event{
			ID:        evt.ID,
			Type:      evt.EventType,
			Payload:   payload,
			Timestamp: time.Now().UTC(),
		})

		// Mark immediately so failed event doesn't block the rest
		if err := w.outboxRepo.MarkPublished(ctx, evt.ID); err != nil {
			// Logged but don't stop — event will be re-published next cycle
			slog.Error("mark outbox event published", "event_id", evt.ID, "error", err)
		}
		published++
	}

	if published > 0 {
		slog.Debug("outbox batch published", "count", published)
	}
}

func unmarshalPayload(data []byte) (interface{}, error) {
	var payload interface{}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, err
	}
	return payload, nil
}
