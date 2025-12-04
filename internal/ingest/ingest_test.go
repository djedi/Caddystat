package ingest

import (
	"testing"
	"time"
)

func TestParseCaddyLog_UnixTimestamp(t *testing.T) {
	line := `{"ts":1700000000.123456,"request":{"host":"example.com","uri":"/page","remote_ip":"192.168.1.1","headers":{"User-Agent":["Mozilla/5.0"],"Referer":["https://google.com"]}},"status":200,"bytes_written":1234,"duration":0.05}`

	entry, err := parseCaddyLog(line)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check timestamp (1700000000 is approximately Nov 14, 2023)
	// Allow small tolerance for floating-point precision
	expectedTime := time.Unix(1700000000, 123456000).UTC()
	timeDiff := entry.Timestamp.Sub(expectedTime)
	if timeDiff < -time.Microsecond || timeDiff > time.Microsecond {
		t.Errorf("Timestamp = %v, want approximately %v (diff: %v)", entry.Timestamp, expectedTime, timeDiff)
	}

	if entry.Host != "example.com" {
		t.Errorf("Host = %q, want %q", entry.Host, "example.com")
	}

	if entry.Path != "/page" {
		t.Errorf("Path = %q, want %q", entry.Path, "/page")
	}

	if entry.Status != 200 {
		t.Errorf("Status = %d, want %d", entry.Status, 200)
	}

	if entry.Bytes != 1234 {
		t.Errorf("Bytes = %d, want %d", entry.Bytes, 1234)
	}

	if entry.RemoteAddr != "192.168.1.1" {
		t.Errorf("RemoteAddr = %q, want %q", entry.RemoteAddr, "192.168.1.1")
	}

	if entry.UserAgent != "Mozilla/5.0" {
		t.Errorf("UserAgent = %q, want %q", entry.UserAgent, "Mozilla/5.0")
	}

	if entry.Referrer != "https://google.com" {
		t.Errorf("Referrer = %q, want %q", entry.Referrer, "https://google.com")
	}

	// Duration is converted from seconds to milliseconds
	if entry.DurationMs != 50 {
		t.Errorf("DurationMs = %f, want %f", entry.DurationMs, 50.0)
	}
}

func TestParseCaddyLog_RFC3339Timestamp(t *testing.T) {
	line := `{"ts":"2023-11-14T12:30:00.123456789Z","request":{"host":"example.com","uri":"/page","remote_ip":"10.0.0.1"},"status":200,"bytes_written":100,"duration":0.001}`

	entry, err := parseCaddyLog(line)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedTime, _ := time.Parse(time.RFC3339Nano, "2023-11-14T12:30:00.123456789Z")
	if !entry.Timestamp.Equal(expectedTime) {
		t.Errorf("Timestamp = %v, want %v", entry.Timestamp, expectedTime)
	}
}

func TestParseCaddyLog_RFC3339WithoutNano(t *testing.T) {
	line := `{"ts":"2023-11-14T12:30:00Z","request":{"host":"example.com","uri":"/page","remote_ip":"10.0.0.1"},"status":200,"bytes_written":100,"duration":0.001}`

	entry, err := parseCaddyLog(line)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedTime, _ := time.Parse(time.RFC3339, "2023-11-14T12:30:00Z")
	if !entry.Timestamp.Equal(expectedTime) {
		t.Errorf("Timestamp = %v, want %v", entry.Timestamp, expectedTime)
	}
}

func TestParseCaddyLog_SizeField(t *testing.T) {
	// Some Caddy versions use "size" instead of "bytes_written"
	line := `{"ts":1700000000,"request":{"host":"example.com","uri":"/","remote_ip":"10.0.0.1"},"status":200,"size":5678,"duration":0.01}`

	entry, err := parseCaddyLog(line)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if entry.Bytes != 5678 {
		t.Errorf("Bytes = %d, want %d", entry.Bytes, 5678)
	}
}

func TestParseCaddyLog_BytesWrittenTakesPrecedence(t *testing.T) {
	// If both are present, bytes_written should take precedence
	line := `{"ts":1700000000,"request":{"host":"example.com","uri":"/","remote_ip":"10.0.0.1"},"status":200,"bytes_written":1000,"size":5678,"duration":0.01}`

	entry, err := parseCaddyLog(line)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if entry.Bytes != 1000 {
		t.Errorf("Bytes = %d, want %d", entry.Bytes, 1000)
	}
}

