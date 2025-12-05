package ingest

import (
	"bufio"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/hpcloud/tail"
	"github.com/oschwald/maxminddb-golang"

	"github.com/dustin/Caddystat/internal/config"
	"github.com/dustin/Caddystat/internal/metrics"
	"github.com/dustin/Caddystat/internal/sse"
	"github.com/dustin/Caddystat/internal/storage"
	"github.com/dustin/Caddystat/internal/useragent"
)

type Ingestor struct {
	cfg     config.Config
	store   *storage.Storage
	hub     *sse.Hub
	geo     *GeoLookup
	metrics *metrics.Metrics
	wg      sync.WaitGroup
	cancel  context.CancelFunc
}

func New(cfg config.Config, store *storage.Storage, hub *sse.Hub, geo *GeoLookup, m *metrics.Metrics) *Ingestor {
	return &Ingestor{
		cfg:     cfg,
		store:   store,
		hub:     hub,
		geo:     geo,
		metrics: m,
	}
}

func (i *Ingestor) Start(ctx context.Context) error {
	// Create a derived context that we can cancel on Stop()
	tailCtx, cancel := context.WithCancel(ctx)
	i.cancel = cancel

	// First, import any existing log files (including rotated/gzipped ones)
	for _, path := range i.cfg.LogPaths {
		if err := i.importHistoricalLogs(ctx, path); err != nil {
			slog.Warn("failed to import historical logs", "path", path, "error", err)
		}
	}

	// Then start tailing for new entries
	for _, path := range i.cfg.LogPaths {
		i.wg.Add(1)
		go func(p string) {
			defer i.wg.Done()
			i.tailFile(tailCtx, p)
		}(path)
	}
	return nil
}

// Stop gracefully stops all log tailing goroutines and waits for them to finish.
func (i *Ingestor) Stop() {
	if i.cancel != nil {
		i.cancel()
	}
	// Wait for all tail goroutines to finish
	i.wg.Wait()
}

// importHistoricalLogs reads existing log files including rotated ones
func (i *Ingestor) importHistoricalLogs(ctx context.Context, basePath string) error {
	// Find all related log files (base + rotated versions)
	dir := filepath.Dir(basePath)
	base := filepath.Base(basePath)

	files, err := filepath.Glob(filepath.Join(dir, base+"*"))
	if err != nil {
		return err
	}

	// Sort files so we process them in order (oldest first)
	sort.Strings(files)

	for _, file := range files {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		slog.Info("importing historical log", "file", file)
		count, err := i.importLogFile(ctx, file)
		if err != nil {
			slog.Error("failed to import log file", "file", file, "error", err)
			continue
		}
		slog.Info("imported log file", "file", file, "entries", count)
	}

	return nil
}

