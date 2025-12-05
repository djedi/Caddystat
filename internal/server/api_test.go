package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dustin/Caddystat/internal/config"
	"github.com/dustin/Caddystat/internal/sse"
	"github.com/dustin/Caddystat/internal/storage"
)

// setupTestServerWithData creates a test server with sample data
func setupTestServerWithData(t *testing.T) (*Server, func()) {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "caddystat-api-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	dbPath := filepath.Join(tmpDir, "test.db")
	store, err := storage.New(dbPath)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("failed to create storage: %v", err)
	}

	hub := sse.NewHub()
	cfg := config.Config{
		ListenAddr: ":8404",
		DBPath:     dbPath,
	}
	srv := New(store, hub, cfg, nil)

	// Insert sample data
	ctx := context.Background()
	now := time.Now().UTC()

	requests := []storage.RequestRecord{
		{
			Timestamp:      now.Add(-1 * time.Hour),
			Host:           "example.com",
			Path:           "/",
			Status:         200,
			Bytes:          12345,
			IP:             "192.168.1.1",
			Referrer:       "https://google.com",
			UserAgent:      "Mozilla/5.0 Chrome/120",
			ResponseTime:   50,
			Country:        "US",
			Region:         "California",
			City:           "San Francisco",
			Browser:        "Chrome",
			BrowserVersion: "120.0",
			OS:             "Windows",
			OSVersion:      "10",
			DeviceType:     "Desktop",
			IsBot:          false,
		},
		{
			Timestamp:      now.Add(-2 * time.Hour),
			Host:           "example.com",
			Path:           "/about",
			Status:         200,
			Bytes:          8765,
			IP:             "192.168.1.2",
			UserAgent:      "Mozilla/5.0 Safari/605",
			ResponseTime:   30,
			Country:        "UK",
			Region:         "England",
			City:           "London",
			Browser:        "Safari",
			BrowserVersion: "17.0",
			OS:             "macOS",
			OSVersion:      "14",
			DeviceType:     "Desktop",
			IsBot:          false,
		},
		{
			Timestamp:      now.Add(-3 * time.Hour),
			Host:           "example.com",
			Path:           "/missing",
			Status:         404,
			Bytes:          512,
			IP:             "192.168.1.3",
			UserAgent:      "Mozilla/5.0 Firefox/120",
			ResponseTime:   10,
			Browser:        "Firefox",
			BrowserVersion: "120.0",
			OS:             "Linux",
			DeviceType:     "Desktop",
			IsBot:          false,
		},
		{
			Timestamp:      now.Add(-4 * time.Hour),
			Host:           "example.com",
			Path:           "/error",
			Status:         500,
			Bytes:          256,
			IP:             "192.168.1.4",
			UserAgent:      "Mozilla/5.0 Chrome/120",
			ResponseTime:   150,
			Browser:        "Chrome",
			BrowserVersion: "120.0",
			OS:             "Windows",
			DeviceType:     "Desktop",
			IsBot:          false,
		},
		{
			Timestamp: now.Add(-5 * time.Hour),
			Host:      "example.com",
			Path:      "/robots.txt",
			Status:    200,
			Bytes:     100,
			IP:        "66.249.66.1",
			UserAgent: "Googlebot/2.1",
			IsBot:     true,
			BotName:   "Googlebot",
		},
		{
			Timestamp:      now.Add(-6 * time.Hour),
			Host:           "blog.example.com",
			Path:           "/posts/hello",
			Status:         200,
			Bytes:          15000,
			IP:             "10.0.0.1",
			Referrer:       "https://twitter.com",
			UserAgent:      "Mozilla/5.0 Chrome/120",
			ResponseTime:   80,
			Country:        "DE",
			Browser:        "Chrome",
			BrowserVersion: "120.0",
			OS:             "Android",
			DeviceType:     "Mobile",
			IsBot:          false,
		},
	}

	for _, req := range requests {
		if err := store.InsertRequest(ctx, req); err != nil {
			store.Close()
			os.RemoveAll(tmpDir)
			t.Fatalf("failed to insert request: %v", err)
		}
	}

	cleanup := func() {
		store.Close()
		os.RemoveAll(tmpDir)
	}

	return srv, cleanup
}

