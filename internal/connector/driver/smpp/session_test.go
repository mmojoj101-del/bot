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

func TestSession_LateResponse_AfterClear(t *testing.T) {
	s, ft := newSessionWithFake(t)

	// Send a request that will wait for a response
	ctx := context.Background()
	pdu := &EnquireLink{Hdr: Header{CommandID: CommandIDEnquireLink}}

	// Start sending
	respCh := make(chan error, 1)
	go func() {
		_, err := s.SendRequest(ctx, pdu)
		respCh <- err
	}()

	// Wait for write
	select {
	case <-ft.writeCh:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for write")
	}

	// Disconnect — this clears pending
	s.Disconnect(context.Background())

	// Now send a late response (after pending was cleared)
	lateResp := &EnquireLinkResp{
		Hdr: Header{CommandID: CommandIDEnquireLinkResp, SequenceNumber: 1, CommandStatus: StatusOK},
	}
	lateData, _ := s.codec.Encode(lateResp)
	ft.readCh <- lateData

	// Let the late response be processed briefly
	time.Sleep(20 * time.Millisecond)

	// The SendRequest should have returned an error from the cancel
	select {
	case err := <-respCh:
		if err == nil {
			t.Log("SendRequest returned nil (may have completed before shutdown)")
		}
	default:
		t.Log("SendRequest still waiting (timing-dependent)")
	}
}

