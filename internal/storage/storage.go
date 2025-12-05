package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

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
}

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
	TopPaths        []PathStat       `json:"top_paths"`
	Hosts           []HostStat       `json:"hosts"`
	Recent          []TimeSeriesStat `json:"recent"`
	ErrorPages      []ErrorPageStat  `json:"error_pages"`
}

type TrafficSummary struct {
	Viewed    TrafficBreakdown `json:"viewed"`
	NotViewed TrafficBreakdown `json:"not_viewed"`
}

type TrafficBreakdown struct {
	Pages          int64 `json:"pages"`
	Hits           int64 `json:"hits"`
	BandwidthBytes int64 `json:"bandwidth_bytes"`
}

type MonthlyStat struct {
	MonthStart     time.Time `json:"month_start"`
	UniqueVisitors int64     `json:"unique_visitors"`
	Visits         int64     `json:"visits"`
	Pages          int64     `json:"pages"`
	Hits           int64     `json:"hits"`
	BandwidthBytes int64     `json:"bandwidth_bytes"`
}

type MonthlyHistory struct {
	Months []MonthlyStat `json:"months"`
	Totals MonthlyStat   `json:"totals"`
}

type DayStat struct {
	Date           time.Time `json:"date"`
	Visits         int64     `json:"visits"`
	Pages          int64     `json:"pages"`
	Hits           int64     `json:"hits"`
	BandwidthBytes int64     `json:"bandwidth_bytes"`
}

type DailyHistory struct {
	Days    []DayStat `json:"days"`
	Totals  DayStat   `json:"totals"`
	Average DayStat   `json:"average"`
}

type TimeSeriesStat struct {
	Bucket     time.Time `json:"bucket"`
	Requests   int64     `json:"requests"`
	Bytes      int64     `json:"bytes"`
	Status2xx  int64     `json:"status_2xx"`
	Status4xx  int64     `json:"status_4xx"`
	Status5xx  int64     `json:"status_5xx"`
	AvgLatency float64   `json:"avg_latency_ms"`
}

type PathStat struct {
	Path  string `json:"path"`
	Count int64  `json:"count"`
}

type HostStat struct {
	Host  string `json:"host"`
	Count int64  `json:"count"`
}

type GeoStat struct {
	Country string `json:"country"`
	Region  string `json:"region"`
	City    string `json:"city"`
	Count   int64  `json:"count"`
}

type ErrorPageStat struct {
	Path   string `json:"path"`
	Status int    `json:"status"`
	Count  int64  `json:"count"`
}

type VisitorStat struct {
	IP             string    `json:"ip"`
	Pages          int64     `json:"pages"`
	Hits           int64     `json:"hits"`
	BandwidthBytes int64     `json:"bandwidth_bytes"`
	LastVisit      time.Time `json:"last_visit"`
	Country        string    `json:"country"`
}

type BrowserStat struct {
	Browser string  `json:"browser"`
	Pages   int64   `json:"pages"`
	Hits    int64   `json:"hits"`
	Percent float64 `json:"percent"`
}

type OSStat struct {
	OS      string  `json:"os"`
	Pages   int64   `json:"pages"`
	Hits    int64   `json:"hits"`
	Percent float64 `json:"percent"`
}

type RobotStat struct {
	Name           string    `json:"name"`
	Hits           int64     `json:"hits"`
	BandwidthBytes int64     `json:"bandwidth_bytes"`
	LastVisit      time.Time `json:"last_visit"`
}

type ReferrerStat struct {
	Referrer string `json:"referrer"`
	Type     string `json:"type"`
	Pages    int64  `json:"pages"`
	Hits     int64  `json:"hits"`
}

type Storage struct {
	db      *sql.DB
	writeMu sync.Mutex
}

func New(dbPath string) (*Storage, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}
	db, err := sql.Open("sqlite", dbPath+"?_busy_timeout=30000&_journal_mode=WAL&_synchronous=NORMAL")
	if err != nil {
		return nil, err
	}
	// Limit to single connection for writes to avoid lock contention
	db.SetMaxOpenConns(1)
	s := &Storage{db: db}
	if err := s.migrate(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Storage) migrate() error {
	// First, create tables without the new columns (for compatibility with existing DBs)
	schema := `
CREATE TABLE IF NOT EXISTS requests (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	ts TIMESTAMP NOT NULL,
	host TEXT,
	path TEXT,
	status INTEGER,
	bytes INTEGER,
	ip TEXT,
	referrer TEXT,
	user_agent TEXT,
	resp_time_ms REAL,
	country TEXT,
	region TEXT,
	city TEXT
);

CREATE INDEX IF NOT EXISTS idx_requests_ts ON requests(ts);
CREATE INDEX IF NOT EXISTS idx_requests_host ON requests(host);
CREATE INDEX IF NOT EXISTS idx_requests_path ON requests(path);

CREATE TABLE IF NOT EXISTS rollups_hourly (
	bucket_start TIMESTAMP NOT NULL,
	host TEXT,
	path TEXT,
	requests INTEGER,
	bytes INTEGER,
	status_2xx INTEGER,
	status_3xx INTEGER,
	status_4xx INTEGER,
	status_5xx INTEGER,
	PRIMARY KEY(bucket_start, host, path)
);

CREATE TABLE IF NOT EXISTS rollups_daily (
	bucket_start TIMESTAMP NOT NULL,
	host TEXT,
	path TEXT,
	requests INTEGER,
	bytes INTEGER,
	status_2xx INTEGER,
	status_3xx INTEGER,
	status_4xx INTEGER,
	status_5xx INTEGER,
	PRIMARY KEY(bucket_start, host, path)
);

CREATE TABLE IF NOT EXISTS import_progress (
	file_path TEXT PRIMARY KEY,
	byte_offset INTEGER NOT NULL,
	file_size INTEGER NOT NULL,
	file_mtime INTEGER NOT NULL,
	updated_at TIMESTAMP NOT NULL
);

CREATE TABLE IF NOT EXISTS sessions (
	token TEXT PRIMARY KEY,
	expires_at TIMESTAMP NOT NULL,
	created_at TIMESTAMP NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_sessions_expires ON sessions(expires_at);
`
	if _, err := s.db.Exec(schema); err != nil {
		return err
	}

	// Add new columns to existing tables (migration for existing databases)
	// These run after table creation, so they work for both new and existing DBs
	migrations := []string{
		"ALTER TABLE requests ADD COLUMN browser TEXT DEFAULT ''",
		"ALTER TABLE requests ADD COLUMN browser_version TEXT DEFAULT ''",
		"ALTER TABLE requests ADD COLUMN os TEXT DEFAULT ''",
		"ALTER TABLE requests ADD COLUMN os_version TEXT DEFAULT ''",
		"ALTER TABLE requests ADD COLUMN device_type TEXT DEFAULT ''",
		"ALTER TABLE requests ADD COLUMN is_bot INTEGER DEFAULT 0",
		"ALTER TABLE requests ADD COLUMN bot_name TEXT DEFAULT ''",
	}
	for _, m := range migrations {
		// Ignore errors - column may already exist
		_, _ = s.db.Exec(m)
	}

	// Create indexes after columns exist
	_, _ = s.db.Exec("CREATE INDEX IF NOT EXISTS idx_requests_ip ON requests(ip)")
	_, _ = s.db.Exec("CREATE INDEX IF NOT EXISTS idx_requests_is_bot ON requests(is_bot)")

	return nil
}

