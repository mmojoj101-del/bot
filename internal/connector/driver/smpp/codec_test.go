package smpp

import (
	"bytes"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func codec() *Codec { return NewCodec(Version3_4) }

// ── Header ──────────────────────────────────────────────────────────────────

func TestDecodeHeader(t *testing.T) {
	data := []byte{
		0x00, 0x00, 0x00, 0x10, // length = 16
		0x00, 0x00, 0x00, 0x15, // command_id = enquire_link
		0x00, 0x00, 0x00, 0x00, // command_status = OK
		0x00, 0x00, 0x00, 0x01, // sequence_number = 1
	}
	hdr := decodeHeader(data)
	if hdr.Length != 16 {
		t.Errorf("Length = %d, want 16", hdr.Length)
	}
	if hdr.CommandID != CommandIDEnquireLink {
		t.Errorf("CommandID = %v, want enquire_link", hdr.CommandID)
	}
	if hdr.CommandStatus != StatusOK {
		t.Errorf("CommandStatus = %v, want OK", hdr.CommandStatus)
	}
	if hdr.SequenceNumber != 1 {
		t.Errorf("SequenceNumber = %d, want 1", hdr.SequenceNumber)
	}
}

func TestEncodeDecodeHeader(t *testing.T) {
	original := &EnquireLink{Hdr: Header{
		CommandID:      CommandIDEnquireLink,
		CommandStatus:  StatusOK,
		SequenceNumber: 42,
	}}
	codec := codec()
	data, err := codec.Encode(original)
	if err != nil {
		t.Fatalf("Encode error: %v", err)
	}
	decoded, err := codec.Decode(data)
	if err != nil {
		t.Fatalf("Decode error: %v", err)
	}
	if decoded.Header().CommandID != CommandIDEnquireLink {
		t.Errorf("decoded command_id = %v", decoded.Header().CommandID)
	}
	if decoded.Header().SequenceNumber != 42 {
		t.Errorf("decoded seq = %d, want 42", decoded.Header().SequenceNumber)
	}
}

// ── Round-Trip Tests ────────────────────────────────────────────────────────

func roundTrip(t *testing.T, original PDU) {
	t.Helper()
	codec := codec()
	data, err := codec.Encode(original)
	if err != nil {
		t.Fatalf("Encode error: %v", err)
	}
	decoded, err := codec.Decode(data)
	if err != nil {
		t.Fatalf("Decode error: %v", err)
	}
	reData, err := codec.Encode(decoded)
	if err != nil {
		t.Fatalf("Re-encode error: %v", err)
	}
	if !bytes.Equal(data, reData) {
		t.Errorf("round-trip bytes differ\noriginal: %s\nre-enc:   %s",
			hex.EncodeToString(data), hex.EncodeToString(reData))
	}
}

func TestEnquireLinkRoundTrip(t *testing.T) {
	roundTrip(t, &EnquireLink{
		Hdr: Header{CommandID: CommandIDEnquireLink, SequenceNumber: 1, CommandStatus: StatusOK},
	})
}

func TestEnquireLinkRespRoundTrip(t *testing.T) {
	roundTrip(t, &EnquireLinkResp{
		Hdr: Header{CommandID: CommandIDEnquireLinkResp, SequenceNumber: 1, CommandStatus: StatusOK},
	})
}

func TestUnbindRoundTrip(t *testing.T) {
	roundTrip(t, &Unbind{
		Hdr: Header{CommandID: CommandIDUnbind, SequenceNumber: 5},
	})
}

func TestUnbindRespRoundTrip(t *testing.T) {
	roundTrip(t, &UnbindResp{
		Hdr: Header{CommandID: CommandIDUnbindResp, SequenceNumber: 5},
	})
}

func TestBindTransceiverRoundTrip(t *testing.T) {
	roundTrip(t, &BindTransceiver{
		Hdr:              Header{CommandID: CommandIDBindTransceiver, SequenceNumber: 1},
		SystemID:          "test-system",
		Password:          "secret",
		SystemType:        "vms",
		InterfaceVersion:  0x34,
		AddrTON:           1,
		AddrNPI:           1,
		AddressRange:      "",
	})
}

func TestBindTransceiverRespRoundTrip(t *testing.T) {
	roundTrip(t, &BindTransceiverResp{
		Hdr:      Header{CommandID: CommandIDBindTransceiverResp, SequenceNumber: 1, CommandStatus: StatusOK},
		SystemID: "smsc-gateway",
	})
}

func TestSubmitSMRoundTrip(t *testing.T) {
	roundTrip(t, &SubmitSM{
		Hdr:                Header{CommandID: CommandIDSubmitSM, SequenceNumber: 100},
		ServiceType:        "",
		SourceAddrTON:      1,
		SourceAddrNPI:      1,
		SourceAddr:         "12345",
		DestAddrTON:        1,
		DestAddrNPI:        1,
		DestinationAddr:    "+201234567890",
		ESMClass:           0,
		ProtocolID:         0,
		PriorityFlag:       0,
		ScheduleDelivery:   "",
		ValidityPeriod:     "",
		RegisteredDelivery: 1,
		ReplaceIfPresent:   0,
		DataCoding:         0,
		SMDefaultMsgID:     0,
		ShortMessage:       []byte("Hello, World! مرحباً"),
	})
}

func TestSubmitSMRespRoundTrip(t *testing.T) {
	roundTrip(t, &SubmitSMResp{
		Hdr:       Header{CommandID: CommandIDSubmitSMResp, SequenceNumber: 100, CommandStatus: StatusOK},
		MessageID: "SMSC-12345-ABC",
	})
}

func TestDeliverSMRoundTrip(t *testing.T) {
	roundTrip(t, &DeliverSM{
		Hdr:                Header{CommandID: CommandIDDeliverSM, SequenceNumber: 200},
		ServiceType:        "",
		SourceAddrTON:      1,
		SourceAddrNPI:      1,
		SourceAddr:         "SMSC",
		DestAddrTON:        1,
		DestAddrNPI:        1,
		DestinationAddr:    "12345",
		ESMClass:           4, // delivery receipt
		ProtocolID:         0,
		PriorityFlag:       0,
		ScheduleDelivery:   "",
		ValidityPeriod:     "",
		RegisteredDelivery: 0,
		ReplaceIfPresent:   0,
		DataCoding:         0,
		SMDefaultMsgID:     0,
		ShortMessage:       []byte("id:SMSC-12345 sub:001 dlvrd:001 submit date:2001010100 done date:2001010100 stat:DELIVRD err:000 text:Hello"),
	})
}

func TestDeliverSMRespRoundTrip(t *testing.T) {
	roundTrip(t, &DeliverSMResp{
		Hdr:       Header{CommandID: CommandIDDeliverSMResp, SequenceNumber: 200, CommandStatus: StatusOK},
		MessageID: "",
	})
}

// ── SubmitSM with TLV ───────────────────────────────────────────────────────

func TestSubmitSMWithTLVRoundTrip(t *testing.T) {
	pdu := &SubmitSM{
		Hdr:                Header{CommandID: CommandIDSubmitSM, SequenceNumber: 101},
		SourceAddr:         "12345",
		DestinationAddr:    "+201234567890",
		RegisteredDelivery: 1,
		DataCoding:         8, // UCS-2
		ShortMessage:       []byte("Long message payload that exceeds 140 bytes and needs message_payload TLV for proper handling in SMPP protocol"),
	}
	pdu.AddTLV(TLVTagMessagePayload, []byte("additional payload data"))
	pdu.AddTLV(TLVTagUserMessageReference, []byte{0x01})

	roundTrip(t, pdu)
}

// ── Binary Test Data ────────────────────────────────────────────────────────

func testdataFile(t *testing.T, name string) []byte {
	t.Helper()
	path := filepath.Join("testdata", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("testdata file not found: %s (%v)", path, err)
	}
	return data
}

func TestDecodeRealPDU_EnquireLink(t *testing.T) {
	data := testdataFile(t, "enquire_link.bin")
	if data == nil {
		return
	}
	pdu, err := codec().Decode(data)
	if err != nil {
		t.Fatalf("Decode error: %v", err)
	}
	if pdu.Header().CommandID != CommandIDEnquireLink {
		t.Errorf("CommandID = %v, want enquire_link", pdu.Header().CommandID)
	}
	reEncoded, err := codec().Encode(pdu)
	if err != nil {
		t.Fatalf("Re-encode error: %v", err)
	}
	if !bytes.Equal(data, reEncoded) {
		t.Error("re-encoded PDU does not match original")
	}
}

func TestDecodeRealPDU_SubmitSM(t *testing.T) {
	data := testdataFile(t, "submit_sm.bin")
	if data == nil {
		return
	}
	pdu, err := codec().Decode(data)
	if err != nil {
		t.Fatalf("Decode error: %v", err)
	}
	if pdu.Header().CommandID != CommandIDSubmitSM {
		t.Errorf("CommandID = %v, want submit_sm", pdu.Header().CommandID)
	}
	reEncoded, err := codec().Encode(pdu)
	if err != nil {
		t.Fatalf("Re-encode error: %v", err)
	}
	if !bytes.Equal(data, reEncoded) {
		t.Error("re-encoded PDU does not match original")
	}
}

func TestDecodeRealPDU_SubmitSMResp(t *testing.T) {
	data := testdataFile(t, "submit_sm_resp.bin")
	if data == nil {
		return
	}
	pdu, err := codec().Decode(data)
	if err != nil {
		t.Fatalf("Decode error: %v", err)
	}
	if pdu.Header().CommandID != CommandIDSubmitSMResp {
		t.Errorf("CommandID = %v, want submit_sm_resp", pdu.Header().CommandID)
	}
	reEncoded, err := codec().Encode(pdu)
	if err != nil {
		t.Fatalf("Re-encode error: %v", err)
	}
	if !bytes.Equal(data, reEncoded) {
		t.Error("re-encoded PDU does not match original")
	}
}

// ── Error Cases ──────────────────────────────────────────────────────────────

func TestDecode_ShortHeader(t *testing.T) {
	_, err := codec().Decode([]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0})
	if !errors.Is(err, ErrShortHeader) {
		t.Errorf("expected ErrShortHeader, got %v", err)
	}
}

