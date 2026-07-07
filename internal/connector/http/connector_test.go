package httpconnector

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/raghna/fury-sms-gateway/internal/connector/rule"
	"github.com/raghna/fury-sms-gateway/internal/domain"
	"github.com/raghna/fury-sms-gateway/internal/template"
)

func TestConnector_Send_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("expected Bearer token, got %q", r.Header.Get("Authorization"))
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected application/json, got %q", r.Header.Get("Content-Type"))
		}

		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body["to"] != "+1234567890" {
			t.Errorf("expected to = +1234567890, got %v", body["to"])
		}
		if body["text"] != "Hello World" {
			t.Errorf("expected text = Hello World, got %v", body["text"])
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"message_id": "prov-msg-123",
			"status":     "ok",
		})
	}))
	defer ts.Close()

	cfg := EndpointConfig{
		Protocol: "http",
		Request: RequestConfig{
			URL:    ts.URL,
			Method: http.MethodPost,
			Headers: []KeyValueConfig{
				{Key: "Authorization", Value: "Bearer test-token"},
			},
			Body: &BodyConfig{
				Template:    `{"to":"{{.Destination}}","text":"{{.Text}}"}`,
				ContentType: "application/json",
			},
		},
		Response: ResponseConfig{
			Rules: []rule.Rule{
				{
					Condition: rule.Condition{Field: "status", Operator: "eq", Value: "200"},
					Actions:   []rule.Action{{Type: "accept"},{Type: "extract", Key: "external_id", Value: "body.message_id"},},
				},
			},
		},
		Timeout: DurationConfig{Seconds: 10},
	}

	msg := &domain.Message{}
	msg.ID = "msg-001"
	msg.Source = "SENDER"
	msg.Destination = "+1234567890"
	msg.Text = "Hello World"
	msg.TenantID = "tenant-1"

	conn := NewConnector("test-http", cfg)
	result, err := conn.Send(context.Background(), &domain.SendRequest{
		Message: msg,
		Prepared: &domain.PreparedMessage{
			Destination: "+1234567890",
			Parts:       1,
			Encoding:    "gsm7",
		},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Acceptance != domain.AcceptanceFinal {
		t.Errorf("expected AcceptanceFinal, got %v", result.Acceptance)
	}
	if result.Parts != 1 {
		t.Errorf("expected parts = 1, got %d", result.Parts)
	}
}

func TestConnector_Send_Rejected(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": "invalid destination",
			"code":  1001,
		})
	}))
	defer ts.Close()

	cfg := EndpointConfig{
		Protocol: "http",
		Request: RequestConfig{
			URL:    ts.URL,
			Method: http.MethodPost,
			Body: &BodyConfig{
				Template:    `{"to":"{{.Destination}}","text":"{{.Text}}"}`,
				ContentType: "application/json",
			},
		},
		Response: ResponseConfig{
			Rules: []rule.Rule{
				{
					Condition: rule.Condition{Field: "body.code", Operator: "eq", Value: "1001"},
					Actions: []rule.Action{
						{Type: "reject"},
					},
				},
			},
		},
		Timeout: DurationConfig{Seconds: 10},
	}

	msg := &domain.Message{}
	msg.Source = "SENDER"
	msg.Destination = "+1234567890"
	msg.Text = "Hello"

	conn := NewConnector("test-http", cfg)
	result, err := conn.Send(context.Background(), &domain.SendRequest{
		Message: msg,
		Prepared: &domain.PreparedMessage{
			Destination: "+1234567890",
			Parts:       1,
			Encoding:    "gsm7",
		},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Acceptance != domain.AcceptanceRejected {
		t.Errorf("expected AcceptanceRejected, got %v", result.Acceptance)
	}
}

func TestConnector_Send_WithAuth(t *testing.T) {
	tests := []struct {
		name  string
		auth  AuthConfig
		check func(*testing.T, *http.Request)
	}{
		{
			name: "bearer",
			auth: AuthConfig{Type: "bearer", Credentials: map[string]string{"token": "my-token"}},
			check: func(t *testing.T, r *http.Request) {
				if v := r.Header.Get("Authorization"); v != "Bearer my-token" {
					t.Errorf("expected Bearer my-token, got %q", v)
				}
			},
		},
		{
			name: "basic",
			auth: AuthConfig{Type: "basic", Credentials: map[string]string{"username": "user", "password": "pass"}},
			check: func(t *testing.T, r *http.Request) {
				u, p, ok := r.BasicAuth()
				if !ok || u != "user" || p != "pass" {
					t.Errorf("expected basic auth user:pass, got %s:%s (ok=%v)", u, p, ok)
				}
			},
		},
		{
			name: "api_key",
			auth: AuthConfig{Type: "api_key", Credentials: map[string]string{"key": "abc123"}},
			check: func(t *testing.T, r *http.Request) {
				if v := r.Header.Get("X-API-Key"); v != "abc123" {
					t.Errorf("expected X-API-Key: abc123, got %q", v)
				}
			},
		},
		{
			name: "query_param",
			auth: AuthConfig{Type: "query_param", Credentials: map[string]string{"param_name": "api_key", "param_value": "abc123"}},
			check: func(t *testing.T, r *http.Request) {
				if v := r.URL.Query().Get("api_key"); v != "abc123" {
					t.Errorf("expected api_key=abc123, got %q", v)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				tt.check(t, r)
				w.WriteHeader(http.StatusOK)
			}))
			defer ts.Close()

			cfg := EndpointConfig{
				Protocol: "http",
				Request: RequestConfig{
					URL:    ts.URL,
					Method: http.MethodPost,
				},
				Auth: tt.auth,
				Response: ResponseConfig{
					Rules: []rule.Rule{
						{
							Condition: rule.Condition{Field: "status", Operator: "eq", Value: "200"},
							Actions:   []rule.Action{{Type: "accept"},{Type: "extract", Key: "external_id", Value: "body.message_id"},},
						},
					},
				},
				Timeout: DurationConfig{Seconds: 10},
			}

			msg := &domain.Message{}
			msg.Destination = "+1234567890"
			msg.Text = "Hello"

			conn := NewConnector("test-auth", cfg)
			_, err := conn.Send(context.Background(), &domain.SendRequest{
				Message: msg,
				Prepared: &domain.PreparedMessage{
					Destination: "+1234567890",
					Parts:       1,
					Encoding:    "gsm7",
				},
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestConnector_Send_TemplateRendering(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"message_id": r.URL.Query().Get("ref"),
		})
	}))
	defer ts.Close()

	cfg := EndpointConfig{
		Protocol: "http",
		Request: RequestConfig{
			URL:    ts.URL + "?ref={{.MessageID}}&src={{.Source}}",
			Method: http.MethodGet,
		},
		Response: ResponseConfig{
			Rules: []rule.Rule{
				{
					Condition: rule.Condition{Field: "status", Operator: "eq", Value: "200"},
					Actions:   []rule.Action{{Type: "accept"},{Type: "extract", Key: "external_id", Value: "body.message_id"},},
				},
			},
		},
		Timeout: DurationConfig{Seconds: 10},
	}

	msg := &domain.Message{}
	msg.ID = "msg-099"
	msg.Source = "MyApp"
	msg.Destination = "+1234567890"
	msg.Text = "Hello World"
	msg.TenantID = "tenant-x"

	conn := NewConnector("test-tmpl", cfg)
	result, err := conn.Send(context.Background(), &domain.SendRequest{
		Message: msg,
		Prepared: &domain.PreparedMessage{
			Destination: "+1234567890",
			Parts:       1,
			Encoding:    "gsm7",
		},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Acceptance != domain.AcceptanceFinal {
		t.Errorf("expected acceptance final")
	}
}

func TestConnector_CheckHealth_Enabled(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer ts.Close()

	cfg := EndpointConfig{
		Health: HealthCheckConfig{
			Enabled: true,
			URL:     ts.URL + "/health",
			Method:  http.MethodGet,
			Rule: rule.Rule{
				Condition: rule.Condition{Field: "body.status", Operator: "eq", Value: "ok"},
				Actions:   []rule.Action{{Type: "accept"},{Type: "extract", Key: "external_id", Value: "body.message_id"},},
			},
		},
		Timeout: DurationConfig{Seconds: 10},
	}
	client := NewClient(5 * time.Second)
	conn := NewConnector("test-health", cfg, WithClient(client))

	err := conn.CheckHealth(context.Background())
	if err != nil {
		t.Fatalf("unexpected health check error: %v", err)
	}
}

func TestConnector_CheckHealth_Disabled(t *testing.T) {
	cfg := EndpointConfig{
		Health: HealthCheckConfig{Enabled: false},
	}

	conn := NewConnector("test-health-disabled", cfg)
	err := conn.CheckHealth(context.Background())
	if err != nil {
		t.Fatalf("expected nil for disabled health check, got: %v", err)
	}
}

func TestConnector_CheckHealth_Unhealthy(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(`{"status":"down"}`))
	}))
	defer ts.Close()

	cfg := EndpointConfig{
		Health: HealthCheckConfig{
			Enabled: true,
			URL:     ts.URL,
			Method:  http.MethodGet,
			Rule: rule.Rule{
				Condition: rule.Condition{Field: "status", Operator: "eq", Value: "200"},
				Actions:   []rule.Action{{Type: "accept"},{Type: "extract", Key: "external_id", Value: "body.message_id"},},
			},
		},
		Timeout: DurationConfig{Seconds: 10},
	}
	client := NewClient(5 * time.Second)
	conn := NewConnector("test-health-bad", cfg, WithClient(client))

	err := conn.CheckHealth(context.Background())
	if err == nil {
		t.Fatal("expected health check error, got nil")
	}
}

