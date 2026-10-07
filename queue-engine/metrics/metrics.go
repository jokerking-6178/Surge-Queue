package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Prometheus metrics exposed at /metrics. These feed into Grafana dashboards
// and HPA autoscaling decisions.
var (
	// Current number of users waiting in the queue.
	QueueDepth = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "surge_queue",
		Name:      "depth",
		Help:      "Number of users currently waiting in the queue",
	})

	// Number of users who have been admitted and are actively using the backend.
	AdmittedActive = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "surge_queue",
		Name:      "admitted_active",
		Help:      "Number of admitted users with active sessions",
	})

	// Total admitted since startup — a counter.
	AdmittedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "surge_queue",
		Name:      "admitted_total",
		Help:      "Total users admitted through the queue",
	})

	// Total users who entered the queue.
	EnteredTotal = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "surge_queue",
		Name:      "entered_total",
		Help:      "Total users who entered the queue",
	})

	// Total users who abandoned (entry expired without admission).
	AbandonedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "surge_queue",
		Name:      "abandoned_total",
		Help:      "Total users who abandoned the queue",
	})

	// Histogram of time-to-admission (seconds).
	WaitTimeSeconds = promauto.NewHistogram(prometheus.HistogramOpts{
		Namespace: "surge_queue",
		Name:      "wait_time_seconds",
		Help:      "Time from entering queue to admission",
		Buckets:   []float64{1, 5, 10, 30, 60, 120, 300, 600},
	})

	// HTTP request latency.
	HTTPRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "surge_queue",
		Name:      "http_request_duration_seconds",
		Help:      "HTTP request latency",
		Buckets:   []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1},
	}, []string{"endpoint", "method"})
)
