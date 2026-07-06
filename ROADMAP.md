# Fury SMS Gateway — Development Roadmap

> **Current Status**: Core Platform Complete (Phase 2.4)  
> **Tag**: `v0.1.1-phase2.4`  
> **Next**: Phase 2.5 — Worker Engine

---

## ✅ Core Platform (Complete) — v0.1.1

### Authentication & JWT
- Login, Refresh, Logout, Switch-Tenant
- Access token (15m) + Refresh token (30d) rotation
- HMAC-SHA256 API key hashing

### Multi-Tenancy
- Tenants CRUD via `tenant_members` (no circular FK)
- Per-tenant RBAC (admin, operator, viewer)
- Tenant context middleware

### API Keys
- 48-char `fx_` prefixed keys
- HMAC-SHA256 storage, prefix-based lookup
- Per-tenant scoping

### Connectors CRUD — `/api/v1/connectors`
- HTTP/SMPP/SIP types via `ConnectorType` enum
- JSONB config storage
- Filtered list (by type, status, search)
- Test connection endpoint

### Routes CRUD — `/api/v1/routes`
- `RouteStrategy` enum: static, round_robin, failover, weighted
- Longest prefix match (`priority DESC, length(prefix) DESC`)
- Connector validation

### Messages CRUD — `/api/v1/messages`
- Full lifecycle state machine (`ValidateTransition()`)
- Idempotency via `client_ref` (unique per tenant)
- Outbox pattern: create → store → queue → send

### DLR Callback — `POST /api/v1/dlr/:connector_id`
- Idempotent version-based updates
- Provider status mapping (`DELIVRD` → delivered)
- Duplicate DLR detection

### Event Bus
- `Bus` interface (`MemoryBus` implementation)
- 20 event types (6 message lifecycle)
- Pub/sub with subscription IDs

### Audit Logs
- Cursor-based pagination (append-only safe)
- Resource, action, user, tenant, IP tracking

### PostgreSQL Repositories
- Soft delete + optimistic locking (`version` field)
- `FOR UPDATE SKIP LOCKED` queue implementation
- `TxManager` for transactions
- Outbox events table

### Docker + Bootstrap
- Multi-stage Dockerfile (golang:1.26 → alpine)
- docker-compose.yml (dev + prod profiles)
- Bootstrap: migrations + super admin + default tenant

### Metrics & Health
- 9 Prometheus metrics (low cardinality — no tenant_id)
- Circuit breaker per connector (Closed → Open → Half-Open)
- `/health` + `/ready` + `/metrics` endpoints
- Queue-aware health checks

### Worker Infrastructure
- QueueWorker (poll → send → ack → retry)
- RetryEngine (exponential backoff + jitter)
- OutboxWorker (batch event publishing)
- Panic recovery with exponential backoff (100ms → 30s)
- Graceful shutdown (HTTP → Queue → Retry → Outbox → eventBus → Redis → PostgreSQL)

### What's Missing in Core
- `go test -race` (blocked: no C compiler on Windows)
- Load test (50k–100k messages)

---

## 📋 Phase 2.5 — Worker Engine

**Goal**: Build the full message lifecycle worker — queue → route → connector → send → retry → audit → metrics

**Key Requirements**:
- Read messages from queue
- Select appropriate route (Routing Engine integration)
- Select appropriate connector
- Execute send
- Retry policies with exponential backoff
- Idempotency (no double-send)
- Status transitions (state machine)
- Audit logs
- Metrics
- Event publishing
- Unit tests for every critical path

**Rules**:
- Repository Pattern only — no business logic in handlers
- Context on all operations
- Pluggable — no connector coupling
- `go build ./...` + `go test ./...` + Docker smoke test

---

## 📋 Phase 2.6 — Routing Engine

**Goal**: Standalone routing package with multiple strategies

**Required Strategies**:
- **Static**: Fixed connector
- **Round Robin**: Distribute across connectors
- **Failover**: Primary → backup(s)
- **Weighted**: Proportional distribution

**Architecture**:
- Separate `internal/routing/` package
- Worker calls routing engine for decision
- Worker has no knowledge of how connectors are selected
- Routing engine returns `ConnectorID`
- Routes loaded from repository, cached in memory

---

## 📋 Phase 2.7 — Connector Framework

**Goal**: Unified interface for all protocol connectors

```go
type Connector interface {
    Send(ctx context.Context, msg *domain.Message) (*SendResult, error)
    Health(ctx context.Context) error
    Close() error
}
```

**Implementations**:
- `HTTPConnector`
- `SMPPConnector`
- `SIPConnector`

**Design**:
- Worker → `Connector.Send()` — no protocol knowledge
- Each connector is a self-contained package
- New protocols = new implementation, no core changes

---

## 📋 Phase 2.8 — HTTP Connectors

**Goal**: Production-grade generic HTTP connector

**Features**:
- JSON / Form URL-encoded requests
- Custom headers
- Authentication (Bearer, Basic, API Key)
- Timeouts
- Retry (provider-level)
- Success code mapping
- Error mapping
- DLR callback support
- Provider adapters (Twilio, Plivo, AWS SNS, etc.)

---

## 📋 Phase 2.9 — SMPP Connector

**Goal**: Production-grade SMPP implementation

**Features**:
- Bind TX / RX / TRX
- `enquire_link` keepalive
- Automatic reconnect
- Windowing (configurable window size)
- Sequence number management
- Multipart SMS (UDH + SAR)
- UCS2 encoding
- TON/NPI handling
- Throttling
- Delivery receipts
- Error mapping
- Session recovery
- Separate Session Manager from business logic

---

## 📋 Phase 2.10 — SIP Connector

**Goal**: Standalone SIP voice gateway connector

**Features**:
- Registration
- Authentication (Digest)
- INVITE / BYE
- SIP Trunks
- Failover
- RTP integration (G.711/Opus)
- Speech-to-Text integration (future)

---

## 📋 Phase 3 — Production Validation

**Goal**: Prove system reliability

**Tests**:
- `go test -race ./...` (requires WSL or MinGW-w64)
- Stress testing (sustained high load)
- Load testing (100k+ messages)
- Chaos testing (cut PostgreSQL/Redis mid-operation)
- Graceful shutdown under load
- Recovery tests (crash → restart → resume)
- Benchmarking
- End-to-end integration tests

---

## Execution Rules

1. **Don't break existing architecture** — build on top, never rewrite
2. **Business logic stays in service/domain layer** — never in API handlers
3. **Every feature must be testable** — unit tests required
4. **Every component must be pluggable** — interface-first design
5. **Code consistency** — match existing patterns (naming, error handling, logging)
6. **After each phase**: `go build ./...` + `go test ./...` + Docker smoke test + written report
