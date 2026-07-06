# ADR-0011: Scheduler as Independent Component

**Status**: Accepted  
**Date**: 2026-07-06  
**Deciders**: Raghna, Mostafastbot  

---

## Context

Several platform features need deferred execution: retry scheduling, delayed message sending, periodic health checks, DLR timeout detection, and batch operations. Embedding scheduling logic in each component leads to duplication and makes it impossible to observe or manage all scheduled tasks centrally. The platform needs a single Scheduler component that any part of the system can use.

## Decision

**Scheduler is an independent component** with a clean interface. Initially backed by PostgreSQL (polling a `scheduled_tasks` table), but designed to be replaced by Redis, RabbitMQ delayed queues, or a dedicated scheduler service.

### Interface

```go
// Scheduler — schedules deferred execution of actions
type Scheduler interface {
    // Schedule an action to run at a specific time
    Schedule(ctx context.Context, req *ScheduleRequest) (*ScheduleResult, error)
    
    // Cancel a previously scheduled action
    Cancel(ctx context.Context, taskID string) error
    
    // Reschedule an existing task with a new run time
    Reschedule(ctx context.Context, taskID string, runAt time.Time) error
    
    // ListScheduled returns all scheduled tasks (with optional filter)
    ListScheduled(ctx context.Context, filter ScheduleFilter) ([]*ScheduledTask, error)
}

type ScheduleRequest struct {
    MessageID   string            `json:"message_id"`
    TenantID    string            `json:"tenant_id"`
    Action      string            `json:"action"`       // "retry", "send", "check", "expire"
    RunAt       time.Time         `json:"run_at"`       // when to execute
    Priority    int               `json:"priority"`     // for ordering when multiple tasks due
    Metadata    map[string]string `json:"metadata,omitempty"` // extensible
}

type ScheduleResult struct {
    TaskID      string    `json:"task_id"`
    ScheduledAt time.Time `json:"scheduled_at"`
    RunAt       time.Time `json:"run_at"`
}

type ScheduledTask struct {
    TaskID      string
    MessageID   string
    TenantID    string
    Action      string
    Status      TaskStatus   // pending, completed, cancelled, failed
    RunAt       time.Time
    CreatedAt   time.Time
    Attempts    int
    LastError   string
    Metadata    map[string]string
}

type TaskStatus string

const (
    TaskStatusPending   TaskStatus = "pending"
    TaskStatusRunning   TaskStatus = "running"
    TaskStatusCompleted TaskStatus = "completed"
    TaskStatusCancelled TaskStatus = "cancelled"
    TaskStatusFailed    TaskStatus = "failed"
)

type ScheduleFilter struct {
    TenantID  string
    Action    string
    Status    TaskStatus
    DueBefore *time.Time  // tasks due before this time
    Limit     int         // max results
}
```

### PostgreSQL Implementation

```sql
CREATE TABLE scheduled_tasks (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    message_id  UUID NOT NULL REFERENCES messages(id),
    tenant_id   UUID NOT NULL,
    action      VARCHAR(50) NOT NULL,        -- 'retry', 'send', 'check', 'expire'
    status      VARCHAR(20) NOT NULL DEFAULT 'pending',
    run_at      TIMESTAMPTZ NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    attempts    INT NOT NULL DEFAULT 0,
    last_error  TEXT,
    metadata    JSONB DEFAULT '{}',
    version     INT NOT NULL DEFAULT 1        -- optimistic locking
);

CREATE INDEX idx_scheduled_tasks_due ON scheduled_tasks (status, run_at) WHERE status = 'pending';
CREATE INDEX idx_scheduled_tasks_message ON scheduled_tasks (message_id) WHERE status = 'pending';
```

### Scheduler Worker

```go
// SchedulerWorker — polls for due tasks and executes them
type SchedulerWorker struct {
    repo      ScheduledTaskRepository
    publisher DomainEventPublisher
    retryer   RetryEngine
}

func (w *SchedulerWorker) Poll(ctx context.Context) error {
    // Claim due tasks with FOR UPDATE SKIP LOCKED
    tasks, err := w.repo.ClaimDue(ctx, time.Now(), 100)
    if err != nil {
        return err
    }
    
    for _, task := range tasks {
        switch task.Action {
        case "retry":
            // Set message status back to queued
            // Publish MessageRetried event
            w.publisher.Publish(ctx, EventEnvelope{
                EventType: EventTypeMessageRetryingV1,
                Payload:   marshal(MessageRetryingV1Payload{...}),
            })
            
        case "expire":
            // Mark message as expired (max retries exceeded)
            w.publisher.Publish(ctx, EventEnvelope{
                EventType: EventTypeMessageExpiredV1,
                Payload:   marshal(MessageExpiredV1Payload{...}),
            })
            
        case "check":
            // Health check a connector
            // ...
        }
    }
    return nil
}
```

### Use Cases

| Use Case | Action | Triggers |
|----------|--------|----------|
| Retry after failure | `retry` | `SendFailed` event |
| Delayed message sending | `send` | API with future `send_at` |
| DLR timeout detection | `expire` | After `Sent` without DLR within TTL |
| Periodic health check | `check` | Configurable interval |
| Batch operations | `batch` | Admin API |

## Consequences

### Positive
- Single component for all deferred operations — consistent, observable
- Retry scheduling is decoupled from the Pipeline (see ADR-0010)
- Can be swapped: PostgreSQL → Redis → Kafka without touching other components
- Centralized management: list, cancel, reschedule any task

### Negative
- PostgreSQL-based polling adds DB load (mitigated by index + batch size control)
- Polling interval introduces latency (1-5 second granularity)

### Mitigations
- Proper indexing on `(status, run_at)` with partial index
- Configurable poll interval and batch size
- Future: Redis sorted set implementation for sub-second precision

## Compliance
- **Compatibility Rule**: New protocol uses same Scheduler — no changes needed
- **8-Question Test**: Scheduler is an interface, independently testable, replaceable

## References
- ADR-0010: Retry Engine
- ARCHITECTURE_PRINCIPLES.md § Horizontal Scaling
- ROADMAP.md § Phase 2.5
