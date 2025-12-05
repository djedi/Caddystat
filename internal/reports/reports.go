// Package reports provides scheduled report generation and PDF export for Caddystat.
// It supports daily, weekly, and monthly reports with email delivery.
package reports

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/smtp"
	"sync"
	"time"

	"github.com/dustin/Caddystat/internal/storage"
)

// ReportType identifies the type of report.
type ReportType string

const (
	ReportTypeDaily   ReportType = "daily"
	ReportTypeWeekly  ReportType = "weekly"
	ReportTypeMonthly ReportType = "monthly"
	ReportTypeCustom  ReportType = "custom"
)

// ReportFormat identifies the output format.
type ReportFormat string

const (
	FormatPDF  ReportFormat = "pdf"
	FormatHTML ReportFormat = "html"
	FormatJSON ReportFormat = "json"
)

// Report represents a generated report.
type Report struct {
	ID          int64        `json:"id"`
	Type        ReportType   `json:"type"`
	Format      ReportFormat `json:"format"`
	Host        string       `json:"host,omitempty"` // Empty means all hosts
	PeriodStart time.Time    `json:"period_start"`
	PeriodEnd   time.Time    `json:"period_end"`
	GeneratedAt time.Time    `json:"generated_at"`
	SizeBytes   int64        `json:"size_bytes"`
	FileName    string       `json:"file_name"`
}

// ReportData contains all the statistics for a report.
type ReportData struct {
	// Period information
	PeriodStart time.Time `json:"period_start"`
	PeriodEnd   time.Time `json:"period_end"`
	Host        string    `json:"host,omitempty"`
	ReportType  string    `json:"report_type"`

	// Summary statistics
	Summary ReportSummary `json:"summary"`

	// Traffic by day/hour
	TimeSeries []storage.TimeSeriesStat `json:"time_series"`

	// Top pages
	TopPages []storage.PathStat `json:"top_pages"`

	// Top referrers
	TopReferrers []storage.ReferrerStat `json:"top_referrers"`

	// Browser breakdown
	Browsers []storage.BrowserStat `json:"browsers"`

	// OS breakdown
	OperatingSystems []storage.OSStat `json:"operating_systems"`

	// Geographic breakdown
	Countries []storage.GeoStat `json:"countries"`

	// Performance stats
	Performance *storage.PerformanceStats `json:"performance,omitempty"`

	// Error pages
	ErrorPages []storage.ErrorPageStat `json:"error_pages"`

	// Bot statistics
	Bots storage.BotStats `json:"bots"`
}

// ReportSummary contains high-level statistics.
type ReportSummary struct {
	TotalRequests    int64   `json:"total_requests"`
	UniqueVisitors   int64   `json:"unique_visitors"`
	TotalVisits      int64   `json:"total_visits"`
	PageViews        int64   `json:"page_views"`
	BandwidthBytes   int64   `json:"bandwidth_bytes"`
	BandwidthHuman   string  `json:"bandwidth_human"`
	AvgResponseTime  float64 `json:"avg_response_time_ms"`
	BounceRate       float64 `json:"bounce_rate"`
	Status2xx        int64   `json:"status_2xx"`
	Status3xx        int64   `json:"status_3xx"`
	Status4xx        int64   `json:"status_4xx"`
	Status5xx        int64   `json:"status_5xx"`
	ErrorRate        float64 `json:"error_rate"`
	AvgSessionLength float64 `json:"avg_session_length_seconds"`
}

// Schedule represents a scheduled report.
type Schedule struct {
	ID          int64        `json:"id"`
	Name        string       `json:"name"`
	Type        ReportType   `json:"type"`
	Format      ReportFormat `json:"format"`
	Host        string       `json:"host,omitempty"`
	Enabled     bool         `json:"enabled"`
	Recipients  []string     `json:"recipients"` // Email addresses
	LastRunAt   *time.Time   `json:"last_run_at,omitempty"`
	NextRunAt   time.Time    `json:"next_run_at"`
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
	SendDay     int          `json:"send_day,omitempty"`  // Day of week (0=Sun) for weekly, day of month for monthly
	SendHour    int          `json:"send_hour"`           // Hour of day (0-23)
	Timezone    string       `json:"timezone,omitempty"`  // Timezone name, defaults to UTC
}

