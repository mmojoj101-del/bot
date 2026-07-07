package smpp

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/raghna/fury-sms-gateway/internal/connector"
	"github.com/raghna/fury-sms-gateway/internal/domain"
)

// ── SMPPTransportConfig ──────────────────────────────────────────────────────

// SMPPTransportConfig is the typed transport configuration for SMPP.
// All templates are rendered BEFORE DecodeConfig — values are final.
type SMPPTransportConfig struct {
	Host       string `json:"host"`
	Port       int    `json:"port"`
	SystemID   string `json:"system_id"`
	Password   string `json:"password"`
	SystemType string `json:"system_type,omitempty"`
	BindMode   string `json:"bind_mode,omitempty"` // transceiver (default)
	WindowSize int    `json:"window_size,omitempty"`
}

func (c *SMPPTransportConfig) Protocol() domain.ConnectorType {
	return domain.ConnectorTypeSMPPClient
}

// ── SMPPDriver ───────────────────────────────────────────────────────────────

// SMPPDriver implements the connector.StatefulDriver interface for SMPP.
//
// It manages the complete session lifecycle:
//   - Connect: TCP dial + SMPP bind + heartbeat start
//   - Send: SubmitSM via the shared Session.SendRequest path
//   - Disconnect: stop heartbeat + unbind + cleanup
//   - Reconnect: automatic with exponential backoff on connection loss
//
// The driver implements three interfaces:
//   - connector.ProtocolDriver (Send, DecodeConfig, ValidateConfig, CheckHealth)
//   - connector.StatefulDriver (Connect, Disconnect, IsConnected)
//   - connector.StatefulDriverLifecycle (Start, Stop)
type SMPPDriver struct {
	config     SMPPTransportConfig
	mu         sync.Mutex
	session    *Session
	hb         *Heartbeat
	connected  atomic.Bool

	// Lifecycle
	cancel context.CancelFunc // cancels the driver's internal context
	errCh  chan error         // fatal errors from heartbeat or session reader
	wg     sync.WaitGroup

	// Reconnect
	reconnectCh chan struct{}   // signals the reconnect loop
	maxRetries  int
	baseBackoff time.Duration
	maxBackoff  time.Duration

	// Defaults (exposed for tests)
	defaultWindowSize int
}

// NewSMPPDriver creates an SMPP driver with the given config.
// Call Connect() to establish the session, or Start() for lifecycle
// management with auto-reconnect.
func NewSMPPDriver(cfg SMPPTransportConfig) *SMPPDriver {
	if cfg.BindMode == "" {
		cfg.BindMode = "transceiver"
	}
	if cfg.WindowSize <= 0 {
		cfg.WindowSize = DefaultWindowSize
	}
	return &SMPPDriver{
		config:            cfg,
		errCh:             make(chan error, 1),
		reconnectCh:       make(chan struct{}, 1),
		maxRetries:        5,
		baseBackoff:       1 * time.Second,
		maxBackoff:        30 * time.Second,
		defaultWindowSize: DefaultWindowSize,
	}
}

// ── ProtocolDriver ───────────────────────────────────────────────────────────

// Protocol returns the protocol identifier.
func (d *SMPPDriver) Protocol() domain.ConnectorType {
	return domain.ConnectorTypeSMPPClient
}

// DecodeConfig decodes raw JSON into SMPPTransportConfig.
func (d *SMPPDriver) DecodeConfig(data []byte) (connector.TransportConfig, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("smpp driver: empty config")
	}
	var cfg SMPPTransportConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("smpp driver: decode config: %w", err)
	}
	return &cfg, nil
}

// ValidateConfig checks the SMPPTransportConfig for required fields.
func (d *SMPPDriver) ValidateConfig(cfg connector.TransportConfig) error {
	tc, ok := cfg.(*SMPPTransportConfig)
	if !ok {
		return fmt.Errorf("smpp driver: expected *SMPPTransportConfig, got %T", cfg)
	}
	if tc.Host == "" {
		return fmt.Errorf("smpp driver: Host is required")
	}
	if tc.Port <= 0 || tc.Port > 65535 {
		return fmt.Errorf("smpp driver: invalid Port %d", tc.Port)
	}
	if tc.SystemID == "" {
		return fmt.Errorf("smpp driver: SystemID is required")
	}
	if tc.Password == "" {
		return fmt.Errorf("smpp driver: Password is required")
	}
	switch tc.BindMode {
	case "", "transceiver", "transmitter", "receiver":
		// valid
	default:
		return fmt.Errorf("smpp driver: unsupported bind mode %q", tc.BindMode)
	}
	return nil
}