func (s *Storage) Close() error {
	return s.db.Close()
}

func (s *Storage) InsertRequest(ctx context.Context, r RequestRecord) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	isBot := 0
	if r.IsBot {
		isBot = 1
	}
	_, err = tx.ExecContext(ctx, `
INSERT INTO requests (ts, host, path, status, bytes, ip, referrer, user_agent, resp_time_ms, country, region, city, browser, browser_version, os, os_version, device_type, is_bot, bot_name)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`, r.Timestamp, r.Host, r.Path, r.Status, r.Bytes, r.IP, r.Referrer, r.UserAgent, r.ResponseTime, r.Country, r.Region, r.City, r.Browser, r.BrowserVersion, r.OS, r.OSVersion, r.DeviceType, isBot, r.BotName)
	if err != nil {
		return err
	}

	buckets := []struct {
		table string
		time  time.Time
	}{
		{"rollups_hourly", r.Timestamp.Truncate(time.Hour)},
		{"rollups_daily", r.Timestamp.Truncate(24 * time.Hour)},
	}
	for _, b := range buckets {
		if err = s.updateRollup(ctx, tx, b.table, b.time, r); err != nil {
			return err
		}
	}

	if err = tx.Commit(); err != nil {
		return err
	}
	committed = true
	return nil
}

func (s *Storage) updateRollup(ctx context.Context, tx *sql.Tx, table string, bucket time.Time, r RequestRecord) error {
	status2xx := 0
	status3xx := 0
	status4xx := 0
	status5xx := 0
	switch {
	case r.Status >= 200 && r.Status < 300:
		status2xx = 1
	case r.Status >= 300 && r.Status < 400:
		status3xx = 1
	case r.Status >= 400 && r.Status < 500:
		status4xx = 1
	case r.Status >= 500:
		status5xx = 1
	}

	_, err := tx.ExecContext(ctx, fmt.Sprintf(`
INSERT INTO %s (bucket_start, host, path, requests, bytes, status_2xx, status_3xx, status_4xx, status_5xx)
VALUES (?, ?, ?, 1, ?, ?, ?, ?, ?)
ON CONFLICT(bucket_start, host, path) DO UPDATE SET
	requests = requests + 1,
	bytes = bytes + excluded.bytes,
	status_2xx = status_2xx + excluded.status_2xx,
	status_3xx = status_3xx + excluded.status_3xx,
	status_4xx = status_4xx + excluded.status_4xx,
	status_5xx = status_5xx + excluded.status_5xx
`, table),
		bucket, r.Host, r.Path, r.Bytes, status2xx, status3xx, status4xx, status5xx)
	return err
}

func (s *Storage) Cleanup(ctx context.Context, retentionDays int) error {
	_, err := s.db.ExecContext(ctx, `
DELETE FROM requests WHERE ts < datetime('now', ?)
`, fmt.Sprintf("-%d days", retentionDays))
	return err
}