func TestBuildRequest_WithTemplateEngine(t *testing.T) {
	tmpl := template.NewEngine()
	data := template.Data{
		Source:      "SENDER",
		Destination: "+1234567890",
		Text:       "Test msg",
		MessageID:  "msg-001",
	}

	cfg := &EndpointConfig{
		Request: RequestConfig{
			URL:    "https://api.example.com/send",
			Method: "POST",
			Headers: []KeyValueConfig{
				{Key: "X-Message-ID", Value: "{{.MessageID}}"},
			},
			Body: &BodyConfig{
				Template:    `{"to":"{{.Destination}}","text":"{{.Text}}"}`,
				ContentType: "application/json",
			},
		},
	}

	req, err := BuildRequest(cfg, data, tmpl)
	if err != nil {
		t.Fatalf("BuildRequest failed: %v", err)
	}

	if req.Method != "POST" {
		t.Errorf("expected POST, got %s", req.Method)
	}
	if req.Header.Get("X-Message-ID") != "msg-001" {
		t.Errorf("expected X-Message-ID: msg-001, got %q", req.Header.Get("X-Message-ID"))
	}

	body := make([]byte, 100)
	n, _ := req.Body.Read(body)
	bodyStr := string(body[:n])
	if bodyStr != `{"to":"+1234567890","text":"Test msg"}` {
		t.Errorf("unexpected body: %s", bodyStr)
	}
}

