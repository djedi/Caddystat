package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// BandwidthStats returns comprehensive bandwidth statistics per host and path.
func (s *Storage) BandwidthStats(ctx context.Context, dur time.Duration, host string, limit int) (BandwidthStats, error) {
	stats := BandwidthStats{
		ByHost:        []HostBandwidth{},
		ByPath:        []PathBandwidth{},
		ByContentType: []ContentBandwidth{},
		TimeSeries:    []BandwidthTimeStat{},
	}
	from := time.Now().Add(-dur)
	if limit <= 0 {
		limit = 10
	}

	// Get total bandwidth
	totalBytes, err := s.totalBandwidth(ctx, from, host)
	if err != nil {
		return stats, fmt.Errorf("total bandwidth: %w", err)
	}
	stats.TotalBytes = totalBytes
	stats.TotalHuman = humanizeBytes(totalBytes)

	// Get bandwidth by host
	byHost, err := s.bandwidthByHost(ctx, from, limit)
	if err != nil {
		return stats, fmt.Errorf("bandwidth by host: %w", err)
	}
	stats.ByHost = byHost

	// Get bandwidth by path
	byPath, err := s.bandwidthByPath(ctx, from, host, limit)
	if err != nil {
		return stats, fmt.Errorf("bandwidth by path: %w", err)
	}
	stats.ByPath = byPath

	// Get bandwidth by content type
	byContentType, err := s.bandwidthByContentType(ctx, from, host, limit)
	if err != nil {
		return stats, fmt.Errorf("bandwidth by content type: %w", err)
	}
	stats.ByContentType = byContentType

	// Get bandwidth time series
	timeSeries, err := s.bandwidthTimeSeries(ctx, from, host)
	if err != nil {
		return stats, fmt.Errorf("bandwidth time series: %w", err)
	}
	stats.TimeSeries = timeSeries

	return stats, nil
}

// totalBandwidth returns the total bytes transferred in the given time range.
func (s *Storage) totalBandwidth(ctx context.Context, from time.Time, host string) (int64, error) {
	query := `SELECT IFNULL(SUM(bytes), 0) FROM requests WHERE ts >= ?`
	args := []any{from}
	if host != "" {
		query += " AND host = ?"
		args = append(args, host)
	}

	var total int64
	if err := s.db.QueryRowContext(ctx, query, args...).Scan(&total); err != nil {
		return 0, err
	}
	return total, nil
}

