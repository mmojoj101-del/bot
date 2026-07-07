// Package httpconnector provides a generic, config-driven HTTP connector.
//
// The connector is purely an orchestrator — it loads EndpointConfig, delegates
// to builder/transport/parser components, and returns domain.SendResult.
//
// No provider-specific code exists. Any HTTP API (REST, SOAP, GraphQL) can
// be integrated solely through EndpointConfig — no Go code changes.
package httpconnector

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/raghna/fury-sms-gateway/internal/connector/rule"
	"github.com/raghna/fury-sms-gateway/internal/domain"
	"github.com/raghna/fury-sms-gateway/internal/template"
)

// Connector implements the protocol-agnostic Connector interface
// for HTTP endpoints. It is fully config-driven via EndpointConfig.
//
// Runtime flow:
//
//	Load Config → Render Templates → Build Request →
//	Apply Auth → Send → Parse Response → Evaluate Rules → Return SendResult
type Connector struct {
	id       string
	protocol domain.ConnectorType
	config   EndpointConfig

	client      *Client
	tmpl        *template.Engine
	ruleEngine  *rule.Engine
	healthCheck *HealthChecker

	mu   sync.Mutex
	once sync.Once
}

// ConnectorOption configures the HTTP connector.
type ConnectorOption func(*Connector)

// WithClient sets a custom HTTP client.
func WithClient(client *Client) ConnectorOption {
	return func(c *Connector) { c.client = client }
}

// WithTemplateEngine sets a custom template engine.
func WithTemplateEngine(tmpl *template.Engine) ConnectorOption {
	return func(c *Connector) { c.tmpl = tmpl }
}

// WithRuleEngine sets a custom rule engine.
func WithRuleEngine(re *rule.Engine) ConnectorOption {
	return func(c *Connector) { c.ruleEngine = re }
}

// WithTimeout overrides the config timeout.
func WithTimeout(timeout time.Duration) ConnectorOption {
	return func(c *Connector) {
		c.config.Timeout = DurationConfig{Seconds: int(timeout.Seconds())}
	}
}

// NewConnector creates a new HTTP connector from config.
//
// The connector is lazy-initialized: the HTTP client, template engine,
// and rule engine are created on first use via sync.Once.
//
// config is the EndpointConfig for this connector (from DB/JSONB).
func NewConnector(id string, config EndpointConfig, opts ...ConnectorOption) *Connector {
	c := &Connector{
		id:       id,
		protocol: domain.ConnectorTypeHTTPClient,
		config:   config,
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// ID returns the unique connector identifier.
func (c *Connector) ID() string {
	return c.id
}

// Protocol returns the connector protocol type.
func (c *Connector) Protocol() domain.ConnectorType {
	return c.protocol
}

// ensureInit initializes shared components on first use.
func (c *Connector) ensureInit() {
	c.once.Do(func() {
		c.mu.Lock()
		defer c.mu.Unlock()

		if c.client == nil {
			c.client = NewClient(c.config.Timeout.Duration())
		}
		if c.tmpl == nil {
			c.tmpl = template.NewEngine()
		}
		if c.ruleEngine == nil {
			c.ruleEngine = rule.NewEngine()
		}
		if c.healthCheck == nil {
			c.healthCheck = NewHealthChecker(c.config.Health, c.client, c.ruleEngine)
		}
	})
}

// Send implements the Connector interface.
//
// Execution flow:
//  1. Render request templates (URL, headers, body)
//  2. Apply authentication
//  3. Send HTTP request
//  4. Read response
//  5. Parse response + evaluate rules
//  6. Return domain.SendResult
func (c *Connector) Send(ctx context.Context, req *domain.SendRequest) (*domain.SendResult, error) {
	c.ensureInit()

	// 1. Build template data from message
	if req.Message == nil {
		return nil, fmt.Errorf("http connector %q: message is nil", c.id)
	}
	if req.Prepared == nil {
		return nil, fmt.Errorf("http connector %q: prepared message is nil (run PrepareStage first)", c.id)
	}

	data := template.Data{
		Source:      req.Message.Source,
		Destination: req.Prepared.Destination,
		Text:        req.Message.Text,
		Parts:       req.Prepared.Parts,
		Encoding:    string(req.Prepared.Encoding),
		ClientRef:   req.Message.ClientRef,
		MessageID:   req.Message.ID,
		TenantID:    req.Message.TenantID,
	}

	// 2. Build HTTP request
	httpReq, err := BuildRequest(&c.config, data, c.tmpl)
	if err != nil {
		return nil, fmt.Errorf("http connector %q: build request: %w", c.id, err)
	}

	// 3. Send and read response
	statusCode, headers, body, err := c.client.DoAndRead(ctx, httpReq)
	if err != nil {
		return nil, fmt.Errorf("http connector %q: send: %w", c.id, err)
	}

	// 4. Reconstruct *http.Response for the parser (needs status + headers)
	resp := &http.Response{
		StatusCode: statusCode,
		Header:     headers,
		Body:       io.NopCloser(nil), // body is already read
	}

	// 5. Parse response (evaluate rules, extract fields)
	result := ParseResponse(resp, body, c.config.Response, c.ruleEngine)

	// 6. Enhance with connector-specific acceptance semantics
	//    (HTTP knows if ExternalID was extracted = final vs pending DLR)
	if result.Acceptance == domain.AcceptanceFinal && result.ExternalID == "" {
		// Success without external ID — DLR may follow
		result.Acceptance = domain.AcceptancePendingDLR
	}

	return result, nil
}

// CheckHealth checks if the connector is healthy.
// Implements the optional HealthChecker interface.
func (c *Connector) CheckHealth(ctx context.Context) error {
	c.ensureInit()
	return c.healthCheck.CheckHealth(ctx)
}
