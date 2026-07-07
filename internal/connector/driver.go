package connector

import (
	"context"
	"fmt"
	"time"

	"github.com/raghna/fury-sms-gateway/internal/domain"
)

// ProtocolDriver is the only interface a protocol implementation needs.
// The driver is responsible ONLY for transport — it knows nothing about
// ConnectorConfig, templates, auth, rules, or business logic.
//
// Lifecycle:
//  1. GenericConnector renders transport JSON (string replace {{key}})
//  2. GenericConnector calls DecodeConfig(renderedJSON) → typed TransportConfig
//  3. GenericConnector optionally calls ValidateConfig(typedConfig) for pre-flight
//  4. GenericConnector renders typedConfig fields (field-by-field, JSON-safe)
//  5. GenericConnector calls Send() with the fully-rendered typed config
//  6. Driver builds the protocol message, sends, returns raw response
//
// The driver has ZERO awareness of auth, templates, rules, circuit breakers,
// metrics, or any ConnectorConfig structure.
type ProtocolDriver interface {
	// Protocol returns the protocol identifier (e.g., "http", "smpp", "sip").
	Protocol() domain.ConnectorType

	// DecodeConfig decodes raw JSON into the driver's typed transport config.
	// The JSON has all templates already rendered by GenericConnector.
	// Returns a TransportConfig (protocol-specific struct implementing the interface).
	DecodeConfig(data []byte) (TransportConfig, error)

	// ValidateConfig checks whether a decoded TransportConfig is valid.
	// Called by GenericConnector after DecodeConfig, and also by the API/UI
	// before creating a connector — no connector instance needed.
	// Returns nil if valid, error describing the problem otherwise.
	ValidateConfig(cfg TransportConfig) error

	// Send transmits a message through this protocol.
	// req.Config is a fully-rendered, validated TransportConfig (from DecodeConfig).
	// The driver returns raw transport data only — no interpretation.
	Send(ctx context.Context, req *TransportRequest) (*TransportResponse, error)

	// CheckHealth verifies the remote endpoint is reachable using the given config.
	// SMPP needs the config (host:port, system_id) to perform a bind check.
	// HTTP can be stateless — the driver may ignore cfg if not needed.
	CheckHealth(ctx context.Context, cfg TransportConfig) error
}

// TransportConfig is the interface for protocol-specific transport configuration.
// Each driver defines its own typed struct implementing this interface.
// GenericConnector never inspects this — it only decodes, validates, and passes through.
//
// Examples:
//   - HTTPTransportConfig{URL, Method, Headers, Body}
//   - SMPPTransportConfig{Host, Port, SystemID, Password, BindMode}
//   - SIPTransportConfig{Proxy, Domain, Credentials}
type TransportConfig interface {
	// Protocol returns which protocol this config is for.
	Protocol() domain.ConnectorType
}

// TransportRequest carries everything a driver needs to send one message.
// The GenericConnector ensures all fields are rendered and ready.
// The driver simply builds the protocol message and sends it.
type TransportRequest struct {
	// Message is the original domain message.
	Message *domain.Message

	// Prepared carries normalization results (destination, encoding, parts).
	Prepared *domain.PreparedMessage

	// Config is the fully-rendered, decoded, validated transport config.
	// The driver type-asserts this to its expected type.
	Config TransportConfig
}

// TransportResponse is the raw protocol response.
// The driver returns this without interpretation — no business logic.
type TransportResponse struct {
	// Status is the protocol status code.
	// HTTP: 200, 202, 400, 500, etc.
	// SMPP: command_status (0 = ESME_ROK, etc.)
	Status int

	// Headers contains response headers or metadata.
	// HTTP: Content-Type, X-Request-ID, etc.
	// SMPP: sequence_number, etc.
	Headers map[string]string

	// Body is the raw response body bytes.
	Body []byte

	// ExternalID is the message ID returned by the remote system, if any.
	ExternalID string

	// Latency is how long the transport took.
	Latency time.Duration
}

// ── Driver Registry ──────────────────────────────────────────────────────────

// DriverRegistry maps protocol types to their ProtocolDriver implementations.
// Adding a new protocol is just: registry.Register(smppDriver)
type DriverRegistry interface {
	// Register adds a driver for a protocol. Panics if protocol already registered.
	Register(driver ProtocolDriver)

	// MustRegister is like Register but panics with a descriptive message on error.
	// Useful in init() or main() for required drivers.
	MustRegister(driver ProtocolDriver)

	// Replace replaces a driver for an existing protocol, or registers if new.
	// Unlike Register, if the protocol is already registered, it's overwritten.
	// Useful for testing (swap HTTP driver with mock) or plugin updates.
	Replace(driver ProtocolDriver)

	// Get returns the driver for the given protocol. Returns error if not found.
	Get(protocol domain.ConnectorType) (ProtocolDriver, error)

	// Unregister removes a driver for a protocol. No-op if not registered.
	// Useful in test cleanup or plugin unload.
	Unregister(protocol domain.ConnectorType)

	// Protocols returns all registered protocol types.
	Protocols() []domain.ConnectorType
}

// NewDriverRegistry creates an empty driver registry.
func NewDriverRegistry() DriverRegistry {
	return &driverRegistry{
		drivers: make(map[domain.ConnectorType]ProtocolDriver),
	}
}

type driverRegistry struct {
	drivers map[domain.ConnectorType]ProtocolDriver
}

func (r *driverRegistry) Register(driver ProtocolDriver) {
	p := driver.Protocol()
	if _, ok := r.drivers[p]; ok {
		panic(fmt.Sprintf("driver already registered for protocol %q — use Replace to overwrite", p))
	}
	r.drivers[p] = driver
}

func (r *driverRegistry) MustRegister(driver ProtocolDriver) {
	p := driver.Protocol()
	if _, ok := r.drivers[p]; ok {
		panic(fmt.Sprintf("must register: driver already exists for protocol %q", p))
	}
	r.drivers[p] = driver
}

func (r *driverRegistry) Replace(driver ProtocolDriver) {
	r.drivers[driver.Protocol()] = driver
}

func (r *driverRegistry) Get(protocol domain.ConnectorType) (ProtocolDriver, error) {
	d, ok := r.drivers[protocol]
	if !ok {
		return nil, fmt.Errorf("no driver registered for protocol %q", protocol)
	}
	return d, nil
}

func (r *driverRegistry) Unregister(protocol domain.ConnectorType) {
	delete(r.drivers, protocol)
}

func (r *driverRegistry) Protocols() []domain.ConnectorType {
	protocols := make([]domain.ConnectorType, 0, len(r.drivers))
	for p := range r.drivers {
		protocols = append(protocols, p)
	}
	return protocols
}
