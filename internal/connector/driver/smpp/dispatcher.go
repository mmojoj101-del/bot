package smpp

import (
	"fmt"
	"sync/atomic"
)

// ── Handler Interfaces ───────────────────────────────────────────────────────

// PendingResponseHandler handles all correlated response PDUs.
// This includes SubmitSMResp, EnquireLinkResp, and UnbindResp — any response
// that was registered as a pending request via WindowManager.
//
// The implementation calls pending.Notify(resp.Header().SequenceNumber, resp).
// It uses the sequence_number for correlation, NOT the PDU type.
// This means ANY response PDU (including vendor-specific SMPP 5.0) works
// automatically without handler changes.
type PendingResponseHandler interface {
	HandleResponse(resp PDU)
}

// DeliverSMHandler handles incoming deliver_sm PDUs (DLR or MO messages).
// Returns a response PDU to send back (typically *DeliverSMResp), or nil if
// no response is needed. Returns an error if handling failed.
//
// Using PDU as return type (not *DeliverSMResp) enables future response types
// like SMPP 5.0 vendor-specific responses without changing the interface.
type DeliverSMHandler interface {
	HandleDeliverSM(pdu *DeliverSM) (PDU, error)
}

// BindRespHandler handles incoming bind_transceiver_resp PDUs.
// Unblocks the Connect() call that is waiting for a bind response.
type BindRespHandler interface {
	HandleBindResp(resp *BindTransceiverResp)
}

// EnquireLinkHandler handles incoming enquire_link PDUs (heartbeat).
// The implementation must send an EnquireLinkResp — ideally via the
// same WindowManager path as any other PDU (no special heartbeat channel).
type EnquireLinkHandler interface {
	HandleEnquireLink(seq uint32)
}

// UnbindHandler handles incoming unbind PDUs from the remote side.
// Returns true if the session should respond with unbind_resp.
type UnbindHandler interface {
	HandleUnbind() (sendResp bool)
}

// GenericNackHandler handles incoming generic_nack PDUs.
type GenericNackHandler interface {
	HandleGenericNack(status CommandStatus)
}

// ── Handler Function ─────────────────────────────────────────────────────────

// DispatchFunc is a single-PDU handler registered in the dispatch table.
// Returns a response PDU to send back (if any), or nil.
type DispatchFunc func(pdu PDU) (respPDU PDU)

// ── Dispatcher ───────────────────────────────────────────────────────────────

// Dispatcher routes decoded PDUs by CommandID using a dispatch table.
//
// It is a pure Router — it knows NOTHING about:
//   - Session state
//   - WindowManager internals
//   - PendingStore
//   - Transport
//
// Handler concurrency contract (IMPORTANT):
//
//	Dispatch() is called from the single Reader goroutine.
//	Blocking in a handler WILL block the Reader (no more PDUs are read).
//	Therefore ALL handlers MUST be non-blocking:
//	  - Channel sends must use non-blocking select (or be buffered enough).
//	  - Transport writes must be asynchronous — the Session MUST provide
//	    a dedicated write goroutine (or write queue) so that handlers
//	    (like enquire_link auto-respond) enqueue writes instead of
//	    writing to the transport directly from the Reader goroutine.
//	  - No mutex-waiting on session state, no I/O, no time.Sleep.
//
//	If a handler needs to send a PDU (e.g., DeliverSMResp, EnquireLinkResp),
//	it should enqueue the PDU on a write channel and return immediately.
//	The Session's write goroutine drains that channel sequentially.
//
// Registration lifecycle:
//   - All standard registrations happen inside NewDispatcher().
//   - After NewDispatcher() returns, no Register() should be called.
//   - Freeze() and auto-freeze (first Dispatch) are safety nets to
//     prevent accidental concurrent modifications.
type Dispatcher struct {
	table map[CommandID]DispatchFunc
	frozen atomic.Bool

	pendingRespHandler PendingResponseHandler
	deliverSMHandler   DeliverSMHandler
	bindRespHandler    BindRespHandler
	enquireLinkHandler EnquireLinkHandler
	unbindHandler      UnbindHandler
	genericNackHandler GenericNackHandler

	// OnError is called when a handler returns an error or an unknown PDU
	// is received. Must be non-blocking.
	OnError func(err error)
}

// NewDispatcher creates a Dispatcher with the given handlers.
// All handlers are optional — nil handlers silently ignore the PDU.
// All standard dispatch entries are pre-registered. To add vendor-specific
// or custom PDU handlers, use Register() AFTER NewDispatcher returns but
// BEFORE the first Dispatch() call.
func NewDispatcher(
	pendingResp PendingResponseHandler,
	deliverSM DeliverSMHandler,
	bindResp BindRespHandler,
	enquireLink EnquireLinkHandler,
	unbind UnbindHandler,
	genericNack GenericNackHandler,
) *Dispatcher {
	d := &Dispatcher{
		table:              make(map[CommandID]DispatchFunc),
		pendingRespHandler: pendingResp,
		deliverSMHandler:   deliverSM,
		bindRespHandler:    bindResp,
		enquireLinkHandler: enquireLink,
		unbindHandler:      unbind,
		genericNackHandler: genericNack,
	}

	// Pre-register all standard dispatch entries.
	d.table[CommandIDSubmitSMResp] = d.dispatchSubmitSMResp
	d.table[CommandIDDeliverSM] = d.dispatchDeliverSM
	d.table[CommandIDBindTransceiverResp] = d.dispatchBindResp
	d.table[CommandIDEnquireLink] = d.dispatchEnquireLink
	d.table[CommandIDEnquireLinkResp] = d.dispatchPendingResponse
	d.table[CommandIDUnbind] = d.dispatchUnbind
	d.table[CommandIDUnbindResp] = d.dispatchPendingResponse
	d.table[CommandIDGenericNack] = d.dispatchGenericNack

	return d
}

