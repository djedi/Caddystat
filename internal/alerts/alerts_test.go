package alerts

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// mockStatsProvider implements StatsProvider for testing.
type mockStatsProvider struct {
	mu    sync.Mutex
	stats *AlertStats
}

func (m *mockStatsProvider) GetAlertStats(ctx context.Context, duration time.Duration, host string) (*AlertStats, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.stats != nil {
		return m.stats, nil
	}
	return &AlertStats{
		StatusCounts: make(map[int]int64),
	}, nil
}

func (m *mockStatsProvider) setStats(stats *AlertStats) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stats = stats
}

func TestManager_CheckErrorRate(t *testing.T) {
	mock := &mockStatsProvider{}

	cfg := Config{
		Enabled:          true,
		EvaluateInterval: time.Minute,
		Rules: []Rule{
			{
				Name:      "test_error_rate",
				Type:      AlertTypeErrorRate,
				Enabled:   true,
				Threshold: 5.0, // 5% error rate
				Duration:  5 * time.Minute,
				Cooldown:  time.Minute,
				Severity:  SeverityCritical,
			},
		},
		Channels: []Channel{},
	}

	m := NewManager(cfg, mock)

	// Test case 1: No errors - should not trigger
	mock.setStats(&AlertStats{
		TotalRequests: 100,
		Status5xx:     0,
		StatusCounts:  map[int]int64{200: 100},
	})

	alert := m.checkRule(cfg.Rules[0], mock.stats)
	if alert != nil {
		t.Errorf("expected no alert for 0%% error rate, got %v", alert)
	}

	// Test case 2: Below threshold - should not trigger
	mock.setStats(&AlertStats{
		TotalRequests: 100,
		Status5xx:     4, // 4%
		StatusCounts:  map[int]int64{200: 96, 500: 4},
	})

	alert = m.checkRule(cfg.Rules[0], mock.stats)
	if alert != nil {
		t.Errorf("expected no alert for 4%% error rate, got %v", alert)
	}

	// Test case 3: At threshold - should trigger
	mock.setStats(&AlertStats{
		TotalRequests: 100,
		Status5xx:     5, // 5%
		StatusCounts:  map[int]int64{200: 95, 500: 5},
	})

	alert = m.checkRule(cfg.Rules[0], mock.stats)
	if alert == nil {
		t.Error("expected alert for 5% error rate, got none")
	} else {
		if alert.Type != AlertTypeErrorRate {
			t.Errorf("expected type %s, got %s", AlertTypeErrorRate, alert.Type)
		}
		if alert.Severity != SeverityCritical {
			t.Errorf("expected severity %s, got %s", SeverityCritical, alert.Severity)
		}
	}

	// Test case 4: Above threshold - should trigger
	mock.setStats(&AlertStats{
		TotalRequests: 100,
		Status5xx:     10, // 10%
		StatusCounts:  map[int]int64{200: 90, 500: 10},
	})

	alert = m.checkRule(cfg.Rules[0], mock.stats)
	if alert == nil {
		t.Error("expected alert for 10% error rate, got none")
	}
}

func TestManager_CheckTrafficSpike(t *testing.T) {
	mock := &mockStatsProvider{}

	cfg := Config{
		Enabled: true,
		Rules: []Rule{
			{
				Name:      "test_traffic_spike",
				Type:      AlertTypeTrafficSpike,
				Enabled:   true,
				Threshold: 50.0, // 50% increase
				Duration:  5 * time.Minute,
				Cooldown:  time.Minute,
				Severity:  SeverityWarning,
			},
		},
	}

	m := NewManager(cfg, mock)

	// Test case 1: No change - should not trigger
	mock.setStats(&AlertStats{
		TotalRequests: 100,
		PrevRequests:  100,
		StatusCounts:  make(map[int]int64),
	})

	alert := m.checkRule(cfg.Rules[0], mock.stats)
	if alert != nil {
		t.Errorf("expected no alert for 0%% increase, got %v", alert)
	}

	// Test case 2: Below threshold - should not trigger
	mock.setStats(&AlertStats{
		TotalRequests: 140,
		PrevRequests:  100, // 40% increase
		StatusCounts:  make(map[int]int64),
	})

	alert = m.checkRule(cfg.Rules[0], mock.stats)
	if alert != nil {
		t.Errorf("expected no alert for 40%% increase, got %v", alert)
	}

	// Test case 3: At threshold - should trigger
	mock.setStats(&AlertStats{
		TotalRequests: 150,
		PrevRequests:  100, // 50% increase
		StatusCounts:  make(map[int]int64),
	})

	alert = m.checkRule(cfg.Rules[0], mock.stats)
	if alert == nil {
		t.Error("expected alert for 50% traffic spike, got none")
	} else {
		if alert.Type != AlertTypeTrafficSpike {
			t.Errorf("expected type %s, got %s", AlertTypeTrafficSpike, alert.Type)
		}
	}
}

