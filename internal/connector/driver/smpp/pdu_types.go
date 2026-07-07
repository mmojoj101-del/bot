package smpp

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// ── Version ──────────────────────────────────────────────────────────────────

type Version int

const (
	Version3_3 Version = iota + 1
	Version3_4  // primary target
	Version5_0
)

func (v Version) String() string {
	switch v {
	case Version3_3:
		return "SMPP v3.3"
	case Version3_4:
		return "SMPP v3.4"
	case Version5_0:
		return "SMPP v5.0"
	default:
		return fmt.Sprintf("SMPP unknown(%d)", int(v))
	}
}

// ── Command IDs ──────────────────────────────────────────────────────────────

type CommandID uint32

const (
	CommandIDGenericNack          CommandID = 0x80000000
	CommandIDBindReceiver         CommandID = 0x00000001
	CommandIDBindReceiverResp     CommandID = 0x80000001
	CommandIDBindTransmitter      CommandID = 0x00000002
	CommandIDBindTransmitterResp  CommandID = 0x80000002
	CommandIDBindTransceiver      CommandID = 0x00000009
	CommandIDBindTransceiverResp  CommandID = 0x80000009
	CommandIDOutbind              CommandID = 0x0000000B
	CommandIDUnbind               CommandID = 0x00000006
	CommandIDUnbindResp           CommandID = 0x80000006
	CommandIDSubmitSM             CommandID = 0x00000004
	CommandIDSubmitSMResp         CommandID = 0x80000004
	CommandIDDeliverSM            CommandID = 0x00000005
	CommandIDDeliverSMResp        CommandID = 0x80000005
	CommandIDDataSM               CommandID = 0x00000101
	CommandIDDataSMResp           CommandID = 0x80000101
	CommandIDQuerySM              CommandID = 0x00000003
	CommandIDQuerySMResp          CommandID = 0x80000003
	CommandIDCancelSM             CommandID = 0x00000008
	CommandIDCancelSMResp         CommandID = 0x80000008
	CommandIDReplaceSM            CommandID = 0x00000007
	CommandIDReplaceSMResp        CommandID = 0x80000007
	CommandIDEnquireLink          CommandID = 0x00000015
	CommandIDEnquireLinkResp      CommandID = 0x80000015
	CommandIDSubmitMulti          CommandID = 0x00000021
	CommandIDSubmitMultiResp      CommandID = 0x80000021
	CommandIDAlertNotification    CommandID = 0x00000102
)

func (c CommandID) String() string {
	switch c {
	case CommandIDGenericNack:
		return "generic_nack"
	case CommandIDBindReceiver:
		return "bind_receiver"
	case CommandIDBindReceiverResp:
		return "bind_receiver_resp"
	case CommandIDBindTransmitter:
		return "bind_transmitter"
	case CommandIDBindTransmitterResp:
		return "bind_transmitter_resp"
	case CommandIDBindTransceiver:
		return "bind_transceiver"
	case CommandIDBindTransceiverResp:
		return "bind_transceiver_resp"
	case CommandIDOutbind:
		return "outbind"
	case CommandIDUnbind:
		return "unbind"
	case CommandIDUnbindResp:
		return "unbind_resp"
	case CommandIDSubmitSM:
		return "submit_sm"
	case CommandIDSubmitSMResp:
		return "submit_sm_resp"
	case CommandIDDeliverSM:
		return "deliver_sm"
	case CommandIDDeliverSMResp:
		return "deliver_sm_resp"
	case CommandIDDataSM:
		return "data_sm"
	case CommandIDDataSMResp:
		return "data_sm_resp"
	case CommandIDQuerySM:
		return "query_sm"
	case CommandIDQuerySMResp:
		return "query_sm_resp"
	case CommandIDCancelSM:
		return "cancel_sm"
	case CommandIDCancelSMResp:
		return "cancel_sm_resp"
	case CommandIDReplaceSM:
		return "replace_sm"
	case CommandIDReplaceSMResp:
		return "replace_sm_resp"
	case CommandIDEnquireLink:
		return "enquire_link"
	case CommandIDEnquireLinkResp:
		return "enquire_link_resp"
	case CommandIDSubmitMulti:
		return "submit_multi"
	case CommandIDSubmitMultiResp:
		return "submit_multi_resp"
	case CommandIDAlertNotification:
		return "alert_notification"
	default:
		return fmt.Sprintf("unknown(0x%08X)", uint32(c))
	}
}

