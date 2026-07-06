# ADR-0007: Event Versioning & Structured Envelope

**Status**: Accepted  
**Date**: 2026-07-06  
**Deciders**: Raghna, Mostafastbot  

---

## Context

As the platform grows, Domain Event schemas will change. New fields will be added, old fields will be deprecated. Without versioning, changing an event schema breaks all existing subscribers. Additionally, every event needs common metadata (trace ID, tenant ID, timestamp) for observability and routing — repeating these fields in every event struct is error-prone and inconsistent.

## Decision

### 1. Event Envelope

Every Domain Event is wrapped in a standard envelope. Only the `Payload` differs between event types:

```go
// EventEnvelope — uniform wrapper for all Domain Events
type EventEnvelope struct {
    EventID       string          `json:"event_id"`       // unique UUID
    EventType     string          `json:"event_type"`     // "message.queued.v1", "message.sent.v2"
    Version       int             `json:"version"`        // 1, 2, 3...
    OccurredAt    time.Time       `json:"occurred_at"`    // when the event happened
    TraceID       string          `json:"trace_id"`       // cross-lifecycle trace
    TenantID      string          `json:"tenant_id"`
    CorrelationID string          `json:"correlation_id"` // groups related events
    Payload       json.RawMessage `json:"payload"`        // the actual event data
    Metadata      map[string]string `json:"metadata,omitempty"` // extensible key-value
}
```

### 2. Event Versioning

Every event type has an explicit version in its name:

```go
const (
    // Message lifecycle events — versioned
    EventTypeMessageQueuedV1     = "message.queued.v1"
    EventTypeMessageClaimedV1    = "message.claimed.v1"
    EventTypeMessageSentV1       = "message.sent.v1"
    EventTypeMessageSentV2       = "message.sent.v2"   // future: added parts info
    EventTypeMessageDeliveredV1  = "message.delivered.v1"
    EventTypeMessageFailedV1     = "message.failed.v1"
    EventTypeMessageRetryingV1   = "message.retrying.v1"
    EventTypeMessageExpiredV1    = "message.expired.v1"

    // Routing events
    EventTypeRouteSelectedV1     = "route.selected.v1"

    // Connector events
    EventTypeConnectorUnavailableV1 = "connector.unavailable.v1"
    EventTypeConnectorHealthyV1     = "connector.healthy.v1"

    // Infrastructure events
    EventTypeCircuitBreakerOpenedV1  = "infra.circuit-breaker.opened.v1"
    EventTypeCircuitBreakerClosedV1  = "infra.circuit-breaker.closed.v1"
)
```

### 3. Event Payloads (Version 1)

```go
// MessageQueuedV1Payload — payload for message.queued.v1
type MessageQueuedV1Payload struct {
    MessageID    string `json:"message_id"`
    TenantID     string `json:"tenant_id"`
    Source       string `json:"source"`
    Destination  string `json:"destination"`
    Parts        int    `json:"parts"`
    ClientRef    string `json:"client_ref,omitempty"`
}

// MessageSentV1Payload — payload for message.sent.v1
type MessageSentV1Payload struct {
    MessageID   string `json:"message_id"`
    ExternalID  string `json:"external_id"`
    ConnectorID string `json:"connector_id"`
    Parts       int    `json:"parts"`
    Price       int64  `json:"price"` // thousandths of a cent
    Cost        int64  `json:"cost"`
}

// MessageDeliveredV1Payload — payload for message.delivered.v1
type MessageDeliveredV1Payload struct {
    MessageID  string `json:"message_id"`
    ExternalID string `json:"external_id"`
    DLRID      string `json:"dlr_id"`
    DoneAt     string `json:"done_at"` // RFC3339
}

// MessageFailedV1Payload — payload for message.failed.v1
type MessageFailedV1Payload struct {
    MessageID    string `json:"message_id"`
    ExternalID   string `json:"external_id,omitempty"`
    ErrorCode    string `json:"error_code"`
    ErrorMessage string `json:"error_message"`
    Attempt      int    `json:"attempt"`
    Retryable    bool   `json:"retryable"`
}
```

### 4. Subscriber Version Handling

```go
// Subscriber checks version before processing
func (s *AuditSubscriber) HandleEvent(ctx context.Context, envelope EventEnvelope) error {
    switch envelope.EventType {
    case EventTypeMessageSentV1:
        var payload MessageSentV1Payload
        json.Unmarshal(envelope.Payload, &payload)
        // handle v1

    case EventTypeMessageSentV2:
        var payload MessageSentV2Payload  // new version with more fields
        json.Unmarshal(envelope.Payload, &payload)
        // handle v2 (or convert to v1 for backward compat)

    default:
        return fmt.Errorf("unknown event version: %s", envelope.EventType)
    }
}
```

### 5. Event Bus Interface (Small Interfaces)

```go
// Publisher — publishes events (Worker uses only this)
type Publisher interface {
    Publish(ctx context.Context, envelope EventEnvelope) error
}

// Subscriber — subscribes to events (Subscribers implement this)
type Subscriber interface {
    Subscribe(ctx context.Context, handler EventHandler) (SubscriptionID, error)
}

// Closer — clean shutdown
type Closer interface {
    Close() error
}

// Composed if needed
type EventBus interface {
    Publisher
    Subscriber
    Closer
}
```

## Consequences

### Positive
- Event schemas can evolve independently (v1 → v2 without breaking subscribers)
- Envelope ensures every event has required metadata (trace, tenant, time)
- Small interfaces are easier to mock and compose
- New subscribers can choose which event versions to handle

### Negative
- Minor overhead: envelope wrapping/unwrapping on each publish
- Version proliferation if schema changes too frequently

### Mitigations
- Envelope overhead is negligible (JSON marshal/unmarshal)
- Version bumps are additive only (new fields, never remove old ones)
- Deprecated versions are supported for at least one major release cycle

## Compliance
- **Compatibility Rule**: Adding a new event version requires zero Worker changes
- **8-Question Test**: Events are interface-versioned, not protocol-versioned

## References
- ADR-0001: Event-Driven Core
- ADR-0006: Observability & Tracing
- ARCHITECTURE_PRINCIPLES.md § Domain Events vs Infrastructure Events
