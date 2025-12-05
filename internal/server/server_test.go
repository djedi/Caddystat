package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dustin/Caddystat/internal/config"
	"github.com/dustin/Caddystat/internal/sse"
	"github.com/dustin/Caddystat/internal/storage"
	"github.com/dustin/Caddystat/internal/version"
)

func setupTestServer(t *testing.T) (*Server, func()) {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "caddystat-test-*")
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

	cleanup := func() {
		store.Close()
		os.RemoveAll(tmpDir)
	}

	return srv, cleanup
}

func TestHealthEndpoint_Healthy(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp["status"] != "ok" {
		t.Errorf("expected status 'ok', got %q", resp["status"])
	}
	if resp["db"] != "connected" {
		t.Errorf("expected db 'connected', got %q", resp["db"])
	}
	if resp["version"] != version.Version {
		t.Errorf("expected version %q, got %q", version.Version, resp["version"])
	}

	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected Content-Type 'application/json', got %q", ct)
	}
}

func TestRobotsTxt(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/robots.txt", nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	expected := "User-agent: *\nDisallow: /\n"
	if w.Body.String() != expected {
		t.Errorf("expected body %q, got %q", expected, w.Body.String())
	}

	ct := w.Header().Get("Content-Type")
	if ct != "text/plain" {
		t.Errorf("expected Content-Type 'text/plain', got %q", ct)
	}
}

func TestXRobotsTagHeader(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	// Test that X-Robots-Tag is set on various endpoints
	endpoints := []string{"/health", "/robots.txt", "/api/auth/check"}

	for _, endpoint := range endpoints {
		t.Run(endpoint, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, endpoint, nil)
			w := httptest.NewRecorder()

			srv.ServeHTTP(w, req)

			robotsTag := w.Header().Get("X-Robots-Tag")
			if robotsTag != "noindex, nofollow" {
				t.Errorf("expected X-Robots-Tag 'noindex, nofollow', got %q", robotsTag)
			}
		})
	}
}

func TestMetricsEndpoint(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	// Should return Prometheus text format (various versions may exist)
	ct := w.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("expected Prometheus text format Content-Type starting with 'text/plain', got %q", ct)
	}

	// Should contain some standard Go metrics
	body := w.Body.String()
	if len(body) == 0 {
		t.Error("expected non-empty metrics body")
	}
}

