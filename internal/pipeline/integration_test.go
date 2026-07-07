package pipeline

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/raghna/fury-sms-gateway/internal/connector"
	httph "github.com/raghna/fury-sms-gateway/internal/connector/driver/http"
	"github.com/raghna/fury-sms-gateway/internal/connector/rule"
	"github.com/raghna/fury-sms-gateway/internal/domain"
	"github.com/raghna/fury-sms-gateway/internal/routing"
	"github.com/raghna/fury-sms-gateway/internal/template"
)

// mockHealthProvider always reports healthy.
type mockHealthProvider struct{}

func (m mockHealthProvider) IsHealthy(_ string) bool { return true }

// TestPipeline_GenericConnector_HTTPIntegration verifies the entire chain:
//
//	Pipeline → RouteStage → SendStage → MemoryRegistry → GenericConnector → HTTPDriver → httptest.Server
func TestPipeline_GenericConnector_HTTPIntegration(t *testing.T) {
	// ── Setup test HTTP server ──────────────────────────────────────
	var receivedBody string
	var receivedHeaders map[string]string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = make(map[string]string)
		for k := range r.Header {
			receivedHeaders[k] = r.Header.Get(k)
		}
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
			if b, ok := body["text"].(string); ok {
				receivedBody = b
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Message-Id", "ext-98765")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok","message_id":"ext-98765"}`))
	}))
	defer ts.Close()

	// ── Setup Driver + Connector ────────────────────────────────────
	httpDriver := httph.NewDriver()
	transportJSON, _ := json.Marshal(map[string]interface{}{
		"url":    ts.URL,
		"method": "POST",
		"headers": map[string]string{
			"Authorization": "Bearer test-token",
			"Content-Type":  "application/json",
		},
		"body": `{"to":"{{destination}}","text":"{{text}}","source":"{{source}}"}`,
	})

	connCfg := connector.ConnectorConfig{
		Metadata:  connector.MetadataConfig{ID: "http-test-1"},
		Transport: transportJSON,
		Rules: connector.RuleConfig{
			Rules: []rule.Rule{
				{
					Condition: rule.Condition{Field: "status", Operator: "eq", Value: "200"},
					Actions:   []rule.Action{{Type: "accept"}},
				},
			},
		},
		Health: connector.HealthCheckConfig{Enabled: false},
	}

	gc := connector.NewGenericConnector(
		"http-test-1",
		domain.ConnectorTypeHTTPClient,
		connCfg,
		httpDriver,
		connector.WithTemplateEngine(template.NewEngine()),
		connector.WithRuleEngine(rule.NewEngine()),
	)

	// ── Setup Registry ─────────────────────────────────────────────
	reg := connector.NewMemoryRegistry()
	reg.MustAdd(gc)

	// ── Setup Routes ────────────────────────────────────────────────
	routes := []domain.Route{
		{
			BaseModel:   domain.BaseModel{ID: "route-1"},
			TenantID:    "tenant-test",
			Name:        "Test Route",
			Type:        domain.RouteTypeSMS,
			Strategy:    domain.RouteStrategyStatic,
			Prefix:      "2", // match normalized Egyptian numbers
			ConnectorID: "http-test-1",
			Enabled:     true,
		},
	}

	// ── Setup Routing Engine ────────────────────────────────────────
	routingEngine := routing.NewEngine(
		routes,
		mockHealthProvider{},
		nil, // default selector factory
	)

	// ── Setup Pipeline ──────────────────────────────────────────────
	pipeline := New(
		NewValidateStage(),
		NewPrepareStage(),
		NewRouteStage(routingEngine),
		NewSendStage(reg),
		NewHandleResultStage(),
		NewBuildEventsStage(),
	)

	// ── Build message ───────────────────────────────────────────────
	msg := &domain.Message{
		BaseModel:   domain.BaseModel{ID: "int-msg-001"},
		TenantID:    "tenant-test",
		Source:      "INTEGRATION",
		Destination: "+201234567890",
		Text:        "Hello from integration test! مرحباً",
		Status:      domain.MessageStatusQueued,
	}

	state := NewPipelineState(msg, "trace-integration-test")

	// ── Execute pipeline ────────────────────────────────────────────
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := pipeline.Execute(ctx, state)
	if err != nil {
		t.Fatalf("pipeline execution failed: %v", err)
	}

	// ── Verify results ──────────────────────────────────────────────
	if state.DeliveryOutcome == nil {
		t.Fatal("DeliveryOutcome is nil — HandleResultStage didn't run")
	}
	if state.DeliveryOutcome.ExternalID != "ext-98765" {
		t.Errorf("ExternalID = %q, want %q", state.DeliveryOutcome.ExternalID, "ext-98765")
	}
	if state.DeliveryOutcome.Status != domain.MessageStatusSent {
		t.Errorf("Status = %v, want %v", state.DeliveryOutcome.Status, domain.MessageStatusSent)
	}
	if state.DeliveryOutcome.ConnectorID != "http-test-1" {
		t.Errorf("ConnectorID = %q, want %q", state.DeliveryOutcome.ConnectorID, "http-test-1")
	}
	if state.DeliveryOutcome.RouteID != "route-1" {
		t.Errorf("RouteID = %q, want %q", state.DeliveryOutcome.RouteID, "route-1")
	}

	// Verify events were built
	if len(state.DomainEvents) == 0 {
		t.Error("no domain events built — BuildEventsStage didn't run")
	}

	// Verify the HTTP server received the right data
	if _, ok := receivedHeaders["Authorization"]; !ok {
		t.Error("HTTP server did not receive Authorization header")
	}
	if receivedBody != "Hello from integration test! مرحباً" {
		t.Errorf("HTTP server received body text = %q, want %q", receivedBody, "Hello from integration test! مرحباً")
	}
}

// TestPipeline_GenericConnector_Rejected tests the pipeline with a 400 response.
func TestPipeline_GenericConnector_Rejected(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"invalid destination"}`))
	}))
	defer ts.Close()

	httpDriver := httph.NewDriver()
	transportJSON, _ := json.Marshal(map[string]interface{}{
		"url":    ts.URL,
		"method": "POST",
		"body":   `{"to":"{{destination}}","text":"{{text}}"}`,
	})

	connCfg := connector.ConnectorConfig{
		Metadata:  connector.MetadataConfig{ID: "http-reject-test"},
		Transport: transportJSON,
		Rules: connector.RuleConfig{
			Rules: []rule.Rule{
				{
					Condition: rule.Condition{Field: "status", Operator: "eq", Value: "400"},
					Actions:   []rule.Action{{Type: "reject"}},
				},
			},
		},
		Health: connector.HealthCheckConfig{Enabled: false},
	}

	gc := connector.NewGenericConnector(
		"http-reject-test",
		domain.ConnectorTypeHTTPClient,
		connCfg,
		httpDriver,
		connector.WithRuleEngine(rule.NewEngine()),
	)

	reg := connector.NewMemoryRegistry()
	reg.MustAdd(gc)

	routes := []domain.Route{
		{
			BaseModel:   domain.BaseModel{ID: "route-rej"},
			TenantID:    "tenant-test",
			Name:        "Reject Route",
			Type:        domain.RouteTypeSMS,
			Strategy:    domain.RouteStrategyStatic,
			Prefix:      "1", // match normalized destination starting with 1
			ConnectorID: "http-reject-test",
			Enabled:     true,
		},
	}

	routingEngine := routing.NewEngine(routes, mockHealthProvider{}, nil)

	pipeline := New(
		NewValidateStage(),
		NewPrepareStage(),
		NewRouteStage(routingEngine),
		NewSendStage(reg),
		NewHandleResultStage(),
		NewBuildEventsStage(),
	)

	msg := &domain.Message{
		BaseModel:   domain.BaseModel{ID: "rej-msg-001"},
		TenantID:    "tenant-test",
		Source:      "TEST",
		Destination: "+1234",
		Text:        "Test",
		Status:      domain.MessageStatusQueued,
	}

	state := NewPipelineState(msg, "trace-rej")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := pipeline.Execute(ctx, state)
	if err != nil {
		t.Fatalf("pipeline execution failed: %v", err)
	}

	if state.DeliveryOutcome == nil {
		t.Fatal("DeliveryOutcome is nil")
	}
	if !state.DeliveryOutcome.IsTerminal() {
		t.Errorf("expected terminal outcome for rejection, got status=%v", state.DeliveryOutcome.Status)
	}
	if state.DeliveryOutcome.Status != domain.MessageStatusFailed {
		t.Errorf("expected Failed status, got %v", state.DeliveryOutcome.Status)
	}
}
