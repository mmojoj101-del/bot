// Package httpconnector provides a generic, config-driven HTTP connector.
//
// The connector is an orchestrator: it loads an EndpointConfig, renders
// templates, builds the request, authenticates, sends, parses the response,
// evaluates rules, and returns a SendResult.
//
// No provider-specific code exists here. Any HTTP API (REST, SOAP, GraphQL)
// can be integrated by configuring an EndpointConfig — no Go code changes.
package httpconnector

import (
	"time"

	"github.com/raghna/fury-sms-gateway/internal/connector/rule"
)

// EndpointConfig is the complete configuration for an HTTP protocol endpoint.
// Stored as JSONB in the database, managed entirely through the UI/API.
//
// Design principles:
//   - Protocol-agnostic structure (shared with SMPP, SIP in future)
//   - Data-driven: all behavior is configured, not hardcoded
//   - Template-powered: URL, headers, body can reference message fields
//   - Rule-based: response handling is configurable conditions+actions
type EndpointConfig struct {
	// Protocol identifies which connector runtime executes this config.
	Protocol string `json:"protocol"` // "http"

	// Request configuration
	Request RequestConfig `json:"request"`

	// Authentication
	Auth AuthConfig `json:"auth"`

	// Response handling rules
	Response ResponseConfig `json:"response"`

	// Behavioral settings
	Timeout DurationConfig `json:"timeout"`
	Retry   RetryConfig    `json:"retry"`

	// Health check
	Health HealthCheckConfig `json:"health"`
}

// RequestConfig describes how to build an HTTP request.
type RequestConfig struct {
	// URL template: "https://api.example.com/send" or "https://{{Custom host}}/send"
	URL string `json:"url"`

	// HTTP method: POST, GET, PUT, DELETE, PATCH
	Method string `json:"method"`

	// Headers is a list of key-value templates.
	// Value can contain {{Source}}, {{Destination}}, {{Text}}, etc.
	Headers []KeyValueConfig `json:"headers,omitempty"`

	// QueryParams is a list of query string key-value templates.
	QueryParams []KeyValueConfig `json:"query_params,omitempty"`

	// Body is the request body configuration (optional for GET).
	Body *BodyConfig `json:"body,omitempty"`
}

// KeyValueConfig is a key-value pair where both key and value support templates.
type KeyValueConfig struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// BodyConfig describes the request body.
type BodyConfig struct {
	// Template is the body content template.
	// JSON: {"to":"{{Destination}}","text":"{{Text}}"}
	// Form: to={{Destination}}&text={{Text}}
	// Plain: {{Text}}
	Template string `json:"template"`

	// ContentType is the Content-Type header value.
	// Examples: application/json, application/x-www-form-urlencoded, text/plain
	ContentType string `json:"content_type"`
}

// AuthConfig describes how to authenticate the request.
type AuthConfig struct {
	// Type of authentication:
	//   "none"        → no authentication
	//   "bearer"      → Authorization: Bearer {{token}}
	//   "basic"       → Authorization: Basic base64(user:pass)
	//   "api_key"     → X-API-Key: {{key}}
	//   "custom_header" → custom header from credentials
	//   "query_param" → append ?api_key={{key}} to URL
	Type string `json:"type"`

	// Credentials holds auth parameters (token, key, username, password).
	Credentials map[string]string `json:"credentials,omitempty"`
}

// ResponseConfig describes how to parse and validate HTTP responses.
type ResponseConfig struct {
	// Rules is an ordered list of conditions and actions.
	Rules []rule.Rule `json:"rules"`
}

// DurationConfig is a JSON-serializable duration.
type DurationConfig struct {
	Seconds int `json:"seconds"`
}

// Duration returns the time.Duration.
func (d DurationConfig) Duration() time.Duration {
	return time.Duration(d.Seconds) * time.Second
}

// RetryConfig describes retry behavior on failures.
type RetryConfig struct {
	MaxAttempts int            `json:"max_attempts"`
	Delay       DurationConfig `json:"delay"`
	Backoff     string         `json:"backoff"` // "fixed", "exponential"
}

// HealthCheckConfig describes how to verify endpoint health.
type HealthCheckConfig struct {
	Enabled    bool           `json:"enabled"`
	URL        string         `json:"url"`
	Method     string         `json:"method"`
	Interval   DurationConfig `json:"interval"`
	Timeout    DurationConfig `json:"timeout"`
	Rule       rule.Rule      `json:"rule"` // single rule to determine health
}