func TestExportCSV(t *testing.T) {
	srv, cleanup := setupTestServerWithData(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/export/csv?range=24h", nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	// Check Content-Type
	ct := w.Header().Get("Content-Type")
	if ct != "text/csv" {
		t.Errorf("expected Content-Type 'text/csv', got %q", ct)
	}

	// Check Content-Disposition (should be attachment with filename)
	cd := w.Header().Get("Content-Disposition")
	if !strings.HasPrefix(cd, "attachment; filename=caddystat-export-") {
		t.Errorf("expected Content-Disposition to start with 'attachment; filename=caddystat-export-', got %q", cd)
	}
	if !strings.HasSuffix(cd, ".csv") {
		t.Errorf("expected Content-Disposition to end with '.csv', got %q", cd)
	}

	// Check CSV content
	body := w.Body.String()
	lines := strings.Split(strings.TrimSpace(body), "\n")

	// Should have header + at least 3 data rows
	if len(lines) < 4 {
		t.Errorf("expected at least 4 lines (header + 3 records), got %d", len(lines))
	}

	// Check header row
	expectedHeader := "id,timestamp,host,path,status,bytes,ip,referrer,user_agent,response_time_ms,country,region,city,browser,browser_version,os,os_version,device_type,is_bot,bot_name"
	if lines[0] != expectedHeader {
		t.Errorf("expected header %q, got %q", expectedHeader, lines[0])
	}

	// Check that data rows contain expected content
	if !strings.Contains(body, "example.com") {
		t.Error("expected CSV to contain 'example.com'")
	}
	if !strings.Contains(body, "/about") {
		t.Error("expected CSV to contain '/about'")
	}
}

func TestExportCSV_HostFilter(t *testing.T) {
	srv, cleanup := setupTestServerWithData(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/export/csv?range=24h&host=nonexistent.com", nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	// Should only have header line (no data for non-existent host)
	body := w.Body.String()
	lines := strings.Split(strings.TrimSpace(body), "\n")
	if len(lines) != 1 {
		t.Errorf("expected 1 line (header only for non-existent host), got %d", len(lines))
	}
}

func TestExportJSON(t *testing.T) {
	srv, cleanup := setupTestServerWithData(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/export/json?range=24h", nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	// Check Content-Type
	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected Content-Type 'application/json', got %q", ct)
	}

	// Check Content-Disposition (should be attachment with filename)
	cd := w.Header().Get("Content-Disposition")
	if !strings.HasPrefix(cd, "attachment; filename=caddystat-export-") {
		t.Errorf("expected Content-Disposition to start with 'attachment; filename=caddystat-export-', got %q", cd)
	}
	if !strings.HasSuffix(cd, ".json") {
		t.Errorf("expected Content-Disposition to end with '.json', got %q", cd)
	}

	// Check JSON content - should be valid JSON array
	var data []map[string]any
	if err := json.NewDecoder(w.Body).Decode(&data); err != nil {
		t.Fatalf("failed to decode JSON response: %v", err)
	}

	// Should have at least 3 records
	if len(data) < 3 {
		t.Errorf("expected at least 3 records, got %d", len(data))
	}

	// Check that records contain expected fields
	for _, record := range data {
		if _, ok := record["id"]; !ok {
			t.Error("expected record to have 'id' field")
		}
		if _, ok := record["timestamp"]; !ok {
			t.Error("expected record to have 'timestamp' field")
		}
		if _, ok := record["host"]; !ok {
			t.Error("expected record to have 'host' field")
		}
	}
}

func TestExportJSON_HostFilter(t *testing.T) {
	srv, cleanup := setupTestServerWithData(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/export/json?range=24h&host=nonexistent.com", nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	// Should be empty JSON array
	var data []map[string]any
	if err := json.NewDecoder(w.Body).Decode(&data); err != nil {
		t.Fatalf("failed to decode JSON response: %v", err)
	}

	if len(data) != 0 {
		t.Errorf("expected 0 records for non-existent host, got %d", len(data))
	}
}

func TestExportBackup(t *testing.T) {
	srv, cleanup := setupTestServerWithData(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/export/backup", nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	// Check Content-Type
	ct := w.Header().Get("Content-Type")
	if ct != "application/x-sqlite3" {
		t.Errorf("expected Content-Type 'application/x-sqlite3', got %q", ct)
	}

	// Check Content-Disposition (should be attachment with filename)
	cd := w.Header().Get("Content-Disposition")
	if !strings.HasPrefix(cd, "attachment; filename=caddystat-backup-") {
		t.Errorf("expected Content-Disposition to start with 'attachment; filename=caddystat-backup-', got %q", cd)
	}
	if !strings.HasSuffix(cd, ".db") {
		t.Errorf("expected Content-Disposition to end with '.db', got %q", cd)
	}

	// Check that we got some data (SQLite file should be non-empty)
	if w.Body.Len() == 0 {
		t.Error("expected non-empty backup body")
	}

	// SQLite files start with "SQLite format 3\000"
	body := w.Body.Bytes()
	if len(body) < 16 || string(body[:16]) != "SQLite format 3\x00" {
		t.Error("expected response to be a valid SQLite database file")
	}

	// Check Content-Length header
	cl := w.Header().Get("Content-Length")
	if cl == "" {
		t.Error("expected Content-Length header to be set")
	}
}

func TestJSONErrorResponse_Unauthorized(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "caddystat-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	store, err := storage.New(dbPath)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	defer store.Close()

	hub := sse.NewHub()
	cfg := config.Config{
		ListenAddr:   ":8404",
		DBPath:       dbPath,
		AuthUsername: "admin",
		AuthPassword: "secret",
	}
	srv := New(store, hub, cfg, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/stats/summary", nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d", http.StatusUnauthorized, w.Code)
	}

	// Check Content-Type is JSON
	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected Content-Type 'application/json', got %q", ct)
	}

	// Check JSON error structure
	var errResp APIError
	if err := json.NewDecoder(w.Body).Decode(&errResp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}

	if errResp.Error != "unauthorized" {
		t.Errorf("expected error 'unauthorized', got %q", errResp.Error)
	}
	if errResp.Code != "UNAUTHORIZED" {
		t.Errorf("expected code 'UNAUTHORIZED', got %q", errResp.Code)
	}
}

func TestJSONErrorResponse_MethodNotAllowed(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	// Try to login with GET instead of POST
	req := httptest.NewRequest(http.MethodGet, "/api/auth/login", nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status %d, got %d", http.StatusMethodNotAllowed, w.Code)
	}

	// Check Content-Type is JSON
	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected Content-Type 'application/json', got %q", ct)
	}

	// Check JSON error structure
	var errResp APIError
	if err := json.NewDecoder(w.Body).Decode(&errResp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}

	if errResp.Error != "method not allowed" {
		t.Errorf("expected error 'method not allowed', got %q", errResp.Error)
	}
	if errResp.Code != "METHOD_NOT_ALLOWED" {
		t.Errorf("expected code 'METHOD_NOT_ALLOWED', got %q", errResp.Code)
	}
}

func TestJSONErrorResponse_RateLimited(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "caddystat-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	store, err := storage.New(dbPath)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	defer store.Close()

	hub := sse.NewHub()
	cfg := config.Config{
		ListenAddr:         ":8404",
		DBPath:             dbPath,
		RateLimitPerMinute: 1, // Very low limit for testing
	}
	srv := New(store, hub, cfg, nil)

	// First request should succeed
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("first request: expected status %d, got %d", http.StatusOK, w.Code)
	}

	// Second request should be rate limited
	req = httptest.NewRequest(http.MethodGet, "/health", nil)
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("expected status %d, got %d", http.StatusTooManyRequests, w.Code)
	}

	// Check Content-Type is JSON
	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected Content-Type 'application/json', got %q", ct)
	}

	// Check JSON error structure
	var errResp APIError
	if err := json.NewDecoder(w.Body).Decode(&errResp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}

	if errResp.Error != "rate limit exceeded" {
		t.Errorf("expected error 'rate limit exceeded', got %q", errResp.Error)
	}
	if errResp.Code != "RATE_LIMITED" {
		t.Errorf("expected code 'RATE_LIMITED', got %q", errResp.Code)
	}
}