func (s *Storage) Summary(ctx context.Context, since time.Duration, host string) (Summary, error) {
	var out Summary
	from := time.Now().Add(-since)

	args := []any{from}
	where := "WHERE ts >= ?"
	if host != "" {
		where += " AND host = ?"
		args = append(args, host)
	}

	row := s.db.QueryRowContext(ctx, fmt.Sprintf(`
WITH filtered AS (
	SELECT
		ts,
		host,
		path,
		status,
		bytes,
		ip,
		user_agent,
		resp_time_ms,
		CAST(strftime('%%s', ts) AS INTEGER) AS ts_epoch,
		lower(CASE WHEN instr(path, '?') > 0 THEN substr(path, 1, instr(path, '?') - 1) ELSE path END) AS clean_path,
		lower(user_agent) AS ua
	FROM requests
	%s
),
classified AS (
	SELECT
		*,
		CASE
			WHEN status >= 400 THEN 0
			WHEN COALESCE(ua, '') LIKE '%%bot%%' OR COALESCE(ua, '') LIKE '%%crawl%%' OR COALESCE(ua, '') LIKE '%%spider%%' OR COALESCE(ua, '') LIKE '%%crawler%%' OR COALESCE(ua, '') LIKE '%%preview%%' OR COALESCE(ua, '') LIKE '%%pingdom%%' OR COALESCE(ua, '') LIKE '%%uptime%%' THEN 0
			ELSE 1
		END AS is_viewed,
		CASE
			WHEN clean_path IS NULL OR clean_path = '' THEN 1
			WHEN clean_path LIKE '%%.css' OR clean_path LIKE '%%.js' OR clean_path LIKE '%%.png' OR clean_path LIKE '%%.jpg' OR clean_path LIKE '%%.jpeg' OR clean_path LIKE '%%.gif' OR clean_path LIKE '%%.svg' OR clean_path LIKE '%%.ico' OR clean_path LIKE '%%.woff%%' OR clean_path LIKE '%%.ttf' OR clean_path LIKE '%%.eot' OR clean_path LIKE '%%.otf' OR clean_path LIKE '%%.map' OR clean_path LIKE '%%.json' OR clean_path LIKE '%%.xml' OR clean_path LIKE '%%.csv' THEN 0
			ELSE 1
		END AS is_page
	FROM filtered
),
visits AS (
	SELECT
		ip,
		ts_epoch,
		CASE
			WHEN LAG(ts_epoch) OVER (PARTITION BY ip ORDER BY ts_epoch) IS NULL THEN 1
			WHEN ts_epoch - LAG(ts_epoch) OVER (PARTITION BY ip ORDER BY ts_epoch) > 1800 THEN 1
			ELSE 0
		END AS new_visit
	FROM classified
)
SELECT
	COUNT(*) AS total_requests,
	SUM(CASE WHEN status BETWEEN 200 AND 299 THEN 1 ELSE 0 END) AS status_2xx,
	SUM(CASE WHEN status BETWEEN 300 AND 399 THEN 1 ELSE 0 END) AS status_3xx,
	SUM(CASE WHEN status BETWEEN 400 AND 499 THEN 1 ELSE 0 END) AS status_4xx,
	SUM(CASE WHEN status >= 500 THEN 1 ELSE 0 END) AS status_5xx,
	IFNULL(SUM(bytes), 0) AS bandwidth_bytes,
	IFNULL(AVG(resp_time_ms), 0) AS avg_resp,
	IFNULL(SUM(CASE WHEN is_viewed = 1 THEN 1 ELSE 0 END), 0) AS viewed_hits,
	IFNULL(SUM(CASE WHEN is_viewed = 0 THEN 1 ELSE 0 END), 0) AS not_viewed_hits,
	IFNULL(SUM(CASE WHEN is_viewed = 1 THEN bytes ELSE 0 END), 0) AS viewed_bandwidth,
	IFNULL(SUM(CASE WHEN is_viewed = 0 THEN bytes ELSE 0 END), 0) AS not_viewed_bandwidth,
	IFNULL(SUM(CASE WHEN is_viewed = 1 AND is_page = 1 THEN 1 ELSE 0 END), 0) AS viewed_pages,
	IFNULL(SUM(CASE WHEN is_viewed = 0 AND is_page = 1 THEN 1 ELSE 0 END), 0) AS not_viewed_pages,
	IFNULL((SELECT SUM(new_visit) FROM visits), 0) AS visits,
	IFNULL((SELECT COUNT(DISTINCT ip) FROM classified), 0) AS unique_visitors
FROM classified
`, where), args...)
	if err := row.Scan(
		&out.TotalRequests,
		&out.Status2xx,
		&out.Status3xx,
		&out.Status4xx,
		&out.Status5xx,
		&out.BandwidthBytes,
		&out.AvgResponseTime,
		&out.Traffic.Viewed.Hits,
		&out.Traffic.NotViewed.Hits,
		&out.Traffic.Viewed.BandwidthBytes,
		&out.Traffic.NotViewed.BandwidthBytes,
		&out.Traffic.Viewed.Pages,
		&out.Traffic.NotViewed.Pages,
		&out.Visits,
		&out.UniqueVisitors,
	); err != nil {
		return out, err
	}

	out.TopPaths, _ = s.topPaths(ctx, from, 5, host)
	out.Hosts, _ = s.hosts(ctx, from)
	out.Recent, _ = s.timeSeries(ctx, from, host)
	out.ErrorPages, _ = s.errorPages(ctx, from, 10, host)
	return out, nil
}

func (s *Storage) MonthlyHistory(ctx context.Context, months int, host string) (MonthlyHistory, error) {
	var out MonthlyHistory
	if months <= 0 || months > 60 {
		months = 12
	}
	now := time.Now().UTC()
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, -months+1, 0)

	args := []any{start}
	where := "WHERE ts >= ? AND ts IS NOT NULL"
	if host != "" {
		where += " AND host = ?"
		args = append(args, host)
	}

	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(`
WITH filtered AS (
	SELECT
		ts,
		host,
		path,
		status,
		bytes,
		ip,
		user_agent,
		CAST(strftime('%%s', ts) AS INTEGER) AS ts_epoch,
		IFNULL(strftime('%%Y-%%m', ts), '') AS month_key,
		lower(CASE WHEN instr(path, '?') > 0 THEN substr(path, 1, instr(path, '?') - 1) ELSE path END) AS clean_path,
		lower(user_agent) AS ua
	FROM requests
	%s
),
classified AS (
	SELECT
		*,
		CASE
			WHEN status >= 400 THEN 0
			WHEN COALESCE(ua, '') LIKE '%%bot%%' OR COALESCE(ua, '') LIKE '%%crawl%%' OR COALESCE(ua, '') LIKE '%%spider%%' OR COALESCE(ua, '') LIKE '%%crawler%%' OR COALESCE(ua, '') LIKE '%%preview%%' OR COALESCE(ua, '') LIKE '%%pingdom%%' OR COALESCE(ua, '') LIKE '%%uptime%%' THEN 0
			ELSE 1
		END AS is_viewed,
		CASE
			WHEN clean_path IS NULL OR clean_path = '' THEN 1
			WHEN clean_path LIKE '%%.css' OR clean_path LIKE '%%.js' OR clean_path LIKE '%%.png' OR clean_path LIKE '%%.jpg' OR clean_path LIKE '%%.jpeg' OR clean_path LIKE '%%.gif' OR clean_path LIKE '%%.svg' OR clean_path LIKE '%%.ico' OR clean_path LIKE '%%.woff%%' OR clean_path LIKE '%%.ttf' OR clean_path LIKE '%%.eot' OR clean_path LIKE '%%.otf' OR clean_path LIKE '%%.map' OR clean_path LIKE '%%.json' OR clean_path LIKE '%%.xml' OR clean_path LIKE '%%.csv' THEN 0
			ELSE 1
		END AS is_page
	FROM filtered
),
visits AS (
	SELECT
		month_key,
		ip,
		ts_epoch,
		CASE
			WHEN LAG(ts_epoch) OVER (PARTITION BY month_key, ip ORDER BY ts_epoch) IS NULL THEN 1
			WHEN ts_epoch - LAG(ts_epoch) OVER (PARTITION BY month_key, ip ORDER BY ts_epoch) > 1800 THEN 1
			ELSE 0
		END AS new_visit
	FROM classified
)
SELECT
	c.month_key,
	COUNT(*) AS hits,
	IFNULL(SUM(CASE WHEN is_page = 1 THEN 1 ELSE 0 END), 0) AS pages,
	IFNULL(SUM(bytes), 0) AS bandwidth_bytes,
	IFNULL((SELECT SUM(new_visit) FROM visits v WHERE v.month_key = c.month_key), 0) AS visits,
	IFNULL(COUNT(DISTINCT ip), 0) AS unique_visitors
FROM classified c
GROUP BY c.month_key
ORDER BY c.month_key ASC
`, where), args...)
	if err != nil {
		return out, err
	}
	defer rows.Close()

	byKey := make(map[string]MonthlyStat)
	for rows.Next() {
		var key sql.NullString
		var m MonthlyStat
		if err := rows.Scan(&key, &m.Hits, &m.Pages, &m.BandwidthBytes, &m.Visits, &m.UniqueVisitors); err != nil {
			return out, err
		}
		if !key.Valid || key.String == "" {
			continue
		}
		t, err := time.Parse("2006-01", key.String)
		if err != nil {
			continue
		}
		m.MonthStart = time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
		byKey[key.String] = m
	}
	if err := rows.Err(); err != nil {
		return out, err
	}

	for i := 0; i < months; i++ {
		ms := start.AddDate(0, i, 0)
		key := ms.Format("2006-01")
		stat, ok := byKey[key]
		if !ok {
			stat = MonthlyStat{MonthStart: ms}
		}
		out.Months = append(out.Months, stat)
		out.Totals.Hits += stat.Hits
		out.Totals.Pages += stat.Pages
		out.Totals.BandwidthBytes += stat.BandwidthBytes
		out.Totals.Visits += stat.Visits
		out.Totals.UniqueVisitors += stat.UniqueVisitors
	}
	return out, nil
}