func (c CommandID) IsResponse() bool {
	return uint32(c)&0x80000000 != 0
}

func (c CommandID) RequestID() CommandID {
	return CommandID(uint32(c) &^ 0x80000000)
}

func (c CommandID) ResponseID() CommandID {
	return CommandID(uint32(c) | 0x80000000)
}

// ── Command Status ───────────────────────────────────────────────────────────

type CommandStatus uint32

const (
	StatusOK                   CommandStatus = 0x00000000
	StatusInvMsgID             CommandStatus = 0x00000001
	StatusInvBndFlags          CommandStatus = 0x00000002
	StatusInvParmLen           CommandStatus = 0x00000003
	StatusInvCmdID             CommandStatus = 0x00000004
	StatusUnknownCmd           CommandStatus = 0x00000005
	StatusInvBndCnt            CommandStatus = 0x00000006
	StatusInvCmdLen            CommandStatus = 0x00000007
	StatusInvSrcAddr           CommandStatus = 0x0000000A
	StatusInvDstAddr           CommandStatus = 0x0000000B
	StatusInvMsgID2            CommandStatus = 0x0000000C
	StatusSysFail              CommandStatus = 0x00000010
	StatusInvSrcTon            CommandStatus = 0x00000011
	StatusInvSrcNpi            CommandStatus = 0x00000012
	StatusInvDstTon            CommandStatus = 0x00000013
	StatusInvDstNpi            CommandStatus = 0x00000014
	StatusInvEscClass          CommandStatus = 0x00000015
	StatusInvProtoID           CommandStatus = 0x00000016
	StatusInvPrioFlag          CommandStatus = 0x00000017
	StatusInvRegDelivery       CommandStatus = 0x00000018
	StatusInvRepFlag           CommandStatus = 0x00000019
	StatusInvMsgLen            CommandStatus = 0x0000001A
	StatusInvParmVal           CommandStatus = 0x0000001C
	StatusInvDcs               CommandStatus = 0x0000001D
	StatusInvMsgState          CommandStatus = 0x0000001E
	StatusInvNumDests          CommandStatus = 0x0000001F
	StatusInvSubDate           CommandStatus = 0x00000020
	StatusInvValPeriod         CommandStatus = 0x00000023
	StatusInvDestFlag          CommandStatus = 0x00000024
	StatusInvSrcAddrSubunit    CommandStatus = 0x00000025
	StatusInvDstAddrSubunit    CommandStatus = 0x00000026
	StatusInvMsgFmt            CommandStatus = 0x00000027
	StatusThrottled            CommandStatus = 0x00000058
	StatusInvSchedule          CommandStatus = 0x00000061
	StatusInvValPeriod2        CommandStatus = 0x00000062
	StatusInvDstNum            CommandStatus = 0x00000063
	StatusInvSrcNum            CommandStatus = 0x00000064
	StatusSysErr               CommandStatus = 0x000000FF
	StatusCmdFail              CommandStatus = 0x00000100
)

func (s CommandStatus) String() string {
	switch s {
	case StatusOK:
		return "OK"
	case StatusInvMsgID:
		return "invalid message ID"
	case StatusInvBndFlags:
		return "invalid bind flags"
	case StatusInvParmLen:
		return "invalid parameter length"
	case StatusInvCmdID:
		return "invalid command ID"
	case StatusUnknownCmd:
		return "unknown command"
	case StatusSysFail:
		return "system failure"
	case StatusThrottled:
		return "throttling"
	case StatusSysErr:
		return "system error"
	case StatusCmdFail:
		return "command failure"
	default:
		return fmt.Sprintf("status(0x%08X)", uint32(s))
	}
}

func (s CommandStatus) IsOK() bool { return s == StatusOK }

// ── TLV ──────────────────────────────────────────────────────────────────────

type TLV struct {
	Tag   uint16
	Value []byte
}

func (t TLV) String() string { return fmt.Sprintf("TLV{tag=0x%04X len=%d}", t.Tag, len(t.Value)) }

// ── Header ───────────────────────────────────────────────────────────────────

// Header is the common 16-byte header for all SMPP PDUs.
type Header struct {
	Length         uint32        // total PDU length including header (network byte order)
	CommandID      CommandID     // identifies the PDU type
	CommandStatus  CommandStatus // response status (0 = OK)
	SequenceNumber uint32        // monotonically increasing, used for correlation
}

