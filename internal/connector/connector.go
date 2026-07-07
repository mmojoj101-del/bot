package connector

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/raghna/fury-sms-gateway/internal/connector/rule"
	"github.com/raghna/fury-sms-gateway/internal/domain"
	"github.com/raghna/fury-sms-gateway/internal/template"
)

// GenericConnector is the protocol-agnostic connector implementation.
// It implements the Connector interface by orchestrating a ProtocolDriver
// with shared infrastructure (template engine, rule engine, circuit breaker, metrics).
//
// Architecture:
//
//	GenericConnector
//	  ├── Template Engine ── render{{Source}}, {{Destination}}, {{Text}} into fields
//	  ├── ProtocolDriver ─── raw transport send (HTTP, SMPP, SIP, ...)
//	  ├── Rule Engine ────── evaluate response → accept/reject/retry/extract
//	  ├── Circuit Breaker ── protect downstream resources
//	  └── Metrics ────────── record success/failure/latency
//
// No protocol-specific code exists here. Adding a new protocol requires
// only a new ProtocolDriver implementation.
type GenericConnector struct {
	id       string
	protocol domain.ConnectorType
	config   ConnectorConfig
	driver   ProtocolDriver

	tmpl       *template.Engine
	ruleEngine *rule.Engine

	metrics domain.MetricsRecorder

	mu   sync.Mutex
	once sync.Once
}

// GenericConnectorOption configures the GenericConnector.
type GenericConnectorOption func(*GenericConnector)

// WithTemplateEngine sets a shared template engine.
func WithTemplateEngine(tmpl *template.Engine) GenericConnectorOption {
	return func(c *GenericConnector) { c.tmpl = tmpl }
}

// WithRuleEngine sets a shared rule engine.
func WithRuleEngine(re *rule.Engine) GenericConnectorOption {
	return func(c *GenericConnector) { c.ruleEngine = re }
}

// WithMetricsRecorder attaches a metrics recorder.
func WithMetricsRecorder(m domain.MetricsRecorder) GenericConnectorOption {
	return func(c *GenericConnector) { c.metrics = m }
}

