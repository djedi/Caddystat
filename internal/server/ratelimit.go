package server

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// RateLimiter implements a per-IP sliding window rate limiter.
type RateLimiter struct {
	mu       sync.Mutex
	visitors map[string]*visitor
	limit    int           // requests allowed per window
	window   time.Duration // time window
	enabled  bool
}

type visitor struct {
	requests []time.Time
}

// NewRateLimiter creates a rate limiter with the given limit per window.
// If limit is 0, rate limiting is disabled.
func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	rl := &RateLimiter{
		visitors: make(map[string]*visitor),
		limit:    limit,
		window:   window,
		enabled:  limit > 0,
	}
	if rl.enabled {
		go rl.cleanup()
	}
	return rl
}

// Allow checks if the given IP is allowed to make a request.
func (rl *RateLimiter) Allow(ip string) bool {
	if !rl.enabled {
		return true
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-rl.window)

	v, exists := rl.visitors[ip]
	if !exists {
		v = &visitor{}
		rl.visitors[ip] = v
	}

	// Remove requests outside the window
	valid := v.requests[:0]
	for _, t := range v.requests {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}
	v.requests = valid

	if len(v.requests) >= rl.limit {
		return false
	}

	v.requests = append(v.requests, now)
	return true
}

// cleanup removes stale visitors periodically.
func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(rl.window)
	defer ticker.Stop()
	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
		cutoff := now.Add(-rl.window)
		for ip, v := range rl.visitors {
			valid := v.requests[:0]
			for _, t := range v.requests {
				if t.After(cutoff) {
					valid = append(valid, t)
				}
			}
			if len(valid) == 0 {
				delete(rl.visitors, ip)
			} else {
				v.requests = valid
			}
		}
		rl.mu.Unlock()
	}
}

// extractIP extracts the client IP from the request.
// It respects X-Forwarded-For and X-Real-IP headers for proxied requests.
func extractIP(r *http.Request) string {
	// Try X-Forwarded-For first (may contain multiple IPs)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first IP (original client), trim whitespace
		if idx := strings.Index(xff, ","); idx != -1 {
			return strings.TrimSpace(xff[:idx])
		}
		return strings.TrimSpace(xff)
	}

	// Try X-Real-IP
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}

	// Fall back to RemoteAddr
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}
