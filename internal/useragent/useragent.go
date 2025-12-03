package useragent

import (
	"strings"

	"github.com/mssola/useragent"
)

// ParsedUA contains the parsed user-agent information
type ParsedUA struct {
	Browser        string
	BrowserVersion string
	OS             string
	OSVersion      string
	DeviceType     string // desktop, mobile, tablet, bot
	IsBot          bool
	BotName        string // name of the bot if IsBot is true
}

// knownBots maps bot signatures to their names
var knownBots = map[string]string{
	"googlebot":      "Googlebot",
	"bingbot":        "Bingbot",
	"yandexbot":      "YandexBot",
	"duckduckbot":    "DuckDuckBot",
	"baiduspider":    "Baiduspider",
	"facebookexternalhit": "Facebook",
	"facebot":        "Facebook",
	"twitterbot":     "Twitterbot",
	"linkedinbot":    "LinkedInBot",
	"slurp":          "Yahoo Slurp",
	"msnbot":         "MSNBot",
	"ahrefsbot":      "AhrefsBot",
	"semrushbot":     "SemrushBot",
	"dotbot":         "DotBot",
	"mj12bot":        "MJ12Bot",
	"petalbot":       "PetalBot",
	"sogou":          "Sogou",
	"exabot":         "Exabot",
	"ia_archiver":    "Alexa",
	"archive.org_bot": "Internet Archive",
	"applebot":       "Applebot",
	"gptbot":         "GPTBot",
	"claudebot":      "ClaudeBot",
	"anthropic":      "Anthropic",
	"chatgpt":        "ChatGPT",
	"bytespider":     "ByteSpider",
	"crawler":        "Unknown Crawler",
	"spider":         "Unknown Spider",
	"bot":            "Unknown Bot",
	"uptimerobot":    "UptimeRobot",
	"pingdom":        "Pingdom",
	"statuscake":     "StatusCake",
	"jetmon":         "Jetmon",
	"nbot":           "nbot",
	"nutch":          "Nutch",
	"ning":           "Ning",
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
		result.BotName = identifyBot(lowerUA)
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
	if ua.Mobile() {
		result.DeviceType = "mobile"
	} else if isTablet(lowerUA) {
		result.DeviceType = "tablet"
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

func identifyBot(lowerUA string) string {
	for signature, name := range knownBots {
		if strings.Contains(lowerUA, signature) {
			return name
		}
	}
	// Check for generic patterns
	if strings.Contains(lowerUA, "http://") || strings.Contains(lowerUA, "https://") {
		return "Unknown Bot"
	}
	return "Unknown Bot"
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
		return "macOS", extractVersion(osInfo)
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

func isTablet(lowerUA string) bool {
	tabletIndicators := []string{"ipad", "tablet", "kindle", "playbook", "silk"}
	for _, indicator := range tabletIndicators {
		if strings.Contains(lowerUA, indicator) {
			return true
		}
	}
	return false
}
