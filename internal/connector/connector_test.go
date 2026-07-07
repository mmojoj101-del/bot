package connector

import (
	"context"
	"errors"
	"testing"

	"github.com/raghna/fury-sms-gateway/internal/connector/rule"
	"github.com/raghna/fury-sms-gateway/internal/domain"
)

// mockDriver implements ProtocolDriver for testing.
type mockDriver struct {
	protocol   domain.ConnectorType
	sendFunc   func(ctx context.Context, req *TransportRequest) (*TransportResponse, error)
	healthFunc func(ctx context.Context) error
}

// mockConfig is a simple TransportConfig for testing.
type mockConfig struct{}
func (m *mockConfig) Protocol() domain.ConnectorType { return "mock" }

func (m *mockDriver) Protocol() domain.ConnectorType { return m.protocol }

func (m *mockDriver) DecodeConfig(_ []byte) (TransportConfig, error) {
	return &mockConfig{}, nil
}

func (m *mockDriver) Send(ctx context.Context, req *TransportRequest) (*TransportResponse, error) {
	if m.sendFunc != nil {
		return m.sendFunc(ctx, req)
	}
	return &TransportResponse{Status: 200}, nil
}

func (m *mockDriver) CheckHealth(ctx context.Context) error {
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
