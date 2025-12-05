package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// DefaultSessionTimeout is the default gap in seconds between requests
// that defines a new session (30 minutes).
const DefaultSessionTimeout = 1800

// VisitorSessions reconstructs visitor sessions by grouping requests from the same
// IP + User Agent that occur within the session timeout window.
func (s *Storage) VisitorSessions(ctx context.Context, dur time.Duration, host string, limit int, sessionTimeout int) (VisitorSessionSummary, error) {
	var out VisitorSessionSummary
	from := time.Now().Add(-dur)
	if limit <= 0 {
		limit = 50
	}
	if sessionTimeout <= 0 {
		sessionTimeout = DefaultSessionTimeout
	}

	// Query to reconstruct sessions using window functions
	// Groups by IP + user_agent and detects session boundaries when gap > sessionTimeout
	hostFilter := ""
	if host != "" {
		hostFilter = " AND host = ?"
	}
	query := fmt.Sprintf(`
WITH filtered AS (
	SELECT
		ip,
		user_agent,
		browser,
		os,
		country,
		ts,
		path,
		bytes,
		CAST(strftime('%%s', substr(replace(ts, 'T', ' '), 1, 19)) AS INTEGER) AS ts_epoch,
		CASE WHEN instr(path, '?') > 0 THEN substr(path, 1, instr(path, '?') - 1) ELSE path END AS clean_path
	FROM requests
	WHERE ts >= ? AND ts IS NOT NULL AND is_bot = 0%s
),
with_gaps AS (
	SELECT
		*,
		CASE
			WHEN LAG(ts_epoch) OVER (PARTITION BY ip, user_agent ORDER BY ts_epoch) IS NULL THEN 1
			WHEN ts_epoch - LAG(ts_epoch) OVER (PARTITION BY ip, user_agent ORDER BY ts_epoch) > %d THEN 1
			ELSE 0
		END AS new_session
	FROM filtered
),
with_session_id AS (
	SELECT
		*,
		SUM(new_session) OVER (PARTITION BY ip, user_agent ORDER BY ts_epoch) AS session_id
	FROM with_gaps
),
with_row_nums AS (
	SELECT
		*,
		ROW_NUMBER() OVER (PARTITION BY ip, user_agent, session_id ORDER BY ts_epoch ASC) AS rn_asc,
		ROW_NUMBER() OVER (PARTITION BY ip, user_agent, session_id ORDER BY ts_epoch DESC) AS rn_desc
	FROM with_session_id
),
session_stats AS (
	SELECT
		ip,
		user_agent,
		MAX(browser) AS browser,
		MAX(os) AS os,
		MAX(country) AS country,
		session_id,
		MIN(ts) AS start_time,
		MAX(ts) AS end_time,
		MAX(ts_epoch) - MIN(ts_epoch) AS duration_seconds,
		SUM(CASE
			WHEN clean_path NOT LIKE '%%.css'
				AND clean_path NOT LIKE '%%.js'
				AND clean_path NOT LIKE '%%.png'
				AND clean_path NOT LIKE '%%.jpg'
				AND clean_path NOT LIKE '%%.jpeg'
				AND clean_path NOT LIKE '%%.gif'
				AND clean_path NOT LIKE '%%.svg'
				AND clean_path NOT LIKE '%%.ico'
				AND clean_path NOT LIKE '%%.woff%%'
				AND clean_path NOT LIKE '%%.ttf'
				AND clean_path NOT LIKE '%%.eot'
				AND clean_path NOT LIKE '%%.map'
			THEN 1 ELSE 0 END) AS page_views,
		COUNT(*) AS hits,
		IFNULL(SUM(bytes), 0) AS bandwidth_bytes,
		MAX(CASE WHEN rn_asc = 1 THEN clean_path END) AS entry_page,
		MAX(CASE WHEN rn_desc = 1 THEN clean_path END) AS exit_page
	FROM with_row_nums
	GROUP BY ip, user_agent, session_id
)
SELECT
	ip,
	user_agent,
	browser,
	os,
	country,
	start_time,
	end_time,
	duration_seconds,
	page_views,
	hits,
	bandwidth_bytes,
	entry_page,
	exit_page
FROM session_stats
ORDER BY start_time DESC
LIMIT ?`, hostFilter, sessionTimeout)

	args := []any{from}
	if host != "" {
		args = append(args, host)
	}
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return out, fmt.Errorf("query sessions: %w", err)
	}
	defer rows.Close()

	var totalDuration int64
	var bounces int64

	for rows.Next() {
		var sess VisitorSession
		var startStr, endStr sql.NullString
		var ua, browser, os, country, entryPage, exitPage sql.NullString
		var duration, pageViews, hits, bandwidth sql.NullInt64

		if err := rows.Scan(
			&sess.IP,
			&ua,
			&browser,
			&os,
			&country,
			&startStr,
			&endStr,
			&duration,
			&pageViews,
			&hits,
			&bandwidth,
			&entryPage,
			&exitPage,
		); err != nil {
			return out, fmt.Errorf("scan session: %w", err)
		}

		if ua.Valid {
			sess.UserAgent = ua.String
		}
		if browser.Valid {
			sess.Browser = browser.String
		}
		if os.Valid {
			sess.OS = os.String
		}
		if country.Valid {
			sess.Country = country.String
		}
		if entryPage.Valid {
			sess.EntryPage = entryPage.String
		}
		if exitPage.Valid {
			sess.ExitPage = exitPage.String
		}
		if duration.Valid {
			sess.Duration = duration.Int64
		}
		if pageViews.Valid {
			sess.PageViews = pageViews.Int64
		}
		if hits.Valid {
			sess.Hits = hits.Int64
		}
		if bandwidth.Valid {
			sess.BandwidthBytes = bandwidth.Int64
		}

		// Parse timestamps
		sess.StartTime = parseTimestamp(startStr.String)
		sess.EndTime = parseTimestamp(endStr.String)

		// A bounce is a session with only 1 page view
		sess.IsBounce = sess.PageViews <= 1

		out.Sessions = append(out.Sessions, sess)
		out.TotalSessions++
		out.TotalPageViews += sess.PageViews
		totalDuration += sess.Duration
		if sess.IsBounce {
			bounces++
		}
	}

	if err := rows.Err(); err != nil {
		return out, fmt.Errorf("rows error: %w", err)
	}

	// Calculate averages
	if out.TotalSessions > 0 {
		out.AvgDuration = float64(totalDuration) / float64(out.TotalSessions)
		out.AvgPageViews = float64(out.TotalPageViews) / float64(out.TotalSessions)
		out.BounceRate = float64(bounces) / float64(out.TotalSessions) * 100
	}

	// Get additional aggregates: sessions by hour, top entry/exit pages
	out.SessionsByHour, _ = s.sessionsByHour(ctx, from, host, sessionTimeout)
	out.TopEntryPages, _ = s.topEntryPages(ctx, from, host, sessionTimeout, 10)
	out.TopExitPages, _ = s.topExitPages(ctx, from, host, sessionTimeout, 10)

	return out, nil
}