// importLogFile reads a single log file (plain or gzipped)
func (i *Ingestor) importLogFile(ctx context.Context, path string) (int, error) {
	// Get file info for progress tracking
	fileInfo, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	fileSize := fileInfo.Size()
	fileMtime := fileInfo.ModTime().Unix()

	// Check if we've already imported this file
	progress, err := i.store.GetImportProgress(ctx, path)
	if err != nil {
		return 0, err
	}

	// For gzipped files, we can only check if fully imported (offset == size)
	// since we can't seek within compressed content
	isGzipped := strings.HasSuffix(path, ".gz")

	// If file hasn't changed and we've fully imported it, skip
	if progress != nil && progress.FileMtime == fileMtime && progress.FileSize == fileSize {
		if isGzipped && progress.ByteOffset == fileSize {
			slog.Debug("skipping already imported gzipped file", "path", path)
			return 0, nil
		} else if !isGzipped && progress.ByteOffset >= fileSize {
			slog.Debug("skipping already imported file", "path", path)
			return 0, nil
		}
	}

	// If the file was modified (different mtime or size), re-import from start
	// This handles log rotation where the file is replaced
	startOffset := int64(0)
	if progress != nil && progress.FileMtime == fileMtime && progress.FileSize == fileSize && !isGzipped {
		startOffset = progress.ByteOffset
	}

	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	var reader io.Reader = f
	var currentOffset int64 = 0

	// Handle gzipped files
	if isGzipped {
		gzReader, err := gzip.NewReader(f)
		if err != nil {
			return 0, err
		}
		defer gzReader.Close()
		reader = gzReader
	} else if startOffset > 0 {
		// Seek to last known position for plain files
		if _, err := f.Seek(startOffset, 0); err != nil {
			return 0, err
		}
		currentOffset = startOffset
		slog.Debug("resuming import from offset", "path", path, "offset", startOffset)
	}

	scanner := bufio.NewScanner(reader)
	// Increase buffer size for potentially long lines
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	count := 0
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return count, ctx.Err()
		default:
		}

		line := scanner.Text()
		lineLen := int64(len(line)) + 1 // +1 for newline
		currentOffset += lineLen

		if line == "" {
			continue
		}

		// Use handleLineNoNotify to avoid spamming SSE during import
		if err := i.handleLineNoNotify(ctx, line); err != nil {
			// Skip malformed lines silently during import
			continue
		}
		count++

		// Log progress and save checkpoint every 10000 entries
		if count%10000 == 0 {
			slog.Debug("import progress", "file", filepath.Base(path), "entries", count)
			// Save progress periodically
			_ = i.store.SetImportProgress(ctx, storage.ImportProgress{
				FilePath:   path,
				ByteOffset: currentOffset,
				FileSize:   fileSize,
				FileMtime:  fileMtime,
			})
		}
	}

	if err := scanner.Err(); err != nil {
		return count, err
	}

	// Save final progress - for gzipped files, use fileSize as offset to mark complete
	finalOffset := currentOffset
	if isGzipped {
		finalOffset = fileSize
	}
	if err := i.store.SetImportProgress(ctx, storage.ImportProgress{
		FilePath:   path,
		ByteOffset: finalOffset,
		FileSize:   fileSize,
		FileMtime:  fileMtime,
	}); err != nil {
		slog.Warn("failed to save import progress", "path", path, "error", err)
	}

	return count, nil
}

// handleLineNoNotify processes a log line without sending SSE notifications
func (i *Ingestor) handleLineNoNotify(ctx context.Context, line string) error {
	entry, err := parseCaddyLog(line)
	if err != nil {
		return err
	}
	ip := normalizeIP(entry.RemoteAddr)
	if i.cfg.PrivacyAnonymizeOctet {
		ip = anonymizeIP(ip)
	}
	if i.cfg.PrivacyHashIPs {
		ip = hashIP(ip, i.cfg.PrivacyHashSalt)
	}

	var country, region, city string
	if i.geo != nil {
		country, region, city = i.geo.Lookup(ip)
	}

	// Parse user-agent
	ua := useragent.Parse(entry.UserAgent)

	record := storage.RequestRecord{
		Timestamp:      entry.Timestamp,
		Host:           entry.Host,
		Path:           entry.Path,
		Status:         entry.Status,
		Bytes:          entry.Bytes,
		IP:             ip,
		Referrer:       entry.Referrer,
		UserAgent:      entry.UserAgent,
		ResponseTime:   entry.DurationMs,
		Country:        country,
		Region:         region,
		City:           city,
		Browser:        ua.Browser,
		BrowserVersion: ua.BrowserVersion,
		OS:             ua.OS,
		OSVersion:      ua.OSVersion,
		DeviceType:     ua.DeviceType,
		IsBot:          ua.IsBot,
		BotName:        ua.BotName,
	}
	return i.store.InsertRequest(ctx, record)
}