// ScheduleInput is the input for creating or updating a schedule.
type ScheduleInput struct {
	Name       string       `json:"name"`
	Type       ReportType   `json:"type"`
	Format     ReportFormat `json:"format"`
	Host       string       `json:"host,omitempty"`
	Enabled    bool         `json:"enabled"`
	Recipients []string     `json:"recipients"`
	SendDay    int          `json:"send_day,omitempty"`
	SendHour   int          `json:"send_hour"`
	Timezone   string       `json:"timezone,omitempty"`
}

// EmailConfig holds email configuration for report delivery.
type EmailConfig struct {
	SMTPHost     string
	SMTPPort     int
	SMTPUsername string
	SMTPPassword string
	SMTPFrom     string
}

// Config holds the complete reports configuration.
type Config struct {
	Enabled          bool
	StoragePath      string // Directory to store generated reports
	Email            EmailConfig
	CheckInterval    time.Duration // How often to check for due reports
	RetentionDays    int           // How long to keep generated reports
}

// StatsProvider interface for fetching stats data.
type StatsProvider interface {
	Summary(ctx context.Context, since time.Duration, host string) (storage.Summary, error)
	TimeSeriesRange(ctx context.Context, dur time.Duration, host string) ([]storage.TimeSeriesStat, error)
	Geo(ctx context.Context, dur time.Duration, host string) ([]storage.GeoStat, error)
	Browsers(ctx context.Context, dur time.Duration, host string, limit int) ([]storage.BrowserStat, error)
	OperatingSystems(ctx context.Context, dur time.Duration, host string, limit int) ([]storage.OSStat, error)
	Referrers(ctx context.Context, dur time.Duration, host string, limit int) ([]storage.ReferrerStat, error)
	PerformanceStats(ctx context.Context, dur time.Duration, host string) (*storage.PerformanceStats, error)
	VisitorSessions(ctx context.Context, dur time.Duration, host string, limit, sessionTimeout int) (*storage.VisitorSessionSummary, error)
}

// ReportStore interface for persisting reports and schedules.
type ReportStore interface {
	CreateReport(ctx context.Context, report *Report, content []byte) error
	GetReport(ctx context.Context, id int64) (*Report, error)
	GetReportContent(ctx context.Context, id int64) ([]byte, error)
	ListReports(ctx context.Context, limit int, host string) ([]Report, error)
	DeleteReport(ctx context.Context, id int64) error
	DeleteOldReports(ctx context.Context, olderThan time.Time) (int64, error)

	CreateSchedule(ctx context.Context, input ScheduleInput) (*Schedule, error)
	GetSchedule(ctx context.Context, id int64) (*Schedule, error)
	UpdateSchedule(ctx context.Context, id int64, input ScheduleInput) (*Schedule, error)
	DeleteSchedule(ctx context.Context, id int64) error
	ListSchedules(ctx context.Context) ([]Schedule, error)
	GetDueSchedules(ctx context.Context, now time.Time) ([]Schedule, error)
	UpdateScheduleLastRun(ctx context.Context, id int64, lastRun, nextRun time.Time) error
}

// Manager handles report generation and scheduling.
type Manager struct {
	cfg      Config
	stats    StatsProvider
	store    ReportStore
	mu       sync.RWMutex
	stopCh   chan struct{}
	wg       sync.WaitGroup
}

// NewManager creates a new report manager.
func NewManager(cfg Config, stats StatsProvider, store ReportStore) *Manager {
	return &Manager{
		cfg:    cfg,
		stats:  stats,
		store:  store,
		stopCh: make(chan struct{}),
	}
}

// Start begins the scheduled report checking loop.
func (m *Manager) Start(ctx context.Context) {
	if !m.cfg.Enabled {
		slog.Info("reports disabled")
		return
	}

	interval := m.cfg.CheckInterval
	if interval == 0 {
		interval = 5 * time.Minute
	}

	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		slog.Info("reports scheduler started", "interval", interval)

		for {
			select {
			case <-ctx.Done():
				return
			case <-m.stopCh:
				return
			case <-ticker.C:
				m.checkAndRunSchedules(ctx)
			}
		}
	}()

	// Also run cleanup periodically
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-m.stopCh:
				return
			case <-ticker.C:
				m.cleanupOldReports(ctx)
			}
		}
	}()
}

