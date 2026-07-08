package smpp

import (
	"fmt"
	"sync/atomic"
)

// ── Decoder Registry ─────────────────────────────────────────────────────────

// DecoderFunc decodes a PDU body (everything after the 16-byte header).
// The header has already been parsed and is passed for context.
type DecoderFunc func(hdr *Header, body []byte) (PDU, error)

// ── Codec ────────────────────────────────────────────────────────────────────

// Codec handles SMPP PDU binary encoding and decoding.
// It is protocol-pure: no knowledge of Session, Window, or Transport.
//
// Decoder registry is per-instance (not global), allowing registration of
// vendor-specific or custom PDUs without modifying the codec.
//
// Error contract (IMPORTANT):
//
//	Decode() errors are NON-FATAL for the transport stream. The PDU was
//	already fully consumed from the transport (the 4-byte length prefix
//	determined exactly how many bytes to read). A decode error means the
//	bytes are malformed (bad C-string, wrong length, unknown encoding)
//	but the stream position is still correct.
//
//	The Reader MUST continue reading after a Decode() error.
//	Only transport.ReadPDU() errors (EOF, reset, timeout) are fatal.
//
//	This contract is part of Codec's responsibility, not Reader's.
//
// Concurrency: RegisterDecoder MUST be called before the first Decode/Encode
// call (i.e., during initialization), NOT concurrently with ongoing operations.
// Freeze() is called automatically on first Decode/Encode via atomic.Bool.
type Codec struct {
	Version  Version
	decoders map[CommandID]DecoderFunc
	frozen   atomic.Bool
}

// NewCodec creates a Codec for the given SMPP version,
// pre-loaded with all standard SMPP v3.4 decoders.
func NewCodec(version Version) *Codec {
	c := &Codec{
		Version:  version,
		decoders: make(map[CommandID]DecoderFunc),
	}
	// Register all standard decoders
	c.RegisterDecoder(CommandIDGenericNack, decodeGenericNack)
	c.RegisterDecoder(CommandIDEnquireLink, decodeEnquireLink)
	c.RegisterDecoder(CommandIDEnquireLinkResp, decodeEnquireLinkResp)
	c.RegisterDecoder(CommandIDUnbind, decodeUnbind)
	c.RegisterDecoder(CommandIDUnbindResp, decodeUnbindResp)
	c.RegisterDecoder(CommandIDBindTransceiver, decodeBindTransceiver)
	c.RegisterDecoder(CommandIDBindTransceiverResp, decodeBindTransceiverResp)
	c.RegisterDecoder(CommandIDSubmitSM, decodeSubmitSM)
	c.RegisterDecoder(CommandIDSubmitSMResp, decodeSubmitSMResp)
	c.RegisterDecoder(CommandIDDeliverSM, decodeDeliverSM)
	c.RegisterDecoder(CommandIDDeliverSMResp, decodeDeliverSMResp)
	return c
}

// RegisterDecoder registers a decoder function for the given command ID.
// If a decoder already exists, it is overwritten.
//
// MUST be called during initialization, before the first Decode/Encode.
// Panics if the codec is frozen. Thread-safe only when called from one goroutine.
func (c *Codec) RegisterDecoder(cmd CommandID, fn DecoderFunc) {
	if c.frozen.Load() {
		panic("smpp: codec is frozen — cannot register decoder after first use")
	}
	c.decoders[cmd] = fn
}

// Freeze makes the decoder registry immutable.
// Call after all decoders are registered, before concurrent usage.
func (c *Codec) Freeze() {
	c.frozen.Store(true)
}

// Decode parses a complete SMPP PDU from binary data.
func (c *Codec) Decode(data []byte) (PDU, error) {
	c.frozen.Store(true) // freeze on first use
	if len(data) < 16 {
		return nil, fmt.Errorf("%w: got %d bytes, need at least 16", ErrShortHeader, len(data))
	}

	hdr := decodeHeader(data)
	if int(hdr.Length) > len(data) {
		return nil, fmt.Errorf("%w: header claims %d bytes, got %d", ErrTruncatedBody, hdr.Length, len(data))
	}
	if hdr.Length < 16 {
		return nil, fmt.Errorf("%w: header length %d is less than minimum 16", ErrTruncatedBody, hdr.Length)
	}

	body := data[16:hdr.Length]
	decoder, ok := c.decoders[hdr.CommandID]
	if !ok {
		// Unknown command — preserve raw body for debugging
		return &GenericPDU{Hdr: *hdr, RawBody: body}, nil
	}
	return decoder(hdr, body)
}

