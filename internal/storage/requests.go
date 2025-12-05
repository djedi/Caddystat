package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// InsertRequest inserts a new request record and updates rollup tables.
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

	// Use prepared statement within the transaction
	stmt := tx.StmtContext(ctx, s.stmtInsertRequest)
	_, err = stmt.ExecContext(ctx, r.Timestamp, r.Host, r.Path, r.Status, r.Bytes, r.IP, r.Referrer, r.UserAgent, r.ResponseTime, r.Country, r.Region, r.City, r.Browser, r.BrowserVersion, r.OS, r.OSVersion, r.DeviceType, isBot, r.BotName, r.BotIntent)
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

// Cleanup deletes requests older than the retention period.
func (s *Storage) Cleanup(ctx context.Context, retentionDays int) error {
	_, err := s.db.ExecContext(ctx, `
DELETE FROM requests WHERE ts < datetime('now', ?)
`, fmt.Sprintf("-%d days", retentionDays))
	return err
}

// Vacuum runs SQLite VACUUM to reclaim space and defragment the database.
// This is useful to run after bulk deletes (like data retention cleanup).
// Returns the bytes freed (approximate, based on file size before/after).
func (s *Storage) Vacuum(ctx context.Context) (int64, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	// Get file size before vacuum
	sizeBefore, _ := s.DBFileSize()

	// Run VACUUM - this rebuilds the database file
	_, err := s.db.ExecContext(ctx, "VACUUM")
	if err != nil {
		return 0, fmt.Errorf("vacuum failed: %w", err)
	}

	// Get file size after vacuum
	sizeAfter, _ := s.DBFileSize()

	// Return bytes freed (positive number if space was reclaimed)
	bytesFreed := sizeBefore - sizeAfter
	if bytesFreed < 0 {
		bytesFreed = 0
	}
	return bytesFreed, nil
}

// RecentRequests returns the most recent N requests, optionally filtered by host.
// Uses a 24-hour time filter to leverage the ts index and avoid full table scans.
func (s *Storage) RecentRequests(ctx context.Context, limit int, host string) ([]RecentRequest, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	query := `
SELECT
	id, ts, host, path, status, bytes, ip, referrer, user_agent,
	resp_time_ms, country, region, city, browser, browser_version,
	os, os_version, device_type, is_bot, bot_name
FROM requests
WHERE ts >= datetime('now', '-24 hours')`

	args := []any{}
	if host != "" {
		query += " AND host = ?"
		args = append(args, host)
	}
	query += " ORDER BY ts DESC LIMIT ?"
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []RecentRequest
	for rows.Next() {
		var r RecentRequest
		var tsStr sql.NullString
		var isBot int
		if err := rows.Scan(
			&r.ID, &tsStr, &r.Host, &r.Path, &r.Status, &r.Bytes, &r.IP, &r.Referrer, &r.UserAgent,
			&r.ResponseTime, &r.Country, &r.Region, &r.City, &r.Browser, &r.BrowserVersion,
			&r.OS, &r.OSVersion, &r.DeviceType, &isBot, &r.BotName,
		); err != nil {
			return nil, err
		}
		r.IsBot = isBot == 1
		if tsStr.Valid {
			r.Timestamp = parseTimestamp(tsStr.String)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ExportRequests iterates over requests in the given time range, calling the callback
// for each batch of requests. This uses streaming to handle large datasets efficiently.
func (s *Storage) ExportRequests(ctx context.Context, dur time.Duration, host string, batchSize int, callback ExportRequestsCallback) error {
	from := time.Now().Add(-dur)
	if batchSize <= 0 {
		batchSize = 1000
	}

	query := `
SELECT
	id, ts, host, path, status, bytes, ip, referrer, user_agent,
	resp_time_ms, country, region, city, browser, browser_version,
	os, os_version, device_type, is_bot, bot_name
FROM requests
WHERE ts >= ?`

	args := []any{from}
	if host != "" {
		query += " AND host = ?"
		args = append(args, host)
	}
	query += " ORDER BY ts ASC"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	batch := make([]ExportRequest, 0, batchSize)
	for rows.Next() {
		var r ExportRequest
		var tsStr sql.NullString
		var isBot int
		if err := rows.Scan(
			&r.ID, &tsStr, &r.Host, &r.Path, &r.Status, &r.Bytes, &r.IP, &r.Referrer, &r.UserAgent,
			&r.ResponseTimeMs, &r.Country, &r.Region, &r.City, &r.Browser, &r.BrowserVersion,
			&r.OS, &r.OSVersion, &r.DeviceType, &isBot, &r.BotName,
		); err != nil {
			return err
		}
		r.IsBot = isBot == 1
		if tsStr.Valid {
			r.Timestamp = parseTimestamp(tsStr.String)
		}
		batch = append(batch, r)

		if len(batch) >= batchSize {
			if err := callback(batch); err != nil {
				return err
			}
			batch = batch[:0]
		}
	}

	// Process remaining items
	if len(batch) > 0 {
		if err := callback(batch); err != nil {
			return err
		}
	}

	return rows.Err()
}
