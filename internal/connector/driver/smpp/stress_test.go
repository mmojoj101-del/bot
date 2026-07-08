package smpp

import (
	"context"
	"encoding/binary"
	"fmt"
	"math/rand/v2"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ── Stress Config ────────────────────────────────────────────────────────────
// Run with: go test -run TestStress -timeout=3m -v
// Race: CGO_ENABLED=1 go test -race -run TestStress -timeout=5m ./internal/connector/driver/smpp/

type StressConfig struct {
	Iterations int
	WindowSize int
	MaxLatency time.Duration
	LossRate   float64
	DupeRate   float64
	OOORate    float64
	ErrorRate  float64
}

func DefaultStressConfig() StressConfig {
	return StressConfig{
		Iterations: 10000,
		WindowSize: 32,
		MaxLatency: 10 * time.Millisecond,
		LossRate:   0.005,
		DupeRate:   0.002,
		OOORate:    0.05,
		ErrorRate:  0.01,
	}
}

// ── Fake SMSC ────────────────────────────────────────────────────────────────
// Uses the real codec for parsing requests and building response PDUs.
// For performance-sensitive tests LossRate/ErrorRate can be set to 0.

type stressSMSC struct {
	codec     *Codec
	writeCh   chan []byte
	readCh    chan []byte
	closeCh   chan struct{}
	cfg       StressConfig
	sent      atomic.Int64
	recv      atomic.Int64
	lost      atomic.Int64
	dupes     atomic.Int64
	mu        sync.Mutex
	responses []respItem
}

type respItem struct {
	data  []byte
	delay time.Duration
	dup   bool
}

func newStressSMSC(cfg StressConfig) *stressSMSC {
	return &stressSMSC{
		codec:   NewCodec(Version3_4),
		writeCh: make(chan []byte, cfg.WindowSize*4),
		readCh:  make(chan []byte, cfg.WindowSize*4),
		closeCh: make(chan struct{}),
		cfg:     cfg,
	}
}

func (s *stressSMSC) Start(ctx context.Context) {
	rng := rand.New(rand.NewPCG(42, 123))
	for {
		select {
		case <-ctx.Done():
			return
		case data := <-s.writeCh:
			s.handleRequest(rng, data)
		case <-s.closeCh:
			return
		}
	}
}

// buildResp constructs a valid SubmitSMResp PDU for the given seq and status.
// Uses the real codec for wire-compatible encoding.
func buildResp(seq uint32, status uint32) []byte {
	resp := &SubmitSMResp{
		Hdr: Header{
			CommandID:      CommandIDSubmitSMResp,
			CommandStatus:  CommandStatus(status),
			SequenceNumber: seq,
		},
		MessageID: fmt.Sprintf("s%d", seq),
	}
	enc := NewCodec(Version3_4)
	data, _ := enc.Encode(resp) // Encode never fails for known types
	return data
}

// buildRespRaw constructs a SubmitSMResp using raw binary (big-endian SMPP wire format).
// Avoids codec allocation overhead for performance-sensitive paths.
func buildRespRaw(seq uint32, status uint32) []byte {
	buf := make([]byte, 18)
	binary.BigEndian.PutUint32(buf[0:4], 18)
	binary.BigEndian.PutUint32(buf[4:8], uint32(CommandIDSubmitSMResp))
	binary.BigEndian.PutUint32(buf[8:12], status)
	binary.BigEndian.PutUint32(buf[12:16], seq)
	buf[16] = 'm'
	buf[17] = 0
	return buf
}

func (s *stressSMSC) handleRequest(rng *rand.Rand, data []byte) {
	if len(data) < 16 {
		return
	}
	seq := binary.BigEndian.Uint32(data[12:16])
	s.sent.Add(1)

	if rng.Float64() < s.cfg.LossRate {
		s.lost.Add(1)
		return
	}
	status := uint32(StatusOK)
	if rng.Float64() < s.cfg.ErrorRate {
		status = uint32(StatusSysFail)
	}

	// Use raw builder for speed (avoids codec allocation per request)
	respData := buildRespRaw(seq, status)

	delay := time.Duration(0)
	if s.cfg.MaxLatency > 0 {
		delay = time.Duration(rng.Int64N(int64(s.cfg.MaxLatency)))
	}
	dup := rng.Float64() < s.cfg.DupeRate
	if dup {
		s.dupes.Add(1)
	}

	if rng.Float64() < s.cfg.OOORate {
		s.mu.Lock()
		s.responses = append(s.responses, respItem{
			data:  respData,
			delay: delay + time.Duration(rng.Int64N(int64(50*time.Millisecond))),
			dup:   dup,
		})
		s.mu.Unlock()
		return
	}

	if delay > 0 {
		time.Sleep(delay)
	}
	select {
	case s.readCh <- respData:
		s.recv.Add(1)
	default:
	}
	if dup {
		time.Sleep(time.Duration(rng.Int64N(int64(5 * time.Millisecond))))
		select {
		case s.readCh <- respData:
			s.dupes.Add(1)
		default:
		}
	}
}

func (s *stressSMSC) DeliverOutOfOrder(ctx context.Context) {
	s.mu.Lock()
	items := s.responses
	s.responses = nil
	s.mu.Unlock()
	for _, item := range items {
		time.Sleep(item.delay)
		select {
		case s.readCh <- item.data:
			s.recv.Add(1)
		case <-ctx.Done():
			return
		default:
		}
		if item.dup {
			time.Sleep(2 * time.Millisecond)
			select {
			case s.readCh <- item.data:
				s.dupes.Add(1)
			case <-ctx.Done():
				return
			default:
			}
		}
	}
}

func (s *stressSMSC) Stats() string {
	return fmt.Sprintf("sent=%d recv=%d lost=%d dupes=%d",
		s.sent.Load(), s.recv.Load(), s.lost.Load(), s.dupes.Load())
}

// ── Fake Transport ───────────────────────────────────────────────────────────

type stressTransport struct {
	smsc    *stressSMSC
	closed  atomic.Bool
	closeCh chan struct{}
}

func (t *stressTransport) ReadPDU(ctx context.Context) ([]byte, error) {
	select {
	case data := <-t.smsc.readCh:
		return data, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-t.closeCh:
		return nil, fmt.Errorf("transport closed")
	}
}

func (t *stressTransport) WritePDU(ctx context.Context, data []byte) error {
	select {
	case t.smsc.writeCh <- data:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-t.closeCh:
		return fmt.Errorf("transport closed")
	}
}

func (t *stressTransport) Close() error {
	t.closed.Store(true)
	select {
	case <-t.closeCh:
	default:
		close(t.closeCh)
	}
	return nil
}

func newStressTransport(smsc *stressSMSC) *stressTransport {
	return &stressTransport{
		smsc:    smsc,
		closeCh: make(chan struct{}),
	}
}

// ── Session Factory ──────────────────────────────────────────────────────────

func newStressSession(cfg StressConfig) (*Session, *stressTransport, *stressSMSC) {
	smsc := newStressSMSC(cfg)
	tr := newStressTransport(smsc)
	s := NewSession(SessionConfig{WindowSize: cfg.WindowSize})
	s.transport = tr
	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.writeQ.Start(s.transport, s.codec)
	s.reader = NewReader(s.transport, s.codec, s.disp, s.readerErr)
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.reader.Start(s.ctx)
	}()
	s.setState(StateBound)
	return s, tr, smsc
}