func (s *Storage) DailyHistory(ctx context.Context, host string) (DailyHistory, error) {
	var out DailyHistory
	now := time.Now().UTC()
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 1, 0)

	args := []any{start, end}
	where := "WHERE ts >= ? AND ts < ? AND ts IS NOT NULL"
	if host != "" {
		where += " AND host = ?"
		args = append(args, host)
	}

	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(`
WITH filtered AS (
	SELECT
		ts,
		host,
		path,
		status,
		bytes,
		ip,
		user_agent,
		CAST(strftime('%%s', ts) AS INTEGER) AS ts_epoch,
		IFNULL(strftime('%%Y-%%m-%%d', ts), '') AS day_key,
		lower(CASE WHEN instr(path, '?') > 0 THEN substr(path, 1, instr(path, '?') - 1) ELSE path END) AS clean_path,
		lower(user_agent) AS ua
	FROM requests
	%s
),
classified AS (
	SELECT
		*,
		CASE
			WHEN status >= 400 THEN 0
			WHEN COALESCE(ua, '') LIKE '%%bot%%' OR COALESCE(ua, '') LIKE '%%crawl%%' OR COALESCE(ua, '') LIKE '%%spider%%' OR COALESCE(ua, '') LIKE '%%crawler%%' OR COALESCE(ua, '') LIKE '%%preview%%' OR COALESCE(ua, '') LIKE '%%pingdom%%' OR COALESCE(ua, '') LIKE '%%uptime%%' THEN 0
			ELSE 1
		END AS is_viewed,
		CASE
			WHEN clean_path IS NULL OR clean_path = '' THEN 1
			WHEN clean_path LIKE '%%.css' OR clean_path LIKE '%%.js' OR clean_path LIKE '%%.png' OR clean_path LIKE '%%.jpg' OR clean_path LIKE '%%.jpeg' OR clean_path LIKE '%%.gif' OR clean_path LIKE '%%.svg' OR clean_path LIKE '%%.ico' OR clean_path LIKE '%%.woff%%' OR clean_path LIKE '%%.ttf' OR clean_path LIKE '%%.eot' OR clean_path LIKE '%%.otf' OR clean_path LIKE '%%.map' OR clean_path LIKE '%%.json' OR clean_path LIKE '%%.xml' OR clean_path LIKE '%%.csv' THEN 0
			ELSE 1
		END AS is_page
	FROM filtered
),
visits AS (
	SELECT
		day_key,
		ip,
		ts_epoch,
		CASE
			WHEN LAG(ts_epoch) OVER (PARTITION BY day_key, ip ORDER BY ts_epoch) IS NULL THEN 1
			WHEN ts_epoch - LAG(ts_epoch) OVER (PARTITION BY day_key, ip ORDER BY ts_epoch) > 1800 THEN 1
			ELSE 0
		END AS new_visit
	FROM classified
)
SELECT
	c.day_key,
	COUNT(*) AS hits,
	IFNULL(SUM(CASE WHEN is_page = 1 THEN 1 ELSE 0 END), 0) AS pages,
	IFNULL(SUM(bytes), 0) AS bandwidth_bytes,
	IFNULL((SELECT SUM(new_visit) FROM visits v WHERE v.day_key = c.day_key), 0) AS visits
FROM classified c
GROUP BY c.day_key
ORDER BY c.day_key ASC
`, where), args...)
	if err != nil {
		return out, err
	}
	defer rows.Close()

	byKey := make(map[string]DayStat)
	for rows.Next() {
		var key sql.NullString
		var d DayStat
		if err := rows.Scan(&key, &d.Hits, &d.Pages, &d.BandwidthBytes, &d.Visits); err != nil {
			return out, err
		}
		if !key.Valid || key.String == "" {
			continue
		}
		t, err := time.Parse("2006-01-02", key.String)
		if err != nil {
			continue
		}
		d.Date = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
		byKey[key.String] = d
	}
	if err := rows.Err(); err != nil {
		return out, err
	}

	daysInMonth := int(end.Sub(start).Hours()/24 + 0.5)
	daysWithData := 0
	for i := 0; i < daysInMonth; i++ {
		day := start.AddDate(0, 0, i)
		key := day.Format("2006-01-02")
		stat, ok := byKey[key]
		if !ok {
			stat = DayStat{Date: day}
		}
		if stat.Hits > 0 || stat.Pages > 0 || stat.BandwidthBytes > 0 || stat.Visits > 0 {
			daysWithData++
		}
		out.Days = append(out.Days, stat)
		out.Totals.Hits += stat.Hits
		out.Totals.Pages += stat.Pages
		out.Totals.BandwidthBytes += stat.BandwidthBytes
		out.Totals.Visits += stat.Visits
	}
	if daysWithData > 0 {
		out.Average.Hits = out.Totals.Hits / int64(daysWithData)
		out.Average.Pages = out.Totals.Pages / int64(daysWithData)
		out.Average.BandwidthBytes = out.Totals.BandwidthBytes / int64(daysWithData)
		out.Average.Visits = out.Totals.Visits / int64(daysWithData)
	}
	return out, nil
}

