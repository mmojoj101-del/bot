package connector

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/raghna/fury-sms-gateway/internal/domain"
)

// HTTPSender implements domain.Sender for HTTP-based connectors.
// Uses a singleton http.Client with connection pooling, template cache,
// and optional per-connector circuit breaker integration.
type HTTPSender struct {
	connectorType  domain.ConnectorType
	client         *http.Client
	tmplCache      sync.Map // map[templateString]*template.Template
	circuitBreakers CircuitBreakerStore

	extractSource string // JSON dot-path for external ID
	statusPath    string // JSON dot-path for provider status
}

// HTTPSenderOption configures an HTTPSender.
type HTTPSenderOption func(*HTTPSender)

// WithCircuitBreakerStore attaches a shared circuit breaker store.
// The sender checks Allow() before each request and records Success()/Failure().
func WithCircuitBreakerStore(cbs CircuitBreakerStore) HTTPSenderOption {
	return func(s *HTTPSender) { s.circuitBreakers = cbs }
}

// NewHTTPSender creates an HTTP sender with connection pooling.
func NewHTTPSender(opts ...HTTPSenderOption) *HTTPSender {
	s := &HTTPSender{
		connectorType: domain.ConnectorTypeHTTPClient,
		client: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:          100,
				MaxConnsPerHost:       20,
				MaxIdleConnsPerHost:   10,
				IdleConnTimeout:       90 * time.Second,
				TLSHandshakeTimeout:   10 * time.Second,
				ResponseHeaderTimeout: 15 * time.Second,
				ExpectContinueTimeout: 1 * time.Second,
				ForceAttemptHTTP2:     true,
			},
		},
		circuitBreakers: noopCircuitBreakerStore{}, // safe default — no-op
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *HTTPSender) Type() domain.ConnectorType {
	return s.connectorType
}

