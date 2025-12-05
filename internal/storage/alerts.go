package storage

import (
	"context"
	"time"

	"github.com/dustin/Caddystat/internal/alerts"
)

// AlertStatsAdapter wraps Storage to satisfy the alerts.StatsProvider interface.
type AlertStatsAdapter struct {
	store *Storage
}

// NewAlertStatsAdapter creates a new adapter for the alerts package.
func NewAlertStatsAdapter(s *Storage) *AlertStatsAdapter {
	return &AlertStatsAdapter{store: s}
}

// GetAlertStats returns statistics needed for alert evaluation.
func (a *AlertStatsAdapter) GetAlertStats(ctx context.Context, duration time.Duration, host string) (*alerts.AlertStats, error) {
	stats, err := a.store.GetAlertStats(ctx, duration, host)
	if err != nil {
		return nil, err
	}

	return &alerts.AlertStats{
		TotalRequests:    stats.TotalRequests,
		Status5xx:        stats.Status5xx,
		Status4xx:        stats.Status4xx,
		StatusCounts:     stats.StatusCounts,
		AvgRequestsPerHr: stats.AvgRequestsPerHr,
		PrevRequests:     stats.PrevRequests,
	}, nil
}
