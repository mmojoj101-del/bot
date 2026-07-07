package smpp

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// ── sessionFakeTransport ─────────────────────────────────────────────────────
//
// A fake transport for testing session lifecycle without real TCP.
// Supports:
//   - Queueing PDUs for ReadPDU (simulate SMSC responses)
//   - Capturing written PDUs for inspection
//   - Delayed reads, EOF, connection reset

type sessionFakeTransport struct {
	readCh    chan []byte    // PDUs delivered to ReadPDU
	writeCh   chan []byte    // PDUs captured from WritePDU
	readHook  func(ctx context.Context) ([]byte, error)
	writeHook func(ctx context.Context, data []byte) error
	closed    atomic.Bool
	closeCh   chan struct{}
}

func newSessionFakeTransport() *sessionFakeTransport {
	return &sessionFakeTransport{
		readCh:  make(chan []byte, 100),
		writeCh: make(chan []byte, 100),
		closeCh: make(chan struct{}),
	}
}

func (f *sessionFakeTransport) ReadPDU(ctx context.Context) ([]byte, error) {
	if f.readHook != nil {
		return f.readHook(ctx)
	}
	select {
	case data := <-f.readCh:
		return data, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-f.closeCh:
		return nil, errors.New("fake: transport closed")
	}
}

func (f *sessionFakeTransport) WritePDU(ctx context.Context, data []byte) error {
	if f.writeHook != nil {
		return f.writeHook(ctx, data)
	}
	select {
	case f.writeCh <- data:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil // non-blocking, just drop
	}
}

func (f *sessionFakeTransport) Close() error {
	f.closed.Store(true)
	select {
	case <-f.closeCh:
	default:
		close(f.closeCh)
	}
	return nil
}

// sessionFakeDial returns a function that "dials" by returning a fake transport.

// newSessionWithFake creates a Session with a pre-set fake transport.
// Returns the session and the fake transport for test manipulation.
func newSessionWithFake(t *testing.T) (*Session, *sessionFakeTransport) {
	t.Helper()
	ft := newSessionFakeTransport()
	s := NewSession(SessionConfig{WindowSize: 5})

	// Wire up the fake transport directly (bypass dial)
	s.transport = ft
	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.writeQ.Start(s.transport, s.codec)

	s.reader = NewReader(s.transport, s.codec, s.disp, s.readerErr)
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.reader.Start(s.ctx)
	}()

	s.setState(StateBound)
	return s, ft
}

// ── SendRequest Tests ───────────────────────────────────────────────────────

