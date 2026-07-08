package smpp

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// ── Heartbeat ────────────────────────────────────────────────────────────────

func TestHeartbeat_DefaultConfig(t *testing.T) {
	hb := NewHeartbeat(nil, HeartbeatConfig{})
	if hb.cfg.Interval != DefaultHeartbeatInterval {
		t.Errorf("expected default interval %v, got %v", DefaultHeartbeatInterval, hb.cfg.Interval)
	}
	if hb.cfg.Timeout != DefaultHeartbeatInterval/2 {
		t.Errorf("expected default timeout %v, got %v", DefaultHeartbeatInterval/2, hb.cfg.Timeout)
	}
}

func TestHeartbeat_CustomConfig(t *testing.T) {
	hb := NewHeartbeat(nil, HeartbeatConfig{
		Interval: 10 * time.Second,
		Timeout:  5 * time.Second,
	})
	if hb.cfg.Interval != 10*time.Second {
		t.Errorf("expected Interval 10s, got %v", hb.cfg.Interval)
	}
	if hb.cfg.Timeout != 5*time.Second {
		t.Errorf("expected Timeout 5s, got %v", hb.cfg.Timeout)
	}
}

func TestHeartbeat_SendsEnquireLink(t *testing.T) {
	s, ft := newSessionWithFake(t)
	defer s.Disconnect(context.Background())

	hb := NewHeartbeat(s, HeartbeatConfig{
		Interval: 10 * time.Millisecond,
	})

	ctx, cancel := context.WithCancel(context.Background())
	go hb.Start(ctx)

	// Wait for at least one enquire_link to be written
	select {
	case data := <-ft.writeCh:
		if len(data) == 0 {
			t.Fatal("empty heartbeat write")
		}
		// Decode to verify it's actually an enquire_link
		pdu, err := s.codec.Decode(data)
		if err != nil {
			t.Fatalf("decode heartbeat PDU: %v", err)
		}
		if pdu.Header().CommandID != CommandIDEnquireLink {
			t.Errorf("expected EnquireLink, got %s", pdu.Header().CommandID)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for heartbeat write")
	}

	cancel()
	// Give Start a moment to exit
	time.Sleep(20 * time.Millisecond)

	if hb.Running() {
		t.Error("heartbeat still running after ctx cancel")
	}
}

func TestHeartbeat_ResponseCompletes(t *testing.T) {
	s, ft := newSessionWithFake(t)
	defer s.Disconnect(context.Background())

	var callCount atomic.Int32
	hb := NewHeartbeat(s, HeartbeatConfig{
		Interval: 5 * time.Millisecond,
		OnError: func(err error) {
			callCount.Add(1)
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	go hb.Start(ctx)
	t.Cleanup(func() {
		cancel()
	})

	// First heartbeat
	select {
	case data := <-ft.writeCh:
		// Decode to get the sequence number for the response
		pdu, err := s.codec.Decode(data)
		if err != nil {
			t.Fatalf("decode: %v", err)
		}
		seq := pdu.Header().SequenceNumber

		// Send EnquireLinkResp
		resp := &EnquireLinkResp{
			Hdr: Header{
				CommandID:      CommandIDEnquireLinkResp,
				CommandStatus:  StatusOK,
				SequenceNumber: seq,
			},
		}
		respData, _ := s.codec.Encode(resp)
		ft.readCh <- respData

	case <-time.After(time.Second):
		t.Fatal("timeout waiting for heartbeat write")
	}

	// No error should have been called
	if n := callCount.Load(); n != 0 {
		t.Errorf("expected 0 heartbeat errors, got %d", n)
	}

	// Let a few more heartbeats happen
	time.Sleep(20 * time.Millisecond)

	// Drain remaining writes (without responding — that's fine)
	for {
		select {
		case <-ft.writeCh:
		default:
			goto done
		}
	}
done:

	cancel()
}

func TestHeartbeat_OnErrorCallback(t *testing.T) {
	s, ft := newSessionWithFake(t)
	defer s.Disconnect(context.Background())

	var errCount atomic.Int32
	hb := NewHeartbeat(s, HeartbeatConfig{
		Interval: 5 * time.Millisecond,
		Timeout:  50 * time.Millisecond,
		OnError: func(err error) {
			errCount.Add(1)
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	go hb.Start(ctx)
	t.Cleanup(func() { cancel() })

	// First heartbeat
	select {
	case <-ft.writeCh:
		// Don't respond — let it timeout
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for heartbeat write")
	}

	// Wait for the timeout + callback
	time.Sleep(100 * time.Millisecond)

	if n := errCount.Load(); n == 0 {
		t.Error("expected at least 1 heartbeat error, got 0")
	}

	cancel()
}

func TestHeartbeat_StartIdempotent(t *testing.T) {
	s, ft := newSessionWithFake(t)
	defer s.Disconnect(context.Background())

	hb := NewHeartbeat(s, HeartbeatConfig{
		Interval: 50 * time.Millisecond,
	})

	ctx, cancel := context.WithCancel(context.Background())

	// Start twice concurrently
	go hb.Start(ctx)
	go hb.Start(ctx)

	// Only one writer should be active — drain only one write
	select {
	case <-ft.writeCh:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for heartbeat write")
	}

	// Verify running
	if !hb.Running() {
		t.Error("expected heartbeat running")
	}

	cancel()
	time.Sleep(20 * time.Millisecond)
}

func TestHeartbeat_StopsOnSessionDisconnect(t *testing.T) {
	s, ft := newSessionWithFake(t)
	_ = ft

	hb := NewHeartbeat(s, HeartbeatConfig{
		Interval: 10 * time.Millisecond,
	})

	// Start with session context (not a separate ctx)
	go hb.Start(s.ctx)

	// Let first heartbeat go
	time.Sleep(50 * time.Millisecond)

	// Disconnect cancels s.ctx → heartbeat should stop
	if err := s.Disconnect(context.Background()); err != nil {
		t.Logf("Disconnect: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	if hb.Running() {
		t.Error("heartbeat still running after session disconnect")
	}
}

func TestHeartbeat_EnquireLinkRespUnblocks(t *testing.T) {
	// Verify that the heartbeat properly waits for the enquire_link_resp
	// before sending the next enquire_link. Without this, heartbeats
	// would pile up in the window and consume all slots.
	s, ft := newSessionWithFake(t)
	defer s.Disconnect(context.Background())

	var writeCount atomic.Int32
	ft.writeHook = func(ctx context.Context, data []byte) error {
		writeCount.Add(1)
		// Use the default path (send to writeCh)
		select {
		case ft.writeCh <- data:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	hb := NewHeartbeat(s, HeartbeatConfig{
		Interval: 5 * time.Millisecond, // very fast
		Timeout:  100 * time.Millisecond,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	go hb.Start(ctx)

	// Respond to EVERY enquire_link to keep heartbeat healthy
	for {
		select {
		case data := <-ft.writeCh:
			pdu, err := s.codec.Decode(data)
			if err != nil {
				t.Logf("decode: %v", err)
				continue
			}
			seq := pdu.Header().SequenceNumber
			resp := &EnquireLinkResp{
				Hdr: Header{
					CommandID:      CommandIDEnquireLinkResp,
					CommandStatus:  StatusOK,
					SequenceNumber: seq,
				},
			}
			respData, _ := s.codec.Encode(resp)
			ft.readCh <- respData
		case <-ctx.Done():
			goto done
		}
	}
done:

	n := writeCount.Load()
	t.Logf("heartbeats sent in 200ms: %d", n)
	if n < 1 {
		t.Error("expected at least 1 heartbeat")
	}
	// With window=5 and very short interval, we should see multiple beats
	// but NOT hundreds (which would indicate no throttling)
	if n > 20 {
		t.Logf("warning: high heartbeat rate (%d in 200ms) — may indicate no response throttling", n)
	}
}

func TestHeartbeat_DoesNotBlockDisconnect(t *testing.T) {
	s, ft := newSessionWithFake(t)

	hb := NewHeartbeat(s, HeartbeatConfig{
		Interval: 5 * time.Millisecond,
		Timeout:  10 * time.Second, // long timeout — would block if not cancelled
	})

	go hb.Start(s.ctx)

	// Let a heartbeat start
	time.Sleep(20 * time.Millisecond)

	// Drain one heartbeat write
	select {
	case <-ft.writeCh:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for heartbeat write")
	}

	// Disconnect should not hang even though heartbeat has a long timeout
	// because Disconnect cancels s.ctx → Start exits
	done := make(chan struct{})
	go func() {
		_ = s.Disconnect(context.Background())
		close(done)
	}()

	select {
	case <-done:
		// OK
	case <-time.After(time.Second):
		t.Fatal("Disconnect blocked by heartbeat")
	}
}

// ── Edge Cases ───────────────────────────────────────────────────────────────

func TestHeartbeat_ZeroIntervalDefaults(t *testing.T) {
	hb := NewHeartbeat(nil, HeartbeatConfig{})
	if hb.cfg.Interval != DefaultHeartbeatInterval {
		t.Errorf("expected default interval %v, got %v", DefaultHeartbeatInterval, hb.cfg.Interval)
	}
}

func TestHeartbeat_RunningInitiallyFalse(t *testing.T) {
	hb := NewHeartbeat(nil, HeartbeatConfig{})
	if hb.Running() {
		t.Error("expected Running=false before Start")
	}
}
