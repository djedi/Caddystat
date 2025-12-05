package useragent

import (
	"encoding/json"
	"log/slog"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/mssola/useragent"
)

// BotIntent represents the purpose/category of a bot
type BotIntent string

const (
	IntentSEO        BotIntent = "seo"        // Search engine crawlers
	IntentSocial     BotIntent = "social"     // Social media preview bots
	IntentMonitoring BotIntent = "monitoring" // Uptime/health monitoring
	IntentAI         BotIntent = "ai"         // AI/ML training crawlers
	IntentArchiver   BotIntent = "archiver"   // Web archiving bots
	IntentUnknown    BotIntent = "unknown"    // Unknown intent
)

// ParsedUA contains the parsed user-agent information
type ParsedUA struct {
	Browser        string
	BrowserVersion string
	OS             string
	OSVersion      string
	DeviceType     string // desktop, mobile, tablet, bot
	IsBot          bool
	BotName        string    // name of the bot if IsBot is true
	BotIntent      BotIntent // intent/category of the bot (seo, social, monitoring, ai, archiver, unknown)
}

// BotSignature represents a single bot signature with its metadata
type BotSignature struct {
	Signature string    `json:"signature"` // Substring to match (case-insensitive)
	Name      string    `json:"name"`      // Display name for the bot
	Intent    BotIntent `json:"intent"`    // Bot intent category
}

// BotSignaturesFile represents the JSON file format for bot signatures
type BotSignaturesFile struct {
	Version     string         `json:"version"`
	Description string         `json:"description"`
	Bots        []BotSignature `json:"bots"`
}

// botRegistry holds the loaded bot signatures
type botRegistry struct {
	mu         sync.RWMutex
	signatures []BotSignature // Sorted by signature length (longest first) for priority matching
}

var registry = &botRegistry{
	signatures: defaultBotSignatures(),
}

// defaultBotSignatures returns the built-in bot signatures
func defaultBotSignatures() []BotSignature {
	sigs := []BotSignature{
		// SEO crawlers
		{Signature: "googlebot", Name: "Googlebot", Intent: IntentSEO},
		{Signature: "bingbot", Name: "Bingbot", Intent: IntentSEO},
		{Signature: "yandexbot", Name: "YandexBot", Intent: IntentSEO},
		{Signature: "duckduckbot", Name: "DuckDuckBot", Intent: IntentSEO},
		{Signature: "baiduspider", Name: "Baiduspider", Intent: IntentSEO},
		{Signature: "slurp", Name: "Yahoo Slurp", Intent: IntentSEO},
		{Signature: "msnbot", Name: "MSNBot", Intent: IntentSEO},
		{Signature: "ahrefsbot", Name: "AhrefsBot", Intent: IntentSEO},
		{Signature: "semrushbot", Name: "SemrushBot", Intent: IntentSEO},
		{Signature: "dotbot", Name: "DotBot", Intent: IntentSEO},
		{Signature: "mj12bot", Name: "MJ12Bot", Intent: IntentSEO},
		{Signature: "petalbot", Name: "PetalBot", Intent: IntentSEO},
		{Signature: "sogou", Name: "Sogou", Intent: IntentSEO},
		{Signature: "exabot", Name: "Exabot", Intent: IntentSEO},
		{Signature: "ia_archiver", Name: "Alexa", Intent: IntentSEO},
		{Signature: "applebot", Name: "Applebot", Intent: IntentSEO},
		{Signature: "nutch", Name: "Nutch", Intent: IntentSEO},

		// Social media bots
		{Signature: "facebookexternalhit", Name: "Facebook", Intent: IntentSocial},
		{Signature: "facebot", Name: "Facebook", Intent: IntentSocial},
		{Signature: "twitterbot", Name: "Twitterbot", Intent: IntentSocial},
		{Signature: "linkedinbot", Name: "LinkedInBot", Intent: IntentSocial},

		// AI/ML crawlers
		{Signature: "gptbot", Name: "GPTBot", Intent: IntentAI},
		{Signature: "claudebot", Name: "ClaudeBot", Intent: IntentAI},
		{Signature: "anthropic", Name: "Anthropic", Intent: IntentAI},
		{Signature: "chatgpt", Name: "ChatGPT", Intent: IntentAI},
		{Signature: "bytespider", Name: "ByteSpider", Intent: IntentAI},

		// Monitoring bots
		{Signature: "uptimerobot", Name: "UptimeRobot", Intent: IntentMonitoring},
		{Signature: "pingdom", Name: "Pingdom", Intent: IntentMonitoring},
		{Signature: "statuscake", Name: "StatusCake", Intent: IntentMonitoring},
		{Signature: "jetmon", Name: "Jetmon", Intent: IntentMonitoring},

		// Archiver bots
		{Signature: "archive.org_bot", Name: "Internet Archive", Intent: IntentArchiver},

		// Unknown/generic patterns (should be last due to priority matching)
		{Signature: "nbot", Name: "nbot", Intent: IntentUnknown},
		{Signature: "ning", Name: "Ning", Intent: IntentUnknown},
		{Signature: "crawler", Name: "Unknown Crawler", Intent: IntentUnknown},
		{Signature: "spider", Name: "Unknown Spider", Intent: IntentUnknown},
		{Signature: "bot", Name: "Unknown Bot", Intent: IntentUnknown},
	}
	sortSignatures(sigs)
	return sigs
}

