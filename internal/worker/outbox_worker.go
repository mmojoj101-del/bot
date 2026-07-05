package worker

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/raghna/fury-sms-gateway/internal/domain"
	"github.com/raghna/fury-sms-gateway/internal/event"
)

// OutboxWorker reads outbox_events and publishes them to the event bus.
type OutboxWorker struct {
	outboxRepo domain.OutboxRepository
	eventBus   event.Bus
	batchSize  int
	pollInterval time.Duration
	stopCh     chan struct{}
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
	go w.loop()
	slog.Info("outbox worker started", "batch_size", w.batchSize, "poll_interval", w.pollInterval)
}

func (w *OutboxWorker) Stop() {
	close(w.stopCh)
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

	for _, evt := range events {
		w.publishEvent(ctx, evt)
	}
}

func (w *OutboxWorker) publishEvent(ctx context.Context, evt domain.OutboxEvent) {
	// Parse the payload
	var payload interface{}
	if err := json.Unmarshal(evt.Payload, &payload); err != nil {
		slog.Error("unmarshal outbox payload", "event_id", evt.ID, "error", err)
		if err := w.outboxRepo.MarkFailed(ctx, evt.ID, err.Error()); err != nil {
			slog.Error("mark outbox event failed", "event_id", evt.ID, "error", err)
		}
		return
	}

	// Publish to event bus
	w.eventBus.Publish(event.Event{
		ID:        evt.ID,
		Type:      evt.EventType,
		Payload:   payload,
		Timestamp: time.Now().UTC(),
	})

	// Mark as published
	if err := w.outboxRepo.MarkPublished(ctx, evt.ID); err != nil {
		slog.Error("mark outbox event published", "event_id", evt.ID, "error", err)
		return
	}
}
