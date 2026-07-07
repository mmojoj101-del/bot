package connector

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/raghna/fury-sms-gateway/internal/connector/rule"
	"github.com/raghna/fury-sms-gateway/internal/domain"
	"github.com/raghna/fury-sms-gateway/internal/template"
)

// GenericConnector is the protocol-agnostic connector implementation.
// It implements Connector + HealthChecker by orchestrating:
//   - Template Engine (render message fields into transport config)
//   - Auth (render credentials into transport config)
//   - ProtocolDriver (raw transport, no business logic)
//   - Rule Engine (evaluate transport response)
//   - Circuit Breaker (protect downstream)
//   - Metrics (record outcomes)
//
// The GenericConnector has zero knowledge of HTTP, SMPP, SIP, or any protocol.
type GenericConnector struct {
	id       string
	protocol domain.ConnectorType
	config   ConnectorConfig
	driver   ProtocolDriver

	tmpl       *template.Engine
	ruleEngine *rule.Engine
	metrics    domain.MetricsRecorder

	mu   sync.Mutex
	once sync.Once
}

type GenericConnectorOption func(*GenericConnector)

func WithTemplateEngine(tmpl *template.Engine) GenericConnectorOption {
	return func(c *GenericConnector) { c.tmpl = tmpl }
}

func WithRuleEngine(re *rule.Engine) GenericConnectorOption {
	return func(c *GenericConnector) { c.ruleEngine = re }
}

func WithMetricsRecorder(m domain.MetricsRecorder) GenericConnectorOption {
	return func(c *GenericConnector) { c.metrics = m }
}

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

func (c *GenericConnector) ID() string                   { return c.id }
func (c *GenericConnector) Protocol() domain.ConnectorType { return c.protocol }

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
// Flow:
//  1. Render template data from message + prepared
//  2. Render auth credentials into fields map
//  3. Render transport JSON (replace {{key}} with field values)
//  4. Decode rendered JSON via driver.DecodeConfig() → typed TransportConfig
//  5. driver.Send(ctx, TransportRequest{Config: typedConfig})
//  6. Build rule.ResponseData from TransportResponse
//  7. Evaluate rules → accept/reject/retry/extract
//  8. Record metrics, return domain.SendResult
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
	// Standard fields always available
	renderedFields["source"] = data.Source
	renderedFields["destination"] = data.Destination
	renderedFields["text"] = data.Text
	renderedFields["message_id"] = data.MessageID
	renderedFields["tenant_id"] = data.TenantID
	renderedFields["client_ref"] = data.ClientRef
	renderedFields["parts"] = fmt.Sprintf("%d", data.Parts)
	renderedFields["encoding"] = data.Encoding

	// 3. Render auth credentials into fields map
	renderAuthCredentials(&c.config.Auth, renderedFields)

	// 4. Render transport JSON: replace {{key}} with rendered values
	renderedTransport := renderJSON(string(c.config.Transport), renderedFields)

	// 5. Decode via driver
	transportConfig, err := c.driver.DecodeConfig([]byte(renderedTransport))
	if err != nil {
		return nil, fmt.Errorf("connector %q: decode transport config: %w", c.id, err)
	}

	// 6. Build TransportRequest (no RenderedFields, no raw config — just typed config)
	transportReq := &TransportRequest{
		Message:  req.Message,
		Prepared: req.Prepared,
		Config:   transportConfig,
	}

	// 7. Send via driver (raw transport, no interpretation)
	transportResp, err := c.driver.Send(ctx, transportReq)
	latency := time.Since(start)

	if err != nil {
		c.recordFailure(req.Message.TenantID, "transport_error", latency)
		return nil, fmt.Errorf("connector %q: send: %w", c.id, err)
	}

	// 8. Build rule response data
	ruleResp := c.buildRuleResponse(transportResp, renderedFields)

	// 9. Evaluate rules
	rulesResult := c.ruleEngine.Evaluate(c.config.Rules.Rules, ruleResp)

	// 10. Determine acceptance
	acceptance := c.determineAcceptance(rulesResult)
	externalID := transportResp.ExternalID
	if externalID == "" {
		externalID = rulesResult.Extract["external_id"]
	}

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

	result := &domain.SendResult{
		ExternalID: externalID,
		Parts:      parts,
		Price:      price,
		Cost:       price,
		RawResponse: transportResp.Body,
		ProviderStatus: rulesResult.Extract["provider_status"],
		Acceptance: acceptance,
	}

	if rulesResult.Accepted {
		c.recordSuccess(req.Message.TenantID, result.Parts, latency)
	} else if rulesResult.Rejected {
		c.recordFailure(req.Message.TenantID, "rejected", latency)
	}

	return result, nil
}

func (c *GenericConnector) CheckHealth(ctx context.Context) error {
	c.lazyInit()
	if !c.config.Health.Enabled {
		return nil
	}
	return c.driver.CheckHealth(ctx)
}

// buildRuleResponse constructs rule.ResponseData from TransportResponse.
func (c *GenericConnector) buildRuleResponse(resp *TransportResponse, fields map[string]string) rule.ResponseData {
	r := rule.ResponseData{
		Status:  resp.Status,
		Headers: resp.Headers,
		Body:    resp.Body,
		Fields:  make(map[string]string),
	}

	// Copy all rendered fields for rule access
	for k, v := range fields {
		r.Fields[k] = v
	}
	// Add transport metadata
	r.Fields["latency_ms"] = fmt.Sprintf("%d", resp.Latency.Milliseconds())
	if resp.ExternalID != "" {
		r.Fields["external_id"] = resp.ExternalID
	}

	// Try JSON parse
	if len(resp.Body) > 0 {
		var parsed map[string]interface{}
		if err := json.Unmarshal(resp.Body, &parsed); err == nil {
			r.Parsed = parsed
		}
	}

	return r
}

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

// renderAuthCredentials renders auth config into the fields map as template variables.
// The transport config references these with {{auth_token}}, {{auth_username}}, etc.
func renderAuthCredentials(cfg *AuthConfig, fields map[string]string) {
	if cfg == nil {
		return
	}
	fields["auth_type"] = cfg.Type
	for k, v := range cfg.Credentials {
		fields["auth_"+k] = v
	}
}

// renderJSON replaces {{key}} patterns with values from the fields map.
// This is a simple string replacement — NOT a full template engine.
// Full Go template rendering is done by template.Engine for template fields.
func renderJSON(s string, fields map[string]string) string {
	if !strings.Contains(s, "{{") {
		return s
	}
	for k, v := range fields {
		s = strings.ReplaceAll(s, "{{"+k+"}}", v)
	}
	return s
}

// noopMetricsRecorder is a safe default.
type noopMetricsRecorder struct{}

func (noopMetricsRecorder) RecordMessageSent(_, _ string, _ int, _ time.Duration) {}
func (noopMetricsRecorder) RecordMessageFailed(_, _, _ string)                    {}
func (noopMetricsRecorder) RecordRetry(_ string, _ int)                           {}
func (noopMetricsRecorder) RecordDLRReceived(_, _ string)                         {}

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

var _ Connector = (*GenericConnector)(nil)
var _ HealthChecker = (*GenericConnector)(nil)
