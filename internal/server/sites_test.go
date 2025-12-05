package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/dustin/Caddystat/internal/storage"
)

func TestListSites(t *testing.T) {
	srv, cleanup := setupTestServerWithData(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/sites", nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp storage.SiteSummary
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Should include hosts from the test data (example.com, blog.example.com)
	if resp.TotalSites < 2 {
		t.Errorf("expected at least 2 sites, got %d", resp.TotalSites)
	}

	// All sites should be enabled by default (unconfigured sites)
	if resp.EnabledSites != resp.TotalSites {
		t.Errorf("expected all sites to be enabled, got %d/%d", resp.EnabledSites, resp.TotalSites)
	}

	// Should have some requests
	if resp.TotalRequests == 0 {
		t.Error("expected TotalRequests > 0")
	}
}

func TestCreateSite(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	// Get CSRF token first
	csrfReq := httptest.NewRequest(http.MethodGet, "/api/auth/check", nil)
	csrfW := httptest.NewRecorder()
	srv.ServeHTTP(csrfW, csrfReq)

	var csrfCookie *http.Cookie
	for _, c := range csrfW.Result().Cookies() {
		if c.Name == "caddystat_csrf" {
			csrfCookie = c
			break
		}
	}
	if csrfCookie == nil {
		t.Fatal("CSRF cookie not set")
	}

	body := `{"host": "newsite.com", "display_name": "New Site", "retention_days": 30}`
	req := httptest.NewRequest(http.MethodPost, "/api/sites", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", csrfCookie.Value)
	req.AddCookie(csrfCookie)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected status %d, got %d: %s", http.StatusCreated, w.Code, w.Body.String())
	}

	var site storage.Site
	if err := json.NewDecoder(w.Body).Decode(&site); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if site.Host != "newsite.com" {
		t.Errorf("expected host 'newsite.com', got %q", site.Host)
	}
	if site.DisplayName != "New Site" {
		t.Errorf("expected display_name 'New Site', got %q", site.DisplayName)
	}
	if site.RetentionDays != 30 {
		t.Errorf("expected retention_days 30, got %d", site.RetentionDays)
	}
	if !site.Enabled {
		t.Error("expected site to be enabled by default")
	}
	if site.ID == 0 {
		t.Error("expected ID to be set")
	}
}

func TestCreateSite_MissingHost(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	// Get CSRF token
	csrfReq := httptest.NewRequest(http.MethodGet, "/api/auth/check", nil)
	csrfW := httptest.NewRecorder()
	srv.ServeHTTP(csrfW, csrfReq)

	var csrfCookie *http.Cookie
	for _, c := range csrfW.Result().Cookies() {
		if c.Name == "caddystat_csrf" {
			csrfCookie = c
			break
		}
	}

	body := `{"display_name": "No Host"}`
	req := httptest.NewRequest(http.MethodPost, "/api/sites", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", csrfCookie.Value)
	req.AddCookie(csrfCookie)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestCreateSite_Duplicate(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	// Get CSRF token
	csrfReq := httptest.NewRequest(http.MethodGet, "/api/auth/check", nil)
	csrfW := httptest.NewRecorder()
	srv.ServeHTTP(csrfW, csrfReq)

	var csrfCookie *http.Cookie
	for _, c := range csrfW.Result().Cookies() {
		if c.Name == "caddystat_csrf" {
			csrfCookie = c
			break
		}
	}

	// Create first site
	body := `{"host": "duplicate.com"}`
	req := httptest.NewRequest(http.MethodPost, "/api/sites", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", csrfCookie.Value)
	req.AddCookie(csrfCookie)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("first create failed: %d", w.Code)
	}

	// Try to create duplicate
	req2 := httptest.NewRequest(http.MethodPost, "/api/sites", bytes.NewBufferString(body))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("X-CSRF-Token", csrfCookie.Value)
	req2.AddCookie(csrfCookie)
	w2 := httptest.NewRecorder()
	srv.ServeHTTP(w2, req2)

	if w2.Code != http.StatusConflict {
		t.Errorf("expected status %d for duplicate, got %d", http.StatusConflict, w2.Code)
	}
}

func TestGetSite(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	// Get CSRF token and create a site first
	csrfReq := httptest.NewRequest(http.MethodGet, "/api/auth/check", nil)
	csrfW := httptest.NewRecorder()
	srv.ServeHTTP(csrfW, csrfReq)

	var csrfCookie *http.Cookie
	for _, c := range csrfW.Result().Cookies() {
		if c.Name == "caddystat_csrf" {
			csrfCookie = c
			break
		}
	}

	body := `{"host": "getsite.com", "display_name": "Get Site Test"}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/sites", bytes.NewBufferString(body))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("X-CSRF-Token", csrfCookie.Value)
	createReq.AddCookie(csrfCookie)
	createW := httptest.NewRecorder()
	srv.ServeHTTP(createW, createReq)

	var createdSite storage.Site
	_ = json.NewDecoder(createW.Body).Decode(&createdSite)

	// Get the site
	req := httptest.NewRequest(http.MethodGet, "/api/sites/"+itoa(createdSite.ID), nil)
	req.Header.Set("X-CSRF-Token", csrfCookie.Value)
	req.AddCookie(csrfCookie)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var site storage.Site
	if err := json.NewDecoder(w.Body).Decode(&site); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if site.Host != "getsite.com" {
		t.Errorf("expected host 'getsite.com', got %q", site.Host)
	}
}

func TestGetSite_NotFound(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	// Get CSRF token
	csrfReq := httptest.NewRequest(http.MethodGet, "/api/auth/check", nil)
	csrfW := httptest.NewRecorder()
	srv.ServeHTTP(csrfW, csrfReq)

	var csrfCookie *http.Cookie
	for _, c := range csrfW.Result().Cookies() {
		if c.Name == "caddystat_csrf" {
			csrfCookie = c
			break
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/sites/99999", nil)
	req.Header.Set("X-CSRF-Token", csrfCookie.Value)
	req.AddCookie(csrfCookie)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestUpdateSite(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	// Get CSRF token and create a site first
	csrfReq := httptest.NewRequest(http.MethodGet, "/api/auth/check", nil)
	csrfW := httptest.NewRecorder()
	srv.ServeHTTP(csrfW, csrfReq)

	var csrfCookie *http.Cookie
	for _, c := range csrfW.Result().Cookies() {
		if c.Name == "caddystat_csrf" {
			csrfCookie = c
			break
		}
	}

	createBody := `{"host": "updatesite.com", "display_name": "Original Name"}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/sites", bytes.NewBufferString(createBody))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("X-CSRF-Token", csrfCookie.Value)
	createReq.AddCookie(csrfCookie)
	createW := httptest.NewRecorder()
	srv.ServeHTTP(createW, createReq)

	var createdSite storage.Site
	_ = json.NewDecoder(createW.Body).Decode(&createdSite)

	// Update the site
	updateBody := `{"display_name": "Updated Name", "retention_days": 14}`
	req := httptest.NewRequest(http.MethodPut, "/api/sites/"+itoa(createdSite.ID), bytes.NewBufferString(updateBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", csrfCookie.Value)
	req.AddCookie(csrfCookie)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	var site storage.Site
	if err := json.NewDecoder(w.Body).Decode(&site); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if site.DisplayName != "Updated Name" {
		t.Errorf("expected display_name 'Updated Name', got %q", site.DisplayName)
	}
	if site.RetentionDays != 14 {
		t.Errorf("expected retention_days 14, got %d", site.RetentionDays)
	}
	if site.Host != "updatesite.com" {
		t.Errorf("expected host to remain 'updatesite.com', got %q", site.Host)
	}
}

func TestDeleteSite(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	// Get CSRF token and create a site first
	csrfReq := httptest.NewRequest(http.MethodGet, "/api/auth/check", nil)
	csrfW := httptest.NewRecorder()
	srv.ServeHTTP(csrfW, csrfReq)

	var csrfCookie *http.Cookie
	for _, c := range csrfW.Result().Cookies() {
		if c.Name == "caddystat_csrf" {
			csrfCookie = c
			break
		}
	}

	createBody := `{"host": "deletesite.com"}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/sites", bytes.NewBufferString(createBody))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("X-CSRF-Token", csrfCookie.Value)
	createReq.AddCookie(csrfCookie)
	createW := httptest.NewRecorder()
	srv.ServeHTTP(createW, createReq)

	var createdSite storage.Site
	_ = json.NewDecoder(createW.Body).Decode(&createdSite)

	// Delete the site
	req := httptest.NewRequest(http.MethodDelete, "/api/sites/"+itoa(createdSite.ID), nil)
	req.Header.Set("X-CSRF-Token", csrfCookie.Value)
	req.AddCookie(csrfCookie)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected status %d, got %d", http.StatusNoContent, w.Code)
	}

	// Verify it's gone
	getReq := httptest.NewRequest(http.MethodGet, "/api/sites/"+itoa(createdSite.ID), nil)
	getReq.Header.Set("X-CSRF-Token", csrfCookie.Value)
	getReq.AddCookie(csrfCookie)
	getW := httptest.NewRecorder()
	srv.ServeHTTP(getW, getReq)

	if getW.Code != http.StatusNotFound {
		t.Errorf("expected status %d after delete, got %d", http.StatusNotFound, getW.Code)
	}
}

func TestDeleteSite_NotFound(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	// Get CSRF token
	csrfReq := httptest.NewRequest(http.MethodGet, "/api/auth/check", nil)
	csrfW := httptest.NewRecorder()
	srv.ServeHTTP(csrfW, csrfReq)

	var csrfCookie *http.Cookie
	for _, c := range csrfW.Result().Cookies() {
		if c.Name == "caddystat_csrf" {
			csrfCookie = c
			break
		}
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/sites/99999", nil)
	req.Header.Set("X-CSRF-Token", csrfCookie.Value)
	req.AddCookie(csrfCookie)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestSitesMethodNotAllowed(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	// Get CSRF token
	csrfReq := httptest.NewRequest(http.MethodGet, "/api/auth/check", nil)
	csrfW := httptest.NewRecorder()
	srv.ServeHTTP(csrfW, csrfReq)

	var csrfCookie *http.Cookie
	for _, c := range csrfW.Result().Cookies() {
		if c.Name == "caddystat_csrf" {
			csrfCookie = c
			break
		}
	}

	// PUT on /api/sites should not be allowed
	req := httptest.NewRequest(http.MethodPut, "/api/sites", nil)
	req.Header.Set("X-CSRF-Token", csrfCookie.Value)
	req.AddCookie(csrfCookie)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status %d, got %d", http.StatusMethodNotAllowed, w.Code)
	}
}

// Helper function to convert int64 to string
func itoa(n int64) string {
	return strconv.FormatInt(n, 10)
}
