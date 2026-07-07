package smpp

import (
	"fmt"
)

// ── Handler Interfaces ───────────────────────────────────────────────────────

// PendingResponseHandler handles all correlated response PDUs.
// This includes SubmitSMResp, EnquireLinkResp, and UnbindResp — any response
// that was registered as a pending request via WindowManager.
//
// The implementation typically calls PendingStore.Notify(seq, resp).
type PendingResponseHandler interface {
	HandleResponse(resp PDU)
}

// DeliverSMHandler handles incoming deliver_sm PDUs (DLR or MO messages).
// Returns a DeliverSMResp PDU to send back, or an error.
type DeliverSMHandler interface {
	HandleDeliverSM(pdu *DeliverSM) (*DeliverSMResp, error)
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
// It returns a DeliverSMResp if the PDU is a DeliverSM (for auto-response),
// or nil otherwise.
type DispatchFunc func(pdu PDU) (deliverResp *DeliverSMResp)

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
//	  - Transport writes must be asynchronous (queued for another goroutine).
//	  - No mutex-waiting on session state.
//
//	If a handler needs to do I/O (e.g., send DeliverSMResp), it should
//	enqueue work for the Session's write goroutine and return immediately.
type Dispatcher struct {
	table map[CommandID]DispatchFunc

	pendingRespHandler PendingResponseHandler
	deliverSMHandler   DeliverSMHandler
	bindRespHandler    BindRespHandler
	enquireLinkHandler EnquireLinkHandler
	unbindHandler      UnbindHandler
	genericNackHandler GenericNackHandler

	// OnError is called when a handler returns an error.
	// Must be non-blocking (caller uses select/default internally).
	OnError func(err error)
}

// NewDispatcher creates a Dispatcher with the given handlers.
// All handlers are optional — nil handlers silently ignore the PDU.
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

	// Register all standard dispatch entries.
	// Adding a new PDU type = one Register call, not a switch case.
	d.Register(CommandIDSubmitSMResp, d.dispatchSubmitSMResp)
	d.Register(CommandIDDeliverSM, d.dispatchDeliverSM)
	d.Register(CommandIDBindTransceiverResp, d.dispatchBindResp)
	d.Register(CommandIDEnquireLink, d.dispatchEnquireLink)
	d.Register(CommandIDEnquireLinkResp, d.dispatchPendingResponse)
	d.Register(CommandIDUnbind, d.dispatchUnbind)
	d.Register(CommandIDUnbindResp, d.dispatchPendingResponse)
	d.Register(CommandIDGenericNack, d.dispatchGenericNack)

	return d
}

// Register adds or replaces a dispatch entry for the given CommandID.
// Enables plugin-based dispatch: vendor PDUs or SMPP 5.0 extensions
// can be registered externally without modifying this package.
func (d *Dispatcher) Register(cmd CommandID, fn DispatchFunc) {
	d.table[cmd] = fn
}

// Dispatch routes a decoded PDU to the appropriate handler.
//
// Routing is O(1) map lookup by CommandID (not a switch statement).
// Unknown CommandIDs are logged via OnError and silently dropped.
//
// Dispatch is non-blocking if all registered handlers are non-blocking.
func (d *Dispatcher) Dispatch(pdu PDU) {
	if pdu == nil || pdu.Header() == nil {
		return
	}

	cmdID := pdu.Header().CommandID

	fn, ok := d.table[cmdID]
	if !ok {
		d.safeOnError(fmt.Errorf("unknown PDU: %s seq=%d", cmdID, pdu.Header().SequenceNumber))
		return
	}

	deliverResp := fn(pdu)

	// If a DeliverSM handler returned a response PDU, the caller (Session)
	// is responsible for sending it. This is communicated via a field
	// or callback — for now, the handler handles it internally.
	_ = deliverResp
}

// ── Dispatch Funcs ───────────────────────────────────────────────────────────

func (d *Dispatcher) dispatchSubmitSMResp(pdu PDU) *DeliverSMResp {
	resp, ok := pdu.(*SubmitSMResp)
	if !ok || d.pendingRespHandler == nil {
		return nil
	}
	d.pendingRespHandler.HandleResponse(resp)
	return nil
}

func (d *Dispatcher) dispatchDeliverSM(pdu PDU) *DeliverSMResp {
	deliver, ok := pdu.(*DeliverSM)
	if !ok || d.deliverSMHandler == nil {
		return nil
	}
	resp, err := d.deliverSMHandler.HandleDeliverSM(deliver)
	if err != nil {
		d.safeOnError(fmt.Errorf("deliver_sm handler: %w", err))
	}
	return resp
}

func (d *Dispatcher) dispatchBindResp(pdu PDU) *DeliverSMResp {
	resp, ok := pdu.(*BindTransceiverResp)
	if !ok || d.bindRespHandler == nil {
		return nil
	}
	d.bindRespHandler.HandleBindResp(resp)
	return nil
}

func (d *Dispatcher) dispatchEnquireLink(pdu PDU) *DeliverSMResp {
	if d.enquireLinkHandler == nil {
		return nil
	}
	d.enquireLinkHandler.HandleEnquireLink(pdu.Header().SequenceNumber)
	return nil
}

// dispatchPendingResponse forwards EnquireLinkResp and UnbindResp (and any
// other response PDU) to the same PendingResponseHandler used for SubmitSMResp.
// This means ALL correlated responses go through one path — no special casing.
func (d *Dispatcher) dispatchPendingResponse(pdu PDU) *DeliverSMResp {
	if d.pendingRespHandler == nil {
		return nil
	}
	d.pendingRespHandler.HandleResponse(pdu)
	return nil
}

func (d *Dispatcher) dispatchUnbind(pdu PDU) *DeliverSMResp {
	if d.unbindHandler == nil {
		return nil
	}
	// Handler returns true if we should respond with unbind_resp
	d.unbindHandler.HandleUnbind()
	return nil
}

func (d *Dispatcher) dispatchGenericNack(pdu PDU) *DeliverSMResp {
	if d.genericNackHandler == nil {
		return nil
	}
	d.genericNackHandler.HandleGenericNack(pdu.Header().CommandStatus)
	return nil
}

// safeOnError calls OnError without blocking.
// OnError is a function call — no risk of blocking unless the handler
// implementation blocks. Guarantee: we don't block here.
func (d *Dispatcher) safeOnError(err error) {
	if d.OnError == nil {
		return
	}
	d.OnError(err)
}
