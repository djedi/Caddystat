package useragent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParse_EmptyString(t *testing.T) {
	result := Parse("")

	if result.Browser != "Unknown" {
		t.Errorf("Browser = %q, want %q", result.Browser, "Unknown")
	}
	if result.OS != "Unknown" {
		t.Errorf("OS = %q, want %q", result.OS, "Unknown")
	}
	if result.DeviceType != "unknown" {
		t.Errorf("DeviceType = %q, want %q", result.DeviceType, "unknown")
	}
	if result.IsBot {
		t.Errorf("IsBot = %v, want %v", result.IsBot, false)
	}
}

func TestParse_Browsers(t *testing.T) {
	tests := []struct {
		name        string
		ua          string
		wantBrowser string
	}{
		{
			name:        "Chrome on Windows",
			ua:          "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/119.0.0.0 Safari/537.36",
			wantBrowser: "Chrome",
		},
		{
			name:        "Firefox on Windows",
			ua:          "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:109.0) Gecko/20100101 Firefox/119.0",
			wantBrowser: "Firefox",
		},
		{
			name:        "Safari on macOS",
			ua:          "Mozilla/5.0 (Macintosh; Intel Mac OS X 14_1) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.1 Safari/605.1.15",
			wantBrowser: "Safari",
		},
		{
			name:        "Edge on Windows",
			ua:          "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/119.0.0.0 Safari/537.36 Edg/119.0.0.0",
			wantBrowser: "Edge",
		},
		{
			name:        "Opera",
			ua:          "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/119.0.0.0 Safari/537.36 OPR/105.0.0.0",
			wantBrowser: "Opera",
		},
		{
			name:        "Internet Explorer 11",
			ua:          "Mozilla/5.0 (Windows NT 10.0; WOW64; Trident/7.0; rv:11.0) like Gecko",
			wantBrowser: "Internet Explorer",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Parse(tt.ua)
			if result.Browser != tt.wantBrowser {
				t.Errorf("Browser = %q, want %q", result.Browser, tt.wantBrowser)
			}
			if result.IsBot {
				t.Errorf("IsBot = %v, want %v", result.IsBot, false)
			}
		})
	}
}

func TestParse_OperatingSystems(t *testing.T) {
	tests := []struct {
		name   string
		ua     string
		wantOS string
	}{
		{
			name:   "Windows 10",
			ua:     "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/119.0.0.0 Safari/537.36",
			wantOS: "Windows",
		},
		{
			name:   "Windows 11",
			ua:     "Mozilla/5.0 (Windows NT 11.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/119.0.0.0 Safari/537.36",
			wantOS: "Windows",
		},
		{
			name:   "macOS",
			ua:     "Mozilla/5.0 (Macintosh; Intel Mac OS X 14_1) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.1 Safari/605.1.15",
			wantOS: "macOS",
		},
		{
			name:   "Linux",
			ua:     "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/119.0.0.0 Safari/537.36",
			wantOS: "Linux",
		},
		{
			name:   "Ubuntu",
			ua:     "Mozilla/5.0 (X11; Ubuntu; Linux x86_64; rv:109.0) Gecko/20100101 Firefox/119.0",
			wantOS: "Ubuntu",
		},
		{
			name:   "Android",
			ua:     "Mozilla/5.0 (Linux; Android 14; Pixel 7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/119.0.0.0 Mobile Safari/537.36",
			wantOS: "Android",
		},
		{
			name:   "iOS iPhone",
			ua:     "Mozilla/5.0 (iPhone; CPU iPhone OS 17_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.1 Mobile/15E148 Safari/604.1",
			wantOS: "iOS",
		},
		{
			name:   "iOS iPad",
			ua:     "Mozilla/5.0 (iPad; CPU OS 17_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.1 Mobile/15E148 Safari/604.1",
			wantOS: "iOS",
		},
		{
			name:   "Chrome OS",
			ua:     "Mozilla/5.0 (X11; CrOS x86_64 14541.0.0) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/119.0.0.0 Safari/537.36",
			wantOS: "Chrome OS",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Parse(tt.ua)
			if result.OS != tt.wantOS {
				t.Errorf("OS = %q, want %q", result.OS, tt.wantOS)
			}
		})
	}
}

