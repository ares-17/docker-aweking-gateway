package gateway

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// RequestsTotal counts total HTTP requests passing through the gateway.
	RequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gateway_requests_total",
			Help: "Total number of HTTP requests processed, including proxy and loading pages.",
		},
		[]string{"container", "status_code"},
	)

	// RequestDuration tracking the time spent processing proxy requests.
	RequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "gateway_request_duration_seconds",
			Help:    "Duration of HTTP requests to container in seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"container"},
	)

	// StartsTotal traces container awakenings.
	StartsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gateway_starts_total",
			Help: "Total container start attempts.",
		},
		[]string{"container", "result"}, // result: "success" or "error"
	)

	// StartDuration tracks how long the awakening process takes (docker start + TCP probe).
	StartDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "gateway_start_duration_seconds",
			Help:    "Time taken for an awakening to successfully complete.",
			Buckets: []float64{0.5, 1, 2.5, 5, 10, 15, 30, 60, 120},
		},
		[]string{"container"},
	)

	// IdleStopsTotal tracks the idle shutdown watcher.
	IdleStopsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gateway_idle_stops_total",
			Help: "Total times a container was stopped due to idle timeout.",
		},
		[]string{"container"},
	)
)

// RecordRequest is a thread-safe helper to bump request metrics.
func RecordRequest(containerName string, statusCode string, durationSec float64) {
	RequestsTotal.WithLabelValues(containerName, statusCode).Inc()
	RequestDuration.WithLabelValues(containerName).Observe(durationSec)
}

// RecordStart is a helper to bump start attempts metrics.
func RecordStart(containerName string, success bool, durationSec float64) {
	result := "error"
	if success {
		result = "success"
		StartDuration.WithLabelValues(containerName).Observe(durationSec)
	}
	StartsTotal.WithLabelValues(containerName, result).Inc()
}

// RecordIdleStop bumps the idle stop counter.
func RecordIdleStop(containerName string) {
	IdleStopsTotal.WithLabelValues(containerName).Inc()
}
