package connector

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"text/template"
	"time"

	"github.com/raghna/fury-sms-gateway/internal/domain"
)

// HTTPSender implements domain.Sender for HTTP-based connectors.
type HTTPSender struct {
	connectorType domain.ConnectorType
}

func NewHTTPSender() *HTTPSender {
	return &HTTPSender{connectorType: domain.ConnectorTypeHTTPClient}
}

func (s *HTTPSender) Type() domain.ConnectorType {
	return s.connectorType
}

func (s *HTTPSender) Send(ctx context.Context, req domain.SendRequest) (*domain.SendResult, error) {
	if req.Connector == nil {
		return nil, fmt.Errorf("connector config is nil")
	}

	// Parse connector config
	cfg, err := parseHTTPConfig(req.Connector.Config)
	if err != nil {
		return nil, fmt.Errorf("parse connector config: %w", err)
	}

	// Build request body from template
	body, err := s.buildBody(cfg.BodyTemplate, req.Message)
	if err != nil {
		return nil, fmt.Errorf("build request body: %w", err)
	}

	// Determine timeout
	timeout := req.Timeout
	if timeout <= 0 {
		timeout = time.Duration(cfg.TimeoutSec) * time.Second
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
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

	// Send
	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http send: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
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
		// Default: accept 200/202
		success = resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusAccepted
	}

	if !success {
		return nil, fmt.Errorf("provider returned status %d: %s", resp.StatusCode, truncateString(string(respBody), 200))
	}

	// Extract external ID from response
	externalID := extractField(respBody, cfg.ExternalIDPath)
	providerStatus := extractField(respBody, cfg.StatusPath)

	// Calculate price/cost (approximated by parts)
	parts := countSMSParts(req.Message.Text, req.Message.Encoding)
	price := int64(parts) * 5000 // placeholder: 0.05 per part, override in production

	return &domain.SendResult{
		ExternalID:     externalID,
		Parts:          parts,
		Price:          price,
		Cost:           price, // simplified: cost = price for now
		RawResponse:    respBody,
		ProviderStatus: providerStatus,
	}, nil
}

func (s *HTTPSender) buildBody(tmpl string, msg *domain.Message) ([]byte, error) {
	if tmpl == "" {
		// Default JSON body
		return json.Marshal(map[string]interface{}{
			"source":      msg.Source,
			"destination": msg.Destination,
			"text":        msg.Text,
			"encoding":    msg.Encoding,
			"client_ref":  msg.ClientRef,
		})
	}

	t, err := template.New("body").Parse(tmpl)
	if err != nil {
		return nil, fmt.Errorf("parse template: %w", err)
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
	if err := t.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("execute template: %w", err)
	}
	return buf.Bytes(), nil
}

// parseHTTPConfig unmarshals the connector config JSONB.
func parseHTTPConfig(data []byte) (*domain.HTTPConnectorConfig, error) {
	if len(data) == 0 {
		// Return defaults
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

// extractField extracts a field from a JSON response using a simple path.
// Supports simple dot-notation paths: "data.id" or "message_id".
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
// GSM-7: 160 chars/part (153 for concatenated)
// UCS-2: 70 chars/part (67 for concatenated)
func countSMSParts(text string, encoding domain.Encoding) int {
	if len(text) == 0 {
		return 1
	}

	var maxPerPart, maxConcat int
	if encoding == domain.EncodingUCS2 {
		maxPerPart = 70
		maxConcat = 67
	} else {
		maxPerPart = 160
		maxConcat = 153
	}

	if len(text) <= maxPerPart {
		return 1
	}

	parts := (len(text) + maxConcat - 1) / maxConcat
	return parts
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// ensure interfaces are satisfied
var _ domain.Sender = (*HTTPSender)(nil)
