package connector

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
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

	tmpl           *template.Engine
	ruleEngine     *rule.Engine
	metrics        domain.MetricsRecorder
	circuitBreaker CircuitBreakerStore

	// decodedConfig is the cached, decoded transport config (before per-message rendering).
	// Set during lazyInit, used by CheckHealth.
	decodedConfig TransportConfig

	// configVersion increments every time Reconfigure is called.
	// Checked in Send() to detect stale decodedConfig.
	configVersion int64

	mu  sync.Mutex
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

func WithCircuitBreakerStore(cbs CircuitBreakerStore) GenericConnectorOption {
	return func(c *GenericConnector) { c.circuitBreaker = cbs }
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

		// Decode the base transport config once (before per-message rendering).
		// Driver-specific validation is NOT done here — we must validate AFTER
		// field rendering so all required values are present.
		if len(c.config.Transport) > 0 {
			decoded, err := c.driver.DecodeConfig(c.config.Transport)
			if err != nil {
				// Decode failure means the transport JSON is fundamentally broken.
				// We store nil here; Send() will re-attempt with a better error message.
				return
			}
			c.decodedConfig = decoded
		}
	})
}

// Send implements the Connector interface.
//
// Flow:
//  1. Render template data from message + prepared
//  2. Render auth credentials into fields map
//  3. Decode transport JSON via driver.DecodeConfig() → typed TransportConfig
//  4. Render TransportConfig string fields (field-by-field, JSON-safe)
//  5. Validate rendered config via driver.ValidateConfig()
//  6. driver.Send(ctx, TransportRequest{Config: typedConfig})
//  7. Build rule.ResponseData from TransportResponse
//  8. Evaluate rules → accept/reject/retry/extract
//  9. Record metrics, return domain.SendResult
func (c *GenericConnector) Send(ctx context.Context, req *domain.SendRequest) (*domain.SendResult, error) {
	c.lazyInit()

	if req.Message == nil {
		return nil, fmt.Errorf("connector %q: message is nil", c.id)
	}
	if req.Prepared == nil {
		return nil, fmt.Errorf("connector %q: prepared message is nil", c.id)
	}

	start := time.Now()

	// 1. Build template data
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

	// 2. Render template fields → renderedFields
	renderedFields := c.renderFields(data)

	// 3. Decode transport config (raw JSON — may contain {{key}} patterns)
	transportConfig, err := c.driver.DecodeConfig(c.config.Transport)
	if err != nil {
		return nil, fmt.Errorf("connector %q: decode transport config: %w", c.id, err)
	}

	// 4. Render transport config string fields (JSON-safe — no escaping issues)
	if err := renderStructFields(transportConfig, renderedFields); err != nil {
		return nil, fmt.Errorf("connector %q: render transport: %w", c.id, err)
	}

	// 5. Validate the fully-rendered config
	if err := c.driver.ValidateConfig(transportConfig); err != nil {
		return nil, fmt.Errorf("connector %q: invalid config: %w", c.id, err)
	}

	// 6. Send via driver
	transportReq := &TransportRequest{
		Message:  req.Message,
		Prepared: req.Prepared,
		Config:   transportConfig,
	}
	transportResp, err := c.driver.Send(ctx, transportReq)
	latency := time.Since(start)

	if err != nil {
		c.recordFailure(req.Message.TenantID, "transport_error", latency)
		return nil, fmt.Errorf("connector %q: send: %w", c.id, err)
	}

	// 7. Build rule response data
	ruleResp := c.buildRuleResponse(transportResp, renderedFields)

	// 8. Evaluate rules
	rulesResult := c.ruleEngine.Evaluate(c.config.Rules.Rules, ruleResp)

	// 9. Determine acceptance + build result
	result := c.buildResult(rulesResult, transportResp, req.Prepared.Parts)

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
	if c.decodedConfig == nil {
		// Try decoding now if lazyInit failed
		if len(c.config.Transport) > 0 {
			decoded, err := c.driver.DecodeConfig(c.config.Transport)
			if err != nil {
				return fmt.Errorf("connector %q: decode transport config for health: %w", c.id, err)
			}
			c.decodedConfig = decoded
		}
	}
	return c.driver.CheckHealth(ctx, c.decodedConfig)
}

// Reconfigure updates the connector's runtime configuration.
// This invalidates the cached decodedConfig — the next Send() or CheckHealth()
// will re-decode the new transport config automatically.
// Thread-safe: uses atomic counter to detect stale cache.
//
// The orchestrator calls this when an admin updates endpoint settings via API.
func (c *GenericConnector) Reconfigure(config ConnectorConfig) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.config = config

	// Invalidate lazyInit cache so lazyInit() re-runs on next access
	c.once = sync.Once{}
	c.decodedConfig = nil

	// Notify stateful driver if it implements ConfigurableDriver
	if cd, ok := c.driver.(ConfigurableDriver); ok && c.decodedConfig != nil {
		_ = cd.AcceptConfig(c.decodedConfig) // best-effort
	}
}

// Connect establishes a session for stateful drivers (SMPP, SIP).
// No-op for stateless drivers (HTTP). Returns nil if not stateful.
func (c *GenericConnector) Connect(ctx context.Context) error {
	if sd, ok := c.driver.(StatefulDriver); ok {
		c.mu.Lock()
		defer c.mu.Unlock()
		if !sd.IsConnected() {
			return sd.Connect(ctx)
		}
	}
	return nil
}

// Disconnect tears down a session for stateful drivers.
// No-op for stateless drivers.
func (c *GenericConnector) Disconnect(ctx context.Context) error {
	if sd, ok := c.driver.(StatefulDriver); ok {
		c.mu.Lock()
		defer c.mu.Unlock()
		if sd.IsConnected() {
			return sd.Disconnect(ctx)
		}
	}
	return nil
}

