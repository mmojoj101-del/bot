package smpp

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ── fakeTransport for Reader Tests ───────────────────────────────────────────
//
// Minimal fake transport that delivers pre-built byte slices.
// Full fakeTransport with configurable delays/errors comes in Session tests.

type readerFakeTransport struct {
	ch     chan []byte // PDUs to deliver
	done   chan struct{}
	close  atomic.Bool
	readFn func(ctx context.Context) ([]byte, error)
}

func (f *readerFakeTransport) ReadPDU(ctx context.Context) ([]byte, error) {
	if f.readFn != nil {
		return f.readFn(ctx)
	}
	select {
	case data := <-f.ch:
		return data, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-f.done:
		return nil, errors.New("fake: closed")
	}
}

func (f *readerFakeTransport) WritePDU(ctx context.Context, data []byte) error {
	return nil // not used by Reader
}

func (f *readerFakeTransport) Close() error {
	f.close.Store(true)
	close(f.done)
	return nil
}

func newReaderFakeTransport() *readerFakeTransport {
	return &readerFakeTransport{
		ch:   make(chan []byte, 10),
		done: make(chan struct{}),
	}
}

// encodeTestPDU is a helper to encode any PDU for the fake transport.
func encodeTestPDU(t *testing.T, codec *Codec, pdu PDU) []byte {
	t.Helper()
	data, err := codec.Encode(pdu)
	if err != nil {
		t.Fatalf("encode failed: %v", err)
	}
	return data
}

// ── Reader Tests ─────────────────────────────────────────────────────────────

func TestReader_DispatcherSubmitSMResp(t *testing.T) {
	codec := NewCodec(Version3_4)
	transport := newReaderFakeTransport()
	pending := &mockPendingRespHandler{}
	dispatcher := NewDispatcher(pending, nil, nil, nil, nil, nil)
	errCh := make(chan error, 1)

	reader := NewReader(transport, codec, dispatcher, errCh)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Encode a SubmitSMResp and queue it on the transport
	pdu := &SubmitSMResp{
		Hdr:       Header{CommandID: CommandIDSubmitSMResp, SequenceNumber: 42, CommandStatus: StatusOK},
		MessageID: "test-msg-001",
	}
	transport.ch <- encodeTestPDU(t, codec, pdu)

	// Start reader in a goroutine
	done := make(chan struct{})
	go func() {
		reader.Start(ctx)
		close(done)
	}()

	// Wait for the handler to be called
	time.Sleep(50 * time.Millisecond)
	cancel()
	<-done

	if n := pending.calls.Load(); n != 1 {
		t.Errorf("expected 1 handler call, got %d", n)
	}
	last := pending.last.(*SubmitSMResp)
	if last.MessageID != "test-msg-001" {
		t.Errorf("expected message_id 'test-msg-001', got '%s'", last.MessageID)
	}
}

func TestReader_MalformedPDU_Continues(t *testing.T) {
	codec := NewCodec(Version3_4)
	transport := newReaderFakeTransport()
	var errorLog atomic.Int32
	dispatcher := NewDispatcher(nil, nil, nil, nil, nil, nil)
	dispatcher.OnError = func(err error) { errorLog.Add(1) }
	errCh := make(chan error, 1)

	reader := NewReader(transport, codec, dispatcher, errCh)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Queue a malformed PDU (short header — only 4 bytes)
	transport.ch <- []byte{0x00, 0x00, 0x00, 0x04}

	// Queue a valid PDU after the malformed one
	validPDU := &EnquireLink{Hdr: Header{CommandID: CommandIDEnquireLink, SequenceNumber: 1}}
	transport.ch <- encodeTestPDU(t, codec, validPDU)

	done := make(chan struct{})
	go func() {
		reader.Start(ctx)
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()
	<-done

	// Should have logged the malformed error and continued
	if n := errorLog.Load(); n != 1 {
		t.Errorf("expected 1 error log for malformed PDU, got %d", n)
	}
}

func TestReader_FatalTransportError_Exits(t *testing.T) {
	codec := NewCodec(Version3_4)
	transport := newReaderFakeTransport()
	dispatcher := NewDispatcher(nil, nil, nil, nil, nil, nil)
	errCh := make(chan error, 1)

	reader := NewReader(transport, codec, dispatcher, errCh)
	ctx := context.Background()

	// Close the transport — ReadPDU should return an error
	go func() {
		time.Sleep(10 * time.Millisecond)
		transport.Close()
	}()

	reader.Start(ctx)

	// Should have received a fatal error
	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("expected non-nil error, got nil")
		}
	default:
		t.Fatal("expected error on errCh, got nothing")
	}
}

