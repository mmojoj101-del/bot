package smpp

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
)

// ── Session State ────────────────────────────────────────────────────────────

// SessionState tracks the lifecycle of an SMPP session.
// Transitions happen in session.go only — no other file modifies state.
//
//	Disconnected ──Connect()──▶ Connecting ──bind sent──▶ Binding ──resp──▶ Bound
//	    ▲                                                            │
//	    │                        ┌───◀─── remote unbind ◀───┐       │
//	    │                        │                            │       │
//	    └── Closed ◀── Disconnecting ◀── Disconnect() ◀───────┘       │
//	        ▲                        │                                │
//	        └────────────────────────┴─── reconnect ◀─────────────────┘
type SessionState int

const (
	StateDisconnected SessionState = iota
	StateConnecting
	StateBinding
	StateBound
	StateDisconnecting
	StateClosed
)

func (s SessionState) String() string {
	switch s {
	case StateDisconnected:
		return "disconnected"
	case StateConnecting:
		return "connecting"
	case StateBinding:
		return "binding"
	case StateBound:
		return "bound"
	case StateDisconnecting:
		return "disconnecting"
	case StateClosed:
		return "closed"
	default:
		return fmt.Sprintf("unknown(%d)", int(s))
	}
}

// ── Errors ──────────────────────────────────────────────────────────────────

var (
	ErrSessionClosed    = errors.New("smpp: session is closed")
	ErrNotBound         = errors.New("smpp: session not in bound state")
	ErrInvalidState     = errors.New("smpp: invalid state transition")
	ErrBindFailed       = errors.New("smpp: bind failed")
	ErrSessionTimeout   = errors.New("smpp: session request timed out")
)

// ── Session ──────────────────────────────────────────────────────────────────

// Session is the single owner of all SMPP session components.
//
// It is NOT a container — it owns the lifecycle:
//   - Creates all components (Transport, Codec, Window, WriteQueue, Dispatcher, Reader)
//   - Starts/stops goroutines (Reader, WriteQueue)
//   - Manages state transitions (Connect → Bind → Send → Disconnect)
//   - Controls shutdown order (cancel → reader exit → fail pending → close)
//
// Session provides a high-level API:
//
//	Connect(ctx, addr, bindPDU)  — transport + bind
//	SendRequest(ctx, pdu)        — generic: acquire window → write → wait response
//	Disconnect(ctx)              — unbind + cleanup
//
// All PDUs (Bind, SubmitSM, Unbind, EnquireLink) use SendRequest internally.
// No PDU type gets a special path — this guarantees uniform behaviour.
//
// ── Write Paths (INTENTIONAL — do not unify without understanding) ──
//
// Two write paths exist:
//
//  1. Direct (SendRequest): application goroutines call slot.Write(transport, pdu).
//     This encodes, registers pending, and writes to transport directly.
//     These goroutines are NOT the Reader — blocking on transport Write is safe.
//
//  2. WriteQueue (handlers): Reader/Dispatcher handlers call writeQ.TryEnqueue(pdu).
//     The WriteQueue goroutine drains and writes to transport.
//     This path is NON-BLOCKING (TryEnqueue) — the Reader never blocks.
//
// Why two paths? Because unifying them would force SendRequest to go through
// the WriteQueue, adding latency under queue backpressure. Application requests
// (SubmitSM, Bind) should not be delayed by handler responses (DeliverSMResp).
//
// Both paths share the same tcpTransport.WritePDU mutex, so they are safe
// for concurrent use. If future features (tracing, rate limiting) need a
// single write hook, consider adding a WriteHook middleware to tcpTransport
// rather than eliminating the direct path.
type Session struct {
	mu    sync.Mutex
	state SessionState

	// Components (owned, not shared)
	transport SMPPTransport
	codec     *Codec
	seq       *SequenceManager
	pending   *PendingStore
	window    *WindowManager
	writeQ    *WriteQueue
	disp      *Dispatcher
	reader    *Reader

	// Lifecycle
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	readerErr chan error // buffered(1), receives fatal reader errors
	closed    chan struct{}

	// Config
	windowSize int
}

// SessionConfig holds constructor parameters for Session.
type SessionConfig struct {
	WindowSize int     // max concurrent outstanding requests (1..100)
	Codec      *Codec // optional: defaults to NewCodec(Version3_4)
}

// DefaultWindowSize is the recommended window size for SMPP sessions.
const DefaultWindowSize = 10

