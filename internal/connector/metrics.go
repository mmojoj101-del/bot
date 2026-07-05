package connector

import (
	"time"

	"github.com/raghna/fury-sms-gateway/internal/domain"
)

// NoopMetricsRecorder is a no-op implementation of MetricsRecorder for development.
type NoopMetricsRecorder struct{}

func NewNoopMetricsRecorder() *NoopMetricsRecorder {
	return &NoopMetricsRecorder{}
}

func (m *NoopMetricsRecorder) RecordMessageSent(tenantID, connectorID string, parts int, latency time.Duration) {}
func (m *NoopMetricsRecorder) RecordMessageFailed(tenantID, connectorID, errorCode string)                      {}
func (m *NoopMetricsRecorder) RecordDLRReceived(tenantID, status string)                                       {}
func (m *NoopMetricsRecorder) RecordRetry(tenantID string, attempt int)                                         {}

// ensure interface compliance
var _ domain.MetricsRecorder = (*NoopMetricsRecorder)(nil)
