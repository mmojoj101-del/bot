package smpp

import (
	"context"
	"fmt"
)

// ── Reader ───────────────────────────────────────────────────────────────────

// Reader reads SMPP PDUs from a transport in a single goroutine.
//
// It is deliberately "dumb":
//   - ReadPDU from transport
//   - Decode bytes into PDU
//   - Dispatch to the appropriate handler
//
// It knows NOTHING about:
//   - Session state
//   - WindowManager internals
//   - PendingStore
//   - Retry or reconnect logic
//
// Error handling:
//   - Decode error (malformed PDU): per Codec contract, the bytes are fully
//     consumed and the stream position is correct. The error is NON-FATAL —
//     log and continue reading. Reader does NOT make this decision; it follows
//     the Codec's error contract.
//   - Transport error (EOF, reset, timeout): fatal — the stream is broken.
//     The Reader exits and sends the error to errCh.
//   - ctx cancellation: clean shutdown, Reader exits with nil error.
//
// The Reader exits when:
//   - ctx is cancelled (session shutdown)
//   - transport.ReadPDU returns a fatal error (EOF, reset)
//
// On exit, the error (if any) is sent to errCh so the Session can react.
// errCh must be buffered (cap >= 1) or use non-blocking receive in Session.
type Reader struct {
	transport  SMPPTransport
	codec      *Codec
	dispatcher *Dispatcher
	errCh      chan<- error
}

// NewReader creates a Reader.
// errCh receives a fatal error when the reader exits (may be nil for clean shutdown).
func NewReader(transport SMPPTransport, codec *Codec, dispatcher *Dispatcher, errCh chan<- error) *Reader {
	return &Reader{
		transport:  transport,
		codec:      codec,
		dispatcher: dispatcher,
		errCh:      errCh,
	}
}

// Start begins the read loop in the current goroutine.
// It blocks until ctx is cancelled or a fatal transport error occurs.
//
// Expected call pattern:
//
//	go reader.Start(ctx)
func (r *Reader) Start(ctx context.Context) {
	for {
		// Read a complete PDU from the transport
		data, err := r.transport.ReadPDU(ctx)
		if err != nil {
			// If the context was cancelled, this is a clean shutdown
			if ctx.Err() != nil {
				r.safeSend(nil) // nil = clean shutdown
				return
			}
			r.safeSend(fmt.Errorf("reader: %w", err))
			return
		}

		// Decode the PDU
		pdu, err := r.codec.Decode(data)
		if err != nil {
			// Malformed PDU — log and continue (don't kill the session)
			if r.dispatcher.OnError != nil {
				r.dispatcher.OnError(fmt.Errorf("reader: decode: %w", err))
			}
			continue
		}

		// Dispatch to the appropriate handler
		r.dispatcher.Dispatch(pdu)
	}
}

// safeSend sends err to errCh without blocking.
// If errCh is full or nil, the error is dropped (Reader is shutting down).
func (r *Reader) safeSend(err error) {
	if r.errCh == nil {
		return
	}
	select {
	case r.errCh <- err:
	default:
	}
}
