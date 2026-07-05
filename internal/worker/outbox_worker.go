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

	ctx     context.Context
	cancel  context.CancelFunc
	stopCh  chan struct{}
	stopOnce sync.Once
	wg      sync.WaitGroup

	running    atomic.Bool
	restartCnt atomic.Int64
}

func NewOutboxWorker(
	outboxRepo domain.OutboxRepository,
	eventBus event.Bus,
	opts ...OutboxWorkerOption,
) *OutboxWorker {
	ctx, cancel := context.WithCancel(context.Background())
	w := &OutboxWorker{
		outboxRepo:   outboxRepo,
		eventBus:     eventBus,
		batchSize:    100,
		pollInterval: 500 * time.Millisecond,
		stopCh:       make(chan struct{}),
		cancel:       cancel,
		ctx:          ctx,
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
		backoff := 100 * time.Millisecond
		for {
			if w.iteration() {
				backoff = 100 * time.Millisecond
				continue
			}
			select {
			case <-w.stopCh:
				return
			default:
				slog.Warn("outbox worker restarting",
					"restart_count", w.restartCnt.Load(),
					"backoff_ms", backoff.Milliseconds(),
				)
				time.Sleep(backoff)
				if backoff < 30*time.Second {
					backoff *= 2
				}
			}
		}
	}()
	slog.Info("outbox worker started", "batch_size", w.batchSize, "poll_interval", w.pollInterval)
}

func (w *OutboxWorker) iteration() (ok bool) {
	defer func() {
		if r := recover(); r != nil {
			w.restartCnt.Add(1)
			slog.Error("outbox worker panic recovered",
				"panic", r,
				"restart_count", w.restartCnt.Load(),
			)
			ok = false
		}
	}()
	select {
	case <-w.stopCh:
		return false
	default:
		w.processBatch()
		time.Sleep(w.pollInterval)
		return true
	}
}

func (w *OutboxWorker) Stop() {
	w.stopOnce.Do(func() {
		w.cancel()
		close(w.stopCh)
	})
	w.wg.Wait()
}

func (w *OutboxWorker) IsHealthy() error {
	if !w.running.Load() {
		return fmt.Errorf("outbox worker is not running")
	}
	select {
	case <-w.stopCh:
		return fmt.Errorf("outbox worker is stopped")
	default:
		return nil
	}
}

func (w *OutboxWorker) HealthDetail() map[string]interface{} {
	return map[string]interface{}{
		"alive":         w.running.Load(),
		"stopped":       isClosed(w.stopCh),
		"restart_count": w.restartCnt.Load(),
		"batch_size":    w.batchSize,
		"poll_interval": w.pollInterval.String(),
		"type":          "outbox_worker",
	}
}

func (w *OutboxWorker) processBatch() {
	ctx, cancel := context.WithTimeout(w.ctx, 10*time.Second)
	defer cancel()

	events, err := w.outboxRepo.GetPending(ctx, w.batchSize)
	if err != nil {
		slog.Error("get pending outbox events", "error", err)
		return
	}
	if len(events) == 0 {
		return
	}

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
