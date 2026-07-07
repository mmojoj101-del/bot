package smpp

import (
	"errors"
	"sync/atomic"
	"testing"
)

// ── Mock Handlers ────────────────────────────────────────────────────────────

type mockPendingRespHandler struct {
	calls atomic.Int32
	last  PDU
}

func (m *mockPendingRespHandler) HandleResponse(resp PDU) {
	m.calls.Add(1)
	m.last = resp
}

type mockDeliverSMHandler struct {
	calls       atomic.Int32
	returnResp  *DeliverSMResp
	returnError error
}

func (m *mockDeliverSMHandler) HandleDeliverSM(pdu *DeliverSM) (*DeliverSMResp, error) {
	m.calls.Add(1)
	return m.returnResp, m.returnError
}

type mockBindRespHandler struct {
	calls atomic.Int32
	last  *BindTransceiverResp
}

func (m *mockBindRespHandler) HandleBindResp(resp *BindTransceiverResp) {
	m.calls.Add(1)
	m.last = resp
}

type mockEnquireLinkHandler struct {
	calls atomic.Int32
	last  uint32
}

func (m *mockEnquireLinkHandler) HandleEnquireLink(seq uint32) {
	m.calls.Add(1)
	m.last = seq
}

type mockUnbindHandler struct {
	calls     atomic.Int32
	returnVal bool
}

func (m *mockUnbindHandler) HandleUnbind() (sendResp bool) {
	m.calls.Add(1)
	return m.returnVal
}

type mockGenericNackHandler struct {
	calls  atomic.Int32
	status CommandStatus
}

func (m *mockGenericNackHandler) HandleGenericNack(status CommandStatus) {
	m.calls.Add(1)
	m.status = status
}

// ── Test Dispatcher Dispatch ─────────────────────────────────────────────────

func TestDispatcher_SubmitSMResp(t *testing.T) {
	pending := &mockPendingRespHandler{}
	d := NewDispatcher(pending, nil, nil, nil, nil, nil)

	pdu := &SubmitSMResp{
		Hdr:       Header{CommandID: CommandIDSubmitSMResp, SequenceNumber: 42},
		MessageID: "test-msg-123",
	}
	d.Dispatch(pdu)

	if n := pending.calls.Load(); n != 1 {
		t.Errorf("expected 1 call, got %d", n)
	}
	last := pending.last.(*SubmitSMResp)
	if last.MessageID != "test-msg-123" {
		t.Errorf("expected message_id 'test-msg-123', got '%s'", last.MessageID)
	}
	if last.Header().SequenceNumber != 42 {
		t.Errorf("expected seq 42, got %d", last.Header().SequenceNumber)
	}
}

func TestDispatcher_SubmitSMResp_NilHandler(t *testing.T) {
	// No pending handler — should silently ignore
	d := NewDispatcher(nil, nil, nil, nil, nil, nil)
	pdu := &SubmitSMResp{Hdr: Header{CommandID: CommandIDSubmitSMResp}}
	d.Dispatch(pdu) // must not panic
}

func TestDispatcher_DeliverSM(t *testing.T) {
	deliver := &mockDeliverSMHandler{returnResp: &DeliverSMResp{MessageID: "resp-1"}}
	d := NewDispatcher(nil, deliver, nil, nil, nil, nil)

	pdu := &DeliverSM{
		Hdr:          Header{CommandID: CommandIDDeliverSM, SequenceNumber: 7},
		SourceAddr:   "12345",
		ShortMessage: []byte("hello"),
	}
	d.Dispatch(pdu)

	if n := deliver.calls.Load(); n != 1 {
		t.Errorf("expected 1 call, got %d", n)
	}
}

func TestDispatcher_DeliverSM_HandlerError(t *testing.T) {
	var errCount atomic.Int32
	deliver := &mockDeliverSMHandler{returnError: errors.New("handler failed")}
	d := NewDispatcher(nil, deliver, nil, nil, nil, nil)
	d.OnError = func(err error) { errCount.Add(1) }

	pdu := &DeliverSM{Hdr: Header{CommandID: CommandIDDeliverSM}}
	d.Dispatch(pdu)

	if n := errCount.Load(); n != 1 {
		t.Errorf("expected 1 error callback, got %d", n)
	}
}

func TestDispatcher_DeliverSM_NilHandler(t *testing.T) {
	d := NewDispatcher(nil, nil, nil, nil, nil, nil)
	pdu := &DeliverSM{Hdr: Header{CommandID: CommandIDDeliverSM}}
	d.Dispatch(pdu) // must not panic
}

func TestDispatcher_BindTransceiverResp(t *testing.T) {
	bind := &mockBindRespHandler{}
	d := NewDispatcher(nil, nil, bind, nil, nil, nil)

	pdu := &BindTransceiverResp{
		Hdr:      Header{CommandID: CommandIDBindTransceiverResp, SequenceNumber: 1},
		SystemID: "smsc-01",
	}
	d.Dispatch(pdu)

	if n := bind.calls.Load(); n != 1 {
		t.Errorf("expected 1 call, got %d", n)
	}
	if bind.last.SystemID != "smsc-01" {
		t.Errorf("expected system_id 'smsc-01', got '%s'", bind.last.SystemID)
	}
}

func TestDispatcher_EnquireLink(t *testing.T) {
	enq := &mockEnquireLinkHandler{}
	d := NewDispatcher(nil, nil, nil, enq, nil, nil)

	pdu := &EnquireLink{Hdr: Header{CommandID: CommandIDEnquireLink, SequenceNumber: 5}}
	d.Dispatch(pdu)

	if n := enq.calls.Load(); n != 1 {
		t.Errorf("expected 1 call, got %d", n)
	}
	if enq.last != 5 {
		t.Errorf("expected seq 5, got %d", enq.last)
	}
}