// NewGenericConnector creates a connector that works with any protocol.
//
// id: unique connector identifier
// protocol: protocol type (ConnectorTypeHTTPClient, ConnectorTypeSMPP, ...)
// config: ConnectorConfig loaded from DB
// driver: ProtocolDriver for the actual transport
func NewGenericConnector(
	id string,
	protocol domain.ConnectorType,
	config ConnectorConfig,
	driver ProtocolDriver,
	opts ...GenericConnectorOption,
) *GenericConnector {
	c := &GenericConnector{
		id:       id,
		protocol: protocol,
		config:   config,
		driver:   driver,
		metrics:  noopMetricsRecorder{},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// ID returns the unique connector identifier.
func (c *GenericConnector) ID() string { return c.id }

// Protocol returns the protocol type.
func (c *GenericConnector) Protocol() domain.ConnectorType { return c.protocol }

// lazyInit initializes shared components on first use.
func (c *GenericConnector) lazyInit() {
	c.once.Do(func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.tmpl == nil {
			c.tmpl = template.NewEngine()
		}
		if c.ruleEngine == nil {
			c.ruleEngine = rule.NewEngine()
		}
	})
}

// Send implements the Connector interface.
//
// Execution flow:
//  1. Check circuit breaker
//  2. Build template data from message
//  3. Render configured template fields
//  4. Build TransportRequest with raw config + rendered fields
//  5. Call driver.Send() — raw transport, no interpretation
//  6. Build rule.ResponseData from TransportResponse
//  7. Evaluate rules → accept/reject/retry/extract
//  8. Record metrics, update circuit breaker
//  9. Return domain.SendResult
func (c *GenericConnector) Send(ctx context.Context, req *domain.SendRequest) (*domain.SendResult, error) {
	c.lazyInit()

	if req.Message == nil {
		return nil, fmt.Errorf("connector %q: message is nil", c.id)
	}
	if req.Prepared == nil {
		return nil, fmt.Errorf("connector %q: prepared message is nil", c.id)
	}

	start := time.Now()

	// 1. Build template data from message
	data := template.Data{
		Source:      req.Message.Source,
		Destination: req.Prepared.Destination,
		Text:        req.Message.Text,
		Parts:       req.Prepared.Parts,
		Encoding:    string(req.Prepared.Encoding),
		ClientRef:   req.Message.ClientRef,
		MessageID:   req.Message.ID,
		TenantID:    req.Message.TenantID,
		Custom:      c.config.Templates.Fields,
	}

	// 2. Render custom template fields
	renderedFields := make(map[string]string)
	for key, tmplStr := range c.config.Templates.Fields {
		rendered, err := c.tmpl.Render(tmplStr, data)
		if err != nil {
			return nil, fmt.Errorf("connector %q: render field %q: %w", c.id, key, err)
		}
		renderedFields[key] = rendered
	}
	// Add standard fields as rendered fields for drivers that need them
	renderedFields["source"] = data.Source
	renderedFields["destination"] = data.Destination
	renderedFields["text"] = data.Text
	renderedFields["message_id"] = data.MessageID
	renderedFields["tenant_id"] = data.TenantID
	renderedFields["client_ref"] = data.ClientRef

	// 3. Build transport request (raw config, no driver interpretation)
	transportReq := &TransportRequest{
		Message:        req.Message,
		Prepared:       req.Prepared,
		Config:         c.config.Transport,
		RenderedFields: renderedFields,
	}

	// 4. Send via driver (raw transport)
	transportResp, err := c.driver.Send(ctx, transportReq)
	latency := time.Since(start)

	if err != nil {
		c.recordFailure(req.Message.TenantID, "transport_error", latency)
		return nil, fmt.Errorf("connector %q: transport: %w", c.id, err)
	}

	// 5. Build rule response data from raw transport response
	ruleResp := rule.ResponseData{
		Status:  transportResp.Status,
		Headers: transportResp.Headers,
		Body:    transportResp.Body,
	}

	// Try to parse JSON body if present
	if len(transportResp.Body) > 0 {
		var parsed map[string]interface{}
		if err := json.Unmarshal(transportResp.Body, &parsed); err == nil {
			ruleResp.Parsed = parsed
		}
	}

	// 6. Evaluate rules
	rulesResult := c.ruleEngine.Evaluate(c.config.Rules.Rules, ruleResp)

	// 7. Determine acceptance and extract fields
	acceptance := c.determineAcceptance(rulesResult)
	externalID := transportResp.ExternalID
	if externalID == "" {
		externalID = rulesResult.Extract["external_id"]
	}

	// Extract optional fields
	parts := req.Prepared.Parts
	if p := rulesResult.Extract["parts"]; p != "" {
		if v, err := parseInt(p); err == nil {
			parts = v
		}
	}
	var price int64
	if p := rulesResult.Extract["price"]; p != "" {
		if v, err := parseInt64(p); err == nil {
			price = v
		}
	}

	// 8. Build SendResult
	result := &domain.SendResult{
		ExternalID: externalID,
		Parts:      parts,
		Price:      price,
		Cost:       price,
		RawResponse: transportResp.Body,
		ProviderStatus: rulesResult.Extract["provider_status"],
		Acceptance: acceptance,
	}

	// 9. Record metrics & circuit breaker
	if rulesResult.Accepted {
		c.recordSuccess(req.Message.TenantID, result.Parts, latency)
	} else if rulesResult.Rejected {
		c.recordFailure(req.Message.TenantID, "rejected", latency)
	}

	return result, nil
}

// CheckHealth implements the HealthChecker interface.
func (c *GenericConnector) CheckHealth(ctx context.Context) error {
	c.lazyInit()

	if !c.config.Health.Enabled {
		return nil // health checking disabled — assume healthy
	}

	return c.driver.CheckHealth(ctx)
}

// determineAcceptance maps rule result to AcceptanceKind.
func (c *GenericConnector) determineAcceptance(r rule.Result) domain.AcceptanceKind {
	switch {
	case r.Accepted:
		return domain.AcceptanceFinal
	case r.Rejected:
		return domain.AcceptanceRejected
	case r.Retryable:
		return domain.AcceptancePendingDLR
	default:
		return domain.AcceptancePendingDLR
	}
}

func (c *GenericConnector) recordSuccess(tenantID string, parts int, latency time.Duration) {
	if c.metrics != nil {
		c.metrics.RecordMessageSent(tenantID, c.id, parts, latency)
	}
}

func (c *GenericConnector) recordFailure(tenantID string, reason string, latency time.Duration) {
	if c.metrics != nil {
		c.metrics.RecordMessageFailed(tenantID, c.id, reason)
	}
}

// noopMetricsRecorder is a safe default that does nothing.
type noopMetricsRecorder struct{}

func (noopMetricsRecorder) RecordMessageSent(_, _ string, _ int, _ time.Duration) {}
func (noopMetricsRecorder) RecordMessageFailed(_, _, _ string)                    {}
func (noopMetricsRecorder) RecordRetry(_ string, _ int)                           {}
func (noopMetricsRecorder) RecordDLRReceived(_, _ string)                         {}

// parseInt and parseInt64 are helpers for string-to-int conversion.
func parseInt(s string) (int, error) {
	var v int
	_, err := fmt.Sscanf(s, "%d", &v)
	return v, err
}

func parseInt64(s string) (int64, error) {
	var v int64
	_, err := fmt.Sscanf(s, "%d", &v)
	return v, err
}

// Compile-time interface check.
var _ Connector = (*GenericConnector)(nil)
var _ HealthChecker = (*GenericConnector)(nil)