// Encode serializes a PDU into binary form.
func (c *Codec) Encode(pdu PDU) ([]byte, error) {
	hdr := pdu.Header()

	// Encode body based on type
	body, err := encodeBody(pdu)
	if err != nil {
		return nil, err
	}

	// Length = header (16) + body
	totalLen := 16 + len(body)
	hdr.Length = uint32(totalLen)

	// Write header
	buf := make([]byte, totalLen)
	be.PutUint32(buf[0:4], hdr.Length)
	be.PutUint32(buf[4:8], uint32(hdr.CommandID))
	be.PutUint32(buf[8:12], uint32(hdr.CommandStatus))
	be.PutUint32(buf[12:16], hdr.SequenceNumber)

	// Copy body
	copy(buf[16:], body)
	return buf, nil
}

// ── Header ───────────────────────────────────────────────────────────────────

func decodeHeader(data []byte) *Header {
	return &Header{
		Length:         be.Uint32(data[0:4]),
		CommandID:      CommandID(be.Uint32(data[4:8])),
		CommandStatus:  CommandStatus(be.Uint32(data[8:12])),
		SequenceNumber: be.Uint32(data[12:16]),
	}
}

// ── Body Encoding ────────────────────────────────────────────────────────────

func encodeBody(pdu PDU) ([]byte, error) {
	switch p := pdu.(type) {
	case *BindTransceiver:
		return encodeBindTransceiver(p), nil
	case *BindTransceiverResp:
		return encodeBindTransceiverResp(p), nil
	case *SubmitSM:
		return encodeSubmitSM(p), nil
	case *SubmitSMResp:
		return encodeSubmitSMResp(p), nil
	case *DeliverSM:
		return encodeDeliverSM(p), nil
	case *DeliverSMResp:
		return encodeDeliverSMResp(p), nil
	case *EnquireLink, *EnquireLinkResp, *Unbind, *UnbindResp, *GenericPDU:
		return nil, nil // header-only PDUs
	default:
		return nil, fmt.Errorf("%w: %v", ErrUnknownCommand, pdu.Header().CommandID)
	}
}

// ── C-String Helpers ─────────────────────────────────────────────────────────

// encodeCString encodes a string as a null-terminated byte sequence.
// An empty string becomes a single null byte.
func encodeCString(s string) []byte {
	if s == "" {
		return []byte{0}
	}
	b := []byte(s)
	b = append(b, 0)
	return b
}

// decodeCString reads a null-terminated string from the beginning of b.
// Returns the string and the number of bytes consumed (including null).
// Returns an error if no null terminator is found.
func decodeCString(b []byte) (string, int, error) {
	for i, v := range b {
		if v == 0 {
			return string(b[:i]), i + 1, nil
		}
	}
	return "", 0, fmt.Errorf("%w: no null terminator found in %d bytes", ErrInvalidCString, len(b))
}

// ── TLV Helpers ──────────────────────────────────────────────────────────────

// encodeTLVs serializes a TLV slice into binary.
func encodeTLVs(tlvs []TLV) []byte {
	if len(tlvs) == 0 {
		return nil
	}
	buf := make([]byte, 0, len(tlvs)*6) // estimate: tag(2)+len(2)+value(avg)
	for _, tlv := range tlvs {
		buf = be.AppendUint16(buf, tlv.Tag)
		buf = be.AppendUint16(buf, uint16(len(tlv.Value)))
		buf = append(buf, tlv.Value...)
	}
	return buf
}

// decodeTLVs reads TLV parameters from the remainder of a PDU body.
// Returns nil if no TLVs are present (empty remaining data).
func decodeTLVs(data []byte) ([]TLV, error) {
	if len(data) == 0 {
		return nil, nil
	}
	var tlvs []TLV
	for offset := 0; offset < len(data); {
		if offset+4 > len(data) {
			return nil, fmt.Errorf("%w: truncated TLV header at offset %d", ErrUnsupportedTLV, offset)
		}
		tag := be.Uint16(data[offset : offset+2])
		length := int(be.Uint16(data[offset+2 : offset+4]))
		offset += 4
		if offset+length > len(data) {
			return nil, fmt.Errorf("%w: TLV 0x%04X claims %d bytes, %d remaining",
				ErrUnsupportedTLV, tag, length, len(data)-offset)
		}
		value := make([]byte, length)
		copy(value, data[offset:offset+length])
		tlvs = append(tlvs, TLV{Tag: tag, Value: value})
		offset += length
	}
	return tlvs, nil
}

// ── Decoder: GenericNack ─────────────────────────────────────────────────────