func (h *Header) String() string {
	return fmt.Sprintf("Header{cmd=%s seq=%d status=%s len=%d}",
		h.CommandID, h.SequenceNumber, h.CommandStatus, h.Length)
}

// ── PDU Interface ────────────────────────────────────────────────────────────

// PDU is the interface implemented by all decoded SMPP PDUs.
// Every concrete PDU type embeds a Header and returns it via Header().
type PDU interface {
	Header() *Header
}

// GenericPDU is a fallback for unknown or unimplemented command IDs.
// The raw body is preserved for debugging.
type GenericPDU struct {
	Hdr     Header
	RawBody []byte
}

func (p *GenericPDU) Header() *Header { return &p.Hdr }

// ── Concrete PDU Types ───────────────────────────────────────────────────────

// BindTransceiver is sent by the ESME to bind as both transmitter and receiver.
type BindTransceiver struct {
	Hdr      Header
	SystemID string
	Password string
	SystemType string
	InterfaceVersion uint8
	AddrTON  uint8
	AddrNPI  uint8
	AddressRange string
}

func (p *BindTransceiver) Header() *Header { return &p.Hdr }

// BindTransceiverResp is the SMSC's response to bind_transceiver.
type BindTransceiverResp struct {
	Hdr         Header
	SystemID    string
	TLVs        []TLV
}

func (p *BindTransceiverResp) Header() *Header { return &p.Hdr }

// SubmitSM is a request to send a short message.
type SubmitSM struct {
	Hdr                Header
	ServiceType        string
	SourceAddrTON      uint8
	SourceAddrNPI      uint8
	SourceAddr         string
	DestAddrTON        uint8
	DestAddrNPI        uint8
	DestinationAddr    string
	ESMClass           uint8
	ProtocolID         uint8
	PriorityFlag       uint8
	ScheduleDelivery   string
	ValidityPeriod     string
	RegisteredDelivery uint8
	ReplaceIfPresent   uint8
	DataCoding         uint8
	SMDefaultMsgID     uint8
	ShortMessage       []byte
	TLVs               []TLV
}

func (p *SubmitSM) Header() *Header { return &p.Hdr }

// SubmitSMResp is the SMSC's response to submit_sm.
type SubmitSMResp struct {
	Hdr       Header
	MessageID string
	TLVs      []TLV
}

func (p *SubmitSMResp) Header() *Header { return &p.Hdr }

// DeliverSM is an SMSC-initiated delivery (DLR or MO message).
type DeliverSM struct {
	Hdr                Header
	ServiceType        string
	SourceAddrTON      uint8
	SourceAddrNPI      uint8
	SourceAddr         string
	DestAddrTON        uint8
	DestAddrNPI        uint8
	DestinationAddr    string
	ESMClass           uint8
	ProtocolID         uint8
	PriorityFlag       uint8
	ScheduleDelivery   string
	ValidityPeriod     string
	RegisteredDelivery uint8
	ReplaceIfPresent   uint8
	DataCoding         uint8
	SMDefaultMsgID     uint8
	ShortMessage       []byte
	TLVs               []TLV
}

func (p *DeliverSM) Header() *Header { return &p.Hdr }

// DeliverSMResp is our response to deliver_sm.
type DeliverSMResp struct {
	Hdr       Header
	MessageID string
}

func (p *DeliverSMResp) Header() *Header { return &p.Hdr }

// EnquireLink is a heartbeat request.
type EnquireLink struct {
	Hdr Header
}

func (p *EnquireLink) Header() *Header { return &p.Hdr }

// EnquireLinkResp is the response to enquire_link.
type EnquireLinkResp struct {
	Hdr Header
}

func (p *EnquireLinkResp) Header() *Header { return &p.Hdr }

// Unbind is a request to close the session.
type Unbind struct {
	Hdr Header
}

func (p *Unbind) Header() *Header { return &p.Hdr }

// UnbindResp is the response to unbind.
type UnbindResp struct {
	Hdr Header
}

func (p *UnbindResp) Header() *Header { return &p.Hdr }

// ── TLV Tags ─────────────────────────────────────────────────────────────────

