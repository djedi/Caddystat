package storage

import (
	"context"
	"database/sql"
	"time"
)

// Visitors returns top visitor IPs with their stats.
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

// Browsers returns browser usage statistics.
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

// OperatingSystems returns OS usage statistics.
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

// Robots returns bot/spider statistics.
func (s *Storage) Robots(ctx context.Context, dur time.Duration, host string, limit int) ([]RobotStat, error) {
	from := time.Now().Add(-dur)
	if limit <= 0 {
		limit = 20
	}

	query := `
SELECT
	CASE WHEN bot_name = '' THEN 'Unknown Bot' ELSE bot_name END as name,
	CASE WHEN bot_intent = '' THEN 'unknown' ELSE bot_intent END as intent,
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
	query += " GROUP BY bot_name, bot_intent ORDER BY hits DESC LIMIT ?"
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
		if err := rows.Scan(&r.Name, &r.Intent, &r.Hits, &r.BandwidthBytes, &lastVisitStr); err != nil {
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

// Referrers returns referrer statistics.
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