// sortSignatures sorts signatures by length (longest first) for priority matching
func sortSignatures(sigs []BotSignature) {
	sort.Slice(sigs, func(i, j int) bool {
		return len(sigs[i].Signature) > len(sigs[j].Signature)
	})
}

// LoadBotSignatures loads bot signatures from a JSON file.
// If the file doesn't exist or can't be parsed, it falls back to defaults.
func LoadBotSignatures(path string) error {
	if path == "" {
		return nil // No file specified, use defaults
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			slog.Debug("bot signatures file not found, using defaults", "path", path)
			return nil
		}
		return err
	}

	var file BotSignaturesFile
	if err := json.Unmarshal(data, &file); err != nil {
		return err
	}

	if len(file.Bots) == 0 {
		slog.Warn("bot signatures file is empty, using defaults", "path", path)
		return nil
	}

	// Validate and normalize signatures
	sigs := make([]BotSignature, 0, len(file.Bots))
	for _, bot := range file.Bots {
		if bot.Signature == "" || bot.Name == "" {
			slog.Warn("skipping invalid bot signature", "signature", bot.Signature, "name", bot.Name)
			continue
		}
		// Normalize signature to lowercase for case-insensitive matching
		sigs = append(sigs, BotSignature{
			Signature: strings.ToLower(bot.Signature),
			Name:      bot.Name,
			Intent:    normalizeIntent(bot.Intent),
		})
	}

	sortSignatures(sigs)

	registry.mu.Lock()
	registry.signatures = sigs
	registry.mu.Unlock()

	slog.Info("loaded bot signatures", "path", path, "count", len(sigs))
	return nil
}

// LoadBotSignaturesList loads and merges bot signatures from multiple JSON files.
// Files are processed in order, with later files overriding earlier ones for
// duplicate signatures. The default built-in signatures are used as a base.
// Empty paths are skipped.
func LoadBotSignaturesList(paths []string) error {
	if len(paths) == 0 {
		return nil // No files specified, use defaults
	}

	// Filter out empty paths
	var validPaths []string
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p != "" {
			validPaths = append(validPaths, p)
		}
	}

	if len(validPaths) == 0 {
		return nil // No valid paths, use defaults
	}

	// Start with defaults as base
	sigMap := make(map[string]BotSignature)
	for _, sig := range defaultBotSignatures() {
		sigMap[sig.Signature] = sig
	}

	totalLoaded := 0
	for _, path := range validPaths {
		loaded, err := loadAndMergeSignatures(path, sigMap)
		if err != nil {
			slog.Warn("failed to load bot signatures file", "path", path, "error", err)
			continue
		}
		totalLoaded += loaded
	}

	// Convert map back to slice
	sigs := make([]BotSignature, 0, len(sigMap))
	for _, sig := range sigMap {
		sigs = append(sigs, sig)
	}
	sortSignatures(sigs)

	registry.mu.Lock()
	registry.signatures = sigs
	registry.mu.Unlock()

	slog.Info("loaded bot signatures from multiple files",
		"files", len(validPaths),
		"signatures_loaded", totalLoaded,
		"total_signatures", len(sigs))
	return nil
}

// loadAndMergeSignatures loads signatures from a file and merges them into the map.
// Returns the number of signatures loaded from this file.
func loadAndMergeSignatures(path string, sigMap map[string]BotSignature) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			slog.Debug("bot signatures file not found", "path", path)
			return 0, nil
		}
		return 0, err
	}

	var file BotSignaturesFile
	if err := json.Unmarshal(data, &file); err != nil {
		return 0, err
	}

	if len(file.Bots) == 0 {
		slog.Debug("bot signatures file is empty", "path", path)
		return 0, nil
	}

	loaded := 0
	for _, bot := range file.Bots {
		if bot.Signature == "" || bot.Name == "" {
			slog.Warn("skipping invalid bot signature", "signature", bot.Signature, "name", bot.Name, "path", path)
			continue
		}
		// Normalize signature to lowercase for case-insensitive matching
		sig := BotSignature{
			Signature: strings.ToLower(bot.Signature),
			Name:      bot.Name,
			Intent:    normalizeIntent(bot.Intent),
		}
		sigMap[sig.Signature] = sig
		loaded++
	}

	slog.Info("loaded bot signatures", "path", path, "count", loaded)
	return loaded, nil
}