// Stop stops the report manager.
func (m *Manager) Stop() {
	close(m.stopCh)
	m.wg.Wait()
	slog.Debug("reports scheduler stopped")
}

// GenerateReport generates a report for the given parameters.
func (m *Manager) GenerateReport(ctx context.Context, reportType ReportType, format ReportFormat, host string, periodStart, periodEnd time.Time) (*Report, []byte, error) {
	// Calculate duration from period
	dur := periodEnd.Sub(periodStart)
	if dur <= 0 {
		return nil, nil, fmt.Errorf("invalid period: end must be after start")
	}

	// Gather report data
	data, err := m.gatherReportData(ctx, reportType, host, periodStart, periodEnd)
	if err != nil {
		return nil, nil, fmt.Errorf("gather report data: %w", err)
	}

	// Generate content in requested format
	var content []byte
	var fileName string

	switch format {
	case FormatPDF:
		content, err = GeneratePDF(data)
		fileName = fmt.Sprintf("caddystat-report-%s-%s.pdf", reportType, periodStart.Format("2006-01-02"))
	case FormatHTML:
		content, err = GenerateHTML(data)
		fileName = fmt.Sprintf("caddystat-report-%s-%s.html", reportType, periodStart.Format("2006-01-02"))
	case FormatJSON:
		content, err = GenerateJSON(data)
		fileName = fmt.Sprintf("caddystat-report-%s-%s.json", reportType, periodStart.Format("2006-01-02"))
	default:
		return nil, nil, fmt.Errorf("unsupported format: %s", format)
	}

	if err != nil {
		return nil, nil, fmt.Errorf("generate %s: %w", format, err)
	}

	report := &Report{
		Type:        reportType,
		Format:      format,
		Host:        host,
		PeriodStart: periodStart,
		PeriodEnd:   periodEnd,
		GeneratedAt: time.Now(),
		SizeBytes:   int64(len(content)),
		FileName:    fileName,
	}

	return report, content, nil
}

// SaveReport saves a generated report to storage.
func (m *Manager) SaveReport(ctx context.Context, report *Report, content []byte) error {
	return m.store.CreateReport(ctx, report, content)
}

// GetReport retrieves a report by ID.
func (m *Manager) GetReport(ctx context.Context, id int64) (*Report, error) {
	return m.store.GetReport(ctx, id)
}

// GetReportContent retrieves the content of a report.
func (m *Manager) GetReportContent(ctx context.Context, id int64) ([]byte, error) {
	return m.store.GetReportContent(ctx, id)
}

// ListReports lists recent reports.
func (m *Manager) ListReports(ctx context.Context, limit int, host string) ([]Report, error) {
	return m.store.ListReports(ctx, limit, host)
}

// DeleteReport deletes a report by ID.
func (m *Manager) DeleteReport(ctx context.Context, id int64) error {
	return m.store.DeleteReport(ctx, id)
}

// CreateSchedule creates a new report schedule.
func (m *Manager) CreateSchedule(ctx context.Context, input ScheduleInput) (*Schedule, error) {
	return m.store.CreateSchedule(ctx, input)
}

// GetSchedule retrieves a schedule by ID.
func (m *Manager) GetSchedule(ctx context.Context, id int64) (*Schedule, error) {
	return m.store.GetSchedule(ctx, id)
}

// UpdateSchedule updates a schedule.
func (m *Manager) UpdateSchedule(ctx context.Context, id int64, input ScheduleInput) (*Schedule, error) {
	return m.store.UpdateSchedule(ctx, id, input)
}

// DeleteSchedule deletes a schedule.
func (m *Manager) DeleteSchedule(ctx context.Context, id int64) error {
	return m.store.DeleteSchedule(ctx, id)
}

// ListSchedules lists all schedules.
func (m *Manager) ListSchedules(ctx context.Context) ([]Schedule, error) {
	return m.store.ListSchedules(ctx)
}

