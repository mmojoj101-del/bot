# Fury SMS Gateway — Development Roadmap

> **Current Status**: Core Platform Complete (Phase 2.4)  
> **Tag**: `v0.1.1-phase2.4`  
> **Next**: Phase 2.5 — Worker Engine  
> **Ultimate Goal**: Communications Platform — not just HTTP/SMPP/SIP, but a foundation where new protocols and providers can be added without redesigning the core.

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

**Goal**: Production-grade worker responsible for the full message lifecycle — from claiming to delivery/retry. Protocol-agnostic, pluggable, observable.

### Scope of Responsibility
The Worker Engine is the brain of the platform. It handles:
- **Claim** messages safely from queue (`FOR UPDATE SKIP LOCKED`)
- **Route selection** — asks Routing Engine (Phase 2.6): "which route + connector for this message?"
- **Connector selection** — receives `ConnectorID` from Routing Engine
- **Execute send** — passes message to the chosen `Connector` (Phase 2.7+)
- **Retry policies** — exponential backoff with jitter, max retry limit
- **Idempotency** — no double-send guarantees
- **Status state machine** — `ValidateTransition()` enforced
- **Audit logs** — every state change logged
- **Metrics** — Prometheus counters/histograms per worker
- **Events** — publish lifecycle events (sent, failed, delivered, etc.)
- **Graceful shutdown** — WaitGroup drain + context cancellation
- **Panic recovery** — loop+recover with exponential restart backoff
- **Context cancellation** — all operations cancellable from parent

### Architecture Rules
- **No protocol coupling**: Worker must NOT know HTTP, SMPP, or SIP details
- **Uses `Connector` interface**: `Send(ctx, req) → (*SendResult, error)`
- **Uses `RoutingEngine` interface**: `Select(ctx, msg) → (*RouteDecision, error)`
- **Repository Pattern** for DB access
- **Dependency Injection** for all components
- **100% unit test coverage** on critical paths (claim → send → ack → retry)
- **Integration test** with real PostgreSQL + mock connector

### Status Flow Through the Worker

```
accepted → queued → sending → sent → delivered  (success path)
                               → failed          (terminal failure)
                               → queued          (retry path, repeats up to MaxRetries)
```

### After Phase 2.5
- `go build ./...` ✅
- `go test ./...` ✅
- `go test -race ./...` (if on Linux/WSL) ✅
- Docker smoke test (create message → worker claims → sends via mock → DLR → delivered) ✅
- Written report with decisions, design rationale, and any technical debt

---

## 📋 Phase 2.6 — Routing Engine

**Goal**: Standalone package (`internal/routing/`) that answers one question:  
> *"What route + connector should handle this message?"*

### Interface
```go
type RoutingEngine interface {
    Select(ctx context.Context, msg *RoutingRequest) (*RoutingDecision, error)
    LoadRoutes(ctx context.Context, tenantID string) error  // warm cache
    Reload(ctx context.Context) error                       // on route changes
}

type RoutingRequest struct {
    TenantID    string
    Source      string
    Destination string
    Type        RouteType    // sms, call
    // future: message content, priority, time-of-day, etc.
}

type RoutingDecision struct {
    Route       *Route
    ConnectorID string
}
```

### Strategies (initial)
| Strategy | Description |
|----------|-------------|
| **Static** | Fixed route + connector per prefix |
| **Round Robin** | Sequential distribution across connectors in a route |
| **Failover** | Primary connector → secondary(s) on failure |
| **Weighted** | Proportional distribution based on configured weight |

### Architecture
- `internal/routing/` package — clean, isolated
- Routes loaded from repository, cached in memory (configurable TTL)
- Each strategy is a self-contained implementation
- New strategies = new file, no existing code changes
- Worker calls `engine.Select()` — has no knowledge of how connectors are chosen

### Design Decisions to Document
- Cache invalidation strategy (event-driven: republish route changes)
- Thread-safety of routing tables (sync.RWMutex or sync.Map)
- Weighted distribution algorithm (smooth weighted / consistent hashing)
- Failover detection (circuit breaker integration — skip open connectors)

---

