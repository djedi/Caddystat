package config

import (
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/dustin/Caddystat/internal/logging"
)

type Config struct {
	LogPaths                []string
	ListenAddr              string
	DBPath                  string
	DataRetentionDays       int
	MaxMindDBPath           string
	PrivacyHashIPs          bool
	PrivacyHashSalt         string
	PrivacyAnonymizeOctet   bool
	RawRetentionHours       int
	AggregationInterval     time.Duration
	AggregationFlushSeconds int
	AuthUsername            string
	AuthPassword            string
	LogLevel                logging.Level
	RateLimitPerMinute      int
	MaxRequestBodyBytes     int64
	DBMaxConnections        int
	DBQueryTimeout          time.Duration
}

func Load() Config {
	cfg := Config{
		LogPaths:                splitEnv("LOG_PATH", []string{"./caddy.log"}),
		ListenAddr:              getEnv("LISTEN_ADDR", ":8404"),
		DBPath:                  getEnv("DB_PATH", "./data/caddystat.db"),
		DataRetentionDays:       getEnvInt("DATA_RETENTION_DAYS", 7),
		MaxMindDBPath:           os.Getenv("MAXMIND_DB_PATH"),
		PrivacyHashIPs:          getEnvBool("PRIVACY_HASH_IPS", false),
		PrivacyHashSalt:         getEnv("PRIVACY_HASH_SALT", "caddystat"),
		PrivacyAnonymizeOctet:   getEnvBool("PRIVACY_ANONYMIZE_LAST_OCTET", false),
		RawRetentionHours:       getEnvInt("RAW_RETENTION_HOURS", 48),
		AggregationInterval:     getEnvDuration("AGGREGATION_INTERVAL", time.Hour),
		AggregationFlushSeconds: getEnvInt("AGGREGATION_FLUSH_SECONDS", 10),
		AuthUsername:            os.Getenv("AUTH_USERNAME"),
		AuthPassword:            os.Getenv("AUTH_PASSWORD"),
		LogLevel:                logging.ParseLevel(getEnv("LOG_LEVEL", "INFO")),
		RateLimitPerMinute:      getEnvInt("RATE_LIMIT_PER_MINUTE", 0),
		MaxRequestBodyBytes:     getEnvInt64("MAX_REQUEST_BODY_BYTES", 1<<20), // 1MB default
		DBMaxConnections:        getEnvInt("DB_MAX_CONNECTIONS", 1),
		DBQueryTimeout:          getEnvDuration("DB_QUERY_TIMEOUT", 30*time.Second),
	}

	return cfg
}

func getEnvInt64(key string, def int64) int64 {
	val := os.Getenv(key)
	if val == "" {
		return def
	}
	parsed, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		slog.Warn("invalid int64 environment variable", "key", key, "value", val, "error", err)
		return def
	}
	return parsed
}

// AuthEnabled returns true if both AUTH_USERNAME and AUTH_PASSWORD are set.
func (c Config) AuthEnabled() bool {
	return c.AuthUsername != "" && c.AuthPassword != ""
}

func splitEnv(key string, def []string) []string {
	val := os.Getenv(key)
	if val == "" {
		return def
	}
	parts := strings.Split(val, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

func getEnv(key, def string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return def
}

func getEnvBool(key string, def bool) bool {
	val := os.Getenv(key)
	if val == "" {
		return def
	}
	parsed, err := strconv.ParseBool(val)
	if err != nil {
		slog.Warn("invalid bool environment variable", "key", key, "value", val, "error", err)
		return def
	}
	return parsed
}

func getEnvInt(key string, def int) int {
	val := os.Getenv(key)
	if val == "" {
		return def
	}
	parsed, err := strconv.Atoi(val)
	if err != nil {
		slog.Warn("invalid int environment variable", "key", key, "value", val, "error", err)
		return def
	}
	return parsed
}

func getEnvDuration(key string, def time.Duration) time.Duration {
	val := os.Getenv(key)
	if val == "" {
		return def
	}
	parsed, err := time.ParseDuration(val)
	if err != nil {
		slog.Warn("invalid duration environment variable", "key", key, "value", val, "error", err)
		return def
	}
	return parsed
}
