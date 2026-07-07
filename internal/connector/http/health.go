package httpconnector

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/raghna/fury-sms-gateway/internal/connector/rule"
)

// HealthChecker performs configurable health checks against an HTTP endpoint.
type HealthChecker struct {
	cfg        HealthCheckConfig
	client     *Client
	ruleEngine *rule.Engine
}

// NewHealthChecker creates a health checker from config.
func NewHealthChecker(cfg HealthCheckConfig, client *Client, ruleEngine *rule.Engine) *HealthChecker {
	return &HealthChecker{
		cfg:        cfg,
		client:     client,
		ruleEngine: ruleEngine,
	}
}

// CheckHealth performs the health check and returns nil if healthy.
func (hc *HealthChecker) CheckHealth(ctx context.Context) error {
	if !hc.cfg.Enabled || hc.cfg.URL == "" {
		return nil // health checking disabled — assume healthy
	}

	method := hc.cfg.Method
	if method == "" {
		method = http.MethodGet
	}

	req, err := http.NewRequestWithContext(ctx, method, hc.cfg.URL, nil)
	if err != nil {
		return fmt.Errorf("health check: create request: %w", err)
	}

	resp, err := hc.client.Do(ctx, req)
	if err != nil {
		return fmt.Errorf("health check: request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("health check: read body: %w", err)
	}

	// Build response data
	respData := rule.ResponseData{
		Status:  resp.StatusCode,
		Headers: make(map[string]string),
		Body:    body,
	}

	// Evaluate health check rule
	result := hc.ruleEngine.Evaluate([]rule.Rule{hc.cfg.Rule}, respData)
	if !result.Accepted {
		return fmt.Errorf("health check: endpoint unhealthy (status %d)", resp.StatusCode)
	}

	return nil
}
