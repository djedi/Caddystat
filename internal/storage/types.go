package storage

import "time"

// RequestRecord represents a single request to be stored.
type RequestRecord struct {
	Timestamp      time.Time
	Host           string
	Path           string
	Status         int
	Bytes          int64
	IP             string
	Referrer       string
	UserAgent      string
	ResponseTime   float64
	Country        string
	Region         string
	City           string
	Browser        string
	BrowserVersion string
	OS             string
	OSVersion      string
	DeviceType     string
	IsBot          bool
	BotName        string
	BotIntent      string
}

// Summary represents aggregated statistics for a time period.
type Summary struct {
	TotalRequests   int64            `json:"total_requests"`
	Status2xx       int64            `json:"status_2xx"`
	Status3xx       int64            `json:"status_3xx"`
	Status4xx       int64            `json:"status_4xx"`
	Status5xx       int64            `json:"status_5xx"`
	BandwidthBytes  int64            `json:"bandwidth_bytes"`
	UniqueVisitors  int64            `json:"unique_visitors"`
	Visits          int64            `json:"visits"`
	AvgResponseTime float64          `json:"avg_response_time_ms"`
	Traffic         TrafficSummary   `json:"traffic"`
	Bots            BotStats         `json:"bots"`
	TopPaths        []PathStat       `json:"top_paths"`
	Hosts           []HostStat       `json:"hosts"`
	Recent          []TimeSeriesStat `json:"recent"`
	ErrorPages      []ErrorPageStat  `json:"error_pages"`
}

// TrafficSummary breaks down traffic into viewed and not-viewed categories.
type TrafficSummary struct {
	Viewed    TrafficBreakdown `json:"viewed"`
	NotViewed TrafficBreakdown `json:"not_viewed"`
}

// TrafficBreakdown contains page/hit/bandwidth counts.
type TrafficBreakdown struct {
	Pages          int64 `json:"pages"`
	Hits           int64 `json:"hits"`
	BandwidthBytes int64 `json:"bandwidth_bytes"`
}

// BotIntentStats holds bot statistics by intent category.
type BotIntentStats struct {
	Hits           int64 `json:"hits"`
	BandwidthBytes int64 `json:"bandwidth_bytes"`
}

// BotStats holds aggregate bot/spider statistics.
type BotStats struct {
	TotalHits      int64                     `json:"total_hits"`
	BandwidthBytes int64                     `json:"bandwidth_bytes"`
	ByIntent       map[string]BotIntentStats `json:"by_intent"`
}

// MonthlyStat represents statistics for a single month.
type MonthlyStat struct {
	MonthStart     time.Time `json:"month_start"`
	UniqueVisitors int64     `json:"unique_visitors"`
	Visits         int64     `json:"visits"`
	Pages          int64     `json:"pages"`
	Hits           int64     `json:"hits"`
	BandwidthBytes int64     `json:"bandwidth_bytes"`
}

// MonthlyHistory contains monthly statistics over a time range.
type MonthlyHistory struct {
	Months []MonthlyStat `json:"months"`
	Totals MonthlyStat   `json:"totals"`
}

// DayStat represents statistics for a single day.
type DayStat struct {
	Date           time.Time `json:"date"`
	Visits         int64     `json:"visits"`
	Pages          int64     `json:"pages"`
	Hits           int64     `json:"hits"`
	BandwidthBytes int64     `json:"bandwidth_bytes"`
}

// DailyHistory contains daily statistics for a month.
type DailyHistory struct {
	Days    []DayStat `json:"days"`
	Totals  DayStat   `json:"totals"`
	Average DayStat   `json:"average"`
}

// TimeSeriesStat represents statistics for a time bucket.
type TimeSeriesStat struct {
	Bucket     time.Time `json:"bucket"`
	Requests   int64     `json:"requests"`
	Bytes      int64     `json:"bytes"`
	Status2xx  int64     `json:"status_2xx"`
	Status4xx  int64     `json:"status_4xx"`
	Status5xx  int64     `json:"status_5xx"`
	AvgLatency float64   `json:"avg_latency_ms"`
}

// PathStat represents request count for a path.
type PathStat struct {
	Path  string `json:"path"`
	Count int64  `json:"count"`
}

// HostStat represents request count for a host.
type HostStat struct {
	Host  string `json:"host"`
	Count int64  `json:"count"`
}

// GeoStat represents request count for a geographic location.
type GeoStat struct {
	Country string `json:"country"`
	Region  string `json:"region"`
	City    string `json:"city"`
	Count   int64  `json:"count"`
}

// ErrorPageStat represents error count for a path/status combination.
type ErrorPageStat struct {
	Path   string `json:"path"`
	Status int    `json:"status"`
	Count  int64  `json:"count"`
}

// VisitorStat represents statistics for a single visitor (IP).
type VisitorStat struct {
	IP             string    `json:"ip"`
	Pages          int64     `json:"pages"`
	Hits           int64     `json:"hits"`
	BandwidthBytes int64     `json:"bandwidth_bytes"`
	LastVisit      time.Time `json:"last_visit"`
	Country        string    `json:"country"`
}

