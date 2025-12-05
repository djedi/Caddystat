package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Site represents a configured site/host in Caddystat.
type Site struct {
	ID                int64      `json:"id"`
	Host              string     `json:"host"`
	DisplayName       string     `json:"display_name,omitempty"`
	RetentionDays     int        `json:"retention_days,omitempty"` // 0 means use global default
	Enabled           bool       `json:"enabled"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
	RequestCount      int64      `json:"request_count,omitempty"` // Populated when listing
	LastRequestAt     *time.Time `json:"last_request_at,omitempty"`
	BandwidthBytes    int64      `json:"bandwidth_bytes,omitempty"`
	UniqueVisitors24h int64      `json:"unique_visitors_24h,omitempty"`
}

// SiteInput represents the input for creating or updating a site.
type SiteInput struct {
	Host          string `json:"host"`
	DisplayName   string `json:"display_name,omitempty"`
	RetentionDays int    `json:"retention_days,omitempty"`
	Enabled       *bool  `json:"enabled,omitempty"` // nil means default to true for create
}

// SiteSummary provides a high-level overview of all sites.
type SiteSummary struct {
	TotalSites     int64  `json:"total_sites"`
	EnabledSites   int64  `json:"enabled_sites"`
	TotalRequests  int64  `json:"total_requests"`
	TotalBandwidth int64  `json:"total_bandwidth_bytes"`
	Sites          []Site `json:"sites"`
}

// migrateSites creates the sites table if it doesn't exist.
// Called from the main migrate function.
func (s *Storage) migrateSites() error {
	schema := `
CREATE TABLE IF NOT EXISTS sites (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	host TEXT NOT NULL UNIQUE,
	display_name TEXT DEFAULT '',
	retention_days INTEGER DEFAULT 0,
	enabled INTEGER DEFAULT 1,
	created_at TIMESTAMP NOT NULL,
	updated_at TIMESTAMP NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_sites_host ON sites(host);
CREATE INDEX IF NOT EXISTS idx_sites_enabled ON sites(enabled);
`
	_, err := s.db.Exec(schema)
	return err
}

// ListSites returns all configured sites with their request statistics.
func (s *Storage) ListSites(ctx context.Context) (*SiteSummary, error) {
	ctx, cancel := context.WithTimeout(ctx, s.queryTimeout)
	defer cancel()

	summary := &SiteSummary{
		Sites: make([]Site, 0),
	}

	// First, get configured sites
	configuredSites := make(map[string]*Site)
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, host, display_name, retention_days, enabled, created_at, updated_at
		FROM sites
		ORDER BY host
	`)
	if err != nil {
		return nil, fmt.Errorf("query sites: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var site Site
		if err := rows.Scan(
			&site.ID, &site.Host, &site.DisplayName, &site.RetentionDays,
			&site.Enabled, &site.CreatedAt, &site.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan site: %w", err)
		}
		configuredSites[site.Host] = &site
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate sites: %w", err)
	}

	// Get stats for all hosts (including unconfigured ones)
	from24h := time.Now().Add(-24 * time.Hour)
	statsRows, err := s.db.QueryContext(ctx, `
		SELECT
			host,
			COUNT(*) as request_count,
			MAX(ts) as last_request,
			IFNULL(SUM(bytes), 0) as bandwidth,
			COUNT(DISTINCT ip || '|' || COALESCE(user_agent, '')) as unique_visitors_24h
		FROM requests
		WHERE ts >= ?
		GROUP BY host
		ORDER BY request_count DESC
	`, from24h)
	if err != nil {
		return nil, fmt.Errorf("query host stats: %w", err)
	}
	defer statsRows.Close()

	seenHosts := make(map[string]bool)
	for statsRows.Next() {
		var host string
		var requestCount int64
		var lastRequestStr sql.NullString
		var bandwidth int64
		var uniqueVisitors int64

		if err := statsRows.Scan(&host, &requestCount, &lastRequestStr, &bandwidth, &uniqueVisitors); err != nil {
			return nil, fmt.Errorf("scan stats: %w", err)
		}

		seenHosts[host] = true
		summary.TotalRequests += requestCount
		summary.TotalBandwidth += bandwidth

		var lastReq *time.Time
		if lastRequestStr.Valid && lastRequestStr.String != "" {
			if t, err := time.Parse(time.RFC3339, lastRequestStr.String); err == nil {
				lastReq = &t
			} else if t, err := time.Parse("2006-01-02 15:04:05", lastRequestStr.String); err == nil {
				lastReq = &t
			}
		}

		if site, exists := configuredSites[host]; exists {
			// Update configured site with stats
			site.RequestCount = requestCount
			site.LastRequestAt = lastReq
			site.BandwidthBytes = bandwidth
			site.UniqueVisitors24h = uniqueVisitors
		} else {
			// Create an "unconfigured" site entry
			configuredSites[host] = &Site{
				ID:                0, // 0 indicates not configured
				Host:              host,
				Enabled:           true, // Unconfigured sites are implicitly enabled
				RequestCount:      requestCount,
				LastRequestAt:     lastReq,
				BandwidthBytes:    bandwidth,
				UniqueVisitors24h: uniqueVisitors,
			}
		}
	}
	if err := statsRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate stats: %w", err)
	}

	// Build the final list
	for _, site := range configuredSites {
		summary.Sites = append(summary.Sites, *site)
		summary.TotalSites++
		if site.Enabled {
			summary.EnabledSites++
		}
	}

	return summary, nil
}

