# ADR-0004: Connector Registry & Plugin Framework

**Status**: Accepted  
**Date**: 2026-07-06  
**Deciders**: Raghna, Mostafastbot  

---

## Context

The current `ConnectorRepository` provides CRUD for connector configs from the database. The actual runtime connector instances (HTTP sender, SMPP session, SIP trunk) need to be managed at runtime — created when enabled, destroyed when disabled, health-checked periodically. The Routing Engine needs metadata (capabilities, protocol, version) to make routing decisions. Future dynamic loading of connector plugins must be supported without redesigning the Core.

## Decision

Adopt a **Connector Registry** with rich metadata, replacing the simpler `map[id]Connector`:

### Registry Structure

```go
// ConnectorRegistry — manages runtime connector instances
type ConnectorRegistry struct {
    connectors map[string]*RegisteredConnector
    mu         sync.RWMutex
}

// RegisteredConnector — a connector instance with full metadata
type RegisteredConnector struct {
    ID           string
    Connector    Connector              // the runtime instance (Send/Health/Close)
    Info         ConnectorInfo          // static metadata
    Health       ConnectorHealth        // latest health check result
    LastChecked  time.Time
}

// ConnectorInfo — static metadata, set at registration
type ConnectorInfo struct {
    Protocol     string       // "http", "smpp", "sip", "xmpp"
    Version      string       // "v1", "v2"
    TenantID     string
    Capabilities Capabilities // SupportsDLR, SupportsUnicode, etc.
    Weight       int          // for weighted routing (0-100)
    Priority     int          // for failover routing (lower = preferred)
    CreatedAt    time.Time
}

// ConnectorHealth — latest health state
type ConnectorHealth struct {
    Status     ConnectorStatus  // active, disabled, error
    LastError  string
    Uptime     time.Duration    // if session-based
    Sessions   int              // current active sessions (SMPP/SIP)
}

// Capabilities — what this connector can do
type Capabilities struct {
    SupportsDLR       bool
    SupportsMultipart bool
    SupportsUnicode   bool
    SupportsAsync     bool
    SupportsSession   bool
    SupportsInbound   bool
    SupportsMedia     bool
    MaxThroughput     int
    MaxMessageLength  int
}
```

### Interface

```go
// Registry operations
func (r *ConnectorRegistry) Register(ctx context.Context, info ConnectorInfo, conn Connector) error
func (r *ConnectorRegistry) Unregister(ctx context.Context, id string) error
func (r *ConnectorRegistry) Get(ctx context.Context, id string) (*RegisteredConnector, error)
func (r *ConnectorRegistry) List(ctx context.Context, filter ConnectorFilter) ([]*RegisteredConnector, error)
func (r *ConnectorRegistry) HealthCheckAll(ctx context.Context) map[string]ConnectorHealth
```

### Lifecycle

```
1. Bootstrap / Config Change
   └── Read connector config from DB (ConnectorRepository)
       └── Create Connector instance (factory by type)
           └── Register(ctx, info, connector) → Registry
               └── Start health check goroutine (if session-based)
                   └── Available for Routing Engine queries

2. Connector Config Updated
   └── Unregister old instance → Close() → Register new instance

3. Health Check Fails
   └── Update RegisteredConnector.Health
       └── Routing Engine excludes unhealthy connectors
           └── Publish InfrastructureEvent: ConnectorUnhealthy
```

### Factory Pattern

```go
// ConnectorFactory — creates connector instances by type
type ConnectorFactory interface {
    CanHandle(protocol string) bool
    Create(ctx context.Context, config json.RawMessage) (Connector, error)
}

// Registry holds factories
type ConnectorRegistry struct {
    // ... existing fields
    factories map[string]ConnectorFactory  // "http" → HTTPFactory, "smpp" → SMPPFactory
}

func (r *ConnectorRegistry) RegisterFactory(protocol string, factory ConnectorFactory) {
    r.factories[protocol] = factory
}
```

## Consequences

### Positive
- Routing Engine can filter connectors by capabilities, health, weight, priority
- Adding a new protocol = implement `Connector` + `ConnectorFactory` + register factory
- Health checks are centralized, not scattered across connectors
- Dynamic discovery is prepared (factory pattern + metadata)
- Registry is injectable via DI, not a global variable

### Negative
- More initial code to set up registry + factories
- Session-based connectors (SMPP, SIP) need lifecycle management (bind/unbind)

### Mitigations
- Registry is a thin coordination layer — heavy logic stays in Connector implementations
- Session lifecycle is the connector's responsibility, not the registry's

## Compliance
- **Compatibility Rule**: New protocol = new factory + new package, no Core changes
- **8-Question Test**: Protocol X → RegisterFactory("x", factory), routing engine considers it automatically

## References
- ARCHITECTURE_PRINCIPLES.md § Plugin Mindset (Connector Registry)
- ROADMAP.md § Phase 2.7 Connector Framework
- ADR-0005: Immutable RoutingDecision
