package smpp

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"
)

// ── Helpers ──────────────────────────────────────────────────────────────────

// newSessionPreBind returns a Session in the Connecting state with all
// internal components running (Reader, WriteQueue) but no bind performed.
// The caller can transition to StateBinding and call Bind().
//
// This is the same as newSessionWithFake except for the initial state.
func newSessionPreBind(t *testing.T) (*Session, *sessionFakeTransport) {
	t.Helper()
	ft := newSessionFakeTransport()
	s := NewSession(SessionConfig{WindowSize: 5})
	s.transport = ft
	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.writeQ.Start(s.transport, s.codec)
	s.reader = NewReader(s.transport, s.codec, s.disp, s.readerErr)
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.reader.Start(s.ctx)
	}()
	s.setState(StateConnecting)
	return s, ft
}

// ── NewBindTransceiver ───────────────────────────────────────────────────────

func TestNewBindTransceiver(t *testing.T) {
	pdu := NewBindTransceiver("esme", "test-secret", "vms")
	if pdu.Hdr.CommandID != CommandIDBindTransceiver {
		t.Errorf("expected CommandIDBindTransceiver, got %s", pdu.Hdr.CommandID)
	}
	if pdu.SystemID != "esme" {
		t.Errorf("expected SystemID esme, got %s", pdu.SystemID)
	}
	if pdu.Password != "test-secret" {
		t.Errorf("expected Password test-secret, got %s", pdu.Password)
	}
	if pdu.SystemType != "vms" {
		t.Errorf("expected SystemType vms, got %s", pdu.SystemType)
	}
	if pdu.InterfaceVersion != 0x34 {
		t.Errorf("expected InterfaceVersion 0x34, got 0x%02X", pdu.InterfaceVersion)
	}
	if pdu.AddrTON != 0 || pdu.AddrNPI != 0 {
		t.Errorf("expected TON=0 NPI=0, got TON=%d NPI=%d", pdu.AddrTON, pdu.AddrNPI)
	}
}

// ── Bind Success ─────────────────────────────────────────────────────────────

func TestSession_Bind_Success(t *testing.T) {
	s, ft := newSessionPreBind(t)
	defer s.Disconnect(context.Background())

	// Transition to Binding
	if err := s.transitionTo(StateBinding); err != nil {
		t.Fatal(err)
	}

	// Start Bind in background — it will block waiting for the response
	errCh := make(chan error, 1)
	ctx := context.Background()
	go func() {
		errCh <- s.Bind(ctx, NewBindTransceiver("esme", "test-secret", ""))
	}()

	// Wait for the bind PDU to be written (SendRequest → slot.Write → transport)
	select {
	case data := <-ft.writeCh:
		if len(data) == 0 {
			t.Fatal("empty write data")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for bind write")
	}

	// Send bind response back through the Reader path
	resp := &BindTransceiverResp{
		Hdr: Header{
			CommandID:       CommandIDBindTransceiverResp,
			CommandStatus:   StatusOK,
			SequenceNumber:  1, // matches seq allocated by WindowManager
		},
		SystemID: "smsc",
	}
	respData, err := s.codec.Encode(resp)
	if err != nil {
		t.Fatalf("encode bind resp: %v", err)
	}
	ft.readCh <- respData

	// Wait for Bind to complete
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Bind failed: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for Bind response")
	}

	// State should be Bound
	if s.state != StateBound {
		t.Errorf("expected StateBound, got %s", s.state)
	}
}

// ── Bind Failure (CommandStatus != OK) ──────────────────────────────────────

func TestSession_Bind_Failure_CommandStatus(t *testing.T) {
	s, ft := newSessionPreBind(t)
	defer s.Disconnect(context.Background())

	if err := s.transitionTo(StateBinding); err != nil {
		t.Fatal(err)
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.Bind(context.Background(), NewBindTransceiver("esme", "test-secret", ""))
	}()

	// Drain write
	select {
	case <-ft.writeCh:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for bind write")
	}

	// Send bind response with error status — use a generic failure
	// that works regardless of SMPP version constant alignment
	resp := &BindTransceiverResp{
		Hdr: Header{
			CommandID:       CommandIDBindTransceiverResp,
			CommandStatus:   StatusSysFail, // generic system failure
			SequenceNumber:  1,
		},
	}
	respData, _ := s.codec.Encode(resp)
	ft.readCh <- respData

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("expected error for failed bind, got nil")
		}
		if !errors.Is(err, ErrBindFailed) {
			t.Errorf("expected ErrBindFailed, got %v", err)
		}
		t.Logf("Bind rejected (expected): %v", err)
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for Bind error")
	}

	// State should fall back to Disconnected
	if s.state != StateDisconnected {
		t.Errorf("expected StateDisconnected after failed bind, got %s", s.state)
	}
}

// ── Bind Timeout ─────────────────────────────────────────────────────────────

