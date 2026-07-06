# Architecture & Design Principles — Fury Communications Platform

> This document captures the architectural mindset and design principles that must guide every phase of development.  
> **Read before starting any new Phase.** Update when a new architectural insight is discovered.

---

## 🧠 Core Mindset

This is a **Communications Platform**, not just an SMS Gateway.  
Every decision in Phase 2.5 must pave the way for Phases 2.6–3.  
**No redesigning the core when SMPP or SIP arrive.**

---

## 🎯 Event-Driven Core (Not Worker-Driven)

The platform is **Event-Driven**, not Worker-Driven.  
The Worker is just an **Executor** — the real engine is Domain Events.

### The Shift

```
❌ Worker-Driven (old mindset):
   Worker → Route → Send → Update DB → Audit → Metrics → Webhook

✅ Event-Driven (platform mindset):
   Worker → Publish(MessageSent)
              ├── AuditSubscriber    (logs the event)
              ├── BillingSubscriber  (deducts credits)
              ├── MetricsSubscriber  (records latency)
              ├── WebhookSubscriber  (calls customer callback)
              ├── AnalyticsSubscriber(stores for dashboard)
              └── ...any future feature = new subscriber
```

### Domain Event Publisher Interface

The Worker must not know the Event Bus implementation. It depends on an interface:

```go
type DomainEventPublisher interface {
    Publish(ctx context.Context, event DomainEvent) error
}
```

The Worker calls `publisher.Publish(ctx, MessageSent{...})` — it never knows if the implementation is:
- In-memory (`MemoryBus`)
- Outbox-pattern (PostgreSQL `outbox_events` table)
- Kafka / NATS / RabbitMQ

This decouples domain logic from infrastructure. Swap the implementation without touching the Worker.

### Every Step is an Event

Instead of a monolithic Worker calling services, each lifecycle step produces a **Domain Event**:

```
MessageQueued
  │ ▼ Queue Worker claims
MessageClaimed
  │ ▼ Routing completed
RoutingCompleted  ← includes the immutable RoutingDecision
  │ ▼ Connector selected
ConnectorSelected
  │ ▼ Send started
SendStarted
  │ ▼ Send completed (success or failure)
SendSucceeded | SendFailed
  │ ▼ Delivery report received (may be minutes later)
DeliveryReportReceived  → MessageDelivered | MessageFailed
  │ ▼ Retry path
RetryScheduled
  │ ▼ Terminal state
MessageCompleted
```

### Worker Does Not Call Services — It Publishes Events

```go
// ❌ Bad: Worker calls services directly
func (w *Worker) process(msg *Message) {
    w.routeService.Select(ctx, msg)      // tight coupling
    w.billingService.Deduct(ctx, msg)     // worker knows about billing
    w.auditService.Log(ctx, msg)          // worker knows about audit
    w.metricsService.Record(ctx, msg)     // worker knows about metrics
}

// ✅ Good: Worker publishes events, subscribers react
func (w *Worker) process(msg *Message) {
    result, err := w.connector.Send(ctx, msg)
    if err != nil {
        w.eventBus.Publish(ctx, MessageFailed{MessageID: msg.ID, Error: err})
        return
    }
    w.eventBus.Publish(ctx, MessageSent{MessageID: msg.ID, ExternalID: result.ExternalID})
    // That's it. Subscribers handle audit, billing, metrics, webhooks.
}
```

Adding a new feature = writing a new subscriber. **The Worker never changes.**

### Domain Events vs Infrastructure Events

These are **separate concerns** with separate event channels:

| Domain Events (Business) | Infrastructure Events (Operations) |
|--------------------------|-----------------------------------|
| `MessageQueued` | `WorkerStarted` |
| `MessageClaimed` | `WorkerStopped` |
| `MessageSent` | `DBReconnected` |
| `MessageDelivered` | `RedisUnavailable` |
| `MessageFailed` | `CircuitBreakerOpened` |
| `ConnectorUnavailable` | `CircuitBreakerHalfOpened` |
| `RouteSelected` | `CircuitBreakerClosed` |
| `RetryScheduled` | `ConnectorHealthChanged` |
| `Expired` | `QueueDepthAlert` |

