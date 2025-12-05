package server

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/dustin/Caddystat/internal/config"
	"github.com/dustin/Caddystat/internal/metrics"
	"github.com/dustin/Caddystat/internal/sse"
	"github.com/dustin/Caddystat/internal/storage"
	"github.com/dustin/Caddystat/internal/version"
)

type Server struct {
	store       *storage.Storage
	hub         *sse.Hub
	mux         *http.ServeMux
	cfg         config.Config
	rateLimiter *RateLimiter
	metrics     *metrics.Metrics
}

func New(store *storage.Storage, hub *sse.Hub, cfg config.Config, m *metrics.Metrics) *Server {
	s := &Server{
		store:       store,
		hub:         hub,
		mux:         http.NewServeMux(),
		cfg:         cfg,
		rateLimiter: NewRateLimiter(cfg.RateLimitPerMinute, time.Minute),
		metrics:     m,
	}
	s.routes()
	return s
}

func (s *Server) routes() {
	// Public endpoints (no auth required)
	s.mux.HandleFunc("/health", s.handleHealth)
	s.mux.HandleFunc("/robots.txt", s.handleRobotsTxt)
	s.mux.Handle("/metrics", promhttp.Handler())

	// Auth endpoints (always accessible, POST endpoints require CSRF)
	s.mux.HandleFunc("/api/auth/login", s.requireCSRF(s.handleLogin))
	s.mux.HandleFunc("/api/auth/logout", s.requireCSRF(s.handleLogout))
	s.mux.HandleFunc("/api/auth/check", s.handleAuthCheck)

	// Protected API endpoints with site permission checks
	// These endpoints accept a "host" query parameter that must be authorized
	s.mux.HandleFunc("/api/stats/summary", s.requireAuth(s.requireSitePermission(s.handleSummary)))
	s.mux.HandleFunc("/api/stats/monthly", s.requireAuth(s.requireSitePermission(s.handleMonthly)))
	s.mux.HandleFunc("/api/stats/daily", s.requireAuth(s.requireSitePermission(s.handleDaily)))
	s.mux.HandleFunc("/api/stats/requests", s.requireAuth(s.requireSitePermission(s.handleRequests)))
	s.mux.HandleFunc("/api/stats/geo", s.requireAuth(s.requireSitePermission(s.handleGeo)))
	s.mux.HandleFunc("/api/stats/hosts", s.requireAuth(s.requireSitePermission(s.handleVisitors)))
	s.mux.HandleFunc("/api/stats/browsers", s.requireAuth(s.requireSitePermission(s.handleBrowsers)))
	s.mux.HandleFunc("/api/stats/os", s.requireAuth(s.requireSitePermission(s.handleOS)))
	s.mux.HandleFunc("/api/stats/robots", s.requireAuth(s.requireSitePermission(s.handleRobots)))
	s.mux.HandleFunc("/api/stats/referrers", s.requireAuth(s.requireSitePermission(s.handleReferrers)))
	s.mux.HandleFunc("/api/stats/recent", s.requireAuth(s.requireSitePermission(s.handleRecentRequests)))
	s.mux.HandleFunc("/api/stats/status", s.requireAuth(s.handleStatus)) // Status doesn't filter by host
	s.mux.HandleFunc("/api/stats/performance", s.requireAuth(s.requireSitePermission(s.handlePerformance)))
	s.mux.HandleFunc("/api/stats/bandwidth", s.requireAuth(s.requireSitePermission(s.handleBandwidth)))
	s.mux.HandleFunc("/api/stats/sessions", s.requireAuth(s.requireSitePermission(s.handleSessions)))
	s.mux.HandleFunc("/api/sse", s.requireAuth(s.requireSitePermission(s.handleSSE)))

	// Export endpoints with site permission checks
	s.mux.HandleFunc("/api/export/csv", s.requireAuth(s.requireSitePermission(s.handleExportCSV)))
	s.mux.HandleFunc("/api/export/json", s.requireAuth(s.requireSitePermission(s.handleExportJSON)))
	s.mux.HandleFunc("/api/export/backup", s.requireAuth(s.handleExportBackup)) // Backup is system-wide, admin only

	// Site management endpoints
	s.mux.HandleFunc("/api/sites", s.requireAuth(s.requireCSRF(s.handleSites)))
	s.mux.HandleFunc("/api/sites/", s.requireAuth(s.requireCSRF(s.handleSiteByID)))

	site := http.Dir(filepath.Join(".", "web", "_site"))
	s.mux.Handle("/", http.FileServer(site))
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	// Prevent search engine indexing
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")

	// Set security headers (CSP, X-Frame-Options, etc.)
	setSecurityHeaders(w)

	// Ensure CSRF cookie is set for all requests
	if _, err := ensureCSRFCookie(w, r); err != nil {
		slog.Warn("failed to set CSRF cookie", "error", err)
	}

	// Apply rate limiting
	if s.rateLimiter.enabled {
		ip := extractIP(r)
		if !s.rateLimiter.Allow(ip) {
			slog.Debug("rate limit exceeded", "ip", ip, "path", r.URL.Path)
			if s.metrics != nil {
				s.metrics.RecordHTTPRequest(r.Method, r.URL.Path, "429", time.Since(start).Seconds())
			}
			writeErrorWithCode(w, http.StatusTooManyRequests, "rate limit exceeded", "RATE_LIMITED")
			return
		}
	}

	// Apply body size limit
	if s.cfg.MaxRequestBodyBytes > 0 && r.ContentLength > s.cfg.MaxRequestBodyBytes {
		if s.metrics != nil {
			s.metrics.RecordHTTPRequest(r.Method, r.URL.Path, "413", time.Since(start).Seconds())
		}
		writeErrorWithCode(w, http.StatusRequestEntityTooLarge, "request body too large", "REQUEST_TOO_LARGE")
		return
	}
	if s.cfg.MaxRequestBodyBytes > 0 && r.Body != nil {
		r.Body = http.MaxBytesReader(w, r.Body, s.cfg.MaxRequestBodyBytes)
	}

	// Wrap response writer to capture status code
	wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
	s.mux.ServeHTTP(wrapped, r)

	// Record metrics (skip /metrics endpoint to avoid self-referential metrics)
	if s.metrics != nil && r.URL.Path != "/metrics" {
		s.metrics.RecordHTTPRequest(r.Method, normalizePath(r.URL.Path), strconv.Itoa(wrapped.statusCode), time.Since(start).Seconds())
	}
}

