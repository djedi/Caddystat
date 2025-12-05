package storage

import (
	"context"
	"fmt"
	"time"
)

// PerformanceStats returns comprehensive performance statistics including response time percentiles.
func (s *Storage) PerformanceStats(ctx context.Context, dur time.Duration, host string) (PerformanceStats, error) {
	var stats PerformanceStats
	from := time.Now().Add(-dur)

	// Get response time percentiles
	rtStats, err := s.responseTimeStats(ctx, from, host)
	if err != nil {
		return stats, fmt.Errorf("response time stats: %w", err)
	}
	stats.ResponseTime = rtStats

	// Get slow pages
	slowPages, err := s.slowPages(ctx, from, host, 10)
	if err != nil {
		return stats, fmt.Errorf("slow pages: %w", err)
	}
	stats.SlowPages = slowPages

	// Get performance by status code range
	byStatus, err := s.perfByStatus(ctx, from, host)
	if err != nil {
		return stats, fmt.Errorf("perf by status: %w", err)
	}
	stats.ByStatus = byStatus

	return stats, nil
}

// responseTimeStats calculates response time percentiles using SQLite's window functions.
func (s *Storage) responseTimeStats(ctx context.Context, from time.Time, host string) (ResponseTimeStats, error) {
	var stats ResponseTimeStats

	query := `
WITH filtered AS (
	SELECT resp_time_ms
	FROM requests
	WHERE ts >= ? AND resp_time_ms > 0`

	args := []any{from}
	if host != "" {
		query += " AND host = ?"
		args = append(args, host)
	}

	query += `
),
ordered AS (
	SELECT
		resp_time_ms,
		ROW_NUMBER() OVER (ORDER BY resp_time_ms) AS rn,
		COUNT(*) OVER () AS total
	FROM filtered
),
percentiles AS (
	SELECT
		MIN(resp_time_ms) AS min_val,
		MAX(resp_time_ms) AS max_val,
		AVG(resp_time_ms) AS avg_val,
		COUNT(*) AS cnt,
		-- Standard deviation calculation
		SQRT(AVG(resp_time_ms * resp_time_ms) - AVG(resp_time_ms) * AVG(resp_time_ms)) AS std_dev
	FROM filtered
),
p_values AS (
	SELECT
		MAX(CASE WHEN rn >= total * 0.50 AND rn < total * 0.50 + 1 THEN resp_time_ms END) AS p50,
		MAX(CASE WHEN rn >= total * 0.90 AND rn < total * 0.90 + 1 THEN resp_time_ms END) AS p90,
		MAX(CASE WHEN rn >= total * 0.95 AND rn < total * 0.95 + 1 THEN resp_time_ms END) AS p95,
		MAX(CASE WHEN rn >= total * 0.99 AND rn < total * 0.99 + 1 THEN resp_time_ms END) AS p99
	FROM ordered
)
SELECT
	IFNULL(p.min_val, 0),
	IFNULL(p.max_val, 0),
	IFNULL(p.avg_val, 0),
	IFNULL(pv.p50, 0),
	IFNULL(pv.p90, 0),
	IFNULL(pv.p95, 0),
	IFNULL(pv.p99, 0),
	IFNULL(p.cnt, 0),
	IFNULL(p.std_dev, 0)
FROM percentiles p, p_values pv`

	row := s.db.QueryRowContext(ctx, query, args...)
	if err := row.Scan(
		&stats.Min, &stats.Max, &stats.Avg,
		&stats.P50, &stats.P90, &stats.P95, &stats.P99,
		&stats.Count, &stats.StdDev,
	); err != nil {
		return stats, err
	}

	return stats, nil
}

