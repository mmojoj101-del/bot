# ADR-0008: Connector Lifecycle & State Management

**Status**: Accepted  
**Date**: 2026-07-06  
**Deciders**: Raghna, Mostafastbot  

---

## Context

The current `Connector` interface has `Health() error` which returns a binary healthy/unhealthy. This is insufficient for session-based protocols (SMPP, SIP) that have complex lifecycle states: disconnected, connecting, bound, reconnecting, degraded, draining. HTTP connectors, being stateless, have a simpler lifecycle. The Connector Registry needs to track each connector's current state and transition history for health checks, routing decisions, and observability.

## Decision

### 1. Connector Lifecycle States

```
                    ┌──────────┐
                    │   New    │  (created but not initialized)
                    └────┬─────┘
                         │ Initialize()
                         ▼
                    ┌──────────┐
              ┌────►│ Starting │  (connecting/binding for session-based)
              │     └────┬─────┘
              │          │ Ready()
              │          ▼
              │     ┌──────────┐
              │     │  Ready   │  (healthy, accepting traffic)
              │     └────┬─────┘
              │          │
              │     ┌────┴─────┐
              │     │          │
              │     ▼          ▼
              │  ┌────────┐ ┌──────────┐
              │  │Degraded│ │ Stopping │  (graceful shutdown)
              │  └───┬────┘ └────┬─────┘
              │      │           │ Stopped()
              │      │           ▼
              │      │      ┌──────────┐
              │      │      │ Stopped  │
              │      │      └──────────┘
              │      │
              │      │ Recover() / Reconnect()
              │      ▼
              │ ┌──────────┐
              └─┤ Starting │  (reconnect attempt)
                └──────────┘

```

| State | Meaning | HTTP Connector | SMPP Connector |
|-------|---------|---------------|----------------|
| **New** | Created but not initialized | Created | Created, no socket |
| **Starting** | Initializing | Resolving DNS | TCP connect + bind |
| **Ready** | Healthy, accepting traffic | Healthy | Bound, sending/receiving |
| **Degraded** | Partially working | N/A | enquire_link timeout, reconnecting |
| **Stopping** | Graceful shutdown | Draining requests | Unbind + close socket |
| **Stopped** | Fully stopped | Idle | Unbound |
| **Failed** | Irrecoverable error | DNS permanently failed | Bind rejected, auth failed |

### 2. Lifecycle Interface

```go
// Lifecycle — full connector lifecycle management
type Lifecycle interface {
    State() ConnectorState
    Start(ctx context.Context) error
    Stop(ctx context.Context) error
    Reconnect(ctx context.Context) error
}

type ConnectorState string

const (
    ConnectorStateNew       ConnectorState = "new"
    ConnectorStateStarting  ConnectorState = "starting"
    ConnectorStateReady     ConnectorState = "ready"
    ConnectorStateDegraded  ConnectorState = "degraded"
    ConnectorStateStopping  ConnectorState = "stopping"
    ConnectorStateStopped   ConnectorState = "stopped"
    ConnectorStateFailed    ConnectorState = "failed"
)

// Connector — combined interface that lifecycle-aware connectors implement
type Connector interface {
    Send(ctx context.Context, req *SendRequest) (*SendResult, error)
    Health(ctx context.Context) error
    Close() error
    Lifecycle
}
```

### 3. Registry Integration

```go
// RegisteredConnector — updated with lifecycle
type RegisteredConnector struct {
    ID           string
    Connector    Connector
    Info         ConnectorInfo
    State        ConnectorState
    StateSince   time.Time       // when current state was entered
    Health       ConnectorHealth
    LastChecked  time.Time
    StateHistory []StateTransition // last N transitions for audit
}

type StateTransition struct {
    From  ConnectorState
    To    ConnectorState
    At    time.Time
    Error string // if transition was due to an error
}
```

### 4. Health Check Integration

```go
// Health check updates the connector's lifecycle state
func (r *ConnectorRegistry) healthCheck(ctx context.Context, id string) {
    rc := r.connectors[id]
    err := rc.Connector.Health(ctx)
    
    if err != nil {
        switch rc.State {
        case ConnectorStateReady:
            rc.transitionTo(ConnectorStateDegraded, err.Error())
        case ConnectorStateDegraded:
            // Already degraded — attempt reconnect for session-based connectors
            if reconnectErr := rc.Connector.Reconnect(ctx); reconnectErr != nil {
                rc.transitionTo(ConnectorStateFailed, reconnectErr.Error())
            } else {
                rc.transitionTo(ConnectorStateReady, "")
            }
        }
    } else {
        if rc.State == ConnectorStateDegraded || rc.State == ConnectorStateStarting {
            rc.transitionTo(ConnectorStateReady, "")
        }
    }
}
```

### 5. Routing Engine Integration

The Routing Engine excludes connectors that are not in `Ready` state:

```go
func (e *RoutingEngine) selectConnector(ctx context.Context, req *RoutingRequest) (*RoutingDecision, error) {
    candidates := e.registry.List(ctx, ConnectorFilter{
        TenantID: req.TenantID,
        State:    ConnectorStateReady,  // only ready connectors
    })
    // ... match by capabilities, weight, priority
}
```

## Consequences

### Positive
- Full visibility into connector health at every stage (not just binary up/down)
- SMPP session recovery is managed through the lifecycle state machine
- Routing Engine avoids degraded/failed connectors automatically
- State history enables post-mortem analysis of connector failures

### Negative
- More complex than a simple `Health() error`
- Session-based connectors need async lifecycle management (background reconnect)

### Mitigations
- HTTP connectors have a simple lifecycle (New → Starting → Ready → Stopped)
- SMPP connectors use the full lifecycle with automatic reconnection
- State transitions emit Infrastructure Events for observability

## Compliance
- **Compatibility Rule**: Protocol X implements `Lifecycle` — no Core changes
- **8-Question Test**: Lifecycle is an interface, mockable, independently testable

## References
- ADR-0004: Connector Registry
- ADR-0005: State Machine & RoutingDecision
- ARCHITECTURE_PRINCIPLES.md § Connector Capabilities
