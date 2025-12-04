package server

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net/http"
	"path/filepath"
	"strconv"
	"time"

	"github.com/dustin/Caddystat/internal/config"
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
}

func New(store *storage.Storage, hub *sse.Hub, cfg config.Config) *Server {
	s := &Server{
		store:       store,
		hub:         hub,
		mux:         http.NewServeMux(),
		cfg:         cfg,
		rateLimiter: NewRateLimiter(cfg.RateLimitPerMinute, time.Minute),
	}
	s.routes()
	return s
}

func (s *Server) routes() {
	// Public endpoints (no auth required)
	s.mux.HandleFunc("/health", s.handleHealth)
	s.mux.HandleFunc("/robots.txt", s.handleRobotsTxt)

	// Auth endpoints (always accessible)
	s.mux.HandleFunc("/api/auth/login", s.handleLogin)
	s.mux.HandleFunc("/api/auth/logout", s.handleLogout)
	s.mux.HandleFunc("/api/auth/check", s.handleAuthCheck)

	// Protected API endpoints
	s.mux.HandleFunc("/api/stats/summary", s.requireAuth(s.handleSummary))
	s.mux.HandleFunc("/api/stats/monthly", s.requireAuth(s.handleMonthly))
	s.mux.HandleFunc("/api/stats/daily", s.requireAuth(s.handleDaily))
	s.mux.HandleFunc("/api/stats/requests", s.requireAuth(s.handleRequests))
	s.mux.HandleFunc("/api/stats/geo", s.requireAuth(s.handleGeo))
	s.mux.HandleFunc("/api/stats/hosts", s.requireAuth(s.handleVisitors))
	s.mux.HandleFunc("/api/stats/browsers", s.requireAuth(s.handleBrowsers))
	s.mux.HandleFunc("/api/stats/os", s.requireAuth(s.handleOS))
	s.mux.HandleFunc("/api/stats/robots", s.requireAuth(s.handleRobots))
	s.mux.HandleFunc("/api/stats/referrers", s.requireAuth(s.handleReferrers))
	s.mux.HandleFunc("/api/stats/recent", s.requireAuth(s.handleRecentRequests))
	s.mux.HandleFunc("/api/sse", s.requireAuth(s.handleSSE))

	site := http.Dir(filepath.Join(".", "web", "_site"))
	s.mux.Handle("/", http.FileServer(site))
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Prevent search engine indexing
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")

	// Apply rate limiting
	if s.rateLimiter.enabled {
		ip := extractIP(r)
		if !s.rateLimiter.Allow(ip) {
			slog.Debug("rate limit exceeded", "ip", ip, "path", r.URL.Path)
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}
	}

	// Apply body size limit
	if s.cfg.MaxRequestBodyBytes > 0 && r.ContentLength > s.cfg.MaxRequestBodyBytes {
		http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
		return
	}
	if s.cfg.MaxRequestBodyBytes > 0 && r.Body != nil {
		r.Body = http.MaxBytesReader(w, r.Body, s.cfg.MaxRequestBodyBytes)
	}

	s.mux.ServeHTTP(w, r)
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
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// If auth is not enabled, return success
	if !s.cfg.AuthEnabled() {
		writeJSON(w, map[string]any{"authenticated": true, "auth_required": false})
		return
	}

	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	// Use constant-time comparison to prevent timing attacks
	usernameMatch := subtle.ConstantTimeCompare([]byte(req.Username), []byte(s.cfg.AuthUsername)) == 1
	passwordMatch := subtle.ConstantTimeCompare([]byte(req.Password), []byte(s.cfg.AuthPassword)) == 1

	if !usernameMatch || !passwordMatch {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}

	token, err := s.createSession(r.Context())
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
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
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	cookie, err := r.Cookie(sessionCookieName)
	if err == nil {
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

	writeJSON(w, map[string]any{"authenticated": true, "auth_required": true})
}

func (s *Server) handleSummary(w http.ResponseWriter, r *http.Request) {
	dur := parseRange(r.URL.Query().Get("range"), 24*time.Hour)
	host := r.URL.Query().Get("host")
	stats, err := s.store.Summary(r.Context(), dur, host)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, stats)
}

func (s *Server) handleRequests(w http.ResponseWriter, r *http.Request) {
	dur := parseRange(r.URL.Query().Get("range"), 24*time.Hour)
	host := r.URL.Query().Get("host")
	stats, err := s.store.TimeSeriesRange(r.Context(), dur, host)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, stats)
}

func (s *Server) handleGeo(w http.ResponseWriter, r *http.Request) {
	dur := parseRange(r.URL.Query().Get("range"), 24*time.Hour)
	host := r.URL.Query().Get("host")
	stats, err := s.store.Geo(r.Context(), dur, host)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
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
		http.Error(w, err.Error(), http.StatusInternalServerError)
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
		http.Error(w, err.Error(), http.StatusInternalServerError)
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
		http.Error(w, err.Error(), http.StatusInternalServerError)
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
		http.Error(w, err.Error(), http.StatusInternalServerError)
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
		http.Error(w, err.Error(), http.StatusInternalServerError)
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
		http.Error(w, err.Error(), http.StatusInternalServerError)
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
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, stats)
}

func (s *Server) handleDaily(w http.ResponseWriter, r *http.Request) {
	host := r.URL.Query().Get("host")
	stats, err := s.store.DailyHistory(r.Context(), host)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, stats)
}

func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	host := r.URL.Query().Get("host")
	dur := parseRange(r.URL.Query().Get("range"), 24*time.Hour)

	ch, cancel := s.hub.Subscribe()
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

func parseRange(val string, def time.Duration) time.Duration {
	if val == "" {
		return def
	}
	if d, err := time.ParseDuration(val); err == nil {
		return d
	}
	return def
}