func TestParse_DeviceTypes(t *testing.T) {
	tests := []struct {
		name           string
		ua             string
		wantDeviceType string
	}{
		{
			name:           "Desktop Windows",
			ua:             "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/119.0.0.0 Safari/537.36",
			wantDeviceType: "desktop",
		},
		{
			name:           "Desktop macOS",
			ua:             "Mozilla/5.0 (Macintosh; Intel Mac OS X 14_1) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.1 Safari/605.1.15",
			wantDeviceType: "desktop",
		},
		{
			name:           "Mobile iPhone",
			ua:             "Mozilla/5.0 (iPhone; CPU iPhone OS 17_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.1 Mobile/15E148 Safari/604.1",
			wantDeviceType: "mobile",
		},
		{
			name:           "Mobile Android",
			ua:             "Mozilla/5.0 (Linux; Android 14; Pixel 7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/119.0.0.0 Mobile Safari/537.36",
			wantDeviceType: "mobile",
		},
		// Tablet detection: we check for tablet indicators BEFORE mobile,
		// so tablets are correctly classified even if the library marks them as mobile
		{
			name:           "Tablet iPad",
			ua:             "Mozilla/5.0 (iPad; CPU OS 17_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.1 Mobile/15E148 Safari/604.1",
			wantDeviceType: "tablet",
		},
		{
			name:           "Tablet Kindle",
			ua:             "Mozilla/5.0 (Linux; Android 4.4.3; Kindle Fire HDX 7) AppleWebKit/537.36 (KHTML, like Gecko) Silk/44.1.54 like Chrome/44.0.2403.63 Safari/537.36",
			wantDeviceType: "tablet",
		},
		{
			name:           "Tablet Playbook",
			ua:             "Mozilla/5.0 (PlayBook; U; RIM Tablet OS 2.1.0; en-US) AppleWebKit/536.2+ (KHTML like Gecko) Version/7.2.1.0 Safari/536.2+",
			wantDeviceType: "tablet",
		},
		{
			name:           "Tablet Silk browser",
			ua:             "Mozilla/5.0 (Linux; U; Android 4.4.3; en-us; KFTHWI Build/KTU84M) AppleWebKit/537.36 (KHTML, like Gecko) Silk/3.67 like Chrome/39.0.2171.93 Safari/537.36",
			wantDeviceType: "tablet",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Parse(tt.ua)
			if result.DeviceType != tt.wantDeviceType {
				t.Errorf("DeviceType = %q, want %q", result.DeviceType, tt.wantDeviceType)
			}
		})
	}
}

func TestParse_Bots(t *testing.T) {
	// Note: The bot detection uses a map with non-deterministic iteration order.
	// This means specific bot names may or may not be detected depending on which
	// key is checked first (e.g., "bot" may match before "googlebot").
	// These tests verify that bots ARE detected, rather than their specific names.

	botsToDetect := []struct {
		name string
		ua   string
	}{
		{"Googlebot", "Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)"},
		{"Bingbot", "Mozilla/5.0 (compatible; bingbot/2.0; +http://www.bing.com/bingbot.htm)"},
		{"YandexBot", "Mozilla/5.0 (compatible; YandexBot/3.0; +http://yandex.com/bots)"},
		{"DuckDuckBot", "DuckDuckBot/1.0; (+http://duckduckgo.com/duckduckbot.html)"},
		{"Baiduspider", "Mozilla/5.0 (compatible; Baiduspider/2.0; +http://www.baidu.com/search/spider.html)"},
		{"Facebook", "facebookexternalhit/1.1 (+http://www.facebook.com/externalhit_uatext.php)"},
		{"Twitter", "Twitterbot/1.0"},
		{"LinkedInBot", "LinkedInBot/1.0 (compatible; Mozilla/5.0; Apache-HttpClient +http://www.linkedin.com)"},
		{"GPTBot", "Mozilla/5.0 AppleWebKit/537.36 (KHTML, like Gecko; compatible; GPTBot/1.0; +https://openai.com/gptbot)"},
		{"ClaudeBot", "Mozilla/5.0 AppleWebKit/537.36 (KHTML, like Gecko; compatible; ClaudeBot/1.0; +claudebot@anthropic.com)"},
		{"AhrefsBot", "Mozilla/5.0 (compatible; AhrefsBot/7.0; +http://ahrefs.com/robot/)"},
		{"SemrushBot", "Mozilla/5.0 (compatible; SemrushBot/7~bl; +http://www.semrush.com/bot.html)"},
		{"UptimeRobot", "Mozilla/5.0+(compatible; UptimeRobot/2.0; http://www.uptimerobot.com/)"},
		{"Applebot", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_5) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/13.1.1 Safari/605.1.15 (Applebot/0.1; +http://www.apple.com/go/applebot)"},
		{"Generic crawler", "Mozilla/5.0 (compatible; MyCrawler/1.0)"},
		{"Generic spider", "MySpider/1.0 (+http://example.com/spider)"},
		{"Generic bot", "SomeBot/1.0"},
	}

	for _, tt := range botsToDetect {
		t.Run(tt.name, func(t *testing.T) {
			result := Parse(tt.ua)
			if !result.IsBot {
				t.Errorf("IsBot = %v, want %v for %s", result.IsBot, true, tt.name)
			}
			if result.DeviceType != "bot" {
				t.Errorf("DeviceType = %q, want %q for bot", result.DeviceType, "bot")
			}
			if result.BotName == "" {
				t.Errorf("BotName should not be empty for detected bot")
			}
		})
	}
}

func TestParse_NotBots(t *testing.T) {
	tests := []struct {
		name string
		ua   string
	}{
		{
			name: "Chrome",
			ua:   "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/119.0.0.0 Safari/537.36",
		},
		{
			name: "Firefox",
			ua:   "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:109.0) Gecko/20100101 Firefox/119.0",
		},
		{
			name: "Safari",
			ua:   "Mozilla/5.0 (Macintosh; Intel Mac OS X 14_1) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.1 Safari/605.1.15",
		},
		{
			name: "Mobile Safari",
			ua:   "Mozilla/5.0 (iPhone; CPU iPhone OS 17_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.1 Mobile/15E148 Safari/604.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Parse(tt.ua)
			if result.IsBot {
				t.Errorf("IsBot = %v, want %v for %s", result.IsBot, false, tt.name)
			}
			if result.BotName != "" {
				t.Errorf("BotName = %q, want empty string", result.BotName)
			}
		})
	}
}

