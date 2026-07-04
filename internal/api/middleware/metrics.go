package middleware

import (
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	httpRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "fury_http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"method", "path", "status"},
	)

	httpRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "fury_http_request_duration_seconds",
			Help:    "HTTP request duration in seconds",
			Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
		},
		[]string{"method", "path"},
	)

	httpRequestsInFlight = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "fury_http_requests_in_flight",
			Help: "Current number of HTTP requests in flight",
		},
	)
)

// MetricsMiddleware collects Prometheus metrics for HTTP requests.
func MetricsMiddleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		httpRequestsInFlight.Inc()
		defer httpRequestsInFlight.Dec()

		start := time.Now()

		err := c.Next()

		duration := time.Since(start)
		status := strconv.Itoa(c.Response().StatusCode())

		httpRequestsTotal.WithLabelValues(c.Method(), c.Route().Path, status).Inc()
		httpRequestDuration.WithLabelValues(c.Method(), c.Route().Path).Observe(duration.Seconds())

		return err
	}
}