// gatherReportData collects all statistics for a report.
func (m *Manager) gatherReportData(ctx context.Context, reportType ReportType, host string, periodStart, periodEnd time.Time) (*ReportData, error) {
	dur := periodEnd.Sub(periodStart)

	data := &ReportData{
		PeriodStart: periodStart,
		PeriodEnd:   periodEnd,
		Host:        host,
		ReportType:  string(reportType),
	}

	// Get summary
	summary, err := m.stats.Summary(ctx, dur, host)
	if err != nil {
		return nil, fmt.Errorf("get summary: %w", err)
	}

	data.Summary = ReportSummary{
		TotalRequests:   summary.TotalRequests,
		UniqueVisitors:  summary.UniqueVisitors,
		TotalVisits:     summary.Visits,
		PageViews:       summary.Traffic.Viewed.Pages,
		BandwidthBytes:  summary.BandwidthBytes,
		BandwidthHuman:  formatBytes(summary.BandwidthBytes),
		AvgResponseTime: summary.AvgResponseTime,
		Status2xx:       summary.Status2xx,
		Status3xx:       summary.Status3xx,
		Status4xx:       summary.Status4xx,
		Status5xx:       summary.Status5xx,
	}

	// Calculate error rate
	if summary.TotalRequests > 0 {
		data.Summary.ErrorRate = float64(summary.Status4xx+summary.Status5xx) / float64(summary.TotalRequests) * 100
	}

	// Get time series
	data.TimeSeries, _ = m.stats.TimeSeriesRange(ctx, dur, host)

	// Get top pages
	data.TopPages = summary.TopPaths

	// Get referrers
	data.TopReferrers, _ = m.stats.Referrers(ctx, dur, host, 20)

	// Get browsers
	data.Browsers, _ = m.stats.Browsers(ctx, dur, host, 10)

	// Get operating systems
	data.OperatingSystems, _ = m.stats.OperatingSystems(ctx, dur, host, 10)

	// Get geographic data
	data.Countries, _ = m.stats.Geo(ctx, dur, host)

	// Get performance stats
	data.Performance, _ = m.stats.PerformanceStats(ctx, dur, host)

	// Get error pages
	data.ErrorPages = summary.ErrorPages

	// Get bot stats
	data.Bots = summary.Bots

	// Get session data for bounce rate and avg session length
	sessions, err := m.stats.VisitorSessions(ctx, dur, host, 0, 0)
	if err == nil && sessions != nil {
		data.Summary.BounceRate = sessions.BounceRate
		data.Summary.AvgSessionLength = sessions.AvgDuration
	}

	return data, nil
}

func (m *Manager) checkAndRunSchedules(ctx context.Context) {
	schedules, err := m.store.GetDueSchedules(ctx, time.Now())
	if err != nil {
		slog.Error("failed to get due schedules", "error", err)
		return
	}

	for _, sched := range schedules {
		if err := m.runSchedule(ctx, sched); err != nil {
			slog.Error("failed to run scheduled report", "schedule_id", sched.ID, "name", sched.Name, "error", err)
		}
	}
}

func (m *Manager) runSchedule(ctx context.Context, sched Schedule) error {
	slog.Info("running scheduled report", "schedule_id", sched.ID, "name", sched.Name, "type", sched.Type)

	// Calculate period based on report type
	periodStart, periodEnd := calculatePeriod(sched.Type, time.Now())

	// Generate the report
	report, content, err := m.GenerateReport(ctx, sched.Type, sched.Format, sched.Host, periodStart, periodEnd)
	if err != nil {
		return fmt.Errorf("generate report: %w", err)
	}

	// Save the report
	if err := m.SaveReport(ctx, report, content); err != nil {
		return fmt.Errorf("save report: %w", err)
	}

	// Send email if configured
	if len(sched.Recipients) > 0 && m.cfg.Email.SMTPHost != "" {
		if err := m.sendReportEmail(sched, report, content); err != nil {
			slog.Error("failed to send report email", "schedule_id", sched.ID, "error", err)
			// Don't return error - the report was still generated successfully
		}
	}

	// Update schedule last run time
	nextRun := calculateNextRun(sched)
	if err := m.store.UpdateScheduleLastRun(ctx, sched.ID, time.Now(), nextRun); err != nil {
		slog.Error("failed to update schedule last run", "schedule_id", sched.ID, "error", err)
	}

	return nil
}

