package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// ReportRecord represents a stored report in the database.
type ReportRecord struct {
	ID          int64     `json:"id"`
	Type        string    `json:"type"`
	Format      string    `json:"format"`
	Host        string    `json:"host,omitempty"`
	PeriodStart time.Time `json:"period_start"`
	PeriodEnd   time.Time `json:"period_end"`
	GeneratedAt time.Time `json:"generated_at"`
	SizeBytes   int64     `json:"size_bytes"`
	FileName    string    `json:"file_name"`
}

// ReportScheduleRecord represents a scheduled report in the database.
type ReportScheduleRecord struct {
	ID         int64      `json:"id"`
	Name       string     `json:"name"`
	Type       string     `json:"type"`
	Format     string     `json:"format"`
	Host       string     `json:"host,omitempty"`
	Enabled    bool       `json:"enabled"`
	Recipients []string   `json:"recipients"`
	LastRunAt  *time.Time `json:"last_run_at,omitempty"`
	NextRunAt  time.Time  `json:"next_run_at"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	SendDay    int        `json:"send_day,omitempty"`
	SendHour   int        `json:"send_hour"`
	Timezone   string     `json:"timezone,omitempty"`
}

// ReportScheduleInput is the input for creating or updating a schedule.
type ReportScheduleInput struct {
	Name       string   `json:"name"`
	Type       string   `json:"type"`
	Format     string   `json:"format"`
	Host       string   `json:"host,omitempty"`
	Enabled    bool     `json:"enabled"`
	Recipients []string `json:"recipients"`
	SendDay    int      `json:"send_day,omitempty"`
	SendHour   int      `json:"send_hour"`
	Timezone   string   `json:"timezone,omitempty"`
}

// CreateReportsTables creates the reports and report_schedules tables if they don't exist.
func (s *Storage) CreateReportsTables(ctx context.Context) error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS reports (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			type TEXT NOT NULL,
			format TEXT NOT NULL,
			host TEXT,
			period_start DATETIME NOT NULL,
			period_end DATETIME NOT NULL,
			generated_at DATETIME NOT NULL,
			size_bytes INTEGER NOT NULL,
			file_name TEXT NOT NULL,
			content BLOB
		)`,
		`CREATE INDEX IF NOT EXISTS idx_reports_generated_at ON reports(generated_at)`,
		`CREATE INDEX IF NOT EXISTS idx_reports_host ON reports(host)`,
		`CREATE TABLE IF NOT EXISTS report_schedules (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			type TEXT NOT NULL,
			format TEXT NOT NULL,
			host TEXT,
			enabled INTEGER NOT NULL DEFAULT 1,
			recipients TEXT NOT NULL,
			last_run_at DATETIME,
			next_run_at DATETIME NOT NULL,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL,
			send_day INTEGER,
			send_hour INTEGER NOT NULL DEFAULT 8,
			timezone TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_report_schedules_next_run ON report_schedules(next_run_at)`,
	}

	for _, q := range queries {
		if _, err := s.db.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("create reports tables: %w", err)
		}
	}

	return nil
}

// CreateReport stores a generated report.
func (s *Storage) CreateReport(ctx context.Context, report *ReportRecord, content []byte) error {
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO reports (type, format, host, period_start, period_end, generated_at, size_bytes, file_name, content)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, report.Type, report.Format, report.Host, report.PeriodStart, report.PeriodEnd,
		report.GeneratedAt, report.SizeBytes, report.FileName, content)
	if err != nil {
		return err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	report.ID = id
	return nil
}

// GetReport retrieves a report by ID (without content).
func (s *Storage) GetReport(ctx context.Context, id int64) (*ReportRecord, error) {
	var report ReportRecord
	err := s.db.QueryRowContext(ctx, `
		SELECT id, type, format, host, period_start, period_end, generated_at, size_bytes, file_name
		FROM reports WHERE id = ?
	`, id).Scan(&report.ID, &report.Type, &report.Format, &report.Host,
		&report.PeriodStart, &report.PeriodEnd, &report.GeneratedAt,
		&report.SizeBytes, &report.FileName)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &report, nil
}

// GetReportContent retrieves the content of a report.
func (s *Storage) GetReportContent(ctx context.Context, id int64) ([]byte, error) {
	var content []byte
	err := s.db.QueryRowContext(ctx, `SELECT content FROM reports WHERE id = ?`, id).Scan(&content)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return content, nil
}