- **Domain Events** → published on `eventBus.Bus` (in-process or NATS/Kafka)
- **Infrastructure Events** → logged + metrics, optionally alert (PagerDuty/Prometheus Alertmanager)

### Single Centralized State Machine

There is **one** state machine for the entire platform. All Connectors map their results to it:

```
Created → Queued → Claimed → Routing → Prepared → Sending → Sent → WaitingDLR → Delivered
                                                                                → Failed
                                     → Failed (reject before send)
                                     → Retrying → Queued (loop)
                                     → Expired (max retries exceeded)
                                     → Cancelled (admin action)
```

Every Connector translates its protocol-specific result to this canonical state:
- HTTP 200 → `Sent`
- SMPP submit_sm_resp → `Sent`
- SMPP deliver_sm (DELIVRD) → `Delivered`
- SIP 200 OK (INVITE) → `Sent`
- Provider error → `Failed` with error code
- Timeout → `Retrying`

### Retry Policy is Independent

Retry is **Business Logic**, not Protocol Logic. It does not live inside Connectors:

```
Connector.Send() → SendResult{Success, ErrorCode}
                          ↓
                   RetryEngine.Decide(result, attempt, maxRetries)
                          ↓
              ┌─── Retry ───┴─── Terminal Failure ───┐
              ↓                                       ↓
        Publish(RetryScheduled)              Publish(MessageFailed)
```

Retry policy can differ per tenant, per route, per message priority — without touching Connectors.

### RoutingDecision is Immutable

Once the Routing Engine selects a connector, the full decision is recorded as an **immutable value object**:

```go
type RoutingDecision struct {
    RouteID         string
    ConnectorID     string
    StrategyUsed    string        // static, round_robin, failover, weighted
    Priority        int
    Cost            int64         // at selection time (thousandths of a cent)
    Reason          string        // why this route was chosen
    CapabilitiesUsed []string     // which capabilities were matched
    SelectedAt      time.Time
}
```

This is critical for:
- **Audit**: know exactly why a message went through a specific connector
- **Diagnostics**: trace routing decisions when debugging delivery issues
- **Billing**: record the cost at routing time (not send time)
- **Analytics**: understand routing patterns over time

### Capability-Based Routing

The Routing Engine doesn't match "Route A" vs "Route B". It matches **message requirements** against **connector capabilities**:

```
Message Requirements:
  • Unicode: true (text contains non-GSM7 chars)
  • DLR: true (requested delivery receipt)
  • Multipart: false (fits in single SMS)
  • Session: not required
  • Priority: high
        ↓
  Routing Engine matches against connector Capabilities
        ↓
  Returns: {ConnectorID: "...", Reason: "supports Unicode + DLR", CapabilitiesUsed: ["unicode", "dlr"]}
```

This means:
- Adding a new connector = it advertises its capabilities
- The Routing Engine automatically considers it for matching messages
- No routing rules need to be updated manually
- Messages that can't be satisfied by any connector → `MessageFailed` with `"no_suitable_connector"`

### Compatibility Rule

**Every commit must pass this test:**

> If we add Protocol X tomorrow (not HTTP, not SMPP, not SIP), would we need to modify the Worker or Core?

If the answer is **yes**, the architecture needs review before committing.

Examples of Protocol X: WhatsApp Business API, Apple Push Notification, Telegram Bot API, XMPP, Amazon SNS, custom WebSocket protocol.

### Testing Philosophy

```
70%  Unit Tests      — each component in isolation with mocks
20%  Integration Tests — real DB + real event bus + mocked connectors
10%  End-to-End Tests  — full Docker stack, API → Queue → Worker → Connector → DLR
```

**Mock every interface in the Core.**
If a component can't be unit-tested with mocks, it has a design problem.

### Definition of Success — Phase 2.5

> The Worker becomes a **generic component** that knows nothing about any protocol or provider.
> It can be used as-is with **any new Connector** that implements the defined interfaces.
> New platform features (billing, rate limiting, compliance) are added as **event subscribers** — no Worker changes.
> New routing strategies are added as **new strategy files** — no Worker or Connector changes.

When this is true, you've built the foundation for a **Communications Platform**, not just an SMS Gateway.

---

## 🔐 Hard Constraints