## 📋 Phase 2.7 — Connector Framework

**Goal**: Before implementing any protocol, build a unified framework so that adding a new protocol means only adding a new package — zero changes to Worker, Routing Engine, or Core.

### Interface
```go
type Connector interface {
    Send(ctx context.Context, req *SendRequest) (*SendResult, error)
    Health(ctx context.Context) error
    Close() error
}

type SendRequest struct {
    MessageID   string
    Source      string
    Destination string
    Text        string
    Encoding    string          // GSM7, UCS2, etc.
    Config      json.RawMessage // protocol-specific config
    Route       *Route
    ConnectorID string
}

type SendResult struct {
    Success      bool
    ExternalID   string          // provider-side message ID
    Parts        int
    ErrorCode    string
    ErrorMessage string
    Status       MessageStatus
}
```

### Implementations (future phases)
| Connector | Package | Phase |
|-----------|---------|-------|
| HTTP | `internal/connector/http` | 2.8 |
| SMPP | `internal/connector/smpp` | 2.9 |
| SIP | `internal/connector/sip` | 2.10 |

### Design Principles
- Worker → `Connector.Send()` — zero protocol knowledge in the Worker
- Each connector is a self-contained package with its own tests
- Connector factory (`ConnectorRegistry`) for discovery and instantiation
- New protocol = implement the interface + register in factory
- Health check per connector for /ready endpoint
- Graceful Close() for cleanup (SMPP unbind, SIP BYE, HTTP idle connections)

---

## 📋 Phase 2.8 — HTTP Connectors

**Goal**: Production-grade generic HTTP connector with provider adapter support.

