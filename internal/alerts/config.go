package alerts

import (
	"encoding/json"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

// LoadConfig loads alerting configuration from environment variables.
// Returns an empty config with Enabled=false if alerting is not configured.
func LoadConfig() Config {
	cfg := Config{
		Enabled:          getEnvBool("ALERT_ENABLED", false),
		EvaluateInterval: getEnvDuration("ALERT_EVALUATE_INTERVAL", time.Minute),
		Rules:            make([]Rule, 0),
		Channels:         make([]Channel, 0),
	}

	if !cfg.Enabled {
		return cfg
	}

	// Load rules from config file if specified
	if rulesPath := os.Getenv("ALERT_RULES_PATH"); rulesPath != "" {
		if rules, err := loadRulesFromFile(rulesPath); err != nil {
			slog.Warn("failed to load alert rules from file", "path", rulesPath, "error", err)
		} else {
			cfg.Rules = append(cfg.Rules, rules...)
		}
	}

	// Load default rules from environment variables
	cfg.Rules = append(cfg.Rules, loadEnvRules()...)

	// Load notification channels
	cfg.Channels = loadChannels()

	return cfg
}

func loadRulesFromFile(path string) ([]Rule, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var rules []Rule
	if err := json.Unmarshal(data, &rules); err != nil {
		return nil, err
	}

	return rules, nil
}

func loadEnvRules() []Rule {
	var rules []Rule

	// Error rate rule: ALERT_ERROR_RATE_THRESHOLD (percentage)
	if threshold := getEnvFloat("ALERT_ERROR_RATE_THRESHOLD", 0); threshold > 0 {
		rules = append(rules, Rule{
			Name:      "error_rate_default",
			Type:      AlertTypeErrorRate,
			Enabled:   true,
			Threshold: threshold,
			Duration:  getEnvDuration("ALERT_ERROR_RATE_DURATION", 5*time.Minute),
			Cooldown:  getEnvDuration("ALERT_ERROR_RATE_COOLDOWN", 15*time.Minute),
			Severity:  AlertSeverity(getEnv("ALERT_ERROR_RATE_SEVERITY", string(SeverityCritical))),
		})
	}

	// Traffic spike rule: ALERT_TRAFFIC_SPIKE_THRESHOLD (percentage increase)
	if threshold := getEnvFloat("ALERT_TRAFFIC_SPIKE_THRESHOLD", 0); threshold > 0 {
		rules = append(rules, Rule{
			Name:      "traffic_spike_default",
			Type:      AlertTypeTrafficSpike,
			Enabled:   true,
			Threshold: threshold,
			Duration:  getEnvDuration("ALERT_TRAFFIC_SPIKE_DURATION", 5*time.Minute),
			Cooldown:  getEnvDuration("ALERT_TRAFFIC_SPIKE_COOLDOWN", 15*time.Minute),
			Severity:  AlertSeverity(getEnv("ALERT_TRAFFIC_SPIKE_SEVERITY", string(SeverityWarning))),
		})
	}

	// Traffic drop rule: ALERT_TRAFFIC_DROP_THRESHOLD (percentage decrease)
	if threshold := getEnvFloat("ALERT_TRAFFIC_DROP_THRESHOLD", 0); threshold > 0 {
		rules = append(rules, Rule{
			Name:      "traffic_drop_default",
			Type:      AlertTypeTrafficDrop,
			Enabled:   true,
			Threshold: threshold,
			Duration:  getEnvDuration("ALERT_TRAFFIC_DROP_DURATION", 5*time.Minute),
			Cooldown:  getEnvDuration("ALERT_TRAFFIC_DROP_COOLDOWN", 15*time.Minute),
			Severity:  AlertSeverity(getEnv("ALERT_TRAFFIC_DROP_SEVERITY", string(SeverityWarning))),
		})
	}

	// 404 threshold rule: ALERT_404_THRESHOLD (count)
	if threshold := getEnvFloat("ALERT_404_THRESHOLD", 0); threshold > 0 {
		rules = append(rules, Rule{
			Name:        "status_404_default",
			Type:        AlertTypeStatusCode,
			Enabled:     true,
			Threshold:   threshold,
			StatusCodes: []int{404},
			Duration:    getEnvDuration("ALERT_404_DURATION", 5*time.Minute),
			Cooldown:    getEnvDuration("ALERT_404_COOLDOWN", 15*time.Minute),
			Severity:    AlertSeverity(getEnv("ALERT_404_SEVERITY", string(SeverityWarning))),
		})
	}

	return rules
}

func loadChannels() []Channel {
	var channels []Channel

	// Email channel
	if smtpHost := os.Getenv("ALERT_SMTP_HOST"); smtpHost != "" {
		emailTo := strings.Split(os.Getenv("ALERT_EMAIL_TO"), ",")
		for i := range emailTo {
			emailTo[i] = strings.TrimSpace(emailTo[i])
		}
		// Filter empty strings
		var validEmails []string
		for _, e := range emailTo {
			if e != "" {
				validEmails = append(validEmails, e)
			}
		}

		if len(validEmails) > 0 {
			channels = append(channels, Channel{
				Type:         ChannelTypeEmail,
				Enabled:      true,
				SMTPHost:     smtpHost,
				SMTPPort:     getEnvInt("ALERT_SMTP_PORT", 587),
				SMTPUsername: os.Getenv("ALERT_SMTP_USERNAME"),
				SMTPPassword: os.Getenv("ALERT_SMTP_PASSWORD"),
				SMTPFrom:     getEnv("ALERT_SMTP_FROM", "caddystat@localhost"),
				EmailTo:      validEmails,
			})
		}
	}

	// Webhook channel
	if webhookURL := os.Getenv("ALERT_WEBHOOK_URL"); webhookURL != "" {
		headers := make(map[string]string)
		if headerStr := os.Getenv("ALERT_WEBHOOK_HEADERS"); headerStr != "" {
			// Parse headers in format "Key1:Value1,Key2:Value2"
			for _, pair := range strings.Split(headerStr, ",") {
				parts := strings.SplitN(pair, ":", 2)
				if len(parts) == 2 {
					headers[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
				}
			}
		}

		channels = append(channels, Channel{
			Type:           ChannelTypeWebhook,
			Enabled:        true,
			WebhookURL:     webhookURL,
			WebhookMethod:  getEnv("ALERT_WEBHOOK_METHOD", "POST"),
			WebhookHeaders: headers,
		})
	}

	return channels
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
		return def
	}
	return parsed
}

func getEnvFloat(key string, def float64) float64 {
	val := os.Getenv(key)
	if val == "" {
		return def
	}
	parsed, err := strconv.ParseFloat(val, 64)
	if err != nil {
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
		return def
	}
	return parsed
}