func (i *Ingestor) tailFile(ctx context.Context, path string) {
	t, err := tail.TailFile(path, tail.Config{
		ReOpen:    true,
		Follow:    true,
		Logger:    tail.DiscardingLogger,
		MustExist: false,
		Location:  &tail.SeekInfo{Offset: 0, Whence: 2},
	})
	if err != nil {
		slog.Error("failed to tail log file", "path", path, "error", err)
		return
	}
	slog.Info("tailing log file", "path", path)
	for {
		select {
		case <-ctx.Done():
			_ = t.Stop()
			return
		case line := <-t.Lines:
			if line == nil {
				continue
			}
			if err := i.handleLine(ctx, line.Text); err != nil {
				slog.Debug("failed to parse log line", "path", path, "error", err)
			}
		}
	}
}

func (i *Ingestor) handleLine(ctx context.Context, line string) error {
	start := time.Now()

	entry, err := parseCaddyLog(line)
	if err != nil {
		if i.metrics != nil {
			i.metrics.RecordIngestError()
		}
		return err
	}
	ip := normalizeIP(entry.RemoteAddr)
	if i.cfg.PrivacyAnonymizeOctet {
		ip = anonymizeIP(ip)
	}
	if i.cfg.PrivacyHashIPs {
		ip = hashIP(ip, i.cfg.PrivacyHashSalt)
	}

	var country, region, city string
	if i.geo != nil {
		country, region, city = i.geo.Lookup(ip)
	}

	// Parse user-agent
	ua := useragent.Parse(entry.UserAgent)

	record := storage.RequestRecord{
		Timestamp:      entry.Timestamp,
		Host:           entry.Host,
		Path:           entry.Path,
		Status:         entry.Status,
		Bytes:          entry.Bytes,
		IP:             ip,
		Referrer:       entry.Referrer,
		UserAgent:      entry.UserAgent,
		ResponseTime:   entry.DurationMs,
		Country:        country,
		Region:         region,
		City:           city,
		Browser:        ua.Browser,
		BrowserVersion: ua.BrowserVersion,
		OS:             ua.OS,
		OSVersion:      ua.OSVersion,
		DeviceType:     ua.DeviceType,
		IsBot:          ua.IsBot,
		BotName:        ua.BotName,
	}
	if err := i.store.InsertRequest(ctx, record); err != nil {
		return err
	}

	// Record metrics
	if i.metrics != nil {
		i.metrics.RecordIngest(time.Since(start).Seconds(), record.Bytes)
		if !record.Timestamp.IsZero() {
			i.metrics.SetLastIngestTimestamp(float64(record.Timestamp.Unix()))
		}
	}

	if i.hub != nil {
		// Broadcast the new request for live request log
		reqEvent := storage.RecentRequest{
			Timestamp:      record.Timestamp,
			Host:           record.Host,
			Path:           record.Path,
			Status:         record.Status,
			Bytes:          record.Bytes,
			IP:             record.IP,
			Referrer:       record.Referrer,
			UserAgent:      record.UserAgent,
			ResponseTime:   record.ResponseTime,
			Country:        record.Country,
			Region:         record.Region,
			City:           record.City,
			Browser:        record.Browser,
			BrowserVersion: record.BrowserVersion,
			OS:             record.OS,
			OSVersion:      record.OSVersion,
			DeviceType:     record.DeviceType,
			IsBot:          record.IsBot,
			BotName:        record.BotName,
		}
		if buf, err := json.Marshal(reqEvent); err == nil {
			i.hub.BroadcastEvent("request", buf)
		}

		// Also broadcast the summary update
		if summary, err := i.store.Summary(ctx, time.Duration(i.cfg.RawRetentionHours)*time.Hour, ""); err == nil {
			if buf, err := json.Marshal(summary); err == nil {
				i.hub.Broadcast(buf)
			}
		}
	}
	return nil
}

type GeoLookup struct {
	db    *maxminddb.Reader
	cache *GeoCache
}

// GeoLookupConfig holds configuration for GeoLookup.
type GeoLookupConfig struct {
	// Path to the MaxMind database file.
	DBPath string

	// Cache configuration. If nil, default configuration is used.
	CacheConfig *GeoCacheConfig
}