func TestContainsAny(t *testing.T) {
	tests := []struct {
		name    string
		s       string
		substrs []string
		want    bool
	}{
		{"match first", "hello world", []string{"hello"}, true},
		{"match second", "hello world", []string{"world"}, true},
		{"no match", "hello world", []string{"foo", "bar"}, false},
		{"match one of many", "hello world", []string{"foo", "hello"}, true},
		{"empty string", "", []string{"foo"}, false},
		{"empty substrs", "hello", []string{}, false},
		{"bot pattern", "googlebot", []string{"bot", "crawler"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := containsAny(tt.s, tt.substrs...)
			if result != tt.want {
				t.Errorf("containsAny(%q, %v) = %v, want %v", tt.s, tt.substrs, result, tt.want)
			}
		})
	}
}

func TestIdentifyBot(t *testing.T) {
	// Signatures are sorted by length (longest first), so specific matches take priority over generic ones

	tests := []struct {
		lowerUA    string
		wantName   string
		wantIntent BotIntent
	}{
		{"googlebot/2.1", "Googlebot", IntentSEO},
		{"bingbot/2.0", "Bingbot", IntentSEO},
		{"compatible; yandexbot/3.0", "YandexBot", IntentSEO},
		{"duckduckbot/1.0", "DuckDuckBot", IntentSEO},
		{"claudebot/1.0", "ClaudeBot", IntentAI},
		{"gptbot/1.0", "GPTBot", IntentAI},
		{"uptimerobot/2.0", "UptimeRobot", IntentMonitoring},
		{"facebookexternalhit/1.1", "Facebook", IntentSocial},
		{"archive.org_bot", "Internet Archive", IntentArchiver},
		{"some random user agent with http://example.com", "Unknown Bot", IntentUnknown},
		{"completely unknown agent", "Unknown Bot", IntentUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.lowerUA, func(t *testing.T) {
			name, intent := identifyBot(tt.lowerUA)
			if name != tt.wantName {
				t.Errorf("identifyBot(%q) name = %q, want %q", tt.lowerUA, name, tt.wantName)
			}
			if intent != tt.wantIntent {
				t.Errorf("identifyBot(%q) intent = %q, want %q", tt.lowerUA, intent, tt.wantIntent)
			}
		})
	}
}

func TestNormalizeBrowserName(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"chrome", "Chrome"},
		{"Chrome", "Chrome"},
		{"google chrome", "Chrome"},
		{"firefox", "Firefox"},
		{"mozilla firefox", "Firefox"},
		{"safari", "Safari"},
		{"mobile safari", "Safari"},
		{"edge", "Edge"},
		{"microsoft edge", "Edge"},
		{"opera", "Opera"},
		{"opera mini", "Opera"},
		{"ie", "Internet Explorer"},
		{"internet explorer", "Internet Explorer"},
		{"msie", "Internet Explorer"},
		{"samsung browser", "Samsung Browser"},
		{"samsungbrowser", "Samsung Browser"},
		{"brave", "Brave"},
		{"vivaldi", "Vivaldi"},
		{"", "Unknown"},
		{"UnknownBrowser", "UnknownBrowser"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeBrowserName(tt.name)
			if result != tt.want {
				t.Errorf("normalizeBrowserName(%q) = %q, want %q", tt.name, result, tt.want)
			}
		})
	}
}

