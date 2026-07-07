package connector

import (
	"context"
	"time"

	"github.com/raghna/fury-sms-gateway/internal/domain"
)

// ProtocolDriver is the only interface a protocol implementation needs.
// The driver is responsible ONLY for transport-level communication:
//   - HTTP driver: build HTTP request → send → return raw response
//   - SMPP driver: bind → submit_sm → return raw PDU response
//   - SIP driver: build SIP MESSAGE → send → return raw response
//
// The driver does NOT handle:
//   - Template rendering (done by GenericConnector)
//   - Rule evaluation (done by GenericConnector)
//   - Retry logic (done by GenericConnector)
//   - Circuit breaker (done by GenericConnector)
//   - Metrics (done by GenericConnector)
//   - Business decisions (all in GenericConnector)
type ProtocolDriver interface {
	// Protocol returns the protocol identifier (e.g., "http", "smpp", "sip").
	Protocol() domain.ConnectorType

	// Send transmits a message through this protocol and returns the raw
	// transport response. The driver does NOT interpret the response —
	// it returns raw status, headers, and body for the GenericConnector
	// to evaluate via the Rule Engine.
	Send(ctx context.Context, req *TransportRequest) (*TransportResponse, error)

	// CheckHealth verifies the remote endpoint is reachable through this protocol.
	CheckHealth(ctx context.Context) error
}

// TransportRequest carries everything a driver needs to send one message.
// The GenericConnector is responsible for rendering templates and preparing
// the request data — the driver just uses it as-is for transport.
type TransportRequest struct {
	// Message is the original domain message (source, destination, text).
	Message *domain.Message

	// Prepared carries normalization results (destination, encoding, parts).
	Prepared *domain.PreparedMessage

	// Config is the protocol-specific transport configuration as JSON.
	// HTTP:  URL, method, headers, body template (already rendered)
	// SMPP:  host, port, system_id, password, bind_mode
	// SIP:   proxy, domain, credentials
	Config []byte

	// RenderedFields contains pre-rendered template values.
	// The driver uses these directly — no template parsing in the driver.
	RenderedFields map[string]string
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

	// Body is the raw response body.
	// HTTP: JSON/XML/HTML response body
	// SMPP: PDU body
	Body []byte

	// ExternalID is the message ID returned by the remote system, if any.
	// HTTP: JSON field like "message_id"
	// SMPP: message_id from submit_sm_resp
	ExternalID string

	// Latency is how long the transport took (end-to-end).
	Latency time.Duration
}
