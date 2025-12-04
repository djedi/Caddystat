package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dustin/Caddystat/internal/config"
	"github.com/dustin/Caddystat/internal/ingest"
	"github.com/dustin/Caddystat/internal/server"
	"github.com/dustin/Caddystat/internal/sse"
	"github.com/dustin/Caddystat/internal/storage"
	"github.com/dustin/Caddystat/internal/version"
)

func main() {
	versionFlag := flag.Bool("version", false, "Print version information and exit")
	flag.Parse()

	if *versionFlag {
		fmt.Printf("caddystat %s (commit: %s, built: %s)\n", version.Version, version.GitCommit, version.BuildTime)
		os.Exit(0)
	}

	cfg := config.Load()

	store, err := storage.New(cfg.DBPath)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer store.Close()

	geo, err := ingest.NewGeo(cfg.MaxMindDBPath)
	if err != nil {
		log.Printf("geo disabled: %v", err)
	}
	hub := sse.NewHub()
	ingestor := ingest.New(cfg, store, hub, geo)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := ingestor.Start(ctx); err != nil {
		log.Fatalf("ingestor: %v", err)
	}

	go func() {
		ticker := time.NewTicker(12 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := store.Cleanup(context.Background(), cfg.DataRetentionDays); err != nil {
					log.Printf("cleanup: %v", err)
				}
			}
		}
	}()

	srv := &http.Server{
		Addr:    cfg.ListenAddr,
		Handler: server.New(store, hub, cfg),
	}

	go func() {
		log.Printf("caddystat %s starting on %s", version.Version, cfg.ListenAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server: %v", err)
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelShutdown()
	_ = srv.Shutdown(shutdownCtx)
	log.Println("shutdown complete")
	os.Exit(0)
}