func TestAPISummary(t *testing.T) {
	srv, cleanup := setupTestServerWithData(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/stats/summary?range=24h", nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp storage.Summary
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.TotalRequests == 0 {
		t.Error("expected TotalRequests > 0")
	}
}

func TestAPISummary_WithHostFilter(t *testing.T) {
	srv, cleanup := setupTestServerWithData(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/stats/summary?range=24h&host=example.com", nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp storage.Summary
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Should only count example.com requests (5 total), not blog.example.com
	if resp.TotalRequests > 5 {
		t.Errorf("expected TotalRequests <= 5 for host filter, got %d", resp.TotalRequests)
	}
}

func TestAPIRequests(t *testing.T) {
	srv, cleanup := setupTestServerWithData(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/stats/requests?range=24h", nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp []storage.TimeSeriesStat
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected Content-Type 'application/json', got %q", ct)
	}
}

func TestAPIGeo(t *testing.T) {
	srv, cleanup := setupTestServerWithData(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/stats/geo?range=24h", nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp []storage.GeoStat
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// We have US, UK, and DE in our test data
	if len(resp) < 1 {
		t.Error("expected at least 1 geo stat")
	}
}

func TestAPIHosts(t *testing.T) {
	srv, cleanup := setupTestServerWithData(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/stats/hosts?range=24h", nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp []storage.VisitorStat
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
}

func TestAPIHosts_WithLimit(t *testing.T) {
	srv, cleanup := setupTestServerWithData(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/stats/hosts?range=24h&limit=2", nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp []storage.VisitorStat
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(resp) > 2 {
		t.Errorf("expected at most 2 results with limit=2, got %d", len(resp))
	}
}

func TestAPIBrowsers(t *testing.T) {
	srv, cleanup := setupTestServerWithData(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/stats/browsers?range=24h", nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp []storage.BrowserStat
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// We should have Chrome, Safari, Firefox in our test data
	if len(resp) < 1 {
		t.Error("expected at least 1 browser stat")
	}
}

func TestAPIOSStats(t *testing.T) {
	srv, cleanup := setupTestServerWithData(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/stats/os?range=24h", nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp []storage.OSStat
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
}

func TestAPIRobots(t *testing.T) {
	srv, cleanup := setupTestServerWithData(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/stats/robots?range=24h", nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp []storage.RobotStat
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// We have Googlebot in test data
	if len(resp) < 1 {
		t.Error("expected at least 1 robot stat")
	}

	foundGooglebot := false
	for _, r := range resp {
		if r.Name == "Googlebot" {
			foundGooglebot = true
			break
		}
	}
	if !foundGooglebot {
		t.Error("expected to find Googlebot in robot stats")
	}
}

func TestAPIReferrers(t *testing.T) {
	srv, cleanup := setupTestServerWithData(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/stats/referrers?range=24h", nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp []storage.ReferrerStat
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
}

func TestAPIRecentRequests(t *testing.T) {
	srv, cleanup := setupTestServerWithData(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/stats/recent?limit=10", nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp []storage.RequestRecord
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(resp) == 0 {
		t.Error("expected at least 1 recent request")
	}

	if len(resp) > 10 {
		t.Errorf("expected at most 10 results with limit=10, got %d", len(resp))
	}
}

func TestAPIRecentRequests_LimitCapped(t *testing.T) {
	srv, cleanup := setupTestServerWithData(t)
	defer cleanup()

	// Request more than the max (100)
	req := httptest.NewRequest(http.MethodGet, "/api/stats/recent?limit=200", nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	// Should succeed but limit is capped at 100
	var resp []storage.RequestRecord
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
}

func TestAPIMonthly(t *testing.T) {
	srv, cleanup := setupTestServerWithData(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/stats/monthly?months=12", nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp storage.MonthlyHistory
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
}

func TestAPIDaily(t *testing.T) {
	srv, cleanup := setupTestServerWithData(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/stats/daily", nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp storage.DailyHistory
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
}

func TestAPIStatus(t *testing.T) {
	srv, cleanup := setupTestServerWithData(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/stats/status", nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp storage.SystemStatus
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Verify the response has expected fields
	if resp.RequestsCount <= 0 {
		t.Error("expected RequestsCount > 0")
	}
	if resp.DBSizeHuman == "" {
		t.Error("expected DBSizeHuman to be set")
	}
}

func TestAPIAuthCheck_NoAuthConfigured(t *testing.T) {
	srv, cleanup := setupTestServerWithData(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/auth/check", nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp["authenticated"] != true {
		t.Error("expected authenticated=true when auth not configured")
	}
	if resp["auth_required"] != false {
		t.Error("expected auth_required=false when auth not configured")
	}
}

func TestAPIAuthCheck_AuthConfigured_NotAuthenticated(t *testing.T) {
	srv, cleanup := setupTestServerWithAuth(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/auth/check", nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp["authenticated"] != false {
		t.Error("expected authenticated=false when not logged in")
	}
	if resp["auth_required"] != true {
		t.Error("expected auth_required=true when auth configured")
	}
}

func setupTestServerWithAuth(t *testing.T) (*Server, func()) {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "caddystat-auth-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	dbPath := filepath.Join(tmpDir, "test.db")
	store, err := storage.New(dbPath)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("failed to create storage: %v", err)
	}

	hub := sse.NewHub()
	cfg := config.Config{
		ListenAddr:   ":8404",
		DBPath:       dbPath,
		AuthUsername: "admin",
		AuthPassword: "secret123",
	}
	srv := New(store, hub, cfg, nil)

	cleanup := func() {
		store.Close()
		os.RemoveAll(tmpDir)
	}

	return srv, cleanup
}

func setupTestServerWithAuthAndData(t *testing.T) (*Server, func()) {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "caddystat-auth-data-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	dbPath := filepath.Join(tmpDir, "test.db")
	store, err := storage.New(dbPath)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("failed to create storage: %v", err)
	}

	hub := sse.NewHub()
	cfg := config.Config{
		ListenAddr:   ":8404",
		DBPath:       dbPath,
		AuthUsername: "admin",
		AuthPassword: "secret123",
	}
	srv := New(store, hub, cfg, nil)

	// Insert sample data
	ctx := context.Background()
	now := time.Now().UTC()
	req := storage.RequestRecord{
		Timestamp:      now.Add(-1 * time.Hour),
		Host:           "example.com",
		Path:           "/",
		Status:         200,
		Bytes:          12345,
		IP:             "192.168.1.1",
		Browser:        "Chrome",
		BrowserVersion: "120.0",
		OS:             "Windows",
		DeviceType:     "Desktop",
		IsBot:          false,
	}
	if err := store.InsertRequest(ctx, req); err != nil {
		store.Close()
		os.RemoveAll(tmpDir)
		t.Fatalf("failed to insert request: %v", err)
	}

	cleanup := func() {
		store.Close()
		os.RemoveAll(tmpDir)
	}

	return srv, cleanup
}

func TestAPILogin_Success(t *testing.T) {
	srv, cleanup := setupTestServerWithAuth(t)
	defer cleanup()

	// First get CSRF token
	csrfReq := httptest.NewRequest(http.MethodGet, "/api/auth/check", nil)
	csrfW := httptest.NewRecorder()
	srv.ServeHTTP(csrfW, csrfReq)

	var csrfToken string
	for _, cookie := range csrfW.Result().Cookies() {
		if cookie.Name == csrfCookieName {
			csrfToken = cookie.Value
			break
		}
	}

	body := `{"username":"admin","password":"secret123"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(csrfHeaderName, csrfToken)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrfToken})
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp["authenticated"] != true {
		t.Error("expected authenticated=true after login")
	}

	// Check session cookie was set
	var sessionCookie *http.Cookie
	for _, cookie := range w.Result().Cookies() {
		if cookie.Name == "caddystat_session" {
			sessionCookie = cookie
			break
		}
	}
	if sessionCookie == nil {
		t.Error("expected session cookie to be set")
	}
}

func TestAPILogin_InvalidCredentials(t *testing.T) {
	srv, cleanup := setupTestServerWithAuth(t)
	defer cleanup()

	// First get CSRF token
	csrfReq := httptest.NewRequest(http.MethodGet, "/api/auth/check", nil)
	csrfW := httptest.NewRecorder()
	srv.ServeHTTP(csrfW, csrfReq)

	var csrfToken string
	for _, cookie := range csrfW.Result().Cookies() {
		if cookie.Name == csrfCookieName {
			csrfToken = cookie.Value
			break
		}
	}

	body := `{"username":"admin","password":"wrongpassword"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(csrfHeaderName, csrfToken)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrfToken})
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d", http.StatusUnauthorized, w.Code)
	}
}

func TestAPILogin_MethodNotAllowed(t *testing.T) {
	srv, cleanup := setupTestServerWithAuth(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/auth/login", nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status %d, got %d", http.StatusMethodNotAllowed, w.Code)
	}
}

func TestAPILogout(t *testing.T) {
	srv, cleanup := setupTestServerWithAuth(t)
	defer cleanup()

	// First get CSRF token
	csrfReq := httptest.NewRequest(http.MethodGet, "/api/auth/check", nil)
	csrfW := httptest.NewRecorder()
	srv.ServeHTTP(csrfW, csrfReq)

	var csrfToken string
	for _, cookie := range csrfW.Result().Cookies() {
		if cookie.Name == csrfCookieName {
			csrfToken = cookie.Value
			break
		}
	}

	req := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	req.Header.Set(csrfHeaderName, csrfToken)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrfToken})
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp["authenticated"] != false {
		t.Error("expected authenticated=false after logout")
	}
}

func TestProtectedEndpoint_RequiresAuth(t *testing.T) {
	srv, cleanup := setupTestServerWithAuth(t)
	defer cleanup()

	endpoints := []string{
		"/api/stats/summary",
		"/api/stats/requests",
		"/api/stats/geo",
		"/api/stats/hosts",
		"/api/stats/browsers",
		"/api/stats/os",
		"/api/stats/robots",
		"/api/stats/referrers",
		"/api/stats/recent",
		"/api/stats/monthly",
		"/api/stats/daily",
		"/api/stats/status",
	}

	for _, endpoint := range endpoints {
		t.Run(endpoint, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, endpoint, nil)
			w := httptest.NewRecorder()

			srv.ServeHTTP(w, req)

			if w.Code != http.StatusUnauthorized {
				t.Errorf("expected status %d for %s without auth, got %d", http.StatusUnauthorized, endpoint, w.Code)
			}
		})
	}
}

func TestProtectedEndpoint_WithValidSession(t *testing.T) {
	// Use setupTestServerWithAuthAndData to have data available
	srv, cleanup := setupTestServerWithAuthAndData(t)
	defer cleanup()

	// Get CSRF token and login
	csrfReq := httptest.NewRequest(http.MethodGet, "/api/auth/check", nil)
	csrfW := httptest.NewRecorder()
	srv.ServeHTTP(csrfW, csrfReq)

	var csrfToken string
	for _, cookie := range csrfW.Result().Cookies() {
		if cookie.Name == csrfCookieName {
			csrfToken = cookie.Value
			break
		}
	}

	loginBody := `{"username":"admin","password":"secret123"}`
	loginReq := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(loginBody))
	loginReq.Header.Set("Content-Type", "application/json")
	loginReq.Header.Set(csrfHeaderName, csrfToken)
	loginReq.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrfToken})
	loginW := httptest.NewRecorder()

	srv.ServeHTTP(loginW, loginReq)

	if loginW.Code != http.StatusOK {
		t.Fatalf("login failed: %d", loginW.Code)
	}

	var sessionCookie *http.Cookie
	for _, cookie := range loginW.Result().Cookies() {
		if cookie.Name == "caddystat_session" {
			sessionCookie = cookie
			break
		}
	}
	if sessionCookie == nil {
		t.Fatal("no session cookie after login")
	}

	// Now access protected endpoint with session
	req := httptest.NewRequest(http.MethodGet, "/api/stats/summary", nil)
	req.AddCookie(sessionCookie)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d with valid session, got %d", http.StatusOK, w.Code)
	}
}

func TestParseRange(t *testing.T) {
	tests := []struct {
		input    string
		def      time.Duration
		expected time.Duration
	}{
		{"", 24 * time.Hour, 24 * time.Hour},
		{"1h", 24 * time.Hour, time.Hour},
		{"24h", time.Hour, 24 * time.Hour},
		{"168h", time.Hour, 168 * time.Hour}, // Go duration format for 7 days
		{"30m", time.Hour, 30 * time.Minute},
		{"7d", 12 * time.Hour, 12 * time.Hour}, // "7d" is not valid Go duration, returns default
		{"invalid", 12 * time.Hour, 12 * time.Hour},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := parseRange(tt.input, tt.def)
			if result != tt.expected {
				t.Errorf("parseRange(%q, %v) = %v, want %v", tt.input, tt.def, result, tt.expected)
			}
		})
	}
}

