package storage

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// Storage provides database operations for Caddystat.
type Storage struct {
	db           *sql.DB
	writeMu      sync.Mutex
	queryTimeout time.Duration

	// Prepared statements for frequently-run queries
	stmtInsertRequest *sql.Stmt
	stmtInsertSession *sql.Stmt
	stmtGetSession    *sql.Stmt
	stmtDeleteSession *sql.Stmt
}

// Options configures the Storage instance.
type Options struct {
	MaxConnections int
	QueryTimeout   time.Duration
}

// New creates a new Storage instance with default options.
// For custom options, use NewWithOptions.
func New(dbPath string) (*Storage, error) {
	return NewWithOptions(dbPath, Options{
		MaxConnections: 1,
		QueryTimeout:   30 * time.Second,
	})
}

// NewWithOptions creates a new Storage instance with the given options.
func NewWithOptions(dbPath string, opts Options) (*Storage, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}
	db, err := sql.Open("sqlite", dbPath+"?_busy_timeout=30000&_journal_mode=WAL&_synchronous=NORMAL")
	if err != nil {
		return nil, err
	}

	// Configure connection pool
	maxConns := opts.MaxConnections
	if maxConns <= 0 {
		maxConns = 1
	}
	db.SetMaxOpenConns(maxConns)
	db.SetMaxIdleConns(maxConns)

	queryTimeout := opts.QueryTimeout
	if queryTimeout <= 0 {
		queryTimeout = 30 * time.Second
	}

	s := &Storage{
		db:           db,
		queryTimeout: queryTimeout,
	}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	if err := s.prepareStatements(); err != nil {
		db.Close()
		return nil, fmt.Errorf("prepare statements: %w", err)
	}
	return s, nil
}

func (s *Storage) migrate() error {
	// Run sites migration first
	if err := s.migrateSites(); err != nil {
		return fmt.Errorf("migrate sites: %w", err)
	}

	// First, create tables without the new columns (for compatibility with existing DBs)
	schema := `
CREATE TABLE IF NOT EXISTS requests (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	ts TIMESTAMP NOT NULL,
	host TEXT,
	path TEXT,
	status INTEGER,
	bytes INTEGER,
	ip TEXT,
	referrer TEXT,
	user_agent TEXT,
	resp_time_ms REAL,
	country TEXT,
	region TEXT,
	city TEXT
);

CREATE INDEX IF NOT EXISTS idx_requests_ts ON requests(ts);
CREATE INDEX IF NOT EXISTS idx_requests_host ON requests(host);
CREATE INDEX IF NOT EXISTS idx_requests_path ON requests(path);

CREATE TABLE IF NOT EXISTS rollups_hourly (
	bucket_start TIMESTAMP NOT NULL,
	host TEXT,
	path TEXT,
	requests INTEGER,
	bytes INTEGER,
	status_2xx INTEGER,
	status_3xx INTEGER,
	status_4xx INTEGER,
	status_5xx INTEGER,
	PRIMARY KEY(bucket_start, host, path)
);

CREATE TABLE IF NOT EXISTS rollups_daily (
	bucket_start TIMESTAMP NOT NULL,
	host TEXT,
	path TEXT,
	requests INTEGER,
	bytes INTEGER,
	status_2xx INTEGER,
	status_3xx INTEGER,
	status_4xx INTEGER,
	status_5xx INTEGER,
	PRIMARY KEY(bucket_start, host, path)
);

CREATE TABLE IF NOT EXISTS import_progress (
	file_path TEXT PRIMARY KEY,
	byte_offset INTEGER NOT NULL,
	file_size INTEGER NOT NULL,
	file_mtime INTEGER NOT NULL,
	updated_at TIMESTAMP NOT NULL
);

CREATE TABLE IF NOT EXISTS sessions (
	token TEXT PRIMARY KEY,
	expires_at TIMESTAMP NOT NULL,
	created_at TIMESTAMP NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_sessions_expires ON sessions(expires_at);

CREATE TABLE IF NOT EXISTS import_errors (
	file_path TEXT PRIMARY KEY,
	error_count INTEGER NOT NULL DEFAULT 0,
	last_error TEXT,
	last_error_at TIMESTAMP,
	updated_at TIMESTAMP NOT NULL
);
`
	if _, err := s.db.Exec(schema); err != nil {
		return err
	}

	// Add new columns to existing tables (migration for existing databases)
	// These run after table creation, so they work for both new and existing DBs
	migrations := []string{
		"ALTER TABLE requests ADD COLUMN browser TEXT DEFAULT ''",
		"ALTER TABLE requests ADD COLUMN browser_version TEXT DEFAULT ''",
		"ALTER TABLE requests ADD COLUMN os TEXT DEFAULT ''",
		"ALTER TABLE requests ADD COLUMN os_version TEXT DEFAULT ''",
		"ALTER TABLE requests ADD COLUMN device_type TEXT DEFAULT ''",
		"ALTER TABLE requests ADD COLUMN is_bot INTEGER DEFAULT 0",
		"ALTER TABLE requests ADD COLUMN bot_name TEXT DEFAULT ''",
		"ALTER TABLE requests ADD COLUMN bot_intent TEXT DEFAULT ''",
	}
	for _, m := range migrations {
		// Ignore errors - column may already exist
		_, _ = s.db.Exec(m)
	}

	// Create indexes after columns exist
	_, _ = s.db.Exec("CREATE INDEX IF NOT EXISTS idx_requests_ip ON requests(ip)")
	_, _ = s.db.Exec("CREATE INDEX IF NOT EXISTS idx_requests_is_bot ON requests(is_bot)")

	return nil
}

func (s *Storage) prepareStatements() error {
	var err error

	// Prepare insert request statement
	s.stmtInsertRequest, err = s.db.Prepare(`
INSERT INTO requests (ts, host, path, status, bytes, ip, referrer, user_agent, resp_time_ms, country, region, city, browser, browser_version, os, os_version, device_type, is_bot, bot_name, bot_intent)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`)
	if err != nil {
		return fmt.Errorf("prepare insert request: %w", err)
	}

	// Prepare session statements
	s.stmtInsertSession, err = s.db.Prepare(`INSERT INTO sessions (token, expires_at, created_at) VALUES (?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare insert session: %w", err)
	}

	s.stmtGetSession, err = s.db.Prepare(`SELECT token, expires_at, created_at FROM sessions WHERE token = ?`)
	if err != nil {
		return fmt.Errorf("prepare get session: %w", err)
	}

	s.stmtDeleteSession, err = s.db.Prepare(`DELETE FROM sessions WHERE token = ?`)
	if err != nil {
		return fmt.Errorf("prepare delete session: %w", err)
	}

	return nil
}

// Close closes the database connection and prepared statements.
func (s *Storage) Close() error {
	// Close prepared statements
	if s.stmtInsertRequest != nil {
		s.stmtInsertRequest.Close()
	}
	if s.stmtInsertSession != nil {
		s.stmtInsertSession.Close()
	}
	if s.stmtGetSession != nil {
		s.stmtGetSession.Close()
	}
	if s.stmtDeleteSession != nil {
		s.stmtDeleteSession.Close()
	}
	return s.db.Close()
}

// QueryTimeout returns the configured query timeout duration.
func (s *Storage) QueryTimeout() time.Duration {
	return s.queryTimeout
}