// NewSession creates a Session with all internal components.
// The session starts in Disconnected state — call Connect() to begin.
func NewSession(cfg SessionConfig) *Session {
	if cfg.WindowSize < 1 {
		cfg.WindowSize = DefaultWindowSize
	}
	codec := cfg.Codec
	if codec == nil {
		codec = NewCodec(Version3_4)
	}

	seq := NewSequenceManager()
	pending := NewPendingStore()
	window := NewWindowManager(cfg.WindowSize, seq, pending, codec)
	writeQ := NewWriteQueue(cfg.WindowSize * 2)

	// Session handlers connect dispatcher to session internals.
	// All responses (including Bind) reach PendingStore via the
	// PendingResponseHandler. The per-type handlers are for
	// action dispatch (like auto-respond to enquire_link).
	pendingHandler := &sessionPendingHandler{pending: pending}
	disp := NewDispatcher(
		pendingHandler, // PendingResponseHandler — ALL correlated responses
		nil,            // DeliverSMHandler — set by caller or default
		pendingHandler, // BindRespHandler — forwards to PendingStore (same as pending)
		&sessionEnquireHandler{writeQ: writeQ}, // EnquireLink auto-respond
		&sessionUnbindHandler{}, // UnbindHandler — triggers disconnect
		nil, // GenericNackHandler — optional
	)

	return &Session{
		state:      StateDisconnected,
		codec:      codec,
		seq:        seq,
		pending:    pending,
		window:     window,
		writeQ:     writeQ,
		disp:       disp,
		readerErr:  make(chan error, 1),
		closed:     make(chan struct{}),
		windowSize: cfg.WindowSize,
	}
}

// ── Connect ──────────────────────────────────────────────────────────────────

// Connect establishes a TCP connection, starts internal goroutines, and
// performs SMPP Bind via Session.Bind().
//
// Steps:
//  1. dial addr
//  2. create tcpTransport
//  3. create + start WriteQueue
//  4. create + start Reader
//  5. Bind(ctx, bindPDU) — via Session.Bind (same SendRequest path)
//  6. transition to Bound state
//
// On failure, cleans up partial state (transport, goroutines, pending).
func (s *Session) Connect(ctx context.Context, addr string, bindPDU *BindTransceiver) error {
	if err := s.transitionTo(StateConnecting); err != nil {
		return err
	}

	// Dial
	conn, err := dialTCP(ctx, addr)
	if err != nil {
		s.setState(StateDisconnected)
		return fmt.Errorf("smpp: dial %s: %w", addr, err)
	}

	s.transport = NewTCPTransport(conn)
	s.ctx, s.cancel = context.WithCancel(context.Background())

	// Start WriteQueue (writer goroutine)
	s.writeQ.Start(s.transport, s.codec)

	// Create and start Reader
	s.reader = NewReader(s.transport, s.codec, s.disp, s.readerErr)
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.reader.Start(s.ctx)
	}()

	// Bind via Bind() — uniform SendRequest path
	if err := s.transitionTo(StateBinding); err != nil {
		s.cleanup()
		return err
	}
	if err := s.Bind(ctx, bindPDU); err != nil {
		s.cleanup()
		return err
	}

	return nil
}

// ── SendRequest ──────────────────────────────────────────────────────────────

// SendRequest sends a PDU and waits for the correlated response.
//
// It is the single path for ALL request/response pairs:
//   - BindTransceiver → BindTransceiverResp
//   - SubmitSM → SubmitSMResp
//   - Unbind → UnbindResp
//   - EnquireLink → EnquireLinkResp
//
// This eliminates duplicated logic for each PDU type.
//
// Steps (all atomic within WindowManager):
//  1. window.Acquire(ctx) — allocates seq + semaphore slot
//  2. slot.Write(transport) — encodes + registers pending + writes to transport
//  3. wait for resp on slot.Response()
//  4. slot.Release()
//
// The transport write happens DIRECTLY from this goroutine (not via WriteQueue).
// This is safe because:
//   - tcpTransport.WritePDU is mutex-protected
//   - This goroutine is NOT the Reader goroutine (no blocking concern)
//   - WriteQueue is ONLY for handler responses (Reader goroutine path)
func (s *Session) SendRequest(ctx context.Context, pdu PDU) (PDU, error) {
	// Quick state check (non-blocking, best-effort)
	s.mu.Lock()
	if s.state != StateBound && s.state != StateBinding {
		s.mu.Unlock()
		return nil, fmt.Errorf("%w: state=%s", ErrNotBound, s.state)
	}
	s.mu.Unlock()

	// Acquire window slot (blocks until slot available or ctx timeout)
	slot, err := s.window.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("smpp: acquire: %w", err)
	}
	defer slot.Release()

	// Encode, register pending, and write to transport (direct, not queue)
	if err := slot.Write(s.transport, pdu); err != nil {
		return nil, fmt.Errorf("smpp: write: %w", err)
	}

	// Wait for response or timeout
	select {
	case resp, ok := <-slot.Response():
		if !ok {
			return nil, fmt.Errorf("smpp: slot closed: %w", ctx.Err())
		}
		return resp, nil
	case <-ctx.Done():
		return nil, fmt.Errorf("smpp: request: %w", ctx.Err())
	case <-s.closed:
		return nil, ErrSessionClosed
	}
}

