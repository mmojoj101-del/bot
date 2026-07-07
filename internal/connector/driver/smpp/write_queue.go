package smpp

import (
	"context"
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

// WriteQueue is a bounded, goroutine-safe queue of PDUs to send.
type WriteQueue struct {
	ch    chan writeItem
	ctx   context.Context
	cancel context.CancelFunc
	wg    sync.WaitGroup
}

type writeItem struct {
	hdr     Header
	encoded []byte // pre-encoded bytes or nil
	pdu     PDU    // to encode at send time
}

// NewWriteQueue creates a write queue with the given buffer size.
// bufSize: number of PDUs that can be queued before backpressure.
func NewWriteQueue(bufSize int) *WriteQueue {
	ctx, cancel := context.WithCancel(context.Background())
	return &WriteQueue{
		ch:     make(chan writeItem, bufSize),
		ctx:    ctx,
		cancel: cancel,
	}
}

// Enqueue adds a PDU to the send queue.
// Non-blocking if the queue has room; blocks if the queue is full.
// Returns an error if the queue is closed.
func (wq *WriteQueue) Enqueue(pdu PDU) error {
	select {
	case wq.ch <- writeItem{pdu: pdu}:
		return nil
	case <-wq.ctx.Done():
		return wq.ctx.Err()
	}
}

// EnqueueEncoded adds a pre-encoded PDU to the send queue.
// Useful for responses that were already encoded during dispatch.
func (wq *WriteQueue) EnqueueEncoded(hdr Header, data []byte) error {
	select {
	case wq.ch <- writeItem{hdr: hdr, encoded: data}:
		return nil
	case <-wq.ctx.Done():
		return wq.ctx.Err()
	}
}

// Start begins draining the queue in a goroutine.
// For each queued PDU, it encodes (if not already) and writes to the transport.
// Exits when the queue context is cancelled.
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
				// Write to transport (context from queue owns the lifecycle)
				_ = transport.WritePDU(wq.ctx, data)
			case <-wq.ctx.Done():
				return
			}
		}
	}()
}

// Stop cancels the queue context and waits for the writer goroutine to exit.
func (wq *WriteQueue) Stop() {
	wq.cancel()
	wq.wg.Wait()
}

// Len returns the number of queued but unsent PDUs.
func (wq *WriteQueue) Len() int {
	return len(wq.ch)
}
