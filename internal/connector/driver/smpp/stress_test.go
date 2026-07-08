package smpp

import (
	"context"
	"fmt"
	"math/rand/v2"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ── Stress Config ────────────────────────────────────────────────────────────

type StressConfig struct {
	Iterations     int
	WindowSize     int
	MaxLatency     time.Duration
	LossRate       float64
	DupeRate       float64
	OOORate        float64
	ErrorRate      float64
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

type stressSMSC struct {
	codec      *Codec
	writeCh    chan []byte
	readCh     chan []byte
	closeCh    chan struct{}
	cfg        StressConfig
	sent       atomic.Int64
	recv       atomic.Int64
	lost       atomic.Int64
	dupes      atomic.Int64
	mu         sync.Mutex
	responses  []respItem
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

func (s *stressSMSC) handleRequest(rng *rand.Rand, data []byte) {
	pdu, err := s.codec.Decode(data)
	if err != nil {
		return
	}
	seq := pdu.Header().SequenceNumber
	s.sent.Add(1)
	if rng.Float64() < s.cfg.LossRate {
		s.lost.Add(1)
		return
	}
	status := CommandStatus(StatusOK)
	if rng.Float64() < s.cfg.ErrorRate {
		status = StatusSysFail
	}
	resp := &SubmitSMResp{
		Hdr: Header{
			CommandID:      CommandIDSubmitSMResp,
			CommandStatus:  status,
			SequenceNumber: seq,
		},
		MessageID: fmt.Sprintf("stress-%d", seq),
	}
	respData, _ := s.codec.Encode(resp)
	delay := time.Duration(0)
	if s.cfg.MaxLatency > 0 {
		delay = time.Duration(rng.Int64N(int64(s.cfg.MaxLatency)))
	}
	dup := rng.Float64() < s.cfg.DupeRate
	if dup {
		s.dupes.Add(1)
	}
	// out-of-order
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
	// normal delivery
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

// ── Stress Tests ─────────────────────────────────────────────────────────────

func TestStress_SubmitSM_RoundTrip(t *testing.T) {
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

	pdu := &SubmitSM{
		Hdr:             Header{CommandID: CommandIDSubmitSM},
		ServiceType:     "stress",
		SourceAddrTON:   0x01,
		SourceAddrNPI:   0x01,
		SourceAddr:      "1234567890",
		DestAddrTON:     0x01,
		DestAddrNPI:     0x01,
		DestinationAddr:  "9876543210",
		ShortMessage:    []byte("Stress test message for verifying SMPP production readiness."),
	}

	t.Logf("sending %d SubmitSM reqs (window=%d, loss=%.1f%%, dupe=%.1f%%, ooo=%.1f%%, err=%.1f%%)",
		cfg.Iterations, cfg.WindowSize, cfg.LossRate*100, cfg.DupeRate*100, cfg.OOORate*100, cfg.ErrorRate*100)

	start := time.Now()
	for i := 0; i < cfg.Iterations; i++ {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			reqCtx, reqCancel := context.WithTimeout(ctx, 15*time.Second)
			defer reqCancel()
			_, err := s.SendRequest(reqCtx, pdu)
			if err != nil {
				sentErr.Add(1)
				return
			}
			sentOK.Add(1)
		}()
	}
	wg.Wait()
	elapsed := time.Since(start)
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

func TestStress_WindowSaturation(t *testing.T) {
	cfg := DefaultStressConfig()
	cfg.WindowSize = 100
	s, tr, smsc := newStressSession(cfg)
	defer s.Disconnect(context.Background())
	defer tr.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	go smsc.Start(ctx)

	t.Log("filling window of size", cfg.WindowSize)
	for i := 0; i < cfg.WindowSize*2; i++ {
		if slot, err := s.window.Acquire(ctx); err == nil {
			slot.Release()
		}
	}

	if n := s.pending.Len(); n != 0 {
		t.Errorf("expected 0 pending after release, got %d", n)
	}
	t.Log("window saturation OK")
}

func TestStress_SequenceAdvance(t *testing.T) {
	cfg := DefaultStressConfig()
	cfg.WindowSize = 100
	cfg.MaxLatency = 0
	cfg.LossRate = 0
	cfg.ErrorRate = 0
	s, tr, smsc := newStressSession(cfg)
	defer s.Disconnect(context.Background())
	defer tr.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	go smsc.Start(ctx)

	start := s.seq.Current()
	t.Logf("sequence start: %d", start)

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

	end := s.seq.Current()
	t.Logf("sequence now: %d (delta=%d)", end, end-start)

	pdu := &SubmitSM{
		Hdr:             Header{CommandID: CommandIDSubmitSM},
		SourceAddr:      "src",
		DestinationAddr: "dst",
		ShortMessage:    []byte("seq"),
	}
	var okCount, errCount atomic.Int64
	var wg sync.WaitGroup
	target := 500
	for i := 0; i < target; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rc, rcancel := context.WithTimeout(ctx, 5*time.Second)
			defer rcancel()
			if _, err := s.SendRequest(rc, pdu); err != nil {
				errCount.Add(1)
				return
			}
			okCount.Add(1)
		}()
	}
	wg.Wait()
	t.Logf("seq advance: ok=%d err=%d", okCount.Load(), errCount.Load())
	if okCount.Load() < int64(target)-100 {
		t.Errorf("too many failures: %d/%d", okCount.Load(), target)
	}
}

func TestStress_ReconnectCycle(t *testing.T) {
	cycles := 10
	g0 := runtime.NumGoroutine()

	for i := 0; i < cycles; i++ {
		cfg := DefaultStressConfig()
		cfg.WindowSize = 16
		cfg.MaxLatency = time.Millisecond

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

func TestStress_NoBlockAfterDisconnect(t *testing.T) {
	cfg := DefaultStressConfig()
	cfg.WindowSize = 10
	cfg.MaxLatency = time.Second

	s, tr, smsc := newStressSession(cfg)
	_ = smsc // no SMSC — no responses

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
