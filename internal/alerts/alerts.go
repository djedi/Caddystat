// Package alerts provides an alerting framework for Caddystat.
// It supports monitoring for error rate spikes, traffic anomalies,
// and status code thresholds, with notifications via email and webhooks.
package alerts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/smtp"
	"sync"
	"time"
)

// AlertType identifies the type of alert condition.
type AlertType string

const (
	AlertTypeErrorRate    AlertType = "error_rate"    // 5xx spike
	AlertTypeTrafficSpike AlertType = "traffic_spike" // Sudden increase
	AlertTypeTrafficDrop  AlertType = "traffic_drop"  // Sudden decrease
	AlertTypeStatusCode   AlertType = "status_code"   // Specific status threshold
)

// AlertSeverity indicates the severity level.
type AlertSeverity string

const (
	SeverityInfo     AlertSeverity = "info"
	SeverityWarning  AlertSeverity = "warning"
	SeverityCritical AlertSeverity = "critical"
)

// Alert represents a triggered alert.
type Alert struct {
	ID          string        `json:"id"`
	Type        AlertType     `json:"type"`
	Severity    AlertSeverity `json:"severity"`
	Host        string        `json:"host,omitempty"`
	Message     string        `json:"message"`
	Value       float64       `json:"value"`
	Threshold   float64       `json:"threshold"`
	TriggeredAt time.Time     `json:"triggered_at"`
	Details     any           `json:"details,omitempty"`
}

// Rule defines an alerting rule.
type Rule struct {
	Name        string        `json:"name"`
	Type        AlertType     `json:"type"`
	Enabled     bool          `json:"enabled"`
	Threshold   float64       `json:"threshold"`
	Duration    time.Duration `json:"duration"` // Evaluation window
	Cooldown    time.Duration `json:"cooldown"` // Min time between alerts
	Severity    AlertSeverity `json:"severity"`
	Host        string        `json:"host,omitempty"` // Optional host filter
	StatusCodes []int         `json:"status_codes,omitempty"`
}

// ChannelType identifies the notification channel type.
type ChannelType string

const (
	ChannelTypeEmail   ChannelType = "email"
	ChannelTypeWebhook ChannelType = "webhook"
)

// Channel represents a notification channel configuration.
type Channel struct {
	Type    ChannelType `json:"type"`
	Enabled bool        `json:"enabled"`
	// Email settings
	SMTPHost     string   `json:"smtp_host,omitempty"`
	SMTPPort     int      `json:"smtp_port,omitempty"`
	SMTPUsername string   `json:"smtp_username,omitempty"`
	SMTPPassword string   `json:"smtp_password,omitempty"`
	SMTPFrom     string   `json:"smtp_from,omitempty"`
	EmailTo      []string `json:"email_to,omitempty"`
	// Webhook settings
	WebhookURL     string            `json:"webhook_url,omitempty"`
	WebhookMethod  string            `json:"webhook_method,omitempty"` // POST (default) or GET
	WebhookHeaders map[string]string `json:"webhook_headers,omitempty"`
}

// Config holds the complete alerting configuration.
type Config struct {
	Enabled          bool          `json:"enabled"`
	EvaluateInterval time.Duration `json:"evaluate_interval"` // How often to check rules
	Rules            []Rule        `json:"rules"`
	Channels         []Channel     `json:"channels"`
}

// AlertStats holds statistics needed for alert evaluation.
type AlertStats struct {
	TotalRequests    int64
	Status5xx        int64
	Status4xx        int64
	StatusCounts     map[int]int64 // Per status code counts
	AvgRequestsPerHr float64
	PrevRequests     int64 // Requests in previous period (for comparison)
}

// StatsProvider interface for fetching stats data.
// Implemented by storage.Storage.
type StatsProvider interface {
	GetAlertStats(ctx context.Context, duration time.Duration, host string) (*AlertStats, error)
}

// Manager handles alert evaluation and notification.
type Manager struct {
	cfg      Config
	stats    StatsProvider
	mu       sync.RWMutex
	lastFire map[string]time.Time // rule name -> last fired time
	history  []Alert              // Recent alerts (for API)
	stopCh   chan struct{}
	wg       sync.WaitGroup
}

// NewManager creates a new alert manager.
func NewManager(cfg Config, stats StatsProvider) *Manager {
	return &Manager{
		cfg:      cfg,
		stats:    stats,
		lastFire: make(map[string]time.Time),
		history:  make([]Alert, 0),
		stopCh:   make(chan struct{}),
	}
}

// Start begins the alert evaluation loop.
func (m *Manager) Start(ctx context.Context) {
	if !m.cfg.Enabled {
		slog.Info("alerting disabled")
		return
	}

	interval := m.cfg.EvaluateInterval
	if interval == 0 {
		interval = time.Minute
	}

	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		slog.Info("alerting started", "interval", interval, "rules", len(m.cfg.Rules))

		for {
			select {
			case <-ctx.Done():
				return
			case <-m.stopCh:
				return
			case <-ticker.C:
				m.evaluate(ctx)
			}
		}
	}()
}

