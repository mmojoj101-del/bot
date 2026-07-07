package smpp

import (
	"sync"
	"time"
)

// PendingRequest represents an outstanding request waiting for a response.
//
// The caller creates a PendingRequest, registers it with PendingStore, sends the
// PDU on the transport, then waits on the Response channel for the correlated
// response or timeout.
//
// Timeout cleanup prevents memory leaks when the SMSC silently drops requests:
//   - Store has a background goroutine (or explicit call) that removes timed-out entries.
//   - On reconnect, all pending requests are cancelled (Response channel closed).
type PendingRequest struct {
	Seq       uint32
	CommandID CommandID
	CreatedAt time.Time
	Deadline  time.Time
	Response  chan PDU // buffered(1), receives the response PDU
	TraceID   string
}

// PendingStore correlates outgoing requests (by sequence number) with their responses.
//
// SMPP is asynchronous: SubmitSM goes out on the socket, SubmitSMResp comes back
// with the same sequence number. PendingStore bridges the two by providing a
// channel-based wait mechanism.
//
// Lifecycle of a pending request:
//  1. SubmitSM flow: Acquire seq → Register(seq, cmd) → write PDU to socket
//  2. Reader receives SubmitSMResp → PendingStore.Notify(seq, respPDU)
//  3. Notify delivers to the waiter's channel and removes from map
//  4. If timeout: caller's ctx expires → Remove(seq) → channel close
//
// Without this store, correlating 100s of concurrent SubmitSM responses would be
// impossible under windowed operation.
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
// Returns the PendingRequest so the caller can wait on Response.
func (ps *PendingStore) Register(seq uint32, cmdID CommandID, deadline time.Time, traceID string) *PendingRequest {
	pr := &PendingRequest{
		Seq:       seq,
		CommandID: cmdID,
		CreatedAt: time.Now(),
		Deadline:  deadline,
		Response:  make(chan PDU, 1),
		TraceID:   traceID,
	}
	ps.mu.Lock()
	ps.requests[seq] = pr
	ps.mu.Unlock()
	return pr
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

	if ok && pr.Response != nil {
		close(pr.Response)
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
	now := time.Now()
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	var expired []uint32
	for seq, pr := range ps.requests {
		if now.After(pr.Deadline) {
			expired = append(expired, seq)
		}
	}
	return expired
}

// Clear removes and closes all pending requests (used on disconnect/reconnect).
func (ps *PendingStore) Clear() {
	ps.mu.Lock()
	requests := ps.requests
	ps.requests = make(map[uint32]*PendingRequest)
	ps.mu.Unlock()

	for _, pr := range requests {
		if pr.Response != nil {
			close(pr.Response)
		}
	}
}
