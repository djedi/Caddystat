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

	"github.com/dustin/Caddystat/internal/config"
	"github.com/dustin/Caddystat/internal/ingest"
	"github.com/dustin/Caddystat/internal/logging"
	"github.com/dustin/Caddystat/internal/server"
	"github.com/dustin/Caddystat/internal/sse"
	"github.com/dustin/Caddystat/internal/storage"
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

	// Print startup banner
	printStartupBanner(cfg)

	store, err := storage.New(cfg.DBPath)
	if err != nil {
		slog.Error("failed to initialize database", "error", err)
		os.Exit(1)
	}
	defer store.Close()
	slog.Debug("database initialized", "path", cfg.DBPath)

	geo, err := ingest.NewGeo(cfg.MaxMindDBPath)
	if err != nil {
		slog.Warn("geo lookups disabled", "reason", err.Error())
	} else if geo != nil {
		slog.Info("geo lookups enabled", "db_path", cfg.MaxMindDBPath)
	} else {
		slog.Debug("geo lookups disabled", "reason", "MAXMIND_DB_PATH not set")
	}

	hub := sse.NewHub()
	ingestor := ingest.New(cfg, store, hub, geo)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := ingestor.Start(ctx); err != nil {
		slog.Error("failed to start ingestor", "error", err)
		os.Exit(1)
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
				slog.Debug("running data cleanup", "retention_days", cfg.DataRetentionDays)
				if err := store.Cleanup(context.Background(), cfg.DataRetentionDays); err != nil {
					slog.Warn("data cleanup failed", "error", err)
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
		Handler: server.New(store, hub, cfg),
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

	// 3. Stop log tailing goroutines
	ingestor.Stop()
	slog.Debug("stopped log tailers")

	// 4. Close GeoIP database if configured
	if geo != nil {
		if err := geo.Close(); err != nil {
			slog.Warn("failed to close GeoIP database", "error", err)
		} else {
			slog.Debug("closed GeoIP database")
		}
	}

	// 5. Database is closed via defer store.Close() above

	slog.Info("shutdown complete")
}

func printStartupBanner(cfg config.Config) {
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
	fmt.Println()
}