// Stop stops the alert manager.
func (m *Manager) Stop() {
	close(m.stopCh)
	m.wg.Wait()
	slog.Debug("alerting stopped")
}

// GetHistory returns recent alerts.
func (m *Manager) GetHistory(limit int) []Alert {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if limit <= 0 || limit > len(m.history) {
		limit = len(m.history)
	}
	result := make([]Alert, limit)
	// Return most recent first
	for i := 0; i < limit; i++ {
		result[i] = m.history[len(m.history)-1-i]
	}
	return result
}

// GetConfig returns the current alerting configuration.
func (m *Manager) GetConfig() Config {
	return m.cfg
}

func (m *Manager) evaluate(ctx context.Context) {
	for _, rule := range m.cfg.Rules {
		if !rule.Enabled {
			continue
		}

		// Check cooldown
		m.mu.RLock()
		lastFire, ok := m.lastFire[rule.Name]
		m.mu.RUnlock()
		if ok && time.Since(lastFire) < rule.Cooldown {
			continue
		}

		duration := rule.Duration
		if duration == 0 {
			duration = 5 * time.Minute
		}

		stats, err := m.stats.GetAlertStats(ctx, duration, rule.Host)
		if err != nil {
			slog.Warn("failed to get alert stats", "rule", rule.Name, "error", err)
			continue
		}

		alert := m.checkRule(rule, stats)
		if alert != nil {
			m.fire(*alert)
		}
	}
}

func (m *Manager) checkRule(rule Rule, stats *AlertStats) *Alert {
	switch rule.Type {
	case AlertTypeErrorRate:
		return m.checkErrorRate(rule, stats)
	case AlertTypeTrafficSpike:
		return m.checkTrafficSpike(rule, stats)
	case AlertTypeTrafficDrop:
		return m.checkTrafficDrop(rule, stats)
	case AlertTypeStatusCode:
		return m.checkStatusCode(rule, stats)
	default:
		return nil
	}
}

func (m *Manager) checkErrorRate(rule Rule, stats *AlertStats) *Alert {
	if stats.TotalRequests == 0 {
		return nil
	}

	errorRate := float64(stats.Status5xx) / float64(stats.TotalRequests) * 100

	if errorRate >= rule.Threshold {
		return &Alert{
			ID:          fmt.Sprintf("%s-%d", rule.Name, time.Now().UnixNano()),
			Type:        AlertTypeErrorRate,
			Severity:    rule.Severity,
			Host:        rule.Host,
			Message:     fmt.Sprintf("Error rate %.2f%% exceeds threshold %.2f%%", errorRate, rule.Threshold),
			Value:       errorRate,
			Threshold:   rule.Threshold,
			TriggeredAt: time.Now(),
			Details: map[string]any{
				"total_requests": stats.TotalRequests,
				"5xx_count":      stats.Status5xx,
				"duration":       rule.Duration.String(),
			},
		}
	}
	return nil
}

func (m *Manager) checkTrafficSpike(rule Rule, stats *AlertStats) *Alert {
	if stats.PrevRequests == 0 || stats.TotalRequests == 0 {
		return nil
	}

	// Calculate percentage increase
	increase := float64(stats.TotalRequests-stats.PrevRequests) / float64(stats.PrevRequests) * 100

	if increase >= rule.Threshold {
		return &Alert{
			ID:          fmt.Sprintf("%s-%d", rule.Name, time.Now().UnixNano()),
			Type:        AlertTypeTrafficSpike,
			Severity:    rule.Severity,
			Host:        rule.Host,
			Message:     fmt.Sprintf("Traffic increased by %.1f%% (threshold: %.1f%%)", increase, rule.Threshold),
			Value:       increase,
			Threshold:   rule.Threshold,
			TriggeredAt: time.Now(),
			Details: map[string]any{
				"current_requests":  stats.TotalRequests,
				"previous_requests": stats.PrevRequests,
				"duration":          rule.Duration.String(),
			},
		}
	}
	return nil
}

func (m *Manager) checkTrafficDrop(rule Rule, stats *AlertStats) *Alert {
	if stats.PrevRequests == 0 {
		return nil
	}

	// Calculate percentage decrease
	decrease := float64(stats.PrevRequests-stats.TotalRequests) / float64(stats.PrevRequests) * 100

	if decrease >= rule.Threshold {
		return &Alert{
			ID:          fmt.Sprintf("%s-%d", rule.Name, time.Now().UnixNano()),
			Type:        AlertTypeTrafficDrop,
			Severity:    rule.Severity,
			Host:        rule.Host,
			Message:     fmt.Sprintf("Traffic dropped by %.1f%% (threshold: %.1f%%)", decrease, rule.Threshold),
			Value:       decrease,
			Threshold:   rule.Threshold,
			TriggeredAt: time.Now(),
			Details: map[string]any{
				"current_requests":  stats.TotalRequests,
				"previous_requests": stats.PrevRequests,
				"duration":          rule.Duration.String(),
			},
		}
	}
	return nil
}