func decodeGenericNack(hdr *Header, _ []byte) (PDU, error) {
	return &GenericPDU{Hdr: *hdr}, nil
}

// ── Decoder: EnquireLink / EnquireLinkResp / Unbind / UnbindResp ─────────────

func decodeEnquireLink(hdr *Header, _ []byte) (PDU, error) {
	return &EnquireLink{Hdr: *hdr}, nil
}

func decodeEnquireLinkResp(hdr *Header, _ []byte) (PDU, error) {
	return &EnquireLinkResp{Hdr: *hdr}, nil
}

func decodeUnbind(hdr *Header, _ []byte) (PDU, error) {
	return &Unbind{Hdr: *hdr}, nil
}

func decodeUnbindResp(hdr *Header, _ []byte) (PDU, error) {
	return &UnbindResp{Hdr: *hdr}, nil
}

// ── Decoder: BindTransceiver ─────────────────────────────────────────────────

func decodeBindTransceiver(hdr *Header, body []byte) (PDU, error) {
	p := &BindTransceiver{Hdr: *hdr}
	var n int
	var err error

	if p.SystemID, n, err = decodeCString(body); err != nil {
		return nil, fmt.Errorf("bind_transceiver: system_id: %w", err)
	}
	body = body[n:]

	if p.Password, n, err = decodeCString(body); err != nil {
		return nil, fmt.Errorf("bind_transceiver: password: %w", err)
	}
	body = body[n:]

	if p.SystemType, n, err = decodeCString(body); err != nil {
		return nil, fmt.Errorf("bind_transceiver: system_type: %w", err)
	}
	body = body[n:]

	if len(body) < 4 {
		return nil, fmt.Errorf("%w: bind_transceiver needs 4 more bytes for TON/NPI/range", ErrMalformedPDU)
	}

	p.InterfaceVersion = body[0]
	p.AddrTON = body[1]
	p.AddrNPI = body[2]
	body = body[3:]

	if p.AddressRange, n, err = decodeCString(body); err != nil {
		return nil, fmt.Errorf("bind_transceiver: address_range: %w", err)
	}

	return p, nil
}

func encodeBindTransceiver(p *BindTransceiver) []byte {
	var buf []byte
	buf = append(buf, encodeCString(p.SystemID)...)
	buf = append(buf, encodeCString(p.Password)...)
	buf = append(buf, encodeCString(p.SystemType)...)
	buf = append(buf, p.InterfaceVersion, p.AddrTON, p.AddrNPI)
	buf = append(buf, encodeCString(p.AddressRange)...)
	return buf
}

// ── Decoder: BindTransceiverResp ─────────────────────────────────────────────

func decodeBindTransceiverResp(hdr *Header, body []byte) (PDU, error) {
	p := &BindTransceiverResp{Hdr: *hdr}
	n, err := 0, error(nil)

	if p.SystemID, n, err = decodeCString(body); err != nil {
		return nil, fmt.Errorf("bind_transceiver_resp: system_id: %w", err)
	}
	body = body[n:]

	// Optional TLVs (sc_interface_version)
	if len(body) > 0 {
		p.TLVs, err = decodeTLVs(body)
		if err != nil {
			return nil, fmt.Errorf("bind_transceiver_resp: tlvs: %w", err)
		}
	}
	return p, nil
}

func encodeBindTransceiverResp(p *BindTransceiverResp) []byte {
	var buf []byte
	buf = append(buf, encodeCString(p.SystemID)...)
	buf = append(buf, encodeTLVs(p.TLVs)...)
	return buf
}

// ── Decoder: SubmitSM ────────────────────────────────────────────────────────