### 1. Protocol Agnostic Worker
The Worker **must not know** HTTP, SMPP, or SIP. All interaction is through interfaces and abstractions only. Any future Connector must plug in without modifying the Worker.

### 2. Routing Engine Separation
```
Worker asks: "Which route + connector for this message?"
Routing Engine answers: { Route, ConnectorID, Capabilities }
```
Worker is not coupled to **how** the route is chosen — static, round-robin, failover, weighted, or future strategies.

### 3. Connector Framework
The Worker only knows three methods:
```go
type Connector interface {
    Send(ctx context.Context, req *SendRequest) (*SendResult, error)
    Health(ctx context.Context) error
    Close() error
}
```
All protocol-specific and provider-specific details live **inside the Connector only**.

### 4. Connector Capabilities
The Worker must not assume every Connector supports the same features.  
Each Connector publishes its capabilities for the Routing Engine to match:

```go
type Capabilities struct {
    SupportsDLR       bool  // delivery receipts callbacks
    SupportsMultipart bool  // long messages split into parts
    SupportsUnicode   bool  // UCS2 / UTF-8 encoding
    SupportsAsync     bool  // non-blocking send (submit → later receipt)
    SupportsSession   bool  // persistent session (SMPP bind, SIP register)
    SupportsMedia     bool  // MMS, images, audio, video
    SupportsInbound   bool  // receive messages from network
    MaxThroughput     int   // messages per second ceiling
    MaxMessageLength  int   // per-part character limit
    Protocols         []string  // "http", "smpp", "sip"
}
```

The Routing Engine uses capabilities to select the right Connector for each message.  
A Unicode message → Connector with `SupportsUnicode=true`.  
A long message → Connector with `SupportsMultipart=true`.

### 5. Canonical Message Model
One single internal Message model across the entire platform.  
Every Connector maps **FROM** Canonical → Protocol-specific and **TO** Canonical ← Protocol-specific.

```
External (HTTP JSON / SMPP PDU / SIP INVITE)
          ↓                          ↑
    Connector Adapter           Connector Adapter
          ↓                          ↑
     ┌──────────────────────┐
     │  Canonical Message   │  ← Core knows ONLY this model
     └──────────────────────┘
```

The Core never knows:
- SMPP PDU (submit_sm, deliver_sm, data_sm)
- SIP INVITE / BYE / REGISTER
- HTTP JSON body from any provider

### 6. Pipeline Architecture
Instead of one monolithic Worker, the message lifecycle is a **Pipeline of pluggable stages**:

```
Claim → Validate → Route → Prepare → Send → Handle Result → Retry/Complete → Emit Events
```

Each stage is:
- Self-contained (single responsibility)
- Pluggable (add/remove/reorder without changing other stages)
- Testable in isolation (mock adjacent stages)
- Observable (trace ID + metrics per stage)

Adding new platform features = adding pipeline stages:
```
Rate Limiting → add a stage after Claim
Billing       → add a stage after Route
Fraud Detect  → add a stage after Validate
Compliance    → add a stage after Prepare
```

### 7. Inbound from the Start
Define interfaces for inbound events even before implementing them:

```go
type InboundHandler interface {
    HandleMessage(ctx context.Context, msg *InboundMessage) error
    HandleDLR(ctx context.Context, dlr *InboundDLR) error
    HandleSessionEvent(ctx context.Context, evt *SessionEvent) error
}

type InboundMessage struct {
    ConnectorID string
    Source      string
    Destination string
    Text        string
    ReceivedAt  time.Time
    Raw         json.RawMessage  // protocol-specific payload for debugging
}

type InboundDLR struct {
    ConnectorID  string
    ExternalID   string
    Status       MessageStatus
    ErrorCode    string
    DeliveredAt  *time.Time
}

type SessionEvent struct {
    ConnectorID string
    EventType   string  // connected, disconnected, reconnecting, rebound
    Timestamp   time.Time
    Error       string
}
```

HTTP (webhook), SMPP (deliver_sm), and SIP (incoming INVITE) all need these interfaces.

### 8. Extensibility Hooks
A Hook System that allows adding behaviour at key lifecycle points **without modifying the Worker**:

```go
type Hook interface {
    BeforeSend(ctx context.Context, msg *Message, connector Connector) error
    AfterSend(ctx context.Context, result *SendResult) error
    BeforeRetry(ctx context.Context, msg *Message, attempt int) error
    AfterRetry(ctx context.Context, msg *Message, attempt int, result *SendResult) error
    OnDelivered(ctx context.Context, msg *Message) error
    OnFailed(ctx context.Context, msg *Message, err error) error
}

type HookRegistry struct {
    hooks []Hook  // injected via DI, extensible by adding more implementations
}
```

Hooks enable features like:
- Content filtering (BeforeSend → check/block content)
- Rate limit enforcement (BeforeSend → check token bucket)
- Billing deduction (AfterSend → deduct credits)
- Analytics (OnDelivered → increment delivery counter)
- Compliance logging (BeforeSend/AfterSend → archive payload)

### 9. Versioned Interfaces
If an interface grows beyond 3-4 methods, **split it** or **version it**:

```go
// v1 — minimal
type ConnectorV1 interface {
    Send(ctx, req) (*SendResult, error)
    Health(ctx) error
    Close() error
}

// v2 — adds capabilities
type ConnectorV2 interface {
    ConnectorV1
    Capabilities() Capabilities
}

// or better: compose small interfaces
type Sender interface { Send(ctx, req) (*SendResult, error) }
type HealthChecker interface { Health(ctx) error }
type Closer interface { Close() error }
type CapabilityReporter interface { Capabilities() Capabilities }
```

Small interfaces are preferable: a Connector implements only what it needs.

### 10. Observability from Day One

```go
type TraceContext struct {
    TraceID    string  // spans the full lifecycle: API → Queue → Worker → Connector → DLR
    SpanID     string  // per-pipeline-stage
    ParentSpan string
}

// Already have RequestID — extend it to CorrelationID that flows through:
// API handler → Message.Create → Queue → Pipeline stages → Connector → DLR callback
```

| Requirement | Current State | Action Needed |
|-------------|--------------|---------------|
| Trace ID / Correlation ID | `request_id` in middleware | Propagate through Queue → Worker → Pipeline → Connector |
| Structured Logging | `slog` with key-value pairs | Ensure trace_id, message_id, connector_id in every log line |
| Per-stage Metrics | Prometheus counters for worker | Add per-pipeline-stage latency histograms |
| End-to-end Tracking | Message ID tracked by status | Ensure trace_id visible in /ready and audit logs |

### 11. Plugin Mindset (Connector Registry)
Design the Connector Registry as if connectors could be loaded dynamically, even if everything is built-in initially:

```go
type ConnectorRegistry struct {
    connectors map[string]Connector  // keyed by connector_id
    mu         sync.RWMutex
}

func (r *ConnectorRegistry) Register(id string, c Connector) { ... }
func (r *ConnectorRegistry) Get(id string) (Connector, bool) { ... }
func (r *ConnectorRegistry) Unregister(id string) { ... }
func (r *ConnectorRegistry) List() []ConnectorInfo { ... }
```

This pattern:
- Makes testing trivial (register mock connectors)
- Allows hot-reload of connector configs
- Prepares for future dynamic plugin loading
- No global state — registry is injected via DI

---

## 🔁 Pipeline Stage Interface

Every pipeline stage follows this contract:

```go
type Stage interface {
    Name() string
    Process(ctx context.Context, msg *PipelineMessage) (*PipelineMessage, error)
}

type PipelineMessage struct {
    Message      *domain.Message
    Route        *domain.Route
    ConnectorID  string
    SendResult   *SendResult
    Error        error
    Attempt      int
    MaxRetries   int
    TraceID      string
    // Stage can attach metadata for downstream stages
    Metadata     map[string]interface{}
}
```

The Pipeline is a slice of Stages executed in order:

```go
type Pipeline struct {
    stages []Stage
}

func (p *Pipeline) Execute(ctx context.Context, msg *PipelineMessage) error {
    for _, stage := range p.stages {
        var err error
        msg, err = stage.Process(ctx, msg)
        if err != nil {
            return fmt.Errorf("pipeline stage %s: %w", stage.Name(), err)
        }
    }
    return nil
}
```

---

## 📐 Execution Ground Rules (Before Phase 2.5 Code)

### 1. Transaction Boundaries

