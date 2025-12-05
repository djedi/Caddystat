package storage

import (
	"context"
	"database/sql"
	"time"
)

// GetImportProgress returns the import progress for a file, or nil if not found.
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

// SetImportProgress updates the import progress for a file.
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

// RecordImportError records an import error for a file.
// It increments the error count and stores the last error message.
func (s *Storage) RecordImportError(ctx context.Context, filePath string, err error) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	now := time.Now()
	errMsg := ""
	if err != nil {
		errMsg = err.Error()
	}

	_, dbErr := s.db.ExecContext(ctx, `
INSERT INTO import_errors (file_path, error_count, last_error, last_error_at, updated_at)
VALUES (?, 1, ?, ?, ?)
ON CONFLICT(file_path) DO UPDATE SET
	error_count = error_count + 1,
	last_error = excluded.last_error,
	last_error_at = excluded.last_error_at,
	updated_at = excluded.updated_at
`, filePath, errMsg, now, now)
	return dbErr
}

// GetImportErrors returns import error stats for all files with errors.
func (s *Storage) GetImportErrors(ctx context.Context) ([]ImportErrorStats, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT file_path, error_count, IFNULL(last_error, ''), last_error_at, updated_at
FROM import_errors
WHERE error_count > 0
ORDER BY error_count DESC, updated_at DESC
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []ImportErrorStats
	for rows.Next() {
		var stat ImportErrorStats
		var lastErrorAt, updatedAt sql.NullString
		if err := rows.Scan(&stat.FilePath, &stat.ErrorCount, &stat.LastError, &lastErrorAt, &updatedAt); err != nil {
			return nil, err
		}
		if lastErrorAt.Valid {
			stat.LastErrorAt, _ = time.Parse("2006-01-02 15:04:05.999999999-07:00", lastErrorAt.String)
			if stat.LastErrorAt.IsZero() {
				stat.LastErrorAt, _ = time.Parse(time.RFC3339Nano, lastErrorAt.String)
			}
		}
		if updatedAt.Valid {
			stat.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05.999999999-07:00", updatedAt.String)
			if stat.UpdatedAt.IsZero() {
				stat.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt.String)
			}
		}
		results = append(results, stat)
	}
	return results, rows.Err()
}

// GetImportErrorsTotal returns the total count of all import errors.
func (s *Storage) GetImportErrorsTotal(ctx context.Context) (int64, error) {
	var total int64
	row := s.db.QueryRowContext(ctx, `SELECT IFNULL(SUM(error_count), 0) FROM import_errors`)
	if err := row.Scan(&total); err != nil {
		return 0, err
	}
	return total, nil
}

// ClearImportErrors clears all error stats for a file (e.g., after successful import).
func (s *Storage) ClearImportErrors(ctx context.Context, filePath string) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_, err := s.db.ExecContext(ctx, `DELETE FROM import_errors WHERE file_path = ?`, filePath)
	return err
}
