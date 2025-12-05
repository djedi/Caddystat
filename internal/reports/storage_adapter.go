package reports

import (
	"context"
	"time"

	"github.com/dustin/Caddystat/internal/storage"
)

// StorageAdapter adapts *storage.Storage to the ReportStore interface.
type StorageAdapter struct {
	store *storage.Storage
}

// NewStorageAdapter creates a new storage adapter.
func NewStorageAdapter(store *storage.Storage) *StorageAdapter {
	return &StorageAdapter{store: store}
}

// CreateReport stores a generated report.
func (a *StorageAdapter) CreateReport(ctx context.Context, report *Report, content []byte) error {
	rec := &storage.ReportRecord{
		Type:        string(report.Type),
		Format:      string(report.Format),
		Host:        report.Host,
		PeriodStart: report.PeriodStart,
		PeriodEnd:   report.PeriodEnd,
		GeneratedAt: report.GeneratedAt,
		SizeBytes:   report.SizeBytes,
		FileName:    report.FileName,
	}
	if err := a.store.CreateReport(ctx, rec, content); err != nil {
		return err
	}
	report.ID = rec.ID
	return nil
}

// GetReport retrieves a report by ID.
func (a *StorageAdapter) GetReport(ctx context.Context, id int64) (*Report, error) {
	rec, err := a.store.GetReport(ctx, id)
	if err != nil || rec == nil {
		return nil, err
	}
	return &Report{
		ID:          rec.ID,
		Type:        ReportType(rec.Type),
		Format:      ReportFormat(rec.Format),
		Host:        rec.Host,
		PeriodStart: rec.PeriodStart,
		PeriodEnd:   rec.PeriodEnd,
		GeneratedAt: rec.GeneratedAt,
		SizeBytes:   rec.SizeBytes,
		FileName:    rec.FileName,
	}, nil
}

// GetReportContent retrieves the content of a report.
func (a *StorageAdapter) GetReportContent(ctx context.Context, id int64) ([]byte, error) {
	return a.store.GetReportContent(ctx, id)
}

// ListReports lists recent reports.
func (a *StorageAdapter) ListReports(ctx context.Context, limit int, host string) ([]Report, error) {
	recs, err := a.store.ListReports(ctx, limit, host)
	if err != nil {
		return nil, err
	}
	reports := make([]Report, len(recs))
	for i, rec := range recs {
		reports[i] = Report{
			ID:          rec.ID,
			Type:        ReportType(rec.Type),
			Format:      ReportFormat(rec.Format),
			Host:        rec.Host,
			PeriodStart: rec.PeriodStart,
			PeriodEnd:   rec.PeriodEnd,
			GeneratedAt: rec.GeneratedAt,
			SizeBytes:   rec.SizeBytes,
			FileName:    rec.FileName,
		}
	}
	return reports, nil
}

// DeleteReport deletes a report by ID.
func (a *StorageAdapter) DeleteReport(ctx context.Context, id int64) error {
	return a.store.DeleteReport(ctx, id)
}

// DeleteOldReports deletes reports older than the specified time.
func (a *StorageAdapter) DeleteOldReports(ctx context.Context, olderThan time.Time) (int64, error) {
	return a.store.DeleteOldReports(ctx, olderThan)
}

// CreateSchedule creates a new report schedule.
func (a *StorageAdapter) CreateSchedule(ctx context.Context, input ScheduleInput) (*Schedule, error) {
	rec, err := a.store.CreateReportSchedule(ctx, storage.ReportScheduleInput{
		Name:       input.Name,
		Type:       string(input.Type),
		Format:     string(input.Format),
		Host:       input.Host,
		Enabled:    input.Enabled,
		Recipients: input.Recipients,
		SendDay:    input.SendDay,
		SendHour:   input.SendHour,
		Timezone:   input.Timezone,
	})
	if err != nil || rec == nil {
		return nil, err
	}
	return convertScheduleRecord(rec), nil
}

// GetSchedule retrieves a schedule by ID.
func (a *StorageAdapter) GetSchedule(ctx context.Context, id int64) (*Schedule, error) {
	rec, err := a.store.GetReportSchedule(ctx, id)
	if err != nil || rec == nil {
		return nil, err
	}
	return convertScheduleRecord(rec), nil
}

// UpdateSchedule updates a schedule.
func (a *StorageAdapter) UpdateSchedule(ctx context.Context, id int64, input ScheduleInput) (*Schedule, error) {
	rec, err := a.store.UpdateReportSchedule(ctx, id, storage.ReportScheduleInput{
		Name:       input.Name,
		Type:       string(input.Type),
		Format:     string(input.Format),
		Host:       input.Host,
		Enabled:    input.Enabled,
		Recipients: input.Recipients,
		SendDay:    input.SendDay,
		SendHour:   input.SendHour,
		Timezone:   input.Timezone,
	})
	if err != nil || rec == nil {
		return nil, err
	}
	return convertScheduleRecord(rec), nil
}

// DeleteSchedule deletes a schedule.
func (a *StorageAdapter) DeleteSchedule(ctx context.Context, id int64) error {
	return a.store.DeleteReportSchedule(ctx, id)
}

// ListSchedules lists all schedules.
func (a *StorageAdapter) ListSchedules(ctx context.Context) ([]Schedule, error) {
	recs, err := a.store.ListReportSchedules(ctx)
	if err != nil {
		return nil, err
	}
	schedules := make([]Schedule, len(recs))
	for i, rec := range recs {
		schedules[i] = *convertScheduleRecord(&rec)
	}
	return schedules, nil
}

// GetDueSchedules returns schedules that are due to run.
func (a *StorageAdapter) GetDueSchedules(ctx context.Context, now time.Time) ([]Schedule, error) {
	recs, err := a.store.GetDueReportSchedules(ctx, now)
	if err != nil {
		return nil, err
	}
	schedules := make([]Schedule, len(recs))
	for i, rec := range recs {
		schedules[i] = *convertScheduleRecord(&rec)
	}
	return schedules, nil
}

// UpdateScheduleLastRun updates the last run time.
func (a *StorageAdapter) UpdateScheduleLastRun(ctx context.Context, id int64, lastRun, nextRun time.Time) error {
	return a.store.UpdateReportScheduleLastRun(ctx, id, lastRun, nextRun)
}

func convertScheduleRecord(rec *storage.ReportScheduleRecord) *Schedule {
	return &Schedule{
		ID:         rec.ID,
		Name:       rec.Name,
		Type:       ReportType(rec.Type),
		Format:     ReportFormat(rec.Format),
		Host:       rec.Host,
		Enabled:    rec.Enabled,
		Recipients: rec.Recipients,
		LastRunAt:  rec.LastRunAt,
		NextRunAt:  rec.NextRunAt,
		CreatedAt:  rec.CreatedAt,
		UpdatedAt:  rec.UpdatedAt,
		SendDay:    rec.SendDay,
		SendHour:   rec.SendHour,
		Timezone:   rec.Timezone,
	}
}