func TestJSONErrorResponse_InvalidCredentials(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "caddystat-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	store, err := storage.New(dbPath)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	defer store.Close()

	hub := sse.NewHub()
	cfg := config.Config{
		ListenAddr:   ":8404",
		DBPath:       dbPath,
		AuthUsername: "admin",
		AuthPassword: "secret",
	}
	srv := New(store, hub, cfg, nil)

	// Set CSRF cookie first
	initReq := httptest.NewRequest(http.MethodGet, "/api/auth/check", nil)
	initW := httptest.NewRecorder()
	srv.ServeHTTP(initW, initReq)
	csrfCookie := initW.Result().Cookies()[0]

	// Try login with wrong credentials
	body := strings.NewReader(`{"username": "admin", "password": "wrong"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", body)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(csrfCookie)
	req.Header.Set("X-CSRF-Token", csrfCookie.Value)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d", http.StatusUnauthorized, w.Code)
	}

	// Check Content-Type is JSON
	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected Content-Type 'application/json', got %q", ct)
	}

	// Check JSON error structure
	var errResp APIError
	if err := json.NewDecoder(w.Body).Decode(&errResp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}

	if errResp.Error != "invalid credentials" {
		t.Errorf("expected error 'invalid credentials', got %q", errResp.Error)
	}
	if errResp.Code != "INVALID_CREDENTIALS" {
		t.Errorf("expected code 'INVALID_CREDENTIALS', got %q", errResp.Code)
	}
}

func TestJSONErrorResponse_CSRFMissing(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "caddystat-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	store, err := storage.New(dbPath)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	defer store.Close()

	hub := sse.NewHub()
	cfg := config.Config{
		ListenAddr:   ":8404",
		DBPath:       dbPath,
		AuthUsername: "admin",
		AuthPassword: "secret",
	}
	srv := New(store, hub, cfg, nil)

	// Try login without CSRF token
	body := strings.NewReader(`{"username": "admin", "password": "secret"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected status %d, got %d", http.StatusForbidden, w.Code)
	}

	// Check Content-Type is JSON
	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected Content-Type 'application/json', got %q", ct)
	}

	// Check JSON error structure
	var errResp APIError
	if err := json.NewDecoder(w.Body).Decode(&errResp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}

	if errResp.Error != "invalid or missing CSRF token" {
		t.Errorf("expected error 'invalid or missing CSRF token', got %q", errResp.Error)
	}
	if errResp.Code != "CSRF_INVALID" {
		t.Errorf("expected code 'CSRF_INVALID', got %q", errResp.Code)
	}
}

