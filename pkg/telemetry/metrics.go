package telemetry

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics holds all Prometheus metrics for the Pipeline.
type Metrics struct {
	// Event Bus Metrics
	EventsPublished    *prometheus.CounterVec
	EventsDropped      *prometheus.CounterVec
	PublishDuration    *prometheus.HistogramVec
	FilterDuration     *prometheus.HistogramVec
	SendDuration       *prometheus.HistogramVec
	SendBlocked        *prometheus.CounterVec
	SubscribersTotal   *prometheus.GaugeVec
	BufferUsage        *prometheus.GaugeVec
	BufferSize         *prometheus.GaugeVec

	// Engine Metrics
	EngineOperations *prometheus.CounterVec
	EngineDuration   *prometheus.HistogramVec
}

var (
	defaultMetrics *Metrics
)

// InitMetrics initializes the Prometheus metrics.
// This should be called once at startup before any metrics are recorded.
func InitMetrics(registry prometheus.Registerer) *Metrics {
	if registry == nil {
		registry = prometheus.DefaultRegisterer
	}

	// Define histogram buckets optimized for microsecond-to-millisecond latencies
	// Buckets: 1µs, 2µs, 5µs, 10µs, 20µs, 50µs, 100µs, 200µs, 500µs, 1ms, 2ms, 5ms, 10ms, 20ms, 50ms, 100ms
	latencyBuckets := []float64{
		0.000001, // 1µs
		0.000002, // 2µs
		0.000005, // 5µs
		0.00001,  // 10µs
		0.00002,  // 20µs
		0.00005,  // 50µs
		0.0001,   // 100µs
		0.0002,   // 200µs
		0.0005,   // 500µs
		0.001,    // 1ms
		0.002,    // 2ms
		0.005,    // 5ms
		0.01,     // 10ms
		0.02,     // 20ms
		0.05,     // 50ms
		0.1,      // 100ms
	}

	m := &Metrics{
		// Event Bus Metrics
		EventsPublished: promauto.With(registry).NewCounterVec(
			prometheus.CounterOpts{
				Name: "pipeline_events_published_total",
				Help: "Total number of events published to the bus",
			},
			[]string{"bus", "event_type"},
		),

		EventsDropped: promauto.With(registry).NewCounterVec(
			prometheus.CounterOpts{
				Name: "pipeline_events_dropped_total",
				Help: "Total number of events dropped due to slow subscribers",
			},
			[]string{"bus", "event_type", "subscription_id"},
		),

		PublishDuration: promauto.With(registry).NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "pipeline_event_publish_duration_seconds",
				Help:    "Time taken to publish an event (including filtering and sending to all subscribers)",
				Buckets: latencyBuckets,
			},
			[]string{"bus", "event_type"},
		),

		FilterDuration: promauto.With(registry).NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "pipeline_event_filter_duration_seconds",
				Help:    "Time taken to filter events against subscription criteria",
				Buckets: latencyBuckets,
			},
			[]string{"bus", "subscription_id"},
		),

		SendDuration: promauto.With(registry).NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "pipeline_event_send_duration_seconds",
				Help:    "Time taken to send event to subscription channel (includes blocking time)",
				Buckets: latencyBuckets,
			},
			[]string{"bus", "subscription_id", "result"},
		),

		SendBlocked: promauto.With(registry).NewCounterVec(
			prometheus.CounterOpts{
				Name: "pipeline_event_send_blocked_total",
				Help: "Number of times event send blocked waiting for channel space",
			},
			[]string{"bus", "subscription_id"},
		),

		SubscribersTotal: promauto.With(registry).NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "pipeline_subscribers_total",
				Help: "Current number of active subscribers",
			},
			[]string{"bus"},
		),

		BufferUsage: promauto.With(registry).NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "pipeline_subscription_buffer_usage",
				Help: "Current number of events in subscription buffer",
			},
			[]string{"bus", "subscription_id"},
		),

		BufferSize: promauto.With(registry).NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "pipeline_subscription_buffer_size",
				Help: "Maximum capacity of subscription buffer",
			},
			[]string{"bus", "subscription_id"},
		),

		// Engine Metrics
		EngineOperations: promauto.With(registry).NewCounterVec(
			prometheus.CounterOpts{
				Name: "pipeline_engine_operations_total",
				Help: "Total number of engine operations",
			},
			[]string{"operation", "status"},
		),

		EngineDuration: promauto.With(registry).NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "pipeline_engine_operation_duration_seconds",
				Help:    "Time taken for engine operations",
				Buckets: latencyBuckets,
			},
			[]string{"operation"},
		),
	}

	defaultMetrics = m
	return m
}

// Default returns the default metrics instance.
// If InitMetrics hasn't been called, it will initialize with the default registry.
func Default() *Metrics {
	if defaultMetrics == nil {
		return InitMetrics(nil)
	}
	return defaultMetrics
}

// Timer is a helper for timing operations.
type Timer struct {
	start time.Time
}

// NewTimer creates a new timer starting now.
func NewTimer() *Timer {
	return &Timer{start: time.Now()}
}

// Observe records the elapsed time in seconds to the given histogram.
func (t *Timer) Observe(histogram prometheus.Observer) {
	histogram.Observe(time.Since(t.start).Seconds())
}

// ObserveWithLabels records the elapsed time to a histogram with labels.
func (t *Timer) ObserveWithLabels(histogram *prometheus.HistogramVec, labels prometheus.Labels) {
	histogram.With(labels).Observe(time.Since(t.start).Seconds())
}

// Elapsed returns the time elapsed since the timer started.
func (t *Timer) Elapsed() time.Duration {
	return time.Since(t.start)
}
