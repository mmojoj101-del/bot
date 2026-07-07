package connector

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/raghna/fury-sms-gateway/internal/connector/rule"
	"github.com/raghna/fury-sms-gateway/internal/domain"
)

// mockDriver implements ProtocolDriver for testing.
type mockDriver struct {
	protocol         domain.ConnectorType
	sendFunc         func(ctx context.Context, req *TransportRequest) (*TransportResponse, error)
	healthFunc       func(ctx context.Context) error
	validateFunc     func(cfg TransportConfig) error
	decodeConfigFunc func(data []byte) (TransportConfig, error)
}

// mockConfig is a simple TransportConfig for testing.
type mockConfig struct{}

func (m *mockConfig) Protocol() domain.ConnectorType { return "mock" }

func (m *mockDriver) Protocol() domain.ConnectorType { return m.protocol }

func (m *mockDriver) DecodeConfig(data []byte) (TransportConfig, error) {
	if m.decodeConfigFunc != nil {
		return m.decodeConfigFunc(data)
	}
	return &mockConfig{}, nil
}

func (m *mockDriver) ValidateConfig(cfg TransportConfig) error {
	if m.validateFunc != nil {
		return m.validateFunc(cfg)
	}
	return nil
}

func (m *mockDriver) Send(ctx context.Context, req *TransportRequest) (*TransportResponse, error) {
	if m.sendFunc != nil {
		return m.sendFunc(ctx, req)
	}
	return &TransportResponse{Status: 200}, nil
}

func (m *mockDriver) CheckHealth(ctx context.Context, _ TransportConfig) error {
	if m.healthFunc != nil {
		return m.healthFunc(ctx)
	}
	return nil
}