// ListReports lists reports with optional host filter.
func (s *Storage) ListReports(ctx context.Context, limit int, host string) ([]ReportRecord, error) {
	var rows *sql.Rows
	var err error

	if host == "" {
		rows, err = s.db.QueryContext(ctx, `
			SELECT id, type, format, host, period_start, period_end, generated_at, size_bytes, file_name
			FROM reports ORDER BY generated_at DESC LIMIT ?
		`, limit)
	} else {
		rows, err = s.db.QueryContext(ctx, `
			SELECT id, type, format, host, period_start, period_end, generated_at, size_bytes, file_name
			FROM reports WHERE host = ? ORDER BY generated_at DESC LIMIT ?
		`, host, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reports []ReportRecord
	for rows.Next() {
		var r ReportRecord
		if err := rows.Scan(&r.ID, &r.Type, &r.Format, &r.Host, &r.PeriodStart, &r.PeriodEnd,
			&r.GeneratedAt, &r.SizeBytes, &r.FileName); err != nil {
			return nil, err
		}
		reports = append(reports, r)
	}
	return reports, rows.Err()
}

// DeleteReport deletes a report by ID.
func (s *Storage) DeleteReport(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM reports WHERE id = ?`, id)
	return err
}

// DeleteOldReports deletes reports older than the specified time.
func (s *Storage) DeleteOldReports(ctx context.Context, olderThan time.Time) (int64, error) {
	result, err := s.db.ExecContext(ctx, `DELETE FROM reports WHERE generated_at < ?`, olderThan)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// CreateReportSchedule creates a new report schedule.
func (s *Storage) CreateReportSchedule(ctx context.Context, input ReportScheduleInput) (*ReportScheduleRecord, error) {
	recipientsJSON, err := json.Marshal(input.Recipients)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	nextRun := calculateInitialNextRun(input.Type, input.SendDay, input.SendHour, input.Timezone)

	result, err := s.db.ExecContext(ctx, `
		INSERT INTO report_schedules (name, type, format, host, enabled, recipients, next_run_at, created_at, updated_at, send_day, send_hour, timezone)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, input.Name, input.Type, input.Format, input.Host, input.Enabled,
		string(recipientsJSON), nextRun, now, now, input.SendDay, input.SendHour, input.Timezone)
	if err != nil {
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	return s.GetReportSchedule(ctx, id)
}

// GetReportSchedule retrieves a schedule by ID.
func (s *Storage) GetReportSchedule(ctx context.Context, id int64) (*ReportScheduleRecord, error) {
	var sched ReportScheduleRecord
	var recipientsJSON string
	var lastRunAt sql.NullTime

	err := s.db.QueryRowContext(ctx, `
		SELECT id, name, type, format, host, enabled, recipients, last_run_at, next_run_at, created_at, updated_at, send_day, send_hour, timezone
		FROM report_schedules WHERE id = ?
	`, id).Scan(&sched.ID, &sched.Name, &sched.Type, &sched.Format, &sched.Host,
		&sched.Enabled, &recipientsJSON, &lastRunAt, &sched.NextRunAt,
		&sched.CreatedAt, &sched.UpdatedAt, &sched.SendDay, &sched.SendHour, &sched.Timezone)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if lastRunAt.Valid {
		sched.LastRunAt = &lastRunAt.Time
	}

	if err := json.Unmarshal([]byte(recipientsJSON), &sched.Recipients); err != nil {
		sched.Recipients = []string{}
	}

	return &sched, nil
}

// UpdateReportSchedule updates a schedule.
func (s *Storage) UpdateReportSchedule(ctx context.Context, id int64, input ReportScheduleInput) (*ReportScheduleRecord, error) {
	recipientsJSON, err := json.Marshal(input.Recipients)
	if err != nil {
		return nil, err
	}

	nextRun := calculateInitialNextRun(input.Type, input.SendDay, input.SendHour, input.Timezone)

	_, err = s.db.ExecContext(ctx, `
		UPDATE report_schedules
		SET name = ?, type = ?, format = ?, host = ?, enabled = ?, recipients = ?,
			next_run_at = ?, updated_at = ?, send_day = ?, send_hour = ?, timezone = ?
		WHERE id = ?
	`, input.Name, input.Type, input.Format, input.Host, input.Enabled,
		string(recipientsJSON), nextRun, time.Now(), input.SendDay, input.SendHour, input.Timezone, id)
	if err != nil {
		return nil, err
	}

	return s.GetReportSchedule(ctx, id)
}

// DeleteReportSchedule deletes a schedule.
func (s *Storage) DeleteReportSchedule(ctx context.Context, id int64) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM report_schedules WHERE id = ?`, id)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("schedule not found")
	}
	return nil
}

// ListReportSchedules lists all schedules.
func (s *Storage) ListReportSchedules(ctx context.Context) ([]ReportScheduleRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, type, format, host, enabled, recipients, last_run_at, next_run_at, created_at, updated_at, send_day, send_hour, timezone
		FROM report_schedules ORDER BY name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var schedules []ReportScheduleRecord
	for rows.Next() {
		var sched ReportScheduleRecord
		var recipientsJSON string
		var lastRunAt sql.NullTime

		if err := rows.Scan(&sched.ID, &sched.Name, &sched.Type, &sched.Format, &sched.Host,
			&sched.Enabled, &recipientsJSON, &lastRunAt, &sched.NextRunAt,
			&sched.CreatedAt, &sched.UpdatedAt, &sched.SendDay, &sched.SendHour, &sched.Timezone); err != nil {
			return nil, err
		}

		if lastRunAt.Valid {
			sched.LastRunAt = &lastRunAt.Time
		}

		if err := json.Unmarshal([]byte(recipientsJSON), &sched.Recipients); err != nil {
			sched.Recipients = []string{}
		}

		schedules = append(schedules, sched)
	}
	return schedules, rows.Err()
}

// GetDueReportSchedules returns schedules that are due to run.
func (s *Storage) GetDueReportSchedules(ctx context.Context, now time.Time) ([]ReportScheduleRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, type, format, host, enabled, recipients, last_run_at, next_run_at, created_at, updated_at, send_day, send_hour, timezone
		FROM report_schedules WHERE enabled = 1 AND next_run_at <= ?
	`, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var schedules []ReportScheduleRecord
	for rows.Next() {
		var sched ReportScheduleRecord
		var recipientsJSON string
		var lastRunAt sql.NullTime

		if err := rows.Scan(&sched.ID, &sched.Name, &sched.Type, &sched.Format, &sched.Host,
			&sched.Enabled, &recipientsJSON, &lastRunAt, &sched.NextRunAt,
			&sched.CreatedAt, &sched.UpdatedAt, &sched.SendDay, &sched.SendHour, &sched.Timezone); err != nil {
			return nil, err
		}

		if lastRunAt.Valid {
			sched.LastRunAt = &lastRunAt.Time
		}

		if err := json.Unmarshal([]byte(recipientsJSON), &sched.Recipients); err != nil {
			sched.Recipients = []string{}
		}

		schedules = append(schedules, sched)
	}
	return schedules, rows.Err()
}

// UpdateReportScheduleLastRun updates the last run time and calculates the next run time.
func (s *Storage) UpdateReportScheduleLastRun(ctx context.Context, id int64, lastRun, nextRun time.Time) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE report_schedules SET last_run_at = ?, next_run_at = ?, updated_at = ? WHERE id = ?
	`, lastRun, nextRun, time.Now(), id)
	return err
}

// calculateInitialNextRun calculates the initial next run time for a new schedule.
func calculateInitialNextRun(reportType string, sendDay, sendHour int, timezone string) time.Time {
	loc := time.UTC
	if timezone != "" {
		if l, err := time.LoadLocation(timezone); err == nil {
			loc = l
		}
	}

	now := time.Now().In(loc)

	switch reportType {
	case "daily":
		next := time.Date(now.Year(), now.Month(), now.Day(), sendHour, 0, 0, 0, loc)
		if next.Before(now) || next.Equal(now) {
			next = next.AddDate(0, 0, 1)
		}
		return next.UTC()
	case "weekly":
		daysUntil := (sendDay - int(now.Weekday()) + 7) % 7
		if daysUntil == 0 && (now.Hour() > sendHour || (now.Hour() == sendHour && now.Minute() > 0)) {
			daysUntil = 7
		}
		next := time.Date(now.Year(), now.Month(), now.Day()+daysUntil, sendHour, 0, 0, 0, loc)
		return next.UTC()
	case "monthly":
		day := sendDay
		if day == 0 {
			day = 1
		}
		next := time.Date(now.Year(), now.Month(), day, sendHour, 0, 0, 0, loc)
		if next.Before(now) || next.Equal(now) {
			next = time.Date(now.Year(), now.Month()+1, day, sendHour, 0, 0, 0, loc)
		}
		// Clamp to last day of month if needed
		lastDay := time.Date(next.Year(), next.Month()+1, 0, 0, 0, 0, 0, loc).Day()
		if day > lastDay {
			next = time.Date(next.Year(), next.Month(), lastDay, sendHour, 0, 0, 0, loc)
		}
		return next.UTC()
	default:
		return now.Add(24 * time.Hour)
	}
}
