package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Summary returns aggregated statistics for the given time range and optional host filter.
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
		user_agent,
		ts_epoch,
		CASE
			WHEN LAG(ts_epoch) OVER (PARTITION BY ip, user_agent ORDER BY ts_epoch) IS NULL THEN 1
			WHEN ts_epoch - LAG(ts_epoch) OVER (PARTITION BY ip, user_agent ORDER BY ts_epoch) > 1800 THEN 1
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
	IFNULL((SELECT COUNT(DISTINCT ip || '|' || COALESCE(user_agent, '')) FROM classified), 0) AS unique_visitors
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
	out.Bots, _ = s.botStats(ctx, from, host)
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

func (s *Storage) botStats(ctx context.Context, from time.Time, host string) (BotStats, error) {
	out := BotStats{
		ByIntent: make(map[string]BotIntentStats),
	}

	// Get total bot hits and bandwidth
	var totalRows *sql.Rows
	var err error
	if host == "" {
		totalRows, err = s.db.QueryContext(ctx, `
SELECT COUNT(*), IFNULL(SUM(bytes), 0)
FROM requests WHERE ts >= ? AND is_bot = 1
`, from)
	} else {
		totalRows, err = s.db.QueryContext(ctx, `
SELECT COUNT(*), IFNULL(SUM(bytes), 0)
FROM requests WHERE ts >= ? AND is_bot = 1 AND host = ?
`, from, host)
	}
	if err != nil {
		return out, err
	}
	if totalRows.Next() {
		if err := totalRows.Scan(&out.TotalHits, &out.BandwidthBytes); err != nil {
			totalRows.Close()
			return out, err
		}
	}
	totalRows.Close() // Close before next query to avoid connection pool deadlock

	// Get breakdown by intent
	var intentRows *sql.Rows
	if host == "" {
		intentRows, err = s.db.QueryContext(ctx, `
SELECT CASE WHEN bot_intent = '' THEN 'unknown' ELSE bot_intent END AS intent,
       COUNT(*) AS hits, IFNULL(SUM(bytes), 0) AS bandwidth
FROM requests WHERE ts >= ? AND is_bot = 1
GROUP BY intent
ORDER BY hits DESC
`, from)
	} else {
		intentRows, err = s.db.QueryContext(ctx, `
SELECT CASE WHEN bot_intent = '' THEN 'unknown' ELSE bot_intent END AS intent,
       COUNT(*) AS hits, IFNULL(SUM(bytes), 0) AS bandwidth
FROM requests WHERE ts >= ? AND is_bot = 1 AND host = ?
GROUP BY intent
ORDER BY hits DESC
`, from, host)
	}
	if err != nil {
		return out, err
	}
	defer intentRows.Close()

	for intentRows.Next() {
		var intent string
		var stats BotIntentStats
		if err := intentRows.Scan(&intent, &stats.Hits, &stats.BandwidthBytes); err != nil {
			return out, err
		}
		out.ByIntent[intent] = stats
	}

	return out, intentRows.Err()
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

// TimeSeriesRange returns time series statistics for the given duration.
func (s *Storage) TimeSeriesRange(ctx context.Context, dur time.Duration, host string) ([]TimeSeriesStat, error) {
	return s.timeSeries(ctx, time.Now().Add(-dur), host)
}

// Geo returns geographic statistics for the given duration.
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
