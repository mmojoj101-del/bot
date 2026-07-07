package smpp

import (
	"context"
	"sync"
	"time"
)

// ── Heartbeat ────────────────────────────────────────────────────────────────
//
// Heartbeat sends period enquire_link PDUs to keep the SMPP session alive.
// It uses Session.SendRequest — the same path as every other request/response
// PDU. This guarantees uniform behaviour: window serialization, pending
// registration, context timeout, and shutdown cleanup.
//
// Design decisions:
//
//   - Heartbeat is a separate struct (not a field on Session) so the caller
//     decides whether to enable it. Session does NOT start heartbeats
//     automatically — the SMPPDriver or the application starts them.
//
//   - The ticker period is configurable. SMPP spec recommends 30-60 seconds,
//     but actual SMSCs may require shorter intervals. The default is 30s.
//
//   - A failed enquire_link does NOT tear down the session. The Driver's
//     reconnect logic handles that. Heartbeat just reports the error through
//     an optional callback.
//
//   - Heartbeat stops when the provided context is cancelled. This is usually
//     the session context, so Disconnect() cancels the context → Heartbeat
//     stops automatically.
//
//   - Heartbeat uses a single-fire ticker pattern (Reset after each send)
//     rather than a persistent ticker, so interval is measured from response
//     receipt (or failure), not from send time. This prevents overlap when
//     the SMSC is slow.

// HeartbeatConfig holds heartbeat parameters.
type HeartbeatConfig struct {
	Interval time.Duration // time between enquire_link sends (default: 30s)
	Timeout  time.Duration // per-request timeout (default: interval / 2)
	OnError  func(error)   // optional: called when enquire_link fails
}

// DefaultHeartbeatInterval is the recommended interval between heartbeats.
const DefaultHeartbeatInterval = 30 * time.Second

// Heartbeat manages periodic enquire_link PDUs for session keep-alive.
// Create with NewHeartbeat, then call Start to begin.
type Heartbeat struct {
	session *Session
	cfg     HeartbeatConfig

	mu     sync.Mutex
	running bool
}

// NewHeartbeat creates a Heartbeat that sends enquire_link PDUs through
// the given Session. The caller must call Start() to begin.
func NewHeartbeat(session *Session, cfg HeartbeatConfig) *Heartbeat {
	if cfg.Interval <= 0 {
		cfg.Interval = DefaultHeartbeatInterval
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = cfg.Interval / 2
		if cfg.Timeout < time.Second {
			cfg.Timeout = time.Second
		}
	}
	return &Heartbeat{
		session: session,
		cfg:     cfg,
	}
}

// Start begins sending periodic enquire_link PDUs on the session.
// It blocks until ctx is cancelled (typically the session context).
// Start returns immediately if the heartbeat is already running.
//
// Call as a goroutine:
//
//	go hb.Start(ctx)
func (h *Heartbeat) Start(ctx context.Context) {
	h.mu.Lock()
	if h.running {
		h.mu.Unlock()
		return
	}
	h.running = true
	h.mu.Unlock()

	defer func() {
		h.mu.Lock()
		h.running = false
		h.mu.Unlock()
	}()

	timer := time.NewTimer(h.cfg.Interval)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			h.sendHeartbeat(ctx)
			// Reset timer from now (not from the original send time)
			timer.Reset(h.cfg.Interval)
		}
	}
}

// sendHeartbeat performs a single enquire_link request/response cycle.
func (h *Heartbeat) sendHeartbeat(parentCtx context.Context) {
	// Per-request timeout so one slow response doesn't stall all heartbeats
	ctx, cancel := context.WithTimeout(parentCtx, h.cfg.Timeout)
	defer cancel()

	pdu := &EnquireLink{
		Hdr: Header{CommandID: CommandIDEnquireLink},
	}

	_, err := h.session.SendRequest(ctx, pdu)
	if err != nil {
		if h.cfg.OnError != nil {
			h.cfg.OnError(err)
		}
	}
}

// Running reports whether the heartbeat loop is active.
func (h *Heartbeat) Running() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.running
}