// normalizeIntent ensures the intent is a valid value
func normalizeIntent(intent BotIntent) BotIntent {
	switch intent {
	case IntentSEO, IntentSocial, IntentMonitoring, IntentAI, IntentArchiver:
		return intent
	default:
		return IntentUnknown
	}
}

// GetBotSignatures returns a copy of the current bot signatures
func GetBotSignatures() []BotSignature {
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	sigs := make([]BotSignature, len(registry.signatures))
	copy(sigs, registry.signatures)
	return sigs
}

// ResetBotSignatures resets the bot signatures to defaults (useful for testing)
func ResetBotSignatures() {
	registry.mu.Lock()
	registry.signatures = defaultBotSignatures()
	registry.mu.Unlock()
}

// Parse parses a user-agent string and extracts browser, OS, and device info
func Parse(uaString string) ParsedUA {
	if uaString == "" {
		return ParsedUA{
			Browser:    "Unknown",
			OS:         "Unknown",
			DeviceType: "unknown",
		}
	}

	ua := useragent.New(uaString)
	result := ParsedUA{}

	// Check if it's a bot first
	result.IsBot = ua.Bot()
	lowerUA := strings.ToLower(uaString)

	// Try to identify specific bots
	if result.IsBot || containsAny(lowerUA, "bot", "crawler", "spider", "crawl", "slurp", "archiver") {
		result.IsBot = true
		result.DeviceType = "bot"
		result.BotName, result.BotIntent = identifyBot(lowerUA)
		result.Browser = result.BotName
		result.OS = "Bot"
		return result
	}

	// Get browser info
	browserName, browserVersion := ua.Browser()
	result.Browser = normalizeBrowserName(browserName)
	result.BrowserVersion = browserVersion

	// Get OS info
	osInfo := ua.OS()
	result.OS, result.OSVersion = parseOS(osInfo, lowerUA)

	// Determine device type
	// Check tablet first because tablets may also be flagged as mobile by the library
	if isTablet(lowerUA) {
		result.DeviceType = "tablet"
	} else if ua.Mobile() {
		result.DeviceType = "mobile"
	} else {
		result.DeviceType = "desktop"
	}

	// Handle empty/unknown values
	if result.Browser == "" {
		result.Browser = "Unknown"
	}
	if result.OS == "" {
		result.OS = "Unknown"
	}
	if result.DeviceType == "" {
		result.DeviceType = "unknown"
	}

	return result
}

