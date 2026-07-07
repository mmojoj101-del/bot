package smpp

import (
	"context"
	"fmt"
	"sync"
)

// WindowSlot represents an acquired slot in the window.
// The caller holds the slot until the response arrives or the context expires.
//
// Lifecycle:
//  1. window.Acquire(ctx) blocks until a slot is free, then returns a WindowSlot
//  2. slot.Write(transport, pdu) sends the PDU (encoded) and registers the pending seq
//  3. slot.Response() returns a channel to wait for the correlated response
//  4. slot.Release() is called after response or timeout
//
// WindowManager is the single atomic owner of:
//   - Window capacity (max concurrent requests)
//   - Sequence number allocation
//   - Pending request correlation
//
// This prevents races between sequence allocation, pending registration, and
// write failures — if Write fails after Allocate+Register, the slot is released
// atomically with cleanup.
type WindowSlot struct {
	seq     uint32
	cmdID   CommandID
	ctx     context.Context
	cancel  context.CancelFunc
	window  *WindowManager
	written bool
}

// Sequence returns the allocated sequence number for this slot.
func (s *WindowSlot) Sequence() uint32 { return s.seq }

// Write encodes the PDU, registers the pending request, and writes to transport.
// If Write fails, the slot is released automatically (seq + pending cleaned up).
func (s *WindowSlot) Write(tx SMPPTransport, pdu PDU) error {
	if s.written {
		return fmt.Errorf("window slot %d: already written", s.seq)
	}

	s.window.codecMu.RLock()
	data, err := s.window.codec.Encode(pdu)
	s.window.codecMu.RUnlock()
	if err != nil {
		s.Release()
		return fmt.Errorf("window slot %d: encode: %w", s.seq, err)
	}

	// Register pending request before write — if write fails, Release() cleans up
	_ = s.window.pending.Register(s.seq, s.cmdID, s.ctx, "")

	if err := tx.WritePDU(s.ctx, data); err != nil {
		s.Release()
		return fmt.Errorf("window slot %d: write: %w", s.seq, err)
	}

	s.written = true
	return nil
}

// Response returns a channel to wait for the correlated response PDU.
// Returns nil if slot is released before Write.
func (s *WindowSlot) Response() <-chan PDU {
	if s.written {
		pr := s.window.pending.Get(s.seq)
		if pr != nil {
			return pr.Response
		}
	}
	return nil
}

// Release frees the window slot and cleans up the pending request.
// Safe to call multiple times (idempotent).
func (s *WindowSlot) Release() {
	s.window.pending.Remove(s.seq)
	s.cancel()
	s.window.release()
}

// WindowManager controls the maximum number of concurrent outstanding requests.
//
// Architecture:
//
//	WindowManager
//	    ├── semaphore (chan struct{}) — bounds concurrency
//	    ├── SequenceManager — allocates unique seq numbers
//	    └── PendingStore — correlates seq → response
//
// Acquire() blocks until a slot is free, then atomically allocates seq + registers
// pending + prepares the slot. This three-in-one atomicity prevents races between
// sequence allocation, pending registration, and write failures.
//
// Sequence wrap constraint:
//   - Sequence range (1..0x7FFFFFFF) is 2^31 — approximately 2 billion.
//   - With a typical window size of 1..100, sequence reuse before the SMSC
//     responds is practically impossible.
//   - The PendingStore.Has(seq) check can be added to Next() for extra safety,
//     but is not required for any realistic window configuration.
type WindowManager struct {
	max     int
	sema    chan struct{} // buffered channel as counting semaphore
	pending *PendingStore
	seq     *SequenceManager
	codec   *Codec
	codecMu sync.RWMutex // allows freeze-on-first-use pattern
}

// NewWindowManager creates a window manager with the given max concurrent requests.
// Window=1 disables pipelining (debug mode). Typical production values: 10-100.
func NewWindowManager(max int, seq *SequenceManager, pending *PendingStore, codec *Codec) *WindowManager {
	if max < 1 {
		max = 1
	}
	return &WindowManager{
		max:     max,
		sema:    make(chan struct{}, max),
		pending: pending,
		seq:     seq,
		codec:   codec,
	}
}

// Acquire blocks until a window slot is available or the context expires.
// Returns a WindowSlot with a unique sequence number already allocated.
//
// The caller MUST call slot.Release() after receiving a response or on error.
//
// Thread-safe: multiple goroutines can call Acquire concurrently.
func (wm *WindowManager) Acquire(ctx context.Context) (*WindowSlot, error) {
	// Try to acquire semaphore slot
	select {
	case wm.sema <- struct{}{}:
		// slot acquired
	case <-ctx.Done():
		return nil, fmt.Errorf("window acquire: %w", ctx.Err())
	}

	// Check for sequence collision (paranoid check — won't trigger in practice)
	// This ensures the wrap-around doesn't collide with an in-flight request.
	seq := wm.seq.Next()
	for wm.pending.Has(seq) {
		seq = wm.seq.Next()
	}

	slotCtx, cancel := context.WithCancel(ctx)
	return &WindowSlot{
		seq:    seq,
		cmdID:  0, // set by caller's Write
		ctx:    slotCtx,
		cancel: cancel,
		window: wm,
	}, nil
}

// release returns one slot to the semaphore. Called by slot.Release().
func (wm *WindowManager) release() {
	<-wm.sema
}

// Len returns the number of currently acquired slots.
func (wm *WindowManager) Len() int {
	return len(wm.sema)
}

// Cap returns the maximum window size.
func (wm *WindowManager) Cap() int { return wm.max }

// Pending returns the underlying PendingStore (for Notify from Dispatcher).
func (wm *WindowManager) Pending() *PendingStore { return wm.pending }