func TestSession_Bind_Timeout(t *testing.T) {
	s, ft := newSessionPreBind(t)
	defer s.Disconnect(context.Background())

	if err := s.transitionTo(StateBinding); err != nil {
		t.Fatal(err)
	}

	// Bind with a short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.Bind(ctx, NewBindTransceiver("esme", "test-secret", ""))
	}()

	// Drain write so the PDU is sent
	select {
	case <-ft.writeCh:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for bind write")
	}

	// DON'T send a response — let the context timeout
	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("expected timeout error, got nil")
		}
		t.Logf("Bind timeout (expected): %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("Bind did not timeout")
	}

	// State should fall back to Disconnected
	if s.state != StateDisconnected {
		t.Errorf("expected StateDisconnected after bind timeout, got %s", s.state)
	}
}

// ── Bind Invalid State ──────────────────────────────────────────────────────

func TestSession_Bind_InvalidState(t *testing.T) {
	s, ft := newSessionWithFake(t) // starts in StateBound
	defer s.Disconnect(context.Background())

	// Bind while already Bound should fail
	err := s.Bind(context.Background(), NewBindTransceiver("esme", "test-secret", ""))
	if err == nil {
		t.Fatal("expected error for bind in Bound state, got nil")
	}
	if !errors.Is(err, ErrInvalidState) {
		t.Errorf("expected ErrInvalidState, got %v", err)
	}
	t.Logf("Bind invalid-state error (expected): %v", err)
	_ = ft
}

// ── Bind Disconnect During Wait ─────────────────────────────────────────────

func TestSession_Bind_DisconnectDuringWait(t *testing.T) {
	s, ft := newSessionPreBind(t)
	_ = ft

	if err := s.transitionTo(StateBinding); err != nil {
		t.Fatal(err)
	}

	// Use a cancellable context so we can abort the Bind
	ctx, cancel := context.WithCancel(context.Background())

	// Start Bind (will block waiting for response)
	errCh := make(chan error, 1)
	go func() {
		errCh <- s.Bind(ctx, NewBindTransceiver("esme", "test-secret", ""))
	}()

	// Wait for write to be sent
	select {
	case <-ft.writeCh:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for bind write")
	}

	// Cancel the bind context — SendRequest will unblock with ctx error
	cancel()

	// Bind should unblock with an error
	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("expected error from Bind after cancel, got nil")
		}
		t.Logf("Bind after ctx cancel (expected): %v", err)
	case <-time.After(time.Second):
		t.Fatal("Bind not unblocked after ctx cancel")
	}

	// Now Disconnect is clean (state is Disconnected after failed Bind)
	_ = s.Disconnect(context.Background())
}

// ── Bind Round-Trip via Connect ──────────────────────────────────────────────
//
// This test verifies the full Connect → Bind → Bound flow using a fake
// TCP connection (in-memory pipe). The fake SMSC responds to the bind.
//
// Because Connect calls dialTCP which does a real TCP dial, we cannot use
// the fake transport directly. Instead, this test uses a real TCP listener
// on localhost and a goroutine acting as a fake SMSC.

func TestSession_Bind_ConnectRoundTrip(t *testing.T) {
	// Skip in short mode — requires a real network listener
	if testing.Short() {
		t.Skip("skipping Connect round-trip in short mode")
	}

	// Start a fake SMSC that accepts one connection and responds to bind
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()

	addr := listener.Addr().String()

	// Fake SMSC goroutine
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := listener.Accept()
		if err != nil {
			t.Logf("fake SMSC accept: %v", err)
			return
		}
		defer conn.Close()

		// Read bind PDU
		sess := NewSession(SessionConfig{})
		buf := make([]byte, 4096)
		n, err := conn.Read(buf)
		if err != nil {
			t.Logf("fake SMSC read: %v", err)
			return
		}

		// Decode the received PDU
		pdu, err := sess.codec.Decode(buf[:n])
		if err != nil {
			t.Logf("fake SMSC decode: %v", err)
			return
		}

		if pdu.Header().CommandID != CommandIDBindTransceiver {
			t.Logf("fake SMSC expected bind, got %s", pdu.Header().CommandID)
			return
		}

		// Build bind response
		resp := &BindTransceiverResp{
			Hdr: Header{
				CommandID:      CommandIDBindTransceiverResp,
				CommandStatus:  StatusOK,
				SequenceNumber: pdu.Header().SequenceNumber,
			},
			SystemID: "fake-smsc",
		}
		respData, _ := sess.codec.Encode(resp)
		_, _ = conn.Write(respData)
	}()

	// ESME: Connect
	s := NewSession(SessionConfig{})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	bindPDU := NewBindTransceiver("esme", "test-secret", "")
	if err := s.Connect(ctx, addr, bindPDU); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer s.Disconnect(context.Background())

	// Verify state is Bound
	if s.state != StateBound {
		t.Errorf("expected StateBound after Connect, got %s", s.state)
	}

	// Wait for fake SMSC to finish
	<-done
}