// ── Disconnect ──────────────────────────────────────────────────────────────

// Disconnect performs an orderly session shutdown.
//
// Shutdown order (documented — do not change):
//  1. Transition to Disconnecting (reject new SendRequest)
//  2. Cancel session context → Reader goroutine exits (EOF from transport)
//  3. Wait for Reader goroutine
//  4. Close WriteQueue (drain remaining or discard)
//  5. PendingStore.Clear() — fail all outstanding requests
//  6. Close transport
//  7. WaitGroup.Wait()
//  8. Transition to Closed
func (s *Session) Disconnect(ctx context.Context) error {
	if err := s.transitionTo(StateDisconnecting); err != nil {
		return err
	}

	// Cancel session context — Reader will exit on next ReadPDU
	if s.cancel != nil {
		s.cancel()
	}

	// Wait for Reader to exit
	s.wg.Wait()

	// Stop WriteQueue (drains remaining PDUs)
	s.writeQ.Stop()

	// Fail all pending requests
	s.pending.Clear()

	// Close transport
	if s.transport != nil {
		_ = s.transport.Close()
	}

	// Close the closed channel (unblocks any waiting SendRequest)
	select {
	case <-s.closed:
	default:
		close(s.closed)
	}

	s.setState(StateClosed)
	return nil
}

// ── State helpers ────────────────────────────────────────────────────────────

func (s *Session) transitionTo(target SessionState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.transitionLocked(target)
}

func (s *Session) transitionLocked(target SessionState) error {
	current := s.state
	valid := false

	switch current {
	case StateDisconnected:
		valid = target == StateConnecting
	case StateConnecting:
		valid = target == StateBinding || target == StateDisconnected
	case StateBinding:
		valid = target == StateBound || target == StateDisconnected
	case StateBound:
		valid = target == StateDisconnecting
	case StateDisconnecting:
		valid = target == StateClosed
	case StateClosed:
		valid = false
	}

	if !valid {
		return fmt.Errorf("%w: %s → %s", ErrInvalidState, current, target)
	}

	s.state = target
	return nil
}

func (s *Session) setState(target SessionState) {
	s.mu.Lock()
	s.state = target
	s.mu.Unlock()
}

func (s *Session) cleanup() {
	s.cancel()
	s.wg.Wait()
	s.writeQ.Stop()
	s.pending.Clear()
	if s.transport != nil {
		_ = s.transport.Close()
	}
	select {
	case <-s.closed:
	default:
		close(s.closed)
	}
	s.setState(StateDisconnected)
}

// ── Handler implementations ──────────────────────────────────────────────────

// sessionPendingHandler handles correlated responses via PendingStore.
// Used as both PendingResponseHandler and BindRespHandler.
type sessionPendingHandler struct {
	pending *PendingStore
}

func (h *sessionPendingHandler) HandleResponse(resp PDU) {
	h.pending.Notify(resp.Header().SequenceNumber, resp)
}

func (h *sessionPendingHandler) HandleBindResp(resp *BindTransceiverResp) {
	// Forward to PendingStore via the generic response path
	h.pending.Notify(resp.Header().SequenceNumber, resp)
}

// sessionBindHandler unblocks Connect() by signalling through a shared channel.
// For simplicity, the Bind response is routed through the same PendingStore.
type sessionBindHandler struct{}

func (h *sessionBindHandler) HandleBindResp(resp *BindTransceiverResp) {
	// The response goes through PendingStore like any other PDU
	// via the PendingResponseHandler. No special handling needed.
}

// sessionEnquireHandler auto-responds to incoming enquire_link.
// It enqueues the response on the WriteQueue (non-blocking TryEnqueue).
// This is called from the Reader goroutine — must never block.
type sessionEnquireHandler struct {
	writeQ *WriteQueue
}

func (h *sessionEnquireHandler) HandleEnquireLink(seq uint32) {
	resp := &EnquireLinkResp{
		Hdr: Header{
			CommandID:       CommandIDEnquireLinkResp,
			CommandStatus:   StatusOK,
			SequenceNumber: seq,
		},
	}
	// Non-blocking: if queue is full, drop the response (SMSC will retry)
	_ = h.writeQ.TryEnqueue(resp)
}

// sessionUnbindHandler handles remote unbind.
type sessionUnbindHandler struct{}

func (h *sessionUnbindHandler) HandleUnbind() (sendResp bool) {
	return true
}

// ── Dial helper ──────────────────────────────────────────────────────────────

func dialTCP(ctx context.Context, addr string) (net.Conn, error) {
	var d net.Dialer
	return d.DialContext(ctx, "tcp", addr)
}