func TestParseCaddyLog_ClientIPPriority(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		wantIP   string
	}{
		{
			name:   "Cloudflare CF-Connecting-IP",
			line:   `{"ts":1700000000,"request":{"host":"example.com","uri":"/","remote_ip":"10.0.0.1","client_ip":"10.0.0.2","headers":{"Cf-Connecting-Ip":["203.0.113.50"]}},"status":200}`,
			wantIP: "203.0.113.50",
		},
		{
			name:   "X-Real-IP",
			line:   `{"ts":1700000000,"request":{"host":"example.com","uri":"/","remote_ip":"10.0.0.1","headers":{"X-Real-Ip":["198.51.100.25"]}},"status":200}`,
			wantIP: "198.51.100.25",
		},
		{
			name:   "X-Forwarded-For single IP",
			line:   `{"ts":1700000000,"request":{"host":"example.com","uri":"/","remote_ip":"10.0.0.1","headers":{"X-Forwarded-For":["192.0.2.100"]}},"status":200}`,
			wantIP: "192.0.2.100",
		},
		{
			name:   "X-Forwarded-For multiple IPs",
			line:   `{"ts":1700000000,"request":{"host":"example.com","uri":"/","remote_ip":"10.0.0.1","headers":{"X-Forwarded-For":["192.0.2.100, 198.51.100.25, 10.0.0.5"]}},"status":200}`,
			wantIP: "192.0.2.100",
		},
		{
			name:   "client_ip field",
			line:   `{"ts":1700000000,"request":{"host":"example.com","uri":"/","remote_ip":"10.0.0.1","client_ip":"172.16.0.50"},"status":200}`,
			wantIP: "172.16.0.50",
		},
		{
			name:   "remote_ip fallback",
			line:   `{"ts":1700000000,"request":{"host":"example.com","uri":"/","remote_ip":"10.0.0.1"},"status":200}`,
			wantIP: "10.0.0.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry, err := parseCaddyLog(tt.line)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if entry.RemoteAddr != tt.wantIP {
				t.Errorf("RemoteAddr = %q, want %q", entry.RemoteAddr, tt.wantIP)
			}
		})
	}
}

func TestParseCaddyLog_ReferrerVariants(t *testing.T) {
	tests := []struct {
		name        string
		line        string
		wantReferrer string
	}{
		{
			name:        "Referer header (correct spelling)",
			line:        `{"ts":1700000000,"request":{"host":"example.com","uri":"/","remote_ip":"10.0.0.1","headers":{"Referer":["https://google.com"]}},"status":200}`,
			wantReferrer: "https://google.com",
		},
		{
			name:        "Referrer header (alternate spelling)",
			line:        `{"ts":1700000000,"request":{"host":"example.com","uri":"/","remote_ip":"10.0.0.1","headers":{"Referrer":["https://bing.com"]}},"status":200}`,
			wantReferrer: "https://bing.com",
		},
		{
			name:        "No referrer",
			line:        `{"ts":1700000000,"request":{"host":"example.com","uri":"/","remote_ip":"10.0.0.1"},"status":200}`,
			wantReferrer: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry, err := parseCaddyLog(tt.line)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if entry.Referrer != tt.wantReferrer {
				t.Errorf("Referrer = %q, want %q", entry.Referrer, tt.wantReferrer)
			}
		})
	}
}

func TestParseCaddyLog_InvalidJSON(t *testing.T) {
	_, err := parseCaddyLog("not json")
	if err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
}

func TestParseCaddyLog_EmptyLine(t *testing.T) {
	_, err := parseCaddyLog("")
	if err == nil {
		t.Error("expected error for empty line, got nil")
	}
}

func TestParseCaddyLog_MinimalValid(t *testing.T) {
	// Minimal valid entry - just required fields
	line := `{"ts":1700000000,"request":{"host":"","uri":"","remote_ip":""},"status":0}`

	entry, err := parseCaddyLog(line)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if entry.Status != 0 {
		t.Errorf("Status = %d, want %d", entry.Status, 0)
	}
}

