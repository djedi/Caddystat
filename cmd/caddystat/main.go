package main

import (
	"context"
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
)

func main() {
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
		log.Printf("listening on %s", cfg.ListenAddr)
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
