package smpp

import (
	"context"
	"sync"
	"time"
)

// PendingRequest represents an outstanding request waiting for a response.
//
// The caller creates a PendingRequest, registers it with PendingStore, sends the
// PDU on the transport, then waits on the Response channel for the correlated
// response or context cancellation.
//
// Timeout cleanup prevents memory leaks when the SMSC silently drops requests:
//   - A background cleanup goroutine calls TimedOut() → Remove() periodically.
//   - On disconnect/reconnect, Clear() cancels all pending via ctx cancellation.
type PendingRequest struct {
	Seq       uint32
	CommandID CommandID
	CreatedAt time.Time
	Ctx       context.Context
	Cancel    context.CancelFunc
	Response  chan PDU // buffered(1), receives the response PDU
	TraceID   string
}

// Deadline returns the timeout from the context, if set.
func (pr *PendingRequest) Deadline() (time.Time, bool) {
	return pr.Ctx.Deadline()
}

// PendingStore correlates outgoing requests (by sequence number) with their responses.
//
// SMPP is asynchronous: SubmitSM goes out on the socket, SubmitSMResp comes back
// with the same sequence number. PendingStore bridges the two by providing a
// channel-based wait mechanism.
//
// Lifecycle of a pending request (orchestrated by WindowManager):
//  1. WindowManager.Acquire(ctx) → seq, slot
//  2. slot.Write(transport, pdu)
//  3. Dispatcher receives SubmitSMResp → PendingStore.Notify(seq, respPDU)
//  4. Notify delivers to the waiter's response channel and removes from map
//  5. If context expires: slot.Cancel() → Remove(seq) → channel close
//
// PendingStore knows NOTHING about sequence generation or window limits.
// It is a pure correlation map. WindowManager owns both SequenceManager
// and PendingStore.
type PendingStore struct {
	mu       sync.RWMutex
	requests map[uint32]*PendingRequest
}

// NewPendingStore creates an empty pending store.
func NewPendingStore() *PendingStore {
	return &PendingStore{
		requests: make(map[uint32]*PendingRequest),
	}
}

// Register adds a pending request keyed by sequence number.
// The ctx is stored for deadline-based cleanup and cancellation.
func (ps *PendingStore) Register(seq uint32, cmdID CommandID, ctx context.Context, traceID string) *PendingRequest {
	ctx, cancel := context.WithCancel(ctx)
	pr := &PendingRequest{
		Seq:       seq,
		CommandID: cmdID,
		CreatedAt: time.Now(),
		Ctx:       ctx,
		Cancel:    cancel,
		Response:  make(chan PDU, 1),
		TraceID:   traceID,
	}
	ps.mu.Lock()
	ps.requests[seq] = pr
	ps.mu.Unlock()
	return pr
}

// Get returns the pending request for a given sequence number, or nil.
func (ps *PendingStore) Get(seq uint32) *PendingRequest {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	return ps.requests[seq]
}

// Has returns true if the given sequence number is registered.
func (ps *PendingStore) Has(seq uint32) bool {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	_, ok := ps.requests[seq]
	return ok
}

// Notify delivers a response PDU to the waiter identified by sequence number.
// Returns true if a waiter was found and notified. Removes the entry from the store.
func (ps *PendingStore) Notify(seq uint32, resp PDU) bool {
	ps.mu.Lock()
	pr, ok := ps.requests[seq]
	if ok {
		delete(ps.requests, seq)
	}
	ps.mu.Unlock()

	if !ok {
		return false
	}

	pr.Cancel() // cancel the context — no longer pending

	// Non-blocking send: channel is buffered(1), receiver is waiting
	select {
	case pr.Response <- resp:
	default:
	}
	return true
}

// Remove explicitly removes a pending request (e.g., on timeout or cancellation).
// Closes the Response channel to signal the waiter.
func (ps *PendingStore) Remove(seq uint32) {
	ps.mu.Lock()
	pr, ok := ps.requests[seq]
	if ok {
		delete(ps.requests, seq)
	}
	ps.mu.Unlock()

	if ok {
		pr.Cancel()
		if pr.Response != nil {
			close(pr.Response)
		}
	}
}

// Len returns the number of outstanding pending requests (current window fill).
func (ps *PendingStore) Len() int {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	return len(ps.requests)
}

// TimedOut returns the sequence numbers of all expired requests.
// The caller should call Remove() for each returned seq.
func (ps *PendingStore) TimedOut() []uint32 {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	var expired []uint32
	for seq, pr := range ps.requests {
		if pr.Ctx.Err() != nil {
			expired = append(expired, seq)
		}
	}
	return expired
}

// Clear removes and cancels all pending requests (used on disconnect/reconnect).
// After Clear, the store is empty and ready for new registrations.
func (ps *PendingStore) Clear() {
	ps.mu.Lock()
	requests := ps.requests
	ps.requests = make(map[uint32]*PendingRequest)
	ps.mu.Unlock()

	for _, pr := range requests {
		pr.Cancel()
		if pr.Response != nil {
			close(pr.Response)
		}
	}
}
