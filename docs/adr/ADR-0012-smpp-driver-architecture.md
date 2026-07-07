# ADR-0012: SMPP Driver Architecture — Session Engine Design

**Status**: Accepted
**Date**: 2026-07-07
**Deciders**: Raghna, Mostafastbot

## Context

Phase 2.8 established a protocol-agnostic Connector Framework (`GenericConnector` + `ProtocolDriver` + `StatefulDriver`). Phase 2.9 will implement the first real stateful driver — SMPP (Short Message Peer-to-Peer).

SMPP is fundamentally different from HTTP:
- **Stateful**: TCP session with bind/unbind lifecycle
- **Session-based**: All PDUs share one connection; responses correlate by sequence number
- **Asymmetric**: SubmitSM (us→SMSC) and DeliverSM (SMSC→us) share the same socket
- **Windowed**: Multiple outstanding requests; limited by receiver window size
- **Binary protocol**: PDU encoding/decoding, fixed fields + TLVs
- **Heartbeat**: enquire_link keeps idle sessions alive
- **Failure-prone**: Timeouts, disconnects, rebinds are expected operational events

This ADR documents the architecture of the SMPP Session Engine — the core internal design of `internal/connector/driver/smpp/`.

## Decision

### 1. Session/Driver Separation

The `SMPPDriver` owns a `Session`. The `Session` contains all session-level state and lifecycle.

```go
// SMPPDriver implements ProtocolDriver + StatefulDriver.
// It is a thin facade that owns one Session.
type SMPPDriver struct {
    session *Session
}

func (d *SMPPDriver) Connect(ctx context.Context) error {
    return d.session.Connect(ctx)
}

func (d *SMPPDriver) Send(ctx context.Context, req *TransportRequest) (*TransportResponse, error) {
    return d.session.Submit(ctx, req)
}
```

The `Session` struct groups all session-level components:

```go
type Session struct {
    mu       sync.Mutex
    state    SessionState
    transport SMPPTransport       // abstract, not net.Conn
    reader   *Reader              // single reader goroutine
    window   *WindowManager       // acquire/release slots
    pending  *PendingStore        // seq → pending request correlation
    seq      *SequenceManager     // monotonic sequence allocator
    heartbeat *HeartbeatScheduler // enquire_link timer
    reconnect *ReconnectManager   // auto-reconnect + backoff
    codec    *Codec               // PDU binary codec
    logger   Logger
}
```

**Rationale**: Session Pool support in the future (multiple binds per driver) requires clear separation. `SMPPDriver` can own `[]*Session` and route messages to the least-loaded session.

### 2. Single Reader Ownership

> **No goroutine reads directly from `net.Conn` except the single Reader.**

```
TCP Socket
    │
    ▼
Reader Goroutine (single, owns the socket read)
    │
    ▼
Decode PDU  ← Codec (pure, knows nothing about session)
    │
    ▼
Dispatcher (goroutine-safe, routes by CommandID)
    │
    ├──► SubmitSMResp  → PendingStore.Notify(seq, resp)
    ├──► DeliverSM     → DLR callback / event bus
    ├──► EnquireLink   → auto-reply with enquire_link_resp
    ├──► BindResp      → unblocks Connect()
    ├──► Unbind        → initiates graceful shutdown
    └──► GenericNack   → log + retry logic
```

**Rationale**: Multiple goroutines reading from the same socket produces corrupt PDU boundaries and is nearly impossible to debug. This rule is absolute.

### 3. Reader → Dispatcher → Handlers

The Reader:
1. Reads from transport (blocking)
2. Decodes binary PDU via Codec
3. Calls `dispatcher.Dispatch(pdu)`

The Dispatcher:
- Switch on `pdu.Header.CommandID`
- Routes to registered handlers
- Never modifies session state directly

Handlers:
- `submitSMRespHandler`: extract seq, notify PendingStore
- `deliverSMHandler`: parse DLR, emit event
- `enquireLinkHandler`: send enquire_link_resp
- `bindRespHandler`: unblock Connect() caller
- `unbindHandler`: initiate graceful shutdown

**Rationale**: The Reader MUST NOT know about submit_sm, deliver_sm, or any business logic. Adding a new PDU type = adding a handler, modifying zero lines in Reader.

### 4. Codec is Pure

```go
type Codec struct{}

func (c *Codec) Encode(pdu *PDU) ([]byte, error)
func (c *Codec) Decode(data []byte) (*PDU, error)
```

- Zero knowledge of Session, State, Window, or Pending
- Testable in isolation: `Encode(SubmitSM)` → `[]byte` → `Decode` → `*PDU` round-trip
- Handles: 32-bit length header, command_id, command_status, sequence_number, mandatory fields, TLVs

