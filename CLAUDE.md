# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Caddystat is a minimal stats and live dashboard for Caddy web server access logs. It tails Caddy's JSON access logs, stores metrics in SQLite, and provides a real-time web dashboard via SSE.

## Common Commands

```bash
# Run locally (Go)
go run ./cmd/caddystat

# Development with Docker (includes test Caddy + nginx site)
./dev              # Start in foreground
./dev up -d        # Start in background
./dev down         # Stop containers
./dev restart      # Restart containers
./dev rebuild      # Full rebuild (--no-cache)
./dev logs         # Follow logs
./dev shell        # Shell into caddystat container

# Frontend build (from web/ directory)
cd web && npm install && npm run build   # outputs to web/_site

# Frontend dev with hot reload (from web/ directory)
cd web && npm run dev

# Run tests
./test                    # Run all tests
./test --coverage         # Show coverage summary
./test --report           # Generate HTML coverage report and open in browser
./test --verbose --race   # Verbose output with race detection
./test --watch            # Re-run tests on file changes
./test ./internal/ingest  # Test specific package
```

## Environment Variables

- `LOG_PATH` - Comma-separated Caddy log paths (default: `./caddy.log`)
- `LISTEN_ADDR` - HTTP bind address (default: `:8404`)
- `DB_PATH` - SQLite database path (default: `./data/caddystat.db`)
- `DATA_RETENTION_DAYS` - Purge raw rows older than N days (default: `7`)
- `RAW_RETENTION_HOURS` - Window for realtime summaries (default: `48`)
- `MAXMIND_DB_PATH` - Optional path to GeoLite2-City.mmdb for geo lookups
- `AUTH_USERNAME` - Optional username for dashboard authentication
- `AUTH_PASSWORD` - Optional password for dashboard authentication (both must be set to enable auth)
- `RATE_LIMIT_PER_MINUTE` - Max requests per minute per IP (default: `0` = disabled)
- `MAX_REQUEST_BODY_BYTES` - Maximum request body size in bytes (default: `1048576` = 1MB)
- `DB_MAX_CONNECTIONS` - Maximum database connections (default: `1`)
- `DB_QUERY_TIMEOUT` - Query timeout duration (default: `30s`)

## Architecture

```
cmd/caddystat/main.go     Entry point, wires up components
internal/
├── config/               Environment-based configuration
├── ingest/               Log file tailing and parsing (supports gzip)
├── storage/              SQLite storage with hourly/daily rollups
├── sse/                  Server-sent events hub for live updates
└── useragent/            User-agent parsing for browser/OS/bot detection

web/                      Frontend (Alpine.js + Tailwind, built with PostCSS)
└── _site/                Built static files served by Go at /
```

**Data Flow:**
1. `ingest` tails Caddy JSON logs, imports historical logs (including rotated .gz files)
2. Each log entry is parsed, enriched with geo (if MaxMind configured), and stored in SQLite
3. `storage` maintains raw `requests` table plus `rollups_hourly` and `rollups_daily` for aggregates
4. `server` exposes REST API at `/api/stats/*` and SSE at `/api/sse`
5. New requests broadcast to SSE subscribers for real-time dashboard updates

**Key Implementation Details:**
- Uses pure-Go SQLite driver `modernc.org/sqlite` (no CGO)
- Log tailing via `github.com/hpcloud/tail` handles rotation
- Privacy controls: can hash IPs with salt and/or anonymize last IPv4 octet
- Import progress tracked in DB to resume after restarts

## API Endpoints

- `GET /api/stats/summary?range=24h&host=` - Dashboard summary stats
- `GET /api/stats/requests?range=24h` - Hourly time series
- `GET /api/stats/geo?range=24h` - Country/region/city counts
- `GET /api/stats/hosts` - Top hosts by request count
- `GET /api/stats/browsers` - Browser usage stats
- `GET /api/stats/os` - OS usage stats
- `GET /api/stats/robots` - Bot/spider stats
- `GET /api/stats/referrers` - Referrer stats
- `GET /api/stats/status` - System status (DB size, row counts, last import time)
- `GET /api/stats/monthly?months=12` - Monthly history
- `GET /api/stats/daily` - Current month daily breakdown
- `GET /api/stats/recent?limit=20` - Recent individual requests
- `GET /api/sse?host=&range=24h` - SSE stream for live updates
- `GET /api/auth/check` - Check authentication status
- `POST /api/auth/login` - Login with username/password
- `POST /api/auth/logout` - Logout and clear session
- `GET /health` - Health check (DB status, version)
- `GET /metrics` - Prometheus metrics endpoint
