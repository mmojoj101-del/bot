package smpp

import (
	"os"
	"path/filepath"
	"testing"
)

// GenerateTestdata creates real PDU binary files in testdata/ by encoding
// known-good PDUs. These files serve as fixtures for TestDecodeRealPDU_*
// tests and catch encoding/decoding drift.
//
// Run with: go test -run TestGenerateTestdata ./internal/connector/driver/smpp/
// Regenerates all .bin files when PDU structures change.
func TestGenerateTestdata(t *testing.T) {
	dir := filepath.Join("testdata")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir testdata: %v", err)
	}

	cases := []struct {
		name string
		pdu  PDU
	}{
		{
			name: "enquire_link.bin",
			pdu:  &EnquireLink{Hdr: Header{CommandID: CommandIDEnquireLink, SequenceNumber: 1}},
		},
		{
			name: "enquire_link_resp.bin",
			pdu:  &EnquireLinkResp{Hdr: Header{CommandID: CommandIDEnquireLinkResp, SequenceNumber: 1}},
		},
		{
			name: "bind_transceiver.bin",
			pdu: &BindTransceiver{
				Hdr:              Header{CommandID: CommandIDBindTransceiver, SequenceNumber: 1},
				SystemID:         "test-system",
				Password:         "test-secret",
				SystemType:       "vms",
				InterfaceVersion: 0x34,
				AddrTON:          1,
				AddrNPI:          1,
			},
		},
		{
			name: "bind_transceiver_resp.bin",
			pdu: &BindTransceiverResp{
				Hdr:      Header{CommandID: CommandIDBindTransceiverResp, SequenceNumber: 1, CommandStatus: StatusOK},
				SystemID: "smsc-gateway",
			},
		},
		{
			name: "submit_sm.bin",
			pdu: &SubmitSM{
				Hdr:                Header{CommandID: CommandIDSubmitSM, SequenceNumber: 100},
				SourceAddr:         "12345",
				DestinationAddr:    "+201234567890",
				RegisteredDelivery: 1,
				DataCoding:         0,
				ShortMessage:       []byte("Hello from SMPP test"),
			},
		},
		{
			name: "submit_sm_resp.bin",
			pdu: &SubmitSMResp{
				Hdr:       Header{CommandID: CommandIDSubmitSMResp, SequenceNumber: 100, CommandStatus: StatusOK},
				MessageID: "SMSC-00001-ABC",
			},
		},
		{
			name: "deliver_sm.bin",
			pdu: &DeliverSM{
				Hdr:                Header{CommandID: CommandIDDeliverSM, SequenceNumber: 200},
				SourceAddr:         "SMSC",
				DestinationAddr:    "12345",
				ESMClass:           4, // delivery receipt
				ShortMessage:       []byte("id:SMSC-00001-ABC sub:001 dlvrd:001 submit date:2401010000 done date:2401010000 stat:DELIVRD err:000 text:Hello"),
			},
		},
		{
			name: "unbind.bin",
			pdu:  &Unbind{Hdr: Header{CommandID: CommandIDUnbind, SequenceNumber: 2}},
		},
		{
			name: "unbind_resp.bin",
			pdu:  &UnbindResp{Hdr: Header{CommandID: CommandIDUnbindResp, SequenceNumber: 2}},
		},
	}

	codec := NewCodec(Version3_4)
	for _, c := range cases {
		data, err := codec.Encode(c.pdu)
		if err != nil {
			t.Fatalf("encode %s: %v", c.name, err)
		}
		path := filepath.Join(dir, c.name)
		if err := os.WriteFile(path, data, 0644); err != nil {
			t.Fatalf("write %s: %v", c.name, err)
		}
		t.Logf("generated %s (%d bytes)", path, len(data))
	}
}