| Operation | Transaction? | Scope |
|-----------|-------------|-------|
| Claim message from queue | ✅ Yes | 1 TX — FOR UPDATE SKIP LOCKED + status update |
| Update state + write outbox event | ✅ Yes | 1 TX — atomic: state change + event written together |
| HTTP/SMPP/SIP Send | ❌ No | Outside TX — network I/O must not hold DB locks |
| Publish event to bus | ❌ No | After TX commits — no I/O inside transactions |
| Retry scheduling | ✅ Yes | 1 TX — insert scheduled_task |

**Rule**: Network calls (send, HTTP request, SMPP PDU) are always outside DB transactions.
Transactions are only for DB state mutations.

### 2. Failure Taxonomy

Every error returned by any component must be classified:

| Category | Example | Retry? |
|----------|---------|--------|
| **Permanent** | Invalid phone number, blocked sender | ❌ No |
| **Retryable** | Provider returned "try again", timeout | ✅ Yes, with backoff |
| **Transient** | Network timeout, DNS failure | ✅ Yes, immediate retry |
| **RateLimited** | HTTP 429, SMPP throttling | ✅ Yes, with longer backoff |
| **Authentication** | Wrong API key, expired token | ❌ No (until re-authorized) |
| **Misconfiguration** | Invalid URL, missing field | ❌ No (until config fixed) |
| **Internal** | DB deadlock, nil pointer | ✅ Yes (with alerting) |

```go
type ErrorCategory string

const (
    ErrorPermanent       ErrorCategory = "permanent"
    ErrorRetryable       ErrorCategory = "retryable"
    ErrorTransient       ErrorCategory = "transient"
    ErrorRateLimited     ErrorCategory = "rate_limited"
    ErrorAuthentication  ErrorCategory = "authentication"
    ErrorMisconfiguration ErrorCategory = "misconfiguration"
    ErrorInternal        ErrorCategory = "internal"
)

type SendError struct {
    Category    ErrorCategory
    Code        string    // provider error code
    Message     string    // human-readable
    RetryAfter  *time.Duration // for rate-limited errors
}
```

### 3. Idempotency Policy

| Property | Value |
|----------|-------|
| **Key** | `tenant_id + client_ref` (unique index, partial `WHERE client_ref IS NOT NULL`) |
| **Scope** | Per-tenant — no cross-tenant collision |
| **TTL** | Never deleted (historical record). Duplicate key → return existing message |
| **Worker restart** | Messages in `queued` status are re-claimed after restart. `client_ref` prevents re-creating the same message |
| **Crash recovery** | Outbox pattern ensures events are written atomically with state changes. If crash occurs after send but before event: last status in DB is `sent`, outbox_event is re-published on recovery |
| **DLR idempotency** | `connector_id + external_id` unique check. Version-based optimistic locking prevents double-DLR updates |

### 4. Command vs Event Separation

Commands and Events are **different things** with different naming and behavior:

```go
// Commands — imperative, directed at a specific handler
// Convention: Verb + Noun + Command
type SendMessageCommand struct {
    MessageID     string
    TenantID      string
    ConnectorID   string
    ScheduledFor  *time.Time  // optional delay
}

// Events — past tense, broadcast to all subscribers
// Convention: Noun + PastTenseVerb + Event, versioned
// Payload is in the envelope, not the struct name
const EventTypeMessageQueuedV1 = "message.queued.v1"
const EventTypeMessageSentV1  = "message.sent.v1"
```

**Rules**:
- Commands go to a **specific handler** (e.g., Pipeline.Execute)
- Events go to **all subscribers** via EventBus
- Never name an event like a command (`MessageSend` ❌ → `MessageSent` ✅)
- Never name a command like an event (`MessageSentCommand` ❌ → `SendMessageCommand` ✅)

### 5. Compatibility Matrix