func TestGenericConnector_Send_Success(t *testing.T) {
	driver := &mockDriver{
		protocol: domain.ConnectorTypeHTTPClient,
		sendFunc: func(_ context.Context, req *TransportRequest) (*TransportResponse, error) {
			return &TransportResponse{
				Status:     200,
				Body:       []byte(`{"message_id":"ext-123","status":"ok"}`),
				ExternalID: "ext-123",
			}, nil
		},
	}

	cfg := ConnectorConfig{
		Rules: RuleConfig{
			Rules: []rule.Rule{
				{
					Condition: rule.Condition{Field: "status", Operator: "eq", Value: "200"},
					Actions:   []rule.Action{{Type: "accept"}},
				},
			},
		},
	}

	conn := NewGenericConnector("test-conn", domain.ConnectorTypeHTTPClient, cfg, driver)
	result, err := conn.Send(context.Background(), &domain.SendRequest{
		Message: &domain.Message{
			Source:      "SENDER",
			Destination: "+1234567890",
			Text:       "Hello World",
		},
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
	if result.ExternalID != "ext-123" {
		t.Errorf("expected ExternalID = ext-123, got %q", result.ExternalID)
	}
}

func TestGenericConnector_Send_Rejected(t *testing.T) {
	driver := &mockDriver{
		sendFunc: func(_ context.Context, req *TransportRequest) (*TransportResponse, error) {
			return &TransportResponse{
				Status: 400,
				Body:   []byte(`{"error":"invalid"}`),
			}, nil
		},
	}

	cfg := ConnectorConfig{
		Rules: RuleConfig{
			Rules: []rule.Rule{
				{
					Condition: rule.Condition{Field: "status", Operator: "eq", Value: "400"},
					Actions:   []rule.Action{{Type: "reject"}},
				},
			},
		},
	}

	conn := NewGenericConnector("test-conn", domain.ConnectorTypeHTTPClient, cfg, driver)
	result, err := conn.Send(context.Background(), &domain.SendRequest{
		Message: &domain.Message{
			Destination: "+1234",
			Text:       "Hello",
		},
		Prepared: &domain.PreparedMessage{
			Destination: "+1234",
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

func TestGenericConnector_Send_TransportError(t *testing.T) {
	driver := &mockDriver{
		sendFunc: func(_ context.Context, req *TransportRequest) (*TransportResponse, error) {
			return nil, errors.New("connection refused")
		},
	}

	cfg := ConnectorConfig{}
	conn := NewGenericConnector("test-conn", domain.ConnectorTypeHTTPClient, cfg, driver)
	_, err := conn.Send(context.Background(), &domain.SendRequest{
		Message:  &domain.Message{Destination: "+1234", Text: "Hello"},
		Prepared: &domain.PreparedMessage{Destination: "+1234", Parts: 1, Encoding: "gsm7"},
	})

	if err == nil {
		t.Fatal("expected error for transport failure")
	}
}

func TestGenericConnector_Send_RuleExtractExternalID(t *testing.T) {
	driver := &mockDriver{
		sendFunc: func(_ context.Context, req *TransportRequest) (*TransportResponse, error) {
			return &TransportResponse{
				Status: 200,
				Body:   []byte(`{"message_id":"ext-999","price":5000}`),
			}, nil
		},
	}

	cfg := ConnectorConfig{
		Rules: RuleConfig{
			Rules: []rule.Rule{
				{
					Condition: rule.Condition{Field: "status", Operator: "eq", Value: "200"},
					Actions: []rule.Action{
						{Type: "accept"},
						{Type: "extract", Key: "external_id", Value: "message_id"},
						{Type: "extract", Key: "price"},
					},
				},
			},
		},
	}

	conn := NewGenericConnector("test-conn", domain.ConnectorTypeHTTPClient, cfg, driver)
	result, err := conn.Send(context.Background(), &domain.SendRequest{
		Message:  &domain.Message{Destination: "+1234", Text: "Hello"},
		Prepared: &domain.PreparedMessage{Destination: "+1234", Parts: 2, Encoding: "gsm7"},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExternalID != "ext-999" {
		t.Errorf("expected ExternalID = ext-999, got %q", result.ExternalID)
	}
	if result.Parts != 2 {
		t.Errorf("expected Parts = 2, got %d", result.Parts)
	}
}

func TestGenericConnector_CheckHealth_Disabled(t *testing.T) {
	cfg := ConnectorConfig{
		Health: HealthCheckConfig{Enabled: false},
	}
	conn := NewGenericConnector("test", domain.ConnectorTypeHTTPClient, cfg, &mockDriver{})
	err := conn.CheckHealth(context.Background())
	if err != nil {
		t.Fatalf("expected nil for disabled health, got: %v", err)
	}
}

func TestGenericConnector_IDAndProtocol(t *testing.T) {
	conn := NewGenericConnector("my-id", domain.ConnectorTypeHTTPClient, ConnectorConfig{}, &mockDriver{})
	if conn.ID() != "my-id" {
		t.Errorf("ID() = %q, want my-id", conn.ID())
	}
	if conn.Protocol() != domain.ConnectorTypeHTTPClient {
		t.Errorf("Protocol() = %v, want http_client", conn.Protocol())
	}
}

// Ensure GenericConnector implements Connector and HealthChecker.
var _ Connector = (*GenericConnector)(nil)
var _ HealthChecker = (*GenericConnector)(nil)

// Benchmark for performance-sensitive paths.
func BenchmarkGenericConnector_Send(b *testing.B) {
	driver := &mockDriver{
		sendFunc: func(_ context.Context, req *TransportRequest) (*TransportResponse, error) {
			return &TransportResponse{Status: 200, Body: []byte(`{"status":"ok"}`)}, nil
		},
	}
	cfg := ConnectorConfig{
		Rules: RuleConfig{
			Rules: []rule.Rule{
				{
					Condition: rule.Condition{Field: "status", Operator: "eq", Value: "200"},
					Actions:   []rule.Action{{Type: "accept"}},
				},
			},
		},
	}
	conn := NewGenericConnector("bench", domain.ConnectorTypeHTTPClient, cfg, driver)
	req := &domain.SendRequest{
		Message:  &domain.Message{Source: "S", Destination: "+1234567890", Text: "Hello World"},
		Prepared: &domain.PreparedMessage{Destination: "+1234567890", Parts: 1, Encoding: "gsm7"},
	}
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = conn.Send(ctx, req)
	}
}

// ── StatefulDriver mock + concurrency tests ──────────────────────────────────

// mockStatefulDriver implements StatefulDriver with connect call counting.
type mockStatefulDriver struct {
	mockDriver
	connectCount   int
	disconnectCount int
	connected      bool
	connectDelay   time.Duration
	mu             sync.Mutex
}

func (m *mockStatefulDriver) Connect(_ context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.connectDelay > 0 {
		time.Sleep(m.connectDelay)
	}
	m.connectCount++
	m.connected = true
	return nil
}

func (m *mockStatefulDriver) Disconnect(_ context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.disconnectCount++
	m.connected = false
	return nil
}

func (m *mockStatefulDriver) IsConnected() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.connected
}

// TestConcurrentConnect_NoDoubleConnect verifies that parallel Connect calls
// result in only one actual driver.Connect() invocation (sessionMu + double-check).
func TestConcurrentConnect_NoDoubleConnect(t *testing.T) {
	sd := &mockStatefulDriver{
		connectDelay: 50 * time.Millisecond,
	}
	conn := NewGenericConnector("stateful-test", "smpp", ConnectorConfig{}, sd)

	// Force lazyInit to discover stateful capabilities
	conn.lazyInit()

	ctx := context.Background()
	const goroutines = 10
	var wg sync.WaitGroup
	errs := make([]error, goroutines)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			errs[idx] = conn.Connect(ctx)
		}(i)
	}
	wg.Wait()

	// Check all succeeded
	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d: Connect error: %v", i, err)
		}
	}

	if sd.connectCount != 1 {
		t.Errorf("expected exactly 1 Connect call, got %d", sd.connectCount)
	}
	if !conn.hooks.statefulDriver.IsConnected() {
		t.Error("connector should be connected after Connect")
	}
}

// TestConcurrentDisconnect_NoDoubleDisconnect verifies parallel Disconnect calls
// result in only one actual driver.Disconnect() invocation.
func TestConcurrentDisconnect_NoDoubleDisconnect(t *testing.T) {
	sd := &mockStatefulDriver{
		connected: true,
	}
	conn := NewGenericConnector("stateful-test", "smpp", ConnectorConfig{}, sd)
	conn.lazyInit()

	ctx := context.Background()
	const goroutines = 10
	var wg sync.WaitGroup
	errs := make([]error, goroutines)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			errs[idx] = conn.Disconnect(ctx)
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d: Disconnect error: %v", i, err)
		}
	}

	if sd.disconnectCount != 1 {
		t.Errorf("expected exactly 1 Disconnect call, got %d", sd.disconnectCount)
	}
	if conn.hooks.statefulDriver.IsConnected() {
		t.Error("connector should be disconnected after Disconnect")
	}
}

