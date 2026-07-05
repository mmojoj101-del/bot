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

func (p *DefaultRetryPolicy) NextDelay(attempt int) time.Duration {
	if attempt <= 1 {
		return p.baseDelay
	}
	exp := math.Pow(2, float64(attempt-1))
	delay := time.Duration(float64(p.baseDelay) * exp)
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

	ctx     context.Context
	cancel  context.CancelFunc
	stopCh  chan struct{}
	stopOnce sync.Once
	wg      sync.WaitGroup

	running    atomic.Bool
	restartCnt atomic.Int64
}

func NewRetryEngine(
	queueRepo domain.QueueRepository,
	retryPolicy domain.RetryPolicy,
	opts ...RetryEngineOption,
) *RetryEngine {
	ctx, cancel := context.WithCancel(context.Background())
	e := &RetryEngine{
		queueRepo:    queueRepo,
		retryPolicy:  retryPolicy,
		batchSize:    100,
		pollInterval: 5 * time.Second,
		stopCh:       make(chan struct{}),
		cancel:       cancel,
		ctx:          ctx,
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
		backoff := 100 * time.Millisecond
		for {
			if e.iteration() {
				backoff = 100 * time.Millisecond
				continue
			}
			select {
			case <-e.stopCh:
				return
			default:
				slog.Warn("retry engine restarting",
					"restart_count", e.restartCnt.Load(),
					"backoff_ms", backoff.Milliseconds(),
				)
				time.Sleep(backoff)
				if backoff < 30*time.Second {
					backoff *= 2
				}
			}
		}
	}()
	slog.Info("retry engine started", "batch_size", e.batchSize, "poll_interval", e.pollInterval)
}

func (e *RetryEngine) iteration() (ok bool) {
	defer func() {
		if r := recover(); r != nil {
			e.restartCnt.Add(1)
			slog.Error("retry engine panic recovered",
				"panic", r,
				"restart_count", e.restartCnt.Load(),
			)
			ok = false
		}
	}()
	select {
	case <-e.stopCh:
		return false
	default:
		e.processRetries()
		time.Sleep(e.pollInterval)
		return true
	}
}

func (e *RetryEngine) Stop() {
	e.stopOnce.Do(func() {
		e.cancel()
		close(e.stopCh)
	})
	e.wg.Wait()
}

func (e *RetryEngine) IsHealthy() error {
	if !e.running.Load() {
		return fmt.Errorf("retry engine is not running")
	}
	select {
	case <-e.stopCh:
		return fmt.Errorf("retry engine is stopped")
	default:
		return nil
	}
}

func (e *RetryEngine) HealthDetail() map[string]interface{} {
	return map[string]interface{}{
		"alive":         e.running.Load(),
		"stopped":       isClosed(e.stopCh),
		"restart_count": e.restartCnt.Load(),
		"batch_size":    e.batchSize,
		"poll_interval": e.pollInterval.String(),
		"type":          "retry_engine",
	}
}

func (e *RetryEngine) processRetries() {
	ctx, cancel := context.WithTimeout(e.ctx, 30*time.Second)
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