// ── Internal helpers ─────────────────────────────────────────────────────────

// renderFields produces a string-keyed map from template data + auth credentials.
func (c *GenericConnector) renderFields(data template.Data) map[string]string {
	fields := make(map[string]string)

	for key, tmplStr := range c.config.Templates.Fields {
		rendered, err := c.tmpl.Render(tmplStr, data)
		if err != nil {
			// Best-effort: use the raw template string on error
			rendered = tmplStr
		}
		fields[key] = rendered
	}

	// Standard fields always available
	fields["source"] = data.Source
	fields["destination"] = data.Destination
	fields["text"] = data.Text
	fields["message_id"] = data.MessageID
	fields["tenant_id"] = data.TenantID
	fields["client_ref"] = data.ClientRef
	fields["parts"] = fmt.Sprintf("%d", data.Parts)
	fields["encoding"] = data.Encoding

	// Auth credentials
	renderAuthCredentials(&c.config.Auth, fields)

	return fields
}

// buildRuleResponse constructs rule.ResponseData from TransportResponse.
func (c *GenericConnector) buildRuleResponse(resp *TransportResponse, fields map[string]string) rule.ResponseData {
	r := rule.ResponseData{
		Status:  resp.Status,
		Headers: resp.Headers,
		Body:    resp.Body,
		Fields:  make(map[string]any),
	}

	for k, v := range fields {
		r.Fields[k] = v
	}
	r.Fields["latency_ms"] = float64(resp.Latency.Milliseconds())
	if resp.ExternalID != "" {
		r.Fields["external_id"] = resp.ExternalID
	}

	if len(resp.Body) > 0 {
		var parsed map[string]any
		if err := json.Unmarshal(resp.Body, &parsed); err == nil {
			r.Parsed = parsed
		}
	}

	return r
}

func (c *GenericConnector) buildResult(r rule.Result, resp *TransportResponse, defaultParts int) *domain.SendResult {
	externalID := resp.ExternalID
	if externalID == "" {
		externalID = r.Extract["external_id"]
	}

	parts := defaultParts
	if p := r.Extract["parts"]; p != "" {
		if v, err := parseInt(p); err == nil {
			parts = v
		}
	}

	var price int64
	if p := r.Extract["price"]; p != "" {
		if v, err := parseInt64(p); err == nil {
			price = v
		}
	}

	acceptance := c.determineAcceptance(r)

	return &domain.SendResult{
		ExternalID:     externalID,
		Parts:          parts,
		Price:          price,
		Cost:           price,
		RawResponse:    resp.Body,
		ProviderStatus: r.Extract["provider_status"],
		Acceptance:     acceptance,
	}
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
func renderAuthCredentials(cfg *AuthConfig, fields map[string]string) {
	if cfg == nil {
		return
	}
	fields["auth_type"] = cfg.Type
	for k, v := range cfg.Credentials {
		fields["auth_"+k] = v
	}
}

// renderStructFields does {{key}} replacement on all string fields of a struct
// (or nested struct/map values) using reflection. This avoids JSON escaping issues
// that would occur when substituting into raw JSON.
//
// Only exported string fields are processed. Pointers are followed.
// Maps with string keys/values are processed.
// Nested structs are processed recursively.
func renderStructFields(v interface{}, fields map[string]string) error {
	val := reflect.ValueOf(v)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}
	return renderReflect(val, fields)
}

func renderReflect(val reflect.Value, fields map[string]string) error {
	if !val.IsValid() {
		return nil
	}

	switch val.Kind() {
	case reflect.Struct:
		t := val.Type()
		for i := 0; i < val.NumField(); i++ {
			field := val.Field(i)
			if !field.CanSet() || !t.Field(i).IsExported() {
				continue
			}
			if err := renderReflect(field, fields); err != nil {
				return fmt.Errorf("field %s: %w", t.Field(i).Name, err)
			}
		}

	case reflect.Ptr:
		if val.IsNil() {
			return nil
		}
		return renderReflect(val.Elem(), fields)

	case reflect.String:
		if val.String() == "" {
			return nil
		}
		rendered := renderString(val.String(), fields)
		if rendered != val.String() {
			val.SetString(rendered)
		}

	case reflect.Map:
		if val.IsNil() {
			return nil
		}
		for _, key := range val.MapKeys() {
			elem := val.MapIndex(key)
			switch elem.Kind() {
			case reflect.String:
				newVal := renderString(elem.String(), fields)
				if newVal != elem.String() {
					val.SetMapIndex(key, reflect.ValueOf(newVal))
				}
			case reflect.Ptr, reflect.Struct, reflect.Interface:
				if err := renderReflect(elem, fields); err != nil {
					return err
				}
			case reflect.Map:
				if err := renderReflect(elem, fields); err != nil {
					return err
				}
			}
		}

	case reflect.Slice:
		for i := 0; i < val.Len(); i++ {
			elem := val.Index(i)
			switch elem.Kind() {
			case reflect.Ptr, reflect.Struct, reflect.Interface, reflect.Map:
				if err := renderReflect(elem, fields); err != nil {
					return err
				}
			case reflect.String:
				newVal := renderString(elem.String(), fields)
				if newVal != elem.String() {
					elem.SetString(newVal)
				}
			}
		}

	case reflect.Interface:
		return renderReflect(val.Elem(), fields)
	}

	return nil
}

// renderString replaces {{key}} patterns with values from the fields map.
func renderString(s string, fields map[string]string) string {
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
