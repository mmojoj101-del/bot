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
	cancel  context.CancelFunc // cancels in-flight operations on shutdown
	stopCh  chan struct{}
	wg      sync.WaitGroup

	running atomic.Bool
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
		for {
			if !w.iteration() {
				return
			}
		}
	}()
	slog.Info("queue worker started", "batch_size", w.batchSize, "poll_interval", w.pollInterval)
}

// iteration runs one poll cycle and returns false if we should stop.
func (w *QueueWorker) iteration() bool {
	// Panic recovery WITH auto-restart
	defer func() {
		if r := recover(); r != nil {
			slog.Error("queue worker panic recovered, restarting...", "panic", r)
			time.Sleep(time.Second) // prevent tight restart loop
		}
	}()

	select {
	case <-w.stopCh:
		slog.Info("queue worker stopped")
		return false
	default:
		w.processBatch()
		time.Sleep(w.pollInterval)
		return true
	}
}

// Stop signals the worker to shut down gracefully.
// Cancels in-flight operations and waits for the goroutine to exit.
func (w *QueueWorker) Stop() {
	w.cancel()     // interrupt in-flight DB queries and HTTP sends
	close(w.stopCh) // signal loop to stop
	w.wg.Wait()     // wait for goroutine to exit
}

// IsHealthy returns nil if the worker is healthy.
func (w *QueueWorker) IsHealthy() error {
	if !w.running.Load() {
		return fmt.Errorf("queue worker goroutine is not running")
	}
	select {
	case <-w.stopCh:
		return fmt.Errorf("queue worker is stopped")
	default:
		return nil
	}
}

func (w *QueueWorker) processBatch() {
	// Use the worker's cancellable context so shutdown interrupts in-flight sends
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
