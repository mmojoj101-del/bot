# Architecture Decision Records

This directory contains Architecture Decision Records (ADRs) for the Fury Communications Platform. Each ADR documents a significant architectural decision, its context, consequences, and compliance with platform principles.

## Index

| # | Title | Status | Date |
|---|-------|--------|------|
| 0000 | [Architecture Vision](ADR-0000-architecture-vision.md) | ✅ Accepted | 2026-07-06 |
| 0001 | [Event-Driven Core](ADR-0001-event-driven-core.md) | ✅ Accepted | 2026-07-06 |
| 0002 | [Canonical Message Model](ADR-0002-canonical-message-model.md) | ✅ Accepted | 2026-07-06 |
| 0003 | [Pipeline Architecture](ADR-0003-pipeline-architecture.md) | ✅ Accepted | 2026-07-06 |
| 0004 | [Connector Registry & Plugin Framework](ADR-0004-connector-registry.md) | ✅ Accepted | 2026-07-06 |
| 0005 | [State Machine & Immutable RoutingDecision](ADR-0005-state-machine-routing-decision.md) | ✅ Accepted | 2026-07-06 |
| 0006 | [Observability & Distributed Tracing](ADR-0006-observability-tracing.md) | ✅ Accepted | 2026-07-06 |
| 0007 | [Event Versioning & Structured Envelope](ADR-0007-event-versioning-envelope.md) | ✅ Accepted | 2026-07-06 |
| 0008 | [Connector Lifecycle & State Management](ADR-0008-connector-lifecycle.md) | ✅ Accepted | 2026-07-06 |
| 0009 | [Capability Negotiation (String-Based)](ADR-0009-capability-negotiation.md) | ✅ Accepted | 2026-07-06 |
| 0010 | [Retry Engine as Independent Service](ADR-0010-retry-engine.md) | ✅ Accepted | 2026-07-06 |
| 0011 | [Scheduler as Independent Component](ADR-0011-scheduler.md) | ✅ Accepted | 2026-07-06 |

## ADR Lifecycle

1. **Proposed** — initial draft, open for discussion
2. **Accepted** — decision made, implementation follows
3. **Deprecated** — superseded by a later ADR
4. **Superseded** — replaced by a newer ADR

## How to Write a New ADR

Each ADR follows a template:

```markdown
# ADR-NNNN: Title

**Status**: Proposed | Accepted | Deprecated | Superseded
**Date**: YYYY-MM-DD
**Deciders**: Names

## Context
Why this decision is needed, what problem it solves, alternatives considered.

## Decision
What was decided, with code examples where appropriate.

## Consequences
Positive and negative outcomes, risks, and mitigations.

## Compliance
How this decision aligns with the Compatibility Rule and 8-Question Architecture Test.

## References
Links to related ADRs, ARCHITECTURE_PRINCIPLES.md, ROADMAP.md, and code.
```

## The 8-Question Architecture Test

Every ADR must implicitly or explicitly address these questions:

1. Can a new protocol be added without modifying the Worker?
2. Can a new provider be added without modifying the Routing Engine?
3. Can a new routing strategy be added without modifying any Connector?
4. Can a new feature (Billing, Rate Limiting) be added as an event subscriber or pipeline stage only?
5. Can every component be tested in isolation using Mocks?
6. Can any single Connector be removed without affecting the rest?
7. Can the system scale horizontally without local assumptions?
8. **Compatibility Rule**: If Protocol X is added tomorrow, would the Worker or Core need changes?