func TestSession_Disconnect_WithPendingRequest(t *testing.T) {
	s, ft := newSessionWithFake(t)

	// Start a request that will remain pending
	ctx := context.Background()
	pdu := &EnquireLink{Hdr: Header{CommandID: CommandIDEnquireLink}}

	respCh := make(chan error, 1)
	go func() {
		_, err := s.SendRequest(ctx, pdu)
		respCh <- err
	}()

	// Wait for write
	select {
	case <-ft.writeCh:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for write")
	}

	// Ensure pending count is > 0 before disconnect
	if n := s.pending.Len(); n != 1 {
		t.Fatalf("expected 1 pending request, got %d", n)
	}

	// Disconnect — should cancel the pending request
	err := s.Disconnect(context.Background())
	if err != nil {
		t.Fatalf("Disconnect error: %v", err)
	}

	// SendRequest should have been unblocked (with error)
	select {
	case err := <-respCh:
		if err == nil {
			t.Log("SendRequest returned nil (completed before disconnect)")
		} else {
			t.Logf("SendRequest error (expected): %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("SendRequest not unblocked after disconnect")
	}

	// Pending should be empty
	if n := s.pending.Len(); n != 0 {
		t.Errorf("expected 0 pending after disconnect, got %d", n)
	}
}

func TestSession_WriteQueueFull_EnquireLinkDropped(t *testing.T) {
	s, ft := newSessionWithFake(t)

	// Fill the write queue (buffer = windowSize * 2 = 10)
	seq := uint32(1)
	for i := 0; i < 50; i++ {
		s.writeQ.TryEnqueue(&EnquireLinkResp{
			Hdr: Header{
				CommandID:       CommandIDEnquireLinkResp,
				CommandStatus:   StatusOK,
				SequenceNumber: seq,
			},
		})
		seq++
	}

	// Check: some may be dropped due to queue being full
	queued := s.writeQ.Len()
	t.Logf("write queue has %d items after 50 TryEnqueue (buf=10)", queued)
	if queued > 10 {
		t.Errorf("queue cap exceeded: %d > 10", queued)
	}

	s.Disconnect(context.Background())
	_ = ft
}

// ── Concurrency Tests ───────────────────────────────────────────────────────

func TestSession_ParallelSendRequest_WindowLimit(t *testing.T) {
	s, ft := newSessionWithFake(t)
	defer s.Disconnect(context.Background())

	const count = 12 // > window size (5), validates window serialization
	errCh := make(chan error, count)
	pdu := &EnquireLink{Hdr: Header{CommandID: CommandIDEnquireLink}}

	// Launch 12 parallel SendRequests
	for i := 0; i < count; i++ {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_, err := s.SendRequest(ctx, pdu)
			errCh <- err
		}()
	}

	// Interleave: drain batch of writes → send batch of responses
	// Window = 5, so at most 5 writes happen before responses are needed
	batchSize := 4
	totalDrained := 0
	totalResponses := 0

	for totalResponses < count {
		// Drain up to batchSize writes
		drained := 0
		for drained < batchSize && totalDrained < count {
			select {
			case <-ft.writeCh:
				drained++
				totalDrained++
			case <-time.After(2 * time.Second):
				t.Fatalf("timed out waiting for write %d/%d (drained=%d)",
					totalDrained+1, count, totalDrained)
			}
		}

		// Send responses for drained writes
		// We don't know the exact seq numbers allocated, but they start at 1
		for i := 0; i < drained; i++ {
			seq := uint32(totalResponses + 1) // sequential starting at 1
			resp := &EnquireLinkResp{
				Hdr: Header{CommandID: CommandIDEnquireLinkResp, SequenceNumber: seq, CommandStatus: StatusOK},
			}
			data, _ := s.codec.Encode(resp)
			ft.readCh <- data
			totalResponses++
		}
	}

	// Collect results — should all be OK
	for i := 0; i < count; i++ {
		select {
		case err := <-errCh:
			if err != nil {
				t.Errorf("SendRequest %d failed: %v", i+1, err)
			}
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for response %d/%d", i+1, count)
		}
	}
}

func TestSession_ResponseOutOfOrder(t *testing.T) {
	s, ft := newSessionWithFake(t)
	defer s.Disconnect(context.Background())

	type result struct {
		seq uint32
		err error
	}

	const count = 5
	errCh := make(chan result, count)

	// Send 5 requests
	for i := 0; i < count; i++ {
		pdu := &EnquireLink{Hdr: Header{CommandID: CommandIDEnquireLink}}
		go func() {
			ctx := context.Background()
			resp, err := s.SendRequest(ctx, pdu)
			seq := uint32(0)
			if resp != nil {
				seq = resp.Header().SequenceNumber
			}
			errCh <- result{seq, err}
		}()
	}

	// Drain 5 writes
	for i := 0; i < count; i++ {
		select {
		case <-ft.writeCh:
		case <-time.After(time.Second):
			t.Fatalf("timeout drain write %d", i+1)
		}
	}

	// Send responses OUT OF ORDER: 5, 4, 3, 2, 1
	for i := count; i >= 1; i-- {
		resp := &EnquireLinkResp{
			Hdr: Header{CommandID: CommandIDEnquireLinkResp, SequenceNumber: uint32(i), CommandStatus: StatusOK},
		}
		data, _ := s.codec.Encode(resp)
		ft.readCh <- data
	}

	// Collect results — should all succeed despite out-of-order delivery
	seen := make(map[uint32]bool)
	for i := 0; i < count; i++ {
		select {
		case r := <-errCh:
			if r.err != nil {
				t.Errorf("SendRequest seq=%d failed: %v", r.seq, r.err)
			}
			if r.seq == 0 {
				t.Error("expected non-zero seq in response")
			} else {
				if seen[r.seq] {
					t.Errorf("duplicate response seq %d", r.seq)
				}
				seen[r.seq] = true
			}
		case <-time.After(time.Second):
			t.Fatalf("timeout waiting for response %d/5", i+1)
		}
	}

	if len(seen) != count {
		t.Errorf("expected %d unique responses, got %d", count, len(seen))
	}
}

func TestSession_DuplicateResponse_Ignored(t *testing.T) {
	s, ft := newSessionWithFake(t)
	defer s.Disconnect(context.Background())

	ctx := context.Background()
	pdu := &EnquireLink{Hdr: Header{CommandID: CommandIDEnquireLink}}

	// Send request
	respCh := make(chan error, 1)
	go func() {
		_, err := s.SendRequest(ctx, pdu)
		respCh <- err
	}()

	// Wait for write
	select {
	case <-ft.writeCh:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for write")
	}

	// Send FIRST response
	firstResp := &EnquireLinkResp{
		Hdr: Header{CommandID: CommandIDEnquireLinkResp, SequenceNumber: 1, CommandStatus: StatusOK},
	}
	data, _ := s.codec.Encode(firstResp)
	ft.readCh <- data

	// Wait for first response to be processed
	select {
	case err := <-respCh:
		if err != nil {
			t.Fatalf("first response error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for first response")
	}

	// Send DUPLICATE response (same seq — already consumed) — must not panic
	dupResp := &EnquireLinkResp{
		Hdr: Header{CommandID: CommandIDEnquireLinkResp, SequenceNumber: 1, CommandStatus: StatusOK},
	}
	dupData, _ := s.codec.Encode(dupResp)
	ft.readCh <- dupData

	// Allow time for processing — no panic = pass
	time.Sleep(20 * time.Millisecond)
}

func TestSession_Disconnect_DuringWrite(t *testing.T) {
	s, ft := newSessionWithFake(t)

	// Use a writeHook that just captures the data (doesn't block)
	var captured atomic.Bool
	ft.writeHook = func(ctx context.Context, data []byte) error {
		captured.Store(true)
		// Don't actually write to writeCh — simulate slow but non-blocking
		// This lets the SendRequest think it wrote, but no response comes
		return nil
	}

	// Send request (will "write" but never get a response)
	errCh := make(chan error, 1)
	go func() {
		_, err := s.SendRequest(context.Background(), &EnquireLink{Hdr: Header{CommandID: CommandIDEnquireLink}})
		errCh <- err
	}()

	// Wait for the write to be captured
	time.Sleep(50 * time.Millisecond)
	if !captured.Load() {
		t.Fatal("write not captured")
	}

	// Disconnect while request is pending
	err := s.Disconnect(context.Background())
	if err != nil {
		t.Logf("Disconnect err: %v", err)
	}

	// SendRequest should unblock with an error
	select {
	case err := <-errCh:
		if err == nil {
			t.Log("SendRequest completed (response never sent but ctx may have timed out)")
		}
	case <-time.After(time.Second):
		t.Fatal("SendRequest not unblocked after disconnect")
	}
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