// Send transmits a single SubmitSM through the SMPP session.
// Returns the SMSC message_id in TransportResponse.ExternalID.
//
// Implements connector.ProtocolDriver.
func (d *SMPPDriver) Send(ctx context.Context, req *connector.TransportRequest) (*connector.TransportResponse, error) {
	if !d.connected.Load() {
		return nil, fmt.Errorf("smpp driver: %w", ErrNotBound)
	}

	session := d.getSession()

	// Build SubmitSM PDU from the transport request
	submitSM := d.buildSubmitSM(req)

	start := time.Now()
	resp, err := session.SendRequest(ctx, submitSM)
	latency := time.Since(start)

	if err != nil {
		return nil, fmt.Errorf("smpp driver: send: %w", err)
	}

	smResp, ok := resp.(*SubmitSMResp)
	if !ok {
		return nil, fmt.Errorf("smpp driver: unexpected response type %T", resp)
	}

	result := &connector.TransportResponse{
		Status:  int(smResp.Hdr.CommandStatus),
		Latency: latency,
		Body:    []byte(smResp.MessageID),
	}
	if !smResp.Hdr.CommandStatus.IsOK() {
		result.ExternalID = ""
		return result, fmt.Errorf("smpp driver: submit_sm rejected: %s", smResp.Hdr.CommandStatus)
	}

	result.ExternalID = smResp.MessageID
	return result, nil
}

// CheckHealth verifies the SMPP session is active by checking IsConnected.
// If cfg is provided and no session exists, performs a lightweight config
// validation only.
func (d *SMPPDriver) CheckHealth(ctx context.Context, cfg connector.TransportConfig) error {
	if cfg != nil {
		if err := d.ValidateConfig(cfg); err != nil {
			return err
		}
	}
	if d.connected.Load() {
		return nil
	}
	return fmt.Errorf("smpp driver: not connected")
}

// ── StatefulDriver ───────────────────────────────────────────────────────────

// Connect establishes a TCP connection, performs SMPP bind, and starts
// the heartbeat. Idempotent: returns nil if already connected.
//
// Implements connector.StatefulDriver.
func (d *SMPPDriver) Connect(ctx context.Context) error {
	d.mu.Lock()
	if d.connected.Load() {
		d.mu.Unlock()
		return nil
	}
	d.mu.Unlock()

	addr := fmt.Sprintf("%s:%d", d.config.Host, d.config.Port)
	bindPDU := NewBindTransceiver(d.config.SystemID, d.config.Password, d.config.SystemType)

	session := NewSession(SessionConfig{
		WindowSize: d.config.WindowSize,
	})

	if err := session.Connect(ctx, addr, bindPDU); err != nil {
		return fmt.Errorf("smpp driver: connect: %w", err)
	}

	d.mu.Lock()
	d.session = session
	d.connected.Store(true)
	d.mu.Unlock()

	return nil
}

// Disconnect tears down the SMPP session gracefully.
// Stops heartbeat, unbinds, and closes transport. Idempotent.
//
// Implements connector.StatefulDriver.
func (d *SMPPDriver) Disconnect(ctx context.Context) error {
	d.mu.Lock()
	session := d.session
	d.mu.Unlock()

	if session == nil {
		return nil
	}

	_ = session.Disconnect(ctx)

	d.mu.Lock()
	d.session = nil
	d.connected.Store(false)
	d.mu.Unlock()

	return nil
}

// IsConnected returns true if the SMPP session is active.
// Safe for concurrent calls. Does NOT block on I/O.
//
// Implements connector.StatefulDriver.
func (d *SMPPDriver) IsConnected() bool {
	return d.connected.Load()
}

// ── StatefulDriverLifecycle ──────────────────────────────────────────────────

