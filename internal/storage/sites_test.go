package storage

import (
	"context"
	"testing"
	"time"
)

func TestStorage_CreateSite(t *testing.T) {
	s, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Test creating a site
	input := SiteInput{
		Host:          "example.com",
		DisplayName:   "Example Site",
		RetentionDays: 30,
	}
	site, err := s.CreateSite(ctx, input)
	if err != nil {
		t.Fatalf("CreateSite() error = %v", err)
	}
	if site.ID == 0 {
		t.Error("CreateSite() did not set ID")
	}
	if site.Host != "example.com" {
		t.Errorf("CreateSite() host = %q, want %q", site.Host, "example.com")
	}
	if site.DisplayName != "Example Site" {
		t.Errorf("CreateSite() display_name = %q, want %q", site.DisplayName, "Example Site")
	}
	if site.RetentionDays != 30 {
		t.Errorf("CreateSite() retention_days = %d, want %d", site.RetentionDays, 30)
	}
	if !site.Enabled {
		t.Error("CreateSite() should default to enabled=true")
	}

	// Test creating with explicit enabled=false
	enabled := false
	input2 := SiteInput{
		Host:    "disabled.com",
		Enabled: &enabled,
	}
	site2, err := s.CreateSite(ctx, input2)
	if err != nil {
		t.Fatalf("CreateSite() error = %v", err)
	}
	if site2.Enabled {
		t.Error("CreateSite() should respect enabled=false")
	}
}

func TestStorage_CreateSite_EmptyHost(t *testing.T) {
	s, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	input := SiteInput{
		DisplayName: "No Host",
	}
	_, err := s.CreateSite(ctx, input)
	if err == nil {
		t.Error("CreateSite() should return error for empty host")
	}
}

func TestStorage_CreateSite_DuplicateHost(t *testing.T) {
	s, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	input := SiteInput{Host: "example.com"}

	_, err := s.CreateSite(ctx, input)
	if err != nil {
		t.Fatalf("CreateSite() first error = %v", err)
	}

	// Try to create duplicate - should fail
	_, err = s.CreateSite(ctx, input)
	if err == nil {
		t.Error("CreateSite() should return error for duplicate host")
	}
}

func TestStorage_GetSite(t *testing.T) {
	s, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	input := SiteInput{
		Host:          "example.com",
		DisplayName:   "Example",
		RetentionDays: 14,
	}
	created, err := s.CreateSite(ctx, input)
	if err != nil {
		t.Fatalf("CreateSite() error = %v", err)
	}

	site, err := s.GetSite(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetSite() error = %v", err)
	}
	if site == nil {
		t.Fatal("GetSite() returned nil")
	}
	if site.Host != "example.com" {
		t.Errorf("GetSite() host = %q, want %q", site.Host, "example.com")
	}
	if site.DisplayName != "Example" {
		t.Errorf("GetSite() display_name = %q, want %q", site.DisplayName, "Example")
	}
}

func TestStorage_GetSite_NotFound(t *testing.T) {
	s, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	site, err := s.GetSite(ctx, 99999)
	if err != nil {
		t.Fatalf("GetSite() error = %v", err)
	}
	if site != nil {
		t.Error("GetSite() should return nil for non-existent site")
	}
}

func TestStorage_GetSiteByHost(t *testing.T) {
	s, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	input := SiteInput{
		Host:        "test.example.com",
		DisplayName: "Test Site",
	}
	_, err := s.CreateSite(ctx, input)
	if err != nil {
		t.Fatalf("CreateSite() error = %v", err)
	}

	site, err := s.GetSiteByHost(ctx, "test.example.com")
	if err != nil {
		t.Fatalf("GetSiteByHost() error = %v", err)
	}
	if site == nil {
		t.Fatal("GetSiteByHost() returned nil")
	}
	if site.Host != "test.example.com" {
		t.Errorf("GetSiteByHost() host = %q, want %q", site.Host, "test.example.com")
	}

	// Test non-existent host
	site2, err := s.GetSiteByHost(ctx, "nonexistent.com")
	if err != nil {
		t.Fatalf("GetSiteByHost() error = %v", err)
	}
	if site2 != nil {
		t.Error("GetSiteByHost() should return nil for non-existent host")
	}
}

