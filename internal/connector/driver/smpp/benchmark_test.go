package smpp

import (
	"context"
	"testing"
)

// ═══════════════════════════════════════════════════════════════════════════════
// Codec Benchmarks
// ═══════════════════════════════════════════════════════════════════════════════

var (
	benchmarkEncodeResult []byte
	benchmarkDecodeResult PDU
)

func makeBenchPDU() *SubmitSM {
	return &SubmitSM{
		Hdr: Header{
			CommandID:     CommandIDSubmitSM,
			CommandStatus: StatusOK,
		},
		ServiceType:        "cmt",
		SourceAddrTON:      0x01,
		SourceAddrNPI:      0x01,
		SourceAddr:         "1234567890",
		DestAddrTON:        0x01,
		DestAddrNPI:        0x01,
		DestinationAddr:    "9876543210",
		ESMClass:           0x00,
		ProtocolID:         0x00,
		PriorityFlag:       0x01,
		DataCoding:         0x00,
		RegisteredDelivery: 0x01,
		ShortMessage:       []byte("Hello, world! This is a benchmark test message for SMPP."),
	}
}

func BenchmarkCodecEncode_SubmitSM(b *testing.B) {
	codec := NewCodec(Version3_4)
	pdu := makeBenchPDU()
	pdu.Hdr.SequenceNumber = 1

	b.ResetTimer()
	var data []byte
	for i := 0; i < b.N; i++ {
		pdu.Hdr.SequenceNumber = uint32(i + 1)
		data, _ = codec.Encode(pdu)
	}
	benchmarkEncodeResult = data
}

func BenchmarkCodecEncode_BindTransceiver(b *testing.B) {
	codec := NewCodec(Version3_4)
	pdu := NewBindTransceiver("esme", "test-secret", "vms")
	pdu.Hdr.SequenceNumber = 1

	b.ResetTimer()
	var data []byte
	for i := 0; i < b.N; i++ {
		pdu.Hdr.SequenceNumber = uint32(i + 1)
		data, _ = codec.Encode(pdu)
	}
	benchmarkEncodeResult = data
}

func BenchmarkCodecEncode_EnquireLink(b *testing.B) {
	codec := NewCodec(Version3_4)
	pdu := &EnquireLink{
		Hdr: Header{CommandID: CommandIDEnquireLink, SequenceNumber: 1},
	}

	b.ResetTimer()
	var data []byte
	for i := 0; i < b.N; i++ {
		pdu.Hdr.SequenceNumber = uint32(i + 1)
		data, _ = codec.Encode(pdu)
	}
	benchmarkEncodeResult = data
}

func BenchmarkCodecDecode_SubmitSM(b *testing.B) {
	codec := NewCodec(Version3_4)
	pdu := makeBenchPDU()
	pdu.Hdr.SequenceNumber = 1
	data, _ := codec.Encode(pdu)

	b.ResetTimer()
	var result PDU
	for i := 0; i < b.N; i++ {
		result, _ = codec.Decode(data)
	}
	benchmarkDecodeResult = result
}

func BenchmarkCodecDecode_EnquireLink(b *testing.B) {
	codec := NewCodec(Version3_4)
	pdu := &EnquireLink{
		Hdr: Header{CommandID: CommandIDEnquireLink, SequenceNumber: 1},
	}
	data, _ := codec.Encode(pdu)

	b.ResetTimer()
	var result PDU
	for i := 0; i < b.N; i++ {
		result, _ = codec.Decode(data)
	}
	benchmarkDecodeResult = result
}

func BenchmarkCodecDecode_HeaderOnly(b *testing.B) {
	codec := NewCodec(Version3_4)
	data := make([]byte, 16)
	be32(data[0:4], 16)
	be32(data[4:8], uint32(CommandIDEnquireLink))

	b.ResetTimer()
	var result PDU
	for i := 0; i < b.N; i++ {
		result, _ = codec.Decode(data)
	}
	benchmarkDecodeResult = result
}

// ═══════════════════════════════════════════════════════════════════════════════
// WindowManager Benchmarks
// ═══════════════════════════════════════════════════════════════════════════════

func BenchmarkWindow_AcquireRelease(b *testing.B) {
	seq := NewSequenceManager()
	pending := NewPendingStore()
	codec := NewCodec(Version3_4)
	window := NewWindowManager(100, seq, pending, codec)

	ctx := context.Background()
	b.ResetTimer()

	var result *WindowSlot
	for i := 0; i < b.N; i++ {
		result, _ = window.Acquire(ctx)
		result.Release()
	}
	benchmarkWindowResult = result
}

func BenchmarkWindow_AcquireRelease_Parallel(b *testing.B) {
	seq := NewSequenceManager()
	pending := NewPendingStore()
	codec := NewCodec(Version3_4)
	window := NewWindowManager(100, seq, pending, codec)

	ctx := context.Background()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			slot, err := window.Acquire(ctx)
			if err != nil {
				b.Error(err)
				return
			}
			slot.Release()
		}
	})
}

// ═══════════════════════════════════════════════════════════════════════════════
// PendingStore Benchmarks
// ═══════════════════════════════════════════════════════════════════════════════

func BenchmarkPendingStore_RegisterNotify(b *testing.B) {
	pending := NewPendingStore()
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		seq := uint32(i + 1)
		if pr := pending.Register(seq, CommandIDSubmitSM, ctx, ""); pr != nil {
			pending.Notify(seq, nil)
		}
	}
}

func BenchmarkPendingStore_RegisterRemove(b *testing.B) {
	pending := NewPendingStore()
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		seq := uint32(i + 1)
		pending.Register(seq, CommandIDSubmitSM, ctx, "")
		pending.Remove(seq)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// Dispatcher Benchmarks
// ═══════════════════════════════════════════════════════════════════════════════

func BenchmarkDispatcher_Dispatch(b *testing.B) {
	d := NewDispatcher(&nopPendingHandler{}, nil, nil, nil, nil, nil)
	resp := &SubmitSMResp{
		Hdr: Header{
			CommandID:      CommandIDSubmitSMResp,
			CommandStatus:  StatusOK,
			SequenceNumber: 1,
		},
		MessageID: "benchmark-message-id",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d.Dispatch(resp)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// WriteQueue Benchmarks
// ═══════════════════════════════════════════════════════════════════════════════

func BenchmarkWriteQueue_TryEnqueue(b *testing.B) {
	wq := NewWriteQueue(100)
	pdu := &EnquireLinkResp{
		Hdr: Header{
			CommandID:      CommandIDEnquireLinkResp,
			CommandStatus:  StatusOK,
			SequenceNumber: 1,
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = wq.TryEnqueue(pdu)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// Helpers
// ═══════════════════════════════════════════════════════════════════════════════

type nopPendingHandler struct{}

func (h *nopPendingHandler) HandleResponse(resp PDU) {}

var benchmarkWindowResult *WindowSlot

func be32(buf []byte, v uint32) {
	buf[0] = byte(v >> 24)
	buf[1] = byte(v >> 16)
	buf[2] = byte(v >> 8)
	buf[3] = byte(v)
}

func init() {
	_ = benchmarkEncodeResult
	_ = benchmarkDecodeResult
	_ = benchmarkWindowResult
}