func TestNewClient_DefaultTimeout(t *testing.T) {
	client := NewClient(30 * time.Second)
	if client.inner.Timeout != 30*time.Second {
		t.Errorf("expected 30s timeout, got %v", client.inner.Timeout)
	}
}

func TestRuleEngine_Integration(t *testing.T) {
	eng := rule.NewEngine()

	// Build ResponseData with parsed JSON
	parsed := map[string]interface{}{
		"message_id": "ext-123",
		"status":     "ok",
		"price":     float64(5000),
	}

	resp := rule.ResponseData{
		Status:  200,
		Headers: map[string]string{"content-type": "application/json"},
		Body:    []byte(`{"message_id":"ext-123","status":"ok","price":5000}`),
		Parsed:  parsed,
	}

	rules := []rule.Rule{
		{
			Condition: rule.Condition{Field: "status", Operator: "eq", Value: "200"},
			Actions: []rule.Action{
				{Type: "accept"},
			},
		},
		{
			Condition: rule.Condition{Field: "body.message_id", Operator: "exists"},
			Actions: []rule.Action{
				{Type: "extract", Key: "external_id", Value: "body.message_id"},
			},
		},
	}

	result := eng.Evaluate(rules, resp)
	if !result.Accepted {
		t.Error("expected accepted")
	}
}

func TestRuleEngine_ExtractFromBody(t *testing.T) {
	eng := rule.NewEngine()

	parsed := map[string]interface{}{
		"message_id": "ext-456",
		"price":     float64(7500),
	}

	resp := rule.ResponseData{
		Status:  200,
		Headers: map[string]string{},
		Body:    []byte(`{"message_id":"ext-456","price":7500}`),
		Parsed:  parsed,
	}

	rules := []rule.Rule{
		{
			Condition: rule.Condition{Field: "status", Operator: "eq", Value: "200"},
			Actions: []rule.Action{
				{Type: "accept"},
				{Type: "extract", Key: "external_id", Value: "body.message_id"},
				{Type: "extract", Key: "price"},
			},
		},
		{
			Condition: rule.Condition{Field: "body.message_id", Operator: "exists"},
			Actions: []rule.Action{
				{Type: "extract", Key: "external_id"},
				{Type: "extract", Key: "price"},
			},
		},
	}

	result := eng.Evaluate(rules, resp)
	if !result.Accepted {
		t.Error("expected accepted")
	}
	if result.Extract["external_id"] != "ext-456" {
		t.Errorf("expected external_id = ext-456, got %q", result.Extract["external_id"])
	}
	if result.Extract["price"] != "7500" {
		t.Errorf("expected price = 7500, got %q", result.Extract["price"])
	}
}
