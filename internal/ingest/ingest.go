package ingest

import (
	"bufio"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hpcloud/tail"
	"github.com/oschwald/maxminddb-golang"

	"github.com/dustin/Caddystat/internal/config"
	"github.com/dustin/Caddystat/internal/sse"
	"github.com/dustin/Caddystat/internal/storage"
	"github.com/dustin/Caddystat/internal/useragent"
)

type Ingestor struct {
	cfg   config.Config
	store *storage.Storage
	hub   *sse.Hub
	geo   *GeoLookup
}

func New(cfg config.Config, store *storage.Storage, hub *sse.Hub, geo *GeoLookup) *Ingestor {
	return &Ingestor{
		cfg:   cfg,
		store: store,
		hub:   hub,
		geo:   geo,
	}
}

func (i *Ingestor) Start(ctx context.Context) error {
	// First, import any existing log files (including rotated/gzipped ones)
	for _, path := range i.cfg.LogPaths {
		if err := i.importHistoricalLogs(ctx, path); err != nil {
			log.Printf("import historical logs for %s: %v", path, err)
		}
	}

	// Then start tailing for new entries
	for _, path := range i.cfg.LogPaths {
		go i.tailFile(ctx, path)
	}
	return nil
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

		log.Printf("importing historical log: %s", file)
		count, err := i.importLogFile(ctx, file)
		if err != nil {
			log.Printf("error importing %s: %v", file, err)
			continue
		}
		log.Printf("imported %d entries from %s", count, file)
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
			log.Printf("skipping already imported gzipped file: %s", path)
			return 0, nil
		} else if !isGzipped && progress.ByteOffset >= fileSize {
			log.Printf("skipping already imported file: %s", path)
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
		log.Printf("resuming import from offset %d: %s", startOffset, path)
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
			log.Printf("  ... imported %d entries from %s", count, filepath.Base(path))
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
		log.Printf("warning: failed to save import progress for %s: %v", path, err)
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
		log.Printf("tail %s: %v", path, err)
		return
	}
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
				log.Printf("parse %s: %v", path, err)
			}
		}
	}
}

func (i *Ingestor) handleLine(ctx context.Context, line string) error {
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
	if err := i.store.InsertRequest(ctx, record); err != nil {
		return err
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
	db *maxminddb.Reader
}

func NewGeo(path string) (*GeoLookup, error) {
	if path == "" {
		return nil, nil
	}
	db, err := maxminddb.Open(path)
	if err != nil {
		return nil, err
	}
	return &GeoLookup{db: db}, nil
}

func (g *GeoLookup) Lookup(ip string) (string, string, string) {
	if g == nil || g.db == nil || ip == "" {
		return "", "", ""
	}
	parsed := net.ParseIP(ip)
	if parsed == nil {
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
		return "", "", ""
	}
	country := record.Country.ISO
	region := ""
	if len(record.Subdivisions) > 0 {
		region = record.Subdivisions[0].Names["en"]
	}
	city := record.City.Names["en"]
	return country, region, city
}

type caddyLogEntry struct {
	Timestamp float64 `json:"ts"`
	Request   struct {
		Host       string              `json:"host"`
		URI        string              `json:"uri"`
		RemoteAddr string              `json:"remote_addr"`
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
	ts := time.Unix(int64(raw.Timestamp), int64((raw.Timestamp-float64(int64(raw.Timestamp)))*1e9)).UTC()

	bytes := raw.Bytes
	if bytes == 0 && raw.Size > 0 {
		bytes = raw.Size
	}
	ref := firstHeader(raw.Request.Headers, "Referer")
	if ref == "" {
		ref = firstHeader(raw.Request.Headers, "Referrer")
	}
	ua := firstHeader(raw.Request.Headers, "User-Agent")

	return parsedEntry{
		Timestamp:  ts,
		Host:       raw.Request.Host,
		Path:       raw.Request.URI,
		Status:     raw.Status,
		Bytes:      bytes,
		RemoteAddr: raw.Request.RemoteAddr,
		Referrer:   ref,
		UserAgent:  ua,
		DurationMs: raw.Duration * 1000,
	}, nil
}

func firstHeader(h map[string][]string, key string) string {
	if len(h) == 0 {
		return ""
	}
	if vals, ok := h[key]; ok && len(vals) > 0 {
		return vals[0]
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
