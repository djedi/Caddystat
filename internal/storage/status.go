package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"time"
)

// DB returns the underlying database connection.
func (s *Storage) DB() *sql.DB {
	return s.db
}

// Health checks database connectivity.
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

// Ping verifies database connectivity by executing a simple query.
func (s *Storage) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
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

// DBFileSize returns the database file size in bytes.
func (s *Storage) DBFileSize() (int64, error) {
	dbPath := s.DBPath()
	if dbPath == "" || dbPath == ":memory:" {
		return 0, nil
	}
	info, err := os.Stat(dbPath)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

// GetLastImportTime returns the most recent request timestamp, or zero if no data.
func (s *Storage) GetLastImportTime(ctx context.Context) (time.Time, error) {
	var ts sql.NullString
	row := s.db.QueryRowContext(ctx, `SELECT MAX(ts) FROM requests`)
	if err := row.Scan(&ts); err != nil {
		return time.Time{}, err
	}
	if !ts.Valid || ts.String == "" {
		return time.Time{}, nil
	}
	// Parse the timestamp string (Go's time format from sql driver)
	parsed, err := time.Parse("2006-01-02 15:04:05 -0700 MST", ts.String)
	if err != nil {
		// Try alternate formats
		parsed, err = time.Parse("2006-01-02T15:04:05Z", ts.String)
		if err != nil {
			parsed, err = time.Parse("2006-01-02 15:04:05", ts.String)
			if err != nil {
				return time.Time{}, fmt.Errorf("parsing timestamp %q: %w", ts.String, err)
			}
		}
	}
	return parsed.UTC(), nil
}

// GetSystemStatus returns comprehensive system status information.
func (s *Storage) GetSystemStatus(ctx context.Context) (SystemStatus, error) {
	var status SystemStatus

	// Get database file size
	dbSize, err := s.DBFileSize()
	if err == nil {
		status.DBSizeBytes = dbSize
		status.DBSizeHuman = humanizeBytes(dbSize)
	}

	// Get table counts
	dbStats, err := s.GetDatabaseStats(ctx)
	if err != nil {
		return status, fmt.Errorf("getting database stats: %w", err)
	}
	status.RequestsCount = dbStats.RequestsCount
	status.HourlyRollups = dbStats.RollupsHourlyCount
	status.DailyRollups = dbStats.RollupsDailyCount
	status.ActiveSessions = dbStats.SessionsCount
	status.TrackedLogFiles = dbStats.ImportProgressCount

	// Get last import time
	lastImport, err := s.GetLastImportTime(ctx)
	if err == nil && !lastImport.IsZero() {
		status.LastImportTime = &lastImport
	}

	// Get import error statistics
	totalErrors, err := s.GetImportErrorsTotal(ctx)
	if err == nil {
		status.TotalParseErrors = totalErrors
	}

	// Get detailed import errors (limit to top 10)
	importErrors, err := s.GetImportErrors(ctx)
	if err == nil && len(importErrors) > 0 {
		if len(importErrors) > 10 {
			importErrors = importErrors[:10]
		}
		status.ImportErrors = importErrors
	}

	return status, nil
}

// humanizeBytes converts bytes to a human-readable string.
func humanizeBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