// bandwidthByHost returns bandwidth statistics grouped by host.
func (s *Storage) bandwidthByHost(ctx context.Context, from time.Time, limit int) ([]HostBandwidth, error) {
	query := `
WITH totals AS (
	SELECT IFNULL(SUM(bytes), 0) AS total_bytes FROM requests WHERE ts >= ?
)
SELECT
	host,
	IFNULL(SUM(bytes), 0) AS bytes,
	COUNT(*) AS requests,
	ROUND(IFNULL(AVG(bytes), 0), 2) AS avg_bytes,
	ROUND(100.0 * IFNULL(SUM(bytes), 0) / NULLIF((SELECT total_bytes FROM totals), 0), 2) AS percent
FROM requests
WHERE ts >= ?
GROUP BY host
ORDER BY bytes DESC
LIMIT ?`

	rows, err := s.db.QueryContext(ctx, query, from, from, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []HostBandwidth
	for rows.Next() {
		var hb HostBandwidth
		if err := rows.Scan(&hb.Host, &hb.Bytes, &hb.Requests, &hb.AvgBytes, &hb.Percent); err != nil {
			return nil, err
		}
		hb.BytesHuman = humanizeBytes(hb.Bytes)
		results = append(results, hb)
	}
	return results, rows.Err()
}

// bandwidthByPath returns bandwidth statistics grouped by path.
func (s *Storage) bandwidthByPath(ctx context.Context, from time.Time, host string, limit int) ([]PathBandwidth, error) {
	query := `
WITH filtered AS (
	SELECT
		CASE WHEN instr(path, '?') > 0 THEN substr(path, 1, instr(path, '?') - 1) ELSE path END AS clean_path,
		bytes
	FROM requests
	WHERE ts >= ?`

	args := []any{from}
	if host != "" {
		query += " AND host = ?"
		args = append(args, host)
	}

	query += `
),
totals AS (
	SELECT IFNULL(SUM(bytes), 0) AS total_bytes FROM filtered
)
SELECT
	clean_path,
	IFNULL(SUM(bytes), 0) AS bytes,
	COUNT(*) AS requests,
	ROUND(IFNULL(AVG(bytes), 0), 2) AS avg_bytes,
	ROUND(100.0 * IFNULL(SUM(bytes), 0) / NULLIF((SELECT total_bytes FROM totals), 0), 2) AS percent
FROM filtered
GROUP BY clean_path
ORDER BY bytes DESC
LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []PathBandwidth
	for rows.Next() {
		var pb PathBandwidth
		if err := rows.Scan(&pb.Path, &pb.Bytes, &pb.Requests, &pb.AvgBytes, &pb.Percent); err != nil {
			return nil, err
		}
		pb.BytesHuman = humanizeBytes(pb.Bytes)
		results = append(results, pb)
	}
	return results, rows.Err()
}

// bandwidthByContentType returns bandwidth statistics grouped by content type (file extension).
func (s *Storage) bandwidthByContentType(ctx context.Context, from time.Time, host string, limit int) ([]ContentBandwidth, error) {
	query := `
WITH filtered AS (
	SELECT
		bytes,
		CASE
			WHEN path LIKE '%.html' OR path LIKE '%.htm' THEN 'HTML'
			WHEN path LIKE '%.css' THEN 'CSS'
			WHEN path LIKE '%.js' THEN 'JavaScript'
			WHEN path LIKE '%.json' THEN 'JSON'
			WHEN path LIKE '%.xml' THEN 'XML'
			WHEN path LIKE '%.png' THEN 'PNG Image'
			WHEN path LIKE '%.jpg' OR path LIKE '%.jpeg' THEN 'JPEG Image'
			WHEN path LIKE '%.gif' THEN 'GIF Image'
			WHEN path LIKE '%.svg' THEN 'SVG Image'
			WHEN path LIKE '%.webp' THEN 'WebP Image'
			WHEN path LIKE '%.ico' THEN 'Icon'
			WHEN path LIKE '%.woff' OR path LIKE '%.woff2' THEN 'Web Font'
			WHEN path LIKE '%.ttf' OR path LIKE '%.otf' OR path LIKE '%.eot' THEN 'Font'
			WHEN path LIKE '%.pdf' THEN 'PDF'
			WHEN path LIKE '%.zip' OR path LIKE '%.gz' OR path LIKE '%.tar' THEN 'Archive'
			WHEN path LIKE '%.mp4' OR path LIKE '%.webm' OR path LIKE '%.avi' THEN 'Video'
			WHEN path LIKE '%.mp3' OR path LIKE '%.wav' OR path LIKE '%.ogg' THEN 'Audio'
			WHEN path NOT LIKE '%.%' OR path LIKE '%/' THEN 'Page'
			ELSE 'Other'
		END AS content_type
	FROM requests
	WHERE ts >= ?`

	args := []any{from}
	if host != "" {
		query += " AND host = ?"
		args = append(args, host)
	}

	query += `
),
totals AS (
	SELECT IFNULL(SUM(bytes), 0) AS total_bytes FROM filtered
)
SELECT
	content_type,
	IFNULL(SUM(bytes), 0) AS bytes,
	COUNT(*) AS requests,
	ROUND(100.0 * IFNULL(SUM(bytes), 0) / NULLIF((SELECT total_bytes FROM totals), 0), 2) AS percent
FROM filtered
GROUP BY content_type
ORDER BY bytes DESC
LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []ContentBandwidth
	for rows.Next() {
		var cb ContentBandwidth
		if err := rows.Scan(&cb.ContentType, &cb.Bytes, &cb.Requests, &cb.Percent); err != nil {
			return nil, err
		}
		cb.BytesHuman = humanizeBytes(cb.Bytes)
		results = append(results, cb)
	}
	return results, rows.Err()
}

// bandwidthTimeSeries returns hourly bandwidth statistics.
func (s *Storage) bandwidthTimeSeries(ctx context.Context, from time.Time, host string) ([]BandwidthTimeStat, error) {
	query := `
SELECT
	strftime('%Y-%m-%dT%H:00:00Z', ts) as bucket,
	IFNULL(SUM(bytes), 0) as bytes,
	COUNT(*) as requests
FROM requests
WHERE ts >= ? AND ts IS NOT NULL`

	args := []any{from}
	if host != "" {
		query += " AND host = ?"
		args = append(args, host)
	}

	query += `
GROUP BY bucket
HAVING bucket IS NOT NULL
ORDER BY bucket ASC`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []BandwidthTimeStat
	for rows.Next() {
		var bs BandwidthTimeStat
		var bucketStr sql.NullString
		if err := rows.Scan(&bucketStr, &bs.Bytes, &bs.Requests); err != nil {
			return nil, err
		}
		if !bucketStr.Valid {
			continue
		}
		bs.Bucket, _ = time.Parse(time.RFC3339, bucketStr.String)
		results = append(results, bs)
	}
	return results, rows.Err()
}