func decodeSubmitSM(hdr *Header, body []byte) (PDU, error) {
	p := &SubmitSM{Hdr: *hdr}
	var n int
	var err error

	if p.ServiceType, n, err = decodeCString(body); err != nil {
		return nil, fmt.Errorf("submit_sm: service_type: %w", err)
	}
	body = body[n:]

	if len(body) < 2 {
		return nil, fmt.Errorf("%w: submit_sm needs source TON+NPI", ErrMalformedPDU)
	}
	p.SourceAddrTON = body[0]
	p.SourceAddrNPI = body[1]
	body = body[2:]

	if p.SourceAddr, n, err = decodeCString(body); err != nil {
		return nil, fmt.Errorf("submit_sm: source_addr: %w", err)
	}
	body = body[n:]

	if len(body) < 2 {
		return nil, fmt.Errorf("%w: submit_sm needs dest TON+NPI", ErrMalformedPDU)
	}
	p.DestAddrTON = body[0]
	p.DestAddrNPI = body[1]
	body = body[2:]

	if p.DestinationAddr, n, err = decodeCString(body); err != nil {
		return nil, fmt.Errorf("submit_sm: destination_addr: %w", err)
	}
	body = body[n:]

	if len(body) < 8 {
		return nil, fmt.Errorf("%w: submit_sm needs 8 mandatory bytes after dest", ErrMalformedPDU)
	}
	p.ESMClass = body[0]
	p.ProtocolID = body[1]
	p.PriorityFlag = body[2]
	body = body[3:]

	if p.ScheduleDelivery, n, err = decodeCString(body); err != nil {
		return nil, fmt.Errorf("submit_sm: schedule_delivery: %w", err)
	}
	body = body[n:]

	if p.ValidityPeriod, n, err = decodeCString(body); err != nil {
		return nil, fmt.Errorf("submit_sm: validity_period: %w", err)
	}
	body = body[n:]

	if len(body) < 4 {
		return nil, fmt.Errorf("%w: submit_sm needs registered_delivery+dcs+msgid+msg_len", ErrMalformedPDU)
	}
	p.RegisteredDelivery = body[0]
	p.ReplaceIfPresent = body[1]
	p.DataCoding = body[2]
	p.SMDefaultMsgID = body[3]
	body = body[4:]

	if len(body) < 1 {
		return nil, fmt.Errorf("%w: submit_sm needs short_message length byte", ErrMalformedPDU)
	}
	msgLen := int(body[0])
	body = body[1:]

	if msgLen > 0 {
		if len(body) < msgLen {
			return nil, fmt.Errorf("%w: submit_sm short_message claims %d bytes, %d remaining",
				ErrMalformedPDU, msgLen, len(body))
		}
		p.ShortMessage = make([]byte, msgLen)
		copy(p.ShortMessage, body[:msgLen])
		body = body[msgLen:]
	}

	// Remaining bytes are TLVs
	if len(body) > 0 {
		p.TLVs, err = decodeTLVs(body)
		if err != nil {
			return nil, fmt.Errorf("submit_sm: tlvs: %w", err)
		}
	}

	return p, nil
}

func encodeSubmitSM(p *SubmitSM) []byte {
	var buf []byte
	buf = append(buf, encodeCString(p.ServiceType)...)
	buf = append(buf, p.SourceAddrTON, p.SourceAddrNPI)
	buf = append(buf, encodeCString(p.SourceAddr)...)
	buf = append(buf, p.DestAddrTON, p.DestAddrNPI)
	buf = append(buf, encodeCString(p.DestinationAddr)...)
	buf = append(buf, p.ESMClass, p.ProtocolID, p.PriorityFlag)
	buf = append(buf, encodeCString(p.ScheduleDelivery)...)
	buf = append(buf, encodeCString(p.ValidityPeriod)...)
	buf = append(buf, p.RegisteredDelivery, p.ReplaceIfPresent, p.DataCoding, p.SMDefaultMsgID)

	// short_message length byte + payload
	msgLen := len(p.ShortMessage)
	buf = append(buf, byte(msgLen))
	if msgLen > 0 {
		buf = append(buf, p.ShortMessage...)
	}

	// TLVs
	buf = append(buf, encodeTLVs(p.TLVs)...)
	return buf
}

// ── Decoder: SubmitSMResp ────────────────────────────────────────────────────

func decodeSubmitSMResp(hdr *Header, body []byte) (PDU, error) {
	p := &SubmitSMResp{Hdr: *hdr}
	n, err := 0, error(nil)

	if p.MessageID, n, err = decodeCString(body); err != nil {
		return nil, fmt.Errorf("submit_sm_resp: message_id: %w", err)
	}
	body = body[n:]

	if len(body) > 0 {
		p.TLVs, err = decodeTLVs(body)
		if err != nil {
			return nil, fmt.Errorf("submit_sm_resp: tlvs: %w", err)
		}
	}
	return p, nil
}

func encodeSubmitSMResp(p *SubmitSMResp) []byte {
	var buf []byte
	buf = append(buf, encodeCString(p.MessageID)...)
	buf = append(buf, encodeTLVs(p.TLVs)...)
	return buf
}

// ── Decoder: DeliverSM ───────────────────────────────────────────────────────

