package extdb

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics holds database operation metrics.
type Metrics struct {
	queryDuration   *prometheus.HistogramVec
	queriesTotal    *prometheus.CounterVec
	queryErrors     *prometheus.CounterVec
	inflightQueries *prometheus.GaugeVec
	rowsAffected    *prometheus.HistogramVec
	connectionPool  *prometheus.GaugeVec
}

// NewMetrics creates a new instance of database Metrics.
// It registers the metrics with the provided registerer.
func NewMetrics(reg prometheus.Registerer, durationBuckets []float64) *Metrics {
	if durationBuckets == nil {
		durationBuckets = []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}
	}

	const maxBucketNumber = 256
	const bucketFactor = 1.1

	return &Metrics{
		queryDuration: promauto.With(reg).NewHistogramVec(
			prometheus.HistogramOpts{
				Subsystem:                      "db",
				Name:                           "query_duration_seconds",
				Help:                           "Histogram of database query durations.",
				Buckets:                        durationBuckets,
				NativeHistogramBucketFactor:    bucketFactor,
				NativeHistogramMaxBucketNumber: maxBucketNumber,
			},
			[]string{"operation", "table", "status"},
		),

		queriesTotal: promauto.With(reg).NewCounterVec(
			prometheus.CounterOpts{
				Subsystem: "db",
				Name:      "queries_total",
				Help:      "Total number of database queries.",
			},
			[]string{"operation", "table", "status"},
		),

		queryErrors: promauto.With(reg).NewCounterVec(
			prometheus.CounterOpts{
				Subsystem: "db",
				Name:      "query_errors_total",
				Help:      "Total number of database query errors.",
			},
			[]string{"operation", "table", "error_type"},
		),

		inflightQueries: promauto.With(reg).NewGaugeVec(
			prometheus.GaugeOpts{
				Subsystem: "db",
				Name:      "inflight_queries",
				Help:      "Current number of in-flight database queries.",
			},
			[]string{"operation", "table"},
		),

		rowsAffected: promauto.With(reg).NewHistogramVec(
			prometheus.HistogramOpts{
				Subsystem:                      "db",
				Name:                           "rows_affected",
				Help:                           "Histogram of rows affected by database operations.",
				Buckets:                        prometheus.ExponentialBuckets(1, 2, 12),
				NativeHistogramBucketFactor:    bucketFactor,
				NativeHistogramMaxBucketNumber: maxBucketNumber,
			},
			[]string{"operation", "table"},
		),

		connectionPool: promauto.With(reg).NewGaugeVec(
			prometheus.GaugeOpts{
				Subsystem: "db",
				Name:      "connection_pool",
				Help:      "Database connection pool statistics.",
			},
			[]string{"state"}, // open, idle, in_use, max
		),
	}
}

// RecordQuery records metrics for a database query.
func (m *Metrics) RecordQuery(operation, table, status string, duration float64) {
	m.queryDuration.WithLabelValues(operation, table, status).Observe(duration)
	m.queriesTotal.WithLabelValues(operation, table, status).Inc()
}

// RecordError records a database error.
func (m *Metrics) RecordError(operation, table, errorType string) {
	m.queryErrors.WithLabelValues(operation, table, errorType).Inc()
}

// RecordRowsAffected records the number of rows affected by an operation.
func (m *Metrics) RecordRowsAffected(operation, table string, rows float64) {
	m.rowsAffected.WithLabelValues(operation, table).Observe(rows)
}

// IncInflight increments the in-flight queries counter.
func (m *Metrics) IncInflight(operation, table string) {
	m.inflightQueries.WithLabelValues(operation, table).Inc()
}

// DecInflight decrements the in-flight queries counter.
func (m *Metrics) DecInflight(operation, table string) {
	m.inflightQueries.WithLabelValues(operation, table).Dec()
}

// SetConnectionPool sets connection pool statistics.
func (m *Metrics) SetConnectionPool(open, idle, inUse, maxOpen float64) {
	m.connectionPool.WithLabelValues("open").Set(open)
	m.connectionPool.WithLabelValues("idle").Set(idle)
	m.connectionPool.WithLabelValues("in_use").Set(inUse)
	m.connectionPool.WithLabelValues("max").Set(maxOpen)
}
