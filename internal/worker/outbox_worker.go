package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
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

	stopCh chan struct{}
	wg     sync.WaitGroup

	running atomic.Bool
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
	w.running.Store(true)
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		defer w.running.Store(false)
		for {
			if !w.iteration() {
				return
			}
		}
	}()
	slog.Info("outbox worker started", "batch_size", w.batchSize, "poll_interval", w.pollInterval)
}

func (w *OutboxWorker) iteration() bool {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("outbox worker panic recovered, restarting...", "panic", r)
			time.Sleep(time.Second)
		}
	}()

	select {
	case <-w.stopCh:
		slog.Info("outbox worker stopped")
		return false
	default:
		w.processBatch()
		time.Sleep(w.pollInterval)
		return true
	}
}

func (w *OutboxWorker) Stop() {
	close(w.stopCh)
	w.wg.Wait()
}

func (w *OutboxWorker) IsHealthy() error {
	if !w.running.Load() {
		return fmt.Errorf("outbox worker goroutine is not running")
	}
	select {
	case <-w.stopCh:
		return fmt.Errorf("outbox worker is stopped")
	default:
		return nil
	}
}

func (w *OutboxWorker) processBatch() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	events, err := w.outboxRepo.GetPending(ctx, w.batchSize)
	if err != nil {
		slog.Error("get pending outbox events", "error", err)
		return
	}

	if len(events) == 0 {
		return
	}

	// Publish then mark each event individually.
	// At-least-once delivery: if MarkPublished fails after publish,
	// the event will be re-published on restart. Subscribers must be idempotent.
	published := 0
	for _, evt := range events {
		select {
		case <-w.stopCh:
			return
		default:
		}

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

		if err := w.outboxRepo.MarkPublished(ctx, evt.ID); err != nil {
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
