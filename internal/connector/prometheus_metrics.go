package connector

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/raghna/fury-sms-gateway/internal/domain"
)

// PrometheusMetricsRecorder implements MetricsRecorder using Prometheus client.
// NOTE: Create ONE instance per binary — promauto panics on duplicate registration.
type PrometheusMetricsRecorder struct {
	smsSentTotal    *prometheus.CounterVec
	smsFailedTotal  *prometheus.CounterVec
	smsRetryTotal   *prometheus.CounterVec
	dlrReceivedTotal *prometheus.CounterVec

	smsSentDuration          *prometheus.HistogramVec
	batchDuration            *prometheus.HistogramVec
	queueDepth               prometheus.Gauge
	workerRestarts           *prometheus.GaugeVec
	circuitBreakerStateChanges *prometheus.CounterVec
}

// NewPrometheusMetricsRecorder creates and registers all Prometheus metrics.
// Use subsystem to differentiate worker groups (e.g. "sms", "dlr").
func NewPrometheusMetricsRecorder(namespace, subsystem string) *PrometheusMetricsRecorder {
	return &PrometheusMetricsRecorder{
		smsSentTotal: promauto.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "sms_sent_total",
			Help:      "Total number of SMS messages sent successfully.",
			// NOTE: intentionally NO tenant_id label to avoid high cardinality.
			// Use connector_id + provider for aggregation.
		}, []string{"connector_id"}),

		smsFailedTotal: promauto.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "sms_failed_total",
			Help:      "Total number of SMS message send failures.",
		}, []string{"connector_id", "error_code"}),

		smsRetryTotal: promauto.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "sms_retry_total",
			Help:      "Total number of SMS message retries.",
		}, []string{"connector_id"}),

		dlrReceivedTotal: promauto.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "dlr_received_total",
			Help:      "Total number of DLR callbacks received.",
		}, []string{"connector_id", "status"}),

		smsSentDuration: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "sms_sent_duration_seconds",
			Help:      "Latency of SMS send operations in seconds.",
			Buckets:   []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		}, []string{"connector_id"}),

		batchDuration: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "batch_duration_seconds",
			Help:      "Duration of worker batch processing in seconds.",
			Buckets:   []float64{0.1, 0.5, 1, 2.5, 5, 10, 30},
		}, []string{"worker_type"}),

		queueDepth: promauto.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "queue_depth",
			Help:      "Current number of queued messages.",
		}),

		workerRestarts: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "worker_restarts_total",
			Help:      "Total number of worker restarts due to panic.",
		}, []string{"worker_type"}),

		circuitBreakerStateChanges: promauto.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "circuit_breaker_state_changes_total",
			Help:      "Total number of circuit breaker state transitions.",
		}, []string{"connector_id", "state"}),
	}
}

func (m *PrometheusMetricsRecorder) RecordMessageSent(tenantID, connectorID string, parts int, latency time.Duration) {
	m.smsSentTotal.WithLabelValues(connectorID).Inc()
	m.smsSentDuration.WithLabelValues(connectorID).Observe(latency.Seconds())
}

func (m *PrometheusMetricsRecorder) RecordMessageFailed(tenantID, connectorID, errorCode string) {
	m.smsFailedTotal.WithLabelValues(connectorID, errorCode).Inc()
}

func (m *PrometheusMetricsRecorder) RecordDLRReceived(tenantID, status string) {
	m.dlrReceivedTotal.WithLabelValues("unknown", status).Inc()
}

func (m *PrometheusMetricsRecorder) RecordRetry(tenantID string, attempt int) {
	m.smsRetryTotal.WithLabelValues("unknown").Inc()
}

// RecordWorkerRestart increments the worker restart counter.
func (m *PrometheusMetricsRecorder) RecordWorkerRestart(workerType string) {
	m.workerRestarts.WithLabelValues(workerType).Inc()
}

// RecordCircuitBreakerStateChange records a circuit breaker state transition.
func (m *PrometheusMetricsRecorder) RecordCircuitBreakerStateChange(connectorID, state string) {
	m.circuitBreakerStateChanges.WithLabelValues(connectorID, state).Inc()
}

// RecordBatchDuration observes a batch processing duration.
func (m *PrometheusMetricsRecorder) RecordBatchDuration(worker string, duration time.Duration) {
	m.batchDuration.WithLabelValues(worker).Observe(duration.Seconds())
}

// SetQueueDepth sets the current queue depth gauge.
func (m *PrometheusMetricsRecorder) SetQueueDepth(depth int64) {
	m.queueDepth.Set(float64(depth))
}

// ensure interface compliance
var _ domain.MetricsRecorder = (*PrometheusMetricsRecorder)(nil)