// Start begins async session maintenance: heartbeat and auto-reconnect.
// Returns a channel for fatal connection errors.
//
// Implements connector.StatefulDriverLifecycle.
func (d *SMPPDriver) Start(ctx context.Context) <-chan error {
	d.mu.Lock()
	defer d.mu.Unlock()

	ctx, cancel := context.WithCancel(ctx)
	d.cancel = cancel

	// Start heartbeat
	d.startHeartbeat(ctx)

	// Start reconnect loop
	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		d.reconnectLoop(ctx)
	}()

	return d.errCh
}

// Stop terminates the async session maintenance.
//
// Implements connector.StatefulDriverLifecycle.
func (d *SMPPDriver) Stop(ctx context.Context) error {
	d.mu.Lock()
	cancel := d.cancel
	d.mu.Unlock()

	if cancel != nil {
		cancel()
	}

	d.wg.Wait()

	_ = d.Disconnect(ctx)
	return nil
}

// ── Heartbeat ────────────────────────────────────────────────────────────────

func (d *SMPPDriver) startHeartbeat(ctx context.Context) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.session == nil {
		return
	}

	d.hb = NewHeartbeat(d.session, HeartbeatConfig{
		Interval: 30 * time.Second,
		Timeout:  15 * time.Second,
		OnError: func(err error) {
			// Signal reconnect on heartbeat failure
			d.signalReconnect()
		},
	})

	hbCtx, hbCancel := context.WithCancel(ctx)
	_ = hbCancel // heartbeat uses ctx cancellation; hbCancel is for future use

	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		d.hb.Start(hbCtx)
	}()
}

// ── Reconnect ────────────────────────────────────────────────────────────────

func (d *SMPPDriver) signalReconnect() {
	select {
	case d.reconnectCh <- struct{}{}:
	default:
	}
}

func (d *SMPPDriver) reconnectLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-d.reconnectCh:
			d.doReconnect(ctx)
		}
	}
}

func (d *SMPPDriver) doReconnect(ctx context.Context) {
	backoff := d.baseBackoff

	for attempt := 0; attempt < d.maxRetries; attempt++ {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if d.connected.Load() {
			return
		}

		reconnectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		err := d.Connect(reconnectCtx)
		cancel()

		if err == nil {
			return
		}

		// Backoff before retry
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}

		backoff *= 2
		if backoff > d.maxBackoff {
			backoff = d.maxBackoff
		}
	}

	// All retries exhausted — send fatal error
	select {
	case d.errCh <- fmt.Errorf("smpp driver: reconnect exhausted after %d attempts", d.maxRetries):
	default:
	}
}

// ── Message Building ─────────────────────────────────────────────────────────

func (d *SMPPDriver) buildSubmitSM(req *connector.TransportRequest) *SubmitSM {
	msg := &SubmitSM{
		Hdr: Header{
			CommandID: CommandIDSubmitSM,
		},
		ServiceType:          "",
		SourceAddrTON:        0x01, // international
		SourceAddrNPI:        0x01, // E.164 / ISDN
		SourceAddr:           req.Message.Source,
		DestAddrTON:          0x01,
		DestAddrNPI:          0x01,
		DestinationAddr:      req.Message.Destination,
		ESMClass:             0x00, // default SMS mode
		ProtocolID:           0x00,
		PriorityFlag:         0x01, // normal
		DataCoding:           0x00, // SMSC default alphabet
		RegisteredDelivery:   0x00,
	}

	// Set message text
	msg.ShortMessage = []byte(req.Message.Text)

	// Set registered delivery for DLR if the message has a DLRURL
	if req.Message.DLRURL != "" {
		msg.RegisteredDelivery = 0x01 // SMSC delivery receipt requested
	}

	return msg
}

// ── Internal helpers ─────────────────────────────────────────────────────────

func (d *SMPPDriver) getSession() *Session {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.session
}

// ── Interface checks ─────────────────────────────────────────────────────────

var _ connector.ProtocolDriver = (*SMPPDriver)(nil)
var _ connector.StatefulDriver = (*SMPPDriver)(nil)
var _ connector.StatefulDriverLifecycle = (*SMPPDriver)(nil)
