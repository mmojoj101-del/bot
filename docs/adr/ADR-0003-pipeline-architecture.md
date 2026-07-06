# ADR-0003: Pipeline Architecture

**Status**: Accepted  
**Date**: 2026-07-06  
**Deciders**: Raghna, Mostafastbot  

---

## Context

The initial Worker is a single function that claims, routes, sends, and handles results. As we add features like rate limiting, billing, fraud detection, and compliance, the Worker would grow into a monolith. Each new feature would require modifying the Worker's core logic.

## Decision

Adopt a **Pipeline of pluggable Stages** for the message lifecycle:

### Stage Interface

```go
// Stage — a single step in the message lifecycle pipeline
type Stage interface {
    Name() string
    Process(ctx context.Context, state *PipelineState) error
}

// PipelineState — the state object passed through all stages
type PipelineState struct {
    Message    *CanonicalMessage
    Decision   *RoutingDecision   // immutable, set once
    SendResult *SendResult
    Error      error
    Attempt    int                // current retry attempt
    MaxRetries int
    TraceID    string

    // Stages can attach metadata for downstream stages
    Metadata   map[string]interface{}
}

// Pipeline — executes stages in order
type Pipeline struct {
    stages []Stage
}

func (p *Pipeline) Execute(ctx context.Context, state *PipelineState) error {
    for _, stage := range p.stages {
        if err := stage.Process(ctx, state); err != nil {
            return fmt.Errorf("pipeline stage %s: %w", stage.Name(), err)
        }
    }
    return nil
}
```

### Default Pipeline Stages

```
1. ClaimStage        — claim message from queue (FOR UPDATE SKIP LOCKED)
2. ValidateStage     — validate message fields, encoding, length
3. RouteStage        — ask Routing Engine for connector decision
4. PrepareStage      — prepare message for sending (split multipart, encode)
5. SendStage         — call Connector.Send()
6. HandleResultStage — process SendResult, update state, schedule retry or complete
7. EmitStage         — publish Domain Events for subscribers
```

### Adding Features via Stages

| Feature | Implementation | Worker Change? |
|---------|---------------|----------------|
| Rate Limiting | `RateLimitStage` after Claim | ❌ |
| Billing | `BillingStage` after Route | ❌ |
| Fraud Detection | `FraudStage` between Validate and Route | ❌ |
| Compliance Logging | `ComplianceStage` after Prepare | ❌ |
| Content Filtering | `FilterStage` during Validate | ❌ |

### Adding Features via Event Subscribers

| Feature | Subscribes To | Worker Change? |
|---------|--------------|----------------|
| Audit Trail | All Domain Events | ❌ |
| Prometheus Metrics | All Domain Events | ❌ |
| Customer Webhook | MessageSent, MessageDelivered | ❌ |
| Email/SMS Alert | MessageFailed | ❌ |
| Analytics Dashboard | All Domain Events | ❌ |
| Retry Engine | SendFailed | ❌ |

## Consequences

### Positive
- New features = new stage or new subscriber — zero Worker changes
- Each stage is independently testable (mock `PipelineState`)
- Stages can be reordered, enabled/disabled per tenant
- Clear separation of concerns — no 1000-line Worker

### Negative
- Pipeline overhead (function call + state object per stage)
- Stages must be careful not to mutate state that affects other stages

### Mitigations
- Pipeline overhead is negligible compared to DB/HTTP calls
- `RoutingDecision` is immutable (ADR-0005) — prevents accidental mutation
- `PipelineState.Metadata` is the only mutable field, namespaced per stage

## Compliance
- **Compatibility Rule**: Protocol X requires no pipeline stage changes
- **8-Question Test**: New feature = add stage or subscriber, Worker unchanged

## References
- ARCHITECTURE_PRINCIPLES.md § Pipeline Architecture
- ADR-0001: Event-Driven Core
- ROADMAP.md § Phase 2.5
