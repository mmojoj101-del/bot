package worker

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/raghna/fury-sms-gateway/internal/domain"
	"github.com/raghna/fury-sms-gateway/internal/event"
)

// QueueWorker pulls queued messages and sends them through the appropriate connector.
type QueueWorker struct {
	queueRepo  domain.QueueRepository
	msgRepo    domain.MessageRepository
	connRepo   domain.ConnectorRepository
	senders    map[domain.ConnectorType]domain.Sender
	retry      domain.RetryPolicy
	metrics    domain.MetricsRecorder
	eventBus   eventPublisher
	batchSize  int
	pollInterval time.Duration

	ctx     context.Context
	cancel  context.CancelFunc
	stopCh  chan struct{}
	stopOnce sync.Once
	wg      sync.WaitGroup

	running    atomic.Bool
	restartCnt atomic.Int64
}

// eventPublisher is a subset of the event bus used by the worker.
type eventPublisher interface {
	Publish(event.Event)
}

// NewQueueWorker creates a new queue worker.
func NewQueueWorker(
	queueRepo domain.QueueRepository,
	msgRepo domain.MessageRepository,
	connRepo domain.ConnectorRepository,
	senders map[domain.ConnectorType]domain.Sender,
	retry domain.RetryPolicy,
	metrics domain.MetricsRecorder,
	eventBus eventPublisher,
	opts ...QueueWorkerOption,
) *QueueWorker {
	ctx, cancel := context.WithCancel(context.Background())
	w := &QueueWorker{
		queueRepo:    queueRepo,
		msgRepo:      msgRepo,
		connRepo:     connRepo,
		senders:      senders,
		retry:        retry,
		metrics:      metrics,
		eventBus:     eventBus,
		batchSize:    100,
		pollInterval: 1 * time.Second,
		stopCh:       make(chan struct{}),
		cancel:       cancel,
		ctx:          ctx,
	}
	for _, opt := range opts {
		opt(w)
	}
	return w
}

// QueueWorkerOption configures the QueueWorker.
type QueueWorkerOption func(*QueueWorker)

func WithBatchSize(size int) QueueWorkerOption {
	return func(w *QueueWorker) { w.batchSize = size }
}

func WithPollInterval(d time.Duration) QueueWorkerOption {
	return func(w *QueueWorker) { w.pollInterval = d }
}

// Start begins the worker loop in a goroutine with panic recovery and auto-restart.
func (w *QueueWorker) Start() {
	w.running.Store(true)
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		defer w.running.Store(false)
		backoff := 100 * time.Millisecond
		for {
			if w.iteration() {
				backoff = 100 * time.Millisecond // reset on success
				continue
			}
			// iteration returned false — check if we should stop or restart
			select {
			case <-w.stopCh:
				return
			default:
				// Restart after panic
				slog.Warn("queue worker restarting",
					"restart_count", w.restartCnt.Load(),
					"backoff_ms", backoff.Milliseconds(),
				)
				time.Sleep(backoff)
				if backoff < 30*time.Second {
					backoff *= 2 // exponential backoff: 100ms → 200ms → 400ms → ...
				}
			}
		}
	}()
	slog.Info("queue worker started", "batch_size", w.batchSize, "poll_interval", w.pollInterval)
}

