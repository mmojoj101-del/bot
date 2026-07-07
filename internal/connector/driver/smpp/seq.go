package smpp

import "sync/atomic"

// SequenceManager allocates monotonically increasing sequence numbers for SMPP PDUs.
//
// SMPP v3.4 specification rules:
//   - SequenceNumber must be unique across all outstanding requests in a session.
//   - SequenceNumber 0 is reserved and MUST NOT be used (SMPP spec §5.1).
//   - The range is 0x00000001 to 0x7FFFFFFF (signed int32 positive).
//   - Upon reaching 0x7FFFFFFF, the counter wraps to 1 (not 0).
//   - The ESME and SMSC each maintain their own independent sequence counter.
//
// This implementation:
//   - Starts at 1 and increments atomically.
//   - Wraps to 1 (not 0) when exceeding 0x7FFFFFFF.
//   - Is safe for concurrent use via atomic.Uint64.
//
// TODO: In future, reclaim seq numbers from timed-out requests to prevent
// premature exhaustion under high throughput with large windows.
type SequenceManager struct {
	counter atomic.Uint64
}

// NewSequenceManager creates a new sequence allocator starting at 1.
func NewSequenceManager() *SequenceManager {
	sm := &SequenceManager{}
	sm.counter.Store(0) // Next() will add 1, so first call returns 1
	return sm
}

// Next returns the next available sequence number.
// Always between 1 and 0x7FFFFFFF inclusive. Wraps to 1 on overflow.
func (sm *SequenceManager) Next() uint32 {
	for {
		cur := sm.counter.Add(1)
		seq := uint32(cur)
		// Skip 0 and wrap at 0x7FFFFFFF
		if seq == 0 || seq > 0x7FFFFFFF {
			sm.counter.Store(0)
			continue
		}
		return seq
	}
}

// Current returns the last allocated sequence number (for debugging).
func (sm *SequenceManager) Current() uint32 {
	return uint32(sm.counter.Load())
}
