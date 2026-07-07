package httpconnector

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/raghna/fury-sms-gateway/internal/template"
)

// BuildRequest constructs an *http.Request from EndpointConfig and message data.
//
// Steps:
//  1. Render URL template (supports query params in URL or via config)
//  2. Add query parameters from config
//  3. Render and set headers
//  4. Render body (if configured)
//  5. Apply authentication
//
// Returns a ready-to-send *http.Request with context set.
func BuildRequest(
	cfg *EndpointConfig,
	data template.Data,
	tmpl *template.Engine,
) (*http.Request, error) {
	// 1. Render URL
	rawURL, err := tmpl.Render(cfg.Request.URL, data)
	if err != nil {
		return nil, fmt.Errorf("render URL: %w", err)
	}

	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("parse URL %q: %w", rawURL, err)
	}

	// Handle special case: if URL has a scheme but no host, url.Parse misparses
	if parsedURL.Host == "" && strings.Contains(rawURL, "://") {
		return nil, fmt.Errorf("invalid URL: %q (no host)", rawURL)
	}

	// 2. Determine method
	method := cfg.Request.Method
	if method == "" {
		method = http.MethodPost
	}

	// 3. Build body
	var bodyReader io.Reader
	if cfg.Request.Body != nil && cfg.Request.Body.Template != "" {
		bodyBytes, err := tmpl.RenderBytes(cfg.Request.Body.Template, data)
		if err != nil {
			return nil, fmt.Errorf("render body: %w", err)
		}
		bodyReader = bytes.NewReader(bodyBytes)
	}

	// 4. Create request
	req, err := http.NewRequest(method, parsedURL.String(), bodyReader)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	// 5. Set content type from body config
	if cfg.Request.Body != nil && cfg.Request.Body.ContentType != "" {
		req.Header.Set("Content-Type", cfg.Request.Body.ContentType)
	}

	// 6. Render and set headers
	for _, h := range cfg.Request.Headers {
		key, err := tmpl.Render(h.Key, data)
		if err != nil {
			return nil, fmt.Errorf("render header key %q: %w", h.Key, err)
		}
		value, err := tmpl.Render(h.Value, data)
		if err != nil {
			return nil, fmt.Errorf("render header value %q: %w", h.Value, err)
		}
		req.Header.Set(key, value)
	}

	// 7. Add query parameters from config
	if len(cfg.Request.QueryParams) > 0 {
		q := req.URL.Query()
		for _, p := range cfg.Request.QueryParams {
			key, err := tmpl.Render(p.Key, data)
			if err != nil {
				return nil, fmt.Errorf("render query param key %q: %w", p.Key, err)
			}
			value, err := tmpl.Render(p.Value, data)
			if err != nil {
				return nil, fmt.Errorf("render query param value %q: %w", p.Value, err)
			}
			q.Add(key, value)
		}
		req.URL.RawQuery = q.Encode()
	}

	// 8. Apply authentication
	if err := applyAuth(req, cfg.Auth); err != nil {
		return nil, fmt.Errorf("apply auth: %w", err)
	}

	return req, nil
}