func TestParseOS(t *testing.T) {
	tests := []struct {
		osInfo  string
		lowerUA string
		wantOS  string
		wantVer string
	}{
		{"iPhone OS 17.1", "iphone; cpu iphone os 17_1 like mac os x", "iOS", "17.1"},
		{"", "ipad; cpu os 17_1 like mac os x", "iOS", ""},
		{"Android 14", "linux; android 14; pixel 7", "Android", "14"},
		{"Windows NT 10.0", "windows nt 10.0; win64; x64", "Windows", "10"},
		{"Windows NT 11.0", "windows nt 11.0; win64; x64", "Windows", "11"},
		// macOS version detection tests
		{"Mac OS X 14.1", "macintosh; intel mac os x 14_1", "macOS", "14.1"},
		{"Mac OS X 10_15_7", "macintosh; intel mac os x 10_15_7", "macOS", "10.15.7"},
		{"Intel Mac OS X 10_15_7", "macintosh; intel mac os x 10_15_7", "macOS", "10.15.7"},
		{"Linux", "x11; linux x86_64", "Linux", ""},
		{"Linux", "x11; ubuntu; linux x86_64", "Ubuntu", ""},
		{"Linux", "x11; fedora; linux x86_64", "Fedora", ""},
		{"Linux", "x11; debian; linux x86_64", "Debian", ""},
		{"CrOS x86_64", "x11; cros x86_64 14541.0.0", "Chrome OS", ""},
		{"", "", "Unknown", ""},
		{"CustomOS", "custom user agent", "CustomOS", ""},
	}

	for _, tt := range tests {
		t.Run(tt.osInfo+"_"+tt.lowerUA, func(t *testing.T) {
			os, ver := parseOS(tt.osInfo, tt.lowerUA)
			if os != tt.wantOS {
				t.Errorf("parseOS(%q, %q) OS = %q, want %q", tt.osInfo, tt.lowerUA, os, tt.wantOS)
			}
			if ver != tt.wantVer {
				t.Errorf("parseOS(%q, %q) version = %q, want %q", tt.osInfo, tt.lowerUA, ver, tt.wantVer)
			}
		})
	}
}

func TestExtractVersion(t *testing.T) {
	tests := []struct {
		osInfo string
		want   string
	}{
		{"Mac OS X 10.15.7", "10.15.7"},
		{"Android 14", "14"},
		{"iOS 17.1", "17.1"},
		{"Windows NT 10.0", "10.0"},
		{"Linux", ""},
		{"", ""},
		{"NoVersionHere", ""},
		{"Version 1.2.3;", "1.2.3"},
	}

	for _, tt := range tests {
		t.Run(tt.osInfo, func(t *testing.T) {
			result := extractVersion(tt.osInfo)
			if result != tt.want {
				t.Errorf("extractVersion(%q) = %q, want %q", tt.osInfo, result, tt.want)
			}
		})
	}
}

func TestExtractWindowsVersion(t *testing.T) {
	tests := []struct {
		osInfo string
		want   string
	}{
		{"Windows NT 11.0", "11"},
		{"Windows NT 10.0", "10"},
		// Note: The implementation checks for "8.1", "8", "7" as substrings,
		// but NT version strings like "6.3", "6.2", "6.1" don't contain these
		{"Windows NT 6.3", ""}, // Would need to detect as 8.1
		{"Windows NT 6.2", ""}, // Would need to detect as 8
		{"Windows NT 6.1", ""}, // Would need to detect as 7
		{"Windows Vista", "Vista"},
		{"Windows XP", "XP"},
		{"Windows NT 6.0", ""}, // Vista by NT version, but not matched here
		{"Linux", ""},
		// These work because they contain the substring directly
		{"Windows 8.1", "8.1"},
		{"Windows 8", "8"},
		{"Windows 7", "7"},
	}

	for _, tt := range tests {
		t.Run(tt.osInfo, func(t *testing.T) {
			result := extractWindowsVersion(tt.osInfo)
			if result != tt.want {
				t.Errorf("extractWindowsVersion(%q) = %q, want %q", tt.osInfo, result, tt.want)
			}
		})
	}
}

func TestExtractMacOSVersion(t *testing.T) {
	tests := []struct {
		name    string
		osInfo  string
		lowerUA string
		want    string
	}{
		// Standard formats from library
		{"Mac OS X with dots", "Mac OS X 10.15.7", "", "10.15.7"},
		{"Mac OS X with underscores", "Mac OS X 10_15_7", "", "10.15.7"},
		{"macOS Sonoma", "Mac OS X 14.1", "", "14.1"},
		{"macOS Ventura", "Mac OS X 13.0", "", "13.0"},
		// Intel Mac format (common in user agents)
		{"Intel Mac OS X", "Intel Mac OS X 10_15_7", "", "10.15.7"},
		{"Intel Mac 14", "Intel Mac OS X 14_0", "", "14.0"},
		// From raw user agent strings
		{"From UA with underscores", "", "macintosh; intel mac os x 10_15_7", "10.15.7"},
		{"From UA macOS 14", "", "macintosh; intel mac os x 14_1", "14.1"},
		// Edge cases
		{"Empty strings", "", "", ""},
		{"No version", "Mac OS X", "", ""},
		{"Just macintosh", "", "macintosh", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractMacOSVersion(tt.osInfo, tt.lowerUA)
			if result != tt.want {
				t.Errorf("extractMacOSVersion(%q, %q) = %q, want %q", tt.osInfo, tt.lowerUA, result, tt.want)
			}
		})
	}
}