// sessionsByHour returns the distribution of sessions by hour of day.
func (s *Storage) sessionsByHour(ctx context.Context, from time.Time, host string, sessionTimeout int) ([]HourlyBucket, error) {
	hostFilter := ""
	if host != "" {
		hostFilter = " AND host = ?"
	}
	query := fmt.Sprintf(`
WITH filtered AS (
	SELECT
		ip,
		user_agent,
		ts,
		CAST(strftime('%%s', substr(replace(ts, 'T', ' '), 1, 19)) AS INTEGER) AS ts_epoch
	FROM requests
	WHERE ts >= ? AND ts IS NOT NULL AND is_bot = 0%s
),
with_gaps AS (
	SELECT
		*,
		CASE
			WHEN LAG(ts_epoch) OVER (PARTITION BY ip, user_agent ORDER BY ts_epoch) IS NULL THEN 1
			WHEN ts_epoch - LAG(ts_epoch) OVER (PARTITION BY ip, user_agent ORDER BY ts_epoch) > %d THEN 1
			ELSE 0
		END AS new_session
	FROM filtered
),
sessions AS (
	SELECT ip, user_agent, ts, new_session
	FROM with_gaps
	WHERE new_session = 1
)
SELECT
	CAST(strftime('%%H', substr(replace(ts, 'T', ' '), 1, 19)) AS INTEGER) AS hour,
	COUNT(*) AS sessions
FROM sessions
GROUP BY hour
ORDER BY hour`, hostFilter, sessionTimeout)

	args := []any{from}
	if host != "" {
		args = append(args, host)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Initialize all 24 hours with 0
	buckets := make([]HourlyBucket, 24)
	for i := range buckets {
		buckets[i].Hour = i
	}

	for rows.Next() {
		var hour int
		var count int64
		if err := rows.Scan(&hour, &count); err != nil {
			return nil, err
		}
		if hour >= 0 && hour < 24 {
			buckets[hour].Sessions = count
		}
	}

	return buckets, rows.Err()
}

// topEntryPages returns the most common entry pages (first page of a session).
func (s *Storage) topEntryPages(ctx context.Context, from time.Time, host string, sessionTimeout int, limit int) ([]PageCount, error) {
	hostFilter := ""
	if host != "" {
		hostFilter = " AND host = ?"
	}
	query := fmt.Sprintf(`
WITH filtered AS (
	SELECT
		ip,
		user_agent,
		ts,
		path,
		CAST(strftime('%%s', substr(replace(ts, 'T', ' '), 1, 19)) AS INTEGER) AS ts_epoch,
		CASE WHEN instr(path, '?') > 0 THEN substr(path, 1, instr(path, '?') - 1) ELSE path END AS clean_path
	FROM requests
	WHERE ts >= ? AND ts IS NOT NULL AND is_bot = 0%s
),
with_gaps AS (
	SELECT
		*,
		CASE
			WHEN LAG(ts_epoch) OVER (PARTITION BY ip, user_agent ORDER BY ts_epoch) IS NULL THEN 1
			WHEN ts_epoch - LAG(ts_epoch) OVER (PARTITION BY ip, user_agent ORDER BY ts_epoch) > %d THEN 1
			ELSE 0
		END AS new_session
	FROM filtered
),
entry_pages AS (
	SELECT clean_path
	FROM with_gaps
	WHERE new_session = 1
	  AND clean_path NOT LIKE '%%.css'
	  AND clean_path NOT LIKE '%%.js'
	  AND clean_path NOT LIKE '%%.png'
	  AND clean_path NOT LIKE '%%.jpg'
	  AND clean_path NOT LIKE '%%.gif'
	  AND clean_path NOT LIKE '%%.svg'
	  AND clean_path NOT LIKE '%%.ico'
)
SELECT clean_path, COUNT(*) as cnt
FROM entry_pages
GROUP BY clean_path
ORDER BY cnt DESC
LIMIT ?`, hostFilter,
		sessionTimeout,
	)

	args := []any{from}
	if host != "" {
		args = append(args, host)
	}
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []PageCount
	for rows.Next() {
		var pc PageCount
		if err := rows.Scan(&pc.Path, &pc.Count); err != nil {
			return nil, err
		}
		results = append(results, pc)
	}
	return results, rows.Err()
}

// topExitPages returns the most common exit pages (last page of a session).
func (s *Storage) topExitPages(ctx context.Context, from time.Time, host string, sessionTimeout int, limit int) ([]PageCount, error) {
	hostFilter := ""
	if host != "" {
		hostFilter = " AND host = ?"
	}
	query := fmt.Sprintf(`
WITH filtered AS (
	SELECT
		ip,
		user_agent,
		ts,
		path,
		CAST(strftime('%%s', substr(replace(ts, 'T', ' '), 1, 19)) AS INTEGER) AS ts_epoch,
		CASE WHEN instr(path, '?') > 0 THEN substr(path, 1, instr(path, '?') - 1) ELSE path END AS clean_path
	FROM requests
	WHERE ts >= ? AND ts IS NOT NULL AND is_bot = 0%s
),
with_gaps AS (
	SELECT
		*,
		CASE
			WHEN LAG(ts_epoch) OVER (PARTITION BY ip, user_agent ORDER BY ts_epoch) IS NULL THEN 1
			WHEN ts_epoch - LAG(ts_epoch) OVER (PARTITION BY ip, user_agent ORDER BY ts_epoch) > %d THEN 1
			ELSE 0
		END AS new_session
	FROM filtered
),
with_session_id AS (
	SELECT
		*,
		SUM(new_session) OVER (PARTITION BY ip, user_agent ORDER BY ts_epoch) AS session_id
	FROM with_gaps
),
exit_pages AS (
	SELECT clean_path
	FROM (
		SELECT
			clean_path,
			ROW_NUMBER() OVER (PARTITION BY ip, user_agent, session_id ORDER BY ts_epoch DESC) AS rn
		FROM with_session_id
		WHERE clean_path NOT LIKE '%%.css'
		  AND clean_path NOT LIKE '%%.js'
		  AND clean_path NOT LIKE '%%.png'
		  AND clean_path NOT LIKE '%%.jpg'
		  AND clean_path NOT LIKE '%%.gif'
		  AND clean_path NOT LIKE '%%.svg'
		  AND clean_path NOT LIKE '%%.ico'
	)
	WHERE rn = 1
)
SELECT clean_path, COUNT(*) as cnt
FROM exit_pages
GROUP BY clean_path
ORDER BY cnt DESC
LIMIT ?`, hostFilter, sessionTimeout)

	args := []any{from}
	if host != "" {
		args = append(args, host)
	}
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []PageCount
	for rows.Next() {
		var pc PageCount
		if err := rows.Scan(&pc.Path, &pc.Count); err != nil {
			return nil, err
		}
		results = append(results, pc)
	}
	return results, rows.Err()
}

// CreateSession creates a new authentication session.
func (s *Storage) CreateSession(ctx context.Context, token string, expiresAt time.Time) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	stmt := s.stmtInsertSession
	_, err := stmt.ExecContext(ctx, token, expiresAt, time.Now())
	return err
}