// BrowserStat represents browser usage statistics.
type BrowserStat struct {
	Browser string  `json:"browser"`
	Pages   int64   `json:"pages"`
	Hits    int64   `json:"hits"`
	Percent float64 `json:"percent"`
}

// OSStat represents operating system usage statistics.
type OSStat struct {
	OS      string  `json:"os"`
	Pages   int64   `json:"pages"`
	Hits    int64   `json:"hits"`
	Percent float64 `json:"percent"`
}

// RobotStat represents bot/spider statistics.
type RobotStat struct {
	Name           string    `json:"name"`
	Intent         string    `json:"intent"`
	Hits           int64     `json:"hits"`
	BandwidthBytes int64     `json:"bandwidth_bytes"`
	LastVisit      time.Time `json:"last_visit"`
}

// ReferrerStat represents referrer statistics.
type ReferrerStat struct {
	Referrer string `json:"referrer"`
	Type     string `json:"type"`
	Pages    int64  `json:"pages"`
	Hits     int64  `json:"hits"`
}

// VisitorSession represents a reconstructed visitor session.
// Sessions are grouped by IP + User Agent and separated by 30-minute gaps.
type VisitorSession struct {
	IP             string    `json:"ip"`
	UserAgent      string    `json:"user_agent"`
	Browser        string    `json:"browser"`
	OS             string    `json:"os"`
	Country        string    `json:"country"`
	StartTime      time.Time `json:"start_time"`
	EndTime        time.Time `json:"end_time"`
	Duration       int64     `json:"duration_seconds"`
	PageViews      int64     `json:"page_views"`
	Hits           int64     `json:"hits"`
	BandwidthBytes int64     `json:"bandwidth_bytes"`
	EntryPage      string    `json:"entry_page"`
	ExitPage       string    `json:"exit_page"`
	IsBounce       bool      `json:"is_bounce"`
}

// VisitorSessionSummary provides aggregate stats about sessions.
type VisitorSessionSummary struct {
	TotalSessions  int64            `json:"total_sessions"`
	TotalPageViews int64            `json:"total_page_views"`
	AvgDuration    float64          `json:"avg_duration_seconds"`
	AvgPageViews   float64          `json:"avg_page_views"`
	BounceRate     float64          `json:"bounce_rate"`
	Sessions       []VisitorSession `json:"sessions"`
	SessionsByHour []HourlyBucket   `json:"sessions_by_hour,omitempty"`
	TopEntryPages  []PageCount      `json:"top_entry_pages,omitempty"`
	TopExitPages   []PageCount      `json:"top_exit_pages,omitempty"`
}

// HourlyBucket represents session counts for an hour.
type HourlyBucket struct {
	Hour     int   `json:"hour"`
	Sessions int64 `json:"sessions"`
}

// PageCount represents a page path with its count.
type PageCount struct {
	Path  string `json:"path"`
	Count int64  `json:"count"`
}

// RecentRequest represents a single request with all its details for display.
type RecentRequest struct {
	ID             int64     `json:"id"`
	Timestamp      time.Time `json:"timestamp"`
	Host           string    `json:"host"`
	Path           string    `json:"path"`
	Status         int       `json:"status"`
	Bytes          int64     `json:"bytes"`
	IP             string    `json:"ip"`
	Referrer       string    `json:"referrer"`
	UserAgent      string    `json:"user_agent"`
	ResponseTime   float64   `json:"response_time_ms"`
	Country        string    `json:"country"`
	Region         string    `json:"region"`
	City           string    `json:"city"`
	Browser        string    `json:"browser"`
	BrowserVersion string    `json:"browser_version"`
	OS             string    `json:"os"`
	OSVersion      string    `json:"os_version"`
	DeviceType     string    `json:"device_type"`
	IsBot          bool      `json:"is_bot"`
	BotName        string    `json:"bot_name"`
}

// ImportProgress tracks how much of a log file has been imported.
type ImportProgress struct {
	FilePath   string
	ByteOffset int64
	FileSize   int64
	FileMtime  int64
}