func TestDispatcher_EnquireLinkResp(t *testing.T) {
	// EnquireLinkResp should be forwarded to PendingResponseHandler
	pending := &mockPendingRespHandler{}
	d := NewDispatcher(pending, nil, nil, nil, nil, nil)

	pdu := &EnquireLinkResp{Hdr: Header{CommandID: CommandIDEnquireLinkResp, SequenceNumber: 5}}
	d.Dispatch(pdu)

	if n := pending.calls.Load(); n != 1 {
		t.Errorf("expected 1 call to pending handler, got %d", n)
	}
}

func TestDispatcher_UnbindResp(t *testing.T) {
	// UnbindResp should be forwarded to PendingResponseHandler
	pending := &mockPendingRespHandler{}
	d := NewDispatcher(pending, nil, nil, nil, nil, nil)

	pdu := &UnbindResp{Hdr: Header{CommandID: CommandIDUnbindResp, SequenceNumber: 99}}
	d.Dispatch(pdu)

	if n := pending.calls.Load(); n != 1 {
		t.Errorf("expected 1 call to pending handler, got %d", n)
	}
}

func TestDispatcher_Unbind(t *testing.T) {
	unb := &mockUnbindHandler{returnVal: true}
	d := NewDispatcher(nil, nil, nil, nil, unb, nil)

	pdu := &Unbind{Hdr: Header{CommandID: CommandIDUnbind, SequenceNumber: 10}}
	d.Dispatch(pdu)

	if n := unb.calls.Load(); n != 1 {
		t.Errorf("expected 1 call, got %d", n)
	}
}

func TestDispatcher_GenericNack(t *testing.T) {
	nack := &mockGenericNackHandler{}
	d := NewDispatcher(nil, nil, nil, nil, nil, nack)

	pdu := &GenericPDU{
		Hdr: Header{CommandID: CommandIDGenericNack, CommandStatus: StatusSysFail, SequenceNumber: 3},
	}
	d.Dispatch(pdu)

	if n := nack.calls.Load(); n != 1 {
		t.Errorf("expected 1 call, got %d", n)
	}
	if nack.status != StatusSysFail {
		t.Errorf("expected status SysFail, got %v", nack.status)
	}
}

func TestDispatcher_UnknownCommandID(t *testing.T) {
	var errCount atomic.Int32
	d := NewDispatcher(nil, nil, nil, nil, nil, nil)
	d.OnError = func(err error) { errCount.Add(1) }

	// GenericPDU with an unknown command ID (not GenericNack)
	pdu := &GenericPDU{
		Hdr: Header{CommandID: 0xDEADBEEF, SequenceNumber: 1},
	}
	d.Dispatch(pdu)

	if n := errCount.Load(); n != 1 {
		t.Errorf("expected 1 error callback for unknown command, got %d", n)
	}
}

func TestDispatcher_NilPDU(t *testing.T) {
	d := NewDispatcher(nil, nil, nil, nil, nil, nil)
	d.Dispatch(nil) // must not panic
	d.Dispatch(&GenericPDU{}) // nil header? Actually GenericPDU has embedded Hdr, let's test with PDU that returns nil
}

func TestDispatcher_MultiplePDUs(t *testing.T) {
	pending := &mockPendingRespHandler{}
	deliver := &mockDeliverSMHandler{returnResp: &DeliverSMResp{}}
	bind := &mockBindRespHandler{}
	d := NewDispatcher(pending, deliver, bind, nil, nil, nil)

	// Dispatch multiple PDUs in sequence (as Reader would)
	inputs := []PDU{
		&SubmitSMResp{Hdr: Header{CommandID: CommandIDSubmitSMResp, SequenceNumber: 1}, MessageID: "m1"},
		&DeliverSM{Hdr: Header{CommandID: CommandIDDeliverSM, SequenceNumber: 2}, SourceAddr: "src1"},
		&BindTransceiverResp{Hdr: Header{CommandID: CommandIDBindTransceiverResp, SequenceNumber: 3}, SystemID: "smsc"},
		&SubmitSMResp{Hdr: Header{CommandID: CommandIDSubmitSMResp, SequenceNumber: 4}, MessageID: "m2"},
	}

	for _, pdu := range inputs {
		d.Dispatch(pdu)
	}

	if n := pending.calls.Load(); n != 2 {
		t.Errorf("expected 2 pending handler calls, got %d", n)
	}
	if n := deliver.calls.Load(); n != 1 {
		t.Errorf("expected 1 deliver handler call, got %d", n)
	}
	if n := bind.calls.Load(); n != 1 {
		t.Errorf("expected 1 bind handler call, got %d", n)
	}
}

// ── Test Dispatcher Registration Extensibility ───────────────────────────────

func TestDispatcher_CustomRegistration(t *testing.T) {
	var customCalls atomic.Int32
	d := NewDispatcher(nil, nil, nil, nil, nil, nil)

	// Register a handler for a custom/vendor command ID
	customCmd := CommandID(0x00000FFF)
	d.Register(customCmd, func(pdu PDU) *DeliverSMResp {
		customCalls.Add(1)
		return nil
	})

	pdu := &GenericPDU{Hdr: Header{CommandID: customCmd, SequenceNumber: 1}}
	d.Dispatch(pdu)

	if n := customCalls.Load(); n != 1 {
		t.Errorf("expected 1 custom handler call, got %d", n)
	}
}
