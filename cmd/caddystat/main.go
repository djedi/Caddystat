package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/dustin/Caddystat/internal/alerts"
	"github.com/dustin/Caddystat/internal/config"
	"github.com/dustin/Caddystat/internal/ingest"
	"github.com/dustin/Caddystat/internal/logging"
	"github.com/dustin/Caddystat/internal/metrics"
	"github.com/dustin/Caddystat/internal/server"
	"github.com/dustin/Caddystat/internal/sse"
	"github.com/dustin/Caddystat/internal/storage"
	"github.com/dustin/Caddystat/internal/useragent"
	"github.com/dustin/Caddystat/internal/version"
)

func main() {
	versionFlag := flag.Bool("version", false, "Print version information and exit")
	healthcheckFlag := flag.Bool("healthcheck", false, "Check health endpoint and exit with status")
	flag.Parse()

	if *versionFlag {
		fmt.Printf("caddystat %s (commit: %s, built: %s)\n", version.Version, version.GitCommit, version.BuildTime)
		os.Exit(0)
	}

	cfg := config.Load()

	// Initialize structured logging
	logging.Setup(cfg.LogLevel)

	if *healthcheckFlag {
		url := fmt.Sprintf("http://localhost%s/health", cfg.ListenAddr)
		resp, err := http.Get(url)
		if err != nil {
			fmt.Fprintf(os.Stderr, "health check failed: %v\n", err)
			os.Exit(1)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			fmt.Fprintf(os.Stderr, "health check failed: status %d\n", resp.StatusCode)
			os.Exit(1)
		}
		os.Exit(0)
	}

	// Load bot signatures if configured (supports multiple files for community lists)
	if len(cfg.BotSignaturesPaths) > 0 {
		if err := useragent.LoadBotSignaturesList(cfg.BotSignaturesPaths); err != nil {
			slog.Warn("failed to load bot signatures, using defaults", "paths", cfg.BotSignaturesPaths, "error", err)
		}
	}

	// Load alerting configuration
	alertCfg := alerts.LoadConfig()

	// Print startup banner
	printStartupBanner(cfg, alertCfg)

	store, err := storage.NewWithOptions(cfg.DBPath, storage.Options{
		MaxConnections: cfg.DBMaxConnections,
		QueryTimeout:   cfg.DBQueryTimeout,
	})
	if err != nil {
		slog.Error("failed to initialize database", "error", err)
		os.Exit(1)
	}
	defer store.Close()
	slog.Debug("database initialized", "path", cfg.DBPath, "max_connections", cfg.DBMaxConnections, "query_timeout", cfg.DBQueryTimeout)

	geo, err := ingest.NewGeo(cfg.MaxMindDBPath)
	if err != nil {
		slog.Warn("geo lookups disabled", "reason", err.Error())
	} else if geo != nil {
		slog.Info("geo lookups enabled", "db_path", cfg.MaxMindDBPath)
	} else {
		slog.Debug("geo lookups disabled", "reason", "MAXMIND_DB_PATH not set")
	}

	hub := sse.NewHub(sse.WithBufferSize(cfg.SSEBufferSize))

	// Initialize Prometheus metrics
	m := metrics.New(
		hub.ClientCount,
		func() int64 {
			dbPath := store.DBPath()
			if dbPath == "" {
				return 0
			}
			fi, err := os.Stat(dbPath)
			if err != nil {
				return 0
			}
			return fi.Size()
		},
		func() metrics.DBStats {
			stats, err := store.GetDatabaseStats(context.Background())
			if err != nil {
				return metrics.DBStats{}
			}
			return metrics.DBStats{
				RequestsCount:       stats.RequestsCount,
				SessionsCount:       stats.SessionsCount,
				RollupsHourlyCount:  stats.RollupsHourlyCount,
				RollupsDailyCount:   stats.RollupsDailyCount,
				ImportProgressCount: stats.ImportProgressCount,
			}
		},
		func() *metrics.GeoCacheStats {
			if geo == nil {
				return nil
			}
			stats := geo.CacheStats()
			if stats == nil {
				return nil
			}
			return &metrics.GeoCacheStats{
				Size:     stats.Size,
				Capacity: stats.Capacity,
				Hits:     stats.Hits,
				Misses:   stats.Misses,
				Evicts:   stats.Evicts,
				HitRate:  stats.HitRate,
			}
		},
	)
	if err := m.Register(); err != nil {
		slog.Warn("failed to register Prometheus metrics", "error", err)
	}

	// Wire up SSE dropped message counter after metrics creation
	hub.SetDroppedCounter(m)

	ingestor := ingest.New(cfg, store, hub, geo, m)

	// Initialize alerting system
	var alertManager *alerts.Manager
	if alertCfg.Enabled {
		alertStatsAdapter := storage.NewAlertStatsAdapter(store)
		alertManager = alerts.NewManager(alertCfg, alertStatsAdapter)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := ingestor.Start(ctx); err != nil {
		slog.Error("failed to start ingestor", "error", err)
		os.Exit(1)
	}

	// Start alerting if enabled
	if alertManager != nil {
		alertManager.Start(ctx)
	}

	go func() {
		dataTicker := time.NewTicker(12 * time.Hour)
		sessionTicker := time.NewTicker(1 * time.Hour)
		defer dataTicker.Stop()
		defer sessionTicker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-dataTicker.C:
				slog.Debug("running data cleanup", "default_retention_days", cfg.DataRetentionDays)
				result, err := store.CleanupWithPerSiteRetention(context.Background(), cfg.DataRetentionDays)
				if err != nil {
					slog.Warn("data cleanup failed", "error", err)
				} else {
					if result.TotalDeleted > 0 {
						slog.Info("data cleanup completed",
							"total_deleted", result.TotalDeleted,
							"global_deleted", result.GlobalDeleted,
							"sites_with_custom_retention", result.SitesProcessed,
						)
						for host, deleted := range result.PerSiteDeleted {
							slog.Debug("per-site cleanup", "host", host, "deleted", deleted)
						}
					} else {
						slog.Debug("data cleanup completed", "total_deleted", 0)
					}
					// Run VACUUM after cleanup to reclaim disk space
					slog.Debug("running database vacuum")
					if bytesFreed, err := store.Vacuum(context.Background()); err != nil {
						slog.Warn("database vacuum failed", "error", err)
					} else if bytesFreed > 0 {
						slog.Info("database vacuum completed", "bytes_freed", bytesFreed)
					} else {
						slog.Debug("database vacuum completed", "bytes_freed", 0)
					}
				}
			case <-sessionTicker.C:
				slog.Debug("running session cleanup")
				if deleted, err := store.CleanupExpiredSessions(context.Background()); err != nil {
					slog.Warn("session cleanup failed", "error", err)
				} else if deleted > 0 {
					slog.Debug("cleaned up expired sessions", "count", deleted)
				}
			}
		}
	}()

	srv := &http.Server{
		Addr:    cfg.ListenAddr,
		Handler: server.New(store, hub, cfg, m),
	}

	go func() {
		slog.Info("server started", "addr", cfg.ListenAddr, "version", version.Version)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server failed", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	slog.Info("received shutdown signal, starting graceful shutdown...")

	// Create a timeout context for the entire shutdown sequence
	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelShutdown()

	// 1. Stop accepting new SSE connections and close existing ones
	sseClients := hub.Close()
	slog.Debug("closed SSE connections", "clients", sseClients)

	// 2. Shutdown HTTP server (stops accepting new requests, waits for in-flight)
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Warn("HTTP server shutdown error", "error", err)
	} else {
		slog.Debug("HTTP server stopped")
	}

	// 3. Stop alerting system
	if alertManager != nil {
		alertManager.Stop()
		slog.Debug("stopped alerting")
	}

	// 4. Stop log tailing goroutines
	ingestor.Stop()
	slog.Debug("stopped log tailers")

	// 5. Close GeoIP database if configured
	if geo != nil {
		if err := geo.Close(); err != nil {
			slog.Warn("failed to close GeoIP database", "error", err)
		} else {
			slog.Debug("closed GeoIP database")
		}
	}

	// 6. Database is closed via defer store.Close() above

	slog.Info("shutdown complete")
}