func containsAny(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// identifyBot returns the bot name and intent for a given user-agent string
func identifyBot(lowerUA string) (string, BotIntent) {
	registry.mu.RLock()
	defer registry.mu.RUnlock()

	// Signatures are sorted by length (longest first), so more specific matches take priority
	for _, sig := range registry.signatures {
		if strings.Contains(lowerUA, sig.Signature) {
			return sig.Name, sig.Intent
		}
	}
	// Check for generic patterns (URLs often indicate bots)
	if strings.Contains(lowerUA, "http://") || strings.Contains(lowerUA, "https://") {
		return "Unknown Bot", IntentUnknown
	}
	return "Unknown Bot", IntentUnknown
}

func normalizeBrowserName(name string) string {
	switch strings.ToLower(name) {
	case "chrome", "google chrome":
		return "Chrome"
	case "firefox", "mozilla firefox":
		return "Firefox"
	case "safari", "mobile safari":
		return "Safari"
	case "edge", "microsoft edge":
		return "Edge"
	case "opera", "opera mini":
		return "Opera"
	case "ie", "internet explorer", "msie":
		return "Internet Explorer"
	case "samsung browser", "samsungbrowser":
		return "Samsung Browser"
	case "brave":
		return "Brave"
	case "vivaldi":
		return "Vivaldi"
	case "":
		return "Unknown"
	default:
		return name
	}
}

func parseOS(osInfo, lowerUA string) (string, string) {
	osLower := strings.ToLower(osInfo)

	// iOS detection
	if strings.Contains(lowerUA, "iphone") || strings.Contains(lowerUA, "ipad") || strings.Contains(osLower, "ios") {
		return "iOS", extractVersion(osInfo)
	}

	// Android detection
	if strings.Contains(osLower, "android") {
		return "Android", extractVersion(osInfo)
	}

	// Windows detection
	if strings.Contains(osLower, "windows") {
		return "Windows", extractWindowsVersion(osInfo)
	}

	// macOS detection
	if strings.Contains(osLower, "mac os") || strings.Contains(osLower, "macos") || strings.Contains(lowerUA, "macintosh") {
		return "macOS", extractMacOSVersion(osInfo, lowerUA)
	}

	// Linux detection
	if strings.Contains(osLower, "linux") {
		if strings.Contains(lowerUA, "ubuntu") {
			return "Ubuntu", ""
		}
		if strings.Contains(lowerUA, "debian") {
			return "Debian", ""
		}
		if strings.Contains(lowerUA, "fedora") {
			return "Fedora", ""
		}
		return "Linux", ""
	}

	// Chrome OS
	if strings.Contains(osLower, "cros") || strings.Contains(lowerUA, "chromeos") {
		return "Chrome OS", ""
	}

	if osInfo == "" {
		return "Unknown", ""
	}

	return osInfo, ""
}

func extractVersion(osInfo string) string {
	// Simple version extraction - looks for patterns like "10.15" or "14.0"
	parts := strings.Fields(osInfo)
	for _, part := range parts {
		if len(part) > 0 && (part[0] >= '0' && part[0] <= '9') {
			return strings.TrimRight(part, ";)")
		}
	}
	return ""
}

func extractWindowsVersion(osInfo string) string {
	osLower := strings.ToLower(osInfo)
	if strings.Contains(osLower, "11") {
		return "11"
	}
	if strings.Contains(osLower, "10") {
		return "10"
	}
	if strings.Contains(osLower, "8.1") {
		return "8.1"
	}
	if strings.Contains(osLower, "8") {
		return "8"
	}
	if strings.Contains(osLower, "7") {
		return "7"
	}
	if strings.Contains(osLower, "vista") {
		return "Vista"
	}
	if strings.Contains(osLower, "xp") {
		return "XP"
	}
	return ""
}

// extractMacOSVersion extracts the macOS version from user agent strings.
// Handles formats like:
//   - "Mac OS X 10_15_7" or "Mac OS X 10.15.7"
//   - "Mac OS X 14_1" (macOS Sonoma)
//   - "Intel Mac OS X 10_15_7"
func extractMacOSVersion(osInfo, lowerUA string) string {
	// Try to find version pattern in osInfo first (from library parsing)
	if ver := extractMacOSVersionFromString(osInfo); ver != "" {
		return ver
	}
	// Fall back to parsing the raw user agent
	if ver := extractMacOSVersionFromString(lowerUA); ver != "" {
		return ver
	}
	return ""
}

// extractMacOSVersionFromString extracts macOS version from a string.
// Looks for patterns like "10_15_7", "10.15.7", "14_1", "14.1"
func extractMacOSVersionFromString(s string) string {
	// Replace underscores with dots for uniform processing
	s = strings.ReplaceAll(s, "_", ".")
	sLower := strings.ToLower(s)

	// Find "mac os x" or "macos" followed by version
	patterns := []string{"mac os x ", "macos ", "macos/"}
	for _, pattern := range patterns {
		if idx := strings.Index(sLower, pattern); idx >= 0 {
			rest := s[idx+len(pattern):]
			if ver := extractVersionNumber(rest); ver != "" {
				return ver
			}
		}
	}

	// Try extracting version from Intel/ARM prefix patterns
	// e.g., "Intel Mac OS X 10.15.7" or "ARM Mac OS X 14.0"
	if strings.Contains(sLower, "intel mac os x") || strings.Contains(sLower, "arm mac os x") {
		for _, prefix := range []string{"intel mac os x ", "arm mac os x "} {
			if idx := strings.Index(sLower, prefix); idx >= 0 {
				rest := s[idx+len(prefix):]
				if ver := extractVersionNumber(rest); ver != "" {
					return ver
				}
			}
		}
	}

	return ""
}

// extractVersionNumber extracts a version number from the start of a string.
// Returns versions like "10.15.7", "14.1", "14"
func extractVersionNumber(s string) string {
	s = strings.TrimSpace(s)
	if len(s) == 0 {
		return ""
	}

	// Must start with a digit
	if s[0] < '0' || s[0] > '9' {
		return ""
	}

	// Extract version: digits and dots only
	end := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= '0' && c <= '9') || c == '.' {
			end = i + 1
		} else {
			break
		}
	}

	ver := strings.TrimRight(s[:end], ".")
	return ver
}

func isTablet(lowerUA string) bool {
	tabletIndicators := []string{"ipad", "tablet", "kindle", "playbook", "silk"}
	for _, indicator := range tabletIndicators {
		if strings.Contains(lowerUA, indicator) {
			return true
		}
	}
	return false
}
