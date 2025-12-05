package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// MonthlyHistory returns monthly statistics for the specified number of months.
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
		user_agent,
		ts_epoch,
		CASE
			WHEN LAG(ts_epoch) OVER (PARTITION BY month_key, ip, user_agent ORDER BY ts_epoch) IS NULL THEN 1
			WHEN ts_epoch - LAG(ts_epoch) OVER (PARTITION BY month_key, ip, user_agent ORDER BY ts_epoch) > 1800 THEN 1
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
	IFNULL(COUNT(DISTINCT ip || '|' || COALESCE(user_agent, '')), 0) AS unique_visitors
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

// DailyHistory returns daily statistics for the current month.
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
		user_agent,
		ts_epoch,
		CASE
			WHEN LAG(ts_epoch) OVER (PARTITION BY day_key, ip, user_agent ORDER BY ts_epoch) IS NULL THEN 1
			WHEN ts_epoch - LAG(ts_epoch) OVER (PARTITION BY day_key, ip, user_agent ORDER BY ts_epoch) > 1800 THEN 1
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