func TestStorage_UpdateSite(t *testing.T) {
	s, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	input := SiteInput{
		Host:          "example.com",
		DisplayName:   "Original",
		RetentionDays: 7,
	}
	created, err := s.CreateSite(ctx, input)
	if err != nil {
		t.Fatalf("CreateSite() error = %v", err)
	}

	// Update display name and retention
	updateInput := SiteInput{
		DisplayName:   "Updated Name",
		RetentionDays: 30,
	}
	updated, err := s.UpdateSite(ctx, created.ID, updateInput)
	if err != nil {
		t.Fatalf("UpdateSite() error = %v", err)
	}
	if updated == nil {
		t.Fatal("UpdateSite() returned nil")
	}
	if updated.DisplayName != "Updated Name" {
		t.Errorf("UpdateSite() display_name = %q, want %q", updated.DisplayName, "Updated Name")
	}
	if updated.RetentionDays != 30 {
		t.Errorf("UpdateSite() retention_days = %d, want %d", updated.RetentionDays, 30)
	}
	// Host should remain unchanged if not provided
	if updated.Host != "example.com" {
		t.Errorf("UpdateSite() host = %q, want %q", updated.Host, "example.com")
	}

	// Update enabled status
	enabled := false
	updateInput2 := SiteInput{Enabled: &enabled}
	updated2, err := s.UpdateSite(ctx, created.ID, updateInput2)
	if err != nil {
		t.Fatalf("UpdateSite() error = %v", err)
	}
	if updated2.Enabled {
		t.Error("UpdateSite() should update enabled status")
	}
}

func TestStorage_UpdateSite_NotFound(t *testing.T) {
	s, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	input := SiteInput{DisplayName: "Update"}
	updated, err := s.UpdateSite(ctx, 99999, input)
	if err != nil {
		t.Fatalf("UpdateSite() error = %v", err)
	}
	if updated != nil {
		t.Error("UpdateSite() should return nil for non-existent site")
	}
}

func TestStorage_DeleteSite(t *testing.T) {
	s, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	input := SiteInput{Host: "delete-me.com"}
	created, err := s.CreateSite(ctx, input)
	if err != nil {
		t.Fatalf("CreateSite() error = %v", err)
	}

	err = s.DeleteSite(ctx, created.ID)
	if err != nil {
		t.Fatalf("DeleteSite() error = %v", err)
	}

	// Verify it's gone
	site, err := s.GetSite(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetSite() error = %v", err)
	}
	if site != nil {
		t.Error("DeleteSite() did not delete the site")
	}
}

func TestStorage_DeleteSite_NotFound(t *testing.T) {
	s, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	err := s.DeleteSite(ctx, 99999)
	if err == nil {
		t.Error("DeleteSite() should return error for non-existent site")
	}
}

func TestStorage_ListSites(t *testing.T) {
	s, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create some sites
	sites := []SiteInput{
		{Host: "site1.com", DisplayName: "Site 1", RetentionDays: 7},
		{Host: "site2.com", DisplayName: "Site 2", RetentionDays: 14},
	}
	for _, input := range sites {
		if _, err := s.CreateSite(ctx, input); err != nil {
			t.Fatalf("CreateSite() error = %v", err)
		}
	}

	// Create a disabled site
	enabled := false
	if _, err := s.CreateSite(ctx, SiteInput{Host: "disabled.com", Enabled: &enabled}); err != nil {
		t.Fatalf("CreateSite() error = %v", err)
	}

	// Add some request data
	now := time.Now()
	for i := 0; i < 5; i++ {
		s.InsertRequest(ctx, RequestRecord{
			Timestamp: now.Add(-time.Duration(i) * time.Hour),
			Host:      "site1.com",
			Path:      "/test",
			Status:    200,
			Bytes:     1024,
			IP:        "1.2.3.4",
		})
	}
	for i := 0; i < 3; i++ {
		s.InsertRequest(ctx, RequestRecord{
			Timestamp: now.Add(-time.Duration(i) * time.Hour),
			Host:      "site2.com",
			Path:      "/test",
			Status:    200,
			Bytes:     2048,
			IP:        "1.2.3.5",
		})
	}

	summary, err := s.ListSites(ctx)
	if err != nil {
		t.Fatalf("ListSites() error = %v", err)
	}

	if summary.TotalSites != 3 {
		t.Errorf("ListSites() total_sites = %d, want %d", summary.TotalSites, 3)
	}
	if summary.EnabledSites != 2 {
		t.Errorf("ListSites() enabled_sites = %d, want %d", summary.EnabledSites, 2)
	}

	// Check that stats were populated
	var site1Found, site2Found bool
	for _, site := range summary.Sites {
		if site.Host == "site1.com" {
			site1Found = true
			if site.RequestCount != 5 {
				t.Errorf("site1 request_count = %d, want %d", site.RequestCount, 5)
			}
		}
		if site.Host == "site2.com" {
			site2Found = true
			if site.RequestCount != 3 {
				t.Errorf("site2 request_count = %d, want %d", site.RequestCount, 3)
			}
		}
	}
	if !site1Found {
		t.Error("site1.com not found in ListSites")
	}
	if !site2Found {
		t.Error("site2.com not found in ListSites")
	}
}