// GetSession retrieves an authentication session by token.
func (s *Storage) GetSession(ctx context.Context, token string) (*Session, error) {
	stmt := s.stmtGetSession
	row := stmt.QueryRowContext(ctx, token)
	var sess Session
	var expiresStr, createdStr sql.NullString
	if err := row.Scan(&sess.Token, &expiresStr, &createdStr); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	if expiresStr.Valid {
		sess.ExpiresAt, _ = time.Parse("2006-01-02 15:04:05.999999999-07:00", expiresStr.String)
		if sess.ExpiresAt.IsZero() {
			sess.ExpiresAt, _ = time.Parse(time.RFC3339Nano, expiresStr.String)
		}
	}
	if createdStr.Valid {
		sess.CreatedAt, _ = time.Parse("2006-01-02 15:04:05.999999999-07:00", createdStr.String)
		if sess.CreatedAt.IsZero() {
			sess.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdStr.String)
		}
	}
	return &sess, nil
}

// DeleteSession deletes an authentication session by token.
func (s *Storage) DeleteSession(ctx context.Context, token string) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	stmt := s.stmtDeleteSession
	_, err := stmt.ExecContext(ctx, token)
	return err
}

// CleanupExpiredSessions removes all expired authentication sessions.
func (s *Storage) CleanupExpiredSessions(ctx context.Context) (int64, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	result, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at < datetime('now')`)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// parseTimestamp attempts to parse a timestamp string in various formats.
func parseTimestamp(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	formats := []string{
		"2006-01-02 15:04:05.999999999-07:00",
		time.RFC3339Nano,
		"2006-01-02T15:04:05Z",
		"2006-01-02 15:04:05",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t
		}
	}
	return time.Time{}
}
