package server

import (
	"encoding/json"
	"log"
	"net/http"
	"path/filepath"
	"strconv"
	"time"

	"github.com/dustin/Caddystat/internal/sse"
	"github.com/dustin/Caddystat/internal/storage"
)

type Server struct {
	store *storage.Storage
	hub   *sse.Hub
	mux   *http.ServeMux
}

func New(store *storage.Storage, hub *sse.Hub) *Server {
	s := &Server{
		store: store,
		hub:   hub,
		mux:   http.NewServeMux(),
	}
	s.routes()
	return s
}

func (s *Server) routes() {
	s.mux.HandleFunc("/api/stats/summary", s.handleSummary)
	s.mux.HandleFunc("/api/stats/monthly", s.handleMonthly)
	s.mux.HandleFunc("/api/stats/daily", s.handleDaily)
	s.mux.HandleFunc("/api/stats/requests", s.handleRequests)
	s.mux.HandleFunc("/api/stats/geo", s.handleGeo)
	s.mux.HandleFunc("/api/stats/hosts", s.handleVisitors)
	s.mux.HandleFunc("/api/stats/browsers", s.handleBrowsers)
	s.mux.HandleFunc("/api/stats/os", s.handleOS)
	s.mux.HandleFunc("/api/stats/robots", s.handleRobots)
	s.mux.HandleFunc("/api/stats/referrers", s.handleReferrers)
	s.mux.HandleFunc("/api/sse", s.handleSSE)

	site := http.Dir(filepath.Join(".", "web", "_site"))
	s.mux.Handle("/", http.FileServer(site))
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) handleSummary(w http.ResponseWriter, r *http.Request) {
	dur := parseRange(r.URL.Query().Get("range"), 24*time.Hour)
	host := r.URL.Query().Get("host")
	stats, err := s.store.Summary(r.Context(), dur, host)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, stats)
}

func (s *Server) handleRequests(w http.ResponseWriter, r *http.Request) {
	dur := parseRange(r.URL.Query().Get("range"), 24*time.Hour)
	host := r.URL.Query().Get("host")
	stats, err := s.store.TimeSeriesRange(r.Context(), dur, host)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, stats)
}

func (s *Server) handleGeo(w http.ResponseWriter, r *http.Request) {
	dur := parseRange(r.URL.Query().Get("range"), 24*time.Hour)
	host := r.URL.Query().Get("host")
	stats, err := s.store.Geo(r.Context(), dur, host)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, stats)
}

func (s *Server) handleVisitors(w http.ResponseWriter, r *http.Request) {
	dur := parseRange(r.URL.Query().Get("range"), 24*time.Hour)
	host := r.URL.Query().Get("host")
	limit := 20
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 {
			limit = v
		}
	}
	stats, err := s.store.Visitors(r.Context(), dur, host, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, stats)
}

func (s *Server) handleBrowsers(w http.ResponseWriter, r *http.Request) {
	dur := parseRange(r.URL.Query().Get("range"), 24*time.Hour)
	host := r.URL.Query().Get("host")
	limit := 10
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 {
			limit = v
		}
	}
	stats, err := s.store.Browsers(r.Context(), dur, host, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, stats)
}

func (s *Server) handleOS(w http.ResponseWriter, r *http.Request) {
	dur := parseRange(r.URL.Query().Get("range"), 24*time.Hour)
	host := r.URL.Query().Get("host")
	limit := 10
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 {
			limit = v
		}
	}
	stats, err := s.store.OperatingSystems(r.Context(), dur, host, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, stats)
}

func (s *Server) handleRobots(w http.ResponseWriter, r *http.Request) {
	dur := parseRange(r.URL.Query().Get("range"), 24*time.Hour)
	host := r.URL.Query().Get("host")
	limit := 20
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 {
			limit = v
		}
	}
	stats, err := s.store.Robots(r.Context(), dur, host, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, stats)
}

func (s *Server) handleReferrers(w http.ResponseWriter, r *http.Request) {
	dur := parseRange(r.URL.Query().Get("range"), 24*time.Hour)
	host := r.URL.Query().Get("host")
	limit := 20
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 {
			limit = v
		}
	}
	stats, err := s.store.Referrers(r.Context(), dur, host, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, stats)
}

func (s *Server) handleMonthly(w http.ResponseWriter, r *http.Request) {
	host := r.URL.Query().Get("host")
	months := 12
	if m := r.URL.Query().Get("months"); m != "" {
		if v, err := strconv.Atoi(m); err == nil && v > 0 {
			months = v
		}
	}
	stats, err := s.store.MonthlyHistory(r.Context(), months, host)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, stats)
}

func (s *Server) handleDaily(w http.ResponseWriter, r *http.Request) {
	host := r.URL.Query().Get("host")
	stats, err := s.store.DailyHistory(r.Context(), host)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, stats)
}

func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	host := r.URL.Query().Get("host")
	dur := parseRange(r.URL.Query().Get("range"), 24*time.Hour)

	ch, cancel := s.hub.Subscribe()
	defer cancel()

	// Send an initial snapshot.
	if summary, err := s.store.Summary(r.Context(), dur, host); err == nil {
		if buf, err := json.Marshal(summary); err == nil {
			writeSSE(w, buf)
			flusher.Flush()
		}
	}

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ch:
			// Re-fetch with host filter on each update
			if summary, err := s.store.Summary(r.Context(), dur, host); err == nil {
				if buf, err := json.Marshal(summary); err == nil {
					writeSSE(w, buf)
					flusher.Flush()
				}
			}
		}
	}
}

func writeSSE(w http.ResponseWriter, payload []byte) {
	_, _ = w.Write([]byte("data: "))
	_, _ = w.Write(payload)
	_, _ = w.Write([]byte("\n\n"))
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("write json: %v", err)
	}
}

func parseRange(val string, def time.Duration) time.Duration {
	if val == "" {
		return def
	}
	if d, err := time.ParseDuration(val); err == nil {
		return d
	}
	return def
}