func TestStorage_ListSites_IncludesUnconfiguredHosts(t *testing.T) {
	s, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Add requests for a host that has no site configuration
	now := time.Now()
	for i := 0; i < 3; i++ {
		s.InsertRequest(ctx, RequestRecord{
			Timestamp: now.Add(-time.Duration(i) * time.Hour),
			Host:      "unconfigured.com",
			Path:      "/test",
			Status:    200,
			Bytes:     512,
			IP:        "1.2.3.4",
		})
	}

	summary, err := s.ListSites(ctx)
	if err != nil {
		t.Fatalf("ListSites() error = %v", err)
	}

	// Should include the unconfigured host
	var found bool
	for _, site := range summary.Sites {
		if site.Host == "unconfigured.com" {
			found = true
			if site.ID != 0 {
				t.Error("unconfigured site should have ID = 0")
			}
			if !site.Enabled {
				t.Error("unconfigured site should be enabled by default")
			}
			if site.RequestCount != 3 {
				t.Errorf("unconfigured site request_count = %d, want %d", site.RequestCount, 3)
			}
		}
	}
	if !found {
		t.Error("unconfigured.com not found in ListSites")
	}
}

func TestStorage_GetSiteRetention(t *testing.T) {
	s, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create site with custom retention
	if _, err := s.CreateSite(ctx, SiteInput{Host: "custom.com", RetentionDays: 30}); err != nil {
		t.Fatalf("CreateSite() error = %v", err)
	}

	// Create site with no retention (use global default)
	if _, err := s.CreateSite(ctx, SiteInput{Host: "default.com", RetentionDays: 0}); err != nil {
		t.Fatalf("CreateSite() error = %v", err)
	}

	// Test custom retention
	ret, err := s.GetSiteRetention(ctx, "custom.com")
	if err != nil {
		t.Fatalf("GetSiteRetention() error = %v", err)
	}
	if ret != 30 {
		t.Errorf("GetSiteRetention() = %d, want %d", ret, 30)
	}

	// Test default retention (0)
	ret, err = s.GetSiteRetention(ctx, "default.com")
	if err != nil {
		t.Fatalf("GetSiteRetention() error = %v", err)
	}
	if ret != 0 {
		t.Errorf("GetSiteRetention() = %d, want %d", ret, 0)
	}

	// Test non-existent site (should return 0)
	ret, err = s.GetSiteRetention(ctx, "nonexistent.com")
	if err != nil {
		t.Fatalf("GetSiteRetention() error = %v", err)
	}
	if ret != 0 {
		t.Errorf("GetSiteRetention() = %d, want %d", ret, 0)
	}
}

func TestStorage_IsSiteEnabled(t *testing.T) {
	s, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create enabled site
	if _, err := s.CreateSite(ctx, SiteInput{Host: "enabled.com"}); err != nil {
		t.Fatalf("CreateSite() error = %v", err)
	}

	// Create disabled site
	enabled := false
	if _, err := s.CreateSite(ctx, SiteInput{Host: "disabled.com", Enabled: &enabled}); err != nil {
		t.Fatalf("CreateSite() error = %v", err)
	}

	// Test enabled site
	isEnabled, err := s.IsSiteEnabled(ctx, "enabled.com")
	if err != nil {
		t.Fatalf("IsSiteEnabled() error = %v", err)
	}
	if !isEnabled {
		t.Error("IsSiteEnabled() = false, want true for enabled site")
	}

	// Test disabled site
	isEnabled, err = s.IsSiteEnabled(ctx, "disabled.com")
	if err != nil {
		t.Fatalf("IsSiteEnabled() error = %v", err)
	}
	if isEnabled {
		t.Error("IsSiteEnabled() = true, want false for disabled site")
	}

	// Test unconfigured site (should be enabled by default)
	isEnabled, err = s.IsSiteEnabled(ctx, "unconfigured.com")
	if err != nil {
		t.Fatalf("IsSiteEnabled() error = %v", err)
	}
	if !isEnabled {
		t.Error("IsSiteEnabled() = false, want true for unconfigured site")
	}
}