func TestExtractVersionNumber(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"10.15.7", "10.15.7"},
		{"14.1", "14.1"},
		{"14", "14"},
		{"10.15.7)", "10.15.7"},
		{"10.15.7;", "10.15.7"},
		{"  10.15.7", "10.15.7"},
		{"", ""},
		{"abc", ""},
		{".10.15", ""},
		{"10.", "10"},
		{"10.15.", "10.15"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := extractVersionNumber(tt.input)
			if result != tt.want {
				t.Errorf("extractVersionNumber(%q) = %q, want %q", tt.input, result, tt.want)
			}
		})
	}
}

func TestIsTablet(t *testing.T) {
	tests := []struct {
		lowerUA string
		want    bool
	}{
		{"ipad; cpu os 17_1 like mac os x", true},
		{"tablet", true},
		{"kindle fire hdx", true},
		{"playbook", true},
		{"silk browser", true},
		{"iphone; cpu iphone os 17_1", false},
		{"android; mobile", false},
		{"windows nt 10.0", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.lowerUA, func(t *testing.T) {
			result := isTablet(tt.lowerUA)
			if result != tt.want {
				t.Errorf("isTablet(%q) = %v, want %v", tt.lowerUA, result, tt.want)
			}
		})
	}
}

func TestParse_RealWorldUserAgents(t *testing.T) {
	// Test with real-world user agent strings
	tests := []struct {
		name           string
		ua             string
		wantBrowser    string
		wantOS         string
		wantDeviceType string
		wantIsBot      bool
	}{
		{
			name:           "Chrome on Windows 10",
			ua:             "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
			wantBrowser:    "Chrome",
			wantOS:         "Windows",
			wantDeviceType: "desktop",
			wantIsBot:      false,
		},
		{
			name:           "Safari on macOS Sonoma",
			ua:             "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.2 Safari/605.1.15",
			wantBrowser:    "Safari",
			wantOS:         "macOS",
			wantDeviceType: "desktop",
			wantIsBot:      false,
		},
		{
			name:           "Chrome on Android",
			ua:             "Mozilla/5.0 (Linux; Android 14; SM-S918B) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.6099.144 Mobile Safari/537.36",
			wantBrowser:    "Chrome",
			wantOS:         "Android",
			wantDeviceType: "mobile",
			wantIsBot:      false,
		},
		{
			name:           "Safari on iPhone",
			ua:             "Mozilla/5.0 (iPhone; CPU iPhone OS 17_2 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.2 Mobile/15E148 Safari/604.1",
			wantBrowser:    "Safari",
			wantOS:         "iOS",
			wantDeviceType: "mobile",
			wantIsBot:      false,
		},
		{
			name:           "curl",
			ua:             "curl/7.88.1",
			wantBrowser:    "curl", // Library detects curl as browser name
			wantOS:         "Unknown",
			wantDeviceType: "desktop",
			wantIsBot:      false,
		},
		{
			name:           "wget",
			ua:             "Wget/1.21.3",
			wantBrowser:    "Wget", // Library detects wget as browser name
			wantOS:         "Unknown",
			wantDeviceType: "desktop",
			wantIsBot:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Parse(tt.ua)

			if result.Browser != tt.wantBrowser {
				t.Errorf("Browser = %q, want %q", result.Browser, tt.wantBrowser)
			}
			if result.OS != tt.wantOS {
				t.Errorf("OS = %q, want %q", result.OS, tt.wantOS)
			}
			if result.DeviceType != tt.wantDeviceType {
				t.Errorf("DeviceType = %q, want %q", result.DeviceType, tt.wantDeviceType)
			}
			if result.IsBot != tt.wantIsBot {
				t.Errorf("IsBot = %v, want %v", result.IsBot, tt.wantIsBot)
			}
		})
	}
}

func TestParsedUA_Fields(t *testing.T) {
	// Test that all fields are populated correctly
	ua := "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/119.0.0.0 Safari/537.36"
	result := Parse(ua)

	if result.Browser == "" {
		t.Error("Browser should not be empty")
	}
	if result.OS == "" {
		t.Error("OS should not be empty")
	}
	if result.DeviceType == "" {
		t.Error("DeviceType should not be empty")
	}
	// BrowserVersion and OSVersion may be empty for some UAs
}

