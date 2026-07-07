// Package httpdriver provides an HTTP ProtocolDriver implementation.
//
// The driver is only responsible for transport-level HTTP communication:
//  1. Decode protocol-specific transport config from raw JSON
//  2. Build *http.Request using rendered fields
//  3. Apply HTTP authentication
//  4. Send via pooled HTTP client
//  5. Return raw TransportResponse (status, headers, body)
//
// The driver does NOT evaluate rules, render templates, or make
// business decisions — that is the GenericConnector's responsibility.
package httpdriver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/raghna/fury-sms-gateway/internal/connector"
	"github.com/raghna/fury-sms-gateway/internal/domain"
)

// Driver implements connector.ProtocolDriver for HTTP.
// Stateless and thread-safe — one instance can serve many connectors.
type Driver struct {
	client     *http.Client
	clientOnce sync.Once
}

// TransportConfig is the protocol-specific transport configuration for HTTP.
// Decoded from ConnectorConfig.Transport (json.RawMessage).
type TransportConfig struct {
	// URL template: "https://api.example.com/send"
	URL string `json:"url"`

	// HTTP method: POST, GET, PUT, DELETE, PATCH
	Method string `json:"method"`

	// Headers is a list of key-value pairs.
	// Values reference {{field_name}} from rendered fields.
	Headers []KeyValue `json:"headers,omitempty"`

	// QueryParams is a list of query string key-value pairs.
	QueryParams []KeyValue `json:"query_params,omitempty"`

	// Body template (optional for GET).
	Body *BodyConfig `json:"body,omitempty"`
}

// KeyValue is a key-value pair.
type KeyValue struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// BodyConfig describes the request body.
type BodyConfig struct {
	// Template uses {{field_name}} references from rendered fields.
	Template string `json:"template"`

	// Content-Type header value.
	ContentType string `json:"content_type"`
}

// NewDriver creates an HTTP protocol driver.
func NewDriver() *Driver {
	return &Driver{}
}

// Protocol returns the protocol identifier.
func (d *Driver) Protocol() domain.ConnectorType {
	return domain.ConnectorTypeHTTPClient
}

// lazyClient initializes the HTTP client on first use.
func (d *Driver) lazyClient() *http.Client {
	d.clientOnce.Do(func() {
		d.client = &http.Client{
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
		}
	})
	return d.client
}

// Send implements connector.ProtocolDriver.
//
// Flow:
//  1. Decode TransportConfig from req.Config
//  2. Build URL, headers, body using rendered fields
//  3. Apply auth
//  4. Send via HTTP client
//  5. Return raw TransportResponse
func (d *Driver) Send(ctx context.Context, req *connector.TransportRequest) (*connector.TransportResponse, error) {
	// 1. Decode transport config
	tc, err := decodeTransportConfig(req.Config)
	if err != nil {
		return nil, fmt.Errorf("http driver: decode config: %w", err)
	}

	// 2. Build URL
	rawURL := render(tc.URL, req.RenderedFields)
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("http driver: parse URL %q: %w", rawURL, err)
	}
	if parsedURL.Host == "" && strings.Contains(rawURL, "://") {
		return nil, fmt.Errorf("http driver: invalid URL %q (no host)", rawURL)
	}

	// 3. Determine method
	method := tc.Method
	if method == "" {
		method = http.MethodPost
	}

	// 4. Build body
	var bodyReader io.Reader
	if tc.Body != nil && tc.Body.Template != "" {
		bodyBytes := []byte(render(tc.Body.Template, req.RenderedFields))
		bodyReader = bytes.NewReader(bodyBytes)
	}

	// 5. Create request
	httpReq, err := http.NewRequestWithContext(ctx, method, parsedURL.String(), bodyReader)
	if err != nil {
		return nil, fmt.Errorf("http driver: create request: %w", err)
	}

	// 6. Set content type
	if tc.Body != nil && tc.Body.ContentType != "" {
		httpReq.Header.Set("Content-Type", tc.Body.ContentType)
	}

	// 7. Set headers
	for _, h := range tc.Headers {
		httpReq.Header.Set(render(h.Key, req.RenderedFields), render(h.Value, req.RenderedFields))
	}

	// 8. Add query parameters
	if len(tc.QueryParams) > 0 {
		q := httpReq.URL.Query()
		for _, p := range tc.QueryParams {
			q.Add(render(p.Key, req.RenderedFields), render(p.Value, req.RenderedFields))
		}
		httpReq.URL.RawQuery = q.Encode()
	}

	// 9. Apply authentication
	if err := applyAuth(httpReq, req); err != nil {
		return nil, fmt.Errorf("http driver: auth: %w", err)
	}

	// 10. Send
	start := time.Now()
	client := d.lazyClient()
	resp, err := client.Do(httpReq)
	latency := time.Since(start)

	if err != nil {
		return nil, fmt.Errorf("http driver: send: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("http driver: read body: %w", err)
	}

	// 11. Extract external ID from headers (X-Message-Id, X-Request-Id) or body
	headers := make(map[string]string)
	for k := range resp.Header {
		headers[strings.ToLower(k)] = resp.Header.Get(k)
	}

	return &connector.TransportResponse{
		Status:     resp.StatusCode,
		Headers:    headers,
		Body:       body,
		ExternalID: extractExternalID(resp, body),
		Latency:    latency,
	}, nil
}

// CheckHealth implements connector.ProtocolDriver.
func (d *Driver) CheckHealth(ctx context.Context) error {
	// HTTP health check is a simple GET to a configured endpoint.
	// The health config and rule evaluation is handled by GenericConnector.
	// This method is for the driver's own health (connection pool, etc.).
	return nil // client pool is always healthy unless OOM
}

// decodeTransportConfig unmarshals HTTP transport config from raw JSON.
func decodeTransportConfig(data []byte) (*TransportConfig, error) {
	if len(data) == 0 {
		return &TransportConfig{Method: "POST"}, nil
	}
	var tc TransportConfig
	if err := json.Unmarshal(data, &tc); err != nil {
		return nil, fmt.Errorf("unmarshal http transport config: %w", err)
	}
	if tc.Method == "" {
		tc.Method = "POST"
	}
	return &tc, nil
}

// render substitutes {{key}} references with values from the rendered fields map.
// This is a simple string replacement — not a full template engine.
// Full template rendering is done by GenericConnector before calling the driver.
func render(s string, fields map[string]string) string {
	if !strings.Contains(s, "{{") {
		return s
	}
	for k, v := range fields {
		old := "{{" + k + "}}"
		s = strings.ReplaceAll(s, old, v)
	}
	return s
}

// extractExternalID looks for a provider message ID in response headers or body.
func extractExternalID(resp *http.Response, body []byte) string {
	// Check common headers
	for _, h := range []string{"X-Message-Id", "X-Request-Id", "Message-Id"} {
		if v := resp.Header.Get(h); v != "" {
			return v
		}
	}

	// Try to extract "message_id" from JSON body
	if len(body) > 0 {
		var parsed map[string]interface{}
		if err := json.Unmarshal(body, &parsed); err == nil {
			if id, ok := parsed["message_id"].(string); ok {
				return id
			}
			if id, ok := parsed["id"].(string); ok {
				return id
			}
		}
	}

	return ""
}