func TestJSONErrorResponse_InvalidRequestBody(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "caddystat-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	store, err := storage.New(dbPath)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	defer store.Close()

	hub := sse.NewHub()
	cfg := config.Config{
		ListenAddr:   ":8404",
		DBPath:       dbPath,
		AuthUsername: "admin",
		AuthPassword: "secret",
	}
	srv := New(store, hub, cfg, nil)

	// Set CSRF cookie first
	initReq := httptest.NewRequest(http.MethodGet, "/api/auth/check", nil)
	initW := httptest.NewRecorder()
	srv.ServeHTTP(initW, initReq)
	csrfCookie := initW.Result().Cookies()[0]

	// Try login with invalid JSON
	body := strings.NewReader(`{invalid json`)
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", body)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(csrfCookie)
	req.Header.Set("X-CSRF-Token", csrfCookie.Value)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}

	// Check Content-Type is JSON
	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected Content-Type 'application/json', got %q", ct)
	}

	// Check JSON error structure
	var errResp APIError
	if err := json.NewDecoder(w.Body).Decode(&errResp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}

	if errResp.Error != "invalid request body" {
		t.Errorf("expected error 'invalid request body', got %q", errResp.Error)
	}
	if errResp.Code != "INVALID_REQUEST" {
		t.Errorf("expected code 'INVALID_REQUEST', got %q", errResp.Code)
	}
}

func TestPerformanceEndpoint(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/stats/performance", nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp storage.PerformanceStats
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Empty database should return zero response time stats
	if resp.ResponseTime.Count != 0 {
		t.Errorf("expected ResponseTime.Count = 0, got %d", resp.ResponseTime.Count)
	}
}

func TestPerformanceEndpoint_WithRangeFilter(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/stats/performance?range=1h&host=example.com", nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp storage.PerformanceStats
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Should return empty stats for filtered query
	if resp.ResponseTime.Count != 0 {
		t.Errorf("expected ResponseTime.Count = 0, got %d", resp.ResponseTime.Count)
	}
}

func TestBandwidthEndpoint(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/stats/bandwidth", nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp storage.BandwidthStats
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Empty database should return zero stats
	if resp.TotalBytes != 0 {
		t.Errorf("expected TotalBytes = 0, got %d", resp.TotalBytes)
	}
	if resp.TotalHuman != "0 B" {
		t.Errorf("expected TotalHuman = '0 B', got %s", resp.TotalHuman)
	}
}

func TestBandwidthEndpoint_WithRangeFilter(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/stats/bandwidth?range=1h&host=example.com&limit=5", nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp storage.BandwidthStats
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Should return empty stats for filtered query
	if resp.TotalBytes != 0 {
		t.Errorf("expected TotalBytes = 0, got %d", resp.TotalBytes)
	}
}

func TestSessionsEndpoint(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/stats/sessions", nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp storage.VisitorSessionSummary
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Empty database should return zero sessions
	if resp.TotalSessions != 0 {
		t.Errorf("expected TotalSessions = 0, got %d", resp.TotalSessions)
	}
	if resp.BounceRate != 0 {
		t.Errorf("expected BounceRate = 0, got %f", resp.BounceRate)
	}

	// Should have SessionsByHour with 24 entries
	if len(resp.SessionsByHour) != 24 {
		t.Errorf("expected 24 hour buckets, got %d", len(resp.SessionsByHour))
	}
}

func TestSessionsEndpoint_WithFilters(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/stats/sessions?range=1h&host=example.com&limit=10&timeout=900", nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp storage.VisitorSessionSummary
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Should return empty stats for filtered query on empty db
	if resp.TotalSessions != 0 {
		t.Errorf("expected TotalSessions = 0, got %d", resp.TotalSessions)
	}
}