func (s *Storage) topPaths(ctx context.Context, from time.Time, limit int, host string) ([]PathStat, error) {
	var rows *sql.Rows
	var err error
	if host == "" {
		rows, err = s.db.QueryContext(ctx, `
SELECT path, COUNT(*) as c FROM requests WHERE ts >= ? GROUP BY path ORDER BY c DESC LIMIT ?
`, from, limit)
	} else {
		rows, err = s.db.QueryContext(ctx, `
SELECT path, COUNT(*) as c FROM requests WHERE ts >= ? AND host = ? GROUP BY path ORDER BY c DESC LIMIT ?
`, from, host, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []PathStat
	for rows.Next() {
		var p PathStat
		if err := rows.Scan(&p.Path, &p.Count); err != nil {
			return nil, err
		}
		list = append(list, p)
	}
	return list, rows.Err()
}

func (s *Storage) hosts(ctx context.Context, from time.Time) ([]HostStat, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT host, COUNT(*) as c FROM requests WHERE ts >= ? GROUP BY host ORDER BY c DESC
`, from)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []HostStat
	for rows.Next() {
		var h HostStat
		if err := rows.Scan(&h.Host, &h.Count); err != nil {
			return nil, err
		}
		list = append(list, h)
	}
	return list, rows.Err()
}

func (s *Storage) errorPages(ctx context.Context, from time.Time, limit int, host string) ([]ErrorPageStat, error) {
	var rows *sql.Rows
	var err error
	if host == "" {
		rows, err = s.db.QueryContext(ctx, `
SELECT path, status, COUNT(*) as c FROM requests
WHERE ts >= ? AND status >= 400
GROUP BY path, status
ORDER BY c DESC LIMIT ?
`, from, limit)
	} else {
		rows, err = s.db.QueryContext(ctx, `
SELECT path, status, COUNT(*) as c FROM requests
WHERE ts >= ? AND status >= 400 AND host = ?
GROUP BY path, status
ORDER BY c DESC LIMIT ?
`, from, host, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []ErrorPageStat
	for rows.Next() {
		var e ErrorPageStat
		if err := rows.Scan(&e.Path, &e.Status, &e.Count); err != nil {
			return nil, err
		}
		list = append(list, e)
	}
	return list, rows.Err()
}

func (s *Storage) timeSeries(ctx context.Context, from time.Time, host string) ([]TimeSeriesStat, error) {
	var rows *sql.Rows
	var err error
	if host == "" {
		rows, err = s.db.QueryContext(ctx, `
SELECT
	strftime('%Y-%m-%dT%H:00:00Z', ts) as bucket,
	COUNT(*),
	IFNULL(SUM(bytes),0),
	SUM(CASE WHEN status BETWEEN 200 AND 299 THEN 1 ELSE 0 END),
	SUM(CASE WHEN status BETWEEN 400 AND 499 THEN 1 ELSE 0 END),
	SUM(CASE WHEN status >= 500 THEN 1 ELSE 0 END),
	IFNULL(AVG(resp_time_ms),0)
FROM requests
WHERE ts >= ? AND ts IS NOT NULL
GROUP BY bucket
HAVING bucket IS NOT NULL
ORDER BY bucket ASC
`, from)
	} else {
		rows, err = s.db.QueryContext(ctx, `
SELECT
	strftime('%Y-%m-%dT%H:00:00Z', ts) as bucket,
	COUNT(*),
	IFNULL(SUM(bytes),0),
	SUM(CASE WHEN status BETWEEN 200 AND 299 THEN 1 ELSE 0 END),
	SUM(CASE WHEN status BETWEEN 400 AND 499 THEN 1 ELSE 0 END),
	SUM(CASE WHEN status >= 500 THEN 1 ELSE 0 END),
	IFNULL(AVG(resp_time_ms),0)
FROM requests
WHERE ts >= ? AND ts IS NOT NULL AND host = ?
GROUP BY bucket
HAVING bucket IS NOT NULL
ORDER BY bucket ASC
`, from, host)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []TimeSeriesStat
	for rows.Next() {
		var tsStr sql.NullString
		var ts TimeSeriesStat
		if err := rows.Scan(&tsStr, &ts.Requests, &ts.Bytes, &ts.Status2xx, &ts.Status4xx, &ts.Status5xx, &ts.AvgLatency); err != nil {
			return nil, err
		}
		if !tsStr.Valid {
			continue
		}
		parsed, _ := time.Parse(time.RFC3339, tsStr.String)
		ts.Bucket = parsed
		list = append(list, ts)
	}
	return list, rows.Err()
}

func (s *Storage) TimeSeriesRange(ctx context.Context, dur time.Duration, host string) ([]TimeSeriesStat, error) {
	return s.timeSeries(ctx, time.Now().Add(-dur), host)
}

func (s *Storage) Geo(ctx context.Context, dur time.Duration, host string) ([]GeoStat, error) {
	var rows *sql.Rows
	var err error
	from := time.Now().Add(-dur)
	if host == "" {
		rows, err = s.db.QueryContext(ctx, `
SELECT country, region, city, COUNT(*) FROM requests WHERE ts >= ? GROUP BY country, region, city ORDER BY COUNT(*) DESC
`, from)
	} else {
		rows, err = s.db.QueryContext(ctx, `
SELECT country, region, city, COUNT(*) FROM requests WHERE ts >= ? AND host = ? GROUP BY country, region, city ORDER BY COUNT(*) DESC
`, from, host)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []GeoStat
	for rows.Next() {
		var g GeoStat
		if err := rows.Scan(&g.Country, &g.Region, &g.City, &g.Count); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (s *Storage) DB() *sql.DB {
	return s.db
}

func (s *Storage) Health(ctx context.Context) error {
	row := s.db.QueryRowContext(ctx, "SELECT 1")
	var n int
	if err := row.Scan(&n); err != nil {
		return err
	}
	if n != 1 {
		return errors.New("unexpected ping result")
	}
	return nil
}

// Visitors returns top visitor IPs with their stats
func (s *Storage) Visitors(ctx context.Context, dur time.Duration, host string, limit int) ([]VisitorStat, error) {
	from := time.Now().Add(-dur)
	if limit <= 0 {
		limit = 20
	}

	query := `
SELECT
	ip,
	SUM(CASE WHEN
		(CASE WHEN instr(path, '?') > 0 THEN substr(path, 1, instr(path, '?') - 1) ELSE path END) NOT LIKE '%.css'
		AND (CASE WHEN instr(path, '?') > 0 THEN substr(path, 1, instr(path, '?') - 1) ELSE path END) NOT LIKE '%.js'
		AND (CASE WHEN instr(path, '?') > 0 THEN substr(path, 1, instr(path, '?') - 1) ELSE path END) NOT LIKE '%.png'
		AND (CASE WHEN instr(path, '?') > 0 THEN substr(path, 1, instr(path, '?') - 1) ELSE path END) NOT LIKE '%.jpg'
		AND (CASE WHEN instr(path, '?') > 0 THEN substr(path, 1, instr(path, '?') - 1) ELSE path END) NOT LIKE '%.gif'
		AND (CASE WHEN instr(path, '?') > 0 THEN substr(path, 1, instr(path, '?') - 1) ELSE path END) NOT LIKE '%.svg'
		AND (CASE WHEN instr(path, '?') > 0 THEN substr(path, 1, instr(path, '?') - 1) ELSE path END) NOT LIKE '%.ico'
		AND (CASE WHEN instr(path, '?') > 0 THEN substr(path, 1, instr(path, '?') - 1) ELSE path END) NOT LIKE '%.woff%'
	THEN 1 ELSE 0 END) as pages,
	COUNT(*) as hits,
	IFNULL(SUM(bytes), 0) as bandwidth,
	MAX(ts) as last_visit,
	IFNULL(MAX(country), '') as country
FROM requests
WHERE ts >= ? AND is_bot = 0`

	args := []any{from}
	if host != "" {
		query += " AND host = ?"
		args = append(args, host)
	}
	query += " GROUP BY ip ORDER BY hits DESC LIMIT ?"
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []VisitorStat
	for rows.Next() {
		var v VisitorStat
		var lastVisitStr sql.NullString
		if err := rows.Scan(&v.IP, &v.Pages, &v.Hits, &v.BandwidthBytes, &lastVisitStr, &v.Country); err != nil {
			return nil, err
		}
		if lastVisitStr.Valid {
			v.LastVisit, _ = time.Parse("2006-01-02 15:04:05.999999999-07:00", lastVisitStr.String)
			if v.LastVisit.IsZero() {
				v.LastVisit, _ = time.Parse(time.RFC3339Nano, lastVisitStr.String)
			}
			if v.LastVisit.IsZero() {
				v.LastVisit, _ = time.Parse("2006-01-02T15:04:05Z", lastVisitStr.String)
			}
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// Browsers returns browser usage statistics
func (s *Storage) Browsers(ctx context.Context, dur time.Duration, host string, limit int) ([]BrowserStat, error) {
	from := time.Now().Add(-dur)
	if limit <= 0 {
		limit = 10
	}

	query := `
WITH stats AS (
	SELECT
		CASE WHEN browser = '' THEN 'Unknown' ELSE browser END as browser,
		SUM(CASE WHEN
			(CASE WHEN instr(path, '?') > 0 THEN substr(path, 1, instr(path, '?') - 1) ELSE path END) NOT LIKE '%.css'
			AND (CASE WHEN instr(path, '?') > 0 THEN substr(path, 1, instr(path, '?') - 1) ELSE path END) NOT LIKE '%.js'
			AND (CASE WHEN instr(path, '?') > 0 THEN substr(path, 1, instr(path, '?') - 1) ELSE path END) NOT LIKE '%.png'
			AND (CASE WHEN instr(path, '?') > 0 THEN substr(path, 1, instr(path, '?') - 1) ELSE path END) NOT LIKE '%.jpg'
			AND (CASE WHEN instr(path, '?') > 0 THEN substr(path, 1, instr(path, '?') - 1) ELSE path END) NOT LIKE '%.gif'
			AND (CASE WHEN instr(path, '?') > 0 THEN substr(path, 1, instr(path, '?') - 1) ELSE path END) NOT LIKE '%.svg'
			AND (CASE WHEN instr(path, '?') > 0 THEN substr(path, 1, instr(path, '?') - 1) ELSE path END) NOT LIKE '%.ico'
		THEN 1 ELSE 0 END) as pages,
		COUNT(*) as hits
	FROM requests
	WHERE ts >= ? AND is_bot = 0`

	args := []any{from}
	if host != "" {
		query += " AND host = ?"
		args = append(args, host)
	}
	query += `
	GROUP BY browser
),
totals AS (SELECT SUM(hits) as total FROM stats)
SELECT browser, pages, hits, ROUND(100.0 * hits / NULLIF((SELECT total FROM totals), 0), 1) as percent
FROM stats
ORDER BY hits DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []BrowserStat
	for rows.Next() {
		var b BrowserStat
		if err := rows.Scan(&b.Browser, &b.Pages, &b.Hits, &b.Percent); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// OperatingSystems returns OS usage statistics
func (s *Storage) OperatingSystems(ctx context.Context, dur time.Duration, host string, limit int) ([]OSStat, error) {
	from := time.Now().Add(-dur)
	if limit <= 0 {
		limit = 10
	}

	query := `
WITH stats AS (
	SELECT
		CASE WHEN os = '' THEN 'Unknown' ELSE os END as os,
		SUM(CASE WHEN
			(CASE WHEN instr(path, '?') > 0 THEN substr(path, 1, instr(path, '?') - 1) ELSE path END) NOT LIKE '%.css'
			AND (CASE WHEN instr(path, '?') > 0 THEN substr(path, 1, instr(path, '?') - 1) ELSE path END) NOT LIKE '%.js'
			AND (CASE WHEN instr(path, '?') > 0 THEN substr(path, 1, instr(path, '?') - 1) ELSE path END) NOT LIKE '%.png'
			AND (CASE WHEN instr(path, '?') > 0 THEN substr(path, 1, instr(path, '?') - 1) ELSE path END) NOT LIKE '%.jpg'
			AND (CASE WHEN instr(path, '?') > 0 THEN substr(path, 1, instr(path, '?') - 1) ELSE path END) NOT LIKE '%.gif'
			AND (CASE WHEN instr(path, '?') > 0 THEN substr(path, 1, instr(path, '?') - 1) ELSE path END) NOT LIKE '%.svg'
			AND (CASE WHEN instr(path, '?') > 0 THEN substr(path, 1, instr(path, '?') - 1) ELSE path END) NOT LIKE '%.ico'
		THEN 1 ELSE 0 END) as pages,
		COUNT(*) as hits
	FROM requests
	WHERE ts >= ? AND is_bot = 0`

	args := []any{from}
	if host != "" {
		query += " AND host = ?"
		args = append(args, host)
	}
	query += `
	GROUP BY os
),
totals AS (SELECT SUM(hits) as total FROM stats)
SELECT os, pages, hits, ROUND(100.0 * hits / NULLIF((SELECT total FROM totals), 0), 1) as percent
FROM stats
ORDER BY hits DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []OSStat
	for rows.Next() {
		var o OSStat
		if err := rows.Scan(&o.OS, &o.Pages, &o.Hits, &o.Percent); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// Robots returns bot/spider statistics
func (s *Storage) Robots(ctx context.Context, dur time.Duration, host string, limit int) ([]RobotStat, error) {
	from := time.Now().Add(-dur)
	if limit <= 0 {
		limit = 20
	}

	query := `
SELECT
	CASE WHEN bot_name = '' THEN 'Unknown Bot' ELSE bot_name END as name,
	COUNT(*) as hits,
	IFNULL(SUM(bytes), 0) as bandwidth,
	MAX(ts) as last_visit
FROM requests
WHERE ts >= ? AND is_bot = 1`

	args := []any{from}
	if host != "" {
		query += " AND host = ?"
		args = append(args, host)
	}
	query += " GROUP BY bot_name ORDER BY hits DESC LIMIT ?"
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []RobotStat
	for rows.Next() {
		var r RobotStat
		var lastVisitStr sql.NullString
		if err := rows.Scan(&r.Name, &r.Hits, &r.BandwidthBytes, &lastVisitStr); err != nil {
			return nil, err
		}
		if lastVisitStr.Valid {
			r.LastVisit, _ = time.Parse("2006-01-02 15:04:05.999999999-07:00", lastVisitStr.String)
			if r.LastVisit.IsZero() {
				r.LastVisit, _ = time.Parse(time.RFC3339Nano, lastVisitStr.String)
			}
			if r.LastVisit.IsZero() {
				r.LastVisit, _ = time.Parse("2006-01-02T15:04:05Z", lastVisitStr.String)
			}
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Referrers returns referrer statistics
func (s *Storage) Referrers(ctx context.Context, dur time.Duration, host string, limit int) ([]ReferrerStat, error) {
	from := time.Now().Add(-dur)
	if limit <= 0 {
		limit = 20
	}

	query := `
SELECT
	CASE
		WHEN referrer IS NULL OR referrer = '' THEN 'Direct / Bookmark'
		ELSE referrer
	END as ref,
	CASE
		WHEN referrer IS NULL OR referrer = '' THEN 'direct'
		WHEN referrer LIKE '%google.%' OR referrer LIKE '%bing.%' OR referrer LIKE '%yahoo.%'
			OR referrer LIKE '%duckduckgo.%' OR referrer LIKE '%baidu.%' OR referrer LIKE '%yandex.%' THEN 'search'
		ELSE 'external'
	END as ref_type,
	SUM(CASE WHEN
		(CASE WHEN instr(path, '?') > 0 THEN substr(path, 1, instr(path, '?') - 1) ELSE path END) NOT LIKE '%.css'
		AND (CASE WHEN instr(path, '?') > 0 THEN substr(path, 1, instr(path, '?') - 1) ELSE path END) NOT LIKE '%.js'
		AND (CASE WHEN instr(path, '?') > 0 THEN substr(path, 1, instr(path, '?') - 1) ELSE path END) NOT LIKE '%.png'
		AND (CASE WHEN instr(path, '?') > 0 THEN substr(path, 1, instr(path, '?') - 1) ELSE path END) NOT LIKE '%.jpg'
		AND (CASE WHEN instr(path, '?') > 0 THEN substr(path, 1, instr(path, '?') - 1) ELSE path END) NOT LIKE '%.gif'
		AND (CASE WHEN instr(path, '?') > 0 THEN substr(path, 1, instr(path, '?') - 1) ELSE path END) NOT LIKE '%.svg'
		AND (CASE WHEN instr(path, '?') > 0 THEN substr(path, 1, instr(path, '?') - 1) ELSE path END) NOT LIKE '%.ico'
	THEN 1 ELSE 0 END) as pages,
	COUNT(*) as hits
FROM requests
WHERE ts >= ? AND is_bot = 0`

	args := []any{from}
	if host != "" {
		query += " AND host = ?"
		args = append(args, host)
	}
	query += " GROUP BY ref ORDER BY hits DESC LIMIT ?"
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ReferrerStat
	for rows.Next() {
		var r ReferrerStat
		if err := rows.Scan(&r.Referrer, &r.Type, &r.Pages, &r.Hits); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// RecentRequest represents a single request with all its details for display
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

// ImportProgress tracks how much of a log file has been imported
type ImportProgress struct {
	FilePath   string
	ByteOffset int64
	FileSize   int64
	FileMtime  int64
}

// GetImportProgress returns the import progress for a file, or nil if not found
func (s *Storage) GetImportProgress(ctx context.Context, filePath string) (*ImportProgress, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT file_path, byte_offset, file_size, file_mtime FROM import_progress WHERE file_path = ?`,
		filePath)
	var p ImportProgress
	err := row.Scan(&p.FilePath, &p.ByteOffset, &p.FileSize, &p.FileMtime)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// SetImportProgress updates the import progress for a file
func (s *Storage) SetImportProgress(ctx context.Context, p ImportProgress) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_, err := s.db.ExecContext(ctx, `
INSERT INTO import_progress (file_path, byte_offset, file_size, file_mtime, updated_at)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(file_path) DO UPDATE SET
	byte_offset = excluded.byte_offset,
	file_size = excluded.file_size,
	file_mtime = excluded.file_mtime,
	updated_at = excluded.updated_at
`, p.FilePath, p.ByteOffset, p.FileSize, p.FileMtime, time.Now())
	return err
}

// RecentRequests returns the most recent N requests, optionally filtered by host
func (s *Storage) RecentRequests(ctx context.Context, limit int, host string) ([]RecentRequest, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	query := `
SELECT
	id, ts, host, path, status, bytes, ip, referrer, user_agent, resp_time_ms,
	IFNULL(country, '') as country, IFNULL(region, '') as region, IFNULL(city, '') as city,
	IFNULL(browser, '') as browser, IFNULL(browser_version, '') as browser_version,
	IFNULL(os, '') as os, IFNULL(os_version, '') as os_version,
	IFNULL(device_type, '') as device_type, IFNULL(is_bot, 0) as is_bot, IFNULL(bot_name, '') as bot_name
FROM requests`

	args := []any{}
	if host != "" {
		query += " WHERE host = ?"
		args = append(args, host)
	}
	query += " ORDER BY id DESC LIMIT ?"
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]RecentRequest, 0)
	for rows.Next() {
		var r RecentRequest
		var tsStr sql.NullString
		var isBot int
		if err := rows.Scan(
			&r.ID, &tsStr, &r.Host, &r.Path, &r.Status, &r.Bytes, &r.IP, &r.Referrer, &r.UserAgent, &r.ResponseTime,
			&r.Country, &r.Region, &r.City, &r.Browser, &r.BrowserVersion,
			&r.OS, &r.OSVersion, &r.DeviceType, &isBot, &r.BotName,
		); err != nil {
			return nil, err
		}
		r.IsBot = isBot != 0
		if tsStr.Valid {
			r.Timestamp, _ = time.Parse("2006-01-02 15:04:05.999999999-07:00", tsStr.String)
			if r.Timestamp.IsZero() {
				r.Timestamp, _ = time.Parse(time.RFC3339Nano, tsStr.String)
			}
			if r.Timestamp.IsZero() {
				r.Timestamp, _ = time.Parse("2006-01-02T15:04:05Z", tsStr.String)
			}
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Ping checks database connectivity by executing a simple query.
func (s *Storage) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

// Session represents a user authentication session.
type Session struct {
	Token     string
	ExpiresAt time.Time
	CreatedAt time.Time
}

// CreateSession stores a new session in the database.
func (s *Storage) CreateSession(ctx context.Context, token string, expiresAt time.Time) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO sessions (token, expires_at, created_at) VALUES (?, ?, ?)`,
		token, expiresAt, time.Now().UTC())
	return err
}

// GetSession retrieves a session by token.
// Returns nil if the session doesn't exist.
func (s *Storage) GetSession(ctx context.Context, token string) (*Session, error) {
	var sess Session
	var expiresStr, createdStr string
	err := s.db.QueryRowContext(ctx,
		`SELECT token, expires_at, created_at FROM sessions WHERE token = ?`,
		token).Scan(&sess.Token, &expiresStr, &createdStr)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	sess.ExpiresAt, _ = time.Parse("2006-01-02 15:04:05.999999999-07:00", expiresStr)
	if sess.ExpiresAt.IsZero() {
		sess.ExpiresAt, _ = time.Parse(time.RFC3339Nano, expiresStr)
	}
	sess.CreatedAt, _ = time.Parse("2006-01-02 15:04:05.999999999-07:00", createdStr)
	if sess.CreatedAt.IsZero() {
		sess.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdStr)
	}

	return &sess, nil
}

// DeleteSession removes a session from the database.
func (s *Storage) DeleteSession(ctx context.Context, token string) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token = ?`, token)
	return err
}

// CleanupExpiredSessions removes all sessions that have expired.
// Returns the number of sessions deleted.
func (s *Storage) CleanupExpiredSessions(ctx context.Context) (int64, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	result, err := s.db.ExecContext(ctx,
		`DELETE FROM sessions WHERE expires_at < ?`,
		time.Now().UTC())
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// DatabaseStats holds statistics about the database tables.
type DatabaseStats struct {
	RequestsCount       int64
	SessionsCount       int64
	RollupsHourlyCount  int64
	RollupsDailyCount   int64
	ImportProgressCount int64
}

// GetDatabaseStats returns row counts for all tables.
func (s *Storage) GetDatabaseStats(ctx context.Context) (DatabaseStats, error) {
	var stats DatabaseStats
	queries := []struct {
		query string
		dest  *int64
	}{
		{"SELECT COUNT(*) FROM requests", &stats.RequestsCount},
		{"SELECT COUNT(*) FROM sessions", &stats.SessionsCount},
		{"SELECT COUNT(*) FROM rollups_hourly", &stats.RollupsHourlyCount},
		{"SELECT COUNT(*) FROM rollups_daily", &stats.RollupsDailyCount},
		{"SELECT COUNT(*) FROM import_progress", &stats.ImportProgressCount},
	}

	for _, q := range queries {
		row := s.db.QueryRowContext(ctx, q.query)
		if err := row.Scan(q.dest); err != nil {
			return stats, fmt.Errorf("query %q: %w", q.query, err)
		}
	}
	return stats, nil
}

// DBPath returns the database file path.
func (s *Storage) DBPath() string {
	// Query the database for its file path
	var path string
	row := s.db.QueryRow("PRAGMA database_list")
	var seq int
	var name string
	if err := row.Scan(&seq, &name, &path); err != nil {
		return ""
	}
	return path
}
