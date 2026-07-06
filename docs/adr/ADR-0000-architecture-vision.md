# ADR-0000: Architecture Vision — Fury Communications Platform

**Status**: Accepted  
**Date**: 2026-07-06  
**Deciders**: Raghna, Mostafastbot  

---

## Why Does This Project Exist?

To build a **Communications Platform** — not just an SMS Gateway — that:

1. Connects businesses to their customers via **any protocol** (HTTP, SMPP, SIP, and future protocols)
2. Is **rentable** — multi-tenant by design, built for resellers and per-tenant pricing
3. Is **extensible** — new protocols, providers, and features are pluggable without core changes
4. Is **production-verified** — survives crashes, network failures, and high load without data loss
5. Is **observable** — every message is traceable across the full lifecycle

## What Is a Communications Platform?

```
┌─────────────────────────────────────────────────────────────┐
│                    Communications Platform                   │
│                                                             │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌────────────┐ │
│  │  HTTP    │  │  SMPP    │  │   SIP    │  │  Protocol  │ │
│  │Connector │  │Connector │  │Connector │  │     X     │ │
│  └──────────┘  └──────────┘  └──────────┘  └────────────┘ │
│                    │                                        │
│         ┌─────────┴──────────────┐                          │
│         │   Connector Registry   │                          │
│         └─────────┬──────────────┘                          │
│                   │                                          │
│         ┌─────────▼──────────────┐                          │
│         │     Routing Engine     │                          │
│         │  (Capability-Based)    │                          │
│         └─────────┬──────────────┘                          │
│                   │                                          │
│         ┌─────────▼──────────────┐                          │
│         │      Pipeline          │                          │
│         │  Claim → Route → Send  │                          │
│         │  → Handle → Emit       │                          │
│         └─────────┬──────────────┘                          │
│                   │                                          │
│         ┌─────────▼──────────────┐                          │
│         │    Domain Events       │  ←── Event-Driven Core   │
│         │  (MessageQueued, Sent, │                          │
│         │   Delivered, Failed)   │                          │
│         └─────────┬──────────────┘                          │
│                   │                                          │
│    ┌──────────────┼──────────────┬──────────────┐          │
│    ▼              ▼              ▼              ▼           │
│ ┌──────┐   ┌────────┐   ┌──────────┐   ┌──────────────┐   │
│ │Audit │   │Billing │   │ Metrics  │   │ Webhooks     │   │
│ └──────┘   └────────┘   └──────────┘   └──────────────┘   │
│                                                             │
│  All of these are pluggable event subscribers               │
└─────────────────────────────────────────────────────────────┘
```

## Why Clean Architecture?

```
Domain ────────┬──── Service ────────┬──── Repository / Connector
               │                     │
         Pure Go types          Interface boundaries
         No frameworks          Implementation injected
         No databases      
```

| Layer | Contains | Knows About |
|-------|----------|-------------|
| **Domain** | Message, Route, Connector, Events, State Machine, Value Objects | Nothing — pure Go types |
| **Service** | Orchestration, Validation, Business Rules | Domain interfaces only |
| **Runtime** | Worker, Pipeline, Registry, Event Bus, Scheduler | Domain interfaces + injected implementations |
| **Infrastructure** | PostgreSQL, Redis, HTTP client, SMPP library | Domain interfaces — implements them |

## Why Event-Driven?

- **Worker is an Executor, not an Orchestrator** — it claims messages and publishes events
- **Features are subscribers** — audit, billing, metrics, webhooks all react to events
- **Worker never changes** when new features are added

```
Worker → Publish(MessageSent)
         ├── AuditSubscriber
         ├── BillingSubscriber
         ├── MetricsSubscriber
         └── WebhookSubscriber
```

## Why Canonical Message Model?

One single model inside the Core. Connectors map to/from it:

```
HTTP JSON → HTTPConnector.ParseResponse() → CanonicalMessage
SMPP PDU → SMPPConnector.ParseResponse() → CanonicalMessage
SIP INVITE → SIPConnector.ParseResponse() → CanonicalMessage
```

The Core never sees protocol-specific formats.

## Why Plugin Architecture?

```
New Protocol = implement Connector interface + register in Registry
New Provider = implement ProviderAdapter (for HTTP) or session config (for SMPP)
New Strategy = new file in routing package
New Feature = new event subscriber
```

**Zero Core changes for any of these.**

## Key Architectural Decisions

| Decision | Rationale |
|----------|-----------|
| Event-Driven Core | Worker never changes; features = subscribers |
| Canonical Message Model | Core is protocol-agnostic by construction |
| Pipeline Architecture | Features = new stages, no worker changes |
| Pluggable Connectors | New protocol = new package |
| Capability-Based Routing | Messages matched to connectors by capability |
| Immutable RoutingDecision | Audit trail, no accidental mutation |
| Centralized State Machine | All connectors map to canonical states |
| Outbox Pattern | Guaranteed event delivery |
| Horizontal Scaling | Stateless workers, queue-based coordination |

## References
- ADR-0001: Event-Driven Core
- ADR-0002: Canonical Message Model
- ADR-0003: Pipeline Architecture
- ADR-0004: Connector Registry
- ADR-0005: State Machine & RoutingDecision
- ADR-0006: Observability & Tracing
- ARCHITECTURE_PRINCIPLES.md
- ROADMAP.md
