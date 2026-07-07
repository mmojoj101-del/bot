package smpp

import (
	"fmt"
)

// ── Handler Interfaces ───────────────────────────────────────────────────────

// SubmitSMRespHandler handles incoming submit_sm_resp PDUs.
// The implementation typically calls PendingStore.Notify(seq, resp).
type SubmitSMRespHandler interface {
	HandleSubmitSMResp(resp *SubmitSMResp)
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

// EnquireLinkHandler handles incoming enquire_link PDUs.
// The implementation typically sends an EnquireLinkResp on the transport.
type EnquireLinkHandler interface {
	HandleEnquireLink(seq uint32)
}

// UnbindHandler handles incoming unbind PDUs.
// Returns true if the session should send an unbind_resp back.
type UnbindHandler interface {
	HandleUnbind() (sendResp bool)
}

// GenericNackHandler handles incoming generic_nack PDUs.
type GenericNackHandler interface {
	HandleGenericNack(status CommandStatus)
}

// ── Dispatcher ───────────────────────────────────────────────────────────────

// Dispatcher routes decoded PDUs to the appropriate handler.
//
// It is a pure Router — it knows NOTHING about:
//   - Session state
//   - WindowManager internals
//   - PendingStore (except via the PendingNotifier interface)
//   - Transport
//
// This makes it independently testable with mock handlers.
type Dispatcher struct {
	submitSMRespHandler SubmitSMRespHandler
	deliverSMHandler    DeliverSMHandler
	bindRespHandler     BindRespHandler
	enquireLinkHandler  EnquireLinkHandler
	unbindHandler       UnbindHandler
	genericNackHandler  GenericNackHandler

	// OnError is called when a handler returns an error.
	// Can be used for logging or session shutdown.
	OnError func(err error)
}

// NewDispatcher creates a Dispatcher with the given handlers.
// All handlers are optional — nil handlers silently ignore the PDU.
func NewDispatcher(
	submitSMResp SubmitSMRespHandler,
	deliverSM DeliverSMHandler,
	bindResp BindRespHandler,
	enquireLink EnquireLinkHandler,
	unbind UnbindHandler,
	genericNack GenericNackHandler,
) *Dispatcher {
	return &Dispatcher{
		submitSMRespHandler: submitSMResp,
		deliverSMHandler:    deliverSM,
		bindRespHandler:     bindResp,
		enquireLinkHandler:  enquireLink,
		unbindHandler:       unbind,
		genericNackHandler:  genericNack,
	}
}

// Dispatch routes a decoded PDU to the appropriate handler.
//
// The routing decision is based solely on CommandID:
//   - SubmitSMResp → SubmitSMRespHandler
//   - DeliverSM → DeliverSMHandler
//   - BindTransceiverResp → BindRespHandler
//   - EnquireLink → EnquireLinkHandler
//   - EnquireLinkResp → silently consumed (our heartbeat)
//   - Unbind → UnbindHandler
//   - UnbindResp → silently consumed (our disconnect)
//   - GenericNack → GenericNackHandler
//   - Unknown → logged via OnError
//
// Dispatch is safe to call from a single goroutine (the Reader).
// It does not block on I/O — handler implementations may call Notify
// which is a channel send, or enqueue work.
func (d *Dispatcher) Dispatch(pdu PDU) {
	if pdu == nil || pdu.Header() == nil {
		return
	}

	cmdID := pdu.Header().CommandID
	seq := pdu.Header().SequenceNumber

	switch typed := pdu.(type) {
	case *SubmitSMResp:
		if d.submitSMRespHandler != nil {
			d.submitSMRespHandler.HandleSubmitSMResp(typed)
		}

	case *DeliverSM:
		if d.deliverSMHandler == nil {
			return
		}
		resp, err := d.deliverSMHandler.HandleDeliverSM(typed)
		if err != nil && d.OnError != nil {
			d.OnError(fmt.Errorf("deliver_sm handler: %w", err))
		}
		_ = resp // The caller (Session) is responsible for sending the response
		// TODO: in Session, the response PDU will be queued for sending

	case *BindTransceiverResp:
		if d.bindRespHandler != nil {
			d.bindRespHandler.HandleBindResp(typed)
		}

	case *EnquireLink:
		if d.enquireLinkHandler != nil {
			d.enquireLinkHandler.HandleEnquireLink(seq)
		}

	case *EnquireLinkResp:
		// Our own heartbeat response — already handled by WindowManager/Notify
		// This arrives through the same Dispatch path but should be forwarded
		// to the PendingStore. The SubmitSMRespHandler handles it since the
		// response was registered as a pending request with matching seq.
		if d.submitSMRespHandler != nil {
			// Wrap as a generic response for correlation
			d.submitSMRespHandler.HandleSubmitSMResp(&SubmitSMResp{
				Hdr: *typed.Header(),
			})
		}

	case *Unbind:
		if d.unbindHandler != nil {
			d.unbindHandler.HandleUnbind()
		}

	case *UnbindResp:
		// Our own unbind response — already handled via WindowManager/Notify
		if d.submitSMRespHandler != nil {
			d.submitSMRespHandler.HandleSubmitSMResp(&SubmitSMResp{
				Hdr: *typed.Header(),
			})
		}

	case *GenericPDU:
		if pdu.Header().CommandID == CommandIDGenericNack {
			if d.genericNackHandler != nil {
				d.genericNackHandler.HandleGenericNack(pdu.Header().CommandStatus)
			}
		} else {
			if d.OnError != nil {
				d.OnError(fmt.Errorf("unknown PDU: %s seq=%d", cmdID, seq))
			}
		}

	default:
		if d.OnError != nil {
			d.OnError(fmt.Errorf("unhandled PDU type: %T cmd=%s seq=%d", pdu, cmdID, seq))
		}
	}
}