### Core Features
- **Request formats**: JSON, Form URL-encoded, XML
- **Custom headers**: arbitrary key-value, template support
- **Authentication**: Bearer token, Basic Auth, API Key (header/query)
- **Timeouts**: connect, write, read — all configurable
- **Retry**: provider-level retry with backoff (separate from Worker's)
- **Success codes**: configurable (200, 201, 202, etc.)
- **Error mapping**: provider error responses → standardized error codes
- **DLR callback**: inject callback URL into provider request, configure endpoint
- **Provider adapters**: Twilio, Plivo, AWS SNS, Vonage, Infobip — each is a small adapter

### Architecture
```go
type HTTPConnector struct {
    client  *http.Client
    config  HTTPConnectorConfig
    adapter ProviderAdapter // optional: provider-specific behaviour
}

type ProviderAdapter interface {
    BuildRequest(ctx context.Context, msg *SendRequest, config HTTPConnectorConfig) (*http.Request, error)
    ParseResponse(ctx context.Context, resp *http.Response) (*SendResult, error)
    ParseDLR(ctx context.Context, payload json.RawMessage) (*DLRPayload, error)
}
```

- Generic connector works out-of-box without an adapter
- Adapters handle provider-specific request/response transformations
- No core changes needed to add a new provider

---

## 📋 Phase 2.9 — SMPP Connector

**Goal**: Production-grade SMPP implementation. This is the most technically demanding phase.

### Features
| Feature | Description |
|---------|-------------|
| **Bind TX** | Transmitter (submit_sm only) |
| **Bind RX** | Receiver (deliver_sm only) |
| **Bind TRX** | Transceiver (both directions) |
| **enquire_link** | Keepalive heartbeat with configurable interval |
| **Windowing** | Configurable window size (outstanding requests) |
| **Sequence numbers** | Monotonic within session, wrapped at boundary |
| **Multipart SMS** | UDH + SAR (6-part reference number scheme) |
| **UCS2 encoding** | Automatic fallback from GSM7 to UCS2 |
| **TON/NPI** | Type of Number / Numbering Plan Indicator |
| **Delivery receipts** | Map SMPP DLR fields → standardized status |
| **Error mapping** | SMPP command_status → standardized error codes |
| **Rate limiting** | Configurable submits/sec per session |
| **Throttling** | Provider-side throttling response handling |
| **Session recovery** | Automatic rebind with exponential backoff |

### Architecture
- **Session Manager** (`internal/connector/smpp/session.go`) — handles bind/unbind/enquire_link/reconnect
- **Sender** (`internal/connector/smpp/sender.go`) — handles submit_sm, sequence numbers, windowing
- **Receiver** (`internal/connector/smpp/receiver.go`) — handles deliver_sm (DLR), enquire_link responses
- **Session state**: disconnected → connecting → bound → reconnecting → disconnected
- Session Manager and Sender are separate concerns — one manages the TCP connection, the other sends messages

### Key Design Decisions
- **No global state** — each session is an independent struct
- **Thread-safe window** — sync.Mutex + slice for outstanding requests (seq_num → PDU)
- **Reconnect with backoff** — first retry 1s, max 30s, reset on successful bind
- **enquire_link** must be sent from a goroutine, not blocking the send path
- **DLR correlation** — use `receipted_message_id` field (some providers use `message_id` in the DLR body)

---

## 📋 Phase 2.10 — SIP Connector

**Goal**: Standalone SIP voice gateway connector.

### Features
- **Registration**: SIP Register with expiry
- **Authentication**: Digest/MD5
- **INVITE**: Call setup with SDP
- **BYE**: Call teardown
- **SIP Trunks**: Provider trunk integration
- **Failover**: Multi-provider SIP failover
- **RTP integration**: G.711/Opus support (for future STT)

### Architecture
- Same `Connector` interface as HTTP and SMPP
- SIP stack can use gosip or custom UDP/TCP transport
- No changes to Worker, Routing Engine, or Core

---

## 📋 Phase 3 — Production Validation

**Goal**: Prove the system is production-ready through rigorous testing.

### Required Tests
| Test | Purpose |
|------|---------|
| `go test -race ./...` | Data race detection |
| Stress testing | Sustained high load over extended period |
| Load testing | 100k+ messages, measure throughput (msg/s) |
| Chaos testing | Cut PostgreSQL/Redis mid-operation, verify recovery |
| Graceful shutdown under load | SIGTERM while processing, verify zero message loss |
| Recovery testing | Crash → restart → resume without duplicate sends |
| Benchmarking | Profile CPU, memory, GC, DB connections |
| End-to-end integration | Create message → send → DLR → delivered, full chain |

### Success Criteria
- No data races
- Zero message loss on graceful shutdown (crash recovery best-effort)
- Load: 100k+ messages processed without DB deadlocks or connection pool exhaustion
- Chaos: system recovers within configured timeouts after service disruption
- All unit tests pass, all integration tests pass

---

## 🏛️ Architecture Rules

1. **Interface-first** — dependencies depend on interfaces, not concretions
2. **Dependency Injection** — no `init()`, no global singletons (except infrastructure like Prometheus registry)
3. **Pluggable Architecture** — every component must be replaceable without rewriting the system
4. **Protocol Agnostic** — Worker does not know HTTP from SMPP from SIP
5. **No Global State** — except infrastructure registries (Prometheus, ConnectorRegistry)
6. **Clean Architecture** — domain → service → repository → handler (inward dependencies only)
7. **Business Logic ≠ API Handlers** — handlers parse input, validate, call service. Business logic is in domain + service layers
8. **No Technical Debt** — if a compromise is made, document it explicitly in this file
9. **Testable by design** — every new feature requires unit tests + integration tests
10. **Consistent patterns** — match existing code style, naming, error handling, logging

## 📝 Documentation Standards

After each phase, produce a report covering:
- What was accomplished
- Key decisions with rationale
- Any technical debt incurred (with plan to resolve)
- Test results (`go build`, `go test`, Docker smoke test)
- Metrics/performance if applicable
- Changes to this ROADMAP.md if applicable

## 🎯 Ultimate Goal

Not just an HTTP/SMPP/SIP gateway. A **Communications Platform** where:
- New protocols are added as pluggable packages — no core changes
- New providers are added as small adapters — no connector changes
- Routing strategies are swapped via configuration — no code changes
- The Worker never needs to change when adding protocols
- The system is testable, observable, and production-verified
- The platform is rentable — multi-tenant by design, ready for resellers
- Scaling is horizontal — add more workers, not more complexity