func TestManager_CheckTrafficDrop(t *testing.T) {
	mock := &mockStatsProvider{}

	cfg := Config{
		Enabled: true,
		Rules: []Rule{
			{
				Name:      "test_traffic_drop",
				Type:      AlertTypeTrafficDrop,
				Enabled:   true,
				Threshold: 50.0, // 50% decrease
				Duration:  5 * time.Minute,
				Cooldown:  time.Minute,
				Severity:  SeverityWarning,
			},
		},
	}

	m := NewManager(cfg, mock)

	// Test case 1: No change - should not trigger
	mock.setStats(&AlertStats{
		TotalRequests: 100,
		PrevRequests:  100,
		StatusCounts:  make(map[int]int64),
	})

	alert := m.checkRule(cfg.Rules[0], mock.stats)
	if alert != nil {
		t.Errorf("expected no alert for 0%% decrease, got %v", alert)
	}

	// Test case 2: Below threshold - should not trigger
	mock.setStats(&AlertStats{
		TotalRequests: 60,
		PrevRequests:  100, // 40% decrease
		StatusCounts:  make(map[int]int64),
	})

	alert = m.checkRule(cfg.Rules[0], mock.stats)
	if alert != nil {
		t.Errorf("expected no alert for 40%% decrease, got %v", alert)
	}

	// Test case 3: At threshold - should trigger
	mock.setStats(&AlertStats{
		TotalRequests: 50,
		PrevRequests:  100, // 50% decrease
		StatusCounts:  make(map[int]int64),
	})

	alert = m.checkRule(cfg.Rules[0], mock.stats)
	if alert == nil {
		t.Error("expected alert for 50% traffic drop, got none")
	} else {
		if alert.Type != AlertTypeTrafficDrop {
			t.Errorf("expected type %s, got %s", AlertTypeTrafficDrop, alert.Type)
		}
	}
}

func TestManager_CheckStatusCode(t *testing.T) {
	mock := &mockStatsProvider{}

	cfg := Config{
		Enabled: true,
		Rules: []Rule{
			{
				Name:        "test_404",
				Type:        AlertTypeStatusCode,
				Enabled:     true,
				Threshold:   10.0, // 10 occurrences
				StatusCodes: []int{404},
				Duration:    5 * time.Minute,
				Cooldown:    time.Minute,
				Severity:    SeverityWarning,
			},
		},
	}

	m := NewManager(cfg, mock)

	// Test case 1: Below threshold - should not trigger
	mock.setStats(&AlertStats{
		TotalRequests: 100,
		StatusCounts:  map[int]int64{200: 95, 404: 5},
	})

	alert := m.checkRule(cfg.Rules[0], mock.stats)
	if alert != nil {
		t.Errorf("expected no alert for 5 404s, got %v", alert)
	}

	// Test case 2: At threshold - should trigger
	mock.setStats(&AlertStats{
		TotalRequests: 100,
		StatusCounts:  map[int]int64{200: 90, 404: 10},
	})

	alert = m.checkRule(cfg.Rules[0], mock.stats)
	if alert == nil {
		t.Error("expected alert for 10 404s, got none")
	} else {
		if alert.Type != AlertTypeStatusCode {
			t.Errorf("expected type %s, got %s", AlertTypeStatusCode, alert.Type)
		}
	}

	// Test case 3: Multiple status codes
	cfg.Rules[0].StatusCodes = []int{404, 410}
	mock.setStats(&AlertStats{
		TotalRequests: 100,
		StatusCounts:  map[int]int64{200: 90, 404: 5, 410: 6}, // 11 total
	})

	alert = m.checkRule(cfg.Rules[0], mock.stats)
	if alert == nil {
		t.Error("expected alert for combined 404+410 count, got none")
	}
}

