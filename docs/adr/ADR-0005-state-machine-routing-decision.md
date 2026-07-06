# ADR-0005: Independent State Machine & Immutable RoutingDecision

**Status**: Accepted  
**Date**: 2026-07-06  
**Deciders**: Raghna, Mostafastbot  

---

## Context

Initially, message status was updated directly by the QueueWorker and Connectors. This created two problems:
1. Different connectors could set different statuses inconsistently (SMPP returns "submitted", HTTP returns "sent")
2. The routing decision (which connector was chosen and why) was not recorded, making audit and debugging difficult

## Decision

### 1. Independent State Machine

A **centralized State Machine** is the sole authority on message status. Connectors return raw results (`SendResult`), and the State Machine maps them to canonical states:

```go
// StateMachine — the single source of truth for message lifecycle
type StateMachine struct {
    current     MessageStatus
    transitions map[MessageStatus][]MessageStatus  // valid transitions
}

func NewMessageStateMachine() *StateMachine {
    return &StateMachine{
        current: MessageStatusCreated,
        transitions: map[MessageStatus][]MessageStatus{
            MessageStatusCreated:    {MessageStatusQueued, MessageStatusFailed},
            MessageStatusQueued:     {MessageStatusClaimed, MessageStatusFailed},
            MessageStatusClaimed:    {MessageStatusSending, MessageStatusFailed},
            MessageStatusSending:    {MessageStatusSent, MessageStatusFailed, MessageStatusRetrying},
            MessageStatusSent:       {MessageStatusWaitingDLR, MessageStatusDelivered, MessageStatusFailed},
            MessageStatusWaitingDLR: {MessageStatusDelivered, MessageStatusFailed},
            MessageStatusRetrying:   {MessageStatusQueued, MessageStatusExpired},
            // Terminal states
            MessageStatusDelivered:  {},
            MessageStatusFailed:     {},
            MessageStatusExpired:    {},
            MessageStatusCancelled:  {},
        },
    }
}

func (sm *StateMachine) Transition(ctx context.Context, from, to MessageStatus) error {
    allowed, ok := sm.transitions[from]
    if !ok {
        return fmt.Errorf("no transitions defined from %s", from)
    }
    for _, s := range allowed {
        if s == to {
            return nil
        }
    }
    return fmt.Errorf("invalid transition: %s → %s", from, to)
}

// MapResult — maps a connector SendResult to a canonical status
func (sm *StateMachine) MapResult(result *SendResult) MessageStatus {
    if result.Success {
        if result.RequestsDLR {
            return MessageStatusWaitingDLR
        }
        return MessageStatusDelivered
    }
    if result.Retryable {
        return MessageStatusRetrying
    }
    return MessageStatusFailed
}
```

### 2. Immutable RoutingDecision

The `RoutingDecision` is created **once** by the Routing Engine and **never modified** by any pipeline stage:

```go
// RoutingDecision — immutable value object, created once by Routing Engine
type RoutingDecision struct {
    RouteID         string
    ConnectorID     string
    StrategyUsed    string       // static, round_robin, failover, weighted
    Priority        int
    Cost            int64        // at selection time (thousandths of a cent)
    Reason          string       // why this route was chosen
    CapabilitiesUsed []string    // which capabilities were matched (e.g., ["unicode", "dlr"])
    SelectedAt      time.Time
}

// Ensure immutability — no setters, copy-on-write if needed
func (d RoutingDecision) WithReason(reason string) RoutingDecision {
    d.Reason = reason
    d.SelectedAt = time.Now()
    return d
}
```

### Flow

```
Connector.Send() → SendResult{Success, ErrorCode, ExternalID, Parts, DLRRequested}
                     ↓
               StateMachine.MapResult(result)
                     ↓
               Canonical Status: Sent, WaitingDLR, Delivered, Failed, Retrying
                     ↓
               Pipeline: update DB + publish Domain Event
```

## Consequences

### Positive
- All connectors map to the same canonical states — no inconsistency
- State Machine is independently testable (unit tests for every transition)
- `RoutingDecision` provides complete audit trail (which connector, why, when, at what cost)
- Immutability prevents pipeline stages from corrupting routing data

### Negative
- Slight indirection: connector result → state machine mapping
- State Machine must be kept in sync with DB schema enums

### Mitigations
- State Machine is a pure function — no DB, no IO — trivially testable
- DB enums are generated from the State Machine's allowed statuses

## Compliance
- **Compatibility Rule**: Protocol X returns `SendResult`, State Machine maps it — no Core changes
- **8-Question Test**: State Machine is testable with mocks, no connector knowledge

## References
- ARCHITECTURE_PRINCIPLES.md § Single Centralized State Machine
- ARCHITECTURE_PRINCIPLES.md § RoutingDecision is Immutable
- ADR-0002: Canonical Message Model
