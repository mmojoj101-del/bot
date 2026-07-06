# Architecture & Design Principles — Fury Communications Platform

> This document captures the architectural mindset and design principles that must guide every phase of development.  
> **Read before starting any new Phase.** Update when a new architectural insight is discovered.

---

## 🧠 Core Mindset

This is a **Communications Platform**, not just an SMS Gateway.  
Every decision in Phase 2.5 must pave the way for Phases 2.6–3.  
**No redesigning the core when SMPP or SIP arrive.**

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

### 5. Architecture Test — 7 Questions
Before declaring any Phase "Done", ask:

| # | Question | Pass Condition |
|---|----------|---------------|
| 1 | Can a new protocol be added without modifying the Worker? | Yes → new package only |
| 2 | Can a new provider be added without modifying the Routing Engine? | Yes → adapter only |
| 3 | Can a new routing strategy be added without modifying any Connector? | Yes → new file only |
| 4 | Can a new feature (Billing, Rate Limiting) be a pipeline stage only? | Yes → hook/stage only |
| 5 | Can every component be tested in isolation using Mocks? | Yes → interface-injected |
| 6 | Can any single Connector be removed without affecting the rest? | Yes → registry pattern |
| 7 | Can the system scale horizontally without local assumptions? | Yes → no in-memory state that can't be rebuilt |

**If any answer is "No", the architecture needs adjustment before proceeding.**

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