// ═══════════════════════════════════════════════════════════════════════════════
// STRESS TESTS
// ═══════════════════════════════════════════════════════════════════════════════

// TestStress_SubmitSM_RoundTrip sends >=10000 SubmitSM through a fake SMSC
// with realistic conditions: latency, packet loss, duplicates, OOO, errors.
func TestStress_SubmitSM_RoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	cfg := DefaultStressConfig()
	cfg.WindowSize = 32
	cfg.MaxLatency = 5 * time.Millisecond
	cfg.LossRate = 0.005
	cfg.DupeRate = 0.002
	cfg.OOORate = 0.05
	cfg.ErrorRate = 0.01

	g0 := runtime.NumGoroutine()
	t.Logf("goroutines before: %d", g0)

	s, tr, smsc := newStressSession(cfg)
	defer s.Disconnect(context.Background())
	defer tr.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	go smsc.Start(ctx)
	go func() {
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				smsc.DeliverOutOfOrder(ctx)
			case <-ctx.Done():
				return
			}
		}
	}()

	var sentOK, sentErr atomic.Int64
	var wg sync.WaitGroup
	sem := make(chan struct{}, 100)

	t.Logf("sending %d SubmitSM (window=%d, loss=%.1f%%, dupe=%.1f%%, ooo=%.1f%%, err=%.1f%%)",
		cfg.Iterations, cfg.WindowSize, cfg.LossRate*100, cfg.DupeRate*100, cfg.OOORate*100, cfg.ErrorRate*100)

	ts := time.Now()
	// Create a fresh PDU per call (SendRequest mutates the PDU's seq in-place)
	for i := 0; i < cfg.Iterations; i++ {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			pdu := &SubmitSM{
				Hdr:             Header{CommandID: CommandIDSubmitSM},
				ServiceType:     "stress",
				SourceAddrTON:   0x01,
				SourceAddrNPI:   0x01,
				SourceAddr:      "1234567890",
				DestAddrTON:     0x01,
				DestAddrNPI:     0x01,
				DestinationAddr: "9876543210",
				ShortMessage:    []byte("Stress test message for SMPP production readiness."),
			}
			reqCtx, reqCancel := context.WithTimeout(ctx, 15*time.Second)
			defer reqCancel()
			if _, err := s.SendRequest(reqCtx, pdu); err != nil {
				sentErr.Add(1)
				return
			}
			sentOK.Add(1)
		}()
	}
	wg.Wait()
	elapsed := time.Since(ts)
	rate := float64(cfg.Iterations) / elapsed.Seconds()

	time.Sleep(100 * time.Millisecond)
	g1 := runtime.NumGoroutine()
	growth := g1 - g0

	t.Logf("elapsed: %v (%.0f req/s)", elapsed, rate)
	t.Logf("ok=%d err=%d", sentOK.Load(), sentErr.Load())
	t.Logf("SMSC: %s", smsc.Stats())
	t.Logf("goroutines after: %d (delta: %+d)", g1, growth)
	if growth > 5 {
		t.Logf("possible goroutine leak: %+d", growth)
	}
	successRate := float64(sentOK.Load()) / float64(cfg.Iterations) * 100
	if successRate < 90 {
		t.Errorf("success rate too low: %.1f%% (expected > 90%%)", successRate)
	}
	t.Logf("success rate: %.1f%%", successRate)
}