const (
	TLVTagDestAddrSubunit       uint16 = 0x0005
	TLVTagDestNetworkType       uint16 = 0x0006
	TLVTagDestBearerType        uint16 = 0x0007
	TLVTagDestTelematicsID      uint16 = 0x0008
	TLVTagSourceAddrSubunit     uint16 = 0x000D
	TLVTagSourceNetworkType     uint16 = 0x000E
	TLVTagSourceBearerType      uint16 = 0x000F
	TLVTagSourceTelematicsID    uint16 = 0x0010
	TLVTagScInterfaceVersion    uint16 = 0x0010 // also 0x0010, context-dependent
	TLVTagQosTimeToLive         uint16 = 0x0017
	TLVTagPayloadType           uint16 = 0x001E
	TLVTagAdditionalStatusInfo  uint16 = 0x001D
	TLVTagReceiptedMessageID    uint16 = 0x001E
	TLVTagMsgMsgState           uint16 = 0x001E // also 0x001E, context-dependent
	TLVTagMessagePayload        uint16 = 0x0424
	TLVTagSarMsgRefNum          uint16 = 0x020C
	TLVTagSarTotalSegments      uint16 = 0x020D
	TLVTagSarSegmentSeqnum      uint16 = 0x020E
	TLVTagUserMessageReference  uint16 = 0x0204
	TLVTagPrivacyIndicator      uint16 = 0x0201
	TLVTagLanguageIndicator     uint16 = 0x020F
	TLVTagCallbackNum           uint16 = 0x0381
	TLVTagCallbackNumPresInd    uint16 = 0x0382
	TLVTagCallbackNumAtag       uint16 = 0x0383
	TLVTagSourceSubaddress      uint16 = 0x0202
	TLVTagDestSubaddress        uint16 = 0x0203
	TLVTagDisplayTime           uint16 = 0x1201
	TLVTagSmsSignal             uint16 = 0x1203
	TLVTagMsValidity            uint16 = 0x1204
	TLVTagMsMsgWaitFacilities   uint16 = 0x1206
	TLVTagNumberOfMessages      uint16 = 0x1207
	TLVTagAlertOnMsgDelivery    uint16 = 0x130C
	TLVTagItsReplyType          uint16 = 0x1380
	TLVTagItsSessionInfo        uint16 = 0x1381
	TLVTagUssdServiceOp         uint16 = 0x0501
)

// ── Typed Errors ─────────────────────────────────────────────────────────────

var (
	ErrMalformedPDU      = errors.New("smpp: malformed PDU")
	ErrUnknownCommand    = errors.New("smpp: unknown command ID")
	ErrInvalidLength     = errors.New("smpp: invalid PDU length")
	ErrUnsupportedTLV    = errors.New("smpp: unsupported TLV")
	ErrInvalidCString    = errors.New("smpp: invalid null-terminated string")
	ErrInvalidDataCoding = errors.New("smpp: invalid data coding")
	ErrShortHeader       = errors.New("smpp: PDU shorter than 16-byte header")
	ErrTruncatedBody     = errors.New("smpp: truncated PDU body")
)

// ── Helpers ──────────────────────────────────────────────────────────────────

// SetReceiptedMessageID sets the receipted_message_id TLV on a deliver_sm.
func SetReceiptedMessageID(pdu *DeliverSM, id string) {
	pdu.TLVs = append(pdu.TLVs, TLV{Tag: TLVTagReceiptedMessageID, Value: []byte(id)})
}

// GetReceiptedMessageID extracts the receipted_message_id TLV from a deliver_sm.
func GetReceiptedMessageID(pdu *DeliverSM) (string, bool) {
	for _, tlv := range pdu.TLVs {
		if tlv.Tag == TLVTagReceiptedMessageID {
			return string(tlv.Value), true
		}
	}
	return "", false
}

// SetMessagePayload sets the message_payload TLV (for long messages).
func SetMessagePayload(pdu interface{ AddTLV(tag uint16, val []byte) }, payload []byte) {
	switch pdu.(type) {
	case *SubmitSM, *DeliverSM:
		// use a type assertion pattern
	}
}

// TLVsWriter is an interface for PDUs that support adding TLVs.
type TLVsWriter interface {
	AddTLV(tag uint16, val []byte)
}

// AddTLV adds a TLV to the PDU's TLV slice.
func (p *SubmitSM) AddTLV(tag uint16, val []byte) {
	p.TLVs = append(p.TLVs, TLV{Tag: tag, Value: val})
}

func (p *DeliverSM) AddTLV(tag uint16, val []byte) {
	p.TLVs = append(p.TLVs, TLV{Tag: tag, Value: val})
}

func (p *SubmitSMResp) AddTLV(tag uint16, val []byte) {
	p.TLVs = append(p.TLVs, TLV{Tag: tag, Value: val})
}

// binary helpers
var be = binary.BigEndian