// Register adds or replaces a dispatch entry for the given CommandID.
// Enables plugin-based dispatch: vendor PDUs or SMPP 5.0 extensions
// can be registered without modifying this package.
//
// MUST be called before the first Dispatch() call (initialization-only).
// Panics if the dispatcher is frozen (auto-frozen on first Dispatch).
func (d *Dispatcher) Register(cmd CommandID, fn DispatchFunc) {
	if d.frozen.Load() {
		panic("smpp: dispatcher is frozen — cannot register after first use")
	}
	d.table[cmd] = fn
}

// Freeze makes the dispatch table immutable.
// Call after all registrations are complete, before concurrent usage.
// Automatically called on first Dispatch() call as a safety net.
func (d *Dispatcher) Freeze() {
	d.frozen.Store(true)
}

// Dispatch routes a decoded PDU to the appropriate handler.
//
// Routing is O(1) map lookup by CommandID (not a switch statement).
// The dispatcher is frozen on first use (safety net — no further registration).
// Unknown CommandIDs are logged via OnError and silently dropped.
//
// Dispatch is non-blocking if all registered handlers are non-blocking.
func (d *Dispatcher) Dispatch(pdu PDU) {
	d.frozen.Store(true) // freeze on first use — safety net

	if pdu == nil || pdu.Header() == nil {
		return
	}

	cmdID := pdu.Header().CommandID

	fn, ok := d.table[cmdID]
	if !ok {
		d.safeOnError(fmt.Errorf("unknown PDU: %s seq=%d", cmdID, pdu.Header().SequenceNumber))
		return
	}

	// The returned response PDU is enqueued by the Session's write goroutine.
	// We ignore it here — the Session's DispatchHandler (wrapping Dispatcher)
	// catches the response and enqueues it on a write channel.
	_ = fn(pdu)
}

// ── Dispatch Funcs ───────────────────────────────────────────────────────────

func (d *Dispatcher) dispatchSubmitSMResp(pdu PDU) PDU {
	resp, ok := pdu.(*SubmitSMResp)
	if !ok || d.pendingRespHandler == nil {
		return nil
	}
	d.pendingRespHandler.HandleResponse(resp)
	return nil
}

func (d *Dispatcher) dispatchDeliverSM(pdu PDU) PDU {
	deliver, ok := pdu.(*DeliverSM)
	if !ok || d.deliverSMHandler == nil {
		return nil
	}
	resp, err := d.deliverSMHandler.HandleDeliverSM(deliver)
	if err != nil {
		d.safeOnError(fmt.Errorf("deliver_sm handler: %w", err))
	}
	return resp // Session's write goroutine will send this
}

func (d *Dispatcher) dispatchBindResp(pdu PDU) PDU {
	resp, ok := pdu.(*BindTransceiverResp)
	if !ok || d.bindRespHandler == nil {
		return nil
	}
	d.bindRespHandler.HandleBindResp(resp)
	return nil
}

func (d *Dispatcher) dispatchEnquireLink(pdu PDU) PDU {
	if d.enquireLinkHandler == nil {
		return nil
	}
	d.enquireLinkHandler.HandleEnquireLink(pdu.Header().SequenceNumber)
	return nil
}

// dispatchPendingResponse forwards EnquireLinkResp, UnbindResp, and any other
// response PDU to the same PendingResponseHandler used for SubmitSMResp.
// This means ALL correlated responses go through one path — no special casing.
// The handler uses sequence_number for correlation, not PDU type.
func (d *Dispatcher) dispatchPendingResponse(pdu PDU) PDU {
	if d.pendingRespHandler == nil {
		return nil
	}
	d.pendingRespHandler.HandleResponse(pdu)
	return nil
}

func (d *Dispatcher) dispatchUnbind(pdu PDU) PDU {
	if d.unbindHandler == nil {
		return nil
	}
	d.unbindHandler.HandleUnbind()
	return nil
}

func (d *Dispatcher) dispatchGenericNack(pdu PDU) PDU {
	if d.genericNackHandler == nil {
		return nil
	}
	d.genericNackHandler.HandleGenericNack(pdu.Header().CommandStatus)
	return nil
}

// safeOnError calls OnError without blocking.
// OnError is a function call on the Reader goroutine — it cannot block
// unless the handler implementation blocks (caller's responsibility).
func (d *Dispatcher) safeOnError(err error) {
	if d.OnError == nil {
		return
	}
	d.OnError(err)
}
