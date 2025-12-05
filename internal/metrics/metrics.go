package metrics

import (
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Metrics holds all Prometheus metrics for Caddystat.
type Metrics struct {
	// HTTP server metrics
	HTTPRequestsTotal   *prometheus.CounterVec
	HTTPRequestDuration *prometheus.HistogramVec

	// SSE metrics
	SSESubscribersGauge prometheus.GaugeFunc

	// Ingestion metrics
	IngestRequestsTotal prometheus.Counter
	IngestErrorsTotal   prometheus.Counter
	IngestDuration      prometheus.Histogram
	LastIngestTimestamp prometheus.Gauge
	IngestBytesTotal    prometheus.Counter

	// Database metrics
	DBSizeBytes      prometheus.GaugeFunc
	DBRequestsTotal  prometheus.GaugeFunc
	DBSessionsTotal  prometheus.GaugeFunc
	DBRollupsHourly  prometheus.GaugeFunc
	DBRollupsDaily   prometheus.GaugeFunc
	DBImportProgress prometheus.GaugeFunc
}

// DBStats represents database statistics returned by the stats provider function.
type DBStats struct {
	RequestsCount       int64
	SessionsCount       int64
	RollupsHourlyCount  int64
	RollupsDailyCount   int64
	ImportProgressCount int64
}

// cachedDBStats caches the result of dbStatsFunc for all gauge funcs in a single scrape.
// Since Prometheus GaugeFuncs are called individually, we cache results for 1 second
// to avoid redundant database queries during a single scrape.
type cachedDBStats struct {
	mu          sync.RWMutex
	getStats    func() DBStats
	cachedStats DBStats
	cachedAt    int64 // Unix nanoseconds
}

func newCachedDBStats(getStats func() DBStats) *cachedDBStats {
	return &cachedDBStats{getStats: getStats}
}

func (c *cachedDBStats) get() DBStats {
	now := time.Now().UnixNano()

	// Fast path: check if cache is still valid with read lock
	c.mu.RLock()
	if now-c.cachedAt <= int64(time.Second) {
		stats := c.cachedStats
		c.mu.RUnlock()
		return stats
	}
	c.mu.RUnlock()

	// Slow path: refresh cache with write lock
	c.mu.Lock()
	defer c.mu.Unlock()
	// Double-check after acquiring write lock
	if now-c.cachedAt > int64(time.Second) {
		c.cachedStats = c.getStats()
		c.cachedAt = now
	}
	return c.cachedStats
}

// New creates and registers all Prometheus metrics.
// The dbStatsFunc is called to retrieve database statistics (cached for 1 second).
func New(
	sseClientCountFunc func() int,
	dbSizeFunc func() int64,
	dbStatsFunc func() DBStats,
) *Metrics {
	cache := newCachedDBStats(dbStatsFunc)

	m := &Metrics{
		HTTPRequestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "caddystat",
				Subsystem: "http",
				Name:      "requests_total",
				Help:      "Total number of HTTP requests handled",
			},
			[]string{"method", "path", "status"},
		),
		HTTPRequestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "caddystat",
				Subsystem: "http",
				Name:      "request_duration_seconds",
				Help:      "HTTP request duration in seconds",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{"method", "path"},
		),
		SSESubscribersGauge: prometheus.NewGaugeFunc(
			prometheus.GaugeOpts{
				Namespace: "caddystat",
				Subsystem: "sse",
				Name:      "subscribers",
				Help:      "Current number of SSE subscribers",
			},
			func() float64 {
				return float64(sseClientCountFunc())
			},
		),
		IngestRequestsTotal: prometheus.NewCounter(
			prometheus.CounterOpts{
				Namespace: "caddystat",
				Subsystem: "ingest",
				Name:      "requests_total",
				Help:      "Total number of log entries ingested",
			},
		),
		IngestErrorsTotal: prometheus.NewCounter(
			prometheus.CounterOpts{
				Namespace: "caddystat",
				Subsystem: "ingest",
				Name:      "errors_total",
				Help:      "Total number of log parsing errors",
			},
		),
		IngestDuration: prometheus.NewHistogram(
			prometheus.HistogramOpts{
				Namespace: "caddystat",
				Subsystem: "ingest",
				Name:      "duration_seconds",
				Help:      "Duration of log entry processing in seconds",
				Buckets:   []float64{.0001, .0005, .001, .005, .01, .05, .1, .5, 1},
			},
		),
		LastIngestTimestamp: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: "caddystat",
				Subsystem: "ingest",
				Name:      "last_timestamp_seconds",
				Help:      "Unix timestamp of the last ingested log entry",
			},
		),
		IngestBytesTotal: prometheus.NewCounter(
			prometheus.CounterOpts{
				Namespace: "caddystat",
				Subsystem: "ingest",
				Name:      "bytes_total",
				Help:      "Total bytes processed from log entries",
			},
		),
		DBSizeBytes: prometheus.NewGaugeFunc(
			prometheus.GaugeOpts{
				Namespace: "caddystat",
				Subsystem: "db",
				Name:      "size_bytes",
				Help:      "Size of the SQLite database in bytes",
			},
			func() float64 {
				return float64(dbSizeFunc())
			},
		),
		DBRequestsTotal: prometheus.NewGaugeFunc(
			prometheus.GaugeOpts{
				Namespace: "caddystat",
				Subsystem: "db",
				Name:      "requests_total",
				Help:      "Total number of rows in the requests table",
			},
			func() float64 {
				return float64(cache.get().RequestsCount)
			},
		),
		DBSessionsTotal: prometheus.NewGaugeFunc(
			prometheus.GaugeOpts{
				Namespace: "caddystat",
				Subsystem: "db",
				Name:      "sessions_total",
				Help:      "Total number of active sessions",
			},
			func() float64 {
				return float64(cache.get().SessionsCount)
			},
		),
		DBRollupsHourly: prometheus.NewGaugeFunc(
			prometheus.GaugeOpts{
				Namespace: "caddystat",
				Subsystem: "db",
				Name:      "rollups_hourly_total",
				Help:      "Total number of rows in the hourly rollups table",
			},
			func() float64 {
				return float64(cache.get().RollupsHourlyCount)
			},
		),
		DBRollupsDaily: prometheus.NewGaugeFunc(
			prometheus.GaugeOpts{
				Namespace: "caddystat",
				Subsystem: "db",
				Name:      "rollups_daily_total",
				Help:      "Total number of rows in the daily rollups table",
			},
			func() float64 {
				return float64(cache.get().RollupsDailyCount)
			},
		),
		DBImportProgress: prometheus.NewGaugeFunc(
			prometheus.GaugeOpts{
				Namespace: "caddystat",
				Subsystem: "db",
				Name:      "import_progress_total",
				Help:      "Total number of tracked import progress entries",
			},
			func() float64 {
				return float64(cache.get().ImportProgressCount)
			},
		),
	}

	return m
}