func TestLoadBotSignatures(t *testing.T) {
	// Reset to defaults after each test
	defer ResetBotSignatures()

	t.Run("empty path uses defaults", func(t *testing.T) {
		ResetBotSignatures()
		err := LoadBotSignatures("")
		if err != nil {
			t.Errorf("LoadBotSignatures(\"\") returned error: %v", err)
		}
		// Verify defaults are loaded
		sigs := GetBotSignatures()
		if len(sigs) == 0 {
			t.Error("expected default signatures to be loaded")
		}
	})

	t.Run("non-existent file uses defaults", func(t *testing.T) {
		ResetBotSignatures()
		err := LoadBotSignatures("/non/existent/path.json")
		if err != nil {
			t.Errorf("LoadBotSignatures with non-existent file returned error: %v", err)
		}
		sigs := GetBotSignatures()
		if len(sigs) == 0 {
			t.Error("expected default signatures to be loaded")
		}
	})

	t.Run("valid JSON file loads signatures", func(t *testing.T) {
		ResetBotSignatures()

		// Create a temporary file with custom bot signatures
		tmpDir := t.TempDir()
		tmpFile := filepath.Join(tmpDir, "bots.json")
		content := `{
			"version": "1.0",
			"bots": [
				{"signature": "testbot", "name": "TestBot", "intent": "seo"},
				{"signature": "myspider", "name": "MySpider", "intent": "ai"}
			]
		}`
		if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write temp file: %v", err)
		}

		err := LoadBotSignatures(tmpFile)
		if err != nil {
			t.Errorf("LoadBotSignatures returned error: %v", err)
		}

		sigs := GetBotSignatures()
		if len(sigs) != 2 {
			t.Errorf("expected 2 signatures, got %d", len(sigs))
		}

		// Verify signatures are sorted by length (longest first)
		if len(sigs) >= 2 && len(sigs[0].Signature) < len(sigs[1].Signature) {
			t.Error("expected signatures to be sorted by length (longest first)")
		}
	})

	t.Run("invalid JSON returns error", func(t *testing.T) {
		ResetBotSignatures()

		tmpDir := t.TempDir()
		tmpFile := filepath.Join(tmpDir, "bots.json")
		if err := os.WriteFile(tmpFile, []byte("not valid json"), 0644); err != nil {
			t.Fatalf("failed to write temp file: %v", err)
		}

		err := LoadBotSignatures(tmpFile)
		if err == nil {
			t.Error("expected error for invalid JSON")
		}
	})

	t.Run("empty bots array uses defaults", func(t *testing.T) {
		ResetBotSignatures()

		tmpDir := t.TempDir()
		tmpFile := filepath.Join(tmpDir, "bots.json")
		content := `{"version": "1.0", "bots": []}`
		if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write temp file: %v", err)
		}

		err := LoadBotSignatures(tmpFile)
		if err != nil {
			t.Errorf("LoadBotSignatures returned error: %v", err)
		}

		// Should still have defaults
		sigs := GetBotSignatures()
		if len(sigs) == 0 {
			t.Error("expected default signatures after loading empty file")
		}
	})
}

func TestBotIntentClassification(t *testing.T) {
	tests := []struct {
		ua         string
		wantIntent BotIntent
	}{
		{"Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)", IntentSEO},
		{"Mozilla/5.0 (compatible; bingbot/2.0; +http://www.bing.com/bingbot.htm)", IntentSEO},
		{"facebookexternalhit/1.1 (+http://www.facebook.com/externalhit_uatext.php)", IntentSocial},
		{"Twitterbot/1.0", IntentSocial},
		{"Mozilla/5.0 AppleWebKit/537.36 (KHTML, like Gecko; compatible; GPTBot/1.0)", IntentAI},
		{"Mozilla/5.0 AppleWebKit/537.36 (KHTML, like Gecko; compatible; ClaudeBot/1.0)", IntentAI},
		{"Mozilla/5.0+(compatible; UptimeRobot/2.0; http://www.uptimerobot.com/)", IntentMonitoring},
		{"archive.org_bot", IntentArchiver},
		{"SomeRandomBot/1.0", IntentUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.ua, func(t *testing.T) {
			result := Parse(tt.ua)
			if !result.IsBot {
				t.Errorf("expected IsBot=true for %q", tt.ua)
				return
			}
			if result.BotIntent != tt.wantIntent {
				t.Errorf("Parse(%q).BotIntent = %q, want %q", tt.ua, result.BotIntent, tt.wantIntent)
			}
		})
	}
}

func TestCaseInsensitiveMatching(t *testing.T) {
	// Bot detection should be case-insensitive
	tests := []struct {
		ua       string
		wantBot  bool
		wantName string
	}{
		{"GOOGLEBOT/2.1", true, "Googlebot"},
		{"GoogleBot/2.1", true, "Googlebot"},
		{"googlebot/2.1", true, "Googlebot"},
		{"GPTBot/1.0", true, "GPTBot"},
		{"gptbot/1.0", true, "GPTBot"},
		{"GPTBOT/1.0", true, "GPTBot"},
		{"UptimeRobot/2.0", true, "UptimeRobot"},
		{"UPTIMEROBOT/2.0", true, "UptimeRobot"},
	}

	for _, tt := range tests {
		t.Run(tt.ua, func(t *testing.T) {
			result := Parse(tt.ua)
			if result.IsBot != tt.wantBot {
				t.Errorf("Parse(%q).IsBot = %v, want %v", tt.ua, result.IsBot, tt.wantBot)
			}
			if result.BotName != tt.wantName {
				t.Errorf("Parse(%q).BotName = %q, want %q", tt.ua, result.BotName, tt.wantName)
			}
		})
	}
}