// responseWriter wraps http.ResponseWriter to capture the status code.
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// Flush implements http.Flusher, required for SSE streaming.
func (rw *responseWriter) Flush() {
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap returns the underlying ResponseWriter for interface assertions.
func (rw *responseWriter) Unwrap() http.ResponseWriter {
	return rw.ResponseWriter
}

// normalizePath reduces cardinality by grouping dynamic path segments.
func normalizePath(path string) string {
	// Normalize common API paths to reduce cardinality
	if len(path) > 0 && path[0] == '/' {
		// Keep API paths as-is since they're already grouped
		if len(path) >= 4 && path[:4] == "/api" {
			return path
		}
		// Static paths
		switch path {
		case "/", "/health", "/metrics", "/robots.txt":
			return path
		}
	}
	// Group all other paths as static assets
	return "/static"
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	status := "ok"
	dbStatus := "connected"
	httpStatus := http.StatusOK

	if err := s.store.Ping(ctx); err != nil {
		status = "error"
		dbStatus = "disconnected"
		httpStatus = http.StatusServiceUnavailable
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpStatus)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":  status,
		"db":      dbStatus,
		"version": version.Version,
	})
}

func (s *Server) handleRobotsTxt(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	_, _ = w.Write([]byte("User-agent: *\nDisallow: /\n"))
}

// Authentication methods

const sessionCookieName = "caddystat_session"
const sessionDuration = 24 * time.Hour

func (s *Server) generateSessionToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

func (s *Server) createSession(ctx context.Context) (string, error) {
	token, err := s.generateSessionToken()
	if err != nil {
		return "", err
	}
	expiresAt := time.Now().Add(sessionDuration)
	if err := s.store.CreateSession(ctx, token, expiresAt); err != nil {
		return "", err
	}
	return token, nil
}

func (s *Server) validateSession(ctx context.Context, token string) bool {
	sess, err := s.store.GetSession(ctx, token)
	if err != nil {
		slog.Warn("failed to get session", "error", err)
		return false
	}
	if sess == nil {
		return false
	}
	if time.Now().After(sess.ExpiresAt) {
		// Clean up expired session
		if err := s.store.DeleteSession(ctx, token); err != nil {
			slog.Warn("failed to delete expired session", "error", err)
		}
		return false
	}
	return true
}