// NewGeo creates a new GeoLookup with an LRU cache.
// If path is empty, returns nil (no geo lookups will be performed).
func NewGeo(path string) (*GeoLookup, error) {
	return NewGeoWithConfig(GeoLookupConfig{DBPath: path})
}

// NewGeoWithConfig creates a new GeoLookup with custom cache configuration.
func NewGeoWithConfig(cfg GeoLookupConfig) (*GeoLookup, error) {
	if cfg.DBPath == "" {
		return nil, nil
	}
	db, err := maxminddb.Open(cfg.DBPath)
	if err != nil {
		return nil, err
	}

	// Use provided cache config or defaults
	var cacheConfig GeoCacheConfig
	if cfg.CacheConfig != nil {
		cacheConfig = *cfg.CacheConfig
	} else {
		cacheConfig = DefaultGeoCacheConfig()
	}

	return &GeoLookup{
		db:    db,
		cache: NewGeoCache(cacheConfig),
	}, nil
}

// Close closes the MaxMind database reader.
func (g *GeoLookup) Close() error {
	if g == nil || g.db == nil {
		return nil
	}
	return g.db.Close()
}

// Lookup returns the country, region, and city for the given IP address.
// Results are cached to improve performance for repeated lookups.
func (g *GeoLookup) Lookup(ip string) (string, string, string) {
	if g == nil || g.db == nil || ip == "" {
		return "", "", ""
	}

	// Check cache first
	if g.cache != nil {
		if result, ok := g.cache.Get(ip); ok {
			return result.Country, result.Region, result.City
		}
	}

	// Cache miss - do the actual lookup
	parsed := net.ParseIP(ip)
	if parsed == nil {
		// Cache empty result for invalid IPs to avoid repeated parsing attempts
		if g.cache != nil {
			g.cache.Set(ip, GeoResult{})
		}
		return "", "", ""
	}
	var record struct {
		Country struct {
			Names map[string]string `maxminddb:"names"`
			ISO   string            `maxminddb:"iso_code"`
		} `maxminddb:"country"`
		Subdivisions []struct {
			Names map[string]string `maxminddb:"names"`
		} `maxminddb:"subdivisions"`
		City struct {
			Names map[string]string `maxminddb:"names"`
		} `maxminddb:"city"`
	}
	if err := g.db.Lookup(parsed, &record); err != nil {
		// Cache empty result for failed lookups (e.g., IP not in database)
		if g.cache != nil {
			g.cache.Set(ip, GeoResult{})
		}
		return "", "", ""
	}
	country := record.Country.ISO
	region := ""
	if len(record.Subdivisions) > 0 {
		region = record.Subdivisions[0].Names["en"]
	}
	city := record.City.Names["en"]

	// Store result in cache
	if g.cache != nil {
		g.cache.Set(ip, GeoResult{
			Country: country,
			Region:  region,
			City:    city,
		})
	}

	return country, region, city
}

// CacheStats returns statistics about the geo cache.
// Returns nil if no cache is configured.
func (g *GeoLookup) CacheStats() *GeoCacheStats {
	if g == nil || g.cache == nil {
		return nil
	}
	stats := g.cache.Stats()
	return &stats
}

type caddyLogEntry struct {
	Timestamp   json.RawMessage `json:"ts"` // Can be float64 or string (RFC3339)
	Request     struct {
		Host       string              `json:"host"`
		URI        string              `json:"uri"`
		RemoteIP   string              `json:"remote_ip"`
		RemotePort string              `json:"remote_port"`
		ClientIP   string              `json:"client_ip"`
		Headers    map[string][]string `json:"headers"`
	} `json:"request"`
	Status      int                 `json:"status"`
	Bytes       int64               `json:"bytes_written"`
	Size        int64               `json:"size"`
	Duration    float64             `json:"duration"`
	UserID      string              `json:"user_id"`
	RespHeaders map[string][]string `json:"resp_headers"`
}

type parsedEntry struct {
	Timestamp  time.Time
	Host       string
	Path       string
	Status     int
	Bytes      int64
	RemoteAddr string
	Referrer   string
	UserAgent  string
	DurationMs float64
}