func setupTestServerWithAuthAndStore(t *testing.T, username, password string) (*Server, *storage.Storage, func()) {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "caddystat-test-*")
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
		AuthUsername: username,
		AuthPassword: password,
	}
	srv := New(store, hub, cfg, nil)

	cleanup := func() {
		store.Close()
		os.RemoveAll(tmpDir)
	}

	return srv, store, cleanup
}

func TestSitePermission_AllowedHost(t *testing.T) {
	srv, store, cleanup := setupTestServerWithAuthAndStore(t, "admin", "secret")
	defer cleanup()

	// Login and get session with specific site permission
	initReq := httptest.NewRequest(http.MethodGet, "/api/auth/check", nil)
	initW := httptest.NewRecorder()
	srv.ServeHTTP(initW, initReq)
	csrfCookie := initW.Result().Cookies()[0]

	body := strings.NewReader(`{"username": "admin", "password": "secret", "allowed_sites": ["allowed.com"]}`)
	loginReq := httptest.NewRequest(http.MethodPost, "/api/auth/login", body)
	loginReq.Header.Set("Content-Type", "application/json")
	loginReq.AddCookie(csrfCookie)
	loginReq.Header.Set("X-CSRF-Token", csrfCookie.Value)
	loginW := httptest.NewRecorder()

	srv.ServeHTTP(loginW, loginReq)

	if loginW.Code != http.StatusOK {
		t.Fatalf("login failed: expected %d, got %d", http.StatusOK, loginW.Code)
	}

	// Get session cookie
	sessionCookie := getSessionCookie(loginW.Result().Cookies())
	if sessionCookie == nil {
		t.Fatal("session cookie not set")
	}

	// Verify permissions in DB
	perms, err := store.GetSessionPermissions(initReq.Context(), sessionCookie.Value)
	if err != nil {
		t.Fatalf("GetSessionPermissions error: %v", err)
	}
	if perms.AllSites {
		t.Error("session should not have AllSites permission")
	}
	if len(perms.AllowedHosts) != 1 || perms.AllowedHosts[0] != "allowed.com" {
		t.Errorf("expected allowed hosts [allowed.com], got %v", perms.AllowedHosts)
	}

	// Test accessing non-allowed host - should be blocked by permission middleware
	req2 := httptest.NewRequest(http.MethodGet, "/api/stats/summary?host=notallowed.com", nil)
	req2.AddCookie(sessionCookie)
	w2 := httptest.NewRecorder()
	srv.ServeHTTP(w2, req2)

	if w2.Code != http.StatusForbidden {
		t.Errorf("accessing non-allowed host: expected %d, got %d", http.StatusForbidden, w2.Code)
	}

	// Check JSON error code
	var errResp APIError
	if err := json.NewDecoder(w2.Body).Decode(&errResp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if errResp.Code != "SITE_ACCESS_DENIED" {
		t.Errorf("expected code 'SITE_ACCESS_DENIED', got %q", errResp.Code)
	}
}

func TestSitePermission_AllSitesDefault(t *testing.T) {
	srv, store, cleanup := setupTestServerWithAuthAndStore(t, "admin", "secret")
	defer cleanup()

	// Login without specifying allowed_sites (should default to all)
	initReq := httptest.NewRequest(http.MethodGet, "/api/auth/check", nil)
	initW := httptest.NewRecorder()
	srv.ServeHTTP(initW, initReq)
	csrfCookie := initW.Result().Cookies()[0]

	body := strings.NewReader(`{"username": "admin", "password": "secret"}`)
	loginReq := httptest.NewRequest(http.MethodPost, "/api/auth/login", body)
	loginReq.Header.Set("Content-Type", "application/json")
	loginReq.AddCookie(csrfCookie)
	loginReq.Header.Set("X-CSRF-Token", csrfCookie.Value)
	loginW := httptest.NewRecorder()

	srv.ServeHTTP(loginW, loginReq)

	if loginW.Code != http.StatusOK {
		t.Fatalf("login failed: expected %d, got %d", http.StatusOK, loginW.Code)
	}

	// Get session cookie
	sessionCookie := getSessionCookie(loginW.Result().Cookies())
	if sessionCookie == nil {
		t.Fatal("session cookie not set")
	}

	// Verify permissions in DB
	perms, err := store.GetSessionPermissions(initReq.Context(), sessionCookie.Value)
	if err != nil {
		t.Fatalf("GetSessionPermissions error: %v", err)
	}
	if !perms.AllSites {
		t.Error("session without allowed_sites should have AllSites permission")
	}

	// Test that the middleware passes for any host with AllSites permission
	// We test with /api/stats/status which doesn't have DB dependency issues
	req := httptest.NewRequest(http.MethodGet, "/api/stats/status", nil)
	req.AddCookie(sessionCookie)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("accessing status with AllSites: expected %d, got %d", http.StatusOK, w.Code)
	}
}

