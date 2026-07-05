package connector

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/raghna/fury-sms-gateway/internal/domain"
)

// PrometheusMetricsRecorder implements MetricsRecorder using Prometheus client.
// Create via NewPrometheusMetricsRecorder(namespace, subsystem).
type PrometheusMetricsRecorder struct {
	smsSentTotal    *prometheus.CounterVec
	smsFailedTotal  *prometheus.CounterVec
	smsRetryTotal   *prometheus.CounterVec
	dlrReceivedTotal *prometheus.CounterVec

	smsSentDuration  *prometheus.HistogramVec
	batchDuration    *prometheus.HistogramVec
	queueDepth       prometheus.Gauge
	workerRestarts   *prometheus.GaugeVec
}

// NewPrometheusMetricsRecorder creates and registers all Prometheus metrics.
// Use subsystem="queue" for the queue worker, "retry" for retry engine, etc.
func NewPrometheusMetricsRecorder(namespace, subsystem string) *PrometheusMetricsRecorder {
	return &PrometheusMetricsRecorder{
		smsSentTotal: promauto.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "sms_sent_total",
			Help:      "Total number of SMS messages sent successfully.",
		}, []string{"tenant_id", "connector_id"}),

		smsFailedTotal: promauto.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "sms_failed_total",
			Help:      "Total number of SMS message send failures.",
		}, []string{"tenant_id", "connector_id", "error_code"}),

		smsRetryTotal: promauto.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "sms_retry_total",
			Help:      "Total number of SMS message retries.",
		}, []string{"tenant_id"}),

		dlrReceivedTotal: promauto.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "dlr_received_total",
			Help:      "Total number of DLR callbacks received.",
		}, []string{"tenant_id", "status"}),

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
		}, []string{"worker"}),

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
	}
}

func (m *PrometheusMetricsRecorder) RecordMessageSent(tenantID, connectorID string, parts int, latency time.Duration) {
	m.smsSentTotal.WithLabelValues(tenantID, connectorID).Inc()
	m.smsSentDuration.WithLabelValues(connectorID).Observe(latency.Seconds())
}

func (m *PrometheusMetricsRecorder) RecordMessageFailed(tenantID, connectorID, errorCode string) {
	m.smsFailedTotal.WithLabelValues(tenantID, connectorID, errorCode).Inc()
}

func (m *PrometheusMetricsRecorder) RecordDLRReceived(tenantID, status string) {
	m.dlrReceivedTotal.WithLabelValues(tenantID, status).Inc()
}

func (m *PrometheusMetricsRecorder) RecordRetry(tenantID string, attempt int) {
	m.smsRetryTotal.WithLabelValues(tenantID).Inc()
}

// RecordWorkerRestart increments the worker restart counter.
func (m *PrometheusMetricsRecorder) RecordWorkerRestart(workerType string) {
	m.workerRestarts.WithLabelValues(workerType).Inc()
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
