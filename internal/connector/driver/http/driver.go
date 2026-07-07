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
// Stateless and thread-safe — one instance serves all HTTP connectors.
type Driver struct {
	client     *http.Client
	clientOnce sync.Once
}

// HTTPTransportConfig is the typed transport config for HTTP.
// All templates are rendered BEFORE DecodeConfig — values are final.
type HTTPTransportConfig struct {
	URL     string            `json:"url"`
	Method  string            `json:"method"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body,omitempty"`
}

func (c *HTTPTransportConfig) Protocol() domain.ConnectorType {
	return domain.ConnectorTypeHTTPClient
}

func NewDriver() *Driver {
	return &Driver{}
}

func (d *Driver) Protocol() domain.ConnectorType {
	return domain.ConnectorTypeHTTPClient
}

func (d *Driver) DecodeConfig(data []byte) (connector.TransportConfig, error) {
	if len(data) == 0 {
		return &HTTPTransportConfig{Method: "POST"}, nil
	}
	var cfg HTTPTransportConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("http driver: decode config: %w", err)
	}
	if cfg.Method == "" {
		cfg.Method = "POST"
	}
	if cfg.Headers == nil {
		cfg.Headers = make(map[string]string)
	}
	return &cfg, nil
}

func (d *Driver) ValidateConfig(cfg connector.TransportConfig) error {
	tc, ok := cfg.(*HTTPTransportConfig)
	if !ok {
		return fmt.Errorf("http driver: expected *HTTPTransportConfig, got %T", cfg)
	}
	if tc.URL == "" {
		return fmt.Errorf("http driver: URL is required")
	}
	parsed, err := url.Parse(tc.URL)
	if err != nil {
		return fmt.Errorf("http driver: invalid URL %q: %w", tc.URL, err)
	}
	if parsed.Host == "" {
		return fmt.Errorf("http driver: URL %q has no host", tc.URL)
	}
	switch tc.Method {
	case "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS":
		// valid
	default:
		return fmt.Errorf("http driver: unsupported method %q", tc.Method)
	}
	return nil
}

func (d *Driver) Send(ctx context.Context, req *connector.TransportRequest) (*connector.TransportResponse, error) {
	tc, ok := req.Config.(*HTTPTransportConfig)
	if !ok {
		return nil, fmt.Errorf("http driver: expected *HTTPTransportConfig, got %T", req.Config)
	}

	parsedURL, err := url.Parse(tc.URL)
	if err != nil {
		return nil, fmt.Errorf("http driver: parse URL %q: %w", tc.URL, err)
	}

	var bodyReader io.Reader
	if tc.Body != "" {
		bodyReader = bytes.NewReader([]byte(tc.Body))
	}

	httpReq, err := http.NewRequestWithContext(ctx, tc.Method, parsedURL.String(), bodyReader)
	if err != nil {
		return nil, fmt.Errorf("http driver: create request: %w", err)
	}

	for k, v := range tc.Headers {
		httpReq.Header.Set(k, v)
	}

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

func (d *Driver) CheckHealth(ctx context.Context, cfg connector.TransportConfig) error {
	if cfg == nil {
		return nil
	}
	// HTTP: just validate the config — no active connection needed
	return d.ValidateConfig(cfg)
}

func (d *Driver) lazyClient() *http.Client {
	d.clientOnce.Do(func() {
		d.client = &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxConnsPerHost:     20,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
				ForceAttemptHTTP2:   true,
			},
		}
	})
	return d.client
}

func extractExternalID(resp *http.Response, body []byte) string {
	for _, h := range []string{"X-Message-Id", "X-Request-Id", "Message-Id"} {
		if v := resp.Header.Get(h); v != "" {
			return v
		}
	}
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

var _ connector.ProtocolDriver = (*Driver)(nil)