func printStartupBanner(cfg config.Config, alertCfg alerts.Config) {
	fmt.Println()
	fmt.Println("  ╔═══════════════════════════════════════════════╗")
	fmt.Println("  ║              Caddystat                        ║")
	fmt.Println("  ║       Web Analytics for Caddy                 ║")
	fmt.Println("  ╚═══════════════════════════════════════════════╝")
	fmt.Println()
	fmt.Printf("  Version:        %s\n", version.Version)
	if version.GitCommit != "" && version.GitCommit != "unknown" {
		fmt.Printf("  Commit:         %s\n", version.GitCommit)
	}
	if version.BuildTime != "" && version.BuildTime != "unknown" {
		fmt.Printf("  Built:          %s\n", version.BuildTime)
	}
	fmt.Println()
	fmt.Printf("  Listen:         %s\n", cfg.ListenAddr)
	fmt.Printf("  Database:       %s\n", cfg.DBPath)
	fmt.Printf("  Log Paths:      %s\n", strings.Join(cfg.LogPaths, ", "))
	fmt.Printf("  Log Level:      %s\n", cfg.LogLevel.String())
	fmt.Printf("  Retention:      %d days\n", cfg.DataRetentionDays)
	if cfg.MaxMindDBPath != "" {
		fmt.Printf("  GeoIP:          %s\n", cfg.MaxMindDBPath)
	}
	if cfg.AuthEnabled() {
		fmt.Printf("  Auth:           enabled\n")
	}
	if cfg.RateLimitPerMinute > 0 {
		fmt.Printf("  Rate Limit:     %d req/min per IP\n", cfg.RateLimitPerMinute)
	}
	if cfg.MaxRequestBodyBytes > 0 {
		fmt.Printf("  Max Body Size:  %d bytes\n", cfg.MaxRequestBodyBytes)
	}
	if cfg.DBMaxConnections > 1 {
		fmt.Printf("  DB Connections: %d\n", cfg.DBMaxConnections)
	}
	if cfg.DBQueryTimeout != 30*time.Second {
		fmt.Printf("  Query Timeout:  %s\n", cfg.DBQueryTimeout)
	}
	if len(cfg.BotSignaturesPaths) > 0 {
		fmt.Printf("  Bot Signatures: %s\n", strings.Join(cfg.BotSignaturesPaths, ", "))
	}
	if alertCfg.Enabled {
		fmt.Printf("  Alerting:       enabled (%d rules, %d channels)\n", len(alertCfg.Rules), len(alertCfg.Channels))
	}
	fmt.Println()
}