func TestCSRFProtection_NoToken(t *testing.T) {
	srv, cleanup := setupTestServerWithAuth(t)
	defer cleanup()

	// Try to login without CSRF token
	body := `{"username":"admin","password":"secret123"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected status %d without CSRF token, got %d", http.StatusForbidden, w.Code)
	}
}

func TestCSRFProtection_MismatchedTokenLogin(t *testing.T) {
	srv, cleanup := setupTestServerWithAuth(t)
	defer cleanup()

	// Get a valid CSRF cookie
	csrfReq := httptest.NewRequest(http.MethodGet, "/api/auth/check", nil)
	csrfW := httptest.NewRecorder()
	srv.ServeHTTP(csrfW, csrfReq)

	var csrfToken string
	for _, cookie := range csrfW.Result().Cookies() {
		if cookie.Name == csrfCookieName {
			csrfToken = cookie.Value
			break
		}
	}

	// Try with wrong token in header
	body := `{"username":"admin","password":"secret123"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(csrfHeaderName, "wrong-token")
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrfToken})
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected status %d with mismatched CSRF token, got %d", http.StatusForbidden, w.Code)
	}
}

func TestAPIInvalidJSON(t *testing.T) {
	srv, cleanup := setupTestServerWithAuth(t)
	defer cleanup()

	// Get CSRF token
	csrfReq := httptest.NewRequest(http.MethodGet, "/api/auth/check", nil)
	csrfW := httptest.NewRecorder()
	srv.ServeHTTP(csrfW, csrfReq)

	var csrfToken string
	for _, cookie := range csrfW.Result().Cookies() {
		if cookie.Name == csrfCookieName {
			csrfToken = cookie.Value
			break
		}
	}

	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(csrfHeaderName, csrfToken)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrfToken})
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d for invalid JSON, got %d", http.StatusBadRequest, w.Code)
	}
}
