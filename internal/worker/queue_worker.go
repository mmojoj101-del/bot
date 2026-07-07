package worker

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/raghna/fury-sms-gateway/internal/connector"
	"github.com/raghna/fury-sms-gateway/internal/domain"
	"github.com/raghna/fury-sms-gateway/internal/event"
)

// MaxConsecutiveRestarts is the threshold after which the OnRestart callback
// receives a critical flag (and the metric counter is incremented).
const MaxConsecutiveRestarts = 10

// RestartEvent carries information about a worker restart.
type RestartEvent struct {
	WorkerType          string
	RestartCount        int64
	ConsecutiveFailures int
	Backoff             time.Duration
	Critical            bool
}

// QueueWorker pulls queued messages and sends them through the appropriate connector.
type QueueWorker struct {
	queueRepo    domain.QueueRepository
	msgRepo      domain.MessageRepository
	registry     connector.ConnectorRegistry
	retry        domain.RetryPolicy
	metrics      domain.MetricsRecorder
	eventBus     eventPublisher
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
	lastBatchDur     atomic.Int64 // nanoseconds
}

// eventPublisher is a subset of the event bus used by the worker.
type eventPublisher interface {
	Publish(event.Event)
}

// NewQueueWorker creates a new queue worker.
func NewQueueWorker(
	parentCtx context.Context,
	queueRepo domain.QueueRepository,
	msgRepo domain.MessageRepository,
	registry connector.ConnectorRegistry,
	retry domain.RetryPolicy,
	metrics domain.MetricsRecorder,
	eventBus eventPublisher,
	opts ...QueueWorkerOption,
) *QueueWorker {
	ctx, cancel := context.WithCancel(parentCtx)
	w := &QueueWorker{
		queueRepo:    queueRepo,
		msgRepo:      msgRepo,
		registry:     registry,
		retry:        retry,
		metrics:      metrics,
		eventBus:     eventBus,
		batchSize:    100,
		pollInterval: 1 * time.Second,
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

// QueueWorkerOption configures the QueueWorker.
type QueueWorkerOption func(*QueueWorker)

func WithBatchSize(size int) QueueWorkerOption {
	return func(w *QueueWorker) { w.batchSize = size }
}

func WithPollInterval(d time.Duration) QueueWorkerOption {
	return func(w *QueueWorker) { w.pollInterval = d }
}

// WithRestartCallback registers a callback invoked on every restart.
func WithRestartCallback(fn func(RestartEvent)) QueueWorkerOption {
	return func(w *QueueWorker) { w.onRestart = fn }
}

// Start begins the worker loop in a goroutine with panic recovery and auto-restart.
func (w *QueueWorker) Start() {
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
			}
			w.restartCnt.Add(1)
			w.lastPanicTime.Store(time.Time{})
			w.lastSuccessTime.Store(time.Time{})

			maxBackoff := 30 * time.Second
			if backoff < maxBackoff {
				backoff *= 2
				if backoff > maxBackoff {
					backoff = maxBackoff
				}
			}

			critical := consecutiveFailures >= MaxConsecutiveRestarts
			w.onRestart(RestartEvent{
				WorkerType:          "queue_worker",
				RestartCount:        w.restartCnt.Load(),
				ConsecutiveFailures: consecutiveFailures,
				Backoff:             backoff,
				Critical:            critical,
			})

			select {
			case <-w.stopCh:
				return
			case <-time.After(backoff):
			}
		}
	}()
}

// Stop signals the worker to stop and waits for it to finish.
func (w *QueueWorker) Stop() {
	w.stopOnce.Do(func() {
		w.cancel()
		close(w.stopCh)
	})
	w.wg.Wait()
}

// IsRunning reports whether the worker loop is currently executing.
func (w *QueueWorker) IsRunning() bool {
	return w.running.Load()
}