| Component | Versioned? | Breaking Change Policy |
|-----------|-----------|----------------------|
| **Core** (Domain models, State Machine) | SemVer (v0.x.y) | Major version for breaking changes |
| **Connector Interface** (`Send/Health/Close`) | Small interfaces, additive | New methods = new interface (never change existing) |
| **Connector Implementations** (HTTP, SMPP, SIP) | Per-connector | No impact on Core — isolated behind interface |
| **Provider Adapters** | Per-provider | Adapter cannot break Core or HTTP Connector |
| **Event Versions** | `message.queued.v1` | New versions are additive. Old subscribers continue working with old versions |
| **API Versions** | `/api/v1/` | New version = new route prefix. Old versions deprecated, not removed |
| **DB Schema** | Migration files | Additive changes (new columns, new tables). Breaking changes = new migration + data migration |

### 6. Performance Targets (SLOs)

Measurable goals — not general statements:

| Metric | Target | Measurement |
|--------|--------|-------------|
| Claim latency | < 50ms P99 | Time from poll to successful claim (FOR UPDATE SKIP LOCKED) |
| Pipeline execution | < 500ms P99 (excluding send) | Time through all pipeline stages |
| HTTP send latency | < 5s P99 | Time from `Connector.Send()` to response |
| SMPP submit_sm latency | < 1s P99 | Time for submit_sm → submit_sm_resp |
| Throughput per worker | 50 msg/s sustained (HTTP), 100 msg/s (SMPP) | Messages processed per second per worker instance |
| Max retry delay | 300s (5 min) | Maximum time between a failure and next retry attempt |
| Connector recovery | < 30s | Time from failure detection to automatic recovery |
| Graceful shutdown | < 30s | All inflight messages complete or returned to queue |
| DLR processing | < 100ms P99 | Time from DLR receipt to message status update |

These SLOs are verified in Phase 3 (Production Validation).

---

## 📐 Horizontal Scaling from Day One

The system must support **horizontal scaling** without shared memory. Every component must be designed as if multiple instances are running simultaneously.

### Design Principles for Scale

| Principle | Implementation |
|-----------|---------------|
| **Stateless Workers** | Workers hold no in-memory state that can't be rebuilt. All state is in PostgreSQL or Redis |
| **Queue-based coordination** | Messages claimed via `FOR UPDATE SKIP LOCKED` — workers don't compete for the same message |
| **Distributed locks** | Only where absolutely necessary (e.g., SMPP session ownership). Use Redis `SET NX EX` or PostgreSQL advisory locks |
| **No leader election** | Every worker is equal. No master/slave assumption |
| **Shared nothing** | No in-process channels between workers. Coordination is through the database or message broker |
| **Idempotent subscribers** | Event subscribers handle duplicate events safely (UPSERT, version check) |
| **Caches are local** | In-memory caches (route tables, templates) are warmable on startup. Loss of cache is not a failure |

### What NOT to Do

```
❌ In-process channel for worker coordination
❌ Global variables that assume single instance
❌ Mutexes that protect single-instance assumptions
❌ Local file system for persistence
❌ In-memory queues with no persistence
```

### Example: 10 Workers Running

```
PostgreSQL Queue (messages table)
  │ 10 workers all polling with FOR UPDATE SKIP LOCKED
  │ Only 1 worker claims each message (SKIP LOCKED)
  ▼
Worker-3 claims message-42
  │ Publishes MessageClaimed
  │ Pipeline executes (Validate → Route → Send → Complete)
  │ Updates message status to sent/failed
  │ Publishes MessageSent/MessageFailed
  ▼
All 10 workers receive the event via event bus
  │ Only subscribers react (audit, billing, metrics are all idempotent)
  ▼
Message delivered. Worker-3 is free to claim the next message.
```

## 📐 Execution Rules

| Rule | Description |
|------|-------------|
| **Interface-first always** | Dependencies depend on interfaces, never concretions |
| **Dependency Injection everywhere** | No `init()`, no global constructors |
| **No Global State** | Except infrastructure registries (Prometheus registry) — injected, not global |
| **Component Independence** | Every component must be testable in isolation (mock all dependencies) |
| **No Shortcuts or Temporary Hacks** | If a compromise is necessary, document it explicitly before implementing |
| **Document Before Execution** | If a design decision affects future phases, write it down before writing code |

---

## 🔁 Per-Phase Review Checklist

After completing each Phase, perform a structured review:

### 1. Self Code Review
- Does the code follow existing patterns? (naming, error handling, logging, structure)
- Are interfaces clean and minimal?
- Is there any unnecessary coupling?

