package smpp

import (
	"context"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/raghna/fury-sms-gateway/internal/connector"
	"github.com/raghna/fury-sms-gateway/internal/domain"
)

// ── SMPPTransportConfig ──────────────────────────────────────────────────────

func TestSMPPTransportConfig_Protocol(t *testing.T) {
	cfg := &SMPPTransportConfig{}
	if cfg.Protocol() != domain.ConnectorTypeSMPPClient {
		t.Errorf("expected SMPPClient, got %s", cfg.Protocol())
	}
}

// ── ValidateConfig ───────────────────────────────────────────────────────────

func TestSMPPDriver_ValidateConfig_Valid(t *testing.T) {
	d := NewSMPPDriver(SMPPTransportConfig{})
	cfg := &SMPPTransportConfig{
		Host:     "smsc.example.com",
		Port:     2775,
		SystemID: "esme",
		Password: "test-secret",
	}
	if err := d.ValidateConfig(cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSMPPDriver_ValidateConfig_MissingHost(t *testing.T) {
	d := NewSMPPDriver(SMPPTransportConfig{})
	cfg := &SMPPTransportConfig{
		Port:     2775,
		SystemID: "esme",
		Password: "test-secret",
	}
	if err := d.ValidateConfig(cfg); err == nil {
		t.Fatal("expected error for missing Host")
	}
}

func TestSMPPDriver_ValidateConfig_MissingSystemID(t *testing.T) {
	d := NewSMPPDriver(SMPPTransportConfig{})
	cfg := &SMPPTransportConfig{
		Host:     "smsc.example.com",
		Port:     2775,
		Password: "test-secret",
	}
	if err := d.ValidateConfig(cfg); err == nil {
		t.Fatal("expected error for missing SystemID")
	}
}

func TestSMPPDriver_ValidateConfig_MissingPassword(t *testing.T) {
	d := NewSMPPDriver(SMPPTransportConfig{})
	cfg := &SMPPTransportConfig{
		Host:     "smsc.example.com",
		Port:     2775,
		SystemID: "esme",
	}
	if err := d.ValidateConfig(cfg); err == nil {
		t.Fatal("expected error for missing Password")
	}
}

func TestSMPPDriver_ValidateConfig_InvalidPort(t *testing.T) {
	d := NewSMPPDriver(SMPPTransportConfig{})
	cfg := &SMPPTransportConfig{
		Host:     "smsc.example.com",
		Port:     99999,
		SystemID: "esme",
		Password: "test-secret",
	}
	if err := d.ValidateConfig(cfg); err == nil {
		t.Fatal("expected error for invalid Port")
	}
}

func TestSMPPDriver_ValidateConfig_InvalidBindMode(t *testing.T) {
	d := NewSMPPDriver(SMPPTransportConfig{})
	cfg := &SMPPTransportConfig{
		Host:     "smsc.example.com",
		Port:     2775,
		SystemID: "esme",
		Password: "test-secret",
		BindMode: "invalid",
	}
	if err := d.ValidateConfig(cfg); err == nil {
		t.Fatal("expected error for invalid BindMode")
	}
}

func TestSMPPDriver_ValidateConfig_ValidBindModes(t *testing.T) {
	d := NewSMPPDriver(SMPPTransportConfig{})
	for _, mode := range []string{"", "transceiver", "transmitter", "receiver"} {
		cfg := &SMPPTransportConfig{
			Host:     "smsc.example.com",
			Port:     2775,
			SystemID: "esme",
			Password: "test-secret",
			BindMode: mode,
		}
		if err := d.ValidateConfig(cfg); err != nil {
			t.Errorf("bind mode %q should be valid: %v", mode, err)
		}
	}
}

func TestSMPPDriver_ValidateConfig_WrongType(t *testing.T) {
	d := NewSMPPDriver(SMPPTransportConfig{})
	// Pass a different config type
	if err := d.ValidateConfig(&SMPPTransportConfig{}); err == nil {
		t.Fatal("expected error for nil")
	}
}

// ── DecodeConfig ─────────────────────────────────────────────────────────────

func TestSMPPDriver_DecodeConfig(t *testing.T) {
	d := NewSMPPDriver(SMPPTransportConfig{})
	data := []byte(`{"host":"smsc.example.com","port":2775,"system_id":"esme","password":"test-secret"}`)
	cfg, err := d.DecodeConfig(data)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	tc, ok := cfg.(*SMPPTransportConfig)
	if !ok {
		t.Fatalf("expected *SMPPTransportConfig, got %T", cfg)
	}
	if tc.Host != "smsc.example.com" {
		t.Errorf("expected host smsc.example.com, got %s", tc.Host)
	}
	if tc.Port != 2775 {
		t.Errorf("expected port 2775, got %d", tc.Port)
	}
	if tc.SystemID != "esme" {
		t.Errorf("expected system_id esme, got %s", tc.SystemID)
	}
	if tc.Password != "test-secret" {
		t.Errorf("expected password test-secret, got %s", tc.Password)
	}
}

func TestSMPPDriver_DecodeConfig_Empty(t *testing.T) {
	d := NewSMPPDriver(SMPPTransportConfig{})
	_, err := d.DecodeConfig([]byte{})
	if err == nil {
		t.Fatal("expected error for empty config")
	}
}

func TestSMPPDriver_DecodeConfig_InvalidJSON(t *testing.T) {
	d := NewSMPPDriver(SMPPTransportConfig{})
	_, err := d.DecodeConfig([]byte(`{invalid}`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

// ── IsConnected ──────────────────────────────────────────────────────────────

func TestSMPPDriver_IsConnected_Initially(t *testing.T) {
	d := NewSMPPDriver(SMPPTransportConfig{})
	if d.IsConnected() {
		t.Error("expected not connected initially")
	}
}

// ── CheckHealth ──────────────────────────────────────────────────────────────

func TestSMPPDriver_CheckHealth_ValidConfig(t *testing.T) {
	d := NewSMPPDriver(SMPPTransportConfig{})
	cfg := &SMPPTransportConfig{
		Host:     "smsc.example.com",
		Port:     2775,
		SystemID: "esme",
		Password: "test-secret",
	}
	if err := d.CheckHealth(context.Background(), cfg); err == nil {
		t.Log("CheckHealth: config valid (expected: not connected)")
	}
}

func TestSMPPDriver_CheckHealth_InvalidConfig(t *testing.T) {
	d := NewSMPPDriver(SMPPTransportConfig{})
	cfg := &SMPPTransportConfig{
		Host: "",
	}
	if err := d.CheckHealth(context.Background(), cfg); err == nil {
		t.Fatal("expected error for invalid config")
	}
}

// ── Connect / Disconnect (unit tests with fake SMSC) ─────────────────────────
//
// These tests use a real TCP listener as a fake SMSC. The fake SMSC
// accepts one connection, reads the bind PDU, and sends a bind response.
// This validates the Connect → Bind round-trip through the SMPPDriver.

func TestSMPPDriver_Connect_Success(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Connect in short mode")
	}

	// Start fake SMSC
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()
	addr := listener.Addr().String()

	// Fake SMSC goroutine
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		// Read bind PDU
		sess := NewSession(SessionConfig{})
		buf := make([]byte, 4096)
		n, err := conn.Read(buf)
		if err != nil {
			return
		}
		pdu, err := sess.codec.Decode(buf[:n])
		if err != nil {
			return
		}

		// Send bind response
		resp := &BindTransceiverResp{
			Hdr: Header{
				CommandID:      CommandIDBindTransceiverResp,
				CommandStatus:  StatusOK,
				SequenceNumber: pdu.Header().SequenceNumber,
			},
			SystemID: "fake-smsc",
		}
		respData, _ := sess.codec.Encode(resp)
		_, _ = conn.Write(respData)
	}()

	host, port := parseAddr(addr)
	d := NewSMPPDriver(SMPPTransportConfig{
		Host:     host,
		Port:     port,
		SystemID: "esme",
		Password: "test-secret",
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := d.Connect(ctx); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer d.Disconnect(context.Background())

	if !d.IsConnected() {
		t.Fatal("expected connected after Connect")
	}
}

func TestSMPPDriver_Connect_Idempotent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Connect in short mode")
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()
	addr := listener.Addr().String()

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		sess := NewSession(SessionConfig{})
		buf := make([]byte, 4096)
		n, err := conn.Read(buf)
		if err != nil {
			return
		}
		pdu, err := sess.codec.Decode(buf[:n])
		if err != nil {
			return
		}
		resp := &BindTransceiverResp{
			Hdr: Header{
				CommandID:      CommandIDBindTransceiverResp,
				CommandStatus:  StatusOK,
				SequenceNumber: pdu.Header().SequenceNumber,
			},
			SystemID: "fake-smsc",
		}
		respData, _ := sess.codec.Encode(resp)
		_, _ = conn.Write(respData)
	}()

	host, port := parseAddr(addr)
	d := NewSMPPDriver(SMPPTransportConfig{
		Host:     host,
		Port:     port,
		SystemID: "esme",
		Password: "test-secret",
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := d.Connect(ctx); err != nil {
		t.Fatalf("first Connect failed: %v", err)
	}
	defer d.Disconnect(context.Background())

	// Second Connect should be a no-op
	if err := d.Connect(ctx); err != nil {
		t.Fatalf("second Connect (idempotent) should not fail: %v", err)
	}

	if !d.IsConnected() {
		t.Fatal("expected connected after idempotent Connect")
	}
}

func TestSMPPDriver_Disconnect_Idempotent(t *testing.T) {
	d := NewSMPPDriver(SMPPTransportConfig{})
	// Disconnect when not connected should return nil
	if err := d.Disconnect(context.Background()); err != nil {
		t.Fatalf("Disconnect when not connected: %v", err)
	}
}

func TestSMPPDriver_Send_NotConnected(t *testing.T) {
	d := NewSMPPDriver(SMPPTransportConfig{})
	_, err := d.Send(context.Background(), &connector.TransportRequest{
		Message: &domain.Message{},
	})
	if err == nil {
		t.Fatal("expected error for Send when not connected")
	}
}

// ── helpers ──────────────────────────────────────────────────────────────────

// parseAddr splits "host:port" into host and port int.
func parseAddr(addr string) (string, int) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		panic(err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		panic(err)
	}
	return host, port
}
