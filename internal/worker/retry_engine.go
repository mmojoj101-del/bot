package worker

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"math/rand"
	"time"

	"github.com/raghna/fury-sms-gateway/internal/domain"
)

// DefaultRetryPolicy implements exponential backoff with jitter.
// Delays: 5s, 30s, 2m, 10m, 30m (or until MaxRetries reached).
type DefaultRetryPolicy struct {
	baseDelay time.Duration
	maxDelay  time.Duration
	maxRetries int
}

func NewDefaultRetryPolicy() *DefaultRetryPolicy {
	return &DefaultRetryPolicy{
		baseDelay:  5 * time.Second,
		maxDelay:   30 * time.Minute,
		maxRetries: 3,
	}
}

func (p *DefaultRetryPolicy) MaxRetries() int { return p.maxRetries }

// NextDelay calculates: min(baseDelay * 2^(attempt-1) + jitter, maxDelay)
// Jitter: ±25% of the calculated delay.
func (p *DefaultRetryPolicy) NextDelay(attempt int) time.Duration {
	if attempt <= 1 {
		return p.baseDelay
	}

	exp := math.Pow(2, float64(attempt-1))
	delay := time.Duration(float64(p.baseDelay) * exp)

	// Add jitter: ±25%
	jitter := time.Duration(float64(delay) * 0.25 * (rand.Float64()*2 - 1))
	delay += jitter

	if delay > p.maxDelay {
		delay = p.maxDelay
	}
	if delay < p.baseDelay {
		delay = p.baseDelay
	}
	return delay
}

// RetryEngine monitors messages in 'retrying' status and re-queues them
// after the backoff delay has elapsed.
type RetryEngine struct {
	queueRepo    domain.QueueRepository
	retryPolicy  domain.RetryPolicy
	batchSize    int
	pollInterval time.Duration
	stopCh       chan struct{}
}

func NewRetryEngine(
	queueRepo domain.QueueRepository,
	retryPolicy domain.RetryPolicy,
	opts ...RetryEngineOption,
) *RetryEngine {
	e := &RetryEngine{
		queueRepo:    queueRepo,
		retryPolicy:  retryPolicy,
		batchSize:    100,
		pollInterval: 5 * time.Second,
		stopCh:       make(chan struct{}),
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

type RetryEngineOption func(*RetryEngine)

func RetryEngineWithBatchSize(size int) RetryEngineOption {
	return func(e *RetryEngine) { e.batchSize = size }
}

func RetryEngineWithPollInterval(d time.Duration) RetryEngineOption {
	return func(e *RetryEngine) { e.pollInterval = d }
}

func (e *RetryEngine) Start() {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("retry engine panic recovered", "panic", r)
			}
		}()
		e.loop()
	}()
	slog.Info("retry engine started", "batch_size", e.batchSize, "poll_interval", e.pollInterval)
}

func (e *RetryEngine) Stop() {
	close(e.stopCh)
}

func (e *RetryEngine) IsHealthy() error {
	select {
	case <-e.stopCh:
		return fmt.Errorf("retry engine stopped")
	default:
		return nil
	}
}

func (e *RetryEngine) loop() {
	for {
		select {
		case <-e.stopCh:
			slog.Info("retry engine stopped")
			return
		default:
			e.processRetries()
			time.Sleep(e.pollInterval)
		}
	}
}

func (e *RetryEngine) processRetries() {
	ctx := context.Background()
	now := time.Now().UTC()

	// For the first attempt, the minimum delay is baseDelay
	minDelay := e.retryPolicy.NextDelay(1)

	messages, err := e.queueRepo.GetRetryable(ctx, now, minDelay, e.batchSize)
	if err != nil {
		slog.Error("get retryable messages", "error", err)
		return
	}

	for _, msg := range messages {
		// Calculate the actual delay for this message's retry count
		requiredDelay := e.retryPolicy.NextDelay(msg.RetryCount + 1)

		// Check if enough time has passed since the message was updated
		if msg.UpdatedAt.Add(requiredDelay).After(now) {
			continue // not yet ready
		}

		// Move back to 'sending' so the QueueWorker picks it up
		// Use AckFailed with retry semantics
		if err := e.queueRepo.AckFailed(ctx, msg.ID, int(msg.Version), "RETRY", "retrying after backoff"); err != nil {
			slog.Error("move to sending for retry", "message_id", msg.ID, "error", err)
			continue
		}
		slog.Info("message re-queued for retry",
			"message_id", msg.ID,
			"attempt", msg.RetryCount+1,
			"delay", requiredDelay,
		)
	}
}