func (s *Server) deleteSession(ctx context.Context, token string) {
	if err := s.store.DeleteSession(ctx, token); err != nil {
		slog.Warn("failed to delete session", "error", err)
	}
}

func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// If auth is not enabled, pass through
		if !s.cfg.AuthEnabled() {
			next(w, r)
			return
		}

		// Check for session cookie
		cookie, err := r.Cookie(sessionCookieName)
		if err != nil || !s.validateSession(r.Context(), cookie.Value) {
			writeErrorWithCode(w, http.StatusUnauthorized, "unauthorized", "UNAUTHORIZED")
			return
		}
		next(w, r)
	}
}

// requireSitePermission wraps a handler to check if the session has permission to access
// the requested site (determined by the "host" query parameter).
// If no host is specified, it allows access (aggregate view).
func (s *Server) requireSitePermission(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// If auth is not enabled, pass through
		if !s.cfg.AuthEnabled() {
			next(w, r)
			return
		}

		host := r.URL.Query().Get("host")
		// If no specific host is requested, allow access (aggregate view)
		// The underlying handlers may need to filter results based on permissions
		if host == "" {
			next(w, r)
			return
		}

		// Get session cookie
		cookie, err := r.Cookie(sessionCookieName)
		if err != nil {
			writeErrorWithCode(w, http.StatusUnauthorized, "unauthorized", "UNAUTHORIZED")
			return
		}

		// Check site permission
		hasPermission, err := s.store.HasSitePermission(r.Context(), cookie.Value, host)
		if err != nil {
			slog.Warn("failed to check site permission", "error", err)
			writeInternalError(w, err, "check site permission")
			return
		}

		if !hasPermission {
			writeErrorWithCode(w, http.StatusForbidden, "access denied for this site", "SITE_ACCESS_DENIED")
			return
		}

		next(w, r)
	}
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErrorWithCode(w, http.StatusMethodNotAllowed, "method not allowed", "METHOD_NOT_ALLOWED")
		return
	}

	// If auth is not enabled, return success
	if !s.cfg.AuthEnabled() {
		writeJSON(w, map[string]any{"authenticated": true, "auth_required": false})
		return
	}

	var req struct {
		Username     string   `json:"username"`
		Password     string   `json:"password"`
		AllowedSites []string `json:"allowed_sites,omitempty"` // Optional: restrict session to specific sites
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorWithCode(w, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}

	// Use constant-time comparison to prevent timing attacks
	usernameMatch := subtle.ConstantTimeCompare([]byte(req.Username), []byte(s.cfg.AuthUsername)) == 1
	passwordMatch := subtle.ConstantTimeCompare([]byte(req.Password), []byte(s.cfg.AuthPassword)) == 1

	if !usernameMatch || !passwordMatch {
		writeErrorWithCode(w, http.StatusUnauthorized, "invalid credentials", "INVALID_CREDENTIALS")
		return
	}

	token, err := s.createSession(r.Context())
	if err != nil {
		writeInternalError(w, err, "create session")
		return
	}

	// Set site permissions for the session
	if err := s.store.SetSessionPermissions(r.Context(), token, req.AllowedSites); err != nil {
		slog.Warn("failed to set session permissions", "error", err)
		// Continue anyway - session will have all-site access by default
	}

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(sessionDuration.Seconds()),
	})

	writeJSON(w, map[string]any{"authenticated": true})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErrorWithCode(w, http.StatusMethodNotAllowed, "method not allowed", "METHOD_NOT_ALLOWED")
		return
	}

	cookie, err := r.Cookie(sessionCookieName)
	if err == nil {
		// Delete permissions first, then session
		if err := s.store.DeleteSessionPermissions(r.Context(), cookie.Value); err != nil {
			slog.Warn("failed to delete session permissions", "error", err)
		}
		s.deleteSession(r.Context(), cookie.Value)
	}

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
	})

	writeJSON(w, map[string]any{"authenticated": false})
}

