# Architecture & Design Principles — Fury Communications Platform

> This document captures the architectural mindset and design principles that must guide every phase of development.  
> Read before starting any new Phase. Update when a new architectural insight is discovered.

---

## 🧠 Core Mindset

This is a **Communications Platform**, not just an SMS Gateway.  
Every decision in Phase 2.5 must pave the way for Phases 2.6–3.  
**No redesigning the core when SMPP or SIP arrive.**

---

## 🔐 Hard Constraints

### Protocol Agnostic Worker
The Worker **must not know** HTTP, SMPP, or SIP. All interaction is through interfaces and abstractions only. Any future Connector must plug in without modifying the Worker.

### Routing Engine Separation (Phase 2.6 — plan now)
From the start, define what information the Worker needs from the Routing Engine:
```
Worker asks: "Which route + connector for this message?"
Routing Engine answers: { Route, ConnectorID }
```
Worker is not coupled to how the route is chosen — static, round-robin, failover, weighted, or future strategies.

### Connector Framework (Phase 2.7 — plan now)
The Worker only knows three methods:
```go
Connector.Send(ctx, req) → (*SendResult, error)
Connector.Health(ctx)    → error
Connector.Close()        → error
```
All protocol-specific and provider-specific details live **inside the Connector only**.

### HTTP Connectors (Phase 2.8 — pave the way)
The current design must allow:
- Generic HTTP (JSON, Form, XML)
- Provider Adapters (Twilio, Vonage, Infobip, Plivo, AWS SNS)
- Webhooks / DLR
- Multiple authentication types
All without Core changes.

### SMPP (Phase 2.9 — no false assumptions)
The Worker must assume sending can be:
- Persistent Session (not one-shot)
- Windowing (multiple outstanding requests)
- Async Responses (submit_sm response vs deliver_sm DLR)
- Delayed DLR (minutes/hours later)
- Automatic Reconnect (session drops and recovers)
The Worker must not assume send = request → response only.

### SIP (Phase 2.10 — session-based design)
Design must support **Session-Based protocols**, not just request/response.  
SIP should later be just another Connector — zero Core changes.

---

## 📐 Execution Rules

| Rule | Description |
|------|-------------|
| **Interface-first always** | Dependencies depend on interfaces, never concretions |
| **Dependency Injection everywhere** | No `init()`, no global constructors |
| **No Global State** | Except infrastructure registries (Prometheus, ConnectorRegistry) — and even those should be injected |
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

### 5. Required Checks
```bash
go build ./...
go test ./...
go vet ./...
# (SMPP phase onwards) go test -race ./...
# Docker smoke test
```

### 6. Phase Report
For every completed phase, produce a report containing:
- **What was built** — summary of the component(s)
- **Architectural decisions** — each non-trivial decision with rationale
- **Risks or Technical Debt** — if any, with a plan to resolve
- **Architecture health check** — does the design still satisfy the ultimate goal?
- **Test results** — all pass/fail counts

---

## 🎯 Ultimate Goal — The Test

The platform passes if:

1. A new protocol (e.g., XMPP, WhatsApp Business API) can be added as a **single new package** implementing the `Connector` interface — no changes to Worker, Routing Engine, or Core
2. A new provider (e.g., Twilio) can be added as a **small adapter** implementing `ProviderAdapter` — no changes to the HTTP Connector or Core
3. A new routing strategy (e.g., geographic, latency-based) can be added as a **new file** in the routing package — no changes to Worker or existing strategies
4. The system can be **rented to multiple tenants** — no single-tenant assumptions in any layer
5. The system survives **production chaos** — crashes, network splits, DB failover — without data loss or silent corruption