### 5. TLV as []TLV, Not Struct Fields

```go
type TLV struct {
    Tag   uint16
    Value []byte
}

type PDU struct {
    Header  PDUHeader
    Body    *SubmitSM // or DeliverSM, etc. — nil if N/A
    TLVs    []TLV     // all optional parameters
}
```

Helpers for common TLVs:

```go
func SetMessagePayload(pdu *PDU, payload []byte)    // tag 0x0424
func GetMessagePayload(pdu *PDU) ([]byte, bool)
func SetReceiptedMessageID(pdu *PDU, id string)     // tag 0x001E
func SetSAROptions(pdu *PDU, ref, total, seq uint8) // tags 0x020C–0x020E
```

**Rationale**: SMPP spec defines 100+ TLVs. Different SMSC providers use vendor-specific TLVs. A struct-field-per-TLV approach would require constant maintenance. `[]TLV` is future-proof.

### 6. PendingRequest Struct (Not Just chan)

```go
type PendingRequest struct {
    Seq       uint32
    CommandID uint32
    CreatedAt time.Time
    Deadline  time.Time
    Response  chan *PDU // buffered(1), receives submit_sm_resp
    TraceID   string
}

type PendingStore struct {
    mu       sync.RWMutex
    requests map[uint32]*PendingRequest
    metrics  MetricsRecorder
}
```

Methods:
- `Register(seq, cmdID, deadline) *PendingRequest` — insert
- `Notify(seq, resp *PDU)` — delivers to channel + removes
- `Remove(seq)` — explicit cleanup
- `TimedOut() []uint32` — returns expired seqs (for cleanup goroutine)
- `Len() int` — current window fill

**Rationale**: Real SMPP drivers need timeout-based cleanup (SMSC may silently drop requests), traceability for debugging, window utilization metrics, and cancellation. A bare `chan` provides none of these.

### 7. Window Manager Owns Pending + Sequence

```
WindowManager
    ├── SequenceManager (allocates next seq)
    ├── PendingStore (tracks outstanding)
    └── semaphore (max concurrent)
```

```go
type WindowManager struct {
    max     int64
    sema    chan struct{} // buffered channel as semaphore
    pending *PendingStore
    seq     *SequenceManager
}

// Acquire blocks until window slot is available, then returns seq.
func (w *WindowManager) Acquire(ctx context.Context) (uint32, error)

// Release completes a slot.
func (w *WindowManager) Release(seq uint32)

// Submit registers a pending request and writes to transport.
// This is the core path: Acquire → seq.Next() → Register → WritePDU.
func (w *WindowManager) Submit(ctx context.Context, pdu *PDU, transport SMPPTransport) (<-chan *PDU, error)
```

**Rationale**: Window, Pending, and Sequence are inseparable in practice. `Acquire()` blocks until a slot is free, then atomically allocates seq + registers pending. `Release()` is called when response arrives or times out. This eliminates a class of races that would exist if these were three independent components.

### 8. SessionState Machine

```
         ┌─────────────────────────────────────┐
         │                                     │
         ▼                                     │
   ┌───────────┐    ┌──────────┐    ┌───────┐  │
   │Disconnected│───►│Connecting│───►│Binding│  │
   └───────────┘    └──────────┘    └───────┘  │
         ▲                           │         │
         │                           ▼         │
         │                      ┌───────┐      │
         │◄──── timeout ────────│ Bound │      │
         │                      └───────┘      │
         │                           │         │
         │                           ▼         │
         │                      ┌──────────┐   │
         │◄──── done ───────────│Disconnect│   │
         │                      └──────────┘   │
         │                           │         │
         │                           ▼         │
         │                    ┌─────────┐      │
         └────────────────────│  Closed │      │
                              └─────────┘      │
                                     │         │
                                     └─────────┘
```

```go
type SessionState int

const (
    StateDisconnected SessionState = iota
    StateConnecting
    StateBinding
    StateBound
    StateDisconnecting
    StateClosed
)

func (s SessionState) String() string { ... }
func (s SessionState) IsTerminal() bool { return s == StateClosed }
func (s SessionState) IsAlive() bool { return s == StateBound }
```

**Rationale**: A bool `IsConnected()` cannot distinguish `Connecting` (wait for bind_resp) from `Disconnected` (need full reconnect). The state machine drives all lifecycle decisions: "should we retry? should we queue? should we reconnect?"

### 9. SMPPTransport Abstraction

```go
type SMPPTransport interface {
    ReadPDU(ctx context.Context) ([]byte, error)
    WritePDU(ctx context.Context, data []byte) error
    Close() error
}

// tcpTransport implements SMPPTransport over net.Conn.
type tcpTransport struct {
    conn   net.Conn
    mu     sync.Mutex // protects write
    buffer *bufio.Reader
}

func NewTCPTransport(conn net.Conn) SMPPTransport { ... }
```