// ImportErrorStats tracks errors during log file import.
type ImportErrorStats struct {
	FilePath    string    `json:"file_path"`
	ErrorCount  int64     `json:"error_count"`
	LastError   string    `json:"last_error"`
	LastErrorAt time.Time `json:"last_error_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Session represents an authentication session.
type Session struct {
	Token     string
	ExpiresAt time.Time
	CreatedAt time.Time
}

// DatabaseStats holds statistics about the database tables.
type DatabaseStats struct {
	RequestsCount       int64
	SessionsCount       int64
	RollupsHourlyCount  int64
	RollupsDailyCount   int64
	ImportProgressCount int64
}

// SystemStatus represents the overall system status.
type SystemStatus struct {
	DBSizeBytes      int64              `json:"db_size_bytes"`
	DBSizeHuman      string             `json:"db_size_human"`
	RequestsCount    int64              `json:"requests_count"`
	HourlyRollups    int64              `json:"hourly_rollups"`
	DailyRollups     int64              `json:"daily_rollups"`
	LastImportTime   *time.Time         `json:"last_import_time,omitempty"`
	ActiveSessions   int64              `json:"active_sessions"`
	TrackedLogFiles  int64              `json:"tracked_log_files"`
	TotalParseErrors int64              `json:"total_parse_errors"`
	ImportErrors     []ImportErrorStats `json:"import_errors,omitempty"`
}

// ResponseTimeStats holds response time percentile statistics.
type ResponseTimeStats struct {
	Min    float64 `json:"min_ms"`
	Max    float64 `json:"max_ms"`
	Avg    float64 `json:"avg_ms"`
	P50    float64 `json:"p50_ms"`
	P90    float64 `json:"p90_ms"`
	P95    float64 `json:"p95_ms"`
	P99    float64 `json:"p99_ms"`
	Count  int64   `json:"count"`
	StdDev float64 `json:"std_dev_ms"`
}

// SlowPageStat represents a slow page with its response time statistics.
type SlowPageStat struct {
	Path          string  `json:"path"`
	Count         int64   `json:"count"`
	AvgResponseMs float64 `json:"avg_response_ms"`
	MaxResponseMs float64 `json:"max_response_ms"`
	P95ResponseMs float64 `json:"p95_response_ms"`
	TotalBytes    int64   `json:"total_bytes"`
	ErrorRate     float64 `json:"error_rate"`
}

// PerformanceStats holds comprehensive performance statistics.
type PerformanceStats struct {
	ResponseTime ResponseTimeStats `json:"response_time"`
	SlowPages    []SlowPageStat    `json:"slow_pages"`
	ByStatus     []StatusPerfStat  `json:"by_status"`
}

// StatusPerfStat holds performance stats grouped by status code range.
type StatusPerfStat struct {
	StatusRange   string  `json:"status_range"`
	Count         int64   `json:"count"`
	AvgResponseMs float64 `json:"avg_response_ms"`
	P95ResponseMs float64 `json:"p95_response_ms"`
}

// BandwidthStats holds comprehensive bandwidth statistics.
type BandwidthStats struct {
	TotalBytes    int64               `json:"total_bytes"`
	TotalHuman    string              `json:"total_human"`
	ByHost        []HostBandwidth     `json:"by_host"`
	ByPath        []PathBandwidth     `json:"by_path"`
	ByContentType []ContentBandwidth  `json:"by_content_type"`
	TimeSeries    []BandwidthTimeStat `json:"time_series"`
}

// HostBandwidth holds bandwidth statistics for a single host.
type HostBandwidth struct {
	Host       string  `json:"host"`
	Bytes      int64   `json:"bytes"`
	BytesHuman string  `json:"bytes_human"`
	Requests   int64   `json:"requests"`
	AvgBytes   float64 `json:"avg_bytes"`
	Percent    float64 `json:"percent"`
}

// PathBandwidth holds bandwidth statistics for a single path.
type PathBandwidth struct {
	Path       string  `json:"path"`
	Bytes      int64   `json:"bytes"`
	BytesHuman string  `json:"bytes_human"`
	Requests   int64   `json:"requests"`
	AvgBytes   float64 `json:"avg_bytes"`
	Percent    float64 `json:"percent"`
}

// ContentBandwidth holds bandwidth statistics grouped by content type (file extension).
type ContentBandwidth struct {
	ContentType string  `json:"content_type"`
	Bytes       int64   `json:"bytes"`
	BytesHuman  string  `json:"bytes_human"`
	Requests    int64   `json:"requests"`
	Percent     float64 `json:"percent"`
}

// BandwidthTimeStat holds bandwidth for a time bucket.
type BandwidthTimeStat struct {
	Bucket   time.Time `json:"bucket"`
	Bytes    int64     `json:"bytes"`
	Requests int64     `json:"requests"`
}

// ExportRequest represents a single request for export purposes (all fields included).
type ExportRequest struct {
	ID             int64     `json:"id"`
	Timestamp      time.Time `json:"timestamp"`
	Host           string    `json:"host"`
	Path           string    `json:"path"`
	Status         int       `json:"status"`
	Bytes          int64     `json:"bytes"`
	IP             string    `json:"ip"`
	Referrer       string    `json:"referrer"`
	UserAgent      string    `json:"user_agent"`
	ResponseTimeMs float64   `json:"response_time_ms"`
	Country        string    `json:"country"`
	Region         string    `json:"region"`
	City           string    `json:"city"`
	Browser        string    `json:"browser"`
	BrowserVersion string    `json:"browser_version"`
	OS             string    `json:"os"`
	OSVersion      string    `json:"os_version"`
	DeviceType     string    `json:"device_type"`
	IsBot          bool      `json:"is_bot"`
	BotName        string    `json:"bot_name"`
}

// ExportRequestsCallback is called for each batch of export requests.
// Return an error to stop iteration.
type ExportRequestsCallback func(requests []ExportRequest) error