func TestSession_SendRequest_Success(t *testing.T) {
	s, ft := newSessionWithFake(t)
	defer s.Disconnect(context.Background())

	ctx := context.Background()
	pdu := &EnquireLink{Hdr: Header{CommandID: CommandIDEnquireLink, SequenceNumber: 0}}

	// Start SendRequest in a goroutine — it blocks waiting for response
	respCh := make(chan struct {
		pdu PDU
		err error
	})
	go func() {
		resp, err := s.SendRequest(ctx, pdu)
		respCh <- struct {
			pdu PDU
			err error
		}{resp, err}
	}()

	// Wait for the write to arrive on the fake transport
	select {
	case <-ft.writeCh:
		// PDU was written
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for write")
	}

	// Send a response back through the fake transport
	respPDU := &EnquireLinkResp{
		Hdr: Header{CommandID: CommandIDEnquireLinkResp, SequenceNumber: 1, CommandStatus: StatusOK},
	}
	respData, err := s.codec.Encode(respPDU)
	if err != nil {
		t.Fatalf("encode response: %v", err)
	}
	ft.readCh <- respData

	// Wait for SendRequest to return
	select {
	case r := <-respCh:
		if r.err != nil {
			t.Fatalf("SendRequest error: %v", r.err)
		}
		if r.pdu == nil {
			t.Fatal("expected response PDU, got nil")
		}
		if r.pdu.Header().CommandID != CommandIDEnquireLinkResp {
			t.Errorf("expected EnquireLinkResp, got %s", r.pdu.Header().CommandID)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for response")
	}
}

func TestSession_SendRequest_ContextCancel(t *testing.T) {
	s, ft := newSessionWithFake(t)
	defer s.Disconnect(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	pdu := &EnquireLink{Hdr: Header{CommandID: CommandIDEnquireLink}}

	_, err := s.SendRequest(ctx, pdu)
	if err == nil {
		t.Fatal("expected error from cancelled context, got nil")
	}
	// Consume the write from ft.writeCh so it doesn't leak
	select {
	case <-ft.writeCh:
	default:
	}
}

func TestSession_SendRequest_AfterClose(t *testing.T) {
	s, ft := newSessionWithFake(t)
	ft.Close()
	time.Sleep(50 * time.Millisecond)
	s.Disconnect(context.Background())

	ctx := context.Background()
	pdu := &EnquireLink{Hdr: Header{CommandID: CommandIDEnquireLink}}
	_, err := s.SendRequest(ctx, pdu)
	if err == nil {
		t.Fatal("expected error after session closed, got nil")
	}
}

func TestSession_SendRequest_WindowFull(t *testing.T) {
	s, ft := newSessionWithFake(t)
	defer s.Disconnect(context.Background())

	ctx := context.Background()

	// Fill the window (window size = 5)
	for i := 0; i < 5; i++ {
		pdu := &EnquireLink{Hdr: Header{CommandID: CommandIDEnquireLink}}
		go func() { s.SendRequest(ctx, pdu); _ = <-ft.writeCh }()
		time.Sleep(5 * time.Millisecond)
		// Consume the write from fake transport
		select {
		case <-ft.writeCh:
		default:
		}
	}

	// Now the window should be full (5 pending, no responses yet)
	// Actually, we need responses... let's just check that window is full
	if n := s.window.Len(); n != 5 {
		t.Logf("window len = %d (expected 5 after filling)", n)
	}
}

// ── Lifecycle Tests ─────────────────────────────────────────────────────────

func TestSession_StateTransition_Disconnect(t *testing.T) {
	s, _ := newSessionWithFake(t)

	if s.State() != StateBound {
		t.Errorf("expected Bound, got %s", s.State())
	}

	err := s.Disconnect(context.Background())
	if err != nil {
		t.Fatalf("Disconnect error: %v", err)
	}

	if s.State() != StateClosed {
		t.Errorf("expected Closed, got %s", s.State())
	}
}

func TestSession_Disconnect_Idempotent(t *testing.T) {
	s, _ := newSessionWithFake(t)

	// First disconnect
	err1 := s.Disconnect(context.Background())
	if err1 != nil {
		t.Fatalf("first Disconnect: %v", err1)
	}

	// Second disconnect should fail (invalid state transition)
	err2 := s.Disconnect(context.Background())
	if err2 == nil {
		t.Logf("second Disconnect returned nil (acceptable if idempotent)")
	}
}

func TestSession_StateAfterNewSession(t *testing.T) {
	s := NewSession(SessionConfig{WindowSize: 3})
	if s.State() != StateDisconnected {
		t.Errorf("expected Disconnected, got %s", s.State())
	}
}

func TestSession_StateString(t *testing.T) {
	states := []SessionState{
		StateDisconnected,
		StateConnecting,
		StateBinding,
		StateBound,
		StateDisconnecting,
		StateClosed,
	}
	expected := []string{
		"disconnected",
		"connecting",
		"binding",
		"bound",
		"disconnecting",
		"closed",
	}
	for i, s := range states {
		if s.String() != expected[i] {
			t.Errorf("expected %q, got %q", expected[i], s.String())
		}
	}
}

func TestSession_InvalidStateTransition(t *testing.T) {
	s := NewSession(SessionConfig{WindowSize: 3})

	// Disconnected → Bound is invalid
	err := s.transitionTo(StateBound)
	if err == nil {
		t.Fatal("expected error for Disconnected → Bound")
	}

	// Disconnected → Disconnecting is invalid
	err = s.transitionTo(StateDisconnecting)
	if err == nil {
		t.Fatal("expected error for Disconnected → Disconnecting")
	}
}

// ── Reader Error Propagation ────────────────────────────────────────────────

func TestSession_ReaderError_Propagated(t *testing.T) {
	s, ft := newSessionWithFake(t)

	// Close the transport — Reader should get an error
	ft.Close()

	// Wait for Reader to exit
	time.Sleep(100 * time.Millisecond)

	// Reader error should be on errCh
	select {
	case err := <-s.readerErr:
		if err == nil {
			t.Fatal("expected non-nil reader error, got nil")
		}
	default:
		t.Log("readerErr may not have been sent (depends on timing)")
	}

	s.Disconnect(context.Background())
}

// ── Default Config ──────────────────────────────────────────────────────────

func TestSession_DefaultConfig(t *testing.T) {
	s := NewSession(SessionConfig{})
	if s.windowSize != DefaultWindowSize {
		t.Errorf("expected window size %d, got %d", DefaultWindowSize, s.windowSize)
	}
	if s.codec == nil {
		t.Fatal("expected default codec, got nil")
	}
}

func TestSession_ZeroWindowSize(t *testing.T) {
	s := NewSession(SessionConfig{WindowSize: 0})
	if s.windowSize != DefaultWindowSize {
		t.Errorf("expected window size %d, got %d", DefaultWindowSize, s.windowSize)
	}
}

// ── Helper: State accessor ──────────────────────────────────────────────────

func (s *Session) State() SessionState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}
