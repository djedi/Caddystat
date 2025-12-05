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
	SSEDroppedMessages  prometheus.Counter

	// Ingestion metrics
	IngestRequestsTotal prometheus.Counter
	IngestErrorsTotal   prometheus.Counter
	IngestDuration      prometheus.Histogram
	LastIngestTimestamp prometheus.Gauge
	IngestBytesTotal    prometheus.Counter

	// Bot ingestion metrics
	IngestBotRequestsTotal *prometheus.CounterVec
	IngestBotBytesTotal    *prometheus.CounterVec

	// Database metrics
	DBSizeBytes      prometheus.GaugeFunc
	DBRequestsTotal  prometheus.GaugeFunc
	DBSessionsTotal  prometheus.GaugeFunc
	DBRollupsHourly  prometheus.GaugeFunc
	DBRollupsDaily   prometheus.GaugeFunc
	DBImportProgress prometheus.GaugeFunc

	// GeoIP cache metrics
	GeoCacheSize     prometheus.GaugeFunc
	GeoCacheHits     prometheus.GaugeFunc
	GeoCacheMisses   prometheus.GaugeFunc
	GeoCacheEvicts   prometheus.GaugeFunc
	GeoCacheHitRate  prometheus.GaugeFunc
	GeoCacheCapacity prometheus.GaugeFunc
}

// DBStats represents database statistics returned by the stats provider function.
type DBStats struct {
	RequestsCount       int64
	SessionsCount       int64
	RollupsHourlyCount  int64
	RollupsDailyCount   int64
	ImportProgressCount int64
}

// GeoCacheStats represents geo cache statistics returned by the stats provider function.
type GeoCacheStats struct {
	Size     int
	Capacity int
	Hits     uint64
	Misses   uint64
	Evicts   uint64
	HitRate  float64
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
// The geoCacheStatsFunc is optional and can be nil if no geo cache is configured.
func New(
	sseClientCountFunc func() int,
	dbSizeFunc func() int64,
	dbStatsFunc func() DBStats,
	geoCacheStatsFunc func() *GeoCacheStats,
) *Metrics {
	cache := newCachedDBStats(dbStatsFunc)

	// Helper to safely get geo cache stats (handles nil function)
	getGeoStats := func() GeoCacheStats {
		if geoCacheStatsFunc == nil {
			return GeoCacheStats{}
		}
		stats := geoCacheStatsFunc()
		if stats == nil {
			return GeoCacheStats{}
		}
		return *stats
	}

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
		SSEDroppedMessages: prometheus.NewCounter(
			prometheus.CounterOpts{
				Namespace: "caddystat",
				Subsystem: "sse",
				Name:      "dropped_messages_total",
				Help:      "Total number of SSE messages dropped due to slow clients",
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
		IngestBotRequestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "caddystat",
				Subsystem: "ingest",
				Name:      "bot_requests_total",
				Help:      "Total number of bot requests ingested, by intent",
			},
			[]string{"intent"},
		),
		IngestBotBytesTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "caddystat",
				Subsystem: "ingest",
				Name:      "bot_bytes_total",
				Help:      "Total bytes from bot requests, by intent",
			},
			[]string{"intent"},
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
		GeoCacheSize: prometheus.NewGaugeFunc(
			prometheus.GaugeOpts{
				Namespace: "caddystat",
				Subsystem: "geo",
				Name:      "cache_size",
				Help:      "Current number of entries in the geo cache",
			},
			func() float64 {
				return float64(getGeoStats().Size)
			},
		),
		GeoCacheCapacity: prometheus.NewGaugeFunc(
			prometheus.GaugeOpts{
				Namespace: "caddystat",
				Subsystem: "geo",
				Name:      "cache_capacity",
				Help:      "Maximum capacity of the geo cache",
			},
			func() float64 {
				return float64(getGeoStats().Capacity)
			},
		),
		GeoCacheHits: prometheus.NewGaugeFunc(
			prometheus.GaugeOpts{
				Namespace: "caddystat",
				Subsystem: "geo",
				Name:      "cache_hits_total",
				Help:      "Total number of geo cache hits",
			},
			func() float64 {
				return float64(getGeoStats().Hits)
			},
		),
		GeoCacheMisses: prometheus.NewGaugeFunc(
			prometheus.GaugeOpts{
				Namespace: "caddystat",
				Subsystem: "geo",
				Name:      "cache_misses_total",
				Help:      "Total number of geo cache misses",
			},
			func() float64 {
				return float64(getGeoStats().Misses)
			},
		),
		GeoCacheEvicts: prometheus.NewGaugeFunc(
			prometheus.GaugeOpts{
				Namespace: "caddystat",
				Subsystem: "geo",
				Name:      "cache_evictions_total",
				Help:      "Total number of geo cache evictions due to capacity",
			},
			func() float64 {
				return float64(getGeoStats().Evicts)
			},
		),
		GeoCacheHitRate: prometheus.NewGaugeFunc(
			prometheus.GaugeOpts{
				Namespace: "caddystat",
				Subsystem: "geo",
				Name:      "cache_hit_rate",
				Help:      "Geo cache hit rate (0.0 to 1.0)",
			},
			func() float64 {
				return getGeoStats().HitRate
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
		m.SSEDroppedMessages,
		m.IngestRequestsTotal,
		m.IngestErrorsTotal,
		m.IngestDuration,
		m.LastIngestTimestamp,
		m.IngestBytesTotal,
		m.IngestBotRequestsTotal,
		m.IngestBotBytesTotal,
		m.DBSizeBytes,
		m.DBRequestsTotal,
		m.DBSessionsTotal,
		m.DBRollupsHourly,
		m.DBRollupsDaily,
		m.DBImportProgress,
		m.GeoCacheSize,
		m.GeoCacheCapacity,
		m.GeoCacheHits,
		m.GeoCacheMisses,
		m.GeoCacheEvicts,
		m.GeoCacheHitRate,
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

// RecordBotIngest records a bot request ingestion with intent label.
func (m *Metrics) RecordBotIngest(intent string, bytes int64) {
	if intent == "" {
		intent = "unknown"
	}
	m.IngestBotRequestsTotal.WithLabelValues(intent).Inc()
	m.IngestBotBytesTotal.WithLabelValues(intent).Add(float64(bytes))
}

// RecordSSEDropped records that an SSE message was dropped due to a slow client.
func (m *Metrics) RecordSSEDropped() {
	m.SSEDroppedMessages.Inc()
}
