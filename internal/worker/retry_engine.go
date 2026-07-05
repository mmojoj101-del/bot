package worker

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/raghna/fury-sms-gateway/internal/domain"
)

// DefaultRetryPolicy implements exponential backoff with jitter.
// Delays: 5s, 30s, 2m, 10m, 30m (or until MaxRetries reached).
type DefaultRetryPolicy struct {
	baseDelay  time.Duration
	maxDelay   time.Duration
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

// RetryEngine monitors messages in 'retrying' status and re-queues them.
type RetryEngine struct {
	queueRepo    domain.QueueRepository
	retryPolicy  domain.RetryPolicy
	batchSize    int
	pollInterval time.Duration

	stopCh chan struct{}
	wg     sync.WaitGroup

	running atomic.Bool
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
	e.running.Store(true)
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		defer e.running.Store(false)
		for {
			if !e.iteration() {
				return
			}
		}
	}()
	slog.Info("retry engine started", "batch_size", e.batchSize, "poll_interval", e.pollInterval)
}

func (e *RetryEngine) iteration() bool {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("retry engine panic recovered, restarting...", "panic", r)
			time.Sleep(time.Second)
		}
	}()

	select {
	case <-e.stopCh:
		slog.Info("retry engine stopped")
		return false
	default:
		e.processRetries()
		time.Sleep(e.pollInterval)
		return true
	}
}

func (e *RetryEngine) Stop() {
	close(e.stopCh)
	e.wg.Wait()
}

func (e *RetryEngine) IsHealthy() error {
	if !e.running.Load() {
		return fmt.Errorf("retry engine goroutine is not running")
	}
	select {
	case <-e.stopCh:
		return fmt.Errorf("retry engine is stopped")
	default:
		return nil
	}
}

func (e *RetryEngine) processRetries() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	now := time.Now().UTC()
	minDelay := e.retryPolicy.NextDelay(1)

	messages, err := e.queueRepo.GetRetryable(ctx, now, minDelay, e.batchSize)
	if err != nil {
		slog.Error("get retryable messages", "error", err)
		return
	}

	for _, msg := range messages {
		select {
		case <-e.stopCh:
			return
		default:
		}

		requiredDelay := e.retryPolicy.NextDelay(msg.RetryCount + 1)
		if msg.UpdatedAt.Add(requiredDelay).After(now) {
			continue
		}

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
