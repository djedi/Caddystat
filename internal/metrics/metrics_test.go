package metrics

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestMetrics_New(t *testing.T) {
	sseCount := 5
	dbSize := int64(1024)
	dbStats := DBStats{
		RequestsCount:       100,
		SessionsCount:       2,
		RollupsHourlyCount:  24,
		RollupsDailyCount:   7,
		ImportProgressCount: 3,
	}

	m := New(
		func() int { return sseCount },
		func() int64 { return dbSize },
		func() DBStats { return dbStats },
		nil, // no geo cache
	)

	if m == nil {
		t.Fatal("expected non-nil Metrics")
	}
	if m.HTTPRequestsTotal == nil {
		t.Error("HTTPRequestsTotal should not be nil")
	}
	if m.HTTPRequestDuration == nil {
		t.Error("HTTPRequestDuration should not be nil")
	}
	if m.IngestRequestsTotal == nil {
		t.Error("IngestRequestsTotal should not be nil")
	}
	if m.IngestErrorsTotal == nil {
		t.Error("IngestErrorsTotal should not be nil")
	}
	if m.IngestDuration == nil {
		t.Error("IngestDuration should not be nil")
	}
	if m.LastIngestTimestamp == nil {
		t.Error("LastIngestTimestamp should not be nil")
	}
	if m.IngestBytesTotal == nil {
		t.Error("IngestBytesTotal should not be nil")
	}
}

func TestMetrics_RecordHTTPRequest(t *testing.T) {
	// Create a new registry to avoid conflicts with other tests
	reg := prometheus.NewRegistry()

	m := &Metrics{
		HTTPRequestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "test",
				Name:      "http_requests_total",
			},
			[]string{"method", "path", "status"},
		),
		HTTPRequestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "test",
				Name:      "http_request_duration_seconds",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{"method", "path"},
		),
	}
	reg.MustRegister(m.HTTPRequestsTotal, m.HTTPRequestDuration)

	m.RecordHTTPRequest("GET", "/api/stats/summary", "200", 0.05)
	m.RecordHTTPRequest("GET", "/api/stats/summary", "200", 0.1)
	m.RecordHTTPRequest("POST", "/api/auth/login", "401", 0.01)

	// Verify counter
	count := testutil.ToFloat64(m.HTTPRequestsTotal.WithLabelValues("GET", "/api/stats/summary", "200"))
	if count != 2 {
		t.Errorf("expected 2 requests, got %v", count)
	}
	count = testutil.ToFloat64(m.HTTPRequestsTotal.WithLabelValues("POST", "/api/auth/login", "401"))
	if count != 1 {
		t.Errorf("expected 1 request, got %v", count)
	}
}

func TestMetrics_RecordIngest(t *testing.T) {
	reg := prometheus.NewRegistry()

	m := &Metrics{
		IngestRequestsTotal: prometheus.NewCounter(
			prometheus.CounterOpts{
				Namespace: "test",
				Name:      "ingest_requests_total",
			},
		),
		IngestDuration: prometheus.NewHistogram(
			prometheus.HistogramOpts{
				Namespace: "test",
				Name:      "ingest_duration_seconds",
				Buckets:   []float64{.001, .01, .1},
			},
		),
		IngestBytesTotal: prometheus.NewCounter(
			prometheus.CounterOpts{
				Namespace: "test",
				Name:      "ingest_bytes_total",
			},
		),
	}
	reg.MustRegister(m.IngestRequestsTotal, m.IngestDuration, m.IngestBytesTotal)

	m.RecordIngest(0.005, 1024)
	m.RecordIngest(0.002, 2048)
	m.RecordIngest(0.001, 512)

	// Verify counter
	count := testutil.ToFloat64(m.IngestRequestsTotal)
	if count != 3 {
		t.Errorf("expected 3 ingests, got %v", count)
	}

	// Verify bytes
	bytes := testutil.ToFloat64(m.IngestBytesTotal)
	if bytes != 3584 { // 1024 + 2048 + 512
		t.Errorf("expected 3584 bytes, got %v", bytes)
	}
}

func TestMetrics_RecordIngestError(t *testing.T) {
	reg := prometheus.NewRegistry()

	m := &Metrics{
		IngestErrorsTotal: prometheus.NewCounter(
			prometheus.CounterOpts{
				Namespace: "test",
				Name:      "ingest_errors_total",
			},
		),
	}
	reg.MustRegister(m.IngestErrorsTotal)

	m.RecordIngestError()
	m.RecordIngestError()

	count := testutil.ToFloat64(m.IngestErrorsTotal)
	if count != 2 {
		t.Errorf("expected 2 errors, got %v", count)
	}
}

func TestMetrics_SetLastIngestTimestamp(t *testing.T) {
	reg := prometheus.NewRegistry()

	m := &Metrics{
		LastIngestTimestamp: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: "test",
				Name:      "last_ingest_timestamp",
			},
		),
	}
	reg.MustRegister(m.LastIngestTimestamp)

	ts := float64(1704067200) // 2024-01-01 00:00:00 UTC
	m.SetLastIngestTimestamp(ts)

	val := testutil.ToFloat64(m.LastIngestTimestamp)
	if val != ts {
		t.Errorf("expected timestamp %v, got %v", ts, val)
	}
}

