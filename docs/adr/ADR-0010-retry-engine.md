# ADR-0010: Retry Engine as Independent Service

**Status**: Accepted  
**Date**: 2026-07-06  
**Deciders**: Raghna, Mostafastbot  

---

## Context

Initially, retry logic was embedded in the QueueWorker (check if failed, schedule retry). As the platform grows, different message types need different retry strategies: exponential backoff, fixed interval, cron-scheduled, provider-specific retry windows, manual admin retry. Embedding retry logic in the Worker or Pipeline violates SRP and makes it impossible to add new retry strategies without modifying core components.

## Decision

**Retry is an independent Service**, not a Pipeline Stage. It has its own lifecycle, its own state, and its own scheduler:

```
Pipeline: SendStage fails
               │
               ▼
        Publisher.SendFailed{MessageID, ErrorCode, Attempt, Retryable}
               │
               ▼
        RetryEngine.Decide(ctx, event)
               │
         ┌─────┴─────┐
         │           │
    Retryable    Terminal
         │           │
         ▼           ▼
  Scheduler    Publisher.MessageFailed
  .Schedule()        (terminal)
         │
         ▼
  Publisher.RetryScheduled{MessageID, ScheduledAt, Attempt}
         │
         ▼
  QueueWorker claims message again (status = queued)
         │
         ▼
  Pipeline executes from ClaimStage again
```

### Interface

```go
// RetryEngine — determines retry policy and schedules retries
type RetryEngine interface {
    // Decide evaluates a send failure and returns what to do next
    Decide(ctx context.Context, event *SendFailedEvent) (*RetryDecision, error)
}

// RetryPolicy — configurable per tenant, route, or message type
type RetryPolicy interface {
    // NextDelay returns the delay before the next retry attempt
    NextDelay(attempt int) time.Duration
    // MaxRetries returns the maximum number of retry attempts
    MaxRetries() int
    // ShouldRetry determines if a specific error is retryable
    ShouldRetry(errorCode string, errorMessage string) bool
}

type RetryDecision struct {
    ShouldRetry bool
    Delay       time.Duration   // delay before next attempt
    Reason      string          // why this decision was made
}

// RetryResult — returned by RetryEngine after execution
type RetryResult struct {
    Decision    RetryDecision
    Attempt     int
    ScheduledAt time.Time
}
```

### Retry Policy Implementations

```go
// ExponentialBackoff — exponential backoff with jitter
type ExponentialBackoff struct {
    InitialDelay time.Duration  // default: 1s
    MaxDelay     time.Duration  // default: 300s (5 min)
    Multiplier   float64        // default: 2.0
    MaxRetries   int            // default: 5
    Jitter       float64        // default: 0.1 (10%)
}

func (e *ExponentialBackoff) NextDelay(attempt int) time.Duration {
    delay := float64(e.InitialDelay) * math.Pow(e.Multiplier, float64(attempt-1))
    delay = math.Min(delay, float64(e.MaxDelay))
    // Add jitter: ±10%
    jitter := delay * e.Jitter * (rand.Float64()*2 - 1)
    return time.Duration(delay + jitter)
}

// FixedInterval — retry at a fixed interval
type FixedInterval struct {
    Interval   time.Duration // default: 60s
    MaxRetries int           // default: 3
}

// ProviderRetryWindow — respects provider-specific retry windows (e.g., SMPP provider's recommended retry timing)
type ProviderRetryWindow struct {
    Windows    []time.Duration // e.g., [30s, 60s, 120s, 300s]
    MaxRetries int
}
```

### Event-Driven Integration

```go
// RetryEngine subscriber — reacts to SendFailed events
func (s *RetryEngineSubscriber) HandleEvent(ctx context.Context, envelope EventEnvelope) error {
    if envelope.EventType != EventTypeMessageFailedV1 {
        return nil // not our event
    }

    var payload MessageFailedV1Payload
    json.Unmarshal(envelope.Payload, &payload)

    if !payload.Retryable {
        return nil // terminal failure, don't retry
    }

    decision, err := s.engine.Decide(ctx, &SendFailedEvent{
        MessageID:    payload.MessageID,
        ErrorCode:    payload.ErrorCode,
        ErrorMessage: payload.ErrorMessage,
        Attempt:      payload.Attempt,
    })
    if err != nil || !decision.ShouldRetry {
        return nil
    }

    // Schedule the retry
    scheduledAt := time.Now().Add(decision.Delay)
    err = s.scheduler.Schedule(ctx, &ScheduleRequest{
        MessageID: payload.MessageID,
        RunAt:     scheduledAt,
        Action:    "retry",
    })
    if err != nil {
        return fmt.Errorf("schedule retry: %w", err)
    }

    // Publish RetryScheduled event
    return s.publisher.Publish(ctx, EventEnvelope{
        EventType: EventTypeMessageRetryingV1,
        Payload: mustMarshal(MessageRetryingV1Payload{
            MessageID:   payload.MessageID,
            Attempt:     payload.Attempt + 1,
            ScheduledAt: scheduledAt.Format(time.RFC3339),
        }),
    })
}
```

### Why Not a Pipeline Stage?

- Retry decisions may happen **long after** the pipeline completes (scheduled retry)
- Retry policy changes should not require pipeline reconfiguration
- Different tenants/routes may have completely different retry strategies
- Manual retry (admin clicks "retry" in dashboard) should reuse the same engine

## Consequences

### Positive
- Retry strategies are pluggable — exponential, fixed, provider, manual, cron
- Pipeline never needs to know about retry logic
- Retry policy can differ per tenant, per route, per message priority
- Scheduler handles timing — RetryEngine handles policy
- Manual retry via admin API reuses the same RetryEngine

### Negative
- More components: RetryEngine + Scheduler + subscriber
- Slightly more latency: SendFailed → subscriber → schedule → queue → claim → retry

### Mitigations
- RetryEngine is a pure function — testable without DB or network
- Scheduler can be embedded in the same process initially (PostgreSQL-based)
- Retry loop latency is intentional — the message was already delayed by seconds/minutes

## Compliance
- **Compatibility Rule**: New protocol uses same RetryEngine — no changes needed
- **8-Question Test**: RetryEngine is an interface, independently testable, pluggable

## References
- ADR-0001: Event-Driven Core
- ADR-0003: Pipeline Architecture
- ADR-0011: Scheduler Component
- ARCHITECTURE_PRINCIPLES.md § Retry Policy is Independent Business Logic
