package connector

import (
	"encoding/json"
	"time"

	"github.com/raghna/fury-sms-gateway/internal/connector/rule"
)

// ConnectorConfig is the complete runtime configuration for a connector instance.
// It is protocol-agnostic: common fields are shared across all protocols,
// and protocol-specific transport configuration is stored as raw JSON.
//
// Stored as JSONB in the database, managed entirely through the UI/API.
// The GenericConnector loads this config, renders templates, evaluates rules,
// and delegates transport to the ProtocolDriver.
type ConnectorConfig struct {
	// Metadata identifies the connector instance.
	Metadata MetadataConfig `json:"metadata"`

	// Templates for rendering message fields into request data.
	Templates TemplateConfig `json:"templates"`

	// Authentication (shared across protocols).
	Auth AuthConfig `json:"auth"`

	// Rules define how to interpret the transport response.
	Rules RuleConfig `json:"rules"`

	// Health check configuration.
	Health HealthCheckConfig `json:"health"`

	// Behavioral settings.
	Timeout DurationConfig `json:"timeout"`
	Retry   RetryConfig    `json:"retry"`

	// Transport is protocol-specific configuration as raw JSON.
	// Each driver knows how to decode its own transport config.
	// HTTP:  {"url":"...", "method":"POST", "headers":[...], "body":{...}}
	// SMPP:  {"host":"...", "port":2775, "system_id":"...", "password":"..."}
	// SIP:   {"proxy":"...", "domain":"...", "credentials":{...}}
	Transport json.RawMessage `json:"transport"`
}

// MetadataConfig identifies a connector instance.
type MetadataConfig struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Protocol string `json:"protocol"` // "http", "smpp", "sip"
}

// TemplateConfig defines how message fields map to request data.
type TemplateConfig struct {
	// Fields is a key-value map. The key is the field name used in templates,
	// and the value is a Go template string.
	// Example: {"api_key": "sk-{{.TenantID}}", "callback": "https://{{.Custom host}}/dlr"}
	Fields map[string]string `json:"fields,omitempty"`
}

// AuthConfig describes authentication for the remote endpoint.
type AuthConfig struct {
	// Type of authentication:
	//   "none"       → no authentication
	//   "bearer"     → Authorization: Bearer {{token}}
	//   "basic"      → Authorization: Basic base64(user:pass)
	//   "api_key"    → X-API-Key: {{key}} (HTTP) or field in PDU (SMPP)
	//   "custom"     → protocol-specific custom auth via credentials
	Type string `json:"type"`

	// Credentials holds authentication parameters.
	Credentials map[string]string `json:"credentials,omitempty"`
}

// RuleConfig defines how to interpret transport responses.
type RuleConfig struct {
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
	Enabled  bool           `json:"enabled"`
	Interval DurationConfig `json:"interval"`
	Timeout  DurationConfig `json:"timeout"`
}
