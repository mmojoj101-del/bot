package connector

import (
	"context"

	"github.com/raghna/fury-sms-gateway/internal/domain"
)

// MockConnector is a test implementation of Connector with controllable output.
type MockConnector struct {
	id       string
	protocol domain.ConnectorType

	// SendFunc is called by Send. Default returns acceptance final.
	SendFunc func(ctx context.Context, req *domain.SendRequest) (*domain.SendResult, error)

	// CalledCount tracks how many times Send was invoked.
	CalledCount int

	// LastRequest holds the last SendRequest received.
	LastRequest *domain.SendRequest
}

// NewMockConnector creates a MockConnector with the given ID and protocol.
func NewMockConnector(id string, protocol domain.ConnectorType) *MockConnector {
	return &MockConnector{
		id:       id,
		protocol: protocol,
		SendFunc: defaultSendFunc,
	}
}

// ID returns the mock connector's identifier.
func (m *MockConnector) ID() string { return m.id }

// Protocol returns the mock connector's protocol.
func (m *MockConnector) Protocol() domain.ConnectorType { return m.protocol }

// Send delegates to SendFunc and tracks calls.
func (m *MockConnector) Send(ctx context.Context, req *domain.SendRequest) (*domain.SendResult, error) {
	m.CalledCount++
	m.LastRequest = req
	return m.SendFunc(ctx, req)
}

// defaultSendFunc returns a default successful SendResult.
func defaultSendFunc(_ context.Context, _ *domain.SendRequest) (*domain.SendResult, error) {
	return &domain.SendResult{
		ExternalID: "mock-ext-id",
		Parts:      1,
		Acceptance: domain.AcceptanceFinal,
	}, nil
}

// compile-time check
var _ Connector = (*MockConnector)(nil)