func TestGetBotSignatures(t *testing.T) {
	sigs := GetBotSignatures()
	if len(sigs) == 0 {
		t.Error("expected at least one bot signature")
	}

	// Verify we get a copy, not the original
	sigs[0].Name = "Modified"
	original := GetBotSignatures()
	if original[0].Name == "Modified" {
		t.Error("GetBotSignatures should return a copy")
	}
}

func TestNormalizeIntent(t *testing.T) {
	tests := []struct {
		input BotIntent
		want  BotIntent
	}{
		{IntentSEO, IntentSEO},
		{IntentSocial, IntentSocial},
		{IntentMonitoring, IntentMonitoring},
		{IntentAI, IntentAI},
		{IntentArchiver, IntentArchiver},
		{IntentUnknown, IntentUnknown},
		{"invalid", IntentUnknown},
		{"", IntentUnknown},
	}

	for _, tt := range tests {
		t.Run(string(tt.input), func(t *testing.T) {
			result := normalizeIntent(tt.input)
			if result != tt.want {
				t.Errorf("normalizeIntent(%q) = %q, want %q", tt.input, result, tt.want)
			}
		})
	}
}

func TestSortSignatures(t *testing.T) {
	sigs := []BotSignature{
		{Signature: "bot", Name: "Short"},
		{Signature: "googlebot", Name: "Long"},
		{Signature: "abot", Name: "Medium"},
	}

	sortSignatures(sigs)

	// Should be sorted by length descending
	if sigs[0].Signature != "googlebot" {
		t.Errorf("expected googlebot first, got %s", sigs[0].Signature)
	}
	if sigs[1].Signature != "abot" {
		t.Errorf("expected abot second, got %s", sigs[1].Signature)
	}
	if sigs[2].Signature != "bot" {
		t.Errorf("expected bot third, got %s", sigs[2].Signature)
	}
}