func TestSitePermission_NoHostFilter(t *testing.T) {
	srv, _, cleanup := setupTestServerWithAuthAndStore(t, "admin", "secret")
	defer cleanup()

	// Login with specific site permission
	initReq := httptest.NewRequest(http.MethodGet, "/api/auth/check", nil)
	initW := httptest.NewRecorder()
	srv.ServeHTTP(initW, initReq)
	csrfCookie := initW.Result().Cookies()[0]

	body := strings.NewReader(`{"username": "admin", "password": "secret", "allowed_sites": ["allowed.com"]}`)
	loginReq := httptest.NewRequest(http.MethodPost, "/api/auth/login", body)
	loginReq.Header.Set("Content-Type", "application/json")
	loginReq.AddCookie(csrfCookie)
	loginReq.Header.Set("X-CSRF-Token", csrfCookie.Value)
	loginW := httptest.NewRecorder()

	srv.ServeHTTP(loginW, loginReq)

	sessionCookie := getSessionCookie(loginW.Result().Cookies())
	if sessionCookie == nil {
		t.Fatal("session cookie not set")
	}

	// Test accessing without host filter (aggregate view should be allowed)
	// Use /api/stats/status which works on empty DB
	req := httptest.NewRequest(http.MethodGet, "/api/stats/status", nil)
	req.AddCookie(sessionCookie)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("accessing without host filter: expected %d, got %d", http.StatusOK, w.Code)
	}
}

func TestAuthCheckIncludesPermissions(t *testing.T) {
	srv, _, cleanup := setupTestServerWithAuthAndStore(t, "admin", "secret")
	defer cleanup()

	// Login with specific site permission
	initReq := httptest.NewRequest(http.MethodGet, "/api/auth/check", nil)
	initW := httptest.NewRecorder()
	srv.ServeHTTP(initW, initReq)
	csrfCookie := initW.Result().Cookies()[0]

	body := strings.NewReader(`{"username": "admin", "password": "secret", "allowed_sites": ["site1.com", "site2.com"]}`)
	loginReq := httptest.NewRequest(http.MethodPost, "/api/auth/login", body)
	loginReq.Header.Set("Content-Type", "application/json")
	loginReq.AddCookie(csrfCookie)
	loginReq.Header.Set("X-CSRF-Token", csrfCookie.Value)
	loginW := httptest.NewRecorder()

	srv.ServeHTTP(loginW, loginReq)

	sessionCookie := getSessionCookie(loginW.Result().Cookies())
	if sessionCookie == nil {
		t.Fatal("session cookie not set")
	}

	// Check auth status includes permissions
	checkReq := httptest.NewRequest(http.MethodGet, "/api/auth/check", nil)
	checkReq.AddCookie(sessionCookie)
	checkW := httptest.NewRecorder()
	srv.ServeHTTP(checkW, checkReq)

	if checkW.Code != http.StatusOK {
		t.Fatalf("auth check failed: expected %d, got %d", http.StatusOK, checkW.Code)
	}

	var resp map[string]any
	if err := json.NewDecoder(checkW.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp["authenticated"] != true {
		t.Error("expected authenticated=true")
	}
	if resp["auth_required"] != true {
		t.Error("expected auth_required=true")
	}

	perms, ok := resp["permissions"].(map[string]any)
	if !ok {
		t.Fatal("expected permissions in response")
	}
	if perms["all_sites"] != false {
		t.Error("expected all_sites=false")
	}
	allowedHosts, ok := perms["allowed_hosts"].([]any)
	if !ok || len(allowedHosts) != 2 {
		t.Errorf("expected 2 allowed hosts, got %v", allowedHosts)
	}
}

func getSessionCookie(cookies []*http.Cookie) *http.Cookie {
	for _, c := range cookies {
		if c.Name == "caddystat_session" {
			return c
		}
	}
	return nil
}