func parseCaddyLog(line string) (parsedEntry, error) {
	var raw caddyLogEntry
	if err := json.Unmarshal([]byte(line), &raw); err != nil {
		return parsedEntry{}, err
	}

	// Parse timestamp - can be either float64 (Unix) or string (RFC3339)
	var ts time.Time
	if len(raw.Timestamp) > 0 {
		// Try parsing as float64 first (Unix timestamp)
		var tsFloat float64
		if err := json.Unmarshal(raw.Timestamp, &tsFloat); err == nil {
			ts = time.Unix(int64(tsFloat), int64((tsFloat-float64(int64(tsFloat)))*1e9)).UTC()
		} else {
			// Try parsing as string (RFC3339)
			var tsStr string
			if err := json.Unmarshal(raw.Timestamp, &tsStr); err == nil {
				if parsed, err := time.Parse(time.RFC3339Nano, tsStr); err == nil {
					ts = parsed.UTC()
				} else if parsed, err := time.Parse(time.RFC3339, tsStr); err == nil {
					ts = parsed.UTC()
				}
			}
		}
	}

	bytes := raw.Bytes
	if bytes == 0 && raw.Size > 0 {
		bytes = raw.Size
	}
	ref := firstHeader(raw.Request.Headers, "Referer")
	if ref == "" {
		ref = firstHeader(raw.Request.Headers, "Referrer")
	}
	ua := firstHeader(raw.Request.Headers, "User-Agent")

	// Get client IP - prefer real client IP from proxy headers, fall back to direct IP
	clientIP := firstHeader(raw.Request.Headers, "Cf-Connecting-Ip") // Cloudflare
	if clientIP == "" {
		clientIP = firstHeader(raw.Request.Headers, "X-Real-Ip") // nginx proxy
	}
	if clientIP == "" {
		clientIP = firstHeader(raw.Request.Headers, "X-Forwarded-For") // general proxy
		// X-Forwarded-For can be comma-separated list, take first
		if idx := strings.Index(clientIP, ","); idx != -1 {
			clientIP = strings.TrimSpace(clientIP[:idx])
		}
	}
	if clientIP == "" {
		clientIP = raw.Request.ClientIP // Caddy's client_ip field
	}
	if clientIP == "" {
		clientIP = raw.Request.RemoteIP // direct connection IP
	}

	return parsedEntry{
		Timestamp:  ts,
		Host:       raw.Request.Host,
		Path:       raw.Request.URI,
		Status:     raw.Status,
		Bytes:      bytes,
		RemoteAddr: clientIP,
		Referrer:   ref,
		UserAgent:  ua,
		DurationMs: raw.Duration * 1000,
	}, nil
}

func firstHeader(h map[string][]string, key string) string {
	if len(h) == 0 {
		return ""
	}
	// Try exact match first (most common case)
	if vals, ok := h[key]; ok && len(vals) > 0 {
		return vals[0]
	}
	// Fall back to case-insensitive search
	keyLower := strings.ToLower(key)
	for k, vals := range h {
		if strings.ToLower(k) == keyLower && len(vals) > 0 {
			return vals[0]
		}
	}
	return ""
}

func normalizeIP(remoteAddr string) string {
	if remoteAddr == "" {
		return ""
	}
	if strings.Contains(remoteAddr, ":") {
		host, _, err := net.SplitHostPort(remoteAddr)
		if err == nil {
			return host
		}
	}
	return remoteAddr
}

func anonymizeIP(ip string) string {
	if ip == "" {
		return ""
	}
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return ""
	}
	if v4 := parsed.To4(); v4 != nil {
		v4[3] = 0
		return v4.String()
	}
	parts := strings.Split(ip, ":")
	if len(parts) > 0 {
		parts[len(parts)-1] = "0"
		return strings.Join(parts, ":")
	}
	return ip
}

func hashIP(ip, salt string) string {
	if ip == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(salt + ip))
	return hex.EncodeToString(sum[:])
}