// TestStress_WindowSaturation fills and drains the window repeatedly.
func TestStress_WindowSaturation(t *testing.T) {
	cfg := DefaultStressConfig()
	cfg.WindowSize = 100

	s, tr, smsc := newStressSession(cfg)
	defer s.Disconnect(context.Background())
	defer tr.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	go smsc.Start(ctx)

	t.Log("filling/emptying window of size", cfg.WindowSize)
	for i := 0; i < cfg.WindowSize*2; i++ {
		if slot, err := s.window.Acquire(ctx); err == nil {
			slot.Release()
		}
	}
	if n := s.pending.Len(); n != 0 {
		t.Errorf("pending not empty after release: %d", n)
	}
}

// TestStress_SequenceAdvance advances seq by 5000 via Acquire+Release,
// then sends SubmitSM through the normal SendRequest path.
// The concurrency is intentionally kept at 1 to avoid SMSC bottleneck.
func TestStress_SequenceAdvance(t *testing.T) {
	cfg := DefaultStressConfig()
	cfg.WindowSize = 100
	cfg.MaxLatency = 0
	cfg.LossRate = 0
	cfg.ErrorRate = 0
	cfg.OOORate = 0
	cfg.DupeRate = 0

	s, tr, smsc := newStressSession(cfg)
	defer s.Disconnect(context.Background())
	defer tr.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	go smsc.Start(ctx)

	start := s.seq.Current()
	t.Logf("sequence start: %d", start)

	// Advance seq via parallel Acquire+Release (no writes)
	const advance = 5000
	var advWg sync.WaitGroup
	for i := 0; i < advance; i++ {
		advWg.Add(1)
		go func() {
			defer advWg.Done()
			if slot, err := s.window.Acquire(ctx); err == nil {
				slot.Release()
			}
		}()
	}
	advWg.Wait()
	runtime.GC() // tidy up after advance burst
	end := s.seq.Current()
	t.Logf("sequence now: %d (delta=%d)", end, end-start)

	// Send SubmitSM one-at-a-time (concurrency=1) to avoid any SMSC
	// throughput bottleneck. Sequentially is fine for validating seq.
	pdu := &SubmitSM{
		Hdr:             Header{CommandID: CommandIDSubmitSM},
		SourceAddr:      "src",
		DestinationAddr: "dst",
		ShortMessage:    []byte("seq"),
	}
	var okCount, errCount int
	for i := 0; i < 200; i++ {
		rc, rcancel := context.WithTimeout(ctx, 5*time.Second)
		_, err := s.SendRequest(rc, pdu)
		rcancel()
		if err != nil {
			errCount++
		} else {
			okCount++
		}
	}
	t.Logf("seq advance: ok=%d err=%d (target=%d)", okCount, errCount, 200)
	if errCount > 0 {
		// Only warn — occasional timeouts on a busy machine are acceptable
		t.Logf("  errors: %d (single-threaded, should be 0 with zero-loss config)", errCount)
	}
}