// iteration polls the queue for messages and processes them.
func (w *QueueWorker) iteration() bool {
	claimed, err := w.queueRepo.ClaimQueued(w.ctx, w.batchSize)
	if err != nil {
		if w.ctx.Err() != nil {
			return false
		}
		return true
	}

	if len(claimed) == 0 {
		select {
		case <-w.stopCh:
			return false
		case <-time.After(w.pollInterval):
		}
		return true
	}

	start := time.Now()
	var wg sync.WaitGroup

	for i := range claimed {
		msg := claimed[i]
		wg.Add(1)
		go func(m domain.Message) {
			defer wg.Done()
			w.processMessage(m)
		}(msg)
	}

	wg.Wait()

	elapsed := time.Since(start)
	w.lastBatchMsgCnt.Store(int64(len(claimed)))
	w.lastBatchDur.Store(elapsed.Nanoseconds())
	w.lastBatchEndTime.Store(time.Now())

	return true
}

// processMessage handles a single claimed message.
// It looks up the connector by ID and sends the message.
func (w *QueueWorker) processMessage(msg domain.Message) {
	logger := slog.With("message_id", msg.ID, "connector_id", msg.ConnectorID)

	if msg.ConnectorID == nil || *msg.ConnectorID == "" {
		logger.Warn("message has no connector, skipping")
		return
	}

	// Resolve connector from registry by ID (not by type).
	// The registry holds pre-initialized GenericConnector instances.
	conn, err := w.registry.Get(*msg.ConnectorID)
	if err != nil {
		logger.Error("connector not found in registry", "error", err)
		w.ackFailed(msg, "CONNECTOR_NOT_FOUND", err.Error())
		return
	}

	// Build prepared message. The message stored in the queue may not have
	// a PreparedMessage — we build one from the message data.
	// Destination uses msg.Prepared.Destination if available, else msg.Destination.
	prepared := &domain.PreparedMessage{
		Destination: msg.Destination,
		Parts:       1,
		Encoding:    string(msg.Encoding),
	}
	if msg.Parts > 0 {
		prepared.Parts = msg.Parts
	}

	start := time.Now()
	result, err := conn.Send(w.ctx, &domain.SendRequest{
		Message:  &msg,
		Prepared: prepared,
	})
	latency := time.Since(start)

	if err != nil {
		logger.Warn("send failed", "error", err, "retry_count", msg.RetryCount, "max_retries", msg.MaxRetries)
		if msg.RetryCount < msg.MaxRetries {
			if err := w.queueRepo.ScheduleRetry(w.ctx, msg.ID, int(msg.Version), "SEND_FAILED", err.Error()); err != nil {
				logger.Error("schedule retry", "error", err)
			} else {
				logger.Info("message scheduled for retry", "attempt", msg.RetryCount+1)
				if w.metrics != nil {
					w.metrics.RecordRetry(msg.TenantID, msg.RetryCount+1)
				}
			}
		} else {
			w.ackFailed(msg, "SEND_FAILED", err.Error())
		}
		if w.metrics != nil {
			w.metrics.RecordMessageFailed(msg.TenantID, *msg.ConnectorID, "SEND_FAILED")
		}
		return
	}

	if err := w.queueRepo.AckSent(w.ctx, msg.ID, int(msg.Version), result.ExternalID, result.Parts, result.Price, result.Cost); err != nil {
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

	w.lastSuccessTime.Store(time.Now())
}

func (w *QueueWorker) ackFailed(msg domain.Message, errorCode, errorMessage string) {
	if err := w.queueRepo.AckFailed(w.ctx, msg.ID, int(msg.Version), errorCode, errorMessage); err != nil {
		slog.Error("ack failed", "message_id", msg.ID, "error", err)
	}
}

func (w *QueueWorker) IsHealthy() error {
	if !w.running.Load() {
		return fmt.Errorf("queue worker is not running")
	}
	// Check for unhealthy idle periods
	if v, ok := w.lastSuccessTime.Load().(time.Time); ok && !v.IsZero() {
		if time.Since(v) > healthyIdleThreshold {
			return fmt.Errorf("queue worker idle for %v (threshold: %v)",
				time.Since(v).Round(time.Second), healthyIdleThreshold)
		}
	}
	return nil
}

func (w *QueueWorker) HealthDetail() map[string]interface{} {
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
		"type":               "queue_worker",
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
