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

// MaxConsecutiveRestarts is the threshold after which the OnRestart callback
// receives a critical flag (and the metric counter is incremented).
const MaxConsecutiveRestarts = 10

// RestartEvent carries information about a worker restart.
type RestartEvent struct {
	WorkerType         string
	RestartCount       int64
	ConsecutiveFailures int
	Backoff            time.Duration
	Critical           bool
}

// QueueWorker pulls queued messages and sends them through the appropriate connector.
type QueueWorker struct {
	queueRepo    domain.QueueRepository
	msgRepo      domain.MessageRepository
	connRepo     domain.ConnectorRepository
	senders      map[domain.ConnectorType]domain.Sender
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
// Accept a parent context so main owns the context tree.
func NewQueueWorker(
	parentCtx context.Context,
	queueRepo domain.QueueRepository,
	msgRepo domain.MessageRepository,
	connRepo domain.ConnectorRepository,
	senders map[domain.ConnectorType]domain.Sender,
	retry domain.RetryPolicy,
	metrics domain.MetricsRecorder,
	eventBus eventPublisher,
	opts ...QueueWorkerOption,
) *QueueWorker {
	ctx, cancel := context.WithCancel(parentCtx)
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
// The callback receives a RestartEvent with context about the failure.
// Use this for Prometheus counters, alert hooks, or circuit-breaker logic.
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
					"queue worker restarting",
					"restart_count", w.restartCnt.Load(),
					"consecutive_failures", consecutiveFailures,
					"backoff_ms", backoff.Milliseconds(),
				)
				w.onRestart(RestartEvent{
					WorkerType:          "queue_worker",
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
	slog.Info("queue worker started", "batch_size", w.batchSize, "poll_interval", w.pollInterval)
}

// iteration runs one poll cycle. Returns true on success, false on panic/shutdown.
func (w *QueueWorker) iteration() (ok bool) {
	defer func() {
		if r := recover(); r != nil {
			w.restartCnt.Add(1)
			w.lastPanicTime.Store(time.Now())
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

// Stop signals the worker to shut down gracefully. Idempotent.
func (w *QueueWorker) Stop() {
	w.stopOnce.Do(func() {
		w.cancel()
		close(w.stopCh)
	})
	w.wg.Wait()
}

// healthyIdleThreshold is the maximum time since the last successful batch
// before the worker is considered unhealthy (stuck / deadlocked).
const healthyIdleThreshold = 5 * time.Minute

// IsHealthy returns nil if the worker is healthy.
func (w *QueueWorker) IsHealthy() error {
	if !w.running.Load() {
		return fmt.Errorf("queue worker is not running")
	}
	select {
	case <-w.stopCh:
		return fmt.Errorf("queue worker is stopped")
	default:
	}
	// Check idle threshold: if the last batch was too long ago,
	// the worker loop may be stuck (deadlock / infinite backoff).
	if v, ok := w.lastBatchEndTime.Load().(time.Time); ok && !v.IsZero() {
		if time.Since(v) > healthyIdleThreshold {
			return fmt.Errorf("queue worker idle for %v (threshold %v)", time.Since(v).Round(time.Second), healthyIdleThreshold)
		}
	}
	return nil
}

// HealthDetail returns detailed health info about the worker.
// The returned map is compatible with JSON serialisation for /ready endpoint.
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
		"type":                "queue_worker",
		"alive":               w.running.Load(),
		"stopped":             isClosed(w.stopCh),
		"restart_count":       w.restartCnt.Load(),
		"batch_size":          w.batchSize,
		"poll_interval":       w.pollInterval.String(),
		"last_panic_at":       nullTime(lastPanic),
		"last_success_at":     nullTime(lastSuccess),
		"last_batch_at":       nullTime(lastBatchEnd),
		"last_batch_msgs":     w.lastBatchMsgCnt.Load(),
		"last_batch_duration": durationMillis(w.lastBatchDur.Load()),
	}
}

func (w *QueueWorker) processBatch() {
	start := time.Now()

	messages, err := w.queueRepo.ClaimQueued(w.ctx, w.batchSize)
	if err != nil {
		slog.Error("claim queued messages", "error", err)
		w.recordBatchEnd(start, 0)
		return
	}
	if len(messages) == 0 {
		w.recordBatchEnd(start, 0)
		return
	}

	for _, msg := range messages {
		select {
		case <-w.stopCh:
			slog.Warn("shutting down, abandoning claimed messages", "count", len(messages))
			w.recordBatchEnd(start, 0)
			return
		default:
		}
		w.processMessage(w.ctx, msg)
	}

	w.recordBatchEnd(start, len(messages))
}

func (w *QueueWorker) recordBatchEnd(start time.Time, msgCount int) {
	elapsed := time.Since(start)
	w.lastBatchEndTime.Store(time.Now())
	w.lastBatchMsgCnt.Store(int64(msgCount))
	w.lastBatchDur.Store(elapsed.Nanoseconds())
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

	w.lastSuccessTime.Store(time.Now())
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

func nullTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func durationMillis(ns int64) float64 {
	return float64(ns) / 1e6
}
