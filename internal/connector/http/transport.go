package httpconnector

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// DefaultTransport creates a shared http.Transport with connection pooling.
// Tuned for high-throughput SMS gateways (many concurrent requests to the same host).
func DefaultTransport() *http.Transport {
	return &http.Transport{
		MaxIdleConns:          100,
		MaxConnsPerHost:       20,
		MaxIdleConnsPerHost:   10,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ForceAttemptHTTP2:     true,
	}
}

// Client wraps *http.Client with retry support for the HTTP connector.
type Client struct {
	inner *http.Client
}

// NewClient creates an HTTP client with configurable timeout.
func NewClient(timeout time.Duration) *Client {
	return &Client{
		inner: &http.Client{
			Timeout:   timeout,
			Transport: DefaultTransport(),
		},
	}
}

// NewClientWithTransport creates a client with a custom transport.
func NewClientWithTransport(inner *http.Client) *Client {
	return &Client{inner: inner}
}

// Do sends an HTTP request and returns the raw response.
func (c *Client) Do(ctx context.Context, req *http.Request) (*http.Response, error) {
	req = req.WithContext(ctx)
	resp, err := c.inner.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	return resp, nil
}

// DoAndRead sends a request and reads the full response body.
// The response body is automatically closed after reading.
func (c *Client) DoAndRead(ctx context.Context, req *http.Request) (int, http.Header, []byte, error) {
	resp, err := c.Do(ctx, req)
	if err != nil {
		return 0, nil, nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, resp.Header, nil, fmt.Errorf("read response body: %w", err)
	}

	return resp.StatusCode, resp.Header, body, nil
}

// CloseIdleConnections closes idle connections on the transport.
func (c *Client) CloseIdleConnections() {
	c.inner.CloseIdleConnections()
}