func (s *Server) handleAuthCheck(w http.ResponseWriter, r *http.Request) {
	// If auth is not enabled, return that auth is not required
	if !s.cfg.AuthEnabled() {
		writeJSON(w, map[string]any{"authenticated": true, "auth_required": false})
		return
	}

	// Check for valid session
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || !s.validateSession(r.Context(), cookie.Value) {
		writeJSON(w, map[string]any{"authenticated": false, "auth_required": true})
		return
	}

	// Get session permissions
	perms, err := s.store.GetSessionPermissions(r.Context(), cookie.Value)
	if err != nil {
		slog.Warn("failed to get session permissions", "error", err)
		// Return basic auth info without permissions on error
		writeJSON(w, map[string]any{"authenticated": true, "auth_required": true})
		return
	}

	writeJSON(w, map[string]any{
		"authenticated": true,
		"auth_required": true,
		"permissions": map[string]any{
			"all_sites":     perms.AllSites,
			"allowed_hosts": perms.AllowedHosts,
		},
	})
}

func (s *Server) handleSummary(w http.ResponseWriter, r *http.Request) {
	dur := parseRange(r.URL.Query().Get("range"), 24*time.Hour)
	host := r.URL.Query().Get("host")
	stats, err := s.store.Summary(r.Context(), dur, host)
	if err != nil {
		writeInternalError(w, err, "get summary")
		return
	}
	writeJSON(w, stats)
}

func (s *Server) handleRequests(w http.ResponseWriter, r *http.Request) {
	dur := parseRange(r.URL.Query().Get("range"), 24*time.Hour)
	host := r.URL.Query().Get("host")
	stats, err := s.store.TimeSeriesRange(r.Context(), dur, host)
	if err != nil {
		writeInternalError(w, err, "get requests")
		return
	}
	writeJSON(w, stats)
}

func (s *Server) handleGeo(w http.ResponseWriter, r *http.Request) {
	dur := parseRange(r.URL.Query().Get("range"), 24*time.Hour)
	host := r.URL.Query().Get("host")
	stats, err := s.store.Geo(r.Context(), dur, host)
	if err != nil {
		writeInternalError(w, err, "get geo")
		return
	}
	writeJSON(w, stats)
}

func (s *Server) handleVisitors(w http.ResponseWriter, r *http.Request) {
	dur := parseRange(r.URL.Query().Get("range"), 24*time.Hour)
	host := r.URL.Query().Get("host")
	limit := 20
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 {
			limit = v
		}
	}
	stats, err := s.store.Visitors(r.Context(), dur, host, limit)
	if err != nil {
		writeInternalError(w, err, "get visitors")
		return
	}
	writeJSON(w, stats)
}

func (s *Server) handleBrowsers(w http.ResponseWriter, r *http.Request) {
	dur := parseRange(r.URL.Query().Get("range"), 24*time.Hour)
	host := r.URL.Query().Get("host")
	limit := 10
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 {
			limit = v
		}
	}
	stats, err := s.store.Browsers(r.Context(), dur, host, limit)
	if err != nil {
		writeInternalError(w, err, "get browsers")
		return
	}
	writeJSON(w, stats)
}

func (s *Server) handleOS(w http.ResponseWriter, r *http.Request) {
	dur := parseRange(r.URL.Query().Get("range"), 24*time.Hour)
	host := r.URL.Query().Get("host")
	limit := 10
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 {
			limit = v
		}
	}
	stats, err := s.store.OperatingSystems(r.Context(), dur, host, limit)
	if err != nil {
		writeInternalError(w, err, "get operating systems")
		return
	}
	writeJSON(w, stats)
}

func (s *Server) handleRobots(w http.ResponseWriter, r *http.Request) {
	dur := parseRange(r.URL.Query().Get("range"), 24*time.Hour)
	host := r.URL.Query().Get("host")
	limit := 20
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 {
			limit = v
		}
	}
	stats, err := s.store.Robots(r.Context(), dur, host, limit)
	if err != nil {
		writeInternalError(w, err, "get robots")
		return
	}
	writeJSON(w, stats)
}

func (s *Server) handleReferrers(w http.ResponseWriter, r *http.Request) {
	dur := parseRange(r.URL.Query().Get("range"), 24*time.Hour)
	host := r.URL.Query().Get("host")
	limit := 20
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 {
			limit = v
		}
	}
	stats, err := s.store.Referrers(r.Context(), dur, host, limit)
	if err != nil {
		writeInternalError(w, err, "get referrers")
		return
	}
	writeJSON(w, stats)
}