func TestManager_Cooldown(t *testing.T) {
	mock := &mockStatsProvider{}

	cfg := Config{
		Enabled:          true,
		EvaluateInterval: 50 * time.Millisecond,
		Rules: []Rule{
			{
				Name:      "test_cooldown",
				Type:      AlertTypeErrorRate,
				Enabled:   true,
				Threshold: 5.0,
				Duration:  5 * time.Minute,
				Cooldown:  200 * time.Millisecond,
				Severity:  SeverityCritical,
			},
		},
		Channels: []Channel{},
	}

	m := NewManager(cfg, mock)

	// Set stats that would trigger alert
	mock.setStats(&AlertStats{
		TotalRequests: 100,
		Status5xx:     10,
		StatusCounts:  map[int]int64{200: 90, 500: 10},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	m.Start(ctx)

	// Wait for first evaluation
	time.Sleep(100 * time.Millisecond)

	history := m.GetHistory(10)
	if len(history) == 0 {
		t.Error("expected at least one alert")
	}

	initialCount := len(history)

	// Wait but not past cooldown
	time.Sleep(100 * time.Millisecond)

	history = m.GetHistory(10)
	if len(history) != initialCount {
		t.Errorf("expected %d alerts during cooldown, got %d", initialCount, len(history))
	}

	// Wait past cooldown
	time.Sleep(200 * time.Millisecond)

	history = m.GetHistory(10)
	if len(history) <= initialCount {
		t.Error("expected more alerts after cooldown expired")
	}

	m.Stop()
}

func TestManager_WebhookNotification(t *testing.T) {
	var receivedAlert Alert
	webhookCalled := make(chan bool, 1)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", r.Header.Get("Content-Type"))
		}
		if r.Header.Get("X-Custom-Header") != "test-value" {
			t.Errorf("expected custom header, got %s", r.Header.Get("X-Custom-Header"))
		}
		webhookCalled <- true
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	mock := &mockStatsProvider{}

	cfg := Config{
		Enabled:          true,
		EvaluateInterval: 50 * time.Millisecond,
		Rules: []Rule{
			{
				Name:      "test_webhook",
				Type:      AlertTypeErrorRate,
				Enabled:   true,
				Threshold: 5.0,
				Duration:  5 * time.Minute,
				Cooldown:  time.Hour, // Long cooldown to only fire once
				Severity:  SeverityCritical,
			},
		},
		Channels: []Channel{
			{
				Type:           ChannelTypeWebhook,
				Enabled:        true,
				WebhookURL:     server.URL,
				WebhookMethod:  "POST",
				WebhookHeaders: map[string]string{"X-Custom-Header": "test-value"},
			},
		},
	}

	m := NewManager(cfg, mock)

	// Set stats that would trigger alert
	mock.setStats(&AlertStats{
		TotalRequests: 100,
		Status5xx:     10,
		StatusCounts:  map[int]int64{200: 90, 500: 10},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	m.Start(ctx)

	select {
	case <-webhookCalled:
		// Success
	case <-time.After(time.Second):
		t.Error("webhook was not called within timeout")
	}

	m.Stop()
	_ = receivedAlert
}

func TestManager_GetHistory(t *testing.T) {
	mock := &mockStatsProvider{}

	cfg := Config{
		Enabled: true,
		Rules:   []Rule{},
	}

	m := NewManager(cfg, mock)

	// Add some alerts manually
	for i := 0; i < 5; i++ {
		m.fire(Alert{
			ID:          "test",
			Type:        AlertTypeErrorRate,
			Severity:    SeverityWarning,
			Message:     "test alert",
			Value:       float64(i),
			Threshold:   5.0,
			TriggeredAt: time.Now(),
		})
	}

	// Get all
	history := m.GetHistory(0)
	if len(history) != 5 {
		t.Errorf("expected 5 alerts, got %d", len(history))
	}

	// Check order (most recent first)
	if history[0].Value != 4 {
		t.Errorf("expected most recent alert first, got value %f", history[0].Value)
	}

	// Get limited
	history = m.GetHistory(3)
	if len(history) != 3 {
		t.Errorf("expected 3 alerts, got %d", len(history))
	}
}

func TestManager_DisabledRule(t *testing.T) {
	mock := &mockStatsProvider{}

	cfg := Config{
		Enabled:          true,
		EvaluateInterval: 50 * time.Millisecond,
		Rules: []Rule{
			{
				Name:      "disabled_rule",
				Type:      AlertTypeErrorRate,
				Enabled:   false, // Disabled
				Threshold: 5.0,
				Duration:  5 * time.Minute,
				Cooldown:  time.Minute,
				Severity:  SeverityCritical,
			},
		},
		Channels: []Channel{},
	}

	m := NewManager(cfg, mock)

	// Set stats that would trigger alert
	mock.setStats(&AlertStats{
		TotalRequests: 100,
		Status5xx:     10,
		StatusCounts:  map[int]int64{200: 90, 500: 10},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	m.Start(ctx)

	time.Sleep(100 * time.Millisecond)

	history := m.GetHistory(10)
	if len(history) != 0 {
		t.Errorf("expected no alerts for disabled rule, got %d", len(history))
	}

	m.Stop()
}

func TestManager_ZeroRequests(t *testing.T) {
	mock := &mockStatsProvider{}

	cfg := Config{
		Enabled: true,
		Rules: []Rule{
			{
				Name:      "test_zero",
				Type:      AlertTypeErrorRate,
				Enabled:   true,
				Threshold: 5.0,
				Duration:  5 * time.Minute,
				Cooldown:  time.Minute,
				Severity:  SeverityCritical,
			},
		},
	}

	m := NewManager(cfg, mock)

	// Zero requests should not trigger
	mock.setStats(&AlertStats{
		TotalRequests: 0,
		Status5xx:     0,
		StatusCounts:  make(map[int]int64),
	})

	alert := m.checkRule(cfg.Rules[0], mock.stats)
	if alert != nil {
		t.Error("expected no alert for zero requests")
	}
}
