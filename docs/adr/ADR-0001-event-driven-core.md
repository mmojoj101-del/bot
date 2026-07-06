# ADR-0001: Event-Driven Core Architecture

**Status**: Accepted  
**Date**: 2026-07-06  
**Deciders**: Raghna, Mostafastbot  

---

## Context

The initial architecture used a Worker-Driven approach where the QueueWorker directly called services (audit, billing, metrics, webhooks) and was tightly coupled to the Event Bus implementation (`MemoryBus`). As we add SMPP, SIP, and multiple providers, the Worker would become a monolithic orchestrator.

## Decision

Adopt an **Event-Driven Core** with the following principles:

### 1. Domain Event Publisher Interface
The Worker depends on an interface, not a concrete Event Bus:

```go
type DomainEventPublisher interface {
    Publish(ctx context.Context, event DomainEvent) error
}
```

The Worker calls `publisher.Publish(ctx, MessageSent{...})` — it never knows if the implementation is in-memory, outbox-pattern, Kafka, or NATS.

### 2. Worker is an Executor, Not an Orchestrator
The Worker executes the message lifecycle pipeline and publishes Domain Events at each step. Subscribers (not the Worker) handle side effects:

- AuditSubscriber → logs state transitions
- BillingSubscriber → deducts credits
- MetricsSubscriber → records latency/counters
- WebhookSubscriber → notifies customer callbacks

### 3. Domain Events ≠ Infrastructure Events
Separate channels for business events vs operations events:

| Domain Events | Infrastructure Events |
|---------------|----------------------|
| MessageQueued | WorkerStarted |
| MessageSent | CircuitBreakerOpened |
| MessageDelivered | DBReconnected |
| RouteSelected | QueueDepthAlert |

## Consequences

### Positive
- Adding new features = adding event subscribers, zero Worker changes
- Event Bus can be swapped without touching domain logic
- Each subscriber is independently testable and deployable
- System is naturally decoupled and extensible

### Negative
- Eventual consistency: subscribers may lag behind the main flow
- Requires proper idempotency in subscribers (same event may arrive twice)
- More moving parts to monitor (event bus health, subscriber lag)

### Mitigations
- Outbox pattern ensures exactly-once delivery for critical events
- Subscribers are idempotent (check before-insert/update)
- Event bus health is part of /ready endpoint

## Compliance
- **Compatibility Rule**: Adding Protocol X tomorrow requires zero Worker changes
- **8-Question Test**: Features added as subscribers only, Worker unchanged

## References
- ARCHITECTURE_PRINCIPLES.md § Event-Driven Core
- ROADMAP.md § Phase 2.5