// slowPages returns the slowest pages by average response time.
func (s *Storage) slowPages(ctx context.Context, from time.Time, host string, limit int) ([]SlowPageStat, error) {
	query := `
WITH page_stats AS (
	SELECT
		CASE WHEN instr(path, '?') > 0 THEN substr(path, 1, instr(path, '?') - 1) ELSE path END AS clean_path,
		resp_time_ms,
		bytes,
		status
	FROM requests
	WHERE ts >= ? AND resp_time_ms > 0`

	args := []any{from}
	if host != "" {
		query += " AND host = ?"
		args = append(args, host)
	}

	query += `
),
ordered AS (
	SELECT
		clean_path,
		resp_time_ms,
		bytes,
		status,
		ROW_NUMBER() OVER (PARTITION BY clean_path ORDER BY resp_time_ms) AS rn,
		COUNT(*) OVER (PARTITION BY clean_path) AS path_total
	FROM page_stats
),
path_percentiles AS (
	SELECT
		clean_path,
		MAX(CASE WHEN rn >= path_total * 0.95 AND rn < path_total * 0.95 + 1 THEN resp_time_ms END) AS p95
	FROM ordered
	GROUP BY clean_path
)
SELECT
	ps.clean_path,
	COUNT(*) AS cnt,
	AVG(ps.resp_time_ms) AS avg_resp,
	MAX(ps.resp_time_ms) AS max_resp,
	IFNULL(pp.p95, AVG(ps.resp_time_ms)) AS p95_resp,
	SUM(ps.bytes) AS total_bytes,
	ROUND(100.0 * SUM(CASE WHEN ps.status >= 400 THEN 1 ELSE 0 END) / NULLIF(COUNT(*), 0), 2) AS error_rate
FROM page_stats ps
LEFT JOIN path_percentiles pp ON ps.clean_path = pp.clean_path
GROUP BY ps.clean_path
HAVING COUNT(*) >= 5
ORDER BY avg_resp DESC
LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []SlowPageStat
	for rows.Next() {
		var sp SlowPageStat
		if err := rows.Scan(&sp.Path, &sp.Count, &sp.AvgResponseMs, &sp.MaxResponseMs, &sp.P95ResponseMs, &sp.TotalBytes, &sp.ErrorRate); err != nil {
			return nil, err
		}
		results = append(results, sp)
	}

	return results, rows.Err()
}

// perfByStatus returns performance statistics grouped by HTTP status code range.
func (s *Storage) perfByStatus(ctx context.Context, from time.Time, host string) ([]StatusPerfStat, error) {
	query := `
WITH status_data AS (
	SELECT
		CASE
			WHEN status >= 200 AND status < 300 THEN '2xx'
			WHEN status >= 300 AND status < 400 THEN '3xx'
			WHEN status >= 400 AND status < 500 THEN '4xx'
			WHEN status >= 500 THEN '5xx'
			ELSE 'other'
		END AS status_range,
		resp_time_ms,
		ROW_NUMBER() OVER (PARTITION BY
			CASE
				WHEN status >= 200 AND status < 300 THEN '2xx'
				WHEN status >= 300 AND status < 400 THEN '3xx'
				WHEN status >= 400 AND status < 500 THEN '4xx'
				WHEN status >= 500 THEN '5xx'
				ELSE 'other'
			END
			ORDER BY resp_time_ms
		) AS rn,
		COUNT(*) OVER (PARTITION BY
			CASE
				WHEN status >= 200 AND status < 300 THEN '2xx'
				WHEN status >= 300 AND status < 400 THEN '3xx'
				WHEN status >= 400 AND status < 500 THEN '4xx'
				WHEN status >= 500 THEN '5xx'
				ELSE 'other'
			END
		) AS group_total
	FROM requests
	WHERE ts >= ? AND resp_time_ms > 0`

	args := []any{from}
	if host != "" {
		query += " AND host = ?"
		args = append(args, host)
	}

	query += `
),
p95_values AS (
	SELECT
		status_range,
		MAX(CASE WHEN rn >= group_total * 0.95 AND rn < group_total * 0.95 + 1 THEN resp_time_ms END) AS p95
	FROM status_data
	GROUP BY status_range
)
SELECT
	sd.status_range,
	COUNT(*) AS cnt,
	AVG(sd.resp_time_ms) AS avg_resp,
	IFNULL(pv.p95, AVG(sd.resp_time_ms)) AS p95_resp
FROM status_data sd
LEFT JOIN p95_values pv ON sd.status_range = pv.status_range
GROUP BY sd.status_range
ORDER BY sd.status_range`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []StatusPerfStat
	for rows.Next() {
		var sp StatusPerfStat
		if err := rows.Scan(&sp.StatusRange, &sp.Count, &sp.AvgResponseMs, &sp.P95ResponseMs); err != nil {
			return nil, err
		}
		results = append(results, sp)
	}

	return results, rows.Err()
}
