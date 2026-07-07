package smpp

import (
	"context"
	"errors"
	"sync"
)

// ── WriteQueue ───────────────────────────────────────────────────────────────
//
// WriteQueue decouples PDU sending from both the Reader goroutine and the
// caller's goroutine. Handlers (like EnquireLink auto-response and
// DeliverSMResp) enqueue PDUs here instead of writing directly to the
// transport. A dedicated write goroutine in Session drains this queue.
//
// This prevents:
//   - Reader blocking on transport writes (Reader must only read).
//   - Callers blocking on flow control (the queue absorbs bursts).
//   - Concurrent WritePDU calls from multiple goroutines (queue serializes).
//
// Backpressure policy:
//
//	Enqueue(ctx, pdu)  — blocking: waits until the PDU is queued or ctx is
//	                     cancelled. Use for application goroutines (SubmitSM)
//	                     that can tolerate backpressure. The context allows
//	                     the caller to bound their wait time.
//
//	TryEnqueue(pdu)    — non-blocking: returns ErrQueueFull immediately if
//	                     the queue is full. Use for Reader/Dispatcher handlers
//	                     (DeliverSMResp, EnquireLinkResp) that MUST NOT block
//	                     the Reader goroutine. If the queue is full, the
//	                     response is dropped — the SMSC will retry or time out.

// ErrQueueFull is returned by TryEnqueue when the write queue has reached its
// capacity. The caller should drop the PDU and move on — never block the Reader.
var ErrQueueFull = errors.New("write queue: queue is full")

// WriteQueue usage rules (MUST follow throughout the project):
//
//	TryEnqueue / TryEnqueueEncoded — ONLY for Reader/Dispatcher handlers.
//	   These are non-blocking. If the queue is full, drop the PDU.
//	   Blocking the Reader goroutine is NEVER acceptable.
//
//	Enqueue / EnqueueEncoded — ONLY for application goroutines
//	   (Session.Submit, Session.Bind, Session.Unbind, Heartbeat).
//	   These CAN block on backpressure. The caller provides a context
//	   to bound their wait time.
//
//	After Stop() — no Enqueue/TryEnqueue should succeed.
//	   Both return the internal context error immediately.
//
// Vioalting these rules WILL block the Reader goroutine and WILL

// WriteQueue is a bounded, goroutine-safe queue of PDUs to send.
type WriteQueue struct {
	ch     chan writeItem
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

type writeItem struct {
	hdr     Header
	encoded []byte // pre-encoded bytes or nil
	pdu     PDU    // to encode at send time
}

// NewWriteQueue creates a write queue with the given buffer size.
// bufSize: how many PDUs can be queued before backpressure kicks in.
// A good starting value is window_size * 2 or 10-100 for most SMPP setups.
func NewWriteQueue(bufSize int) *WriteQueue {
	ctx, cancel := context.WithCancel(context.Background())
	return &WriteQueue{
		ch:     make(chan writeItem, bufSize),
		ctx:    ctx,
		cancel: cancel,
	}
}

// Enqueue adds a PDU to the send queue, blocking if the queue is full.
//
// Blocking behavior:
//   - If the queue has room: returns immediately.
//   - If the queue is full: blocks until a slot frees up or ctx is cancelled.
//   - If the queue's internal context is cancelled (Stop called): returns
//     the internal context error immediately.
//
// Use for application goroutines (SubmitSM, BindTransceiver) where
// blocking on backpressure is acceptable and expected.
func (wq *WriteQueue) Enqueue(ctx context.Context, pdu PDU) error {
	select {
	case wq.ch <- writeItem{pdu: pdu}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-wq.ctx.Done():
		return wq.ctx.Err()
	}
}

// TryEnqueue adds a PDU to the send queue without blocking.
// Returns ErrQueueFull immediately if the queue is at capacity.
//
// Use for Reader/Dispatcher handler paths where blocking is NOT allowed.
// The caller (handler) must drop the PDU on ErrQueueFull and continue.
func (wq *WriteQueue) TryEnqueue(pdu PDU) error {
	select {
	case wq.ch <- writeItem{pdu: pdu}:
		return nil
	default:
		return ErrQueueFull
	}
}

// EnqueueEncoded adds a pre-encoded PDU to the send queue, blocking if full.
// Useful for responses that were already encoded during dispatch.
func (wq *WriteQueue) EnqueueEncoded(ctx context.Context, hdr Header, data []byte) error {
	select {
	case wq.ch <- writeItem{hdr: hdr, encoded: data}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-wq.ctx.Done():
		return wq.ctx.Err()
	}
}

// TryEnqueueEncoded adds a pre-encoded PDU without blocking.
func (wq *WriteQueue) TryEnqueueEncoded(hdr Header, data []byte) error {
	select {
	case wq.ch <- writeItem{hdr: hdr, encoded: data}:
		return nil
	default:
		return ErrQueueFull
	}
}

// Start begins draining the queue in a background goroutine.
// For each queued PDU, it encodes (if not already) and writes to the transport.
// Exits when the queue context is cancelled (via Stop()).
func (wq *WriteQueue) Start(transport SMPPTransport, codec *Codec) {
	wq.wg.Add(1)
	go func() {
		defer wq.wg.Done()
		for {
			select {
			case item, ok := <-wq.ch:
				if !ok {
					return
				}
				var data []byte
				if item.encoded != nil {
					data = item.encoded
				} else if item.pdu != nil {
					var err error
					data, err = codec.Encode(item.pdu)
					if err != nil {
						continue
					}
				} else {
					continue
				}
				_ = transport.WritePDU(wq.ctx, data)
			case <-wq.ctx.Done():
				return
			}
		}
	}()
}

// Stop cancels the queue context and waits for the writer goroutine to exit.
//
// Stop behavior:
//   - Cancels the internal context immediately.
//   - The writer goroutine exits on the next select iteration (after current
//     write completes — no mid-write cancellation).
//   - Any PDUs remaining in the buffer are DROPPED (not drained).
//   - After Stop, Enqueue/TryEnqueue return the internal context error.
//
// Design rationale: during shutdown, PendingStore.Clear() fails all
// outstanding requests. The responses for those requests (if any) are
// irrelevant. Draining would unnecessarily delay shutdown.
func (wq *WriteQueue) Stop() {
	wq.cancel()
	wq.wg.Wait()
}

// Len returns the number of queued but unsent PDUs.
func (wq *WriteQueue) Len() int {
	return len(wq.ch)
}