func TestNormalizeIP(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"192.168.1.1", "192.168.1.1"},
		{"192.168.1.1:8080", "192.168.1.1"},
		{"10.0.0.1:443", "10.0.0.1"},
		{"[2001:db8::1]:8080", "2001:db8::1"},
		{"2001:db8::1", "2001:db8::1"},
		{"", ""},
		{"invalid:not:a:port", "invalid:not:a:port"}, // Not a valid host:port, returned as-is
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeIP(tt.input)
			if got != tt.want {
				t.Errorf("normalizeIP(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestAnonymizeIP(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"192.168.1.100", "192.168.1.0"},
		{"10.0.0.255", "10.0.0.0"},
		{"172.16.50.123", "172.16.50.0"},
		{"2001:db8:85a3::8a2e:370:7334", "2001:db8:85a3::8a2e:370:0"},
		{"", ""},
		{"invalid", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := anonymizeIP(tt.input)
			if got != tt.want {
				t.Errorf("anonymizeIP(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestHashIP(t *testing.T) {
	tests := []struct {
		ip   string
		salt string
	}{
		{"192.168.1.1", "test-salt"},
		{"10.0.0.1", "another-salt"},
		{"", "salt"},
	}

	for _, tt := range tests {
		t.Run(tt.ip, func(t *testing.T) {
			result := hashIP(tt.ip, tt.salt)

			if tt.ip == "" {
				if result != "" {
					t.Errorf("hashIP(%q, %q) = %q, want empty string", tt.ip, tt.salt, result)
				}
				return
			}

			// Result should be a 64-character hex string (SHA256)
			if len(result) != 64 {
				t.Errorf("hashIP(%q, %q) length = %d, want 64", tt.ip, tt.salt, len(result))
			}

			// Same input should produce same output
			result2 := hashIP(tt.ip, tt.salt)
			if result != result2 {
				t.Errorf("hashIP is not deterministic: got %q and %q", result, result2)
			}

			// Different salt should produce different output
			resultDiffSalt := hashIP(tt.ip, tt.salt+"-different")
			if result == resultDiffSalt {
				t.Errorf("hashIP with different salt produced same result")
			}
		})
	}
}

func TestFirstHeader(t *testing.T) {
	tests := []struct {
		name    string
		headers map[string][]string
		key     string
		want    string
	}{
		{
			name:    "exact match",
			headers: map[string][]string{"User-Agent": {"Mozilla/5.0"}},
			key:     "User-Agent",
			want:    "Mozilla/5.0",
		},
		{
			name:    "case insensitive",
			headers: map[string][]string{"user-agent": {"Mozilla/5.0"}},
			key:     "User-Agent",
			want:    "Mozilla/5.0",
		},
		{
			name:    "multiple values",
			headers: map[string][]string{"Accept": {"text/html", "application/json"}},
			key:     "Accept",
			want:    "text/html",
		},
		{
			name:    "not found",
			headers: map[string][]string{"User-Agent": {"Mozilla/5.0"}},
			key:     "X-Custom",
			want:    "",
		},
		{
			name:    "nil headers",
			headers: nil,
			key:     "User-Agent",
			want:    "",
		},
		{
			name:    "empty headers",
			headers: map[string][]string{},
			key:     "User-Agent",
			want:    "",
		},
		{
			name:    "empty value slice",
			headers: map[string][]string{"User-Agent": {}},
			key:     "User-Agent",
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := firstHeader(tt.headers, tt.key)
			if got != tt.want {
				t.Errorf("firstHeader(%v, %q) = %q, want %q", tt.headers, tt.key, got, tt.want)
			}
		})
	}
}

func TestParseCaddyLog_CompleteEntry(t *testing.T) {
	// Test a complete, realistic Caddy log entry
	line := `{
		"ts": 1700000000.5,
		"request": {
			"host": "example.com",
			"uri": "/api/users?page=1",
			"remote_ip": "10.0.0.1",
			"remote_port": "54321",
			"client_ip": "192.168.1.100",
			"headers": {
				"User-Agent": ["Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36"],
				"Referer": ["https://example.com/dashboard"],
				"Accept-Language": ["en-US,en;q=0.9"],
				"X-Forwarded-For": ["203.0.113.50"]
			}
		},
		"status": 200,
		"bytes_written": 4567,
		"duration": 0.125,
		"resp_headers": {
			"Content-Type": ["application/json"]
		}
	}`

	entry, err := parseCaddyLog(line)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if entry.Host != "example.com" {
		t.Errorf("Host = %q, want %q", entry.Host, "example.com")
	}

	if entry.Path != "/api/users?page=1" {
		t.Errorf("Path = %q, want %q", entry.Path, "/api/users?page=1")
	}

	if entry.Status != 200 {
		t.Errorf("Status = %d, want %d", entry.Status, 200)
	}

	if entry.Bytes != 4567 {
		t.Errorf("Bytes = %d, want %d", entry.Bytes, 4567)
	}

	// X-Forwarded-For should take priority
	if entry.RemoteAddr != "203.0.113.50" {
		t.Errorf("RemoteAddr = %q, want %q", entry.RemoteAddr, "203.0.113.50")
	}

	if entry.Referrer != "https://example.com/dashboard" {
		t.Errorf("Referrer = %q, want %q", entry.Referrer, "https://example.com/dashboard")
	}

	expectedUA := "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36"
	if entry.UserAgent != expectedUA {
		t.Errorf("UserAgent = %q, want %q", entry.UserAgent, expectedUA)
	}

	if entry.DurationMs != 125 {
		t.Errorf("DurationMs = %f, want %f", entry.DurationMs, 125.0)
	}
}

func TestParseCaddyLog_StatusCodes(t *testing.T) {
	tests := []struct {
		status int
	}{
		{200},
		{201},
		{301},
		{302},
		{400},
		{401},
		{403},
		{404},
		{500},
		{502},
		{503},
	}

	for _, tt := range tests {
		t.Run(string(rune(tt.status)), func(t *testing.T) {
			line := `{"ts":1700000000,"request":{"host":"example.com","uri":"/","remote_ip":"10.0.0.1"},"status":` + string(rune('0'+tt.status/100)) + string(rune('0'+(tt.status%100)/10)) + string(rune('0'+tt.status%10)) + `}`
			entry, err := parseCaddyLog(line)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if entry.Status != tt.status {
				t.Errorf("Status = %d, want %d", entry.Status, tt.status)
			}
		})
	}
}

func TestParseCaddyLog_VariousStatusCodes(t *testing.T) {
	tests := []struct {
		name   string
		line   string
		status int
	}{
		{"200 OK", `{"ts":1700000000,"request":{"host":"example.com","uri":"/","remote_ip":"10.0.0.1"},"status":200}`, 200},
		{"201 Created", `{"ts":1700000000,"request":{"host":"example.com","uri":"/","remote_ip":"10.0.0.1"},"status":201}`, 201},
		{"301 Redirect", `{"ts":1700000000,"request":{"host":"example.com","uri":"/","remote_ip":"10.0.0.1"},"status":301}`, 301},
		{"404 Not Found", `{"ts":1700000000,"request":{"host":"example.com","uri":"/","remote_ip":"10.0.0.1"},"status":404}`, 404},
		{"500 Server Error", `{"ts":1700000000,"request":{"host":"example.com","uri":"/","remote_ip":"10.0.0.1"},"status":500}`, 500},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry, err := parseCaddyLog(tt.line)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if entry.Status != tt.status {
				t.Errorf("Status = %d, want %d", entry.Status, tt.status)
			}
		})
	}
}

func TestParseCaddyLog_IPv6Addresses(t *testing.T) {
	tests := []struct {
		name   string
		line   string
		wantIP string
	}{
		{
			name:   "full IPv6",
			line:   `{"ts":1700000000,"request":{"host":"example.com","uri":"/","remote_ip":"2001:0db8:85a3:0000:0000:8a2e:0370:7334"},"status":200}`,
			wantIP: "2001:0db8:85a3:0000:0000:8a2e:0370:7334",
		},
		{
			name:   "compressed IPv6",
			line:   `{"ts":1700000000,"request":{"host":"example.com","uri":"/","remote_ip":"2001:db8::1"},"status":200}`,
			wantIP: "2001:db8::1",
		},
		{
			name:   "loopback IPv6",
			line:   `{"ts":1700000000,"request":{"host":"example.com","uri":"/","remote_ip":"::1"},"status":200}`,
			wantIP: "::1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry, err := parseCaddyLog(tt.line)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if entry.RemoteAddr != tt.wantIP {
				t.Errorf("RemoteAddr = %q, want %q", entry.RemoteAddr, tt.wantIP)
			}
		})
	}
}