func TestReader_CtxCancellation_ExitsClean(t *testing.T) {
	codec := NewCodec(Version3_4)
	transport := newReaderFakeTransport()
	dispatcher := NewDispatcher(nil, nil, nil, nil, nil, nil)
	errCh := make(chan error, 1)

	reader := NewReader(transport, codec, dispatcher, errCh)
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		reader.Start(ctx)
		close(done)
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()
	<-done

	// Should have exited with nil error
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("expected nil error for clean shutdown, got: %v", err)
		}
	default:
		// clean shutdown may not send anything — also fine
	}
}

func TestReader_DecodeError_Continues(t *testing.T) {
	codec := NewCodec(Version3_4)
	transport := newReaderFakeTransport()
	var errorLog atomic.Int32
	dispatcher := NewDispatcher(nil, nil, nil, nil, nil, nil)
	dispatcher.OnError = func(err error) { errorLog.Add(1) }
	errCh := make(chan error, 1)

	reader := NewReader(transport, codec, dispatcher, errCh)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Queue a PDU with length > actual data (truncated body)
	transport.ch <- []byte{
		0x00, 0x00, 0x00, 0x20, // length = 32 (but only 16 bytes follow)
		0x00, 0x00, 0x00, 0x04, // command_id = submit_sm
		0x00, 0x00, 0x00, 0x00, // command_status
		0x00, 0x00, 0x00, 0x01, // seq = 1
		// truncated — body claims 16 more bytes but only 0 provided
	}

	done := make(chan struct{})
	go func() {
		reader.Start(ctx)
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()
	<-done

	if n := errorLog.Load(); n != 1 {
		t.Errorf("expected 1 decode error log, got %d", n)
	}
}

// collectorPendingHandler implements PendingResponseHandler with a hook.
type collectorPendingHandler struct {
	hook func(resp PDU)
}

func (c *collectorPendingHandler) HandleResponse(resp PDU) {
	if c.hook != nil {
		c.hook(resp)
	}
}

func TestReader_MultiplePDUs_InOrder(t *testing.T) {
	codec := NewCodec(Version3_4)
	transport := newReaderFakeTransport()

	var mu sync.Mutex
	var seqs []uint32
	var callCount int32

	// Custom handler that collects seq numbers in order
	pending := &collectorPendingHandler{
		hook: func(resp PDU) {
			mu.Lock()
			seqs = append(seqs, resp.Header().SequenceNumber)
			callCount++
			mu.Unlock()
		},
	}

	dispatcher := NewDispatcher(pending, nil, nil, nil, nil, nil)
	errCh := make(chan error, 1)

	reader := NewReader(transport, codec, dispatcher, errCh)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Queue 3 PDUs in sequence
	for _, seq := range []uint32{10, 20, 30} {
		pdu := &SubmitSMResp{
			Hdr:       Header{CommandID: CommandIDSubmitSMResp, SequenceNumber: seq, CommandStatus: StatusOK},
			MessageID: "",
		}
		transport.ch <- encodeTestPDU(t, codec, pdu)
	}

	done := make(chan struct{})
	go func() {
		reader.Start(ctx)
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()
	<-done

	mu.Lock()
	defer mu.Unlock()
	if callCount != 3 {
		t.Errorf("expected 3 handler calls, got %d", callCount)
	}
	if len(seqs) != 3 || seqs[0] != 10 || seqs[1] != 20 || seqs[2] != 30 {
		t.Errorf("expected seqs [10,20,30], got %v", seqs)
	}
}