func (m *Manager) sendReportEmail(sched Schedule, report *Report, content []byte) error {
	if m.cfg.Email.SMTPHost == "" {
		return fmt.Errorf("SMTP not configured")
	}

	subject := fmt.Sprintf("[Caddystat] %s Report - %s",
		capitalizeFirst(string(sched.Type)),
		report.PeriodStart.Format("Jan 2, 2006"),
	)

	// Build email body
	var body bytes.Buffer
	body.WriteString(fmt.Sprintf("Caddystat %s Report\n", capitalizeFirst(string(sched.Type))))
	body.WriteString(fmt.Sprintf("Period: %s - %s\n\n",
		report.PeriodStart.Format("Jan 2, 2006 15:04"),
		report.PeriodEnd.Format("Jan 2, 2006 15:04"),
	))
	if sched.Host != "" {
		body.WriteString(fmt.Sprintf("Site: %s\n", sched.Host))
	}
	body.WriteString("\nPlease find the attached report.\n")

	// Build MIME message with attachment
	boundary := "caddystat-report-boundary"
	msg := buildMIMEMessage(m.cfg.Email.SMTPFrom, sched.Recipients, subject, body.String(), report.FileName, content, boundary)

	addr := fmt.Sprintf("%s:%d", m.cfg.Email.SMTPHost, m.cfg.Email.SMTPPort)
	var auth smtp.Auth
	if m.cfg.Email.SMTPUsername != "" {
		auth = smtp.PlainAuth("", m.cfg.Email.SMTPUsername, m.cfg.Email.SMTPPassword, m.cfg.Email.SMTPHost)
	}

	return smtp.SendMail(addr, auth, m.cfg.Email.SMTPFrom, sched.Recipients, []byte(msg))
}

func (m *Manager) cleanupOldReports(ctx context.Context) {
	if m.cfg.RetentionDays <= 0 {
		return
	}

	olderThan := time.Now().AddDate(0, 0, -m.cfg.RetentionDays)
	deleted, err := m.store.DeleteOldReports(ctx, olderThan)
	if err != nil {
		slog.Error("failed to cleanup old reports", "error", err)
		return
	}

	if deleted > 0 {
		slog.Info("cleaned up old reports", "deleted", deleted)
	}
}

// calculatePeriod returns the start and end time for a report period.
func calculatePeriod(reportType ReportType, now time.Time) (time.Time, time.Time) {
	switch reportType {
	case ReportTypeDaily:
		// Yesterday
		end := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		start := end.AddDate(0, 0, -1)
		return start, end
	case ReportTypeWeekly:
		// Last 7 days ending at start of today
		end := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		start := end.AddDate(0, 0, -7)
		return start, end
	case ReportTypeMonthly:
		// Previous month
		end := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		start := end.AddDate(0, -1, 0)
		return start, end
	default:
		// Default to last 24 hours
		end := now
		start := now.Add(-24 * time.Hour)
		return start, end
	}
}

// calculateNextRun calculates the next run time for a schedule.
func calculateNextRun(sched Schedule) time.Time {
	loc := time.UTC
	if sched.Timezone != "" {
		if l, err := time.LoadLocation(sched.Timezone); err == nil {
			loc = l
		}
	}

	now := time.Now().In(loc)

	switch sched.Type {
	case ReportTypeDaily:
		// Next day at SendHour
		next := time.Date(now.Year(), now.Month(), now.Day()+1, sched.SendHour, 0, 0, 0, loc)
		return next.UTC()
	case ReportTypeWeekly:
		// Next occurrence of SendDay at SendHour
		daysUntil := (sched.SendDay - int(now.Weekday()) + 7) % 7
		if daysUntil == 0 && now.Hour() >= sched.SendHour {
			daysUntil = 7
		}
		next := time.Date(now.Year(), now.Month(), now.Day()+daysUntil, sched.SendHour, 0, 0, 0, loc)
		return next.UTC()
	case ReportTypeMonthly:
		// SendDay of next month at SendHour
		nextMonth := time.Date(now.Year(), now.Month()+1, 1, sched.SendHour, 0, 0, 0, loc)
		day := sched.SendDay
		if day == 0 {
			day = 1
		}
		// Clamp to last day of month if needed
		lastDay := time.Date(nextMonth.Year(), nextMonth.Month()+1, 0, 0, 0, 0, 0, loc).Day()
		if day > lastDay {
			day = lastDay
		}
		next := time.Date(nextMonth.Year(), nextMonth.Month(), day, sched.SendHour, 0, 0, 0, loc)
		return next.UTC()
	default:
		return now.Add(24 * time.Hour)
	}
}

