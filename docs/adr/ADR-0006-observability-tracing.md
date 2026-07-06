# ADR-0006: Observability & Distributed Tracing

**Status**: Accepted  
**Date**: 2026-07-06  
**Deciders**: Raghna, Mostafastbot  

---

## Context

As the platform grows to multiple workers, connectors, and subscribers, debugging a single message's journey across the system becomes increasingly difficult. The existing `request_id` middleware only covers the HTTP API layer — it does not propagate through the Queue, Worker, Pipeline Stages, Connectors, or DLR callbacks. Without end-to-end tracing, we cannot reliably diagnose delivery failures, performance bottlenecks, or data inconsistencies.

## Decision

Adopt a **Trace Context** that propagates through the entire message lifecycle:

### Trace Context

```go
// TraceContext — propagates through all system boundaries
type TraceContext struct {
    TraceID       string    // unique per-message, spans full lifecycle
    SpanID        string    // unique per-pipeline-stage or component
    ParentSpanID  string    // parent span for building trace tree
    TenantID      string
    ConnectorID   string
    RouteID       string
    MessageID     string
}
```

### Propagation Path

```
API Handler (CreateMessage)
  │  TraceID: abc-123, SpanID: api-1, ParentSpanID: -
  │  Publish(MessageQueued)
  ▼
Queue (message row)
  │  TraceID stored in message.trace_id column
  ▼
QueueWorker (ClaimMessage)
  │  TraceID: abc-123, SpanID: worker-claim, ParentSpanID: api-1
  │  Publish(MessageClaimed)
  ▼
Pipeline
  ├── Validate Stage:  SpanID: pipeline-validate, ParentSpanID: worker-claim
  ├── Route Stage:     SpanID: pipeline-route,    ParentSpanID: pipeline-validate
  ├── Prepare Stage:   SpanID: pipeline-prepare,  ParentSpanID: pipeline-route
  ├── Send Stage:      SpanID: pipeline-send,     ParentSpanID: pipeline-prepare
  │                    → Connector generates own span
  └── Result Stage:    SpanID: pipeline-result,   ParentSpanID: pipeline-send
  │  Publish(MessageSent)
  ▼
Subscribers
  ├── AuditSubscriber:    SpanID: subscriber-audit
  ├── MetricsSubscriber:  SpanID: subscriber-metrics
  └── WebhookSubscriber:  SpanID: subscriber-webhook
  ▼
DLR Callback
  │  TraceID: abc-123, SpanID: dlr-callback, ParentSpanID: api-1 (matched via ExternalID)
  │  Publish(MessageDelivered)
```

### Structured Logging

Every log line includes the Trace Context:

```go
logger.With(
    "trace_id",   tc.TraceID,
    "span_id",    tc.SpanID,
    "message_id", msgID,
    "connector_id", connID,
    "tenant_id",  tenantID,
).Info("message sent")
```

### Metrics

Every metric is tagged with Trace Context where appropriate:

```go
// Goal: low cardinality — no trace_id in metric labels
// Use trace_id only for log correlation, not metric aggregation

messageSendDuration.With(prometheus.Labels{
    "connector_id": connID,
    "status":       status,
    "stage":        stageName,
}).Observe(duration.Seconds())
```

### Storage

| Piece | Where |
|-------|-------|
| TraceID | `messages.trace_id` column (DB) |
| Span events | Structured logs (`slog`) |
| Aggregated metrics | Prometheus |
| Trace tree (future) | Jaeger/Zipkin via OpenTelemetry |

## Consequences

### Positive
- Full traceability: API → Queue → Worker → Pipeline → Connector → DLR
- Debugging production issues: filter logs by `trace_id` to see every step
- Performance analysis: per-stage latency histograms
- Multi-tenancy: filter by `tenant_id` in all observability tools

### Negative
- TraceID storage in messages table (extra column, index)
- Structured logging overhead (negligible with `slog`)

### Mitigations
- TraceID index is a B-tree, efficient for point lookups
- `slog` has minimal allocation overhead in Go 1.21+

## Compliance
- **Compatibility Rule**: Protocol X propagates TraceID — no Core changes
- **8-Question Test**: Tracing is interface-injected, not hardcoded

## References
- ARCHITECTURE_PRINCIPLES.md § Observability from Day One