// Register registers all metrics with the default Prometheus registry.
func (m *Metrics) Register() error {
	collectors := []prometheus.Collector{
		m.HTTPRequestsTotal,
		m.HTTPRequestDuration,
		m.SSESubscribersGauge,
		m.IngestRequestsTotal,
		m.IngestErrorsTotal,
		m.IngestDuration,
		m.LastIngestTimestamp,
		m.IngestBytesTotal,
		m.DBSizeBytes,
		m.DBRequestsTotal,
		m.DBSessionsTotal,
		m.DBRollupsHourly,
		m.DBRollupsDaily,
		m.DBImportProgress,
	}

	for _, c := range collectors {
		if err := prometheus.Register(c); err != nil {
			return err
		}
	}
	return nil
}

// RecordHTTPRequest records an HTTP request metric.
func (m *Metrics) RecordHTTPRequest(method, path, status string, duration float64) {
	m.HTTPRequestsTotal.WithLabelValues(method, path, status).Inc()
	m.HTTPRequestDuration.WithLabelValues(method, path).Observe(duration)
}

// RecordIngest records a successful log ingestion.
func (m *Metrics) RecordIngest(durationSec float64, bytes int64) {
	m.IngestRequestsTotal.Inc()
	m.IngestDuration.Observe(durationSec)
	m.IngestBytesTotal.Add(float64(bytes))
}

// RecordIngestError records a log parsing error.
func (m *Metrics) RecordIngestError() {
	m.IngestErrorsTotal.Inc()
}

// SetLastIngestTimestamp sets the timestamp of the last ingested entry.
func (m *Metrics) SetLastIngestTimestamp(ts float64) {
	m.LastIngestTimestamp.Set(ts)
}
