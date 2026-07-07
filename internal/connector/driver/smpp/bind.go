package smpp

import (
	"context"
	"fmt"
)

// ── BindTransceiver Helper ────────────────────────────────────────────────────

const defaultInterfaceVersion uint8 = 0x34 // SMPP 3.4

// NewBindTransceiver creates a BindTransceiver PDU with sensible defaults.
// The InterfaceVersion is set to SMPP 3.4 (0x34). TON/NPI default to 0
// (unknown) and AddressRange is empty, which is correct for most ESMEs.
func NewBindTransceiver(systemID, password, systemType string) *BindTransceiver {
	return &BindTransceiver{
		Hdr: Header{
			CommandID:     CommandIDBindTransceiver,
			CommandStatus: StatusOK,
		},
		SystemID:         systemID,
		Password:         password,
		SystemType:       systemType,
		InterfaceVersion: defaultInterfaceVersion,
		AddrTON:          0,
		AddrNPI:          0,
		AddressRange:     "",
	}
}

// ── Session.Bind() ───────────────────────────────────────────────────────────

// Bind sends a BindTransceiver PDU and waits for the correlated response.
//
// Bind uses Session.SendRequest internally — the same path as SubmitSM,
// Unbind, and EnquireLink. There is no special bind path. This guarantees
// uniform behaviour: window serialization, pending registration, context
// timeout, and shutdown cleanup all work identically for Bind and for
// every other request/response PDU.
//
// State flow:
//
//	current        → Bind() → target
//	Connecting     → StateBinding → (wait) → StateBound
//	Binding        → (already there) → StateBound
//	Bound          → ErrInvalidState (already bound)
//	Disconnected   → ErrInvalidState (use Connect() instead)
//
// On failure the session transitions to StateDisconnected so the caller
// can retry with a fresh Connect cycle.
func (s *Session) Bind(ctx context.Context, bindPDU *BindTransceiver) error {
	// Transition to Binding (idempotent — skip if already Binding)
	s.mu.Lock()
	switch s.state {
	case StateBinding:
		// already there — proceed
	case StateConnecting, StateDisconnected:
		if err := s.transitionLocked(StateBinding); err != nil {
			s.mu.Unlock()
			return err
		}
	default:
		s.mu.Unlock()
		return fmt.Errorf("%w: bind requires Connecting/Binding state, got %s",
			ErrInvalidState, s.state)
	}
	s.mu.Unlock()

	resp, err := s.SendRequest(ctx, bindPDU)
	if err != nil {
		s.setState(StateDisconnected)
		return fmt.Errorf("smpp: bind: %w", err)
	}

	btr, ok := resp.(*BindTransceiverResp)
	if !ok {
		s.setState(StateDisconnected)
		return fmt.Errorf("smpp: bind: unexpected response type %T", resp)
	}
	if !btr.Hdr.CommandStatus.IsOK() {
		s.setState(StateDisconnected)
		return fmt.Errorf("%w: SMSC replied %s", ErrBindFailed, btr.Hdr.CommandStatus)
	}

	if err := s.transitionTo(StateBound); err != nil {
		return err
	}

	return nil
}