// buildMIMEMessage builds a multipart MIME email with an attachment.
func buildMIMEMessage(from string, to []string, subject, body, attachName string, attachData []byte, boundary string) string {
	var msg bytes.Buffer

	msg.WriteString(fmt.Sprintf("From: %s\r\n", from))
	msg.WriteString(fmt.Sprintf("To: %s\r\n", to[0]))
	msg.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	msg.WriteString("MIME-Version: 1.0\r\n")
	msg.WriteString(fmt.Sprintf("Content-Type: multipart/mixed; boundary=\"%s\"\r\n", boundary))
	msg.WriteString("\r\n")

	// Body part
	msg.WriteString(fmt.Sprintf("--%s\r\n", boundary))
	msg.WriteString("Content-Type: text/plain; charset=\"utf-8\"\r\n")
	msg.WriteString("\r\n")
	msg.WriteString(body)
	msg.WriteString("\r\n")

	// Attachment part
	msg.WriteString(fmt.Sprintf("--%s\r\n", boundary))
	contentType := "application/octet-stream"
	if len(attachName) > 4 && attachName[len(attachName)-4:] == ".pdf" {
		contentType = "application/pdf"
	} else if len(attachName) > 5 && attachName[len(attachName)-5:] == ".html" {
		contentType = "text/html"
	} else if len(attachName) > 5 && attachName[len(attachName)-5:] == ".json" {
		contentType = "application/json"
	}
	msg.WriteString(fmt.Sprintf("Content-Type: %s\r\n", contentType))
	msg.WriteString(fmt.Sprintf("Content-Disposition: attachment; filename=\"%s\"\r\n", attachName))
	msg.WriteString("Content-Transfer-Encoding: base64\r\n")
	msg.WriteString("\r\n")

	// Base64 encode attachment
	encoded := base64Encode(attachData)
	msg.WriteString(encoded)
	msg.WriteString("\r\n")

	msg.WriteString(fmt.Sprintf("--%s--\r\n", boundary))

	return msg.String()
}

// base64Encode encodes data as base64 with line breaks every 76 characters.
func base64Encode(data []byte) string {
	const lineLen = 76
	encoded := make([]byte, ((len(data)+2)/3)*4)
	encodeBase64(encoded, data)

	// Add line breaks
	var result bytes.Buffer
	for i := 0; i < len(encoded); i += lineLen {
		end := i + lineLen
		if end > len(encoded) {
			end = len(encoded)
		}
		result.Write(encoded[i:end])
		result.WriteString("\r\n")
	}
	return result.String()
}

// encodeBase64 is a simple base64 encoder.
func encodeBase64(dst, src []byte) {
	const encodeStd = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	di, si := 0, 0
	n := (len(src) / 3) * 3
	for si < n {
		val := uint(src[si+0])<<16 | uint(src[si+1])<<8 | uint(src[si+2])
		dst[di+0] = encodeStd[val>>18&0x3F]
		dst[di+1] = encodeStd[val>>12&0x3F]
		dst[di+2] = encodeStd[val>>6&0x3F]
		dst[di+3] = encodeStd[val&0x3F]
		si += 3
		di += 4
	}
	remain := len(src) - si
	if remain == 0 {
		return
	}
	val := uint(src[si+0]) << 16
	if remain == 2 {
		val |= uint(src[si+1]) << 8
	}
	dst[di+0] = encodeStd[val>>18&0x3F]
	dst[di+1] = encodeStd[val>>12&0x3F]
	if remain == 2 {
		dst[di+2] = encodeStd[val>>6&0x3F]
		dst[di+3] = '='
	} else {
		dst[di+2] = '='
		dst[di+3] = '='
	}
}

func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func capitalizeFirst(s string) string {
	if len(s) == 0 {
		return s
	}
	return string(s[0]-32) + s[1:]
}