// TestConcurrentConnectDisconnect_NoRace verifies that Connect and Disconnect
// running concurrently don't race — serialized by sessionMu.
func TestConcurrentConnectDisconnect_NoRace(t *testing.T) {
	sd := &mockStatefulDriver{
		connectDelay:    20 * time.Millisecond,
		connected:       false,
	}
	conn := NewGenericConnector("stateful-test", "smpp", ConnectorConfig{}, sd)
	conn.lazyInit()

	ctx := context.Background()
	errs := make([]error, 4)
	var wg sync.WaitGroup

	// Two Connect + two Disconnect concurrently
	wg.Add(1)
	go func() {
		defer wg.Done()
		errs[0] = conn.Connect(ctx)
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		errs[1] = conn.Connect(ctx)
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		time.Sleep(5 * time.Millisecond) // ensure Connect starts first
		errs[2] = conn.Disconnect(ctx)
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		time.Sleep(10 * time.Millisecond)
		errs[3] = conn.Disconnect(ctx)
	}()
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d: error: %v", i, err)
		}
	}

	// Total connect calls should be exactly 1 (second sees connected via double-check)
	if sd.connectCount != 1 {
		t.Errorf("expected 1 Connect call, got %d", sd.connectCount)
	}
	// Total disconnect calls should be exactly 1 (second sees not-connected via double-check)
	if sd.disconnectCount != 1 {
		t.Errorf("expected 1 Disconnect call, got %d", sd.disconnectCount)
	}

	// Final state depends on ordering — both are valid
	t.Logf("Final connected state: %v (valid regardless)", conn.hooks.statefulDriver.IsConnected())
}

// ── renderStructFields tests ──────────────────────────────────────────────────

// TestRenderStructFields_NestedMaps tests deep recursion:
// map[string]interface{} → string, map, slice, nested
func TestRenderStructFields_NestedMaps(t *testing.T) {
	fields := map[string]string{
		"token":   "bearer-secret",
		"name":    "Ahmed",
		"phone":   "+201234567890",
		"city":    "Cairo",
		"service": "production",
	}

	// map[string]interface{} with multiple levels
	cfg := map[string]interface{}{
		"auth": map[string]interface{}{
			"type":   "Bearer",
			"token":  "{{token}}",
			"origin": map[string]string{"dc": "{{service}}"},
		},
		"endpoints": []map[string]interface{}{
			{
				"url":  "https://api.example.com/{{service}}/send",
				"name": "{{service}}-primary",
				"headers": map[string]string{"X-User": "{{name}}"},
			},
		},
		"user": map[string]interface{}{
			"contact": map[string]string{"phone": "{{phone}}"},
		},
	}

	if err := renderStructFields(&cfg, fields); err != nil {
		t.Fatalf("renderStructFields error: %v", err)
	}

	// Verify auth.token rendered
	auth := cfg["auth"].(map[string]interface{})
	if auth["token"] != "bearer-secret" {
		t.Errorf("auth.token = %q, want bearer-secret", auth["token"])
	}

	// Verify nested origin map
	origin := auth["origin"].(map[string]string)
	if origin["dc"] != "production" {
		t.Errorf("auth.origin.dc = %q, want production", origin["dc"])
	}

	// Verify slice-of-maps with nested string keys
	endpoints := cfg["endpoints"].([]map[string]interface{})
	if len(endpoints) != 1 {
		t.Fatalf("expected 1 endpoint, got %d", len(endpoints))
	}
	if endpoints[0]["url"] != "https://api.example.com/production/send" {
		t.Errorf("endpoint url = %q, want rendered", endpoints[0]["url"])
	}
	if endpoints[0]["name"] != "production-primary" {
		t.Errorf("endpoint name = %q, want production-primary", endpoints[0]["name"])
	}

	// Verify nested map[int]string inside slice element's map
	headers := endpoints[0]["headers"].(map[string]string)
	if headers["X-User"] != "Ahmed" {
		t.Errorf("headers[X-User] = %q, want Ahmed", headers["X-User"])
	}

	// Verify deeply nested user.contact.phone
	user := cfg["user"].(map[string]interface{})
	contact := user["contact"].(map[string]string)
	if contact["phone"] != "+201234567890" {
		t.Errorf("user.contact.phone = %q, want +201234567890", contact["phone"])
	}
}