// GetSite returns a single site by ID.
func (s *Storage) GetSite(ctx context.Context, id int64) (*Site, error) {
	ctx, cancel := context.WithTimeout(ctx, s.queryTimeout)
	defer cancel()

	var site Site
	err := s.db.QueryRowContext(ctx, `
		SELECT id, host, display_name, retention_days, enabled, created_at, updated_at
		FROM sites WHERE id = ?
	`, id).Scan(
		&site.ID, &site.Host, &site.DisplayName, &site.RetentionDays,
		&site.Enabled, &site.CreatedAt, &site.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query site: %w", err)
	}

	// Get stats for this host
	from24h := time.Now().Add(-24 * time.Hour)
	var lastRequest sql.NullTime
	err = s.db.QueryRowContext(ctx, `
		SELECT
			COUNT(*),
			MAX(ts),
			IFNULL(SUM(bytes), 0),
			COUNT(DISTINCT ip || '|' || COALESCE(user_agent, ''))
		FROM requests
		WHERE host = ? AND ts >= ?
	`, site.Host, from24h).Scan(&site.RequestCount, &lastRequest, &site.BandwidthBytes, &site.UniqueVisitors24h)
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("query site stats: %w", err)
	}
	if lastRequest.Valid {
		site.LastRequestAt = &lastRequest.Time
	}

	return &site, nil
}

// GetSiteByHost returns a site by its host name.
func (s *Storage) GetSiteByHost(ctx context.Context, host string) (*Site, error) {
	ctx, cancel := context.WithTimeout(ctx, s.queryTimeout)
	defer cancel()

	var site Site
	err := s.db.QueryRowContext(ctx, `
		SELECT id, host, display_name, retention_days, enabled, created_at, updated_at
		FROM sites WHERE host = ?
	`, host).Scan(
		&site.ID, &site.Host, &site.DisplayName, &site.RetentionDays,
		&site.Enabled, &site.CreatedAt, &site.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query site by host: %w", err)
	}
	return &site, nil
}

// CreateSite creates a new site configuration.
func (s *Storage) CreateSite(ctx context.Context, input SiteInput) (*Site, error) {
	ctx, cancel := context.WithTimeout(ctx, s.queryTimeout)
	defer cancel()

	if input.Host == "" {
		return nil, fmt.Errorf("host is required")
	}

	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}

	now := time.Now()
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO sites (host, display_name, retention_days, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, input.Host, input.DisplayName, input.RetentionDays, enabled, now, now)
	if err != nil {
		return nil, fmt.Errorf("insert site: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("get last insert id: %w", err)
	}

	return &Site{
		ID:            id,
		Host:          input.Host,
		DisplayName:   input.DisplayName,
		RetentionDays: input.RetentionDays,
		Enabled:       enabled,
		CreatedAt:     now,
		UpdatedAt:     now,
	}, nil
}

// UpdateSite updates an existing site configuration.
func (s *Storage) UpdateSite(ctx context.Context, id int64, input SiteInput) (*Site, error) {
	ctx, cancel := context.WithTimeout(ctx, s.queryTimeout)
	defer cancel()

	// First check if site exists
	existing, err := s.GetSite(ctx, id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, nil
	}

	// Build update with only provided fields
	host := existing.Host
	if input.Host != "" {
		host = input.Host
	}
	displayName := existing.DisplayName
	if input.DisplayName != "" {
		displayName = input.DisplayName
	}
	retentionDays := existing.RetentionDays
	if input.RetentionDays > 0 {
		retentionDays = input.RetentionDays
	}
	enabled := existing.Enabled
	if input.Enabled != nil {
		enabled = *input.Enabled
	}

	now := time.Now()
	_, err = s.db.ExecContext(ctx, `
		UPDATE sites
		SET host = ?, display_name = ?, retention_days = ?, enabled = ?, updated_at = ?
		WHERE id = ?
	`, host, displayName, retentionDays, enabled, now, id)
	if err != nil {
		return nil, fmt.Errorf("update site: %w", err)
	}

	return &Site{
		ID:            id,
		Host:          host,
		DisplayName:   displayName,
		RetentionDays: retentionDays,
		Enabled:       enabled,
		CreatedAt:     existing.CreatedAt,
		UpdatedAt:     now,
	}, nil
}

// DeleteSite removes a site configuration.
// Note: This does not delete the request data for the site.
func (s *Storage) DeleteSite(ctx context.Context, id int64) error {
	ctx, cancel := context.WithTimeout(ctx, s.queryTimeout)
	defer cancel()

	result, err := s.db.ExecContext(ctx, `DELETE FROM sites WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete site: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("site not found")
	}

	return nil
}

// GetSiteRetention returns the retention days for a specific host.
// Returns 0 if no specific retention is configured (use global default).
func (s *Storage) GetSiteRetention(ctx context.Context, host string) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, s.queryTimeout)
	defer cancel()

	var retentionDays int
	err := s.db.QueryRowContext(ctx, `
		SELECT retention_days FROM sites WHERE host = ? AND retention_days > 0
	`, host).Scan(&retentionDays)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("query retention: %w", err)
	}
	return retentionDays, nil
}

// IsSiteEnabled checks if a site is enabled.
// Returns true for unconfigured sites (enabled by default).
func (s *Storage) IsSiteEnabled(ctx context.Context, host string) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, s.queryTimeout)
	defer cancel()

	var enabled bool
	err := s.db.QueryRowContext(ctx, `
		SELECT enabled FROM sites WHERE host = ?
	`, host).Scan(&enabled)
	if err == sql.ErrNoRows {
		return true, nil // Unconfigured sites are enabled by default
	}
	if err != nil {
		return false, fmt.Errorf("query enabled: %w", err)
	}
	return enabled, nil
}
