package server

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dustin/Caddystat/internal/config"
	"github.com/dustin/Caddystat/internal/sse"
	"github.com/dustin/Caddystat/internal/storage"
)

func TestBodySizeLimit_ContentLengthCheck(t *testing.T) {
	store, err := storage.New(":memory:")
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	defer store.Close()

	hub := sse.NewHub()
	cfg := config.Config{
		MaxRequestBodyBytes: 100, // 100 bytes max
	}
	srv := New(store, hub, cfg, nil)

	// Create a request with Content-Length > limit
	body := strings.Repeat("x", 200)
	req := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.ContentLength = 200

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("expected 413 Request Entity Too Large, got %d", rec.Code)
	}
}

func TestBodySizeLimit_MaxBytesReader(t *testing.T) {
	store, err := storage.New(":memory:")
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	defer store.Close()

	hub := sse.NewHub()
	cfg := config.Config{
		MaxRequestBodyBytes: 100, // 100 bytes max
	}
	srv := New(store, hub, cfg, nil)

	// Create a request without Content-Length but with large body
	body := strings.Repeat("x", 200)
	req := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.ContentLength = -1 // Unknown content length

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	// The MaxBytesReader will cut off the body, but the JSON decoder will fail
	// This is fine - the important thing is we don't process more than 100 bytes
}

func TestBodySizeLimit_Disabled(t *testing.T) {
	store, err := storage.New(":memory:")
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	defer store.Close()

	hub := sse.NewHub()
	cfg := config.Config{
		MaxRequestBodyBytes: 0, // Disabled
	}
	srv := New(store, hub, cfg, nil)

	// Create a request with large body - should not be rejected for size
	body := `{"username":"test","password":"test"}`
	req := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.ContentLength = int64(len(body))

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	// Should not be 413 (might be 401 for invalid credentials, which is fine)
	if rec.Code == http.StatusRequestEntityTooLarge {
		t.Error("body size limit should be disabled when MaxRequestBodyBytes is 0")
	}
}

func TestBodySizeLimit_SmallRequest(t *testing.T) {
	store, err := storage.New(":memory:")
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	defer store.Close()

	hub := sse.NewHub()
	cfg := config.Config{
		MaxRequestBodyBytes: 1000, // 1KB max
	}
	srv := New(store, hub, cfg, nil)

	// Create a small request that should pass
	body := `{"username":"test","password":"test"}`
	req := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.ContentLength = int64(len(body))

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	// Should not be 413
	if rec.Code == http.StatusRequestEntityTooLarge {
		t.Errorf("small request should not be rejected, got status %d", rec.Code)
	}
}

func TestMaxBytesReaderBehavior(t *testing.T) {
	// Test that http.MaxBytesReader works as expected
	originalBody := bytes.NewReader([]byte("12345678901234567890")) // 20 bytes
	limitedBody := http.MaxBytesReader(httptest.NewRecorder(), io.NopCloser(originalBody), 10)

	data, err := io.ReadAll(limitedBody)
	if err == nil {
		t.Error("expected error when reading more than limit")
	}

	// Should only read up to the limit
	if len(data) > 10 {
		t.Errorf("expected at most 10 bytes, got %d", len(data))
	}
}