func TestParseCaddyLog_LargeBytes(t *testing.T) {
	// Test with large file sizes (> 32-bit int)
	line := `{"ts":1700000000,"request":{"host":"example.com","uri":"/","remote_ip":"10.0.0.1"},"status":200,"bytes_written":5368709120}`

	entry, err := parseCaddyLog(line)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 5GB in bytes
	var expected int64 = 5368709120
	if entry.Bytes != expected {
		t.Errorf("Bytes = %d, want %d", entry.Bytes, expected)
	}
}

func TestParseCaddyLog_URIWithSpecialChars(t *testing.T) {
	tests := []struct {
		name string
		uri  string
	}{
		{"query string", "/search?q=hello+world"},
		{"encoded chars", "/path%20with%20spaces"},
		{"fragment", "/page#section"},
		{"unicode", "/путь/к/файлу"},
		{"special chars", "/api/v1/users/@me"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			line := `{"ts":1700000000,"request":{"host":"example.com","uri":"` + tt.uri + `","remote_ip":"10.0.0.1"},"status":200}`
			entry, err := parseCaddyLog(line)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if entry.Path != tt.uri {
				t.Errorf("Path = %q, want %q", entry.Path, tt.uri)
			}
		})
	}
}

func TestIngestor_StopBeforeStart(t *testing.T) {
	// Stop should not panic if called before Start
	ingestor := &Ingestor{}
	ingestor.Stop() // Should not panic
}

func TestGeoLookup_CloseNil(t *testing.T) {
	// Close on nil GeoLookup should not panic
	var g *GeoLookup
	err := g.Close()
	if err != nil {
		t.Errorf("Close() = %v, want nil", err)
	}
}

func TestGeoLookup_CloseNilDB(t *testing.T) {
	// Close with nil db should not panic
	g := &GeoLookup{db: nil}
	err := g.Close()
	if err != nil {
		t.Errorf("Close() = %v, want nil", err)
	}
}