func (s *HTTPSender) Send(ctx context.Context, req domain.SendRequest) (*domain.SendResult, error) {
	if req.Connector == nil {
		return nil, fmt.Errorf("connector config is nil")
	}

	// ── Circuit breaker check ──────────────────────────────────────
	if !s.circuitBreakers.Allow(req.Connector.ID) {
		return nil, fmt.Errorf("circuit breaker open for connector %s", req.Connector.ID)
	}

	// Parse connector config
	cfg, err := parseHTTPConfig(req.Connector.Config)
	if err != nil {
		return nil, fmt.Errorf("parse connector config: %w", err)
	}

	// Build request body from cached template
	body, err := s.buildBody(cfg.BodyTemplate, req.Message)
	if err != nil {
		return nil, fmt.Errorf("build request body: %w", err)
	}

	// Build HTTP request
	method := cfg.Method
	if method == "" {
		method = http.MethodPost
	}

	httpReq, err := http.NewRequestWithContext(ctx, method, cfg.URL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	// Set headers
	contentType := cfg.ContentType
	if contentType == "" {
		contentType = "application/json"
	}
	httpReq.Header.Set("Content-Type", contentType)
	for k, v := range cfg.Headers {
		httpReq.Header.Set(k, v)
	}

	// Set auth
	switch cfg.AuthType {
	case "bearer":
		httpReq.Header.Set("Authorization", "Bearer "+cfg.AuthToken)
	case "basic":
		httpReq.SetBasicAuth(cfg.AuthUsername, cfg.AuthPassword)
	case "api_key":
		httpReq.Header.Set("X-API-Key", cfg.AuthToken)
	}

	// Send using the pooled client
	resp, err := s.client.Do(httpReq)
	if err != nil {
		s.circuitBreakers.Failure(req.Connector.ID)
		return nil, fmt.Errorf("http send: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		s.circuitBreakers.Failure(req.Connector.ID)
		return nil, fmt.Errorf("read response: %w", err)
	}

	// Check success codes
	success := false
	for _, code := range cfg.SuccessCodes {
		if resp.StatusCode == code {
			success = true
			break
		}
	}
	if len(cfg.SuccessCodes) == 0 {
		success = resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusAccepted
	}

	if !success {
		// Check for Retry-After header
		retryAfter := resp.Header.Get("Retry-After")
		if retryAfter != "" {
			return nil, &ProviderRetryRequiredError{
				StatusCode: resp.StatusCode,
				RetryAfter: retryAfter,
				Body:       truncateString(string(respBody), 200),
			}
		}
		// Non-retryable failure — record circuit breaker failure
		if resp.StatusCode >= 500 || resp.StatusCode == 429 {
			s.circuitBreakers.Failure(req.Connector.ID)
		}
		return nil, fmt.Errorf("provider returned status %d: %s", resp.StatusCode, truncateString(string(respBody), 200))
	}

	// Success — record circuit breaker success
	s.circuitBreakers.Success(req.Connector.ID)

	// Extract external ID from response
	externalID := extractField(respBody, cfg.ExternalIDPath)
	providerStatus := extractField(respBody, cfg.StatusPath)

	// Calculate parts
	parts := countSMSParts(req.Message.Text, req.Message.Encoding)
	price := int64(parts) * 5000 // placeholder: 0.05 per part

	return &domain.SendResult{
		ExternalID:     externalID,
		Parts:          parts,
		Price:          price,
		Cost:           price,
		RawResponse:    respBody,
		ProviderStatus: providerStatus,
	}, nil
}

// buildBody uses a cached template to avoid re-parsing on every send.
func (s *HTTPSender) buildBody(tmpl string, msg *domain.Message) ([]byte, error) {
	if tmpl == "" {
		return json.Marshal(map[string]interface{}{
			"source":      msg.Source,
			"destination": msg.Destination,
			"text":        msg.Text,
			"encoding":    msg.Encoding,
			"client_ref":  msg.ClientRef,
		})
	}

	// Cache the parsed template
	t, ok := s.tmplCache.Load(tmpl)
	if !ok {
		parsed, err := template.New("body").Parse(tmpl)
		if err != nil {
			return nil, fmt.Errorf("parse template: %w", err)
		}
		t = parsed
		s.tmplCache.Store(tmpl, parsed)
	}

	data := map[string]interface{}{
		"Source":      msg.Source,
		"Destination": msg.Destination,
		"Text":        msg.Text,
		"Parts":       countSMSParts(msg.Text, msg.Encoding),
		"Encoding":    msg.Encoding,
		"ClientRef":   msg.ClientRef,
		"MessageID":   msg.ID,
		"TenantID":    msg.TenantID,
	}

	var buf bytes.Buffer
	if err := t.(*template.Template).Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("execute template: %w", err)
	}
	return buf.Bytes(), nil
}

// ProviderRetryRequiredError indicates the provider asked us to retry after a delay.
type ProviderRetryRequiredError struct {
	StatusCode int
	RetryAfter string // seconds or HTTP-date
	Body       string
}

func (e *ProviderRetryRequiredError) Error() string {
	return fmt.Sprintf("provider %d: retry after %s", e.StatusCode, e.RetryAfter)
}

// parseHTTPConfig unmarshals the connector config JSONB.
func parseHTTPConfig(data []byte) (*domain.HTTPConnectorConfig, error) {
	if len(data) == 0 {
		return &domain.HTTPConnectorConfig{
			Method:       "POST",
			ContentType:  "application/json",
			TimeoutSec:   30,
			SuccessCodes: []int{200, 202},
		}, nil
	}
	var cfg domain.HTTPConnectorConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("unmarshal http config: %w", err)
	}
	if cfg.TimeoutSec <= 0 {
		cfg.TimeoutSec = 30
	}
	if cfg.Method == "" {
		cfg.Method = "POST"
	}
	if len(cfg.SuccessCodes) == 0 {
		cfg.SuccessCodes = []int{200, 202}
	}
	return &cfg, nil
}

// extractField extracts a field from a JSON response using dot-notation path.
func extractField(body []byte, path string) string {
	if path == "" || len(body) == 0 {
		return ""
	}

	var data interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return ""
	}

	parts := strings.Split(path, ".")
	current := data
	for _, part := range parts {
		m, ok := current.(map[string]interface{})
		if !ok {
			return ""
		}
		current, ok = m[part]
		if !ok {
			return ""
		}
	}

	switch v := current.(type) {
	case string:
		return v
	case float64:
		return fmt.Sprintf("%.0f", v)
	default:
		b, _ := json.Marshal(v)
		return string(b)
	}
}

// countSMSParts estimates the number of SMS parts for a message.
func countSMSParts(text string, encoding domain.Encoding) int {
	if len(text) == 0 {
		return 1
	}

	maxPerPart := 160
	maxConcat := 153
	if encoding == domain.EncodingUCS2 {
		maxPerPart = 70
		maxConcat = 67
	}

	if len(text) <= maxPerPart {
		return 1
	}
	return (len(text) + maxConcat - 1) / maxConcat
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

var _ domain.Sender = (*HTTPSender)(nil)
