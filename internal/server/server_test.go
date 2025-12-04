package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
	srv := New(store, hub, cfg)

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
