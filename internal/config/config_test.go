package config

import (
	"os"
	"testing"
	"time"

	"github.com/dustin/Caddystat/internal/logging"
)

func TestLoad_Defaults(t *testing.T) {
	// Clear relevant env vars
	envVars := []string{
		"LOG_PATH", "LISTEN_ADDR", "DB_PATH", "DATA_RETENTION_DAYS",
		"MAXMIND_DB_PATH", "PRIVACY_HASH_IPS", "PRIVACY_HASH_SALT",
		"PRIVACY_ANONYMIZE_LAST_OCTET", "RAW_RETENTION_HOURS",
		"AGGREGATION_INTERVAL", "AGGREGATION_FLUSH_SECONDS",
		"AUTH_USERNAME", "AUTH_PASSWORD", "LOG_LEVEL",
		"RATE_LIMIT_PER_MINUTE", "MAX_REQUEST_BODY_BYTES",
		"DB_MAX_CONNECTIONS", "DB_QUERY_TIMEOUT",
	}
	for _, v := range envVars {
		os.Unsetenv(v)
	}

	cfg := Load()

	if len(cfg.LogPaths) != 1 || cfg.LogPaths[0] != "./caddy.log" {
		t.Errorf("LogPaths = %v, want [./caddy.log]", cfg.LogPaths)
	}
	if cfg.ListenAddr != ":8404" {
		t.Errorf("ListenAddr = %q, want %q", cfg.ListenAddr, ":8404")
	}
	if cfg.DBPath != "./data/caddystat.db" {
		t.Errorf("DBPath = %q, want %q", cfg.DBPath, "./data/caddystat.db")
	}
	if cfg.DataRetentionDays != 7 {
		t.Errorf("DataRetentionDays = %d, want 7", cfg.DataRetentionDays)
	}
	if cfg.RawRetentionHours != 48 {
		t.Errorf("RawRetentionHours = %d, want 48", cfg.RawRetentionHours)
	}
	if cfg.DBMaxConnections != 1 {
		t.Errorf("DBMaxConnections = %d, want 1", cfg.DBMaxConnections)
	}
	if cfg.DBQueryTimeout != 30*time.Second {
		t.Errorf("DBQueryTimeout = %v, want %v", cfg.DBQueryTimeout, 30*time.Second)
	}
	if cfg.LogLevel != logging.LevelInfo {
		t.Errorf("LogLevel = %v, want INFO", cfg.LogLevel)
	}
}

func TestLoad_DBMaxConnections(t *testing.T) {
	os.Setenv("DB_MAX_CONNECTIONS", "10")
	defer os.Unsetenv("DB_MAX_CONNECTIONS")

	cfg := Load()

	if cfg.DBMaxConnections != 10 {
		t.Errorf("DBMaxConnections = %d, want 10", cfg.DBMaxConnections)
	}
}

func TestLoad_DBQueryTimeout(t *testing.T) {
	os.Setenv("DB_QUERY_TIMEOUT", "1m30s")
	defer os.Unsetenv("DB_QUERY_TIMEOUT")

	cfg := Load()

	if cfg.DBQueryTimeout != 90*time.Second {
		t.Errorf("DBQueryTimeout = %v, want %v", cfg.DBQueryTimeout, 90*time.Second)
	}
}

func TestLoad_InvalidDBMaxConnections(t *testing.T) {
	os.Setenv("DB_MAX_CONNECTIONS", "invalid")
	defer os.Unsetenv("DB_MAX_CONNECTIONS")

	cfg := Load()

	// Should use default on invalid value
	if cfg.DBMaxConnections != 1 {
		t.Errorf("DBMaxConnections = %d, want 1 (default)", cfg.DBMaxConnections)
	}
}

func TestLoad_InvalidDBQueryTimeout(t *testing.T) {
	os.Setenv("DB_QUERY_TIMEOUT", "not-a-duration")
	defer os.Unsetenv("DB_QUERY_TIMEOUT")

	cfg := Load()

	// Should use default on invalid value
	if cfg.DBQueryTimeout != 30*time.Second {
		t.Errorf("DBQueryTimeout = %v, want %v (default)", cfg.DBQueryTimeout, 30*time.Second)
	}
}

func TestAuthEnabled(t *testing.T) {
	tests := []struct {
		name     string
		username string
		password string
		want     bool
	}{
		{"both empty", "", "", false},
		{"only username", "admin", "", false},
		{"only password", "", "secret", false},
		{"both set", "admin", "secret", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("AUTH_USERNAME", tt.username)
			os.Setenv("AUTH_PASSWORD", tt.password)
			defer os.Unsetenv("AUTH_USERNAME")
			defer os.Unsetenv("AUTH_PASSWORD")

			cfg := Load()
			if got := cfg.AuthEnabled(); got != tt.want {
				t.Errorf("AuthEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLoad_MultipleLogPaths(t *testing.T) {
	os.Setenv("LOG_PATH", "/var/log/caddy1.log, /var/log/caddy2.log")
	defer os.Unsetenv("LOG_PATH")

	cfg := Load()

	if len(cfg.LogPaths) != 2 {
		t.Fatalf("len(LogPaths) = %d, want 2", len(cfg.LogPaths))
	}
	if cfg.LogPaths[0] != "/var/log/caddy1.log" {
		t.Errorf("LogPaths[0] = %q, want %q", cfg.LogPaths[0], "/var/log/caddy1.log")
	}
	if cfg.LogPaths[1] != "/var/log/caddy2.log" {
		t.Errorf("LogPaths[1] = %q, want %q", cfg.LogPaths[1], "/var/log/caddy2.log")
	}
}
