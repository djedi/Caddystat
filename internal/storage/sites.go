package storage

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
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

// SitePermission represents a session's permission for a specific site.
type SitePermission struct {
	ID           int64     `json:"id"`
	SessionToken string    `json:"session_token"`
	SiteHost     string    `json:"site_host"` // "*" means all sites
	CreatedAt    time.Time `json:"created_at"`
}

// SessionPermissions represents the permissions granted to a session.
type SessionPermissions struct {
	SessionToken string   `json:"session_token"`
	AllSites     bool     `json:"all_sites"`     // true if user has access to all sites
	AllowedHosts []string `json:"allowed_hosts"` // list of specific hosts (empty if AllSites is true)
}

// migrateSites creates the sites and site_permissions tables if they don't exist.
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

CREATE TABLE IF NOT EXISTS site_permissions (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	session_token TEXT NOT NULL,
	site_host TEXT NOT NULL,
	created_at TIMESTAMP NOT NULL,
	FOREIGN KEY (session_token) REFERENCES sessions(token) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_site_permissions_token ON site_permissions(session_token);
CREATE INDEX IF NOT EXISTS idx_site_permissions_host ON site_permissions(site_host);
CREATE UNIQUE INDEX IF NOT EXISTS idx_site_permissions_unique ON site_permissions(session_token, site_host);
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

// SetSessionPermissions sets the site permissions for a session.
// Pass nil or empty slice for allSites access, or specific hosts for restricted access.
// If allowedHosts contains "*", the session gets access to all sites.
func (s *Storage) SetSessionPermissions(ctx context.Context, sessionToken string, allowedHosts []string) error {
	ctx, cancel := context.WithTimeout(ctx, s.queryTimeout)
	defer cancel()

	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	// Start transaction
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Delete existing permissions for this session
	_, err = tx.ExecContext(ctx, `DELETE FROM site_permissions WHERE session_token = ?`, sessionToken)
	if err != nil {
		return fmt.Errorf("delete existing permissions: %w", err)
	}

	// If no hosts specified or empty, grant all sites access
	if len(allowedHosts) == 0 {
		allowedHosts = []string{"*"}
	}

	// Insert new permissions
	now := time.Now()
	for _, host := range allowedHosts {
		host = strings.TrimSpace(host)
		if host == "" {
			continue
		}
		_, err = tx.ExecContext(ctx, `
			INSERT INTO site_permissions (session_token, site_host, created_at)
			VALUES (?, ?, ?)
		`, sessionToken, host, now)
		if err != nil {
			return fmt.Errorf("insert permission: %w", err)
		}
	}

	return tx.Commit()
}

// GetSessionPermissions returns the site permissions for a session.
func (s *Storage) GetSessionPermissions(ctx context.Context, sessionToken string) (*SessionPermissions, error) {
	ctx, cancel := context.WithTimeout(ctx, s.queryTimeout)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, `
		SELECT site_host FROM site_permissions WHERE session_token = ?
	`, sessionToken)
	if err != nil {
		return nil, fmt.Errorf("query permissions: %w", err)
	}
	defer rows.Close()

	perms := &SessionPermissions{
		SessionToken: sessionToken,
		AllowedHosts: make([]string, 0),
	}

	for rows.Next() {
		var host string
		if err := rows.Scan(&host); err != nil {
			return nil, fmt.Errorf("scan permission: %w", err)
		}
		if host == "*" {
			perms.AllSites = true
			perms.AllowedHosts = nil // Clear hosts when all sites is granted
			break
		}
		perms.AllowedHosts = append(perms.AllowedHosts, host)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate permissions: %w", err)
	}

	// If no permissions found, default to all sites for backward compatibility
	// (existing sessions without explicit permissions get full access)
	if !perms.AllSites && len(perms.AllowedHosts) == 0 {
		perms.AllSites = true
	}

	return perms, nil
}

// HasSitePermission checks if a session has permission to access a specific site/host.
// Returns true if the session has access to all sites or the specific host.
func (s *Storage) HasSitePermission(ctx context.Context, sessionToken string, host string) (bool, error) {
	perms, err := s.GetSessionPermissions(ctx, sessionToken)
	if err != nil {
		return false, err
	}

	if perms.AllSites {
		return true, nil
	}

	for _, allowedHost := range perms.AllowedHosts {
		if strings.EqualFold(allowedHost, host) {
			return true, nil
		}
	}

	return false, nil
}

// DeleteSessionPermissions deletes all permissions for a session.
func (s *Storage) DeleteSessionPermissions(ctx context.Context, sessionToken string) error {
	ctx, cancel := context.WithTimeout(ctx, s.queryTimeout)
	defer cancel()

	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	_, err := s.db.ExecContext(ctx, `DELETE FROM site_permissions WHERE session_token = ?`, sessionToken)
	if err != nil {
		return fmt.Errorf("delete permissions: %w", err)
	}
	return nil
}

// CleanupOrphanedPermissions removes permissions for sessions that no longer exist.
func (s *Storage) CleanupOrphanedPermissions(ctx context.Context) (int64, error) {
	ctx, cancel := context.WithTimeout(ctx, s.queryTimeout)
	defer cancel()

	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	result, err := s.db.ExecContext(ctx, `
		DELETE FROM site_permissions
		WHERE session_token NOT IN (SELECT token FROM sessions)
	`)
	if err != nil {
		return 0, fmt.Errorf("cleanup orphaned permissions: %w", err)
	}
	return result.RowsAffected()
}