For testing:

```go
// fakeTransport simulates an SMSC for tests.
type fakeTransport struct {
    readCh  chan []byte // PDU bytes to return from ReadPDU
    writeCh chan []byte // captured writes for assertions
    err     error       // simulated error
}
```

**Rationale**: Every SMPP feature (PDU encoding, state machine transitions, window behavior, timeout, reconnect) can be tested with `fakeTransport` without a real socket. Tests run in microseconds, not seconds.

### 10. Driver Testability — Four Layers

| Layer | Scope | Dependencies | Speed |
|-------|-------|-------------|-------|
| **Codec Test** | Encode/Decode round-trip for every PDU type | None | μs |
| **Window Test** | Acquire/Release/Timeout behavior | `fakeTransport` | μs |
| **Session Test** | State transitions, Bind, Submit, Deliver | `fakeTransport` | ms |
| **Driver Test** | Full SMPPDriver via GenericConnector | `fakeTransport` | ms |
| **Integration** | Against real/emulated SMSC | TCP socket | s |

### 11. Single Reader Ownership

This decision is formalized as a rule:

> **No goroutine in the SMPP driver reads directly from `net.Conn` except the single Reader goroutine owned by `Session`.**
>
> - Write operations (`SubmitSM`, `enquire_link_resp`) use `SMPPTransport.WritePDU()` which is mutex-protected.
> - Read operations go through Reader → Decode → Dispatcher.
> - Violating this rule (even accidentally) produces corrupted PDU boundaries that are nearly impossible to debug.

## Consequences

### Positive

- **Testable**: `fakeTransport` replaces TCP in all unit/component tests — no flaky network tests
- **Extensible**: Handlers registered per CommandID; new PDU types = new handler, zero changes to Reader
- **Debugable**: PendingRequest carries TraceID, timestamps, and CommandID — every request is traceable
- **Resilient**: State machine + auto-reconnect + heartbeat + timeout cleanup cover all failure modes
- **Window-safe**: Acquire/Release atomicity prevents over-windowing races
- **Session Pool ready**: `SMPPDriver` owns `[]*Session`; separating Session from Driver is already done

### Negative

- **Complexity**: More components than a naive implementation (Reader, Dispatcher, Handlers, Pending, Window, State Machine)
- **Single Reader goroutine**: One goroutine must handle all responses; handler must be fast (no blocking I/O)
- **Session state replication**: State machine state must be consistent with transport state

### Mitigations

- Handler dispatch is in-memory switch — sub-microsecond overhead
- State machine is protected by `Session.mu`; state transitions are atomic
- Long-running handlers (like DLR event emission) spawn goroutines or use an event bus channel

## Compliance

**8-Question Architecture Test:**
1. New protocol = new ProtocolDriver → zero Worker changes ✅
2. New SMSC provider = new config → zero Routing Engine changes ✅
3. New routing strategy = new Selector → zero Connector changes ✅
4. New feature (DLR correlation, billing) = event subscriber only ✅
5. Everything testable in isolation via fakeTransport ✅
6. Remove SMPP Driver = delete package, no core changes ✅
7. Horizontal scaling: each session is independent, no local assumptions ✅
8. **Compatibility Rule**: SMPP added tomorrow → zero Worker/Core changes ✅

## Execution Order (Phase 2.9)

1. Codec — PDU types, binary encode/decode, TLV helpers
2. SequenceManager — monotonic seq, thread-safe
3. PendingStore — register, notify, timeout, metrics
4. SMPPTransport — interface + tcpTransport + fakeTransport
5. WindowManager — Acquire/Release/Submit (integrates seq + pending)
6. Reader — single goroutine, decode → dispatch
7. Dispatcher + Handlers — BindResp, SubmitSMResp, DeliverSM, EnquireLink, GenericNack
8. Session StateMachine — connect, disconnect, reconnect, state transitions
9. Bind/Unbind procedures
10. SubmitSM flow — full path: Acquire → Encode → Write → Wait → Decode → Handle
11. DeliverSM + DLR correlation
12. enquire_link heartbeat scheduler
13. Auto-reconnect + backoff
14. Graceful shutdown

## References

- [ADR-0004: Connector Registry & Plugin Framework](ADR-0004-connector-registry.md)
- [ADR-0008: Connector Lifecycle & State Management](ADR-0008-connector-lifecycle.md)
- [ARCHITECTURE_PRINCIPLES.md](../../ARCHITECTURE_PRINCIPLES.md)
- [ROADMAP.md](../../ROADMAP.md)
- SMPP v3.4 Specification (GSM 03.38)
