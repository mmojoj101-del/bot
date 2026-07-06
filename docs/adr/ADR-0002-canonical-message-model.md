# ADR-0002: Canonical Message Model

**Status**: Accepted  
**Date**: 2026-07-06  
**Deciders**: Raghna, Mostafastbot  

---

## Context

The Core needs to support multiple protocols (HTTP, SMPP, SIP) and multiple providers (Twilio, Vonage, Infobip). If the Core directly handles protocol-specific formats, adding a new protocol or provider requires changes across the entire system. The SMPP PDU (`submit_sm`, `deliver_sm`), SIP INVITE body (SDP), and HTTP provider JSON are all different — the Core must not know any of them.

## Decision

Adopt a **Canonical Message Model**: one single internal message representation that the Core works with. Every Connector maps:

- **Outbound**: Canonical → Protocol-specific format (before sending)
- **Inbound**: Protocol-specific format → Canonical (after receiving)

### Structure

```go
// CanonicalMessage — the only message model the Core ever sees
type CanonicalMessage struct {
    ID              string
    TenantID        string
    Source          string        // sender address
    Destination     string        // recipient address
    Text            string        // message content (plain text)

    // Encoding
    Encoding        EncodingType   // GSM7, UCS2, UTF8, Binary

    // Message properties
    Parts           int           // number of parts (after splitting)
    ClientRef       string        // idempotency key
    ExternalID      string        // provider-side message ID

    // Status
    Status          MessageStatus // current canonical status

    // Routing
    RouteID         string
    ConnectorID     string
    RouteType       RouteType     // sms, call

    // Pricing (int64 = thousandths of a cent)
    Price           int64
    Cost            int64

    // Timing
    CreatedAt       time.Time
    SentAt          *time.Time
    DeliveredAt     *time.Time

    // Tracing
    TraceID         string
    CorrelationID   string

    // Provider-specific metadata (for debugging/audit only)
    RawProviderResponse json.RawMessage
}
```

### Mapping Boundaries

```
HTTP Provider JSON  ──→  HTTPConnector.ParseResponse()  ──→  CanonicalMessage
SMPP deliver_sm     ──→  SMPPConnector.ParseDLR()       ──→  CanonicalMessage
SIP 200 OK (INVITE) ──→  SIPConnector.ParseResponse()   ──→  CanonicalMessage

CanonicalMessage    ──→  HTTPConnector.BuildRequest()   ──→  HTTP JSON body
CanonicalMessage    ──→  SMPPConnector.BuildSubmit()    ──→  submit_sm PDU
CanonicalMessage    ──→  SIPConnector.BuildInvite()     ──→  SIP INVITE
```

### What the Core Never Sees
- SMPP `submit_sm` / `deliver_sm` PDU fields (command_status, seq_num, ton, npi, etc.)
- SIP `INVITE` headers (Via, From, To, Call-ID, CSeq, Contact, SDP body)
- Provider-specific JSON fields (Twilio `SmsSid`, Vonage `message-id`, Infobip `bulkId`)
- HTTP status codes from provider responses

## Consequences

### Positive
- Core is protocol-agnostic by construction
- Adding a new protocol requires only a new Connector implementation
- Adding a new provider requires only a small adapter (mapping to/from Canonical)
- Unit tests for Core logic never need protocol-specific fixtures

### Negative
- Connector implementations must do mapping → slightly more code per Connector
- Some protocol-specific information (SMPP TON/NPI, SIP SDP) is not visible in Core

### Mitigations
- `RawProviderResponse` field preserves original response for debugging
- Connector-specific metadata can be stored in a separate `connector_metadata` JSONB column

## Compliance
- **Compatibility Rule**: Protocol X maps to/from Canonical — Core unchanged
- **8-Question Test**: New protocol = new Connector package + mapping only

## References
- ARCHITECTURE_PRINCIPLES.md § Canonical Message Model
- ROADMAP.md § Phase 2.7 Connector Framework