func TestMetrics_GaugeFuncs(t *testing.T) {
	sseCount := 3
	dbSize := int64(2048)
	dbStats := DBStats{
		RequestsCount:       500,
		SessionsCount:       5,
		RollupsHourlyCount:  48,
		RollupsDailyCount:   14,
		ImportProgressCount: 2,
	}

	m := New(
		func() int { return sseCount },
		func() int64 { return dbSize },
		func() DBStats { return dbStats },
		nil, // no geo cache
	)

	// Test SSE subscribers gauge
	val := testutil.ToFloat64(m.SSESubscribersGauge)
	if val != float64(sseCount) {
		t.Errorf("SSE subscribers: expected %d, got %v", sseCount, val)
	}

	// Test DB size gauge
	val = testutil.ToFloat64(m.DBSizeBytes)
	if val != float64(dbSize) {
		t.Errorf("DB size: expected %d, got %v", dbSize, val)
	}

	// Test DB requests gauge
	val = testutil.ToFloat64(m.DBRequestsTotal)
	if val != float64(dbStats.RequestsCount) {
		t.Errorf("DB requests: expected %d, got %v", dbStats.RequestsCount, val)
	}

	// Test DB sessions gauge
	val = testutil.ToFloat64(m.DBSessionsTotal)
	if val != float64(dbStats.SessionsCount) {
		t.Errorf("DB sessions: expected %d, got %v", dbStats.SessionsCount, val)
	}

	// Test rollups hourly gauge
	val = testutil.ToFloat64(m.DBRollupsHourly)
	if val != float64(dbStats.RollupsHourlyCount) {
		t.Errorf("Rollups hourly: expected %d, got %v", dbStats.RollupsHourlyCount, val)
	}

	// Test rollups daily gauge
	val = testutil.ToFloat64(m.DBRollupsDaily)
	if val != float64(dbStats.RollupsDailyCount) {
		t.Errorf("Rollups daily: expected %d, got %v", dbStats.RollupsDailyCount, val)
	}

	// Test import progress gauge
	val = testutil.ToFloat64(m.DBImportProgress)
	if val != float64(dbStats.ImportProgressCount) {
		t.Errorf("Import progress: expected %d, got %v", dbStats.ImportProgressCount, val)
	}
}

func TestMetrics_Register(t *testing.T) {
	// Reset default registry for this test
	prometheus.DefaultRegisterer = prometheus.NewRegistry()

	m := New(
		func() int { return 0 },
		func() int64 { return 0 },
		func() DBStats { return DBStats{} },
		nil, // no geo cache
	)

	err := m.Register()
	if err != nil {
		t.Errorf("unexpected error registering metrics: %v", err)
	}

	// Double registration should fail
	err = m.Register()
	if err == nil {
		t.Error("expected error on double registration")
	}
	if !strings.Contains(err.Error(), "already registered") && !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("expected 'already registered' or 'duplicate' error, got: %v", err)
	}
}

func TestMetrics_GeoCacheGaugeFuncs(t *testing.T) {
	geoStats := &GeoCacheStats{
		Size:     500,
		Capacity: 10000,
		Hits:     1000,
		Misses:   200,
		Evicts:   50,
		HitRate:  0.833,
	}

	m := New(
		func() int { return 0 },
		func() int64 { return 0 },
		func() DBStats { return DBStats{} },
		func() *GeoCacheStats { return geoStats },
	)

	// Test geo cache size
	val := testutil.ToFloat64(m.GeoCacheSize)
	if val != float64(geoStats.Size) {
		t.Errorf("Geo cache size: expected %d, got %v", geoStats.Size, val)
	}

	// Test geo cache capacity
	val = testutil.ToFloat64(m.GeoCacheCapacity)
	if val != float64(geoStats.Capacity) {
		t.Errorf("Geo cache capacity: expected %d, got %v", geoStats.Capacity, val)
	}

	// Test geo cache hits
	val = testutil.ToFloat64(m.GeoCacheHits)
	if val != float64(geoStats.Hits) {
		t.Errorf("Geo cache hits: expected %d, got %v", geoStats.Hits, val)
	}

	// Test geo cache misses
	val = testutil.ToFloat64(m.GeoCacheMisses)
	if val != float64(geoStats.Misses) {
		t.Errorf("Geo cache misses: expected %d, got %v", geoStats.Misses, val)
	}

	// Test geo cache evictions
	val = testutil.ToFloat64(m.GeoCacheEvicts)
	if val != float64(geoStats.Evicts) {
		t.Errorf("Geo cache evicts: expected %d, got %v", geoStats.Evicts, val)
	}

	// Test geo cache hit rate
	val = testutil.ToFloat64(m.GeoCacheHitRate)
	if val != geoStats.HitRate {
		t.Errorf("Geo cache hit rate: expected %v, got %v", geoStats.HitRate, val)
	}
}

func TestMetrics_GeoCacheNilStatsFunc(t *testing.T) {
	// Test with nil stats function
	m := New(
		func() int { return 0 },
		func() int64 { return 0 },
		func() DBStats { return DBStats{} },
		nil,
	)

	// All geo cache metrics should return 0 when stats function is nil
	if val := testutil.ToFloat64(m.GeoCacheSize); val != 0 {
		t.Errorf("expected 0 for nil geo stats, got %v", val)
	}
	if val := testutil.ToFloat64(m.GeoCacheHitRate); val != 0 {
		t.Errorf("expected 0 for nil geo stats hit rate, got %v", val)
	}
}

func TestMetrics_GeoCacheReturnsNilStats(t *testing.T) {
	// Test with stats function that returns nil
	m := New(
		func() int { return 0 },
		func() int64 { return 0 },
		func() DBStats { return DBStats{} },
		func() *GeoCacheStats { return nil },
	)

	// All geo cache metrics should return 0 when stats function returns nil
	if val := testutil.ToFloat64(m.GeoCacheSize); val != 0 {
		t.Errorf("expected 0 for nil geo stats, got %v", val)
	}
	if val := testutil.ToFloat64(m.GeoCacheHits); val != 0 {
		t.Errorf("expected 0 for nil geo stats hits, got %v", val)
	}
}