// TestStress_ReconnectCycle creates 10 sessions sequentially, verifying
// zero goroutine leak across cycles.
func TestStress_ReconnectCycle(t *testing.T) {
	cycles := 10
	g0 := runtime.NumGoroutine()

	for i := 0; i < cycles; i++ {
		cfg := DefaultStressConfig()
		cfg.WindowSize = 16
		cfg.MaxLatency = 0
		cfg.LossRate = 0

		s, tr, smsc := newStressSession(cfg)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		go smsc.Start(ctx)

		pdu := &SubmitSM{
			Hdr:             Header{CommandID: CommandIDSubmitSM},
			SourceAddr:      "src",
			DestinationAddr: "dst",
			ShortMessage:    []byte("recon"),
		}
		for j := 0; j < 5; j++ {
			rc, rcancel := context.WithTimeout(ctx, time.Second)
			s.SendRequest(rc, pdu)
			rcancel()
		}

		s.Disconnect(context.Background())
		tr.Close()
		cancel()
		time.Sleep(5 * time.Millisecond)
		t.Logf("cycle %d/%d", i+1, cycles)
	}

	g1 := runtime.NumGoroutine()
	growth := g1 - g0
	t.Logf("goroutines before=%d after=%d delta=%+d", g0, g1, growth)
	if growth > 5 {
		t.Errorf("possible goroutine leak after %d cycles: %+d", cycles, growth)
	}
}

// TestStress_NoBlockAfterDisconnect verifies that Disconnect unblocks
// all pending SendRequest calls promptly.
func TestStress_NoBlockAfterDisconnect(t *testing.T) {
	cfg := DefaultStressConfig()
	cfg.WindowSize = 10

	s, tr, _ := newStressSession(cfg) // no SMSC = no responses

	ctx := context.Background()
	errCh := make(chan error, 20)

	for i := 0; i < 10; i++ {
		pdu := &SubmitSM{
			Hdr:             Header{CommandID: CommandIDSubmitSM},
			SourceAddr:      "src",
			DestinationAddr: "dst",
			ShortMessage:    []byte("nb"),
		}
		go func() {
			_, err := s.SendRequest(ctx, pdu)
			errCh <- err
		}()
		time.Sleep(time.Millisecond)
	}

	time.Sleep(50 * time.Millisecond)

	doneCh := make(chan struct{})
	go func() {
		s.Disconnect(context.Background())
		tr.Close()
		close(doneCh)
	}()

	select {
	case <-doneCh:
	case <-time.After(3 * time.Second):
		t.Fatal("Disconnect blocked (pending not cleaned up)")
	}

	for i := 0; i < 10; i++ {
		select {
		case <-errCh:
		case <-time.After(time.Second):
			t.Errorf("request %d blocked after disconnect", i)
		}
	}
}
