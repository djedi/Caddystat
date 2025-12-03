package config

import (
	"log"
	"os"
	"strconv"
	"strings"
	"time"
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
	}

	return cfg
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
		log.Printf("invalid bool for %s: %v", key, err)
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
		log.Printf("invalid int for %s: %v", key, err)
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
		log.Printf("invalid duration for %s: %v", key, err)
		return def
	}
	return parsed
}