func (m *Manager) checkStatusCode(rule Rule, stats *AlertStats) *Alert {
	var totalMatched int64
	for _, code := range rule.StatusCodes {
		totalMatched += stats.StatusCounts[code]
	}

	if float64(totalMatched) >= rule.Threshold {
		return &Alert{
			ID:          fmt.Sprintf("%s-%d", rule.Name, time.Now().UnixNano()),
			Type:        AlertTypeStatusCode,
			Severity:    rule.Severity,
			Host:        rule.Host,
			Message:     fmt.Sprintf("Status code count %d exceeds threshold %.0f", totalMatched, rule.Threshold),
			Value:       float64(totalMatched),
			Threshold:   rule.Threshold,
			TriggeredAt: time.Now(),
			Details: map[string]any{
				"status_codes": rule.StatusCodes,
				"counts":       stats.StatusCounts,
				"duration":     rule.Duration.String(),
			},
		}
	}
	return nil
}

func (m *Manager) fire(alert Alert) {
	m.mu.Lock()
	// Extract rule name from alert ID (format: "rulename-timestamp")
	ruleName := alert.ID
	if idx := len(alert.ID) - 20; idx > 0 && idx < len(alert.ID) {
		ruleName = alert.ID[:idx]
	}
	m.lastFire[ruleName] = alert.TriggeredAt
	m.history = append(m.history, alert)
	// Keep only last 100 alerts
	if len(m.history) > 100 {
		m.history = m.history[len(m.history)-100:]
	}
	m.mu.Unlock()

	slog.Warn("alert triggered",
		"type", alert.Type,
		"severity", alert.Severity,
		"message", alert.Message,
		"value", alert.Value,
		"threshold", alert.Threshold,
	)

	// Send to all enabled channels
	for _, ch := range m.cfg.Channels {
		if !ch.Enabled {
			continue
		}
		go m.sendToChannel(ch, alert)
	}
}

func (m *Manager) sendToChannel(ch Channel, alert Alert) {
	switch ch.Type {
	case ChannelTypeEmail:
		if err := m.sendEmail(ch, alert); err != nil {
			slog.Error("failed to send email alert", "error", err)
		}
	case ChannelTypeWebhook:
		if err := m.sendWebhook(ch, alert); err != nil {
			slog.Error("failed to send webhook alert", "error", err)
		}
	}
}

func (m *Manager) sendEmail(ch Channel, alert Alert) error {
	if ch.SMTPHost == "" || len(ch.EmailTo) == 0 {
		return fmt.Errorf("email channel not properly configured")
	}

	subject := fmt.Sprintf("[Caddystat Alert] %s - %s", alert.Severity, alert.Type)
	body := fmt.Sprintf(`Alert: %s
Severity: %s
Time: %s
Message: %s
Value: %.2f
Threshold: %.2f
`,
		alert.Type,
		alert.Severity,
		alert.TriggeredAt.Format(time.RFC3339),
		alert.Message,
		alert.Value,
		alert.Threshold,
	)

	if alert.Host != "" {
		body += fmt.Sprintf("Host: %s\n", alert.Host)
	}

	if alert.Details != nil {
		detailsJSON, _ := json.MarshalIndent(alert.Details, "", "  ")
		body += fmt.Sprintf("\nDetails:\n%s\n", string(detailsJSON))
	}

	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\n\r\n%s",
		ch.SMTPFrom,
		ch.EmailTo[0], // Primary recipient in header
		subject,
		body,
	)

	addr := fmt.Sprintf("%s:%d", ch.SMTPHost, ch.SMTPPort)
	var auth smtp.Auth
	if ch.SMTPUsername != "" {
		auth = smtp.PlainAuth("", ch.SMTPUsername, ch.SMTPPassword, ch.SMTPHost)
	}

	return smtp.SendMail(addr, auth, ch.SMTPFrom, ch.EmailTo, []byte(msg))
}

func (m *Manager) sendWebhook(ch Channel, alert Alert) error {
	if ch.WebhookURL == "" {
		return fmt.Errorf("webhook URL not configured")
	}

	payload, err := json.Marshal(alert)
	if err != nil {
		return fmt.Errorf("failed to marshal alert: %w", err)
	}

	method := ch.WebhookMethod
	if method == "" {
		method = http.MethodPost
	}

	req, err := http.NewRequest(method, ch.WebhookURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Caddystat-Alerting/1.0")
	for k, v := range ch.WebhookHeaders {
		req.Header.Set(k, v)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("webhook request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}

	slog.Debug("webhook alert sent", "url", ch.WebhookURL, "status", resp.StatusCode)
	return nil
}