func (s *Server) handleRecentRequests(w http.ResponseWriter, r *http.Request) {
	host := r.URL.Query().Get("host")
	limit := 20
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 && v <= 100 {
			limit = v
		}
	}
	stats, err := s.store.RecentRequests(r.Context(), limit, host)
	if err != nil {
		writeInternalError(w, err, "get recent requests")
		return
	}
	writeJSON(w, stats)
}

func (s *Server) handleMonthly(w http.ResponseWriter, r *http.Request) {
	host := r.URL.Query().Get("host")
	months := 12
	if m := r.URL.Query().Get("months"); m != "" {
		if v, err := strconv.Atoi(m); err == nil && v > 0 {
			months = v
		}
	}
	stats, err := s.store.MonthlyHistory(r.Context(), months, host)
	if err != nil {
		writeInternalError(w, err, "get monthly history")
		return
	}
	writeJSON(w, stats)
}

func (s *Server) handleDaily(w http.ResponseWriter, r *http.Request) {
	host := r.URL.Query().Get("host")
	stats, err := s.store.DailyHistory(r.Context(), host)
	if err != nil {
		writeInternalError(w, err, "get daily history")
		return
	}
	writeJSON(w, stats)
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	status, err := s.store.GetSystemStatus(r.Context())
	if err != nil {
		writeInternalError(w, err, "get system status")
		return
	}
	writeJSON(w, status)
}

func (s *Server) handlePerformance(w http.ResponseWriter, r *http.Request) {
	dur := parseRange(r.URL.Query().Get("range"), 24*time.Hour)
	host := r.URL.Query().Get("host")
	stats, err := s.store.PerformanceStats(r.Context(), dur, host)
	if err != nil {
		writeInternalError(w, err, "get performance stats")
		return
	}
	writeJSON(w, stats)
}

func (s *Server) handleBandwidth(w http.ResponseWriter, r *http.Request) {
	dur := parseRange(r.URL.Query().Get("range"), 24*time.Hour)
	host := r.URL.Query().Get("host")
	limit := 10
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 {
			limit = v
		}
	}
	stats, err := s.store.BandwidthStats(r.Context(), dur, host, limit)
	if err != nil {
		writeInternalError(w, err, "get bandwidth stats")
		return
	}
	writeJSON(w, stats)
}

func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	dur := parseRange(r.URL.Query().Get("range"), 24*time.Hour)
	host := r.URL.Query().Get("host")
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 {
			limit = v
		}
	}
	// Session timeout in seconds (default 30 minutes)
	sessionTimeout := 0 // 0 means use default
	if t := r.URL.Query().Get("timeout"); t != "" {
		if v, err := strconv.Atoi(t); err == nil && v > 0 {
			sessionTimeout = v
		}
	}
	sessions, err := s.store.VisitorSessions(r.Context(), dur, host, limit, sessionTimeout)
	if err != nil {
		writeInternalError(w, err, "get visitor sessions")
		return
	}
	writeJSON(w, sessions)
}

func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeErrorWithCode(w, http.StatusInternalServerError, "streaming unsupported", "STREAMING_UNSUPPORTED")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	host := r.URL.Query().Get("host")
	dur := parseRange(r.URL.Query().Get("range"), 24*time.Hour)

	ch, cancel := s.hub.Subscribe()
	if ch == nil {
		// Hub is closed (server shutting down)
		writeErrorWithCode(w, http.StatusServiceUnavailable, "service unavailable", "SERVICE_UNAVAILABLE")
		return
	}
	defer cancel()

	// Send an initial snapshot.
	if summary, err := s.store.Summary(r.Context(), dur, host); err == nil {
		if buf, err := json.Marshal(summary); err == nil {
			writeSSE(w, "", buf)
			flusher.Flush()
		}
	}

	// Also send initial recent requests
	if recent, err := s.store.RecentRequests(r.Context(), 20, host); err == nil {
		if buf, err := json.Marshal(recent); err == nil {
			writeSSE(w, "recent", buf)
			flusher.Flush()
		}
	}

	for {
		select {
		case <-r.Context().Done():
			return
		case evt := <-ch:
			if evt.Type == "request" {
				// New request event - send directly
				writeSSE(w, "request", evt.Payload)
				flusher.Flush()
			} else {
				// Summary update - re-fetch with host filter
				if summary, err := s.store.Summary(r.Context(), dur, host); err == nil {
					if buf, err := json.Marshal(summary); err == nil {
						writeSSE(w, "", buf)
						flusher.Flush()
					}
				}
			}
		}
	}
}