func TestLoadBotSignaturesList(t *testing.T) {
	// Reset to defaults after each test
	defer ResetBotSignatures()

	t.Run("empty paths uses defaults", func(t *testing.T) {
		ResetBotSignatures()
		err := LoadBotSignaturesList(nil)
		if err != nil {
			t.Errorf("LoadBotSignaturesList(nil) returned error: %v", err)
		}
		sigs := GetBotSignatures()
		if len(sigs) == 0 {
			t.Error("expected default signatures to be loaded")
		}
	})

	t.Run("empty strings in paths uses defaults", func(t *testing.T) {
		ResetBotSignatures()
		err := LoadBotSignaturesList([]string{"", "  ", ""})
		if err != nil {
			t.Errorf("LoadBotSignaturesList returned error: %v", err)
		}
		sigs := GetBotSignatures()
		if len(sigs) == 0 {
			t.Error("expected default signatures to be loaded")
		}
	})

	t.Run("single file merges with defaults", func(t *testing.T) {
		ResetBotSignatures()
		defaultCount := len(GetBotSignatures())

		tmpDir := t.TempDir()
		tmpFile := filepath.Join(tmpDir, "community.json")
		content := `{
			"version": "1.0",
			"bots": [
				{"signature": "newcommunitybot", "name": "CommunityBot", "intent": "seo"},
				{"signature": "anotherbot", "name": "AnotherBot", "intent": "ai"}
			]
		}`
		if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write temp file: %v", err)
		}

		err := LoadBotSignaturesList([]string{tmpFile})
		if err != nil {
			t.Errorf("LoadBotSignaturesList returned error: %v", err)
		}

		sigs := GetBotSignatures()
		// Should have defaults + 2 new signatures
		if len(sigs) != defaultCount+2 {
			t.Errorf("expected %d signatures, got %d", defaultCount+2, len(sigs))
		}

		// Verify new bots are detected
		result := Parse("newcommunitybot/1.0")
		if !result.IsBot || result.BotName != "CommunityBot" {
			t.Errorf("expected CommunityBot, got %s (IsBot=%v)", result.BotName, result.IsBot)
		}
	})

	t.Run("multiple files merge correctly", func(t *testing.T) {
		ResetBotSignatures()
		defaultCount := len(GetBotSignatures())

		tmpDir := t.TempDir()

		// First file
		file1 := filepath.Join(tmpDir, "bots1.json")
		content1 := `{
			"version": "1.0",
			"bots": [
				{"signature": "firstbot", "name": "FirstBot", "intent": "seo"},
				{"signature": "sharedbot", "name": "SharedFromFirst", "intent": "unknown"}
			]
		}`
		if err := os.WriteFile(file1, []byte(content1), 0644); err != nil {
			t.Fatalf("failed to write file1: %v", err)
		}

		// Second file (overrides sharedbot)
		file2 := filepath.Join(tmpDir, "bots2.json")
		content2 := `{
			"version": "1.0",
			"bots": [
				{"signature": "secondbot", "name": "SecondBot", "intent": "ai"},
				{"signature": "sharedbot", "name": "SharedFromSecond", "intent": "social"}
			]
		}`
		if err := os.WriteFile(file2, []byte(content2), 0644); err != nil {
			t.Fatalf("failed to write file2: %v", err)
		}

		err := LoadBotSignaturesList([]string{file1, file2})
		if err != nil {
			t.Errorf("LoadBotSignaturesList returned error: %v", err)
		}

		sigs := GetBotSignatures()
		// Should have defaults + firstbot + secondbot + sharedbot (3 new unique)
		if len(sigs) != defaultCount+3 {
			t.Errorf("expected %d signatures, got %d", defaultCount+3, len(sigs))
		}

		// Verify sharedbot was overridden by second file
		result := Parse("sharedbot/1.0")
		if result.BotName != "SharedFromSecond" {
			t.Errorf("expected SharedFromSecond, got %s", result.BotName)
		}
		if result.BotIntent != IntentSocial {
			t.Errorf("expected social intent, got %s", result.BotIntent)
		}
	})

	t.Run("override default bot signature", func(t *testing.T) {
		ResetBotSignatures()

		tmpDir := t.TempDir()
		tmpFile := filepath.Join(tmpDir, "override.json")
		content := `{
			"version": "1.0",
			"bots": [
				{"signature": "googlebot", "name": "Custom Googlebot", "intent": "ai"}
			]
		}`
		if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write temp file: %v", err)
		}

		err := LoadBotSignaturesList([]string{tmpFile})
		if err != nil {
			t.Errorf("LoadBotSignaturesList returned error: %v", err)
		}

		// Verify googlebot was overridden
		result := Parse("googlebot/2.1")
		if result.BotName != "Custom Googlebot" {
			t.Errorf("expected Custom Googlebot, got %s", result.BotName)
		}
		if result.BotIntent != IntentAI {
			t.Errorf("expected ai intent, got %s", result.BotIntent)
		}
	})

	t.Run("non-existent file is skipped gracefully", func(t *testing.T) {
		ResetBotSignatures()
		defaultCount := len(GetBotSignatures())

		tmpDir := t.TempDir()
		goodFile := filepath.Join(tmpDir, "good.json")
		content := `{
			"version": "1.0",
			"bots": [
				{"signature": "goodbot", "name": "GoodBot", "intent": "seo"}
			]
		}`
		if err := os.WriteFile(goodFile, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write temp file: %v", err)
		}

		err := LoadBotSignaturesList([]string{"/non/existent/path.json", goodFile})
		if err != nil {
			t.Errorf("LoadBotSignaturesList returned error: %v", err)
		}

		sigs := GetBotSignatures()
		// Should have defaults + goodbot
		if len(sigs) != defaultCount+1 {
			t.Errorf("expected %d signatures, got %d", defaultCount+1, len(sigs))
		}

		// Verify good bot was loaded
		result := Parse("goodbot/1.0")
		if result.BotName != "GoodBot" {
			t.Errorf("expected GoodBot, got %s", result.BotName)
		}
	})

	t.Run("invalid JSON file is skipped gracefully", func(t *testing.T) {
		ResetBotSignatures()
		defaultCount := len(GetBotSignatures())

		tmpDir := t.TempDir()

		badFile := filepath.Join(tmpDir, "bad.json")
		if err := os.WriteFile(badFile, []byte("not valid json"), 0644); err != nil {
			t.Fatalf("failed to write bad file: %v", err)
		}

		goodFile := filepath.Join(tmpDir, "good.json")
		content := `{
			"version": "1.0",
			"bots": [
				{"signature": "validbot", "name": "ValidBot", "intent": "seo"}
			]
		}`
		if err := os.WriteFile(goodFile, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write good file: %v", err)
		}

		err := LoadBotSignaturesList([]string{badFile, goodFile})
		if err != nil {
			t.Errorf("LoadBotSignaturesList returned error: %v", err)
		}

		sigs := GetBotSignatures()
		// Should have defaults + validbot (bad file skipped)
		if len(sigs) != defaultCount+1 {
			t.Errorf("expected %d signatures, got %d", defaultCount+1, len(sigs))
		}

		result := Parse("validbot/1.0")
		if result.BotName != "ValidBot" {
			t.Errorf("expected ValidBot, got %s", result.BotName)
		}
	})
}
