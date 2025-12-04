package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRateLimiter_Disabled(t *testing.T) {
	rl := NewRateLimiter(0, time.Minute)
	if rl.enabled {
		t.Error("rate limiter should be disabled when limit is 0")
	}

	// Should always allow when disabled
	for i := 0; i < 100; i++ {
		if !rl.Allow("192.168.1.1") {
			t.Errorf("disabled rate limiter should always allow, failed on request %d", i+1)
		}
	}
}

func TestRateLimiter_EnforcesLimit(t *testing.T) {
	rl := NewRateLimiter(5, time.Minute)

	// First 5 requests should be allowed
	for i := 0; i < 5; i++ {
		if !rl.Allow("192.168.1.1") {
			t.Errorf("request %d should be allowed", i+1)
		}
	}

	// 6th request should be blocked
	if rl.Allow("192.168.1.1") {
		t.Error("6th request should be blocked")
	}
}

func TestRateLimiter_PerIP(t *testing.T) {
	rl := NewRateLimiter(2, time.Minute)

	// IP 1: use up limit
	if !rl.Allow("192.168.1.1") {
		t.Error("IP1 request 1 should be allowed")
	}
	if !rl.Allow("192.168.1.1") {
		t.Error("IP1 request 2 should be allowed")
	}
	if rl.Allow("192.168.1.1") {
		t.Error("IP1 request 3 should be blocked")
	}

	// IP 2: should have its own limit
	if !rl.Allow("192.168.1.2") {
		t.Error("IP2 request 1 should be allowed")
	}
	if !rl.Allow("192.168.1.2") {
		t.Error("IP2 request 2 should be allowed")
	}
}

func TestRateLimiter_WindowExpiry(t *testing.T) {
	// Use a very short window for testing
	rl := NewRateLimiter(2, 50*time.Millisecond)

	// Use up the limit
	if !rl.Allow("192.168.1.1") {
		t.Error("request 1 should be allowed")
	}
	if !rl.Allow("192.168.1.1") {
		t.Error("request 2 should be allowed")
	}
	if rl.Allow("192.168.1.1") {
		t.Error("request 3 should be blocked")
	}

	// Wait for window to expire
	time.Sleep(60 * time.Millisecond)

	// Should be allowed again
	if !rl.Allow("192.168.1.1") {
		t.Error("request after window expiry should be allowed")
	}
}

func TestExtractIP_RemoteAddr(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "192.168.1.1:12345"

	ip := extractIP(r)
	if ip != "192.168.1.1" {
		t.Errorf("expected 192.168.1.1, got %s", ip)
	}
}

func TestExtractIP_XForwardedFor(t *testing.T) {
	tests := []struct {
		name     string
		xff      string
		expected string
	}{
		{
			name:     "single IP",
			xff:      "10.0.0.1",
			expected: "10.0.0.1",
		},
		{
			name:     "multiple IPs",
			xff:      "10.0.0.1, 10.0.0.2, 10.0.0.3",
			expected: "10.0.0.1",
		},
		{
			name:     "with spaces",
			xff:      " 10.0.0.1 , 10.0.0.2",
			expected: "10.0.0.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/", nil)
			r.Header.Set("X-Forwarded-For", tt.xff)
			r.RemoteAddr = "192.168.1.1:12345"

			ip := extractIP(r)
			if ip != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, ip)
			}
		})
	}
}

func TestExtractIP_XRealIP(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("X-Real-IP", "10.0.0.1")
	r.RemoteAddr = "192.168.1.1:12345"

	ip := extractIP(r)
	if ip != "10.0.0.1" {
		t.Errorf("expected 10.0.0.1, got %s", ip)
	}
}

func TestExtractIP_XForwardedForPrecedence(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("X-Forwarded-For", "10.0.0.1")
	r.Header.Set("X-Real-IP", "10.0.0.2")
	r.RemoteAddr = "192.168.1.1:12345"

	ip := extractIP(r)
	if ip != "10.0.0.1" {
		t.Errorf("X-Forwarded-For should take precedence, expected 10.0.0.1, got %s", ip)
	}
}

func TestExtractIP_NoPort(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "192.168.1.1" // No port

	ip := extractIP(r)
	if ip != "192.168.1.1" {
		t.Errorf("expected 192.168.1.1, got %s", ip)
	}
}

func TestRateLimitIntegration(t *testing.T) {
	// Test that rate limiting works in the context of an HTTP server
	rl := NewRateLimiter(2, time.Minute)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := extractIP(r)
		if !rl.Allow(ip) {
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	// First two requests should succeed
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "192.168.1.1:12345"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("request %d: expected 200, got %d", i+1, rec.Code)
		}
	}

	// Third request should be rate limited
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429 Too Many Requests, got %d", rec.Code)
	}
}
