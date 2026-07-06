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
	onRestart    func(RestartEvent)

	ctx     context.Context
	cancel  context.CancelFunc
	stopCh  chan struct{}
	stopOnce sync.Once
	wg      sync.WaitGroup

	running    atomic.Bool
	restartCnt atomic.Int64

	lastPanicTime    atomic.Value // time.Time
	lastSuccessTime  atomic.Value // time.Time
	lastBatchEndTime atomic.Value // time.Time
	lastBatchMsgCnt  atomic.Int64
	lastBatchDur     atomic.Int64
}

func NewOutboxWorker(
	parentCtx context.Context,
	outboxRepo domain.OutboxRepository,
	eventBus event.Bus,
	opts ...OutboxWorkerOption,
) *OutboxWorker {
	ctx, cancel := context.WithCancel(parentCtx)
	w := &OutboxWorker{
		outboxRepo:   outboxRepo,
		eventBus:     eventBus,
		batchSize:    100,
		pollInterval: 500 * time.Millisecond,
		stopCh:       make(chan struct{}),
		cancel:       cancel,
		ctx:          ctx,
		onRestart:    func(RestartEvent) {},
	}
	w.lastPanicTime.Store(time.Time{})
	w.lastSuccessTime.Store(time.Time{})
	w.lastBatchEndTime.Store(time.Time{})
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

func OutboxWorkerWithRestartCallback(fn func(RestartEvent)) OutboxWorkerOption {
	return func(w *OutboxWorker) { w.onRestart = fn }
}

func (w *OutboxWorker) Start() {
	w.running.Store(true)
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		defer w.running.Store(false)
		backoff := 100 * time.Millisecond
		consecutiveFailures := 0
		for {
			if w.iteration() {
				backoff = 100 * time.Millisecond
				consecutiveFailures = 0
				w.lastPanicTime.Store(time.Time{})
				continue
			}
			select {
			case <-w.stopCh:
				return
			default:
				consecutiveFailures++
				critical := consecutiveFailures >= MaxConsecutiveRestarts
				level := slog.LevelWarn
				if critical {
					level = slog.LevelError
				}
				slog.Log(w.ctx, level,
					"outbox worker restarting",
					"restart_count", w.restartCnt.Load(),
					"consecutive_failures", consecutiveFailures,
					"backoff_ms", backoff.Milliseconds(),
				)
				w.onRestart(RestartEvent{
					WorkerType:           "outbox_worker",
					RestartCount:        w.restartCnt.Load(),
					ConsecutiveFailures: consecutiveFailures,
					Backoff:             backoff,
					Critical:            critical,
				})
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
			w.lastPanicTime.Store(time.Now())
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
		close(w.stopCh)
	})

	done := make(chan struct{})
	go func() {
		w.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(shutdownTimeout):
		slog.Warn("outbox worker shutdown timed out, forcing cancel",
			"timeout", shutdownTimeout,
			"restart_count", w.restartCnt.Load(),
		)
	}

	w.cancel()
}

func (w *OutboxWorker) IsHealthy() error {
	if !w.running.Load() {
		return fmt.Errorf("outbox worker is not running")
	}
	select {
	case <-w.stopCh:
		return fmt.Errorf("outbox worker is stopped")
	default:
	}
	if v, ok := w.lastBatchEndTime.Load().(time.Time); ok && !v.IsZero() {
		if w.lastBatchMsgCnt.Load() > 0 && time.Since(v) > healthyIdleThreshold {
			return fmt.Errorf("outbox worker idle for %v with work (threshold %v)",
				time.Since(v).Round(time.Second), healthyIdleThreshold)
		}
	}
	return nil
}

func (w *OutboxWorker) HealthDetail() map[string]interface{} {
	var lastPanic, lastSuccess, lastBatchEnd time.Time
	if v, ok := w.lastPanicTime.Load().(time.Time); ok {
		lastPanic = v
	}
	if v, ok := w.lastSuccessTime.Load().(time.Time); ok {
		lastSuccess = v
	}
	if v, ok := w.lastBatchEndTime.Load().(time.Time); ok {
		lastBatchEnd = v
	}
	return map[string]interface{}{
		"type":               "outbox_worker",
		"alive":              w.running.Load(),
		"stopped":            isClosed(w.stopCh),
		"restart_count":      w.restartCnt.Load(),
		"batch_size":         w.batchSize,
		"poll_interval":      w.pollInterval.String(),
		"last_panic_at":      nullTime(lastPanic),
		"last_success_at":    nullTime(lastSuccess),
		"last_batch_at":      nullTime(lastBatchEnd),
		"last_batch_msgs":    w.lastBatchMsgCnt.Load(),
		"last_batch_duration": durationMillis(w.lastBatchDur.Load()),
	}
}

func (w *OutboxWorker) processBatch() {
	start := time.Now()

	ctx, cancel := context.WithTimeout(w.ctx, 10*time.Second)
	defer cancel()

	events, err := w.outboxRepo.GetPending(ctx, w.batchSize)
	if err != nil {
		slog.Error("get pending outbox events", "error", err)
		w.recordBatchEnd(start, 0)
		return
	}
	if len(events) == 0 {
		w.recordBatchEnd(start, 0)
		return
	}

	published := 0
	for _, evt := range events {
		select {
		case <-w.stopCh:
			w.recordBatchEnd(start, published)
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

	w.recordBatchEnd(start, published)
}

func (w *OutboxWorker) recordBatchEnd(start time.Time, published int) {
	elapsed := time.Since(start)
	w.lastBatchEndTime.Store(time.Now())
	w.lastBatchMsgCnt.Store(int64(published))
	w.lastBatchDur.Store(elapsed.Nanoseconds())
	if published > 0 {
		w.lastSuccessTime.Store(time.Now())
	}
}

func unmarshalPayload(data []byte) (interface{}, error) {
	var payload interface{}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, err
	}
	return payload, nil
}