### 2. Performance & Concurrency Review
- Are goroutines properly managed? (WaitGroup, context cancellation, panic recovery)
- Are locks fine-grained enough? (no global mutexes)
- Is there any goroutine leak risk?
- Are channel operations bounded and non-blocking where possible?

### 3. Scalability Review
- Can this component handle 10x the load?
- Are there any O(n) operations in hot paths?
- Is database access properly indexed and paginated?
- Is memory usage bounded?

### 4. Pluggability Review
- Can a new protocol be added without modifying this component?
- Can a new provider be added without modifying this component?
- Can a new routing strategy be added without modifying this component?

### 5. Architecture Test — 8 Questions
Before declaring any Phase "Done", ask:

| # | Question | Pass Condition |
|---|----------|---------------|
| 1 | Can a new protocol be added without modifying the Worker? | Yes → new package only |
| 2 | Can a new provider be added without modifying the Routing Engine? | Yes → adapter only |
| 3 | Can a new routing strategy be added without modifying any Connector? | Yes → new file only |
| 4 | Can a new feature (Billing, Rate Limiting, Compliance) be added as an event subscriber or pipeline stage without touching the Worker? | Yes → subscriber/stage only |
| 5 | Can every component be tested in isolation using Mocks? | Yes → interface-injected |
| 6 | Can any single Connector be removed without affecting the rest? | Yes → registry pattern |
| 7 | Can the system scale horizontally without local assumptions? | Yes → no in-memory state that can't be rebuilt |
| 8 | **Compatibility Rule**: If Protocol X is added tomorrow, would the Worker or Core need changes? | **No** → architecture is sound. If Yes → redesign needed. |

**If any answer violates the pass condition, the architecture needs adjustment before proceeding.**

### 6. Required Checks
```bash
go build ./...
go test ./...
go vet ./...
# (from SMPP phase onwards) go test -race ./...
# Docker smoke test
```

### 7. Phase Report
For every completed phase, produce a report containing:
- **What was built** — summary of the component(s)
- **Architectural decisions** — each non-trivial decision with rationale
- **Risks or Technical Debt** — if any, with a plan to resolve
- **Architecture health check** — does the design still satisfy the ultimate goal?
- **7-question architecture test results**
- **Test results** — all pass/fail counts

---

## 📋 Connector Implementation Matrix

| Feature | Phase 2.8 HTTP | Phase 2.9 SMPP | Phase 2.10 SIP |
|---------|---------------|----------------|----------------|
| Send | HTTP Request | submit_sm | INVITE |
| SupportsDLR | ✓ (callback URL) | ✓ (deliver_sm) | ✓ (SIP NOTIFY?) |
| SupportsMultipart | N/A (done by adapter) | ✓ (UDH + SAR) | N/A |
| SupportsUnicode | ✓ | ✓ (UCS2) | N/A |
| SupportsAsync | ✓ (202 Accepted) | ✓ (window + receipt) | ✓ (session-based) |
| SupportsSession | ✗ (stateless) | ✓ (bound session) | ✓ (dialog) |
| SupportsInbound | ✓ (webhooks) | ✓ (deliver_sm) | ✓ (INVITE) |
| Auth | Bearer/Basic/API Key | system_id/password | Digest MD5 |
| Retry | Configurable codes | Auto-reconnect | Failover trunk |
| DLR | Callback to platform | deliver_sm fields | INFO/NOTIFY |

---

## 🎯 Ultimate Goal — The Test

The platform passes if:

1. A new protocol (e.g., XMPP, WhatsApp Business API) can be added as a **single new package** implementing the `Connector` interface — no changes to Worker, Routing Engine, or Core
2. A new provider (e.g., Twilio) can be added as a **small adapter** implementing `ProviderAdapter` — no changes to the HTTP Connector or Core
3. A new routing strategy (e.g., geographic, latency-based) can be added as a **new file** in the routing package — no changes to Worker or existing strategies
4. A new platform feature (e.g., billing, rate limiting, compliance logging) can be added as a **pipeline stage or hook** — no changes to Worker or Connectors
5. The system can be **rented to multiple tenants** — no single-tenant assumptions in any layer
6. The system survives **production chaos** — crashes, network splits, DB failover — without data loss or silent corruption