// iteration runs one poll cycle. Returns true on success, false on panic/shutdown.
func (w *QueueWorker) iteration() (ok bool) {
	defer func() {
		if r := recover(); r != nil {
			w.restartCnt.Add(1)
			slog.Error("queue worker panic recovered",
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

// Stop signals the worker to shut down gracefully. Idempotent — safe to call multiple times.
func (w *QueueWorker) Stop() {
	w.stopOnce.Do(func() {
		w.cancel()
		close(w.stopCh)
	})
	w.wg.Wait()
}

// IsHealthy returns nil if the worker is healthy.
func (w *QueueWorker) IsHealthy() error {
	if !w.running.Load() {
		return fmt.Errorf("queue worker is not running")
	}
	select {
	case <-w.stopCh:
		return fmt.Errorf("queue worker is stopped")
	default:
		return nil
	}
}

// HealthDetail returns detailed health info about the worker.
func (w *QueueWorker) HealthDetail() map[string]interface{} {
	return map[string]interface{}{
		"alive":          w.running.Load(),
		"stopped":        isClosed(w.stopCh),
		"restart_count":  w.restartCnt.Load(),
		"batch_size":     w.batchSize,
		"poll_interval":  w.pollInterval.String(),
		"type":           "queue_worker",
	}
}

func (w *QueueWorker) processBatch() {
	messages, err := w.queueRepo.ClaimQueued(w.ctx, w.batchSize)
	if err != nil {
		slog.Error("claim queued messages", "error", err)
		return
	}
	if len(messages) == 0 {
		return
	}

	for _, msg := range messages {
		select {
		case <-w.stopCh:
			slog.Warn("shutting down, abandoning claimed messages", "count", len(messages))
			return
		default:
		}
		w.processMessage(w.ctx, msg)
	}
}

func (w *QueueWorker) processMessage(ctx context.Context, msg domain.Message) {
	logger := slog.With("message_id", msg.ID, "tenant_id", msg.TenantID)

	if msg.ConnectorID == nil || *msg.ConnectorID == "" {
		logger.Warn("message has no connector, skipping")
		return
	}

	connector, err := w.connRepo.GetByID(ctx, *msg.ConnectorID)
	if err != nil {
		logger.Error("fetch connector", "connector_id", *msg.ConnectorID, "error", err)
		w.ackFailed(ctx, msg, "CONNECTOR_NOT_FOUND", err.Error())
		return
	}

	sender, ok := w.senders[connector.Type]
	if !ok {
		logger.Error("no sender registered", "connector_type", connector.Type)
		w.ackFailed(ctx, msg, "NO_SENDER", fmt.Sprintf("no sender for type %s", connector.Type))
		return
	}

	start := time.Now()
	result, err := sender.Send(ctx, domain.SendRequest{
		Message:   &msg,
		Connector: connector,
		Timeout:   30 * time.Second,
	})
	latency := time.Since(start)

	if err != nil {
		logger.Warn("send failed", "error", err, "retry_count", msg.RetryCount, "max_retries", msg.MaxRetries)

		if msg.RetryCount < msg.MaxRetries {
			if err := w.queueRepo.ScheduleRetry(ctx, msg.ID, int(msg.Version), "SEND_FAILED", err.Error()); err != nil {
				logger.Error("schedule retry", "error", err)
			} else {
				logger.Info("message scheduled for retry", "attempt", msg.RetryCount+1)
				if w.metrics != nil {
					w.metrics.RecordRetry(msg.TenantID, msg.RetryCount+1)
				}
			}
		} else {
			w.ackFailed(ctx, msg, "SEND_FAILED", err.Error())
		}
		if w.metrics != nil {
			w.metrics.RecordMessageFailed(msg.TenantID, *msg.ConnectorID, "SEND_FAILED")
		}
		return
	}

	if err := w.queueRepo.AckSent(ctx, msg.ID, int(msg.Version), result.ExternalID, result.Parts, result.Price, result.Cost); err != nil {
		logger.Error("ack sent", "error", err)
		return
	}

	logger.Info("message sent successfully",
		"external_id", result.ExternalID,
		"parts", result.Parts,
		"latency_ms", latency.Milliseconds(),
	)

	if w.metrics != nil {
		w.metrics.RecordMessageSent(msg.TenantID, *msg.ConnectorID, result.Parts, latency)
	}
}

func (w *QueueWorker) ackFailed(ctx context.Context, msg domain.Message, errorCode, errorMessage string) {
	if err := w.queueRepo.AckFailed(ctx, msg.ID, int(msg.Version), errorCode, errorMessage); err != nil {
		slog.Error("ack failed", "message_id", msg.ID, "error", err)
	}
}

// isClosed reports whether a channel is closed.
func isClosed(ch <-chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}