// TestRenderStructFields_InterfaceSliceMap tests interface{} → Slice → Map path.
func TestRenderStructFields_InterfaceSliceMap(t *testing.T) {
	fields := map[string]string{"key": "resolved_value"}

	// []interface{} where each element is map[string]string
	cfg := map[string]interface{}{
		"items": []interface{}{
			map[string]string{"id": "{{key}}", "type": "test"},
			map[string]string{"id": "static", "type": "{{key}}"},
		},
		"meta": []interface{}{
			[]interface{}{
				map[string]string{"deep": "{{key}}"},
			},
		},
	}

	if err := renderStructFields(&cfg, fields); err != nil {
		t.Fatalf("renderStructFields error: %v", err)
	}

	items := cfg["items"].([]interface{})
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}

	item0 := items[0].(map[string]string)
	if item0["id"] != "resolved_value" {
		t.Errorf("items[0].id = %q, want resolved_value", item0["id"])
	}
	if item0["type"] != "test" {
		t.Errorf("items[0].type = %q, want test", item0["type"])
	}

	item1 := items[1].(map[string]string)
	if item1["type"] != "resolved_value" {
		t.Errorf("items[1].type = %q, want resolved_value", item1["type"])
	}

	// Verify nested interface{} → Slice → Map
	meta := cfg["meta"].([]interface{})
	inner := meta[0].([]interface{})
	deep := inner[0].(map[string]string)
	if deep["deep"] != "resolved_value" {
		t.Errorf("meta[0][0].deep = %q, want resolved_value", deep["deep"])
	}
}

// TestRenderStructFields_StructNested tests struct → slice → map → struct recursion.
func TestRenderStructFields_StructNested(t *testing.T) {
	type Inner struct {
		Value string
		Tags  []string
	}
	type Request struct {
		URL     string
		Headers map[string]string
		Parts   []Inner
		Config  map[string][]Inner
	}

	fields := map[string]string{
		"host":  "api.example.com",
		"token": "abc",
		"tag1":  "primary",
		"tag2":  "secondary",
	}

	req := &Request{
		URL:     "https://{{host}}/v1/send",
		Headers: map[string]string{"Auth": "Bearer {{token}}"},
		Parts: []Inner{
			{Value: "{{token}}", Tags: []string{"{{tag1}}", "static"}},
			{Value: "static", Tags: []string{"{{tag2}}"}},
		},
		Config: map[string][]Inner{
			"default": {
				{Value: "{{host}}", Tags: []string{"{{tag1}}", "{{tag2}}"}},
			},
		},
	}

	if err := renderStructFields(req, fields); err != nil {
		t.Fatalf("renderStructFields error: %v", err)
	}

	if req.URL != "https://api.example.com/v1/send" {
		t.Errorf("URL = %q", req.URL)
	}
	if req.Headers["Auth"] != "Bearer abc" {
		t.Errorf("Headers[Auth] = %q", req.Headers["Auth"])
	}
	if req.Parts[0].Value != "abc" {
		t.Errorf("Parts[0].Value = %q", req.Parts[0].Value)
	}
	if req.Parts[0].Tags[0] != "primary" {
		t.Errorf("Parts[0].Tags[0] = %q", req.Parts[0].Tags[0])
	}
	if req.Parts[0].Tags[1] != "static" {
		t.Errorf("Parts[0].Tags[1] = %q", req.Parts[0].Tags[1])
	}
	if req.Parts[1].Tags[0] != "secondary" {
		t.Errorf("Parts[1].Tags[0] = %q", req.Parts[1].Tags[0])
	}
	// map[string][]Inner
	inner := req.Config["default"][0]
	if inner.Value != "api.example.com" {
		t.Errorf("Config[default][0].Value = %q", inner.Value)
	}
	if inner.Tags[0] != "primary" || inner.Tags[1] != "secondary" {
		t.Errorf("Config[default][0].Tags = %v", inner.Tags)
	}
}