func TestDecode_TruncatedBody(t *testing.T) {
	// length=20 but only 16 bytes provided
	data := []byte{
		0x00, 0x00, 0x00, 0x14, // length = 20
		0x00, 0x00, 0x00, 0x15, // command_id = enquire_link
		0x00, 0x00, 0x00, 0x00, // command_status = OK
		0x00, 0x00, 0x00, 0x01, // sequence_number = 1
	}
	_, err := codec().Decode(data)
	if !errors.Is(err, ErrTruncatedBody) {
		t.Errorf("expected ErrTruncatedBody, got %v", err)
	}
}

func TestDecode_UnknownCommand(t *testing.T) {
	data := []byte{
		0x00, 0x00, 0x00, 0x10, // length = 16
		0xFF, 0xFF, 0xFF, 0xFF, // unknown command_id
		0x00, 0x00, 0x00, 0x00, // command_status
		0x00, 0x00, 0x00, 0x01, // seq
	}
	pdu, err := codec().Decode(data)
	if err != nil {
		t.Fatalf("Decode unknown command should succeed as GenericPDU, got: %v", err)
	}
	if _, ok := pdu.(*GenericPDU); !ok {
		t.Errorf("expected *GenericPDU, got %T", pdu)
	}
}

func TestDecode_EmptyCString(t *testing.T) {
	s, n, err := decodeCString([]byte{0})
	if err != nil {
		t.Fatalf("decodeCString error: %v", err)
	}
	if s != "" {
		t.Errorf("string = %q, want empty", s)
	}
	if n != 1 {
		t.Errorf("consumed = %d, want 1", n)
	}
}

func TestDecodeCString_NoNull(t *testing.T) {
	_, _, err := decodeCString([]byte("hello"))
	if !errors.Is(err, ErrInvalidCString) {
		t.Errorf("expected ErrInvalidCString, got %v", err)
	}
}