func writeSSE(w http.ResponseWriter, eventType string, payload []byte) {
	if eventType != "" {
		_, _ = w.Write([]byte("event: "))
		_, _ = w.Write([]byte(eventType))
		_, _ = w.Write([]byte("\n"))
	}
	_, _ = w.Write([]byte("data: "))
	_, _ = w.Write(payload)
	_, _ = w.Write([]byte("\n\n"))
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Warn("failed to write JSON response", "error", err)
	}
}

// APIError represents a structured JSON error response.
type APIError struct {
	Error   string `json:"error"`
	Code    string `json:"code,omitempty"`
	Details string `json:"details,omitempty"`
}

// writeErrorWithCode writes a structured JSON error response with a machine-readable error code.
func writeErrorWithCode(w http.ResponseWriter, statusCode int, message, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(APIError{Error: message, Code: code}); err != nil {
		slog.Warn("failed to write JSON error response", "error", err)
	}
}

// writeInternalError writes an internal server error, logging the original error
// while returning a generic message to the client.
func writeInternalError(w http.ResponseWriter, err error, context string) {
	slog.Error("internal error", "context", context, "error", err)
	writeErrorWithCode(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
}

func parseRange(val string, def time.Duration) time.Duration {
	if val == "" {
		return def
	}
	if d, err := time.ParseDuration(val); err == nil {
		return d
	}
	return def
}

func (s *Server) handleExportCSV(w http.ResponseWriter, r *http.Request) {
	dur := parseRange(r.URL.Query().Get("range"), 24*time.Hour)
	host := r.URL.Query().Get("host")

	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=caddystat-export-%s.csv", time.Now().Format("2006-01-02")))

	csvWriter := csv.NewWriter(w)
	defer csvWriter.Flush()

	// Write header
	header := []string{
		"id", "timestamp", "host", "path", "status", "bytes", "ip", "referrer", "user_agent",
		"response_time_ms", "country", "region", "city", "browser", "browser_version",
		"os", "os_version", "device_type", "is_bot", "bot_name",
	}
	if err := csvWriter.Write(header); err != nil {
		// At this point content type is already set to CSV, can't return JSON error
		slog.Warn("failed to write CSV header", "error", err)
		return
	}

	err := s.store.ExportRequests(r.Context(), dur, host, 1000, func(requests []storage.ExportRequest) error {
		for _, req := range requests {
			record := []string{
				strconv.FormatInt(req.ID, 10),
				req.Timestamp.Format(time.RFC3339),
				req.Host,
				req.Path,
				strconv.Itoa(req.Status),
				strconv.FormatInt(req.Bytes, 10),
				req.IP,
				req.Referrer,
				req.UserAgent,
				strconv.FormatFloat(req.ResponseTimeMs, 'f', 2, 64),
				req.Country,
				req.Region,
				req.City,
				req.Browser,
				req.BrowserVersion,
				req.OS,
				req.OSVersion,
				req.DeviceType,
				strconv.FormatBool(req.IsBot),
				req.BotName,
			}
			if err := csvWriter.Write(record); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		slog.Warn("failed to export CSV", "error", err)
	}
}

func (s *Server) handleExportJSON(w http.ResponseWriter, r *http.Request) {
	dur := parseRange(r.URL.Query().Get("range"), 24*time.Hour)
	host := r.URL.Query().Get("host")

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=caddystat-export-%s.json", time.Now().Format("2006-01-02")))

	// Write opening bracket
	if _, err := w.Write([]byte("[\n")); err != nil {
		return
	}

	first := true
	err := s.store.ExportRequests(r.Context(), dur, host, 1000, func(requests []storage.ExportRequest) error {
		for _, req := range requests {
			if !first {
				if _, err := w.Write([]byte(",\n")); err != nil {
					return err
				}
			}
			first = false

			data, err := json.Marshal(req)
			if err != nil {
				return err
			}
			if _, err := w.Write(data); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		slog.Warn("failed to export JSON", "error", err)
	}

	// Write closing bracket
	_, _ = w.Write([]byte("\n]"))
}

func (s *Server) handleExportBackup(w http.ResponseWriter, r *http.Request) {
	dbPath := s.cfg.DBPath

	// Open the database file for reading
	file, err := os.Open(dbPath)
	if err != nil {
		writeInternalError(w, err, "open database for backup")
		return
	}
	defer file.Close()

	// Get file info for size
	info, err := file.Stat()
	if err != nil {
		writeInternalError(w, err, "stat database for backup")
		return
	}

	w.Header().Set("Content-Type", "application/x-sqlite3")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=caddystat-backup-%s.db", time.Now().Format("2006-01-02")))
	w.Header().Set("Content-Length", strconv.FormatInt(info.Size(), 10))

	if _, err := io.Copy(w, file); err != nil {
		slog.Warn("failed to send database backup", "error", err)
	}
}

// Site management handlers

func (s *Server) handleSites(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleListSites(w, r)
	case http.MethodPost:
		s.handleCreateSite(w, r)
	default:
		writeErrorWithCode(w, http.StatusMethodNotAllowed, "method not allowed", "METHOD_NOT_ALLOWED")
	}
}

func (s *Server) handleListSites(w http.ResponseWriter, r *http.Request) {
	summary, err := s.store.ListSites(r.Context())
	if err != nil {
		writeInternalError(w, err, "list sites")
		return
	}
	writeJSON(w, summary)
}

func (s *Server) handleCreateSite(w http.ResponseWriter, r *http.Request) {
	var input storage.SiteInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeErrorWithCode(w, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}

	if input.Host == "" {
		writeErrorWithCode(w, http.StatusBadRequest, "host is required", "MISSING_HOST")
		return
	}

	// Check if site already exists
	existing, err := s.store.GetSiteByHost(r.Context(), input.Host)
	if err != nil {
		writeInternalError(w, err, "check existing site")
		return
	}
	if existing != nil {
		writeErrorWithCode(w, http.StatusConflict, "site already exists", "SITE_EXISTS")
		return
	}

	site, err := s.store.CreateSite(r.Context(), input)
	if err != nil {
		writeInternalError(w, err, "create site")
		return
	}

	w.WriteHeader(http.StatusCreated)
	writeJSON(w, site)
}

func (s *Server) handleSiteByID(w http.ResponseWriter, r *http.Request) {
	// Extract site ID from path: /api/sites/{id}
	path := r.URL.Path
	prefix := "/api/sites/"
	if len(path) <= len(prefix) {
		writeErrorWithCode(w, http.StatusBadRequest, "site ID required", "MISSING_ID")
		return
	}

	idStr := path[len(prefix):]
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeErrorWithCode(w, http.StatusBadRequest, "invalid site ID", "INVALID_ID")
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.handleGetSite(w, r, id)
	case http.MethodPut:
		s.handleUpdateSite(w, r, id)
	case http.MethodDelete:
		s.handleDeleteSite(w, r, id)
	default:
		writeErrorWithCode(w, http.StatusMethodNotAllowed, "method not allowed", "METHOD_NOT_ALLOWED")
	}
}

func (s *Server) handleGetSite(w http.ResponseWriter, r *http.Request, id int64) {
	site, err := s.store.GetSite(r.Context(), id)
	if err != nil {
		writeInternalError(w, err, "get site")
		return
	}
	if site == nil {
		writeErrorWithCode(w, http.StatusNotFound, "site not found", "NOT_FOUND")
		return
	}
	writeJSON(w, site)
}

func (s *Server) handleUpdateSite(w http.ResponseWriter, r *http.Request, id int64) {
	var input storage.SiteInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeErrorWithCode(w, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}

	site, err := s.store.UpdateSite(r.Context(), id, input)
	if err != nil {
		writeInternalError(w, err, "update site")
		return
	}
	if site == nil {
		writeErrorWithCode(w, http.StatusNotFound, "site not found", "NOT_FOUND")
		return
	}
	writeJSON(w, site)
}

func (s *Server) handleDeleteSite(w http.ResponseWriter, r *http.Request, id int64) {
	if err := s.store.DeleteSite(r.Context(), id); err != nil {
		if err.Error() == "site not found" {
			writeErrorWithCode(w, http.StatusNotFound, "site not found", "NOT_FOUND")
			return
		}
		writeInternalError(w, err, "delete site")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
