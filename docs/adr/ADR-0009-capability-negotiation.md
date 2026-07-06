# ADR-0009: Capability Negotiation (String-Based)

**Status**: Accepted  
**Date**: 2026-07-06  
**Deciders**: Raghna, Mostafastbot  

---

## Context

The initial `Capabilities` struct used boolean fields (`SupportsDLR bool`, `SupportsUnicode bool`). This approach breaks when new capabilities are added — every connector must be updated to implement the new field. For a platform that aims to support arbitrary future protocols, capabilities must be **extensible without interface changes**.

## Decision

Adopt **string-based capability keys** instead of boolean struct fields:

### Capability Constants

```go
// Capability keys — extensible via naming convention, no interface change needed
const (
    CapabilityUnicode    = "unicode"       // UCS2 / UTF-8 encoding
    CapabilityDLR        = "dlr"           // delivery receipts
    CapabilityMultipart  = "multipart"     // long message splitting
    CapabilityFlashSMS   = "flash-sms"     // class 0 flash messages
    CapabilityBinary     = "binary"        // binary data (not text)
    CapabilitySession    = "session"       // persistent session (SMPP bind, SIP register)
    CapabilityInbound    = "inbound"       // receive messages from network
    CapabilityAsync      = "async"         // non-blocking with deferred result
    CapabilityVoice      = "voice"         // voice call (SIP)
    CapabilityUSSD       = "ussd"          // USSD session
    CapabilityMedia      = "media"         // MMS / RCS media
    CapabilityHighThroughput = "high-throughput" // >100 msg/s
)
```

### Connector Capabilities

```go
// Capabilities — set of capability strings this connector supports
// Extensible by adding new constants — no interface changes needed
type Capabilities []string

// Has returns true if the connector supports the given capability
func (c Capabilities) Has(capability string) bool {
    for _, cap := range c {
        if cap == capability {
            return true
        }
    }
    return false
}

// HasAny returns true if the connector supports any of the given capabilities
func (c Capabilities) HasAny(capabilities ...string) bool {
    for _, cap := range capabilities {
        if c.Has(cap) {
            return true
        }
    }
    return false
}

// HasAll returns true if the connector supports all of the given capabilities
func (c Capabilities) HasAll(capabilities ...string) bool {
    for _, cap := range capabilities {
        if !c.Has(cap) {
            return false
        }
    }
    return true
}
```

### Message Requirements

```go
// MessageRequirements — what this message needs from a connector
type MessageRequirements struct {
    RequiredCapabilities Capabilities  // connector MUST have all of these
    PreferredCapabilities Capabilities // connector SHOULD have these (scoring)
    Priority             int           // message priority for routing
    MaxCost              int64         // maximum cost per message (thousandths of a cent)
}
```

### Routing Engine Matching

```go
func (e *RoutingEngine) matchConnector(connector *RegisteredConnector, req *MessageRequirements) bool {
    caps := Capabilities(connector.Info.Capabilities)
    // All required capabilities must be present
    return caps.HasAll(req.RequiredCapabilities...)
}

func (e *RoutingEngine) scoreConnector(connector *RegisteredConnector, req *MessageRequirements) int {
    score := 0
    caps := Capabilities(connector.Info.Capabilities)
    // Score 1 point for each preferred capability
    for _, pref := range req.PreferredCapabilities {
        if caps.Has(pref) {
            score++
        }
    }
    // Higher weight = higher score
    score += connector.Info.Weight
    // Lower priority (numerically) = higher score
    score += (100 - connector.Info.Priority)
    return score
}
```

### Registration

```go
// HTTP Connector registers with its capabilities
registry.Register(ctx, ConnectorInfo{
    ID:           "connector-http-1",
    Protocol:     "http",
    Capabilities: []string{CapabilityUnicode, CapabilityDLR, CapabilityInbound},
    Weight:       50,
    Priority:     1,
}, httpConnector)

// SMPP Connector registers with its capabilities
registry.Register(ctx, ConnectorInfo{
    ID:           "connector-smpp-1",
    Protocol:     "smpp",
    Capabilities: []string{CapabilityUnicode, CapabilityDLR, CapabilityMultipart,
                           CapabilitySession, CapabilityAsync, CapabilityInbound,
                           CapabilityBinary, CapabilityFlashSMS, CapabilityHighThroughput},
    Weight:       100,
    Priority:     1,
}, smppConnector)

// SIP Connector registers with its capabilities
registry.Register(ctx, ConnectorInfo{
    ID:           "connector-sip-1",
    Protocol:     "sip",
    Capabilities: []string{CapabilitySession, CapabilityVoice, CapabilityInbound, CapabilityAsync},
    Weight:       30,
    Priority:     2,
}, sipConnector)
```

## Consequences

### Positive
- New capabilities are added as string constants — no interface changes
- All existing connectors continue to work unchanged
- Routing Engine can match/filter/score based on arbitrary capability combinations
- Third-party connector plugins can define their own capabilities

### Negative
- String comparison is slightly slower than boolean field access (negligible)
- No compile-time check for valid capability names

### Mitigations
- Capability lookup uses a map internally (not linear search for `Has()`)
- Test helper validates that capability constants match known patterns

## Compliance
- **Compatibility Rule**: New protocol defines its own capability constants — Core unchanged
- **8-Question Test**: Capabilities are strings, fully extensible, no interface change

## References
- ADR-0004: Connector Registry
- ADR-0005: State Machine & RoutingDecision
- ARCHITECTURE_PRINCIPLES.md § Connector Capabilities