func decodeDeliverSM(hdr *Header, body []byte) (PDU, error) {
	// DeliverSM has the same mandatory fields as SubmitSM
	p := &DeliverSM{Hdr: *hdr}
	var n int
	var err error

	if p.ServiceType, n, err = decodeCString(body); err != nil {
		return nil, fmt.Errorf("deliver_sm: service_type: %w", err)
	}
	body = body[n:]

	if len(body) < 2 {
		return nil, fmt.Errorf("%w: deliver_sm needs source TON+NPI", ErrMalformedPDU)
	}
	p.SourceAddrTON = body[0]
	p.SourceAddrNPI = body[1]
	body = body[2:]

	if p.SourceAddr, n, err = decodeCString(body); err != nil {
		return nil, fmt.Errorf("deliver_sm: source_addr: %w", err)
	}
	body = body[n:]

	if len(body) < 2 {
		return nil, fmt.Errorf("%w: deliver_sm needs dest TON+NPI", ErrMalformedPDU)
	}
	p.DestAddrTON = body[0]
	p.DestAddrNPI = body[1]
	body = body[2:]

	if p.DestinationAddr, n, err = decodeCString(body); err != nil {
		return nil, fmt.Errorf("deliver_sm: destination_addr: %w", err)
	}
	body = body[n:]

	if len(body) < 8 {
		return nil, fmt.Errorf("%w: deliver_sm needs 8 mandatory bytes after dest", ErrMalformedPDU)
	}
	p.ESMClass = body[0]
	p.ProtocolID = body[1]
	p.PriorityFlag = body[2]
	body = body[3:]

	if p.ScheduleDelivery, n, err = decodeCString(body); err != nil {
		return nil, fmt.Errorf("deliver_sm: schedule_delivery: %w", err)
	}
	body = body[n:]

	if p.ValidityPeriod, n, err = decodeCString(body); err != nil {
		return nil, fmt.Errorf("deliver_sm: validity_period: %w", err)
	}
	body = body[n:]

	if len(body) < 4 {
		return nil, fmt.Errorf("%w: deliver_sm needs registered_delivery+dcs+msgid+msg_len", ErrMalformedPDU)
	}
	p.RegisteredDelivery = body[0]
	p.ReplaceIfPresent = body[1]
	p.DataCoding = body[2]
	p.SMDefaultMsgID = body[3]
	body = body[4:]

	if len(body) < 1 {
		return nil, fmt.Errorf("%w: deliver_sm needs short_message length byte", ErrMalformedPDU)
	}
	msgLen := int(body[0])
	body = body[1:]

	if msgLen > 0 {
		if len(body) < msgLen {
			return nil, fmt.Errorf("%w: deliver_sm short_message claims %d bytes, %d remaining",
				ErrMalformedPDU, msgLen, len(body))
		}
		p.ShortMessage = make([]byte, msgLen)
		copy(p.ShortMessage, body[:msgLen])
		body = body[msgLen:]
	}

	if len(body) > 0 {
		p.TLVs, err = decodeTLVs(body)
		if err != nil {
			return nil, fmt.Errorf("deliver_sm: tlvs: %w", err)
		}
	}

	return p, nil
}

func encodeDeliverSM(p *DeliverSM) []byte {
	var buf []byte
	buf = append(buf, encodeCString(p.ServiceType)...)
	buf = append(buf, p.SourceAddrTON, p.SourceAddrNPI)
	buf = append(buf, encodeCString(p.SourceAddr)...)
	buf = append(buf, p.DestAddrTON, p.DestAddrNPI)
	buf = append(buf, encodeCString(p.DestinationAddr)...)
	buf = append(buf, p.ESMClass, p.ProtocolID, p.PriorityFlag)
	buf = append(buf, encodeCString(p.ScheduleDelivery)...)
	buf = append(buf, encodeCString(p.ValidityPeriod)...)
	buf = append(buf, p.RegisteredDelivery, p.ReplaceIfPresent, p.DataCoding, p.SMDefaultMsgID)

	msgLen := len(p.ShortMessage)
	buf = append(buf, byte(msgLen))
	if msgLen > 0 {
		buf = append(buf, p.ShortMessage...)
	}
	buf = append(buf, encodeTLVs(p.TLVs)...)
	return buf
}

// ── Decoder: DeliverSMResp ───────────────────────────────────────────────────

func decodeDeliverSMResp(hdr *Header, body []byte) (PDU, error) {
	p := &DeliverSMResp{Hdr: *hdr}
	n, err := 0, error(nil)

	if len(body) > 0 {
		if p.MessageID, n, err = decodeCString(body); err != nil {
			return nil, fmt.Errorf("deliver_sm_resp: message_id: %w", err)
		}
		_ = n
	}
	return p, nil
}

func encodeDeliverSMResp(p *DeliverSMResp) []byte {
	return encodeCString(p.MessageID)
}
