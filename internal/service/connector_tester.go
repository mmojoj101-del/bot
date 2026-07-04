package service

import (
	"context"
	"fmt"

	"github.com/raghna/fury-sms-gateway/internal/domain"
)

// HTTPConnectorTester tests HTTP connections.
type HTTPConnectorTester struct{}

func NewHTTPConnectorTester() *HTTPConnectorTester {
	return &HTTPConnectorTester{}
}

func (t *HTTPConnectorTester) Test(ctx context.Context, c *domain.Connector) error {
	return fmt.Errorf("http connector test not yet implemented")
}

func (t *HTTPConnectorTester) Type() domain.ConnectorType {
	return domain.ConnectorTypeHTTPClient
}

// SMPPConnectorTester tests SMPP connections.
type SMPPConnectorTester struct{}

func NewSMPPConnectorTester() *SMPPConnectorTester {
	return &SMPPConnectorTester{}
}

func (t *SMPPConnectorTester) Test(ctx context.Context, c *domain.Connector) error {
	return fmt.Errorf("smpp connector test not yet implemented")
}

func (t *SMPPConnectorTester) Type() domain.ConnectorType {
	return domain.ConnectorTypeSMPPClient
}

// SIPTester tests SIP connections.
type SIPTester struct{}

func NewSIPTester() *SIPTester {
	return &SIPTester{}
}

func (t *SIPTester) Test(ctx context.Context, c *domain.Connector) error {
	return fmt.Errorf("sip connector test not yet implemented")
}

func (t *SIPTester) Type() domain.ConnectorType {
	return domain.ConnectorTypeSIPClient
}

// MockConnectorTester always succeeds — for development/testing only.
type MockConnectorTester struct{}

func NewMockConnectorTester() *MockConnectorTester {
	return &MockConnectorTester{}
}

func (t *MockConnectorTester) Test(_ context.Context, _ *domain.Connector) error {
	return nil
}

func (t *MockConnectorTester) Type() domain.ConnectorType {
	return domain.ConnectorTypeMock
}
